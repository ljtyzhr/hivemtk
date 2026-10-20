# Bug Test: 向量召回排序子句被 GORM 静默丢弃的回归验证

- **Slug**: gorm-order-clause-expr-silently-dropped
- **Tested**: 2026-10-20
- **Fix**: [fix.md](fix.md)
- **Result**: verified
- **Status**: verified

## Verification Commands

### 1. 上游行为取证（判据来源）

```bash
GOMOD=$(go env GOMODCACHE)/$(go list -m gorm.io/gorm | tr ' ' '@')
grep -n "func (db \*DB) Order" -A 20 "$GOMOD/chainable_api.go"
grep -n "type OrderBy struct" -A 5 "$GOMOD/clause/order_by.go"
```

**结果**：

- `Order(value interface{})` 的 `switch` 只有 `clause.OrderBy` / `clause.OrderByColumn` / `string` 三个 case，**无 default 分支** ⇒ 传 `clause.Expr` 无事发生，**不报错**。
- `clause.OrderBy` 结构体字段为 `Columns []OrderByColumn` + `Expression Expression`；`Build` 里 `if orderBy.Expression != nil { orderBy.Expression.Build(builder) }` ⇒ 把 `clause.Expr` 包进 `OrderBy.Expression` 即可生效，且 `clause.Expr` 的 `?` 会被替换成绑定参数。
- `clause.OrderByColumn` 只有 `Column` / `Desc` / `Reorder` 三个字段，**没有 Expression 字段** ⇒ 走 `OrderByColumn` 路线只能内联列名（向量字面量进 SQL 文本）。

### 2. 公共包单测（子句形状锁）

```bash
cd user-server
go test ./internal/pkg/pgvector/ -count=1 -v
```

**结果**：

```
--- PASS: TestOrderByDistance_SQLContainsOrderBy (0.00s)
--- PASS: TestOrderByDistance_QueryVectorColumn (0.00s)
ok  	hivemtk-user/internal/pkg/pgvector	0.309s
```

### 3. 真实库回归（Top1 相似度最高）

```bash
PW="$(docker exec mtk-postgres printenv POSTGRES_PASSWORD)"
export POSTGRES_TEST_HOST=127.0.0.1 POSTGRES_TEST_PORT=8202 \
       POSTGRES_TEST_USER=admin POSTGRES_TEST_PASSWORD="$PW"
go test ./internal/aiagent/knowledge/service -count=1
```

**结果**：`ok hivemtk-user/internal/aiagent/knowledge/service 29.222s` ✅
**修复前**：`FAIL` —— `TestRagSearcher_RealVectorSearch` 的「退货政策检索(pgvector余弦)」「发货时间检索(pgvector余弦)」两个子用例红（ORDER BY 缺失 ⇒ `LIMIT topK` 截的是无序结果 ⇒ Top1 不是最相似条目）。

### 4. 相关包回归（5 个调用点全覆盖）

```bash
go test ./internal/aiagent/knowledge/controller -count=1
go test ./internal/aiagent/rag/... ./internal/repository/... ./internal/pkg/pgvector/... -count=1 -p 1
```

**结果**：全 ok ✅

| 包 | 结果 |
|----|------|
| `internal/aiagent/knowledge/controller` | ok 52.799s |
| `internal/aiagent/rag/cache` | ok 4.625s |
| `internal/aiagent/rag/core` | ok 0.427s |
| `internal/aiagent/rag/customer_service` | ok 0.860s |
| `internal/aiagent/rag/eval` | ok 2.097s |
| `internal/aiagent/rag/incremental` | ok 2.280s |
| `internal/aiagent/rag/retrieval` | ok 6.224s |
| `internal/aiagent/rag/service` | ok 0.487s |
| `internal/repository` | ok 107.716s |
| `internal/pkg/pgvector` | ok 0.309s |

### 5. 静态确认：5 个调用点已改，且 `clause.Expr` 不再直接进 `Order`

```bash
cd user-server
grep -rn "Order(clause.Expr" --include=*.go . || echo "NO_DIRECT_EXPR_ORDER"
grep -rn "pgvector.OrderBy" --include=*.go . | grep -v _test
```

**结果**：`NO_DIRECT_EXPR_ORDER` ✅；5 个调用点全部走 `pgvector.OrderBy*` ✅

### 6. 编译与格式

```bash
go build ./... && gofmt -l internal/pkg/pgvector internal/repository internal/aiagent/rag internal/aiagent/knowledge/service
```

**结果**：`BUILD_OK`；`gofmt -l` 输出为空 ✅

## Test Results

| 检查项 | 结果 |
|--------|------|
| GORM `Order()` 无 default 分支（丢弃 `clause.Expr`） | ✅ 上游源码取证 |
| `clause.OrderBy.Expression` 分支可承载 `clause.Expr` 且走绑定参数 | ✅ 上游源码取证 + DryRun 探针 |
| `OrderByColumn` 无 `Expression` 字段（不能走该路线） | ✅ 上游源码取证 |
| `internal/pkg/pgvector` 2 例 | ✅ PASS |
| `TestRagSearcher_RealVectorSearch` 由红转绿 | ✅ knowledge/service ok 29.222s |
| 10 个相关包全 ok | ✅ |
| 全仓无 `Order(clause.Expr` 残留 | ✅ |
| `go build` + `gofmt -l` | ✅ 干净 |

## Conclusion

向量召回的 ORDER BY 真正进 SQL。修法是新增公共包 `internal/pkg/pgvector`，把排序子句收口成 `clause.OrderBy{Expression: clause.Expr{...}}`：距离表达式以绑定参数（`$1`）下发，向量字面量不拼进 SQL 文本，5 个调用点统一走这一个入口，不会再有人手写 `Order(clause.Expr{...})` 被静默丢弃。
