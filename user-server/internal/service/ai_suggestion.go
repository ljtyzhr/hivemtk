package service

import (
	"context"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
)

// AISuggestionService AI建议服务
type AISuggestionService struct {
	suggestionRepo *repository.AISuggestionRepository
}

// NewAISuggestionServiceWithRepo 创建AI建议服务实例（仓储由装配层注入）
func NewAISuggestionServiceWithRepo(repo *repository.AISuggestionRepository) *AISuggestionService {
	return &AISuggestionService{suggestionRepo: repo}
}

// NewAISuggestionService 兼容旧装配点：绑定全局 DB 的仓储。
// 新代码一律经 NewAISuggestionServiceWithRepo 由 router 注入。
func NewAISuggestionService() *AISuggestionService {
	return NewAISuggestionServiceWithRepo(repository.NewAISuggestionRepository())
}

// GetSuggestions 获取会话的AI建议
func (s *AISuggestionService) GetSuggestions(ctx context.Context, sessionID string) ([]*model.AISuggestion, error) {
	return s.suggestionRepo.GetBySessionID(ctx, sessionID)
}

// UseSuggestion 使用AI建议
func (s *AISuggestionService) UseSuggestion(ctx context.Context, id uint, agentID uint) error {
	return s.suggestionRepo.MarkAsUsed(ctx, id, agentID)
}
