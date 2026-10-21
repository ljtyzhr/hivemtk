package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/metrics"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"
)

// MetricSnapshot 当前指标快照
type MetricSnapshot struct {
	Name  string
	Value float64
}

// MetricProvider 指标取数接口，由 metrics 包 / 业务侧实现
type MetricProvider interface {
	// Snapshot 返回当前所有指标；checker 按规则 source 名称匹配
	Snapshot(ctx context.Context) ([]MetricSnapshot, error)
}

// AlertNotifier 告警通知器接口（与 ReachSender 解耦）
type AlertNotifier interface {
	Notify(ctx context.Context, rule *model.AlertRule, history *model.AlertHistory) error
}

// AlertChecker 告警规则检查器
//
// 设计要点（plan v3.1 §T8）：
//   - 定时扫描启用的规则，比对指标值与阈值
//   - 冷却期内不重复触发（last_triggered_at + cooldown_seconds）
//   - 触发即写 AlertHistory，记录快照
//   - 恢复检测：若历史中存在 firing 状态且当前未触发，则写恢复时间
//
// 不依赖外部 Alertmanager；通知由注入的 AlertNotifier 实现（邮件 / 钉钉 / webhook）。
type AlertChecker struct {
	ruleRepo repository.AlertRuleRepository
	histRepo repository.AlertHistoryRepository
	provider MetricProvider
	notifier AlertNotifier
	interval time.Duration
	stop     chan struct{}
	wg       sync.WaitGroup
}

// NewAlertCheckerWithRepos 构造（仓储由装配层注入）
func NewAlertCheckerWithRepos(
	provider MetricProvider,
	notifier AlertNotifier,
	interval time.Duration,
	ruleRepo repository.AlertRuleRepository,
	histRepo repository.AlertHistoryRepository,
) *AlertChecker {
	return &AlertChecker{
		ruleRepo: ruleRepo,
		histRepo: histRepo,
		provider: provider,
		notifier: notifier,
		interval: interval,
		stop:     make(chan struct{}),
	}
}

// NewAlertChecker 兼容旧装配点：绑定全局 DB 的仓储。
func NewAlertChecker(provider MetricProvider, notifier AlertNotifier, interval time.Duration) *AlertChecker {
	return NewAlertCheckerWithRepos(provider, notifier, interval,
		repository.NewAlertRuleRepository(), repository.NewAlertHistoryRepository())
}

// Start 启动后台检查器
func (c *AlertChecker) Start() {
	c.wg.Add(1)
	go c.loop()
	logger.Infof("[AlertChecker] 已启动，间隔 %v", c.interval)
}

// Stop 停止
func (c *AlertChecker) Stop() {
	close(c.stop)
	c.wg.Wait()
	logger.Info("[AlertChecker] 已停止")
}

func (c *AlertChecker) loop() {
	defer c.wg.Done()
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	// 平台级监控循环（specs/001-saas-multi-tenant T020）：后台 goroutine 起步，
	// 没有 HTTP 请求 ctx 可绑，而 alert_rules / alert_histories 都是租户表 ——
	// TENANT_MODE=on 下裸 ctx 会被隔离 callback fail-closed（生产日志实证：
	// "[AlertChecker] 拉取规则失败/首轮检查失败/周期检查失败"）。修法是 Fanout
	// （checkOnce 内）而不是 system scope：规则拉取/触发记账是**逐租户**的业务
	// 动作，system scope 会把规则读成跨租户 union、历史 Create 落列默认租户 1。
	// CheckOnce 虽导出但无 HTTP 调用方；其透传语义保留：ctx 已带租户时 checkOnce
	// 不扇出、只查该租户。
	ctx := context.Background()
	if _, err := c.checkOnce(ctx); err != nil {
		logger.Warnf("[AlertChecker] 首轮检查失败: %v", err)
	}
	for {
		select {
		case <-c.stop:
			return
		case <-ticker.C:
			if _, err := c.checkOnce(ctx); err != nil {
				logger.Warnf("[AlertChecker] 周期检查失败: %v", err)
			}
		}
	}
}

// CheckOnce 单次执行（暴露给定时任务/手动触发）。
// 透传语义：ctx 已绑定租户 ⇒ 只查该租户（不扇出）；无租户绑定 ⇒ on 模式经
// Fanout 逐活跃租户执行、off 模式退化单默认租户。
func (c *AlertChecker) CheckOnce(ctx context.Context) (int, error) {
	return c.checkOnce(ctx)
}

