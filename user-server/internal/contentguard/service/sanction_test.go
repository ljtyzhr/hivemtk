// sanction_test.go 处置（冻结/黑名单）服务的行为断言。
//
// 三条线上最容易搞错的口径，逐条钉住：
//  1. 生效判定**只看时间**（不依赖定时任务）；
//  2. 幂等**升不降**（永久不会被一条新的临时记录减刑）；
//  3. 缓存**必须立刻失效**（运营点了"解除"而用户还在被拦 = 系统坏了）。
package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"hivemtk-user/internal/contentguard/memory"
	"hivemtk-user/internal/contentguard/model"
	"hivemtk-user/internal/contentguard/service"
)

func newSanctionSvc(t *testing.T) (*service.SanctionService, *memory.Store, *fakeClock) {
	t.Helper()
	clock := newClock()
	store := memory.New()
	store.SetNow(clock.Now)
	deps := store.Deps()
	rt := service.NewRuntime(service.RuntimeDeps{Now: clock.Now, Sanctions: deps.Sanctions})
	return rt.Sanction, store, clock
}

func TestSanctionApply_TempFreezeHasExpiry(t *testing.T) {
	svc, _, clock := newSanctionSvc(t)
	ctx := context.Background()

	row, err := svc.Apply(ctx, service.ApplyInput{
		SubjectType: model.SubjectUser, SubjectID: "u_1",
		Level: model.SanctionLevelTemp, Reason: "首次违规", DurationMinutes: 60,
	})
	if err != nil {
		t.Fatalf("Apply 报错：%v", err)
	}
	if row.ID == 0 || !row.Active || row.Kind != model.SanctionKindFreeze {
		t.Fatalf("处置行 = %+v，期望 active/freeze", row)
	}
	if row.ExpiresAt == nil || !row.ExpiresAt.Equal(clock.Now().Add(time.Hour)) {
		t.Fatalf("到期时间 = %v，期望 now+1h", row.ExpiresAt)
	}
	if _, ok := svc.IsBlocked(ctx, model.SubjectUser, "u_1"); !ok {
		t.Fatal("刚施加的处置应生效")
	}
}

func TestSanctionApply_DefaultDurationAndPermanent(t *testing.T) {
	svc, _, clock := newSanctionSvc(t)
	ctx := context.Background()

	row, err := svc.Apply(ctx, service.ApplyInput{
		SubjectType: model.SubjectUser, SubjectID: "u_temp", Level: model.SanctionLevelTemp,
	})
	if err != nil {
		t.Fatalf("Apply 报错：%v", err)
	}
	if row.ExpiresAt == nil || !row.ExpiresAt.Equal(clock.Now().Add(24*time.Hour)) {
		t.Fatalf("缺省临时冻结时长应为 24h，得 %v", row.ExpiresAt)
	}

	perm, err := svc.Apply(ctx, service.ApplyInput{
		SubjectType: model.SubjectUser, SubjectID: "u_perm", Level: model.SanctionLevelPermanent,
	})
	if err != nil {
		t.Fatalf("Apply 报错：%v", err)
	}
	if perm.ExpiresAt != nil {
		t.Fatalf("永久冻结不得有到期时间：%v", perm.ExpiresAt)
	}
	// 时间判定而非定时任务：一年后仍应生效。
	clock.Advance(365 * 24 * time.Hour)
	svc.InvalidateAll()
	if _, ok := svc.IsBlocked(ctx, model.SubjectUser, "u_perm"); !ok {
		t.Fatal("永久冻结不随时间失效")
	}
}

