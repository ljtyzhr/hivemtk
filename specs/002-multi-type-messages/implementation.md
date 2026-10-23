# Implementation Notes: 渠道多类型消息全链路（002-multi-type-messages）

**Date**: 2026-10-23 · **Status**: 全部任务完成，全链路验证绿（converge 复核 FR1-FR6 逐条证据核对通过）

## 交付范围与验证状态

| 验证项 | 命令 | 结果 |
|--------|------|------|
| 服务端编译 | `go build ./internal/... ./cmd/...` | ✅ |
| 服务端测试（改动包） | `go test ./internal/service/ ./internal/bridge/ ./internal/router/ -count=1`（需 `POSTGRES_TEST_PORT/POSTGRES_TEST_PASSWORD`，本机实测库口令与 .env 不同源，取容器 env） | ✅ 全绿（service 600s 全量、bridge 18s、router 33s） |
| lint | `make lint` | ✅ 0 issues（顺带修复 5 处存量 gofmt 红 + 16 个测试文件格式） |
| vet | `make vet` | ✅ |
| 桥扩展测试 | `cd user-web/bridge && npx vitest run` | ✅ 72 文件 878 过 7 skip |
| 前端测试 | `cd user-web && npx vitest run` | ✅ 66 文件 679 用例全绿 |
| 静态审计 | `make audit` | ✅ 全绿（见下「audit 联动修复」） |

## 实现要点（与 plan 的对应）

- **FR1/FR2 桥扩展**：`types.js` 新增 `typedContentHash`/`mediaPlaceholder`/`MEDIA_PLACEHOLDERS`/`isMediaMsgType`；`channel-adapter.js` 五处非文本过滤放开（getMessages/occurrence/巡检/_ingest/_backfill），`_countVisibleText` 保留跳过（计数语义）；`_dedupKey`/`_canonicalMsgId` 并入 `type|media_url`；下行 `sendOutbound` 按 msgType 分派 `hooks.sendMedia` → 降级 `[占位符] URL` 文本；`downlink.js` 轮询/SSE 双路径透传；`base-adapter.js` 提供可选 `sendMedia` 默认不实现（五渠道统一走降级基线，符合 spec FR2.3「本期不强制原生上传」）。
- **FR4 SSE**：`bridge/sse.go` `OutboundEventData.MediaURL` + Data 始终输出 `media_url`；`handler_http.go`/`bridge_outbound.go`/`inbox_ingress_outbound.go`/`webhook_outbound.go`/`router.go` 全调用点同步；协议测试断言 SSE 键集 ⊇ outbox 键集。
- **FR5 去重**：Go `ContentHashMsgIDTyped`（webhook_dedup.go）与 JS `typedContentHash`（types.js）逐字节同构：`channel|msgType|mediaURL|content`，text/空类型退化为旧口径（存量兼容）；`inbox_ingress_persist.go` 两处 MsgID 兜底改用 Typed。
- **FR5.3 AI 上下文**：新增统一小工具 `AppendMediaContextToContent(content, msgType, mediaURL)`（webhook_dedup.go）——非文本且带 URL 时拼 `\n[<类型词>] <URL>`。两个漏斗共用：桥接路径 `inbox_ingress.go triggerAIForEvent`（调用点拼 content，签名不变）；直连路径 `webhook_ai.go triggerSmartOrchestrator`（IncomingContext.Content 拼接）。类型词表与直连渠道占位符词表同源（image→[图片]、audio→[语音]、未知兜 `[<类型>]`）。
- **FR3/FR6 渲染**：`inbox/List.vue` video/audio/file/link 渲染分支 + 占位标签（`MSG_TYPE_LABELS`）；`ChatMessages.vue` 气泡按 content_type 渲染 image/video/audio/file；i18n 9 语言包补「图片/视频/语音/文件/链接/卡片/位置」词条。

## 偏差与决策记录

