# Bug Verification: 文档规定的业务错误码 10001 触发 `panic: invalid WriteHeader code`

- **Slug**: business-error-code-panic
- **Tested**: 2026-10-13
- **Assessment**: ./assessment.md
- **Fix**: ./fix.md
- **Result**: verified

Slug 解析路径：conversation context（本轮刚跑完 fix，`fix.md` 存在且逐字读过）。

## Summary

原始症状不再复现：`response.Error(c, 10001, …)` 现在记一条越界日志并回 HTTP 500 +
`code:"UNKNOWN_1000"` 的响应体，而不是把请求打成 panic/断连。改动前后对照跑过同一枚复现用例
（摘掉守卫即读到真红因 `invalid WriteHeader code 10001`）。14 个消费包回归全绿，唯一一组红
（`internal/controller` 9 枚）现证属并行泳道的未跟踪在途用例。验证阶段另发现并修掉同一条
「写响应」路上的第二处引信（`localeOf` 在无 Request 的 Context 上 SIGSEGV），A/B 已定死早于本轮就在场。

验证不止于本轮那四个文件：推送出去的是「本轮内容 ∪ 上游 12 笔 dependabot 抬版」的合并树，
该树在干净克隆里跑过 `make -k audit`（rc=0）、`go vet ./...`（rc=0）、`./internal/pkg/...`
全包（39 ok / 0 FAIL），且合并即红的那道 action 运行时门已先收账再推。

## Checks Performed