func TestSanctionIsBlocked_ExpiresWithoutAnyJob(t *testing.T) {
	svc, _, clock := newSanctionSvc(t)
	ctx := context.Background()
	if _, err := svc.Apply(ctx, service.ApplyInput{
		SubjectType: model.SubjectUser, SubjectID: "u_1",
		Level: model.SanctionLevelTemp, DurationMinutes: 30,
	}); err != nil {
		t.Fatalf("Apply 报错：%v", err)
	}

	clock.Advance(29 * time.Minute)
	if _, ok := svc.IsBlocked(ctx, model.SubjectUser, "u_1"); !ok {
		t.Fatal("未到期应仍生效")
	}
	clock.Advance(2 * time.Minute)
	// 缓存 TTL 小于本次时间跳跃，因此这里同时验证了"过期数据回源"。
	if row, ok := svc.IsBlocked(ctx, model.SubjectUser, "u_1"); ok {
		t.Fatalf("到期后应自动失效，得 %+v", row)
	}
}

func TestSanctionApply_NeverDowngradesPermanent(t *testing.T) {
	svc, store, clock := newSanctionSvc(t)
	ctx := context.Background()

	if _, err := svc.Apply(ctx, service.ApplyInput{
		SubjectType: model.SubjectUser, SubjectID: "u_1", Level: model.SanctionLevelPermanent,
		Reason: "严重违规",
	}); err != nil {
		t.Fatalf("Apply 报错：%v", err)
	}
	// 新的一条临时处置不得给永久处罚"减刑"。
	if _, err := svc.Apply(ctx, service.ApplyInput{
		SubjectType: model.SubjectUser, SubjectID: "u_1",
		Level: model.SanctionLevelTemp, DurationMinutes: 10, Reason: "又来一次",
	}); err != nil {
		t.Fatalf("Apply 报错：%v", err)
	}

	rows := store.Sanctions()
	if len(rows) != 1 {
		t.Fatalf("处置行数 = %d，期望 1（只续期/升级，不新增）", len(rows))
	}
	if rows[0].Level != model.SanctionLevelPermanent || rows[0].ExpiresAt != nil {
		t.Fatalf("永久档被降级：%+v", rows[0])
	}
	clock.Advance(time.Hour)
	if _, ok := svc.IsBlocked(ctx, model.SubjectUser, "u_1"); !ok {
		t.Fatal("永久冻结应持续生效")
	}
}

func TestSanctionApply_UpgradesAndRenews(t *testing.T) {
	svc, store, clock := newSanctionSvc(t)
	ctx := context.Background()

	first, err := svc.Apply(ctx, service.ApplyInput{
		SubjectType: model.SubjectUser, SubjectID: "u_1",
		Level: model.SanctionLevelTemp, DurationMinutes: 30,
	})
	if err != nil {
		t.Fatalf("Apply 报错：%v", err)
	}
	// 续期：新的到期时间更晚才覆盖（不缩短已有处罚）。
	clock.Advance(10 * time.Minute)
	if _, err := svc.Apply(ctx, service.ApplyInput{
		SubjectType: model.SubjectUser, SubjectID: "u_1",
		Level: model.SanctionLevelTemp, DurationMinutes: 60,
	}); err != nil {
		t.Fatalf("Apply 报错：%v", err)
	}
	rows := store.Sanctions()
	if len(rows) != 1 {
		t.Fatalf("处置行数 = %d，期望 1", len(rows))
	}
	if rows[0].ID != first.ID {
		t.Fatalf("续期应复用同一行：%d vs %d", rows[0].ID, first.ID)
	}
	if rows[0].ExpiresAt == nil || !rows[0].ExpiresAt.Equal(clock.Now().Add(time.Hour)) {
		t.Fatalf("续期后到期时间 = %v，期望 now+1h", rows[0].ExpiresAt)
	}

	// 升级：temp → permanent。
	if _, err := svc.Apply(ctx, service.ApplyInput{
		SubjectType: model.SubjectUser, SubjectID: "u_1", Level: model.SanctionLevelPermanent,
	}); err != nil {
		t.Fatalf("Apply 报错：%v", err)
	}
	up := store.Sanctions()[0]
	if up.Level != model.SanctionLevelPermanent || up.ExpiresAt != nil {
		t.Fatalf("未升级为永久：%+v", up)
	}
}

