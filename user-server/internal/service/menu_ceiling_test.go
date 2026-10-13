// 菜单授权上界的行为件：注册表 Roles 是**每次读**都要过的上界，不只是首空表的初值。
//
// 判据分三条腿，缺一就不算这条规则落地：
//   - 读侧（侧栏树 / 分配回显 / 能力面守卫的授予集合）都把库里的分配行收进上界内；
//   - 写侧直接拒收上界外的 key（落库会被读侧抹掉＝"保存成功但没生效"，最难查）；
//   - admin 恒全量，不受上界影响。
//
// 用 fake repository 打桩，不碰库：这里判的是"两份授权怎么折算成一份"，与 SQL 无关。
package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
)

// fakeMenuRepo 只记录调用，不做持久化。
type fakeMenuRepo struct {
	rowsByRole map[string][]string
	totalRows  int64

	seeded     int
	replaced   []string // 每次 ReplaceRoleKeys 记录的 key（按调用顺序拼接）
	replaceCnt int
}

func newFakeMenuRepo(rows map[string][]string, total int64) *fakeMenuRepo {
	return &fakeMenuRepo{rowsByRole: rows, totalRows: total}
}

func (f *fakeMenuRepo) ListKeysByRole(_ context.Context, role string) ([]string, error) {
	return append([]string(nil), f.rowsByRole[role]...), nil
}

func (f *fakeMenuRepo) ReplaceRoleKeys(_ context.Context, _ string, keys []string) error {
	f.replaceCnt++
	f.replaced = append(f.replaced, keys...)
	return nil
}

func (f *fakeMenuRepo) CountAll(context.Context) (int64, error) { return f.totalRows, nil }

func (f *fakeMenuRepo) BulkInsert(context.Context, []*model.RoleMenuPermission) error {
	f.seeded++
	return nil
}

var _ repository.RoleMenuPermissionRepository = (*fakeMenuRepo)(nil)

// menuKeysIn 把过滤后的树摊平成 key 集合。
func menuKeysIn(nodes []model.SystemMenuNode) map[string]struct{} {
	out := map[string]struct{}{}
	var walk func([]model.SystemMenuNode)
	walk = func(ns []model.SystemMenuNode) {
		for _, n := range ns {
			out[n.Key] = struct{}{}
			walk(n.Children)
		}
	}
	walk(nodes)
	return out
}

func mustHave(t *testing.T, set map[string]struct{}, key string, msg string) {
	t.Helper()
	if _, ok := set[key]; !ok {
		t.Fatalf("%s：%s 应在集合里", msg, key)
	}
}

func mustLack(t *testing.T, set map[string]struct{}, key, msg string) {
	t.Helper()
	if _, ok := set[key]; ok {
		t.Fatalf("%s：%s 不该在集合里（注册表已收归超管）", msg, key)
	}
}

// TestMenuService_LegacyGrantRowsOutsideCeilingAreInert 老库里留着超管专属节点的分配行，
// 读侧三处都必须不再放行。
//
// browserAutomation 是收归超管的目录，baTasks 是它自身词表为空的子节点——
// 子节点靠祖先链一起出局，这条如果不成立，"父菜单藏了、子接口还能调"就还在。
func TestMenuService_LegacyGrantRowsOutsideCeilingAreInert(t *testing.T) {
	granted := []string{"inbox", "customerSession", "browserAutomation", "baTasks", "sopAgent"}
	repo := newFakeMenuRepo(map[string][]string{"customer_service": granted}, int64(len(granted)))
	svc := NewMenuServiceWithRepo(repo)
	ctx := context.Background()

	tree, err := svc.GetUserMenuTree(ctx, "customer_service")
	if err != nil {
		t.Fatalf("GetUserMenuTree: %v", err)
	}
	visibleKeys := menuKeysIn(tree)
	mustHave(t, visibleKeys, "inbox", "侧栏树")
	mustHave(t, visibleKeys, "customerSession", "侧栏树")
	for _, k := range []string{"browserAutomation", "baTasks", "sopAgent"} {
		mustLack(t, visibleKeys, k, "侧栏树")
	}

	keys, err := svc.GetRoleMenuKeys(ctx, "customer_service")
	if err != nil {
		t.Fatalf("GetRoleMenuKeys: %v", err)
	}
	echoed := map[string]struct{}{}
	for _, k := range keys {
		echoed[k] = struct{}{}
	}
	mustHave(t, echoed, "inbox", "分配回显")
	mustLack(t, echoed, "sopAgent", "分配回显")
	if len(echoed) != 2 {
		t.Fatalf("分配回显应只剩上界内的 2 个 key，实际 %d：%v", len(echoed), keys)
	}

	grants, err := svc.GrantProvider(ctx, "customer_service")
	if err != nil {
		t.Fatalf("GrantProvider: %v", err)
	}
	mustHave(t, grants, "inbox", "能力面授予")
	mustLack(t, grants, "browserAutomation", "能力面授予")
	mustLack(t, grants, "baTasks", "能力面授予")
	mustLack(t, grants, "sopAgent", "能力面授予")
}

