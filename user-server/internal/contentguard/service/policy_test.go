// policy_test.go 策略裁决的纯函数断言。
//
// 这一组是本功能的"产品口径"落地处：用户与商户走**两条不同的规则线**，
// 阈值/动作/冻结时长/升级次数都不同。口径一旦被改错，线上表现为
// "该拦的没拦"或"该只转人工的直接把人冻了"，两者都看不见（日志里只有 pass/block）。
package service

import (
	"testing"

	"hivemtk-user/internal/contentguard/matcher"
	"hivemtk-user/internal/contentguard/model"
)

func hit(sev int) matcher.Hit {
	return matcher.Hit{PatternID: 1, Word: "x", Severity: sev}
}

func TestDefaultPolicy_UserVsMerchantAreDifferentRules(t *testing.T) {
	u := DefaultPolicy(model.SubjectUser)
	if u.BlockSeverity != 3 || u.ReviewSeverity != 2 {
		t.Fatalf("user 阈值 = block %d / review %d，期望 3 / 2（聊天场景误杀成本高）",
			u.BlockSeverity, u.ReviewSeverity)
	}
	if u.BlockAction != model.ActionReject {
		t.Fatalf("user 拦截动作 = %s，期望 reject", u.BlockAction)
	}
	if u.TempFreezeMinutes != 24*60 {
		t.Fatalf("user 临时冻结 = %d 分钟，期望 1440", u.TempFreezeMinutes)
	}
	if u.EscalatePermanent != 3 {
		t.Fatalf("user 永久升级阈值 = %d，期望 3", u.EscalatePermanent)
	}
	if u.Scenes != model.SceneInboundMessage {
		t.Fatalf("user 审查场景 = %q，期望仅 %q", u.Scenes, model.SceneInboundMessage)
	}
	if u.APIEnabled || u.APISampleRate != 0 {
		t.Fatal("出厂策略必须关闭外部审核（数据零出域）")
	}

	m := DefaultPolicy(model.SubjectMerchant)
	if m.BlockSeverity != 2 || m.ReviewSeverity != 1 {
		t.Fatalf("merchant 阈值 = block %d / review %d，期望 2 / 1（对外内容发出即收不回）",
			m.BlockSeverity, m.ReviewSeverity)
	}
	if m.BlockAction != model.ActionBlockPublish {
		t.Fatalf("merchant 拦截动作 = %s，期望 block_publish", m.BlockAction)
	}
	if m.TempFreezeMinutes != 7*24*60 {
		t.Fatalf("merchant 临时冻结 = %d 分钟，期望 10080", m.TempFreezeMinutes)
	}
	if m.EscalatePermanent != 2 {
		t.Fatalf("merchant 永久升级阈值 = %d，期望 2", m.EscalatePermanent)
	}
	if CoversScene(m, model.SceneInboundMessage) {
		t.Fatal("merchant 策略不应覆盖终端用户入站消息（那是 user 侧职责）")
	}
	for _, s := range []string{model.SceneMaterial, model.SceneKBDocument, model.SceneOutboundReply, model.SceneMarketingCopy} {
		if !CoversScene(m, s) {
			t.Fatalf("merchant 策略应覆盖场景 %s", s)
		}
	}
}

func TestDefaultPolicy_UnknownSubjectFallsBackToUser(t *testing.T) {
	p := DefaultPolicy("whatever")
	if p.SubjectType != model.SubjectUser {
		t.Fatalf("未知主体类型的出厂策略应为 user，得 %q", p.SubjectType)
	}
}

func TestEvaluate_UserThresholds(t *testing.T) {
	p := DefaultPolicy(model.SubjectUser)
	cases := []struct {
		sev         int
		wantVerdict string
		wantAction  string
		wantFreeze  string
	}{
		{3, model.VerdictBlock, model.ActionReject, model.SanctionLevelTemp},
		{2, model.VerdictReview, model.ActionReview, ""},
		{1, model.VerdictPass, model.ActionNone, ""},
	}
	for _, tc := range cases {
		d := Evaluate(p, []matcher.Hit{hit(tc.sev)}, 0)
		if d.Verdict != tc.wantVerdict || d.Action != tc.wantAction || d.FreezeLevel != tc.wantFreeze {
			t.Fatalf("sev=%d 裁决 = {%s,%s,%s}，期望 {%s,%s,%s}", tc.sev,
				d.Verdict, d.Action, d.FreezeLevel, tc.wantVerdict, tc.wantAction, tc.wantFreeze)
		}
		if d.MaxSeverity != tc.sev {
			t.Fatalf("sev=%d 的 MaxSeverity = %d", tc.sev, d.MaxSeverity)
		}
	}
}

