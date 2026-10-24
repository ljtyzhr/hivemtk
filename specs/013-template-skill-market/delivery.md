# Delivery Report: 模板/技能市场（F2 · 最小可用版）

**Feature**: `013-template-skill-market` ｜ **交付日期**: 2026-10-24 ｜ **Spec/Plan/Tasks**: 同目录

## 一句话

F2 从零落地「最小可用市场」：后端新增**进程内注册的只读目录数据源**（repository 接口 +
内存假仓储，不连 DB）+ **查询服务**（列表/分类/详情、未知 ID fail-closed），前端新增独立
市场浏览页（卡片网格 + 类型/分类/关键词过滤）。**不含支付/发布/审核闭环**；controller/router
按规则 7 只出接线建议，不实施。

## 交付物清单

| 类型 | 文件 | 规模 | 说明 |
|---|---|---|---|
| 新增 | `user-server/internal/repository/template_market_repo.go` | ~180 行 | 契约常量 + `TemplateMarketEntry` + 接口 + 内存仓储 + 13 条种子目录 |
| 新增 | `user-server/internal/repository/template_market_repo_test.go` | ~110 行 | 种子契约完整性 / All 副本 / Get 命中与未命中 |
| 新增 | `user-server/internal/service/template_market.go` | ~140 行 | `TemplateMarketService`（List/Get/Categories）+ `ErrMarketEntryNotFound` |
| 新增 | `user-server/internal/service/template_market_test.go` | ~210 行 | 表驱动过滤 8 例 / 稳定序 / Get 命中 / fail-closed 3 例 / 分类 / 错误透传 |
| 新增 | `user-web/src/api/templateMarket.js` | ~40 行 | list/categories/detail + kind/availability 展示常量 |
| 新增 | `user-web/src/views/templateMarket/Market.vue` | ~270 行 | 卡片网格 + 过滤 + 详情抽屉 + 空态 |
| 新增 | `user-web/src/router/modules/templateMarket.js` | ~20 行 | 懒装配路由模块（登记点见下） |
| 新增 | `user-web/tests/unit/templateMarket_render.test.js` | ~110 行 | 渲染 / 关键词过滤 / 空态 3 例 |
| 文档 | `specs/013-template-skill-market/{spec,plan,tasks,delivery}.md` | — | spec-kit 四件套 |

## 验证读数（本机实测，2026-10-24）

| 命令 | 结果 |
|---|---|
| `gofmt -l`（4 个新增 Go 文件） | ✅ 干净（无输出） |
| `go build ./internal/repository/` | ✅ 通过 |
| `go build ./internal/service/` | ⚠️ 受他人在途文件阻塞（见下「并发 lane 冲突」），用 `-overlay` 置空他人文件后 ✅ 通过 |
| `go build ./...` | ❌ 红在 `internal/service/observability_dashboard.go`（**他人在途文件**，非本切片），错误：`FunnelStage`/`DashboardSnapshot` 与 `dashboard_sse_stats.go` 重复声明 |
| `go vet ./internal/repository/` | ✅ 通过 |
| `go vet ./internal/service/`（`-overlay` 置空他人文件） | ✅ 通过 |
| `go test ./internal/repository/ -run 'Market\|MemoryTemplateMarket' -count=1` | ✅ 全绿（3 个测试函数） |
| `go test ./internal/service/ -run 'TemplateMarketService' -count=1`（`-overlay`） | ✅ 全绿（6 个测试函数） |
| `npx vitest run tests/unit/templateMarket_render.test.js` | ✅ 3/3 通过 |
| `npm run build` | ✅ 通过（`✓ built in 28.48s`；chunk size 与 locale 动态导入告警均为既有告警） |

### 并发 lane 冲突（如实记录，非本切片缺陷）

`internal/service` 与 `internal/repository` 是并发 lane 的施工热点。开发期间观测到的**他人在途
未跟踪文件（`??`，本 lane 一概未碰）**：

