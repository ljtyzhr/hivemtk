// pipeline_test.go 异步流水线的端到端断言（内存 Store，不连库）。
//
// 这一组覆盖本功能最核心的几条承诺：
//  1. Submit **不阻塞**：只入队，不扫描、不调外部 API、不处置；
//  2. **幂等**：同一内容重复提交不产生第二条任务、不罚两次；
//  3. 处置**升级**：窗口内屡次拦截 ⇒ 临时冻结升级为永久冻结；
//  4. 失败**语义区分**：可重试回 pending，超出次数进死信；
//  5. 外部 API 默认不调用，开启后失败降级为仅本地结论。
package service_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"hivemtk-user/internal/contentguard/memory"
	"hivemtk-user/internal/contentguard/model"
	"hivemtk-user/internal/contentguard/provider"
	"hivemtk-user/internal/contentguard/service"
)

// ── 夹具 ───────────────────────────────────────────────────────────────

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newClock() *fakeClock {
	return &fakeClock{t: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

type env struct {
	store  *memory.Store
	clock  *fakeClock
	rt     *service.Runtime
	audit  *fakeAuditor
	ctx    context.Context
	policy *model.GuardPolicy
}

type envOptions struct {
	words     []model.GuardWord
	policy    *model.GuardPolicy
	auditor   provider.Auditor
	maxAttemp int
	records   service.RecordStore
}

func newEnv(t *testing.T, opt envOptions) *env {
	t.Helper()
	clock := newClock()
	store := memory.New()
	store.SetNow(clock.Now)
	store.SeedWords(opt.words...)
	if opt.policy != nil {
		store.SetPolicy(*opt.policy)
	}
	cfg := service.PipelineConfig{
		Workers:      1,
		BatchSize:    16,
		PollInterval: time.Hour, // 不启动 worker；测试只用 ProcessDue 精确驱动
		MaxAttempts:  opt.maxAttemp,
		RetryDelay:   30 * time.Second,
		APITimeout:   time.Second,
	}
	deps := store.Deps()
	if opt.records != nil {
		deps.Records = opt.records
	}
	deps.Auditor = opt.auditor
	deps.Config = cfg
	deps.Now = clock.Now
	rt := service.NewRuntime(deps)
	e := &env{store: store, clock: clock, rt: rt, ctx: context.Background(), policy: opt.policy}
	if fa, ok := opt.auditor.(*fakeAuditor); ok {
		e.audit = fa
	}
	return e
}

func (e *env) submit(t *testing.T, subject, subjectID, scene, content string) bool {
	t.Helper()
	created, err := e.rt.SubmitReview(e.ctx, service.ScanRequest{
		SubjectType: subject, SubjectID: subjectID, Scene: scene, Content: content,
	})
	if err != nil {
		t.Fatalf("Submit 报错：%v", err)
	}
	return created
}

func (e *env) process(t *testing.T) int {
	t.Helper()
	n, err := e.rt.Pipeline.ProcessDue(e.ctx, 16)
	if err != nil {
		t.Fatalf("ProcessDue 报错：%v", err)
	}
	return n
}

func word(w string, sev int) model.GuardWord {
	return model.GuardWord{ListKind: model.ListKindDeny, Word: w, Category: "test", Severity: sev, Enabled: true}
}

type fakeAuditor struct {
	calls atomic.Int64
	res   provider.Result
	err   error
	block chan struct{} // 非空时 Audit 阻塞到 channel 关闭（模拟慢上游）
}

func (f *fakeAuditor) Name() string { return "api" }

func (f *fakeAuditor) Audit(ctx context.Context, _ provider.Request) (provider.Result, error) {
	f.calls.Add(1)
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return provider.Result{}, ctx.Err()
		}
	}
	return f.res, f.err
}

// flakyRecordStore 让 Create 可失败（模拟写记录时的库抖动）。
type flakyRecordStore struct {
	service.RecordStore
	failing atomic.Bool
}

func (f *flakyRecordStore) Create(ctx context.Context, r *model.GuardRecord) error {
	if f.failing.Load() {
		return errors.New("record insert boom")
	}
	return f.RecordStore.Create(ctx, r)
}

// ── 用例 ───────────────────────────────────────────────────────────────

