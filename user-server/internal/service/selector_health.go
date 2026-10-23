package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
)

// B1-a：桥接选择器失效检测（服务端判定面）。
//
// 分工（这是本模块最重要的一条设计）：
//   · **扩展侧只上报事实**：某个渠道的某个选择器字段「试了几次、命中几次」。
//     它不做判断——扩展不知道「平台改版」和「今天没客户」的区别，让它判阈值只会
//     在各版本客户端里出现 N 套互相矛盾的规则，且改一次阈值要发一次扩展版本。
//   · **服务端做判定**：阈值走参数中心（`bridge.selector_health_degraded_below`），
//     改一次即时生效，不需要用户重装扩展。
//
// 为什么计「尝试次数」而不是只计命中：只报命中数时，一个从来不被调用的字段
// （比如个人版 WhatsApp 不支持的模板路径）会显示「0 命中」，与「选择器坏了」同形；
// 有分母才能区分「命中率低」（选择器失效）与「样本少」（这段路径没跑过）。

// ── 判据常量与阈值 ──────────────────────────────────────────────────────────

// SelectorHealthDegradedBelow 命中率低于该百分比即判 degraded 的**代码兜底**。
// 生效值由参数中心 `bridge.selector_health_degraded_below` 决定（改完即时生效）；
// 这个常量只在库/参数服务不可用时生效，且必须与种子 DefaultValue 一致
// （判据见 scripts/check-config-param-defaults.py）。
const SelectorHealthDegradedBelow = 60

const selectorHealthDegradedParam = "selector_health_degraded_below"

// SelectorHealthMinAttempts 判定所需的**最小样本**。少于这个数的「0 命中」只说明
// 这段路径刚起来（例如用户刚打开页面），判 broken 会让运营面在每次打开扩展时红一下。
const SelectorHealthMinAttempts = 20

// SelectorHealthMaxItemsPerReport 单次上报允许的字段数上限（防一行请求撑爆写入）。
const SelectorHealthMaxItemsPerReport = 32

// SelectorHealthMaxDeltaPerReport 单字段单次上报允许的增量上限。
//
// 扩展的计数是本地累计差额；扩展被浏览器休眠几天后醒来，一次上报的差额会是几十万。
// 这里不拒收（那样会丢真实数据），而是夹到这个上限——夹的含义是「一次上报最多算这么多」，
// 剩下的差额在扩展侧继续累计、下次再报。夹而不是截断丢弃的理由：命中率仍然会收敛到真值，
// 只是收敛慢一点；而丢掉分母会让命中率**偏高**，正好掩盖问题。
const SelectorHealthMaxDeltaPerReport = 100000

// selectorHealthFields 允许上报的字段名白名单。
//
// 与扩展侧 `user-web/bridge/src/core/selector-ai.js` 的 SELECTOR_FIELDS 同源。
// 白名单而不是自由文本：字段名由上报方决定时，一行坏数据就能让这张表按渠道无限长，
// 而运营面是按行渲染的。
var selectorHealthFields = map[string]struct{}{
	"conversationList": {},
	"messageList":      {},
	"messageItem":      {},
	"text":             {},
	"input":            {},
	"send":             {},
}

// selectorHealthChannelRe 渠道标识形状：小写字母数字 + `_`/`-`。
// 不枚举渠道——新增渠道是扩展侧的事，枚举会让新渠道的健康上报被 400 挡回，
// 而那种失败在扩展侧只表现为一行日志。
var selectorHealthChannelRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

// 状态取值。warming 不是「没问题」，而是「还看不出问题」——运营面上它必须与 ok 分色，
// 否则新装的扩展会一片绿，而它其实还没被验证过。
const (
	SelectorHealthStatusOK       = "ok"
	SelectorHealthStatusDegraded = "degraded"
	SelectorHealthStatusBroken   = "broken"
	SelectorHealthStatusWarming  = "warming"
)

// ErrSelectorHealthInvalidReport 上报内容不合形状（调用方应回 400，而不是静默吞掉）。
var ErrSelectorHealthInvalidReport = errors.New("invalid selector health report")

// ErrSelectorHealthRepoNotReady 仓储未注入（装配缺失）。
var ErrSelectorHealthRepoNotReady = errors.New("selector health repository not initialized")