// TestMenuService_AdminUnaffectedByCeiling 超管恒全量：上界只收窄非超管。
func TestMenuService_AdminUnaffectedByCeiling(t *testing.T) {
	repo := newFakeMenuRepo(map[string][]string{}, 0)
	svc := NewMenuServiceWithRepo(repo)
	ctx := context.Background()

	tree, err := svc.GetUserMenuTree(ctx, "admin")
	if err != nil {
		t.Fatalf("GetUserMenuTree(admin): %v", err)
	}
	if len(tree) != len(model.SystemMenuTree) {
		t.Fatalf("admin 应拿到整棵注册表：%d 个顶层节点，实际 %d", len(model.SystemMenuTree), len(tree))
	}
	adminKeys, err := svc.GetRoleMenuKeys(ctx, "admin")
	if err != nil {
		t.Fatalf("GetRoleMenuKeys(admin): %v", err)
	}
	if len(adminKeys) != len(model.FlattenMenuKeys(model.SystemMenuTree)) {
		t.Fatalf("admin 的 key 数应与注册表全量一致，实际 %d", len(adminKeys))
	}
	echoed := map[string]struct{}{}
	for _, k := range adminKeys {
		echoed[k] = struct{}{}
	}
	for _, k := range []string{"browserAutomation", "sopAgent"} {
		mustHave(t, echoed, k, "admin 分配回显")
	}
	// 守卫对 admin 短路（nil 集合＝不走授予判断），上界不该把它算成"无授予"
	grants, err := svc.GrantProvider(ctx, "admin")
	if err != nil {
		t.Fatalf("GrantProvider(admin): %v", err)
	}
	if grants != nil {
		t.Fatalf("admin 不该有授予集合（守卫短路），实际 %d 个", len(grants))
	}
}

// TestMenuService_UpdateRejectsCeilingViolation 写侧拒收上界外的 key，且不落库。
func TestMenuService_UpdateRejectsCeilingViolation(t *testing.T) {
	repo := newFakeMenuRepo(map[string][]string{}, 10)
	svc := NewMenuServiceWithRepo(repo)
	ctx := context.Background()

	err := svc.UpdateRoleMenuKeys(ctx, "staff", []string{"inbox", "geoTools"})
	if err == nil {
		t.Fatal("授予超管专属节点应当被拒")
	}
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("拒绝理由应是 ErrInvalidInput，实际 %v", err)
	}
	if !strings.Contains(err.Error(), "geoTools") {
		t.Fatalf("拒绝理由要点名被拒的菜单 key，实际 %v", err)
	}
	if repo.replaceCnt != 0 {
		t.Fatalf("被拒的分配不该落库，实际写了 %d 次", repo.replaceCnt)
	}

	// 同一次调用里上界内的 key 也不该被部分写入（覆盖式保存要么整批成、要么整批不动）
	if err := svc.UpdateRoleMenuKeys(ctx, "staff", []string{"inbox"}); err != nil {
		t.Fatalf("上界内的 key 应可授予: %v", err)
	}
	if repo.replaceCnt != 1 || repo.replaced[0] != "inbox" {
		t.Fatalf("上界内的授予应原样落库，实际 cnt=%d keys=%v", repo.replaceCnt, repo.replaced)
	}
}

