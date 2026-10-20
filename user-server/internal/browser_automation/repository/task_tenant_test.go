package repository

// 契约锁（实体审查 #12，TENANT_MODE=on）：browser_tasks 是全域的「调度事实表」，
// 它的按主键直写口与那条刻意不带 user 过滤的读口都站在租户地基上。
//
// 为什么这张表要单独钉，且形状与 #10/#11 不同：
//   - 写口 `UpdateStatus`/`UpdateRunResult`/`SetNextRetryAt`/`ReconcileRunResult` 的 WHERE
//     只有 `id = ?`（`SoftDelete` 是唯一带 user 的），执行协程与重试扫描器手里也只有 task.ID；
//     归属早在 `RunTask`/`GetByID` 判过，于是**租户谓词是这条链上唯一还在生效的遮挡**。
//   - 读口 `GetByIDAnyUser`（依赖检查）**刻意不带 user 过滤**（另一条同语义的
//     `ListDependents` 已在 4.4 收口时删除，见仓储注释），
//     靠租户谓词遮住别人的任务号；`checkDependency` 身后还有一道 `dep.UserID != t.UserID`，
//     但那道判据用的是「user_id 全局唯一」这个前提，租户谓词先失效它就会退化成
//     「跨租户读到别人的任务存在性与状态」。
//   - 两条后台腿的口径与在线腿**相反**，必须分头钉：重试认领 `ClaimDueRetries` 由
//     `tenant.Fanout` 逐租户跑（跨租户认领 = 在自己的 registry 上烧别人的重试额度），
//     而对账 `FindStaleRunningAll`/`ReconcileRunResult` 必须 system scope 跨租户生效
//     （漏了就是别人的任务永久砖化）。
//
// 本文件另有两条与租户无关、但同样落在这张表写面上的锁，一起放这里是因为夹具共用：
// `TestTaskByIDWritesReportZeroRows`（0 行不许报成功）与
// `TestTaskSaveDoesNotRevertExecutorColumns`（Save 的写面不得覆盖执行侧窄口）。

import (
	"context"
	"errors"
	"testing"
	"time"

	"hivemtk-user/internal/browser_automation/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

const (
	taskTenantA = uint(41)
	taskTenantB = uint(63)
	// 号段避开 sess*（793xxx）与 step*（795xxx）：同包并行时共用行号会让对方的清理改掉本方计数。
	taskIDA   = uint(796001)
	taskIDB   = uint(796002)
	taskUserA = uint(796101)
	taskUserB = uint(796102)
	taskURL   = "https://www.xiaohongshu.com/explore"
)

func taskTenantDB(t *testing.T) (BrowserTaskRepository, *gorm.DB, context.Context, context.Context) {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })
	db := testutil.NewTestDB(t, &model.BrowserTask{})
	if db == nil {
		t.Fatal("测试库不可达：任务行的租户归属无法判定（不 Skip，跳过等于没锁）")
	}
	return NewBrowserTaskRepositoryWithDB(db), db,
		tenant.WithTenantID(context.Background(), taskTenantA),
		tenant.WithTenantID(context.Background(), taskTenantB)
}

func newTask(id, userID uint, status string) *model.BrowserTask {
	return &model.BrowserTask{ID: id, Name: "任务", Url: taskURL, UserID: userID,
		TaskType: "one_shot", Status: status, Platform: "xiaohongshu"}
}

// taskSeed 走被测的 Create 落一行：写侧的租户填充本身就是被审的那件事。
func taskSeed(t *testing.T, repo BrowserTaskRepository, ctx context.Context, id, userID uint, status string) *model.BrowserTask {
	t.Helper()
	row := newTask(id, userID, status)
	if err := repo.Create(ctx, row); err != nil {
		t.Fatalf("任务 %d 落库失败: %v", id, err)
	}
	return row
}

