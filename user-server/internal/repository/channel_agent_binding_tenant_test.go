package repository

// 契约锁（实体审查 #63，ChannelAgentBinding）：渠道账号↔智能体绑定的读写各锁在自己租户的分片里。
//
// 本实体此前零租户测试，而它有两格特别值得钉：
//
// ①**1:1 主绑定保证**：`ReplacePrimaryBinding` 的语义是「同一 (channel_type, account_id)
//    只能有一条 is_primary=true 的主绑定」。这个保证由**数据库唯一索引**承担
//    （模型与三处注释都写着 `uq_channel_account_primary (channel_type, account_id)
//    WHERE is_primary = true`）——但开发库里的实际索引是另一个形状：
//    `idx_channel_binding_unique UNIQUE (tenant_id, channel_type, account_id,
//    COALESCE(chat_id,''))`。
//    两者语义**不同**：前者管「一个渠道账号只能有一个主绑定」，后者管「同一渠道账号
//    （含 chat_id 维度）只能有一行」。后者并不禁止同一 (channel_type, account_id) 下
//    出现两条 is_primary=true、chat_id 不同的行 ⇒ 那个 1:1 承诺在库层并不成立，
//    只靠业务层的 Clear+Create 顺序维持。
//    本腿要钉的是**库里真正在的那个形状**：两租户各自绑同一个 (channel_type, account_id)
//    必须都能落地（索引以 tenant_id 打头 ⇒ 不跨租户互撞），而同租户内同键重复必须被拒。
//
// ②**跨租户不可见**：同 (channel_type, account_id) 在两租户各有一条时，
//    读口与写口（Clear/Delete）都必须只作用在自己那一条上——尤其 Clear：它把
//    is_primary 置 false，若漏了租户条件，A 租户的一次「换绑」会把 B 租户的主绑定
//    悄悄降级（而 B 的主绑定正是它渠道的默认 agent 来源）。
//
// 种子刻意让两租户用**同一个** (channel_type, account_id)：那样断言的才是 tenant_id
// 谓词本身，而不是「两个租户恰好用了不同账号号」这种随时会被复用拆穿的巧合。

import (
	"context"
	"errors"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

const (
	cabTenantA = uint(45)
	cabTenantB = uint(46)
	// 两租户共用同一个渠道账号号。
	cabChannelType = "xiaohongshu"
	cabAccountID   = "acc-shared"
)

func cabDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.ResetRegistryForTest(); tenant.SetMode(tenant.ModeOff) })
	db := testutil.NewTestDB(t, &model.ChannelAgentBinding{})
	if db == nil {
		t.Fatal("测试库不可达：绑定归属无法判定（不 Skip，跳过等于没锁）")
	}
	return db
}

// cabSeed 走被测的 Create 落一行（写侧的归属填充本身就是被审的那件事）。
func cabSeed(t *testing.T, db *gorm.DB, ctx context.Context, agentID uint, isPrimary bool) *model.ChannelAgentBinding {
	t.Helper()
	row := &model.ChannelAgentBinding{
		ChannelType: cabChannelType, AccountID: cabAccountID,
		AgentID: agentID, IsPrimary: boolPtr(isPrimary), Enabled: boolPtr(true),
	}
	repo := NewChannelAgentBindingRepositoryWithDB(db)
	if err := repo.Create(ctx, row); err != nil {
		t.Fatalf("落绑定失败: %v", err)
	}
	return row
}

