package feedbackloop

import (
	"context"
	"errors"
	"fmt"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"

	"gorm.io/gorm"
)

const refluxBatchSize = 500

var refluxSignalSet = map[string]bool{
	string(dtoSignalConversion):   true,
	string(dtoSignalComplaint):    true,
	string(dtoSignalTransfer):     true,
	string(dtoSignalChampionMark): true,
}

// BanditRewardReflux 回流 worker 纯逻辑（可独立单测）
type BanditRewardReflux struct {
	repo      *repository.FeedbackLoopRepository
	bandit    BanditUpdater
	batchSize int
}

// BanditUpdater UpdateReward 最小接口（便于测试注入）
type BanditUpdater interface {
	UpdateReward(ctx context.Context, experimentID, armKey string, success bool, reward float64) error
}

// NewBanditRewardReflux 构造
//
// 参数 db 仅为构造签名兼容保留，内部用 db 构造 repository，不存 gorm 直连写路径
func NewBanditRewardReflux(db *gorm.DB, bandit BanditUpdater) *BanditRewardReflux {
	return &BanditRewardReflux{
		repo:      repository.NewFeedbackLoopRepositoryWithDB(db),
		bandit:    bandit,
		batchSize: refluxBatchSize,
	}
}

// NewBanditRewardRefluxDefault 默认句柄变体：cron 侧不再经手 db（2026-10-25
// 句柄收口；GetDB 解析留在 repository 层的 FeedbackLoopRepositoryDefault）。
func NewBanditRewardRefluxDefault(bandit BanditUpdater) *BanditRewardReflux {
	return &BanditRewardReflux{
		repo:      repository.NewFeedbackLoopRepositoryDefault(),
		bandit:    bandit,
		batchSize: refluxBatchSize,
	}
}

// RefluxStats 单次回流统计
type RefluxStats struct {
	Scanned  int
	Refluxed int
	Skipped  int
	Duped    int
	Failed   int
}

// RefluxOnce 扫描 (since, until] 窗口内事件并回流。返回统计。
//
// 幂等顺序：先写台账再更新臂后验。台账插入撞唯一索引（event_id 或
// session/sop/signal 转化键）即视为重复，直接跳过；臂更新失败则删除刚插入的
// 台账做补偿，使该事件下一轮还能重试。
func (r *BanditRewardReflux) RefluxOnce(ctx context.Context, since, until time.Time) (RefluxStats, error) {
	var stats RefluxStats
	if r.repo == nil || r.bandit == nil {
		return stats, fmt.Errorf("reflux not initialized")
	}

	var afterID uint
	limit := r.batchSize
	if limit <= 0 {
		limit = refluxBatchSize
	}
	for {
		events, err := r.repo.ListRewardRefluxEvents(ctx, since, until, afterID, limit)
		if err != nil {
			return stats, fmt.Errorf("load events: %w", err)
		}
		for i := range events {
			ev := &events[i]
			afterID = ev.ID
			if !refluxSignalSet[ev.SignalKey] {
				continue
			}
			stats.Scanned++

			target, err := r.resolveArm(ctx, *ev)
			if err != nil {
				// 解析失败（DB 故障/租户未绑定）不能记 Skip：Skip 会让本轮成功返回，
				// cron 随即推进游标，这笔奖励永久丢失。向上返回让调用方保留旧游标重扫。
				return stats, fmt.Errorf("resolve arm for event %s: %w", ev.EventID, err)
			}
			if target == nil {
				stats.Skipped++
				continue
			}

			success := ev.Reward > 0
			reward := ev.Reward
			if reward < 0 {
				reward = -reward
			}
			log := model.BanditRefluxLog{
				EventID:      ev.EventID,
				ExperimentID: target.ExperimentID,
				ArmKey:       target.ArmKey,
				SignalKey:    ev.SignalKey,
				SessionID:    ev.SessionID,
				SOPID:        ev.SOPID,
				Reward:       reward,
				Success:      success,
			}
			inserted, err := r.repo.CreateRefluxLog(ctx, &log)
			if err != nil {
				stats.Failed++
				continue
			}
			if !inserted {
				stats.Duped++
				continue
			}
			if err := r.bandit.UpdateReward(ctx, target.ExperimentID, target.ArmKey, success, reward); err != nil {
				if delErr := r.repo.DeleteRefluxLogByID(ctx, log.ID); delErr != nil {
					logger.Warnf("[reflux] 臂更新失败且台账补偿删除失败 event=%s log_id=%d: %v",
						ev.EventID, log.ID, delErr)
				}
				stats.Failed++
				continue
			}
			stats.Refluxed++
		}
		if len(events) < limit {
			break
		}
	}
	return stats, nil
}

type refluxTarget struct {
	ExperimentID string
	ArmKey       string
}

// resolveArm 把反馈事件解析到实验臂。返回 (nil, nil) 表示确实无臂可记（Skip）；
// 返回 error 表示解析过程本身失败——两者必须区分，否则 DB 抖动会被当成
// 「这个事件没有实验」而永久跳过。
func (r *BanditRewardReflux) resolveArm(ctx context.Context, ev model.FeedbackEvent) (*refluxTarget, error) {
	if ev.PromptCandidateID > 0 {
		arm, err := r.repo.ResolveArmForPromptCandidate(ctx, ev.PromptCandidateID)
		if err == nil {
			return &refluxTarget{ExperimentID: arm.ExperimentID, ArmKey: arm.ArmKey}, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("resolve prompt candidate arm: %w", err)
		}
	}

	if ev.SOPID > 0 {
		arm, err := r.repo.ResolveArmForSOP(ctx, ev.SOPID)
		if err == nil {
			return &refluxTarget{ExperimentID: arm.ExperimentID, ArmKey: arm.ArmKey}, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("resolve sop arm: %w", err)
		}
	}
	return nil, nil
}

type dtoSignalKey string

const (
	dtoSignalConversion   dtoSignalKey = "conversion"
	dtoSignalComplaint    dtoSignalKey = "complaint"
	dtoSignalTransfer     dtoSignalKey = "transfer"
	dtoSignalChampionMark dtoSignalKey = "champion_mark"
)
