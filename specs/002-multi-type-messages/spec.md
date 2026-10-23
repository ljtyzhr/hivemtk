# Feature Specification: 渠道多类型消息全链路（图文/视频/语音/文件/混合）

**Feature Branch**: `002-multi-type-messages`

**Created**: 2026-10-24

**Status**: Approved

**Input**: User description: "渠道消息内容补全：当前仅仅文字补全，图片、视频、语音、文件等。背景：当前各种渠道接入消息中心，全链路只有文字，没有考虑其他类型消息（图文、视频、文件、混合图文等）。任务：1 调研论证当前系统现状；2 完成所有渠道的多类型消息的综合实现。"

## 现状调研结论（Argumentation）

对代码库的实际调查结果（2026-10-24，主 Agent 逐文件核验）：

### 已经支持多类型消息的环节（无需重做）

| 环节 | 文件 | 现状 |
|------|------|------|
| 中台数据模型 | `user-server/internal/model/message_event.go`、`ai_sales_champion.go`(MessageHub) | `msg_type` 词表齐全（text/image/file/audio/video/link/card/location/event），`media_url` 字段已有 |
| 渠道官方类型归一 | `user-server/internal/service/message_hub.go` | `InboundHubMsgType` + `inboundHubMsgTypeAliases` 收口表（voice→audio、document→file、media→video 等全套别名） |
| 直连渠道入站（TG/WA/企微/飞书/钉钉/公众号/抖音/QQ） | `telegram_media.go`、`channel_media.go`、`webhook_channel_*.go` | 媒体下载 + 转存 + `media_url` 回填已实现（占位符正文 + 异步转存） |
| Bridge 协议 | `channelgw/protocol.go` | `IngestMessage`/`OutboundReply`/`OutboxMessage`/`HistoryItem` 均带 `msg_type` + `media_url` |
| SSE/Outbox 出站投递 | `bridge/sse.go`、`bridge/handler_http.go` | `media_url` 逐键下发 |
| 统一收件箱消息读接口 | `service/inbox.go` GetMessagesByConversation | 已输出 `content_type` + `media_url` |
| 工作台消息中台列表/推送页 | `user-web/src/views/messageHub/List.vue`、`WeComSendDialog.vue` | msg_type 选择、media_url 输入/展示已有 |

### 真正的断点（本功能要修的）

1. **Bridge 扩展（Chrome 桥）上行丢弃全部非文本消息** —— `user-web/bridge/src/core/channel-adapter.js` 中 5 处 `if (parsed.msg_type && parsed.msg_type !== 'text') continue`（getMessages / 增量 / 巡检 / 回查计数 / _ingest），尽管五个渠道适配器（douyin/kuaishou/tiktok/xhs/xianyu）的 `parseMessageItem` 已能识别 image/video/voice/link/card 并给出 `mediaUrl`，识别结果在上行管线被整行丢弃。结果：桥接五端（抖音/快手/TikTok/小红书/闲鱼）的客户图片/视频/语音根本到不了消息中心。
2. **Bridge 扩展下行只会打字** —— `channel-adapter.js` 的 `sendOutbound` 只调 `rawSendText`；`downlink.js` 轮询/SSE 路径只透传 `sanitized` 文本。服务端下发的 `msg_type`+`media_url` 被无视，非文本出站消息在扩展端静默失败（ack 永不回、行反复重推）。
3. **工作台统一收件箱会话窗只渲染图片** —— `user-web/src/views/inbox/List.vue` 的 `isImage()`：只有 image 渲染 `<el-image>`，video/audio/file/link/card 全部 fallback 成 `{{ msg.content }}`（即一行「[视频]」「[语音]」文字）。
4. **网页客服聊天窗不渲染媒体** —— `user-web/src/views/chat/embed/components/ChatMessages.vue` 只渲染文本气泡 + RichCard；访客上传的 image/video/audio/file（`media_url`/`content_type` 已入库）在聊天气泡里不显示。
5. **SSE 出站事件不带 media_url** —— `bridge/sse.go` 的 `OutboundEventData`/`BuildOutboundSSEEvent` 与 `service/bridge_outbound.go` 的 `GlobalSSEPublisher` 签名都没有 media_url 字段；轮询 outbox 路径带、SSE 路径不带，两路径逐键一致性契约（R-B1）被破坏。
6. **AI 触发链路内容单一** —— `TriggerInboundAI(..., content, ...)` 只传正文文本；非文本消息的 AI 上下文只有「[图片]」占位符，无 URL。混合图文消息（multi item）上行时服务端只落一条 text。
7. **去重键不含类型** —— `_dedupKey`/`ContentHashMsgID`/`contentHash` 均为 (channel, conv, content)；非文本消息 content 常为空/占位符，同会话两张图即哈希碰撞。

