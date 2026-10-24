# Implementation Plan: 模板/技能市场（F2 · 最小可用版）

**Feature**: `013-template-skill-market` ｜ **Created**: 2026-10-24 ｜ **Spec**: `./spec.md`

## 技术路线（一句话）

市场是既有模板/技能之上的**只读目录层**：`repository` 包放一个进程内注册的目录数据源
（接口 + 内存假仓储，不连 DB），`service` 包放查询/过滤/分类聚合与 fail-closed 哨兵错误；
controller 与 router 只出接线建议（写进 delivery.md），前端新增一个独立市场浏览页
（卡片网格 + 类型/分类/关键词过滤）。

## 分层落点

| 层 | 是否改动 | 说明 |
|----|---------|------|
| Model | ❌ 不改 | 不建表、不动 `module_tables.go`；契约值对象放 repository 包（数据源侧） |
| Repository | ✅ 新增 `template_market_repo.go` + 测试 | 接口 `TemplateMarketRepository` + `MemoryTemplateMarketRepository` + 种子目录 |
| Service | ✅ 新增 `template_market.go` + 测试 | `TemplateMarketService`：List/Get/Categories + `ErrMarketEntryNotFound` |
| Controller | ⭕ 建议不实施 | 接线建议 + 完整代码见 delivery.md（避免与并发 lane 抢共享 router） |
| Router | ⭕ 建议不实施 | 同上；挂载点见 delivery.md |
| 前端 API | ✅ 新增 `src/api/templateMarket.js` | 列表/分类/详情 + 展示常量 |
| 前端页面 | ✅ 新增 `src/views/templateMarket/Market.vue` | 独立页面，卡片网格 + 过滤 |
| 前端路由 | ✅ 新增 `src/router/modules/templateMarket.js` | 懒装配模块文件；登记点见 delivery.md |
| 前端测试 | ✅ 新增 `tests/unit/templateMarket_render.test.js` | 3 例渲染/过滤/空态 |

## 关键设计决策（含被否决的方案）

### D1 目录数据源：进程内注册的只读目录（否决：建 `market_catalog` 表）

- 采纳理由：写侧（发布/审核/支付）未立项，建表只会产生「一份事实抄两份且无同步路径」的
  裂脑；目录条目 `source` 指向既有实现（HTML 文件 / sop_templates / MCP 守卫…），
  这些事实源分布在文件系统与多张表里，单表无法权威化它们。P2 先立契约，量级到写侧时再迁真仓储。
- 被否决方案：`market_catalog` 表 + CRUD。代价：迁移 + 仓储 + 前端两页 + 契约门，
  收益仅是「结构化查询」，而本轮只有读。
- 已知代价：目录内容靠发版更新（编译期种子）；进程内多实例不共享。对只读橱窗可接受。

### D2 过滤放 service 而不是仓储（否决：仓储内过滤）

- 采纳理由：目录数据源（仓储）只做「取数」两件事（All/Get），过滤/分类是**查询语义**，
  属业务层；放仓储会让假仓储背上真仓储的业务语义，接真仓储时返工。
- 测试口径：过滤/未知 ID 的用例全部落在 service 层；仓储层只测「种子契约完整性 + 取数副本语义」。

### D3 未知 ID fail-closed：哨兵 `ErrMarketEntryNotFound`（否决：返回 nil, nil 或 500）

- 采纳理由：`Get` 的调用方（未来 controller）必须能区分「查无」与「真故障」；
  nil, nil 会逼每个调用方自己判，漏一个就是 500。哨兵错误 + `errors.Is` 是单一判据。
- 空 ID 与乱敲 ID 同等对待：都算「查无」，不单独造参数错误——详情页拿空 ID 本来就是查无。

### D4 目录顺序：种子字面量顺序即默认序（否决：按 ID 字典序重排）

