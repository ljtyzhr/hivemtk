package repository

import (
	"context"
	"time"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

type NegativeSample struct {
	CustomerMsg string  `gorm:"column:customer_msg"`
	AIReply     string  `gorm:"column:ai_reply"`
	Reward      float64 `gorm:"column:reward"`
	SignalKey   string  `gorm:"column:signal_key"`
}

// GetActivePromptCandidate 查询 SOP 节点当前 active Prompt 候选；
// 未找到时返回 gorm.ErrRecordNotFound（调用方按需转换为业务错误）
func (r *FeedbackLoopRepository) GetActivePromptCandidate(ctx context.Context, sopID uint, nodeID string) (*model.PromptCandidate, error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrRecordNotFound
	}
	var current model.PromptCandidate
	err := r.db.WithContext(ctx).
		Where("sop_id = ? AND sop_node_id = ? AND status = ?", sopID, nodeID, model.PromptCandidateStatusActive).
		First(&current).Error
	if err != nil {
		return nil, err
	}
	return &current, nil
}

func (r *FeedbackLoopRepository) CreatePromptCandidate(ctx context.Context, candidate *model.PromptCandidate) error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.WithContext(ctx).Create(candidate).Error
}

// FetchNegativeSamples 拉取负反馈样本（reward 最低的 Top-20，用于 LLM 上下文，避免 token 爆炸）。
// 不能用于阈值判定，阈值判定应使用 CountNegativeSamples。
func (r *FeedbackLoopRepository) FetchNegativeSamples(ctx context.Context, sopID uint, since time.Time, rewardThreshold float64) ([]NegativeSample, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	var rows []NegativeSample
	err := r.db.WithContext(ctx).Table("feedback_events fe").
		Select("fe.customer_msg, fe.ai_reply, fe.reward, fe.signal_key").
		Where("fe.sop_id = ? AND fe.created_at >= ? AND fe.reward < ?", sopID, since, rewardThreshold).
		Order("fe.reward ASC").Limit(20).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// CountNegativeSamples 统计时间窗口内负反馈样本总数（阈值判定，与 FetchNegativeSamples 的 LIMIT 20 解耦）。
func (r *FeedbackLoopRepository) CountNegativeSamples(ctx context.Context, sopID uint, since time.Time, rewardThreshold float64) (int64, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	var count int64
	err := r.db.WithContext(ctx).Table("feedback_events fe").
		Where("fe.sop_id = ? AND fe.created_at >= ? AND fe.reward < ?", sopID, since, rewardThreshold).
		Count(&count).Error
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (r *FeedbackLoopRepository) CreatePromptABTest(ctx context.Context, test *model.PromptABTest) error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.WithContext(ctx).Create(test).Error
}

func (r *FeedbackLoopRepository) CreateBanditArmsInBatches(ctx context.Context, arms []model.BanditArm, batchSize int) error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.WithContext(ctx).CreateInBatches(arms, batchSize).Error
}