func TestSubmit_EnqueuesWithoutScanning(t *testing.T) {
	e := newEnv(t, envOptions{words: []model.GuardWord{word("毒品", 3)}})

	if created := e.submit(t, model.SubjectUser, "u_1", model.SceneInboundMessage, "我要买毒品"); !created {
		t.Fatal("首次提交应入队")
	}
	// 关键断言：Submit 之后**没有任何**审查产物——不阻塞的含义就是这一步只做插入。
	if n := len(e.store.Records()); n != 0 {
		t.Fatalf("Submit 不得同步产出审查记录，得 %d 条", n)
	}
	if n := len(e.store.Sanctions()); n != 0 {
		t.Fatalf("Submit 不得同步施加处置，得 %d 条", n)
	}
	tasks := e.store.Tasks()
	if len(tasks) != 1 || tasks[0].Status != model.TaskStatusPending {
		t.Fatalf("任务应为 1 条 pending：%+v", tasks)
	}

	if n := e.process(t); n != 1 {
		t.Fatalf("应处理 1 条，得 %d", n)
	}

	recs := e.store.Records()
	if len(recs) != 1 {
		t.Fatalf("审查记录数 = %d，期望 1", len(recs))
	}
	rec := recs[0]
	if rec.Verdict != model.VerdictBlock || rec.Action != model.ActionReject {
		t.Fatalf("结论 = {%s,%s}，期望 block / reject", rec.Verdict, rec.Action)
	}
	if rec.Sources != "dict" {
		t.Fatalf("sources = %q，期望 dict", rec.Sources)
	}
	if rec.ContentSnippet == "" || rec.ContentDigest == "" {
		t.Fatalf("记录缺少片段/摘要：%+v", rec)
	}
	if len(rec.Hits) != 1 || rec.Hits[0].Word != "毒品" {
		t.Fatalf("命中证据不对：%+v", rec.Hits)
	}
	if t2 := e.store.Tasks(); t2[0].Status != model.TaskStatusDone {
		t.Fatalf("任务应标记 done，得 %s", t2[0].Status)
	}

	sanctions := e.store.Sanctions()
	if len(sanctions) != 1 {
		t.Fatalf("处置条数 = %d，期望 1", len(sanctions))
	}
	s := sanctions[0]
	if s.Level != model.SanctionLevelTemp || !s.Active || s.Source != "auto" {
		t.Fatalf("处置 = %+v，期望 temp/active/auto", s)
	}
	if s.ExpiresAt == nil || !s.ExpiresAt.Equal(e.clock.Now().Add(24*time.Hour)) {
		t.Fatalf("临时冻结到期时间 = %v，期望 now+24h", s.ExpiresAt)
	}
	if rec.SanctionID != s.ID {
		t.Fatalf("记录未回填处置号：record.SanctionID=%d sanction.ID=%d", rec.SanctionID, s.ID)
	}
	if blocked, ok := e.rt.IsBlocked(e.ctx, model.SubjectUser, "u_1"); !ok || blocked.ID != s.ID {
		t.Fatalf("处置后热路径应判定为拦：%+v ok=%v", blocked, ok)
	}

	st := e.rt.Pipeline.Stats()
	if st.Submitted != 1 || st.Enqueued != 1 || st.Processed != 1 || st.Blocked != 1 {
		t.Fatalf("计数不对：%+v", st)
	}
}

func TestSubmit_IsIdempotentByContentAndSubject(t *testing.T) {
	e := newEnv(t, envOptions{words: []model.GuardWord{word("毒品", 3)}})

	if !e.submit(t, model.SubjectUser, "u_1", model.SceneInboundMessage, "同样的内容") {
		t.Fatal("首次应入队")
	}
	if e.submit(t, model.SubjectUser, "u_1", model.SceneInboundMessage, "同样的内容") {
		t.Fatal("重复提交不得产生第二条任务（上游重投/前端重发都会这样）")
	}
	if !e.submit(t, model.SubjectUser, "u_2", model.SceneInboundMessage, "同样的内容") {
		t.Fatal("不同主体的同一内容必须各审一次（处罚分主体）")
	}
	if len(e.store.Tasks()) != 2 {
		t.Fatalf("任务数 = %d，期望 2", len(e.store.Tasks()))
	}
	if st := e.rt.Pipeline.Stats(); st.Deduped != 1 || st.Enqueued != 2 {
		t.Fatalf("计数不对：%+v", st)
	}

	e.process(t)
	if n := len(e.store.Records()); n != 2 {
		t.Fatalf("记录数 = %d，期望 2", n)
	}
}

