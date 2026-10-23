// content_guard_wiring.go 内容安全闸（ContentGuard）的装配点。
//
// 装配内容（四件，缺一件就不是完整链路）：
//  1. 五个 GORM 仓储 → service.Runtime（业务门面）；
//  2. 外部审核源（**默认不建**，见 provider 包的"数据零出域"说明）；
//  3. 异步审查 worker 启动（Start）+ 进程退出时 Stop；
//  4. 订阅 customer.message.received：入站客人消息自动进审查队列（业务侧零改动）。
//
// 为什么订阅放在装配层而不是 webhook/service 里：业务侧不该知道"有内容审查"这件事
// （也不该因为审查装不上而受影响）。这里失败只会让"自动提交"缺失，消息照常入库与回复。
package app

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	agent_runtime "hivemtk-user/internal/aiagent/agent/runtime"
	"hivemtk-user/internal/contentguard/model"
	"hivemtk-user/internal/contentguard/provider"
	"hivemtk-user/internal/contentguard/repository"
	guardservice "hivemtk-user/internal/contentguard/service"
	"hivemtk-user/internal/event"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/service"
)

// InitContentGuardRuntime 装配内容审查运行时并启动 worker。
//
// 幂等：多次调用会重建运行时并替换全局登记处（旧实例的 worker 会由调用方
// 或进程退出关闭；本仓装配点在 main 里只跑一次）。
func InitContentGuardRuntime(db *gorm.DB) *guardservice.Runtime {
	if db == nil {
		guardservice.SetGlobalRuntime(nil)
		agent_runtime.SetGuardGate(nil)
		logger.Warnf("[content-guard] ⚠️ 无 DB 句柄 ⇒ 内容审查底座不装配：/api/manage/content-guard/* 回 503，" +
			"入站消息不再自动提交审查（消息链路本身不受影响）")
		return nil
	}

	auditor := buildContentGuardAuditor()
	rt := guardservice.NewRuntime(guardservice.RuntimeDeps{
		Words:     repository.NewGuardWordRepository(db),
		Policies:  repository.NewGuardPolicyRepository(db),
		Tasks:     repository.NewGuardTaskRepository(db),
		Records:   repository.NewGuardRecordRepository(db),
		Sanctions: repository.NewGuardSanctionRepository(db),
		Auditor:   auditor,
		Config:    contentGuardPipelineConfig(),
		// 审查长度上限走参数中心（content_guard.max_scan_runes），兜底值是领域常量
		// guardservice.MaxScanRunes。这是**同一把旋钮**的两层：常量管"默认多长"，
		// 参数中心管"这台部署多长"，越界一律回落到常量（比不设配置安全）。
		MaxScanRunes: contentGuardMaxScanRunes(),
	})
	guardservice.SetGlobalRuntime(rt)
	rt.Start()
	subscribeCustomerMessages(rt)
	installGuardGate(rt)

	apiState := "关闭（数据零出域，默认）"
	if auditor != nil {
		apiState = "开启"
	}
	logger.Infof("[content-guard] ✅ 内容审查已装配：表 guard_words/guard_policies/guard_tasks/guard_records/guard_sanctions，"+
		"外部审核 API=%s，workers=%d", apiState, contentGuardPipelineConfig().Workers)
	return rt
}

