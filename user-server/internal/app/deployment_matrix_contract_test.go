package app

// A15（`未完成任务.md` §9）：把 2×2 部署形态矩阵从**文档**变成**可执行断言**。
//
// 依据：docs/architecture/DEPLOYMENT_MODES_EVALUATION_2026-10.md §1.2/§2（四格契约）
// 与 SAAS_FULLCHAIN_AUDIT_2026-10-23.md §6（方案 E：不写成门禁就会被下一个人改回去）。
//
// 为什么必须做成断言：四个格子里有两格（`on+false`、`off+true`）在文档里长期不存在，
// 而它们的行为由**两个正交开关**在 91 + 26 个分支点上分别决定。文档契约与代码之间
// 没有任何编译期或测试期联系 ⇒ 任何人改一处 `if tenant.IsMultiTenant()` 或
// `if config.PlatformEnabled()`，契约就静默变了，且**所有既有用例照绿**。
//
// 本文件只钉"能红"的判据：每格取六条可观测行为，全部走**活体组件**
// （租户中间件 / 隔离 callback / 自助注册服务 / 初始化守卫 / 后台扇出 / 平台地址解析），
// 不 assert 任何"文档里写着"的字符串，也不 assert 本文件自己的辅助函数。
//
// 判据（每格六条，见 DEPLOYMENT_MODES §2）：
//  1. 出站请求：平台轴关 ⇒ 地址恒空串（一个请求都不会发）；开 ⇒ 取配置值，不回落任何域名；
//  2. 租户上下文：off ⇒ 一律绑默认租户 1（带 claim 也不改变）；on ⇒ 无 claim 401、有 claim 用 claim；
//  3. 自助开户：off ⇒ 业务码 4104（注册未开放）；on ⇒ 不再被 4104 拦；
//  4. 初始化态：off ⇒ 未写 install.lock 时打回 INIT_REQUIRED；on ⇒ 恒已初始化（无单机 setup 概念）；
//  5. 后台任务：off ⇒ 单次回调、绑默认租户；on ⇒ 按活跃租户逐个回调，枚举器缺失即 fail-closed；
//  6. 隔离 callback：off ⇒ 直通（SQL 与历史版本等价，无 tenant_id 条件）；on ⇒ 注入租户条件且
//     无租户 ctx 时 fail-closed。
//
// 两个轴**正交**是本矩阵的核心断言：2/3/4/5/6 各条在两列数值完全相同 ——
// 若哪天有人在平台开关里顺手加了租户判定（或反之），这一格就会红。

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"hivemtk-user/internal/config"
	bizerr "hivemtk-user/internal/domain/errors"
	"hivemtk-user/internal/middleware"
	"hivemtk-user/internal/pkg/tenancyidx"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// matrixCell 一格：租户轴 × 平台轴。
type matrixCell struct {
	name             string
	tenantMode       tenant.Mode
	platformEnabled  bool
	platformEnvValue string
}

// matrixCells 四格（顺序与 DEPLOYMENT_MODES §1.2 真值表一致：①②③④）。
func matrixCells() []matrixCell {
	return []matrixCell{
		{"①off_平台关_纯离线单机", tenant.ModeOff, false, "false"},
		{"②off_平台开_单机联平台", tenant.ModeOff, true, "true"},
		{"③on_平台关_SaaS无平台联动", tenant.ModeOn, false, "false"},
		{"④on_平台开_云端SaaS全功能", tenant.ModeOn, true, "true"},
	}
}

// enterCell 把进程切到该格（用完还原：租户形态与平台配置都是进程级全局）。
func enterCell(t *testing.T, c matrixCell) {
	t.Helper()
	prevMode := tenant.CurrentMode()
	tenant.SetMode(c.tenantMode)
	t.Cleanup(func() { tenant.SetMode(prevMode) })

	prevCfg := config.PlatformCfg
	config.PlatformCfg = nil
	t.Cleanup(func() { config.PlatformCfg = prevCfg })

	// 平台地址**逐格显式配一个真实形态的地址**：这样"关"那两格断言空串才有意义
	// （空串必须来自开关，而不是"碰巧没配"）。
	t.Setenv("PLATFORM_ENABLED", c.platformEnvValue)
	t.Setenv("PLATFORM_API_HOST", "https://platform.example.com")
	t.Setenv("PLATFORM_API_URL", "")
}

