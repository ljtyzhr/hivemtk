# Bug Assessment: 巡检换会话与下发键入互不知情 —— 键入期被切走即「发给下一个用户」

- **Slug**: bridge-patrol-switch-vs-send
- **Created**: 2026-10-23T20:30:00+08:00
- **Source**: 用户指令「如果当前正在输入栏输入准备下发转发消息，巡检机制不应该切换下一个，避免正在输入文字却跳转了下一个用户」
- **Verdict**: valid
- **Severity**: high（不可逆：私信发错人不可撤回，且记账侧照样 ack 成功）

## Report (verbatim)

> 如果当前正在输入栏输入准备下发转发消息 巡检机制不应该切换下一个 避免正在输入文字 但是跳转了下一个用户

## Symptom

桥接扩展里有两条互相独立、都会**换右侧会话**的链路：

- 通道 A·上报巡检（`BaseAdapter.patrol` → `_patrolVisit` → `openConversation`）：每 30s 一轮，逐会话点左栏项切换、抓消息上行；
- 通道 C·下发（`PollingLoop` 每 1.5s 拉 `/api/bridge/outbox` → `BaseAdapter.sendOutbound` → `openConversation` + 逐段拟人键入 + 点发送）。

两者对同一个页面 DOM、同一个输入框没有任何互斥。于是：下发把 A 的文案**正往输入框里打**（逐段 insertText，30 字≈2.7s、100 字≈9s），巡检在这几秒里点走了另一位用户 ⇒ 点发送时输入框已属于 B 会话 —— 客户 A 该收到的回复发给了客户 B。用户可见现象就是「正在给这个人打字，界面突然跳到下一个人」。

## Reproduction

1. 打开任一渠道私信页（抖音/小红书/TikTok/闲鱼/快手），扩展激活（巡检自动启动）。
2. 让服务端 outbox 里存在一条**长文案**（≥60 字，键入窗 ≥5s）的下发。
3. 下发开始键入时，巡检轮正好在跑（30s 一轮，一轮最多 6 个会话、每次切换后停 3–5s）。
4. 观察到活动会话在键入期被切走：文案落在新会话的输入框（或被平台按会话分草稿的机制清空），随后点击落在新会话上。

不需要刻意构造时序：**切换节拍（3–5s）< 键入窗（3–9s）**，只要发送落在巡检轮内就几乎必然命中——这是结构性必现，不是偶发竞态。

## Evidence (读码取证)

| 环节 | 位置 | 事实 |
|---|---|---|
| 巡检换会话 | `src/core/channel-adapter.js:patrol()` / `_patrolVisit()` | `openConversation(conv.id)` 真点左栏；每会话后 `sleep(randInt(switchMinMs,switchMaxMs))` = 3–5s |
| 巡检参数 | `src/core/constants.js:PATROL_DEFAULTS` | `intervalMs 30000`、`switchMinMs 3000`、`switchMaxMs 5000`、`maxPerRound 6`、`waitActiveMs 5000` |
| 巡检收尾再切一次 | `channel-adapter.js:patrol()` 末尾 | 把活动会话切回 `beforeConv` |
| 下发节拍 | `constants.js:BRIDGE_THREE_CHANNEL.outboxPollIntervalMs` | 1500ms，随机落在巡检轮内 |
| 下发换会话 + 起始校验 | `channel-adapter.js:sendOutbound()` | `openConversation(exact)` → 等约 2s 稳定 → 二次校验 cid 未漂移，**之后键入期再无复核** |
| 键入 | `src/channels/xhs.js:sendText` / `src/core/dom.js:fillContentEditableHumanized` | 清空 → 逐段拟人 `insertText` + 高斯停顿 → `humanDelay('click')` → 点发送 |
| 时长预算 | `constants.js:humanSendTimeoutMs` | 注释自述「逐段停顿均值≈90ms/码点、上探 250ms/码点」，封顶 120s |
| 同源先例（已修的一侧） | `src/core/downlink.js:pollDownlink` 注释 | 已写明「多个会话并行 openConversation 会相互竞争，A 的文案被填进 B 的输入框 ⇒ 必须按会话串行」——**同一个坑，当时只堵了「下发 vs 下发」这条边** |
| 既有隐患注释 | `xhs.js:sendText` 注释 | 已承认「用户正在打字时 extension 同时下发」导致内容拼接，用 `clearBefore` 清空解决；**但没解决「输入框属于哪个会话」** |
| 键入主体（口径确认） | `downlink.js:pollDownlink` → `channel-adapter.js:sendOutbound` | 下发转发整条链路（轮询 outbox → 切会话 → 键入 → 点击 → 回查 → 自报上行）都由扩展自己实现，**键入是脚本自动的、没有人工参与** ⇒ 互斥只需覆盖「下发在途」这一侧，不必引入「人工草稿」判据 |

