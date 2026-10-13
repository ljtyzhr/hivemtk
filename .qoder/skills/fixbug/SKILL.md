---
name: fixbug
description: hivemtk 修 bug 的唯一入口。一条指令跑完 spec-kit 三段流（assess→fix→test）、守卫反向验证、确定性验证阶梯与双远端推送。当用户报修任何缺陷（"XX 返回 500"、"这个字段没落库"、"文档口径和代码不一致"、panic、错文案、门假绿）时使用。
---

# /fixbug —— 一条指令修完一个 bug

`CLAUDE.md` 规则3 把本文件指认为 bug 修复的唯一流程。用户只需要给一条指令，
其余全在这里；不要把流程拆回"先给我 slug"、"要不要跑审计"这类回问。

## 触发形态（三种都算同一条指令）

- `/fixbug slug=<kebab-slug> <一句话现象>`
- `/fixbug <一句话现象>` —— slug 由现象推导（kebab-case，动词/名词落在缺陷本身，
  如 `business-error-code-panic`），推导结果必须在回复第一句点名。
- 自然语言报修（不带任何参数）—— 同样进本流程；"没给 slug"不是退回问答的理由。

参数只有一个可选开关：`--keep-open`（只落 assessment.md，把 fix 留给下一次；
默认一定跑完整闭环）。

## 第 0 步：认领现场（本仓工作树与并行泳道共用）

1. `git rev-parse HEAD` 记下开工基线，`git status --porcelain -- <本轮要动的路径>` 逐个看是不是 ` M`。
2. 路径干净 ⇒ 直接改。
3. 路径正被并行泳道改（` M`）⇒ 走热文件四步：在 `git show HEAD:<file>` 的内容上只加自己的行
   → commit（`--stat` 必须零删除）→ 把自己的行合并回对方那份 → 比较改前/改后 `git diff`，
   两者应逐字相同（只差 blob 与 hunk 行号）。
4. 工作树里冒出的红先做归属判定：命中文件是 `??`（未跟踪）或 ` M` 就是别人的现场，
   只记录不代改；本卡范围内的红当轮修。

## 闭环六步（顺序不许换，每步的完成判据都写在表里）

| 步 | 动作 | 完成判据 |
|----|------|----------|
| 1 | `/speckit-bug-assess slug=<slug>` | `.specify/bugs/<slug>/assessment.md` 有 `- **Verdict**` 行 |
| 2 | 复现（红必须先在场） | 一段最小复现物 + 它本轮真打出来的红因原文（panic 串 / `--- FAIL` 行 / 门的 `命中 N 处`）。复现不出来时 verdict 只能是 `likely valid, needs reproduction`，且不许直接改代码 |
| 3 | `/speckit-bug-fix slug=<slug>` | `fix.md` 的 `- **Status**` 定稿；改动面 ⊆ assessment 的文件清单，超出的一律写进 Deviations |
| 4 | 守卫反向验证 | 把刚加的守卫/判据摘掉 ⇒ 新用例必须红成第 2 步那个**原始红因**；还原后 `md5` 与「自己注码前记的基线」和 `git show HEAD:<file>` 的 blob **两头都对账**（共享树下只比自己那份会把"对方撤码"读成"我复原成功"） |
| 5 | `/speckit-bug-test slug=<slug>` | `test.md` 的 `- **Result**` 定稿；这一步**只读不改源码**，每条读数都要附命令与产物行 |
| 6 | `bash scripts/bug-verify.sh --slug <slug> --packages "<受影响包>" --lint-md --push-plan` | rc=0。报告里每个读数都要点名它打印的 `MEASURED_COMMIT` |

第 4、5 步是本仓加给 spec-kit 的：上游三段流只规定"写报告"，不规定"证明这枚守卫真能拦"。
`make audit` 里没有 markdownlint（它唯一的执行点是 `Markdown Lint` 作业），所以第 6 步要带
`--lint-md`，否则 `**/*.md` 的形状违规只能等 CI 变红才被发现。

## 提交与推送（规则0 / 规则1）

- 只 `git add` 本轮点名的路径；commit 之前 `git diff --cached` 逐 hunk 认账 —— 同一个文件里
  压着对方的行也会被一起带走，`--stat` 只列文件名抓不到它。
- 待推清单逐笔 `git show --stat` 读一遍：含 DELETE / 迁移 / 权限类语义的那笔单拎出来问，
  不混在"推我的修复"里。
- 分两个远端各 fetch 一次（`git fetch upstream`、`git fetch gitee-upstream`，不是一条命令带两名）。
  远端有独有提交 ⇒ 不在脏树上硬合：在 `git clone --shared` 出来的干净克隆里 merge，再推。
  不改写已推的历史、不 rebase、不走 PR。
- 双远端都推完各读一次 `git ls-remote <remote> refs/heads/master`，用回读值收口。

## 终态只有两种

- `verified`：三段产物 + 第 6 步 rc=0 + 已推双远端。
- `blocked`：**只**允许用于本机真做不到的腿（要用户设备/真机/外部账号/上游平台权限/
  Xcode 许可未同意）。`test.md` 的 Residual Risks 里逐条写明是哪条腿、为什么、用什么代替。
  改动面大、要重跑电池、怕碰别人的文件都**不是** blocked 的理由。

## 不许

- 不许编辑 `assessment.md`（它是契约）；分歧记在 `fix.md` 的 Deviations。
- 不许"记录待办、下轮处理、专项重构轮"（规则2）。
- 不许拿工作树的绿当结论：工作树里压着别人未跟踪的文件时，那口绿可能验的不是你的字节。
- 不许把 `SKIP` 满屏读成绿：连不上测试库时用例是 Skip 不是 Fail，阶梯会先把端口探一遍。
- 报告里不许出现"应该/大概/理论上"——只有跑出来的读数才写，读数要带 `文件:行` 或产物原文。
