package router

// 公开可达的"铸超管"路由，在这台实例已经装完之后必须一律 403。
//
// 面取自运行期事实（生产装配入口 Setup 跑出来的路由表），不是取自源码里的正则或名单：
// 有人新增一条别名、把 /system/init-admin 挪去别的组、或者给它换个 handler，
// 都会自动落进这条断言的射程里——"某处加了个没带守卫的入口"正是这条门要拦的形状，
// 而按路径写死的清单只会在新别名出现的那一天恰好失明。
//
// 断言里带一条正控制（命中的路由数下界）：枚举为空时 for 循环是"逐条都通过"的，
// 没有它这条门会在路由被改名后静默变成橡皮章。

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hivemtk-user/internal/model"
	dbutil "hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/system/install"

	"github.com/gin-gonic/gin"
)

// minAdminBootstrapRoutes 是"会铸超管"的公开路由条数下界（当前是 init-admin 与
// create-default-admin 两条，两条都是 AuthController 那对互为别名的 handler）。
// 下界不许下调：下调意味着有人把某条入口从装配里摘了却没回来看这条门。
const minAdminBootstrapRoutes = 2

// isAdminBootstrapHandler 判定一个 handler 全名是否会走到"建超管"。
// 按函数名匹配（含 -fm 后缀），因为这三条 handler 里有两个是纯委派，路径前缀并不可靠。
func isAdminBootstrapHandler(handlerName string) bool {
	for _, fn := range []string{
		"controller.(*AuthController).InitAdmin",
		"controller.(*AuthController).CreateDefaultAdmin",
		"controller.(*SystemUserController).CreateDefaultAdmin",
	} {
		if strings.Contains(handlerName, fn) {
			return true
		}
	}
	return false
}

// adminBareEngine 用生产装配入口跑一台引擎（前端静态面指到空临时目录，与 liveRoutes 同一手法）
func adminBareEngine(t *testing.T) *gin.Engine {
	t.Helper()
	t.Setenv("USER_WEB_DIST", t.TempDir())
	database := testutil.NewTestDB(t, &model.SystemUser{}, &model.SystemConfigKV{})
	prev := dbutil.GetDB()
	dbutil.SetTestDB(database)
	t.Cleanup(func() { dbutil.SetTestDB(prev) })

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	Setup(engine, database)
	return engine
}

func TestAdminBootstrapRoutes_BlockedOnceInitialized(t *testing.T) {
	// 先把安装态钉成"已装完"，再装引擎：Setup 里读安装态的分支要跟着这个前提走。
	lockPath := filepath.Join(t.TempDir(), "install.lock")
	t.Setenv("INSTALL_LOCK_PATH", lockPath)
	if err := install.Save(&install.Lock{
		InstallID:     "ins-route-test",
		AdminUsername: "admin",
		Initialized:   true,
		Version:       "test",
	}); err != nil {
		t.Fatalf("写测试 install.lock 失败: %v", err)
	}

	engine := adminBareEngine(t)

	var targets []string
	for _, ri := range engine.Routes() {
		if ri.Method == http.MethodPost && isAdminBootstrapHandler(ri.Handler) {
			targets = append(targets, ri.Path)
		}
	}
	if len(targets) < minAdminBootstrapRoutes {
		t.Fatalf("只枚举到 %d 条会铸超管的路由（下界 %d）：%v —— 装配面变了就回来改这条门，别把它调松",
			len(targets), minAdminBootstrapRoutes, targets)
	}

	// 载荷是"能真的建出一个超管"的形状：用户名合规、密码够强。
	// 若守卫与 service 判定同时失效，这些请求会返回 200 并在库里留下账号。
	payload := `{"username":"zz_attacker","password":"Str0ng!Pass123","email":"zz@attacker.test"}`
	for _, path := range targets {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Fatalf("POST %s 在已初始化的实例上应被匿名拦下 403，实际 %d body=%s", path, w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "禁止重复创建超管") {
			t.Fatalf("POST %s 的 403 要说清是重复初始化被拒，实际 body=%s", path, w.Body.String())
		}
		if strings.Contains(w.Body.String(), "超管创建成功") || strings.Contains(w.Body.String(), `"code":0`) {
			t.Fatalf("POST %s 出现了成功语义，说明请求走到了建号分支：body=%s", path, w.Body.String())
		}
	}
	t.Logf("已核 %d 条公开铸超管路由：%v", len(targets), targets)
}

// TestAdminBootstrapRoutes_GuardWiredAtRegistration 上面那条行为断言打的是"结果"：
// 三道门（路由守卫 / handler 早检 / service 计数）任一道还站着，匿名请求都会拿到 403，
// 所以它单靠自己在"只拆掉路由守卫"这一格里是绿的——那一格必须有一道门看得见装配面。
// 这里逐行读注册代码：凡是用会铸超管的 handler 挂在公开组上的，那一行必须显式带 setupOnly。
func TestAdminBootstrapRoutes_GuardWiredAtRegistration(t *testing.T) {
	src, err := os.ReadFile("admin_routes.go")
	if err != nil {
		t.Fatalf("读注册代码失败: %v", err)
	}
	// 注册代码里引用这两个 handler 的写法是变量.方法（authCtrl.InitAdmin），
	// 不是运行期那个带包名与 -fm 的全名——两套字符串不能混用，否则这条门会恒空。
	createsAdmin := []string{".InitAdmin", ".CreateDefaultAdmin"}
	guarded, naked := 0, 0
	for i, line := range strings.Split(string(src), "\n") {
		if !strings.Contains(line, "public.POST(") {
			continue
		}
		hit := false
		for _, sym := range createsAdmin {
			if strings.Contains(line, sym) {
				hit = true
				break
			}
		}
		if !hit {
			continue
		}
		if strings.Contains(line, "setupOnly") {
			guarded++
			continue
		}
		naked++
		t.Errorf("admin_routes.go:%d 公开挂载铸超管 handler 却没带 SetupOnlyGuard：%s", i+1, strings.TrimSpace(line))
	}
	if guarded < minAdminBootstrapRoutes {
		t.Fatalf("只找到 %d 条带守卫的公开铸超管注册（下界 %d）——装配写法改了（例如换成整组中间件）就要同步改这条门",
			guarded, minAdminBootstrapRoutes)
	}
	if naked != 0 {
		t.Fatalf("有 %d 条公开铸超管注册没带守卫", naked)
	}
}
