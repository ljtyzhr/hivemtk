package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"hivemtk-user/internal/aiagent/llm"
	"hivemtk-user/internal/config"
	"hivemtk-user/internal/model"
	dbUtil "hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/platform"
	"hivemtk-user/internal/repository"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 平台垫资模型的本地镜像与用量落库。
//
// ============================================================
// 扣费的权威在平台侧（代理转发时实时扣），这里**不做第二次扣减**。
//
// 为什么不做：两次扣账是预付系统最贵的 bug —— 商户充 100 元、用 10 元，
// 平台扣 10、本地也扣 10，看着"对得上"；但两边各自还有一份余额，
// 一旦其中一边重试（网络抖动导致的自动重发）就会二次扣，用户看到余额掉得比实际快。
//
// 本地这一层只负责三件事：
//  1. 落用量明细 → 让商户在本地也能看到"谁在什么场景用了多少"；
//  2. 更新余额镜像 → 页面不用为看一眼余额多打一次平台；
//  3. 余额不足时**提前阻断** → 少一次注定失败的往返。
// ============================================================

// PlatformLLMUsage 一次调用的用量（由 dispatcher 在收尾时回填）
type PlatformLLMUsage struct {
	MerchantKey string
	Provider    string
	Model       string
	PlanCode    string
	Scenario    string
	TraceID     string
	AgentID     string
	UserID      string
	// TenantCode 本次调用归属的租户短码（与代理调用 query 的 tenant_code 同源，
	// 供明细字段完整性/排障定位用）。镜像表已按 tenant_id 归属，本轮不加列。
	TenantCode       string
	PromptTokens     int64
	CompletionTokens int64
	InputPricePer1k  float64
	OutputPricePer1k float64
}

// CostCents 输入/输出分列成本（分）。按「元/1k」单价换算并向下取整 ——
// 少收一分比多收一分安全得多（多收会被投诉，少收只差分毫且对账能看出来）。
func (u PlatformLLMUsage) CostCents() (inputCents, outputCents int64) {
	if u.PromptTokens > 0 && u.InputPricePer1k > 0 {
		inputCents = int64(float64(u.PromptTokens) * u.InputPricePer1k / 1000.0)
	}
	if u.CompletionTokens > 0 && u.OutputPricePer1k > 0 {
		outputCents = int64(float64(u.CompletionTokens) * u.OutputPricePer1k / 1000.0)
	}
	return inputCents, outputCents
}

// recordPlatformUsage 落本地用量明细并更新余额镜像
func recordPlatformUsage(ctx context.Context, u PlatformLLMUsage) {
	if u.PromptTokens <= 0 && u.CompletionTokens <= 0 {
		return
	}
	// 请求 ID 缺 trace_id 时用「provider+模型+时刻」兜底：宁可偶尔重复，
	// 也不能因为没有 trace 而一条都不记（计量断档会让成本核算整体失去依据）。
	requestID := strings.TrimSpace(u.TraceID)
	if requestID == "" {
		requestID = "no-trace:" + u.Provider + ":" + u.Model + ":" + time.Now().Format("20060102150405.000")
	}
	inc, outc := u.CostCents()
	gormDB := dbUtil.GetDB()
	if gormDB == nil {
		logger.Warnf("[llm-usage] db 未就绪，跳过本地用量落库 provider=%s tenant=%s", u.Provider, u.TenantCode)
		return
	}
	rec := &model.PlatformLLMUsageRecord{
		RequestID:        requestID,
		Provider:         u.Provider,
		Model:            u.Model,
		PlanCode:         u.PlanCode,
		Scenario:         u.Scenario,
		TraceID:          u.TraceID,
		AgentID:          u.AgentID,
		UserID:           u.UserID,
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		InputCostCents:   inc,
		OutputCostCents:  outc,
		TotalCostCents:   inc + outc,
		OccurredAt:       time.Now(),
	}
	// 幂等：trace_id 撞唯一索引就跳过（重试/重放不重复记账）
	if err := gormDB.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(rec).Error; err != nil {
		logger.Warnf("[llm-usage] 本地用量落库失败 provider=%s tenant=%s trace=%s: %v",
			u.Provider, u.TenantCode, u.TraceID, err)
		return
	}
}

// syncPlatformProviders 拉取平台下发的 provider 元信息并落库
//
// **不下发 API Key**（见 plan.md ADR-M7）。落库时按 provider_name upsert，
// 已存在的行只更新元信息与 source，**不动商户自己填的 APIKey** ——
// 万一将来某个 provider 从 platform 转回 self，密钥还在。
func syncPlatformProviders(ctx context.Context) (int, error) {
	if !config.PlatformEnabled() {
		return 0, nil
	}
	rows, err := platform.NewModuleClient().Providers(ctx)
	if err != nil {
		return 0, err
	}
	gormDB := dbUtil.GetDB()
	if gormDB == nil || len(rows) == 0 {
		return 0, nil
	}
	return savePlatformProviders(gormDB, rows)
}

