package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hivemtk-user/internal/model"

	"github.com/gin-gonic/gin"
)

// 这一层判的是"这一行数据是不是调用者的"，判据分支多、且每一条的失效形态都是静默的：
// 少一条 return 就变成"拦了但没生效"或"根本没参与判定"。所以逐档点名断，而不是只测
// 一条"别人的数据打不通"的正例——后者在"守卫恒拦"和"守卫恒放"两种坏法里各能骗过一半。

type dsCall struct {
	method   string
	path     string // 注册模板（守卫查登记表用的形状）
	row      model.DataScopeRow
	register bool
	lookup   OwnerLookupFunc
	role     string
	scope    string
	uid      uint
	username string
}

// dsRequestPath 把注册模板里的 :param 段换成一个具体值，得到一次真实请求的路径。
// 模板与请求路径必须分开：守卫查登记表用的是注册形状（c.FullPath），取定位值用的是
// 具体请求（c.Param），两者混成一个就测不出"参数名写错"这类缺陷。
func dsRequestPath(template string) string {
	segs := strings.Split(template, "/")
	for i, s := range segs {
		if strings.HasPrefix(s, ":") {
			segs[i] = "S-1"
		}
	}
	return strings.Join(segs, "/")
}

// dsRun 起一个只挂被测守卫的最小引擎：登记表读取口与归属查询源都按用例注入，
// 用"到没到达 handler + 响应体文案"判定守卫的动作，不依赖任何业务 handler。
func dsRun(t *testing.T, c dsCall) (int, string, bool) {
	t.Helper()

	origLookup, origOwner := dataScopeLookup, currentOwnerLookup()
	t.Cleanup(func() {
		dataScopeLookup = origLookup
		SetOwnerLookup(origOwner)
	})

	row := c.row
	dataScopeLookup = func(method, fullPath string) (model.DataScopeRow, bool) {
		if !c.register || method != c.method || fullPath != c.path {
			return model.DataScopeRow{}, false
		}
		return row, true
	}
	SetOwnerLookup(c.lookup)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	handled := false
	engine.Use(func(ctx *gin.Context) {
		if c.role != "" {
			ctx.Set("role", c.role)
		}
		if c.scope != "" {
			ctx.Set("data_scope", c.scope)
		}
		if c.uid != 0 {
			ctx.Set("user_id", c.uid)
		}
		if c.username != "" {
			ctx.Set("username", c.username)
		}
		ctx.Next()
	})
	engine.Use(DataScopeGuard())

	switch c.method {
	case http.MethodGet:
		engine.GET(c.path, func(ctx *gin.Context) { handled = true; ctx.Status(http.StatusTeapot) })
	case http.MethodPut:
		engine.PUT(c.path, func(ctx *gin.Context) { handled = true; ctx.Status(http.StatusTeapot) })
	case http.MethodPost:
		engine.POST(c.path, func(ctx *gin.Context) { handled = true; ctx.Status(http.StatusTeapot) })
	case http.MethodDelete:
		engine.DELETE(c.path, func(ctx *gin.Context) { handled = true; ctx.Status(http.StatusTeapot) })
	default:
		t.Fatalf("未支持的 method: %s", c.method)
	}

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(c.method, dsRequestPath(c.path), nil))
	return w.Code, w.Body.String(), handled
}

// dsSessionRow 客服会话那批口的真实登记形状（归属列存 user id、未分配放行）。
func dsSessionRow() model.DataScopeRow {
	return model.DataScopeRow{
		Table: "customer_sessions", IDColumn: "session_id", Param: "sess",
		OwnerColumn: "agent_id", AllowUnowned: true,
	}
}

func okLookup(owner string, found bool, err error) OwnerLookupFunc {
	return func(ctx context.Context, table, idColumn, ownerColumn, idValue string) (string, bool, error) {
		return owner, found, err
	}
}

