// runtime.go 运行时装配与业务门面。
//
// Runtime 是 controller 唯一可见的服务对象：它把 Store 接口、Engine、Pipeline、
// SanctionService 汇到一处，并对"词库/策略/记录/处置"四类操作给出业务级方法。
// 装配（从 *gorm.DB 造仓储、读环境变量造审核源、启动 worker）在 internal/app 的
// wiring 里做——service 层不碰 *gorm.DB，也不读环境变量。
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"hivemtk-user/internal/contentguard/model"
	"hivemtk-user/internal/contentguard/provider"
	"hivemtk-user/internal/pkg/tenant"
)

// RuntimeDeps 装配依赖。
type RuntimeDeps struct {
	Words     WordStore
	Policies  PolicyStore
	Tasks     TaskStore
	Records   RecordStore
	Sanctions SanctionStore
	// Auditor 外部审核源；nil = 不启用 L3。
	Auditor provider.Auditor
	Config  PipelineConfig
	// MaxScanRunes 单条内容审查上限（rune）；<=0 或越界时用 MaxScanRunes 兜底。
	// 由装配处从参数中心 content_guard.max_scan_runes 读出后注入（领域层不读配置中心）。
	MaxScanRunes int
	Now          func() time.Time
}

// Runtime 内容安全闸运行时。
type Runtime struct {
	Words     WordStore
	Policies  PolicyStore
	Tasks     TaskStore
	Records   RecordStore
	Sanctions SanctionStore

	Engine   *Engine
	Pipeline *Pipeline
	Sanction *SanctionService

	now func() time.Time
}

// NewRuntime 构造运行时（不启动 worker，见 Start）。
func NewRuntime(deps RuntimeDeps) *Runtime {
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	engine := NewEngine(deps.Words, deps.Policies).WithMaxScanRunes(deps.MaxScanRunes)
	sanctionSvc := NewSanctionService(deps.Sanctions)
	pipeline := NewPipeline(engine, deps.Tasks, deps.Records, sanctionSvc, deps.Auditor, deps.Config)
	rt := &Runtime{
		Words:     deps.Words,
		Policies:  deps.Policies,
		Tasks:     deps.Tasks,
		Records:   deps.Records,
		Sanctions: deps.Sanctions,
		Engine:    engine,
		Pipeline:  pipeline,
		Sanction:  sanctionSvc,
		now:       now,
	}
	// 时间源统一：引擎/流水线/处置用同一个 now，测试里才能把"冻结 24h 到期"确定性地验出来。
	engine.now = now
	pipeline.now = now
	sanctionSvc.now = now
	return rt
}

// Start 启动异步审查 worker。
func (r *Runtime) Start() {
	if r == nil || r.Pipeline == nil {
		return
	}
	r.Pipeline.Start()
}

// Stop 停止 worker。
func (r *Runtime) Stop(ctx context.Context) {
	if r == nil || r.Pipeline == nil {
		return
	}
	r.Pipeline.Stop(ctx)
}

// Available 运行时是否可用（路由据此决定 503）。
func (r *Runtime) Available() bool {
	return r != nil && r.Words != nil && r.Policies != nil && r.Records != nil && r.Sanctions != nil
}

// ── 词库 ───────────────────────────────────────────────────────────────

// WordInput 词条写入入参。
type WordInput struct {
	ListKind string `json:"list_kind"`
	Word     string `json:"word"`
	Category string `json:"category"`
	Severity int    `json:"severity"`
	Enabled  *bool  `json:"enabled"`
	Remark   string `json:"remark"`
}

// ListWords 词库分页查询。
func (r *Runtime) ListWords(ctx context.Context, q WordQuery) ([]model.GuardWord, int64, error) {
	if !r.Available() {
		return nil, 0, ErrRuntimeUnavailable
	}
	return r.Words.List(ctx, q)
}

