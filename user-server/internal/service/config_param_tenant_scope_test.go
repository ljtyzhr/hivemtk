package service

import (
	"context"
	"strings"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
)

// 参数中心三档租户可见性的自洽性。
//
// 这组用例守的是两张表本身：它们是本页「租户能不能改」的唯一回答，一旦与种子、
// 与分组定义漂移，管理端就会对不存在的旋钮给出口径，或者反过来把一个真能用的
// 旋钮说成不能用。

func TestTenantScopeRegistryKeysAreWellFormedAndDisjoint(t *testing.T) {
	for full, reason := range platformScopedParams {
		if strings.TrimSpace(reason) == "" {
			t.Errorf("platformScopedParams[%s] 没写原因", full)
		}
		if strings.Count(full, ".") != 1 {
			t.Errorf("platformScopedParams 键 %s 不是 group.key 形状", full)
		}
		if _, dup := tenantPendingParams[full]; dup {
			t.Errorf("%s 同时登记在平台级与待接线两张表里", full)
		}
	}
	for full, reason := range tenantPendingParams {
		if strings.TrimSpace(reason) == "" {
			t.Errorf("tenantPendingParams[%s] 没写原因", full)
		}
		if strings.Count(full, ".") != 1 {
			t.Errorf("tenantPendingParams 键 %s 不是 group.key 形状", full)
		}
	}
}

// 登记的每一个键都必须在种子里存在：表里写一个种子里没有的键，
// 说明它指的是已下架的旧坐标，拦错对象还自以为是安全的。
func TestTenantScopeRegistryKeysExistInSeedDefs(t *testing.T) {
	seeded := make(map[string]bool)
	for _, d := range DefaultParamDefs() {
		seeded[fullParamKey(d.Group, d.Key)] = true
	}
	for _, tbl := range []map[string]string{platformScopedParams, tenantPendingParams} {
		for full := range tbl {
			if !seeded[full] {
				t.Errorf("登记表里的 %s 在种子定义里查无此键", full)
			}
		}
	}
}

// 分组也要对得上：登记的键所属组必须已声明，否则管理端侧栏里没有它落脚的位置。
func TestTenantScopeRegistryGroupsAreDeclared(t *testing.T) {
	groups := make(map[string]bool)
	for _, g := range DefaultParamGroupDefs() {
		groups[g.Group] = true
	}
	for _, tbl := range []map[string]string{platformScopedParams, tenantPendingParams} {
		for full := range tbl {
			g := full[:strings.Index(full, ".")]
			if !groups[g] {
				t.Errorf("%s 的分组 %s 没有分组定义", full, g)
			}
		}
	}
}

func TestParamTenantScopeThreeStates(t *testing.T) {
	cases := []struct {
		group, key, wantScope string
	}{
		{"cache", "max_keys", tenantScopePlatform},
		{"knowledge", "similarity_threshold", tenantScopePending},
		{"urls", "public_base_url", tenantScopeInherit},
		{"limits", "made_up_key", tenantScopeInherit},
	}
	for _, c := range cases {
		got, _ := paramTenantScope(c.group, c.key)
		if c.wantScope == tenantScopeInherit {
			if got != "" {
				t.Errorf("%s.%s 应落默认档（空串），实际 %q", c.group, c.key, got)
			}
			continue
		}
		if got != c.wantScope {
			t.Errorf("%s.%s 期望 %s，实际 %q", c.group, c.key, c.wantScope, got)
		}
	}
}

// 三档各自给的必须是不同的话：把「还没接线」说成「平台级」是把债说成设计。
func TestTenantScopedWriteErrDistinguishesDebtFromDesign(t *testing.T) {
	if err := tenantScopedWriteErr("urls", "public_base_url"); err != nil {
		t.Errorf("默认档不该拦租户写，实际拦了: %v", err)
	}
	pl := tenantScopedWriteErr("cache", "max_keys")
	pd := tenantScopedWriteErr("knowledge", "similarity_threshold")
	if pl == nil || pd == nil {
		t.Fatalf("平台级与待接线都必须拦下: platform=%v pending=%v", pl, pd)
	}
	if !strings.Contains(pl.Error(), "平台级参数") {
		t.Errorf("平台级的话术不对: %s", pl)
	}
	if !strings.Contains(pd.Error(), "还没接线") {
		t.Errorf("待接线的话术不对: %s", pd)
	}
	if pl.Error() == pd.Error() {
		t.Error("两档给了同一句话，用户读不出该等接线还是该找平台")
	}
}

