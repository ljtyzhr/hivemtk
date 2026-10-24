package ragretrieval

import (
	"sort"
)

// RRFFusion Reciprocal Rank Fusion 融合器
type RRFFusion struct {
	k int
}

// NewRRFFusion 创建 RRF 融合器，k <= 0 时使用默认值 60（Cormack 2009 SIGIR 推荐值）。
func NewRRFFusion(k int) *RRFFusion {
	if k <= 0 {
		k = 60
	}
	return &RRFFusion{k: k}
}

func (f *RRFFusion) K() int { return f.k }

type rrfEntry struct {
	chunk Chunk
	score float64
}

// Fuse 融合多路召回结果：vecResults / bm25Results 须按各自 score 降序，
// 同一 chunk ID 在两路都出现时聚合分数，输出按 RRF score 降序的 topN（默认 20）。
// 融合后 Chunk.Score 被覆盖为 RRF score（单路命中 1/k，双路命中 2/k）。
func (f *RRFFusion) Fuse(vecResults, bm25Results []Chunk, topN int) []Chunk {
	if topN <= 0 {
		topN = 20
	}
	merged := make(map[string]*rrfEntry, len(vecResults)+len(bm25Results))

	for i, c := range vecResults {
		id := c.ID
		rrfScore := 1.0 / float64(f.k+i+1)
		if e, ok := merged[id]; ok {
			e.score += rrfScore
		} else {
			merged[id] = &rrfEntry{chunk: c, score: rrfScore}
		}
	}
	for i, c := range bm25Results {
		id := c.ID
		rrfScore := 1.0 / float64(f.k+i+1)
		if e, ok := merged[id]; ok {
			e.score += rrfScore
		} else {
			merged[id] = &rrfEntry{chunk: c, score: rrfScore}
		}
	}

	list := make([]*rrfEntry, 0, len(merged))
	for _, e := range merged {
		list = append(list, e)
	}
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].score != list[j].score {
			return list[i].score > list[j].score
		}
		return list[i].chunk.ID < list[j].chunk.ID
	})
	if len(list) > topN {
		list = list[:topN]
	}

	out := make([]Chunk, 0, len(list))
	for _, e := range list {
		e.chunk.Score = e.score
		out = append(out, e.chunk)
	}
	return out
}

func trimChunks(chunks []Chunk, topK int) []Chunk { //nolint:unused // 仅被 *_test.go 引用，生产路径未用
	if topK <= 0 {
		return chunks
	}
	if len(chunks) > topK {
		return chunks[:topK]
	}
	return chunks
}
