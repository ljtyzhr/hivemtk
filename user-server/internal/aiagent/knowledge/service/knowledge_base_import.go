// Package service 知识库子域 —— 文档导入与异步处理
//
// 文档导入（保存文件 + 创建记录）+ 异步处理流水线（分片/向量化/入索引）。
// 单一职责：所有"把外部文件变成可检索内容"的代码集中在此文件。
package service

import (
	"context"
	"errors"
	"fmt"
	"hivemtk-user/internal/aiagent/knowledge/model"
	rag_core "hivemtk-user/internal/aiagent/rag/core"
	ragretrieval "hivemtk-user/internal/aiagent/rag/retrieval"
	"hivemtk-user/internal/pkg/utils/async"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/storage"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ImportDocumentResult 单文档导入结果
type ImportDocumentResult struct {
	DocumentID uint   `json:"document_id"`
	Title      string `json:"title"`
	FilePath   string `json:"file_path"`
	Status     string `json:"status"`
}

// DefaultMaxUploadFileSize 是参数中心 knowledge.max_upload_file_size 的兜底值（50MB）。
const DefaultMaxUploadFileSize int64 = 50 << 20

// maxUploadFileSizeProvider 由 internal/app 的参数装配层注入。
// 传 nil 表示不注入（装配顺序错时不该把兜底值顶掉）。
var maxUploadFileSizeProvider = func() int64 { return DefaultMaxUploadFileSize }

// SetMaxUploadFileSizeProvider 注入知识库上传大小上限读取器，仅装配层调用。
func SetMaxUploadFileSizeProvider(fn func() int64) {
	if fn != nil {
		maxUploadFileSizeProvider = fn
	}
}

// MaxUploadFileSize 返回生效中的知识库上传大小上限（字节）。
// 调大会同步抬高 io.LimitReader 的读取上限与 body_limit 中间件需要容忍的单文件体积。
func MaxUploadFileSize() int64 {
	n := maxUploadFileSizeProvider()
	if n <= 0 {
		return DefaultMaxUploadFileSize
	}
	return n
}

// ImportDocument 导入文档:保存文件 + 创建记录(status=pending) + 异步处理
func (s *KnowledgeBaseService) ImportDocument(ctx context.Context, title string, file multipart.File, header *multipart.FileHeader) (*ImportDocumentResult, error) {
	if header == nil {
		return nil, errors.New("文件不能为空")
	}
	if header.Size > MaxUploadFileSize() {
		return nil, fmt.Errorf("文件过大: %d 字节, 上限 %d MB", header.Size, MaxUploadFileSize()>>20)
	}

	ext := strings.ToLower(filepath.Ext(header.Filename))
	allowed := map[string]bool{".pdf": true, ".docx": true, ".doc": true, ".txt": true, ".md": true}
	if !allowed[ext] {
		return nil, fmt.Errorf("不支持的文件类型: %s", ext)
	}

	uploadBase, _, _ := storage.LocalSource()
	// 租户分域（A7）：on 模式落 `tenant_{id}/`，与 LocalDriver 用**同一个**前缀函数
	// （storage.TenantPathPrefix）。off 模式前缀为空串 ⇒ 路径与升级前逐字节一致，
	// 存量文件（根域 `knowledge-base/…`）原地可读，零迁移。
	uploadDir := filepath.Join(uploadBase, storage.TenantPathPrefix(ctx), "knowledge-base", time.Now().Format("20060102"))
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return nil, fmt.Errorf("创建上传目录失败: %w", err)
	}
	filename := uuid.New().String() + ext
	filePath := filepath.Join(uploadDir, filename)

	dst, err := os.Create(filePath)
	if err != nil {
		return nil, fmt.Errorf("创建文件失败: %w", err)
	}
	defer func() { _ = dst.Close() }()

	if _, err := io.Copy(dst, io.LimitReader(file, MaxUploadFileSize()+1)); err != nil {
		_ = os.Remove(filePath)
		return nil, fmt.Errorf("写入文件失败: %w", err)
	}
	if err := dst.Close(); err != nil {
		_ = os.Remove(filePath)
		return nil, fmt.Errorf("写入文件失败: %w", err)
	}
	if size, _ := getFileSize(filePath); size > MaxUploadFileSize() {
		_ = os.Remove(filePath)
		return nil, fmt.Errorf("文件超过大小上限 %d MB", MaxUploadFileSize()>>20)
	}

	size, _ := getFileSize(filePath)

	if title == "" {
		title = strings.TrimSuffix(header.Filename, ext)
	}

	doc := &model.KBDocument{
		Title:    title,
		FilePath: filePath,
		FileSize: size,
		FileType: ext,
		Status:   model.KBDocumentStatusPending,
	}
	if err := s.docs.Create(ctx, doc); err != nil {
		_ = os.Remove(filePath)
		return nil, fmt.Errorf("保存文档记录失败: %w", err)
	}

	async.RunWithTimeout(ctx, AsyncProcessingTimeout(), func(procCtx context.Context) {
		s.processDocumentAsync(procCtx, doc.ID, filePath)
	})

	return &ImportDocumentResult{
		DocumentID: doc.ID,
		Title:      doc.Title,
		FilePath:   filePath,
		Status:     string(doc.Status),
	}, nil
}