// UpsertWord 新增/更新词条。
func (r *Runtime) UpsertWord(ctx context.Context, in WordInput) (*model.GuardWord, error) {
	if !r.Available() {
		return nil, ErrRuntimeUnavailable
	}
	word, err := buildWord(in, "manual")
	if err != nil {
		return nil, err
	}
	saved, err := r.Words.Upsert(ctx, word)
	if err != nil {
		return nil, err
	}
	r.invalidateDict(ctx)
	return saved, nil
}

// ImportWords 批量导入词条（覆盖同名词条）。
func (r *Runtime) ImportWords(ctx context.Context, inputs []WordInput) (int, error) {
	if !r.Available() {
		return 0, ErrRuntimeUnavailable
	}
	if len(inputs) == 0 {
		return 0, nil
	}
	if len(inputs) > 5000 {
		return 0, fmt.Errorf("单次导入不得超过 5000 条（收到 %d 条）", len(inputs))
	}
	words := make([]model.GuardWord, 0, len(inputs))
	seen := make(map[string]bool, len(inputs))
	for _, in := range inputs {
		w, err := buildWord(in, "import")
		if err != nil {
			continue
		}
		key := w.ListKind + "|" + strings.ToLower(w.Word)
		if seen[key] {
			continue
		}
		seen[key] = true
		words = append(words, *w)
	}
	n, err := r.Words.BulkUpsert(ctx, words)
	if err != nil {
		return n, err
	}
	r.invalidateDict(ctx)
	return n, nil
}

// DeleteWord 删除词条。
func (r *Runtime) DeleteWord(ctx context.Context, id uint) error {
	if !r.Available() {
		return ErrRuntimeUnavailable
	}
	if err := r.Words.Delete(ctx, id); err != nil {
		return err
	}
	r.invalidateDict(ctx)
	return nil
}

