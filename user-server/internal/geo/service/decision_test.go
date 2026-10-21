package service

import (
	"context"
	"errors"
	"testing"

	"hivemtk-user/internal/geo/model"
)

type stubDecisionTaskRepo struct {
	counts map[string]int64
	err    error
}

func (r *stubDecisionTaskRepo) Create(context.Context, *model.GeoContentTask) error { return nil }
func (r *stubDecisionTaskRepo) ListPending(_ context.Context, limit int) ([]*model.GeoContentTask, error) {
	// 列表刻意只回 limit 条：报表若用 len(ListPending) 就会把总量报成上限。
	n := r.counts["pending"]
	if int64(limit) < n {
		n = int64(limit)
	}
	out := make([]*model.GeoContentTask, n)
	return out, nil
}
func (r *stubDecisionTaskRepo) MarkDone(context.Context, string) error { return nil }
func (r *stubDecisionTaskRepo) CountByStatus(_ context.Context, status string) (int64, error) {
	return r.counts[status], r.err
}

type stubLeadReporter struct {
	n   int64
	err error
}

func (r *stubLeadReporter) CountCapturedLeads(context.Context) (int64, error) { return r.n, r.err }

func TestDecisionService_Report按总量而非列表长度(t *testing.T) {
	svc := NewDecisionService(
		&stubDecisionTaskRepo{counts: map[string]int64{"pending": 500, "done": 3}},
		&stubLeadReporter{n: 12},
	)
	report, err := svc.Report(context.Background())
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if report.TasksPending != 500 || report.TasksDone != 3 || report.LeadsCaptured != 12 {
		t.Fatalf("报表口径不符（待办被列表上限截断？）: %+v", report)
	}

	// 统计失败必须上抛，不能把 0 当成"没有任务"返回。
	svc = NewDecisionService(&stubDecisionTaskRepo{err: errors.New("db down")}, nil)
	if _, err := svc.Report(context.Background()); err == nil {
		t.Fatal("任务统计失败应报错")
	}
	svc = NewDecisionService(
		&stubDecisionTaskRepo{counts: map[string]int64{}},
		&stubLeadReporter{err: errors.New("clue down")},
	)
	if _, err := svc.Report(context.Background()); err == nil {
		t.Fatal("线索统计失败应报错")
	}
}