// IngestLocalFile 连接器入口：把已在本地磁盘的文件导入知识库（复用
// ImportDocument 的落库+异步处理流程，绕过 multipart 上传）。
// ext 必须带点（如 ".pdf"）且在白名单内；size 为 0 时自动 stat。
func (s *KnowledgeBaseService) IngestLocalFile(ctx context.Context, title, filePath, ext string) (uint, error) {
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	ext = strings.ToLower(ext)
	allowed := map[string]bool{".pdf": true, ".docx": true, ".doc": true, ".txt": true, ".md": true}
	if !allowed[ext] {
		return 0, fmt.Errorf("不支持的文件类型: %s", ext)
	}
	size, err := getFileSize(filePath)
	if err != nil {
		return 0, fmt.Errorf("读取文件失败: %w", err)
	}
	if size > MaxUploadFileSize() {
		return 0, fmt.Errorf("文件过大: %d 字节, 上限 %d MB", size, MaxUploadFileSize()>>20)
	}
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(filePath), ext)
	}
	doc := &model.KBDocument{
		Title:    title,
		FilePath: filePath,
		FileSize: size,
		FileType: ext,
		Status:   model.KBDocumentStatusPending,
	}
	if err := s.docs.Create(ctx, doc); err != nil {
		return 0, fmt.Errorf("保存文档记录失败: %w", err)
	}
	async.RunWithTimeout(ctx, AsyncProcessingTimeout(), func(procCtx context.Context) {
		s.processDocumentAsync(procCtx, doc.ID, filePath)
	})
	return doc.ID, nil
}

// ProcessDocumentByID 对已存在（content 直存列里）的文档跑同一套异步处理链路
// （分段→向量化→索引）。T4 爬虫与 QA 候选发布使用——爬得页面不走磁盘文件，
// content 直接写 kb_documents.content，因此不能复用 processDocumentAsync 的
// os.ReadFile 路径。复用的是同一套 processor/vector/indexer 装配。
func (s *KnowledgeBaseService) ProcessDocumentByID(ctx context.Context, documentID uint) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("[knowledge-base] ProcessDocumentByID panic doc=%d: %v", documentID, r)
			s.markDocumentFailed(ctx, documentID, fmt.Sprintf("处理异常: %v", r))
		}
	}()
	var doc model.KBDocument
	if err := s.db.WithContext(ctx).First(&doc, documentID).Error; err != nil {
		logger.Errorf("[KBImport] ProcessDocumentByID 读文档失败 doc=%d: %v", documentID, err)
		return
	}
	s.processContent(ctx, &doc, doc.Content)
}

// processContent content → 分片 → 向量化 → 入索引 → 状态推进
// （processDocumentAsync 的读文件后半段收敛到此，共用同一套 processor/vector/indexer）。
func (s *KnowledgeBaseService) processContent(ctx context.Context, doc *model.KBDocument, content string) {
	documentID := doc.ID
	if err := s.docs.UpdateStatusFields(ctx, documentID, map[string]any{
		"status": model.KBDocumentStatusProcessing, "error_msg": "",
	}); err != nil {
		// 状态没推进就继续跑没有任何意义：文档会一直停在 pending，重试也无从下手。
		logger.Errorf("[KBImport] 标记文档处理中失败 doc=%d: %v", documentID, err)
	}

	ragDoc := rag_core.Document{
		ID:      fmt.Sprintf("kbdoc_%d", documentID),
		Content: content,
		Metadata: map[string]any{
			"document_id": documentID,
			"title":       doc.Title,
		},
		CreatedAt: time.Now(),
	}

	chunks, err := s.processor.ProcessDocument(ctx, ragDoc)
	if err != nil {
		s.markDocumentFailed(ctx, documentID, fmt.Sprintf("分片失败: %v", err))
		return
	}

	if len(chunks) == 0 {
		s.markDocumentFailed(ctx, documentID, "文档分片结果为空")
		return
	}

	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Content
	}
	embeddings, err := s.vector.EmbedBatch(texts)
	if err != nil {
		s.markDocumentFailed(ctx, documentID, fmt.Sprintf("向量化失败: %v", err))
		return
	}

	idxChunks := make([]ragretrieval.Chunk, len(chunks))
	for i, c := range chunks {
		idxChunks[i] = ragretrieval.Chunk{
			ID:         c.ID,
			DocumentID: c.DocumentID,
			Content:    c.Content,
			Metadata:   c.Metadata,
			Embedding:  embeddings[i],
			Score:      c.Score,
			TokenCount: c.TokenCount,
		}
	}

	kbIndexID := fmt.Sprintf("kb_%d", documentID)
	if err := s.indexer.BuildIndex(ctx, kbIndexID, idxChunks); err != nil {
		s.markDocumentFailed(ctx, documentID, fmt.Sprintf("构建索引失败: %v", err))
		return
	}

	if err := s.docs.UpdateStatusFields(ctx, documentID, map[string]any{
		"status":      model.KBDocumentStatusIndexed,
		"chunk_count": len(chunks),
		"content":     content,
		"error_msg":   "",
	}); err != nil {
		// 索引已建成但状态写失败：文档将滞留 processing，前端无法感知——必须落日志供运维介入
		logger.Errorf("[KBImport] 标记文档已索引失败 doc=%d: %v", documentID, err)
	}
}

