package repository

// 契约锁（实体审查 #60，ChampionBaseline）：销冠基线的读写各锁在自己租户的分片里。
//
// 为什么这一格要单独钉：这张表的**版本号**是它全部语义的核心
// （`persona+industry+intent` 三元组 + version 递增 + enabled 只有一条为真），
// 而版本号来自「查 MAX(version) 再 +1」这个读-改-写形状。两个租户各自维护同一组
// 三元组的基线时，任何一处读漏了租户谓词，版本就会串：
//   - MAX(version) 跨租户 ⇒ A 租户写出版本 7、B 租户也写出版本 7 ⇒
//     同一 (persona, industry, intent) 下出现两个 enabled=true 且 version 相同，
//     而 `FindByPersonaIndustryIntent` 的 `Order("version DESC").First()` 在两个
//     同版本行之间**取哪一条是不确定的**（没有 tiebreaker）⇒ 基线读出来会随机翻面。
//   - 写侧漏绑租户 ⇒ 隔离 callback fail-closed，行根本写不进去（比串号好，但一样是故障）。
//
// 种子刻意让两个租户用**同一个**三元组：那样断言的才是 tenant_id 谓词本身，
// 而不是「两个租户恰好用了不同的 persona」这种随时会被复用拆穿的巧合。

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"
)

const (
	cbTenantA = uint(17)
	cbTenantB = uint(29)
	// 三元组两租户共用：版本串号的判据靠它。
	cbPersona  = "p-tenant-shared"
	cbIndustry = "i-shared"
	cbIntent   = "all"
)

// cbSeed 走被测的 CreateBaseline 落一行（不是夹具直插 db）：写侧的归属填充本身
// 就是被审的那件事。
func cbSeed(t *testing.T, ctx context.Context, version int, enabled bool) *model.ChampionBaseline {
	t.Helper()
	row := &model.ChampionBaseline{
		Persona: cbPersona, Industry: cbIndustry, Intent: cbIntent,
		Naturalness: 0.5, Conciseness: 0.5, Empathy: 0.5,
		Professionalism: 0.5, Persuasiveness: 0.5,
		SampleCount: 10, Version: version, Enabled: enabled,
	}
	if err := NewChampionBaselineRepository().CreateBaseline(ctx, row); err != nil {
		t.Fatalf("落基线失败: %v", err)
	}
	return row
}

// TestChampionBaselineCreateFillsTenantAndRejectsBareContext 写侧两格：绑租户的 ctx
// 落自己的号，裸 ctx 被拒。
func TestChampionBaselineCreateFillsTenantAndRejectsBareContext(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.ResetRegistryForTest(); tenant.SetMode(tenant.ModeOff) })

	db := testutil.NewTestDB(t, &model.ChampionBaseline{})
	if db == nil {
		t.Fatal("测试库不可达：基线归属无法判定（不 Skip，跳过等于没锁）")
	}
	ctxA := tenant.WithTenantID(context.Background(), cbTenantA)
	repo := NewChampionBaselineRepository()

	row := cbSeed(t, ctxA, 1, true)
	if row.TenantID != cbTenantA {
		t.Errorf("Create 后 TenantID got=%d want=%d：隔离 callback 没填发起租户，"+
			"这一行会落进默认租户名下", row.TenantID, cbTenantA)
	}

	if err := repo.CreateBaseline(context.Background(), &model.ChampionBaseline{
		Persona: cbPersona, Industry: cbIndustry, Intent: cbIntent, Version: 1, Enabled: true,
	}); err == nil {
		t.Error("裸 ctx 的 CreateBaseline 未被拒绝：on 模式下这等于允许把基线记到任意租户")
	}
}