| Check | Command / Action | Result | Notes |
|-------|------------------|--------|-------|
| Reproduction (post-fix) | 干净 HEAD 克隆叠上本轮 4 份文件，`go test ./internal/pkg/utils/probe/ -run TestDocumentedBusinessCode10001 -v -count=1` | pass | `--- PASS`，且日志行 `ERR response.Error received non-HTTP status code 10001; business codes must go through ErrorWithBusinessCode` 出声；原 panic 消失 |
| Reproduction (pre-fix，同一枚用例) | 摘掉 `case int:` 合法域块后跑仓内用例 | fail（预期） | `--- FAIL` + `panic: invalid WriteHeader code 10001 [recovered, repanicked]` ⇒ 复现确实用的是原症状，不是别的崩溃 |
| New / updated tests | `go test ./internal/pkg/utils/response/ -v -count=1` | pass | 27 行 `--- PASS`、0 行 `--- FAIL`，含 3 枚守卫用例与 1 枚 4 子用例的 nil-Request 用例；`ok … 0.567s` |
| Regression suite（消费方 14 包） | `EMBEDDING_ALLOW_FALLBACK=true POSTGRES_TEST_PASSWORD=<.env> go test <14 pkg> -count=1 -p 1 -timeout 900s` | pass | 13 包 `ok`；`internal/controller` 9 枚红现证为他泳道未跟踪用例（见下） |
| Call-site blast radius | 全仓 `response.Error(<ctx>, <int literal>)` 量纲化计数 | pass | 148 处、取值只有 400/401/404/409/500/503，全在 100–999 内 ⇒ 守卫对既有调用零行为变更 |
| Lint / format | `gofmt -l internal/pkg/utils/response/`、`go vet ./internal/pkg/utils/response/` | pass | 前者空输出，后者无诊断 |
| Build | `go build ./...` | pass | 无输出 |
| Doc gate: 断链 | `python3 scripts/check-md-links-offline.py .` | pass | 扫描 206 md（只认 git 索引），断链 0 |
| Doc gate: 文本编码 | `python3 scripts/check-text-encoding.py` | pass | `NOT_UTF8=0 FFFD=0 MOJIBAKE=0 STALE=0` |
| Doc gate: 一致性 | `bash scripts/check-doc-consistency.sh` | pass | 6/6（含 ADR 编号连续性、27 关键文件存在性） |
| 架构门 | `bash scripts/check-architecture.sh` | fail → 归因 pass | 活树 rc=1 红在 `[L4] service 取 DB 句柄宽口径与基线不符`（全在 `internal/service/`）；同克隆只叠本轮 4 份文件后 `rc=0` ⇒ 他泳道在途态 |
| 文档码值对账 | `python3 .specify/bugs/business-error-code-panic/check-doc-code-literals.py` | pass | 登记表 13 数值 / 36 字符串，10 处引用全对得上；三枚反向格各自开火后 `cp` 还原 md5 一致 |
| 上游合并 | `git merge --no-edit upstream/master`（12 笔 dependabot，tip `8a00b975`）于干净克隆 `/tmp/sk-push/hivemtk` | pass | ort 零冲突，只吃 5 份文件；`git rev-parse HEAD:<f>` vs `d28a8336:<f>` 比 blob ⇒ 本轮 4 份文件 4/4 SAME |
| 合并后常驻门 | `python3 scripts/check-action-runtime.py --repo .` | fail → 修后 pass | 合并即红 rc=2（`gitleaks…@v3` 未登记 + 3 条豁免 STALE）；现读该 tag `action.yml` 得 `node24` 登记、删 3 条失效豁免后 rc=0（`站点 81 处／node20 命中 3 全在豁免内`） |
| 该门的两向电池 | `bash scripts/check-action-runtime.test.sh` | pass | `PASS=43 FAIL=0`，含格10「跑真仓库，表与账必须对得上磁盘」 |
| 静态门全链 | `make -k audit`（合并树） | pass | rc=0、`✅ 静态审计通过`；`check-ci-gate-paths.py` 门站点 46 处不变；`check-vite-version.sh` 读到 dependabot 的 `^8.3.4` 判 GREEN |
| 测试可编译性 | `go vet ./...`（合并树，vet 才编 `_test.go`） | pass | rc=0，无诊断 ⇒ 抬版后的依赖集不破坏任何已提交测试的编译 |
| 合并树包级回归 | `EMBEDDING_ALLOW_FALLBACK=true POSTGRES_TEST_PASSWORD=<.env> go test ./internal/pkg/... -count=1` | pass | `^ok` 39 包 / `^FAIL` 0 包（`db 377.773s`、`testutil 52.278s`）；无口令时 4 包 `testdb.go:320` SASL 红＝环境前提 |
| 合并树响应包用例 | `go test ./internal/pkg/utils/response/ -v -count=1` | pass | 全 `--- PASS`，`ok … 0.541s`（本轮 4 枚新用例在内，含 `TestResponsePath_RequestlessContextDoesNotPanic` 的 4 个子用例） |

## Output Excerpts

```
=== RUN   TestDocumentedBusinessCode10001
2026-10-13T10:16:26+08:00 ERR response.Error received non-HTTP status code 10001; business codes
must go through ErrorWithBusinessCode component=user-server
--- PASS: TestDocumentedBusinessCode10001 (0.00s)
ok  hivemtk-user/internal/pkg/utils/probe  0.416s
```

摘掉守卫后的反向格（真红因，非装架红）：

```
--- FAIL: TestError_BusinessCodeDoesNotPanicWriter (0.00s)
panic: invalid WriteHeader code 10001 [recovered, repanicked]
    response_test.go:314: Error(1) panic: invalid WriteHeader code 1
```

`internal/controller` 9 枚红的红因（同一句，指向未跟踪文件）：

```
config_param_write_test.go:151: int 数字载荷…… PUT bridge.ingest_max_body_bytes = 8388608 被拒
    （code=400 msg="参数错误: json: cannot unmarshal number into Go struct field .value of type string"）
$ git cat-file -e HEAD:user-server/internal/controller/config_param_write_test.go
致命错误：路径在磁盘上，但是不在 'HEAD' 中
```

第二处引信的 A/B（同一枚用例，只换 `response.go` 的份）：

