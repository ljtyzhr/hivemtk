package repository

import (
	"context"
	"fmt"

	"hivemtk-user/internal/browser_automation/model"

	"gorm.io/gorm"
)

// BrowserStepRepository 步骤仓储
//
// 方法面收口（实体审查 #11，4.4）：原有 `ListBySessionIDAnyUser`、`UpdateStatus`、
// `DeleteBySessionID` 三个方法在全仓（含测试）零引用，已删。三条各自都是一张会误导人的门牌：
//   - `ListBySessionIDAnyUser` 是 `ListBySessionID` 的别名版：逐字同实现，注释声称
//     「供归属校验后的内部流程使用」，但两者都不带 user 过滤——名字承诺的区别并不存在，
//     留着就是让下一个读者以为「AnyUser 那条是内部专用、可以不做归属校验」。
//     实际读家只有一条：`SessionService.ListSteps` 先 `GetByID(sessionID, userID)` 判归属，
//     再调 `ListBySessionID`。
//   - `UpdateStatus` 只写 status/error_msg 两列，而步终态的写入方一律同时要写 result 与
//     duration（`UpdateResult`，executor.go:172 是唯一写家）。留着它等于摆一条
//     「可以只改状态不改时长/结果」的旁路，把 G10 那条「终态三件套同刻落库」的口径拆开口子。
//   - `DeleteBySessionID`：步骤行与 command_log 同为 append-only 凭据，任务删除走的是
//     会话收敛（`convergeInFlightSessions`），全仓没有任何删步路径。留着就是给「顺手清一下
//     某会话的步骤」留门，而那一刀删掉的是审计链的一半。
type BrowserStepRepository interface {
	BatchCreate(ctx context.Context, steps []*model.BrowserStep) error
	ListBySessionID(ctx context.Context, sessionID uint) ([]*model.BrowserStep, error)
	// UpdateResult 步终态的唯一写入口：status/result/duration/error_msg 同刻落库，
	// 命中 0 行报错（终态丢失没有兜底收敛器，见实现注释）。
	UpdateResult(ctx context.Context, id uint, status string, result []byte, durationMs int64, errMsg string) error
	// UpdateSubmitState 写台账状态转移：只写 submit_state/text_hash 两列，
	// 与 UpdateResult 分道——步终态（status）由步收口写，提交归因由台账写，两者在
	// 「已提交但验证未见」时必然不同值，合成一列就会把结果未知态当成可重发。
	UpdateSubmitState(ctx context.Context, id uint, state, textHash string) error
	// FindSubmitAttempt 跨 session 查「同任务、同文本」是否已存在提交尝试
	// （sent/verified/unattributed 三态都算尝试过；prepared 不算——点击从未发生）。
	// 无尝试返回 gorm.ErrRecordNotFound。
	// 键里不再有 step_index：Brain 模式的步下标每轮递增，带下标的键会让同一条
	// 评论在换轮重放时落到不同下标上而漏闸。
	FindSubmitAttempt(ctx context.Context, taskID uint, textHash string, excludeID uint) (*model.BrowserStep, error)
	// SubmitStateOf 读单行当前的台账态（/ A12：写步收尾时要判「这一步到底留下尝试凭据没有」，
	// 以此决定存储层声明是留是腾）。行不存在返回 gorm.ErrRecordNotFound——调用方按「读不到」
	// 保守处理，不许当成「没记过」。
	SubmitStateOf(ctx context.Context, id uint) (string, error)
}

// StepResultJSON result 列的 JSON 载荷
type StepResultJSON = []byte

type browserStepRepo struct {
	db *gorm.DB
}

// NewBrowserStepRepositoryWithDB 显式注入 gormDB（路由装配用，测试可替换）
func NewBrowserStepRepositoryWithDB(db *gorm.DB) BrowserStepRepository {
	return &browserStepRepo{db: db}
}

func (r *browserStepRepo) BatchCreate(ctx context.Context, steps []*model.BrowserStep) error {
	if len(steps) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&steps).Error
}

func (r *browserStepRepo) ListBySessionID(ctx context.Context, sessionID uint) ([]*model.BrowserStep, error) {
	var list []*model.BrowserStep
	err := r.db.WithContext(ctx).Where("session_id = ?", sessionID).Order("step_index ASC").Find(&list).Error
	return list, err
}

// UpdateResult 落步终态。0 行受影响**必须报错**（口径同下方 UpdateSubmitState）：
// `finishStep` 把「落库失败」当错误上报，而 gorm 对 0 行只回 Error==nil——放行就等于
// 这一步永远停在 running 且日志里一行红都没有。session/task 行有 stale_reconcile 兜底收敛，
// 步行没有任何兜底，这里就是终态丢失的唯一无声出口。
func (r *browserStepRepo) UpdateResult(ctx context.Context, id uint, status string, result StepResultJSON, durationMs int64, errMsg string) error {
	updates := map[string]any{
		"status":      status,
		"duration_ms": durationMs,
		"error_msg":   errMsg,
	}
	if len(result) > 0 {
		updates["result"] = result
	}
	res := r.db.WithContext(ctx).Model(&model.BrowserStep{}).Where("id = ?", id).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("步终态落库命中 0 行（step=%d status=%s）：该行不存在、已软删或不属于当前租户，面板将永远看不到这一步的结论", id, status)
	}
	return nil
}

// UpdateSubmitState 写台账。0 行受影响**必须报错**：本方法是「重发闸门的唯一事实来源」的
// 唯一写入口，而 gorm 对 0 行只回 Error==nil。两种真实形态都会命中 0 行——
// 行不存在（上层拿到过另一个库的 id）与行已软删（BrowserStep 带 DeletedAt，gorm 自动加
// deleted_at IS NULL；同一条件也让 FindSubmitAttempt 查不到它）。放行即「已记为尝试」
// 变成一句谎话，下一轮闸门查空 ⇒ 双发。口径同 task.go:125 / cron.go:101 的条件更新。
func (r *browserStepRepo) UpdateSubmitState(ctx context.Context, id uint, state, textHash string) error {
	res := r.db.WithContext(ctx).Model(&model.BrowserStep{}).Where("id = ?", id).
		Updates(map[string]any{"submit_state": state, "text_hash": textHash})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("台账写入命中 0 行（step=%d state=%s）：该行不存在或已被软删，闸门查不到这次提交", id, state)
	}
	return nil
}

func (r *browserStepRepo) FindSubmitAttempt(ctx context.Context, taskID uint, textHash string, excludeID uint) (*model.BrowserStep, error) {
	if textHash == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var row model.BrowserStep
	q := r.db.WithContext(ctx).Where("task_id = ? AND text_hash = ? AND submit_state IN ?",
		taskID, textHash, model.StepSubmitAttemptedStates())
	if excludeID > 0 {
		q = q.Where("id <> ?", excludeID)
	}
	if err := q.Order("id asc").First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

// SubmitStateOf 只读 submit_state 一列。独立零值 struct（不复用调用方手里那行）：
// 复用一个已填充的 struct 再 First() 会把旧字段并进 WHERE，读出来是一条 record not found 假红。
func (r *browserStepRepo) SubmitStateOf(ctx context.Context, id uint) (string, error) {
	var row model.BrowserStep
	if err := r.db.WithContext(ctx).Model(&model.BrowserStep{}).Where("id = ?", id).
		Select("submit_state").First(&row).Error; err != nil {
		return "", err
	}
	return row.SubmitState, nil
}
