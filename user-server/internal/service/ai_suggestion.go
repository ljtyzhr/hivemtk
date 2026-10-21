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

// NewAISuggestionService 创建AI建议服务实例
func NewAISuggestionService() *AISuggestionService {
	return &AISuggestionService{
		suggestionRepo: repository.NewAISuggestionRepository(),
	}
}

// GetSuggestions 获取会话的AI建议
func (s *AISuggestionService) GetSuggestions(ctx context.Context, sessionID string) ([]*model.AISuggestion, error) {
	return s.suggestionRepo.GetBySessionID(ctx, sessionID)
}

// UseSuggestion 使用AI建议
func (s *AISuggestionService) UseSuggestion(ctx context.Context, id uint, agentID uint) error {
	return s.suggestionRepo.MarkAsUsed(ctx, id, agentID)
}
