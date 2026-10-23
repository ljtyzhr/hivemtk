package migrations

import (
	"context"
	"testing"

	"hivemtk-user/internal/migration"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

// v3.26.0 建的是触达域两张表 reach_compliance_log / reach_delayed_outbound，
// 实现了却从未进 RegisterMigrations 清单。这两张表的 model 挂在 internal/service 的
// init() → db.RegisterExtraModels 上，所以现网有表；但迁移一旦建出的形状与 model
// 对不上，走纯迁移链恢复出来的库就会在第一次读写时报「列不存在」。
// 下面的用例就是把「迁移建的表 ⊇ model 声明的列」钉成契约。

func modelColumnNames(t *testing.T, db *gorm.DB, m any) []string {
	t.Helper()
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(m); err != nil {
		t.Fatalf("解析模型失败 %T: %v", m, err)
	}
	return append([]string(nil), stmt.Schema.DBNames...)
}

func TestReachTablesMigration_Meta(t *testing.T) {
	m := NewReachTablesMigration(nil)
	if m.Version() != "v3.26.0" {
		t.Errorf("Version()=%q want=v3.26.0", m.Version())
	}
	if m.Name() == "" || m.Description() == "" {
		t.Error("Name/Description should not be empty")
	}
	if err := m.Up(context.Background()); err == nil {
		t.Error("nil db Up() 应返回错误")
	}
	if err := m.Down(context.Background()); err == nil {
		t.Error("nil db Down() 应返回错误")
	}
}

func TestReachTablesMigration_IsRegistered(t *testing.T) {
	reg := migration.NewMigrationRegistry()
	RegisterMigrations(reg, nil)
	m, ok := reg.Get("v3.26.0")
	if !ok {
		t.Fatal("v3.26.0 未注册进迁移链")
	}
	if m.Name() != NewReachTablesMigration(nil).Name() {
		t.Errorf("注册到的迁移名不符：%s", m.Name())
	}
}

// TestReachTablesMigration_UpBuildsModelReadableTables 空库里由迁移建表，
// 建出来的表必须能直接承载 model 的每一个字段。
func TestReachTablesMigration_UpBuildsModelReadableTables(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()

	tables := []struct {
		name  string
		model any
	}{
		{"reach_compliance_log", &model.ReachComplianceLog{}},
		{"reach_delayed_outbound", &model.DelayedOutboundReply{}},
	}
	for _, tb := range tables {
		if err := db.Migrator().DropTable(tb.name); err != nil {
			t.Fatalf("预置清场失败 %s: %v", tb.name, err)
		}
		if db.Migrator().HasTable(tb.name) {
			t.Fatalf("预置条件不成立：%s 应已被清除", tb.name)
		}
	}

	m := NewReachTablesMigration(db)
	if err := m.Up(ctx); err != nil {
		t.Fatalf("Up() failed: %v", err)
	}
	if err := m.Up(ctx); err != nil {
		t.Fatalf("二次 Up() 应幂等: %v", err)
	}

	for _, tb := range tables {
		if !db.Migrator().HasTable(tb.name) {
			t.Fatalf("Up 后 %s 应存在", tb.name)
		}
		for _, col := range modelColumnNames(t, db, tb.model) {
			if !db.Migrator().HasColumn(tb.name, col) {
				t.Errorf("Up 建出的 %s 缺 model 列 %s：纯迁移链恢复的库读写即报错", tb.name, col)
			}
		}
	}

	// 队列靠 status+send_at 扫到期消息，缺索引就是每轮全表扫
	for _, idx := range []string{
		"idx_reach_compliance_log_channel", "idx_reach_compliance_log_created",
		"idx_reach_delayed_outbound_platform", "idx_reach_delayed_outbound_conversation",
		"idx_reach_delayed_outbound_send_at", "idx_reach_delayed_outbound_status",
	} {
		if indexDefinition(t, db, idx) == "" {
			t.Errorf("Up 后索引 %s 应存在", idx)
		}
	}

	// Down 走 declineTableDrop：这两张表在真实部署里由 AutoMigrate 持有
	// （model 挂在 RegisterExtraModels 上），按名字 DROP 会把现网正在写的合规
	// 审计日志与延迟出站队列一起抹掉，且回滚后的进程永不自动补回。
	// 故这里钉"降级不销毁在用对象"，见 down_drop_policy.go。
	if err := m.Down(ctx); err != nil {
		t.Fatalf("Down() failed: %v", err)
	}
	if err := m.Down(ctx); err != nil {
		t.Fatalf("二次 Down() 应幂等: %v", err)
	}
	for _, tb := range tables {
		if !db.Migrator().HasTable(tb.name) {
			t.Errorf("Down 把 %s 删了：该表在现网由 AutoMigrate 持有，"+
				"回滚等于不可恢复的数据丢失，降级只允许放弃删除", tb.name)
			continue
		}
		// 放弃删除不等于迁移坏掉：Down 之后 Up 必须仍然幂等可用
		if err := m.Up(ctx); err != nil {
			t.Errorf("Down 之后再 Up 应当幂等，实际: %v", err)
		}
	}
}