// TestDeploymentMatrixContract 2×2 矩阵契约（六条行为 × 四格 = 24 条断言）。
func TestDeploymentMatrixContract(t *testing.T) {
	for _, c := range matrixCells() {
		c := c
		t.Run(c.name, func(t *testing.T) {
			enterCell(t, c)

			// ── 1. 出站请求 ───────────────────────────────────────────────
			t.Run("1_出站请求", func(t *testing.T) {
				got := config.PlatformURL()
				if c.platformEnabled {
					if got != "https://platform.example.com" {
						t.Fatalf("平台轴开：地址应取配置值，实际 %q", got)
					}
					return
				}
				if got != "" {
					t.Fatalf("平台轴关：地址必须为空串（零出站），实际 %q——开关没拦住出站", got)
				}
			})

			// ── 2. 租户上下文 ─────────────────────────────────────────────
			t.Run("2_租户上下文", func(t *testing.T) {
				code, bound := probeTenantBindingWithClaim(t, 0)
				if c.tenantMode == tenant.ModeOff {
					if code != 200 || bound != tenant.DefaultTenantID {
						t.Fatalf("off：应 200 且绑默认租户 %d，实际 code=%d tenant=%d", tenant.DefaultTenantID, code, bound)
					}
					// off 不看 claim：单租户形态下历史令牌（无 claim）与"带别的租户"的令牌
					// 都必须绑到默认租户——否则离线单机会因为一个奇怪的令牌就查不到数据。
					if code2, bound2 := probeTenantBindingWithClaim(t, 7); code2 != 200 || bound2 != tenant.DefaultTenantID {
						t.Fatalf("off：带 claim=7 也该绑默认租户 %d，实际 code=%d tenant=%d",
							tenant.DefaultTenantID, code2, bound2)
					}
					return
				}
				if code != 401 {
					t.Fatalf("on：无 claim 必须 401 fail-closed，实际 %d", code)
				}
				if code2, bound2 := probeTenantBindingWithClaim(t, 7); code2 != 200 || bound2 != 7 {
					t.Fatalf("on：带 claim=7 应 200 且绑 7，实际 code=%d tenant=%d", code2, bound2)
				}
			})

			// ── 3. 自助开户 ───────────────────────────────────────────────
			t.Run("3_自助开户", func(t *testing.T) {
				_, err := service.NewTenantService().RegisterTenant(
					context.Background(), service.RegisterTenantRequest{}, "ip", "ua")
				if err == nil {
					t.Fatalf("空表单不该注册成功（这条只用来区分闸门与校验，不该通过）")
				}
				var be *bizerr.BizError
				code := 0
				if errors.As(err, &be) {
					code = be.Code
				}
				if c.tenantMode == tenant.ModeOff {
					if code != bizerr.CodeTenantRegisterClosed {
						t.Fatalf("off：自助开户必须被业务码 %d 拦下，实际 code=%d err=%v",
							bizerr.CodeTenantRegisterClosed, code, err)
					}
					return
				}
				if code == bizerr.CodeTenantRegisterClosed {
					t.Fatalf("on：自助开户不该再被 4104 拦下（那是离线的语义），实际 %v", err)
				}
			})

			// ── 4. 初始化态 ───────────────────────────────────────────────
			t.Run("4_初始化态", func(t *testing.T) {
				body := probeInitGuard(t)
				if c.tenantMode == tenant.ModeOff {
					if body != "INIT_REQUIRED" {
						t.Fatalf("off：未写 install.lock 时应打回 INIT_REQUIRED，实际 %q", body)
					}
					return
				}
				if body != "ok" {
					t.Fatalf("on：恒已初始化，不该打回 setup（实际 %q）——SaaS 实例会把前端全推进 /setup", body)
				}
			})

			// ── 5. 后台任务扇出 ───────────────────────────────────────────
			t.Run("5_后台任务扇出", func(t *testing.T) {
				seen := probeFanout(t)
				if c.tenantMode == tenant.ModeOff {
					if len(seen) != 1 || seen[0] != tenant.DefaultTenantID {
						t.Fatalf("off：扇出应退化为单次默认租户 %d，实际 %v", tenant.DefaultTenantID, seen)
					}
					return
				}
				if len(seen) != 2 || seen[0] != 5 || seen[1] != 6 {
					t.Fatalf("on：应按活跃租户逐个回调（5,6），实际 %v", seen)
				}
			})

			// ── 6. 隔离 callback ─────────────────────────────────────────
			t.Run("6_隔离callback", func(t *testing.T) {
				probeIsolationCallback(t, c)
			})
		})
	}
}

// ── 探针实现（每个都用活体组件，不做字符串比对）──────────────────────────

