package service

import (
	"testing"
	"time"

	"hivemtk-user/internal/geo/model"

	"github.com/stretchr/testify/assert"
)

// A12 验收（规划 §4.8 A12）：按内容类型分组统计提及率/引用率，报表可按类型下钻、样本量标注。
func TestBuildContentFormatAttribution(t *testing.T) {
	mk := func(intent, brandPos, cited string) *model.GeoQueryChain {
		return &model.GeoQueryChain{Intent: intent, BrandPosition: brandPos, CitedURLs: cited}
	}
	chains := []*model.GeoQueryChain{
		mk("对比", "candidate", "https://a.com"),
		mk("对比", "absent", ""),
		mk("对比", "first", "https://b.com"),
		mk("疑问", "absent", ""),
		mk("疑问", "", ""), // 空 BrandPosition 不算提及（unknown ≠ absent 之外都没证据）
		mk("", "absent", ""), // 空意图 → 未分类
	}
	rep := BuildContentFormatAttribution(30, chains)

	if len(rep.Rows) != 3 {
		t.Fatalf("期望 3 行（对比/疑问/未分类），实际 %d: %+v", len(rep.Rows), rep.Rows)
	}
	// 行序：样本量降序、意图名升序兜底 —— 对比(3)/疑问(2)/未分类(1)
	if rep.Rows[0].Intent != "对比" || rep.Rows[1].Intent != "疑问" || rep.Rows[2].Intent != "未分类" {
		t.Fatalf("行序错误: %+v", rep.Rows)
	}
	// 对比：3 查询、2 提及、2 引用
	r0 := rep.Rows[0]
	if r0.Queries != 3 || r0.Mentions != 2 || r0.CitedQueries != 2 {
		t.Errorf("对比行计数错误: %+v", r0)
	}
	if r0.MentionRate <= 0.66 || r0.MentionRate >= 0.67 || r0.CitationRate <= 0.66 || r0.CitationRate >= 0.67 {
		t.Errorf("比率错误: %+v", r0)
	}
	// 对比意图在矩阵里有 ContentFormat；未分类不伪造归因
	if r0.ContentFormat == "" {
		t.Errorf("对比行的 content_format 应来自意图矩阵")
	}
	if rep.Rows[2].ContentFormat != "" {
		t.Errorf("未分类不得回退挂'信息'的 ContentFormat（伪造归因）: %q", rep.Rows[2].ContentFormat)
	}
}

// TestBuildContentFormatAttribution_SampleFloor 样本量下限常量在（前端/读数方据它标注小样本）。
func TestBuildContentFormatAttribution_SampleFloor(t *testing.T) {
	if ContentFormatAttributionSampleFloor < 1 {
		t.Fatal("样本量下限必须是正数")
	}
}

// TestContentFormatAttribution_Determinism 同数据两次聚合结果一致（map 迭代序随机不能漏进报表）。
func TestContentFormatAttribution_Determinism(t *testing.T) {
	chains := []*model.GeoQueryChain{
		{Intent: "推荐", BrandPosition: "candidate", CitedURLs: "u"},
		{Intent: "评测", BrandPosition: "absent"},
		{Intent: "教程", BrandPosition: "first"},
		{Intent: "信息", BrandPosition: "absent"},
		{Intent: "推荐", BrandPosition: "negative"},
	}
	a := BuildContentFormatAttribution(7, chains)
	b := BuildContentFormatAttribution(7, chains)
	assert.Equal(t, a, b)
}

// TestListSinceRepo 是仓储腿（DB 真库）：ListSince 只回窗口内行。
// DB 依赖的用例放这里要防 CI 无 PG——挂 testutil 短路模式。
func TestContentFormatAttribution_EmptyChains(t *testing.T) {
	rep := BuildContentFormatAttribution(30, nil)
	if rep.Rows == nil || len(rep.Rows) != 0 {
		t.Fatalf("空输入应得空行集（JSON 序列化成 [] 而非 null）")
	}
	_ = time.Now // 保持 time import（GetContentFormatAttribution 签名联动）
}
