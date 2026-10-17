package db

import (
	"context"
	"sync"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenancyidx"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// tenancyProbeModel 回调测试专用模型（独立表，不进 allModels）
type tenancyProbeModel struct {
	ID       uint   `gorm:"primaryKey" json:"id"`
	TenantID uint   `gorm:"not null;default:1;index" json:"tenant_id"`
	Name     string `gorm:"type:varchar(60)" json:"name"`
}

func (tenancyProbeModel) TableName() string { return "tenancy_probe" }

// tenancyGlobalModel 无 TenantID 的全局表探针
type tenancyGlobalModel struct {
	ID   uint   `gorm:"primaryKey" json:"id"`
	Name string `gorm:"type:varchar(60)" json:"name"`
}

func (tenancyGlobalModel) TableName() string { return "tenancy_global_probe" }

func setupTenantCallbackDB(t *testing.T) *gorm.DB {
	t.Helper()
	database := testutil.NewTestDB(t, &tenancyProbeModel{}, &tenancyGlobalModel{})
	// 登记表由 testdb 钩子内省装配（覆盖本夹具两张表）；生产由 db.AutoMigrate 装配
	RegisterTenantCallbacks(database)
	t.Cleanup(func() {
		tenant.SetMode(tenant.ModeOff)
		tenancyidx.ResetRegistryForTest()
	})
	return database
}

func TestTenantCallback_OffModePassthrough(t *testing.T) {
	database := setupTenantCallbackDB(t)
	tenant.SetMode(tenant.ModeOff)

	ctx := tenant.WithTenantID(context.Background(), 42)
	if err := database.WithContext(ctx).Create(&tenancyProbeModel{Name: "a"}).Error; err != nil {
		t.Fatalf("off 模式创建失败: %v", err)
	}
	var got tenancyProbeModel
	if err := database.WithContext(ctx).First(&got, "name = ?", "a").Error; err != nil {
		t.Fatalf("off 模式查询失败: %v", err)
	}
	if got.TenantID != 1 {
		t.Fatalf("off 模式 tenant_id 应为列默认值 1，实际 %d", got.TenantID)
	}
	// off 模式无租户 ctx 也不报错（直通）
	if err := database.WithContext(context.Background()).First(&got, "name = ?", "a").Error; err != nil {
		t.Fatalf("off 模式无租户 ctx 查询应直通: %v", err)
	}
}

func TestTenantCallback_OnModeFillAndScope(t *testing.T) {
	database := setupTenantCallbackDB(t)
	tenant.SetMode(tenant.ModeOn)

	ctxA := tenant.WithTenantID(context.Background(), 42)
	ctxB := tenant.WithTenantID(context.Background(), 43)

	// A 写两行：tenant_id 由 callback 自动填充
	for _, name := range []string{"a1", "a2"} {
		if err := database.WithContext(ctxA).Create(&tenancyProbeModel{Name: name}).Error; err != nil {
			t.Fatalf("on 模式创建失败: %v", err)
		}
	}
	// B 写一行
	if err := database.WithContext(ctxB).Create(&tenancyProbeModel{Name: "b1"}).Error; err != nil {
		t.Fatalf("on 模式创建失败: %v", err)
	}

	var nA, nB int64
	if err := database.WithContext(ctxA).Model(&tenancyProbeModel{}).Count(&nA).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.WithContext(ctxB).Model(&tenancyProbeModel{}).Count(&nB).Error; err != nil {
		t.Fatal(err)
	}
	if nA != 2 || nB != 1 {
		t.Fatalf("租户隔离计数错误：A=%d(期望2) B=%d(期望1)", nA, nB)
	}

	// A 按 B 的行名查不到（不存在的行）
	var got tenancyProbeModel
	if err := database.WithContext(ctxA).First(&got, "name = ?", "b1").Error; err == nil {
		t.Fatal("租户 A 不应看到租户 B 的数据")
	}

	// 非租户表（无 TenantID）不受隔离：全局表 A/B 均可见
	if err := database.WithContext(ctxA).Create(&tenancyGlobalModel{Name: "g"}).Error; err != nil {
		t.Fatalf("全局表创建失败: %v", err)
	}
	if err := database.WithContext(ctxB).First(&tenancyGlobalModel{}, "name = ?", "g").Error; err != nil {
		t.Fatalf("非租户表不应被隔离拦截: %v", err)
	}
}

func TestTenantCallback_OnModeFailClosed(t *testing.T) {
	database := setupTenantCallbackDB(t)
	tenant.SetMode(tenant.ModeOn)

	// 无租户 ctx：查询/创建/更新/删除全部 fail-closed
	if err := database.WithContext(context.Background()).Find(&[]tenancyProbeModel{}).Error; err == nil {
		t.Fatal("on 模式无租户 ctx 查询必须 fail-closed")
	}
	if err := database.WithContext(context.Background()).Create(&tenancyProbeModel{Name: "x"}).Error; err == nil {
		t.Fatal("on 模式无租户 ctx 创建必须 fail-closed")
	}
	if err := database.WithContext(context.Background()).Model(&tenancyProbeModel{}).Where("1=1").Update("name", "y").Error; err == nil {
		t.Fatal("on 模式无租户 ctx 更新必须 fail-closed")
	}
	if err := database.WithContext(context.Background()).Where("1=1").Delete(&tenancyProbeModel{}).Error; err == nil {
		t.Fatal("on 模式无租户 ctx 删除必须 fail-closed")
	}

	// 行自证写入：显式携带 tenant_id 的无 ctx 写入放行（消费端读行取租户的前提）
	row := tenancyProbeModel{TenantID: 42, Name: "self-proof"}
	if err := database.WithContext(context.Background()).Create(&row).Error; err != nil {
		t.Fatalf("行自证写入（显式 tenant_id）应放行: %v", err)
	}

	// 系统作用域豁免：无租户 ctx 系统操作放行（列默认值兜底）
	sysCtx := tenant.WithSystemScope(context.Background())
	if err := database.WithContext(sysCtx).Create(&tenancyProbeModel{Name: "sys"}).Error; err != nil {
		t.Fatalf("系统作用域创建应放行: %v", err)
	}
	if err := database.WithContext(sysCtx).Find(&[]tenancyProbeModel{}).Error; err != nil {
		t.Fatalf("系统作用域查询应放行: %v", err)
	}
}

func TestTenantCallback_OnModeCrossTenantWriteRejected(t *testing.T) {
	database := setupTenantCallbackDB(t)
	tenant.SetMode(tenant.ModeOn)

	ctxA := tenant.WithTenantID(context.Background(), 42)
	// 上下文租户 42，行显式声明 tenant_id=43 → 跨租户写拒绝
	if err := database.WithContext(ctxA).Create(&tenancyProbeModel{TenantID: 43, Name: "evil"}).Error; err == nil {
		t.Fatal("跨租户写入必须被拒绝")
	}
	// 一致声明放行
	if err := database.WithContext(ctxA).Create(&tenancyProbeModel{TenantID: 42, Name: "ok"}).Error; err != nil {
		t.Fatalf("租户一致的显式写入应放行: %v", err)
	}
}

func TestTenantCallback_RawGuard(t *testing.T) {
	database := setupTenantCallbackDB(t)

	// off 模式裸 SQL 直通
	tenant.SetMode(tenant.ModeOff)
	if err := database.WithContext(context.Background()).Exec("SELECT 1").Error; err != nil {
		t.Fatalf("off 模式裸 SQL 应直通: %v", err)
	}

	// on 模式裸 SQL 引用租户表 → 拒绝（GORM-only 终态的运行时第二道闸）
	tenant.SetMode(tenant.ModeOn)
	if err := database.WithContext(context.Background()).Exec("SELECT count(*) FROM tenancy_probe").Error; err == nil {
		t.Fatal("on 模式裸 SQL 引用租户表必须被拒绝")
	}
	// 引用非租户表 → 放行
	if err := database.WithContext(context.Background()).Exec("SELECT count(*) FROM tenancy_global_probe").Error; err != nil {
		t.Fatalf("on 模式裸 SQL 引用非租户表应放行: %v", err)
	}
	// 系统作用域豁免
	if err := database.WithContext(tenant.WithSystemScope(context.Background())).Exec("SELECT count(*) FROM tenancy_probe").Error; err != nil {
		t.Fatalf("系统作用域裸 SQL 应放行: %v", err)
	}
}

func TestTenantRegistry_InitializeAndProbe(t *testing.T) {
	// 夹具显式建两张探针表，登记表内省覆盖它们
	database := testutil.NewTestDB(t, &tenancyProbeModel{}, &tenancyGlobalModel{})
	tenancyidx.InitTenantRegistry(database)
	t.Cleanup(func() { tenancyidx.ResetRegistryForTest() })

	if !tenancyidx.IsTenantTable("tenancy_probe") {
		t.Fatal("声明 TenantID 的模型表必须进登记表")
	}
	if tenancyidx.IsTenantTable("tenancy_global_probe") {
		t.Fatal("未声明 TenantID 的模型表不应进登记表")
	}
	if tenancyidx.IsTenantTable("") || tenancyidx.IsTenantTable("nonexistent") {
		t.Fatal("空表名/未知表必须返回 false")
	}
	// 共享槽位库会积累其他测试的租户表：只断言相对事实，不锁死数量
	snap := tenancyidx.RegistrySnapshot()
	if !snap["tenancy_probe"] {
		t.Fatalf("登记表快照缺 tenancy_probe: %v", snap)
	}
	_ = database
}

func TestTenantRegistry_ModelDeclarations(t *testing.T) {
	// 对账不变量（模型级，DB 无关）：
	//   ① 核心业务模型必须声明 TenantID（行级隔离的建模前提）
	//   ② 全局/系统模型必须不声明（tenants 自身/参数中心/功能开关/租约等）
	// DB 侧的列存在性由 T007 加列迁移 + AutoMigrate 保证（全链路迁移测试覆盖）。
	models := append(allModels(), ExtraModels()...)
	// ExtraModels 注册点在 service 等包的 init()，db 包测试不 import 它们：
	// 这几个模型直接点名断言（与 RegisterExtraModels 清单人工对账）。
	models = append(models,
		&model.ReachComplianceLog{},
		&model.DelayedOutboundReply{},
		&model.MsgHourlySummary{},
	)
	cache := &sync.Map{}

	tableHasTenant := map[string]bool{}
	structOfTable := map[string]string{}
	for _, m := range models {
		s, err := schema.Parse(m, cache, schema.NamingStrategy{})
		if err != nil {
			continue
		}
		has := false
		for _, f := range s.Fields {
			if f.DBName == "tenant_id" {
				has = true
				break
			}
		}
		tableHasTenant[s.Table] = has
		structOfTable[s.Table] = s.Name
	}

	for _, table := range []string{
		"customers", "system_users", "llm_providers", "sop_timers", "short_links",
		"chat_channels", "bridge_accounts", "reach_delayed_outbound",
		"aggregation_watermarks", "msg_hourly_summary",
	} {
		if !tableHasTenant[table] {
			t.Errorf("核心业务表 %s（模型 %s）必须声明 TenantID（specs/001-saas-multi-tenant）", table, structOfTable[table])
		}
	}
	for _, table := range []string{
		"tenants", "migration_records", "migration_checkpoints",
		"config_params", "config_param_groups", "config_param_audit_logs",
		"feature_flags", "feature_flag_audit_logs", "feature_flag_eval_logs",
		"cron_job_leases", "dictionaries", "dictionary_transitions",
		"backups", "restore_records", "system_config", "system_config_kv",
	} {
		if tableHasTenant[table] {
			t.Errorf("全局/系统表 %s 不应声明 TenantID（应与迁移豁免清单一致）", table)
		}
	}
}

// 防止 sync import 在未来重构中丢失（登记表并发语义依赖它）
var _ = sync.RWMutex{}