- 采纳理由：模板在前、技能在后、同类相邻，阅读顺序即产品意图；字典序会把
  `card-*` 打散到 `integration-erp` 中间。稳定性由测试钉住（两次 List 逐条一致）。

### D5 前端不碰共享 router/index.js 与 i18n locales（否决：直接改 router + 9 语种文案）

- 采纳理由：`router/index.js` 是并发 lane 高风险文件（规则 2）；i18n 9 语种 JSON 当前均被他人在途
  修改（`git status` 已见 `M user-web/src/i18n/locales/*.json`）。新增独立页面 + 独立路由模块文件，
  登记点写进 delivery.md，由接线方合并。文案暂用中文硬编码，对齐既有 `moduleMarket/Market.vue`
  先例（该页同样硬编码中文，未走 t()）。
- 被否决方案：为文案加 9 语种 key。代价是并发改 9 个共享文件，冲突风险 > 收益（P2 最小可用版）。

## 语义细则（实现判据）

- **kind/category** 精确匹配；**keyword** 在 `name + " " + summary + " " + join(tags)` 小写化后子串匹配。
- **Categories**：`map` 去重 → `sort.Strings` 升序；空目录返回空切片（非 nil）。
- **Get**：先 `strings.TrimSpace(id)`，空则直接 `ErrMarketEntryNotFound`；仓储 `(nil, nil)` 同样转哨兵。
- **仓储 All**：返回 `copy`（新切片），防调用方原地改写目录；`Get` 返回条目副本。
- **种子契约**：ID 唯一、kind∈{template,skill}、availability∈{builtin,needs_module,coming_soon}、
  五个元数据字段非空，测试钉住。

## 实施阶段

| 阶段 | 内容 | 产物 |
|---|---|---|
| P1 | 仓储接口 + 内存实现 + 种子目录 + 契约测试 | `internal/repository/template_market_repo{,_test}.go` |
| P2 | service 查询/过滤/分类 + 哨兵错误 + 表驱动测试 | `internal/service/template_market{,_test}.go` |
| P3 | 前端 API + 市场浏览页 + 路由模块文件 + 渲染测试 | `src/api/templateMarket.js`、`src/views/templateMarket/Market.vue`、`src/router/modules/templateMarket.js`、`tests/unit/templateMarket_render.test.js` |
| P4 | 验证 + 四件套 + 接线建议 | `go build/vet/test`、`npm run build`、`specs/013/*.md` |

## 验证口径

| 改动范围 | 命令 |
|---|---|
| Go 构建 | `cd user-server && go build ./internal/service/ ./internal/repository/`（注：全仓 `./...` 受他人在途文件影响，见 delivery.md） |
| Go 测试 | `go test ./internal/service/ -run 'TemplateMarketService' -count=1`；`go test ./internal/repository/ -run 'Market|MemoryTemplateMarket' -count=1` |
| Go 风格 | `gofmt -l`（4 个新文件）；`go vet ./internal/service/ ./internal/repository/` |
| 前端 | `cd user-web && npx vitest run tests/unit/templateMarket_render.test.js`；`npm run build` |

## 风险与回滚

| 风险 | 处置 |
|---|---|
| 全仓构建因并发 lane 的 `observability_dashboard.go` 变红 | 只对**本切片 4 个文件**负责；验证用 `go build ./...` 快照如实记录他人在途冲突，不用 `-overlay` 之外的手段篡改他人文件 |
| 目录种子写错（source 指向不存在实现） | 种子契约测试钉住「Source 非空」；code review 逐条核对 source 是否真实存在 |
| 前端硬编码中文触发 i18n 门 | 硬编码中文与既有 market 页同口径（CN 类属清单类，非硬门禁）；若 CI 红再按接线方排期补 9 语种 key |
| 路由未登记导致页面不可达 | 本切片不实施接线（规则 7）；`src/router/modules/templateMarket.js` 与登记点已就绪，接线方按 delivery.md 两处合并即通 |
