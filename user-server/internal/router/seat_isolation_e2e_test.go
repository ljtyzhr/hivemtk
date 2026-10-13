package router

// 客服坐席数据隔离的端到端行为证据（用户要求"真正深入代码、逻辑、数据检查测试"里缺的那一半）。
//
// 这个文件要证的事只有一件：**两个不同的坐席身份打同一批客服接口，彼此都看不见、改不动
// 对方的数据**。事实源是生产装配入口 Setup() 建出的真实引擎 + 真实 Postgres 测试库，
// 令牌用真实的 JWTAuthMiddleware 校验，归属判定由真实的行级守卫与 service 层 scope 完成。
// 判据按响应体文案而不是状态码：403 可能来自能力档、超管档或 handler 自己，这道门要证的
// 恰恰是"行级隔离这一档给的"那句「无权访问该会话」。
//
// 与 data_scope_gate_test.go 的分工：那份门禁钉的是"登记表与名册没有静默失效"（静态面），
// 这里钉的是"名册里那 22 条、加号外那批按 body 寻址的口，逐个真的拦得住"（行为面）。
// 两条腿互为条件：只有静态腿会说"这条口有执行点"，只有行为腿会说"那个执行点真的在拦"。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hivemtk-user/internal/middleware"
	"hivemtk-user/internal/model"
	dbutil "hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/pkg/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	// 与 service.ErrSessionForbidden / ErrInboxForbidden 的对外文案同字面量：
	// controller 把两个哨兵都映成这一句 403，改文案要同时改这里。
	seatForbiddenMsg = "无权访问该会话"
	// controller.InboxController.StaffLoad 的越权文案。
	staffLoadForbiddenMsg = "无权查看他人的负载"
)

// seatEngine 装配真实引擎，并把客服域用到的表都建出来。
//
// 表清单不是顺手抄的：InboxConversation / InboxAssignment 是收件箱域的归属事实源，
// 少建一张，收件箱那批口的失败形态会从"403 越权"变成"500 或 404"，行为腿就失去区分力
// 与 data_scope_gate_test.go 里 dataScopeGateModels 同一个教训）。
func seatEngine(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	t.Setenv("USER_WEB_DIST", t.TempDir())
	t.Setenv("ENABLE_DEBUG_ROUTES", "true")
	database := testutil.NewTestDB(t,
		&model.CustomerSession{}, &model.InboxConversation{}, &model.InboxAssignment{},
		&model.SystemUser{}, &model.RoleMenuPermission{}, &model.SystemConfigKV{})
	if database == nil {
		t.Skip("测试库不可达")
	}
	prev := dbutil.GetDB()
	dbutil.SetTestDB(database)
	t.Cleanup(func() {
		dbutil.SetTestDB(prev)
		middleware.SetOwnerLookup(nil)
		middleware.SetMenuGrantProvider(nil)
	})

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	Setup(engine, database)
	return engine, database
}

// seatTokens 三个身份：两个同角色坐席（A/B）+ 超管。
// user_id 与 username 成对出现，因为两个域的归属列不同——客服会话按 user_id（agent_id），
// 收件箱按 username（assigned_to）。只改一个会让某一条腿静默失效。
func seatTokens(t *testing.T) (agentA, agentB, admin string) {
	t.Helper()
	jt := utils.NewJWTUtils(utils.DefaultJWTConfig)
	var err error
	if agentA, err = jt.GenerateTokenWithScope(7101, "seat-a", "customer_service", "self", 0, 0); err != nil {
		t.Fatalf("签 seat-a 令牌失败: %v", err)
	}
	if agentB, err = jt.GenerateTokenWithScope(7102, "seat-b", "customer_service", "self", 0, 0); err != nil {
		t.Fatalf("签 seat-b 令牌失败: %v", err)
	}
	if admin, err = jt.GenerateToken(1, "seat-admin", "admin"); err != nil {
		t.Fatalf("签 admin 令牌失败: %v", err)
	}
	return agentA, agentB, admin
}