func buildWord(in WordInput, defaultSource string) (*model.GuardWord, error) {
	word := strings.TrimSpace(in.Word)
	if word == "" {
		return nil, errors.New("词条内容不能为空")
	}
	if runes := []rune(word); len(runes) > 200 {
		return nil, errors.New("词条长度不得超过 200 个字符")
	}
	kind := in.ListKind
	if kind == "" {
		kind = model.ListKindDeny
	}
	if kind != model.ListKindDeny && kind != model.ListKindAllow {
		return nil, errors.New("词条类型只支持 deny / allow")
	}
	severity := in.Severity
	if severity <= 0 {
		severity = 1
	}
	if severity > 3 {
		severity = 3
	}
	category := strings.TrimSpace(in.Category)
	if category == "" {
		category = "custom"
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	return &model.GuardWord{
		ListKind: kind,
		Word:     word,
		Category: category,
		Severity: severity,
		Enabled:  enabled,
		Source:   defaultSource,
		Remark:   in.Remark,
	}, nil
}

func (r *Runtime) invalidateDict(ctx context.Context) {
	id, ok := tenant.FromContext(ctx)
	if !ok {
		id = tenant.DefaultTenantID
	}
	if d := r.Engine.Dictionary(); d != nil {
		d.Invalidate(id)
	}
}

// ── 策略 ───────────────────────────────────────────────────────────────

// PolicyInput 策略写入入参（缺省字段保留原值）。
type PolicyInput struct {
	SubjectType       string `json:"subject_type"`
	Enabled           *bool  `json:"enabled"`
	Scenes            string `json:"scenes"`
	BlockSeverity     int    `json:"block_severity"`
	ReviewSeverity    int    `json:"review_severity"`
	BlockAction       string `json:"block_action"`
	EscalatePermanent int    `json:"escalate_permanent"`
	TempFreezeMinutes int    `json:"temp_freeze_minutes"`
	WindowMinutes     int    `json:"window_minutes"`
	APISampleRate     int    `json:"api_sample_rate"`
	APIEnabled        *bool  `json:"api_enabled"`
	OperatorID        uint   `json:"-"`
}

// ListPolicies 列出策略；缺失的主体类型按出厂策略补齐（读不到≠没有策略）。
func (r *Runtime) ListPolicies(ctx context.Context) ([]model.GuardPolicy, error) {
	if !r.Available() {
		return nil, ErrRuntimeUnavailable
	}
	rows, err := r.Policies.List(ctx)
	if err != nil {
		return nil, err
	}
	have := make(map[string]bool, len(rows))
	for _, row := range rows {
		have[row.SubjectType] = true
	}
	for _, st := range []string{model.SubjectUser, model.SubjectMerchant} {
		if !have[st] {
			rows = append(rows, *NormalizePolicy(DefaultPolicy(st)))
		}
	}
	return rows, nil
}

// SavePolicy 保存策略（缺失字段回落当前值或出厂值）。
func (r *Runtime) SavePolicy(ctx context.Context, in PolicyInput) (*model.GuardPolicy, error) {
	if !r.Available() {
		return nil, ErrRuntimeUnavailable
	}
	if err := ValidateSubjectType(in.SubjectType); err != nil {
		return nil, err
	}
	cur, err := r.Policies.Get(ctx, in.SubjectType)
	if err != nil || cur == nil {
		cur = DefaultPolicy(in.SubjectType)
	}
	next := NormalizePolicy(cur)
	next.SubjectType = in.SubjectType
	if in.Enabled != nil {
		next.Enabled = *in.Enabled
	}
	if strings.TrimSpace(in.Scenes) != "" {
		for _, s := range strings.Split(in.Scenes, ",") {
			if err := ValidateScene(strings.TrimSpace(s)); err != nil {
				return nil, err
			}
		}
		next.Scenes = in.Scenes
	}
	if in.BlockSeverity > 0 {
		next.BlockSeverity = in.BlockSeverity
	}
	if in.ReviewSeverity > 0 {
		next.ReviewSeverity = in.ReviewSeverity
	}
	if in.BlockAction != "" {
		next.BlockAction = in.BlockAction
	}
	if in.EscalatePermanent > 0 {
		next.EscalatePermanent = in.EscalatePermanent
	}
	if in.TempFreezeMinutes > 0 {
		next.TempFreezeMinutes = in.TempFreezeMinutes
	}
	if in.WindowMinutes > 0 {
		next.WindowMinutes = in.WindowMinutes
	}
	if in.APISampleRate >= 0 {
		next.APISampleRate = in.APISampleRate
	}
	if in.APIEnabled != nil {
		next.APIEnabled = *in.APIEnabled
	}
	next.UpdatedBy = in.OperatorID
	saved, err := r.Policies.Save(ctx, NormalizePolicy(next))
	if err != nil {
		return nil, err
	}
	return saved, nil
}

// ── 审查 ───────────────────────────────────────────────────────────────

// ScanNow 同步试算（只返回结论，不写任务、不写记录、不处置）。
//
// 用途：运营调词库时点"测试一下"。它跑的是与 worker 完全相同的引擎，
// 所以"这里显示拦、线上却放行"这类口径分裂不会发生。
func (r *Runtime) ScanNow(ctx context.Context, req ScanRequest) (*Assessment, error) {
	if r == nil || r.Engine == nil {
		return nil, ErrRuntimeUnavailable
	}
	return r.Engine.Scan(ctx, req)
}

// SubmitReview 提交异步审查（业务侧唯一入口，不阻塞）。
func (r *Runtime) SubmitReview(ctx context.Context, req ScanRequest) (bool, error) {
	if r == nil || r.Pipeline == nil {
		return false, ErrRuntimeUnavailable
	}
	return r.Pipeline.Submit(ctx, req)
}

// IsBlocked 热路径判断：该主体当前是否被冻结/拉黑。
func (r *Runtime) IsBlocked(ctx context.Context, subjectType, subjectID string) (*model.GuardSanction, bool) {
	if r == nil || r.Sanction == nil {
		return nil, false
	}
	return r.Sanction.IsBlocked(ctx, subjectType, subjectID)
}

// ── 记录 ───────────────────────────────────────────────────────────────

// ListRecords 审查记录分页查询。
func (r *Runtime) ListRecords(ctx context.Context, q RecordQuery) ([]model.GuardRecord, int64, error) {
	if !r.Available() {
		return nil, 0, ErrRuntimeUnavailable
	}
	return r.Records.List(ctx, q)
}

// GetRecord 记录详情。
func (r *Runtime) GetRecord(ctx context.Context, id uint64) (*model.GuardRecord, error) {
	if !r.Available() {
		return nil, ErrRuntimeUnavailable
	}
	rec, err := r.Records.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, ErrRecordNotFound
	}
	return rec, nil
}

