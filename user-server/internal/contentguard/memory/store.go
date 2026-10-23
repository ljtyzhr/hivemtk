// Package memory 内容安全闸五个 Store 的内存实现（测试 / 离线演示用）。
//
// 存在的理由：审查链路的**正确性**（是否命中、是否升级、是否解除、是否幂等）必须能在
// 不连数据库的前提下被断言。本仓的测试库需要先 export POSTGRES_TEST_* 才能连
// （见 CLAUDE.md 3.5），把"谁被冻结多久"这类断言绑在有状态的外部库上，
// 会让这类测试既慢又脆。生产装配一律走 contentguard/repository（GORM）。
//
// 结构：一个 Store 持数据 + 五张"仓储视图"（WordRepo/PolicyRepo/...）。
//
//	为什么不用一个结构体直接实现五个接口：五个接口的方法名会撞车
//	（PolicyStore.Get(string) 与 RecordStore.Get(uint64)、SanctionStore.Create(*GuardSanction)
//	与 RecordStore.Create(*GuardRecord)），一个类型只能有一份方法集；
//	硬凑成同一个类型在 Go 里根本编译不过（这是一次真实的编译教训，不是假想）。
//
// 租户：本实现不模拟隔离 callback（测试都在单租户语义下进行），
// 跨租户隔离由 GORM callback + 真实库承担，不在内存实现的职责里。
package memory

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"hivemtk-user/internal/contentguard/model"
	"hivemtk-user/internal/contentguard/service"
)

// Store 内存实现的核心（数据 + 测试夹具）。
type Store struct {
	mu sync.Mutex

	wordSeq     uint
	policySeq   uint
	taskSeq     uint64
	recordSeq   uint64
	sanctionSeq uint64

	words     []model.GuardWord
	policies  []model.GuardPolicy
	tasks     []model.GuardTask
	records   []model.GuardRecord
	sanctions []model.GuardSanction

	now func() time.Time
}

// New 构造空存储。
func New() *Store {
	return &Store{now: time.Now}
}

// SetNow 注入时间源（与 Runtime 共用同一个时钟，测试才能确定性地验"到期"）。
func (s *Store) SetNow(now func() time.Time) {
	s.mu.Lock()
	s.now = now
	s.mu.Unlock()
}

// Deps 一次性拿到五个仓储视图（装配 Runtime 用）。
//
// 注意：返回的 deps 里 Auditor/Config/Now 由调用方按需覆盖。
func (s *Store) Deps() service.RuntimeDeps {
	return service.RuntimeDeps{
		Words:     WordRepo{s},
		Policies:  PolicyRepo{s},
		Tasks:     TaskRepo{s},
		Records:   RecordRepo{s},
		Sanctions: SanctionRepo{s},
	}
}

// ── 仓储视图 ───────────────────────────────────────────────────────────

// WordRepo 词库仓储视图。
type WordRepo struct{ s *Store }

// PolicyRepo 策略仓储视图。
type PolicyRepo struct{ s *Store }

// TaskRepo 任务队列仓储视图。
type TaskRepo struct{ s *Store }

// RecordRepo 审查记录仓储视图。
type RecordRepo struct{ s *Store }

// SanctionRepo 处置仓储视图。
type SanctionRepo struct{ s *Store }

var (
	_ service.WordStore     = WordRepo{}
	_ service.PolicyStore   = PolicyRepo{}
	_ service.TaskStore     = TaskRepo{}
	_ service.RecordStore   = RecordRepo{}
	_ service.SanctionStore = SanctionRepo{}
)

// ── 词库 ───────────────────────────────────────────────────────────────

// SeedWords 覆盖写入词条（测试夹具）。
func (s *Store) SeedWords(words ...model.GuardWord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, w := range words {
		s.wordSeq++
		w.ID = s.wordSeq
		s.words = append(s.words, w)
	}
}

// ListEnabled 取启用词条。
func (r WordRepo) ListEnabled(_ context.Context) ([]model.GuardWord, error) {
	s := r.s
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]model.GuardWord, 0, len(s.words))
	for _, w := range s.words {
		if w.Enabled {
			out = append(out, w)
		}
	}
	return out, nil
}