// ── 类型 ────────────────────────────────────────────────────────────────────

// SelectorHealthObservation 扩展上报的单字段增量。
type SelectorHealthObservation struct {
	Field    string `json:"field"`
	Attempts uint64 `json:"attempts"`
	Hits     uint64 `json:"hits"`
}

// SelectorHealthReport 一次上报。
//
// AccountID 只用于日志与排障（哪一个浏览器实例在报），不落库：
// 同一个渠道的多台机器最终要合成一条渠道级健康度，按账号分行会让运营面看到
// 「同渠道 20 行，19 行绿 1 行红」而不知道平台到底改没改版。
type SelectorHealthReport struct {
	Channel   string                      `json:"channel"`
	AccountID string                      `json:"account_id"`
	Items     []SelectorHealthObservation `json:"items"`
}

// SelectorHealthView 运营面读数（服务端算出来的判定，不是原始计数）。
type SelectorHealthView struct {
	Channel      string  `json:"channel"`
	Field        string  `json:"field"`
	Attempts     uint64  `json:"attempts"`
	Hits         uint64  `json:"hits"`
	HitRate      float64 `json:"hit_rate"`       // 0–100，保留一位小数
	Status       string  `json:"status"`         // ok / degraded / broken / warming
	LastReportAt string  `json:"last_report_at"` // RFC3339；空表示从未上报
	StaleSeconds int64   `json:"stale_seconds"`  // 距最后一次上报的秒数，运营面据此判「这条读数是历史还是现在」
}

// SelectorHealthService 选择器健康服务。
type SelectorHealthService struct {
	repo repository.BridgeSelectorHealthRepository
	// now 可注入，便于用例构造「陈旧读数」这类时间相关场景。
	now func() time.Time
}

// NewSelectorHealthService 构造服务。repo 为 nil 时返回的服务仍可用（Ingest 会报错），
// 这样装配点少一处 nil 判空，而失败是显式的一次错误而不是 panic。
func NewSelectorHealthService(repo repository.BridgeSelectorHealthRepository) *SelectorHealthService {
	return &SelectorHealthService{repo: repo, now: time.Now}
}

// Threshold 生效的降级阈值（百分比）。非正数或超过 100 一律回落兜底——
// 阈值 0 会让所有行恒 degraded（门恒红等于没有门），超过 100 则永远不会降级。
func (s *SelectorHealthService) Threshold(ctx context.Context) int {
	v := GlobalConfigParam().GetInt(ctx, "bridge", selectorHealthDegradedParam, SelectorHealthDegradedBelow)
	if v <= 0 || v > 100 {
		return SelectorHealthDegradedBelow
	}
	return v
}

// Ingest 校验 + 累加一次上报，返回**判定后**的读数（供扩展侧日志/自检用）。
func (s *SelectorHealthService) Ingest(ctx context.Context, rep SelectorHealthReport) ([]SelectorHealthView, error) {
	if s.repo == nil {
		return nil, ErrSelectorHealthRepoNotReady
	}
	if !selectorHealthChannelRe.MatchString(rep.Channel) {
		return nil, fmt.Errorf("%w: channel %q 形状非法（小写字母数字 + _/-，≤32）",
			ErrSelectorHealthInvalidReport, rep.Channel)
	}
	if len(rep.Items) == 0 {
		return nil, fmt.Errorf("%w: items 为空（空上报不产生任何信息，只会掩盖「扩展侧没采集」）",
			ErrSelectorHealthInvalidReport)
	}
	if len(rep.Items) > SelectorHealthMaxItemsPerReport {
		return nil, fmt.Errorf("%w: items %d 超过上限 %d",
			ErrSelectorHealthInvalidReport, len(rep.Items), SelectorHealthMaxItemsPerReport)
	}

	now := s.now()
	deltas := make([]repository.SelectorHealthDelta, 0, len(rep.Items))
	seen := make(map[string]struct{}, len(rep.Items))
	for _, it := range rep.Items {
		if _, ok := selectorHealthFields[it.Field]; !ok {
			return nil, fmt.Errorf("%w: field %q 不在白名单（conversationList/messageList/messageItem/text/input/send）",
				ErrSelectorHealthInvalidReport, it.Field)
		}
		if _, dup := seen[it.Field]; dup {
			return nil, fmt.Errorf("%w: field %q 在同一次上报里出现两次", ErrSelectorHealthInvalidReport, it.Field)
		}
		seen[it.Field] = struct{}{}
		// Hits > Attempts 是上报侧的 bug（计数口径不一致）。不静默夹平：夹平等于
		// 用「看起来正常」的数据掩盖一个会让命中率失真的缺陷。
		if it.Hits > it.Attempts {
			return nil, fmt.Errorf("%w: field %q 的 hits(%d) > attempts(%d)",
				ErrSelectorHealthInvalidReport, it.Field, it.Hits, it.Attempts)
		}
		deltas = append(deltas, repository.SelectorHealthDelta{
			Channel:    rep.Channel,
			Field:      it.Field,
			Attempts:   clampDelta(it.Attempts),
			Hits:       clampDelta(it.Hits),
			ReportedAt: now,
		})
	}
	if err := s.repo.Upsert(ctx, deltas); err != nil {
		return nil, err
	}
	// 返回当前渠道的判定结果：扩展侧只在自检/排障时看它，判定口径始终只有这一处。
	rows, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	return s.views(ctx, rows, rep.Channel), nil
}