func probeTenantBindingWithClaim(t *testing.T, claim uint) (int, uint) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	if claim != 0 {
		// 模拟 JWTAuthMiddleware：把 claim 写进 gin ctx（TenantMiddleware 消费它）
		r.Use(func(c *gin.Context) { c.Set(middleware.TenantContextKey, claim); c.Next() })
	}
	r.Use(middleware.TenantMiddleware())
	r.GET("/probe", func(c *gin.Context) {
		id, _ := tenant.FromContext(c.Request.Context())
		c.JSON(200, gin.H{"tenant": id})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		return w.Code, 0
	}
	var out struct {
		Tenant uint `json:"tenant"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("解析探针响应失败: %v body=%s", err, w.Body.String())
	}
	return 200, out.Tenant
}

// probeInitGuard 走一遍 InitGuard：返回 INIT_REQUIRED 或 "ok"。
func probeInitGuard(t *testing.T) string {
	t.Helper()
	// 指向一个不存在的 install.lock：这台实例"还没装完"
	t.Setenv("INSTALL_LOCK_PATH", t.TempDir()+"/install.lock")
	middleware.InitInstallStatus()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.InitGuard())
	r.GET("/api/business", func(c *gin.Context) { c.String(200, "ok") })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/business", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("InitGuard 探针状态码异常: %d body=%s", w.Code, w.Body.String())
	}
	var out struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err == nil && out.Code != "" {
		return out.Code
	}
	return w.Body.String()
}

// probeFanout 记录 Fanout 实际回调到的租户序列。
func probeFanout(t *testing.T) []uint {
	t.Helper()
	tenant.ResetRegistryForTest()
	if tenant.CurrentMode() == tenant.ModeOn {
		tenant.SetActiveTenantLoader(func(ctx context.Context) ([]uint, error) {
			return []uint{5, 6}, nil
		})
	}
	t.Cleanup(tenant.ResetRegistryForTest)

	var seen []uint
	_, _ = tenant.Fanout(context.Background(), nil, func(ctx context.Context, id uint) (int64, error) {
		// 回调里的 ctx 必须真的绑了这个租户（业务 SQL 靠它拿隔离）
		if bound, ok := tenant.FromContext(ctx); !ok || bound != id {
			t.Errorf("扇出回调 ctx 未绑租户 %d（实际 %d, ok=%v）", id, bound, ok)
		}
		seen = append(seen, id)
		return 0, nil
	})
	return seen
}

// matrixProbe 隔离探针表：只用本地测试模型，不碰任何业务表。
type matrixProbe struct {
	ID       uint `gorm:"primaryKey"`
	TenantID uint
	Name     string
}

func (matrixProbe) TableName() string { return "matrix_contract_probes" }

// probeIsolationCallback 真库上验"off 直通 / on 注入 + fail-closed"。
func probeIsolationCallback(t *testing.T, c matrixCell) {
	t.Helper()
	db := testutil.NewTestDBOrSkip(t, &matrixProbe{})
	if db == nil {
		return
	}
	t.Cleanup(func() { _ = db.Migrator().DropTable(&matrixProbe{}) })

	// 登记表（哪些表有 tenant_id 列）必须在插数据前建好：callback 的判据来自库内省。
	tenancyidx.ResetRegistryForTest()
	if err := tenancyidx.InitTenantRegistry(db); err != nil {
		t.Fatalf("初始化租户登记表失败: %v", err)
	}
	t.Cleanup(tenancyidx.ResetRegistryForTest)
	tenancyidx.RegisterTenantCallbacks(db)

	ctx5 := tenant.WithTenantID(context.Background(), 5)
	ctx6 := tenant.WithTenantID(context.Background(), 6)

	if c.tenantMode == tenant.ModeOff {
		// off：callback 直通 —— 无租户 ctx 也能写能读，且读不加租户条件（与历史 SQL 等价）。
		if err := db.WithContext(context.Background()).Create(&matrixProbe{TenantID: 5, Name: "a"}).Error; err != nil {
			t.Fatalf("off：无租户 ctx 的写入不该被拒绝（隔离 callback 应直通）: %v", err)
		}
		if err := db.WithContext(context.Background()).Create(&matrixProbe{TenantID: 6, Name: "b"}).Error; err != nil {
			t.Fatalf("off：第二次写入失败: %v", err)
		}
		var all []matrixProbe
		if err := db.WithContext(ctx5).Find(&all).Error; err != nil {
			t.Fatalf("off：查询失败: %v", err)
		}
		if len(all) != 2 {
			t.Fatalf("off：查询不该带租户条件（两行都该看得见），实际 %d 行——off 模式被注入了隔离？", len(all))
		}
		return
	}

	// on：无租户 ctx 必须 fail-closed
	var zero matrixProbe
	err := db.WithContext(context.Background()).Create(&zero).Error
	if err == nil {
		t.Fatal("on：无租户 ctx 的写入必须被隔离 callback 拒绝")
	}
	if rows := db.WithContext(context.Background()).Find(&[]matrixProbe{}); rows.Error == nil {
		t.Fatal("on：无租户 ctx 的查询必须被拒绝")
	}

	// on：写入自动带上 ctx 租户，跨租户不可见
	if err := db.WithContext(ctx5).Create(&matrixProbe{Name: "a"}).Error; err != nil {
		t.Fatalf("on：租户 5 写入失败: %v", err)
	}
	var seenBy5, seenBy6 []matrixProbe
	if err := db.WithContext(ctx5).Find(&seenBy5).Error; err != nil {
		t.Fatalf("on：租户 5 查询失败: %v", err)
	}
	if len(seenBy5) != 1 || seenBy5[0].TenantID != 5 {
		t.Fatalf("on：租户 5 应看到自己那 1 行且 tenant_id=5，实际 %+v", seenBy5)
	}
	if err := db.WithContext(ctx6).Find(&seenBy6).Error; err != nil {
		t.Fatalf("on：租户 6 查询失败: %v", err)
	}
	if len(seenBy6) != 0 {
		t.Fatalf("on：租户 6 不该看到租户 5 的行，实际 %+v", seenBy6)
	}
}
