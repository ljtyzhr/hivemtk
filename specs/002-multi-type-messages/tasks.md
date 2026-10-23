# Tasks: 渠道多类型消息全链路

**Feature**: 002-multi-type-messages | **Plan**: [plan.md](plan.md)

---

## 1. 服务端：SSE 出站协议补 media_url（FR4）

- [x] 1.1 `bridge/sse.go`：`OutboundEventData` 增加 `MediaURL string`；`BuildOutboundSSEEvent` 的 Data 输出 `"media_url": d.MediaURL`（始终输出，与 outbox JSON 键集对齐）
- [x] 1.2 `bridge/handler_http.go`：`buildOutboundEvent` 传 `MediaURL: row.MediaURL`（DB 补拉路径）
- [x] 1.3 `service/bridge_outbound.go` + `inbox_ingress_outbound.go` + `router/router.go`：`GlobalSSEPublisher` 签名加 `mediaURL string` 参数，三个调用点传 `h.MediaURL`；router.go 内闭包同步
- [x] 1.4 测试 `internal/bridge/sse_media_url_test.go`：断言 SSE Data 键集合 ⊇ outbox JSON 键集合（逐键一致），media_url 值正确
- [x] 1.5 验证：`go build ./...` + `go test ./internal/bridge/... ./internal/service/... ./internal/router/...`

## 2. 服务端：去重哈希带类型（FR5.1/5.2）

- [x] 2.1 定位 `ContentHashMsgID`（webhook.go 或同文件）；新增 `ContentHashMsgIDTyped(channel, convID, msgType, mediaURL, content)`：msgType 为空或 text 时与旧函数逐字节一致；非文本时哈希输入并入 `msgType|mediaURL`
- [x] 2.2 入站落库路径（`inbox_ingress_persist.go`、`bridge_outbound.go`、相关 webhook dispatch）在 msg_type != text 时使用 Typed 变体
- [x] 2.3 核验 `InboundHubMsgType` 词表对桥端上报类型（image/video/voice/audio/file/link/card/system/recall）全覆盖；缺口补别名
- [x] 2.4 测试 `multi_type_hash_test.go`：同会话两张图（同占位符、不同 URL）msg_id 不同；文本消息 msg_id 与旧口径一致（存量兼容）
- [x] 2.5 验证：`go test ./internal/service/...`

## 3. 服务端：AI 上下文携带媒体 URL（FR5.3）

- [x] 3.1 `inbox_ingress.go` TriggerInboundAI 调用点：content 为占位符且 event.MediaURL 非空时拼接 `\n[<类型词>] <media_url>` 后传入（签名不变）
- [x] 3.2 webhook 直连渠道 dispatch（webhook_ai.go / 各 channel dispatch）同口径核验：占位符正文 + media_url 的拼接逻辑统一为一个小工具函数并复用
- [x] 3.3 测试：图片入站触发 AI 时 mock aiTrigger.lastContent 含 URL

## 4. 桥扩展：上行放开非文本（FR1）

- [x] 4.1 `types.js`：导出 `MEDIA_PLACEHOLDER` 词表与 `mediaPlaceholder(msgType)`；`makeUnifiedMessage` 不变（字段既有）
- [x] 4.2 `channel-adapter.js` 五处硬过滤（getMessages:295、_collectUnseenText/巡检:1035、_ingest:1089、_countVisibleText:348/227）改为：非文本消息保留，content 取 `text || mediaPlaceholder(msgType)`，media_url/media_type 透传；`_pushWindow` 与 occurrence 计数同步纳入
- [x] 4.3 `_dedupKey` 与 `_canonicalMsgId`：非文本时哈希输入并入 `msgType + '|' + mediaUrl`（与服务端 Typed 口径同构）；`contentHash` 工具加 typed 变体
- [x] 4.4 `_emitPatrolMessage` 顶层 msg_type 取 `last.msg_type || 'text'`；`_historyItem` 携带 msg_type/media_url（核验既有结构，缺则补）
- [x] 4.5 五渠道 `parseMessageItem` 核验：mediaUrl 已输出的（douyin/kuaishou/tiktok/xhs/xianyu）不重写；仅确认字段名对齐 `media_url`
- [x] 4.6 vitest：DOM 夹具（image/video 气泡）→ adapter.getMessages()/巡检批输出含 msg_type='image'、media_url 非空；两张同占位图不互吞
- [x] 4.7 验证：`cd user-web && npm run test`

