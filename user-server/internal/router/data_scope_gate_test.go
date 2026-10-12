package router

// 行级数据隔离的常驻门。
//
// 这一层的事实源是 model 里那张编译期登记表，没有编译器管它，四种失效都长得像"正常工作"：
//   一、登记的路径写错或接口已下线 ⇒ 这一行永远命中不到，那条口的越权面照旧敞着；
//   二、登记的路由注册在 auth.Use(DataScopeGuard()) 快照之前 ⇒ 链上根本没这道守卫，
//       与当年 7 条 /api/monitor/* 匿名可读是同一种缺陷（gin 在注册时快照 handler 链）；
//   三、表名/列名写歪 ⇒ 归属查询报错或恒查不到，症状是"所有人都读不到"，看着像接口坏了；
//   四、查询源没装配 ⇒ 守卫按无归属拦，症状是"全都 403"，看着像权限配错。
//
// 前三条静态腿各钉一种，第四条由行为腿钉（真的拦得住别人的行、真的放行自己的行）。
// 判据按响应体文案而不是状态码：403 可能是能力档、超管档或 handler 自己给的，
// 这道门要证的恰恰是"是行级隔离这一档给的"。

import (
	"net/http"
	"strings"
	"testing"

	"hivemtk-user/internal/middleware"
	"hivemtk-user/internal/model"
	dbutil "hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/pkg/utils"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	guardDataScope   = "DataScopeGuard"
	dataScopeDenyMsg = "无权访问该数据（它不属于你）"
	dataScopeGoneMsg = "数据不存在"
)

// dataScopeGateModels 登记表里的表名 → 建它的那个 model。
// 新增一行登记若指向新表，必须在这里补上对应 model，否则本用例直接红：
// 这不是形式主义——列名写错、表名写错、model 没进 AutoMigrate 这三种缺陷，
// 都只有在真建出这张表后才量得出来。
var dataScopeGateModels = map[string]any{
	"customer_sessions": &model.CustomerSession{},
}

// TestDataSetGate_RegistrationsAreLiveAndGuarded 一、二两条腿合一：
// 登记键必须命中真实路由，且那条路由链上同时有 JWT（身份来源）与行级隔离守卫（裁决者）。
func TestDataSetGate_RegistrationsAreLiveAndGuarded(t *testing.T) {
	chains := liveRouteGuardChains(t)
	keys := model.DataScopeRegistrations()
	if len(keys) == 0 {
		t.Fatal("行级隔离登记表是空的：这道门恒绿，但这一层并不存在")
	}

	missing, naked, noIdentity := []string{}, []string{}, []string{}
	for _, key := range keys {
		chain, ok := chains[key]
		if !ok {
			missing = append(missing, key)
			continue
		}
		if !chainHasGuard(chain, guardDataScope) {
			naked = append(naked, key)
			continue
		}
		if !chainHasGuard(chain, guardJWT) {
			noIdentity = append(noIdentity, key)
		}
	}
	if len(missing) > 0 {
		t.Errorf("登记表里有 %d 条命中不到 live 路由（路径或方法写错、接口已下线）：%v"+
			"（死行不会拦任何人，只会让覆盖率读数虚高）", len(missing), missing)
	}
	if len(naked) > 0 {
		t.Errorf("有 %d 条登记路由链上没有 DataScopeGuard（注册在 auth.Use 快照之前）：%v", len(naked), naked)
	}
	if len(noIdentity) > 0 {
		t.Errorf("有 %d 条登记路由链上没有 JWTAuthMiddleware（守卫读不到 user_id/role）：%v", len(noIdentity), noIdentity)
	}
	t.Logf("登记表 %d 行，全部命中 live 路由且链上带 JWT + DataScopeGuard", len(keys))
}

// TestDataSetGate_RegisteredColumnsExist 第三条腿：登记表里的三元组必须真的建得出来。
func TestDataSetGate_RegisteredColumnsExist(t *testing.T) {
	models := []any{&model.SystemUser{}, &model.RoleMenuPermission{}}
	seenTable := map[string]bool{}
	type triple struct{ table, idCol, ownerCol string }
	triples := []triple{}
	for _, key := range model.DataScopeRegistrations() {
		row, ok := model.DataScopeRowOf(key)
		if !ok {
			t.Errorf("%s 查不到登记内容", key)
			continue
		}
		m, ok := dataScopeGateModels[row.Table]
		if !ok {
			t.Errorf("登记表 %s 指向表 %s，本用例不知道它由哪个 model 建"+
				"（新表：请在 dataScopeGateModels 里补一行，并确认它已进 AutoMigrate）", key, row.Table)
			continue
		}
		if !seenTable[row.Table] {
			seenTable[row.Table] = true
			models = append(models, m)
		}
		triples = append(triples, triple{row.Table, row.IDColumn, row.OwnerColumn})
	}
	if len(triples) == 0 {
		t.Fatal("没有任何可核对的登记行")
	}

	database := testutil.NewTestDB(t, models...)
	if database == nil {
		t.Skip("测试库不可达")
	}
	for _, tr := range triples {
		var tables int64
		database.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_name = ?", tr.table).Scan(&tables)
		if tables == 0 {
			t.Errorf("登记表指向的表 %s 在库里不存在（model 没建它 / 表名写错）", tr.table)
			continue
		}
		for _, col := range []string{tr.idCol, tr.ownerCol} {
			var cols int64
			database.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_name = ? AND column_name = ?",
				tr.table, col).Scan(&cols)
			if cols == 0 {
				t.Errorf("表 %s 上没有列 %s（登记写歪，归属查询会报错或恒查不到）", tr.table, col)
			}
		}
	}
	t.Logf("登记三元组 %d 组，表与两列全部存在", len(triples))
}

