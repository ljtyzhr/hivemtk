// account_repo_update_test.go 桥接账号**更新路径**与冲突识别的回归
// （实体 #57 审计补充）。
//
// 三条判据各自对应一种曾经真实存在的静默坏法：
//  1. 更新走 `Save` 时，行在读与写之间消失会让 GORM 退化成一次"带旧主键的 Create"，
//     而那次插入不再经过任何归属校验；改成列白名单 map + 0 行报 ErrRecordNotFound 后，
//     本用例钉住"更新只动在线位/昵称/智能体，身份与创建时间不被改写"。
//  2. 唯一冲突识别只看 "duplicate key value" 时依赖 PG 消息本地化，非英文实例上
//     并发首建不会被识别成"已存在"而直接失败；本用例钉住 SQLSTATE + 索引名两个条件。
//  3. 管理面列表没有上限时，一次 GET 就是一次无界全表读；本用例钉住读取上限。
package bridge

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"
)

// TestBridgeAccountRepository_UpsertUpdateTouchesOnlyWhitelistedColumns
// 已存在行（归属同一用户）的再次上报只能刷新在线位/昵称/智能体：
// user_id / channel / account_id / created_at / tenant_id 必须原样保留。
func TestBridgeAccountRepository_UpsertUpdateTouchesOnlyWhitelistedColumns(t *testing.T) {
	db := testutil.NewTestDB(t, &model.BridgeAccount{}, &model.ChannelAgentBinding{})
	modeOn(t)
	repo := NewBridgeAccountRepository(db)
	ctx := baCtx(baTenantA)

	baUpsert(t, repo, ctx, BridgeAccountUpsert{
		UserID: 5, Channel: "douyin", AccountID: "acc-u", AccountName: "旧昵称", Status: "offline",
	})
	var before model.BridgeAccount
	if err := db.WithContext(ctx).Where("channel = ? AND account_id = ?", "douyin", "acc-u").
		First(&before).Error; err != nil {
		t.Fatalf("回读初值失败: %v", err)
	}

	baUpsert(t, repo, ctx, BridgeAccountUpsert{
		UserID: 5, Channel: "douyin", AccountID: "acc-u", AccountName: "新昵称", AgentID: 9, Status: "online",
	})

	var after model.BridgeAccount
	if err := db.WithContext(ctx).Where("id = ?", before.ID).First(&after).Error; err != nil {
		t.Fatalf("回读更新后失败: %v", err)
	}
	if after.UserID != 5 || after.Channel != "douyin" || after.AccountID != "acc-u" || after.TenantID != baTenantA {
		t.Errorf("身份列被改写: user_id=%d channel=%q account_id=%q tenant_id=%d",
			after.UserID, after.Channel, after.AccountID, after.TenantID)
	}
	if !after.CreatedAt.Equal(before.CreatedAt) {
		t.Errorf("created_at 被改写: %v → %v", before.CreatedAt, after.CreatedAt)
	}
	if after.Status != "online" || after.AccountName != "新昵称" || after.AgentID != 9 {
		t.Errorf("白名单列未更新: status=%q account_name=%q agent_id=%d",
			after.Status, after.AccountName, after.AgentID)
	}
	if after.LastSyncAt == nil {
		t.Error("last_sync_at 应被刷新")
	}
}

// TestBridgeAccountRepository_UniqueViolationIsRecognizedByName
// 冲突识别必须同时认 SQLSTATE 23505 与本表索引名：只认前者时，
// 主键/别的表的冲突会被读成"账号已注册"；只认文案时，非英文实例直接失效。
func TestBridgeAccountRepository_UniqueViolationIsRecognizedByName(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{
			"PG 23505 + 本表索引",
			errors.New(`ERROR: duplicate key value violates unique constraint "uk_bridge_ch_acc" (SQLSTATE 23505)`),
			true,
		},
		{
			"只有索引名没有 SQLSTATE",
			errors.New(`ERROR: duplicate key value violates unique constraint "uk_bridge_ch_acc"`),
			false,
		},
		{
			"别的唯一索引",
			errors.New(`ERROR: duplicate key value violates unique constraint "bridge_accounts_pkey" (SQLSTATE 23505)`),
			false,
		},
		{
			// 这一格是本次修改的理由：文案本地化后 "duplicate key value" 不再出现，
			// 但 SQLSTATE 与索引名仍在 —— 必须照样认出来，否则非英文实例的并发首建直接失败。
			"本地化文案（有索引名）",
			errors.New(`ERROR: 违反唯一约束 "uk_bridge_ch_acc" (SQLSTATE 23505)`),
			true,
		},
		{"普通错误", errors.New("connection reset by peer"), false},
	}
	for _, tc := range cases {
		if got := isUniqueViolation(tc.err); got != tc.want {
			t.Errorf("%s: isUniqueViolation=%v want %v", tc.name, got, tc.want)
		}
	}
}

// TestBridgeAccountRepository_ListForUserIsBounded 列表读取必须有上限：
// 无界时一次 GET 就是一次租户内全表捞，而网页扩展长期运行会累积历史账号行。
func TestBridgeAccountRepository_ListForUserIsBounded(t *testing.T) {
	db := testutil.NewTestDB(t, &model.BridgeAccount{})
	modeOn(t)
	repo := NewBridgeAccountRepository(db)
	ctx := baCtx(baTenantA)

	rows := make([]model.BridgeAccount, 0, bridgeAccountListLimit+5)
	for i := 0; i < bridgeAccountListLimit+5; i++ {
		rows = append(rows, model.BridgeAccount{
			Channel: "douyin", AccountID: "acc-" + strconv.Itoa(i), Status: "offline",
		})
	}
	if err := db.WithContext(ctx).CreateInBatches(rows, 100).Error; err != nil {
		t.Fatalf("批量造行失败: %v", err)
	}

	list, err := repo.ListForUser(ctx, 0)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if len(list) != bridgeAccountListLimit {
		t.Errorf("列表长度=%d want %d（读取上限）", len(list), bridgeAccountListLimit)
	}

	var total int64
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).
		Model(&model.BridgeAccount{}).Count(&total).Error; err != nil {
		t.Fatalf("计数失败: %v", err)
	}
	if total != int64(bridgeAccountListLimit+5) {
		t.Fatalf("夹具应存在 %d 行，实际 %d", bridgeAccountListLimit+5, total)
	}
}