func TestDataScopeGuard_UnregisteredRouteIsNotThisLayersJob(t *testing.T) {
	// 未登记 ⇒ 必须原样放行给后续守卫与 handler。若这里也拦，登记表就等于全站白名单，
	// 任何一次登记面收窄都会把无关接口一起打死。
	code, body, handled := dsRun(t, dsCall{
		method: http.MethodGet, path: "/api/whatever/:sess",
		register: false, role: "staff", uid: 7,
		lookup: okLookup("999", true, nil),
	})
	if !handled || code != http.StatusTeapot {
		t.Errorf("未登记的路由应直达 handler，实得 code=%d handled=%v body=%s", code, handled, strings.TrimSpace(body))
	}
}

func TestDataScopeGuard_NilLookupFailsClosed(t *testing.T) {
	// 漏装配查询源（SetOwnerLookup）必须是**拦**：漏装配的后果不能是整层静默失效。
	code, body, handled := dsRun(t, dsCall{
		method: http.MethodGet, path: "/api/customer-sessions/:sess/transcript",
		register: true, row: dsSessionRow(), role: "staff", uid: 7,
		lookup: nil,
	})
	if handled {
		t.Error("没有归属查询源却放行了（fail-open）")
	}
	if code != http.StatusForbidden || !strings.Contains(body, DataScopeDeniedMessage) {
		t.Errorf("缺查询源应 403+行级隔离文案，实得 %d / %s", code, strings.TrimSpace(body))
	}
}

func TestDataScopeGuard_OnlyAdminSeesEverything(t *testing.T) {
	// 放开全量的条件**只有角色**：data_scope=all 不再宽化。理由是同域的既有两处行级 scope
	// （service.SessionScope / service.InboxScope）都只按角色放开，守卫多认一档就会变成
	// "列表看不到、按 id 读得到"的两套口径，窄的那套被宽的旁路。
	bypass := []struct{ name, role, scope string }{
		{"超管", "admin", "all"},
		{"超管即使 data_scope=self 也全量", "admin", "self"},
	}
	for _, c := range bypass {
		code, body, handled := dsRun(t, dsCall{
			method: http.MethodGet, path: "/api/customer-sessions/:sess/transcript",
			register: true, row: dsSessionRow(), role: c.role, scope: c.scope, uid: 7,
			lookup: okLookup("999", true, nil), // 归属是别人的
		})
		if !handled || code != http.StatusTeapot {
			t.Errorf("%s 应全量可见并直达 handler，实得 code=%d handled=%v body=%s",
				c.name, code, handled, strings.TrimSpace(body))
		}
	}

	// 反例：非超管带 data_scope=all，别人的数据必须仍是 403。
	code, body, handled := dsRun(t, dsCall{
		method: http.MethodGet, path: "/api/customer-sessions/:sess/transcript",
		register: true, row: dsSessionRow(), role: "customer_service", scope: "all", uid: 7,
		lookup: okLookup("8", true, nil),
	})
	if handled {
		t.Error("非超管带 data_scope=all 却读到了别人的会话（宽化没被摘掉）")
	}
	if code != http.StatusForbidden || !strings.Contains(body, DataScopeDeniedMessage) {
		t.Errorf("非超管 data_scope=all 别人的数据应 403+行级隔离文案，实得 %d / %s",
			code, strings.TrimSpace(body))
	}

	// 同一档的正例：all 也不该把"自己的数据"一起拦掉（否则上一条反例在"恒拦"里也能骗过）。
	code, body, handled = dsRun(t, dsCall{
		method: http.MethodGet, path: "/api/customer-sessions/:sess/transcript",
		register: true, row: dsSessionRow(), role: "customer_service", scope: "all", uid: 7,
		lookup: okLookup("7", true, nil),
	})
	if !handled || code != http.StatusTeapot {
		t.Errorf("非超管 data_scope=all 时自己的数据仍应放行，实得 code=%d handled=%v body=%s",
			code, handled, strings.TrimSpace(body))
	}

	// 身份读不出（role 为空）时按受限处理：缺登录态不能等于全量可见。
	code, _, handled = dsRun(t, dsCall{
		method: http.MethodGet, path: "/api/customer-sessions/:sess/transcript",
		register: true, row: dsSessionRow(), role: "", scope: "all", uid: 7,
		lookup: okLookup("999", true, nil),
	})
	if handled || code != http.StatusForbidden {
		t.Errorf("读不出角色却放行别人的数据：code=%d handled=%v", code, handled)
	}
}

