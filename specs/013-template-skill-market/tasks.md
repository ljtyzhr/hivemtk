# Tasks: 模板/技能市场（F2 · 最小可用版）

**Feature**: `013-template-skill-market` ｜ 勾选即以「已验证」为准（无验证不算完成）

## Phase 1 目录数据源（`internal/repository/template_market_repo.go`，新建）

- [x] T001 定义契约常量：`EntryKindTemplate/EntryKindSkill`、`AvailabilityBuiltin/NeedsModule/ComingSoon`
- [x] T002 定义契约值对象 `TemplateMarketEntry`（id/kind/name/category/summary/source/availability/tags）
- [x] T003 定义只读接口 `TemplateMarketRepository`（All / Get；Get 未命中返回 nil,nil）
- [x] T004 实现 `MemoryTemplateMarketRepository` + `New...` / `New...With`（测试注入）；All 返回副本
- [x] T005 编写种子目录 `marketCatalogSeed`（9 模板 + 4 技能，source 逐条指向真实实现）
- [x] T006 测试：种子契约完整性（ID 唯一/值域合法/字段非空/双 kind 覆盖）+ All 副本语义 + Get 命中与未命中

## Phase 2 查询服务（`internal/service/template_market.go`，新建）

- [x] T007 定义哨兵 `ErrMarketEntryNotFound` 与 API DTO `MarketItem` / `MarketQuery`
- [x] T008 实现 `List`（kind → category → keyword 过滤，目录稳定序）
- [x] T009 实现 `Categories`（去重 + 升序）
- [x] T010 实现 `Get`（TrimSpace 空 ID → 哨兵；仓储 nil → 哨兵；命中 → DTO）
- [x] T011 测试：表驱动过滤（全量/kind/category/keyword 大小写/tags 命中/叠加/空结果）、
  稳定序、Get 命中、未知/空/空白 ID fail-closed、仓储错误透传

## Phase 3 前端（独立文件，不碰共享 router/i18n）

- [x] T012 新增 `src/api/templateMarket.js`（list/categories/detail + 展示常量）
- [x] T013 新增 `src/views/templateMarket/Market.vue`（卡片网格 + 类型/分类/关键词过滤 + 详情抽屉 + 空态）
- [x] T014 新增 `src/router/modules/templateMarket.js`（懒装配模块文件，登记点见 delivery.md）
- [x] T015 测试 `tests/unit/templateMarket_render.test.js`：渲染/关键词过滤/空态 3 例

## Phase 4 验证与交付

- [x] T016 后端：`gofmt -l` 干净、`go build`（service/repository，含他人在途冲突说明）、`go vet`、
  service 与 repository 定向 `go test`
- [x] T017 前端：`npx vitest run tests/unit/templateMarket_render.test.js` 全绿、`npm run build` 通过
- [x] T018 四件套：`specs/013-template-skill-market/{spec,plan,tasks,delivery}.md`
- [x] T019 delivery.md 写清：改动文件、测试命令与结果、未实现/留待后续决策点、建议接线点（controller 代码 + router 登记 + menu 节点）