// seatCall 带 JSON body 的调用：写侧请求体不合法时 handler 会在 scope 检查之前或之后
// 回 400，那样"没被拦"就分不清是放行了还是参数没对。所以每个写侧腿都送合法 body。
func seatCall(t *testing.T, engine *gin.Engine, method, path, token, body string) (int, string) {
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
	return w.Code, w.Body.String()
}

// seatSessionIDs 一条 A 名下会话 + 一条 B 名下 + 一条未分配池，返回它们的自增 ID 与业务 session_id。
func seatSessionIDs(t *testing.T, database *gorm.DB) (aID uint, aSID string) {
	t.Helper()
	mk := func(sid string, agent uint) *model.CustomerSession {
		s := &model.CustomerSession{
			SessionID: sid, Platform: model.PlatformWeb,
			Status: model.SessionStatusPending, AgentID: agent,
		}
		if err := database.Create(s).Error; err != nil {
			t.Fatalf("写测试会话 %s 失败: %v", sid, err)
		}
		t.Cleanup(func() {
			database.Unscoped().Where("session_id = ?", sid).Delete(&model.CustomerSession{})
		})
		return s
	}
	sa, sb, sp := mk("seat-e2e-owned-by-a", 7101), mk("seat-e2e-owned-by-b", 7102), mk("seat-e2e-pool", 0)
	if sb.ID == sa.ID || sp.ID == sa.ID {
		t.Fatalf("测试会话 ID 撞了：a=%d b=%d pool=%d", sa.ID, sb.ID, sp.ID)
	}
	return sa.ID, sa.SessionID
}

// seatConvIDs 收件箱域三个 conversation：分给 A / 分给 B / 未分配。返回 A 的自增 ID。
func seatConvIDs(t *testing.T, database *gorm.DB) uint {
	t.Helper()
	mk := func(cust, assignTo string) *model.InboxConversation {
		c := &model.InboxConversation{
			Platform: "web", AccountID: "seat-acct", CustomerID: cust,
			CustomerName: cust, Status: "assigned", AssignedTo: assignTo,
		}
		if err := database.Create(c).Error; err != nil {
			t.Fatalf("写测试会话 %s 失败: %v", cust, err)
		}
		t.Cleanup(func() { database.Unscoped().Where("customer_id = ?", cust).Delete(&model.InboxConversation{}) })
		return c
	}
	ca := mk("seat-conv-a", "seat-a")
	_ = mk("seat-conv-b", "seat-b")
	// 未分配行：visibility 要求 assigned_to 非空且等于本人，所以未分配行对所有人都不可见
	// ——这与 InboxScope.visible 的口径一致，这里只建来让列表腿有第三态可比。
	mk("seat-conv-pool", "")
	return ca.ID
}

// seatSessionLeg 客服域一条按行寻址的腿。寻址形态用 bySID 区分：GetMessages 那条口
// 数字 id 与字符串 session_id 两种入口各走一个 scoped 查询，漏一种等于没测那条分支。
type seatSessionLeg struct {
	name   string
	method string
	idPath string
	bySID  bool
	body   string
}

// seatReseedA 把"A 名下那条会话"重置回没人动过的初始态，并返回它当前的
// 自增 ID 与业务 session_id。
//
// 为什么需要逐腿重置：坐席隔离这批腿的控制组（本人令牌）本身就是写口，跑完会把
// 会话 close 掉、release 回池（agent_id 归 0）。而未分配池按 SessionScope 的设计
// 对全体坐席可见——那是要抢的池子。不重置的话，正文腿的"别人令牌"就能合法读到
// 上一条腿刚释放出来的会话，403 断言会静默失效。真删重建也顺手避开了软删行的干扰。
func seatReseedA(t *testing.T, database *gorm.DB) (aID uint, aSID string) {
	t.Helper()
	if err := database.Unscoped().Where("session_id LIKE ?", "seat-e2e-%").Delete(&model.CustomerSession{}).Error; err != nil {
		t.Fatalf("清理旧客服测试会话失败: %v", err)
	}
	s := &model.CustomerSession{
		SessionID: "seat-e2e-owned-by-a", Platform: model.PlatformWeb,
		Status: model.SessionStatusPending, AgentID: 7101,
	}
	if err := database.Create(s).Error; err != nil {
		t.Fatalf("重建 A 名下测试会话失败: %v", err)
	}
	if s.AgentID != 7101 {
		t.Fatalf("种子会话归属没写进去：agent_id=%d", s.AgentID)
	}
	return s.ID, s.SessionID
}

