package service

import (
	"context"
	"encoding/json"
	"time"

	"hivemtk-user/internal/browser_automation/model"
	"hivemtk-user/internal/browser_automation/repository"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/utils/logger"
)

// SessionService 会话查询 + 手动中断
type SessionService struct {
	sessionRepo repository.BrowserSessionRepository
	stepRepo    repository.BrowserStepRepository
	cmdLogRepo  repository.BrowserCommandLogRepository
	planRepo    repository.BrowserLLMPlanRepository
	// digestRepo 裁剪摘要读侧（/ A6）。它与 cmdLogRepo 是**同一份事实的两面**：
	// 命令流给出留下的行，摘要给出被裁掉的行。只装一个就等于让导出包有能力沉默。
	digestRepo repository.BrowserAuditDigestRepository
	executor   *Executor
}

func NewSessionService(sessionRepo repository.BrowserSessionRepository, stepRepo repository.BrowserStepRepository, executor *Executor) *SessionService {
	return &SessionService{sessionRepo: sessionRepo, stepRepo: stepRepo, executor: executor}
}

// SetCommandLogRepository D1：命令流查询仓储注入（装配期一次性，路由未注入时查询返回空不报错）
func (s *SessionService) SetCommandLogRepository(r repository.BrowserCommandLogRepository) {
	s.cmdLogRepo = r
}

// SetAuditDigestRepository 审计导出取裁剪摘要（未装配时该分量置空，同 D1 纪律）
func (s *SessionService) SetAuditDigestRepository(r repository.BrowserAuditDigestRepository) {
	s.digestRepo = r
}

// SetLLMPlanRepository I5：LLM 记录仓储注入（导出取 plan/judge/summary 成本账）
func (s *SessionService) SetLLMPlanRepository(r repository.BrowserLLMPlanRepository) {
	s.planRepo = r
}

func (s *SessionService) Get(ctx context.Context, id, userID uint) (*model.BrowserSession, error) {
	return s.sessionRepo.GetByID(ctx, id, userID)
}

func (s *SessionService) ListByUser(ctx context.Context, userID uint, status string, page, limit int) ([]*model.BrowserSession, int64, error) {
	return s.sessionRepo.ListByUser(ctx, userID, status, page, limit)
}

// FailRunningForUser Host 断连收口：该用户所有在途会话置 failed，返回收口条数。
//
// 走 system scope 是因为调用方给不了租户：`HostRegistry.onDisconnect` 在 WS 读协程里
// 触发，递给钩子的是 `context.Background()`（host_registry.go:231），而 browser_sessions
// 是租户表 ⇒ saas 形态下这一刀被隔离 callback fail-closed，掉线后的 active 会话永久躺着，
// 并一直占住并发闸（CountRunningByUser）——用户重开 Chrome 也起不了新任务。
// 收口判据是全局唯一的 user_id（一个用户只属一个租户），所以 system scope 触达的行集
// 与「已知租户时租户内执行」相同；口径与本域对账器（stale_reconcile 整轮跑在
// WithSystemScope 下）一致，不是新开的例外通道。
func (s *SessionService) FailRunningForUser(ctx context.Context, userID uint, reason string) (int, error) {
	sessions, err := s.sessionRepo.FailRunningByUser(tenant.WithSystemScope(ctx), userID, reason)
	if err != nil {
		return 0, err
	}
	return len(sessions), nil
}

func (s *SessionService) ListByTask(ctx context.Context, taskID, userID uint, page, limit int) ([]*model.BrowserSession, int64, error) {
	return s.sessionRepo.ListByTaskID(ctx, taskID, userID, page, limit)
}

func (s *SessionService) ListSteps(ctx context.Context, sessionID, userID uint) ([]*model.BrowserStep, error) {
	// 归属校验：session 必须属于该用户
	if _, err := s.sessionRepo.GetByID(ctx, sessionID, userID); err != nil {
		return nil, err
	}
	return s.stepRepo.ListBySessionID(ctx, sessionID)
}

// ListCommandLogs D1（G1 补口）：session 归属校验 + append-only 命令流查询（审计链读侧）。
// direction 空=全部；command/event/judge 三类帧按 seq 升序（铁律 4「日志可还原每一步」的兑现）。
func (s *SessionService) ListCommandLogs(ctx context.Context, sessionID, userID uint, direction string) ([]*model.BrowserCommandLog, error) {
	if s.cmdLogRepo == nil {
		return []*model.BrowserCommandLog{}, nil
	}
	if _, err := s.sessionRepo.GetByID(ctx, sessionID, userID); err != nil {
		return nil, err
	}
	all, err := s.cmdLogRepo.ListBySessionID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if direction == "" {
		return all, nil
	}
	out := make([]*model.BrowserCommandLog, 0, len(all))
	for _, l := range all {
		if l.Direction == direction {
			out = append(out, l)
		}
	}
	return out, nil
}