```
HEAD 那份（md5 62ad21e6…）：nilreq_test.go:20: red: runtime error: invalid memory address or nil pointer dereference
本轮那份（md5 3970552d…）：--- PASS: TestNilRequestOnValidStatus (0.00s)
```

合并上游后、收账前那道门的原样红（读数取自 `/tmp/sk-push/hivemtk`，非活树）：

```
❌ 以下 pin 不在 RUNTIMES 表里，无法判定其 runs.using：
   gitleaks/gitleaks-action@v3
   去读该 tag 的 action.yml（子路径 action 要读子路径下那份），把 runs.using 登记进表；不要凭版本号猜。
❌ 豁免账本里有匹配不到站点的条目（该文件已迁完或已改名，请删条目）：
   user-server-ci.yml: actions/setup-go@v5  （原由：同上）
   user-server-ci.yml: actions/upload-artifact@v4  （原由：同上）
   user-server-ci.yml: gitleaks/gitleaks-action@v2  （原由：同上）
项目根: /private/tmp/sk-push/hivemtk
扫描工作流 15 份；uses 站点 81 处（可判定），跳过 1 处（可复用工作流/本地/容器）
```

`gh api "repos/gitleaks/gitleaks-action/contents/action.yml?ref=v3"` 解码后的判定行（登记值的出处，
zsh 下 `?` 必须整体加引号，否则被当 glob 报 `no matches found`）：

```
12:runs:
13:  using: "node24"
```

## Residual Risks

- **降级不是编译期拦截**：照越界码调用仍会编译通过，只是运行时得 500 并出声。真正的结构性防线是
  `CLAUDE.md` 不再重抄码值清单（改为指向两张登记表），以及本轮的对账脚本。
- **对账脚本没做成常驻门**：`/speckit-bug-assess` 对本缺陷预设的退化方案就是「文档只引用文件路径、
  不列码值」，本轮按该方案落地，脚本作为其可复现验证留在 bug 目录。升格为常驻门要同时改
  `Makefile` 的 audit 步骤、`user-server-ci.yml` 的两份 `paths` 镜像与 `check-ci-gate-paths.py`
  的站点计数——那三处正是门禁泳道此刻在写的热文件；这里不是「怕碰」而是收益判断：根因面（文档抄清单）
  已被结构性消除，门的边际收益低。若要把这条判据常驻，应按 `/speckit-specify` 单开一轮而不是并进本轮。
- **`LogBindError` 一类仍裸读 `c.Request`**（`response.go:352`、`:362`）：调用前提是真实请求，
  gin 的 `ServeHTTP` 保证非空，本轮不扩面。
- **没有跑全量 `go test ./...`**：本工作树有并行泳道 200+ 个未提交文件，全量读数无法归因。
  实际跑的是两个有界集合：活树上「含整数状态码调用的 14 个包」，以及合并克隆里
  `./internal/pkg/...` 全包（39 包 `ok`／0 包 `FAIL`）+ 全树 `go vet ./...`（rc=0，含 `_test.go` 编译）。
  **未跑**：`./internal/service/...`（单包 450–880s 随负载摆动）与 `./internal/controller/...`
  （他泳道在途用例即在此包，跑了也无法归因）。
- **本机测试库前提**：不带 `POSTGRES_TEST_PASSWORD` 时 9 个包会同一句 SASL 红（`testdb.go:331`），
  文档里的 dev 端口 8232 本机无监听（实听 5432 与容器 8202）。这是环境前提，不是仓库红。

## Recommendation

Close the bug — 原始症状经实际复现双向验证已消失，三层修复（运行时降级 + 文档结构性去清单 +
ADR 对账）各自有判据守着，且现证对既有 148 处整数状态码调用零行为变更。验证阶段顺带修掉的
`localeOf` 无 Request 崩溃是同一族的独立缺陷，已一并落测。若要让「文档码值必须在登记表里」常驻，
按 spec-kit 另开一轮 specify→tasks，与门禁泳道的 CI 站点改动错峰。
