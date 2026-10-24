package service

import (
	"context"
	"encoding/json"
	"sync"

	"hivemtk-user/internal/browser_automation/dto"
	"hivemtk-user/internal/browser_automation/model"
	"hivemtk-user/internal/browser_automation/repository"
	"hivemtk-user/internal/pkg/utils/logger"
)

// StepParams 步骤扩展参数（wait/scroll/extract/type 等），落库到 params JSONB
type StepParams struct {
	Ms            int               `json:"ms,omitempty"`
	ClearFirst    bool              `json:"clear_first,omitempty"`
	SubmitOnEnter bool              `json:"submit_on_enter,omitempty"`
	Direction     string            `json:"direction,omitempty"`
	Amount        int               `json:"amount,omitempty"`
	Selector      string            `json:"selector,omitempty"`
	TimeoutMs     int               `json:"timeout_ms,omitempty"`
	Selectors     map[string]string `json:"selectors,omitempty"`
	// click_near 用
	Anchor     string `json:"anchor,omitempty"`
	ButtonText string `json:"button_text,omitempty"`
	// assert/query 用（洞察层）
	AssertKind string `json:"assert_kind,omitempty"` // contains_text / selector_exists
	QueryKind  string `json:"query_kind,omitempty"`  // text / exists / count / attr
	Attribute  string `json:"attribute,omitempty"`   // query attr 用：属性名（D2 贯通）
}

// StepRuntime 步骤编排里的错误处理策略
type StepRuntime struct {
	ContinueOnError bool `json:"continue_on_error"`
	RetryCount      int  `json:"retry_count"`
	RetryBackoffMs  int  `json:"retry_backoff_ms"`
}

// parsedStep 编排 JSON 里的一个步骤
type parsedStep struct {
	dto.StepItem
	ContinueOnError bool
	RetryCount      int
	RetryBackoffMs  int
}

// brainPlanner Executor 对 Brain 层的全部依赖（从具体 *BrainService 改接缝）。
// *BrainService 会让整条 executeBrain 循环在单测里无法驱动（plan 必然打真 LLM），
// 于是「闸门是否在落库前生效」这类事实只能停在谓词层。有了这一层，测试可以塞进脚本化 plan，
// 其余部件（真 WS、真 DB、真步循环）全部照旧。
type brainPlanner interface {
	GeneratePlanReflect(ctx context.Context, taskID, sessionID uint, goal, platformID, snapshot string, st *reflectState) (stepsJSON []byte, done bool, err error)
	LastPlanTokens() int
	LastPlanModel() string // 审计帧每步的 llm_model（空串=该步没走 LLM，调用方不写该字段）
	LastAuxTokens() int
	JudgeDone(ctx context.Context, taskID, sessionID uint, goal, evidence string) (approve bool, reason string)
	SummarizeSession(ctx context.Context, taskID, sessionID uint, goal string, success, total int, extracts string) string
}