## 5. 桥扩展：下行多类型投递（FR2）

- [x] 5.1 `downlink.js` pollDownlink：透传 `m.msg_type`/`m.media_url` 到 sendOutbound 第三参 opts（`{viaAdapter, msgType, mediaUrl}`）；空 content 但 msg_type 非 text 不再 continue
- [x] 5.2 `downlink.js` SSE 路径（startSSEDelivery 内 handlers.sendOutbound 调用点）：Data 已含 media_url（服务端 1.1 后），同样透传
- [x] 5.3 `channel-adapter.js` sendOutbound：opts.msgType 非 text 且 opts.mediaUrl 非空 → 优先 `this.hooks.sendMedia(msgType, mediaUrl, conv)`（存在且函数时）；否则降级 `rawSendText(mediaPlaceholder + ' ' + mediaUrl)`；成功判定/限速/回查沿用文本路径
- [x] 5.4 `sanitize.js`：降级文本经 sanitizeForDisplay 不破坏 URL（核验，必要时 URL 白名单保号）
- [x] 5.5 五渠道 hooks 各暴露可选 `sendMedia`（默认不实现 → 统一降级路径生效）；在 kuaishou.js 或 douyin.js 至少一个渠道实现「点击输入框旁上传按钮不存在时的降级」即维持降级（基线）
- [x] 5.6 `_emitMessage` 自报出站（sendOutbound 成功后）带真实 msg_type/media_url，避免服务端回声去重误判
- [x] 5.7 vitest：mock outbox image 行 → sendOutbound 收到 opts.msgType/mediaUrl；无 sendMedia hook 时降级文本包含 URL；行 ack 路径不回归
- [x] 5.8 验证：`npm run test`

## 6. 前端：工作台与聊天窗渲染（FR3）

- [x] 6.1 `inbox/List.vue`：新增 `msgKind(msg)` 判定（image/video/audio/file/link/card/text）与渲染分支——video `<video controls>`、audio `<audio controls>`、file 下载卡片（el-link + 文件图标）、link 卡片；空 media_url 显示占位标签；保留既有 isImage 语义为 msgKind 的一部分
- [x] 6.2 `ChatMessages.vue`：气泡内按 content_type 渲染媒体（image/video/audio/file），媒体与文字共存时媒体在上文字在下；RichCard 分支不动
- [x] 6.3 i18n：新增「图片/视频/语音/文件/链接/媒体加载失败」等标签文案，按仓库语言包全集同步
- [x] 6.4 vitest（或既有测试范式）：msgKind 判定与渲染分支
- [x] 6.5 验证：`npm run test`

## 7. 回归与收尾

- [x] 7.1 `cd user-server && go build ./...`
- [x] 7.2 `go test ./internal/...`（改动包全集；含需 DB 的测试按 3.5 前提 export POSTGRES_TEST_*）
- [x] 7.3 `make lint` + `make vet`
- [x] 7.4 `cd user-web && npm run test`
- [x] 7.5 `make audit`（已知红如实记录，不为绿灯改别人文件）
- [x] 7.6 文档：`docs/` 下若有消息类型/协议文档则补 media_url 键与占位符词表说明；无则在本 spec 目录补 implementation.md 记录偏差
- [x] 7.7 converge 复核：对照 spec FR1-FR6 逐条核对实现与测试证据，缺漏回补