### 渠道清单与能力矩阵（现状 → 目标）

| 渠道 | 接入方式 | 入站非文本 | 出站非文本 |
|------|----------|-----------|-----------|
| Telegram/WhatsApp/企微/飞书/钉钉/公众号/QQ/抖音(webhook) | 官方 webhook | ✅ 已支持（下载+转存） | ✅ 已支持（官方 API） |
| 抖音/快手/TikTok/小红书/闲鱼 | Chrome 桥（bridge） | ❌ 扩展丢弃 | ❌ 只会打字 |
| 网页客服 web_embed | WebSocket/HTTP | 部分（访客可传，AI/客服回不了媒体） | 部分 |
| 短信/邮件 | reach | ✅（文本通道，无多类型诉求） | ✅ |

**结论**：断点集中在 ①Chrome 桥扩展（上行+下行）、②前端渲染（工作台+聊天窗）、③SSE 协议补字段、④去重键、⑤AI 上下文。中台与直连渠道的底座已就绪，不需要动数据模型。

## User Scenarios & Testing

### User Story 1 - 桥接渠道客户发图，坐席在统一收件箱看到图（Priority: P1）

客户在抖音/小红书/快手/TikTok/闲鱼私信里发图片、视频、语音或文件；坐席打开统一收件箱该会话，能看到真实媒体（可预览的图、可播放的视频/语音、可下载的文件），而不是一行「[图片]」占位文字或「该会话暂无消息」。

**Why this priority**: 桥接五端是本系统主打能力，当前客户发媒体 = 消息蒸发，属于数据丢失级缺陷，价值最高。

**Independent Test**: 在桥接渠道页发一条图片消息，查 message_hub 新行 `msg_type='image'` 且 `media_url` 非空，工作台会话窗渲染出 `<el-image>`。

**Acceptance Scenarios**:

1. Given 抖音私信客户发送图片, When 扩展巡检捕获, Then 上行帧 `msg_type='image'`、`media_url` 为图片直链，message_hub 落库 msg_type=image，工作台渲染图片。
2. Given 客户发送视频, When 同上, Then msg_type=video 落库，工作台渲染 `<video>` 可播放。
3. Given 客户连续发送两张内容相同的图片, When 上行, Then 两条独立消息各自落库（去重键含类型与 media_url）。
4. Given 一条气泡同时含图片和文字（混合图文）, When 解析, Then media 部分按 image 上行、文字进 content，不丢任一半。

### User Story 2 - 坐席/AI 给桥接渠道客户回媒体（Priority: P1）

服务端（AI 回复或坐席触达）产生一条 `msg_type='image'` + `media_url` 的出站消息；桥扩展通过 SSE/轮询取回后能把它投进目标会话（受各平台页面能力限制，无法自动上传媒体的渠道降级为发送链接文本，并正确 ack）。

**Why this priority**: 出站与入站对称，否则坐席看见了图却永远回不了图。

**Independent Test**: 在 message_hub 插入 pending 的 image 出站行，扩展拉取后目标会话收到「图片链接消息」或原生媒体，行状态翻 delivered。

**Acceptance Scenarios**:

1. Given pending 出站行 msg_type=image 且 media_url 有效, When 扩展拉取, Then 目标会话出现该媒体/其 URL，行翻 delivered（不再无限重推）。
2. Given msg_type=video 出站, When 扩展投递, Then 同上。
3. Given 渠道页面无上传能力, When 投递, Then 发送「[图片] URL」降级文本并 ack，不静默失败。

### User Story 3 - 网页客服聊天窗双向媒体（Priority: P2）

