package repository

import (
	"context"
	"fmt"
	"time"

	"hivemtk-user/internal/browser_automation/model"

	"gorm.io/gorm"
)

// BrowserTaskRepository 任务仓储。除 GetByIDAnyUser 外全部强制 userID 过滤，防越权串号。
//
// 方法面收口（实体审查 #12，4.4）：`FindRunning`、`ListDependents`、`ErrTaskNotFound`
// 三个符号在全仓（含测试、含 user-web 的接口调用面）零引用，已删。三条都不是
// 「还没人用但该留」：
//   - `FindRunning(userID)`：用户侧「我的执行中」走的是 `List(userID, status="running")`
//     （带分页与计数，是详情页真正用的那条），对账用的走 `FindStaleRunningAll`——
//     两条真实需求各有出口，这一条夹在中间谁也不接。留着它反而给了「按用户查在途任务」
//     一个绕过分页上限的口子。
//   - `ListDependents(taskID)`：删除任务时的「谁还依赖我」反查。真实现的口径不是反查而是
//     **执行期 fail-closed**：`SoftDelete` 之后前置行读不到，`checkDependency`
//     （task.go:404）直接回「前置任务不可用」，用户看得到明确原因。前端也只有正向的
//     `depends_on_task_id` 编辑器（Editor.vue:417），没有任何反查消费面。
//   - `ErrTaskNotFound`：从未被 `errors.Is` 判过，也从未被抛过——任务读不到一律走
//     `gorm.ErrRecordNotFound`（`SoftDelete` 就是显式回它）。留一个没人抛、没人判的
//     哨兵，等于让下一个人以为存在这条错误契约。
type BrowserTaskRepository interface {
	Create(ctx context.Context, t *model.BrowserTask) error
	GetByID(ctx context.Context, id, userID uint) (*model.BrowserTask, error)
	// GetByIDAnyUser 仅用于依赖检查（读取前置任务的归属/状态），业务读写仍必须走 GetByID
	GetByIDAnyUser(ctx context.Context, id uint) (*model.BrowserTask, error)
	List(ctx context.Context, userID uint, status, taskType string, page, limit int) ([]*model.BrowserTask, int64, error)
	// Update 用户可编辑列的唯一写口（读改写）。执行侧的七列不在它的写面上，见 taskOwnedByExecutorCols。
	Update(ctx context.Context, t *model.BrowserTask) error
	UpdateStatus(ctx context.Context, id uint, status, errMsg string) error
	UpdateRunResult(ctx context.Context, id uint, status, lastResult, errMsg string, retryCount int) error
	SoftDelete(ctx context.Context, id, userID uint) error
	// 对账器（stale_reconcile.go）专用两条，见各自注释
	FindStaleRunningAll(ctx context.Context, minAge time.Duration, limit int) ([]*model.BrowserTask, error)
	ReconcileRunResult(ctx context.Context, id uint, status, lastResult, errMsg string) (bool, error)
	// D4b：重试持久化——scheduleRetry 落 next_retry_at，扫描器条件认领（置 NULL）防双触发
	SetNextRetryAt(ctx context.Context, id uint, at *time.Time) error
	// ownerUserIDs：本进程持有 Host 连接的用户白名单（空=不加此过滤，见 ClaimDueRetries 注释）
	ClaimDueRetries(ctx context.Context, now time.Time, limit int, ownerUserIDs []uint) ([]*model.BrowserTask, error)
	// TryConsumeCampaignActBudget 触达 P0 Chunk4：原子扣减活动触达条数预算。
	// 返回 ok=false 表示「预算已耗尽或未配置预算，调用方不得下发该写步」；err 非空是 DB 故障。
	TryConsumeCampaignActBudget(ctx context.Context, taskID uint) (ok bool, err error)
}

type browserTaskRepo struct {
	db *gorm.DB
}

// NewBrowserTaskRepositoryWithDB 显式注入 gormDB（路由装配用，测试可替换）
func NewBrowserTaskRepositoryWithDB(db *gorm.DB) BrowserTaskRepository {
	return &browserTaskRepo{db: db}
}

