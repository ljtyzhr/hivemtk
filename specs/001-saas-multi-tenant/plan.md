# Implementation Plan: SaaS 多租户与离线单机双形态一体化

**Branch**: `001-saas-multi-tenant` | **Date**: 2026-10-17 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/001-saas-multi-tenant/spec.md`

## Summary

一套代码支撑两种部署形态：SaaS 多租户（自助开户、租户行级隔离、LLM 双轨）与离线单机（默认租户、行为等价现状）。核心路径：`TENANT_MODE` 环境开关 → 租户上下文（ctx）贯通入口/后台 → GORM 全局 callback 按"租户表登记表"自动注入隔离条件 → 业务包原生 SQL 历史债务清偿为 GORM API 并以 CI 门禁永久防回归 → 后台任务租户扇出 → 各外部入口按权威解析链定位租户 → 245 表分批加列 + 唯一索引复合化迁移 → user-web 注册页与管理界面适配。

## Technical Context

**Language/Version**: Go 1.25（user-server，五层架构 Router→Controller→Service→Repository→Model）；前端 Vue 3.5 + Element Plus 2.9 + Pinia 3（user-web）；embed-sdk 原生 JS 不直连业务 API

**Primary Dependencies**: Gin + GORM（数据访问唯一通道）+ go-redis v9；推理栈 llama.cpp/TEI（OpenAI 兼容，本期不改动推理栈本身）

**Storage**: PostgreSQL 15 + pgvector（共享库，全部租户同库行级隔离）；Redis 7（键空间租户分域）；本地文件 uploads（目录租户分域）

**Testing**: Go 原生 test（testdb 集成测试基建已具备，`POSTGRES_TEST_*` 环境变量）；前端 vitest；提交门禁 `make audit/lint/vet/test-go`

**Target Platform**: Docker Compose 双形态部署（离线：现有 compose 不变；SaaS：增加租户模式 env）

**Project Type**: web-service（B 端 SaaS + 可私有化交付）

**Performance Goals**: 隔离注入后单查询开销可忽略（索引级）；租户表查询必须命中 `(tenant_id, ...)` 复合索引

**Constraints**: 数据零出域默认不变；离线模式行为零回归（存量测试全绿）；业务代码零手工隔离逻辑；fail-closed（上下文缺租户即拒绝）

**Scale/Scope**: 245 张 AutoMigrate 表、164 个 model 文件、74+ repository、20+ 后台任务、9 个外部入口类别（JWT/webhook/bridge/访客/轮询/回调/事件/WS/系统）

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| 宪法条款 | 状态 | 说明 |
|---------|------|------|
| 五层架构（Router→Controller→Service→Repository→Model） | ✅ | 新增 tenant 域按四层齐全落地；隔离机制在 middleware（入口）与 repository 层（callback 装配）实施，不产生跨层调用 |
| 禁止内联闭包 | ✅ | 租户中间件为独立 gin.HandlerFunc 文件 |
| 禁止子 Agent 并行 | ✅ | 全流程主 Agent 串行执行 |
| API 响应契约（code/data/message + 双通道错误码） | ✅ | 新端点沿用 response.Success/ErrorWithBusinessCode |
| 数据零出域默认 | ✅ | 平台池 LLM 仅 SaaS 模式预置，离线模式不新增任何外呼 |
| 直推双远端、不走 PR | ✅ | 沿用 |
| spec-kit 单一交付入口 | ✅ | 本计划即 SDD plan 阶段产物 |

Phase 1 设计后复检：隔离机制落点为 `internal/pkg/db`（callback 注册）+ `internal/middleware`（租户注入）+ `internal/repository`（既有 dbFromCtx 通路），均为既有层的自然延伸，**无新增跨层违规**。GORM-only 清偿会触碰 repository 层内既有 Exec/Raw 写法，属于层内改造，不违宪。

## Project Structure

### Documentation (this feature)

```text
specs/001-saas-multi-tenant/
├── plan.md              # 本文件
├── research.md          # Phase 0：架构决策记录（含备选方案否决理由）
├── data-model.md        # Phase 1：租户数据模型与表登记分类
├── quickstart.md        # Phase 1：双形态端到端验证指南
├── contracts/           # Phase 1：新增/变更 API 契约
│   └── api.md
└── tasks.md             # Phase 2（speckit-tasks 产出）
```

### Source Code (repository root)

```text
user-server/
├── cmd/api/main.go                          # 装配：TENANT_MODE 解析、callback 注册、TenantRunner
├── internal/
│   ├── pkg/db/                              # [改] GORM callback 注册（隔离执行者）+ 租户表登记表
│   ├── pkg/tenant/                          # [新] 租户上下文（ctx 读写、SystemTenant、RawBypass 移除后的唯一豁免口）
│   ├── middleware/tenant.go                 # [新] JWT tenant claim → ctx；off 模式注入默认租户
│   ├── controller/tenant.go                 # [新] 注册/租户信息端点
│   ├── service/tenant.go                    # [新] 开户事务、租户解析链入口
│   ├── repository/tenant.go                 # [新] tenants 表数据访问
│   ├── model/tenant.go                      # [新] 租户模型；既有模型批量加 TenantID
│   ├── migration/migrations/…               # [新] tenant_id 加列 + 唯一索引复合化（幂等分批）
│   ├── service/*_cron|dispatcher|worker     # [改] 后台任务经 TenantRunner 扇出
│   ├── aiagent/llm/dispatcher_persistence   # [改] provider 加载按 ctx 租户 → 平台池兜底
│   └── repository/**                        # [改] Exec/Raw 清偿为 GORM API（约 293 处，migration 包豁免）
├── scripts/check-gorm-only.sh               # [新] CI 门禁：业务包禁 .Exec(/.Raw(
user-web/
├── src/views/tenant/Register.vue            # [新] 自助注册页
├── src/api/tenant.js                        # [新] 租户 API 出口
├── src/router/modules/…                     # [改] 注册路由（SaaS 模式显隐）
└── src/stores/…                             # [改] 用户态携带租户上下文
```

**Structure Decision**: 全部落点在既有五层结构与目录内；`internal/pkg/tenant` 为新增横切工具包（pkg 层，不依赖业务层，符合宪法辅助层职责）。

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| 无 | — | — |

> 设计未引入宪法之外的新复杂度：隔离机制是 repository 层既有 `dbFromCtx` 通路的系统化；"GORM-only 清偿"是既有层内债务偿还而非新架构。