// TestChampionBaselineMaxVersionIsTenantLocal 版本号必须租户内递增。
//
// 判据：两个租户各自写出版本 1、2、3，A 租户的 MaxVersion 必须是 3 而不是 6
// （跨租户求 MAX 会得到 6 ⇒ A 的下一版写成 7，中间空出 4/5/6 三版，且
// 同一三元组下出现两个 version=3 的 enabled 行）。
func TestChampionBaselineMaxVersionIsTenantLocal(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.ResetRegistryForTest(); tenant.SetMode(tenant.ModeOff) })

	db := testutil.NewTestDB(t, &model.ChampionBaseline{})
	if db == nil {
		t.Fatal("测试库不可达：版本号租户性无法判定")
	}
	ctxA := tenant.WithTenantID(context.Background(), cbTenantA)
	ctxB := tenant.WithTenantID(context.Background(), cbTenantB)
	repo := NewChampionBaselineRepository()

	for v := 1; v <= 3; v++ {
		cbSeed(t, ctxA, v, v == 3)
	}
	for v := 1; v <= 2; v++ {
		cbSeed(t, ctxB, v, v == 2)
	}

	maxA, err := repo.MaxVersion(ctxA, cbPersona, cbIndustry, cbIntent)
	if err != nil {
		t.Fatalf("A 租户查最大版本失败: %v", err)
	}
	if maxA != 3 {
		t.Errorf("A 租户的最大版本 got=%d want=3：MAX(version) 跨了租户"+
			"（A 自己的三版是 1/2/3，两租户合起来才是 5）⇒ 同三元组下会出现两个 "+
			"version 相同且都 enabled 的行，而读侧 Order(version DESC) 没有 tiebreaker", maxA)
	}

	maxB, err := repo.MaxVersion(ctxB, cbPersona, cbIndustry, cbIntent)
	if err != nil {
		t.Fatalf("B 租户查最大版本失败: %v", err)
	}
	if maxB != 2 {
		t.Errorf("B 租户的最大版本 got=%d want=2", maxB)
	}
}

// TestChampionBaselineReadIsTenantScoped 读侧：同一三元组在两租户各有一行时，
// 各自只读得到自己那条。
func TestChampionBaselineReadIsTenantScoped(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.ResetRegistryForTest(); tenant.SetMode(tenant.ModeOff) })

	db := testutil.NewTestDB(t, &model.ChampionBaseline{})
	if db == nil {
		t.Fatal("测试库不可达：读侧租户性无法判定")
	}
	ctxA := tenant.WithTenantID(context.Background(), cbTenantA)
	ctxB := tenant.WithTenantID(context.Background(), cbTenantB)
	repo := NewChampionBaselineRepository()

	rowA := cbSeed(t, ctxA, 1, true)
	rowA.SampleCount = 111
	rowB := cbSeed(t, ctxB, 1, true)
	rowB.SampleCount = 222

	gotA, err := repo.FindByPersonaIndustryIntent(ctxA, cbPersona, cbIndustry, cbIntent)
	if err != nil {
		t.Fatalf("A 租户读基线失败: %v", err)
	}
	if gotA == nil {
		t.Fatal("A 租户读不到自己的基线")
	}
	gotB, err := repo.FindByPersonaIndustryIntent(ctxB, cbPersona, cbIndustry, cbIntent)
	if err != nil {
		t.Fatalf("B 租户读基线失败: %v", err)
	}
	if gotB == nil {
		t.Fatal("B 租户读不到自己的基线")
	}
	// 两行的 version 相同（都是 1），所以读侧的区分只能靠 tenant_id：
	// 若读侧漏了租户谓词，Order(version DESC) 的 First() 会在两条同版本行里任取一条
	// ⇒ 读到的样本数会随机在 111/222 之间翻。
	if gotA.TenantID != cbTenantA || gotB.TenantID != cbTenantB {
		t.Errorf("读到的行租户不对：A=%d(应 %d) B=%d(应 %d)",
			gotA.TenantID, cbTenantA, gotB.TenantID, cbTenantB)
	}

	// 裸 ctx 同样 fail-closed：读侧一旦放行，「按三元组读别人的基线」就写回了库里。
	if _, err := repo.FindByPersonaIndustryIntent(context.Background(), cbPersona, cbIndustry, cbIntent); err == nil {
		t.Error("裸 ctx 的 FindByPersonaIndustryIntent 未被拒绝")
	}
}