func (r *browserTaskRepo) Create(ctx context.Context, t *model.BrowserTask) error {
	return r.db.WithContext(ctx).Create(t).Error
}

func (r *browserTaskRepo) GetByID(ctx context.Context, id, userID uint) (*model.BrowserTask, error) {
	var t model.BrowserTask
	err := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&t).Error
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *browserTaskRepo) GetByIDAnyUser(ctx context.Context, id uint) (*model.BrowserTask, error) {
	var t model.BrowserTask
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&t).Error
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *browserTaskRepo) List(ctx context.Context, userID uint, status, taskType string, page, limit int) ([]*model.BrowserTask, int64, error) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := r.db.WithContext(ctx).Model(&model.BrowserTask{}).Where("user_id = ?", userID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if taskType != "" {
		q = q.Where("task_type = ?", taskType)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []*model.BrowserTask
	if err := q.Order("id DESC").Offset((page - 1) * limit).Limit(limit).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// taskOwnedByExecutorCols 执行侧专用写口认领的列，`Update`(Save) 一律不写。
//
// `TaskService.Update` 的形状是「GetByID 读快照 → mutator 改字段 → Save 写回」，而 Save 写
// **全部**列——于是快照之后落库的窄写会被这次编辑覆回旧值。这七列每一列都有另一个写主：
//   - status / last_run_at / last_result / error_msg / retry_count —— `UpdateStatus`(档位转移)
//     与 `UpdateRunResult`(执行收口) 的产物。retry_count 被覆回快照值就是「重试额度复活」：
//     一个已经烧完 MaxRetryTimes 的任务重新拿到满把重试，而重试会下写步；
//   - next_retry_at —— `SetNextRetryAt` / `ClaimDueRetries` 的产物。快照里挂着、库里已被
//     认领清空，Save 挂回去就是同一轮失败的第二发（防双发只剩写台账一道闸）；
//   - campaign_act_used —— `TryConsumeCampaignActBudget` 的原子条件更新产物。覆回一条额度
//     就是一条计划外的真实触达，而这条口的全部意义正是「裁决权落在库里」。
//
// 窗口是真实存在的：`Update` 只在读到的快照上判 `status != running`，而 failed/paused/ready
// 都既允许编辑、又同时允许执行侧写列（重试腿就跑在 failed 上）。
var taskOwnedByExecutorCols = []string{"status", "last_run_at", "last_result", "error_msg", "retry_count", "next_retry_at", "campaign_act_used"}

// Update 用户可编辑列的唯一写口（含 SetDependency 走的依赖两列）。
// 读改写会带出执行侧列的旧值，所以按 taskOwnedByExecutorCols 收口写面。
func (r *browserTaskRepo) Update(ctx context.Context, t *model.BrowserTask) error {
	return r.db.WithContext(ctx).Omit(taskOwnedByExecutorCols...).Save(t).Error
}

// 三个按主键直写的写口（UpdateStatus/UpdateRunResult/SetNextRetryAt）命中 0 行**必须报错**：
// gorm 对 0 行只回 `Error==nil`，放行就是「界面说得通、库里没发生」。具体形状有两处——
//   - `RunTask` 写 `running` 白写：`FindStaleRunningAll` 按 `status='running'` 筛，
//     这一轮执行从此不在任何对账视野里，而调用方以为已经开始了；
//   - `scheduleRetry` 白写：紧接着的日志是「已挂起…重启不丢」（feedback.go:181），
//     而库里一个字段都没变，那一次重试永远不会来。
//
// 0 行的两种真实成因（行不存在 / 已被软删）都不是「正常情况」，而 `ReconcileRunResult`
// 不在此列：它的 `status='running'` 条件与 bool 返回值本就是「谁先收口谁算」的正常信号。
func (r *browserTaskRepo) UpdateStatus(ctx context.Context, id uint, status, errMsg string) error {
	updates := map[string]any{"status": status, "error_msg": errMsg}
	if status == "running" {
		now := time.Now()
		updates["last_run_at"] = &now
	}
	res := r.db.WithContext(ctx).Model(&model.BrowserTask{}).Where("id = ?", id).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("任务状态写入命中 0 行（task=%d → %s）：该行不存在或已被软删，这一轮状态转移没有落库", id, status)
	}
	return nil
}

func (r *browserTaskRepo) UpdateRunResult(ctx context.Context, id uint, status, lastResult, errMsg string, retryCount int) error {
	now := time.Now()
	res := r.db.WithContext(ctx).Model(&model.BrowserTask{}).Where("id = ?", id).Updates(map[string]any{
		"status":      status,
		"last_run_at": &now,
		"last_result": lastResult,
		"error_msg":   errMsg,
		"retry_count": retryCount,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("任务执行快照写入命中 0 行（task=%d → %s）：该行不存在或已被软删，对账器不会补收敛这一轮", id, status)
	}
	return nil
}

func (r *browserTaskRepo) SoftDelete(ctx context.Context, id, userID uint) error {
	res := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).Delete(&model.BrowserTask{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// FindStaleRunningAll 对账器专用：跨用户扫「running 且 updated_at 老于 minAge」的任务。
// 与用户侧的 `List(userID, status="running")` 分列而非复用：那条带 userID 与分页语义，
// 对账是全库后台职责，混用会让「加个 userID 参数」变成越权读取的入口。
// minAge 只是粗筛下限，
// 每任务真实预算（含 D7 确认等待）由调用方按 taskExecBudget 判。
func (r *browserTaskRepo) FindStaleRunningAll(ctx context.Context, minAge time.Duration, limit int) ([]*model.BrowserTask, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	var list []*model.BrowserTask
	err := r.db.WithContext(ctx).
		Where("status = ? AND deleted_at IS NULL AND updated_at <= ?", "running", time.Now().Add(-minAge)).
		Order("id ASC").Limit(limit).Find(&list).Error
	return list, err
}

// ReconcileRunResult 对账回填：仅当任务仍是 running 时写终态，返回是否由本次回填生效。
// 条件 WHERE 不可省——对账器与执行协程可能同刻收口，无条件更新会把刚落下的真实终态盖成对账值。
func (r *browserTaskRepo) ReconcileRunResult(ctx context.Context, id uint, status, lastResult, errMsg string) (bool, error) {
	res := r.db.WithContext(ctx).Model(&model.BrowserTask{}).
		Where("id = ? AND status = ?", id, "running").
		Updates(map[string]any{"status": status, "last_result": lastResult, "error_msg": errMsg})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

// SetNextRetryAt D4b：设置/清除任务的重试到期时间（nil=取消挂起重试）。
// 命中 0 行报错——挂起与取消都是「只有落了库才成立」的声明，
// 调用方紧接着要说「已挂起，重启不丢」，白写就是把一次不会发生的调度说成了已安排。
func (r *browserTaskRepo) SetNextRetryAt(ctx context.Context, id uint, at *time.Time) error {
	res := r.db.WithContext(ctx).Model(&model.BrowserTask{}).Where("id = ?", id).
		Update("next_retry_at", at)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		verb := "挂起"
		if at == nil {
			verb = "取消"
		}
		return fmt.Errorf("重试%s写入命中 0 行（task=%d at=%v）：该行不存在或已被软删，这一次调度不会发生", verb, id, at)
	}
	return nil
}

// ClaimDueRetries D4b：原子认领到期重试——条件更新置 NULL，多副本同库仅一方 RowsAffected=1；
// 认领成功后进程崩溃则重试丢失（与改造前语义相同），但重启不再丢挂起重试。
//
// 归属门：Host 连接是**进程内**状态（registry.conns），挂起重试却是**库内**共享队列。
// 无门时任一实例都能认领任一用户的重试，然后在自己空空的 registry 上判「browser host
// 未连接」，把 MaxRetryTimes 的额度烧在一次根本不可能执行的认领上（真机验证：task=377
// session=429 由另一实例认领，而本机 Host 全程在线）。ownerUserIDs 即本进程持有连接的用户
// 白名单。**空=不加过滤**：装配遗漏时退化成改造前行为，也不要静默停掉全部重试
// （调用方若要表达「本机无人」，应自行不调用，见 feedback.scanDueRetries）。
//
// 状态门：挂起重试只在**仍处于失败态**时才有意义。next_retry_at 是失败时写下的，
// 之后行的状态可以走到任何一处，而这四条路都不会回头清这个字段：
// done —— 用户手工重跑成功了，RunTask 明确放行 done 态执行，扫描器到点就把一条已经成功
// 的任务再跑一遍（含写步，防双发只剩台账这一道，等于把「成功即终止」推翻）；
// paused —— 同样被 RunTask 放行（它是 Resume 的合法前态），于是「用户按了暂停」被后台
// 重试悄悄解除；archived / running —— RunTask 会拒，但认领已经把字段置空，挂起被无声吞掉。
// 认领条件与条件更新两处都带 status='failed'：只挑一处会留下 Pluck→Update 之间用户正好
// 归档/重跑的竞态窗口（也正因如此，单独摘掉任一处都测不出红，两处一起摘才红——
// 与 deleted_at 条件同性质，见 retry_claim_b9_test.go）。软删行另有 deleted_at 条件兜。
func (r *browserTaskRepo) ClaimDueRetries(ctx context.Context, now time.Time, limit int, ownerUserIDs []uint) ([]*model.BrowserTask, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	ids := []uint{}
	q := r.db.WithContext(ctx).Model(&model.BrowserTask{}).
		Where("next_retry_at IS NOT NULL AND next_retry_at <= ? AND deleted_at IS NULL AND status = ?", now, "failed")
	if len(ownerUserIDs) > 0 {
		q = q.Where("user_id IN ?", ownerUserIDs)
	}
	err := q.Order("next_retry_at ASC").Limit(limit).Pluck("id", &ids).Error
	if err != nil || len(ids) == 0 {
		return nil, err
	}
	claimed := make([]uint, 0, len(ids))
	for _, id := range ids {
		res := r.db.WithContext(ctx).Model(&model.BrowserTask{}).
			Where("id = ? AND next_retry_at IS NOT NULL AND next_retry_at <= ? AND status = ?", id, now, "failed").
			Update("next_retry_at", nil)
		if res.Error == nil && res.RowsAffected == 1 {
			claimed = append(claimed, id)
		}
	}
	if len(claimed) == 0 {
		return nil, nil
	}
	var list []*model.BrowserTask
	err = r.db.WithContext(ctx).Where("id IN ?", claimed).Find(&list).Error
	return list, err
}

// TryConsumeCampaignActBudget 触达 P0 Chunk4：活动触达条数预算的原子扣减。
//
// 条件更新（`used < budget` 写进 WHERE）而不是「先 SELECT 判额度、再 UPDATE」：
// 后者在多副本/多 session 并发下是经典的 check-then-act 竞态——两条腿同时读到
// used=9,budget=10，于是都判「还有 1 条」，最后写出 used=11。预算超发一次，
// 意味着一条计划外的触达真的被下发出去（不可逆动作），所以裁决权必须落在库里。
//
// 条件里的三条门各自有理由：
//   - campaign_key <> ”：未编入活动的任务不受活动预算约束（活动键是显式配置，不是默认全员参与）
//   - campaign_act_budget > 0：0 = 不限。不限时既不扣减也不拦停——存量行为必须逐字不变
//   - campaign_act_used < campaign_act_budget：额度判据，RowsAffected=0 即耗尽
//
// 返回 ok=false 有两种成因（调用方对外表现一致：预算耗尽，不许下发）：耗尽，或未配置预算。
// 两者刻意不区分，也**不再回查一次库**去说清是哪一种——回查只为日志，而不限任务每条写步都会
// 白付一次 SELECT。要判成因该在服务层按 task 快照判断，那里本来就有值。
// err 非空才是真故障（连接池/DDL 问题），调用方须与 ok=false 区别对待。
func (r *browserTaskRepo) TryConsumeCampaignActBudget(ctx context.Context, taskID uint) (bool, error) {
	res := r.db.WithContext(ctx).Model(&model.BrowserTask{}).
		Where("id = ? AND deleted_at IS NULL AND campaign_key <> '' AND campaign_act_budget > 0 AND campaign_act_used < campaign_act_budget",
			taskID).
		Update("campaign_act_used", gorm.Expr("campaign_act_used + 1"))
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}
