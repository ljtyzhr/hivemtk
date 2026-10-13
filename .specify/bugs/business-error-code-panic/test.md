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
- **没有跑全量 `go test ./...`**：本工作树有并行泳道 200+ 个未提交文件，全量读数无法归因；
  改跑的是「含整数状态码调用的 14 个包」这一有界集合，外加对越界码的 0 调用点计数。
- **本机测试库前提**：不带 `POSTGRES_TEST_PASSWORD` 时 9 个包会同一句 SASL 红（`testdb.go:331`），
  文档里的 dev 端口 8232 本机无监听（实听 5432 与容器 8202）。这是环境前提，不是仓库红。

## Recommendation

Close the bug — 原始症状经实际复现双向验证已消失，三层修复（运行时降级 + 文档结构性去清单 +
ADR 对账）各自有判据守着，且现证对既有 148 处整数状态码调用零行为变更。验证阶段顺带修掉的
`localeOf` 无 Request 崩溃是同一族的独立缺陷，已一并落测。若要让「文档码值必须在登记表里」常驻，
按 spec-kit 另开一轮 specify→tasks，与门禁泳道的 CI 站点改动错峰。
