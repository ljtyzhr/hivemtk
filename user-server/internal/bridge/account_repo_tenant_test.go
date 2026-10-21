// account_repo_tenant_test.go bridge_accounts 的 SaaS 契约：
// 复合唯一键、租户读侧遮挡、裸 ctx fail-closed、归属冲突不得改写主人。
package bridge

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
	baTenantA uint = 401
	baTenantB uint = 402
)

func baCtx(id uint) context.Context {
	return tenant.WithTenantID(context.Background(), id)
}

func baUpsert(t *testing.T, repo *BridgeAccountRepository, ctx context.Context, u BridgeAccountUpsert) {
	t.Helper()
	if err := repo.Upsert(ctx, u); err != nil {
		t.Fatalf("Upsert(%s/%s) 失败: %v", u.Channel, u.AccountID, err)
	}
}

func baRawRows(t *testing.T, db *gorm.DB) []model.BridgeAccount {
	t.Helper()
	var rows []model.BridgeAccount
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).
		Order("id").Find(&rows).Error; err != nil {
		t.Fatalf("系统作用域回读失败: %v", err)
	}
	return rows
}

// TestBridgeAccountRepository_TenantScopedAndCompositeUnique 同一 (channel, account_id)
// 在两个租户各有一行：这依赖 Normalize 后的 UNIQUE (tenant_id, channel, account_id)，
// 单列唯一键会在第二个租户 Upsert 时撞 23505。读侧（列表/在线位/账号反查）必须只看本租户。
func TestBridgeAccountRepository_TenantScopedAndCompositeUnique(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.BridgeAccount{})
	modeOn(t)
	repo := NewBridgeAccountRepository(db)

	baUpsert(t, repo, baCtx(baTenantA), BridgeAccountUpsert{
		Channel: "douyin", AccountID: "acc-shared", Status: "online",
	})
	baUpsert(t, repo, baCtx(baTenantB), BridgeAccountUpsert{
		Channel: "douyin", AccountID: "acc-shared", Status: "online",
	})
	baUpsert(t, repo, baCtx(baTenantB), BridgeAccountUpsert{
		Channel: "douyin", AccountID: "acc-only-b", Status: "online",
	})

	rows := baRawRows(t, db)
	if len(rows) != 3 {
		t.Fatalf("应有 3 行（A 1 行 + B 2 行），实际 %d", len(rows))
	}
	wantTenants := []uint{baTenantA, baTenantB, baTenantB}
	for i, r := range rows {
		if r.TenantID != wantTenants[i] {
			t.Errorf("第 %d 行 %s 租户章=%d want %d（Create 必须按 ctx 填 tenant_id）",
				i, r.AccountID, r.TenantID, wantTenants[i])
		}
	}

	listA, err := repo.ListForUser(baCtx(baTenantA), 7)
	if err != nil {
		t.Fatalf("ListForUser(A): %v", err)
	}
	if len(listA) != 1 || listA[0].AccountID != "acc-shared" {
		t.Fatalf("A 只该看见自己的 1 行，实际 %+v", listA)
	}

	// B 掉线不能把 A 的在线位一起改掉。
	if err := repo.SetOffline(baCtx(baTenantB), "douyin", "acc-shared"); err != nil {
		t.Fatalf("SetOffline(B): %v", err)
	}
	if online, err := repo.IsOnline(baCtx(baTenantA), "douyin", "acc-shared"); err != nil || !online {
		t.Errorf("A 的账号应仍在线：online=%v err=%v", online, err)
	}
	if online, err := repo.IsOnline(baCtx(baTenantB), "douyin", "acc-shared"); err != nil || online {
		t.Errorf("B 的账号应已离线：online=%v err=%v", online, err)
	}

	// 跨租户反查不得命中也查不到别人的行；查无此行返回 (nil, nil)，供归属校验回 403。
	acc, err := repo.GetByChannelAccount(baCtx(baTenantA), "douyin", "acc-only-b")
	if err != nil {
		t.Fatalf("跨租户反查不该报错: %v", err)
	}
	if acc != nil {
		t.Errorf("跨租户反查命中了别的租户的行: %+v", acc)
	}
}