- `user-server/internal/service/observability_dashboard.go` + `_test.go`（D2 可观测看板）
  与已跟踪的 `dashboard_sse_stats.go` 重复声明 `FunnelStage`/`DashboardSnapshot`；
- `user-server/internal/repository/observability_dashboard.go` 早期与 `dashboard_stats.go`
  重复声明 `FunnelStageRow`，**该 lane 在我开发期间自行修复**（改名 `ObsDashFunnelStageRow`），
  repository 包现已真实可编译；
- 另有 `service/ltc_pipeline*.go`、`service/approval_auto_send_bridge*.go` 等其他 lane 的新文件。

按铁律 1/2，我**未改动任何他人文件**。验证方式：`go build -overlay`（把上述他人未跟踪文件
映射为空 `package service` 文件）证明本切片代码真实可编译、测试真实通过；repository 包现已
无需 overlay 直接编译通过。`gofmt`/`go vet` 只针对本切片文件与包。
**接线方合并后应重跑全仓 `go build ./...` 与全量测试**——当前红与 013 无关。

## 设计要点（为什么这么做）

1. **目录层而非内容层**：`source` 字段只指向既有实现（HTML 文件路径 / `sop_templates` /
   `mcp_guard.go`…），不复制、不重写内容语义。`internal/template` 与 `reach/card/template`
   一行未改。
2. **进程内注册、不建表**：写侧（发布/审核/支付）未立项，建 `market_catalog` 表 = 一份事实抄两份
   且无同步路径（E1/模块市场同款论证）。P2 先立契约，量级到写侧再迁真仓储。
3. **过滤在 service、取数在仓储**：仓储接口只有 `All`/`Get` 两个只读方法；过滤/分类是查询语义，
   放业务层。假仓储不背真仓储的查询债。
4. **fail-closed 用哨兵错误**：`ErrMarketEntryNotFound` + `errors.Is` 是「查无」的单一判据；
   空 ID、空白 ID、乱敲 ID 同口径，不让调用方各自判 nil。
5. **前端只加文件、不碰共享**：`router/index.js` 与 9 个 i18n locale JSON 均被并发 lane 在途修改；
   本切片新增独立页面 + 独立路由模块文件，登记点留给接线方两处合并。

## 未实现 / 留待后续的决策点

1. **发布/审核/支付闭环**：本轮刻意不做（计划文档口径 P2 只交付「只读浏览 + 元数据 + 分类」）。
   发布闭环立项时需新增写侧接口 + 仓储写方法 +（可能）一张目录表。
2. **新表需求（只记录，不实施）**：若写侧立项，建议表 `market_catalog_entries`
   （归属 `internal/model/`，GORM AutoMigrate 经既有 `RegisterExtraModels` 登记），
   列对齐本 spec §2.1 契约 + `enabled_at/disabled_at` + `tenant_id`（多租户兜底）。
   **本轮未建表、未改 `module_tables.go`**。
3. **目录内容更新方式**：当前目录是编译期种子，改目录要发版。若运营要热更，下一轮可选
   「参数中心 JSON 目录」（E1 先例）或平台端下发（模块市场先例），接口层不用改。
4. **详情页/独立路由**：详情暂用列表页内抽屉；独立详情路由与深链在 router 登记后按需加。
5. **i18n**：页面文案中文硬编码（对齐 `moduleMarket/Market.vue` 先例）；9 语种 key 留给
   接线方统一排期（locale JSON 在途被改，本 lane 不碰）。

## 建议接线点（规则 7：只建议不实施）

### 1) 后端 controller（建议新增 `internal/controller/template_market.go`，完整代码）

