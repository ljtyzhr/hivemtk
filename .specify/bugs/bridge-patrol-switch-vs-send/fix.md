# Bug Fix: 换会话互斥 —— 下发在途（含 linger）持锁，巡检让路

- **Slug**: bridge-patrol-switch-vs-send
- **Fixed**: 2026-10-23T21:00:00+08:00
- **Assessment**: ./assessment.md
- **Status**: applied

## Summary

给桥接扩展引入**单向换会话互斥**（下发 > 巡检）：`sendOutbound` 从 `openConversation` 起到「键入 + 点击 + `send_verified` 回查 + 自报上行入队」全程持 `_outboundInFlight`；`endOutbound` 归零后再留 `outboundLingerMs`(1500ms) 的 **linger 尾巴**，把 downlink 逐条发送之间的 `await` 空档也盖住（否则巡检会在两条消息之间插进来切一次会话、下一条再切回去）。换会话唯一入口 `openConversation` 增加 `preemptible` 判定，巡检/巡检收尾回切/全量同步遇「下发在途」一律让路（下一轮或下个窗口再来），下发永不因巡检让路；各渠道 `sendText` 在点发送前一刻复核会话归属，漂移则清空输入框并抛 `conv-drift`（不点、不 ack、留 pending 重投）；同时补上对称方向——巡检采集前复核"还停在 openConversation 打开的那个会话"，不一致即丢弃，杜绝串台上行。

这样"正在输入栏里打字却被跳到下一个用户"这条不可逆路径被三道闸门同时封住：①全程持锁（含 linger）让巡检根本切不走；②万一仍漂移（SPA 改路由、用户手点），点之前止损；③巡检侧采集前再复核一次，避免反向串台。

## 口径（重要）

下发转发**整条链路都是 bridge 自己实现的**（`PollingLoop` 拉 outbox → `pollDownlink` 按会话串行 → `sendOutbound` 切会话 → 渠道 `sendText` 逐段拟人 `insertText` → 点发送 → `_verifySendLanded` 回查 → `_emitMessage(SENDER.AGENT)` 自报上行），**键入是脚本自动的、没有人工参与**。因此需求就是一句：**下发在途 / 完成输入与上报之前，巡检不得切会话**。

首版曾额外实现「人工键入让路」层（`isHumanTyping` + `draftQuietMs`/`draftStaleMs` + document 级键入监听 + `readMessageInputDraft`）；按用户口径**已整体移除**——它换不来对下发链路的任何保护，却会在 contenteditable 误判的页面把巡检按停。巡检现在的让路判据只有一条：`isSendBusy()`。

## Changes

| File | Change | Notes |
|------|--------|-------|
| `user-web/bridge/src/core/channel-adapter.js` | modified | 新增 `beginOutbound/endOutbound/isSendBusy`（含 linger）、`convSwitchBlocker`、`_waitForSwitchWindow`、`OUTBOUND_LINGER_MS`；`openConversation` 增加 `preemptible`；`patrol` 每次切换前等窗口并在让路时返回 `yielded`；`_patrolVisit` 采集前复核归属；`sendOutbound` 拆为持锁外壳 + `_sendOutboundLocked`，`preemptible:false` 且把 `{expectConv, assertConv}` 传给渠道 `sendText`；`patrolStatus` 增加 `yielded.sendInFlight`/`sendBusy` |
| `user-web/bridge/src/core/constants.js` | modified | `PATROL_DEFAULTS` 新增 `switchBlockWaitMs 20000` / `switchBlockPollMs 500`；`BRIDGE_THREE_CHANNEL` 新增 `outboundLingerMs 1500`（含取 1.5s 的论证：盖条间空档、不盖限速重试） |
| `user-web/bridge/src/core/dom.js` | modified | 新增 `clearMessageInput(el)`、`assertConvOrAbort(el, assertConv)`（首版的 `readMessageInputDraft` 已随人工键入层删除） |
| `user-web/bridge/src/channels/{douyin,kuaishou,tiktok,xianyu,xhs}.js` | modified | `sendText(text, opts)` 签名 + 点发送前 `assertConvOrAbort(input, opts.assertConv)` |
| `user-web/bridge/src/popup/index.js` | modified | 巡检状态面板显示让路次数（下发中） |
| `user-web/bridge/test/adapter-b25-conv-lock.test.js` | added test | 9 条回归用例（见 test.md） |
| `user-web/bridge/test/{adapter,b7-humanized-typing,batch3-hygiene,douyin-walker}.test.js` | modified | 对齐新签名（`sendText(text, opts)`）并给静态契约加"复核必须在点击前"的新牙 |

