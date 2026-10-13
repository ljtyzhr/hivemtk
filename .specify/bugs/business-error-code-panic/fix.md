# Bug Fix: 文档规定的业务错误码 10001 触发 `panic: invalid WriteHeader code`

- **Slug**: business-error-code-panic
- **Fixed**: 2026-10-13
- **Assessment**: ./assessment.md
- **Status**: applied

## Summary

`CLAUDE.md` §错误码 登记过一段并不存在于任何码表的 `10000-19999` 业务码，按它写出的
`response.Error(c, 10001, msg)` 会打到 gin 的 `panic: invalid WriteHeader code 10001`，
调用方拿不到响应体。本轮在 `response.Error` 的 `int` 分支加合法域校验（越界降级 500 + 出声），
把文档改成「两条通道 + 码值事实源指向代码」的写法，并为 ADR-009 补一节与现行代码的对账。

## Changes

| File | Change | Notes |
|------|--------|-------|
| `user-server/internal/pkg/utils/response/response.go` | modified（+10 行，`case int:` 内，`:112`） | `v < 100 \|\| v > 999` 时不进 `c.JSON`：`httpCode = 500`、`errorCode = utils.ErrorCodeUnknown`，并 `logger.Errorf` 记下越界值。合法 HTTP 码路径逐字不变 |
| `user-server/internal/pkg/utils/response/response_test.go` | added 3 tests（`:289`、`:308`、`:327`） | 越界不 panic、越界集合全覆盖、合法状态码原样透传（极性对） |
| `CLAUDE.md` | modified（§错误码 整节重写，`:95-113`） | 改为「传输语义 / 业务语义」两通道表，码值事实源指向 `error_code.go` 与 `codes.go`，明确本文件不再重抄清单 |
| `docs/architecture/adr/ADR-009-error-handling.md` | modified（状态行 + 文末新增「与现行代码的对账（2026-10-13）」，共 7 条） | 不改写 ADR 历史正文；把 7 处与现行代码不符的表述逐条列证据作废 |
| `user-server/internal/pkg/utils/response/response.go` | modified（第二处，`localeOf`，`:29-35`） | **验证阶段新发现的引信**：gin 1.12 的 `GetHeader` 直取 `c.Request.Header`，`localeOf` 只判了 `c != nil`，于是在没有 Request 的 Context 上以 SIGSEGV 收场。补 `c.Request == nil` 早退到 `i18n.ZH` |
| `user-server/internal/pkg/utils/response/response_test.go` | added 1 test（4 个子用例） | `TestResponsePath_RequestlessContextDoesNotPanic`：`Error`/`Success`/`ErrorWithBusinessCode`/`LocaleOf` 在裸 `gin.CreateTestContext` 上都不许崩 |
| `.specify/bugs/business-error-code-panic/check-doc-code-literals.py` | added | 把「文档抄码值」这一根因做成可复现对账：文档里出现的每个码值字面量必须能在两张登记表里找到且取值一致 |
| `scripts/check-action-runtime.py` | modified（合并阶段收账，非 assessment 预估） | 把本轮的 merge 提交推上双远端时，上游 12 笔 dependabot 抬版（`setup-go@v5→v7`、`upload-artifact@v4→v7`、`gitleaks…@v2→v3`）使这道常驻门 rc=2：`gitleaks…@v3` 不在 RUNTIMES 表 + 三条豁免账本条目匹配不到站点。按门自己的要求现读该 tag 的 `action.yml` 登记 `node24`，删三条失效豁免，保留仍在场的 `codecov…@v4`×2 |

## Diff Highlights

```go
case int:
    // 业务错误码（如 10001/5001）不是合法的 HTTP 状态码，直传 c.JSON 会打到
    // gin 的 "invalid WriteHeader code" panic，整条请求拿不到响应体。
    // 这里降级为 500 并出声，让写错的调用方可诊断，而不是静默断连。
    // 业务码应当经 ErrorWithBusinessCode 走响应体 code 字段。
    if v < 100 || v > 999 {
        logger.Errorf("response.Error received non-HTTP status code %d; business codes must go through ErrorWithBusinessCode", v)
        httpCode = http.StatusInternalServerError
        errorCode = utils.ErrorCodeUnknown
        break
    }
    httpCode = v
    errorCode = errorCodeFromHTTPCode(v)
```

## Tests Added or Updated

- `response_test.go::TestError_BusinessCodeDoesNotPanicWriter` — 传 `10001` 时 `ResponseRecorder` 不 panic，
  落 HTTP 500，且响应体 `code` 是字符串 `UNKNOWN_1000`（业务码不得冒充 HTTP 状态码语义）。
