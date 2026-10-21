package repository

import (
	"context"
	"time"

	"hivemtk-user/internal/browser_automation/model"

	"gorm.io/gorm"
)

// BrowserCronTriggerRepository 定时触发器仓储
type BrowserCronTriggerRepository interface {
	Create(ctx context.Context, t *model.BrowserCronTrigger) error
	GetByID(ctx context.Context, id, userID uint) (*model.BrowserCronTrigger, error)
	GetByTaskID(ctx context.Context, taskID uint) (*model.BrowserCronTrigger, error)
	ListByUser(ctx context.Context, userID uint) ([]*model.BrowserCronTrigger, error)
	ListAllEnabled(ctx context.Context) ([]*model.BrowserCronTrigger, error)
	Update(ctx context.Context, t *model.BrowserCronTrigger) error
	UpdateEnabled(ctx context.Context, id uint, enabled bool) error
	UpdateTimes(ctx context.Context, id uint, nextRunAt, lastRunAt *time.Time) error
	Delete(ctx context.Context, id, userID uint) error
	// DeleteByTaskID 宿主任务被删除或类型改成非 cron 时的回收：按 task_id 删行，没有行不算失败。
	DeleteByTaskID(ctx context.Context, taskID uint) error
}

type browserCronTriggerRepo struct {
	db *gorm.DB
}

// NewBrowserCronTriggerRepositoryWithDB 显式注入 gormDB（路由装配用，测试可替换）
func NewBrowserCronTriggerRepositoryWithDB(db *gorm.DB) BrowserCronTriggerRepository {
	return &browserCronTriggerRepo{db: db}
}

func (r *browserCronTriggerRepo) Create(ctx context.Context, t *model.BrowserCronTrigger) error {
	return r.db.WithContext(ctx).Create(t).Error
}

func (r *browserCronTriggerRepo) GetByID(ctx context.Context, id, userID uint) (*model.BrowserCronTrigger, error) {
	var t model.BrowserCronTrigger
	err := r.db.WithContext(ctx).
		Joins("JOIN browser_tasks ON browser_tasks.id = browser_cron_triggers.task_id AND browser_tasks.user_id = ?", userID).
		Where("browser_cron_triggers.id = ?", id).
		First(&t).Error
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *browserCronTriggerRepo) GetByTaskID(ctx context.Context, taskID uint) (*model.BrowserCronTrigger, error) {
	var t model.BrowserCronTrigger
	err := r.db.WithContext(ctx).Where("task_id = ?", taskID).First(&t).Error
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *browserCronTriggerRepo) ListByUser(ctx context.Context, userID uint) ([]*model.BrowserCronTrigger, error) {
	var list []*model.BrowserCronTrigger
	err := r.db.WithContext(ctx).
		Joins("JOIN browser_tasks ON browser_tasks.id = browser_cron_triggers.task_id AND browser_tasks.user_id = ?", userID).
		Order("browser_cron_triggers.id DESC").
		Find(&list).Error
	return list, err
}

func (r *browserCronTriggerRepo) ListAllEnabled(ctx context.Context) ([]*model.BrowserCronTrigger, error) {
	var list []*model.BrowserCronTrigger
	err := r.db.WithContext(ctx).Where("enabled = ?", true).Find(&list).Error
	return list, err
}

// cronSchedulerOwnedCols 调度记账列，`Update`(Save) 一律不写。
//
// `CronService.Update` 的形状是「GetByID 读快照 → mutator 改 cron_expr/time_zone →
// Save 写回」，而 Save 写**全部**列 —— 于是快照之后落库的窄写会被这次编辑覆回旧值。
// 这两列每一列都有另一个写主：
//   - next_run_at —— applySchedule→storeNextRun 与触发回调 UpdateTimes 的产物。
//     覆回一条旧时刻 = 列表页显示的「下次触发」与实际排期不符；
//   - last_run_at —— 触发回调 UpdateTimes 的产物。覆回旧值 = 列表页那一列
//     「上次触发」比实际早一次，而它恰恰是运维判断「这个定时器还活着吗」的那一列。
//
// 窗口真实存在：改表达式是 HTTP 入口，触发回调是调度器 goroutine，两者并发。
// 与 browser_tasks 的 taskOwnedByExecutorCols 同一修法：写面收口到用户可编辑列。
var cronSchedulerOwnedCols = []string{"next_run_at", "last_run_at"}

// Update 用户可编辑列的唯一写口（cron_expr / time_zone）。
// 读改写会带出调度记账列的旧值，所以按 cronSchedulerOwnedCols 收口写面。
func (r *browserCronTriggerRepo) Update(ctx context.Context, t *model.BrowserCronTrigger) error {
	return r.db.WithContext(ctx).Omit(cronSchedulerOwnedCols...).Save(t).Error
}

func (r *browserCronTriggerRepo) UpdateEnabled(ctx context.Context, id uint, enabled bool) error {
	return r.db.WithContext(ctx).Model(&model.BrowserCronTrigger{}).Where("id = ?", id).
		Update("enabled", enabled).Error
}

// UpdateTimes 成对写这两个时刻；nil = 写 NULL（不是「不改」——map 形态的 Updates 不走零值跳过，
// 调用方要保住的值自己读回来传，见 service.storeNextRun）。
func (r *browserCronTriggerRepo) UpdateTimes(ctx context.Context, id uint, nextRunAt, lastRunAt *time.Time) error {
	return r.db.WithContext(ctx).Model(&model.BrowserCronTrigger{}).Where("id = ?", id).
		Updates(map[string]any{"next_run_at": nextRunAt, "last_run_at": lastRunAt}).Error
}

// Delete 硬删，不是软删：(tenant_id, task_id) 上的唯一键位必须由删除释放。
//
// 软删的行仍占着键位，而 service 的重名预检查（GetByTaskID）带 deleted_at IS NULL，
// 读不到那行于是放行，紧接着 INSERT 撞约束 ⇒ 用户「删掉再建一个」（改时间的常规走法）
// 必然拿到 500。触发行没有任何历史读家（审计在 browser_command_log / browser_steps），
// 留一个占位的死行只换来故障。
func (r *browserCronTriggerRepo) Delete(ctx context.Context, id, userID uint) error {
	res := r.db.WithContext(ctx).Unscoped().
		Joins("JOIN browser_tasks ON browser_tasks.id = browser_cron_triggers.task_id AND browser_tasks.user_id = ?", userID).
		Where("browser_cron_triggers.id = ?", id).
		Delete(&model.BrowserCronTrigger{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// DeleteByTaskID 不判 RowsAffected：调用方问的是「这条任务的定时器别再响」，
// 「它本来就没有触发器」是同一种期望结果，不是错误。
// 同样硬删（键位理由见 Delete）。
func (r *browserCronTriggerRepo) DeleteByTaskID(ctx context.Context, taskID uint) error {
	return r.db.WithContext(ctx).Unscoped().Where("task_id = ?", taskID).
		Delete(&model.BrowserCronTrigger{}).Error
}
