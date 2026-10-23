package service

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
)

// 种子里的文案改了，存量库读到的却还是旧文案——因为 SeedConfigParams 原本只补插缺失行。
// 本用例钉的就是"定义列会随种子刷新，而运营改过的值一个字都不动"这一对。

func findDef(t *testing.T, group, key string) ParamDef {
	t.Helper()
	for _, d := range DefaultParamDefs() {
		if d.Group == group && d.Key == key {
			return d
		}
	}
	t.Fatalf("DefaultParamDefs 里没有 %s.%s（本用例的夹具要先满足判据前置）", group, key)
	return ParamDef{}
}

func TestSeedConfigParamsRefreshesStaleDefinition(t *testing.T) {
	db := testutil.NewTestDB(t, &model.ConfigParam{}, &model.ConfigParamAuditLog{})
	if db == nil {
		t.Fatal("测试库不可达：种子刷新无法判定（不 Skip，跳过等于没锁）")
	}
	ctx := context.Background()
	def := findDef(t, "bridge", "polling_max_timeout")

	// 预置"建库那一次"留下的旧行：旧文案 + 运营改过的值 + 一个不属于任何分组的类目。
	stale := &model.ConfigParam{
		Group:        def.Group,
		Key:          def.Key,
		Name:         "轮询最大超时",
		Description:  "客户端 HTTP 长轮询允许的最长等待时间（秒），超时由服务端主动返回",
		ValueType:    def.ValueType,
		Value:        "120",
		DefaultValue: "999",
		Min:          strPtr("10"),
		Max:          strPtr("3600"),
		Step:         strPtr("5"),
		Category:     "不存在的类目",
	}
	if err := db.Create(stale).Error; err != nil {
		t.Fatalf("预置存量行失败: %v", err)
	}

	if err := SeedConfigParams(ctx, db); err != nil {
		t.Fatalf("SeedConfigParams failed: %v", err)
	}

	var got model.ConfigParam
	if err := db.Where("param_group = ? AND key = ?", def.Group, def.Key).First(&got).Error; err != nil {
		t.Fatalf("读回参数行失败: %v", err)
	}
	if got.Name != def.Name {
		t.Errorf("name 没随种子刷新: got=%q want=%q", got.Name, def.Name)
	}
	if got.Description != def.Description {
		t.Errorf("description 没随种子刷新: got=%q want=%q", got.Description, def.Description)
	}
	if got.Category != def.Category {
		t.Errorf("category 没随种子刷新: got=%q want=%q", got.Category, def.Category)
	}
	if got.DefaultValue != def.DefaultValue {
		t.Errorf("default_value 没随种子刷新: got=%q want=%q", got.DefaultValue, def.DefaultValue)
	}
	if got.Value != "120" {
		t.Errorf("运营改过的值被种子覆盖了: got=%q want=120 —— 定义刷新唯一不许碰的就是 value", got.Value)
	}

	var rows int64
	if err := db.Model(&model.ConfigParam{}).
		Where("param_group = ? AND key = ?", def.Group, def.Key).Count(&rows).Error; err != nil {
		t.Fatalf("计数失败: %v", err)
	}
	if rows != 1 {
		t.Errorf("同一 (group,key) 出现 %d 行 —— 定义刷新不该变成再插一行", rows)
	}

	// 二次 seed 必须是无操作：否则每次重启都在写库，"改了值"与"没改"就再也区分不出来了。
	before := got
	if err := SeedConfigParams(ctx, db); err != nil {
		t.Fatalf("二次 SeedConfigParams failed: %v", err)
	}
	var again model.ConfigParam
	if err := db.Where("param_group = ? AND key = ?", def.Group, def.Key).First(&again).Error; err != nil {
		t.Fatalf("二次读回失败: %v", err)
	}
	if again.Value != before.Value || again.Description != before.Description || again.ID != before.ID {
		t.Errorf("二次 seed 仍在改写已对齐的行: id %v->%v, value %q->%q",
			before.ID, again.ID, before.Value, again.Value)
	}
}

// 跨 group 同名 key 是合法形状（v3.47.0 那条索引迁移要买到的行为）：
// seed 必须按 (group,key) 定位，不能只按 key 找到别的组的行就当作"已存在"。
func TestSeedConfigParamsDoesNotMatchOtherGroupSameKey(t *testing.T) {
	db := testutil.NewTestDB(t, &model.ConfigParam{}, &model.ConfigParamAuditLog{})
	if db == nil {
		t.Fatal("测试库不可达：跨 group 定位无法判定（不 Skip，跳过等于没锁）")
	}
	ctx := context.Background()
	def := findDef(t, "bridge", "polling_max_timeout")
	if err := db.Create(&model.ConfigParam{
		Group: "another_group", Key: def.Key, Name: "同名不同组的占位",
		ValueType: "duration", Value: "1", DefaultValue: "1",
	}).Error; err != nil {
		t.Fatalf("预置同 key 别的组行失败: %v", err)
	}

	if err := SeedConfigParams(ctx, db); err != nil {
		t.Fatalf("SeedConfigParams failed: %v", err)
	}

	var mine, other int64
	db.Model(&model.ConfigParam{}).Where("param_group = ? AND key = ?", def.Group, def.Key).Count(&mine)
	db.Model(&model.ConfigParam{}).Where("param_group = ? AND key = ?", "another_group", def.Key).Count(&other)
	if mine != 1 {
		t.Errorf("bridge.%s 没被 seed 出来（got=%d）：占位行在别的组、同 key，不该让本参数消失", def.Key, mine)
	}
	if other != 1 {
		t.Errorf("别的组的同 key 行被动了（got=%d want=1）", other)
	}
}