## Diff Highlights

```js
// ① 下发 = 临界区（不是只包住"切会话"那一瞬，而是整段键入期）
async sendOutbound(text, targetConvId, opts = {}) {
  this.beginOutbound();
  try { return await this._sendOutboundLocked(text, targetConvId, opts); }
  finally { this.endOutbound(); }
}

// ② 释放后留 linger 尾巴，盖住 downlink 逐条发送之间的 await 空档
endOutbound() {
  this._outboundInFlight = Math.max(0, (this._outboundInFlight || 0) - 1);
  if (this._outboundInFlight === 0) {
    this._outboundStartedAt = 0;
    this._outboundLingerUntil = Date.now() + OUTBOUND_LINGER_MS;
  }
}
isSendBusy() {
  if ((this._outboundInFlight || 0) > 0) return true;
  return Date.now() < (this._outboundLingerUntil || 0);
}

// ③ 换会话唯一入口收口：谁能让路由调用点声明；判据只有一条「下发是否占着页面」
async openConversation(cid, { ..., preemptible = true } = {}) {
  if (preemptible) {
    const blocker = this.convSwitchBlocker();      // 'send-in-flight' | null
    if (blocker) { this._noteYield(blocker); return null; }
  }
  ...
}

// ④ 巡检每次切换前等窗口（粒度=每次切换，不在发送间隙饿死）
const blocker = await this._waitForSwitchWindow(switchBlockWaitMs, switchBlockPollMs);
if (blocker) { yieldReason = blocker; yieldSkipped = targets.length - targets.indexOf(conv); break; }

// ⑤ 点发送前最后一道闸（渠道侧）
await humanDelay('click', { channel: CHANNELS.XHS, account: getAccountId() });
assertConvOrAbort(input, opts.assertConv);   // 漂移 ⇒ 清空输入框 + throw code='conv-drift'
const sendBtn = findSendButton();

// ⑥ 对称方向：巡检采集前复核归属
const cur = this.getConversationId();
if (cur && cur !== opened) return { ok: false, newCount: 0, drifted: true };
const batch = this._collectUnseenText();
```

## Local Verification

- `npx vitest run`（bridge 全量）：**72 个文件 / 878 passed, 7 skipped, 0 failed**（含本批 9 条）。
- `npx eslint src test scripts --max-warnings=80`：**0 error**（69 warning 全为既有风格告警）。
- `npm run build`：8 个产物全部生成，`build done`。
- 回归有效性：新增用例里"持锁让路 / linger 让路 / 点击前复核 / 采集前复核"四条腿各自独立可红（改回任一实现即红，见 test.md）。

## Notes / 取舍

- 让路意味着下行密集时巡检通过率下降，但**每一次切换**都会重新抢窗口，间隙（含 linger 过期后）即可推进；下发一直占着页面才整轮作废，不会把"误发"换成"新消息永不上报"。
- linger 取 1.5s 是"够盖条间空档、又不至于让一次长限速把巡检饿死"的折中；它只是**延后**巡检，不阻塞下发。
- 移除「人工键入」层后，真人若恰好在同一输入框手打，扩展下发会 `clearBefore` 清框——这是修复前就有的既有行为（`xhs.js` 注释已承认），不在本次范围内，也不属于本 bug 的不可逆路径。
- 本仓工作区当时存在另一并发会话的在途改动（002-multi-type-messages 等），本批改动与之同文件共存，`npx vitest run` 全绿证明两者不冲突；提交范围需与并发会话协调。