// ConfirmPending D7 读侧：session 是否正等人工放行（不落库，实时取 Executor 挂起态）
func (s *SessionService) ConfirmPending(sessionID uint) bool {
	return s.executor.ConfirmPending(sessionID)
}

// ConfirmGate D7 读侧详情：前端放行按钮要显示「正在等批的是哪一步、哪份内容、
// 什么时候到期」，并把 payload_hash 原样带回 confirm——布尔值撑不起一次有对象的批准。
// 归属校验与 ListSteps 同构：预览里含正文，越权读到就是外泄。
func (s *SessionService) ConfirmGate(ctx context.Context, sessionID, userID uint) (PendingConfirmGate, bool, error) {
	if _, err := s.sessionRepo.GetByID(ctx, sessionID, userID); err != nil {
		return PendingConfirmGate{}, false, err
	}
	gate, pending := s.executor.PendingGate(sessionID)
	if !pending {
		return PendingConfirmGate{}, false, nil
	}
	return gate, true, nil
}

// ConfirmStatus 放行请求的服务层结论（四态，与 executor 的三态差一个「在别的进程」）。
type ConfirmStatus string

const (
	ConfirmStatusGranted       ConfirmStatus = "granted"
	ConfirmStatusNone          ConfirmStatus = "no_gate"
	ConfirmStatusMismatch      ConfirmStatus = "payload_mismatch"
	ConfirmStatusOtherInstance ConfirmStatus = "gate_on_another_instance"
)

// ConfirmResult 放行结论 + 本次挂起闸门的详情（mismatch 时前端要重取哈希）。
type ConfirmResult struct {
	Status ConfirmStatus       `json:"status"`
	Gate   *PendingConfirmGate `json:"gate,omitempty"`
}