// —— 行为腿 ——

func dsGateEngine(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	t.Setenv("USER_WEB_DIST", t.TempDir())
	t.Setenv("ENABLE_DEBUG_ROUTES", "true")
	database := testutil.NewTestDB(t,
		&model.CustomerSession{}, &model.SystemUser{}, &model.RoleMenuPermission{}, &model.SystemConfigKV{})
	if database == nil {
		t.Skip("测试库不可达")
	}
	prev := dbutil.GetDB()
	dbutil.SetTestDB(database)
	t.Cleanup(func() {
		dbutil.SetTestDB(prev)
		// 查询源是包级全局：留给下一个用例一个指向已回收库的 provider，
		// 它读到的是 stale 连接而非真判定（与菜单授予源同一条清理口径）。
		middleware.SetOwnerLookup(nil)
	})

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	Setup(engine, database)
	return engine, database
}

func dsGateSession(t *testing.T, database *gorm.DB, sessionID string, agentID uint) {
	t.Helper()
	sess := model.CustomerSession{
		SessionID: sessionID,
		Platform:  model.PlatformWeb,
		Status:    model.SessionStatusPending,
		AgentID:   agentID,
	}
	if err := database.Create(&sess).Error; err != nil {
		t.Fatalf("写入测试会话 %s 失败: %v", sessionID, err)
	}
	t.Cleanup(func() {
		// Unscoped：软删除用例留下的行必须真清掉，否则它以 deleted_at 非空的状态留在共享测试库里，
		// 下一个按 session_id 取数的用例读到的就是上一轮的残留。
		database.Unscoped().Where("session_id = ?", sessionID).Delete(&model.CustomerSession{})
	})
}

func dsGateTokens(t *testing.T) (agentA, agentB, admin string) {
	t.Helper()
	jwtu := utils.NewJWTUtils(utils.DefaultJWTConfig)
	var err error
	agentA, err = jwtu.GenerateTokenWithScope(7, "ds-agent-a", "customer_service", "self", 0, 0)
	if err != nil {
		t.Fatalf("签 agentA 令牌失败: %v", err)
	}
	agentB, err = jwtu.GenerateTokenWithScope(8, "ds-agent-b", "customer_service", "self", 0, 0)
	if err != nil {
		t.Fatalf("签 agentB 令牌失败: %v", err)
	}
	admin, err = jwtu.GenerateToken(1, "ds-admin", "admin")
	if err != nil {
		t.Fatalf("签 admin 令牌失败: %v", err)
	}
	return agentA, agentB, admin
}

func dsDenied(code int, body string) bool {
	return code == http.StatusForbidden && strings.Contains(body, dataScopeDenyMsg)
}

// TestDataSetGate_RowOwnershipDrivesAccess 一条登记口（读整段转写）四种身份/四种归属：
// 别人的行拦、自己的行放、未分配的行放、超管放。缺任何一档，守卫都能在"恒拦"或"恒放"
// 下恒绿：只看反例会漏掉"把所有人关在外面"，只看正例会漏掉"越权面还在"。
func TestDataSetGate_RowOwnershipDrivesAccess(t *testing.T) {
	engine, database := dsGateEngine(t)
	agentA, agentB, admin := dsGateTokens(t)

	const sessA = "ds-gate-owned-by-a"
	const sessPool = "ds-gate-unassigned"
	dsGateSession(t, database, sessA, 7)
	dsGateSession(t, database, sessPool, 0)

	path := func(id string) string { return "/api/customer-sessions/" + id + "/transcript" }

	// 控制组：令牌确实能穿过 JWT，否则"没被行级隔离拦"可能是"压根没到守卫"。
	if code, body := gateCall(t, engine, "GET", path(sessA), ""); code != http.StatusUnauthorized ||
		!strings.Contains(body, jwtNoTokenMsg) {
		t.Fatalf("匿名打登记口应被 JWT 拦成 401+%q，实得 %d / %s（判据无从谈起）",
			jwtNoTokenMsg, code, strings.TrimSpace(body))
	}

	code, body := gateCall(t, engine, "GET", path(sessA), agentB)
	if !dsDenied(code, body) {
		t.Errorf("agent B 读 A 名下会话的转写没被行级隔离拦下（越权面仍敞着）：%d / %s",
			code, strings.TrimSpace(body))
	}

	code, body = gateCall(t, engine, "GET", path(sessA), agentA)
	if dsDenied(code, body) {
		t.Errorf("本人名下会话被行级隔离拦了（归属判定或查询源有问题）：%d / %s", code, strings.TrimSpace(body))
	}
	if code == http.StatusUnauthorized && strings.Contains(body, jwtNoTokenMsg) {
		t.Errorf("agentA 的令牌被 JWT 拒了：%d / %s，上面的\"没被拦\"是自证不了通过的读数",
			code, strings.TrimSpace(body))
	}

	// 未分配池：待领取的会话必须对所有坐席可见，否则把工作流一起关掉了。
	code, body = gateCall(t, engine, "GET", path(sessPool), agentB)
	if dsDenied(code, body) {
		t.Errorf("未分配会话被行级隔离拦了（AllowUnowned 没生效）：%d / %s", code, strings.TrimSpace(body))
	}

	// 超管恒可见。
	code, body = gateCall(t, engine, "GET", path(sessA), admin)
	if dsDenied(code, body) {
		t.Errorf("超管被行级隔离拦了（admin 档没短路）：%d / %s", code, strings.TrimSpace(body))
	}

	// 不存在的行：404 而不是 403，不把"没有这一行"说成"不是你的行"。
	code, body = gateCall(t, engine, "GET", path("ds-gate-not-exist"), agentB)
	if code != http.StatusNotFound || !strings.Contains(body, dataScopeGoneMsg) {
		t.Errorf("读不存在的会话应 404+%q，实得 %d / %s", dataScopeGoneMsg, code, strings.TrimSpace(body))
	}
}

