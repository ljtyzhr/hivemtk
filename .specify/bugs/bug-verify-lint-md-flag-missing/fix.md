# Fix Report: bug-verify.sh 补上文档已承诺的 --lint-md 档

- **Slug**: bug-verify-lint-md-flag-missing
- **Fixed**: 2026-10-14
- **Assessment**: [assessment.md](assessment.md)
- **Status**: applied
- **Applied in commit**: `642b0ae8` — `fix(verify): bug-verify.sh 补上文档已承诺的 --lint-md 档`（2026-10-14）

## Summary

`scripts/bug-verify.sh` 的文件头把 `--lint-md` 写成正式选项（L20-22 给了存在理由：「`make audit` 里没有 markdownlint，它唯一的执行点是 CI 的 `Markdown Lint` 作业 ⇒ 不带这一档，`**/*.md` 的形状违规只能等 CI 变红才被发现」），但实现层三处同时缺失：

| 层 | 位置 | 缺失 |
|----|------|------|
| 文档层 | `scripts/bug-verify.sh:20-22` 文件头 | 已有（承诺了） |
| 选项清单 | `usage()` | 没有 `--lint-md` |
| 解析层 | 参数 `case` | 没有 `--lint-md` 分支 ⇒ 落进 `*)` 未知参数分支 |
| 调用层 | `run_step` 调用点 | 脚本内 `markdownlint` 出现 0 次 |

结果：`/fixbug` 协议第 6 步那条强制命令
`bash scripts/bug-verify.sh --slug <slug> --packages "internal/app" --lint-md --push-plan`
直接印 `未知参数: --lint-md` → `usage()` → `exit 2`。

`exit 2` 属于「前提不满足」而非「通过」，所以**不是静默假绿**，但该命令不可用 ⇒ `/fixbug` 闭环第 6 步无法执行。

修法：把这一档补齐（变量 + 解析 + 选项文案 + 调用点），并加一条本脚本原本就缺的能力（红因前缀模式），否则这一档红了只会印「见 <log>」，违背本脚本「红因当场读出来」的约定。

## Changes

| 文件 | 改动 |
|------|------|
| `scripts/bug-verify.sh` | ① 变量区加 `LINT_MD=0`；② 解析 `case` 加 `--lint-md) LINT_MD=1; shift ;;`；③ `usage()` 选项清单补 `--lint-md` 一行（与文件头文案对齐）；④ `make -k audit` 之后新增 markdownlint-cli2 档 `run_step "markdownlint-cli2 (--lint-md)" 14-markdownlint.log "${WORK}" npx -y markdownlint-cli2`（跑在干净克隆里，与其余各档同一原则）；⑤ `run_step` 新增 `EXTRA_REASON_RE` 可选前缀模式 |

改动量：`scripts/bug-verify.sh | 22 ++++++++++++++++++++++-`（+21 / -1）。

## Diff Highlights

变量与解析：

```diff
 SLUG=''
 PACKAGES=''
 RACE=0
 SKIP_AUDIT=0
+LINT_MD=0
 PUSH_PLAN=0
 KEEP=0
 DIRTY_LIST=()
+
+# run_step 拼红因 grep 时用的可选前缀模式。默认必须「永不匹配」：空串会让那个
+# 交替表达式变成 `|undefined:|...`，开头的空分支匹配一切，于是任何一档红了都把
+# 日志头三行当红因印出来。
+EXTRA_REASON_RE='ZZZ_NEVER_MATCH_ZZZ'
 
 while [[ $# -gt 0 ]]; do
     case "$1" in
         --slug)       SLUG="${2:-}"; shift 2 ;;
         --packages)   PACKAGES="${2:-}"; shift 2 ;;
         --race)       RACE=1; shift ;;
         --skip-audit) SKIP_AUDIT=1; shift ;;
+        --lint-md)    LINT_MD=1; shift ;;
         --push-plan)  PUSH_PLAN=1; shift ;;
```

红因提取（加可选前缀，默认永不匹配）：

```diff
-        reason="$(grep -m3 -E -- 'undefined:|cannot find|Error:|FAIL|panic:|\[ERROR\]' "${log}" | tr '\n' ';')"
+        reason="$(grep -m3 -E -- "${EXTRA_REASON_RE}|undefined:|cannot find|Error:|FAIL|panic:|\[ERROR\]" "${log}" | tr '\n' ';')"
```

新档（放在 `make -k audit` 之后）：

```diff
+if [[ "${LINT_MD}" -eq 1 ]]; then
+    # markdownlint 的红因行形如 `MD047/single-trailing-newline`，上面那组通用模式一个都匹配不到，
+    # 不补这个变量的话这一档红了只会印「见 <log>」，违背本脚本「红因当场读出来」的约定。
+    EXTRA_REASON_RE='MD[0-9]{2}/'
+    run_step "markdownlint-cli2 (--lint-md)" 14-markdownlint.log "${WORK}" npx -y markdownlint-cli2
+    EXTRA_REASON_RE='ZZZ_NEVER_MATCH_ZZZ'
+fi
```

`EXTRA_REASON_RE` 默认值刻意设为永不匹配串而非空串：空串会让交替表达式变成 `|undefined:|...`，开头的空分支匹配一切，于是任何一档红了都会把日志头三行当红因印出来（假红因）。

## Tests Added or Updated

- 未新增测试文件（被测对象是 shell 脚本的开关行为，验证靠命令级回归，见 [test.md](test.md)）。
- `scripts/bug-verify.sh` 改动行数：+21 / -1。

## Local Verification

见 [test.md](test.md)。

## Deviations from Assessment

- assessment 的 5 步 remediation 全部落地，无删减。
- 额外补了 assessment 未明说、但实现里必须的第 5 步扩展：`EXTRA_REASON_RE` 的**默认值必须是永不匹配串**。assessment 只说「加一个可选 `EXTRA_REASON_RE` 变量拼进那个 `grep`」；若默认空串，会让交替表达式出现开头的空分支，匹配一切 ⇒ 所有档的红因都变成日志头三行 ⇒ 假红因。实现用 `ZZZ_NEVER_MATCH_ZZZ` 规避。

## Follow-ups

- 无。`make audit` 整链里仍没有 markdownlint 档 —— 这是**刻意**的：`--lint-md` 是显式点名档，CI 的 `Markdown Lint` 作业才是常规执行点；两处不重复。
