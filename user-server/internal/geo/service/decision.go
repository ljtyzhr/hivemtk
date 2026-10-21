package service

import (
	"context"

	"hivemtk-user/internal/geo/model"
	"hivemtk-user/internal/geo/repository"
)

// DecisionReportQuerier L4 结果指标查询端口（主域线索统计由装配层实现）
type DecisionReportQuerier interface {
	CountCapturedLeads(ctx context.Context) (int64, error)
}

// DecisionReport 决策链总览（思维链规模 → 补位任务漏斗 → 捕获线索）
type DecisionReport struct {
	TasksPending  int64 `json:"tasks_pending"`
	TasksDone     int64 `json:"tasks_done"`
	LeadsCaptured int64 `json:"leads_captured"`
}

// DecisionService 决策链补位任务的读写编排
type DecisionService struct {
	taskRepo repository.GeoContentTaskRepository
	report   DecisionReportQuerier
}

func NewDecisionService(taskRepo repository.GeoContentTaskRepository, report DecisionReportQuerier) *DecisionService {
	return &DecisionService{taskRepo: taskRepo, report: report}
}

// ListTasks 待处理补位任务（内容生产管线消费入口）
func (s *DecisionService) ListTasks(ctx context.Context, limit int) ([]*model.GeoContentTask, error) {
	return s.taskRepo.ListPending(ctx, limit)
}

// MarkDone 标记任务完成；任务不存在（含已软删/跨租户）返回 gorm.ErrRecordNotFound
func (s *DecisionService) MarkDone(ctx context.Context, id string) error {
	return s.taskRepo.MarkDone(ctx, id)
}

// Report 汇总；任务计数走 CountByStatus 而不是 len(ListPending)：
// 列表有 200 条上限，待办超过 200 时会把总量报成 200。
func (s *DecisionService) Report(ctx context.Context) (DecisionReport, error) {
	var out DecisionReport
	pending, err := s.taskRepo.CountByStatus(ctx, "pending")
	if err != nil {
		return out, err
	}
	done, err := s.taskRepo.CountByStatus(ctx, "done")
	if err != nil {
		return out, err
	}
	out.TasksPending = pending
	out.TasksDone = done
	if s.report != nil {
		leads, err := s.report.CountCapturedLeads(ctx)
		if err != nil {
			return out, err
		}
		out.LeadsCaptured = leads
	}
	return out, nil
}