- `response_test.go::TestError_OutOfRangeCodesAllDegraded` — `{0,1,99,1000,5001,10001,19999,50301,-1}`
  全包在 `recover()` 里逐枚调用，任何一枚触发 panic 即 `t.Fatalf`，其余断言 500。
- `response_test.go::TestError_ValidHTTPStatusUnchanged` — 极性对：`{400,401,403,404,409,422,429,500,503,504,999}`
  原样透传。缺这一枚时，「把 `case int:` 整支改成恒 500」也能让上两条绿。

## Local Verification

- `go test ./internal/pkg/utils/response/ -v -count=1` → 27 行 `--- PASS`、0 行 `--- FAIL`，
  `ok hivemtk-user/internal/pkg/utils/response 0.216s`（含 3 枚新用例，名单已逐条读出）。
- 反向测（判据有牙，本轮实跑，非引用上一轮结论）：按锚点字符串精确删除 `case int:` 的
  合法域校验块（`guard_block_hits 1` → 写盘 → `grep "non-HTTP status code"` rc=1 确认落地），
  逐枚单独跑三枚新用例：
  - `TestError_BusinessCodeDoesNotPanicWriter` → `--- FAIL`，真红因
    `panic: invalid WriteHeader code 10001 [recovered, repanicked]`；该 panic 会带走整个测试
    二进制（同批另两枚因此不印结果），故下两枚单独 `-run`。
  - `TestError_OutOfRangeCodesAllDegraded` → `--- FAIL`，红因
    `response_test.go:314: Error(1) panic: invalid WriteHeader code 1`（`recover()` 生效，
    逐枚报出第一支越界码，不会把进程带走）。
  - `TestError_ValidHTTPStatusUnchanged` → `--- PASS`（符合预期：摘掉守卫不影响合法码，
    这一枚是对「恒 500」那种改法开火的，不是对本枚）。
  用 `cp` 备份写回，还原后 `md5` 与注码前逐字一致
  （`e86b6a7514907062338182a894120867`，本轮磁盘值），`gofmt -l` 空，
  复跑 `-v` 读到 27 行 `--- PASS`、`ok hivemtk-user/internal/pkg/utils/response 0.197s`。
- `gofmt -l internal/pkg/utils/response/` → 空输出。
- `go vet ./internal/pkg/utils/response/` → 无诊断。
- `go build ./...` → 无输出（`internal/pkg/utils/response` 之外的消费方一并不受改动影响，见下）。
- **既有调用方面零影响（现测，非推断）**：全仓整数字面量状态码用量纲化统计——
  `response.Error(<ctx>, <int literal>)` 命中 148 处，取值只有
  `400×94 / 401×7 / 404×7 / 409×2 / 500×13 / 503×25`，全部落在 100–999 内；
  以业务码（5 位）作状态码的调用点 **0 处**。即本轮修复的是「照文档写就会崩」的引信，
  不是线上正在崩的调用。
- `python3 scripts/check-md-links-offline.py .` → 扫描 206 个 md（CI 口径只认 git 索引），断链 0 处。
- `python3 scripts/check-text-encoding.py` → `NOT_UTF8=0 FFFD(超出基线)=0 MOJIBAKE=0 STALE=0`。
- `bash scripts/check-doc-consistency.sh` → 6/6 通过（含 ADR 编号连续性、27 个关键文件存在性）。
- `bash scripts/check-architecture.sh`：**活树 rc=1**，红因是
  `[L4] service 取 DB 句柄宽口径与基线不符`（`ai_agent.go 5>4`、`tiktok_card.go`/`inbox_scope.go` 新增 `.GetDB`、
  `message_hub.go` 基线条目失效）——命中的全在 `user-server/internal/service/`，与本轮四个文件无关。
  归因方式：在干净 HEAD 克隆（`15ccabb7`）里只叠上本轮这 4 份文件复跑，`rc=0`、无 `❌`。
  该红属并行泳道未提交的 `*gorm.DB`→Repository 重构中间态，不由本轮认领、也不代为改写基线。
- 文档码值 ↔ 登记表对账（`.specify/bugs/business-error-code-panic/check-doc-code-literals.py`）：
  两张登记表现读 13 条数值码 / 36 条字符串码，两份文档共 10 处码值字面量全部对得上，`rc=0`。
  三枚反向格现证判据有牙：往 ADR-009 追加 `CodeGhostOnly = 4242` → 「常量不存在」开火；
  追加 `GHOST_CODE_9999` → 「字符串码不在码表」开火；追加 `CodeParamInvalid = 4999` →
  「文档写 4999，代码实际是 4000」开火；三格后 `cp` 还原，ADR-009 md5 回到
  `bb8013ae87b32d518ead31229b4ea33a`、复跑 `rc=0`。
  占位形状（`CodeXxx`、`bizerr.CodeXxx`）按「含 `Xxx` 即通配」排除，不参与对账——
  这一族若不排除，任何写法示例都会被误判成码值。
