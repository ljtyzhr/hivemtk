# Bug Assessment: 9 处 markdownlint 存量红使 CI Markdown Lint 作业与 --lint-md 档永久判红

- **Slug**: markdownlint-preexisting-red-blocks-ci
- **Created**: 2026-10-20
- **Source**: 自查（前置任务一第 3 段对抗性检查期间跑 `npx -y markdownlint-cli2` 与 `scripts/bug-verify.sh --lint-md`）
- **Verdict**: valid
- **Severity**: medium

## Report (verbatim or summarized)

本轮为「前置任务一」第 3 段（对抗性检查、审核、测试）的延伸。提交前文档门与 CI 作业都依赖 markdownlint：CI `.github/workflows/markdownlint.yml` 的 `markdownlint` job 用 `DavidAnson/markdownlint-cli2-action@v24`，`continue-on-error: false`（阻断），push paths 含 `**/*.md`；`scripts/bug-verify.sh --lint-md` 档（HEAD 已实现，commit 642b0ae8）也跑同一工具。实测发现仓内 HEAD 存量红 9 处 ⇒ 任何含 `.md` 改动的推送都会让该 CI 作业判红，`--lint-md` 档也永远红。

## Symptom

```
npx -y markdownlint-cli2            # 仓库根
```

输出 `Summary: 9 issues in 9 files`（306 files）：

- 8 × `MD047/single-trailing-newline`（文件末尾缺换行）
  - `.specify/002-modular-subscription/plan.md:239:165`
  - `.specify/002-modular-subscription/spec.md:230:209`
  - `.specify/bugs/_md047_probe.md:3:36`
  - `docs/architecture/ALL_ENDS_CAPABILITY_ANALYSIS.md:302:60`
  - `docs/architecture/LLM_BILLING_FLOW.md:247:12`
  - `docs/architecture/MODULAR_MODULES.md:543:262`
  - `docs/architecture/MODULE_MARKET_UI.md:162:49`
  - `docs/architecture/PLATFORM_USER_COMMUNICATION.md:291:28`
- 1 × `MD004/ul-style`（期望 dash，实际 plus）
  - `docs/handbook/SAAS_PRODUCTION_CONSTRAINTS.md:85:3`

`scripts/bug-verify.sh --slug stale-test-fixtures-after-tenant-kv-split --skip-audit --lint-md` 实测：markdownlint 档 `rc=1`，红因 `Summary: 9 issues in 9 files`。

## Reproduction

```bash
cd <repo root> && npx -y markdownlint-cli2 ; echo "RC=$?"
# => Summary: 9 issues in 9 files, RC=1
```

## Suspected Code Paths

- 8 个 MD047 文件末尾字节实测（python `repr`）全部缺 `\n`：
  - `.specify/002-modular-subscription/plan.md` 末尾 `…对应任务已在 tasks.md 标注。`
  - `.specify/002-modular-subscription/spec.md` 末尾 `…路由闸 fail-closed 刻意相反）。`
  - `.specify/bugs/_md047_probe.md` 末尾 `…用来验证 --lint-md 档会真的变红。`
  - `docs/architecture/ALL_ENDS_CAPABILITY_ANALYSIS.md` 末尾 `…+ 缺口清单 |`
  - `docs/architecture/LLM_BILLING_FLOW.md` 末尾 `…合并会迫使两边语义混合。`
  - `docs/architecture/MODULAR_MODULES.md` 末尾 `…§零 结论表收费/支付两行同步 003 |`
  - `docs/architecture/MODULE_MARKET_UI.md` 末尾 `…（数百模块）时才需要 |`
  - `docs/architecture/PLATFORM_USER_COMMUNICATION.md` 末尾 `…三件事关联起来的能力。`
- `docs/handbook/SAAS_PRODUCTION_CONSTRAINTS.md:83-86` 现场：
  ```
  - **现行契约**（commit 见 deploy 仓）：Dockerfile COPY `config/platform.yaml`
    + deploy.sh 同步该文件到服务器 + 服务器 .env 必须有
    `PLATFORM_ADMIN_PASSWORD`（yaml 里 `${PLATFORM_ADMIN_PASSWORD}` 展开用）。
  ```
  行首 2 空格缩进的 `+` 被 markdown 解析成**嵌套无序列表标记**（plus 风格），而 `.markdownlint.json` 设 `MD004: { "style": "dash" }` ⇒ 判 red。作者原意是「与」的连接词，不是列表。
