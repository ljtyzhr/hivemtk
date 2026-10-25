// Package dto 答案引用溯源（T2，见 docs/architecture/design/T2-answer-citation.md）
//
// 引用结构在 HTTP 同步响应（VisitorSendMessageResult.Citations）、
// SalesResponse.Citations、HandleResult.Citations 与
// session_messages.citations 快照四处共用同一份定义。
package dto

// 引用类型与匹配方式取值
const (
	// CitationTypeFAQ FAQ 级引用（Chatbase 式来源卡片）
	CitationTypeFAQ = "faq"
	// CitationTypeChunk chunk 级引用（FastGPT/RAGFlow 式，阶段二）
	CitationTypeChunk = "chunk"

	// FAQMatchExact 问题串/关键词原样命中（强证据，可作高置信引用）
	FAQMatchExact = "exact"
	// FAQMatchToken 词元命中（兜底）
	FAQMatchToken = "token"
	// FAQMatchKeywordFallback 泛词兜底命中（前端折叠展示，低分直接丢弃）
	FAQMatchKeywordFallback = "keyword_fallback"
	// FAQMatchSemantic 语义/向量命中
	FAQMatchSemantic = "semantic"
)

// FAQCitation 阶段一 FAQ 级引用
type FAQCitation struct {
	FAQEntryID uint    `json:"faq_entry_id"`     // faq_entries.id
	Question   string  `json:"question"`         // FAQ 问句（卡片标题）
	Answer     string  `json:"answer,omitempty"` // 标准答案摘要（前 200 字）
	MatchType  string  `json:"match_type"`       // exact | token | keyword_fallback | semantic
	Score      float64 `json:"score"`            // ScoreCandidates 得分
	Confidence float64 `json:"confidence"`       // 命中时点的 confidence 快照
}

// ChunkCitation 阶段二 chunk 级引用（同文档聚合）
type ChunkCitation struct {
	ChunkID     uint64  `json:"chunk_id"`
	DocumentID  uint    `json:"document_id"`
	DocTitle    string  `json:"doc_title"`
	ChunkIndex  int     `json:"chunk_index"`
	Score       float64 `json:"score"`
	Snippet     string  `json:"snippet"`      // chunk 内容（截断 500 字）
	StartOffset int64   `json:"start_offset"` // 0 = 未回填，前端不高亮
	EndOffset   int64   `json:"end_offset"`
	Page        int     `json:"page"`
}

// DocAgg RAGFlow 式按文档聚合的来源汇总
type DocAgg struct {
	DocumentID uint   `json:"document_id"`
	DocTitle   string `json:"doc_title"`
	ChunkCount int    `json:"chunk_count"`
}

// Citation 一次回答的单条引用（type 区分 FAQ/chunk）
type Citation struct {
	Type    string          `json:"type"`
	FAQ     *FAQCitation    `json:"faq,omitempty"`
	Chunks  []ChunkCitation `json:"chunks,omitempty"`
	DocAggs []DocAgg        `json:"doc_aggs,omitempty"`
}

// CitationCountMax 引用透传条数上限（FAQ top1~top3，防引用面板变检索 dump）
const CitationCountMax = 3

// CitationSnippetMax chunk 摘要截断长度
const CitationSnippetMax = 500

// FAQAnswerSummaryMax FAQ 卡片答案摘要长度
const FAQAnswerSummaryMax = 200

// ShouldDropCitation 泛化防护红线（设计 §6.3）：
// 泛词兜底命中且低分直接丢弃，不进引用透传。
func ShouldDropCitation(c *Citation) bool {
	if c == nil || c.Type != CitationTypeFAQ || c.FAQ == nil {
		return false
	}
	return c.FAQ.MatchType == FAQMatchKeywordFallback && c.FAQ.Score < 0.3
}

// TruncateRunes 按 rune 截断（UTF-8 安全）
func TruncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}
