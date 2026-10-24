// Package repository 提供系统监控统计的数据库访问层。
//
// 本文件封装 system_monitor service 中所需的全部统计查询。
// 严格遵循五层架构:Service 层不直接调用 db.GetDB(),必须通过 Repository。
//
// 注意:
//   - 每个读口都显式 WithContext:on 模式下租户绑定就在 ctx 里,丢了 ctx 的
//     那条 query 会被隔离 callback fail-closed 拒掉(旧注释说"ctx 暂未透传",
//     那句话早就与代码不符了)
//   - 不在 Repository 层做业务判断(仅按表聚合)
package repository

import (
	"context"
	"errors"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// SystemStatsRepository 系统统计仓库接口
type SystemStatsRepository interface {
	CountSystemUsers(ctx context.Context) (int64, error)
	CountSystemUsersByRole(ctx context.Context, role string) (int64, error)
	CountActiveSystemUsers(ctx context.Context, sinceUnix int64) (int64, error)
	CountOrders(ctx context.Context) (int64, error)
	CountCards(ctx context.Context) (int64, error)
	CountShortLinks(ctx context.Context) (int64, error)
	CountTodayVisits(ctx context.Context, sinceUnix int64) (int64, error)
	CountEmailLists(ctx context.Context) (int64, error)
	CountEmailJobs(ctx context.Context) (int64, error)
	CountMaterials(ctx context.Context) (int64, error)
	ListRecentSystemMetrics(ctx context.Context, limit int) ([]model.SystemMetrics, error)
}

type systemStatsRepo struct {
	db *gorm.DB
}

// NewSystemStatsRepository 创建系统统计仓库实例
func NewSystemStatsRepository() SystemStatsRepository {
	return &systemStatsRepo{db: _db.GetDB()}
}

// ErrStatsStoreUnavailable 仓储句柄缺失（装配错，不是「没数据」）。
//
// 本仓储没有 WithDB 变体，句柄来自构造那一刻的 db.GetDB()：装配顺序不对时
// 拿到的是 nil，而 WithContext 在 nil *gorm.DB 上直接空指针崩 —— 崩在监控页
// 等于整个后台 500。这里的每个数字本来就只是"尽力而为"的展示值（调用侧
// service/system_monitor.go 一律 `_` 丢弃错误、缺值渲染成 0），所以句柄缺失
// 应当是「这一页的某个数字是 0」，不是「这一页打不开」。
var ErrStatsStoreUnavailable = errors.New("system_stats: 未装配数据库句柄")

// ready 取本次查询要用的句柄。
func (r *systemStatsRepo) ready() (*gorm.DB, error) {
	if r == nil || r.db == nil {
		return nil, ErrStatsStoreUnavailable
	}
	return r.db, nil
}

func (r *systemStatsRepo) CountSystemUsers(ctx context.Context) (int64, error) {
	q, err := r.ready()
	if err != nil {
		return 0, err
	}
	var n int64
	if err := q.WithContext(ctx).Model(&model.SystemUser{}).Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

func (r *systemStatsRepo) CountSystemUsersByRole(ctx context.Context, role string) (int64, error) {
	q, err := r.ready()
	if err != nil {
		return 0, err
	}
	var n int64
	if err := q.WithContext(ctx).Model(&model.SystemUser{}).Where("role = ?", role).Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

func (r *systemStatsRepo) CountActiveSystemUsers(ctx context.Context, sinceUnix int64) (int64, error) {
	q, err := r.ready()
	if err != nil {
		return 0, err
	}
	var n int64
	if err := q.WithContext(ctx).Model(&model.SystemUser{}).Where("updated_at >= to_timestamp(?)", sinceUnix).Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

func (r *systemStatsRepo) CountOrders(ctx context.Context) (int64, error) {
	q, err := r.ready()
	if err != nil {
		return 0, err
	}
	var n int64
	if err := q.WithContext(ctx).Model(&model.Order{}).Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

// CountCards 汇总四个平台的卡片总数。
//
// 单表失败只跳过、不整体报错：四张卡片表是按平台逐个上线的，离线部署里
// 真有可能缺某一张（`relation does not exist`），而「系统监控概览页整页 500」
// 比「少算一个平台的卡片数」糟糕得多。缺表这件事按租户维度看是噪声，
// 所以这里不写日志——每天都在刷的日志只会被忽略。
func (r *systemStatsRepo) CountCards(ctx context.Context) (int64, error) {
	q, err := r.ready()
	if err != nil {
		return 0, err
	}
	var total int64
	cardTables := []string{"douyin_cards", "kuaishou_cards", "xiaohongshu_cards", "xianyu_cards"}
	for _, table := range cardTables {
		var c int64
		if err := q.WithContext(ctx).Table(table).Count(&c).Error; err != nil {
			continue
		}
		total += c
	}
	return total, nil
}

func (r *systemStatsRepo) CountShortLinks(ctx context.Context) (int64, error) {
	q, err := r.ready()
	if err != nil {
		return 0, err
	}
	var n int64
	if err := q.WithContext(ctx).Model(&model.ShortLink{}).Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

func (r *systemStatsRepo) CountTodayVisits(ctx context.Context, sinceUnix int64) (int64, error) {
	q, err := r.ready()
	if err != nil {
		return 0, err
	}
	var n int64
	if err := q.WithContext(ctx).Model(&model.VisitLog{}).Where("created_at >= to_timestamp(?)", sinceUnix).Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

func (r *systemStatsRepo) CountEmailLists(ctx context.Context) (int64, error) {
	q, err := r.ready()
	if err != nil {
		return 0, err
	}
	var n int64
	if err := q.WithContext(ctx).Model(&model.EmailList{}).Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

func (r *systemStatsRepo) CountEmailJobs(ctx context.Context) (int64, error) {
	q, err := r.ready()
	if err != nil {
		return 0, err
	}
	var n int64
	if err := q.WithContext(ctx).Model(&model.EmailJobs{}).Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

func (r *systemStatsRepo) CountMaterials(ctx context.Context) (int64, error) {
	q, err := r.ready()
	if err != nil {
		return 0, err
	}
	var n int64
	if err := q.WithContext(ctx).Table("materials").Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

// ListRecentSystemMetrics 取最近 N 条系统指标。
//
// limit 在仓储侧夹取：GORM 的 `Limit(0)` 等于**不加 limit**，而本表是采集器
// 周期性写入的时序表（早期每分钟一条），一次误传 0 就把整张表读进内存。
func (r *systemStatsRepo) ListRecentSystemMetrics(ctx context.Context, limit int) ([]model.SystemMetrics, error) {
	if limit < 1 {
		limit = 10
	}
	if limit > 200 {
		limit = 200
	}
	q, err := r.ready()
	if err != nil {
		return []model.SystemMetrics{}, err
	}
	var list []model.SystemMetrics
	if err := q.WithContext(ctx).Limit(limit).Order("created_at DESC").Find(&list).Error; err != nil {
		return []model.SystemMetrics{}, err
	}
	return list, nil
}
