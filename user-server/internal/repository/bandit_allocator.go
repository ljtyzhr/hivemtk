package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GetRunningPromptABTestBySOPNode 查询指定 SOP 节点 running 状态的 Prompt A/B 测试
//
// 未找到时返回 gorm.ErrRecordNotFound（调用方按需转换为业务错误或默认值）
func (r *FeedbackLoopRepository) GetRunningPromptABTestBySOPNode(ctx context.Context, sopID uint, nodeID string) (*model.PromptABTest, error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrRecordNotFound
	}
	var test model.PromptABTest
	err := r.db.WithContext(ctx).
		Where("sop_id = ? AND sop_node_id = ? AND status = ?", sopID, nodeID, model.PromptABTestStatusRunning).
		First(&test).Error
	if err != nil {
		return nil, err
	}
	return &test, nil
}

// GetBanditArmByExperimentAndKey 查询指定实验 + arm_key 的 bandit arm
//
// 未找到时返回 gorm.ErrRecordNotFound
func (r *FeedbackLoopRepository) GetBanditArmByExperimentAndKey(ctx context.Context, experimentID, armKey string) (*model.BanditArm, error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrRecordNotFound
	}
	var arm model.BanditArm
	err := r.db.WithContext(ctx).
		Where("experiment_id = ? AND arm_key = ?", experimentID, armKey).
		First(&arm).Error
	if err != nil {
		return nil, err
	}
	return &arm, nil
}

// UpdateBanditArmReward 更新臂的奖励（成功/失败）
//
// 成功：alpha += 1, success_trials += 1
// 失败：beta += 1
// 通用：total_trials += 1, sum_reward += reward, avg_reward 重算, updated_at 刷新
func (r *FeedbackLoopRepository) UpdateBanditArmReward(ctx context.Context, experimentID, armKey string, success bool, reward float64) error {
	if r == nil || r.db == nil {
		return nil
	}
	updates := map[string]any{
		"total_trials": gorm.Expr("total_trials + 1"),
		"sum_reward":   gorm.Expr("sum_reward + ?", reward),
		"avg_reward":   gorm.Expr("CASE WHEN total_trials + 1 > 0 THEN (sum_reward + ?) / (total_trials + 1) ELSE 0 END", reward),
		"updated_at":   time.Now(),
	}
	if success {
		updates["alpha"] = gorm.Expr("alpha + 1")
		updates["success_trials"] = gorm.Expr("success_trials + 1")
	} else {
		updates["beta"] = gorm.Expr("beta + 1")
	}
	res := r.db.WithContext(ctx).Model(&model.BanditArm{}).
		Where("experiment_id = ? AND arm_key = ?", experimentID, armKey).
		Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		// 臂不存在（实验已下线/被清理，或跨租户）时静默成功会让回流台账
		// 记下一笔"已入账"而臂的后验一字未动，下一轮又因台账重复而永不重试。
		return gorm.ErrRecordNotFound
	}
	return nil
}

// PromoteBanditArmWinner 事务：提升胜出臂 + 淘汰其他臂
//
//  1. winner → status=promoted, promoted_at=NOW()
//  2. 其他 → status=retired, retired_at=NOW()
func (r *FeedbackLoopRepository) PromoteBanditArmWinner(ctx context.Context, experimentID, winnerKey string) error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		res := tx.Model(&model.BanditArm{}).
			Where("experiment_id = ? AND arm_key = ?", experimentID, winnerKey).
			Updates(map[string]any{
				"status":      model.BanditArmStatusPromoted,
				"promoted_at": now,
				"updated_at":  now,
			})
		if res.Error != nil {
			return fmt.Errorf("promote winner: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			// 胜出臂必须真实存在：否则下面那条"退役其余臂"会把整组实验
			// 全部退光而没有一条 promoted —— 一个拼错的 arm_key 就能让实验消失。
			return fmt.Errorf("promote winner %q: %w", winnerKey, gorm.ErrRecordNotFound)
		}
		if err := tx.Model(&model.BanditArm{}).
			Where("experiment_id = ? AND arm_key != ?", experimentID, winnerKey).
			Updates(map[string]any{
				"status":     model.BanditArmStatusRetired,
				"retired_at": now,
				"updated_at": now,
			}).Error; err != nil {
			return fmt.Errorf("retire losers: %w", err)
		}
		return nil
	})
}