func TestEvaluate_MerchantThresholds(t *testing.T) {
	p := DefaultPolicy(model.SubjectMerchant)
	if d := Evaluate(p, []matcher.Hit{hit(2)}, 0); d.Verdict != model.VerdictBlock || d.Action != model.ActionBlockPublish {
		t.Fatalf("merchant sev2 裁决 = {%s,%s}，期望 block / block_publish", d.Verdict, d.Action)
	}
	if d := Evaluate(p, []matcher.Hit{hit(1)}, 0); d.Verdict != model.VerdictReview {
		t.Fatalf("merchant sev1 裁决 = %s，期望 review", d.Verdict)
	}
}

func TestEvaluate_UsesMaxSeverityOfAllHits(t *testing.T) {
	p := DefaultPolicy(model.SubjectUser)
	d := Evaluate(p, []matcher.Hit{hit(1), hit(2), hit(3)}, 0)
	if d.MaxSeverity != 3 || d.Verdict != model.VerdictBlock {
		t.Fatalf("多个命中应取最高级别：%+v", d)
	}
}

func TestEvaluate_EscalationToPermanent(t *testing.T) {
	user := DefaultPolicy(model.SubjectUser)
	cases := []struct {
		prior      int
		wantFreeze string
	}{
		{0, model.SanctionLevelTemp},      // 第 1 次
		{1, model.SanctionLevelTemp},      // 第 2 次
		{2, model.SanctionLevelPermanent}, // 第 3 次 ⇒ 升级永久
		{5, model.SanctionLevelPermanent},
	}
	for _, tc := range cases {
		d := Evaluate(user, []matcher.Hit{hit(3)}, tc.prior)
		if d.FreezeLevel != tc.wantFreeze {
			t.Fatalf("user prior=%d ⇒ 冻结档 %q，期望 %q", tc.prior, d.FreezeLevel, tc.wantFreeze)
		}
	}

	merchant := DefaultPolicy(model.SubjectMerchant)
	if d := Evaluate(merchant, []matcher.Hit{hit(2)}, 0); d.FreezeLevel != model.SanctionLevelTemp {
		t.Fatalf("merchant 第 1 次应临时冻结，得 %q", d.FreezeLevel)
	}
	if d := Evaluate(merchant, []matcher.Hit{hit(2)}, 1); d.FreezeLevel != model.SanctionLevelPermanent {
		t.Fatalf("merchant 第 2 次应升级永久，得 %q", d.FreezeLevel)
	}
}

func TestEvaluate_NoFreezeWhenPolicySaysZero(t *testing.T) {
	p := DefaultPolicy(model.SubjectUser)
	p.TempFreezeMinutes = 0
	p.EscalatePermanent = 0
	d := Evaluate(p, []matcher.Hit{hit(3)}, 0)
	if d.Verdict != model.VerdictBlock || d.FreezeLevel != "" {
		t.Fatalf("关掉冻结后应只拦当次内容，得 %+v", d)
	}
}

func TestEvaluate_DisabledPolicyAndNoHits(t *testing.T) {
	p := DefaultPolicy(model.SubjectUser)
	p.Enabled = false
	d := Evaluate(p, []matcher.Hit{hit(3)}, 0)
	if d.Verdict != model.VerdictPass || d.Skipped != "policy_disabled" {
		t.Fatalf("策略关闭应 pass 且留 skipped 原因，得 %+v", d)
	}

	d = Evaluate(DefaultPolicy(model.SubjectUser), nil, 0)
	if d.Verdict != model.VerdictPass || d.MaxSeverity != 0 || d.Skipped != "" {
		t.Fatalf("无命中应 pass 且不是 skipped，得 %+v", d)
	}

	if d := Evaluate(nil, []matcher.Hit{hit(3)}, 0); d.Verdict != model.VerdictPass {
		t.Fatalf("nil 策略应 pass（不能拦），得 %+v", d)
	}
}

