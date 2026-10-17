package ragcustomerservice

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/pkg/utils/logger"
)

type PgDialogManager struct {
	db     *gorm.DB
	config *DialogManagerConfig
}

func NewPgDialogManager(db *gorm.DB, config *DialogManagerConfig) *PgDialogManager {
	if config == nil {
		config = &DialogManagerConfig{
			DefaultMaxHistoryLength: 10,
			DefaultSessionTimeout:   30 * time.Minute,
			SessionCleanupInterval:  5 * time.Minute,
		}
	}
	dm := &PgDialogManager{db: db, config: config}
	go dm.startSessionCleanup()
	return dm
}

func (dm *PgDialogManager) CreateSession(ctx context.Context, userID, platform, kbID string, config SessionConfig) (*Session, error) {
	now := time.Now()
	session := &Session{
		ID:        fmt.Sprintf("sess_%d", now.UnixNano()),
		UserID:    userID,
		Platform:  platform,
		KBID:      kbID,
		Status:    SessionActive,
		CreatedAt: now,
		UpdatedAt: now,
		Config:    config,
	}

	cfgJSON, _ := json.Marshal(config)
	if err := dm.db.WithContext(ctx).Table("rag_sessions").
		Create(map[string]any{
			"id": session.ID, "user_id": userID, "platform": platform,
			"kb_id": kbID, "status": "active", "config": string(cfgJSON),
			"created_at": now, "updated_at": now,
		}).Error; err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	return session, nil
}

func (dm *PgDialogManager) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	var row struct {
		ID        string
		UserID    string
		Platform  string
		KBID      string
		Status    string
		Config    string
		CreatedAt time.Time
		UpdatedAt time.Time
	}

	if err := dm.db.WithContext(ctx).Table("rag_sessions").
		Select("id, user_id, platform, kb_id, status, config, created_at, updated_at").
		Where("id = ?", sessionID).
		Scan(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}

	if row.ID == "" {
		return nil, nil
	}

	var config SessionConfig
	if err := json.Unmarshal([]byte(row.Config), &config); err != nil {
		logger.Warnf("[PGDialog] 解析会话配置失败 sessionID=%s: %v", row.ID, err)
	}

	return &Session{
		ID:        row.ID,
		UserID:    row.UserID,
		Platform:  row.Platform,
		KBID:      row.KBID,
		Status:    SessionStatus(row.Status),
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
		Config:    config,
	}, nil
}

func (dm *PgDialogManager) AddMessage(ctx context.Context, sessionID string, message Message) error {
	contentJSON, _ := json.Marshal(message)
	return dm.db.WithContext(ctx).Table("rag_messages").
		Create(map[string]any{
			"session_id": sessionID, "message_id": message.ID,
			"role": string(message.Role), "content": string(contentJSON),
			"timestamp": message.Timestamp,
		}).Error
}

func (dm *PgDialogManager) GetConversationHistory(ctx context.Context, sessionID string, limit int) (*Conversation, error) {
	var rows []struct {
		Content string
	}

	if err := dm.db.WithContext(ctx).Table("rag_messages").
		Select("content").
		Where("session_id = ?", sessionID).
		Order("timestamp DESC").Limit(limit).
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	var messages []Message
	for _, r := range rows {
		var msg Message
		if err := json.Unmarshal([]byte(r.Content), &msg); err != nil {
			logger.Warnf("[PGDialog] 解析会话消息失败 sessionID=%s: %v", sessionID, err)
			continue
		}
		messages = append(messages, msg)
	}

	return &Conversation{
		Messages: messages,
	}, nil
}

func (dm *PgDialogManager) UpdateSessionMetadata(ctx context.Context, sessionID string, metadata map[string]any) error {
	metaJSON, _ := json.Marshal(metadata)
	return dm.db.WithContext(ctx).Table("rag_sessions").
		Where("id = ?", sessionID).
		Updates(map[string]any{
			"config":     gorm.Expr("json_set(config, '$.metadata', ?)", string(metaJSON)),
			"updated_at": time.Now(),
		}).Error
}

func (dm *PgDialogManager) CloseSession(ctx context.Context, sessionID string) error {
	return dm.db.WithContext(ctx).Table("rag_sessions").
		Where("id = ?", sessionID).
		Updates(map[string]any{"status": "closed", "updated_at": time.Now()}).Error
}

func (dm *PgDialogManager) CleanupExpiredSessions(ctx context.Context) error {
	cutoff := time.Now().Add(-dm.config.DefaultSessionTimeout)
	return dm.db.WithContext(ctx).Table("rag_sessions").
		Where("status = 'active' AND updated_at < ?", cutoff).
		Updates(map[string]any{"status": "expired", "updated_at": time.Now()}).Error
}

func (dm *PgDialogManager) ListUserSessions(ctx context.Context, userID, platform string, status SessionStatus) ([]Session, error) {
	var rows []struct {
		ID        string
		UserID    string
		Platform  string
		KBID      string
		Status    string
		Config    string
		CreatedAt time.Time
		UpdatedAt time.Time
	}

	q := dm.db.WithContext(ctx).Table("rag_sessions").
		Select("id, user_id, platform, kb_id, status, config, created_at, updated_at").
		Where("user_id = ? AND platform = ?", userID, platform)
	if status != "" {
		q = q.Where("status = ?", string(status))
	}
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}

	var sessions []Session
	for _, r := range rows {
		var config SessionConfig
		if err := json.Unmarshal([]byte(r.Config), &config); err != nil {
			logger.Warnf("[PGDialog] 解析会话配置失败 sessionID=%s: %v", r.ID, err)
		}
		sessions = append(sessions, Session{
			ID: r.ID, UserID: r.UserID, Platform: r.Platform,
			KBID: r.KBID, Status: SessionStatus(r.Status),
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Config: config,
		})
	}
	return sessions, nil
}

func (dm *PgDialogManager) startSessionCleanup() {
	ticker := time.NewTicker(dm.config.SessionCleanupInterval)
	defer ticker.Stop()
	for range ticker.C {
		if err := dm.CleanupExpiredSessions(context.Background()); err != nil {
			logger.Errorf("[PgDialogManager] 清理过期会话失败: %v", err)
		}
	}
}
