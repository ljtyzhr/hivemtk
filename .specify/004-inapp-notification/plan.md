# Implementation Plan: 站内消息通知模块

**Branch**: `004-inapp-notification` | **Date**: 2026-10-22 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `.specify/004-inapp-notification/spec.md`

## Summary

把"平台端发布消息 → 用户端全员收到 → 查看详情"这条链路打通并发布上线。

现状是**两端骨架都在、中间那段是断的**：平台端有 `platform_messages` 表与
`/platform/message/{list,send,latest,read}` 端点，用户端有 `notifications` 表与
`/api/auth/notifications*` 端点、通知中心页面、铃铛角标位，但平台发的消息
**从不进入用户端**（两库无同步），平台端也**没有任何发布界面**，铃铛角标
`unreadCount` 声明后**从未被赋值**（恒为 0）。

技术路线：**用户端周期拉取**（pull），完全复用仓内已在线上跑通的
`module-sync` 模式（模块下发同步，周期 10 分钟，HMAC 签名走 `merchant-api`）。
平台端不主动回调 —— 生产是单机部署，user-server 没有公网回调地址。
已读语义用**独立的回执表**按人记录，解决 `is_read` 挂在消息行上导致
"一人点开、所有人清零"的根本缺陷。

## Technical Context

**Language/Version**: Go 1.24（user-server / platform-server）；Vue 3 + Element Plus（platform-web / user-web）
**Primary Dependencies**: Gin、GORM、PostgreSQL、Axios、Pinia
**Storage**: PostgreSQL —— 平台库 `platform_db`（容器 `mtk-platform-postgres`，端口 8201）；用户库 `user_db`（容器 `mtk-postgres`，端口 8202）
**Testing**: `go test`（表驱动 + 真实 PG 测试库）、Vitest（前端）、`scripts/api_verify_full.py`（三端验证）
**Target Platform**: Linux 服务器（`101.35.252.112:57122`，宝塔 + Docker Compose）
**Project Type**: 双后端 + 双前端的 web-service（平台运营端 + 用户端）
**Performance Goals**: 通知中心首屏 < 1s；未读数查询 < 200ms；单租户通知列表 1000 条内分页稳定
**Constraints**: 投递周期 ≥ 3 分钟（对齐心跳节奏）；投递不得因平台不可达而阻断用户端；不得引入数据出域
**Scale/Scope**: 线上 16 租户 / 21 用户（`system_users`），平台 1 商户 15 条租户登记；本次新增 1 张表（用户端）+ 1 张表（平台端）+ 4 处扩列

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| 原则 | 适用 | 结论 |
|---|---|---|
| I. 五层架构（NON-NEGOTIABLE） | 是 | ✅ 新增域按 Router→Controller→Service→Repository→Model 落位，四文件齐全，DI 在装配层完成 |
| II. 禁止子 Agent 并行（NON-NEGOTIABLE） | 是 | ✅ 全程主 Agent 串行执行，不派子 Agent |
| III. 验证纪律：不许吹大 | 是 | ✅ 门禁命令贴真实输出；未跑的检查记 skipped 并写明原因 |
| IV. 契约一致性：外部成败以业务码为准 | 是 | ✅ 平台→用户端走仓内既有 HMAC 通道，判据对齐 `module-sync` 的既有口径 |
| V. API 契约 | 是 | ✅ 用户端 `/api/{domain}/{resource}`、平台端 `/platform/{resource}`；响应 `{code,data,message}` |

**Pre-Phase-0 结论**：无违反，全部 PASS。

**Post-Phase-1 复查**（设计完成后重新评）：见文末「设计后复查」。

## Project Structure

### Documentation (this feature)

```text
.specify/004-inapp-notification/
├── spec.md              # 需求规格（Phase: specify）
├── plan.md              # 本文件
├── research.md          # Phase 0 技术决策
├── data-model.md        # Phase 1 实体与字段
├── quickstart.md        # Phase 1 端到端验证指南
├── checklists/
│   └── requirements.md  # 规格质量校验清单
├── contracts/
│   ├── platform-notice-api.md   # 平台端对外接口契约
│   └── user-notice-api.md       # 用户端对外接口契约
└── tasks.md             # Phase 2（由 /speckit-tasks 生成）
```

### Source Code (repository root)

本仓是**多仓多栈**结构，本次改动跨 2 个 Go 后端 + 2 个 Web 前端：

