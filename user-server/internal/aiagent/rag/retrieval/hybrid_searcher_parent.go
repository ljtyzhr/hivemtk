package ragretrieval

// hybrid_searcher_parent.go 父子分段的检索侧回拼（P1-1 前置小刀）。
//
// 命中子块后把父块文本换进返回结果的 Content，让进 prompt 的上下文覆盖整窗；
// 分数、ID、文档归属一律不动——父块只是「上下文扩窗」，不改变命中口径。

import (
	"context"

	"hivemtk-user/internal/aiagent/knowledge/model"
)

// parentStitchMaxChars 回拼后的字符上限。父块构建侧已按 parentMaxChars(1800) 卡过，
// 这里再兜一次：父块内容可能来自更早的参数口径（改了窗口大小后存量行仍是旧长度），
// 超限就不回拼——宁可少给上下文，也不让一条命中挤掉其余命中的预算。
var parentStitchMaxChars = 1800

// attachParentContext 把命中条目的 Content 换成父块内容。
//
// 行为闸（AC「父块不存在时行为与现状逐字节一致」）：
//   - 无父块引用（历史数据/短文档）：整条不动；
//   - 父块行缺失（被清理/悬空引用）：该条不动；
//   - 父块超字符上限：该条不动；
//   - db 未装配（纯单测构造）：整批不动。
func (h *HybridSearcher) attachParentContext(ctx context.Context, chunks []Chunk) []Chunk {
	if h.db == nil || len(chunks) == 0 {
		return chunks
	}
	ids := make([]uint64, 0, len(chunks))
	seen := make(map[uint64]struct{}, len(chunks))
	for _, c := range chunks {
		if c.ParentChunkID == 0 {
			continue
		}
		if _, ok := seen[c.ParentChunkID]; ok {
			continue
		}
		seen[c.ParentChunkID] = struct{}{}
		ids = append(ids, c.ParentChunkID)
	}
	if len(ids) == 0 {
		return chunks
	}

	var rows []model.KnowledgeChunkParent
	if err := h.db.WithContext(ctx).
		Select("id, content, token_count, char_count").
		Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return chunks // 父块查询失败按「无父块」处理：不因扩窗失败丢掉已命中的结果
	}
	byID := make(map[uint64]string, len(rows))
	for _, r := range rows {
		if r.Content == "" || len([]rune(r.Content)) > parentStitchMaxChars {
			continue
		}
		byID[r.ID] = r.Content
	}
	if len(byID) == 0 {
		return chunks
	}
	out := make([]Chunk, len(chunks))
	copy(out, chunks)
	for i := range out {
		if parent, ok := byID[out[i].ParentChunkID]; ok {
			out[i].Content = parent
		}
	}
	return out
}
