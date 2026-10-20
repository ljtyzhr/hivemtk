package repository

// 契约锁（实体审查 #3，TENANT_MODE=on）：治理裁剪按租户分片，一次扫描只动得上一个租户的行。
//
// 为什么这一格必须单独钉：`PruneBefore` 的调用方是后台 goroutine，拿的是
// `context.Background()`（见 router/browser_automation_routes.go 的 bootstrap），
// 不是某个请求的 ctx。saas 形态下这条路上有两种失效，且都不报错得很好看：
//   - ctx 未绑租户 ⇒ 隔离 callback fail-closed ⇒ 裁剪整体停摆（service 层用 Fanout 修，
//     本包这条腿证的是 Fanout 之后**每个租户各自裁各自的**）；
//   - 若裁剪语句本身不接隔离 ⇒ 一次扫描把全部租户的审计行一起带走，而摘要与留痕只写了
//     发起方的租户号——被删的那些行连「存在过」都没留下，正是 A6 立项要防的那件事。

import (
	"context"
	"strconv"
	"testing"
	"time"

	"hivemtk-user/internal/browser_automation/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	pruneTenantA = uint(11)
	pruneTenantB = uint(32)
	// 不与 a6* 的 session 号段重叠：并行残留会改变批次切分（口径同 a6SessionLate 的注释）。
	pruneSessionA = uint(782301)
	pruneSessionB = uint(782302)
	pruneSource   = "test:retention_tenant_fanout"
)

// pruneSeedExpired 以指定租户 ctx 落若干命令日志并推到保留期之前。
// 种子与位移都必须带 ctx：on 模式下裸 ctx 写租户表会被 fail-closed 拒掉，
// 而这里的种子是「被测事实」的一部分，不是夹具捷径。
func pruneSeedExpired(t *testing.T, db *gorm.DB, ctx context.Context, sessionID uint, n int) {
	t.Helper()
	rows := make([]*model.BrowserCommandLog, 0, n)
	for seq := 1; seq <= n; seq++ {
		rows = append(rows, &model.BrowserCommandLog{
			SessionID: sessionID, TaskID: a6Task, Seq: seq,
			Direction: "command", Action: "click",
			Payload: datatypes.JSON([]byte(`{"x":` + strconv.Itoa(seq) + `}`)),
		})
	}
	if err := db.WithContext(ctx).Create(rows).Error; err != nil {
		t.Fatalf("租户 ctx 种子命令日志失败: %v", err)
	}
	if err := db.WithContext(ctx).Model(&model.BrowserCommandLog{}).
		Where("session_id = ?", sessionID).
		Update("created_at", a6Cutoff().Add(-24*time.Hour)).Error; err != nil {
		t.Fatalf("位移 created_at 失败: %v", err)
	}
}

func pruneCountByTenant(t *testing.T, db *gorm.DB, tid uint) int64 {
	t.Helper()
	var n int64
	err := db.WithContext(tenant.WithTenantID(context.Background(), tid)).
		Model(&model.BrowserCommandLog{}).Count(&n).Error
	if err != nil {
		t.Fatalf("计数失败: %v", err)
	}
	return n
}

// TestPruneBeforeTouchesOnlyItsOwnTenant 租户 11 的扫描只裁 11 的行：
// 32 的行、摘要、留痕都不许被这次扫描动过。
func TestPruneBeforeTouchesOnlyItsOwnTenant(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	db := testutil.NewTestDB(t, &model.BrowserCommandLog{}, &model.BrowserAuditDigest{}, &model.BrowserAuditPruneRun{})
	if db == nil {
		t.Fatal("测试库不可达")
	}
	ctxA := tenant.WithTenantID(context.Background(), pruneTenantA)
	ctxB := tenant.WithTenantID(context.Background(), pruneTenantB)
	pruneSeedExpired(t, db, ctxA, pruneSessionA, 3)
	pruneSeedExpired(t, db, ctxB, pruneSessionB, 5)

	pruned, err := NewBrowserCommandLogRepositoryWithDB(db).PruneBefore(ctxA, a6Cutoff(), pruneSource)
	if err != nil {
		t.Fatalf("租户 11 的裁剪报错: %v", err)
	}
	if pruned != 3 {
		t.Errorf("裁剪行数 got=%d want=3（只该裁发起租户那 3 行）", pruned)
	}
	if got := pruneCountByTenant(t, db, pruneTenantA); got != 0 {
		t.Errorf("租户 11 剩余行 got=%d want=0", got)
	}
	if got := pruneCountByTenant(t, db, pruneTenantB); got != 5 {
		t.Errorf("租户 32 剩余行 got=%d want=5（被别的租户裁掉了＝跨租户销毁审计事实）", got)
	}

	// 摘要与留痕同样不得跨租户：它们描述的是「被删掉的那些行」，混进 32 的行就成了假凭据。
	var digests []*model.BrowserAuditDigest
	if err := db.WithContext(ctxA).Find(&digests).Error; err != nil {
		t.Fatalf("读摘要失败: %v", err)
	}
	if len(digests) != 1 || digests[0].RowCount != 3 || digests[0].SessionID != pruneSessionA {
		t.Errorf("租户 11 的摘要 got=%+v want 一条 3 行、session=%d", digests, pruneSessionA)
	}
	var digestsB int64
	if err := db.WithContext(ctxB).Model(&model.BrowserAuditDigest{}).Count(&digestsB).Error; err != nil {
		t.Fatalf("读租户 32 摘要失败: %v", err)
	}
	if digestsB != 0 {
		t.Errorf("租户 32 的摘要行数 got=%d want=0（替别人落摘要=描述不实的凭据）", digestsB)
	}

	var runs []*model.BrowserAuditPruneRun
	if err := db.WithContext(ctxA).Order("id ASC").Find(&runs).Error; err != nil {
		t.Fatalf("读留痕失败: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("租户 11 留痕行数 got=%d want=1", len(runs))
	}
	if runs[0].TenantID != pruneTenantA {
		t.Errorf("留痕 tenant_id got=%d want=%d（隔离 callback 必须在 Create 时填上发起租户）",
			runs[0].TenantID, pruneTenantA)
	}
	if runs[0].CutoffSource != pruneSource {
		t.Errorf("留痕 cutoff_source got=%q want=%q", runs[0].CutoffSource, pruneSource)
	}
}

// TestPruneBeforeRejectsUnboundContext on 模式裸 ctx 必须 fail-closed 报错且一行不删。
// 这条腿钉的是 service 层那道 Fanout 的**必要性**：少了扇出，后台裁剪不会静默少删，
// 而是每小时报错、整个保留期治理停摆（command_log 无界增长＝G19 立项要防的事）。
func TestPruneBeforeRejectsUnboundContext(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	db := testutil.NewTestDB(t, &model.BrowserCommandLog{}, &model.BrowserAuditDigest{}, &model.BrowserAuditPruneRun{})
	if db == nil {
		t.Fatal("测试库不可达")
	}
	ctxA := tenant.WithTenantID(context.Background(), pruneTenantA)
	pruneSeedExpired(t, db, ctxA, pruneSessionA, 2)

	if _, err := NewBrowserCommandLogRepositoryWithDB(db).
		PruneBefore(context.Background(), a6Cutoff(), pruneSource); err == nil {
		t.Fatal("裸 ctx 的裁剪未被拒绝（未绑租户即扫全表）")
	}
	if got := pruneCountByTenant(t, db, pruneTenantA); got != 2 {
		t.Errorf("被拒的裁剪仍删了行：剩余 got=%d want=2", got)
	}
}
