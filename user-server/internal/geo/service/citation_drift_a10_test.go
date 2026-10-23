package service

import (
	"testing"

	"hivemtk-user/internal/geo/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A10 验收（规划 §4.8 A10）：定时快照 CitedURLs，两次快照可 diff（增删/排名变化可复现）。

func TestAggregateRunCitationCounts(t *testing.T) {
	runs := []*model.GeoProbeRun{
		{Citations: []byte(`[{"url":"https://a.com/x"},{"url":"https://b.com/y"}]`)},
		{Citations: []byte(`[{"url":"https://a.com/x"}]`)},
		{Citations: []byte(`bad json`)}, // 坏行跳过不炸
		{Citations: nil},                // 空行跳过
	}
	counts := AggregateRunCitationCounts(runs)
	assert.Equal(t, map[string]int{"https://a.com/x": 2, "https://b.com/y": 1}, counts)
}

func TestBuildDrift_AddRemoveChange(t *testing.T) {
	from := map[string]int{"https://a.com/1": 3, "https://b.com/2": 5}
	to := map[string]int{"https://a.com/1": 1, "https://c.com/3": 2, "https://b.com/2": 5}
	res := buildDrift("2026-10-23", "2026-10-24", from, to)

	assert.Equal(t, "2026-10-23", res.FromDate)
	assert.Equal(t, "2026-10-24", res.ToDate)

	// 新增：c.com（from 无 → to 有）
	require.Len(t, res.Added, 1)
	assert.Equal(t, "https://c.com/3", res.Added[0].URL)
	assert.Equal(t, 2, res.Added[0].Count)
	assert.Equal(t, "c.com", res.Added[0].Domain)

	// 消失：b.com/2（from 有 → to 无）——注意 to 里 b/2 计数同为 5，
	// 应视为"消失"还是"不变"？from 有 b/2=5、to 有 b/2=5 ⇒ 不变；无 removed。
	// 上面 to 里 b/2=5、from 里 b/2=5，但循环遍历 to ⇒ b/2 走 default 分支（不变）。
	// removed 应为空：
	assert.Empty(t, res.Removed, "两侧同 URL 同计数不是漂移")

	// 变化：a.com/1 3→1
	require.Len(t, res.Changed, 1)
	assert.Equal(t, "https://a.com/1", res.Changed[0].URL)
	assert.Equal(t, -2, res.Changed[0].Delta)
}

func TestBuildDrift_RemovedAndRank(t *testing.T) {
	from := map[string]int{
		"https://a.com/1":  9, // 排名1
		"https://b.com/2":  5, // 排名2
		"https://c.com/3":  1, // 排名3
		"https://gone.com": 4,
	}
	to := map[string]int{
		"https://a.com/1":   2, // 9→2：计数变 + 排名 1→3
		"https://b.com/2":   5, // 不变
		"https://c.com/3":   8, // 1→8：计数变 + 排名 3→1
		"https://new.com/4": 6,
	}
	res := buildDrift("d1", "d2", from, to)

	// 消失：gone.com
	require.Len(t, res.Removed, 1)
	assert.Equal(t, "https://gone.com", res.Removed[0].URL)
	assert.Equal(t, 4, res.Removed[0].Count)

	// 新增：new.com/4
	require.Len(t, res.Added, 1)
	assert.Equal(t, "https://new.com/4", res.Added[0].URL)

	// 计数变化：a(9→2) 与 c(1→8)；b 不变
	require.Len(t, res.Changed, 2)

	// 排名变化：a 1→4、c 3→1（to 侧排名：c=1, new=2, b=3, a=4）；b 排名不变（2→3？）
	// b: from 排名 2（a9,b5,c1,gone4 → a1,b2,gone3,c4）… 重算：from 排名 a=1,b=2,gone=3,c=4。
	// to 排名：c=1,new=2,b=3,a=4。b 从 2→3 也变了！rank_moves 应含 a/b/c 三条。
	require.Len(t, res.RankMoves, 3)
	moves := map[string]CitationRankMove{}
	for _, m := range res.RankMoves {
		moves[m.URL] = m
	}
	assert.Equal(t, 1, moves["https://a.com/1"].FromRank)
	assert.Equal(t, 4, moves["https://a.com/1"].ToRank)
	assert.Equal(t, 4, moves["https://c.com/3"].FromRank)
	assert.Equal(t, 1, moves["https://c.com/3"].ToRank)
	assert.Equal(t, 2, moves["https://b.com/2"].FromRank)
	assert.Equal(t, 3, moves["https://b.com/2"].ToRank)
}

func TestBuildDrift_DeterministicOrder(t *testing.T) {
	from := map[string]int{"https://b.com": 1, "https://a.com": 1, "https://c.com": 1}
	to := map[string]int{"https://x.com": 1, "https://y.com": 1}
	r1 := buildDrift("d1", "d2", from, to)
	r2 := buildDrift("d1", "d2", from, to)
	assert.Equal(t, r1, r2, "两次 diff 必须同输出（URL 升序钉死）")
	// 新增按 URL 升序
	assert.Equal(t, "https://x.com", r1.Added[0].URL)
	assert.Equal(t, "https://y.com", r1.Added[1].URL)
}

// TestBuildDrift_NilMaps 空快照 diff 不炸：两侧都空 ⇒ 全空 diff。
func TestBuildDrift_NilMaps(t *testing.T) {
	res := buildDrift("d1", "d2", nil, nil)
	assert.Empty(t, res.Added)
	assert.Empty(t, res.Removed)
	assert.Empty(t, res.Changed)
	assert.Empty(t, res.RankMoves)
}
