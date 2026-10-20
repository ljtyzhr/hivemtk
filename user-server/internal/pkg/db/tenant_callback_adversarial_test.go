package db

// tenant_callback_adversarial_test.go —— 租户隔离的**对抗性回归用例**。
//
// 这组用例的来历：2026-10-20 对 001-saas-multi-tenant 的隔离机制做对抗审计时，
// 用「主动攻击」的方式（构造会踩坑的 GORM 写法）打出三个真实缺陷：
//
//  1. Create + Omit("tenant_id") / Select("name") ⇒ INSERT **缺 tenant_id 列**
//     ⇒ 行落列默认值 1（默认租户）= 静默跨租户写。修复前 SQL：
//     INSERT INTO "tenancy_probe" ("name") VALUES ($1)
//  2. Save（Selects=*）把结构体零值 tenant_id 一并写进 SET ⇒ 行的租户被覆写成 0
//     ⇒ 行掉出本租户。修复前 SQL：
//     UPDATE "tenancy_probe" SET "tenant_id"=$1,"name"=$2 WHERE ...
//  3. JOIN 串自带 WHERE 时，追加的租户条件落进 WHERE 侧 ⇒ LEFT JOIN 退化成 INNER JOIN
//     ⇒ 未匹配的父行被挤掉（静默少数据）。修复前 SQL：
//     … LEFT JOIN tenancy_probe AS tp ON tp.id = g.id + 1000
//       WHERE g.id = 1 AND "tp"."tenant_id" = $1
//
// 本文件把三处缺陷与「验证过没问题」的攻击路径一并钉死，防止回归。
// 读回一律用**系统作用域**（tenant.WithSystemScope）：用租户作用域读回会把
// 「行落到别的租户」伪装成「record not found」，让缺陷以假绿通过。