// TestChannelAgentBindingSameAccountAcrossTenants 两租户绑同一个渠道账号必须都能落地。
//
// 判据：库里那个唯一索引是 `idx_channel_binding_unique (tenant_id, channel_type,
// account_id, COALESCE(chat_id,”))`——**以 tenant_id 打头** ⇒ 两个租户绑同一个
// (channel_type, account_id) 不会互相撞约束。若索引漏了 tenant_id（迁移 035 建的
// `uq_channel_account_primary (channel_type, account_id) WHERE is_primary = true`
// 就是那个形状，v3.15 建的 `idx_channel_binding (channel_type, account_id)` 也是），
// 第二个租户的绑定会撞 23505 ⇒ 一条渠道账号只能属于一个租户。
func TestChannelAgentBindingSameAccountAcrossTenants(t *testing.T) {
	db := cabDB(t)
	ctxA := tenant.WithTenantID(context.Background(), cabTenantA)
	ctxB := tenant.WithTenantID(context.Background(), cabTenantB)

	rowA := cabSeed(t, db, ctxA, 9001, true)
	rowB := cabSeed(t, db, ctxB, 9002, true)

	if rowA.TenantID != cabTenantA || rowB.TenantID != cabTenantB {
		t.Errorf("租户归属不对：A=%d(应 %d) B=%d(应 %d)：隔离 callback 没填发起租户",
			rowA.TenantID, cabTenantA, rowB.TenantID, cabTenantB)
	}
	var n int64
	if err := db.WithContext(ctxA).Model(&model.ChannelAgentBinding{}).Count(&n).Error; err != nil {
		t.Fatalf("计数失败: %v", err)
	}
	if n != 1 {
		t.Errorf("A 租户读到 %d 行 want=1", n)
	}
}

// TestChannelAgentBindingClearPrimaryStaysInTenant ClearPrimary 只许清自己那一条。
//
// 这是本实体最危险的一格：ClearPrimary 把 is_primary 置 false，而主绑定正是
// 「这个渠道账号的默认 agent」的来源（ResolveBinding / GetPrimaryByChannelAccount
// 都按它取）。若清的时候漏了租户条件，A 租户的一次「换绑」会把 B 租户的主绑定
// 悄悄降级 ⇒ B 的渠道从此走不到自己绑的 agent，且 B 的界面上看不出任何异常。
func TestChannelAgentBindingClearPrimaryStaysInTenant(t *testing.T) {
	db := cabDB(t)
	ctxA := tenant.WithTenantID(context.Background(), cabTenantA)
	ctxB := tenant.WithTenantID(context.Background(), cabTenantB)

	cabSeed(t, db, ctxA, 9001, true)
	bRow := cabSeed(t, db, ctxB, 9002, true)

	repo := NewChannelAgentBindingRepositoryWithDB(db)
	if err := repo.ClearPrimaryByChannelAccount(ctxA, cabChannelType, cabAccountID); err != nil {
		t.Fatalf("A 租户清主绑定报错: %v", err)
	}

	var bStillPrimary bool
	if err := db.WithContext(ctxB).Model(&model.ChannelAgentBinding{}).
		Where("id = ?", bRow.ID).Pluck("is_primary", &bStillPrimary).Error; err != nil {
		t.Fatalf("回读 B 的行失败: %v", err)
	}
	if !bStillPrimary {
		t.Error("B 租户的主绑定被 A 租户的 Clear 降级了：写口漏了租户条件 ⇒ " +
			"B 的渠道从此走不到自己绑的 agent，且界面上看不出异常")
	}
}

// TestChannelAgentBindingReadsAreTenantScoped 读口：同渠道账号在两租户各一行时，
// 各自只读得到自己那条。
func TestChannelAgentBindingReadsAreTenantScoped(t *testing.T) {
	db := cabDB(t)
	ctxA := tenant.WithTenantID(context.Background(), cabTenantA)
	ctxB := tenant.WithTenantID(context.Background(), cabTenantB)

	cabSeed(t, db, ctxA, 9001, true)
	cabSeed(t, db, ctxB, 9002, true)

	repo := NewChannelAgentBindingRepositoryWithDB(db)
	for _, tc := range []struct {
		name     string
		ctx      context.Context
		wantAgen uint
	}{
		{"tenantA", ctxA, 9001},
		{"tenantB", ctxB, 9002},
	} {
		list, err := repo.ListByChannelAccount(tc.ctx, cabChannelType, cabAccountID)
		if err != nil {
			t.Fatalf("%s 读绑定列表失败: %v", tc.name, err)
		}
		if len(list) != 1 {
			t.Errorf("%s 读到 %d 条 want=1（多出来的那条是别的租户的绑定）", tc.name, len(list))
			continue
		}
		if list[0].AgentID != tc.wantAgen {
			t.Errorf("%s 读到的 agent_id=%d want=%d", tc.name, list[0].AgentID, tc.wantAgen)
		}
	}

	// 裸 ctx 同样 fail-closed：读侧一旦放行，「按渠道账号号读别人的绑定」就写回了库里。
	if _, err := repo.ListByChannelAccount(context.Background(), cabChannelType, cabAccountID); err == nil {
		t.Error("裸 ctx 的 ListByChannelAccount 未被拒绝")
	}
}

