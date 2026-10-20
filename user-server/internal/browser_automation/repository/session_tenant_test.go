package repository

// 契约锁（实体审查 #10，TENANT_MODE=on）：browser_sessions 的每一条读写都站在租户地基上。
//
// 为什么这张表要单独钉：它是本域里**唯一由执行协程按主键直写**的表——
// `UpdateStatus/UpdateChromeTabID/UpdateMetrics/UpdateSnapshot/UpdateExtractedData/
// UpdateArtifacts` 六个写口的 WHERE 只有 `id = ?`，一个 user_id 条件都没有。
// 这是有意的：执行协程手里只有 session.ID，归属早在 RunTask 的
// `taskRepo.GetByID(taskID, userID)` 判过一次。于是租户谓词就成了这条链上
// **唯一还在生效的遮挡**——它一旦失效（漏绑 ctx / 被换成 system scope），
// 拿错行号的执行协程就能改别人的会话状态、把别人的快照与提取数据覆盖掉，
// 而库里看不出任何异常（写的都是合法列）。
//
// 另一头同样要钉：`HasSuccess`/`GetLatestByTaskID` 是依赖调度（checkDependency）的读口，
// 它们不带 user 过滤，靠租户谓词遮住别人的任务号；而对账器
// （stale_reconcile）恰恰**必须**跨租户——它跑在 system scope 下收敛全实例的僵尸会话。
// 这两种口径写反了都不会报错：前者泄别人的执行终态，后者让自己的租户以外永久砖化。
// 所以两侧各一条断言，而不是只测「读得到 / 读不到」。

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
	sessTenantA = uint(21)
	sessTenantB = uint(43)
	// 号段避开 session_duration_test.go 的 791xxx 与 outreach_receipt 的 792xxx。
	sessTaskA     = uint(793001)
	sessTaskB     = uint(793002)
	sessUserA     = uint(793101)
	sessUserB     = uint(793102)
	sessIDA       = uint(793201)
	sessIDB       = uint(793202)
	sessURL       = "https://www.xiaohongshu.com/explore"
	sessOtherTask = uint(793003)
)

func sessionTenantDB(t *testing.T) (BrowserSessionRepository, *gorm.DB, context.Context, context.Context) {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })
	db := testutil.NewTestDB(t, &model.BrowserSession{}, &model.BrowserTask{})
	if db == nil {
		t.Fatal("测试库不可达：会话行的租户归属无法判定（不 Skip，跳过等于没锁）")
	}
	return NewBrowserSessionRepositoryWithDB(db), db,
		tenant.WithTenantID(context.Background(), sessTenantA),
		tenant.WithTenantID(context.Background(), sessTenantB)
}

// sessSeed 落一行会话。ID 与 Status 显式给定，让断言能指着具体行号说话。
func sessSeed(t *testing.T, db *gorm.DB, ctx context.Context, id, taskID, userID uint, status string) {
	t.Helper()
	row := &model.BrowserSession{ID: id, TaskID: taskID, UserID: userID,
		Url: sessURL, Status: status}
	if err := db.WithContext(ctx).Create(row).Error; err != nil {
		t.Fatalf("会话 %d 落库失败: %v", id, err)
	}
}

// sessPhysical 跨租户物理计数（system scope 是本域对账器已有的合法通道）。
func sessPhysical(t *testing.T, db *gorm.DB, taskID uint) int64 {
	t.Helper()
	var n int64
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).
		Model(&model.BrowserSession{}).Where("task_id = ?", taskID).Count(&n).Error; err != nil {
		t.Fatalf("物理计数失败: %v", err)
	}
	return n
}

