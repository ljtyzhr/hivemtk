// Package rag 提供 RAG 知识库相关能力，当前仅含 IncrementalIndexer：
// 订阅 event.TopicKnowledgeDocumentChanged 事件，监听知识库文档变更
// （创建/更新/删除）并触发向量化与 chunk 更新。
package rag

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/aiagent/knowledge/model"
	knowledgerepo "hivemtk-user/internal/aiagent/knowledge/repository"
	"hivemtk-user/internal/aiagent/llm"
	rag_core "hivemtk-user/internal/aiagent/rag/core"
	"hivemtk-user/internal/etl"
	"hivemtk-user/internal/event"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/utils/logger"
)

// IncrementalIndexer 知识库文档增量索引器。
// 订阅文档变更事件，按 ChangeType 分发：create/update 走解析 → 切块 →
// Embedding → 写 chunks；delete 只清本索引器的内存视图——分段行的级联删除
// 由 KnowledgeService.Delete 在同一请求里落库（这里再删会因事件重放变成二次删除）。
// 失败仅记录日志不重试（主流程可重新发布事件）。
type IncrementalIndexer struct {
	mu          sync.RWMutex
	chunksByDoc map[string][]*eventIndexedChunk
	embedder    llm.EmbeddingServiceInterface
	processor   *etl.DocumentProcessor
	docRepo     *knowledgerepo.KnowledgeDocumentRepository
	chunkRepo   *knowledgerepo.KnowledgeChunkRepository
	db          *gorm.DB
	stopped     bool
}

type eventIndexedChunk struct {
	DocumentID  string
	ChunkIndex  int
	Content     string
	ContentHash string
	Embedding   []float32
	TokenCount  int
	IndexedAt   time.Time
}

// NewIncrementalIndexer 创建增量索引器。
// embedder 为 nil 时跳过 embedding 仅切块；processor 为 nil 时使用默认配置；
// db 为 nil 时退化为内存索引模式（不查库、不持久化 chunks）。
func NewIncrementalIndexer(embedder llm.EmbeddingServiceInterface, processor *etl.DocumentProcessor, db *gorm.DB) *IncrementalIndexer {
	if processor == nil {
		processor = etl.NewDocumentProcessor(nil)
	}
	idx := &IncrementalIndexer{
		chunksByDoc: make(map[string][]*eventIndexedChunk),
		embedder:    embedder,
		processor:   processor,
		db:          db,
	}
	if db != nil {
		idx.docRepo = knowledgerepo.NewKnowledgeDocumentRepository(db)
		idx.chunkRepo = knowledgerepo.NewKnowledgeChunkRepository(db)
	}
	return idx
}

// Handle 处理知识库文档变更事件（实现 event.Handler 接口），异常 panic 被隔离
// 以不影响 worker。
func (i *IncrementalIndexer) Handle(evt event.Event) error {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("[rag] panic in incremental_indexer topic=%s err=%v\n%s",
				evt.Topic, r, debug.Stack())
		}
	}()

	if i.stopped {
		return nil
	}

	payload, ok := evt.Payload.(event.KnowledgeDocumentChangePayload)
	if !ok {
		if p, ok := evt.Payload.(*event.KnowledgeDocumentChangePayload); ok && p != nil {
			return i.handlePayload(ctxFromPayload(*p), *p)
		}
		return nil
	}

	return i.handlePayload(ctxFromPayload(payload), payload)
}

// ctxFromPayload 从事件载荷恢复租户上下文：事件线程拿不到发布点的 ctx，
// 租户归属靠载荷随行传递。载荷没带租户（off 模式或系统级发布）时保持无绑定——
// on 模式下下游隔离 callback 会 fail-closed，绝不回落默认租户，
// 那会把某个租户的文档索引进别人的库。
func ctxFromPayload(payload event.KnowledgeDocumentChangePayload) context.Context {
	ctx := context.Background()
	if payload.TenantID != 0 {
		ctx = tenant.WithTenantID(ctx, payload.TenantID)
	}
	return ctx
}

func (i *IncrementalIndexer) handlePayload(ctx context.Context, payload event.KnowledgeDocumentChangePayload) error {
	start := time.Now()

	docIDStr := strconv.FormatUint(uint64(payload.DocumentID), 10)
	logger.Infof("[rag] incremental indexer received workspace=%v doc=%d change=%s hash=%s",
		payload.WorkspaceID, payload.DocumentID, payload.ChangeType, payload.ContentHash)

	var err error
	switch payload.ChangeType {
	case "create", "update":
		err = i.indexDocument(ctx, payload, docIDStr)
	case "delete":
		err = i.deleteDocument(docIDStr)
	default:
		logger.Infof("[rag] unknown change_type=%s, ignored", payload.ChangeType)
		return nil
	}

	if err != nil {
		logger.Errorf("[rag] index document failed doc=%d err=%v", payload.DocumentID, err)
		return err
	}

	logger.Infof("[rag] incremental indexed doc=%d change=%s duration=%s",
		payload.DocumentID, payload.ChangeType, time.Since(start))
	return nil
}

