package repository

// 契约锁（实体审查 #11，TENANT_MODE=on）：browser_steps 是「正文级」证据表，
// 它的每一条读写都必须落在自己租户的分片里。
//
// 为什么这张表要单独钉——它跟前面几张表的风险形状不一样：
//   - 步行的 `value` 存的是要发出去的正文原文，`result` 存的是快照/抽取结果。
//     这一张表是全域**内容密度最高**的一张：一次跨租户读不是「看到别人的状态」，
//     而是拿到别人准备发/已经发的原文。
//   - `UpdateResult`/`UpdateSubmitState`/`SubmitStateOf` 三个口的 WHERE 只有 `id = ?`，
//     没有 user、也没有 session 条件——租户谓词是这条链上唯一还在生效的遮挡。
//   - `FindSubmitAttempt`（双发闸）反过来：它**必须**只看本租户。同一条文案在两个
//     租户各自发一次是完全合法的，一旦闸门跨了租户，B 发过之后 A 就永远发不出去；
//     而它返回的那一行会被上层当成「前一轮事实」写进日志与回执，跨租户即外泄。
//
// 两个方向的断言都得有：遮挡别读到我 + 闸别跨到我。只测前者会留下「闸门全局化」
// 这类把可用性做成锁死的改法，只测后者会留下正文外泄。

import (
	"context"
	"errors"
	"testing"

	"hivemtk-user/internal/browser_automation/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

const (
	stepTenantA = uint(31)
	stepTenantB = uint(57)
	// 号段避开 sess*（793xxx）与 receipt*（792xxx）：同包并行时共用行号会让对方的
	// 清理改掉本方的计数。
	stepSessionShared = uint(795100) // 两个租户刻意用**同一个** session 号：断言的才是 tenant_id 谓词
	stepTaskA         = uint(795001)
	stepTaskB         = uint(795002)
	stepIDA           = uint(795201)
	stepIDB           = uint(795202)
	stepTextHash      = "0badc0de00000001"
	stepAction        = "click"
)

func stepTenantDB(t *testing.T) (BrowserStepRepository, *gorm.DB, context.Context, context.Context) {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })
	db := testutil.NewTestDB(t, &model.BrowserStep{})
	if db == nil {
		t.Fatal("测试库不可达：步行归属与双发闸的租户边界都无法判定（不 Skip，跳过等于没锁）")
	}
	return NewBrowserStepRepositoryWithDB(db), db,
		tenant.WithTenantID(context.Background(), stepTenantA),
		tenant.WithTenantID(context.Background(), stepTenantB)
}

// stepSeed 走被测的 BatchCreate 落一行：写侧的租户填充本身就是被审的那件事，不走夹具直插。
func stepSeed(t *testing.T, repo BrowserStepRepository, ctx context.Context,
	id, sessionID, taskID uint, textHash, submitState string) *model.BrowserStep {
	t.Helper()
	row := &model.BrowserStep{
		ID: id, SessionID: sessionID, TaskID: taskID, StepIndex: 1,
		Action: stepAction, Value: "正文原文-" + submitState,
		Status: "running", SubmitState: submitState, TextHash: textHash,
	}
	if err := repo.BatchCreate(ctx, []*model.BrowserStep{row}); err != nil {
		t.Fatalf("步行 %d 落库失败: %v", id, err)
	}
	return row
}

// stepPhysical 跨租户物理读（system scope 只用于「这一行到底长什么样」的取证断言）。
func stepPhysical(t *testing.T, db *gorm.DB, id uint) *model.BrowserStep {
	t.Helper()
	var row model.BrowserStep
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).
		Where("id = ?", id).First(&row).Error; err != nil {
		t.Fatalf("物理读取 step=%d 失败: %v", id, err)
	}
	return &row
}