// Executor 执行引擎：steps 解释 + Hand 调用 + Session 流转 + 停止/超时控制。
// 状态收口原则：session/step 状态只由 Executor 写。
type Executor struct {
	hand        *Hand
	sessionRepo repository.BrowserSessionRepository
	stepRepo    repository.BrowserStepRepository
	cmdLogRepo  repository.BrowserCommandLogRepository
	// writeClaimRepo 不可逆写的存储层独占声明。它不是台账的缓存或兜底——
	// 台账（stepRepo）回答「过往发生过尝试没有」，本仓储回答「此刻谁有权去跨提交点」，
	// 后者只能由一条 INSERT 的原子性给出，所以 Executor 上不放任何内存态。
	// nil（未接线）在写步上是 fail-close，见 claimWriteSlot。
	writeClaimRepo repository.BrowserWriteClaimRepository
	brain          brainPlanner
	feedback       *FeedbackService

	// jevClient JEV 步决策客户端（nil=按环境变量即时构造；默认关闭，见 jev.go）。
	jevClient *JevClient

	// outreachReceiptRepo 触达回执仓储。
	// nil=未接线（回执全空，执行不受影响）——回执是增强层，落不落得住都不改「已发布」这个事实。
	outreachReceiptRepo repository.BrowserOutreachReceiptRepository

	// pendingReceipts sessionID → 待落回执行。
	// 只在「本会话已经发过触达」这一罕见路径上有内容，纯只读任务恒为空。
	receiptMu       sync.Mutex
	pendingReceipts map[uint][]*receiptPending

	// profileHealthRepo 主 Profile 健康熔断仓储（单主 Profile 健康监护）。
	// nil=未接线（熔断写入跳过，执行不受影响）——熔断是增强不是硬闸。
	profileHealthRepo repository.BrowserProfileHealthRepository

	// outreachDedupeRepo 跨任务触达去重仓储（同一平台同一目标同一动作同一文案只触达一次）。
	// nil=未接线（去重检查跳过，执行不受影响）——去重是防扰民层，真安全网是写声明与台账。
	outreachDedupeRepo repository.BrowserOutreachDedupeRepository

	// taskRepo 任务仓储（活动触达条数预算的原子扣减落在这里）。
	// nil=未接线 ⇒ 不扣减也不拦停（fail-open），与存量行为一致。
	// 之所以敢 fail-open：预算的失败形态是「多打扰了几次」，而双发的失败形态是「用户被重复骚扰
	// 且无法撤回」。预算若因接线缺失而把整条触达线锁死，那是更大的事故。
	taskRepo repository.BrowserTaskRepository

	// relocateLLM 自愈 LLM 接缝（默认 defaultRelocateLLM；测试替换免真机 LLM）
	relocateLLM func(ctx context.Context, systemPrompt, prompt string) (relocateOutcome, error)

	stopMu       sync.Mutex
	stopRegistry map[uint]chan struct{} // sessionID → stopCh

	// confirmRegistry sessionID → 待放行闸门（仅在写步提交点前挂起时存在）。
	// 与 stopRegistry 同构但生命周期不同：stop 通道随 session 全程注册，确认通道只在等待期存在
	// ——「存在即挂起」使 ConfirmPending 无需额外状态位。
	// 存的不再是裸通道而是闸门自身——「等谁批、批的是哪份载荷、什么时候到期」
	// 必须和通道同生死，否则读侧只有一个布尔，放行就是一张空白支票。
	confirmMu       sync.Mutex
	confirmRegistry map[uint]*confirmGate

	// 台账写失败后的两张降级表，与上面两张同构但语义相反——它们是「不能再派发写」的记录。
	//   ledgerBroken: sessionID → 最初那次台账写失败的原因，随会话结束清除（会话级降级）；
	//   ledgerGaps:   "taskID|text_hash" → 提交尝试点已跨越但库里没落成，跨会话保留到进程结束
	//                 （自动重试是同进程换新 session 跑同一任务，会话级降级挡不住它）。
	// 两张表都只在「台账写失败」这条罕见路径上增长，各自有界（gaps 见 ledgerGapCap）。
	// gaps 不跨重启 ⇒ 反馈层挂重试前必须查它一次（接线见 NewExecutor）。
	ledgerMu       sync.Mutex
	ledgerBroken   map[uint]string
	ledgerGaps     map[string]bool
	ledgerGapOrder []string
}

func NewExecutor(hand *Hand, sessionRepo repository.BrowserSessionRepository, stepRepo repository.BrowserStepRepository, brain *BrainService, feedback *FeedbackService) *Executor {
	e := &Executor{
		hand:         hand,
		sessionRepo:  sessionRepo,
		stepRepo:     stepRepo,
		relocateLLM:  defaultRelocateLLM,
		stopRegistry: make(map[uint]chan struct{}),

		confirmRegistry: make(map[uint]*confirmGate),

		ledgerBroken: make(map[uint]string),
		ledgerGaps:   make(map[string]bool),

		pendingReceipts: make(map[uint][]*receiptPending),
	}
	// 不把 *BrainService(nil) 直接赋给接口字段：那会得到一个「非 nil 的 nil 接口」，
	// e.brain == nil 判假，随后 executeBrain 就在 nil receiver 上发起真调用。
	if brain != nil {
		e.brain = brain
	}
	e.feedback = feedback
	if feedback != nil {
		// 单点接线：缺口表长在 Executor 上、挂自动重试的动作在 FeedbackService 上，
		// 两边互相看不见。接在构造处而不是某个调用点——漏接线时「不重试」的抑制会整条静默消失。
		feedback.SetLedgerGapProvider(e.HasCrossedLedgerGap)
	}
	return e
}

// finishStep 步行终态落库（唯一写入口）。
//
// 不能沿用执行 ctx：超时/中止腿上 ctx 此刻必然已 Done，用它的 UPDATE 会被 DB 驱动取消，
// 于是这一行永远停在 running——「写步失败是可见、可人工重跑的」这条承诺就地失效。
// 失败必须上报（不能 `_ =` 吞掉）：session/task 行有 stale_reconcile 兜底收敛，步行没有。
func (e *Executor) finishStep(ctx context.Context, stepRowID uint, status string, result []byte, durationMs int64, msg string) {
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), stepFinalWriteBudget)
	defer cancel()
	if err := e.stepRepo.UpdateResult(writeCtx, stepRowID, status, result, durationMs, msg); err != nil {
		logger.Errorf("[BrowserExec] 步终态落库失败 step=%d status=%s（面板将看不到这一步的结论，需从日志核对）: %v", stepRowID, status, err)
	}
}