// TestChannelAgentBindingExplicitFalsePersists 显式 false 必须落库。
//
// 这两列带 `default:true`：用 bool 时 GORM 把 false 当零值，写入 DB 默认值 true ——
// 「建一条辅助绑定 (is_primary=false)」会落成第二条主绑定（业务层也没走 Clear），
// 「建一条停用绑定 (enabled=false)」会直接上线接流量。
func TestChannelAgentBindingExplicitFalsePersists(t *testing.T) {
	db := cabDB(t)
	ctxA := tenant.WithTenantID(context.Background(), cabTenantA)
	repo := NewChannelAgentBindingRepositoryWithDB(db)

	row := &model.ChannelAgentBinding{
		ChannelType: cabChannelType, AccountID: "acc-aux",
		AgentID: 9101, IsPrimary: boolPtr(false), Enabled: boolPtr(false),
	}
	if err := repo.Create(ctxA, row); err != nil {
		t.Fatalf("创建辅助绑定失败: %v", err)
	}

	var got model.ChannelAgentBinding
	if err := db.WithContext(ctxA).First(&got, row.ID).Error; err != nil {
		t.Fatalf("回读失败: %v", err)
	}
	if flagOrTrue(got.IsPrimary) {
		t.Error("is_primary 显式 false 被 DB 默认值覆盖成 true：辅助绑定会变成第二条主绑定")
	}
	if flagOrTrue(got.Enabled) {
		t.Error("enabled 显式 false 被 DB 默认值覆盖成 true：停用绑定会直接上线")
	}

	// nil = 未传，仍走默认 true（与显式 false 互不干扰）。
	implicit := &model.ChannelAgentBinding{
		ChannelType: cabChannelType, AccountID: "acc-default",
		AgentID: 9102,
	}
	if err := repo.Create(ctxA, implicit); err != nil {
		t.Fatalf("创建默认绑定失败: %v", err)
	}
	var gotImplicit model.ChannelAgentBinding
	if err := db.WithContext(ctxA).First(&gotImplicit, implicit.ID).Error; err != nil {
		t.Fatalf("回读默认绑定失败: %v", err)
	}
	if !flagOrTrue(gotImplicit.IsPrimary) || !flagOrTrue(gotImplicit.Enabled) {
		t.Errorf("未传开关时应落默认 true，实际 primary=%v enabled=%v",
			flagOrTrue(gotImplicit.IsPrimary), flagOrTrue(gotImplicit.Enabled))
	}
}

// TestChannelAgentBindingUpdateDeleteStayInTenant Update/Delete 只作用自己的租户：
// 0 行命中必须报 ErrRecordNotFound，而不是假成功或按主键搬行。
func TestChannelAgentBindingUpdateDeleteStayInTenant(t *testing.T) {
	db := cabDB(t)
	ctxA := tenant.WithTenantID(context.Background(), cabTenantA)
	ctxB := tenant.WithTenantID(context.Background(), cabTenantB)
	repo := NewChannelAgentBindingRepositoryWithDB(db)

	bRow := cabSeed(t, db, ctxB, 9201, true)

	stolen := *bRow
	stolen.AgentID = 9999
	if err := repo.Update(ctxA, &stolen); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户 Update 应报 ErrRecordNotFound，实际: %v", err)
	}
	if err := repo.Delete(ctxA, bRow.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户 Delete 应报 ErrRecordNotFound，实际: %v", err)
	}

	var bAfter model.ChannelAgentBinding
	if err := db.WithContext(ctxB).First(&bAfter, bRow.ID).Error; err != nil {
		t.Fatalf("租户 B 的行不应被删除/搬走: %v", err)
	}
	if bAfter.AgentID != 9201 || bAfter.TenantID != cabTenantB {
		t.Fatalf("租户 B 的行被跨租户改写: tenant=%d agent_id=%d", bAfter.TenantID, bAfter.AgentID)
	}
}