func (i *IncrementalIndexer) indexDocument(ctx context.Context, payload event.KnowledgeDocumentChangePayload, docIDStr string) error {
	content, err := i.loadDocumentContent(ctx, payload, docIDStr)
	if err != nil {
		logger.Errorf("[rag] load content failed doc=%d err=%v, fallback to title", payload.DocumentID, err)
		content = fmt.Sprintf("[Content unavailable] %s", payload.ContentHash)
	}

	doc := rag_core.Document{
		ID:      docIDStr,
		Content: content,
	}
	chunks, err := i.processor.ProcessDocument(ctx, doc)
	if err != nil {
		return err
	}

	indexed := make([]*eventIndexedChunk, 0, len(chunks))
	persisted := make([]model.KnowledgeChunk, 0, len(chunks))
	for idx, c := range chunks {
		var embedding []float32
		if i.embedder != nil {
			embeddings, embedErr := i.embedder.Embed(ctx, i.embedder.DefaultConfig(), []string{c.Content})
			if embedErr != nil {
				logger.Errorf("[rag] embed chunk failed doc=%d chunk=%d err=%v", payload.DocumentID, idx, embedErr)
			} else if len(embeddings) > 0 {
				embedding = embeddings[0]
			}
		}

		chunk := &eventIndexedChunk{
			DocumentID:  docIDStr,
			ChunkIndex:  idx,
			Content:     c.Content,
			ContentHash: hashContent(c.Content),
			TokenCount:  c.TokenCount,
			Embedding:   embedding,
			IndexedAt:   time.Now(),
		}
		indexed = append(indexed, chunk)

		if i.chunkRepo != nil {
			kc := model.KnowledgeChunk{
				DocumentID:  uint64(payload.DocumentID),
				ChunkIndex:  idx,
				Content:     c.Content,
				ContentHash: chunk.ContentHash,
				TokenCount:  c.TokenCount,
				CharCount:   len([]rune(c.Content)),
				CreatedAt:   time.Now(),
			}
			persisted = append(persisted, kc)
		}
	}

	i.mu.Lock()
	delete(i.chunksByDoc, docIDStr)
	i.chunksByDoc[docIDStr] = indexed
	i.mu.Unlock()

	if i.chunkRepo != nil {
		if delErr := i.chunkRepo.DeleteByDocumentID(ctx, uint64(payload.DocumentID)); delErr != nil {
			logger.Errorf("[rag] delete old chunks failed doc=%d err=%v", payload.DocumentID, delErr)
		}
		if persistErr := i.chunkRepo.BatchCreate(ctx, persisted); persistErr != nil {
			return fmt.Errorf("persist chunks: %w", persistErr)
		}
	}

	logger.Infof("[rag] indexed doc=%s chunks=%d persisted=%t", docIDStr, len(indexed), i.chunkRepo != nil)
	return nil
}

func (i *IncrementalIndexer) loadDocumentContent(ctx context.Context, payload event.KnowledgeDocumentChangePayload, docIDStr string) (string, error) {
	if i.docRepo != nil {
		docID := uint64(payload.DocumentID)
		doc, err := i.docRepo.GetByID(ctx, docID)
		if err == nil && doc != nil {
			if doc.FilePath != "" {
				if data, readErr := os.ReadFile(doc.FilePath); readErr == nil {
					return string(data), nil
				} else {
					logger.Errorf("[rag] read file failed path=%s err=%v", doc.FilePath, readErr)
				}
			}
			if doc.Title != "" {
				return fmt.Sprintf("# %s\n\nSource: %s\nReference: %s", doc.Title, doc.SourceType, doc.SourceRef), nil
			}
		}
	}

	return "", fmt.Errorf("no content source for doc %s", docIDStr)
}

func (i *IncrementalIndexer) deleteDocument(docIDStr string) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	chunks, ok := i.chunksByDoc[docIDStr]
	if !ok {
		return nil
	}

	delete(i.chunksByDoc, docIDStr)
	logger.Infof("[rag] deleted doc=%s chunks=%d", docIDStr, len(chunks))
	return nil
}

// GetIndexedChunks 查询已索引的 chunks（返回副本）。
func (i *IncrementalIndexer) GetIndexedChunks(docIDStr string) []*eventIndexedChunk {
	i.mu.RLock()
	defer i.mu.RUnlock()
	chunks := i.chunksByDoc[docIDStr]
	result := make([]*eventIndexedChunk, len(chunks))
	copy(result, chunks)
	return result
}

func (i *IncrementalIndexer) ChunkCount(docIDStr string) int {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return len(i.chunksByDoc[docIDStr])
}

// Stop 关闭索引器，后续事件不再处理。
func (i *IncrementalIndexer) Stop() {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.stopped = true
	i.chunksByDoc = nil
}

func hashContent(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}
