// community_tenant_test.go 社群三张表的租户契约与写路径原子性（实体 #62 审计补充）。
//
// community_groups / community_members / community_messages 都带 tenant_id，读写由隔离
// callback 兜底。本文件锁住：
//  1. 两租户的群组、成员、消息互不可见，统计只算本租户；
//  2. AddMember 的「插成员 + 加计数」在群组不存在时整体回滚（不留孤儿成员）；
//  3. RemoveMember 在 member_count 已是 0 时不会减成负数；
//  4. 分页 page=0/pageSize=0 被归一（不会算出负 Offset 或退化成不限制）。
package repository

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	communityTenantA = uint(311)
	communityTenantB = uint(312)
)

func communitySetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t,
		&model.CommunityGroup{},
		&model.CommunityMember{},
		&model.CommunityMessage{},
	)
	if db == nil {
		t.Fatal("测试库不可达：community 的租户契约判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func TestCommunityRepository_TenantIsolation(t *testing.T) {
	db := communitySetupDB(t)
	repo := NewCommunityRepository(db)
	ctxA := tenant.WithTenantID(context.Background(), communityTenantA)
	ctxB := tenant.WithTenantID(context.Background(), communityTenantB)
	sys := tenant.WithSystemScope(context.Background())

	groupA, err := repo.CreateGroup(ctxA, &model.CommunityGroup{ID: uuid.New().String(), Name: "A 群"})
	if err != nil {
		t.Fatalf("A 建群失败: %v", err)
	}
	if _, err := repo.CreateGroup(ctxB, &model.CommunityGroup{ID: uuid.New().String(), Name: "B 群"}); err != nil {
		t.Fatalf("B 建群失败: %v", err)
	}
	if _, err := repo.AddMember(ctxA, &model.CommunityMember{
		ID: uuid.New().String(), GroupID: groupA.ID, Name: "成员", Username: "sameuser", Role: "member", Status: "active",
	}); err != nil {
		t.Fatalf("A 加成员失败: %v", err)
	}
	if err := db.WithContext(ctxA).Create(&model.CommunityMessage{
		ID: uuid.New().String(), GroupID: groupA.ID, UserID: "u1", UserName: "u1", Content: "hi", MessageType: "text",
	}).Error; err != nil {
		t.Fatalf("A 写消息失败: %v", err)
	}

	groupsA, totalA, err := repo.GetGroups(ctxA, 1, 20, "")
	if err != nil {
		t.Fatalf("A 列群失败: %v", err)
	}
	if totalA != 1 || len(groupsA) != 1 || groupsA[0].ID != groupA.ID {
		t.Fatalf("A 只该看见自己的群: total=%d len=%d", totalA, len(groupsA))
	}
	groupsB, totalB, err := repo.GetGroups(ctxB, 1, 20, "")
	if err != nil {
		t.Fatalf("B 列群失败: %v", err)
	}
	if totalB != 1 || groupsB[0].ID == groupA.ID {
		t.Fatalf("B 不该看见 A 的群: total=%d", totalB)
	}

	if exists, err := repo.ExistsMemberByUsername(ctxB, groupA.ID, "sameuser"); err != nil || exists {
		t.Errorf("B 不该看见 A 群里的成员: exists=%v err=%v", exists, err)
	}
	if members, total, err := repo.GetMembers(ctxA, groupA.ID, 1, 20, ""); err != nil || total != 1 || len(members) != 1 {
		t.Errorf("A 的成员列表应只有 1 条: total=%d err=%v", total, err)
	}
	if _, total, err := repo.GetMessages(ctxA, groupA.ID, 1, 20); err != nil || total != 1 {
		t.Errorf("A 的消息列表应只有 1 条: total=%d err=%v", total, err)
	}
	if _, total, err := repo.GetMessages(ctxB, groupA.ID, 1, 20); err != nil || total != 0 {
		t.Errorf("B 不该看见 A 群的消息: total=%d err=%v", total, err)
	}

	statsA, err := repo.GetStatistics(ctxA)
	if err != nil {
		t.Fatalf("A 统计失败: %v", err)
	}
	if statsA.TotalGroups != 1 || statsA.TotalMembers != 1 || statsA.TotalMessages != 1 {
		t.Errorf("A 统计串租户: %+v", statsA)
	}
	statsB, err := repo.GetStatistics(ctxB)
	if err != nil {
		t.Fatalf("B 统计失败: %v", err)
	}
	if statsB.TotalGroups != 1 || statsB.TotalMembers != 0 || statsB.TotalMessages != 0 {
		t.Errorf("B 统计串租户: %+v", statsB)
	}

	var allGroups int64
	if err := db.WithContext(sys).Model(&model.CommunityGroup{}).Count(&allGroups).Error; err != nil {
		t.Fatalf("系统作用域回读失败: %v", err)
	}
	if allGroups != 2 {
		t.Errorf("系统作用域应看到 2 个群，实际 %d", allGroups)
	}
}

// TestCommunityRepository_AddMemberRollsBackWhenGroupMissing 群组不存在时，
// 成员插入必须随事务回滚，不能留下 group_id 指向空号的孤儿行。
func TestCommunityRepository_AddMemberRollsBackWhenGroupMissing(t *testing.T) {
	db := communitySetupDB(t)
	repo := NewCommunityRepository(db)
	ctx := tenant.WithTenantID(context.Background(), communityTenantA)

	_, err := repo.AddMember(ctx, &model.CommunityMember{
		ID: uuid.New().String(), GroupID: "missing-group", Name: "孤儿", Username: "orphan", Role: "member", Status: "active",
	})
	if err == nil {
		t.Fatal("群组不存在时 AddMember 应失败")
	}
	sys := tenant.WithSystemScope(context.Background())
	var count int64
	if err := db.WithContext(sys).Model(&model.CommunityMember{}).Count(&count).Error; err != nil {
		t.Fatalf("回读失败: %v", err)
	}
	if count != 0 {
		t.Errorf("事务应回滚成员插入，实际残留 %d 行", count)
	}
}

// TestCommunityRepository_RemoveMemberKeepsCountNonNegative 历史数据的 member_count
// 可能已经是 0，移除成员不得把它减成负数。
func TestCommunityRepository_RemoveMemberKeepsCountNonNegative(t *testing.T) {
	db := communitySetupDB(t)
	repo := NewCommunityRepository(db)
	ctx := tenant.WithTenantID(context.Background(), communityTenantA)

	group, err := repo.CreateGroup(ctx, &model.CommunityGroup{ID: uuid.New().String(), Name: "零计数群", MemberCount: 0})
	if err != nil {
		t.Fatalf("建群失败: %v", err)
	}
	member := &model.CommunityMember{
		ID: uuid.New().String(), GroupID: group.ID, Name: "成员", Username: "u1", Role: "member", Status: "active",
	}
	if err := db.WithContext(ctx).Create(member).Error; err != nil {
		t.Fatalf("seed 成员失败: %v", err)
	}
	if err := repo.RemoveMember(ctx, member.ID); err != nil {
		t.Fatalf("移除成员失败: %v", err)
	}
	got, err := repo.GetGroupByID(ctx, group.ID)
	if err != nil || got == nil {
		t.Fatalf("回读群失败: %v", err)
	}
	if got.MemberCount != 0 {
		t.Errorf("member_count 不应为负: %d", got.MemberCount)
	}
}

// TestCommunityRepository_NormalizesPaging page=0/pageSize=0 不能落成负 Offset
// 或「不限制」（PG 语义），否则首屏直接 500 / 全表载入。
func TestCommunityRepository_NormalizesPaging(t *testing.T) {
	db := communitySetupDB(t)
	repo := NewCommunityRepository(db)
	ctx := tenant.WithTenantID(context.Background(), communityTenantA)

	group, err := repo.CreateGroup(ctx, &model.CommunityGroup{ID: uuid.New().String(), Name: "分页群"})
	if err != nil {
		t.Fatalf("建群失败: %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := repo.AddMember(ctx, &model.CommunityMember{
			ID: uuid.New().String(), GroupID: group.ID, Name: "成员", Username: "u" + string(rune('a'+i)), Role: "member", Status: "active",
		}); err != nil {
			t.Fatalf("加成员失败: %v", err)
		}
	}

	groups, total, err := repo.GetGroups(ctx, 0, 0, "")
	if err != nil {
		t.Fatalf("GetGroups(0,0) 应被归一: %v", err)
	}
	if total != 1 || len(groups) != 1 {
		t.Errorf("GetGroups(0,0) total=%d len=%d", total, len(groups))
	}
	members, total, err := repo.GetMembers(ctx, group.ID, 0, 0, "")
	if err != nil {
		t.Fatalf("GetMembers(0,0) 应被归一: %v", err)
	}
	if total != 3 || len(members) != 3 {
		t.Errorf("GetMembers(0,0) total=%d len=%d", total, len(members))
	}
	messages, total, err := repo.GetMessages(ctx, group.ID, 0, 0)
	if err != nil {
		t.Fatalf("GetMessages(0,0) 应被归一: %v", err)
	}
	if total != 0 || len(messages) != 0 {
		t.Errorf("GetMessages(0,0) total=%d len=%d", total, len(messages))
	}
}