func TestSanctionRelease_IsImmediate(t *testing.T) {
	svc, store, _ := newSanctionSvc(t)
	ctx := context.Background()

	row, err := svc.Apply(ctx, service.ApplyInput{
		SubjectType: model.SubjectUser, SubjectID: "u_1", Level: model.SanctionLevelPermanent,
	})
	if err != nil {
		t.Fatalf("Apply 报错：%v", err)
	}
	if _, ok := svc.IsBlocked(ctx, model.SubjectUser, "u_1"); !ok {
		t.Fatal("前置条件失败")
	}

	rel, err := svc.Release(ctx, row.ID, 7, "人工复核：误判")
	if err != nil {
		t.Fatalf("Release 报错：%v", err)
	}
	if rel.Active || rel.ReleasedAt == nil || rel.ReleaseReason == "" || rel.OperatorID != 7 {
		t.Fatalf("解除结果 = %+v", rel)
	}
	// 缓存必须立刻失效：否则运营点了"解除"用户还在被拦。
	if got, ok := svc.IsBlocked(ctx, model.SubjectUser, "u_1"); ok {
		t.Fatalf("解除后应立即放行，得 %+v", got)
	}
	if rows := store.Sanctions(); len(rows) != 1 || rows[0].Active {
		t.Fatalf("库里的处置未标记解除：%+v", rows)
	}
}

func TestSanctionRelease_NotFound(t *testing.T) {
	svc, _, _ := newSanctionSvc(t)
	if _, err := svc.Release(context.Background(), 123, 1, "x"); !errors.Is(err, service.ErrSanctionNotFound) {
		t.Fatalf("不存在的处置应回 ErrSanctionNotFound，得 %v", err)
	}
}

func TestSanctionCache_ServesHotPathAndCanBeDropped(t *testing.T) {
	clock := newClock()
	store := memory.New()
	store.SetNow(clock.Now)
	deps := store.Deps()
	svc := service.NewSanctionService(deps.Sanctions)
	svc.SetCacheTTL(time.Hour)
	ctx := context.Background()

	if _, ok := svc.IsBlocked(ctx, model.SubjectUser, "u_1"); ok {
		t.Fatal("没有任何处置时应放行")
	}
	// 绕过服务层直接写库：缓存未失效期间读不到（这是"热路径不查库"的代价）。
	if err := deps.Sanctions.Create(ctx, &model.GuardSanction{
		SubjectType: model.SubjectUser, SubjectID: "u_1",
		Kind: model.SanctionKindFreeze, Level: model.SanctionLevelPermanent, Active: true,
	}); err != nil {
		t.Fatalf("写入处置失败：%v", err)
	}
	if _, ok := svc.IsBlocked(ctx, model.SubjectUser, "u_1"); ok {
		t.Fatal("缓存有效期内应命中缓存（热路径不查库）")
	}
	// 走服务层施加处置必须自己失效缓存。
	if _, err := svc.Apply(ctx, service.ApplyInput{
		SubjectType: model.SubjectUser, SubjectID: "u_2", Level: model.SanctionLevelPermanent,
	}); err != nil {
		t.Fatalf("Apply 报错：%v", err)
	}
	if _, ok := svc.IsBlocked(ctx, model.SubjectUser, "u_2"); !ok {
		t.Fatal("施加后必须立刻生效（缓存已失效）")
	}
	svc.InvalidateAll()
	if _, ok := svc.IsBlocked(ctx, model.SubjectUser, "u_1"); !ok {
		t.Fatal("清空缓存后应读到库里那条处置")
	}
}

