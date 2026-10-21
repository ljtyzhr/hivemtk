package migrations

// v3.68.0 修的是 A/B 实验结果表的键位：从 (tenant_id, experiment_id) 恢复为
// (tenant_id, experiment_id, variant_id)。
//
// 旧模型把 experiment_id 标成单列 uniqueIndex，AutoMigrate 在多租户库上落成
// 「每实验只能有一条结果」。CalculateResults 逐变体 Upsert，第二个变体必撞
// 23505，多变异实验的整体结果计算失败。用例必须钉住「迁移后能插第二个变体」，
// 只断言索引名存在会放过「索引建了但列不对」的假修复。

import (
	"context"
	"testing"

	"hivemtk-user/internal/migration"
	"hivemtk-user/internal/ops/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

const (
	abKeyTenantA = uint(91)
	abKeyExpID   = uint(601)
)

func abKeySystemCtx() context.Context { return tenant.WithSystemScope(context.Background()) }

// abKeyFixture 建表并把索引还原成 v3.68.0 之前的错误形态：
// (tenant_id, experiment_id) 唯一。同时落第一行结果。
func abKeyFixture(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.NewTestDB(t, &model.ABExperimentResult{})
	if db == nil {
		t.Fatal("测试库不可达：键位是否恢复无法判定（不 Skip，跳过等于没锁）")
	}
	t.Cleanup(func() {
		_ = db.WithContext(abKeySystemCtx()).
			Where("experiment_id = ?", abKeyExpID).
			Delete(&model.ABExperimentResult{}).Error
	})

	if err := db.Exec(`DROP INDEX IF EXISTS idx_ab_experiment_results_exp_variant`).Error; err != nil {
		t.Fatalf("删除现有复合索引失败: %v", err)
	}
	if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_ab_experiment_results_experiment_id
ON ab_experiment_results (tenant_id, experiment_id)`).Error; err != nil {
		t.Fatalf("重建旧单列唯一索引失败: %v", err)
	}

	ctx := tenant.OnlyTenant(abKeySystemCtx(), abKeyTenantA)
	if err := db.WithContext(ctx).Create(&model.ABExperimentResult{
		TenantID: abKeyTenantA, ExperimentID: abKeyExpID, VariantID: 1, VariantName: "Control",
	}).Error; err != nil {
		t.Fatalf("落第一行结果失败: %v", err)
	}
	return db
}

func TestABExperimentResultUniqueKeyMigrationAllowsSecondVariant(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	db := abKeyFixture(t)
	if err := NewABExperimentResultUniqueKeyMigration(db).Up(abKeySystemCtx()); err != nil {
		t.Fatalf("Up 失败: %v", err)
	}

	ctx := tenant.OnlyTenant(abKeySystemCtx(), abKeyTenantA)
	if err := db.WithContext(ctx).Create(&model.ABExperimentResult{
		TenantID: abKeyTenantA, ExperimentID: abKeyExpID, VariantID: 2, VariantName: "Treatment",
	}).Error; err != nil {
		t.Fatalf("迁移后第二个变体仍写不进去（键位没恢复）: %v", err)
	}

	var oldIndex, newIndex int64
	if err := db.Raw(`SELECT COUNT(*) FROM pg_indexes WHERE tablename = 'ab_experiment_results'
AND indexname = 'idx_ab_experiment_results_experiment_id'`).Scan(&oldIndex).Error; err != nil {
		t.Fatalf("查询旧索引失败: %v", err)
	}
	if oldIndex != 0 {
		t.Error("旧单列唯一索引仍在：第二个变体迟早再撞 23505")
	}
	if err := db.Raw(`SELECT COUNT(*) FROM pg_indexes WHERE tablename = 'ab_experiment_results'
AND indexname = 'idx_ab_experiment_results_exp_variant'`).Scan(&newIndex).Error; err != nil {
		t.Fatalf("查询复合索引失败: %v", err)
	}
	if newIndex != 1 {
		t.Error("复合唯一索引缺失：每实验每变体一行没有键位保护")
	}

	// 幂等：第二次 Up 不报错，行数不变。
	if err := NewABExperimentResultUniqueKeyMigration(db).Up(abKeySystemCtx()); err != nil {
		t.Fatalf("第二次 Up 失败: %v", err)
	}
	var rows int64
	if err := db.WithContext(abKeySystemCtx()).Model(&model.ABExperimentResult{}).
		Where("experiment_id = ?", abKeyExpID).Count(&rows).Error; err != nil {
		t.Fatalf("统计行数失败: %v", err)
	}
	if rows != 2 {
		t.Errorf("结果行数=%d，期望 2（迁移不应动数据）", rows)
	}
}

func TestABExperimentResultUniqueKeyMigrationOnMissingTableIsNoop(t *testing.T) {
	db := testutil.NewTestDB(t)
	if db == nil {
		t.Fatal("测试库不可达")
	}
	if err := NewABExperimentResultUniqueKeyMigration(db).Up(abKeySystemCtx()); err != nil {
		t.Errorf("表还不存在时 Up 返回 %v，want nil（全新部署不该因此停摆）", err)
	}
}

func TestABExperimentResultUniqueKeyMigrationNilDBFailsClosed(t *testing.T) {
	if err := NewABExperimentResultUniqueKeyMigration(nil).Up(context.Background()); err == nil {
		t.Fatal("db 为 nil 时 Up 静默返回 nil：迁移会带着「已执行」的结论跳过真实修复")
	}
	if err := NewABExperimentResultUniqueKeyMigration(nil).Down(context.Background()); err != nil {
		t.Errorf("Down 无操作却返回 %v，want nil", err)
	}
}

func TestABExperimentResultUniqueKeyMigrationIsRegistered(t *testing.T) {
	reg := migration.NewMigrationRegistry()
	RegisterMigrations(reg, nil)
	for _, m := range reg.GetAll() {
		if m.Version() == "v3.68.0" {
			return
		}
	}
	t.Error("v3.68.0 未注册进迁移表：存量库的错误唯一索引不会被修复")
}