func (s *KnowledgeBaseService) processDocumentAsync(ctx context.Context, documentID uint, filePath string) {
	bgCtx := ctx
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("[knowledge-base] processDocumentAsync panic doc=%d: %v\n%s", documentID, r, debug.Stack())
			s.markDocumentFailed(bgCtx, documentID, fmt.Sprintf("处理异常: %v", r))
		}
	}()

	if err := s.docs.UpdateStatusFields(bgCtx, documentID, map[string]any{
		"status": model.KBDocumentStatusProcessing, "error_msg": "",
	}); err != nil {
		// 状态没推进就继续跑没有任何意义：文档会一直停在 pending，重试也无从下手。
		logger.Errorf("[KBImport] 标记文档处理中失败 doc=%d: %v", documentID, err)
	}

	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		s.markDocumentFailed(bgCtx, documentID, fmt.Sprintf("读取文件失败: %v", err))
		return
	}
	content := string(contentBytes)

	ragDoc := rag_core.Document{
		ID:      fmt.Sprintf("kbdoc_%d", documentID),
		Content: content,
		Metadata: map[string]any{
			"document_id": documentID,
			"file_path":   filePath,
			"title":       filepath.Base(filePath),
		},
		CreatedAt: time.Now(),
	}

	chunks, err := s.processor.ProcessDocument(bgCtx, ragDoc)
	if err != nil {
		s.markDocumentFailed(bgCtx, documentID, fmt.Sprintf("分片失败: %v", err))
		return
	}

	if len(chunks) == 0 {
		s.markDocumentFailed(bgCtx, documentID, "文档分片结果为空")
		return
	}

	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Content
	}
	embeddings, err := s.vector.EmbedBatch(texts)
	if err != nil {
		s.markDocumentFailed(bgCtx, documentID, fmt.Sprintf("向量化失败: %v", err))
		return
	}

	idxChunks := make([]ragretrieval.Chunk, len(chunks))
	for i, c := range chunks {
		idxChunks[i] = ragretrieval.Chunk{
			ID:         c.ID,
			DocumentID: c.DocumentID,
			Content:    c.Content,
			Metadata:   c.Metadata,
			Embedding:  embeddings[i],
			Score:      c.Score,
			TokenCount: c.TokenCount,
		}
	}

	kbIndexID := fmt.Sprintf("kb_%d", documentID)
	if err := s.indexer.BuildIndex(bgCtx, kbIndexID, idxChunks); err != nil {
		s.markDocumentFailed(bgCtx, documentID, fmt.Sprintf("构建索引失败: %v", err))
		return
	}

	if err := s.docs.UpdateStatusFields(bgCtx, documentID, map[string]any{
		"status":      model.KBDocumentStatusIndexed,
		"chunk_count": len(chunks),
		"content":     content,
		"error_msg":   "",
	}); err != nil {
		// 索引已建成但状态写失败：文档将滞留 processing，前端无法感知——必须落日志供运维介入
		logger.Errorf("[KBImport] 标记文档已索引失败 doc=%d: %v", documentID, err)
	}
}

func (s *KnowledgeBaseService) markDocumentFailed(ctx context.Context, documentID uint, errMsg string) {
	if err := s.docs.UpdateStatusFields(ctx, documentID, map[string]any{
		"status":    model.KBDocumentStatusFailed,
		"error_msg": errMsg,
	}); err != nil {
		logger.Errorf("[KBImport] 标记文档失败状态失败 doc=%d (原错误: %s): %v", documentID, errMsg, err)
	}
}
