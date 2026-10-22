package confidence

import (
	"context"
	"sync"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/repository"
)

// ThresholdPolicyEngine 动态阈值策略引擎
//
// 策略按租户隔离缓存：同一进程里不同租户的阈值配置互不覆盖。
type ThresholdPolicyEngine struct {
	mu       sync.RWMutex
	policies map[uint]map[string]*model.ThresholdPolicy
	repo     *repository.ThresholdPolicyRepository
}

// NewThresholdPolicyEngine 创建策略引擎
func NewThresholdPolicyEngine(repo *repository.ThresholdPolicyRepository) *ThresholdPolicyEngine {
	return &ThresholdPolicyEngine{
		policies: make(map[uint]map[string]*model.ThresholdPolicy),
		repo:     repo,
	}
}

// LoadPolicies 从数据库重新加载调用方租户的 active 策略并覆盖该租户缓存。
// GetPolicy 首次访问某租户时自动调用；运营后台写入后由 RefreshThresholdPolicies 失效重载。
func (e *ThresholdPolicyEngine) LoadPolicies(ctx context.Context) error {
	policies, err := e.loadPolicies(ctx)
	if err != nil {
		return err
	}
	e.mu.Lock()
	e.policies[tenantKey(ctx)] = policies
	e.mu.Unlock()
	return nil
}

// Invalidate 丢弃指定租户的缓存，下次 GetPolicy 重新从数据库加载。
func (e *ThresholdPolicyEngine) Invalidate(tenantID uint) {
	e.mu.Lock()
	delete(e.policies, tenantID)
	e.mu.Unlock()
}

func (e *ThresholdPolicyEngine) loadPolicies(ctx context.Context) (map[string]*model.ThresholdPolicy, error) {
	policies := builtinPolicies()
	if e.repo == nil {
		return policies, nil
	}
	rows, err := e.repo.ListActive(ctx)
	if err != nil {
		// 数据库不可用不能让评分链路整体失败：回退内置默认口径。
		return policies, err
	}
	for i := range rows {
		p := &rows[i]
		if existing, ok := policies[p.IntentType]; ok && existing.Version > p.Version {
			continue
		}
		policies[p.IntentType] = p
	}
	return policies, nil
}

// GetPolicy 获取指定意图的策略（无则返回 default）。
//
// 租户缓存未命中时按 ctx 绑定懒加载；多租户模式下未绑定租户的调用直接回退内置默认，
// 不写缓存（否则会把默认口径缓存到 0 号位，被后续无绑定调用复用）。
func (e *ThresholdPolicyEngine) GetPolicy(ctx context.Context, intentType string) *model.ThresholdPolicy {
	if e == nil {
		return builtinPolicies()["default"]
	}
	key := tenantKey(ctx)
	e.mu.RLock()
	policies, ok := e.policies[key]
	e.mu.RUnlock()

	if !ok {
		loaded, err := e.loadPolicies(ctx)
		if err != nil || (tenant.IsMultiTenant() && !tenantBound(ctx)) {
			return policyOrDefault(loaded, intentType)
		}
		e.mu.Lock()
		if existing, exists := e.policies[key]; exists {
			loaded = existing
		} else {
			e.policies[key] = loaded
		}
		e.mu.Unlock()
		policies = loaded
	}
	return policyOrDefault(policies, intentType)
}

func tenantBound(ctx context.Context) bool {
	_, ok := tenant.FromContext(ctx)
	return ok
}

func tenantKey(ctx context.Context) uint {
	if id, ok := tenant.FromContext(ctx); ok {
		return id
	}
	return 0
}

func policyOrDefault(policies map[string]*model.ThresholdPolicy, intentType string) *model.ThresholdPolicy {
	if p, ok := policies[intentType]; ok {
		return p
	}
	return policies["default"]
}

// builtinPolicies 内置默认口径：DB 未配置时按意图给出的基线策略。
func builtinPolicies() map[string]*model.ThresholdPolicy {
	return map[string]*model.ThresholdPolicy{
		"default":       defaultPolicy(),
		"complaint":     policyFor("complaint", 0.85),
		"churn":         policyFor("churn", 0.85),
		"objection":     policyFor("objection", 0.75),
		"ask_product":   policyFor("ask_product", 0.70),
		"ask_service":   policyFor("ask_service", 0.70),
		"price_inquiry": policyFor("price_inquiry", 0.65),
		"purchase":      policyFor("purchase", 0.65),
		"after_sale":    policyFor("after_sale", 0.80),
		"social":        policyFor("social", 0.50),
		"greeting":      policyFor("greeting", 0.50),
	}
}

func defaultPolicy() *model.ThresholdPolicy {
	return policyFor("default", 0.70)
}

func policyFor(intentType string, base float64) *model.ThresholdPolicy {
	return &model.ThresholdPolicy{
		PolicyID:                "policy_" + intentType,
		IntentType:              intentType,
		BaseThreshold:           base,
		CustomerLevelWeight:     0.05,
		TimeslotWeight:          0.05,
		AgentAvailabilityWeight: 0.10,
		BandHandoffUpper:        0.40,
		BandFallbackUpper:       0.60,
		BandReviewUpper:         0.75,
		ReviewSLASeconds:        30,
		IsActive:                true,
		Version:                 1,
	}
}
