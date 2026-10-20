package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"hivemtk-user/internal/browser_automation/model"
)

// BrowserOutreachReceiptRepository 触达回执读写（Chunk5 验收交付物）。
//
// 与去重表的三处分表形状差异记在 `model.BrowserOutreachReceipt` 的文档注释里。对本仓储只有一条硬约束：
// 去重表是闸门（有唯一键、重复插入意为「已经发过了」），回执表是交付物（重复插入意为「多留一份证据」）。
type BrowserOutreachReceiptRepository interface {
	// RecordOutreachReceipt 落一条回执。**刻意不做 OnConflict DoNothing**：
	// 去重表那样做是因为重复行等于重复打扰，这里重复行等于重复交付——吞掉会让
	// 「内容已发出但列表里没有回执」与「这步根本没执行过」无法区分。
	RecordOutreachReceipt(ctx context.Context, rec *model.BrowserOutreachReceipt) error
	// ListByTaskID 任务级回执行（新到旧）。userID=0 不做归属过滤，仅供后台/测试。
	ListByTaskID(ctx context.Context, taskID uint, userID uint, limit int) ([]*model.BrowserOutreachReceipt, error)
}

type browserOutreachReceiptRepository struct {
	db *gorm.DB
}

// NewBrowserOutreachReceiptRepositoryWithDB 显式注入 gormDB（路由装配用，测试可替换）。
//
// 与 #7 同一处收口：这里曾有第二条无参构造 `NewBrowserOutreachReceiptRepository()`（取
// `_db.GetDB()` 全局句柄），装配与测试全走 `...WithDB`，零调用方 ⇒ 已删；本域仓储不再有
// 「不接 db 也能摸库」的后门（#8 四关 4.4）。
func NewBrowserOutreachReceiptRepositoryWithDB(db *gorm.DB) BrowserOutreachReceiptRepository {
	return &browserOutreachReceiptRepository{db: db}
}

func (r *browserOutreachReceiptRepository) RecordOutreachReceipt(ctx context.Context, rec *model.BrowserOutreachReceipt) error {
	if r.db == nil || rec == nil {
		return errors.New("回执仓储未装配")
	}
	return r.db.WithContext(ctx).Create(rec).Error
}

// ListByTaskID 归属过滤：回执行自己没有 user_id 列，归属经 task_id 落到所属任务上再判——
// 否则一个知道 task_id 的用户就能读别人的触达内容。
func (r *browserOutreachReceiptRepository) ListByTaskID(ctx context.Context, taskID uint, userID uint, limit int) ([]*model.BrowserOutreachReceipt, error) {
	if r.db == nil {
		return nil, errors.New("回执仓储未装配")
	}
	q := r.db.WithContext(ctx).Model(&model.BrowserOutreachReceipt{}).Where("task_id = ?", taskID)
	if userID != 0 {
		// 子查询必须带 ctx：`r.db.Model(...)` 拿的是无租户上下文的全局句柄，on 模式下租户 callback
		// 对它 fail-closed ⇒ 子会话 SQL 建为空 ⇒ 渲染成 `task_id IN ()` ⇒ PG 42601 语法错，
		// 任务详情的回执列表整条读路径 500（off 模式直通所以长期没暴露）。
		q = q.Where("task_id IN (?)",
			r.db.WithContext(ctx).Model(&model.BrowserTask{}).Select("id").Where("id = ? AND user_id = ?", taskID, userID))
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var out []*model.BrowserOutreachReceipt
	return out, q.Order("created_at desc, id desc").Limit(limit).Find(&out).Error
}

// 会话级读口（曾有 ListBySessionID）已删：路由、service、前端调用点三处交叉验证为零消费者，
// 触达 P0 Chunk5 定的展示面是**任务详情**回执（docs/superpowers/plans/2026-09-30-outreach-p0.md
// Chunk5），会话侧从来没有要过回执列表。日后若 I5 审计导出要带回执，正确做法是把它加进
// `SessionService.SessionExport` 那个返回元组（元组少一个值调用方编译不过），
// 而不是再摆一个没人读的独立方法。（#8 四关 4.4）