func TestNormalizePolicy_ClampsBadValues(t *testing.T) {
	// block_severity=0 会让"命中即拦"退化成"永不拦"，而日志里完全看不到 ⇒ 必须回落出厂值。
	p := &model.GuardPolicy{SubjectType: model.SubjectUser, BlockSeverity: 0}
	got := NormalizePolicy(p)
	if got.BlockSeverity != 3 {
		t.Fatalf("block_severity 回落后 = %d，期望 3", got.BlockSeverity)
	}
	if got.ReviewSeverity > got.BlockSeverity {
		t.Fatalf("review 阈值不得高于 block：%d > %d", got.ReviewSeverity, got.BlockSeverity)
	}
	if got.BlockAction == "" || got.Scenes == "" || got.WindowMinutes <= 0 {
		t.Fatalf("缺省字段应回落出厂值：%+v", got)
	}

	over := NormalizePolicy(&model.GuardPolicy{
		SubjectType: model.SubjectUser, BlockSeverity: 2, ReviewSeverity: 3,
		APISampleRate: 500, TempFreezeMinutes: -10, WindowMinutes: 0,
	})
	if over.ReviewSeverity != 2 {
		t.Fatalf("review>block 应被压到 block，得 %d", over.ReviewSeverity)
	}
	if over.APISampleRate != 100 {
		t.Fatalf("抽样比例应夹到 100，得 %d", over.APISampleRate)
	}
	if over.TempFreezeMinutes != 0 {
		t.Fatalf("负的冻结时长应归零，得 %d", over.TempFreezeMinutes)
	}
	if over.WindowMinutes <= 0 {
		t.Fatal("窗口应回落出厂值")
	}

	if NormalizePolicy(nil) != nil {
		t.Fatal("nil 策略应返回 nil")
	}
}

func TestCoversScene(t *testing.T) {
	p := &model.GuardPolicy{Scenes: " material , kb_document "}
	if !CoversScene(p, model.SceneMaterial) || !CoversScene(p, model.SceneKBDocument) {
		t.Fatal("逗号分隔且带空格的场景应能匹配（配置由人手填）")
	}
	if CoversScene(p, model.SceneInboundMessage) {
		t.Fatal("未列出的场景不应命中")
	}
	if CoversScene(nil, model.SceneMaterial) {
		t.Fatal("nil 策略不覆盖任何场景")
	}
}

func TestShouldSampleAPI(t *testing.T) {
	p := DefaultPolicy(model.SubjectUser)
	if ShouldSampleAPI(p, "digest") {
		t.Fatal("APIEnabled=false 时不得抽样（数据零出域默认）")
	}
	p.APIEnabled = true
	if ShouldSampleAPI(p, "digest") {
		t.Fatal("APISampleRate=0 时不得抽样")
	}
	p.APISampleRate = 100
	if !ShouldSampleAPI(p, "digest") {
		t.Fatal("100% 抽样必须命中")
	}
	// 确定性：同一摘要恒定命中同一结果，否则重试/重放会让记录不可复现。
	p.APISampleRate = 50
	first := ShouldSampleAPI(p, "abc")
	for i := 0; i < 20; i++ {
		if ShouldSampleAPI(p, "abc") != first {
			t.Fatal("抽样不确定：同一摘要两次结论不同")
		}
	}
	if ShouldSampleAPI(nil, "abc") {
		t.Fatal("nil 策略不抽样")
	}
}

func TestValidateSubjectTypeAndScene(t *testing.T) {
	if err := ValidateSubjectType(model.SubjectUser); err != nil {
		t.Fatalf("user 应合法：%v", err)
	}
	if err := ValidateSubjectType(model.SubjectMerchant); err != nil {
		t.Fatalf("merchant 应合法：%v", err)
	}
	if err := ValidateSubjectType("tenant"); err == nil {
		t.Fatal("拼错的主体类型必须在入口被拦下")
	}
	for _, s := range []string{
		model.SceneInboundMessage, model.SceneOutboundReply, model.SceneMaterial,
		model.SceneKBDocument, model.SceneMarketingCopy,
	} {
		if err := ValidateScene(s); err != nil {
			t.Fatalf("%s 应合法：%v", s, err)
		}
	}
	if err := ValidateScene("chat"); err == nil {
		t.Fatal("未知场景必须报错")
	}
}
