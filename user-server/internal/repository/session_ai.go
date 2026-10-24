// session_ai.go AI 会话摘要仓储
package repository

import (
	"context"
	"errors"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

type SessionAIRepo struct {
	db *gorm.DB
}

func NewSessionAIRepo(db *gorm.DB) *SessionAIRepo {
	return &SessionAIRepo{db: db}
}

type MsgRow struct {
	SenderType string
	SenderName string
	Content    string
	CreatedAt  string
}

func (r *SessionAIRepo) ListRecentMessages(ctx context.Context, sessionID string, limit int) ([]MsgRow, error) {
	var rows []MsgRow
	err := r.db.WithContext(ctx).
		Table("session_messages").
		Select("sender_type, COALESCE(sender_name,'') AS sender_name, content, created_at").
		Where("session_id = ? AND is_internal = ?", sessionID, false).
		Order("created_at ASC").Limit(limit).Scan(&rows).Error
	return rows, err
}

func (r *SessionAIRepo) UpsertSummary(ctx context.Context, rec *model.SessionAISummary) error {
	if err := r.db.WithContext(ctx).
		Where("session_id = ?", rec.SessionID).
		Delete(&model.SessionAISummary{}).Error; err != nil {
		return err
	}
	return r.db.WithContext(ctx).Create(rec).Error
}

func (r *SessionAIRepo) GetLatestSummary(ctx context.Context, sessionID string) (*model.SessionAISummary, bool, error) {
	var rec model.SessionAISummary
	err := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("id DESC").First(&rec).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return &rec, true, nil
}
