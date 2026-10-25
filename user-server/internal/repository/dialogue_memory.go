package repository

import (
	"context"

	"gorm.io/gorm"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"
)

type DialogueMemoryRepository interface {
	GetDialogueMemoryBySession(ctx context.Context, sessionID string) (*model.DialogueMemory, error)
	CreateDialogueMemory(ctx context.Context, mem *model.DialogueMemory) error
	SaveDialogueMemory(ctx context.Context, mem *model.DialogueMemory) error
	ListDialogueMemoriesByCustomer(ctx context.Context, customerID string, limit int) ([]*model.DialogueMemory, int64, error)

	ListMessageHubByConversation(ctx context.Context, conversationID string, limit int) ([]model.MessageHub, error)
}

type dialogueMemoryRepository struct {
	db *gorm.DB
}

func NewDialogueMemoryRepository() DialogueMemoryRepository {
	// 全局句柄未装配（早期启动/单测）时返回 nil，保持 service 侧「repo 未装配」旧语义。
	if db := _db.GetDB(); db != nil {
		return &dialogueMemoryRepository{db: db}
	}
	return nil
}

func NewDialogueMemoryRepositoryWithDB(db *gorm.DB) DialogueMemoryRepository {
	return &dialogueMemoryRepository{db: db}
}

func (r *dialogueMemoryRepository) GetDialogueMemoryBySession(ctx context.Context, sessionID string) (*model.DialogueMemory, error) {
	var mem model.DialogueMemory
	err := r.db.WithContext(ctx).Where("session_id = ?", sessionID).First(&mem).Error
	if err != nil {
		return nil, err
	}
	return &mem, nil
}

func (r *dialogueMemoryRepository) CreateDialogueMemory(ctx context.Context, mem *model.DialogueMemory) error {
	return r.db.WithContext(ctx).Create(mem).Error
}

// SaveDialogueMemory 更新已存在的记忆行（新建走 CreateDialogueMemory）
//
// 不用 Save：0 行命中会退化成按裸主键 ON CONFLICT，跨租户能把整行搬走/复活。
func (r *dialogueMemoryRepository) SaveDialogueMemory(ctx context.Context, mem *model.DialogueMemory) error {
	res := r.db.WithContext(ctx).Model(&model.DialogueMemory{}).
		Where("id = ?", mem.ID).
		Updates(map[string]any{
			"session_id":             mem.SessionID,
			"customer_id":            mem.CustomerID,
			"summary":                mem.Summary,
			"key_facts":              mem.KeyFacts,
			"customer_name":          mem.CustomerName,
			"customer_phone":         mem.CustomerPhone,
			"customer_wechat":        mem.CustomerWechat,
			"budget":                 mem.Budget,
			"demand":                 mem.Demand,
			"objections":             mem.Objections,
			"purchase_intent":        mem.PurchaseIntent,
			"intent_trail":           mem.IntentTrail,
			"sop_history":            mem.SOPHistory,
			"last_action":            mem.LastAction,
			"next_action_suggestion": mem.NextActionSuggestion,
			"last_active_at":         mem.LastActiveAt,
			"message_count":          mem.MessageCount,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *dialogueMemoryRepository) ListDialogueMemoriesByCustomer(ctx context.Context, customerID string, limit int) ([]*model.DialogueMemory, int64, error) {
	query := r.db.WithContext(ctx).Model(&model.DialogueMemory{}).Where("customer_id = ?", customerID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var mems []*model.DialogueMemory
	if err := query.Order("updated_at DESC").Limit(limit).Find(&mems).Error; err != nil {
		return nil, 0, err
	}
	return mems, total, nil
}

func (r *dialogueMemoryRepository) ListMessageHubByConversation(ctx context.Context, conversationID string, limit int) ([]model.MessageHub, error) {
	var records []model.MessageHub
	err := r.db.WithContext(ctx).Where("conversation_id = ?", conversationID).
		Order("sent_at DESC").Limit(limit).Find(&records).Error
	if err != nil {
		return nil, err
	}
	return records, nil
}