// 写路径落地：拦下必须发生在落库之前，且不能顺手没收「删掉历史覆盖行」这条退路。
func TestTenantWriteRejectsPlatformAndPendingBeforePersisting(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })
	svc, db := setupTenantParamFixture(t)
	seedBaseParam(t, db, "cache", "max_keys", "20000")
	seedBaseParam(t, db, "agent_llm", "max_tokens", "4000")
	seedBaseParam(t, db, "limits", "max_items", "10")

	ctxA := tenantCtx(41)
	if err := svc.UpdateValue(ctxA, "cache", "max_keys", "30000", 7); err == nil {
		t.Error("平台级参数的租户写入必须被拒")
	}
	if err := svc.UpdateValue(ctxA, "agent_llm", "max_tokens", "8000", 7); err == nil {
		t.Error("待接线参数的租户写入必须被拒")
	}
	var n int64
	if err := db.WithContext(ctxA).Model(&model.TenantConfigParam{}).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("被拒的写入不得留下覆盖行，实际 %d 行", n)
	}

	// 默认档照旧可写，否则整条租户覆盖链被一起钉死。
	if err := svc.UpdateValue(ctxA, "limits", "max_items", "20", 7); err != nil {
		t.Fatalf("默认档写入被误拦: %v", err)
	}

	// 重置是清理历史覆盖行的唯一入口，不能因为这一档被拒就一并没收。
	if err := svc.ResetToDefault(ctxA, "cache", "max_keys", 7); err != nil {
		t.Errorf("平台级参数的重置应放行（它是退路不是写入）: %v", err)
	}
}

// 列表读出的每一行都带档位，包括没有租户上下文的那次。
func TestListStampsTenantScopeForEveryRow(t *testing.T) {
	tenant.SetMode(tenant.ModeOff)
	svc, db := setupTenantParamFixture(t)
	seedBaseParam(t, db, "cache", "max_keys", "20000")
	seedBaseParam(t, db, "agent_llm", "max_tokens", "4000")
	seedBaseParam(t, db, "limits", "max_items", "10")

	want := map[string]string{
		"cache.max_keys":                 tenantScopePlatform,
		"agent_llm.max_tokens":            tenantScopePending,
		"limits.max_items":               tenantScopeInherit,
	}
	// off 模式下没有租户上下文，档位仍必须出现在列表里：这一列回答的是
	// 「这个键能不能按租户改」，与本次请求带不带租户无关。
	cases := []struct {
		name string
		ctx  context.Context
	}{
		{"无租户上下文", context.Background()},
		{"A 租户上下文", tenantCtx(41)},
	}
	for _, c := range cases {
		rows, err := svc.List(c.ctx, false)
		if err != nil {
			t.Fatalf("%s 列表失败: %v", c.name, err)
		}
		seen := 0
		for _, r := range rows {
			w, ok := want[fullParamKey(r.Group, r.Key)]
			if !ok {
				continue
			}
			seen++
			if r.TenantScope != w {
				t.Errorf("%s：%s.%s 档位应为 %s，实际 %q", c.name, r.Group, r.Key, w, r.TenantScope)
			}
			if w == tenantScopeInherit && r.TenantScopeReason != "" {
				t.Errorf("%s：默认档不该带原因，实际 %q", c.name, r.TenantScopeReason)
			}
			if w != tenantScopeInherit && r.TenantScopeReason == "" {
				t.Errorf("%s：%s 档位缺原因", c.name, fullParamKey(r.Group, r.Key))
			}
		}
		if seen != len(want) {
			t.Errorf("%s：只对 %d/%d 行做了档位对账，夹具或列表少给了行", c.name, seen, len(want))
		}
	}
}