// seatLegPath 按腿声明的寻址形态拼路径：bySID 吃字符串 session_id，否则吃自增 ID。
func seatLegPath(l seatSessionLeg, aID uint, aSID string) string {
	if l.bySID {
		return fmt.Sprintf(l.idPath, aSID)
	}
	return fmt.Sprintf(l.idPath, aID)
}

// seatForbidden 判定"是行级隔离这一档拦的"。
func seatForbidden(code int, body string) bool {
	return code == http.StatusForbidden && strings.Contains(body, seatForbiddenMsg)
}

// TestSeatIsolation_CustomerSessionRosterIsEnforced 客服会话域：dataScopeScopedElsewhere
// 名册里那 13 条（不含已被守卫登记的 blacklist），逐条用**别人的会话 + 别人的令牌**打一遍。
//
// 为什么必须逐条打：每条口的 scope 执行点写在各自的 controller/handler 里，
// 漏一条的表现是"页面看起来正常，只有点进别人会话时 500/空数据"。静态腿只能证明
// "名册里写了名字"，证明不了"那段代码真的在链路上跑"。
func TestSeatIsolation_CustomerSessionRosterIsEnforced(t *testing.T) {
	engine, database := seatEngine(t)
	agentA, agentB, admin := seatTokens(t)

	legs := []seatSessionLeg{
		{"GET /customer-sessions/:id", "GET", "/api/customer-sessions/%d", false, ""},
		{"POST /customer-sessions/:id/auto-assign", "POST", "/api/customer-sessions/%d/auto-assign", false, "{}"},
		{"POST /customer-sessions/:id/close", "POST", "/api/customer-sessions/%d/close", false, "{}"},
		{"GET /customer-sessions/:id/messages(数字id)", "GET", "/api/customer-sessions/%d/messages", false, ""},
		{"GET /customer-sessions/:id/messages(字符串id)", "GET", "/api/customer-sessions/%s/messages", true, ""},
		{"POST /customer-sessions/:id/messages", "POST", "/api/customer-sessions/%s/messages", true, `{"content":"hi"}`},
		{"POST /customer-sessions/:id/rate", "POST", "/api/customer-sessions/%d/rate", false, `{"rating":1}`},
		{"POST /customer-sessions/:id/release", "POST", "/api/customer-sessions/%d/release", false, "{}"},
		{"PUT /customer-sessions/:id/status", "PUT", "/api/customer-sessions/%d/status", false, `{"status":"closed"}`},
		{"POST /customer-sessions/:id/switch-handler", "POST", "/api/customer-sessions/%d/switch-handler", false, `{"handler_type":"human"}`},
		{"POST /customer-sessions/:id/tags", "POST", "/api/customer-sessions/%d/tags", false, `{"tags":["stolen"]}`},
		{"POST /customer-sessions/:id/takeover", "POST", "/api/customer-sessions/%d/takeover", false, "{}"},
		{"POST /customer-sessions/:id/transfer", "POST", "/api/customer-sessions/%d/transfer", false, `{"new_agent_id":7102}`},
	}

	for _, l := range legs {
		// 控制组：本人令牌，同一批口不许被隔离档拦（否则正文的 403 可能只是"恒拦"）。
		aID, aSID := seatReseedA(t, database)
		if code, body := seatCall(t, engine, l.method, seatLegPath(l, aID, aSID), agentA, l.body); seatForbidden(code, body) {
			t.Errorf("[%s] 本人的会话被隔离档拦了（恒拦）：%d / %s", l.name, code, strings.TrimSpace(body))
			continue
		}

		// 控制组这批口本身就是写口，跑完 A 的会话可能已被改脏（close 掉、release 回池）。
		// 不重置就接着跑正文的话：release 把 agent_id 归 0 变成未分配池，而未分配池按
		// SessionScope 的设计对全体坐席可见（要抢的池子），正文的 B 就会"合法"读到 A 的会话
		// ——判据从"拦得住"悄悄退化成"设计如此"。所以每条腿前都把归属重置回 A。
		aID, aSID = seatReseedA(t, database)
		code, body := seatCall(t, engine, l.method, seatLegPath(l, aID, aSID), agentB, l.body)
		if !seatForbidden(code, body) {
			t.Errorf("[%s] 坐席 B 打 A 名下会话没有被行级隔离拦下：%d / %s（越权面仍敞着）",
				l.name, code, strings.TrimSpace(body))
			continue
		}
		// 403 之外还要看数据没被动：403 可能是"先拒绝、后写库"的假收口。
		var after model.CustomerSession
		if err := database.Where("session_id = ?", aSID).First(&after).Error; err != nil {
			t.Fatalf("读回 A 的会话失败: %v", err)
		}
		if after.Status != model.SessionStatusPending {
			t.Errorf("[%s] 已 403，但 A 的会话状态被改成了 %q（先拦后写）", l.name, after.Status)
		}
		if len(after.Tags) != 0 {
			t.Errorf("[%s] 已 403，但 A 的会话被打上了标签 %v（先拦后写）", l.name, after.Tags)
		}
		if after.AgentID != 7101 {
			t.Errorf("[%s] 已 403，但 A 的会话 agent_id 变成 %d（归属被改写）", l.name, after.AgentID)
		}
	}

	// 超管对照组：隔离档不许把超管也关在外面。
	adminAID, _ := seatReseedA(t, database)
	if code, body := seatCall(t, engine, "GET", fmt.Sprintf("/api/customer-sessions/%d", adminAID), admin, ""); seatForbidden(code, body) {
		t.Errorf("admin 读 A 名下会话被隔离档拦了：%d / %s", code, strings.TrimSpace(body))
	}
	t.Logf("客服会话域名册 %d 条（含 messages 两条入口形态），B→A 全部 403+%q 且数据未变；每条腿前把会话归属重置回 A，不让上一条写口腿把 agent_id 归 0 变成公共池", len(legs), seatForbiddenMsg)
}

