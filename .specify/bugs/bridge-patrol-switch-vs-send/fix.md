# Bug Fix: 换会话互斥 —— 下发/人工键入持锁，巡检让路

- **Slug**: bridge-patrol-switch-vs-send
- **Fixed**: 2026-10-23T20:45:00+08:00
- **Assessment**: ./assessment.md
- **Status**: applied

## Summary

给桥接扩展引入**单向换会话互斥**（人工键入 > 下发 > 巡检）：`sendOutbound` 全程持 `_outboundInFlight`；换会话唯一入口 `openConversation` 增加 `preemptible` 判定，巡检/巡检收尾回切/全量同步遇锁或遇人工草稿一律让路（下一轮或下个窗口再来），下发永不因巡检让路；各渠道 `sendText` 在点发送前一刻复核会话归属，漂移则清空输入框并抛 `conv-drift`（不点、不 ack、留 pending 重投）；同时补上对称方向——巡检采集前复核"还停在 openConversation 打开的那个会话"，不一致即丢弃，杜绝串台上行。

这样"正在输入栏里打字却被跳到下一个用户"这条不可逆路径被三道闸门同时封住：①全程持锁让巡检根本切不走；②草稿/键入活动让巡检主动让路；③万一仍漂移（SPA 改路由、用户手点），点之前止损。

## Changes

| File | Change | Notes |
|------|--------|-------|
| `user-web/bridge/src/core/channel-adapter.js` | modified | 新增 `beginOutbound/endOutbound/isSendBusy`、`getInputDraftText`/`isHumanTyping`/`convSwitchBlocker`/`_waitForSwitchWindow`、`_bindInputActivityWatch`（document 捕获级键入监听）；`openConversation` 增加 `preemptible`；`patrol` 每次切换前等窗口并在让路时返回 `yielded`；`_patrolVisit` 采集前复核归属；`sendOutbound` 拆为持锁外壳 + `_sendOutboundLocked`，`preemptible:false` 且把 `{expectConv, assertConv}` 传给渠道 `sendText`；`patrolStatus` 增加 `yielded`/`sendBusy` |
| `user-web/bridge/src/core/constants.js` | modified | `PATROL_DEFAULTS` 新增 `draftQuietMs 15000` / `draftStaleMs 300000` / `switchBlockWaitMs 20000` / `switchBlockPollMs 500` |
| `user-web/bridge/src/core/dom.js` | modified | 新增 `readMessageInputDraft()`、`clearMessageInput(el)`、`assertConvOrAbort(el, assertConv)` |
| `user-web/bridge/src/channels/{douyin,kuaishou,tiktok,xianyu,xhs}.js` | modified | `sendText(text, opts)` 签名 + 点发送前 `assertConvOrAbort(input, opts.assertConv)` |
| `user-web/bridge/src/popup/index.js` | modified | 巡检状态面板显示让路计数（下发中/输入中） |
| `user-web/bridge/test/adapter-b25-conv-lock.test.js` | added test | 10 条回归用例（见 test.md） |
| `user-web/bridge/test/{adapter,b7-humanized-typing,batch3-hygiene,douyin-walker}.test.js` | modified | 对齐新签名（`sendText(text, opts)`）并给静态契约加"复核必须在点击前"的新牙 |

## Diff Highlights

```js
// ① 下发 = 临界区（不是只包住"切会话"那一瞬，而是整段键入期）
async sendOutbound(text, targetConvId, opts = {}) {
  this.beginOutbound();
  try { return await this._sendOutboundLocked(text, targetConvId, opts); }
  finally { this.endOutbound(); }
}

// ② 换会话唯一入口收口：谁能让路由调用点声明
async openConversation(cid, { ..., preemptible = true } = {}) {
  if (preemptible) {
    const blocker = this.convSwitchBlocker();      // 'send-in-flight' | 'human-typing' | null
    if (blocker) { this._noteYield(blocker); return null; }
  }
  ...
}

// ③ 巡检每次切换前等窗口（粒度=每次切换，不在发送间隙饿死）
const blocker = await this._waitForSwitchWindow(switchBlockWaitMs, switchBlockPollMs);
if (blocker) { yieldReason = blocker; yieldSkipped = targets.length - targets.indexOf(conv); break; }

// ④ 点发送前最后一道闸（渠道侧）
await humanDelay('click', { channel: CHANNELS.XHS, account: getAccountId() });
assertConvOrAbort(input, opts.assertConv);   // 漂移 ⇒ 清空输入框 + throw code='conv-drift'
const sendBtn = findSendButton();

// ⑤ 对称方向：巡检采集前复核归属
const cur = this.getConversationId();
if (cur && cur !== opened) return { ok: false, newCount: 0, drifted: true };
const batch = this._collectUnseenText();
```

## Local Verification

- `npx vitest run`（bridge 全量）：**71 个文件 / 866 passed, 7 skipped, 0 failed**（含新增 10 条）。
- `npx eslint src test scripts --max-warnings=80`：**0 error**（69 warning 全为既有风格告警）。
- `npm run build`：8 个产物全部生成，`build done`。
- 回归有效性：新增用例里"让路/持锁/点击前复核/采集前复核"四条腿各自独立可红（改回任一实现即红，见 test.md）。

## Notes / 取舍

- 让路意味着下行密集时巡检通过率下降，但**每一次切换**都会重新抢窗口，间隙即可推进；人工长时间输入才整轮作废（并被"草稿停留上界"兜底），不会把"误发"换成"新消息永不上报"。
- 本仓工作区当时存在另一并发会话的在途改动（002-multi-type-messages 等），本批改动与之同文件共存，`npx vitest run` 全绿证明两者不冲突；提交范围需与并发会话协调。