// List 运营面读数（全渠道）。
func (s *SelectorHealthService) List(ctx context.Context) ([]SelectorHealthView, error) {
	if s.repo == nil {
		return nil, ErrSelectorHealthRepoNotReady
	}
	rows, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	return s.views(ctx, rows, ""), nil
}

// views 把计数行转成判定读数。channel 为空＝不过滤。
func (s *SelectorHealthService) views(ctx context.Context, rows []model.BridgeSelectorHealth, channel string) []SelectorHealthView {
	threshold := s.Threshold(ctx)
	now := s.now()
	out := make([]SelectorHealthView, 0, len(rows))
	for _, r := range rows {
		if channel != "" && r.Channel != channel {
			continue
		}
		out = append(out, SelectorHealthView{
			Channel:      r.Channel,
			Field:        r.Field,
			Attempts:     r.Attempts,
			Hits:         r.Hits,
			HitRate:      selectorHitRate(r.Attempts, r.Hits),
			Status:       ClassifySelectorHealth(r.Attempts, r.Hits, threshold),
			LastReportAt: formatSelectorHealthTime(r.LastReportAt),
			StaleSeconds: staleSeconds(now, r.LastReportAt),
		})
	}
	return out
}

// ClassifySelectorHealth 判定单条健康度。
//
// 顺序有意义：先看样本（warming 而不是 broken），再看「一次都没命中」（broken），
// 最后看命中率（degraded）。反过来先判命中率时，样本 1 次 0 命中的行会以 0% 直接
// 判 broken —— 那正是新装扩展第一次打开页面的常态。
func ClassifySelectorHealth(attempts, hits uint64, degradedBelow int) string {
	if attempts < SelectorHealthMinAttempts {
		return SelectorHealthStatusWarming
	}
	if hits == 0 {
		return SelectorHealthStatusBroken
	}
	if degradedBelow <= 0 || degradedBelow > 100 {
		degradedBelow = SelectorHealthDegradedBelow
	}
	if selectorHitRate(attempts, hits) < float64(degradedBelow) {
		return SelectorHealthStatusDegraded
	}
	return SelectorHealthStatusOK
}

// selectorHitRate 命中率（0–100，一位小数）。attempts 为 0 时返回 0 而不是 NaN。
func selectorHitRate(attempts, hits uint64) float64 {
	if attempts == 0 {
		return 0
	}
	rate := float64(hits) * 100 / float64(attempts)
	// 保留一位小数：运营面要的是「97.4%」而不是「97.39534883720929%」。
	return float64(int64(rate*10+0.5)) / 10
}

func clampDelta(v uint64) uint64 {
	if v > SelectorHealthMaxDeltaPerReport {
		return SelectorHealthMaxDeltaPerReport
	}
	return v
}

func formatSelectorHealthTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func staleSeconds(now, last time.Time) int64 {
	if last.IsZero() {
		return -1 // -1 ＝从未上报过（与「0 秒前刚报过」必须区分开）
	}
	d := now.Sub(last)
	if d < 0 {
		return 0 // 时钟回拨：宁可显示「刚报过」，也不显示一个负数秒
	}
	return int64(d / time.Second)
}