// List 分页查询。
func (r WordRepo) List(_ context.Context, q service.WordQuery) ([]model.GuardWord, int64, error) {
	s := r.s
	s.mu.Lock()
	defer s.mu.Unlock()
	matched := make([]model.GuardWord, 0, len(s.words))
	for _, w := range s.words {
		if q.ListKind != "" && w.ListKind != q.ListKind {
			continue
		}
		if q.Category != "" && w.Category != q.Category {
			continue
		}
		if q.Enabled != nil && w.Enabled != *q.Enabled {
			continue
		}
		if q.Keyword != "" && !containsFold(w.Word, q.Keyword) {
			continue
		}
		matched = append(matched, w)
	}
	return paginate(matched, q.Page, q.PageSize), int64(len(matched)), nil
}

// Get 取词条。
func (r WordRepo) Get(_ context.Context, id uint) (*model.GuardWord, error) {
	s := r.s
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.words {
		if s.words[i].ID == id {
			row := s.words[i]
			return &row, nil
		}
	}
	return nil, nil
}

// Upsert 按 (list_kind, word) 幂等写入。
func (r WordRepo) Upsert(_ context.Context, w *model.GuardWord) (*model.GuardWord, error) {
	s := r.s
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.words {
		if s.words[i].ListKind == w.ListKind && s.words[i].Word == w.Word {
			s.words[i].Category = w.Category
			s.words[i].Severity = w.Severity
			s.words[i].Enabled = w.Enabled
			s.words[i].Source = w.Source
			s.words[i].Remark = w.Remark
			row := s.words[i]
			return &row, nil
		}
	}
	s.wordSeq++
	w.ID = s.wordSeq
	s.words = append(s.words, *w)
	return w, nil
}

// BulkUpsert 批量写入。
func (r WordRepo) BulkUpsert(ctx context.Context, words []model.GuardWord) (int, error) {
	n := 0
	for i := range words {
		if _, err := r.Upsert(ctx, &words[i]); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// Delete 删除词条。
func (r WordRepo) Delete(_ context.Context, id uint) error {
	s := r.s
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.words {
		if s.words[i].ID == id {
			s.words = append(s.words[:i], s.words[i+1:]...)
			return nil
		}
	}
	return nil
}

// Words 词条快照（测试断言用）。
func (s *Store) Words() []model.GuardWord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]model.GuardWord, len(s.words))
	copy(out, s.words)
	return out
}

// ── 策略 ───────────────────────────────────────────────────────────────

// SetPolicy 覆盖写入策略（测试夹具）。
func (s *Store) SetPolicy(p model.GuardPolicy) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.policies {
		if s.policies[i].SubjectType == p.SubjectType {
			p.ID = s.policies[i].ID
			s.policies[i] = p
			return
		}
	}
	s.policySeq++
	p.ID = s.policySeq
	s.policies = append(s.policies, p)
}

// Get 取策略（读不到返回 nil, nil，与 GORM 的 ErrRecordNotFound 口径对齐由调用方处理）。
func (r PolicyRepo) Get(_ context.Context, subjectType string) (*model.GuardPolicy, error) {
	s := r.s
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.policies {
		if s.policies[i].SubjectType == subjectType {
			row := s.policies[i]
			return &row, nil
		}
	}
	return nil, nil
}

// List 全部策略。
func (r PolicyRepo) List(context.Context) ([]model.GuardPolicy, error) {
	s := r.s
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]model.GuardPolicy, len(s.policies))
	copy(out, s.policies)
	return out, nil
}

// Save 保存策略。
func (r PolicyRepo) Save(_ context.Context, p *model.GuardPolicy) (*model.GuardPolicy, error) {
	r.s.SetPolicy(*p)
	return p, nil
}

// ── 任务 ───────────────────────────────────────────────────────────────

// Enqueue 入队（TaskKey 幂等）。
func (r TaskRepo) Enqueue(_ context.Context, t *model.GuardTask) (bool, error) {
	s := r.s
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.tasks {
		if s.tasks[i].TaskKey == t.TaskKey {
			return false, nil
		}
	}
	s.taskSeq++
	t.ID = s.taskSeq
	if t.CreatedAt.IsZero() {
		t.CreatedAt = s.now()
	}
	s.tasks = append(s.tasks, *t)
	return true, nil
}

