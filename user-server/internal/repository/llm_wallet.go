package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
)

// LLMWalletRepository LLM 余额的本地镜像仓储
//
// 一期只做「镜像展示」，不做本地扣减：扣减的权威在平台侧（代理转发时实时扣），
// 本地再扣一遍就是两本账。镜像的作用是**平台不可达时仍能给用户一个可解释的
// 余额数字**，而不是在页面上直接崩掉。
type LLMWalletRepository interface {
	ApplyMirror(ctx context.Context, inputRemaining, outputRemaining int64) error
	ListTxns(ctx context.Context, reason string, page, pageSize int) ([]*model.LLMWalletTxn, int64, error)
}

type llmWalletRepo struct{ db *gorm.DB }

// NewLLMWalletRepository 构造
func NewLLMWalletRepository() LLMWalletRepository { return &llmWalletRepo{db: db.GetDB()} }

// NewLLMWalletRepositoryWithDB 带显式 DB（测试注入）
func NewLLMWalletRepositoryWithDB(gdb *gorm.DB) LLMWalletRepository {
	return &llmWalletRepo{db: gdb}
}

// ApplyMirror 把平台余额落到本地单行镜像
func (r *llmWalletRepo) ApplyMirror(ctx context.Context, inputRemaining, outputRemaining int64) error {
	row := &model.LLMWallet{
		InputTokensRemaining:  inputRemaining,
		OutputTokensRemaining: outputRemaining,
		SyncedAt:              time.Now(),
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		// tenant_id 在前：租户索引策略的产物（见 repository/tenant_module.go 同处注释）
		Columns: []clause.Column{{Name: "tenant_id"}, {Name: "scope"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"input_tokens_remaining", "output_tokens_remaining", "synced_at",
		}),
	}).Create(row).Error
}

func (r *llmWalletRepo) ListTxns(ctx context.Context, reason string, page, pageSize int) ([]*model.LLMWalletTxn, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	q := r.db.WithContext(ctx).Model(&model.LLMWalletTxn{})
	if reason != "" {
		q = q.Where("reason = ?", reason)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []*model.LLMWalletTxn
	err := q.Order("created_at DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&out).Error
	return out, total, err
}

var _ = errors.Is