func TestSubmit_DropsEmptyAndRejectsBadInput(t *testing.T) {
	e := newEnv(t, envOptions{})
	if e.submit(t, model.SubjectUser, "u_1", model.SceneInboundMessage, "   ") {
		t.Fatal("空白内容不应入队")
	}
	if st := e.rt.Pipeline.Stats(); st.Dropped != 1 {
		t.Fatalf("空内容应计入 Dropped：%+v", st)
	}
	if _, err := e.rt.SubmitReview(e.ctx, service.ScanRequest{
		SubjectType: "tenant", SubjectID: "u_1", Scene: model.SceneInboundMessage, Content: "x",
	}); err == nil {
		t.Fatal("未知主体类型必须报错")
	}
	if _, err := e.rt.SubmitReview(e.ctx, service.ScanRequest{
		SubjectType: model.SubjectUser, SubjectID: "u_1", Scene: "chat", Content: "x",
	}); err == nil {
		t.Fatal("未知场景必须报错")
	}

	// 装配缺失时返回 ErrRuntimeUnavailable（路由据此回 503），不是 panic。
	rt := service.NewRuntime(service.RuntimeDeps{})
	if _, err := rt.SubmitReview(e.ctx, service.ScanRequest{
		SubjectType: model.SubjectUser, Scene: model.SceneInboundMessage, Content: "x",
	}); !errors.Is(err, service.ErrRuntimeUnavailable) {
		t.Fatalf("未装配时应回 ErrRuntimeUnavailable，得 %v", err)
	}
}

func TestProcessDue_EscalatesToPermanentFreeze(t *testing.T) {
	e := newEnv(t, envOptions{words: []model.GuardWord{word("毒品", 3)}})

	for i, content := range []string{"第一条：毒品", "第二条：毒品", "第三条：毒品"} {
		e.submit(t, model.SubjectUser, "u_1", model.SceneInboundMessage, content)
		e.process(t)

		sanctions := e.store.Sanctions()
		if len(sanctions) != 1 {
			t.Fatalf("第 %d 次拦截后处置行数 = %d，期望 1（幂等：只续期/升级，不新增行）", i+1, len(sanctions))
		}
		want := model.SanctionLevelTemp
		if i == 2 {
			want = model.SanctionLevelPermanent
		}
		if sanctions[0].Level != want {
			t.Fatalf("第 %d 次拦截 ⇒ 档位 %q，期望 %q", i+1, sanctions[0].Level, want)
		}
	}

	perm := e.store.Sanctions()[0]
	if perm.ExpiresAt != nil {
		t.Fatalf("永久冻结不得有到期时间：%v", perm.ExpiresAt)
	}
	// 永久冻结不随时间失效（这是它区别于临时的唯一判据）。
	e.clock.Advance(365 * 24 * time.Hour)
	if _, ok := e.rt.IsBlocked(e.ctx, model.SubjectUser, "u_1"); !ok {
		t.Fatal("永久冻结一年后仍应生效")
	}
}

func TestProcessDue_TempFreezeExpires(t *testing.T) {
	e := newEnv(t, envOptions{words: []model.GuardWord{word("毒品", 3)}})
	e.submit(t, model.SubjectUser, "u_1", model.SceneInboundMessage, "卖毒品")
	e.process(t)

	if _, ok := e.rt.IsBlocked(e.ctx, model.SubjectUser, "u_1"); !ok {
		t.Fatal("刚施加的临时冻结应生效")
	}
	e.clock.Advance(23 * time.Hour)
	if _, ok := e.rt.IsBlocked(e.ctx, model.SubjectUser, "u_1"); !ok {
		t.Fatal("23 小时后仍应生效")
	}
	e.clock.Advance(2 * time.Hour)
	if s, ok := e.rt.IsBlocked(e.ctx, model.SubjectUser, "u_1"); ok {
		t.Fatalf("到期后必须自动失效（不依赖定时任务），得 %+v", s)
	}
}