// TestSessionCreateFillsTenantAndRejectsBareContext 写侧：租户由 ctx 填，裸 ctx 必须被拒。
// 被拒而不是落进默认租户 1 —— 后者会让别人租户的执行记录混进 1 号租户的列表与并发闸。
func TestSessionCreateFillsTenantAndRejectsBareContext(t *testing.T) {
	repo, db, ctxA, _ := sessionTenantDB(t)
	row := &model.BrowserSession{TaskID: sessTaskA, UserID: sessUserA, Url: sessURL, Status: "created"}
	if err := repo.Create(ctxA, row); err != nil {
		t.Fatalf("A 的会话落库失败: %v", err)
	}
	if row.TenantID != sessTenantA {
		t.Errorf("落库后 tenant_id = %d，期望 %d（由隔离 callback 从 ctx 填）", row.TenantID, sessTenantA)
	}
	bare := &model.BrowserSession{TaskID: sessOtherTask, UserID: sessUserA, Url: sessURL, Status: "created"}
	if err := repo.Create(context.Background(), bare); err == nil {
		t.Error("裸 ctx 的会话落库未被拒绝")
	}
	if n := sessPhysical(t, db, sessOtherTask); n != 0 {
		t.Errorf("裸 ctx 被拒后仍留下 %d 行", n)
	}
}

// TestSessionByIDWritesAreTenantFactored 六个按主键直写的写口，在别人的租户名下必须写不动。
func TestSessionByIDWritesAreTenantFactored(t *testing.T) {
	repo, db, ctxA, ctxB := sessionTenantDB(t)
	sessSeed(t, db, ctxB, sessIDB, sessTaskB, sessUserB, "active")

	type write struct {
		name string
		run  func(context.Context, uint) error
	}
	writes := []write{
		{"UpdateStatus", func(c context.Context, id uint) error { return repo.UpdateStatus(c, id, "failed", "越权写") }},
		{"UpdateChromeTabID", func(c context.Context, id uint) error { return repo.UpdateChromeTabID(c, id, 99999) }},
		{"UpdateMetrics", func(c context.Context, id uint) error { return repo.UpdateMetrics(c, id, 9, 9, 0) }},
		{"UpdateSnapshot", func(c context.Context, id uint) error { return repo.UpdateSnapshot(c, id, "别人的页面快照") }},
		{"UpdateExtractedData", func(c context.Context, id uint) error {
			return repo.UpdateExtractedData(c, id, []byte(`{"who":"B"}`))
		}},
		{"UpdateArtifacts", func(c context.Context, id uint) error {
			return repo.UpdateArtifacts(c, id, []byte(`{"who":"B"}`), "https://cdn/x.png", "别人的总结")
		}},
	}
	for _, w := range writes {
		if err := w.run(ctxA, sessIDB); err != nil {
			t.Errorf("%s 用 A 的 ctx 写 B 的行失败: %v（期望被租户谓词挡成 0 行，而不是报错）", w.name, err)
		}
	}
	var got model.BrowserSession
	if err := db.WithContext(ctxB).First(&got, sessIDB).Error; err != nil {
		t.Fatalf("回读 B 的会话失败: %v", err)
	}
	if got.Status != "active" {
		t.Errorf("B 的会话状态被改成 %q，期望仍是 active", got.Status)
	}
	if got.Snapshot != "" || got.TotalSteps != 0 || got.ChromeTabID != 0 || got.LlmSummary != "" {
		t.Errorf("B 的会话内容列被越权写覆盖：snapshot=%q steps=%d tab=%d summary=%q",
			got.Snapshot, got.TotalSteps, got.ChromeTabID, got.LlmSummary)
	}
	if len(got.ExtractedData) != 0 {
		t.Errorf("B 的 extracted_data 被越权写成 %s", string(got.ExtractedData))
	}

	// 同一批写口在裸 ctx 上一律 fail-closed：执行协程丢了租户时，宁可写不动也不能盲写。
	for _, w := range writes {
		if err := w.run(context.Background(), sessIDB); err == nil {
			t.Errorf("%s 在裸 ctx 下未被拒绝", w.name)
		}
	}
}

