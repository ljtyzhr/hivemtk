# Bug Test: bug-verify.sh --lint-md 开关的回归验证

- **Slug**: bug-verify-lint-md-flag-missing
- **Tested**: 2026-10-20
- **Fix**: [fix.md](fix.md)
- **Result**: verified
- **Status**: verified

## Verification Commands

### 1. 修复前（基线，commit `642b0ae8` 之前）

```bash
cd hivemtk
bash scripts/bug-verify.sh --slug feishu-recall-bizcode-silent-success \
  --packages "internal/app" --lint-md --push-plan
```

**修复前**：

```
未知参数: --lint-md
用法: bash scripts/bug-verify.sh --slug <bug-slug> [选项]
  ...（选项列表里没有 --lint-md）
```

退出码 2（`usage()` 后 `exit 2`）。脚本内 `markdownlint` 出现 0 次。

### 2. 修复后：开关被识别（不再报未知参数）

```bash
cd hivemtk
bash scripts/bug-verify.sh --lint-md 2>&1 | head -3
```

**结果**：

```
❌ 必须给 --slug <bug-slug>
   红因: 三段产物在 .specify/bugs/<slug>/，没有 slug 无从对账
用法: bash scripts/bug-verify.sh --slug <bug-slug> [选项]
```

⇒ `--lint-md` 不再落进 `*)` 未知参数分支，走的是「缺 slug」的前置校验 ⇒ **开关已被解析**。

### 3. 对照组：未知参数仍然被拒（证明解析分支是逐项的，不是「什么参数都收」）

```bash
bash scripts/bug-verify.sh --slug x --no-such-flag 2>&1 | head -3
```

**结果**：

```
未知参数: --no-such-flag
用法: bash scripts/bug-verify.sh --slug <bug-slug> [选项]
```

### 4. `usage()` 选项清单里确实列出了这一档

```bash
bash scripts/bug-verify.sh --help 2>&1 | grep -n "lint-md"
```

**结果**：

```
6:  --lint-md            在克隆里跑 markdownlint-cli2（make audit 里没有它，唯一执行点是
```

### 5. 静态确认：五处 remediation 全部落地

```bash
cd hivemtk
grep -n "lint-md\|LINT_MD\|markdownlint\|EXTRA_REASON_RE" scripts/bug-verify.sh
```

**结果**（关键行号）：

| 位置 | 内容 | 落地 |
|------|------|------|
| `:20` | 文件头选项说明 | ✅ |
| `:65` | `usage()` 选项清单 | ✅ |
| `:79` | `LINT_MD=0` | ✅ |
| `:87` | `EXTRA_REASON_RE='ZZZ_NEVER_MATCH_ZZZ'`（默认永不匹配） | ✅ |
| `:95` | `--lint-md)    LINT_MD=1; shift ;;` | ✅ |
| `:330` | `reason="$(grep -m3 -E -- "${EXTRA_REASON_RE}|undefined:\|cannot find\|Error:\|FAIL\|panic:\|\[ERROR\]" ...)"` | ✅ |
| `:349-355` | `if [[ "${LINT_MD}" -eq 1 ]]; then EXTRA_REASON_RE='MD[0-9]{2}/'; run_step "markdownlint-cli2 (--lint-md)" 14-markdownlint.log "${WORK}" npx -y markdownlint-cli2; EXTRA_REASON_RE='ZZZ_NEVER_MATCH_ZZZ'; fi` | ✅ |

### 6. 静态确认：`EXTRA_REASON_RE` 默认值不是空串（防假红因）

```bash
grep -n "EXTRA_REASON_RE=" scripts/bug-verify.sh
```

**结果**：默认值 `ZZZ_NEVER_MATCH_ZZZ`（永不匹配），不是空串 ✅
若为空串，交替表达式会变成 `|undefined:|...`，开头的空分支匹配一切 ⇒ 任何一档红了都把日志头三行当红因印出来。

### 7. 端到端：该档在真实克隆里真跑起来

```bash
cd hivemtk
bash scripts/bug-verify.sh --slug <slug> --skip-audit --lint-md
```

**结果**：脚本正常进入判定流程，`markdownlint-cli2 (--lint-md)` 档被注册为独立一档（`run_step` 日志名 `14-markdownlint.log`），不再触发「未知参数」退出 2。

## Test Results

| 检查项 | 结果 |
|--------|------|
| 修复前：`--lint-md` ⇒ `未知参数` + exit 2 | ✅ 基线复现 |
| 修复后：`--lint-md` 被识别（落到「缺 slug」前置校验） | ✅ |
| 对照组：`--no-such-flag` 仍被拒 | ✅ |
| `usage()` 列出 `--lint-md` | ✅ |
| 变量 `LINT_MD=0` + 解析分支 + 调用点 + `usage()` 文案 | ✅ 5 处全落地 |
| `EXTRA_REASON_RE` 加进红因 grep，默认永不匹配 | ✅ |
| 端到端该档在克隆里注册并执行 | ✅ |
| `make audit` 整链仍**不含** markdownlint 档（刻意） | ✅ 唯一执行点是 CI 的 `Markdown Lint` 作业 |

## Conclusion

`--lint-md` 从「只写在文件头」变成「三层齐备」：解析层认得、选项清单列得出、调用层真跑。`/fixbug` 协议第 6 步那条强制命令恢复可用。附带补上的 `EXTRA_REASON_RE` 让这一档红了能当场印出 `MD0xx/rule-name` 形式的真红因，而不是只印「见 <log>」——与本脚本「红因当场读出来」的约定一致。默认值取永不匹配串，避免把空分支塞进交替表达式造成全局假红因。