// ClaimDue 认领到期任务（置为 running 并累加 attempts，模拟 FOR UPDATE SKIP LOCKED 的效果）。
func (r TaskRepo) ClaimDue(_ context.Context, limit int, now time.Time) ([]model.GuardTask, error) {
	s := r.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 {
		limit = 32
	}
	claimed := make([]model.GuardTask, 0, limit)
	for i := range s.tasks {
		if len(claimed) >= limit {
			break
		}
		t := &s.tasks[i]
		if t.Status != model.TaskStatusPending || t.NextRunAt.After(now) {
			continue
		}
		t.Status = model.TaskStatusRunning
		t.Attempts++
		claimed = append(claimed, *t)
	}
	return claimed, nil
}

// MarkDone 完成。
func (r TaskRepo) MarkDone(_ context.Context, id uint64) error {
	s := r.s
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.tasks {
		if s.tasks[i].ID == id {
			s.tasks[i].Status = model.TaskStatusDone
			s.tasks[i].LastError = ""
		}
	}
	return nil
}

// MarkFailed 失败（retryAt 非零回到 pending，零值置 failed 死信）。
func (r TaskRepo) MarkFailed(_ context.Context, id uint64, errMsg string, retryAt time.Time) error {
	s := r.s
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.tasks {
		if s.tasks[i].ID != id {
			continue
		}
		s.tasks[i].LastError = errMsg
		if retryAt.IsZero() {
			s.tasks[i].Status = model.TaskStatusFailed
		} else {
			s.tasks[i].Status = model.TaskStatusPending
			s.tasks[i].NextRunAt = retryAt
		}
	}
	return nil
}

// CountPending 待处理数。
func (r TaskRepo) CountPending(context.Context) (int64, error) {
	return r.s.CountPending()
}

// CountPending 待处理数（快照口径，测试与 Stats 共用）。
func (s *Store) CountPending() (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var n int64
	for _, t := range s.tasks {
		if t.Status == model.TaskStatusPending {
			n++
		}
	}
	return n, nil
}

// Tasks 任务快照（测试断言用）。
func (s *Store) Tasks() []model.GuardTask {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]model.GuardTask, len(s.tasks))
	copy(out, s.tasks)
	return out
}

// ── 记录 ───────────────────────────────────────────────────────────────

// Create 写入记录。
func (r RecordRepo) Create(_ context.Context, rec *model.GuardRecord) error {
	s := r.s
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recordSeq++
	rec.ID = s.recordSeq
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = s.now()
	}
	s.records = append(s.records, *rec)
	return nil
}

// Get 取记录。
func (r RecordRepo) Get(_ context.Context, id uint64) (*model.GuardRecord, error) {
	s := r.s
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.records {
		if s.records[i].ID == id {
			row := s.records[i]
			return &row, nil
		}
	}
	return nil, nil
}

// List 分页查询。
func (r RecordRepo) List(_ context.Context, q service.RecordQuery) ([]model.GuardRecord, int64, error) {
	s := r.s
	s.mu.Lock()
	defer s.mu.Unlock()
	matched := make([]model.GuardRecord, 0, len(s.records))
	for _, rec := range s.records {
		if q.SubjectType != "" && rec.SubjectType != q.SubjectType {
			continue
		}
		if q.SubjectID != "" && rec.SubjectID != q.SubjectID {
			continue
		}
		if q.Scene != "" && rec.Scene != q.Scene {
			continue
		}
		if q.Verdict != "" && rec.Verdict != q.Verdict {
			continue
		}
		if q.ManualStatus != "" && rec.ManualStatus != q.ManualStatus {
			continue
		}
		if q.TraceID != "" && rec.TraceID != q.TraceID {
			continue
		}
		matched = append(matched, rec)
	}
	sort.SliceStable(matched, func(i, j int) bool { return matched[i].ID > matched[j].ID })
	return paginate(matched, q.Page, q.PageSize), int64(len(matched)), nil
}