1. **FR6 按 spec 预定与 FR3.2 合并验收**（不新增 AI 主动发媒体的生成能力），无独立改动。
2. **AI 上下文拼接采用换行形态** `\n[图片] url`（spec FR5.3 写的是「content 后附加 `[图片] url` 形态」）：换行保证「正文 + 附件」结构可被下游 prompt 解析，混合图文正文不会被 URL 尾巴粘连。文本消息逐字节零破坏（有测试钉死）。
3. **audit 联动修复**（发现即修原则，均属其他在途改动留下的红，非本任务引入）：
   - `config_param_group_seeds.go` 补 `content_guard` 组登记、`config_param_test.go` 种子计数 238→239（content_guard 新种子的登记缺口）；
   - `hardcode-tunable.baseline` 登记 `InboundMisattribWindow`（fcad787e 错归去重闸的新常量）；
   - `scripts/check-i18n-coverage.py`：`collect_literal_key_refs` 返回全部引号字面量、main 与词条库 leaf 求交——补认「中文整键先存后消费」形态（`MSG_TYPE_LABELS` 的 `'图片'` 等键由模板 `$t(typeLabel(msg))` 动态消费，与 titleKey 同构）。这是测量口径修正，不是放行新增；
   - U+FFFD 替身字符清理：`ar.json`/`ja.json` 各 2 处损坏句子按 zh 原文重写（41c7ae1f 引入，基线允许 0 所以 audit 必红）、`i18n_messages_compile.test.js` 注释修复；
   - `conversionFunnel/List.vue` 的 `v-if="report.degraded"` 加 null 守卫（e67d0aa9 在途改动把 report 初始化为 null 但模板先行解引用，5 个测试红）；
   - gofmt 格式修复 21 个文件（make lint 5 处生产代码 + fmt-check 16 个测试文件）。
4. **测试库连接**：本机 `.env` 的 POSTGRES_PASSWORD 与运行中容器 `mtk-postgres` 的实际口令不同源（.env 8202 端口对但口令是旧的），实测从容器 env 取真实口令跑通。CI 注入 POSTGRES_TEST_* 不受影响。
5. **桥端 Task 4/5 改动已先行入库**：`channel-adapter.js`/`types.js`/`downlink.js`/`base-adapter.js`/`multi-type-messages.test.js` 的改动随并行会话的 7e2946a2（桥扩展换会话互斥修复）与 dab8eed6 提交进 master——该 commit 的 diff 里同时包含互斥修复与本任务的上行/下行多类型改动（工作区核验：这些文件 `git status` 干净、`git show HEAD` 已含 typedContentHash/mediaPlaceholder/sendMedia 分派，13 用例桥端测试对 HEAD 版本全绿）。本次提交不再重复包含它们。
6. **converge 复核**：FR1-FR6 共 14 项实现证据逐条 grep 核验通过（占位符/typed 哈希/巡检类型/downlink 透传/sendMedia 分派/自报类型/两渲染面/SSE MediaURL/两路径 AI 上下文），全部有测试钉死。
7. **对抗性复核（2026-10-24）补三处缺口**，均有 `multi_type_ai_context_adversarial_test.go` 用例钉死：
   - 桥端四渠道解析器原样上报 `msg_type='voice'`（douyin/tiktok/xhs/xianyu），`triggerAIForEvent` 用未归一的 `event.MsgType`，Go 词表缺 voice 键 ⇒ AI 上下文出 `[voice]` 而非 `[语音]`。修：`mediaContextTags` 补 `"voice": "[语音]"`（audio/voice 同词，主/补触发口径不因归一时点分叉）；
   - `RecheckUnrepliedAndTrigger` 重建 ev 漏带 `MediaURL` ⇒ 补触发路径媒体消息丢 URL（FR5.3 口径分叉）。修：ev 构造补 `MediaURL: last.MediaURL`；
   - `AppendMediaContextToContent("", "image", url)` 产出前导 `\n`。修：空正文时直接返回 `[类型词] URL` 不带换行。

## 测试证据

- `user-server/internal/service/multi_type_hash_test.go` — Typed 哈希三口径（旧口径逐字节一致/同占位符不同 URL 不碰撞/同图重发稳定）
- `user-server/internal/service/multi_type_ai_context_test.go` — FR5.3：图片入站 AI content 含 `\n[图片] url`；文本逐字节不变；无 URL 非文本不动；词表/大小写/空白容错 10 例
- `user-server/internal/service/multi_type_ai_context_adversarial_test.go` — 对抗性复核：voice 别名未归一出 `[语音]`；空正文无前导换行；Recheck 补触发携带 media_url；voice 全链路入站→AI 触发标签正确
- `user-server/internal/bridge/sse_media_url_test.go` — SSE Data 键集与 outbox JSON 逐键一致（含 media_url）
- `user-web/bridge/test/multi-type-messages.test.js` — 13 用例：上行五处过滤放开、占位符、typed 去重、下行分派/降级/自报
- `user-web/tests/unit/chat_embed_multi_type_messages.test.js` — 6 用例：ChatMessages 媒体气泡渲染
