package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// A11 验收（规划 §4.8 A11）：验证 prompt 要求输出品牌语境；解析入库；未知短语保留。
//
// 入库链路在 VerifyArticle（LLM 依赖，live 测试面）；本腿钉的是纯解析与归一层——
// parseVerifyResponse 吃到带 brand_context 的 JSON 后字段确实被带出来，
// normalizeBrandContext 的归并口径与前端展示一致。

func TestParseVerifyResponse_BrandContext(t *testing.T) {
	content := `前置说明 {"response":"摘要","brand_mentioned":true,"mention_count":2,
"mention_positions":["开篇首句"],"sentiment":"positive","brand_context":"价格对比",
"competitors_mentioned":["竞品A"]} 尾部`
	parsed := parseVerifyResponse(content)
	if parsed.BrandContext != "价格对比" {
		t.Fatalf("brand_context 应解析出来，实际 %q", parsed.BrandContext)
	}
	if !parsed.BrandMentioned || parsed.MentionCount != 2 {
		t.Fatalf("其余字段解析被破坏: %+v", parsed)
	}
}

func TestNormalizeBrandContext(t *testing.T) {
	cases := []struct{ in, want string }{
		{"价格对比", "价格对比"},
		{"price_comparison", "价格对比"}, // 英文枚举归并为中文展示键
		{"  Feature_Intro ", "功能介绍"}, // 大小写与空白
		{"", ""},                        // 空 ⇒ 空（不造语义）
		{"行业排名 vs 竞品", "行业排名 vs 竞品"}, // 未知短语原样保留——不静默归并掉语义
	}
	for _, c := range cases {
		assert.Equal(t, c.want, normalizeBrandContext(c.in), "input=%q", c.in)
	}
}

// TestVerifySearchPrompt_AsksBrandContext prompt 必须真的要求 brand_context——
// 模型列悬空半年（只定义不写）的根因就是 prompt 和解析两头都没有；这一句红说明有人改了 prompt 没同步解析（或反之）。
func TestVerifySearchPrompt_AsksBrandContext(t *testing.T) {
	out := VerifySearchPrompt(t.Context(), "Acme", "best crm 2026", "搜索结果…")
	if !strings.Contains(out, "brand_context") || !strings.Contains(out, "品牌语境") {
		t.Fatalf("VerifySearchPrompt 未要求 brand_context 输出（A11 契约）")
	}
}