// ReviewRecord 人工复核：confirmed 维持原判，overturned 改判并**连带解除**自动处置。
//
// 为什么"改判必须解除处置"：误杀的代价不只是这一条内容被拦，而是这个人/这个商户
// 被冻结了一整天。只改记录不解冻，等于复核按钮是个安慰剂。
func (r *Runtime) ReviewRecord(ctx context.Context, id uint64, status string, operatorID uint) (*model.GuardRecord, error) {
	if !r.Available() {
		return nil, ErrRuntimeUnavailable
	}
	switch status {
	case model.ManualStatusConfirmed, model.ManualStatusOverturned:
	default:
		return nil, ErrReviewStatusInvalid
	}
	rec, err := r.GetRecord(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := r.Records.UpdateManual(ctx, id, status, operatorID, r.now()); err != nil {
		return nil, err
	}
	if status == model.ManualStatusOverturned && rec.SanctionID != 0 && r.Sanction != nil {
		if _, rErr := r.Sanction.Release(ctx, rec.SanctionID, operatorID, "人工复核改判：误杀，撤销自动处置"); rErr != nil {
			return nil, rErr
		}
	}
	rec.ManualStatus = status
	rec.OperatorID = operatorID
	at := r.now()
	rec.ReviewedAt = &at
	return rec, nil
}

// ── 处置 ───────────────────────────────────────────────────────────────

// ApplySanction 人工施加处置（冻结/拉黑）。
func (r *Runtime) ApplySanction(ctx context.Context, in ApplyInput) (*model.GuardSanction, error) {
	if r == nil || r.Sanction == nil {
		return nil, ErrRuntimeUnavailable
	}
	return r.Sanction.Apply(ctx, in)
}

// ListSanctions 处置列表。
func (r *Runtime) ListSanctions(ctx context.Context, q SanctionQuery) ([]model.GuardSanction, int64, error) {
	if r == nil || r.Sanction == nil {
		return nil, 0, ErrRuntimeUnavailable
	}
	return r.Sanction.List(ctx, q)
}

// ReleaseSanction 解除处置。
func (r *Runtime) ReleaseSanction(ctx context.Context, id uint64, operatorID uint, reason string) (*model.GuardSanction, error) {
	if r == nil || r.Sanction == nil {
		return nil, ErrRuntimeUnavailable
	}
	return r.Sanction.Release(ctx, id, operatorID, reason)
}

// Stats 运行态统计（运维面板与健康检查）。
func (r *Runtime) Stats(ctx context.Context) map[string]any {
	out := map[string]any{}
	if r == nil {
		return out
	}
	if r.Pipeline != nil {
		out["pipeline"] = r.Pipeline.Stats()
		if r.Tasks != nil {
			if n, err := r.Tasks.CountPending(ctx); err == nil {
				out["pending_tasks"] = n
			}
		}
	}
	if r.Engine != nil && r.Engine.Dictionary() != nil {
		out["dictionary"] = r.Engine.Dictionary().Stats()
	}
	return out
}

// ── 全局登记处 ─────────────────────────────────────────────────────────

var (
	globalRuntimeMu sync.RWMutex
	globalRuntime   *Runtime
)

// SetGlobalRuntime 装配/卸载全局运行时（传 nil = 卸载，路由端点回 503）。
func SetGlobalRuntime(rt *Runtime) {
	globalRuntimeMu.Lock()
	globalRuntime = rt
	globalRuntimeMu.Unlock()
}

// GlobalRuntime 读取全局运行时（未装配返回 nil）。
func GlobalRuntime() *Runtime {
	globalRuntimeMu.RLock()
	defer globalRuntimeMu.RUnlock()
	return globalRuntime
}
