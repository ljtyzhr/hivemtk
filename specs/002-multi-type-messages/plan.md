# Implementation Plan: 渠道多类型消息全链路（图文/视频/语音/文件/混合）

**Branch**: `002-multi-type-messages` | **Date**: 2026-10-24 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/002-multi-type-messages/spec.md`

## Summary

消息中台数据模型与直连渠道（webhook 系）的多类型消息底座已就绪（msg_type 词表、media_url 列、媒体下载转存管线）。真正的断点集中在四段：①Chrome 桥扩展上行管线丢弃全部非文本消息（5 处硬过滤）；②桥扩展下行只会打字（sendOutbound 仅 rawSendText）；③前端两个渲染面（工作台 inbox List.vue、聊天窗 ChatMessages.vue）只渲染 image 或纯文本；④SSE 出站事件协议缺 media_url 键 + 去重哈希不含类型导致同会话两张图碰撞。方案：扩展端放开过滤并按统一占位符词表上行、下行按 msg_type 分派 sendMedia/降级 URL 文本、前端补全 video/audio/file/link 渲染分支、服务端 SSE Data 补 media_url 恢复 R-B1 逐键一致、哈希口径并入 msg_type+media_url。

## Technical Context

**Language/Version**: Go 1.26（user-server）、JavaScript ES Modules（Chrome MV3 扩展 user-web/bridge）、Vue 3 + Element Plus（user-web）

**Primary Dependencies**: gin、GORM、PostgreSQL 15+、vitest、Playwright（既有桥端测试夹具）

**Storage**: PostgreSQL `message_hub`（msg_type varchar(20)、media_url varchar(500) 既有列，无需 DDL）

**Testing**: `go test ./internal/<包>/...`、`cd user-web && npm run test`（vitest 含 bridge 端用例）

**Target Platform**: Linux server（Go 端）、Chrome 扩展（桥端）、浏览器（工作台/聊天窗）

**Project Type**: web-service + browser-extension + SPA

**Performance Goals**: 不劣化现状——上行批处理、SSE 推送频率、去重判定 O(1) 哈希维持不变

**Constraints**: 五层架构（Router→Controller→Service→Repository→Model）；R-B1「SSE 总线与 DB 补拉路径逐键一致」契约；协议 v2 向后兼容（老客户端按 v1 忽略新增字段）；数据零出域（媒体转存走内部 OBS 配置）

**Scale/Scope**: 桥端 ~5 文件、前端 2 视图 + i18n、服务端 3 文件、测试若干

## Constitution Check

- ✅ 五层架构：服务端改动全部落在 service/bridge 层内既有结构（SSEEvent 构造器、ContentHashMsgID 工具函数），不新增跨层调用。
- ✅ 禁止子 Agent：全程主 Agent 串行。
- ✅ API 响应契约：无新端点；SSE Data 增键属于帧协议 v2 兼容扩展。
- ✅ Router 只做映射：不涉及 router 改动。
- ✅ 前端规范：改 user-web 既有视图与 bridge src，不新增 API 模块。
- ✅ 数据零出域：媒体 URL 全部指向内部存储（/files 或 OBS）。

## Project Structure

### Documentation (this feature)

```text
specs/002-multi-type-messages/
├── plan.md              # 本文件
├── research.md          # Phase 0 调研证据（现状结论已并入 spec，此处存文件级证据索引）
└── tasks.md             # Phase 2 输出
```

### Source Code (repository root)

```text
user-web/
├── bridge/src/
│   ├── core/
│   │   ├── channel-adapter.js   # FR1 上行五处过滤放开 + 媒体占位符 + 去重键；FR2.2 sendOutbound 分派
│   │   ├── downlink.js          # FR2.1 轮询路径透传 msg_type/media_url
│   │   ├── polling-loop.js      # FR2.1 SSE 路径 sendOutbound 包装透传 payload
│   │   └── types.js             # 占位符词表导出（与服务端同词表）
│   ├── src/channels/
│   │   ├── douyin.js / kuaishou.js / tiktok.js / xhs.js / xianyu.js  # FR2.3 sendMedia 降级 hook
│   └── tests/                   # 桥端 vitest 用例
└── src/
    ├── views/inbox/List.vue                    # FR3.1 全类型渲染
    ├── views/chat/embed/components/ChatMessages.vue  # FR3.2 聊天窗媒体渲染
    └── i18n/                                   # FR3.3 文案