- 消费方回归（`response.Error` 的 14 个含整数状态码调用的包）：
  `EMBEDDING_ALLOW_FALLBACK=true POSTGRES_TEST_PASSWORD=<.env 值> go test <14 pkg> -count=1 -p 1`
  → `internal/pkg/utils/response`、`internal/pkg/utils`、`internal/pkg/utils/pagination`、
  `internal/pkg/errhttp`、`internal/middleware`、`internal/monitor`、`internal/router`、
  `internal/app`、`internal/aiagent/knowledge/controller`、`internal/browser_automation/controller`、
  `internal/content/controller`、`internal/geo/controller`、`internal/ops/controller` 全 `ok`；
  唯 `internal/controller` 9 枚 `--- FAIL`，红因全是
  `config_param_write_test.go:151 PUT …… cannot unmarshal number into Go struct field .value of type string`，
  而该测试文件在 `git status` 里是 `??`（未跟踪、不在 HEAD：`git cat-file -e HEAD:…` 直接报
  「在磁盘上，但不在 HEAD 中」），并行泳道同批还在改 `controller/config_param.go` 与
  `service/config_param.go` ⇒ 属他泳道在途用例，不由本轮认领。
- 环境前提订正（避免把假红读成仓库红）：首轮不带 `POSTGRES_TEST_PASSWORD` 跑时，9 个包在
  `testdb.go:331` 同一句红（`127.0.0.1:8202 … failed SASL auth … "admin" (SQLSTATE 28P01)`），
  补齐口令后同一批逐包转绿 ⇒ 那一轮的红是缺测试库口令，不是代码红。
  本机现听端口为 5432（Homebrew PG）与 8202（`mtk-postgres` 容器），文档里的 8232 dev 端口无进程监听。

## Integration Verification（推送前在干净克隆里合并上游后现测）

本轮的提交要推的树不是活树，而是「本轮内容 ∪ 上游 12 笔 dependabot 抬版」，因此所有读数在
`/tmp/sk-push/hivemtk`（以仓库目录为源的克隆，HEAD 到 `4eb0ec7b`）重跑一遍：

- `git merge --no-edit upstream/master`（tip `8a00b975`）⇒ ort 策略干净合并，**零冲突**，
  只吃进 5 份文件（`user-server-ci.yml`、`go.mod`、`go.sum`、`user-web/bridge/package.json`、
  `package-lock.json`）；合并后用 `git rev-parse HEAD:<file>` 对 `git rev-parse d28a8336:<file>`
  逐枚比 blob，本轮四个文件 **4/4 SAME**（合并没动过我的字节）。
- 合并把 `check-action-runtime.py` 这道常驻门打成 **rc=2**（三条 STALE + 一个未登记的 `gitleaks…@v3`），
  即「合并即坏」，按规则2 当场收账（见 Changes 表最后一行），收账后：
  - `python3 scripts/check-action-runtime.py --repo .` → rc=0，`扫描工作流 15 份；uses 站点 81 处，
    跳过 1 处；node20/node16 命中 3 处：未豁免 0，豁免内 3`。
  - `bash scripts/check-action-runtime.test.sh` → `PASS=43 FAIL=0`，含「格10 出厂基线自洽
    （跑真仓库，表与账必须对得上磁盘）」这一枚真仓库控制格。
  - `python3 scripts/check-ci-gate-paths.py --repo .` → rc=0，`门站点 46 处`（抬版只改 step 的
    `uses:`，没增删门，故 paths 触发面无需跟着改）。
- `make -k audit` → **rc=0**（`✅ 静态审计通过`），含 i18n 覆盖、seam-guard、battery-identity、
  `check-vite-version.sh`（现读到 bridge `^8.3.4`/lock 8.3.4，与 dependabot 的 npm 抬版同源，
  自证行 `扫描package.json=6 含vite的pin=5 判定=GREEN`）。
- `go build ./...` → 无输出；`go vet ./...` → **rc=0**（vet 会编译 `_test.go`，即已提交的测试
  在这套新依赖上全部编得过）。
- `EMBEDDING_ALLOW_FALLBACK=true POSTGRES_TEST_PASSWORD=<.env 值> go test ./internal/pkg/... -count=1`
  → `^ok` 39 个包、`^FAIL` **0** 个包（`grep -c` 取 0 时末句退 1，是 grep 的语义不是测试红）；
  其中 `internal/pkg/db 377.773s`、`internal/pkg/testutil 52.278s`。不带口令时同一批里
  4 个包在 `testdb.go:320` 报 `failed SASL auth … "admin" (SQLSTATE 28P01)` ⇒ 环境前提，非代码红。