// TestSessionOwnerScopedReadsAreTenantScoped 带 user/task 过滤的读口：过滤条件挡不住
// 跨租户的同名读数（尤其 ListByTaskID 的 task_id 是调用方从 URL 拿来的），租户谓词是第二层。
func TestSessionOwnerScopedReadsAreTenantScoped(t *testing.T) {
	repo, db, ctxA, ctxB := sessionTenantDB(t)
	sessSeed(t, db, ctxA, sessIDA, sessTaskA, sessUserA, "completed")
	sessSeed(t, db, ctxB, sessIDB, sessTaskA, sessUserB, "completed")

	if _, err := repo.GetByID(ctxA, sessIDB, sessUserB); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("A 拿 B 的 user_id 去取 B 的会话行 = %v，期望 ErrRecordNotFound", err)
	}
	if _, err := repo.GetByID(ctxB, sessIDA, sessUserA); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("B 拿 A 的 user_id 去取 A 的会话行 = %v，期望 ErrRecordNotFound", err)
	}
	// 同一 task_id 挂在两个租户名下：各读各的执行历史。
	listA, totalA, err := repo.ListByTaskID(ctxA, sessTaskA, sessUserA, 1, 10)
	if err != nil || totalA != 1 || len(listA) != 1 || listA[0].ID != sessIDA {
		t.Errorf("A 的任务执行历史 = %+v/%d（err=%v），期望恰好自己那 1 行", listA, totalA, err)
	}
	listB, totalB, err := repo.ListByTaskID(ctxB, sessTaskA, sessUserB, 1, 10)
	if err != nil || totalB != 1 || len(listB) != 1 || listB[0].ID != sessIDB {
		t.Errorf("B 的任务执行历史 = %+v/%d（err=%v），期望恰好自己那 1 行", listB, totalB, err)
	}
	// A 拿 B 的 user_id 查：user 过滤 + 租户过滤都成立 ⇒ 空。
	if _, n, err := repo.ListByTaskID(ctxA, sessTaskA, sessUserB, 1, 10); err != nil || n != 0 {
		t.Errorf("A 用 B 的 user_id 查任务历史得到 %d 条（err=%v），期望 0", n, err)
	}
	if _, _, err := repo.ListByTaskID(context.Background(), sessTaskA, sessUserA, 1, 10); err == nil {
		t.Error("裸 ctx 的任务历史查询未被拒绝")
	}
	// 并发闸：同名的 user_id 落在两个租户时各自计数，不互相顶掉名额。
	cntA, err := repo.CountRunningByUser(ctxA, sessUserA)
	if err != nil || cntA != 0 {
		t.Errorf("A 的在途会话数 = %d（err=%v），期望 0（completed 不占名额）", cntA, err)
	}
	if _, err := repo.CountRunningByUser(context.Background(), sessUserA); err == nil {
		t.Error("裸 ctx 的并发闸计数未被拒绝")
	}
}

// TestSessionDependencyReadsStayInTenant checkDependency 的两个无 user 过滤读口：
// 别人的任务号恰好撞上前置任务号时，不能把别人的执行终态透进本用户的错误文案。
func TestSessionDependencyReadsStayInTenant(t *testing.T) {
	repo, db, ctxA, ctxB := sessionTenantDB(t)
	// 前置任务号在两个租户都存在：A 的那条从未成功（failed），B 的那条成功过。
	sessSeed(t, db, ctxA, sessIDA, sessTaskB, sessUserA, "failed")
	sessSeed(t, db, ctxB, sessIDB, sessTaskB, sessUserB, "completed")

	if ok, err := repo.HasSuccess(ctxA, sessTaskB); err != nil || ok {
		t.Errorf("A 的前置成功判定 = %v（err=%v），期望 false：A 名下只有 failed", ok, err)
	}
	if ok, err := repo.HasSuccess(ctxB, sessTaskB); err != nil || !ok {
		t.Errorf("B 的前置成功判定 = %v（err=%v），期望 true", ok, err)
	}
	last, err := repo.GetLatestByTaskID(ctxA, sessTaskB)
	if err != nil || last == nil || last.ID != sessIDA {
		t.Errorf("A 的前置最近一次执行 = %+v（err=%v），期望只有自己那行", last, err)
	}
	if last.Status != "failed" {
		t.Errorf("A 读到的前置状态 = %q，期望 failed（B 的 completed 不能进来，否则 A 的 all_done 依赖被别人的成功判成满足）", last.Status)
	}
	if _, err := repo.HasSuccess(context.Background(), sessTaskB); err == nil {
		t.Error("裸 ctx 的前置成功判定未被拒绝")
	}
	if _, err := repo.GetLatestByTaskID(context.Background(), sessTaskB); err == nil {
		t.Error("裸 ctx 的前置最近执行读取未被拒绝")
	}
}

