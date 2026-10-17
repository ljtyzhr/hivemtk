package router

// RBAC 菜单授权的端到端行为证据：菜单树与接口能力面必须始终是**同一份授予**的两次消费。
//
// 要钉的三件事：
//  1. 三个后端角色看到的 `GET /api/menus` 树各不相同，且都等于注册表折算出的口径
//     （admin 全量、customer_service 含客服专属节点、staff 更窄）——"除超管外每个人
//     只能看到自己所拥有全线菜单"在后端的落点。
//  2. 超管改分配（PUT /api/system/roles/:code/menus）后，被改角色的树**立刻就变**：
//     撤空即空树、授权即出现、越界授权被拒且不落地。这条腿盯的是
//     middleware.InvalidateMenuGrants：缓存不撤的话，改完分配最长要等一个 TTL 才生效，
//     而"菜单里已经没有、接口还能调"正是越权窗口。
//  3. 同一个授予同时驱动接口档位：能力面规则里点名菜单 key 的接口，授权前 403
//     「权限不足，该功能需先被授予对应菜单」、授权后放行。菜单与接口两张皮就是
//     这套机制要消灭的形态。
//
// 与 route_policy_gate_test.go 的分工：那份门禁钉"能力表 251 条没有静默失效"（静态面），
// 这里钉的是"树、分配接口、能力裁决三处对同一份授予的读数一致"（行为面）。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils"

	"github.com/gin-gonic/gin"
)

// menuResp 响应外壳。Code 用 any 而不是 int：response.Error 既可能写数字状态码，
// 也可能写字符串业务错误码（ErrorCode 形态），写死 int 会让整个解析失败、
// 把失败腿误判成"解析不到就是 0"。
type menuResp struct {
	Code    any            `json:"code"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data"`
}

// menuReq 打一次菜单域请求，返回 HTTP 状态码与解析后的响应体。
// menuCodeZero 判断响应体 code 字段为 0。
// 必须走字符串化：JSON 数字反序列化进 any 是 float64，直接和 int 0 比永远不等，
// 会把每条成功腿误判成失败。
func menuCodeZero(code any) bool {
	return fmt.Sprint(code) == "0"
}

// menuReq 打一次菜单域请求，返回 HTTP 状态码与解析后的响应体。
func menuReq(t *testing.T, engine *gin.Engine, method, path, token, body string) (int, menuResp) {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, r)
	var resp menuResp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("%s %s 响应不是合法 JSON：%v；body=%s", method, path, err, w.Body.String())
	}
	return w.Code, resp
}

// menuTreeKeySet 摊平菜单树，取全部 key。
func menuTreeKeySet(data map[string]any) map[string]struct{} {
	set := map[string]struct{}{}
	var walk func(nodes any)
	walk = func(nodes any) {
		arr, ok := nodes.([]any)
		if !ok {
			return
		}
		for _, n := range arr {
			m, ok := n.(map[string]any)
			if !ok {
				continue
			}
			if k, ok := m["key"].(string); ok && k != "" {
				set[k] = struct{}{}
			}
			walk(m["children"])
		}
	}
	walk(data["tree"])
	return set
}

// menuKeysOf 把 key 切片转成集合，便于与摊平结果对账。
func menuKeysOf(keys []string) map[string]struct{} {
	set := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		set[k] = struct{}{}
	}
	return set
}

// menuKeysEqual 两个集合逐字相等（多一个少一个都算不等）。
func menuKeysEqual(a, b map[string]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			return false
		}
	}
	return true
}

// menuKeysOfStr 把 set 渲染成稳定顺序的字符串，失败时打印用。
func menuKeysOfStr(set map[string]struct{}) string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	// 只按字典序排，够定位 diff 即可，不追求性能。
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return strings.Join(keys, ",")
}

// menuNodeKeys 摊平类型化菜单树（model.SystemMenuNode）取 key。
func menuNodeKeys(nodes []model.SystemMenuNode) map[string]struct{} {
	set := map[string]struct{}{}
	var walk func(ns []model.SystemMenuNode)
	walk = func(ns []model.SystemMenuNode) {
		for _, n := range ns {
			if n.Key != "" {
				set[n.Key] = struct{}{}
			}
			walk(n.Children)
		}
	}
	walk(nodes)
	return set
}

// expectedTreeKeys 某角色在"出厂分配"下应当看到的菜单树 key 集合。
//
// 口径与 service.MenuService.GetUserMenuTree 完全同源：有效授予 =
// DefaultMenuKeys（首空表 seed 落库的那份）∩ EligibleMenuKeys（授权上界），
// 再过 model.FilterMenuTree。这里刻意复用 model 的函数而不是另写一套扁平化规则，
// 因为**祖先目录会被带回来**这件事正是实现细节：只授 agentStatus 时树上还有
// agentWorkbench 与 aiAgent 两级目录。断言要钉的是"端点拿到的树 == 这把尺子量出来的树"，
// 不是"树上只有被授予的叶子"——后者是对 FilterMenuTree 语义的误读。