- `go test ./internal/pkg/utils/response/ -v -count=1` 在合并树上 → 全 `--- PASS`（含本轮 4 枚新用例），
  `ok hivemtk-user/internal/pkg/utils/response 0.541s`。
- `.specify/bugs/business-error-code-panic/check-doc-code-literals.py` 在合并树上 → rc=0（10 处码值全对得上）。
- 推送前祖先判定：`git merge-base --is-ancestor` 现证 `upstream/master`(8a00b975) 与新 tip 的父系
  含自身、`gitee-upstream/master`(15ccabb7) 亦是本轮两笔提交的基 ⇒ 两推都是 fast-forward，无 `--force`。

## Deviations from Assessment

0. **验证阶段扩了一次面（新增文件外的一处代码改动）**：跑 assessment §Reproduction 的字面复现时，
   `response.Error(c, 10001, …)` 在裸 `gin.CreateTestContext` 上不再是 `invalid WriteHeader code` panic，
   却以 `SIGSEGV（addr=0x38）@ localeOf → response.go:29` 收场。这不是本轮改动引入的，A/B 现证：在干净 HEAD 克隆（`15ccabb7`）里把 `response.go` 换成
   `git show HEAD:` 那份（md5 `62ad21e6…`，两处守卫都没有），用**合法**状态码 400 配裸
   `gin.CreateTestContext` 调用，同样 `nil pointer dereference`；换回本轮那份
   （md5 `3970552d…`）同一枚用例 `--- PASS`。即该引信与业务码无关，早于本轮就在场。
   但它挡在同一条「写响应」的路上，且本仓 104 处测试用 `gin.CreateTestContext`，属同一族的
   「响应路径被非 HTTP 语义打断」。按规则2（发现即修）当场修掉并补测，未另开 slug。
   同文件里 `LogBindError` 一类（`:352`、`:362`）也裸读 `c.Request.Method`，但那些只在真实
   请求的绑定失败路径上被调用，`c.Request` 由 gin 的 `ServeHTTP` 保证非空 —— 不属本轮入口，未改。
1. **修复项 3 的做法变了**：assessment 写「`ADR-009`：改掉悬空的 §4.3 状态行、「5 位数字」表述，
   以及那处无法编译的 4 参示例」。实际只改了状态行，正文不改写——ADR 是决策记录，
   把历史正文改成「与今天一致」会抹掉决策痕迹且无法证伪。改为文末追加
   `## 与现行代码的对账（2026-10-13）`，冲突处以对账为准。
2. **陈旧表述比 assessment 记的多**：assessment 列 3 条，实测 7 条。多出的 4 条是
   落地路径 `internal/middleware/error_handler.go` 与 `docs/api/error-codes.md` 均不存在、
   `response/error.go` 实为 `errors.go` + `response.go`、§5 的
   `logger.Error(ctx, "unhandled", "err", err)` 那种 kv 形式在 `logger` 里不存在
   （现行是 `func Error(err error, msg string)`，带 ctx 走 `logger.Ctx(ctx)`）。
3. **assessment 的数字是二手的，已现测订正**：controller 文件 157 → 实测 179；
   非测试 `response.Error(` 调用点写的是估值，实测 2053。订正已回写在本报告的
   Local Verification 里（按 spec-kit 规矩 `assessment.md` 本身不编辑）。
4. **收账这一改与并行泳道在途的同一枚改动重叠**：主工作树里 `scripts/check-action-runtime.py`
   与 `.github/workflows/user-server-ci.yml` 均处于 ` M`（未提交）状态，对方正在做的正是
   「抬 gitleaks/upload-artifact pin + 删对应豁免」，其中 `RUNTIMES` 新增行的日期与取值
   （`gitleaks…@v3 = node24`）与本轮现读结果逐字一致。差异只在 `("user-server-ci.yml",
   "actions/setup-go@v5"): (6, …)` 这一条——对方的树里 v5 站点仍在（其基线是 `15ccabb7`），
   那条豁免对他们仍是**必需**的，删了会把他们的本地门判红；而本轮推的是「已合并上游」的树，
   v5 站点为 0，R3 要求删。故本轮**只**在克隆里按已合并内容收账，没有把这一行删进对方的
   工作树副本（那会用他们的树读不到的语义去改他们的文件）。对方下次合并本轮 tip 时，
   该条会由 R3 自然要求删除，冲突面为 1 行、可机械解决。

## Follow-ups

- `/speckit-bug-test slug=business-error-code-panic`：把上面的本地读数换成可重跑的验证清单。
- 按 `CLAUDE.md` 规则1 提交并推双远端（`gitee-upstream` / `upstream`），推送前 `git fetch` 核 fast-forward；
  本工作树与并行泳道共享，只 `git add` 显式路径，且 commit 前逐 hunk 核 `git diff --cached`。