// SetCommandLogRepository 日志仓储注入（路由装配可选——nil 时静默跳过埋点）
func (e *Executor) SetCommandLogRepository(r repository.BrowserCommandLogRepository) {
	e.cmdLogRepo = r
}

// SetWriteClaimRepository 存储层写声明闸门注入。
// 与日志注入**相反**，这里 nil 不是「可选」而是「闸门没接好」：写步会一律拒绝下发
// （见 claimWriteSlot）。宁可把任务跑成一屏红步，也不开一条没有独占裁决的写路径——
// 少接一行装配是可见的、可当天补的，一次双发是不可见、撤不回的。
func (e *Executor) SetWriteClaimRepository(r repository.BrowserWriteClaimRepository) {
	e.writeClaimRepo = r
}

// SetJevClient JEV 客户端注入（测试/未来 kv 接线用；nil=按环境变量即时构造）。
func (e *Executor) SetJevClient(c *JevClient) {
	e.jevClient = c
}

// SetProfileHealthRepo 主 Profile 健康仓储注入（路由装配可选——nil 时熔断写入跳过）。
func (e *Executor) SetProfileHealthRepo(r repository.BrowserProfileHealthRepository) {
	e.profileHealthRepo = r
}

// SetOutreachDedupeRepository 触达去重仓储注入（路由装配可选——nil 时去重检查跳过）。
func (e *Executor) SetOutreachDedupeRepository(r repository.BrowserOutreachDedupeRepository) {
	e.outreachDedupeRepo = r
}

// SetTaskRepository 任务仓储注入（活动触达条数预算扣减；nil 时不扣减）。
func (e *Executor) SetTaskRepository(r repository.BrowserTaskRepository) {
	e.taskRepo = r
}

// appendCommandLog append-only 命令-事件日志。失败仅告警不阻断执行：
// 日志是审计增强，不能反过来拖垮业务执行路径。
// seq 由调用方传入（session 局部计数）——Executor 是多 session 共享单例，
// 共享计数器在并发 session 下是 data race 且 seq 跨 session 交错。
// ok 是这一帧自带的结论，三态（见 model.BrowserCommandLog.Ok）：nil=此帧不携带结论，
// 下发帧恒为 nil——下发那一刻「成/败」都还没发生，填任何一个值都是替读者编一个结论。
func (e *Executor) appendCommandLog(ctx context.Context, sessionID, taskID, stepID uint, seq int, direction, action string, payload any, durationMs int64, ok *bool) {
	if e.cmdLogRepo == nil {
		return
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		blob = []byte(`{"marshal_error":"` + err.Error() + `"}`)
	}
	entry := &model.BrowserCommandLog{
		SessionID: sessionID, TaskID: taskID, StepID: stepID,
		Seq: seq, Direction: direction, Action: action,
		Payload: blob, DurationMs: durationMs, Ok: ok,
	}
	if err := e.cmdLogRepo.Append(ctx, entry); err != nil {
		logger.Errorf("[BrowserExec] 命令日志落库失败 session=%d seq=%d: %v", sessionID, entry.Seq, err) // P2-3 升级 error
	}
}

// verdict 把一个布尔观测升成审计帧的三态结论。名字刻意不叫 ok()：调用点写 verdict(err == nil)
// 才读得出「这是这一帧的结论」，而不是「这次成功了」。
func verdict(v bool) *bool { return &v }

// auditEventPayload 写步回包进审计流前的收口：丢掉 evidence 键。
// comment_verify 的回包含「页面上命中那条评论的文本节选」（primitives.js evidenceOf），
// 而命中项就是我们自己发出去的正文——写进 browser_steps.result / extracted_data 是追溯面板
// 要的凭据，写进 append-only 命令流则是把用户内容带进离线导出件。审计流要的是结论
// （已受理/已确认/未确认），内容证据另有其位。
func auditEventPayload(result []byte) []byte {
	if len(result) == 0 {
		return result
	}
	var m map[string]any
	if err := json.Unmarshal(result, &m); err != nil {
		return result // 非对象回包（数组/标量）原样进流：收口只针对带 evidence 的那一类
	}
	if _, has := m["evidence"]; !has {
		return result
	}
	delete(m, "evidence")
	blob, err := json.Marshal(m)
	if err != nil {
		return result
	}
	return blob
}