// StopContentGuardRuntime 停止 worker（进程退出时调用）。
func StopContentGuardRuntime(rt *guardservice.Runtime) {
	if rt == nil {
		return
	}
	// 先摘掉闸门：worker 停了但闸门还挂着，会让"冻结判定"读到正在关闭的运行时。
	agent_runtime.SetGuardGate(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rt.Stop(ctx)
}

// installGuardGate 把"冻结判定"接到 agent 运行时的入站处理链上。
//
// 效果：某终端用户被内容合规冻结后，后续消息不再生成 Agent 回复（= 拒收）。
// 注意这是**出站侧**的拒收（消息本身已由 webhook 落库），真正的入口拒绝需要
// 在渠道适配层做，见 docs/architecture/CONTENT_MODERATION_RESEARCH.md 的差距清单。
func installGuardGate(rt *guardservice.Runtime) {
	if rt == nil {
		agent_runtime.SetGuardGate(nil)
		return
	}
	agent_runtime.SetGuardGate(guardGateAdapter{rt: rt})
	logger.Infof("[content-guard] 已接管入站处理闸门：被冻结主体的消息不再生成回复")
}

// guardGateAdapter 把 contentguard 运行时适配成 agent 运行时认识的最小接口。
type guardGateAdapter struct{ rt *guardservice.Runtime }

func (g guardGateAdapter) Blocked(ctx context.Context, subjectType, subjectID string) bool {
	if g.rt == nil || subjectID == "" {
		return false
	}
	_, blocked := g.rt.IsBlocked(ctx, subjectType, subjectID)
	return blocked
}

// contentGuardPipelineConfig 从环境变量读流水线参数（装配期读一次，运行期不变）。
func contentGuardPipelineConfig() guardservice.PipelineConfig {
	return guardservice.PipelineConfig{
		Workers:      envIntWithFallback("CONTENT_GUARD_WORKERS", 2),
		BatchSize:    envIntWithFallback("CONTENT_GUARD_BATCH", 32),
		PollInterval: time.Duration(envIntWithFallback("CONTENT_GUARD_POLL_MS", 2000)) * time.Millisecond,
		MaxAttempts:  envIntWithFallback("CONTENT_GUARD_MAX_ATTEMPTS", 5),
	}
}

// buildContentGuardAuditor 依环境变量构造外部审核源；未开启返回 nil（不建、不调）。
//
// 默认 nil 是刻意的：本仓的默认形态是"数据零出域"，外部审核必须由部署方
// 显式配置 endpoint 并打开开关，且策略里还要再开一次 APIEnabled（双重显式）。
func buildContentGuardAuditor() provider.Auditor {
	if !envBool("CONTENT_GUARD_API_ENABLED", false) {
		return nil
	}
	endpoint := strings.TrimSpace(os.Getenv("CONTENT_GUARD_API_ENDPOINT"))
	if endpoint == "" {
		logger.Warnf("[content-guard] CONTENT_GUARD_API_ENABLED=true 但未配置 CONTENT_GUARD_API_ENDPOINT ⇒ 外部审核不启用")
		return nil
	}
	hosts := make([]string, 0, 2)
	for _, h := range strings.Split(os.Getenv("CONTENT_GUARD_API_HOSTS"), ",") {
		if h = strings.TrimSpace(h); h != "" {
			hosts = append(hosts, h)
		}
	}
	auditor, err := provider.NewHTTPAuditor(provider.HTTPConfig{
		Endpoint:      endpoint,
		Token:         strings.TrimSpace(os.Getenv("CONTENT_GUARD_API_TOKEN")),
		AllowedHosts:  hosts,
		Timeout:       time.Duration(envIntWithFallback("CONTENT_GUARD_API_TIMEOUT_MS", 3000)) * time.Millisecond,
		AllowInsecure: envBool("CONTENT_GUARD_API_ALLOW_INSECURE", false),
	})
	if err != nil {
		// 构造期报错就是"配错了"，不能静默降级成"没有外部审核但也没人知道"。
		logger.Errorf("[content-guard] 外部审核源配置非法，已禁用：%v", err)
		return nil
	}
	return auditor
}

// subscribeCustomerMessages 把入站客人消息投进审查队列（非阻塞）。
//
// 只做"提交"：判定/处置全在 worker。此处任何失败都只记日志，
// 绝不 return error 影响同 topic 的其他订阅者（EventBus 的 handler 错误只记日志，
// 但保持本函数恒返回 nil 可以让"审查开通与否"不成为消息链路的变量）。
func subscribeCustomerMessages(rt *guardservice.Runtime) {
	bus := event.GetGlobalBus()
	if bus == nil {
		logger.Warnf("[content-guard] 事件总线未初始化 ⇒ 入站消息不会自动提交审查（可在业务侧手动调 SubmitReview）")
		return
	}
	bus.Subscribe(event.TopicCustomerMessageReceived, func(evt event.Event) error {
		payload, ok := evt.Payload.(event.CustomerMessagePayload)
		if !ok {
			return nil
		}
		content := strings.TrimSpace(payload.Content)
		if content == "" {
			return nil
		}
		// 审查主体是"人"而不是"会话"：同一个访客换个会话再来必须继续累计，
		// 所以优先 CustomerID，只有拿不到人才退到 SessionID。
		subjectID := payload.CustomerID
		if subjectID == "" {
			subjectID = payload.SessionID
		}
		if subjectID == "" {
			return nil
		}
		ctx := context.Background()
		if payload.TenantID != 0 {
			ctx = tenant.WithTenantID(ctx, payload.TenantID)
		} else {
			ctx = tenant.ContextWithDefault(ctx)
		}
		if _, err := rt.SubmitReview(ctx, guardservice.ScanRequest{
			SubjectType: model.SubjectUser,
			SubjectID:   subjectID,
			Scene:       model.SceneInboundMessage,
			Content:     content,
			TraceID:     payload.TraceID,
		}); err != nil {
			logger.Warnf("[content-guard] 入站消息提交审查失败（消息链路不受影响）：subject=%s err=%v", subjectID, err)
		}
		return nil
	})
	logger.Infof("[content-guard] 已订阅 %s：入站客人消息自动进异步审查队列", event.TopicCustomerMessageReceived)
}

// contentGuardMaxScanRunes 读参数中心里的审查长度上限，越界/未配置一律回落到
// 领域常量 guardservice.MaxScanRunes。
//
// 为什么在装配期读一次而不是每次扫描读：这是"这台部署一次审多长的内容"的静态口径，
// 运行中每轮再查一次库只会给 worker 加一次读，却换不来任何"实时"的价值。
// 代价是：改了这个参数要重启进程才生效（参数中心的页面应据此提示）。
func contentGuardMaxScanRunes() int {
	lo, hi := guardservice.ScanRunesBounds()
	v := service.GlobalConfigParam().GetInt(context.Background(), "content_guard", "max_scan_runes", guardservice.MaxScanRunes)
	if v < lo || v > hi {
		logger.Warnf("[content-guard] 参数中心 content_guard.max_scan_runes=%d 超出可用区间 [%d,%d] ⇒ 沿用兜底值 %d",
			v, lo, hi, guardservice.MaxScanRunes)
		return guardservice.MaxScanRunes
	}
	return v
}

func envIntWithFallback(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return v
}

func envBool(key string, fallback bool) bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if raw == "" {
		return fallback
	}
	return raw == "1" || raw == "true" || raw == "yes" || raw == "on"
}