// CountBlocksSince 窗口内拦截次数（升级判据）。
func (r RecordRepo) CountBlocksSince(_ context.Context, subjectType, subjectID string, since time.Time) (int, error) {
	s := r.s
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, rec := range s.records {
		if rec.SubjectType == subjectType && rec.SubjectID == subjectID &&
			rec.Verdict == model.VerdictBlock && !rec.CreatedAt.Before(since) {
			n++
		}
	}
	return n, nil
}

// UpdateManual 更新人工复核。
func (r RecordRepo) UpdateManual(_ context.Context, id uint64, status string, operatorID uint, at time.Time) error {
	s := r.s
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.records {
		if s.records[i].ID == id {
			s.records[i].ManualStatus = status
			s.records[i].OperatorID = operatorID
			s.records[i].ReviewedAt = &at
		}
	}
	return nil
}

// SetSanctionID 回填处置号。
func (r RecordRepo) SetSanctionID(_ context.Context, id, sanctionID uint64) error {
	s := r.s
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.records {
		if s.records[i].ID == id {
			s.records[i].SanctionID = sanctionID
		}
	}
	return nil
}

// Records 记录快照（测试断言用）。
func (s *Store) Records() []model.GuardRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]model.GuardRecord, len(s.records))
	copy(out, s.records)
	return out
}

// ── 处置 ───────────────────────────────────────────────────────────────

// FindActive 取生效处置（无则 nil, nil）。
func (r SanctionRepo) FindActive(_ context.Context, subjectType, subjectID string, now time.Time) (*model.GuardSanction, error) {
	s := r.s
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := len(s.sanctions) - 1; i >= 0; i-- {
		row := s.sanctions[i]
		if row.SubjectType != subjectType || row.SubjectID != subjectID {
			continue
		}
		if row.Effective(now) {
			return &row, nil
		}
	}
	return nil, nil
}

// Get 取处置。
func (r SanctionRepo) Get(_ context.Context, id uint64) (*model.GuardSanction, error) {
	s := r.s
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.sanctions {
		if s.sanctions[i].ID == id {
			row := s.sanctions[i]
			return &row, nil
		}
	}
	return nil, nil
}

// Create 新建处置。
func (r SanctionRepo) Create(_ context.Context, row *model.GuardSanction) error {
	s := r.s
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sanctionSeq++
	row.ID = s.sanctionSeq
	if row.CreatedAt.IsZero() {
		row.CreatedAt = s.now()
	}
	s.sanctions = append(s.sanctions, *row)
	return nil
}

// Update 更新处置。
func (r SanctionRepo) Update(_ context.Context, row *model.GuardSanction) error {
	s := r.s
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.sanctions {
		if s.sanctions[i].ID == row.ID {
			s.sanctions[i] = *row
			return nil
		}
	}
	s.sanctions = append(s.sanctions, *row)
	return nil
}

// List 分页查询处置。
func (r SanctionRepo) List(_ context.Context, q service.SanctionQuery) ([]model.GuardSanction, int64, error) {
	s := r.s
	s.mu.Lock()
	defer s.mu.Unlock()
	matched := make([]model.GuardSanction, 0, len(s.sanctions))
	for _, row := range s.sanctions {
		if q.SubjectType != "" && row.SubjectType != q.SubjectType {
			continue
		}
		if q.SubjectID != "" && row.SubjectID != q.SubjectID {
			continue
		}
		if q.Kind != "" && row.Kind != q.Kind {
			continue
		}
		if q.Level != "" && row.Level != q.Level {
			continue
		}
		if q.ActiveOnly && !row.Active {
			continue
		}
		matched = append(matched, row)
	}
	sort.SliceStable(matched, func(i, j int) bool { return matched[i].ID > matched[j].ID })
	return paginate(matched, q.Page, q.PageSize), int64(len(matched)), nil
}

// Sanctions 处置快照（测试断言用）。
func (s *Store) Sanctions() []model.GuardSanction {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]model.GuardSanction, len(s.sanctions))
	copy(out, s.sanctions)
	return out
}

func paginate[T any](rows []T, page, size int) []T {
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	start := (page - 1) * size
	if start >= len(rows) {
		return []T{}
	}
	end := start + size
	if end > len(rows) {
		end = len(rows)
	}
	return rows[start:end]
}

func containsFold(s, sub string) bool {
	return sub == "" || strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}