// TestStepBatchCreateFillsTenantAndRejectsBareContext 写侧：租户由 ctx 填；
// 裸 ctx 被拒而不是落进默认租户 1——后者会把别人的正文与提交台账混进 1 号租户。
func TestStepBatchCreateFillsTenantAndRejectsBareContext(t *testing.T) {
	repo, db, ctxA, _ := stepTenantDB(t)
	rows := []*model.BrowserStep{
		{ID: stepIDA, SessionID: stepSessionShared, TaskID: stepTaskA, StepIndex: 1, Action: stepAction, Status: "running"},
		{ID: stepIDA + 1, SessionID: stepSessionShared, TaskID: stepTaskA, StepIndex: 2, Action: stepAction, Status: "running"},
	}
	if err := repo.BatchCreate(ctxA, rows); err != nil {
		t.Fatalf("A 的批量落步失败: %v", err)
	}
	for _, r := range rows {
		if r.TenantID != stepTenantA {
			t.Errorf("批量落步后 step=%d tenant_id = %d，期望 %d（隔离 callback 需逐元素填充）", r.ID, r.TenantID, stepTenantA)
		}
	}
	bare := &model.BrowserStep{ID: stepIDA + 2, SessionID: stepSessionShared, TaskID: stepTaskA,
		StepIndex: 3, Action: stepAction, Status: "running"}
	if err := repo.BatchCreate(context.Background(), []*model.BrowserStep{bare}); err == nil {
		t.Error("裸 ctx 的步行落库未被拒绝")
	}
	var n int64
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).
		Model(&model.BrowserStep{}).Where("id = ?", stepIDA+2).Count(&n).Error; err != nil {
		t.Fatalf("物理计数失败: %v", err)
	}
	if n != 0 {
		t.Errorf("裸 ctx 被拒后仍留下 %d 行", n)
	}
}

// TestStepListBySessionIsTenantScoped 同一个 session 号在两个租户各有一行：
// ListBySessionID 只能看到自己那条。这一口是导出/回放面板的读家，看到别人的
// 就等于把别人的正文与快照整页端走。
func TestStepListBySessionIsTenantScoped(t *testing.T) {
	repo, _, ctxA, ctxB := stepTenantDB(t)
	stepSeed(t, repo, ctxA, stepIDA, stepSessionShared, stepTaskA, "", "prepared")
	stepSeed(t, repo, ctxB, stepIDB, stepSessionShared, stepTaskB, "", "prepared")

	for _, tc := range []struct {
		name string
		ctx  context.Context
		want uint
	}{
		{"A 读自己的 session", ctxA, stepIDA},
		{"B 读同一 session 号", ctxB, stepIDB},
	} {
		list, err := repo.ListBySessionID(tc.ctx, stepSessionShared)
		if err != nil {
			t.Fatalf("%s 失败: %v", tc.name, err)
		}
		if len(list) != 1 {
			t.Errorf("%s：期望 1 行，实得 %d 行", tc.name, len(list))
			continue
		}
		if list[0].ID != tc.want {
			t.Errorf("%s：读到 step=%d，期望 %d", tc.name, list[0].ID, tc.want)
		}
	}
	if _, err := repo.ListBySessionID(context.Background(), stepSessionShared); err == nil {
		t.Error("裸 ctx 的 ListBySessionID 未被拒绝（fail-closed 失效）")
	}
}