访客在聊天窗上传的图片/文件在坐席侧与访客侧气泡中都正确渲染；AI/坐席回复中的媒体（若有）同样渲染。

**Acceptance Scenarios**:

1. Given 访客发送图片消息（content_type=image, media_url 非空）, When 访客端/坐席端渲染, When 双侧都显示图片缩略图（可预览）。
2. Given content_type=file, When 渲染, Then 显示文件名与下载链接。
3. Given content_type=audio/video, When 渲染, Then 显示原生播放器。

### User Story 4 - 统一收件箱全类型渲染（Priority: P2）

工作台会话窗对 hub 与 session 两个来源的消息统一按 content_type 渲染：text（现状）、image（现状）、video/audio（原生播放器）、file（文件卡片+下载）、link/card（链接卡片）。

**Acceptance Scenarios**:

1. Given 会话里存在 video 消息, When 打开会话窗, Then 显示 `<video controls>` 播放器。
2. Given 存在 file 消息, When 渲染, Then 显示文件下载入口。
3. Given media_url 为空但 msg_type 非 text, When 渲染, Then 显示占位标签（如「[语音]」）不报错。

### User Story 5 - AI 上下文携带媒体信息（Priority: P3）

非文本入站消息触发 AI 时，AI 收到的内容里除了占位符还包含媒体 URL 与类型，多模态/后续人工可追溯。

**Acceptance Scenarios**:

1. Given 客户发图片, When AI 触发, Then prompt 内容包含 `[图片] <media_url>` 形态的上下文（不破坏现有文本链路）。

## Functional Requirements

### FR1 桥扩展上行多类型（改造 channel-adapter.js 及五渠道 hook）

- FR1.1 `getMessages`、增量处理、巡检批（`_collectUnseen*`/patrol）、`_ingest`、`_countVisibleText` 五处删除「非 text 即丢」的硬过滤，改为：非文本消息以占位符/描述文本 + `msg_type` + `media_url` 上行；纯媒体无文字时 content 使用统一占位符（与直连渠道同词表：`[图片]`/`[视频]`/`[语音]`/`[文件]`/`[卡片]`/`[链接]`）。
- FR1.2 混合图文（同一气泡既有图又有文字）：文字进 content、媒体 URL 进 media_url，msg_type 取主导媒体类型；无法拆分时按现有渠道解析器输出。
- FR1.3 去重键 `_dedupKey` 与 `_canonicalMsgId` 的哈希输入并入 `msg_type` 与 `media_url`，杜绝两张同会话图片哈希碰撞；服务端 `ContentHashMsgID` 同口径（见 FR5）。
- FR1.4 巡检组帧 `_emitPatrolMessage` 的顶层 `msg_type` 不再写死 `'text'`，取最后一条消息的真实类型；history item 同步携带 msg_type/media_url。
- FR1.5 系统消息（system/recall）维持现状（`<type>:<id>` 前缀），不受影响。

### FR2 桥扩展下行多类型（sendOutbound 媒体路径）

- FR2.1 `downlink.js` 轮询与 SSE 两条路径把 outbox 消息的 `msg_type`/`media_url` 透传给 `sendOutbound`（新增第三参 payload 或 opts 扩展），空内容但非 text 类型不再被 `if (!raw) continue` 拦截。
- FR2.2 `channel-adapter.js` `sendOutbound` 按 msg_type 分派：text 走现有 rawSendText；image/video/file/audio 走各渠道 `hooks.sendMedia`（若实现）；未实现 sendMedia 的渠道降级发送 `[图片] URL` 文本；降级也算投递成功并 ack。
- FR2.3 五个渠道适配器（douyin/kuaishou/tiktok/xhs/xianyu）各实现力所能及的 `sendMedia`：至少实现「打开会话→粘贴 URL 文本」的降级统一路径；对 DOM 上有原生上传按钮的渠道可后续增强（本期不强制）。
- FR2.4 出站回查（`_verifySendLanded`）对降级文本按文本口径回查；对 sendMedia 成功返回的由渠道 hook 自证，不强制 DOM 回查。

### FR3 工作台与聊天窗渲染（user-web）

