package migrations

// v3.70.0 修的是 channel_agent_bindings 的键位：从
// (tenant_id, channel_type, account_id) 改为
// (tenant_id, channel_type, account_id, COALESCE(chat_id, ''))。
//
// 事实：v3.15 迁移建的 `idx_channel_binding (channel_type, account_id)` 被
// tenancyidx.Normalize 改写成不含 chat_id 的形状，而 chat_id 是模型自己声明的列
// （"群组/会话 ID，NULL=account 级默认"）——telegram 群组功能正是靠它区分
// 「同一账号下的多个群」。不含 chat_id 的唯一键会让「同一账号多个群」根本建不出绑定行。
//
// 用例必须钉住**行为**（同一账号多 chat 能共存、account 级默认仍唯一、跨租户不互撞），
// 只断言索引名存在会放过「索引建了但列不对」的假修复。

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
	cbKeyTenantA = uint(251)
	cbKeyTenantB = uint(252)
	cbKeyChannel = "telegram"
	cbKeyAccount = "acc-key-1"
	cbKeyChatOne = "-100111"
	cbKeyChatTwo = "-100222"
)

func cbKeySystemCtx() context.Context { return tenant.WithSystemScope(context.Background()) }

// cbKeyFixture 建表并把索引还原成 v3.70.0 之前的错误形态：
// (tenant_id, channel_type, account_id) 唯一（不含 chat_id）。
func cbKeyFixture(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.NewTestDB(t, &model.ChannelAgentBinding{})
	if db == nil {
		t.Fatal("测试库不可达：键位是否修好无法判定（不 Skip，跳过等于没锁）")
	}
	t.Cleanup(func() {
		_ = db.WithContext(cbKeySystemCtx()).
			Where("account_id = ?", cbKeyAccount).
			Delete(&model.ChannelAgentBinding{}).Error
	})

	for _, name := range []string{"idx_channel_binding", "idx_channel_binding_unique"} {
		if err := db.Exec(`DROP INDEX IF EXISTS ` + name).Error; err != nil {
			t.Fatalf("删除现有唯一索引 %s 失败: %v", name, err)
		}
	}
	if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_channel_binding
ON channel_agent_bindings (tenant_id, channel_type, account_id)`).Error; err != nil {
		t.Fatalf("重建旧唯一索引失败: %v", err)
	}
	// 落一行 account 级默认（chat_id 为空）——迁移前就该存在，且迁移后必须还在。
	if err := db.WithContext(cbKeySystemCtx()).Create(&model.ChannelAgentBinding{
		TenantID: cbKeyTenantA, ChannelType: cbKeyChannel, AccountID: cbKeyAccount,
		AgentID: 9001, IsPrimary: true, Enabled: true,
	}).Error; err != nil {
		t.Fatalf("落 account 级默认绑定失败: %v", err)
	}
	return db
}

func TestChannelBindingChatIDUniqueKeyMigrationAllowsSameAccountMultipleChats(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	db := cbKeyFixture(t)
	if err := NewChannelBindingChatIDUniqueKeyMigration(db).Up(cbKeySystemCtx()); err != nil {
		t.Fatalf("Up 失败: %v", err)
	}

	// 同一账号下的两个群必须能各自建绑定（迁移前会被判重：键里没有 chat_id）。
	for _, chat := range []string{cbKeyChatOne, cbKeyChatTwo} {
		if err := db.WithContext(cbKeySystemCtx()).Create(&model.ChannelAgentBinding{
			TenantID: cbKeyTenantA, ChannelType: cbKeyChannel, AccountID: cbKeyAccount,
			ChatID: chat, AgentID: 9002, IsPrimary: false, Enabled: true,
		}).Error; err != nil {
			t.Fatalf("迁移后同一账号的群 %s 写不进绑定（键位没加 chat_id 维度）: %v", chat, err)
		}
	}

	// account 级默认仍然只允许一条：chat_id 为 NULL 与空串视为同一个键。
	dupDefault := &model.ChannelAgentBinding{
		TenantID: cbKeyTenantA, ChannelType: cbKeyChannel, AccountID: cbKeyAccount,
		AgentID: 9003, IsPrimary: true, Enabled: true,
	}
	if err := db.WithContext(cbKeySystemCtx()).Create(dupDefault).Error; err == nil {
		t.Error("同账号的第二条 account 级默认未被唯一键挡住：account 级默认唯一性丢失")
	}

	// 跨租户同账号同群必须能共存。
	if err := db.WithContext(cbKeySystemCtx()).Create(&model.ChannelAgentBinding{
		TenantID: cbKeyTenantB, ChannelType: cbKeyChannel, AccountID: cbKeyAccount,
		ChatID: cbKeyChatOne, AgentID: 9004, IsPrimary: false, Enabled: true,
	}).Error; err != nil {
		t.Fatalf("迁移后另一租户写不进同账号同群的绑定（唯一键漏了 tenant_id）: %v", err)
	}

	var indexDef string
	if err := db.Raw(`SELECT indexdef FROM pg_indexes WHERE tablename = 'channel_agent_bindings'
AND indexname = 'idx_channel_binding_unique'`).Scan(&indexDef).Error; err != nil {
		t.Fatalf("查询索引定义失败: %v", err)
	}
	if !strings.Contains(indexDef, "tenant_id") || !strings.Contains(indexDef, "chat_id") {
		t.Errorf("索引列不对，实际 %q，期望含 tenant_id 与 chat_id 两列", indexDef)
	}
	// 旧索引必须已消失：留着它，同账号多群仍会被它判重。
	var oldCount int64
	if err := db.Raw(`SELECT count(*) FROM pg_indexes WHERE tablename = 'channel_agent_bindings'
AND indexname = 'idx_channel_binding'`).Scan(&oldCount).Error; err != nil {
		t.Fatalf("统计旧索引失败: %v", err)
	}
	if oldCount != 0 {
		t.Error("旧的不含 chat_id 的唯一索引还在：同账号多群仍会被它判重")
	}

	// 幂等：第二次 Up 不报错，行数不变。
	if err := NewChannelBindingChatIDUniqueKeyMigration(db).Up(cbKeySystemCtx()); err != nil {
		t.Fatalf("第二次 Up 失败: %v", err)
	}
	var rows int64
	if err := db.WithContext(cbKeySystemCtx()).Model(&model.ChannelAgentBinding{}).
		Where("account_id = ?", cbKeyAccount).Count(&rows).Error; err != nil {
		t.Fatalf("统计行数失败: %v", err)
	}
	if rows != 4 {
		t.Errorf("绑定行数=%d，期望 4（A 的 account 默认 + A 的两个群 + B 的一个群）", rows)
	}
}

// TestChannelBindingChatIDUniqueKeyMigrationDedupsBeforeIndex 同键重复行必须先清再建索引。
//
// 不清就直接建索引 ⇒ 建索引失败 ⇒ 一次索引修正变成一次停服。
func TestChannelBindingChatIDUniqueKeyMigrationDedupsBeforeIndex(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	db := cbKeyFixture(t)
	// 制造一组同键重复行：绕开唯一键直接插（先删掉旧索引）。
	if err := db.Exec(`DROP INDEX IF EXISTS idx_channel_binding`).Error; err != nil {
		t.Fatalf("删除旧索引失败: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := db.WithContext(cbKeySystemCtx()).Create(&model.ChannelAgentBinding{
			TenantID: cbKeyTenantA, ChannelType: cbKeyChannel, AccountID: cbKeyAccount,
			ChatID: cbKeyChatOne, AgentID: uint(9100 + i), IsPrimary: false, Enabled: true,
		}).Error; err != nil {
			t.Fatalf("造重复行失败: %v", err)
		}
	}

	if err := NewChannelBindingChatIDUniqueKeyMigration(db).Up(cbKeySystemCtx()); err != nil {
		t.Fatalf("Up 失败（重复行没清干净，建索引应当失败）: %v", err)
	}
	var n int64
	if err := db.WithContext(cbKeySystemCtx()).Model(&model.ChannelAgentBinding{}).
		Where("account_id = ? AND chat_id = ?", cbKeyAccount, cbKeyChatOne).
		Count(&n).Error; err != nil {
		t.Fatalf("统计失败: %v", err)
	}
	if n != 1 {
		t.Errorf("同键重复行清理后剩 %d 行 want=1（保留 id 最大的那条）", n)
	}
}

func TestChannelBindingChatIDUniqueKeyMigrationOnMissingTableIsNoop(t *testing.T) {
	db := testutil.NewTestDB(t)
	if db == nil {
		t.Fatal("测试库不可达")
	}
	if err := NewChannelBindingChatIDUniqueKeyMigration(db).Up(cbKeySystemCtx()); err != nil {
		t.Errorf("表还不存在时 Up 返回 %v，want nil（全新部署不该因此停摆）", err)
	}
}

func TestChannelBindingChatIDUniqueKeyMigrationNilDBFailsClosed(t *testing.T) {
	if err := NewChannelBindingChatIDUniqueKeyMigration(nil).Up(context.Background()); err == nil {
		t.Fatal("db 为 nil 时 Up 静默返回 nil：迁移会带着「已执行」的结论跳过真实修复")
	}
	if err := NewChannelBindingChatIDUniqueKeyMigration(nil).Down(context.Background()); err != nil {
		t.Errorf("Down 无操作却返回 %v，want nil", err)
	}
}

func TestChannelBindingChatIDUniqueKeyMigrationIsRegistered(t *testing.T) {
	reg := migration.NewMigrationRegistry()
	RegisterMigrations(reg, nil)
	for _, m := range reg.GetAll() {
		if m.Version() == "v3.70.0" {
			return
		}
	}
	t.Error("v3.70.0 未注册进迁移表：存量库的错误唯一索引不会被修复")
}
