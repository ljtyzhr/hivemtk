// session_chain_repo.go 会话生命周期链与自动化规则仓储。
package repository

import (
	"context"
	"errors"
	_db "hivemtk-user/internal/pkg/db"
	"time"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

type SessionChainRepository struct {
	db *gorm.DB
}

// NewSessionChainRepository 构造（无参，内部取全局 DB）。
func NewSessionChainRepository() *SessionChainRepository {
	return NewSessionChainRepositoryWithDB(_db.GetDB())
}

func NewSessionChainRepositoryWithDB(db *gorm.DB) *SessionChainRepository {
	return &SessionChainRepository{db: db}
}

func (r *SessionChainRepository) ListStaleSessions(ctx context.Context, threshold time.Time) ([]*model.CustomerSession, error) {
	if r.db == nil {
		return nil, nil
	}
	var sessions []*model.CustomerSession
	err := r.db.WithContext(ctx).
		Where("status IN ? AND updated_at < ?", []model.SessionStatus{
			model.SessionStatusPending, model.SessionStatusAIHandling, model.SessionStatusWaiting,
		}, threshold).
		Limit(200).
		Find(&sessions).Error
	return sessions, err
}

func (r *SessionChainRepository) UpdateSessionTags(ctx context.Context, id uint, tagsJSON string) error {
	if r.db == nil {
		return nil
	}
	return r.db.WithContext(ctx).Model(&model.CustomerSession{}).
		Where("id = ?", id).Update("tags", tagsJSON).Error
}

func (r *SessionChainRepository) CloseSessionByPK(ctx context.Context, id uint) error {
	if r.db == nil {
		return nil
	}
	return r.db.WithContext(ctx).Model(&model.CustomerSession{}).
		Where("id = ?", id).Update("status", model.SessionStatusClosed).Error
}

func (r *SessionChainRepository) ReopenSessionOnInbound(ctx context.Context, sessionID string) (bool, error) {
	if r.db == nil {
		return false, nil
	}
	res := r.db.WithContext(ctx).
		Model(&model.CustomerSession{}).
		Where("session_id = ? AND status IN ?", sessionID, []model.SessionStatus{
			model.SessionStatusResolved, model.SessionStatusClosed,
		}).
		Update("status", model.SessionStatusWaiting)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

func (r *SessionChainRepository) GetSessionBySessionID(ctx context.Context, sessionID string) (*model.CustomerSession, error) {
	if r.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	var sess model.CustomerSession
	if err := r.db.WithContext(ctx).Where("session_id = ?", sessionID).First(&sess).Error; err != nil {
		return nil, err
	}
	return &sess, nil
}

type AutomationRuleRepository struct {
	db *gorm.DB
}

// NewAutomationRuleRepository 构造（无参，内部取全局 DB）。
func NewAutomationRuleRepository() *AutomationRuleRepository {
	return NewAutomationRuleRepositoryWithDB(_db.GetDB())
}

func NewAutomationRuleRepositoryWithDB(db *gorm.DB) *AutomationRuleRepository {
	return &AutomationRuleRepository{db: db}
}

func (r *AutomationRuleRepository) ListRules(ctx context.Context, event string) ([]*model.AutomationRule, error) {
	if r.db == nil {
		return nil, nil
	}
	q := r.db.WithContext(ctx).Model(&model.AutomationRule{})
	if event != "" {
		q = q.Where("event = ?", event)
	}
	var list []*model.AutomationRule
	err := q.Order("priority ASC, id ASC").Find(&list).Error
	return list, err
}

func (r *AutomationRuleRepository) ListEnabledRulesByEvent(ctx context.Context, event string) ([]*model.AutomationRule, error) {
	if r.db == nil {
		return nil, nil
	}
	var rules []*model.AutomationRule
	err := r.db.WithContext(ctx).
		Where("event = ? AND enabled = ?", event, true).
		Order("priority ASC, id ASC").Limit(20).
		Find(&rules).Error
	return rules, err
}

func (r *AutomationRuleRepository) IncrementRuleRunCount(ctx context.Context, rule *model.AutomationRule) error {
	if r.db == nil {
		return nil
	}
	return r.db.WithContext(ctx).Model(rule).UpdateColumn("run_count", gorm.Expr("run_count + 1")).Error
}

func (r *AutomationRuleRepository) CreatePendingRuleExecution(ctx context.Context, pending *model.RulePendingExecution) error {
	if r.db == nil {
		return nil
	}
	return r.db.WithContext(ctx).Create(pending).Error
}

func (r *AutomationRuleRepository) ListDuePendingExecutions(ctx context.Context, now time.Time) ([]*model.RulePendingExecution, error) {
	if r.db == nil {
		return nil, nil
	}
	var pendings []*model.RulePendingExecution
	err := r.db.WithContext(ctx).
		Where("status = ? AND execute_at <= ?", "pending", now).
		Order("execute_at ASC").Limit(50).Find(&pendings).Error
	return pendings, err
}

func (r *AutomationRuleRepository) UpdatePendingExecutionStatus(ctx context.Context, id uint, status string) error {
	if r.db == nil {
		return nil
	}
	return r.db.WithContext(ctx).Model(&model.RulePendingExecution{}).
		Where("id = ?", id).Update("status", status).Error
}

func (r *AutomationRuleRepository) GetRuleByID(ctx context.Context, id uint) (*model.AutomationRule, error) {
	if r.db == nil {
		return nil, errors.New("automation rule repository not initialized")
	}
	var rule model.AutomationRule
	if err := r.db.WithContext(ctx).First(&rule, id).Error; err != nil {
		return nil, err
	}
	return &rule, nil
}

func (r *AutomationRuleRepository) GetSessionBySessionID(ctx context.Context, sessionID string) (*model.CustomerSession, error) {
	if r.db == nil {
		return nil, errors.New("automation rule repository not initialized")
	}
	var sess model.CustomerSession
	if err := r.db.WithContext(ctx).Where("session_id = ?", sessionID).First(&sess).Error; err != nil {
		return nil, err
	}
	return &sess, nil
}

func (r *AutomationRuleRepository) UpdateSessionFieldsBySessionID(ctx context.Context, sessionID, field string, value any) error {
	if r.db == nil {
		return nil
	}
	return r.db.WithContext(ctx).Model(&model.CustomerSession{}).
		Where("session_id = ?", sessionID).Update(field, value).Error
}

func (r *AutomationRuleRepository) CreateRuleOutboundMessage(ctx context.Context, rec *model.MessageHub) error {
	if r.db == nil {
		return nil
	}
	return r.db.WithContext(ctx).Create(rec).Error
}