// taskPhysical 跨租户物理读（system scope 是本域对账器已有的合法通道）。
func taskPhysical(t *testing.T, db *gorm.DB, id uint) *model.BrowserTask {
	t.Helper()
	var row model.BrowserTask
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).
		Where("id = ?", id).First(&row).Error; err != nil {
		t.Fatalf("物理读取 task=%d 失败: %v", id, err)
	}
	return &row
}

// retryTime 把可空的到期时间变成可比的字符串：直接比 *time.Time 比的是两个查询各自的指针，
// 同值也会「不相等」，会把「没被改动」读成「被改动」。
func retryTime(ts *time.Time) string {
	if ts == nil {
		return "-"
	}
	return ts.Format(time.RFC3339Nano)
}

// TestTaskCreateFillsTenantAndRejectsBareContext 写侧：租户由 ctx 填，裸 ctx 必须被拒
// 而不是落进默认租户 1（那会让别人的任务出现在 1 号租户的列表、并发闸与活动预算里）。
func TestTaskCreateFillsTenantAndRejectsBareContext(t *testing.T) {
	repo, db, ctxA, _ := taskTenantDB(t)
	row := taskSeed(t, repo, ctxA, taskIDA, taskUserA, "draft")
	if row.TenantID != taskTenantA {
		t.Errorf("落库后 tenant_id = %d，期望 %d（由隔离 callback 从 ctx 填）", row.TenantID, taskTenantA)
	}
	bare := newTask(0, taskUserA, "draft")
	if err := repo.Create(context.Background(), bare); err == nil {
		t.Error("裸 ctx 的任务落库未被拒绝")
	}
	// 行自证（结构体里显式带 tenant_id）只在**无租户上下文的系统腿**里才合法；
	// 有 ctx 租户时带别人的号必须被拒——否则「换个号就能把任务写进别人的库区」。
	other := newTask(0, taskUserA, "draft")
	other.TenantID = taskTenantB
	if err := repo.Create(ctxA, other); err == nil {
		t.Error("A 的 ctx 用自证 tenant_id=B 落库未被拒绝")
	}
	var n int64
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).
		Model(&model.BrowserTask{}).Where("user_id = ? AND id <> ?", taskUserA, taskIDA).Count(&n).Error; err != nil {
		t.Fatalf("物理计数失败: %v", err)
	}
	if n != 0 {
		t.Errorf("裸 ctx/自证被拒后仍留下 %d 行", n)
	}
}

// TestTaskSaveAndSoftDeletePortsAreTenantFactored 补上另两个写口：`Update`(Save 全字段)
// 与 `SoftDelete`。它们不在 `UpdateStatus` 那一组里，形状也不同，必须分头钉：
//   - `TaskService.Update` 是「GetByID 读出来 → 改字段 → Save」的读改写，Save 的 SET
//     带**全部**列（含 tenant_id）。A 拿着从系统视角读到的 B 的行来 Save，正确结果是
//     被 `guardUpdateTenantAssignments` 拦下（改租户列），而不是「WHERE 打空 = 假成功」。
//   - `SoftDelete` 是唯一自带 user 过滤的写口：这里的红必须是「行还活着」，
//     软删一旦跨租户生效，别人的任务、会话与在途执行一起消失。
func TestTaskSaveAndSoftDeletePortsAreTenantFactored(t *testing.T) {
	repo, db, ctxA, ctxB := taskTenantDB(t)
	taskSeed(t, repo, ctxB, taskIDB, taskUserB, "ready")

	// A 从系统视角拿到 B 的行（模拟装配缺陷/越权读取拿到的快照），再走 Save 写回。
	steal := taskPhysical(t, db, taskIDB)
	steal.Name = "越权改名"
	if err := repo.Update(ctxA, steal); err == nil {
		t.Error("A 的 ctx 用 Save 覆写 B 的任务竟被当作成功")
	}
	if err := repo.SoftDelete(ctxA, taskIDB, taskUserB); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("A 软删 B 的任务 err=%v，期望 ErrRecordNotFound", err)
	}
	if alive := taskPhysical(t, db, taskIDB); alive.Name != "任务" || alive.DeletedAt.Valid {
		t.Errorf("B 的任务被 A 改动：name=%q deleted_at=%v", alive.Name, alive.DeletedAt.Time)
	}
	// 同一租户自证相等时 Save 必须写得动（否则上面的红是「Save 整体坏」造成的）。
	own, err := repo.GetByID(ctxB, taskIDB, taskUserB)
	if err != nil {
		t.Fatalf("B 读自己的任务失败: %v", err)
	}
	own.Name = "自己的改名"
	if err := repo.Update(ctxB, own); err != nil {
		t.Errorf("B 走 Save 改自己失败: %v", err)
	}
	if got := taskPhysical(t, db, taskIDB); got.Name != "自己的改名" {
		t.Errorf("B 的 Save 未落库，name=%q", got.Name)
	}
	if err := repo.SoftDelete(ctxB, taskIDB, taskUserB); err != nil {
		t.Errorf("B 软删自己的任务失败: %v", err)
	}
}

