# Bug Assessment: bug-verify.sh 文档承诺的 --lint-md 开关根本没实现

- **Slug**: bug-verify-lint-md-flag-missing
- **Created**: 2026-10-14
- **Source**: 自查发现。执行 `/fixbug` 协议第 6 步时撞上。
- **Verdict**: valid
- **Severity**: medium

## Report

执行 CLAUDE.md 规则3 指定的三段流收尾命令

```bash
bash scripts/bug-verify.sh --slug <slug> --packages "internal/app" --lint-md --push-plan
```

脚本直接拒绝：

```
未知参数: --lint-md
用法: bash scripts/bug-verify.sh --slug <bug-slug> [选项]
  ...（选项列表里没有 --lint-md）
```

## Symptom

`scripts/bug-verify.sh` 自己的文件头把 `--lint-md` 写成了正式选项，并说明「不带这一档，`**/*.md` 的形状违规会一路活到 CI 才红」；但脚本实际既不认这个参数、也从不调用 markdownlint。预期：该开关存在并按其文档所述在干净克隆里跑 markdownlint。

## Reproduction

1. `cd hivemtk/ && bash scripts/bug-verify.sh --slug feishu-recall-bizcode-silent-success --packages "internal/app" --lint-md --push-plan`
2. 观察：印 `未知参数: --lint-md`，随后 `usage()` 并 `exit 2`。
3. 读 `scripts/bug-verify.sh` 文件头 L20-22 —— `--lint-md` 明明写在选项里。

## Suspected Code Paths

- `hivemtk/scripts/bug-verify.sh:20-22` — 文件头把 `--lint-md` 列为正式选项并给出存在理由
- `hivemtk/scripts/bug-verify.sh:59-68` — `usage()` 的选项清单里**没有** `--lint-md`
- `hivemtk/scripts/bug-verify.sh:80-97` — 参数解析 `case` 里**没有** `--lint-md` 分支，落进 `*)` 未知参数分支
- `hivemtk/scripts/bug-verify.sh:310-336` — `run_step` 与 build/vet/audit/test 各档调用点，**没有任何 markdownlint 调用**
- `hivemtk/.qoder/skills/fixbug/SKILL.md`（闭环六步第 6 步）— **强制要求**这条命令带上 `--lint-md`，并写明「`make audit` 里没有 markdownlint（它唯一的执行点是 `Markdown Lint` 作业），所以第 6 步要带 `--lint-md`，否则 `**/*.md` 的形状违规只能等 CI 变红才被发现」

## Root Cause Hypothesis

选项只被写进了文档层（文件头注释），从未落到实现层（`usage()` + 解析 `case` + `run_step` 调用点）——典型的文档与实现脱钩。置信度：**high**。三处实现层同时缺失，且全仓 `markdownlint` 在 `bug-verify.sh` 内出现 0 次（grep 可证），不是"实现了但有条件失效"。

## Proposed Remediation

**Preferred**：把这一档补齐，三处缺一不可。

1. 变量区加 `LINT_MD=0`。
2. 解析 `case` 加 `--lint-md) LINT_MD=1; shift ;;`。
3. `usage()` 选项清单补一行，与文件头文案对齐。
4. 在 `make -k audit` 之后、`go test` 附近加一档 `run_step "markdownlint-cli2" 14-markdownlint.log "${WORK}" npx -y markdownlint-cli2`（放干净克隆里跑，与其余各档同一原则）。
5. 顺带补 `run_step` 的红因提取：现有 `grep -m3 -E 'undefined:|cannot find|Error:|FAIL|panic:|\[ERROR\]'` **匹配不到** markdownlint 的 `MD0xx/rule-name` 行，会退化成只印 `见 <log>`。加一个可选的 `EXTRA_REASON_RE` 变量拼进该 `grep`，让这一档当场印出真红因。

**Alternatives**：
- 只改文档、删掉文件头里的 `--lint-md` 与 fixbug 的要求。能解一致性，但把一个真实缺口掩盖掉：markdown 形状违规仍然只能等 CI。与该开关存在的理由相悖，不推荐。

**Files likely to change**:
- `hivemtk/scripts/bug-verify.sh`

**Tests to add or update**:
- 红：`bash scripts/bug-verify.sh --slug <existing> --lint-md` → 当前 `未知参数` + rc=2
- 绿：同一命令修复后不再报未知参数，且 markdownlint 档出现在取证日志里
- 负：人为制造一处 MD047，确认该档 rc≠0 **且能当场印出 `MD047` 红因**（验证第 5 点不是空改）
- 回归：不带该开关时行为与修复前逐字一致（该开关必须是纯增量）

## Risks & Considerations

- **会被新开关扫红的存量文件**：全仓 markdownlint 当前若已有违规，一开这档就会把本轮无关的存量红带进判定。已实测：`npx -y markdownlint-cli2` 扫 260 个文件，仅 4 处 `MD047`（缺尾换行），且**全部**是本轮我自己刚写的文件；补齐尾换行后复跑为 `Summary: 0 issues in 0 files`。存量干净，不会引入无关红。
- `npx -y` 首次执行要联网取包，克隆内会慢一档；失败应记为该档 rc≠0（红因），不得当通过。
- 不改退出码语义：未知参数仍是 rc=2（前提不满足），本开关**不得**被做成"没实现也装作支持"。

## Open Questions

- 无。