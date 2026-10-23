package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/model"
)

// BridgeSelectorHealthTableName 选择器健康表名（service 侧打印与自检用）。
const BridgeSelectorHealthTableName = "bridge_selector_health"

// SelectorHealthDelta 一次上报里的单字段增量。
//
// 是**增量**不是快照：扩展把自己的计数差额报上来，服务端累加。
// 这样扩展被浏览器休眠、断网、重装都不会丢历史（丢的只是它自己那一段）。
type SelectorHealthDelta struct {
	Channel  string
	Field    string
	Attempts uint64
	Hits     uint64
	// ReportedAt 扩展侧的采集时刻，服务端用它做「这条增量有没有倒退」的兜底判断
	// （时钟漂移时以服务端时间为准，见 Upsert 内注释）。
	ReportedAt time.Time
}

// BridgeSelectorHealthRepository 选择器健康计数持久化。
type BridgeSelectorHealthRepository interface {
	// Upsert 累加增量。同一 (channel, field) 不存在则建行。
	Upsert(ctx context.Context, deltas []SelectorHealthDelta) error
	// List 列出当前租户全部计数（按渠道 + 字段排序，供运营面直接渲染）。
	List(ctx context.Context) ([]model.BridgeSelectorHealth, error)
}

type bridgeSelectorHealthRepo struct {
	db *gorm.DB
}

// NewBridgeSelectorHealthRepository 构造仓储。db 为 nil 时返回 nil 接口——
// 与仓内其它 repository 同口径（ARC-01：未初始化守卫由调用方用 repo == nil 表达，
// 不在这里塞一个「能创建但永远失败」的空实现）。
func NewBridgeSelectorHealthRepository(db *gorm.DB) BridgeSelectorHealthRepository {
	if db == nil {
		return nil
	}
	return &bridgeSelectorHealthRepo{db: db}
}

func (r *bridgeSelectorHealthRepo) Upsert(ctx context.Context, deltas []SelectorHealthDelta) error {
	if len(deltas) == 0 {
		return nil
	}
	now := time.Now()
	for _, d := range deltas {
		// 先更新后插入，而不是 `ON CONFLICT (tenant_id, channel, field)`：
		// 那条唯一索引由 tenancyidx.Normalize 在迁移期重建（列序与名字是**运维事实**，
		// 不是本文件能假设的东西）；把它写进 ON CONFLICT 的推断里，等于让这张表的写入
		// 依赖「另一个包在迁移期做对了什么」。两条语句的代价可以忽略——健康上报是分钟级。
		res := r.db.WithContext(ctx).Model(&model.BridgeSelectorHealth{}).
			Where("channel = ? AND field = ?", d.Channel, d.Field).
			Updates(map[string]any{
				"attempts":       gorm.Expr("attempts + ?", d.Attempts),
				"hits":           gorm.Expr("hits + ?", d.Hits),
				"last_report_at": now,
				"updated_at":     now,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected > 0 {
			continue
		}
		row := model.BridgeSelectorHealth{
			Channel:      d.Channel,
			Field:        d.Field,
			Attempts:     d.Attempts,
			Hits:         d.Hits,
			FirstSeenAt:  now,
			LastReportAt: now,
		}
		if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
			if isSelectorHealthUniqueViolation(err) {
				// 并发首建：另一路已经插进去了，把增量补加在它那行上。
				if uerr := r.db.WithContext(ctx).Model(&model.BridgeSelectorHealth{}).
					Where("channel = ? AND field = ?", d.Channel, d.Field).
					Updates(map[string]any{
						"attempts":       gorm.Expr("attempts + ?", d.Attempts),
						"hits":           gorm.Expr("hits + ?", d.Hits),
						"last_report_at": now,
						"updated_at":     now,
					}).Error; uerr != nil {
					return uerr
				}
				continue
			}
			return err
		}
	}
	return nil
}

func (r *bridgeSelectorHealthRepo) List(ctx context.Context) ([]model.BridgeSelectorHealth, error) {
	var rows []model.BridgeSelectorHealth
	// 上限与 bridge 账号列表同思路：这张表按 (渠道 × 字段) 收敛，正常规模是十几行；
	// 没有上限时一行坏渠道名就能让运营面一次全表读。
	err := r.db.WithContext(ctx).
		Order("channel asc, field asc").
		Limit(BridgeSelectorHealthListLimit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// BridgeSelectorHealthListLimit 运营面单次读取行数上限。
const BridgeSelectorHealthListLimit = 500

// isSelectorHealthUniqueViolation 判定 PG 23505（并发首建撞唯一键）。
func isSelectorHealthUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "23505") || strings.Contains(msg, "duplicate key value")
}