// TestTaskBudgetPortStaysInsideTenant 活动预算扣减口：跨租户必须既不放行也不扣，
// 同租户才扣得动。这条口的后果不是数据错，而是**一条计划外的触达真的发出去**（不可逆）。
func TestTaskBudgetPortStaysInsideTenant(t *testing.T) {
	repo, db, ctxA, ctxB := taskTenantDB(t)
	taskSeed(t, repo, ctxB, taskIDB, taskUserB, "ready")
	// 预算列走 Save 写回，必须用「读出来再改」的结构体：手动构造的零 TenantID 结构体会被
	// guardUpdateTenantAssignments 判成「把行的租户改成 0」，那是另一条口（上面已钉）。
	withBudget, err := repo.GetByID(ctxB, taskIDB, taskUserB)
	if err != nil {
		t.Fatalf("B 读自己的任务失败: %v", err)
	}
	withBudget.CampaignKey = "camp-12"
	withBudget.CampaignActBudget = 2
	if err := repo.Update(ctxB, withBudget); err != nil {
		t.Fatalf("写入活动预算失败: %v", err)
	}
	// 跨租户：条件 UPDATE 打不到别人的行 ⇒ ok=false 且不报错（调用方据此拦停写步）。
	// 裸 ctx：走的是「ctx 未绑定即拒绝」那道闸，必须是 err —— 两条腿的红法不同，
	// 混成一个循环就会把 fail-closed 报错当成断言失败（本用例第一版正是这样读错的）。
	if ok, err := repo.TryConsumeCampaignActBudget(ctxA, taskIDB); err != nil || ok {
		t.Errorf("A 的 ctx 扣 B 的活动预算 ok=%v err=%v，期望 false/nil", ok, err)
	}
	if _, err := repo.TryConsumeCampaignActBudget(context.Background(), taskIDB); err == nil {
		t.Error("裸 ctx 的预算扣减未被拒绝")
	}
	if got := taskPhysical(t, db, taskIDB); got.CampaignActUsed != 0 {
		t.Errorf("跨租户扣减后 campaign_act_used=%d，期望 0", got.CampaignActUsed)
	}
	// 自己的两条腿：额度 2 用满后第三条例止。
	for i := 1; i <= 2; i++ {
		if ok, err := repo.TryConsumeCampaignActBudget(ctxB, taskIDB); err != nil || !ok {
			t.Fatalf("第 %d 次自扣失败 ok=%v err=%v", i, ok, err)
		}
	}
	if ok, err := repo.TryConsumeCampaignActBudget(ctxB, taskIDB); err != nil || ok {
		t.Errorf("额度用满后仍放行 ok=%v err=%v", ok, err)
	}
	if got := taskPhysical(t, db, taskIDB); got.CampaignActUsed != 2 {
		t.Errorf("campaign_act_used=%d，期望 2", got.CampaignActUsed)
	}
}