// TestSessionReconcileWritesArePlatformWide 对账器一侧的口径与调度读口相反，必须钉住：
// FailStaleUnfinished 跑在 tenant.WithSystemScope 下收敛**全租户**的僵尸会话，
// 改成租户内就等于别的租户永久砖化；而它同时是并发闸的释放口，收不干净会一直挡新执行。
func TestSessionReconcileWritesArePlatformWide(t *testing.T) {
	repo, db, ctxA, ctxB := sessionTenantDB(t)
	sessSeed(t, db, ctxA, sessIDA, sessTaskA, sessUserA, "active")
	sessSeed(t, db, ctxB, sessIDB, sessTaskB, sessUserB, "active")
	before := time.Now().Add(time.Minute)

	sys := tenant.WithSystemScope(context.Background())
	n, err := repo.FailStaleUnfinished(sys, sessTaskA, before, "对账收口")
	if err != nil || n != 1 {
		t.Fatalf("system scope 下对 A 任务的僵尸会话收口得到 %d 行（err=%v），期望 1", n, err)
	}
	var a model.BrowserSession
	if err := db.WithContext(ctxA).First(&a, sessIDA).Error; err != nil {
		t.Fatalf("回读 A 的会话失败: %v", err)
	}
	if a.Status != "failed" || a.ErrorMsg != "对账收口" {
		t.Errorf("A 的僵尸会话未被对账收口: status=%q err=%q", a.Status, a.ErrorMsg)
	}
	// A 的并发闸随之释放：在途数必须归零。
	if c, err := repo.CountRunningByTask(ctxA, sessUserA, sessTaskA); err != nil || c != 0 {
		t.Errorf("收口后 A 的在途数 = %d（err=%v），期望 0", c, err)
	}
	// 同一次 system scope 扫描里 B 的任务是另一行判据：不受 A 的收口牵连。
	if c, err := repo.CountRunningByUser(ctxB, sessUserB); err != nil || c != 1 {
		t.Errorf("B 的在途会话数 = %d（err=%v），期望仍是 1", c, err)
	}
	// 反向：漏掉 system scope 的租户 ctx 只能结自己租户的行——
	// 这条钉住「对账器一旦忘了 WithSystemScope 就静默只收敛发起租户」。
	if n, err := repo.FailStaleUnfinished(ctxA, sessTaskB, before, "越租户收口"); err != nil || n != 0 {
		t.Errorf("用 A 的 ctx 收 B 任务得到 %d 行（err=%v），期望 0", n, err)
	}
}

// TestSessionListInFlightByTaskNeedsBoundCtx 任务删除级联的在途清单：它是唯一
// 「不带 user_id 条件」的读口（归属已由任务侧 SoftDelete 判过一次），
// 所以租户谓词是它唯一的遮挡——裸 ctx 必须被拒，而不是拿到全实例的在途会话。
func TestSessionListInFlightByTaskNeedsBoundCtx(t *testing.T) {
	repo, db, ctxA, ctxB := sessionTenantDB(t)
	sessSeed(t, db, ctxA, sessIDA, sessTaskA, sessUserA, "active")
	sessSeed(t, db, ctxB, sessIDB, sessTaskB, sessUserB, "active")

	list, err := repo.ListInFlightByTask(ctxA, sessTaskA)
	if err != nil || len(list) != 1 || list[0].ID != sessIDA {
		t.Errorf("A 的在途清单 = %+v（err=%v），期望恰好 sessIDA", list, err)
	}
	if _, err := repo.ListInFlightByTask(context.Background(), sessTaskA); err == nil {
		t.Error("裸 ctx 的在途清单未被拒绝：删除级联会把别人的会话一起收口")
	}
}
