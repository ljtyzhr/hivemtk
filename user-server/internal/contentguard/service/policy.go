// Package service 内容安全闸（ContentGuard）的业务层：策略裁决、词库快照、
// 异步审查流水线、处置（冻结/黑名单）。
//
// 分工边界（五层架构内）：
//   - service 不持有 *gorm.DB，只依赖本包定义的 Store 接口（仓储实现见 contentguard/repository）；
//   - 匹配与归一化在 contentguard/{matcher,normalize}，纯函数、零依赖、可独立测试；
//   - 外部审核 API 在 contentguard/provider，默认关闭。
package service

import (
	"fmt"
	"strings"

	"hivemtk-user/internal/contentguard/matcher"
	"hivemtk-user/internal/contentguard/model"
)

// Decision 一次裁决的结论。
type Decision struct {
	// Verdict pass / review / block
	Verdict string
	// Action 基础动作：none / review / reject / block_publish
	Action string
	// FreezeLevel "" / temp / permanent（"" = 本次不处置，仅拦当次内容）
	FreezeLevel string
	// MaxSeverity 命中的最高严重级别（0 = 无命中）
	MaxSeverity int
	// Skipped 未审查原因（策略关闭 / 场景不在覆盖范围），仅供排查，不对外当判定
	Skipped string
}

// DefaultPolicy 主体类型的出厂策略。
//
// 两类主体的差异是**产品口径**，不是实现细节，所以写在这里而不是散在调用处：
//
//	user（访客/终端用户）：只审入站消息；sev>=3 才拦（聊天场景误杀成本高，
//	      中危只转人工）；首次拦截冻结 24h；窗口内第 3 次拦截升级永久。
//	merchant（商户/租户）：审对外发布与落库的内容资产（素材/知识库/营销文案/出站回复）；
//	      sev>=2 即拦（对外内容一旦发出就收不回）；首次拦截冻结 7 天；
//	      窗口内第 2 次升级永久（商户是经营主体，处置权在平台运营方）。
func DefaultPolicy(subjectType string) *model.GuardPolicy {
	if subjectType == model.SubjectMerchant {
		return &model.GuardPolicy{
			SubjectType: model.SubjectMerchant,
			Enabled:     true,
			Scenes: strings.Join([]string{
				model.SceneMaterial, model.SceneKBDocument,
				model.SceneOutboundReply, model.SceneMarketingCopy,
			}, ","),
			BlockSeverity:     2,
			ReviewSeverity:    1,
			BlockAction:       model.ActionBlockPublish,
			EscalatePermanent: 2,
			TempFreezeMinutes: 7 * 24 * 60,
			WindowMinutes:     7 * 24 * 60,
			APISampleRate:     0,
			APIEnabled:        false,
		}
	}
	return &model.GuardPolicy{
		SubjectType:       model.SubjectUser,
		Enabled:           true,
		Scenes:            model.SceneInboundMessage,
		BlockSeverity:     3,
		ReviewSeverity:    2,
		BlockAction:       model.ActionReject,
		EscalatePermanent: 3,
		TempFreezeMinutes: 24 * 60,
		WindowMinutes:     24 * 60,
		APISampleRate:     0,
		APIEnabled:        false,
	}
}

// NormalizePolicy 把库里的策略规整成可用形态（缺省字段回落出厂值）。
//
// 为什么要这一步：策略行是人手工改的，`block_severity=0` 之类的空值会让
// "命中即拦"退化成"永不拦"，而这种退化在日志里完全看不到。规整后越界一律夹取，
// 并把 block/review 的先后关系纠正（review 阈值不得高于 block 阈值，否则 review 永不触发）。
func NormalizePolicy(p *model.GuardPolicy) *model.GuardPolicy {
	if p == nil {
		return nil
	}
	out := *p
	def := DefaultPolicy(out.SubjectType)
	if out.BlockSeverity <= 0 || out.BlockSeverity > 3 {
		out.BlockSeverity = def.BlockSeverity
	}
	if out.ReviewSeverity <= 0 || out.ReviewSeverity > out.BlockSeverity {
		out.ReviewSeverity = out.BlockSeverity
	}
	if out.BlockAction == "" {
		out.BlockAction = def.BlockAction
	}
	if out.TempFreezeMinutes < 0 {
		out.TempFreezeMinutes = 0
	}
	if out.WindowMinutes <= 0 {
		out.WindowMinutes = def.WindowMinutes
	}
	if out.APISampleRate < 0 {
		out.APISampleRate = 0
	}
	if out.APISampleRate > 100 {
		out.APISampleRate = 100
	}
	if out.Scenes == "" {
		out.Scenes = def.Scenes
	}
	return &out
}

