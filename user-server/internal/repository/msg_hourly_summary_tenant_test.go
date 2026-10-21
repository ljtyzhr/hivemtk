// msg_hourly_summary_tenant_test.go 聚合水位线与小时汇总的租户契约。
//
// 水位线主键已升级为 (tenant_id, source)：各租户消费游标互不越线。
// 汇总表由 map Create + ON CONFLICT 写入（无模型字段可填），这里同时钉住
// 隔离 callback 对 map 写入的 tenant_id 填充不会被 upsert 路径绕过。
package repository

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

const (
	mhsTenantA = uint(97)
	mhsTenantB = uint(98)
)

func mhsSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t,
		&model.AggregationWatermark{},
		&model.MsgHourlySummary{},
		&model.MessageHub{},
	)
	if db == nil {
		t.Fatal("测试库不可达：租户遮挡判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func mhsSystemCtx() context.Context {
	return tenant.WithSystemScope(context.Background())
}

func TestAggregationWatermarkRepository_TenantScopedUpsert(t *testing.T) {
	db := mhsSetupDB(t)
	repo := NewMessageHubSummaryRepository(db)
	ctxA := tenant.WithTenantID(context.Background(), mhsTenantA)
	ctxB := tenant.WithTenantID(context.Background(), mhsTenantB)

	if wm, err := repo.LoadWatermark(ctxA, model.SummarySourceMessageHub); err != nil || wm != 0 {
		t.Fatalf("初始水位线期望 (0,nil)，实际 (%d,%v)", wm, err)
	}
	if err := repo.UpsertIncrementBatch(ctxA, model.SummarySourceMessageHub, 10, nil); err != nil {
		t.Fatalf("租户 A 推进水位线失败: %v", err)
	}
	if wm, err := repo.LoadWatermark(ctxB, model.SummarySourceMessageHub); err != nil || wm != 0 {
		t.Fatalf("租户 B 不应看到 A 的水位线，实际 (%d,%v)", wm, err)
	}

	if err := repo.UpsertIncrementBatch(ctxB, model.SummarySourceMessageHub, 20, nil); err != nil {
		t.Fatalf("租户 B 推进水位线失败: %v", err)
	}
	if wm, _ := repo.LoadWatermark(ctxA, model.SummarySourceMessageHub); wm != 10 {
		t.Fatalf("租户 A 水位线被 B 覆盖: %d", wm)
	}

	var rows []model.AggregationWatermark
	if err := db.WithContext(mhsSystemCtx()).Find(&rows).Error; err != nil {
		t.Fatalf("读取全量水位线失败: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("期望两租户各一行水位线，实际 %d 行: %+v", len(rows), rows)
	}
}

func TestAggregationWatermarkRepository_SummaryDeltaTenantScoped(t *testing.T) {
	db := mhsSetupDB(t)
	repo := NewMessageHubSummaryRepository(db)
	ctxA := tenant.WithTenantID(context.Background(), mhsTenantA)
	ctxB := tenant.WithTenantID(context.Background(), mhsTenantB)
	bucket := time.Now().Truncate(time.Hour)

	delta := func(n int64) []MsgHourlyDelta {
		return []MsgHourlyDelta{{
			HourBucket:   bucket,
			Platform:     "whatsapp",
			SessionCount: n,
			MessageCount: n * 10,
		}}
	}
	if err := repo.UpsertIncrementBatch(ctxA, model.SummarySourceMessageHub, 1, delta(3)); err != nil {
		t.Fatalf("租户 A 汇总失败: %v", err)
	}
	if err := repo.UpsertIncrementBatch(ctxB, model.SummarySourceMessageHub, 1, delta(5)); err != nil {
		t.Fatalf("租户 B 汇总失败（同 bucket/platform 必须能各存一行）: %v", err)
	}

	rowsA, err := repo.QuerySince(ctxA, bucket.Add(-time.Minute))
	if err != nil {
		t.Fatalf("租户 A 查询失败: %v", err)
	}
	if len(rowsA) != 1 || rowsA[0].SessionCount != 3 || rowsA[0].TenantID != mhsTenantA {
		t.Fatalf("租户 A 汇总被串写: %+v", rowsA)
	}
	rowsB, err := repo.QuerySince(ctxB, bucket.Add(-time.Minute))
	if err != nil {
		t.Fatalf("租户 B 查询失败: %v", err)
	}
	if len(rowsB) != 1 || rowsB[0].SessionCount != 5 || rowsB[0].TenantID != mhsTenantB {
		t.Fatalf("租户 B 汇总被串写: %+v", rowsB)
	}
}

func TestAggregationWatermarkRepository_LoadBatchSinceTenantScoped(t *testing.T) {
	db := mhsSetupDB(t)
	now := time.Now()
	for _, spec := range []struct {
		tenantID uint
		msgID    string
	}{
		{mhsTenantA, "a-1"},
		{mhsTenantA, "a-2"},
		{mhsTenantB, "b-1"},
	} {
		row := &model.MessageHub{
			TenantID:       spec.tenantID,
			Platform:       "whatsapp",
			MsgID:          spec.msgID,
			AccountID:      "acc",
			Direction:      "inbound",
			Status:         "done",
			MsgType:        "text",
			ConversationID: spec.msgID,
			SentAt:         now,
		}
		if err := db.WithContext(mhsSystemCtx()).Create(row).Error; err != nil {
			t.Fatalf("预置 message_hub 失败: %v", err)
		}
	}

	repo := NewMessageHubSummaryRepository(db)
	rows, err := repo.LoadBatchSince(tenant.WithTenantID(context.Background(), mhsTenantA), 0, 100)
	if err != nil {
		t.Fatalf("LoadBatchSince 失败: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("期望仅租户 A 的 2 条原始消息，实际 %d 条", len(rows))
	}
	for _, r := range rows {
		if r.TenantID != mhsTenantA {
			t.Fatalf("跨租户消息泄漏进聚合批次: tenant_id=%d msg_id=%s", r.TenantID, r.MsgID)
		}
	}
}