// TestSeedParamGroups_PrunesRetiredGroupWithoutLiveParams 钉住"从种子里摘掉的组"在存量库上
// 真的消失，以及那条边界：组里**还有活参数就绝不删**。
//
// 现场样本（2026-10-23 审计 #65）：D-7 删掉的 "wechat" 组在库里留着，
// 带 3 条全部 deprecated 的参数；侧栏一直渲染「微信 0」，点进去是空表 ——
// 前端有意保留 0 条数的组（好让"整组已下架"仍可点进去），所以这口锅在写入侧：
// 分组表只有 upsert、没有删除路径，"摘掉"等于没摘。
func TestSeedParamGroups_PrunesRetiredGroupWithoutLiveParams(t *testing.T) {
	db := testutil.NewTestDB(t, &model.ConfigParam{}, &model.ConfigParamGroup{})
	if db == nil {
		t.Fatal("测试库不可达：分组清理无法判定（不 Skip，跳过等于没锁）")
	}
	ctx := context.Background()

	// 表由本用例独占：先清干净，免得别的用例留下的行干扰"删了几个"的读数
	if err := db.WithContext(ctx).Exec(`DELETE FROM config_param_groups`).Error; err != nil {
		t.Fatalf("清空分组表失败: %v", err)
	}
	if err := db.WithContext(ctx).Where("1 = 1").Delete(&model.ConfigParam{}).Error; err != nil {
		t.Fatalf("清空参数表失败: %v", err)
	}

	// ① 已下架组：种子里没有 "zz_retired"，组内只剩一条 deprecated 参数 ⇒ 应被清掉
	// ② 还有活参数的组：种子里同样没有 "zz_live_group"，但有 deprecated=false 的参数 ⇒ 必须留
	retired := []model.ConfigParamGroup{
		{Group: "zz_retired", Label: "已下架组"},
		{Group: "zz_live_group", Label: "还有活参数的组"},
	}
	if err := db.WithContext(ctx).Create(&retired).Error; err != nil {
		t.Fatalf("预置分组失败: %v", err)
	}
	params := []model.ConfigParam{
		{Group: "zz_retired", Key: "dead_knob", Name: "死旋钮", ValueType: "int",
			Value: "1", DefaultValue: "1", Deprecated: true},
		{Group: "zz_live_group", Key: "live_knob", Name: "活旋钮", ValueType: "int",
			Value: "1", DefaultValue: "1", Deprecated: false},
	}
	if err := db.WithContext(ctx).Create(&params).Error; err != nil {
		t.Fatalf("预置参数失败: %v", err)
	}

	if err := SeedParamGroups(ctx, db); err != nil {
		t.Fatalf("SeedParamGroups 失败: %v", err)
	}

	var retiredLeft, liveLeft int64
	if err := db.WithContext(ctx).Model(&model.ConfigParamGroup{}).
		Where("param_group = ?", "zz_retired").Count(&retiredLeft).Error; err != nil {
		t.Fatalf("读回 zz_retired 失败: %v", err)
	}
	if err := db.WithContext(ctx).Model(&model.ConfigParamGroup{}).
		Where("param_group = ?", "zz_live_group").Count(&liveLeft).Error; err != nil {
		t.Fatalf("读回 zz_live_group 失败: %v", err)
	}
	if retiredLeft != 0 {
		t.Error("组内只剩已下架参数的分组没被清掉：侧栏会一直挂着一个点进去是空表的分组")
	}
	if liveLeft != 1 {
		t.Error("组内还有在用参数，分组却被清掉了：真正的旋钮连入口一起消失，比留个空组更糟")
	}

	// 种子里的每个组都必须还在（清理不能把种子声明的组也带走）
	var keptSeedGroups int64
	keep := make([]string, 0, len(DefaultParamGroupDefs()))
	for _, d := range DefaultParamGroupDefs() {
		keep = append(keep, d.Group)
	}
	if err := db.WithContext(ctx).Model(&model.ConfigParamGroup{}).
		Where("param_group IN ?", keep).Count(&keptSeedGroups).Error; err != nil {
		t.Fatalf("读回种子分组失败: %v", err)
	}
	if int(keptSeedGroups) != len(keep) {
		t.Errorf("种子声明的 %d 个分组只有 %d 个在库里：清理误伤了在用分组", len(keep), keptSeedGroups)
	}
}
