// community_tenant_test.go 社群 service 层的租户契约与新增业务规则（实体 #62 审计补充）。
//
// 判重与「群组必须存在」是业务规则，已从仓储搬到 service。本文件锁住：
//  1. 加成员前必须能在**调用方租户内**看到该群组，否则拒绝（不产生孤儿成员）；
//  2. 列表与统计只反映本租户的数据；
//  3. 统计结果直接来自仓储的类型化结构，不再有 map 类型断言。
package service

import (
	"context"
	"strings"
	"testing"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"hivemtk-user/internal/repository"
)

const (
	communitySvcTenantA = uint(313)
	communitySvcTenantB = uint(314)
)

func communityServiceSetup(t *testing.T) *CommunityService {
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
		t.Fatal("测试库不可达：community service 的租户契约判不了（不 Skip，跳过等于没锁）")
	}
	return NewCommunityService(repository.NewCommunityRepository(db))
}

func TestCommunityService_AddMemberRequiresVisibleGroup(t *testing.T) {
	svc := communityServiceSetup(t)
	ctxA := tenant.WithTenantID(context.Background(), communitySvcTenantA)
	ctxB := tenant.WithTenantID(context.Background(), communitySvcTenantB)

	group, err := svc.CreateGroup(ctxA, &dto.CreateCommunityGroupRequest{Name: "A 群"})
	if err != nil {
		t.Fatalf("A 建群失败: %v", err)
	}

	// B 看不到 A 的群：不能往里面加人。
	_, err = svc.AddMember(ctxB, &dto.AddCommunityMemberRequest{
		GroupID: group.ID, Name: "越界成员", Username: "intruder", Role: "member",
	})
	if err == nil || !strings.Contains(err.Error(), "社群不存在") {
		t.Fatalf("跨租户加成员应报「社群不存在」，err=%v", err)
	}

	// 群组完全不存在时同样拒绝。
	_, err = svc.AddMember(ctxA, &dto.AddCommunityMemberRequest{
		GroupID: "missing-group", Name: "孤儿", Username: "orphan", Role: "member",
	})
	if err == nil || !strings.Contains(err.Error(), "社群不存在") {
		t.Fatalf("群组不存在应报「社群不存在」，err=%v", err)
	}

	members, total, err := svc.GetMembers(ctxA, group.ID, 1, 20, "")
	if err != nil {
		t.Fatalf("A 读成员失败: %v", err)
	}
	if total != 0 || len(members) != 0 {
		t.Errorf("被拒绝的加成员不应落库: total=%d", total)
	}
}

func TestCommunityService_ListsAndStatsAreTenantScoped(t *testing.T) {
	svc := communityServiceSetup(t)
	ctxA := tenant.WithTenantID(context.Background(), communitySvcTenantA)
	ctxB := tenant.WithTenantID(context.Background(), communitySvcTenantB)

	group, err := svc.CreateGroup(ctxA, &dto.CreateCommunityGroupRequest{Name: "A 群"})
	if err != nil {
		t.Fatalf("A 建群失败: %v", err)
	}
	if _, err := svc.AddMember(ctxA, &dto.AddCommunityMemberRequest{
		GroupID: group.ID, Name: "A 成员", Username: "user-a", Role: "member",
	}); err != nil {
		t.Fatalf("A 加成员失败: %v", err)
	}

	groupsB, totalB, err := svc.GetGroups(ctxB, 1, 20, "")
	if err != nil {
		t.Fatalf("B 列群失败: %v", err)
	}
	if totalB != 0 || len(groupsB) != 0 {
		t.Errorf("B 不该看见 A 的群: total=%d", totalB)
	}

	statsB, err := svc.GetStatistics(ctxB)
	if err != nil {
		t.Fatalf("B 统计失败: %v", err)
	}
	if statsB.TotalGroups != 0 || statsB.TotalMembers != 0 || statsB.TotalMessages != 0 {
		t.Errorf("B 的统计串租户: %+v", statsB)
	}

	statsA, err := svc.GetStatistics(ctxA)
	if err != nil {
		t.Fatalf("A 统计失败: %v", err)
	}
	if statsA.TotalGroups != 1 || statsA.TotalMembers != 1 {
		t.Errorf("A 的统计不正确: %+v", statsA)
	}

	// 同租户内同名用户名仍被 service 判重。
	if _, err := svc.AddMember(ctxA, &dto.AddCommunityMemberRequest{
		GroupID: group.ID, Name: "重复", Username: "user-a", Role: "member",
	}); err == nil || !strings.Contains(err.Error(), "该用户名已在群组中") {
		t.Fatalf("同群同名用户名应被拒绝，err=%v", err)
	}
}