```text
hivemtk/user-server/                 # 用户端 Go 后端
├── internal/model/
│   ├── notification.go              # 改：加 source_msg_id / level / source 字段
│   └── notification_read.go         # 新：回执模型
├── internal/repository/
│   ├── notification.go              # 改：加幂等 upsert / 回执写读
│   └── notification_read.go         # 新：回执仓储
├── internal/service/
│   ├── notification.go              # 改：列表 join 回执算 is_read
│   ├── notice_sync.go               # 新：平台消息拉取落库服务
│   └── notice_sync_runner.go        # 新：周期同步 runner
├── internal/controller/
│   └── notification.go              # 改：MarkRead 写回执
├── internal/platform/
│   └── notice_client.go             # 新：平台端 HMAC 客户端
├── internal/migration/migrations/
│   └── inapp_notification_migration.go  # 新：建表 + 加列
├── internal/router/
│   └── auth_routes.go               # 改：挂新路由（只做映射）
└── cmd/api/main.go                  # 改：装配同步 runner

hivemtk/user-web/                    # 用户端前端
├── src/api/notification.js          # 新：通知中心 API 出口
├── src/views/Notifications.vue      # 改：去 fallback、按来源区分
├── src/layout/Layout.vue            # 改：铃铛角标接线
└── src/components/MessageNotification.vue  # 改：飘条改读本地通知中心

hivemtk-platform/platform-server/    # 平台端 Go 后端
├── internal/model/
│   ├── message.go                   # 改：加 tenant_scope / published_at
│   └── notice_delivery_log.go       # 新：投递结果台账
├── internal/repository/
│   └── message.go                   # 改：按租户过滤 + 游标查询
├── internal/service/
│   └── message_service.go           # 改：level/targets 真正生效 + 发布校验
├── internal/controller/
│   └── merchant_notice_controller.go  # 新：merchant-api 拉取端点
└── internal/router/router.go        # 改：挂路由

hivemtk-platform/platform-web/       # 平台运营前端
├── src/api/notice.js                # 新
├── src/views/platform/notice/List.vue  # 新：发布页
└── src/router/modules/platform.js   # 改：挂路由
```

**Structure Decision**：沿用仓内既有的双仓双栈结构，不引入新仓库、不新建
顶层目录。**平台端与用户端是两个独立 git 仓库**（`hivemtk` 与 `hivemtk-platform`），
提交与推送分别进行（各自远端名以现测 `git remote -v` 为准）。

## 跨仓协作的关键契约

平台消息要跨库到达用户端，靠的是**两侧共有的两个标识**：

| 标识 | 平台侧 | 用户侧 | 作用 |
|---|---|---|---|
| `merchant_key` | `merchants.key` | `config/.merchant_key` | 标识哪个部署实例 |
| `tenant_code` | `tenant_registrations.tenant_code` | `tenants.code` | 标识哪个租户 |

线上实测两侧已逐条对齐（`tz3rmdj` / `tgzgc8d` / … 共 15 条 + 本地默认租户
`local-xw0hgtdo`）。**投递按 `tenant_code` 过滤**，用户端收到后按本地
`tenants.code → tenants.id` 反查出租户号落库。

## 复杂度追踪

> 无宪法违反。以下记录的是**实现复杂度**而非合规豁免。

| 复杂度点 | 为什么必须 | 更简方案为何被否 |
|---|---|---|
| 新增回执表而非在消息行上加已读位 | `notifications.is_read` 挂在消息行上，广播消息（`user_id=0`）一行代表 N 个人，A 点了 B 也变已读 | 加 `notification_reads` 表是表达"按人已读"的最小结构；改 `is_read` 语义会破坏既有 SLA / 坏例生产者 |
| 平台侧新增投递台账表 | FR-019 要求可追溯；用户端投递失败只落日志，运营方看不到 | 复用 `audit_logs` 会把"投递流水"与"管理操作审计"混在一起，无法按消息维度查 |
| 投递走拉取而非推送 | 生产单机部署，user-server 无公网回调地址；推送需引入 WS 长连接接登录用户 | 引入 WS 会把工作量翻倍（hub 现只服务访客/智能体），且 FR-002 的 5 分钟可见性目标已满足 |

## 设计后复查（Post-Phase-1 Constitution Re-check）

| 原则 | 结论 | 依据 |
|---|---|---|
| I. 五层架构 | ✅ PASS | 平台端新增 `merchant_notice_controller.go` 只做参数绑定 + 调 service；用户端 `notice_sync.go` 在 service 层，`notice_client.go` 在 platform 包（叶子，不 import service/repository），不构成环 |
| II. 禁并行子 Agent | ✅ PASS | 全程主 Agent 串行 |
| III. 验证纪律 | ✅ PASS | quickstart.md 给出可执行验证命令与预期输出 |
| IV. 契约一致性 | ✅ PASS | 复用既有 `platform.Client.do` 的签名与业务码判据，不新造一套 |
| V. API 契约 | ✅ PASS | 响应统一 `{code,data,message}`；错误码取 0/400/401/403/404/500 |

**结论**：Post-Phase-1 无违反，设计可进入 `/speckit-tasks`。
