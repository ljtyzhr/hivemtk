package service

import (
	"context"
	"math"
	"runtime/debug"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"
)

const memoryDecayLambda = math.Ln2 / 168

const (
	memoryDecayArchiveScore      = 0.15
	memoryDecayArchiveConfidence = 0.2
	memoryDecayScanLimit         = 1000
)

// MemoryDecayStats L-4 衰减归档作业统计
type MemoryDecayStats struct {
	Scanned  int
	Stale    int
	Archived int
	DryRun   bool
}

// DecayScore L-4 记忆衰减得分 = confidence * exp(-λ*Δh)，λ=ln2/168（7 天半衰期）
//   - lastEventAt 晚于 now（未来事件）不衰减，Δh clamp 到 0
//   - confidence 越界 clamp 到 [0,1]
func DecayScore(confidence float64, lastEventAt, now time.Time) float64 {
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 1 {
		confidence = 1
	}
	hours := now.Sub(lastEventAt).Hours()
	if hours < 0 {
		hours = 0
	}
	return confidence * math.Exp(-memoryDecayLambda*hours)
}

func shouldArchiveMemory(score, confidence float64) bool {
	return score < memoryDecayArchiveScore || confidence < memoryDecayArchiveConfidence
}

func itemDecayConfidence(it model.MemoryItem) float64 {
	if it.Metadata != nil {
		if c, ok := it.Metadata["confidence"].(float64); ok && c > 0 {
			if c > 1 {
				return 1
			}
			return c
		}
	}
	return float64(it.Importance) / 10.0
}

// RunMemoryDecayJob L-4：扫描 L2 事实表（memory_items 事实行 + customer_long_term_memory），
// score<0.15 或 confidence<0.2 的记录归档软标记：InvalidAt=now（无 Archived 列的替代语义，不物理删）
// dryRun=true 只统计不写。db 为 nil 时返回零统计。
//
// **本函数是单租户单元**：ctx 必须已绑定租户（正常路径由 RunMemoryDecayAllTenants 经
// tenant.Fanout 提供）。不要在 ctx 没有租户绑定时直接调它——on 模式下隔离 callback 会
// fail-closed，而"跑不起来"与"没有可归档数据"在统计上长得一模一样。
func RunMemoryDecayJob(ctx context.Context, repo *repository.MemoryDecayRepository, dryRun bool) (*MemoryDecayStats, error) {
	stats := &MemoryDecayStats{DryRun: dryRun}
	if repo == nil {
		return stats, nil
	}
	now := time.Now()

	items, err := repo.ListActiveLongTermItems(ctx, memoryDecayScanLimit)
	if err != nil {
		return nil, err
	}
	staleItemIDs := make([]uint, 0, 16)
	for _, it := range items {
		stats.Scanned++
		conf := itemDecayConfidence(it)
		lastEvent := effectiveValidFrom(timePtrValue(it.ValidFrom), it.CreatedAt)
		if !shouldArchiveMemory(DecayScore(conf, lastEvent, now), conf) {
			continue
		}
		stats.Stale++
		staleItemIDs = append(staleItemIDs, it.ID)
	}

	l2v, err := repo.ListActiveLongTermMemory(ctx, memoryDecayScanLimit)
	if err != nil {
		return nil, err
	}
	staleL2IDs := make([]uint64, 0, 16)
	for _, it := range l2v {
		stats.Scanned++
		conf := float64(it.Importance) / 10.0
		lastEvent := effectiveValidFrom(timePtrValue(it.ValidFrom), it.CreatedAt)
		if !shouldArchiveMemory(DecayScore(conf, lastEvent, now), conf) {
			continue
		}
		stats.Stale++
		staleL2IDs = append(staleL2IDs, it.ID)
	}

	if dryRun {
		return stats, nil
	}
	if err := repo.MarkItemsInvalid(ctx, staleItemIDs, now); err != nil {
		return stats, err
	}
	stats.Archived += len(staleItemIDs)
	if err := repo.MarkLongTermMemoryInvalid(ctx, staleL2IDs, now); err != nil {
		return stats, err
	}
	stats.Archived += len(staleL2IDs)
	return stats, nil
}

// RunMemoryDecayAllTenants L-4 跨租户入口：按租户扇出后汇总。
//
// 为什么必须扇出而不是一把梭全表扫：memory_items / customer_long_term_memory 都是
// 租户表，on 模式下隔离 callback 只认 ctx 里的租户。用 context.Background() 直接扫
// 会 fail-closed（一行也读不到、每小时静默空转），用 SystemScope 强扫则是拿"后台任务"
// 当万能钥匙、绕开整套隔离。tenant.Fanout 两头都不碰：off 模式给默认租户 ctx，on 模式
// 逐个租户给 ctx，单租户失败不阻断其余租户（见 fanout.go 的取舍说明）。
//
// 五层边界（架构 §3.2）：入口收仓储而不是 *gorm.DB——原实参 db 只用来建
// MemoryDecayRepository 和喂 tenant.Fanout，而 Fanout 的 db 形参不参与任何查询
// （隔离靠 ctx 里的租户绑定），于是服务层不再需要句柄。repo 为 nil 时返回零统计。
func RunMemoryDecayAllTenants(ctx context.Context, repo *repository.MemoryDecayRepository, dryRun bool) (*MemoryDecayStats, error) {
	stats := &MemoryDecayStats{DryRun: dryRun}
	if repo == nil {
		return stats, nil
	}
	_, err := tenant.Fanout(ctx, nil, func(tctx context.Context, _ uint) (int64, error) {
		one, jerr := RunMemoryDecayJob(tctx, repo, dryRun)
		if jerr != nil {
			return 0, jerr
		}
		stats.Scanned += one.Scanned
		stats.Stale += one.Stale
		stats.Archived += one.Archived
		return int64(one.Archived), nil
	})
	if err != nil {
		return stats, err
	}
	return stats, nil
}

// StartMemoryDecayLoop L-4：后台周期衰减归档循环（panic recover + 周期 tick）
// interval <=0 时取默认 1h；ctx 取消时退出；单轮 panic 不终止循环。
// repo 由装配点（cmd/api/main.go）从句柄构造后注入，本包不再收 *gorm.DB。
func StartMemoryDecayLoop(ctx context.Context, repo *repository.MemoryDecayRepository, interval time.Duration) {
	if interval <= 0 {
		interval = time.Hour
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("[MemoryDecay] 循环退出 panic: %v\n%s", r, debug.Stack())
			}
		}()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runMemoryDecayOnce(ctx, repo)
			}
		}
	}()
}

func runMemoryDecayOnce(ctx context.Context, repo *repository.MemoryDecayRepository) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("[MemoryDecay] 单轮 panic: %v\n%s", r, debug.Stack())
		}
	}()
	stats, err := RunMemoryDecayAllTenants(ctx, repo, false)
	if err != nil {
		logger.Warnf("[MemoryDecay] 衰减归档失败: %v", err)
		return
	}
	if stats.Stale > 0 {
		logger.Infof("[MemoryDecay] 衰减归档完成 scanned=%d stale=%d archived=%d", stats.Scanned, stats.Stale, stats.Archived)
	}
}