func (c *AlertChecker) checkOnce(ctx context.Context) (int, error) {
	if c.provider == nil {
		return 0, fmt.Errorf("MetricProvider 未注入")
	}
	// 指标快照是**进程全局量**（metricsProvider 忽略 ctx）：在扇出之外取一次、
	// 逐租户回调复用，避免 N 个租户重复采集同一份快照。
	snapshots, err := c.provider.Snapshot(ctx)
	if err != nil {
		logger.Error(err, "[AlertChecker] 拉取指标失败")
		return 0, err
	}
	snapMap := make(map[string]float64, len(snapshots))
	for _, s := range snapshots {
		snapMap[s.Name] = s.Value
	}

	// ctx 已带租户（请求路径/手动触发）：不扇出，单租户执行——ListEnabled/fire
	// 全链路在该 ctx 下自然只读写本租户。
	if _, ok := tenant.FromContext(ctx); ok {
		return c.checkOnceForTenant(ctx, snapMap)
	}

	// 无租户绑定：逐活跃租户各查各的规则、各写各的历史。fanout.go 只在
	// "全部租户失败"时返回错误；单租户失败由 Fanout 记 warn 日志后继续。
	total, err := tenant.Fanout(ctx, nil, func(tctx context.Context, _ uint) (int64, error) {
		n, err := c.checkOnceForTenant(tctx, snapMap)
		return int64(n), err
	})
	if err != nil {
		return 0, err
	}
	return int(total), nil
}

// checkOnceForTenant 在**单租户** ctx 下执行一轮规则检查（触发即写本租户历史）。
func (c *AlertChecker) checkOnceForTenant(ctx context.Context, snapMap map[string]float64) (int, error) {
	rules, err := c.ruleRepo.ListEnabled(ctx)
	if err != nil {
		logger.Error(err, "[AlertChecker] 拉取规则失败")
		return 0, err
	}
	if len(rules) == 0 {
		return 0, nil
	}

	fired := 0
	now := time.Now()
	for _, r := range rules {

		if r.LastTriggeredAt != nil && now.Sub(*r.LastTriggeredAt) < time.Duration(r.CooldownSeconds)*time.Second {
			continue
		}
		val, ok := snapMap[r.Source]
		if !ok {

			_ = c.tryResolve(ctx, r.ID)
			continue
		}
		if !evaluateOperator(r.Operator, val, r.Threshold) {
			_ = c.tryResolve(ctx, r.ID)
			continue
		}
		if err := c.fire(ctx, r, val, now); err != nil {
			logger.Error(err, "[AlertChecker] 触发告警失败: "+r.Name)
			continue
		}
		fired++
	}
	return fired, nil
}

func (c *AlertChecker) fire(ctx context.Context, rule *model.AlertRule, val float64, at time.Time) error {
	msg := fmt.Sprintf("[%s] %s: %s 当前值 %.4f %s 阈值 %.4f（窗口 %ds）",
		strings.ToUpper(string(rule.Severity)), rule.Source, rule.Name, val, rule.Operator, rule.Threshold, rule.WindowSeconds)
	h := &model.AlertHistory{
		RuleID:      rule.ID,
		RuleName:    rule.Name,
		Source:      rule.Source,
		Value:       val,
		Threshold:   rule.Threshold,
		Severity:    rule.Severity,
		Message:     msg,
		Status:      model.AlertHistoryFiring,
		Channels:    rule.Channels,
		TriggeredAt: at,
	}
	if err := c.histRepo.Create(ctx, h); err != nil {
		return fmt.Errorf("写入告警历史失败: %w", err)
	}
	if c.notifier != nil {
		notifyFn := func() {
			// 异步通知可能排在请求 ctx 取消之后：保留租户、脱离取消
			nctx := tenant.BackgroundWithTenant(ctx)
			if err := c.notifier.Notify(nctx, rule, h); err != nil {
				logger.Error(err, "[AlertChecker] 通知下发失败: "+rule.Name)
				h.NotifyResult = "notify_error: " + err.Error()
			} else {
				h.NotifyResult = "ok"
			}
			_ = updateNotifyResult(nctx, c.histRepo, h)
		}

		if d := GetAlertDispatcher(); d != nil {
			d.Dispatch("alertrule:"+rule.Name, notifyFn)
		} else {
			notifyFn()
		}
	}
	if err := c.ruleRepo.UpdateLastTriggered(ctx, rule.ID, at); err != nil {
		logger.Error(err, "[AlertChecker] 更新 last_triggered_at 失败")
	}
	logger.Warnf("[AlertChecker] TRIGGER %s | src=%s val=%.4f thr=%.4f", rule.Name, rule.Source, val, rule.Threshold)
	return nil
}

func (c *AlertChecker) tryResolve(ctx context.Context, ruleID uint) error {
	return c.histRepo.ResolveFiring(ctx, ruleID, time.Now())
}

