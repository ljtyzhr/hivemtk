# Bug Verification: 换会话互斥（巡检让路 + 点击前复核）

- **Slug**: bridge-patrol-switch-vs-send
- **Tested**: 2026-10-23T20:50:00+08:00
- **Assessment**: ./assessment.md
- **Fix**: ./fix.md
- **Result**: verified（本地；无线上环境可复现该竞态，取证以用例 + 变异杀为主）

## Summary

新增 10 条回归用例锁住四条腿：①下发持锁期间巡检一律不换会话；②人工键入/草稿期间巡检让路、草稿清空即恢复、草稿停留超上界后不再让路且不会无限期挂住；③点发送前会话归属复核（漂移 ⇒ 清框 + `conv-drift` + 不 ack 不回声）；④对称方向——巡检采集前复核，漂移即丢弃不上行。四条腿各自做了"改回实现即红"的变异验证。

## Checks Performed

| Check | Command / Action | Result | Notes |
|-------|------------------|--------|-------|
| 新增回归用例 | `npx vitest run test/adapter-b25-conv-lock.test.js` | pass | 10/10 |
| 全量回归 | `npx vitest run`（bridge 全部） | pass | **72 文件 / 879 passed, 7 skipped, 0 failed** |
| Lint | `npx eslint src test scripts --max-warnings=80` | pass | 0 error，69 warning 全为既有风格告警，新增文件 0 告警 |
| 构建 | `npm run build` | pass | 8 个产物生成，`build done` |
| 变异杀 ①（去掉 `openConversation` 的 preemptible 判定） | `if (preemptible)` → `if (false && preemptible)` | pass | 「下发持锁期间换会话一律让路」红 |
| 变异杀 ②（去掉巡检等窗口） | `_waitForSwitchWindow(...)` → `null` | pass | 让路四例全红（4 failed） |
| 变异杀 ③（不把 assertConv 传给渠道 sendText） | `rawSendText(sendText, {…})` → `rawSendText(sendText)` | pass | 「漂移 ⇒ ok=false/drifted=true」红 |
| 变异杀 ④（去掉巡检采集前复核） | 删除 `if (cur && cur !== opened) return …` | pass | 「_patrolVisit 采集前被切走 ⇒ 不上行」红 |
| 静态契约（渠道侧） | `test/b7-humanized-typing.test.js` 新增断言 | pass | 五个渠道的 `assertConvOrAbort(` 必须存在且**排在点发送之前** |
| 静态契约（下发链路） | `test/batch3-hygiene.test.js` 更新断言 | pass | `withTimeout(rawSendText(..., {expectConv, assertConv}))` 不得被绕过；`assertConv` 必须与 `rawSendText` 同段且在它之前 |

## 用例清单（test/adapter-b25-conv-lock.test.js）

1. 下发持锁期间换会话一律让路：`openConversation` 返回 null、活动会话不动
2. 巡检一轮在下发在途时整轮让路：`visited=0`、`yielded='send-in-flight'`、会话不变
3. 人工正在输入（输入框有草稿）⇒ 巡检让路；草稿清空后下一轮恢复（切换序列 `['b2','a1']`）
4. 草稿停留超过上界后不再让路（防残留草稿把巡检永久按停）
5. 等窗口有上界：人工一直输入时巡检不会无限期挂住，到点放弃本轮
6. 键入期会话漂移 ⇒ 清空输入框 + 抛 `conv-drift`（不点发送）
7. 未漂移 ⇒ 原样放行、不碰输入框
8. `sendOutbound` 把 `assertConv` 传给渠道 `sendText`：漂移 ⇒ `ok=false`/`drifted=true`/`notFound=false`/无 AGENT 回声帧
9. `sendOutbound` 全程持锁：键入期间 `isSendBusy()=true`，结束后必须放锁
10. `_patrolVisit` 打开会话后、采集前被切走 ⇒ 不上行（`drifted=true`）

## Output Excerpts

```
RUN  v4.1.11 .../user-web/bridge
 Test Files  72 passed (72)
      Tests  879 passed | 7 skipped (886)
```

```
$ npx eslint src test scripts --max-warnings=80
✖ 69 problems (0 errors, 69 warnings)
```

## 遗留 / 观察

- 该竞态在**真实平台页面**上的最终确认需要真人/真机复核（本地 jsdom 只能证"逻辑上不再允许发生"，不能证"平台 SPA 表现"。建议灰度时看 `patrolStatus.yielded` 与 `巡检让路：…` 日志，确认让路按预期出现且新消息仍在 ≤1 轮内上报。
- 巡检 `MAX_VISIT_PARALLEL=2` 的 semaphore 因循环内 `await visitPromise` 实际串行，是另一条独立竞态（巡检 vs 巡检），本次未处置，已在 assessment 中登记。