## Root Cause Hypothesis

**置信度：高。**

三条独立的判断缺口叠加：

1. **缺互斥**：`sendOutbound` 没有任何"我正在用 DOM"的状态位，巡检判据只有 `_syncingAll`（全量同步）一条，临界区清单里没有"下发中"。
2. **复核位置错**：`sendOutbound` 的会话漂移校验只在 `openConversation` 后 2s 内跑一次（`channel-adapter.js` 的 `sendConvDrift`），真正危险的键入期（数秒到上百秒）完全裸奔；填字与点击在同一次 `rawSendText` 调用内，中间没有复核点。
3. **失败被记账吃掉**：点击漂移后，`_verifySendLanded` 在**错误会话**里数气泡 ⇒ 判"未见"；而该设计明确"未见只记结论位、不重发"，`ok=true` 照样 `ack` ⇒ 服务端记"已送达"，实际发给了别人，且永不重发。线上不会报错，所以只能靠客户投诉发现。

对称方向还有一个同源缺陷：`_patrolVisit` 在 `openConversation` 之后 `sleep(≤600ms)` 再 `_collectUnseenText()`，然后把 batch 挂在 `openConversation` 返回的 `opened` 名下上行。这 600ms 里若下发把会话切走，采集到的就是**另一个会话**的消息，却记在 `opened` 头上 ⇒ 串台（AI 拿 B 的话当 A 的上下文回复）。

## Proposed Remediation

优先级按**不可逆性**排序：**下发 > 巡检**。巡检是可延迟的只读扫描（晚一轮只是时延，`seenNodes` + 稳定键去重保证延迟不漏不重）；下发是不可撤销的写入（错发人＝隐私事故）。所以**让巡检让路**，而不是让发送等待（单向让路，无反向等待 ⇒ 无死锁）。下发转发与键入都由扩展自己实现（见上方「键入主体」一行），因此互斥只需覆盖「下发在途」。

1. `sendOutbound` 全程持 `_outboundInFlight`（`beginOutbound/endOutbound`），覆盖切会话→稳定等待→键入→点击→回查→自报上行入队整段；释放后只剩纯 HTTP（uplink 合并窗 + outbox ack），不碰 DOM。
2. 换会话唯一入口 `openConversation` 收口 `preemptible`：巡检/巡检收尾回切/全量同步传 `true`（遇锁直接让路，下一轮/下个窗口再来），下发传 `false`（永不让路）。
3. 让路粒度是**每次切换**而非整轮：`_waitForSwitchWindow` 在发送间隙里推进巡检，避免下行密集时整轮饿死；等不到窗口就让整轮作废，下个轮询周期再来。
4. **linger**（`BRIDGE_THREE_CHANNEL.outboundLingerMs` = 1500ms）：`endOutbound` 归零后仍算忙一小段。理由是 downlink 是**逐条** `sendOutbound` 的，两条消息之间有 `await cache.flush()` 的空档，不留尾巴巡检就会插进来切一次会话、下一条再切回去（正确性有 `assertConv` 兜底，但白跑两次切换）。取 1.5s 只盖「存储读+写+下一条进锁」的常态空档，**不盖**限速重试那种秒级等待——那一段本就该让巡检有机会干活。
5. 最后一道闸：各渠道 `sendText` 在**点发送前一刻**调 `assertConvOrAbort` 复核归属，漂移 ⇒ 清空输入框 + 抛 `conv-drift` ⇒ 本条不 ack、留 pending 重投（锁挡不住 SPA 自身改路由和用户手点）。
6. 对称方向：`_patrolVisit` 在采集前复核「还停在 opened」，不一致即丢弃（`_collectUnseenText` 是同步的，采集一开始就不会被打断，故只需采集前一次）。

**Alternatives rejected**：
- 给巡检加锁、让下发抢锁——引入排队与死锁面，而巡检本就允许延迟。
- 「人工键入/草稿」让路层（首版曾实现 `isHumanTyping` + `draftQuietMs/draftStaleMs` 与 document 级键入监听）——按用户口径移除：这条链路的键入全部由扩展完成，混进「人的草稿」判据换不来对下发链路的任何保护，却会在未知页面上把巡检按停（contenteditable 误判）。代码更小、行为更可预测。
- 只靠 linger 不靠锁 / 只靠锁不加 linger——前者盖不住长键入窗（3–9s ≫ 1.5s），后者盖不住批量的条间空档，故两者都要。