// ListActiveBanditArms 查询指定实验中处于 exploring/exploiting 状态的臂
//
// 用于 Thompson Sampling 采样前的臂列表加载
func (r *FeedbackLoopRepository) ListActiveBanditArms(ctx context.Context, experimentID string) ([]*model.BanditArm, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	var arms []*model.BanditArm
	err := r.db.WithContext(ctx).
		Where("experiment_id = ? AND status IN ?", experimentID, []string{model.BanditArmStatusExploring, model.BanditArmStatusExploiting}).
		Find(&arms).Error
	if err != nil {
		return nil, err
	}
	return arms, nil
}

// ListRewardRefluxEvents 按 id 游标分页查询 (since, until] 窗口内 reward != 0 的反馈事件
//
// afterID 为上一批最后一条的 id（首批传 0），调用方循环到返回条数小于 limit 为止。
// 早期实现固定 Limit(500) 且无游标，窗口内第 501 笔之后的事件会被静默丢弃。
func (r *FeedbackLoopRepository) ListRewardRefluxEvents(ctx context.Context, since, until time.Time, afterID uint, limit int) ([]model.FeedbackEvent, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("feedback loop repository not initialized")
	}
	if limit <= 0 {
		return nil, errors.New("feedback loop repository: limit must be positive")
	}
	var events []model.FeedbackEvent
	err := r.db.WithContext(ctx).
		Where("created_at > ? AND created_at <= ? AND reward != 0 AND id > ?", since, until, afterID).
		Order("id ASC").Limit(limit).
		Find(&events).Error
	return events, err
}

// CreateRefluxLog 幂等写入回流台账
//
// 返回是否真正插入：event_id 或 (session_id,sop_id,signal_key) 撞唯一索引时为 false。
// 调用方必须先插入、再更新臂后验——插入失败即代表这笔已被别的进程/上一轮入账。
func (r *FeedbackLoopRepository) CreateRefluxLog(ctx context.Context, log *model.BanditRefluxLog) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("feedback loop repository not initialized")
	}
	res := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(log)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// DeleteRefluxLogByID 删除一笔回流台账（臂后验更新失败时的补偿，使下一轮可重试）
func (r *FeedbackLoopRepository) DeleteRefluxLogByID(ctx context.Context, id uint) error {
	if r == nil || r.db == nil {
		return errors.New("feedback loop repository not initialized")
	}
	res := r.db.WithContext(ctx).Delete(&model.BanditRefluxLog{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		// 补偿删除必须真的删掉：0 行说明台账不在本租户可见范围，
		// 静默返回 nil 会让调用方以为补偿成功，而这笔事件下一轮仍被判重。
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ResolveArmForPromptCandidate 按 prompt candidate 解析运行中实验臂
//
// JOIN prompt_ab_tests 限定 running 状态；未命中返回 gorm.ErrRecordNotFound 语义由调用方处理
func (r *FeedbackLoopRepository) ResolveArmForPromptCandidate(ctx context.Context, promptCandidateID uint) (*model.BanditArm, error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrRecordNotFound
	}
	var arm model.BanditArm
	err := r.db.WithContext(ctx).
		Joins("JOIN prompt_ab_tests t ON t.experiment_id = bandit_arms.experiment_id AND t.status = 'running'").
		Where("bandit_arms.prompt_candidate_id = ? AND bandit_arms.experiment_type = ?", promptCandidateID, model.BanditExperimentTypePrompt).
		First(&arm).Error
	if err != nil {
		return nil, err
	}
	return &arm, nil
}

// ResolveArmForSOP 按事件所属 SOP 解析运行中 SOP 变体实验臂
//
// 必须按**臂侧** sop_id 判定，不能先按测试行 sop_id 找实验再按事件 SOPID 取臂：
// 克隆变体的臂 sop_id 是克隆 SOP 的 ID，而实验行的 sop_id 仍是原 SOP 的 ID，
// 旧实现下克隆变体产生的反馈永远匹配不到实验（直接 skip，奖励永久丢失）。
//
// 不放宽到 prompt 实验：prompt 实验的每个臂也都带 sop_id，只按 sop_id 命中
// 会把普通对话反馈错记到 prompt 实验上。
func (r *FeedbackLoopRepository) ResolveArmForSOP(ctx context.Context, sopID uint) (*model.BanditArm, error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrRecordNotFound
	}
	var arm model.BanditArm
	err := r.db.WithContext(ctx).
		Joins("JOIN prompt_ab_tests t ON t.experiment_id = bandit_arms.experiment_id AND t.status = 'running'").
		Where("bandit_arms.sop_id = ? AND bandit_arms.experiment_type = ?", sopID, model.BanditExperimentTypeSOPVariant).
		Order("t.id ASC, bandit_arms.id ASC").
		First(&arm).Error
	if err != nil {
		return nil, err
	}
	return &arm, nil
}
