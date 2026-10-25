package feedbackloop

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/repository"
)

// BanditAllocator Multi-Armed Bandit 分配器
//
// 负责臂后验更新、收敛判定与胜者晋升。臂选择（SelectArm/SelectPrompt）的唯一
// 调用方随 self_learning 模块一起下线后无人引用，且持锁调用 betaSample 会死锁，
// 已在 2026-10 审计中整段删除；当前 SOP 变体分流走 SOPAgent.ABTestConfig。
type BanditAllocator struct {
	banditRepo *repository.FeedbackLoopRepository
	mu         sync.RWMutex
	cache      map[string][]*model.BanditArm
	rngMu      sync.Mutex
	rng        *rand.Rand
	config     BanditConfig
}

// NewBanditAllocator 构造分配器
//
// 参数：
//
//	db     - GORM DB（用于读写 bandit_arms 表，内部下沉到 repository 层使用）
//	cfg    - 配置（零值字段会用默认值填充）
//	seed   - PRNG 种子（测试时可固定；生产用 time.Now().UnixNano()）
func NewBanditAllocator(repo *repository.FeedbackLoopRepository, cfg BanditConfig, seed int64) *BanditAllocator {
	if cfg.ConvergenceThreshold == 0 {
		cfg.ConvergenceThreshold = 0.95
	}
	if cfg.MinSamplesForPromote == 0 {
		cfg.MinSamplesForPromote = 100
	}
	if cfg.PosteriorSamples == 0 {
		cfg.PosteriorSamples = 1000
	}
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	return &BanditAllocator{
		banditRepo: repoOrNil(repo),
		cache:      make(map[string][]*model.BanditArm),
		rng:        rand.New(rand.NewSource(seed)),
		config:     cfg,
	}
}

// UpdateReward 更新臂的奖励（成功/失败）
//
// 在 FeedbackCollector 写入 feedback_signals 后异步调用
// 成功：alpha += 1, success_trials += 1
// 失败：beta += 1
// 通用：total_trials += 1, sum_reward += reward, avg_reward = sum_reward / total_trials
func (b *BanditAllocator) UpdateReward(ctx context.Context, experimentID, armKey string, success bool, reward float64) error {
	if experimentID == "" || armKey == "" {
		return ErrInvalidInput
	}
	if err := b.banditRepo.UpdateBanditArmReward(ctx, experimentID, armKey, success, reward); err != nil {
		return fmt.Errorf("update bandit arm: %w", err)
	}
	b.invalidateCache(ctx, experimentID)
	return nil
}

// CheckConvergence 检查实验是否收敛
//
// 收敛条件：
//  1. 每臂样本数 ≥ MinSamplesForPromote
//  2. 蒙特卡洛采样 PosteriorSamples 次，P(winner 最优) ≥ ConvergenceThreshold
//
// 返回：
//
//	winnerKey - 胜出臂 key（未收敛时为空字符串）
//	ok        - 是否收敛
func (b *BanditAllocator) CheckConvergence(ctx context.Context, experimentID string) (string, bool) {
	arms, err := b.loadArms(ctx, experimentID)
	if err != nil || len(arms) < 2 {
		return "", false
	}
	for _, a := range arms {
		if a.TotalTrials < int64(b.config.MinSamplesForPromote) {
			return "", false
		}
	}
	winCounts := make(map[string]int)
	samples := b.config.PosteriorSamples
	for s := 0; s < samples; s++ {
		bestKey := ""
		bestTheta := -1.0
		for _, a := range arms {
			theta := b.betaSample(a.Alpha, a.Beta)
			if theta > bestTheta {
				bestTheta = theta
				bestKey = a.ArmKey
			}
		}
		if bestKey != "" {
			winCounts[bestKey]++
		}
	}

	var winnerKey string
	winnerProb := 0.0
	for k, v := range winCounts {
		prob := float64(v) / float64(samples)
		if prob > winnerProb {
			winnerProb = prob
			winnerKey = k
		}
	}
	if winnerProb >= b.config.ConvergenceThreshold {
		return winnerKey, true
	}
	return "", false
}

// PromoteArm 提升胜出臂，淘汰其他臂
//
// 事务操作：
//  1. winner → status=promoted, promoted_at=NOW()
//  2. 其他 → status=retired, retired_at=NOW()
func (b *BanditAllocator) PromoteArm(ctx context.Context, experimentID, winnerKey string) error {
	if experimentID == "" || winnerKey == "" {
		return ErrInvalidInput
	}
	if err := b.banditRepo.PromoteBanditArmWinner(ctx, experimentID, winnerKey); err != nil {
		return err
	}
	b.invalidateCache(ctx, experimentID)
	return nil
}

// cacheKey 租户参与缓存键
//
// 只按 experiment_id 做键时，多租户进程里先加载的租户 A 的臂列表会被
// 租户 B 的 CheckConvergence 直接命中，后验数据跨租户串读。
func cacheKey(ctx context.Context, experimentID string) string {
	tenantID, _ := tenant.FromContext(ctx)
	return fmt.Sprintf("%d:%s", tenantID, experimentID)
}

func (b *BanditAllocator) invalidateCache(ctx context.Context, experimentID string) {
	b.mu.Lock()
	delete(b.cache, cacheKey(ctx, experimentID))
	b.mu.Unlock()
}

func (b *BanditAllocator) loadArms(ctx context.Context, experimentID string) ([]*model.BanditArm, error) {
	key := cacheKey(ctx, experimentID)
	b.mu.RLock()
	arms, ok := b.cache[key]
	b.mu.RUnlock()
	if ok {
		return arms, nil
	}

	arms, err := b.banditRepo.ListActiveBanditArms(ctx, experimentID)
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	b.cache[key] = arms
	b.mu.Unlock()
	return arms, nil
}

// betaSample 采样 Beta(alpha,beta)
//
// rng 不是并发安全的，用独立 rngMu 保护。早期实现复用 cache 的 b.mu，
// 而 SelectArm 在持 b.mu 的情况下再调用本方法，必然自锁死。
func (b *BanditAllocator) betaSample(alpha, beta float64) float64 {
	if alpha <= 0 || beta <= 0 {
		return 0.5
	}
	b.rngMu.Lock()
	defer b.rngMu.Unlock()
	x := gammaSample(alpha, 1.0, b.rng)
	y := gammaSample(beta, 1.0, b.rng)
	sum := x + y
	if sum <= 0 {
		return 0.5
	}
	return x / sum
}

func gammaSample(alpha, theta float64, rng *rand.Rand) float64 {
	if alpha < 1 {
		u := rng.Float64()
		if u <= 0 {
			u = 1e-10
		}
		return gammaSample(alpha+1, theta, rng) * math.Pow(u, 1.0/alpha)
	}
	d := alpha - 1.0/3.0
	c := 1.0 / math.Sqrt(9*d)
	for attempts := 0; attempts < 1000; attempts++ {
		x := rng.NormFloat64()
		v := 1 + c*x
		if v <= 0 {
			continue
		}
		v = v * v * v
		u := rng.Float64()
		if u < 1-0.0331*x*x*x*x {
			return d * v * theta
		}
		if math.Log(u) < 0.5*x*x+d*(1-v+math.Log(v)) {
			return d * v * theta
		}
	}
	return alpha * theta
}

func sqrtF32(f float32) float32 {
	return float32(math.Sqrt(float64(f)))
}