// TestTaskSaveDoesNotRevertExecutorColumns `Update`(Save 全字段) 不得把执行侧的窄写口
// 写出来的列退回读取快照那一刻的值。
//
// 这不是并发洁癖，是三条已被窄口各自认领的列：
//   - `retry_count`（UpdateRunResult 写）——回退就是「MaxRetryTimes 额度复活」：
//     一个已经放弃自动重试的任务重新拿到满把重试，而重试会下写步；
//   - `next_retry_at`（SetNextRetryAt/ClaimDueRetries 写）——快照里挂着、库里已被认领清空，
//     Save 把它再挂回去就是同一轮失败的第二发；
//   - `campaign_act_used`（TryConsumeCampaignActBudget 原子条件更新写）——回退掉的每一条额度
//     都会换成一条计划外的真实触达（不可逆），而这条口的全部意义正是「裁决权落在库里」。
//
// 竞态窗口是真实的：`TaskService.Update` 只在**读到的快照**上判 `status != running`，
// 而 failed/paused/ready 都是允许编辑、又同时允许执行侧写列的状态（重试腿就在 failed 上跑）。
func TestTaskSaveDoesNotRevertExecutorColumns(t *testing.T) {
	repo, db, _, ctxA := taskTenantDB(t)
	taskSeed(t, repo, ctxA, taskIDA, taskUserA, "ready")
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).
		Model(&model.BrowserTask{}).Where("id = ?", taskIDA).
		Updates(map[string]any{"campaign_key": "camp-12", "campaign_act_budget": 3}).Error; err != nil {
		t.Fatalf("预算夹具写入失败: %v", err)
	}

	snap, err := repo.GetByID(ctxA, taskIDA, taskUserA)
	if err != nil {
		t.Fatalf("读快照失败: %v", err)
	}
	snap.Name = "用户改名"

	// 快照之后，执行侧三条窄写落库。
	if err := repo.UpdateRunResult(ctxA, taskIDA, "failed", "steps 3/5 成功", "验证码超时", 2); err != nil {
		t.Fatalf("执行侧写快照失败: %v", err)
	}
	at := time.Now().Add(time.Minute)
	if err := repo.SetNextRetryAt(ctxA, taskIDA, &at); err != nil {
		t.Fatalf("执行侧挂起重试失败: %v", err)
	}
	if ok, err := repo.TryConsumeCampaignActBudget(ctxA, taskIDA); err != nil || !ok {
		t.Fatalf("执行侧扣预算失败 ok=%v err=%v", ok, err)
	}

	if err := repo.Update(ctxA, snap); err != nil {
		t.Fatalf("用户编辑落库失败: %v", err)
	}
	got := taskPhysical(t, db, taskIDA)
	if got.Name != "用户改名" {
		t.Errorf("用户这一腿没落库，name=%q（夹具坏了，下面的结论都不成立）", got.Name)
	}
	if got.RetryCount != 2 {
		t.Errorf("retry_count 被编辑覆写回快照值 %d，期望保住执行侧的 2：额度复活=满把重试再来一轮", got.RetryCount)
	}
	if got.NextRetryAt == nil {
		t.Error("next_retry_at 被编辑覆写成快照里的 NULL：挂起重试静默消失")
	}
	if got.LastResult != "steps 3/5 成功" || got.ErrorMsg != "验证码超时" {
		t.Errorf("执行结果被编辑覆写：last_result=%q error_msg=%q", got.LastResult, got.ErrorMsg)
	}
	if got.Status != "failed" {
		t.Errorf("status 被编辑覆写成快照值 %q（状态转移有专用写口，编辑腿不该碰）", got.Status)
	}
	if got.CampaignActUsed != 1 {
		t.Errorf("campaign_act_used 被编辑覆写回 %d，期望保住 1：每退一条就是一条计划外触达", got.CampaignActUsed)
	}
}

