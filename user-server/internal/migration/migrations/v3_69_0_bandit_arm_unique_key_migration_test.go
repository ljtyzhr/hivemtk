package migrations

// v3.69.0 修的是 bandit_arms 的键位：从 (tenant_id, arm_key) 恢复为
// (tenant_id, experiment_id, arm_key)。
//
// 旧模型只给 ArmKey 标了 uniqueIndex，多租户规范化后落成「同租户内 arm_key 全局唯一」。
// 但 arm_key 只在实验内唯一：prompt 迭代固定复用 arm_0_original。用例必须钉住
// 「迁移后第二个实验能复用同名臂」，只断言索引名存在会放过「索引建了但列不对」的假修复。

import (
	"context"
	"strings"
	"testing"

	"hivemtk-user/internal/migration"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

const (
	baKeyTenantA = uint(241)
	baKeyTenantB = uint(242)
	baKeyExpOne  = "exp-key-one"
	baKeyExpTwo  = "exp-key-two"
	baKeyArm     = "arm_0_original"
)

func baKeySystemCtx() context.Context { return tenant.WithSystemScope(context.Background()) }

// baKeyFixture 建表并把索引还原成 v3.69.0 之前的错误形态：
// (tenant_id, arm_key) 唯一。同时落第一个实验的臂。
func baKeyFixture(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.NewTestDB(t, &model.BanditArm{})
	if db == nil {
		t.Fatal("测试库不可达：键位是否恢复无法判定（不 Skip，跳过等于没锁）")
	}
	t.Cleanup(func() {
		_ = db.WithContext(baKeySystemCtx()).
			Where("experiment_id IN ?", []string{baKeyExpOne, baKeyExpTwo}).
			Delete(&model.BanditArm{}).Error
	})

	if err := db.Exec(`DROP INDEX IF EXISTS idx_bandit_exp_arm`).Error; err != nil {
		t.Fatalf("删除现有复合索引失败: %v", err)
	}
	if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_bandit_exp_arm
ON bandit_arms (tenant_id, arm_key)`).Error; err != nil {
		t.Fatalf("重建旧单列唯一索引失败: %v", err)
	}
	if err := db.WithContext(baKeySystemCtx()).Create(&model.BanditArm{
		TenantID: baKeyTenantA, ExperimentID: baKeyExpOne,
		ExperimentType: model.BanditExperimentTypePrompt, ArmKey: baKeyArm,
	}).Error; err != nil {
		t.Fatalf("落第一个实验的臂失败: %v", err)
	}
	return db
}

func TestBanditArmUniqueKeyMigrationAllowsSameArmKeyInAnotherExperiment(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	db := baKeyFixture(t)
	if err := NewBanditArmUniqueKeyMigration(db).Up(baKeySystemCtx()); err != nil {
		t.Fatalf("Up 失败: %v", err)
	}

	if err := db.WithContext(baKeySystemCtx()).Create(&model.BanditArm{
		TenantID: baKeyTenantA, ExperimentID: baKeyExpTwo,
		ExperimentType: model.BanditExperimentTypePrompt, ArmKey: baKeyArm,
	}).Error; err != nil {
		t.Fatalf("迁移后第二个实验仍写不进同名臂（键位没恢复）: %v", err)
	}

	// 跨租户同名实验的同名臂也必须能共存。
	if err := db.WithContext(baKeySystemCtx()).Create(&model.BanditArm{
		TenantID: baKeyTenantB, ExperimentID: baKeyExpOne,
		ExperimentType: model.BanditExperimentTypePrompt, ArmKey: baKeyArm,
	}).Error; err != nil {
		t.Fatalf("迁移后另一租户写不进同名臂: %v", err)
	}

	// 同一租户同一实验内重复 arm_key 仍必须被唯一键挡住。
	dup := &model.BanditArm{
		TenantID: baKeyTenantA, ExperimentID: baKeyExpOne,
		ExperimentType: model.BanditExperimentTypePrompt, ArmKey: baKeyArm,
	}
	if err := db.WithContext(baKeySystemCtx()).Create(dup).Error; err == nil {
		t.Error("同实验重复 arm_key 未被唯一键挡住：实验内唯一性丢失")
	}

	var indexDef string
	if err := db.Raw(`SELECT indexdef FROM pg_indexes WHERE tablename = 'bandit_arms'
AND indexname = 'idx_bandit_exp_arm'`).Scan(&indexDef).Error; err != nil {
		t.Fatalf("查询索引定义失败: %v", err)
	}
	if !strings.Contains(indexDef, "(tenant_id, experiment_id, arm_key)") {
		t.Errorf("索引列顺序不对，实际 %q，期望前缀列 (tenant_id, experiment_id, arm_key)", indexDef)
	}

	// 幂等：第二次 Up 不报错，行数不变。
	if err := NewBanditArmUniqueKeyMigration(db).Up(baKeySystemCtx()); err != nil {
		t.Fatalf("第二次 Up 失败: %v", err)
	}
	var rows int64
	if err := db.WithContext(baKeySystemCtx()).Model(&model.BanditArm{}).
		Where("arm_key = ?", baKeyArm).Count(&rows).Error; err != nil {
		t.Fatalf("统计行数失败: %v", err)
	}
	if rows != 3 {
		t.Errorf("臂行数=%d，期望 3（迁移不应动数据）", rows)
	}
}

func TestBanditArmUniqueKeyMigrationOnMissingTableIsNoop(t *testing.T) {
	db := testutil.NewTestDB(t)
	if db == nil {
		t.Fatal("测试库不可达")
	}
	if err := NewBanditArmUniqueKeyMigration(db).Up(baKeySystemCtx()); err != nil {
		t.Errorf("表还不存在时 Up 返回 %v，want nil（全新部署不该因此停摆）", err)
	}
}

func TestBanditArmUniqueKeyMigrationNilDBFailsClosed(t *testing.T) {
	if err := NewBanditArmUniqueKeyMigration(nil).Up(context.Background()); err == nil {
		t.Fatal("db 为 nil 时 Up 静默返回 nil：迁移会带着「已执行」的结论跳过真实修复")
	}
	if err := NewBanditArmUniqueKeyMigration(nil).Down(context.Background()); err != nil {
		t.Errorf("Down 无操作却返回 %v，want nil", err)
	}
}

func TestBanditArmUniqueKeyMigrationIsRegistered(t *testing.T) {
	reg := migration.NewMigrationRegistry()
	RegisterMigrations(reg, nil)
	for _, m := range reg.GetAll() {
		if m.Version() == "v3.69.0" {
			return
		}
	}
	t.Error("v3.69.0 未注册进迁移表：存量库的错误唯一索引不会被修复")
}