func TestSanction_Validation(t *testing.T) {
	svc, store, _ := newSanctionSvc(t)
	ctx := context.Background()

	cases := []struct {
		name string
		in   service.ApplyInput
		want error
	}{
		{"缺主体", service.ApplyInput{SubjectType: model.SubjectUser},
			service.ErrSanctionSubjectRequired},
		{"主体类型非法", service.ApplyInput{SubjectType: "tenant", SubjectID: "x"},
			nil},
		{"处置类型非法", service.ApplyInput{SubjectType: model.SubjectUser, SubjectID: "x", Kind: "ban"},
			service.ErrSanctionKindInvalid},
		{"时长档非法", service.ApplyInput{SubjectType: model.SubjectUser, SubjectID: "x", Level: "forever"},
			service.ErrSanctionLevelInvalid},
	}
	for _, tc := range cases {
		_, err := svc.Apply(ctx, tc.in)
		if err == nil {
			t.Fatalf("%s：应报错", tc.name)
		}
		if tc.want != nil && !errors.Is(err, tc.want) {
			t.Fatalf("%s：错误 = %v，期望 %v", tc.name, err, tc.want)
		}
	}
	if n := len(store.Sanctions()); n != 0 {
		t.Fatalf("非法入参不得落库，得 %d 条", n)
	}

	// 黑名单类型是合法档位（会话运营与内容合规共用本表的两类动作）。
	if _, err := svc.Apply(ctx, service.ApplyInput{
		SubjectType: model.SubjectMerchant, SubjectID: "t_1",
		Kind: model.SanctionKindBlacklist, Level: model.SanctionLevelPermanent,
	}); err != nil {
		t.Fatalf("黑名单应合法：%v", err)
	}

	// 未装配仓储时返回 sentinel（控制器映射 503/400），不是 panic。
	var nilSvc *service.SanctionService
	if _, err := nilSvc.Apply(ctx, service.ApplyInput{SubjectType: model.SubjectUser, SubjectID: "x"}); !errors.Is(err, service.ErrSanctionStoreUnavailable) {
		t.Fatalf("未装配应回 ErrSanctionStoreUnavailable，得 %v", err)
	}
}

func TestSanctionList(t *testing.T) {
	svc, _, _ := newSanctionSvc(t)
	ctx := context.Background()
	for _, id := range []string{"u_1", "u_2", "u_3"} {
		if _, err := svc.Apply(ctx, service.ApplyInput{
			SubjectType: model.SubjectUser, SubjectID: id, Level: model.SanctionLevelPermanent,
		}); err != nil {
			t.Fatalf("Apply 报错：%v", err)
		}
	}
	rows, total, err := svc.List(ctx, service.SanctionQuery{SubjectType: model.SubjectUser})
	if err != nil {
		t.Fatalf("List 报错：%v", err)
	}
	if total != 3 || len(rows) != 3 {
		t.Fatalf("处置总数 = %d / %d，期望 3", total, len(rows))
	}
	empty, total, err := svc.List(ctx, service.SanctionQuery{SubjectID: "nobody"})
	if err != nil || total != 0 || len(empty) != 0 {
		t.Fatalf("按主体过滤应得 0 条：%v %d", err, total)
	}
}

func TestSanctionEffective_Judgement(t *testing.T) {
	now := time.Now()
	future := now.Add(time.Hour)
	past := now.Add(-time.Hour)

	cases := []struct {
		name string
		row  *model.GuardSanction
		want bool
	}{
		{"nil", nil, false},
		{"已解除", &model.GuardSanction{Active: false, Level: model.SanctionLevelPermanent}, false},
		{"永久", &model.GuardSanction{Active: true, Level: model.SanctionLevelPermanent}, true},
		{"临时未到期", &model.GuardSanction{Active: true, Level: model.SanctionLevelTemp, ExpiresAt: &future}, true},
		{"临时已到期", &model.GuardSanction{Active: true, Level: model.SanctionLevelTemp, ExpiresAt: &past}, false},
		// temp 无到期时间 = 数据缺陷 ⇒ fail-closed，按永久处理（宁可让人工来解）。
		{"临时缺到期时间", &model.GuardSanction{Active: true, Level: model.SanctionLevelTemp}, true},
	}
	for _, tc := range cases {
		if got := tc.row.Effective(now); got != tc.want {
			t.Fatalf("%s：Effective = %v，期望 %v", tc.name, got, tc.want)
		}
	}
}