// TestBridgeAccountRepository_OnModeUnboundContextFailsClosed 裸 ctx 在 on 模式下
// 必须拒绝一切读写；尤其反查不能把租户闸读成"账号不存在"（那会让归属校验回 403，
// 把一次租户配置故障伪装成越权拒绝）。
func TestBridgeAccountRepository_OnModeUnboundContextFailsClosed(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.BridgeAccount{})
	modeOn(t)
	repo := NewBridgeAccountRepository(db)
	ctx := context.Background()

	if err := repo.Upsert(ctx, BridgeAccountUpsert{Channel: "douyin", AccountID: "acc", Status: "online"}); err == nil {
		t.Error("裸 ctx 的 Upsert 应被租户闸拒绝")
	}
	if _, err := repo.ListForUser(ctx, 1); err == nil {
		t.Error("裸 ctx 的 ListForUser 应被租户闸拒绝")
	}
	if _, err := repo.IsOnline(ctx, "douyin", "acc"); err == nil {
		t.Error("裸 ctx 的 IsOnline 应被租户闸拒绝")
	}
	acc, err := repo.GetByChannelAccount(ctx, "douyin", "acc")
	if err == nil {
		t.Errorf("裸 ctx 的 GetByChannelAccount 应报租户上下文错误，实际 acc=%+v err=nil", acc)
	}
	if acc != nil {
		t.Errorf("租户闸失败时不得返回账号行: %+v", acc)
	}

	var n int64
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).
		Model(&model.BridgeAccount{}).Count(&n).Error; err != nil {
		t.Fatalf("计数失败: %v", err)
	}
	if n != 0 {
		t.Errorf("fail-closed 后仍写入了 %d 行", n)
	}
}

// TestBridgeAccountRepository_OwnershipConflictKeepsOwner 同一租户内已认领给 user 1 的账号，
// user 2 再上报只能刷新在线位，不能改归属/昵称；错误必须可被 errors.Is 识别。
func TestBridgeAccountRepository_OwnershipConflictKeepsOwner(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.BridgeAccount{})
	modeOn(t)
	repo := NewBridgeAccountRepository(db)
	ctx := baCtx(baTenantA)

	baUpsert(t, repo, ctx, BridgeAccountUpsert{
		UserID: 1, Channel: "douyin", AccountID: "acc-owned", AccountName: "主人一号", Status: "online",
	})
	if err := repo.SetOffline(ctx, "douyin", "acc-owned"); err != nil {
		t.Fatalf("SetOffline: %v", err)
	}

	err := repo.Upsert(ctx, BridgeAccountUpsert{
		UserID: 2, Channel: "douyin", AccountID: "acc-owned", AccountName: "篡改名", Status: "online",
	})
	if !errors.Is(err, ErrAccountOwnedByOther) {
		t.Fatalf("期望 ErrAccountOwnedByOther，实际 %v", err)
	}

	var row model.BridgeAccount
	if err := db.WithContext(ctx).Where("channel = ? AND account_id = ?", "douyin", "acc-owned").
		First(&row).Error; err != nil {
		t.Fatalf("回读失败: %v", err)
	}
	if row.UserID != 1 {
		t.Errorf("归属被改写: user_id=%d want 1", row.UserID)
	}
	if row.AccountName != "主人一号" {
		t.Errorf("昵称被改写: %q want %q", row.AccountName, "主人一号")
	}
	if row.Status != "online" {
		t.Errorf("冲突分支仍须刷新在线位: status=%q want online", row.Status)
	}
}

// TestBridgeAccountRepository_ListForUserVisibility user_id=0 是租户资产（登记入口没有
// 用户身份），租户内用户都可见；非零认领只对本人可见。
func TestBridgeAccountRepository_ListForUserVisibility(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.BridgeAccount{})
	modeOn(t)
	repo := NewBridgeAccountRepository(db)
	ctx := baCtx(baTenantA)

	baUpsert(t, repo, ctx, BridgeAccountUpsert{UserID: 0, Channel: "douyin", AccountID: "acc-tenant", Status: "online"})
	baUpsert(t, repo, ctx, BridgeAccountUpsert{UserID: 7, Channel: "douyin", AccountID: "acc-mine", Status: "online"})
	baUpsert(t, repo, ctx, BridgeAccountUpsert{UserID: 8, Channel: "douyin", AccountID: "acc-other", Status: "online"})

	list, err := repo.ListForUser(ctx, 7)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	got := map[string]bool{}
	for _, v := range list {
		got[v.AccountID] = true
	}
	if !got["acc-tenant"] || !got["acc-mine"] {
		t.Errorf("应看见租户资产与本人账号，实际 %v", got)
	}
	if got["acc-other"] {
		t.Errorf("不该看见他人认领的账号，实际 %v", got)
	}
}