import (
	"context"
	"testing"

	"hivemtk-user/internal/pkg/tenancyidx"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

// ---- 探针用父子模型（关联写入路径） ----

type probeParent struct {
	ID       uint         `gorm:"primaryKey"`
	TenantID uint         `gorm:"not null;default:1"`
	Name     string       `gorm:"type:varchar(60)"`
	Children []probeChild `gorm:"foreignKey:ParentID"`
}

func (probeParent) TableName() string { return "tenancy_probe_parent" }

type probeChild struct {
	ID       uint `gorm:"primaryKey"`
	TenantID uint `gorm:"not null;default:1"`
	ParentID uint
	Name     string `gorm:"type:varchar(60)"`
}

func (probeChild) TableName() string { return "tenancy_probe_child" }

// setupAdversarialDB 自建夹具（可带自定义模型），与 setupTenantCallbackDB 同口径
func setupAdversarialDB(t *testing.T, extra ...any) *gorm.DB {
	t.Helper()
	models := []any{&tenancyProbeModel{}, &tenancyGlobalModel{}}
	models = append(models, extra...)
	database := testutil.NewTestDB(t, models...)
	tenancyidx.RegisterTenantCallbacks(database)
	t.Cleanup(func() {
		tenant.SetMode(tenant.ModeOff)
		tenancyidx.ResetRegistryForTest()
	})
	return database
}

// seedTwoTenants 探针数据：租户 91 持有 "ja"，租户 92 持有 "jb"，全局表两行 id=1/2。
func seedTwoTenants(t *testing.T) (*gorm.DB, context.Context) {
	t.Helper()
	database := setupAdversarialDB(t)
	tenant.SetMode(tenant.ModeOn)
	ctxA := tenant.WithTenantID(context.Background(), 91)
	ctxB := tenant.WithTenantID(context.Background(), 92)
	if err := database.WithContext(ctxA).Create(&tenancyProbeModel{Name: "ja"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.WithContext(ctxB).Create(&tenancyProbeModel{Name: "jb"}).Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range []uint{1, 2} {
		if err := database.WithContext(ctxA).Create(&tenancyGlobalModel{ID: id, Name: "g"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	return database, ctxA
}

// allProbeRows 系统作用域读回全部探针行（绕过隔离，用于对账真实落库租户）
func allProbeRows(t *testing.T, database *gorm.DB) []tenancyProbeModel {
	t.Helper()
	var rows []tenancyProbeModel
	sysCtx := tenant.WithSystemScope(context.Background())
	if err := database.WithContext(sysCtx).Order("id").Find(&rows).Error; err != nil {
		t.Fatalf("系统作用域读回失败: %v", err)
	}
	return rows
}

// TestTenantCallback_CreateOmitTenantIDRejected Omit("tenant_id") 会让 INSERT 缺列，
// 行落列默认值 1（默认租户）= 跨租户写。必须 fail-closed。
func TestTenantCallback_CreateOmitTenantIDRejected(t *testing.T) {
	database, ctxA := seedTwoTenants(t)
	m := tenancyProbeModel{Name: "omit"}
	err := database.WithContext(ctxA).Omit("tenant_id").Create(&m).Error
	if err == nil {
		t.Fatalf("Omit(\"tenant_id\") 的 Create 必须被拒绝，实际成功执行")
	}
	for _, r := range allProbeRows(t, database) {
		if r.Name == "omit" {
			t.Errorf("被拒的 Create 仍落了行: tenant_id=%d", r.TenantID)
		}
	}
}

// TestTenantCallback_CreateSelectSubsetRejected Select("name") 同样让 INSERT 缺列。
func TestTenantCallback_CreateSelectSubsetRejected(t *testing.T) {
	database, ctxA := seedTwoTenants(t)
	m := tenancyProbeModel{Name: "selonly"}
	err := database.WithContext(ctxA).Select("name").Create(&m).Error
	if err == nil {
		t.Fatalf("Select(\"name\") 的 Create 必须被拒绝，实际成功执行")
	}
	for _, r := range allProbeRows(t, database) {
		if r.Name == "selonly" {
			t.Errorf("被拒的 Create 仍落了行: tenant_id=%d", r.TenantID)
		}
	}
}

// TestTenantCallback_SaveZeroTenantIDRejected Save 会把结构体零值 tenant_id 写进 SET，
// 把行踢出本租户（tenant_id 覆写成 0）。必须 fail-closed。
func TestTenantCallback_SaveZeroTenantIDRejected(t *testing.T) {
	database, ctxA := seedTwoTenants(t)
	err := database.WithContext(ctxA).Save(&tenancyProbeModel{ID: 1, Name: "saved"}).Error
	if err == nil {
		t.Fatalf("Save 零值 tenant_id 必须被拒绝，实际成功执行")
	}
	for _, r := range allProbeRows(t, database) {
		if r.Name == "ja" && r.TenantID != 91 {
			t.Errorf("Save 把行的 tenant_id 改成了 %d（期望保持 91）", r.TenantID)
		}
	}
}

// TestTenantCallback_LeftJoinWithWhereKeepsParentRow JOIN 串自带 WHERE 时，
// 租户条件必须落进 ON 子句：落进 WHERE 会把 LEFT JOIN 退化成 INNER JOIN，父行被挤掉。
func TestTenantCallback_LeftJoinWithWhereKeepsParentRow(t *testing.T) {
	database, ctxA := seedTwoTenants(t)
	var rows []aliasedProbeRow
	err := database.WithContext(ctxA).Table("tenancy_global_probe AS g").
		Select("g.id, tp.tenant_id, tp.name").
		Joins("LEFT JOIN tenancy_probe AS tp ON tp.id = g.id + 1000 WHERE g.id = 1").
		Scan(&rows).Error
	if err != nil {
		t.Fatalf("LEFT JOIN 查询失败: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("LEFT JOIN 语义被破坏：父行被租户条件挤掉（期望 1 行，实际 %d 行）", len(rows))
	}
	if rows[0].ID != 1 {
		t.Errorf("父行 id 期望 1，实际 %d", rows[0].ID)
	}
}

// TestTenantCallback_SubqueryScoped 子查询（Where("id IN (?)", sub)）也必须带租户条件：
// 外层查全局表时，若子查询未被隔离，租户 91 会通过子查询命中租户 92 的数据。
func TestTenantCallback_SubqueryScoped(t *testing.T) {
	database, ctxA := seedTwoTenants(t)
	sub := database.WithContext(ctxA).Model(&tenancyProbeModel{}).Select("id").Where("name = ?", "jb")
	var g []tenancyGlobalModel
	if err := database.WithContext(ctxA).Model(&tenancyGlobalModel{}).Where("id IN (?)", sub).Find(&g).Error; err != nil {
		t.Fatalf("子查询查询失败: %v", err)
	}
	if len(g) > 0 {
		t.Errorf("子查询未隔离：租户 91 的查询命中了 %d 行（子查询看到了租户 92 的数据）", len(g))
	}
}

// TestTenantCallback_UnscopedStillScoped Unscoped 只跳软删除，不得连带跳过租户隔离。
func TestTenantCallback_UnscopedStillScoped(t *testing.T) {
	database, ctxA := seedTwoTenants(t)
	var rows []tenancyProbeModel
	err := database.WithContext(ctxA).Unscoped().Where("name IN ?", []string{"ja", "jb"}).Find(&rows).Error
	if err != nil {
		t.Fatalf("Unscoped 查询失败: %v", err)
	}
	if len(rows) != 1 || rows[0].TenantID != 91 {
		t.Errorf("Unscoped 读到了非本租户行: %+v", rows)
	}
}

// TestTenantCallback_GlobalDeleteScoped AllowGlobalUpdate 的删除也必须带租户条件。
func TestTenantCallback_GlobalDeleteScoped(t *testing.T) {
	database, ctxA := seedTwoTenants(t)
	err := database.WithContext(ctxA).Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&tenancyProbeModel{}).Error
	if err != nil {
		t.Fatalf("全局删除失败: %v", err)
	}
	for _, r := range allProbeRows(t, database) {
		if r.Name == "ja" {
			t.Errorf("租户 91 的行没被删掉：全局删除未作用于本租户")
		}
	}
}

// TestTenantCallback_UpdateColumnTenantIDRejected UpdateColumn 改写 tenant_id 必须被拒绝。
func TestTenantCallback_UpdateColumnTenantIDRejected(t *testing.T) {
	database, ctxA := seedTwoTenants(t)
	err := database.WithContext(ctxA).Model(&tenancyProbeModel{}).Where("name = ?", "ja").
		UpdateColumn("tenant_id", 92).Error
	if err == nil {
		t.Fatalf("UpdateColumn 改写 tenant_id 必须被拒绝")
	}
	for _, r := range allProbeRows(t, database) {
		if r.Name == "ja" && r.TenantID != 91 {
			t.Errorf("tenant_id 被改写为 %d", r.TenantID)
		}
	}
}

// TestTenantCallback_AssociationCreateFillsChild 父带子一起 Create，子行 tenant_id 必须被填充。
func TestTenantCallback_AssociationCreateFillsChild(t *testing.T) {
	database := setupAdversarialDB(t, &probeParent{}, &probeChild{})
	tenant.SetMode(tenant.ModeOn)
	ctxA := tenant.WithTenantID(context.Background(), 91)

	p := probeParent{Name: "p", Children: []probeChild{{Name: "c1"}}}
	if err := database.WithContext(ctxA).Create(&p).Error; err != nil {
		t.Fatalf("关联 Create 失败: %v", err)
	}
	sysCtx := tenant.WithSystemScope(context.Background())
	var kids []probeChild
	if err := database.WithContext(sysCtx).Find(&kids).Error; err != nil {
		t.Fatalf("读子行失败: %v", err)
	}
	if len(kids) == 0 {
		t.Fatalf("没有读到子行")
	}
	for _, k := range kids {
		if k.TenantID != 91 {
			t.Errorf("子行 tenant_id=%d（期望 91）", k.TenantID)
		}
	}
}

// TestTenantCallback_FirstOrCreateFills FirstOrCreate 新建的行 tenant_id 必须被填充。
func TestTenantCallback_FirstOrCreateFills(t *testing.T) {
	database, ctxA := seedTwoTenants(t)
	m := tenancyProbeModel{Name: "foc"}
	if err := database.WithContext(ctxA).FirstOrCreate(&m, tenancyProbeModel{Name: "foc"}).Error; err != nil {
		t.Fatalf("FirstOrCreate 失败: %v", err)
	}
	for _, r := range allProbeRows(t, database) {
		if r.Name == "foc" && r.TenantID != 91 {
			t.Errorf("行落 tenant_id=%d（期望 91）", r.TenantID)
		}
	}
}