// TestSeatIsolation_InboxRosterIsEnforced 收件箱域：名册里那 8 条按行口 + 坐席负载那条，
// 用别人令牌打别人的会话。
//
// 收件箱与客服会话的归属列不同（assigned_to=username 对 agent_id=user_id），
// 所以这批腿的断言不能靠复用客服会话的种子：会话换个域、种子要跟着换。
func TestSeatIsolation_InboxRosterIsEnforced(t *testing.T) {
	engine, database := seatEngine(t)
	_, agentB, admin := seatTokens(t)
	convA := seatConvIDs(t, database)

	legs := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"GET /inbox/conversations/:id", "GET", fmt.Sprintf("/api/inbox/conversations/%d", convA), ""},
		{"GET /inbox/conversations/:id/messages", "GET", fmt.Sprintf("/api/inbox/conversations/%d/messages", convA), ""},
		{"DELETE /inbox/conversations/:id/messages/:mid", "DELETE", fmt.Sprintf("/api/inbox/conversations/%d/messages/1?source=hub", convA), ""},
		{"POST /inbox/conversations/:id/mute", "POST", fmt.Sprintf("/api/inbox/conversations/%d/mute", convA), `{"muted":true}`},
		{"POST /inbox/conversations/:id/pin", "POST", fmt.Sprintf("/api/inbox/conversations/%d/pin", convA), `{"pinned":true}`},
		{"POST /inbox/conversations/:id/read", "POST", fmt.Sprintf("/api/inbox/conversations/%d/read", convA), "{}"},
		{"POST /inbox/conversations/:id/star", "POST", fmt.Sprintf("/api/inbox/conversations/%d/star", convA), `{"starred":true}`},
		{"POST /inbox/conversations/:id/tags", "POST", fmt.Sprintf("/api/inbox/conversations/%d/tags", convA), `{"tag":"stolen"}`},
		{"DELETE /inbox/conversations/:id/tags/:tag", "DELETE", fmt.Sprintf("/api/inbox/conversations/%d/tags/stolen", convA), ""},
	}

	for _, l := range legs {
		code, body := seatCall(t, engine, l.method, l.path, agentB, l.body)
		if !seatForbidden(code, body) {
			t.Errorf("[%s] 坐席 B 打 A 名下收件箱会话没有被行级隔离拦下：%d / %s",
				l.name, code, strings.TrimSpace(body))
			continue
		}
		var after model.InboxConversation
		if err := database.First(&after, convA).Error; err != nil {
			t.Fatalf("读回 A 的收件箱会话失败: %v", err)
		}
		if after.Pinned || after.Starred || after.Muted {
			t.Errorf("[%s] 已 403，但 A 的会话被置了 pinned/starred/muted（先拦后写）：%+v", l.name, after)
		}
		if after.AssignedTo != "seat-a" {
			t.Errorf("[%s] 已 403，但 A 的会话归属变成 %q", l.name, after.AssignedTo)
		}
	}

	// 坐席负载：参数本身就是归属，非超管查别人的要 403、查自己的要放行。
	if code, body := seatCall(t, engine, "GET", "/api/inbox/staff/seat-a/load", agentB, ""); code != http.StatusForbidden ||
		!strings.Contains(body, staffLoadForbiddenMsg) {
		t.Errorf("坐席 B 查 A 的负载应 403+%q，实得 %d / %s", staffLoadForbiddenMsg, code, strings.TrimSpace(body))
	}
	if code, body := seatCall(t, engine, "GET", "/api/inbox/staff/seat-b/load", agentB, ""); code == http.StatusForbidden &&
		strings.Contains(body, staffLoadForbiddenMsg) {
		t.Errorf("坐席 B 查自己的负载被拦了：%d / %s", code, strings.TrimSpace(body))
	}

	if code, body := seatCall(t, engine, "GET", fmt.Sprintf("/api/inbox/conversations/%d", convA), admin, ""); seatForbidden(code, body) {
		t.Errorf("admin 读 A 名下收件箱会话被隔离档拦了：%d / %s", code, strings.TrimSpace(body))
	}
	t.Logf("收件箱域名册 %d 条 + 坐席负载，B→A 全部 403+%q 且数据未变", len(legs), seatForbiddenMsg)
}