// TestStepByIDWritesAreTenantFactored 三个按主键直写的写口在别人的租户名下必须写不动。
func TestStepByIDWritesAreTenantFactored(t *testing.T) {
	repo, db, ctxA, ctxB := stepTenantDB(t)
	stepSeed(t, repo, ctxB, stepIDB, stepSessionShared, stepTaskB, stepTextHash, "sent")
	before := stepPhysical(t, db, stepIDB)

	if err := repo.UpdateResult(ctxA, stepIDB, "success", []byte(`{"ok":true}`), 1200, ""); err == nil {
		t.Error("A 的 ctx 改 B 的步终态竟被当作成功：应报错而不是静默 0 行")
	}
	if err := repo.UpdateSubmitState(ctxA, stepIDB, "verified", stepTextHash); err == nil {
		t.Error("A 的 ctx 改 B 的提交台账未被拒绝")
	}
	if _, err := repo.SubmitStateOf(ctxA, stepIDB); err == nil {
		t.Error("A 的 ctx 读到 B 的台账态")
	}
	after := stepPhysical(t, db, stepIDB)
	if after.Status != before.Status || after.SubmitState != before.SubmitState ||
		after.DurationMs != before.DurationMs {
		t.Errorf("B 的步行被 A 改动：status %s→%s submit_state %s→%s duration %d→%d",
			before.Status, after.Status, before.SubmitState, after.SubmitState,
			before.DurationMs, after.DurationMs)
	}

	// 同一行在自己的租户下三处都写得动——上面三条红不能是「写口整体坏掉」造成的。
	if err := repo.UpdateResult(ctxB, stepIDB, "success", []byte(`{"ok":true}`), 1200, ""); err != nil {
		t.Errorf("B 改自己的步终态失败: %v", err)
	}
	if err := repo.UpdateSubmitState(ctxB, stepIDB, "verified", stepTextHash); err != nil {
		t.Errorf("B 改自己的提交台账失败: %v", err)
	}
	state, err := repo.SubmitStateOf(ctxB, stepIDB)
	if err != nil || state != model.StepSubmitVerified {
		t.Errorf("B 读自己的台账态 = %q err=%v，期望 %q", state, err, model.StepSubmitVerified)
	}
}

// TestStepTerminalWriteReportsZeroRows 步终态写口命中 0 行必须报错。
//
// 为什么这是缺陷而不是洁癖：`finishStep`（executor.go:167）把「落库失败」当错误上报，
// 而 gorm 对 0 行只回 Error==nil——于是这一步永远停在 running，而日志里一行红都没有。
// session/task 行有 stale_reconcile 兜底收敛，步行没有任何兜底：这条旁路是终态丢失的
// 唯一无声出口。触发形态与 UpdateSubmitState 注释记的同源（拿到别的库的行号 / 行已软删）。
func TestStepTerminalWriteReportsZeroRows(t *testing.T) {
	repo, _, ctxA, _ := stepTenantDB(t)
	if err := repo.UpdateResult(ctxA, stepIDB, "failed", nil, 10, "不该写进去"); err == nil {
		t.Error("UpdateResult 命中 0 行却返回成功：步终态静默丢失，该行不存在而无人知晓")
	}
}

// TestSubmitAttemptGateStaysInsideTenant 双发闸的两个方向：
//   - 同租户同文案必须命中（闸门有效）；
//   - 别的租户发了同样的文案，本租户不能被拦（可用性），更不得把那行读回来
//     （上层会把「前一轮事实」的 step id / session id / 正文写进日志与回执）。
func TestSubmitAttemptGateStaysInsideTenant(t *testing.T) {
	repo, _, ctxA, ctxB := stepTenantDB(t)
	// 两个租户用**同一个 task 号 + 同一份 text_hash**：断言的才是 tenant_id 谓词本身。
	prior := stepSeed(t, repo, ctxB, stepIDB, stepSessionShared, stepTaskA, stepTextHash, model.StepSubmitSent)

	got, err := repo.FindSubmitAttempt(ctxB, stepTaskA, stepTextHash, 0)
	if err != nil {
		t.Fatalf("同租户闸门应命中自己的提交尝试: %v", err)
	}
	if got.ID != prior.ID {
		t.Errorf("闸门命中 step=%d，期望 %d", got.ID, prior.ID)
	}

	if other, err := repo.FindSubmitAttempt(ctxA, stepTaskA, stepTextHash, 0); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("A 的闸门读到 step=%v err=%v，期望 record not found（B 的提交不该拦停 A，也不该把 B 的正文交给 A）",
			other, err)
	}
	// prepared 从未发生点击，任何租户都不算尝试过
	stepSeed(t, repo, ctxA, stepIDA, stepSessionShared, stepTaskA, "0badc0de00000002", model.StepSubmitPrepared)
	if _, err := repo.FindSubmitAttempt(ctxA, stepTaskA, "0badc0de00000002", 0); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("prepared 态被当成提交尝试（err=%v）：闸门口径漂了", err)
	}
	if _, err := repo.FindSubmitAttempt(context.Background(), stepTaskA, stepTextHash, 0); err == nil {
		t.Error("裸 ctx 的双发闸查询未被拒绝")
	}
}