func TestDataScopeGuard_OwnerMatchReachesHandlerMismatchDenies(t *testing.T) {
	// 一正一反同一口：只有正例会被"恒放"骗过，只有反例会被"恒拦"骗过。
	code, body, handled := dsRun(t, dsCall{
		method: http.MethodGet, path: "/api/customer-sessions/:sess/transcript",
		register: true, row: dsSessionRow(), role: "customer_service", scope: "self", uid: 7,
		lookup: okLookup("7", true, nil),
	})
	if !handled || code != http.StatusTeapot {
		t.Errorf("本人名下会话应放行，实得 code=%d handled=%v body=%s", code, handled, strings.TrimSpace(body))
	}

	code, body, handled = dsRun(t, dsCall{
		method: http.MethodGet, path: "/api/customer-sessions/:sess/transcript",
		register: true, row: dsSessionRow(), role: "customer_service", scope: "self", uid: 7,
		lookup: okLookup("8", true, nil),
	})
	if handled {
		t.Error("别人名下的会话却到达了 handler（越权面没关上）")
	}
	if code != http.StatusForbidden || !strings.Contains(body, DataScopeDeniedMessage) {
		t.Errorf("非归属应 403+行级隔离文案，实得 %d / %s", code, strings.TrimSpace(body))
	}
}

func TestDataScopeGuard_PassesRegisteredIdentifiersToLookup(t *testing.T) {
	// 守卫交给查询源的三元组必须逐字来自登记表：写错一列（比如把 user_id 当 agent_id）
	// 在这里就会露出来，而不是变成"所有人都读不到"或"所有人都读得到"。
	var gotTable, gotIDCol, gotOwnerCol, gotValue string
	dsRun(t, dsCall{
		method: http.MethodPost, path: "/api/customer-sessions/:sess/priority",
		register: true, row: dsSessionRow(), role: "staff", uid: 7,
		lookup: func(ctx context.Context, table, idColumn, ownerColumn, idValue string) (string, bool, error) {
			gotTable, gotIDCol, gotOwnerCol, gotValue = table, idColumn, ownerColumn, idValue
			return "7", true, nil
		},
	})
	if gotTable != "customer_sessions" || gotIDCol != "session_id" || gotOwnerCol != "agent_id" {
		t.Errorf("查询源收到的三元组不是登记表里的值：%s / %s / %s", gotTable, gotIDCol, gotOwnerCol)
	}
	if gotValue != "S-1" {
		t.Errorf("归属查询收到的定位值是 %q，不是路径参数 %q —— 路由参数名与登记不一致时每一行都查不到",
			gotValue, dsSessionRow().Param)
	}
}

func TestDataScopeGuard_UnassignedPool(t *testing.T) {
	// 未分配（agent_id=0）必须放行：待领取池关掉等于把工作流关掉。
	row := dsSessionRow()
	code, body, handled := dsRun(t, dsCall{
		method: http.MethodGet, path: "/api/customer-sessions/:sess/internal-notes",
		register: true, row: row, role: "staff", uid: 7,
		lookup: okLookup("0", true, nil),
	})
	if !handled || code != http.StatusTeapot {
		t.Errorf("AllowUnowned 的口上未分配数据应放行，实得 code=%d handled=%v body=%s",
			code, handled, strings.TrimSpace(body))
	}

	// 同一份数据、AllowUnowned=false ⇒ 拦。这一档当前登记表里还没有真实行，
	// 所以靠注入的登记表读取口测（见 dataScopeLookup 的注释）。
	row.AllowUnowned = false
	code, body, handled = dsRun(t, dsCall{
		method: http.MethodGet, path: "/api/customer-sessions/:sess/internal-notes",
		register: true, row: row, role: "staff", uid: 7,
		lookup: okLookup("0", true, nil),
	})
	if handled || code != http.StatusForbidden || !strings.Contains(body, DataScopeDeniedMessage) {
		t.Errorf("不允许未分配放行的口却读到了共享池：code=%d handled=%v body=%s",
			code, handled, strings.TrimSpace(body))
	}
}