// TestTaskByIDWritesAreTenantFactored 三个按主键直写的状态口用别人的租户 ctx 一律写不动。
func TestTaskByIDWritesAreTenantFactored(t *testing.T) {
	repo, db, ctxA, ctxB := taskTenantDB(t)
	taskSeed(t, repo, ctxB, taskIDB, taskUserB, "failed")
	before := taskPhysical(t, db, taskIDB)

	writes := []struct {
		name string
		run  func(context.Context) error
	}{
		{"UpdateStatus", func(c context.Context) error { return repo.UpdateStatus(c, taskIDB, "archived", "越权改写") }},
		{"UpdateRunResult", func(c context.Context) error {
			return repo.UpdateRunResult(c, taskIDB, "done", "越权结果", "", 9)
		}},
		{"SetNextRetryAt", func(c context.Context) error {
			at := time.Now().Add(time.Minute)
			return repo.SetNextRetryAt(c, taskIDB, &at)
		}},
	}
	for _, w := range writes {
		if err := w.run(ctxA); err == nil {
			t.Errorf("A 的 ctx 走 %s 改 B 的任务竟被当作成功", w.name)
		}
	}
	// ReconcileRunResult 与上面三口**口径相反**：它的 0 行是设计好的「没抢到收口权」信号
	// （bool 返回值），不是故障。跨租户必须 ok=false，且不能报错——报错会让对账器把
	// 别人的行当成数据库故障反复重试。
	if ok, err := repo.ReconcileRunResult(ctxA, taskIDB, "failed", "越权对账", ""); err != nil || ok {
		t.Errorf("A 的 ctx 对账 B 的任务 ok=%v err=%v，期望 false/nil", ok, err)
	}
	after := taskPhysical(t, db, taskIDB)
	if after.Status != before.Status || after.ErrorMsg != before.ErrorMsg ||
		after.RetryCount != before.RetryCount || retryTime(after.NextRetryAt) != retryTime(before.NextRetryAt) {
		t.Errorf("B 的任务被 A 改动：status %s→%s retry_count %d→%d next_retry_at %s→%s",
			before.Status, after.Status, before.RetryCount, after.RetryCount,
			retryTime(before.NextRetryAt), retryTime(after.NextRetryAt))
	}

	// 同一行在自己的租户下四口都写得动——上面的红不能是「写口整体坏掉」造成的。
	if err := repo.UpdateStatus(ctxB, taskIDB, "running", ""); err != nil {
		t.Errorf("B 改自己的状态失败: %v", err)
	}
	if err := repo.UpdateRunResult(ctxB, taskIDB, "failed", "r", "e", 1); err != nil {
		t.Errorf("B 写自己的执行结果失败: %v", err)
	}
	at := time.Now().Add(time.Minute)
	if err := repo.SetNextRetryAt(ctxB, taskIDB, &at); err != nil {
		t.Errorf("B 挂自己的重试失败: %v", err)
	}
	// 对账口的条件 WHERE 要 `status='running'`，所以自己这一腿必须先把它放回 running 再收口——
	// 上面那腿的 ok=false 是租户挡的，这一腿必须是 1 行命中，两腿合起来才排除「条件写坏了」。
	if err := repo.UpdateStatus(ctxB, taskIDB, "running", ""); err != nil {
		t.Errorf("B 复位为 running 失败: %v", err)
	}
	if ok, err := repo.ReconcileRunResult(ctxB, taskIDB, "failed", "自己的对账", ""); err != nil || !ok {
		t.Errorf("B 对账自己的任务 ok=%v err=%v", ok, err)
	}
}

