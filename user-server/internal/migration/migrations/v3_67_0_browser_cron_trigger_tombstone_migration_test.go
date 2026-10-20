package migrations

// v3.67.0 摘的是 browser_cron_triggers 里的软删残留行。
//
// 这些行是上一版缺陷留下的疤：删除走软删，而 (tenant_id, task_id) 上的唯一键位不认软删——
// 行还占着位，重名预检查（带 deleted_at IS NULL）却读不到它，于是「删掉再建一个」必然撞约束。
// 修了删除侧只挡住新疤，已有的 tombstone 仍把那些任务永久锁死（开发库实测 52 行 tombstone、
// 50 条 cron 任务因此挂不上触发器）。本迁移补这一刀：把占位的死行清掉，键位还给任务。
//
// 四句承诺各钉一条腿，缺一条就退化成「日志好看但事没做成」：
//   - 只摘软删行，活行逐字不动（尤其 tenant_id 不许被顺手改成默认租户）；
//   - 跨租户都摘（迁移跑在 system scope，本就不该按单一租户收手）；
//   - 摘完键位真的空出来（同 (tenant, task) 能再插一条活行——这才是用户看得见的那件事）；
//   - 重复执行不炸（幂等，第二次是 0 行的正常收口），表还没建时也不报错。

import (
	"context"
	"testing"

	browsermodel "hivemtk-user/internal/browser_automation/model"
	"hivemtk-user/internal/migration"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

const (
	pruneTenantA = uint(11)
	pruneTenantB = uint(22)
	// 与既有夹具的 id 段错开（7822xx 属 retention_a6，7823xx 属 fanout 腿）
	pruneTaskLiveA = uint(792411)
	pruneTaskDeadA = uint(792412)
	pruneTaskLiveB = uint(792413)
	pruneTaskDeadB = uint(792414)
)

// pruneSystemCtx 与迁移执行器同源的 ctx（internal/migration/service.go:102）。
// 夹具在 on 模式下读写必须带它，否则隔离层先 fail-closed，测不到迁移本身。
func pruneSystemCtx() context.Context { return tenant.WithSystemScope(context.Background()) }

// newCronTombstoneFixture 建表并造四行：两个租户各一活一软删。
// 软删行必须真的带 deleted_at（走 GORM 软删 API 造，不是手搓 UPDATE），
// 否则本迁移空跑也能被读成收敛成功。
func newCronTombstoneFixture(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.NewTestDB(t, &browsermodel.BrowserCronTrigger{})
	if db == nil {
		t.Fatal("测试库不可达：残留行是否摘干净无法判定（不 Skip，跳过等于没锁）")
	}
	t.Cleanup(func() {
		_ = db.WithContext(pruneSystemCtx()).Unscoped().Where("task_id IN ?", []uint{
			pruneTaskLiveA, pruneTaskDeadA, pruneTaskLiveB, pruneTaskDeadB,
		}).Delete(&browsermodel.BrowserCronTrigger{}).Error
	})

	seed := []struct {
		taskID uint
		tenID  uint
		dead   bool
	}{
		{pruneTaskLiveA, pruneTenantA, false},
		{pruneTaskDeadA, pruneTenantA, true},
		{pruneTaskLiveB, pruneTenantB, false},
		{pruneTaskDeadB, pruneTenantB, true},
	}
	for _, s := range seed {
		// 行自证 tenant_id（迁移跑在 system scope，Create 不自动填充）：
		// 租户归属由夹具写清楚，同时也验了「迁移不会把行改写成默认租户」。
		ctx := tenant.OnlyTenant(pruneSystemCtx(), s.tenID)
		tr := &browsermodel.BrowserCronTrigger{
			TenantID: s.tenID, TaskID: s.taskID,
			CronExpr: "0 0 1 1 *", Enabled: true,
		}
		if err := db.WithContext(ctx).Create(tr).Error; err != nil {
			t.Fatalf("种子触发行落库失败：%v", err)
		}
		if s.dead {
			if err := db.WithContext(ctx).Delete(tr).Error; err != nil {
				t.Fatalf("把种子行压成软删失败：%v", err)
			}
			var got browsermodel.BrowserCronTrigger
			if err := db.WithContext(pruneSystemCtx()).Unscoped().First(&got, tr.ID).Error; err != nil {
				t.Fatalf("回读软删行失败：%v", err)
			}
			if !got.DeletedAt.Valid {
				t.Fatalf("先决条件坏了：id=%d 期望是软删行，deleted_at 却是 NULL（本迁移将无事可做）", tr.ID)
			}
		}
	}
	return db
}

// countCronPhysical 物理行数（含软删）——键位占没占就看这个。
func countCronPhysical(t *testing.T, db *gorm.DB, taskID uint) int64 {
	t.Helper()
	var n int64
	if err := db.WithContext(pruneSystemCtx()).Unscoped().
		Model(&browsermodel.BrowserCronTrigger{}).
		Where("task_id = ?", taskID).Count(&n).Error; err != nil {
		t.Fatalf("数触发行失败：%v", err)
	}
	return n
}

func runCronTombstonePrune(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := NewBrowserCronTriggerTombstonePruneMigration(db).Up(pruneSystemCtx()); err != nil {
		t.Fatalf("Up 失败：%v", err)
	}
}

func TestCronTombstonePruneRemovesOnlyDeletedRowsAcrossTenants(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	db := newCronTombstoneFixture(t)
	// 先决条件：四行都在（两活两死），否则下面的「只剩活行」是被空库判绿的
	for _, taskID := range []uint{pruneTaskLiveA, pruneTaskDeadA, pruneTaskLiveB, pruneTaskDeadB} {
		if got := countCronPhysical(t, db, taskID); got != 1 {
			t.Fatalf("先决条件坏了：task=%d 物理行数 = %d，want 1", taskID, got)
		}
	}

	runCronTombstonePrune(t, db)

	// 软删行：两个租户都必须归零（跨租户收手 = 一半任务继续被锁死）
	for _, taskID := range []uint{pruneTaskDeadA, pruneTaskDeadB} {
		if got := countCronPhysical(t, db, taskID); got != 0 {
			t.Errorf("task=%d 的软删残留还剩 %d 行（物理），want 0：残留占着 (tenant_id, task_id) 唯一键位，"+
				"该任务永远挂不上新触发器", taskID, got)
		}
	}
	// 活行：必须原样留着，且租户归属没被顺手改写
	for taskID, wantTenant := range map[uint]uint{pruneTaskLiveA: pruneTenantA, pruneTaskLiveB: pruneTenantB} {
		var got browsermodel.BrowserCronTrigger
		if err := db.WithContext(pruneSystemCtx()).First(&got, "task_id = ?", taskID).Error; err != nil {
			t.Errorf("task=%d 的活触发器被摘掉了：%v（本迁移只该动 deleted_at IS NOT NULL 的行）", taskID, err)
			continue
		}
		if got.TenantID != wantTenant {
			t.Errorf("task=%d 的活触发器 tenant_id = %d，want %d：迁移不得改写租户归属", taskID, got.TenantID, wantTenant)
		}
	}

	// 幂等：再跑一次不该报错，也不该再动任何行
	before := countCronPhysical(t, db, pruneTaskLiveA)
	runCronTombstonePrune(t, db)
	if after := countCronPhysical(t, db, pruneTaskLiveA); after != before {
		t.Errorf("第二次 Up 动了活行：%d → %d", before, after)
	}
}

// 迁移要给用户的那件事：残留清掉后，同一个任务的键位真的能再插一条活行。
// 这条腿打在唯一约束上——只数行数会放过「行删了但键位没腾出来」这类想象中的失败。
func TestCronTombstonePruneFreesKeySlotForRecreate(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	db := newCronTombstoneFixture(t)
	runCronTombstonePrune(t, db)

	ctx := tenant.OnlyTenant(pruneSystemCtx(), pruneTenantA)
	if err := db.WithContext(ctx).Create(&browsermodel.BrowserCronTrigger{
		TenantID: pruneTenantA, TaskID: pruneTaskDeadA, CronExpr: "0 0 2 2 *", Enabled: true,
	}).Error; err != nil {
		t.Fatalf("摘完残留后仍挂不上新触发器：%v（键位没腾出来 = 用户依旧撞唯一约束）", err)
	}
	if got := countCronPhysical(t, db, pruneTaskDeadA); got != 1 {
		t.Errorf("task=%d 物理行数 = %d，want 1", pruneTaskDeadA, got)
	}
}

// 表还没建（全新部署首启动的迁移位）不该把启动判失败。
func TestCronTombstonePruneOnMissingTableIsNoop(t *testing.T) {
	db := testutil.NewTestDB(t)
	if db == nil {
		t.Fatal("测试库不可达")
	}
	if err := NewBrowserCronTriggerTombstonePruneMigration(db).Up(pruneSystemCtx()); err != nil {
		t.Errorf("表还不存在时 Up 返回 %v，want nil（全新部署不该因此停摆）", err)
	}
}

func TestCronTombstonePruneNilDBFailsClosed(t *testing.T) {
	err := NewBrowserCronTriggerTombstonePruneMigration(nil).Up(context.Background())
	if err == nil {
		t.Fatal("db 为 nil 时 Up 静默返回 nil：迁移会带着「已执行」的结论跳过真实数据")
	}
	if err := NewBrowserCronTriggerTombstonePruneMigration(nil).Down(context.Background()); err != nil {
		t.Errorf("Down 无操作却返回 %v，want nil", err)
	}
}

// 版本与在册登记：迁移没进 RegisterMigrations = 生产永远不跑这一刀
// （全链路迁移测试只跑注册表里的，漏注册不会红，只会静默不执行）。
func TestCronTombstonePruneIsRegistered(t *testing.T) {
	reg := migration.NewMigrationRegistry()
	RegisterMigrations(reg, nil)
	found := false
	for _, m := range reg.GetAll() {
		if m.Version() == "v3.67.0" {
			found = true
			break
		}
	}
	if !found {
		t.Error("v3.67.0 未注册进迁移表：存量残留行在生产不会被清理")
	}
}
