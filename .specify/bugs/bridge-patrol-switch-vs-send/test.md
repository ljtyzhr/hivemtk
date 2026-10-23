# Bug Verification: 换会话互斥（巡检让路 + 点击前复核）

- **Slug**: bridge-patrol-switch-vs-send
- **Tested**: 2026-10-23T21:05:00+08:00
- **Assessment**: ./assessment.md
- **Fix**: ./fix.md
- **Result**: verified（本地；无线上环境可复现该竞态，取证以用例 + 变异杀为主）

## Summary

回归用例锁住四条腿：①下发持锁期间巡检一律不换会话；②`endOutbound` 后的 linger 期内仍算忙（换会话继续让路），尾巴过期即恢复；③点发送前会话归属复核（漂移 ⇒ 清框 + `conv-drift` + 不 ack 不回声）；④对称方向——巡检采集前复核，漂移即丢弃不上行。另有一例验"短时持锁 + linger 释放后巡检在同一轮里等到窗口就继续"，证明让路是**每次切换**粒度、不会因为一次下发整轮作废。四条腿各自做了"改回实现即红"的变异验证。

## 口径（重要）

下发转发与键入全由扩展完成（无人工参与），因此让路判据只有 `isSendBusy()` 一条；首版的「人工键入/草稿」用例（3 条：草稿让路、草稿超上界、人工长输入等窗口到点）随该层一并删除。用例数由 10 条变为 **9 条**：删 3 条人工键入相关、新增 1 条 linger、原「等窗口有上界」改写为「下发短时持锁释放后巡检继续」。

## Checks Performed

| Check | Command / Action | Result | Notes |
|-------|------------------|--------|-------|
| 新增回归用例 | `npx vitest run test/adapter-b25-conv-lock.test.js` | pass | 9/9 |
| 全量回归 | `npx vitest run`（bridge 全部） | pass | **72 文件 / 878 passed, 7 skipped, 0 failed** |
| Lint | `npx eslint src test scripts --max-warnings=80` | pass | 0 error，69 warning 全为既有风格告警，新增/改动文件 0 告警 |
| 构建 | `npm run build` | pass | 8 个产物生成，`build done` |
| 死代码清理 | `grep -rn "humanTyping\|isHumanTyping\|readMessageInputDraft\|getInputDraftText\|draftQuietMs\|draftStaleMs" src test` | pass | 0 命中（`dom.js` 的 `readMessageInputDraft`、`popup` 的"输入中"显示、骨架里的 `getInputDraftText` 钩子均已移除） |
| 变异杀 ①（去掉 `openConversation` 的 preemptible 判定） | `if (preemptible)` → `if (false && preemptible)` | pass | 2 红：用例 1、3 |
| 变异杀 ②（去掉巡检等窗口） | `_waitForSwitchWindow(...)` 开头直接 `return null` | pass | 3 红：用例 2、3、4 |
| 变异杀 ③（去掉 linger） | `endOutbound` 里 `_outboundLingerUntil = 0` | pass | 3 红：用例 3、4、8 |
| 变异杀 ④（不把 assertConv 传给渠道 sendText） | `this.rawSendText(sendText, { expectConv: conv, assertConv })` → `this.rawSendText(sendText)` | pass | 2 红：用例 7 + `batch3-hygiene` 静态契约 |
| 变异杀 ⑤（去掉巡检采集前复核） | `if (cur && cur !== opened)` → `if (false && …)` | pass | 1 红：用例 9 |
| 静态契约（渠道侧） | `test/b7-humanized-typing.test.js` | pass | 五个渠道的 `assertConvOrAbort(` 必须存在且**排在点发送之前** |
| 静态契约（下发链路） | `test/batch3-hygiene.test.js` | pass | `withTimeout(rawSendText(..., {expectConv, assertConv}))` 不得被绕过；`assertConv` 必须与 `rawSendText` 同段且在它之前 |

## 用例清单（test/adapter-b25-conv-lock.test.js）

1. 下发持锁期间换会话一律让路：`openConversation` 返回 null、活动会话不动、`yielded.sendInFlight=1`
2. 巡检一轮在下发在途时整轮让路：`visited=0`、`yielded='send-in-flight'`、会话不变
3. `endOutbound` 后 linger 未过仍算忙：`isSendBusy()=true`、换会话继续让路、巡检整轮让路；尾巴过期 ⇒ `sendBusy=false` 且能切会话
4. 下发短时持锁（含 linger）释放后，巡检在同一轮里等到窗口就继续：`yielded=null`、`visited=1`、切换序列 `['b2','a1']`、`elapsed ≥ outboundLingerMs`
5. 键入期会话漂移 ⇒ 清空输入框 + 抛 `conv-drift`（不点发送）
6. 未漂移 ⇒ 原样放行、不碰输入框
7. `sendOutbound` 把 `assertConv` 传给渠道 `sendText`：漂移 ⇒ `ok=false`/`drifted=true`/`notFound=false`/无 AGENT 回声帧
8. `sendOutbound` 全程持锁：键入期间 `isSendBusy()=true`；结束后只剩 linger 尾巴，尾巴过期才 `false`（否则巡检永久让路）
9. `_patrolVisit` 打开会话后、采集前被切走 ⇒ 不上行（`drifted=true`）

## Output Excerpts

```
RUN  v4.1.11 .../user-web/bridge
 Test Files  72 passed (72)
      Tests  878 passed | 7 skipped (885)
```

```
$ npx eslint src test scripts --max-warnings=80
✖ 69 problems (0 errors, 69 warnings)
```

## 遗留 / 观察

- 该竞态在**真实平台页面**上的最终确认需要真人/真机复核（本地 jsdom 只能证"逻辑上不再允许发生"，不能证"平台 SPA 表现"）。建议灰度时看 `patrolStatus.yielded`（`sendInFlight` 计数）与 `巡检让路：send-in-flight …` 日志，确认让路按预期出现且新消息仍在 ≤1 轮内上报。
- 巡检 `MAX_VISIT_PARALLEL=2` 的 semaphore 因循环内 `await visitPromise` 实际串行，是另一条独立竞态（巡检 vs 巡检），本次未处置，已在 assessment 中登记。
- 真人若恰好在同一输入框手打，扩展下发会 `clearBefore` 清框（既有行为，`xhs.js` 注释已承认）；移除「人工键入」层后不再为此让巡检让路——这是按用户口径的显式取舍。