func TestDataScopeGuard_UsernameOwnedRow(t *testing.T) {
	// 另一类归属列存的是 username（inbox_conversations.assigned_to 那种口径）。
	// 猜错存的是 id 还是用户名的结果都不是报错，而是所有人都读不到或都读得到。
	row := model.DataScopeRow{
		Table: "inbox_conversations", IDColumn: "id", Param: "id",
		OwnerColumn: "assigned_to", OwnerIsUsername: true, AllowUnowned: true,
	}
	for _, c := range []struct {
		name, username, owner string
		wantHandled           bool
		wantCode              int
	}{
		{"用户名匹配", "agent-a", "agent-a", true, http.StatusTeapot},
		{"用户名不匹配", "agent-a", "agent-b", false, http.StatusForbidden},
	} {
		code, body, handled := dsRun(t, dsCall{
			method: http.MethodGet, path: "/api/inbox/conversations/:id",
			register: true, row: row, role: "staff", uid: 7, username: c.username,
			lookup: okLookup(c.owner, true, nil),
		})
		if handled != c.wantHandled || code != c.wantCode {
			t.Errorf("%s：实得 code=%d handled=%v body=%s，期望 code=%d handled=%v",
				c.name, code, handled, strings.TrimSpace(body), c.wantCode, c.wantHandled)
		}
	}
}

func TestDataScopeGuard_DepartmentAndTeamNarrowToSelf(t *testing.T) {
	// 成员关系表 team_users 已被 DROP，department/team 没有事实源 ⇒ 只能按 self 收。
	// 这条断的是"没有偷偷按 all 处理"：按 all 就是越权面。
	for _, scope := range []string{model.DataScopeDepartment, model.DataScopeTeam} {
		code, body, handled := dsRun(t, dsCall{
			method: http.MethodDelete, path: "/api/customer-sessions/:sess/edit-lock",
			register: true, row: dsSessionRow(), role: "staff", scope: scope, uid: 7,
			lookup: okLookup("8", true, nil),
		})
		if handled || code != http.StatusForbidden {
			t.Errorf("data_scope=%s 时别人的数据被放行了（没有向 self 收窄）：%d / %s",
				scope, code, strings.TrimSpace(body))
		}
		// 自己的那行仍然要能读：收窄不能变成"这两档所有人都读不到"。
		code, body, handled = dsRun(t, dsCall{
			method: http.MethodDelete, path: "/api/customer-sessions/:sess/edit-lock",
			register: true, row: dsSessionRow(), role: "staff", scope: scope, uid: 7,
			lookup: okLookup("7", true, nil),
		})
		if !handled || code != http.StatusTeapot {
			t.Errorf("data_scope=%s 时自己的数据应放行：%d / %s", scope, code, strings.TrimSpace(body))
		}
	}
}

func TestDataScopeGuard_LookupOutcomes(t *testing.T) {
	// 查询源回三种结果对应三种对外口径，混用会把"没有这行"说成"不是你的行"。
	code, body, handled := dsRun(t, dsCall{
		method: http.MethodGet, path: "/api/customer-sessions/:sess/ai-summary",
		register: true, row: dsSessionRow(), role: "staff", uid: 7,
		lookup: okLookup("", false, nil),
	})
	if handled || code != http.StatusNotFound || !strings.Contains(body, "数据不存在") {
		t.Errorf("行不存在应 404，实得 %d / %s", code, strings.TrimSpace(body))
	}

	code, body, handled = dsRun(t, dsCall{
		method: http.MethodGet, path: "/api/customer-sessions/:sess/ai-summary",
		register: true, row: dsSessionRow(), role: "staff", uid: 7,
		lookup: okLookup("", false, errors.New("connection reset")),
	})
	if handled || code != http.StatusInternalServerError {
		t.Errorf("查询失败应 500（不能降级成放行或 404），实得 %d / %s", code, strings.TrimSpace(body))
	}
}