func expectedTreeKeys(role string) map[string]struct{} {
	eligible := model.EligibleMenuKeys(role)
	eff := map[string]struct{}{}
	for _, k := range model.DefaultMenuKeys(role) {
		if _, ok := eligible[k]; ok {
			eff[k] = struct{}{}
		}
	}
	return menuNodeKeys(model.FilterMenuTree(model.SystemMenuTree, eff))
}

// TestRBACMenuGrants_TreeAndCapabilityFollowRoleGrants 见文件头注释。
func TestRBACMenuGrants_TreeAndCapabilityFollowRoleGrants(t *testing.T) {
	// 隔离模块闸：本题考的是 **RBAC 授权**这一个维度，而菜单树现在还要过
	// 模块闸（未开通模块的菜单对所有人隐藏，见 service/menu.go 的双闸说明）。
	// 不清掉它，本用例的断言就会被「模块数据当前是什么」污染 ——
	// 症状是同一份 RBAC 代码在 CI 上绿、在本地红，而两者只差一个测试执行顺序。
	//
	// 注意这里**不是**给模块闸开后门：admin 在生产里同样只看到已开通的模块，
	// 那是商业模式的落地点（见 TestAdminAlsoSeesModuleGate）。本题只关心
	// 「在模块闸恒开的前提下，RBAC 的授予是否生效」。
	// seatEngine 调的正是生产装配入口 Setup()，而 Setup 内部会装配模块闸
	// （那是它该做的事）。所以「清掉闸」必须发生在 Setup **之后**：
	// 放在前面会被 Setup 重新装上，等于什么都没做。
	//
	// seatEngine 建的正是这套表（含 RoleMenuPermission）与真实装配，菜单域复用它
	// 比另起一份引擎更接近生产链路。
	engine, _ := seatEngine(t)

	model.SetModuleEnabledFilter(nil)
	t.Cleanup(func() { model.SetModuleEnabledFilter(nil) })
	agentA, _, admin := seatTokens(t)

	jt := utils.NewJWTUtils(utils.DefaultJWTConfig)
	staffTok, err := jt.GenerateTokenWithScope(7103, "seat-c", model.SystemRoleCodeStaff, "self", 0, 0)
	if err != nil {
		t.Fatalf("签 staff 令牌失败: %v", err)
	}

	csRole := model.SystemRoleCodeCustomerService
	assignPath := "/api/system/roles/" + csRole + "/menus"

	// 腿1：超管全量树。
	code, resp := menuReq(t, engine, "GET", "/api/menus", admin, "")
	if code != http.StatusOK || !menuCodeZero(resp.Code) {
		t.Fatalf("admin 读菜单失败：http=%d code=%v msg=%s", code, resp.Code, resp.Message)
	}
	if all, _ := resp.Data["all"].(bool); !all {
		t.Error("admin 的 /api/menus 必须 all=true")
	}
	adminKeys := menuTreeKeySet(resp.Data)
	if want := menuKeysOf(model.DefaultMenuKeys(model.SystemRoleCodeAdmin)); !menuKeysEqual(adminKeys, want) {
		t.Errorf("admin 菜单树与注册表全量不一致：树=%s", menuKeysOfStr(adminKeys))
	}

	// 腿2：客服角色看到出厂分配，含客服专属节点、不含超管专属节点。
	code, resp = menuReq(t, engine, "GET", "/api/menus", agentA, "")
	if code != http.StatusOK || !menuCodeZero(resp.Code) {
		t.Fatalf("customer_service 读菜单失败：http=%d code=%v msg=%s", code, resp.Code, resp.Message)
	}
	if all, _ := resp.Data["all"].(bool); all {
		t.Error("非超管角色的 /api/menus 不该 all=true")
	}
	csKeys := menuTreeKeySet(resp.Data)
	if want := expectedTreeKeys(csRole); !menuKeysEqual(csKeys, want) {
		t.Errorf("customer_service 菜单树与注册表折算口径不一致：树=%s", menuKeysOfStr(csKeys))
	}
	if _, ok := csKeys["agentWorkbench"]; !ok {
		t.Error("customer_service 应看得到客服专属节点 agentWorkbench")
	}
	if _, ok := csKeys["messageHub"]; ok {
		t.Error("customer_service 不该看得到超管专属节点 messageHub")
	}

	// 腿3：staff 更窄——同一个 agentWorkbench 对它不合格，两棵树的 key 集合必须不同。
	code, resp = menuReq(t, engine, "GET", "/api/menus", staffTok, "")
	if code != http.StatusOK || !menuCodeZero(resp.Code) {
		t.Fatalf("staff 读菜单失败：http=%d code=%v msg=%s", code, resp.Code, resp.Message)
	}
	staffKeys := menuTreeKeySet(resp.Data)
	if want := expectedTreeKeys(model.SystemRoleCodeStaff); !menuKeysEqual(staffKeys, want) {
		t.Errorf("staff 菜单树与注册表折算口径不一致：树=%s", menuKeysOfStr(staffKeys))
	}
	if len(staffKeys) == 0 {
		t.Error("staff 的菜单树不该为空（出厂分配至少有员工视角节点）")
	}
	if _, ok := staffKeys["agentWorkbench"]; ok {
		t.Error("staff 不该看得到客服专属节点 agentWorkbench（授权上界按 Roles 词表折算）")
	}
	if menuKeysEqual(csKeys, staffKeys) {
		t.Error("customer_service 与 staff 的菜单树完全相同 ⇒ 角色区分失效")
	}

	// 腿4：超管撤空该角色分配后，被改角色的树必须立刻变空。
	// 注意这条腿只钉"树跟着库走"（MenuService 直读库、不过中间件缓存）；
	// 缓存失效本身由腿 6 的能力面那一下钉住——见那里。
	code, resp = menuReq(t, engine, "PUT", assignPath, admin, `{"menu_keys":[]}`)
	if code != http.StatusOK || !menuCodeZero(resp.Code) {
		t.Fatalf("撤空 customer_service 菜单分配失败：http=%d code=%v msg=%s", code, resp.Code, resp.Message)
	}
	code, resp = menuReq(t, engine, "GET", "/api/menus", agentA, "")
	if code != http.StatusOK || !menuCodeZero(resp.Code) {
		t.Fatalf("撤空后 customer_service 读菜单失败：http=%d code=%v msg=%s", code, resp.Code, resp.Message)
	}
	if got := menuTreeKeySet(resp.Data); len(got) != 0 {
		t.Errorf("撤空后 customer_service 仍看得到 %d 个菜单节点 ⇒ 分配改动没有即时生效：%s", len(got), menuKeysOfStr(got))
	}

	// 腿5：越界授予（EligibleMenuKeys 之外的超管专属节点）必须被拒，且不落地。
	code, resp = menuReq(t, engine, "PUT", assignPath, admin, `{"menu_keys":["messageHub"]}`)
	if code == http.StatusOK || menuCodeZero(resp.Code) {
		t.Fatalf("把超管专属节点 messageHub 授给 customer_service 竟然成功了：http=%d code=%v", code, resp.Code)
	}
	if !strings.Contains(resp.Message, "超管专属") {
		t.Errorf("越界授予的拒绝文案没点明原因：%s", resp.Message)
	}
	code, resp = menuReq(t, engine, "GET", "/api/menus", agentA, "")
	if got := menuTreeKeySet(resp.Data); len(got) != 0 {
		t.Errorf("越界授予被拒后菜单树仍被改写：%s", menuKeysOfStr(got))
	}

	// 腿6：授予后树与能力面同时跟上。
	code, resp = menuReq(t, engine, "PUT", assignPath, admin, `{"menu_keys":["agentStatus"]}`)
	if code != http.StatusOK || !menuCodeZero(resp.Code) {
		t.Fatalf("授予 agentStatus 失败：http=%d code=%v msg=%s", code, resp.Code, resp.Message)
	}
	code, resp = menuReq(t, engine, "GET", "/api/menus", agentA, "")
	if code != http.StatusOK || !menuCodeZero(resp.Code) {
		t.Fatalf("授予后 customer_service 读菜单失败：http=%d code=%v msg=%s", code, resp.Code, resp.Message)
	}
	wantOne := menuNodeKeys(model.FilterMenuTree(model.SystemMenuTree, map[string]struct{}{"agentStatus": {}}))
	if got := menuTreeKeySet(resp.Data); !menuKeysEqual(got, wantOne) {
		t.Errorf("授予 agentStatus 后菜单树与该授予应有的树不一致：got=%s want=%s", menuKeysOfStr(got), menuKeysOfStr(wantOne))
	}
	// 能力面：/api/quick-replies 的能力规则点名 agentStatus，此刻必须放行。
	capCode, capBody := seatCall(t, engine, "GET", "/api/quick-replies", agentA, "")
	if capCode == http.StatusForbidden && strings.Contains(capBody, capabilityDeniedMsg) {
		t.Errorf("已授予 agentStatus，能力面仍拦 /api/quick-replies：%s", capBody)
	}
	// 再撤空：同一个请求必须翻成能力面 403。两次请求只差一份授予，
	// 所以这条翻面正是"授予驱动接口档位"的证据。
	code, _ = menuReq(t, engine, "PUT", assignPath, admin, `{"menu_keys":[]}`)
	if code != http.StatusOK {
		t.Fatalf("再次撤空失败：http=%d", code)
	}
	capCode, capBody = seatCall(t, engine, "GET", "/api/quick-replies", agentA, "")
	if capCode != http.StatusForbidden || !strings.Contains(capBody, capabilityDeniedMsg) {
		t.Errorf("撤空后 /api/quick-replies 应被能力面拦下（403 + %s），实际 http=%d body=%s", capabilityDeniedMsg, capCode, capBody)
	}
}
