package pgvector

import (
	"strings"
	"testing"

	"gorm.io/gorm/clause"
)

// TestOrderByDistance_SQLContainsOrderBy 锁定核心回归：
// 子句必须带 ORDER BY 语义，且向量字面量走绑定参数而不是拼进 SQL 文本。
func TestOrderByDistance_SQLContainsOrderBy(t *testing.T) {
	ob := OrderByEmbeddingDistance("[1,2,3]")

	expr, ok := ob.Expression.(clause.Expr)
	if !ok {
		t.Fatalf("OrderBy.Expression 应为 clause.Expr，实际 %T", ob.Expression)
	}
	if expr.SQL != "embedding <=> ?::vector" {
		t.Fatalf("ORDER BY 表达式不对: %q", expr.SQL)
	}
	if len(expr.Vars) != 1 || expr.Vars[0] != "[1,2,3]" {
		t.Fatalf("ORDER BY 绑定变量不对: %#v", expr.Vars)
	}
	if strings.Contains(expr.SQL, "[1,2,3]") {
		t.Fatal("向量字面量不应出现在 SQL 文本里，必须走绑定参数")
	}
}

// TestOrderByDistance_QueryVectorColumn 另一个白名单列也要能生成表达式。
func TestOrderByDistance_QueryVectorColumn(t *testing.T) {
	ob := OrderByQueryVectorDistance("[4,5]")

	expr, ok := ob.Expression.(clause.Expr)
	if !ok {
		t.Fatalf("OrderBy.Expression 应为 clause.Expr，实际 %T", ob.Expression)
	}
	if expr.SQL != "query_vector <=> ?::vector" {
		t.Fatalf("ORDER BY 表达式不对: %q", expr.SQL)
	}
}
