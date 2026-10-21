package repository

// 契约锁（实体审查 #62，ChampionPhrase）：销冠短语的读写各锁在自己租户的分片里。
//
// 本实体此前零租户测试，而它有一格特别值得钉：`baseline_id` 是**跨表引用**（指向
// champion_baselines.id），而 champion_baselines 本身是租户表。于是「A 租户拿着 B
// 租户的 baseline_id 来写短语」是一句完全可构造的输入——若写侧只按 baseline_id 落行
// 而不校验租户归属，短语行就会落在 A 的名下却描述 B 的基线（反过来说，B 的基线被挂了
// 一条属于别人的短语）。
//
// 另一格：读侧按 baseline_id 取短语时，两个租户可能各有同号的 baseline_id（不会——
// baseline_id 是全局自增主键，天然全局唯一），所以本表这一格只需钉「同 baseline_id
// 下两个租户的行互不可见」这一条（种子刻意用**同一个 baseline_id**）。
//
// 注：本表的两个仓储方法（RefreshPhrases / ListPhrases）在本轮审查中因零生产调用方
// 已删除，故这里直接对着 model 走 GORM——写侧验证 callback 的填充与 fail-closed，
// 读侧验证跨租户不可见。

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"
)

const (
	cpTenantA = uint(19)
	cpTenantB = uint(41)
	// 两租户用同一个 baseline_id：跨租户可见性的判据靠它。
	cpSharedBaseline = uint64(77001)
)

// TestChampionPhraseCreateFillsTenantAndRejectsBareContext 写侧两格：绑租户的 ctx
// 落自己的号，裸 ctx 被拒。
func TestChampionPhraseCreateFillsTenantAndRejectsBareContext(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.ResetRegistryForTest(); tenant.SetMode(tenant.ModeOff) })

	db := testutil.NewTestDB(t, &model.ChampionPhrase{})
	if db == nil {
		t.Fatal("测试库不可达：短语归属无法判定（不 Skip，跳过等于没锁）")
	}
	ctxA := tenant.WithTenantID(context.Background(), cpTenantA)

	row := &model.ChampionPhrase{
		BaselineID: cpSharedBaseline, Phrase: "高意向短语", TFIDFScore: 0.5,
		TF: 3, DF: 2, Rank: 1,
	}
	if err := db.WithContext(ctxA).Create(row).Error; err != nil {
		t.Fatalf("落短语失败: %v", err)
	}
	if row.TenantID != cpTenantA {
		t.Errorf("Create 后 TenantID got=%d want=%d：隔离 callback 没填发起租户，"+
			"这一行会落进默认租户名下", row.TenantID, cpTenantA)
	}

	// 裸 ctx 必须被拒：on 模式下这等于允许把短语记到任意租户。
	bad := &model.ChampionPhrase{
		BaselineID: cpSharedBaseline, Phrase: "越界短语", TFIDFScore: 0.5,
		TF: 1, DF: 1, Rank: 2,
	}
	if err := db.WithContext(context.Background()).Create(bad).Error; err == nil {
		t.Error("裸 ctx 的 Create 未被拒绝：on 模式下这等于允许把短语记到任意租户")
	}
}

// TestChampionPhraseListIsTenantScoped 同 baseline_id 下两个租户各有一行：各自只读得到自己那条。
func TestChampionPhraseListIsTenantScoped(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.ResetRegistryForTest(); tenant.SetMode(tenant.ModeOff) })

	db := testutil.NewTestDB(t, &model.ChampionPhrase{})
	if db == nil {
		t.Fatal("测试库不可达：跨租户可读性无法判定")
	}
	ctxA := tenant.WithTenantID(context.Background(), cpTenantA)
	ctxB := tenant.WithTenantID(context.Background(), cpTenantB)

	mk := func(ctx context.Context, phrase string, rank int) {
		t.Helper()
		row := &model.ChampionPhrase{
			BaselineID: cpSharedBaseline, Phrase: phrase, TFIDFScore: 0.5,
			TF: 1, DF: 1, Rank: rank,
		}
		if err := db.WithContext(ctx).Create(row).Error; err != nil {
			t.Fatalf("落短语失败: %v", err)
		}
	}
	mk(ctxA, "A 的短语", 1)
	mk(ctxB, "B 的短语", 1)

	for _, tc := range []struct {
		name       string
		ctx        context.Context
		wantPhrase string
	}{
		{"tenantA", ctxA, "A 的短语"},
		{"tenantB", ctxB, "B 的短语"},
	} {
		var list []model.ChampionPhrase
		if err := db.WithContext(tc.ctx).
			Where("baseline_id = ?", cpSharedBaseline).
			Order("rank ASC").Find(&list).Error; err != nil {
			t.Fatalf("%s 读取失败: %v", tc.name, err)
		}
		if len(list) != 1 {
			t.Errorf("%s 读到 %d 条 want=1（多出来的那条是别的租户的短语）", tc.name, len(list))
			continue
		}
		if list[0].Phrase != tc.wantPhrase {
			t.Errorf("%s 读到的短语 = %q want %q", tc.name, list[0].Phrase, tc.wantPhrase)
		}
	}

	// 裸 ctx 同样 fail-closed：读侧一旦放行，「按基线号读别人的短语」就写回了库里。
	var list []model.ChampionPhrase
	if err := db.WithContext(context.Background()).
		Where("baseline_id = ?", cpSharedBaseline).Find(&list).Error; err == nil {
		t.Error("裸 ctx 的读取未被拒绝")
	}
}
