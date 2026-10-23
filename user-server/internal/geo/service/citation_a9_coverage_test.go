package service

import (
	"testing"

	"hivemtk-user/internal/geo/model"

	"github.com/stretchr/testify/assert"
)

// A9 验收（规划 §4.8 A9）：报告含"自有域引用/总引用"覆盖率及样本（Top 外链）。
func TestOwnDomainCoverage(t *testing.T) {
	stats := []CitationDomainStat{
		{Domain: "example.com", Category: "own", Count: 3},
		{Domain: "media.com", Category: "earned", Count: 5},
		{Domain: "comp.com", Category: "competitor", Count: 2},
	}
	own, total, cov := OwnDomainCoverage(stats)
	assert.Equal(t, 3, own)
	assert.Equal(t, 10, total)
	assert.InDelta(t, 0.3, cov, 1e-9)

	// 无引用 ⇒ 覆盖率 0（不是 NaN、不是除零 panic）
	own, total, cov = OwnDomainCoverage(nil)
	assert.Equal(t, 0, own)
	assert.Equal(t, 0, total)
	assert.Equal(t, float64(0), cov)

	// 全是自有 ⇒ 100%
	_, _, cov = OwnDomainCoverage([]CitationDomainStat{{Domain: "example.com", Category: "own", Count: 7}})
	assert.InDelta(t, 1.0, cov, 1e-9)
}

func TestTopExternalLinks(t *testing.T) {
	stats := []CitationDomainStat{
		{Domain: "example.com", Category: "own", Count: 99},
		{Domain: "b.com", Category: "earned", Count: 5},
		{Domain: "a.com", Category: "earned", Count: 5},
		{Domain: "comp.com", Category: "competitor", Count: 9},
	}
	links := TopExternalLinks(stats, 10)
	// 域级排除 own；competitor 保留（外链榜是"别人在哪些域发声"的读数，
	// 分类字段 category 已单独标出，不重复过滤）。并列 count 按 domain 升序钉死。
	if len(links) != 3 {
		t.Fatalf("期望 3 条外链（排除 own），实际 %d: %+v", len(links), links)
	}
	// comp.com 9 次排第一，earned 两条并列按域名升序
	assert.Equal(t, "comp.com", links[0].Domain)
	assert.Equal(t, "a.com", links[1].Domain)
	assert.Equal(t, "b.com", links[2].Domain)
	// n 截断
	assert.Len(t, TopExternalLinks(stats, 1), 1)
}

// TestTopExternalLinksWithURLs URL 级明细：非自有/非竞品的 URL 计数与并列序。
func TestTopExternalLinksWithURLs(t *testing.T) {
	runs := []*model.GeoProbeRun{
		{Citations: []byte(`[{"url":"https://media.com/a"},{"url":"https://media.com/a"},{"url":"https://example.com/x"}]`)},
		{Citations: []byte(`[{"url":"https://media.com/b"}]`)},
	}
	links := topExternalLinksWithURLs(runs, "example.com", []string{"comp.com"}, 10)
	if len(links) != 2 {
		t.Fatalf("期望 2 条 URL 明细（自有域 URL 不计），实际 %d: %+v", len(links), links)
	}
	if links[0].URL != "https://media.com/a" || links[0].Count != 2 {
		t.Errorf("Top1 应为 a(2 次)，实际 %+v", links[0])
	}
	if links[0].Domain != "media.com" {
		t.Errorf("domain 应归一化，实际 %q", links[0].Domain)
	}
}