// TestTaskByIDWritesReportZeroRows 三个按主键直写的写口命中 0 行必须报错。
//
// 为什么这是缺陷而不是洁癖：gorm 对 0 行只回 `Error==nil`，于是
//   - `UpdateStatus(...,"running")` 白写：RunTask 之后任务行还是 draft/ready，
//     而 `FindStaleRunningAll` 按 `status='running'` 筛，这一轮执行从此**不在任何对账视野里**；
//   - `SetNextRetryAt` 白写：调用方紧接着 `logger.Infof("…已挂起…重启不丢")`
//     （feedback.go:181），库里其实一个字段都没变，那一次重试永远不会来。
//
// 两条都是「界面上说得通、库里没发生」的形状，与 #11 的步终态、#10 的会话收口同源。
func TestTaskByIDWritesReportZeroRows(t *testing.T) {
	repo, _, ctxA, _ := taskTenantDB(t)
	const ghost = uint(796999)
	if err := repo.UpdateStatus(ctxA, ghost, "running", ""); err == nil {
		t.Error("UpdateStatus 命中 0 行却返回成功：这一轮执行不在任何对账视野里，而调用方以为写成了")
	}
	at := time.Now().Add(time.Minute)
	if err := repo.SetNextRetryAt(ctxA, ghost, &at); err == nil {
		t.Error("SetNextRetryAt 命中 0 行却返回成功：日志宣称「已挂起、重启不丢」，而库里一个字段都没变")
	}
	if err := repo.SetNextRetryAt(ctxA, ghost, nil); err == nil {
		t.Error("SetNextRetryAt(nil) 命中 0 行却返回成功：取消挂起重试同样会静默落空")
	}
	// 合法形状：真实行上清除挂起重试（nil）必须是 1 行、不报错。
	taskSeed(t, repo, ctxA, taskIDA, taskUserA, "failed")
	if err := repo.SetNextRetryAt(ctxA, taskIDA, nil); err != nil {
		t.Errorf("清除自己任务的挂起重试失败: %v", err)
	}
}

// TestTaskOwnerScopedReadsAreTenantScoped 带 user 的读口：越权与跨租户都得读不到。
func TestTaskOwnerScopedReadsAreTenantScoped(t *testing.T) {
	repo, _, ctxA, ctxB := taskTenantDB(t)
	taskSeed(t, repo, ctxA, taskIDA, taskUserA, "draft")
	taskSeed(t, repo, ctxB, taskIDB, taskUserB, "draft")

	if _, err := repo.GetByID(ctxA, taskIDB, taskUserB); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("A 的 ctx 用 B 的 (id,user) 读到任务（err=%v）：租户谓词失效", err)
	}
	list, total, err := repo.List(ctxA, taskUserB, "", "", 1, 20)
	if err != nil || total != 0 || len(list) != 0 {
		t.Errorf("A 的 ctx 列 B 的用户任务：len=%d total=%d err=%v，期望 0/0/nil", len(list), total, err)
	}
	// 同一 user_id 出现在两个租户名下（跨租户迁移/复制出来的形状）：各自只看各自的。
	if _, err := repo.GetByID(ctxA, taskIDA, taskUserA); err != nil {
		t.Errorf("A 读自己的任务失败: %v", err)
	}
	if _, err := repo.GetByID(context.Background(), taskIDA, taskUserA); err == nil {
		t.Error("裸 ctx 的 GetByID 未被拒绝")
	}
}

// TestDependencyReadHasNoCrossTenantLeak `GetByIDAnyUser` 是依赖检查专用、刻意不带 user 过滤：
// 租户谓词是它唯一的遮挡。别人租户的前置任务不能被判成「存在」。
func TestDependencyReadHasNoCrossTenantLeak(t *testing.T) {
	repo, _, ctxA, ctxB := taskTenantDB(t)
	taskSeed(t, repo, ctxB, taskIDB, taskUserB, "done")
	if dep, err := repo.GetByIDAnyUser(ctxA, taskIDB); err == nil {
		t.Errorf("A 的依赖检查读到 B 的任务 %+v：前置存在性与终态跨租户外泄", dep)
	}
	if dep, err := repo.GetByIDAnyUser(ctxB, taskIDB); err != nil || dep.Status != "done" {
		t.Errorf("B 自己的依赖检查失败: dep=%+v err=%v", dep, err)
	}
	// 反向：A 用 system scope 能读到 B 的行——说明上面那条红是租户谓词造成的，不是行不存在。
	if _, err := repo.GetByIDAnyUser(tenant.WithSystemScope(context.Background()), taskIDB); err != nil {
		t.Errorf("system scope 读不到 B 的任务（夹具不成立）: %v", err)
	}
}

