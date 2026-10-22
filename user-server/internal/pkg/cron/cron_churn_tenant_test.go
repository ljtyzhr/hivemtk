// cron_churn_tenant_test.go 流失预测定时任务的租户扇出契约。
//
// 背景（任务三逐路由 curl 实测）：ChurnCalculationCron 原先直接用
// context.Background() 调 RunChurnCalculationForAllCustomers，而同文件的活码
// 轮询 / 过期令牌清理都走 fanoutTask（按活跃租户逐个绑定 ctx）。
//
// 而 customer_events / churn_predictions 都带 tenant_id 列：on 模式下
// ctx 未绑定租户即被隔离守卫 fail-closed 拒绝 ⇒ 整轮流失计算读不到任何行为
// 数据、算不出任何预测。生产表现是「流失预测页面永远是空的」，而接口层
// 完全无感（失败只落在 cron 的一行日志里）。
//
// 判据：注入一个活跃租户，给它种一条客户行为事件，跑完 cron 之后，
// 该租户名下必须落一条 churn_predictions（读数 0 ⇒ 扇出没生效，
// 被守卫拒了）。
package cron

import (
	"testing"
	"time"

	"hivemtk-user/internal/model"
	opsmodel "hivemtk-user/internal/ops/model"
	pkgdb "hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"github.com/google/uuid"
)

func TestChurnCalculationCron_LandsInEachActiveTenant(t *testing.T) {
	withActiveTenants(t, 12)

	gdb := testutil.NewTestDB(t, &model.CustomerEvent{}, &opsmodel.ChurnPrediction{}, &opsmodel.ChurnModelConfig{})
	if gdb == nil {
		t.Fatal("测试库不可达：流失预测的租户扇出判不了（不 Skip，跳过等于没锁）")
	}
	prevDB := pkgdb.GetDB()
	pkgdb.SetTestDB(gdb)
	t.Cleanup(func() { pkgdb.SetTestDB(prevDB) })

	ctx12 := tenant.WithTenantID(t.Context(), 12)
	event := &model.CustomerEvent{
		ID:          uuid.NewString(),
		CustomerID:  "churn-cron-customer-12",
		EventType:   model.EventTypePurchase,
		EventSource: model.EventSourceWebsite,
		EventData:   "{}",
		OccurredAt:  time.Now().Add(-72 * time.Hour),
	}
	if err := gdb.WithContext(ctx12).Create(event).Error; err != nil {
		t.Fatalf("seed 客户行为事件失败: %v", err)
	}

	ChurnCalculationCron()

	var preds []opsmodel.ChurnPrediction
	// churn_predictions 的列名是 user_id（行为快照的 customer_id 落进这一列）
	if err := gdb.WithContext(ctx12).Where("user_id = ?", event.CustomerID).Find(&preds).Error; err != nil {
		t.Fatalf("读回流失预测失败（若为 [tenant] fail-closed 报错即扇出未生效）: %v", err)
	}
	if len(preds) != 1 {
		t.Fatalf("流失预测数=%d want 1（0 表示聚合查询被守卫拒绝，即扇出未生效）", len(preds))
	}
	if preds[0].TenantID != 12 {
		t.Errorf("预测行租户归属=%d want 12", preds[0].TenantID)
	}
}