func TestDataScopeGuard_MissingIdentityDenies(t *testing.T) {
	// 有归属值却读不到调用者 ⇒ 守卫挂在 JWT 之前或令牌缺字段，按 401 拦并出声。
	code, body, handled := dsRun(t, dsCall{
		method: http.MethodGet, path: "/api/customer-sessions/:sess/transcript",
		register: true, row: dsSessionRow(), role: "staff",
		lookup: okLookup("8", true, nil),
	})
	if handled || code != http.StatusUnauthorized {
		t.Errorf("无身份打登记口应 401，实得 %d / %s", code, strings.TrimSpace(body))
	}

	// uid 存在但 username 缺失（username 口径的行）：同样是 401，不能算成"不匹配"给 403。
	row := dsSessionRow()
	row.OwnerIsUsername = true
	code, body, handled = dsRun(t, dsCall{
		method: http.MethodGet, path: "/api/customer-sessions/:sess/transcript",
		register: true, row: row, role: "staff", uid: 7,
		lookup: okLookup("agent-a", true, nil),
	})
	if handled || code != http.StatusUnauthorized {
		t.Errorf("username 口径缺用户名应 401，实得 %d / %s", code, strings.TrimSpace(body))
	}
}

func TestDataScopeGuard_RegistryShapeMismatchDenies(t *testing.T) {
	// 登记表写的参数名在这条路由上不存在（路由改名/参数改名 while 登记没跟上）：
	// 拿不到定位值就没法判归属，向拒的方向收并出声，不能拿空串去查库。
	row := dsSessionRow()
	row.Param = "not_on_this_route"
	code, body, handled := dsRun(t, dsCall{
		method: http.MethodGet, path: "/api/customer-sessions/:sess/transcript",
		register: true, row: row, role: "staff", uid: 7,
		lookup: okLookup("7", true, nil),
	})
	if handled || code != http.StatusForbidden {
		t.Errorf("登记表与路由形状不符应 403，实得 %d / %s", code, strings.TrimSpace(body))
	}
}

// TestRealDataScopeRegistry_Shape 直接扫真实登记表（不注入）：
// 常量表没有编译器管，写歪一格就是这一档静默失效。
func TestRealDataScopeRegistry_Shape(t *testing.T) {
	if len(model.DataScopeRegistrations()) == 0 {
		t.Fatal("行级隔离登记表是空的：DataScopeGuard 恒真，这一层不存在")
	}
	if bad := model.DataScopeIdentifiersSafe(); len(bad) > 0 {
		t.Errorf("登记表里有 %d 个表名/列名不是合法标识符（会被拼进 SQL）：%v", len(bad), bad)
	}
	for _, key := range model.DataScopeRegistrations() {
		row, ok := model.DataScopeRowOf(key)
		if !ok {
			t.Errorf("%s 在名单里却查不到登记（键与表不成对）", key)
			continue
		}
		parts := strings.SplitN(key, " ", 2)
		if len(parts) != 2 {
			t.Errorf("登记键 %q 不是 \"METHOD /path\" 形状", key)
			continue
		}
		if !strings.Contains(parts[1], ":"+row.Param) {
			t.Errorf("登记键 %s 的路径模板上没有 :%s：守卫永远取不到定位值", key, row.Param)
		}
		if row.Table == "" || row.IDColumn == "" || row.OwnerColumn == "" {
			t.Errorf("登记 %s 缺少表/定位列/归属列之一：%+v", key, row)
		}
	}
	t.Logf("登记表 %d 行，形状全部合法", len(model.DataScopeRegistrations()))
}