// gateElsewhere 库里最近一帧 d7_wait 是否仍「未到期」（动作名单源见 executor.go 的 gateWaitFrame）。
//
// expires_at 只是「写下的期望」，它与「本进程内存里没有闸门」AND 起来才构成结论：
// 挂起协程是进程内的，重启或换副本后内存 map 必空，而那一帧还躺在审计里。到期了就说到期了
// （回 no_gate），否则一条三年前的旧帧能让这个 session 永远被回答「闸门在别的实例上」。
func (s *SessionService) gateElsewhere(ctx context.Context, sessionID uint) bool {
	if s.cmdLogRepo == nil {
		return false
	}
	logs, err := s.cmdLogRepo.ListBySessionID(ctx, sessionID)
	if err != nil {
		// 读不动不等于「闸门在别处」，也不等于「不在」：按 no_gate 收口，让调用方看到布尔而不是猜测，
		// 但把原因留在日志——这是增强判据，不该把一次放行请求整体判死。
		logger.Warnf("[BrowserSession] session=%d 查 d7_wait 帧失败（跨进程归因降级为 no_gate）: %v", sessionID, err)
		return false
	}
	var latest *model.BrowserCommandLog
	for _, l := range logs {
		if l.Direction == "event" && l.Action == gateWaitFrame && (latest == nil || l.Seq > latest.Seq) {
			latest = l
		}
	}
	if latest == nil {
		return false
	}
	var frame struct {
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.Unmarshal(latest.Payload, &frame); err != nil || frame.ExpiresAt == "" {
		return false
	}
	exp, err := time.Parse(time.RFC3339Nano, frame.ExpiresAt)
	return err == nil && time.Now().Before(exp)
}

// Confirm D7：人工放行停在 require_confirm 闸门上的写操作提交点（起绑载荷）。
// payloadHash 必须是读侧（ConfirmGate）取到的那一份：不符即拒（fail-closed），闸门仍在、
// 一帧命令都不下发。归属校验与 Stop 同构。
func (s *SessionService) Confirm(ctx context.Context, sessionID, userID uint, payloadHash string) (ConfirmResult, error) {
	sess, err := s.sessionRepo.GetByID(ctx, sessionID, userID)
	if err != nil {
		return ConfirmResult{}, err
	}
	if sess.Status != "created" && sess.Status != "active" {
		return ConfirmResult{Status: ConfirmStatusNone}, nil
	}
	switch s.executor.SignalConfirm(sessionID, payloadHash) {
	case VerdictGranted:
		return ConfirmResult{Status: ConfirmStatusGranted}, nil
	case VerdictMismatch:
		gate, _ := s.executor.PendingGate(sessionID)
		return ConfirmResult{Status: ConfirmStatusMismatch, Gate: &gate}, nil
	default:
		// 本进程没有闸门：要么从没挂起过，要么闸门在另一个副本上（多实例部署 / 刚重启）。
		// 两者对用户的下一步完全不同（前者去查会话状态，后者去换一台实例或等对账收敛），
		// 所以拿审计帧再问一次库，不许折成一句「没有待确认的提交点」。
		if s.gateElsewhere(ctx, sessionID) {
			return ConfirmResult{Status: ConfirmStatusOtherInstance}, nil
		}
		return ConfirmResult{Status: ConfirmStatusNone}, nil
	}
}

// Stop 手动中断（session 置 stopped 由 Executor 收口；此处仅发信号 + 返回是否命中运行中）
func (s *SessionService) Stop(ctx context.Context, sessionID, userID uint, reason string) (bool, error) {
	sess, err := s.sessionRepo.GetByID(ctx, sessionID, userID)
	if err != nil {
		return false, err
	}
	if sess.Status != "created" && sess.Status != "active" {
		return false, nil
	}
	ok := s.executor.SignalStop(sessionID)
	return ok, nil
}

// ---- I5 审计导出（一次请求归并 session 全量审计事实，可离线归档）----

// SessionExportRow LLM 记录导出行（snapshot 大文本不带：G19 后本就可能为空且导出体积敏感；
// 成本账/归因全字段保留。快照原文只有库里那一份，`browser_llm_plans` 没有对外的查询端点，
// 要 replay 输入必须走 DB——别按「导出里没有」推断这件事没有发生）
type SessionExportRow struct {
	ID        uint      `json:"id"`
	Kind      string    `json:"kind"` // plan / judge / summary / jev_choice
	Goal      string    `json:"goal"`
	Steps     []byte    `json:"steps,omitempty"`
	Reasoning string    `json:"reasoning,omitempty"`
	Model     string    `json:"model"`
	TokenIn   int       `json:"token_in"`
	TokenOut  int       `json:"token_out"`
	CreatedAt time.Time `json:"created_at"`
}

// SessionExport I5：session 全量审计包=会话元数据+步流水+命令流+LLM 成本账+裁剪摘要。
// 归属校验与 ListCommandLogs 同构（GetByID 带 userID）。
// 分量 missing 与分量 read-failure 是两件事，必须分开：
//   - 仓储**未装配**（nil）⇒ 该分量置空、整包照样返回（与 D1 读侧同纪律，装配缺位是部署问题，
//     不该把一次导出判死）；
//   - 装配了却**查询失败** ⇒ 整包报错。空数组在审计包里只有一种读法——「这一段没发生过」，
//     把一次库没答上话伪装成「这个会话没跑过任何步」，正是 A6 立项要消灭的那个形状。
//
// 摘要分量（/ A6）刻意做成**同一个返回元组**而不是另开一个方法：元组少一个值，
// 调用方编译不过；两个方法则允许「只导了命令流、忘了导摘要」这种静默残缺长期存在，
// 而那种残缺正是本批要消灭的形状（空数组没人分得清是「没发生」还是「被裁了」）。
func (s *SessionService) SessionExport(ctx context.Context, sessionID, userID uint) (*model.BrowserSession, []*model.BrowserStep, []*model.BrowserCommandLog, []*SessionExportRow, []*model.BrowserAuditDigest, error) {
	sess, err := s.sessionRepo.GetByID(ctx, sessionID, userID)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	steps, err := s.stepRepo.ListBySessionID(ctx, sessionID)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	var logs []*model.BrowserCommandLog
	if s.cmdLogRepo != nil {
		l, err := s.cmdLogRepo.ListBySessionID(ctx, sessionID)
		if err != nil {
			return nil, nil, nil, nil, nil, err
		}
		logs = l
	}
	var digests []*model.BrowserAuditDigest
	if s.digestRepo != nil {
		d, err := s.digestRepo.ListBySessionID(ctx, sessionID)
		if err != nil {
			return nil, nil, nil, nil, nil, err
		}
		digests = d
	}
	var plans []*SessionExportRow
	if s.planRepo != nil {
		ps, err := s.planRepo.ListBySessionID(ctx, sessionID, 200)
		if err != nil {
			return nil, nil, nil, nil, nil, err
		}
		for _, p := range ps {
			plans = append(plans, &SessionExportRow{
				ID: p.ID, Kind: p.Kind, Goal: p.Goal, Steps: []byte(p.Steps),
				Reasoning: p.Reasoning, Model: p.Model,
				TokenIn: p.TokenIn, TokenOut: p.TokenOut, CreatedAt: p.CreatedAt,
			})
		}
	}
	return sess, steps, logs, plans, digests, nil
}