func TestProcessDue_ReviewVerdictGoesToManualQueue(t *testing.T) {
	e := newEnv(t, envOptions{words: []model.GuardWord{word("有点黄", 2)}})
	e.submit(t, model.SubjectUser, "u_1", model.SceneInboundMessage, "这段话有点黄")
	e.process(t)

	recs := e.store.Records()
	if len(recs) != 1 {
		t.Fatalf("记录数 = %d", len(recs))
	}
	if recs[0].Verdict != model.VerdictReview || recs[0].ManualStatus != model.ManualStatusPending {
		t.Fatalf("转人工记录 = {%s,%s}，期望 review/pending", recs[0].Verdict, recs[0].ManualStatus)
	}
	if n := len(e.store.Sanctions()); n != 0 {
		t.Fatalf("转人工不得同时处置，得 %d 条处置", n)
	}
}

func TestReviewRecord_OverturnedReleasesSanction(t *testing.T) {
	e := newEnv(t, envOptions{words: []model.GuardWord{word("毒品", 3)}})
	e.submit(t, model.SubjectUser, "u_1", model.SceneInboundMessage, "买毒品")
	e.process(t)
	rec := e.store.Records()[0]
	if rec.SanctionID == 0 {
		t.Fatal("前置条件失败：应已施加处置")
	}

	got, err := e.rt.ReviewRecord(e.ctx, rec.ID, model.ManualStatusOverturned, 9)
	if err != nil {
		t.Fatalf("复核报错：%v", err)
	}
	if got.ManualStatus != model.ManualStatusOverturned || got.OperatorID != 9 {
		t.Fatalf("复核结果 = %+v", got)
	}
	// 改判必须连带解除：只改记录不解冻等于复核按钮是个安慰剂。
	if s, ok := e.rt.IsBlocked(e.ctx, model.SubjectUser, "u_1"); ok {
		t.Fatalf("改判后仍被冻结：%+v", s)
	}
	rel := e.store.Sanctions()[0]
	if rel.Active || rel.ReleasedAt == nil || rel.ReleaseReason == "" {
		t.Fatalf("处置未正确解除：%+v", rel)
	}

	// confirmed 维持原判：处置必须仍然生效。
	e2 := newEnv(t, envOptions{words: []model.GuardWord{word("毒品", 3)}})
	e2.submit(t, model.SubjectUser, "u_1", model.SceneInboundMessage, "买毒品")
	e2.process(t)
	rec2 := e2.store.Records()[0]
	if _, err := e2.rt.ReviewRecord(e2.ctx, rec2.ID, model.ManualStatusConfirmed, 9); err != nil {
		t.Fatalf("复核报错：%v", err)
	}
	if _, ok := e2.rt.IsBlocked(e2.ctx, model.SubjectUser, "u_1"); !ok {
		t.Fatal("维持原判时处置不得被解除")
	}
}

func TestReviewRecord_Errors(t *testing.T) {
	e := newEnv(t, envOptions{})
	if _, err := e.rt.ReviewRecord(e.ctx, 1, "maybe", 1); !errors.Is(err, service.ErrReviewStatusInvalid) {
		t.Fatalf("非法状态应回 ErrReviewStatusInvalid，得 %v", err)
	}
	if _, err := e.rt.ReviewRecord(e.ctx, 1, model.ManualStatusConfirmed, 1); !errors.Is(err, service.ErrRecordNotFound) {
		t.Fatalf("不存在的记录应回 ErrRecordNotFound，得 %v", err)
	}
	if _, err := e.rt.GetRecord(e.ctx, 42); !errors.Is(err, service.ErrRecordNotFound) {
		t.Fatalf("GetRecord 不存在应回 ErrRecordNotFound，得 %v", err)
	}
}