func evaluateOperator(op string, val, threshold float64) bool {
	switch strings.ToLower(op) {
	case "gt":
		return val > threshold
	case "ge":
		return val >= threshold
	case "lt":
		return val < threshold
	case "le":
		return val <= threshold
	case "eq":
		return val == threshold
	case "ne":
		return val != threshold
	}
	return false
}

func updateNotifyResult(ctx context.Context, repo repository.AlertHistoryRepository, h *model.AlertHistory) error {
	return repo.UpdateNotify(ctx, h.ID, h.NotifyResult)
}

type metricsProvider struct{}

// NewMetricsAlertProvider 构造基于 metrics 包的 Provider
func NewMetricsAlertProvider() MetricProvider { return &metricsProvider{} }

func (p *metricsProvider) Snapshot(ctx context.Context) ([]MetricSnapshot, error) {
	_ = ctx
	snap := metrics.Snapshot()
	out := make([]MetricSnapshot, 0, len(snap))
	for name, v := range snap {
		out = append(out, MetricSnapshot{Name: name, Value: v})
	}
	return out, nil
}

type logNotifier struct{}

// NewLogAlertNotifier 构造
func NewLogAlertNotifier() AlertNotifier { return &logNotifier{} }

func (n *logNotifier) Notify(ctx context.Context, rule *model.AlertRule, h *model.AlertHistory) error {
	_ = ctx
	var chans []string
	_ = json.Unmarshal([]byte(rule.Channels), &chans)
	logger.Warnf("[AlertNotify] rule=%s severity=%s channels=%v msg=%s", rule.Name, rule.Severity, chans, h.Message)
	return nil
}

// MultiNotifier 遍历调用所有子 Notifier，单个失败不中断，返回首个 error
type MultiNotifier struct {
	notifiers []AlertNotifier
}

func NewMultiNotifier(notifiers ...AlertNotifier) AlertNotifier {
	filtered := make([]AlertNotifier, 0, len(notifiers))
	for _, n := range notifiers {
		if n != nil {
			filtered = append(filtered, n)
		}
	}
	return &MultiNotifier{notifiers: filtered}
}

func (m *MultiNotifier) Notify(ctx context.Context, rule *model.AlertRule, h *model.AlertHistory) error {
	var firstErr error
	for _, n := range m.notifiers {
		if err := n.Notify(ctx, rule, h); err != nil {
			logger.Warnf("[MultiNotifier] 子通知器失败: %v", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// EmailSender 抽象 EmailService 避免循环依赖，语义与 EmailService.Send 对齐
type EmailSender interface {
	Send(ctx context.Context, accountID uint, to, subject, content string, attachments []string) (string, error)
}

type EmailAlertNotifier struct {
	sender     EmailSender
	recipients []string
	accountID  uint
}

func NewEmailAlertNotifier(sender EmailSender, recipients []string, accountID uint) AlertNotifier {
	return &EmailAlertNotifier{sender: sender, recipients: recipients, accountID: accountID}
}

func (n *EmailAlertNotifier) Notify(ctx context.Context, rule *model.AlertRule, h *model.AlertHistory) error {
	if n.sender == nil || len(n.recipients) == 0 {
		return nil
	}
	subject := fmt.Sprintf("[HiveMTK 告警] %s (%s)", rule.Name, rule.Severity)
	var firstErr error
	for _, to := range n.recipients {
		to = strings.TrimSpace(to)
		if to == "" {
			continue
		}
		if _, err := n.sender.Send(ctx, n.accountID, to, subject, h.Message, nil); err != nil {
			logger.Warnf("[EmailAlertNotifier] 发送告警给 %s 失败: %v", to, err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

type WebhookAlertNotifier struct {
	url string
}

func NewWebhookAlertNotifier(url string) AlertNotifier {
	return &WebhookAlertNotifier{url: url}
}

func (n *WebhookAlertNotifier) Notify(ctx context.Context, rule *model.AlertRule, h *model.AlertHistory) error {
	if n.url == "" {
		return nil
	}
	payload := map[string]any{
		"rule_name":    rule.Name,
		"severity":     rule.Severity,
		"source":       rule.Source,
		"message":      h.Message,
		"value":        h.Value,
		"threshold":    h.Threshold,
		"status":       h.Status,
		"triggered_at": h.TriggeredAt.Format(time.RFC3339),
		"channels":     rule.Channels,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("webhook marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("webhook new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	hc := &http.Client{Timeout: 10 * time.Second}
	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("webhook post: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook 返回非 2xx: %d", resp.StatusCode)
	}
	return nil
}