- `.specify/bugs/_md047_probe.md`：commit `c8e4cfa0`「test(probe): 故意制造 MD047 违规，验证 --lint-md 档反向验证」故意提交，全仓 grep 无任何引用（除该 slug 自身）⇒ 它的存在使 CI Markdown Lint 作业与 `--lint-md` 档永久红。

## Root Cause Hypothesis

两类成因：

1. 8 个 MD047：文件写入时漏了末尾换行（编辑器/脚本保存差异），`.markdownlint.json` 保留 `MD047: true`（本仓裁剪规则时只关了 MD007/MD040/MD013/MD033/034/036/041/060/MD024/MD037，MD047 是刻意保留的硬门）⇒ 漏一个字节就红。
2. 1 个 MD004：作者用「+」做并列连接词，却写成行首缩进两格的形态，被 markdown 解析为 plus 风格嵌套列表；配置要求 dash。
3. 探针文件把「反向验证用的故意违规样本」提交进了仓内，而不是留在本地临时目录 ⇒ 护栏被样本本身永久钉红。

## Proposed Remediation

### Preferred

1. 8 个 MD047 文件末尾补 `\n`（纯字节修复，不改任何正文语义）。
2. `docs/handbook/SAAS_PRODUCTION_CONSTRAINTS.md:85` 把行首 `+` 改为顿号 `、`（该文件已有 15 处顿号用法，是仓内既定中文标点风格），使该段成为同一 bullet 的并列句，语义不变；改后必须重跑 markdownlint 确认 MD004 转绿且没引入新问题。
3. `.specify/bugs/_md047_probe.md` **改写**（不删除、不加配置豁免）为一份 lint-clean 的「MD047 反向验证配方」文档：说明如何在本地临时造出一个缺尾换行的文件、跑 `npx -y markdownlint-cli2` 观察 MD047 判红、再删除临时文件 ⇒ 反向验证能力保留在树内可查，CI 与 `--lint-md` 档同时转绿。
   - 保留原路径，不删文件：CLAUDE.md 规则 3 明确「不覆盖既有报告、已有 bug 目录一律不删」，且零删除、零配置豁免是最小风险路径。

### Alternatives

- 删除 `.specify/bugs/_md047_probe.md`：会让「反向验证配方」丢失，且触碰规则 3 的不删约束 ⇒ 否。
- 在 `.markdownlint-cli2.jsonc` 的 ignores 里排掉这 9 个文件，或在 `.markdownlint.json` 关掉 MD047/MD004：为绿灯改护栏配置，与仓内口径相反 ⇒ 否。
- 用 `markdownlint --fix` 批量自动改：会顺带重排这 9 个文件里其它可能触碰的规则，且对 MD004 的自动修复会把连接词改写成列表，语义漂移 ⇒ 否（只做定点字节/标点修复）。

### Files likely to change

- `.specify/002-modular-subscription/plan.md`
- `.specify/002-modular-subscription/spec.md`
- `.specify/bugs/_md047_probe.md`
- `docs/architecture/ALL_ENDS_CAPABILITY_ANALYSIS.md`
- `docs/architecture/LLM_BILLING_FLOW.md`
- `docs/architecture/MODULAR_MODULES.md`
- `docs/architecture/MODULE_MARKET_UI.md`
- `docs/architecture/PLATFORM_USER_COMMUNICATION.md`
- `docs/handbook/SAAS_PRODUCTION_CONSTRAINTS.md`

### Tests to add or update

不新增自动化用例。回归手段：

- `npx -y markdownlint-cli2`（仓库根）⇒ `Summary: 0 issues`，rc=0
- `python3 scripts/check-md-links-offline.py .` ⇒ 通过（只认 git 索引，新写的 .md 需先 `git add`）
- `bash scripts/bug-verify.sh --slug <本 slug> --skip-audit --lint-md` ⇒ markdownlint 档绿

## Risks & Considerations

- 8 个文件补末尾换行是纯字节变化，不改渲染语义；但它们都是已发布架构文档，改动会进入 diff 噪音，评审时需知道这是纯换行。
- `SAAS_PRODUCTION_CONSTRAINTS.md` 的改法要保持「三个并列要求」的语义（Dockerfile COPY + deploy.sh 同步 + 服务器 .env 备键），改标点不改约束内容。
- 探针文件改写后，其反向验证价值从「仓内常驻样本」变成「配方 + 一次性本地复现」；本轮 test 阶段会真的造临时违规文件复现一次 MD047 判红，验证配方有效。
- `check-md-links-offline.py` 以 git 索引为准 ⇒ 新写的 spec-kit 文档要 `git add` 之后该门才看得到。

## Open Questions

无。修法由仓内既定风格（顿号 + 零配置豁免 + 不删既有文件）定死。