func TestProcessDue_RetryThenSucceed(t *testing.T) {
	base := memory.New()
	clock := newClock()
	base.SetNow(clock.Now)
	base.SeedWords(word("毒品", 3))
	// flaky 只包住 Create，其余方法与数据仍走 base。
	records := &flakyRecordStore{RecordStore: base.Deps().Records}

	deps := base.Deps()
	deps.Records = records
	deps.Config = service.PipelineConfig{MaxAttempts: 3, RetryDelay: 30 * time.Second, BatchSize: 8}
	deps.Now = clock.Now
	rt := service.NewRuntime(deps)
	ctx := context.Background()
	if _, err := rt.SubmitReview(ctx, service.ScanRequest{
		SubjectType: model.SubjectUser, SubjectID: "u_1",
		Scene: model.SceneInboundMessage, Content: "买毒品",
	}); err != nil {
		t.Fatalf("Submit 报错：%v", err)
	}

	records.failing.Store(true)
	if n, err := rt.Pipeline.ProcessDue(ctx, 8); err != nil || n != 1 {
		t.Fatalf("ProcessDue = %d, %v", n, err)
	}
	tasks := base.Tasks()
	if tasks[0].Status != model.TaskStatusPending {
		t.Fatalf("可重试失败必须回到 pending，得 %s", tasks[0].Status)
	}
	if !tasks[0].NextRunAt.After(clock.Now()) {
		t.Fatalf("重试任务必须顺延：next_run_at=%v", tasks[0].NextRunAt)
	}
	if tasks[0].LastError == "" {
		t.Fatal("失败必须留错误文本")
	}
	if n, _ := base.CountPending(); n != 1 {
		t.Fatalf("积压数 = %d，期望 1", n)
	}
	if st := rt.Pipeline.Stats(); st.Failed != 1 || st.Processed != 0 {
		t.Fatalf("计数不对：%+v", st)
	}

	// 抖动恢复后重试成功。
	records.failing.Store(false)
	clock.Advance(31 * time.Second)
	if n, err := rt.Pipeline.ProcessDue(ctx, 8); err != nil || n != 1 {
		t.Fatalf("重试 ProcessDue = %d, %v", n, err)
	}
	if got := base.Tasks()[0].Status; got != model.TaskStatusDone {
		t.Fatalf("重试成功后应 done，得 %s", got)
	}
	if n := len(base.Records()); n != 1 {
		t.Fatalf("记录数 = %d，期望 1", n)
	}
}

func TestProcessDue_ExhaustedAttemptsGoToDeadLetter(t *testing.T) {
	base := memory.New()
	clock := newClock()
	base.SetNow(clock.Now)
	base.SeedWords(word("毒品", 3))
	records := &flakyRecordStore{RecordStore: base.Deps().Records}
	records.failing.Store(true)

	deps := base.Deps()
	deps.Records = records
	deps.Config = service.PipelineConfig{MaxAttempts: 1, RetryDelay: 30 * time.Second, BatchSize: 8}
	deps.Now = clock.Now
	rt := service.NewRuntime(deps)
	ctx := context.Background()
	if _, err := rt.SubmitReview(ctx, service.ScanRequest{
		SubjectType: model.SubjectUser, SubjectID: "u_1",
		Scene: model.SceneInboundMessage, Content: "买毒品",
	}); err != nil {
		t.Fatalf("Submit 报错：%v", err)
	}
	if _, err := rt.Pipeline.ProcessDue(ctx, 8); err != nil {
		t.Fatalf("ProcessDue 报错：%v", err)
	}

	task := base.Tasks()[0]
	// 死信与可重试必须分开：把死信留在 pending 里会让"积压数"永远清不掉。
	if task.Status != model.TaskStatusFailed {
		t.Fatalf("超出最大尝试次数应置 failed（死信），得 %s", task.Status)
	}
	if n, _ := base.CountPending(); n != 0 {
		t.Fatalf("积压数 = %d，期望 0", n)
	}
}

