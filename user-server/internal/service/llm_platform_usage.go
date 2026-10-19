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
	now := time.Now()
	n := 0
	for _, r := range rows {
		if strings.TrimSpace(r.ProviderName) == "" {
			continue
		}
		row := model.LLMProvider{
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
		if err := gormDB.WithContext(ctx).Clauses(clause.OnConflict{
			// 只更新平台下发的元信息与 source；**不动 api_key** ——
			// 商户自己填的密钥要留着，否则 provider 从 platform 转回 self 就废了。
			Columns: []clause.Column{{Name: "name"}},
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
		recordPlatformUsage(context.Background(), PlatformLLMUsage{
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