// CoversScene 策略是否覆盖该场景。
func CoversScene(p *model.GuardPolicy, scene string) bool {
	if p == nil {
		return false
	}
	for _, s := range strings.Split(p.Scenes, ",") {
		if strings.TrimSpace(s) == scene {
			return true
		}
	}
	return false
}

// Evaluate 裁决（纯函数）。
//
// priorBlocks 是**窗口内该主体此前已被拦截的次数**（不含本次）：
// 它把"偶发一次"和"屡教不改"区分开——前者冻结一个短窗口就够，后者必须升级为永久，
// 否则处置永远停在"罚了等于没罚"（冻结到期 → 再来一次 → 再冻结，循环）。
func Evaluate(p *model.GuardPolicy, hits []matcher.Hit, priorBlocks int) Decision {
	d := Decision{Verdict: model.VerdictPass, Action: model.ActionNone}
	if p == nil || !p.Enabled {
		d.Skipped = "policy_disabled"
		return d
	}
	maxSev := 0
	for _, h := range hits {
		if h.Severity > maxSev {
			maxSev = h.Severity
		}
	}
	d.MaxSeverity = maxSev
	if maxSev == 0 {
		return d
	}

	blockSev := p.BlockSeverity
	if blockSev <= 0 {
		blockSev = 3
	}
	reviewSev := p.ReviewSeverity
	if reviewSev <= 0 || reviewSev > blockSev {
		reviewSev = blockSev
	}

	if maxSev >= blockSev {
		d.Verdict = model.VerdictBlock
		d.Action = p.BlockAction
		if d.Action == "" {
			d.Action = model.ActionReject
		}
		switch {
		case p.EscalatePermanent > 0 && priorBlocks+1 >= p.EscalatePermanent:
			d.FreezeLevel = model.SanctionLevelPermanent
		case p.TempFreezeMinutes > 0:
			d.FreezeLevel = model.SanctionLevelTemp
		}
		return d
	}
	if maxSev >= reviewSev {
		d.Verdict = model.VerdictReview
		d.Action = model.ActionReview
	}
	return d
}

// ShouldSampleAPI 外部审核抽样判定（确定性抽样：同一内容摘要恒定命中同一结果）。
//
// 为什么不用随机数：随机会让同一条内容在重试/重放时一会儿送审一会儿不送，
// 使"记录里有没有 api 结论"变成不可复现的事实，排障时无法判断是策略生效还是抽样漏了。
func ShouldSampleAPI(p *model.GuardPolicy, digest string) bool {
	if p == nil || !p.APIEnabled || p.APISampleRate <= 0 {
		return false
	}
	if p.APISampleRate >= 100 {
		return true
	}
	var sum uint32
	for i := 0; i < len(digest); i++ {
		sum = sum*31 + uint32(digest[i])
	}
	return int(sum%100) < p.APISampleRate
}

// ValidateSubjectType 主体类型校验（对外入口统一用它，避免写进库才发现拼错）。
func ValidateSubjectType(t string) error {
	switch t {
	case model.SubjectUser, model.SubjectMerchant:
		return nil
	}
	return fmt.Errorf("未知审查主体类型: %s", t)
}

// ValidateScene 场景校验。
func ValidateScene(s string) error {
	switch s {
	case model.SceneInboundMessage, model.SceneOutboundReply, model.SceneMaterial,
		model.SceneKBDocument, model.SceneMarketingCopy:
		return nil
	}
	return fmt.Errorf("未知审查场景: %s", s)
}