- FR3.1 `views/inbox/List.vue`：新增 isVideo/isAudio/isFile/isLink/isMedia 判定与对应渲染分支（`<video controls>`、`<audio controls>`、文件下载卡片、link 外链卡片）；content 为占位符时优先显示媒体本体。
- FR3.2 `views/chat/embed/components/ChatMessages.vue`：气泡内按 `content_type` 渲染媒体（image/video/audio/file），媒体在上文字在下的混合布局；RichCard 分支保持不变。
- FR3.3 i18n：新增占位/标签文案四语言（实际按仓库语言包全集）同步。

### FR4 SSE 出站事件补 media_url（服务端）

- FR4.1 `bridge/sse.go` `OutboundEventData` 增加 `MediaURL` 字段并在 `BuildOutboundSSEEvent` 的 Data 输出 `media_url` 键；`handler_http.go` 的 `buildOutboundEvent`（DB 补拉路径）与 `service/bridge_outbound.go`/`inbox_ingress_outbound.go` 的 `GlobalSSEPublisher` 调用点同步传值——恢复 R-B1「两路径逐键一致」契约。
- FR4.2 `channelgw/ws.go` pushOnce 已带 MediaURL，无需改（核验项）。

### FR5 服务端去重与占位符一致性

- FR5.1 `ContentHashMsgID` 增加带类型的变体（或并入 media_url 的重载），入站非文本消息的 msg_id 生成使用新口径；仅影响新增行，存量行 msg_id 不迁移。
- FR5.2 `InboundHubMsgType` 词表补桥接五端扩展上报的类型名（voice/link/card 已有别名或同名词，核验 image/video/file/text 直接命中）。
- FR5.3 AI 触发：`TriggerInboundAI` 调用点在 content 后附加媒体 URL（`[图片] url` 形态），签名不破坏（拼进 content 字符串）。

### FR6 网页客服出站媒体（chat embed）

- FR6.1 `visitor_handler`/AI 回复路径核验：AI 回复若带卡片（RichCard）已有通道；本期不新增 AI 主动发媒体的生成能力，仅保证「会话窗能渲染任何 content_type」（与 FR3.2 合并验收）。

## Success Criteria

1. 桥接渠道（至少抖音+小红书两个代表渠道的 DOM 夹具）入站 image/video 各一条：message_hub 行 msg_type 正确、media_url 非空、前端渲染正确 —— 全链路证据（上行帧 JSON → DB 行 → 前端渲染分支）。
2. 出站 image 行经 outbox/SSE 投递：扩展端收到 media_url，降级文本送达目标会话，行状态 delivered。
3. GetMessagesByConversation 返回的 video/audio/file 消息在 inbox List.vue 与 ChatMessages.vue 均有对应渲染分支（组件测试或 vitest 断言）。
4. SSE Data 键集合与 outbox JSON 键集合一致（含 media_url）——协议测试断言。
5. 同会话两条相同占位符的非文本消息（两张无文字图片）不互相吞并（去重测试）。
6. 全部现有测试不回归：`go build ./...`、改动包 `go test`、`user-web npm run test` 全绿。

## Key Entities

- `model.MessageHub`（msg_type/media_url 既有列）
- `channelgw.OutboundReply`/`OutboxMessage`/`IngestMessage`（协议既有字段，补齐使用方）
- `bridge.SSEEvent`/`OutboundEventData`（新增 MediaURL）
- 扩展端 `makeUnifiedMessage`/`parseUnifiedReply`/`sendOutbound` 契约
- 前端渲染判定函数族 isImage→isMedia*

## Assumptions

- 桥接渠道的页面 DOM 解析（五渠道 parseMessageItem 的媒体识别）当前已能给出 mediaUrl，本期只修管线丢弃与下行能力，不重写各渠道选择器。
- 桥接渠道下行媒体以「降级 URL 文本」为基线交付；原生 DOM 上传自动化（文件选择器注入）风险高，本期不做。
- 直连渠道（webhook 系）入站/出站多类型已达标，不在本期改动范围（仅协议一致性核验）。
- 短信/邮件渠道无多类型诉求。

## Out of Scope

- 媒体内容的 OCR/ASR/视觉理解（AI 多模态消费媒体内容本身）。
- 桥接渠道的原生文件上传自动化（input[type=file] 注入）。
- 存量历史数据的 msg_id 重算与迁移。