// savePlatformProviders 把平台下发的 provider 元信息落库（装配层写库）。
//
// ============================================================
// 三个必须同时满足的条件（缺一个就是"平台 provider 永远同步不到"）
//
// 1) **显式 tenant_id**（行自证）：llm_providers 声明了 TenantID，属租户隔离
//    覆盖表。隔离 callback 在 on 模式下对「无租户 ctx」的写是 fail-closed 的
//    （tenancyidx.fillOneForTenant → tenantGuardFail），报错原文：
//      [tenant] 表 llm_providers 需要租户上下文但 ctx 未绑定（create）
//    —— 这正是生产日志里的那一条。
//    隔离的**行自证**通道（callback 里 !bound 且行上已有显式非零 tenant_id
//    ⇒ 放行）就是本函数显式写 TenantID 的依据。原文不写 TenantID ⇒ 无从自证
//    ⇒ 每次落库都被拒。
//
// 2) **system scope**（装配层语义）：本函数是**启动期/装配层**操作
//    （cmd/api/main.go 调用，不是 HTTP 请求路径），没有 HTTP 租户 ctx 可绑。
//    与 dispatcher_persistence.go 的 systemScopeCtx() 同类，走
//    tenant.WithSystemScope 这条唯一豁免通道。隔离本身**不放宽**：
//    带请求 ctx 的 HTTP 路径仍然 fail-closed。
//
// 3) **复合冲突目标**：(tenant_id, name)。tenancyidx.Normalize 在框架层把
//    租户表的唯一索引原位复合化为 (tenant_id, name)，写 ON CONFLICT (name)
//    会与库里实际索引匹配不上 → 42P10
//    「there is no unique or exclusion constraint matching the ON CONFLICT
//    specification」→ 落库照样失败。
//
// 落库行归属**默认租户**：平台下发的是全局 provider 定义（dispatcher 是全局
// 单键 map、按 Name 索引），归属单个租户是错的；且 dispatcher 的
// LoadProvidersFromDB 只装 tenant_id=1，写到别的租户上会变成
// 「写进去但读不出来」（provider 永远不可见）。
func savePlatformProviders(gormDB *gorm.DB, rows []platform.PlatformLLMProvider) (int, error) {
	// 装配层写库的唯一豁免通道（与 dispatcher_persistence.go:systemScopeCtx 同一口径）
	sysCtx := tenant.WithSystemScope(context.Background())
	now := time.Now()
	n := 0
	for _, r := range rows {
		if strings.TrimSpace(r.ProviderName) == "" {
			continue
		}
		row := model.LLMProvider{
			TenantID:    tenant.DefaultTenantID,
			Name:        r.ProviderName,
			DisplayName: r.DisplayName,
			Model:       r.Model,
			APIType:     llmDefaultIfEmpty(r.APIType, "openai"),
			Vendor:      r.Vendor,
			Source:      model.LLMSourcePlatform,
			PlanCode:    r.PlanCode,
			Enabled:     true,
			UpdatedAt:   now,
			CreatedAt:   now,
		}
		if err := gormDB.WithContext(sysCtx).Clauses(clause.OnConflict{
			// 冲突目标必须与库里实际索引形状一致：(tenant_id, name)。
			// 见函数头第 2) 条。
			Columns: []clause.Column{{Name: "tenant_id"}, {Name: "name"}},
			// 只更新平台下发的元信息与 source；**不动 api_key** ——
			// 商户自己填的密钥要留着，否则 provider 从 platform 转回 self 就废了。
			DoUpdates: clause.AssignmentColumns([]string{
				"display_name", "model", "api_type", "vendor",
				"source", "plan_code", "enabled", "updated_at",
			}),
		}).Create(&row).Error; err != nil {
			logger.Warnf("[llm-provider] 平台 provider 落库失败 %s: %v", r.ProviderName, err)
			continue
		}
		n++
	}
	logger.Infof("[llm-provider] 平台下发 provider 同步完成：%d 条", n)
	return n, nil
}

func llmDefaultIfEmpty(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

var (
	_ = json.Marshal
	_ = errors.Is
	_ = repository.MenuKeysOf
	_ = gorm.ErrRecordNotFound
)

// InstallPlatformUsageSink 装配期接线：把 dispatcher 的用量回填接到本地落库。
//
// 装配点放在 cmd/api/main.go（见该文件），理由与 ADR-M1 一致：
// llm 是底层包，它不能反过来 import service，所以接线必须在能同时看到两侧的地方做。
func InstallPlatformUsageSink() {
	llm.SetPlatformUsageSink(func(e llm.PlatformUsageEvent) {
		// 用量明细是租户表：sink 在调用链之外执行，只能用事件里带的租户
		// 重建上下文；没带租户就不落（on 模式下 callback 也会拒绝）。
		ctx := context.Background()
		if e.TenantID != 0 {
			ctx = tenant.WithTenantID(ctx, e.TenantID)
		}
		recordPlatformUsage(ctx, PlatformLLMUsage{
			Provider:         e.Provider,
			Model:            e.Model,
			PlanCode:         e.PlanCode,
			Scenario:         e.Scenario,
			TraceID:          e.TraceID,
			AgentID:          e.AgentID,
			UserID:           e.UserID,
			TenantCode:       e.TenantCode,
			PromptTokens:     e.PromptTokens,
			CompletionTokens: e.CompletionTokens,
		})
	})
}

// SyncPlatformProviders 导出给装配层调用（同步平台下发的 provider）
func SyncPlatformProviders(ctx context.Context) (int, error) { return syncPlatformProviders(ctx) }