// TestMenuService_EmptyTableSeedIsWithinCeiling 首空表的出厂分配本身就在上界内，
// 且守卫那条读路径不写库。
func TestMenuService_EmptyTableSeedIsWithinCeiling(t *testing.T) {
	repo := newFakeMenuRepo(map[string][]string{}, 0)
	svc := NewMenuServiceWithRepo(repo)
	ctx := context.Background()

	for _, role := range []string{"customer_service", "staff"} {
		keys, err := svc.GetRoleMenuKeys(ctx, role)
		if err != nil {
			t.Fatalf("GetRoleMenuKeys(%s): %v", role, err)
		}
		def := model.DefaultMenuKeys(role)
		if len(keys) != len(def) {
			t.Fatalf("%s 首空表应给出厂分配 %d 个，实际 %d 个", role, len(def), len(keys))
		}
		eligible := model.EligibleMenuKeys(role)
		for _, k := range def {
			if _, ok := eligible[k]; !ok {
				t.Fatalf("出厂分配 %s 越过了注册表上界（role=%s）", k, role)
			}
		}
	}
	if repo.seeded == 0 {
		t.Fatal("界面读路径在首空表时应 seed 一次")
	}

	// 守卫路径：同一份空表，只读不写
	before := repo.seeded
	if _, err := svc.GrantProvider(ctx, "customer_service"); err != nil {
		t.Fatalf("GrantProvider: %v", err)
	}
	if repo.seeded != before {
		t.Fatalf("守卫不该在请求链路上写库（seed %d → %d）", before, repo.seeded)
	}
}

// TestMenuService_GrantProviderRejectsUnknownRole 非法/空角色的授予集合是空集：
// 能力面对"认不出身份档位"的请求按无授予拦，而不是放行。
func TestMenuService_GrantProviderRejectsUnknownRole(t *testing.T) {
	repo := newFakeMenuRepo(map[string][]string{"customer_service": {"inbox"}}, 1)
	svc := NewMenuServiceWithRepo(repo)
	ctx := context.Background()
	for _, role := range []string{"no_such_role", ""} {
		grants, err := svc.GrantProvider(ctx, role)
		if err != nil {
			t.Fatalf("GrantProvider(%q): %v", role, err)
		}
		if len(grants) != 0 {
			t.Fatalf("非法角色 %q 的授予集合应为空，实际 %d 个", role, len(grants))
		}
	}
}

// TestEligibleMenuKeys_CeilingShapes 把上界的四种词表形状钉死：
// 空词表＝全员可见、陌生词＝只有超管、viewer 折算给员工不折算给客服、非法角色＝空集。
//
// 审计器（scripts/menu-route-role-audit.py 的 role_allows/visible）按同一套形状建模，
// 它的 --self-check 里钉的是 Python 那份；这里钉 Go 那份。两边同时改才会对得上，
// 单边分叉的读数是"审计绿、运行时漏"。
func TestEligibleMenuKeys_CeilingShapes(t *testing.T) {
	cs := model.EligibleMenuKeys("customer_service")
	staff := model.EligibleMenuKeys("staff")

	// 空词表＝全员可见（这是原设计，收口时只动运维面）
	if _, ok := cs["workspace"]; !ok {
		t.Fatal("workspace（词表为空）应对客服可见")
	}
	// manager 这类不在 3 档里的词，只有 admin 命中
	if _, ok := staff["sopAgent"]; ok {
		t.Fatal("sopAgent 已收归超管/经理档，普通员工不该可见")
	}
	// viewer 词表折算给 staff，不折算给 customer_service
	if _, ok := staff["tagSegmentation"]; !ok {
		t.Fatal("tagSegmentation（viewer 词表）应对员工可见")
	}
	if _, ok := cs["tagSegmentation"]; ok {
		t.Fatal("tagSegmentation（viewer 词表）不该对客服可见")
	}
	// 非法角色＝空集合（向拒的方向收）
	if len(model.EligibleMenuKeys("no_such_role")) != 0 {
		t.Fatal("非法角色的上界应为空集")
	}
	if len(model.EligibleMenuKeys("")) != 0 {
		t.Fatal("空角色的上界应为空集")
	}
	// 上界必须是全量 key 的子集，且 admin 拿到全量
	all := model.FlattenMenuKeys(model.SystemMenuTree)
	if len(model.EligibleMenuKeys("admin")) != len(all) {
		t.Fatalf("admin 上界应为全量 %d", len(all))
	}
	if len(cs) >= len(all) || len(staff) >= len(all) {
		t.Fatalf("非超管上界必须严格小于全量（cs=%d staff=%d all=%d）", len(cs), len(staff), len(all))
	}
}