```go
package controller

import (
    "errors"
    "net/http"
    "strconv"

    "github.com/gin-gonic/gin"
    bizerr "hivemtk-user/internal/domain/errors"
    "hivemtk-user/internal/pkg/utils/response"
    "hivemtk-user/internal/service"
)

// TemplateMarketController 模板/技能市场（F2 只读橱窗）。
type TemplateMarketController struct{ svc *service.TemplateMarketService }

func NewTemplateMarketController() *TemplateMarketController {
    return &TemplateMarketController{svc: service.NewTemplateMarketService()}
}

// List GET /api/template-market/list（public 档，对齐 module-market/list）
func (c *TemplateMarketController) List(ctx *gin.Context) {
    page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
    pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "50"))
    if pageSize < 1 || pageSize > 200 { pageSize = 50 }
    list, err := c.svc.List(ctx.Request.Context(), service.MarketQuery{
        Kind:     ctx.Query("kind"),
        Category: ctx.Query("category"),
        Keyword:  ctx.Query("keyword"),
    })
    if err != nil { response.ErrorWithBusinessCode(ctx, bizerr.CodeInternal, err.Error(), gin.H{}); return }
    start := (page - 1) * pageSize
    if start < 0 { start = 0 }
    end := start + pageSize
    if end > len(list) { end = len(list) }
    if start > len(list) { start = len(list) }
    response.Success(ctx, gin.H{"list": list[start:end], "total": len(list)}, "ok")
}

// Categories GET /api/template-market/categories（public 档）
func (c *TemplateMarketController) Categories(ctx *gin.Context) {
    cats, err := c.svc.Categories(ctx.Request.Context())
    if err != nil { response.ErrorWithBusinessCode(ctx, bizerr.CodeInternal, err.Error(), gin.H{}); return }
    response.Success(ctx, cats, "ok")
}

// Detail GET /api/template-market/detail/:id（public 档；未知 ID → 404 业务码）
func (c *TemplateMarketController) Detail(ctx *gin.Context) {
    item, err := c.svc.Get(ctx.Request.Context(), ctx.Param("id"))
    if errors.Is(err, service.ErrMarketEntryNotFound) {
        response.ErrorWithBusinessCode(ctx, bizerr.CodeNotFound, "模板/技能不存在", gin.H{})
        return
    }
    if err != nil { response.ErrorWithBusinessCode(ctx, bizerr.CodeInternal, err.Error(), gin.H{}); return }
    response.Success(ctx, item, "ok")
}
```

### 2) 后端 router（建议在 `internal/router/module_routes.go` 的 `setupModuleRoutes` 或新函数内挂 public 档）

```go
tmCtrl := controller.NewTemplateMarketController()
if public != nil {
    public.GET("/template-market/list", tmCtrl.List)
    public.GET("/template-market/categories", tmCtrl.Categories)
    public.GET("/template-market/detail/:id", tmCtrl.Detail)
}
```

三档理由：list/categories/detail 都是**公开橱窗**（无个人数据、无租户上下文，对齐
`module-market/list` 的 OptionalAuth 语义，但本目录不含 owned 概念，可不上 OptionalAuth）。

### 3) 前端 router 登记（`user-web/src/router/index.js`，两处，勿整文件覆盖）

- `moduleNames` 数组追加 `"templateMarket"`；
- `pathToModule` 增加 `'template-market': 'templateMarket',`。

路由模块文件 `src/router/modules/templateMarket.js` 已就绪，登记后深链 `/#/template-market` 即通。

### 4) 侧栏菜单（`internal/model/menu.go`，建议在 system 顶级下与 moduleCenterGroup 平级加一组）

```go
{
    Key: "templateMarketGroup", Title: "模板/技能", Icon: "Shop",
    Roles: []string{"admin", "manager"},
    Children: []SystemMenuNode{
        {Key: "templateMarket", Title: "模板/技能市场", Icon: "Shop", Path: "/template-market"},
    },
},
```

> 注意：`menu.go` 属接线方职责；本 lane 未改（与 router 同理）。

## 下一轮建议

写侧闭环（发布/审核/支付）→ 目录热更（参数中心 JSON 或平台下发）→ 独立详情页 + 9 语种 i18n →
（若量级需要）`market_catalog_entries` 表 + 真仓储。承接计划文档 Wave 4 的 F1/F2 收口。