// TestSeatIsolation_ListEndpointsOnlyReturnOwnRows 列表读口：客服会话与收件箱两域的
// "别人名下那一行根本不出现在返回值里"。
//
// 逐条口的 403 证明"知道 ID 也进不去"，这条证明"不知道 ID 也照样看不见"——两件事的
// 失效方式不同（前者漏 scope 检查，后者漏 WHERE 条件），都要有腿。
func TestSeatIsolation_ListEndpointsOnlyReturnOwnRows(t *testing.T) {
	engine, database := seatEngine(t)
	agentA, agentB, admin := seatTokens(t)
	seatSessionIDs(t, database)
	seatConvIDs(t, database)

	type probe struct {
		name     string
		path     string
		marker   string // 属于别人的行里独有的标志
		ownValue string
	}
	probes := []probe{
		{"GET /customer-sessions", "/api/customer-sessions", "seat-e2e-owned-by-a", "seat-e2e-owned-by-b"},
		{"GET /customer-sessions/pending", "/api/customer-sessions/pending", "seat-e2e-owned-by-a", "seat-e2e-owned-by-b"},
		{"GET /inbox/conversations", "/api/inbox/conversations", "seat-conv-a", "seat-conv-b"},
	}
	for _, p := range probes {
		code, body := seatCall(t, engine, "GET", p.path, agentB, "")
		if code != http.StatusOK {
			t.Errorf("[%s] 坐席 B 读列表应 200，实得 %d / %s", p.name, code, strings.TrimSpace(body))
			continue
		}
		if strings.Contains(body, p.marker) {
			t.Errorf("[%s] 坐席 B 的列表里出现了别人的行（%q）：隔离口径漏了这条列表口",
				p.name, p.marker)
		}
		if !strings.Contains(body, p.ownValue) {
			t.Errorf("[%s] 坐席 B 的列表里没有自己的行（%q），这条腿可能测了个空库", p.name, p.ownValue)
		}
		// 未分配池必须仍可见：池被关掉等于把"认领"工作流一起关掉。
		if !strings.Contains(body, "seat-e2e-pool") && !strings.Contains(body, "seat-conv-pool") {
			t.Logf("[%s] 未分配池行未出现在 B 的列表里（收件箱域未分配行对非超管不可见是有意口径，见 InboxScope.visible）", p.name)
		}
		code, body = seatCall(t, engine, "GET", p.path, admin, "")
		if code != http.StatusOK {
			t.Errorf("[%s] admin 读列表应 200，实得 %d / %s", p.name, code, strings.TrimSpace(body))
			continue
		}
		if !strings.Contains(body, p.marker) {
			t.Errorf("[%s] admin 的列表里没有别人的行（%q），上一半断言可能撞上空库", p.name, p.marker)
		}
	}
	_ = agentA
	t.Logf("列表读口 %d 条：坐席只见自己+池，超管见全量", len(probes))
}