user-server/internal/
├── bridge/
│   ├── sse.go                  # FR4.1 OutboundEventData.MediaURL + Data 输出 media_url
│   └── handler_http.go          # FR4.1 buildOutboundEvent 传值
├── service/
│   ├── bridge_outbound.go       # FR4.1 GlobalSSEPublisher 调用点传 media_url（签名加参）
│   ├── inbox_ingress_outbound.go# FR4.1 同上
│   ├── message_hub.go 或 webhook.go (ContentHashMsgID 定义处)  # FR5.1 带类型哈希变体
│   └── inbox_ingress.go         # FR5.3 AI 触发 content 附媒体 URL
└── router/router.go             # FR4.1 publisher 闭包同步（仅传值，不新增路由）

tests（新增）：
user-server/internal/service/multi_type_hash_test.go          # FR5
user-server/internal/bridge/sse_media_url_test.go             # FR4
user-web/bridge/tests/...                                     # FR1/FR2
user-web/src/...vitest                                        # FR3
```

## Phases

### Phase 0: 调研

已完成（spec「现状调研结论」）。关键文件级证据：
- 上行丢弃：`channel-adapter.js:227,295,348,1035,1089`（`msg_type !== 'text'` continue）
- 渠道解析已就绪：五渠道 `parseMessageItem` 返回 `{msgType, mediaUrl, text}`
- 下行只打字：`channel-adapter.js:1333` rawSendText；`downlink.js:424-425` 只取 content
- SSE 缺键：`sse.go:124-161` OutboundEventData 无 MediaURL；`bridge_outbound.go:63` 签名无 media
- outbox HTTP 路径已带 media_url：`handler_http.go:722`
- 渲染缺口：`inbox/List.vue:690-691` isImage 单分支；`ChatMessages.vue` 仅文本+RichCard
- 哈希碰撞：`ContentHashMsgID(channel, conversationID, content)` 无类型入参（`webhook.go`/前端 `contentHash` 同源）
- 中台底座就绪：`InboundHubMsgType` 词表、`message_hub` 列、直连渠道媒体转存（`channel_media.go`）

### Phase 1: 设计

- 协议契约：SSE Data 增 `media_url`（空字符串也输出，保证两路径逐键一致）；桥上行帧不增字段（msg_type/media_url 既有）。
- 占位符词表单一来源：服务端各 webhook_channel_*.go 与扩展 types.js 共用同一中文词表（`[图片]`/`[视频]`/`[语音]`/`[文件]`/`[卡片]`/`[链接]`），文档化在 plan 而非复制散落。
- 哈希新口径：`ContentHashMsgIDTyped(channel, convID, msgType, mediaURL, content)`——文本消息走旧函数（存量行为不变），非文本走新函数；前端 `contentHash` 同步增加 typed 变体并仅在非文本时使用。
- 下行分派：`sendOutbound(text, convId, opts)` 的 opts 扩展 `{msgType, mediaUrl}`；渠道 `hooks.sendMedia` 可选，未实现统一降级 `[图片] URL`。
- AI 上下文：`TriggerInboundAI` 调用点拼接 `content + "\n[图片] url"` 形态（签名不变）。

### Phase 2: tasks（见 tasks.md）

按「服务端协议→桥端上行→桥端下行→前端渲染→AI 上下文→回归验证」顺序串行实施。

## Risks & Mitigations

| 风险 | 缓解 |
|------|------|
| 放开上行过滤后老后端把占位符当正文去重吞并 | 哈希口径同步改（FR5.1），且 `#<n>` 机制兜底 |
| 渠道 DOM 变动导致 mediaUrl 提取失败 | 现有选择器不动，仅消费其输出；解析失败回落 text 占位符 |
| SSE 新键破坏老扩展 | v2 协议新增字段向后兼容（老客户端忽略未知键） |
| 降级发送 URL 文本被平台风控 | 走既有拟人限速/风控管线（sendOutbound 原有 rate limiter 不变） |
| 五渠道 sendMedia 各自实现工作量 | 基线统一降级路径收敛在 channel-adapter，渠道 hook 只做覆盖点 |

## Rollout Plan

1. 服务端先行（SSE 键、哈希变体、AI 拼接）——向后兼容，可独立上线。
2. 桥扩展与前端同版本发布（npm build 产物）。
3. 全量回归：`go build ./...` + 改动包 `go test` + `make lint` + `make vet` + `user-web npm run test`。

## Verification Matrix

| FR | 验证 |
|----|------|
| FR1 | vitest：DOM 夹具构造 image/video 气泡 → getMessages/patrol 输出含 msg_type/media_url |
| FR2 | vitest：mock outbox 返回 image 行 → sendOutbound 收到 {msgType,mediaUrl}，降级文本生成 |
| FR3 | vitest：isMedia 判定 + 渲染分支快照 |
| FR4 | go test：BuildOutboundSSEEvent Data 含 media_url 键，与 outbox JSON 键集一致 |
| FR5 | go test：两张同占位符图片哈希不同；文本哈希与旧值一致 |
| FR5.3 | go test：TriggerInboundAI 收到含 URL 的 content |