func TestProcessDue_APIEscalatesVerdictWhenEnabled(t *testing.T) {
	policy := service.DefaultPolicy(model.SubjectUser)
	policy.APIEnabled = true
	policy.APISampleRate = 100
	auditor := &fakeAuditor{res: provider.Result{RiskLevel: 3, Labels: []string{"porn", "ad"}, Score: 0.97}}

	e := newEnv(t, envOptions{policy: policy, auditor: auditor})
	e.submit(t, model.SubjectUser, "u_1", model.SceneInboundMessage, "本地词库查不出但很危险的内容")
	e.process(t)

	if auditor.calls.Load() != 1 {
		t.Fatalf("抽样 100%% 应调用 1 次外部审核，实际 %d", auditor.calls.Load())
	}
	rec := e.store.Records()[0]
	// 本地词库无命中、外部判高风险 ⇒ 必须拦。这条最容易被"重算升级"逻辑吞掉。
	if rec.Verdict != model.VerdictBlock {
		t.Fatalf("外部高风险必须升级为 block，得 %s（max_severity=%d）", rec.Verdict, rec.MaxSeverity)
	}
	if rec.MaxSeverity != 3 {
		t.Fatalf("MaxSeverity = %d，期望 3", rec.MaxSeverity)
	}
	if !strings.Contains(rec.Sources, "api") {
		t.Fatalf("sources 应包含 api：%q", rec.Sources)
	}
	if len(e.store.Sanctions()) != 1 {
		t.Fatal("block 结论应施加处置")
	}
}

func TestProcessDue_APIFailureDegradesToLocalVerdict(t *testing.T) {
	policy := service.DefaultPolicy(model.SubjectUser)
	policy.APIEnabled = true
	policy.APISampleRate = 100
	auditor := &fakeAuditor{err: errors.New("upstream 502")}

	e := newEnv(t, envOptions{policy: policy, auditor: auditor})
	e.submit(t, model.SubjectUser, "u_1", model.SceneInboundMessage, "本地也没有命中")
	e.process(t)

	if auditor.calls.Load() != 1 {
		t.Fatalf("应尝试调用 1 次，实际 %d", auditor.calls.Load())
	}
	rec := e.store.Records()[0]
	if rec.Verdict != model.VerdictPass {
		t.Fatalf("上游失败必须降级为仅本地结论（pass），得 %s", rec.Verdict)
	}
	if strings.Contains(rec.Sources, "api") {
		t.Fatalf("失败不得留下「api 已参与」的假象：%q", rec.Sources)
	}
}

func TestProcessDue_APINotCalledByDefault(t *testing.T) {
	auditor := &fakeAuditor{res: provider.Result{RiskLevel: 3}}
	e := newEnv(t, envOptions{auditor: auditor})
	e.submit(t, model.SubjectUser, "u_1", model.SceneInboundMessage, "随便一段话")
	e.process(t)

	// 出厂策略 APIEnabled=false ⇒ 一个请求都不发（数据零出域）。
	if n := auditor.calls.Load(); n != 0 {
		t.Fatalf("默认配置下不得调用外部审核，实际调用 %d 次", n)
	}
}

func TestProcessDue_ConcurrentWorkersAreSafe(t *testing.T) {
	// 多个 worker 同时 drain 时不能重复处理同一任务、不能重复写记录（-race 下跑）。
	e := newEnv(t, envOptions{
		words: []model.GuardWord{word("毒品", 3)},
	})
	const total = 40
	for i := 0; i < total; i++ {
		e.submit(t, model.SubjectUser, "u_1", model.SceneInboundMessage, "卖毒品-"+strings.Repeat("x", i))
	}

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = e.rt.Pipeline.ProcessDue(context.Background(), 8)
		}()
	}
	wg.Wait()
	for {
		n, err := e.rt.Pipeline.ProcessDue(context.Background(), 16)
		if err != nil {
			t.Fatalf("ProcessDue 报错：%v", err)
		}
		if n == 0 {
			break
		}
	}
	if got := len(e.store.Records()); got != total {
		t.Fatalf("记录数 = %d，期望 %d（并发认领不得重复处理）", got, total)
	}
	if n, _ := e.store.CountPending(); n != 0 {
		t.Fatalf("应无积压，得 %d", n)
	}
}

func TestRuntimeStats(t *testing.T) {
	e := newEnv(t, envOptions{words: []model.GuardWord{word("毒品", 3)}})
	e.submit(t, model.SubjectUser, "u_1", model.SceneInboundMessage, "卖毒品")
	stats := e.rt.Stats(e.ctx)
	if _, ok := stats["pipeline"]; !ok {
		t.Fatalf("stats 缺 pipeline：%+v", stats)
	}
	if n, ok := stats["pending_tasks"]; !ok || n.(int64) != 1 {
		t.Fatalf("pending_tasks 不对：%+v", stats["pending_tasks"])
	}
}