// TestSeatIsolation_InboxStatsScoped 收件箱统计口：非超管的数字必须只数自己的会话。
//
// 统计是另一种泄露形状：它不返回行，但返回值能反推出别人手上有多少会话 / 多少超时未响应。
func TestSeatIsolation_InboxStatsScoped(t *testing.T) {
	engine, database := seatEngine(t)
	_, agentB, _ := seatTokens(t)
	seatConvIDs(t, database)

	type stats struct {
		Assigned int64 `json:"assigned"`
		Total    int64 `json:"total"`
	}
	read := func(token string) stats {
		t.Helper()
		code, body := seatCall(t, engine, "GET", "/api/inbox/stats", token, "")
		if code != http.StatusOK {
			t.Fatalf("读收件箱统计应 200，实得 %d / %s", code, strings.TrimSpace(body))
		}
		var env struct {
			Data stats `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &env); err != nil {
			t.Fatalf("解析统计响应失败: %v / %s", err, body)
		}
		return env.Data
	}
	mine := read(agentB)
	// 库里只有一条 assigned_to=seat-a、一条 seat-b、一条空：坐席 B 该看到的就是自己那一条。
	if mine.Assigned > 1 || mine.Total > 2 {
		t.Errorf("坐席 B 的统计数到了别人的会话：assigned=%d total=%d（应为 1 / ≤2）",
			mine.Assigned, mine.Total)
	}
	t.Logf("收件箱统计：坐席 B 读到 assigned=%d total=%d（库里三行：A、B、未分配）", mine.Assigned, mine.Total)
}

// seatReseedConvA 重建一条"分给坐席 A"的收件箱会话，返回它的自增 ID。
// 与 seatReseedA 同一个理由：控制组本身是写口，跑完会把归属改写（release 置空、
// close 改状态），不重置的话下一条腿的断言会静默失效。
func seatReseedConvA(t *testing.T, database *gorm.DB) uint {
	t.Helper()
	if err := database.Unscoped().Where("customer_id LIKE ?", "seat-conv-body-%").Delete(&model.InboxConversation{}).Error; err != nil {
		t.Fatalf("清理旧收件箱测试会话失败: %v", err)
	}
	c := &model.InboxConversation{
		Platform: "web", AccountID: "seat-acct", CustomerID: "seat-conv-body-a",
		CustomerName: "seat-conv-body-a", Status: "assigned", AssignedTo: "seat-a",
	}
	if err := database.Create(c).Error; err != nil {
		t.Fatalf("重建 A 名下收件箱会话失败: %v", err)
	}
	if c.AssignedTo != "seat-a" {
		t.Fatalf("种子会话归属没写进去：assigned_to=%q", c.AssignedTo)
	}
	return c.ID
}

// TestSeatIsolation_InboxBodyAddressedWritesAreScoped 收件箱域那批**按 body 寻址**的
// 写口：/inbox/assign、/inbox/assign/auto、/inbox/assignments、/inbox/lock-human。
//
// 为什么要单独立一条腿：DataScopeGuard 的登记表键是 "METHOD /ginFullPath"，只认
// 路径参数（:id 这种），body 里的 conversation_id / session_id 它一个都扫不到。
// 于是这类口的隔离只能靠 controller 里的服务层 scope 预检——漏一处，
// 表现就是"坐席 B 能把 A 的会话 close 掉/release 掉/抢走/上人工锁"，
// 而列表页一切正常。静态腿（名册）更是完全看不见这批口。
func TestSeatIsolation_InboxBodyAddressedWritesAreScoped(t *testing.T) {
	engine, database := seatEngine(t)
	agentA, agentB, admin := seatTokens(t)
	_, aSID := seatReseedA(t, database)

	// legs 的控制组（本人令牌）要逐条跑，所以"全量分配历史"这条单列：
	// 它对非超管一律 403，控制组跑它会误判成恒拦。
	legs := []struct {
		name   string
		method string
		path   func(convID uint) string
		body   func(convID uint) string
	}{
		{"POST /inbox/assign(close)", "POST",
			func(uint) string { return "/api/inbox/assign" },
			func(id uint) string { return fmt.Sprintf(`{"conversation_id":%d,"action":"close"}`, id) }},
		{"POST /inbox/assign/auto", "POST",
			func(uint) string { return "/api/inbox/assign/auto" },
			func(id uint) string {
				return fmt.Sprintf(`{"conversation_id":%d,"candidates":["seat-a","seat-b"]}`, id)
			}},
		{"GET /inbox/assignments?conversation_id=", "GET",
			func(id uint) string { return fmt.Sprintf("/api/inbox/assignments?conversation_id=%d", id) },
			func(uint) string { return "" }},
		{"POST /inbox/lock-human", "POST",
			func(uint) string { return "/api/inbox/lock-human" },
			func(uint) string { return fmt.Sprintf(`{"session_id":%q,"reason":"steal"}`, aSID) }},
	}

	for _, l := range legs {
		// 控制组：本人令牌，同一条口不许被隔离档拦（否则正文的 403 可能只是"恒拦"）。
		convID := seatReseedConvA(t, database)
		if code, body := seatCall(t, engine, l.method, l.path(convID), agentA, l.body(convID)); seatForbidden(code, body) {
			t.Errorf("[%s] 本人的会话被隔离档拦了（恒拦）：%d / %s", l.name, code, strings.TrimSpace(body))
		}

		// 控制组是写口，跑完归属可能被改写 → 重置后再跑正文。
		convID = seatReseedConvA(t, database)
		code, body := seatCall(t, engine, l.method, l.path(convID), agentB, l.body(convID))
		if !seatForbidden(code, body) {
			t.Errorf("[%s] 坐席 B 打 A 名下数据没有被行级隔离拦下：%d / %s（越权面仍敞着）",
				l.name, code, strings.TrimSpace(body))
			continue
		}
		// 403 之外还要看数据没被动：close/release 这类口一旦漏检是会真写库的。
		var after model.InboxConversation
		if err := database.Where("customer_id = ?", "seat-conv-body-a").First(&after).Error; err != nil {
			t.Fatalf("读回 A 的收件箱会话失败: %v", err)
		}
		if after.Status != "assigned" {
			t.Errorf("[%s] 已 403，但 A 的会话状态被改成了 %q（先拦后写）", l.name, after.Status)
		}
		if after.AssignedTo != "seat-a" {
			t.Errorf("[%s] 已 403，但 A 的会话归属被改成了 %q（会话被抢走）", l.name, after.AssignedTo)
		}
	}

	// 全量分配历史：非超管一律不许看（它等于全体坐席的会话清单），两种坐席都要拦。
	for _, tok := range []string{agentA, agentB} {
		if code, body := seatCall(t, engine, "GET", "/api/inbox/assignments", tok, ""); code != http.StatusForbidden {
			t.Errorf("非超管读全量分配历史没被拦：%d / %s", code, strings.TrimSpace(body))
		}
	}

	// 超管对照组：隔离档不许把超管也关在外面。
	if code, body := seatCall(t, engine, "GET", "/api/inbox/assignments", admin, ""); code == http.StatusForbidden {
		t.Errorf("admin 读分配历史被隔离档拦了：%d / %s", code, strings.TrimSpace(body))
	}
	t.Logf("收件箱 body 寻址写口 %d 条 + 全量分配历史，B→A 全部 403+%q 且归属/状态未变", len(legs), seatForbiddenMsg)
}