// TestDataSetGate_SoftDeletedRowReadsAsMissing 软删除的会话必须与业务读口同口径判成"不存在"。
// 归属查询走的是原生 Table() 查询，不自动套模型的 deleted_at 条件；漏掉这一条时，
// 已删除的行仍会带着旧归属被查出来：归属者一路走到 handler（症状是"删掉的会话还能操作"），
// 非归属者收到 403（症状是"这行明明不存在却说它属于别人"）。
func TestDataSetGate_SoftDeletedRowReadsAsMissing(t *testing.T) {
	engine, database := dsGateEngine(t)
	_, agentB, _ := dsGateTokens(t)

	const sess = "ds-gate-soft-deleted"
	dsGateSession(t, database, sess, 8) // 归 B 自己：不加软删除条件时这一行对 B 是"放行"的
	if err := database.Where("session_id = ?", sess).Delete(&model.CustomerSession{}).Error; err != nil {
		t.Fatalf("软删除测试会话失败: %v", err)
	}

	code, body := gateCall(t, engine, "GET", "/api/customer-sessions/"+sess+"/transcript", agentB)
	if code != http.StatusNotFound || !strings.Contains(body, dataScopeGoneMsg) {
		t.Errorf("软删除后的会话应 404+%q，实得 %d / %s（归属查询没跟业务读口同口径过滤 deleted_at）",
			dataScopeGoneMsg, code, strings.TrimSpace(body))
	}
}

// TestDataSetGate_WriteSideAlsoCovered 写侧同一条登记口（改优先级）也要拦得住：
// 只测读侧会漏掉"读收口了、写没收口"，而写侧才是能改别人数据的那一半。
func TestDataSetGate_WriteSideAlsoCovered(t *testing.T) {
	engine, database := dsGateEngine(t)
	_, agentB, _ := dsGateTokens(t)

	const sessA = "ds-gate-write-owned-by-a"
	dsGateSession(t, database, sessA, 7)

	code, body := gateCall(t, engine, "PUT", "/api/customer-sessions/"+sessA+"/priority", agentB)
	if !dsDenied(code, body) {
		t.Errorf("agent B 改 A 名下会话的优先级没被拦下：%d / %s", code, strings.TrimSpace(body))
	}
}

// TestDataSetGate_LookupProviderReadsRealOwner 装配腿：真实查询源能读出归属值。
// 只测守卫会漏掉"注入的查询源压根没接上/读错列"，那时的表现是所有人都 403 或都读不到。
func TestDataSetGate_LookupProviderReadsRealOwner(t *testing.T) {
	_, database := dsGateEngine(t)
	const sess = "ds-gate-lookup-value"
	dsGateSession(t, database, sess, 4242)

	svc := service.NewDataScopeService()
	owner, found, err := svc.LookupOwner(t.Context(), "customer_sessions", "session_id", "agent_id", sess)
	if err != nil {
		t.Fatalf("归属查询报错（列名/CAST/表名哪一环有问题）: %v", err)
	}
	if !found {
		t.Fatal("刚写进去的会话查不到（定位列写错或查询没按业务主键走）")
	}
	if owner != "4242" {
		t.Errorf("归属读数 = %q，期望 \"4242\"（bigint 归属列取回形态不对）", owner)
	}

	// 不在登记表里的三元组必须报错，而不是把任意列名拼进 SQL。
	if _, _, err := svc.LookupOwner(t.Context(), "customer_sessions", "session_id", "user_name", sess); err == nil {
		t.Error("用未登记的归属列查到了结果：查询源没校验登记表")
	}
}