// TestTaskReconcileAndRetryClaimScopes 两条后台腿的**相反**口径各钉一次：
//   - 对账必须跨租户（用 A 的 ctx 收敛 B 的砖化任务必须写不动，而 system scope 必须写得动）；
//   - 重试认领必须留在本租户（A 的 ctx 不得认领 B 的到期重试，否则在自己的 registry 上
//     烧掉别人的 MaxRetryTimes 额度，见 repository/task.go 的归属门注释）。
func TestTaskReconcileAndRetryClaimScopes(t *testing.T) {
	repo, db, ctxA, ctxB := taskTenantDB(t)
	taskSeed(t, repo, ctxA, taskIDA, taskUserA, "running")
	taskSeed(t, repo, ctxB, taskIDB, taskUserB, "running")

	// 把两行的 updated_at 推到对账下限之前（谓词带 id，不跨租户位移）。
	old := time.Now().Add(-time.Hour)
	for _, id := range []uint{taskIDA, taskIDB} {
		if err := db.WithContext(tenant.WithSystemScope(context.Background())).
			Model(&model.BrowserTask{}).Where("id = ?", id).
			Update("updated_at", old).Error; err != nil {
			t.Fatalf("位移 updated_at 失败 task=%d: %v", id, err)
		}
	}

	sysCtx := tenant.WithSystemScope(context.Background())
	stale, err := repo.FindStaleRunningAll(sysCtx, 10*time.Minute, 50)
	if err != nil {
		t.Fatalf("system scope 对账扫描失败: %v", err)
	}
	if len(stale) != 2 {
		t.Errorf("对账扫描得 %d 行，期望 2（跨租户全量收敛；只扫到本租户就是永久砖化）", len(stale))
	}
	if mine, err := repo.FindStaleRunningAll(ctxA, 10*time.Minute, 50); err != nil || len(mine) != 1 {
		t.Errorf("用 A 的 ctx 扫得 %d 行 err=%v，期望 1（在线 ctx 只能看见自己的分片）", len(mine), err)
	}

	// 认领腿：两租户各挂一次到期重试。
	due := old
	if err := repo.SetNextRetryAt(ctxA, taskIDA, &due); err != nil {
		t.Fatalf("A 挂自己的重试失败: %v", err)
	}
	if err := repo.SetNextRetryAt(ctxB, taskIDB, &due); err != nil {
		t.Fatalf("B 挂自己的重试失败: %v", err)
	}
	// 认领要求 status='failed'（状态门），先把两行都置成 failed。
	if err := repo.UpdateStatus(ctxA, taskIDA, "failed", ""); err != nil {
		t.Fatalf("A 置 failed 失败: %v", err)
	}
	if err := repo.UpdateStatus(ctxB, taskIDB, "failed", ""); err != nil {
		t.Fatalf("B 置 failed 失败: %v", err)
	}
	claimed, err := repo.ClaimDueRetries(ctxA, time.Now(), 10, nil)
	if err != nil {
		t.Fatalf("A 的重试认领失败: %v", err)
	}
	if len(claimed) != 1 || claimed[0].ID != taskIDA {
		t.Errorf("A 认领到 %+v，期望只含 task=%d（跨租户认领即在自己的 registry 上烧别人的重试额度）", claimed, taskIDA)
	}
	if b := taskPhysical(t, db, taskIDB); b.NextRetryAt == nil {
		t.Error("B 的挂起重试被 A 的认领清掉了")
	}
	if _, err := repo.ClaimDueRetries(context.Background(), time.Now(), 10, nil); err == nil {
		t.Error("裸 ctx 的重试认领未被拒绝")
	}
}
