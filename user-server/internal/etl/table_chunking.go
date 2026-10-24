package etl

// table_chunking.go P1-1 步骤 4：表格感知分块（外挂解析启用时生效）。
//
// 问题：默认分块按标点/定长切，markdown 表格（外挂引擎 RAGFlow/MinerU 的常见输出）
// 会被从行中间切断——半张表进一个 chunk，"SKU=B-7 的单价"与"B-7"分家，
// 单元格级召回直接失效（表头也只在第一块里，后面几块无从解释）。
//
// 口径：
//   - 表格块整体成块（表头 + 分隔行 + 行），不与正文混切；
//   - 超长表格按**行**拆，每片重复表头+分隔行；
//   - 单行超长也只整行保留（切行等于切单元格，宁超限不破行）；
//   - 表格块不受 MinChunkSize 过滤（小表也是完整语义单元，滤掉等于丢内容）。
//
// 只有 ProcessingConfig.TableAware=true 时启用；默认值在 NewDocumentProcessor
// 里由「是否配置了外挂解析」推导（spec：表格感知分块选项，启用外挂解析时生效）。
// 关闭时 ProcessDocument 走原路径，一个字节都不变。

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	rag_core "hivemtk-user/internal/aiagent/rag/core"
)

// tableLineRe 表格行：以 | 开头（pandoc/markdown 输出的标准形态）。
var tableLineRe = regexp.MustCompile(`^\s*\|.*\|\s*$`)

// tableSepRe 分隔行：|---|---| 或 | :--- | ---: |
var tableSepRe = regexp.MustCompile(`^\s*\|[\s:|-]+\|\s*$`)

// segment 文档切段结果：table=false 走常规分块，table=true 整体走表格分块。
type segment struct {
	text  string
	table bool
}

// splitTableSegments 把正文与表格块切开（保持原顺序）。
// 表格块 = 连续的表行（含分隔行）；允许多张表。
func splitTableSegments(text string) []segment {
	lines := strings.Split(text, "\n")
	var segs []segment
	var prose []string
	var table []string

	flushProse := func() {
		if len(prose) > 0 {
			segs = append(segs, segment{text: strings.Join(prose, "\n")})
			prose = nil
		}
	}
	flushTable := func() {
		if len(table) > 0 {
			segs = append(segs, segment{text: strings.Join(table, "\n"), table: true})
			table = nil
		}
	}

	for _, line := range lines {
		if tableLineRe.MatchString(line) {
			flushProse()
			table = append(table, line)
			continue
		}
		// 表块内遇到空行 ⇒ 表结束（空行不进表）；其余非表行一律结束表块
		flushTable()
		if strings.TrimSpace(line) == "" && len(prose) == 0 {
			continue
		}
		prose = append(prose, line)
	}
	flushTable()
	flushProse()
	return segs
}

// chunkTableBlock 表格块分块：行边界切分 + 每片重复表头。
// seq 由调用方维护（跨表连续编号，便于排查"这条 chunk 来自第几张表"）。
func (dp *DocumentProcessor) chunkTableBlock(doc rag_core.Document, block string, seq *int) []rag_core.Chunk {
	lines := strings.Split(block, "\n")
	if len(lines) == 0 {
		return nil
	}

	// 表头：第一行为表头；第二行若是分隔行（|---|）也算表头的一部分
	headerLines := 1
	if len(lines) > 1 && tableSepRe.MatchString(lines[1]) {
		headerLines = 2
	}
	header := strings.Join(lines[:headerLines], "\n")
	rows := lines[headerLines:]

	var chunks []rag_core.Chunk
	// 表头本身已超限：整块保留（切表头等于破坏语义）
	if utf8.RuneCountInString(header) > dp.config.MaxLengthPerChunk {
		(*seq)++
		return []rag_core.Chunk{dp.createTableChunk(doc, block, *seq)}
	}

	current := header
	for _, row := range rows {
		candidate := current + "\n" + row
		if utf8.RuneCountInString(candidate) > dp.config.MaxLengthPerChunk && current != header {
			(*seq)++
			chunks = append(chunks, dp.createTableChunk(doc, current, *seq))
			current = header + "\n" + row
			continue
		}
		current = candidate
	}
	(*seq)++
	chunks = append(chunks, dp.createTableChunk(doc, current, *seq))
	return chunks
}

// createTableChunk 表格块专用 chunk：复制 doc.Metadata 后打 chunk_type 标记，
// 不改共享 map（doc.Metadata 是同一份引用，直接写会污染同文档其他 chunk）。
func (dp *DocumentProcessor) createTableChunk(doc rag_core.Document, content string, seq int) rag_core.Chunk {
	meta := make(map[string]any, len(doc.Metadata)+1)
	for k, v := range doc.Metadata {
		meta[k] = v
	}
	meta["chunk_type"] = "table"
	c := dp.createChunk(doc, strings.TrimSpace(content), fmt.Sprintf("%s_table_%d", doc.ID, seq))
	c.Metadata = meta
	return c
}
