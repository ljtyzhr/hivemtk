# Bug Assessment: `make test-go` 缺少 `-p 1`，本地跑全量 Go 测试必然超时失败

- **Slug**: `make-test-go-missing-p1`
- **Created**: 2026-10-13
- **Source**: pasted text（本地实跑 `make test-go` 的失败日志 `/tmp/fulltest.log`）
- **Verdict**: valid
- **Severity**: high

## Report (verbatim or summarized)

2026-10-13 在仓库根 `/Users/xiaofang/Documents/www/go/hivemtk` 实跑
`make test-go`（即 `cd user-server && go test ./... -count=1`，测试库连接变量已正确导出
`POSTGRES_TEST_HOST=127.0.0.1` / `POSTGRES_TEST_PORT=8202` / `POSTGRES_TEST_PASSWORD=<.env 值>`），
结果 4 个包全部 `panic: test timed out after 10m0s`：

```
FAIL	hivemtk-user/internal/controller	603.710s
FAIL	hivemtk-user/internal/pkg/db	602.724s
FAIL	hivemtk-user/internal/repository	602.591s
FAIL	hivemtk-user/internal/service	601.230s
```

四个包的失败信息完全一致，均为 `panic: test timed out after 10m0s`（Go 默认 per-package 超时），
`/tmp/fulltest.log` 中该 panic 出现 4 次。

## Symptom

`make test-go` 用 Go 默认并行度（`-p=GOMAXPROCS`）并发跑多个共享测试库的包，包之间互相争抢
测试库槽位与自增主键，导致彼此挂起等待，10 分钟后集体触发 `test timed out`。
**预期**：与 CI 一致，本地一键跑全量 Go 测试应能通过（至少不该因并发撞库而无谓超时）。

## Reproduction

1. `cd /Users/xiaofang/Documents/www/go/hivemtk`
2. `export POSTGRES_TEST_HOST=127.0.0.1 POSTGRES_TEST_PORT=8202 POSTGRES_TEST_PASSWORD="$(grep '^POSTGRES_PASSWORD=' .env | cut -d= -f2)"`
3. `make test-go`
4. 观察 `internal/{controller,pkg/db,repository,service}` 四个包 FAIL，报 `test timed out after 10m0s`

对照实验（证明不是被测代码错）：单独跑同一批包即可通过，例如
`cd user-server && go test -timeout 20m ./internal/pkg/db/...` ⇒ `ok  207.101s`。
⇒ 失败只在**并发**时出现，串行正常。

## Suspected Code Paths

- `Makefile:449-450` — `test-go:` 目标体为 `cd user-server && go test ./... -count=1`。
  无 `-p 1`、无 `-timeout`、无 `-short`，直接用 Go 默认并行度跑包。**这是缺陷本体。**
- `.github/workflows/user-server-ci.yml:562-566` — CI 侧同一条命令带
  `-count=1 -timeout 900s -p 1 -short -race`，且 562-564 行注释原文说明 `-p 1` 是硬要求：
  「用例共享同一批测试库槽位（见 testutil procTestDBSlots），并发跑会互相认领对方的行
  （reply_guard 认领表 / delayed-<id> 撞自增主键）」。
  **Makefile 与 CI 不同步，是本次缺陷的直接反证。**
- `user-server/internal/pkg/testutil/testdb.go` — 测试库槽位/自增主键分配逻辑的落点
  （CI 注释所指 `procTestDBSlots` 的继承者所在，未逐行核对实现，仅按 CI 注释与现象交叉印证）。

## Root Cause Hypothesis

**置信度：high。** `Makefile` 的 `test-go` 目标在迁移/演进中漏掉了 CI 已经明确的
`-p 1`（以及配套的 `-timeout 900s`）。Go 的默认 `-p` 等于 `GOMAXPROCS`，会让
`internal/controller`、`internal/repository`、`internal/service`、`internal/pkg/db`
四个重度依赖共享测试库的包同时开工；这些包通过 `testutil` 认领同一批进程级测试库槽位并写入
自增主键，并发下互相踩踏（认领对方刚删/刚占的行、自增 ID 撞车），最终表现为四个包**同时**
在 10 分钟默认超时点集体挂起——而非某个具体断言失败。

支撑证据：(1) 四个 FAIL 包的耗时几乎相同（601–604s），是「同时挂起」而非「某包慢」；
(2) 单独跑任何一个包都能通过（`ok 207.101s`）；(3) CI 明确把 `-p 1` 标为硬要求并给出机制说明。

## Proposed Remediation

**Preferred**: 修改 `Makefile` 的 `test-go` 目标，与 CI 口径对齐——至少补 `-p 1` 与
`-timeout 900s`：

```make
test-go:
	cd user-server && go test ./... -count=1 -p 1 -timeout 900s
```

`-p 1` 是根因修复（串行跑包，杜绝撞槽位）；`-timeout 900s` 是配套保护，否则单包超过 Go 默认
10 分钟仍会白超时——CI 注释里已记录 core 片最慢包 `browser_automation/service` 98s、
`internal/service` 连 300s 都不够，所以 900s 是 CI 实测出来的余量值。

**不加 `-short` 和 `-race`**（与 CI 的取舍不同，需明说）：`-short` 会让 `testing.Short()`
门控的用例整体跳过，本地总门禁跳用例会漏问题；`-race` 会让全量耗时数倍拉长，本地不适用。
⇒ 本地 gate 与 CI 的差异是**有意的**：CI 求快且有分片，本地 gate 求全。

**Alternatives**:
- 只加 `-p 1` 不加 `-timeout` — 能解撞库，但仍有包被默认 10 分钟卡死的风险，不完整。
- 改成按 CI 的 `pkgs.txt` 分片跑 — 复杂化且要维护清单文件，本地一键跑的意义丧失，否决。

**Files likely to change**:
- `Makefile`（`test-go` 目标，450 行）

**Tests to add or update**:
- 本仓库没有针对 Makefile 目标定义做静态断言的现成测试；建议后续补一个
  「`test-go` 必须含 `-p 1`」的轻量断言（可挂在 `scripts/` 下与 `make audit` 同族），
  防止再次退化。本轮先以实跑 `make test-go` 全绿作为验证口径。

## Risks & Considerations

- `-p 1` 串行跑 ⇒ 本地全量测试耗时变长（CI 记录 core 片最慢包 98s，全量串行预计数分钟量级）。
  这是 correctness 优先于 speed 的正确取舍，因为并行跑根本跑不完。
- `-timeout 900s` 只放宽单包预算，不影响 `-p 1` 的串行语义。
- 不改任何 Go 测试代码——本次是**门禁命令缺陷**，不是被测代码缺陷，改测试反而是掩盖问题。
- `Makefile` 当前在并行泳道手里有**已暂存但未提交**的改动（`git status` = `M `），
  但 `git diff --cached Makefile` 的 hunk 只在 `audit:` 目标（约 540 行，新增
  `check-text-encoding.py` 两步），与 `test-go`（450 行）相距很远，不会冲突。fix 阶段只动 450 行。

## Open Questions

- [NEEDS CLARIFICATION: 是否需要把 `-p 1` 的约束写进 `user-server/docs/dev/CONVENTIONS.md`
  / `CLAUDE.md` 验证口径，让后续 agent 手敲 `go test` 时也知道不能并发？倾向：是，随本轮补一句。]
