package repository

import (
	"context"
	"time"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/timeutil"

	"gorm.io/gorm"
)

// LiveCodeQRRepository 活码二维码仓储接口
type LiveCodeQRRepository interface {
	Create(ctx context.Context, qrCode *model.LiveCodeQR) error
	Update(ctx context.Context, qrCode *model.LiveCodeQR) error
	Delete(ctx context.Context, id string) error
	GetByID(ctx context.Context, id string) (*model.LiveCodeQR, error)
	GetByLiveCodeID(ctx context.Context, liveCodeID string) ([]*model.LiveCodeQR, error)
	GetStats(ctx context.Context, qrID string) ([]*model.LiveCodeQRStat, error)
	IncrementViewStat(ctx context.Context, qrID string) error
	IncrementClickStat(ctx context.Context, qrID string) error
	SumStats(ctx context.Context, qrID string) (views int64, clicks int64, err error)
	SumLiveCodeStats(ctx context.Context, liveCodeID string) (views int64, clicks int64, err error)
}

type liveCodeQRRepository struct {
	db *gorm.DB
}

// NewLiveCodeQRRepository 创建活码二维码仓储实例
func NewLiveCodeQRRepository(db *gorm.DB) LiveCodeQRRepository {
	return &liveCodeQRRepository{db: db}
}

func (r *liveCodeQRRepository) Create(ctx context.Context, qrCode *model.LiveCodeQR) error {
	return r.db.WithContext(ctx).Create(qrCode).Error
}

func (r *liveCodeQRRepository) Update(ctx context.Context, qrCode *model.LiveCodeQR) error {
	return _db.SaveWholeRow(r.db.WithContext(ctx), qrCode)
}

func (r *liveCodeQRRepository) Delete(ctx context.Context, id string) error {
	return _db.EnsureAffected(r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.LiveCodeQR{}))
}

func (r *liveCodeQRRepository) GetByID(ctx context.Context, id string) (*model.LiveCodeQR, error) {
	var qrCode model.LiveCodeQR
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&qrCode).Error
	if err != nil {
		return nil, err
	}
	return &qrCode, nil
}

func (r *liveCodeQRRepository) GetByLiveCodeID(ctx context.Context, liveCodeID string) ([]*model.LiveCodeQR, error) {
	var qrCodes []*model.LiveCodeQR
	err := r.db.WithContext(ctx).Where("live_code_id = ?", liveCodeID).Order("created_at DESC").Find(&qrCodes).Error
	if err != nil {
		return nil, err
	}
	return qrCodes, nil
}

func (r *liveCodeQRRepository) GetStats(ctx context.Context, qrID string) ([]*model.LiveCodeQRStat, error) {
	var stats []*model.LiveCodeQRStat
	err := r.db.WithContext(ctx).Where("qr_code_id = ?", qrID).Order("date DESC").Find(&stats).Error
	if err != nil {
		return nil, err
	}
	return stats, nil
}

// incrementStat 给「二维码 × 业务日」那一行的计数列 +1。
//
// 先 UPDATE、命中 0 行才 INSERT：原先的「先 First 再 Create/UpdateColumn」多一次往返，
// 且把"查不到行"写成 `err == gorm.ErrRecordNotFound`（哨兵比较，包一层就失效）。
// 表上没有 (tenant_id, qr_code_id, date) 唯一索引，所以首次并发点击仍可能插出两行日桶；
// 这不影响 SumStats 的合计（SUM 跨行累加），只是日桶重复 —— 没有唯一索引就用不了
// ON CONFLICT，这个窗口记在 model-list.md #116 的观察里。
//
// column 只由本文件两个固定调用点传入（view_count / click_count），不接受外部输入。
func (r *liveCodeQRRepository) incrementStat(ctx context.Context, qrID, column string) error {
	day := timeutil.StartOfBusinessDay(time.Now())
	res := r.db.WithContext(ctx).Model(&model.LiveCodeQRStat{}).
		Where("qr_code_id = ? AND date = ?", qrID, day).
		UpdateColumn(column, gorm.Expr(column+" + 1"))
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected > 0 {
		return nil
	}
	stat := model.LiveCodeQRStat{QRCodeID: qrID, Date: day}
	if column == "view_count" {
		stat.ViewCount = 1
	} else {
		stat.ClickCount = 1
	}
	return r.db.WithContext(ctx).Create(&stat).Error
}

func (r *liveCodeQRRepository) IncrementViewStat(ctx context.Context, qrID string) error {
	return r.incrementStat(ctx, qrID, "view_count")
}

func (r *liveCodeQRRepository) IncrementClickStat(ctx context.Context, qrID string) error {
	return r.incrementStat(ctx, qrID, "click_count")
}

func (r *liveCodeQRRepository) SumStats(ctx context.Context, qrID string) (int64, int64, error) {
	var views, clicks int64
	if err := r.db.WithContext(ctx).Model(&model.LiveCodeQRStat{}).
		Where("qr_code_id = ?", qrID).
		Select("COALESCE(SUM(view_count),0), COALESCE(SUM(click_count),0)").
		Row().Scan(&views, &clicks); err != nil {
		return 0, 0, err
	}
	return views, clicks, nil
}

func (r *liveCodeQRRepository) SumLiveCodeStats(ctx context.Context, liveCodeID string) (int64, int64, error) {
	var views, clicks int64
	// 子查询选 live_code_qrs 的**主键 id**（qr_code_id 是 live_code_qr_stats 的列，
	// live_code_qrs 上没有这一列 —— 选了它 PG 会直接报 column does not exist）。
	if err := r.db.WithContext(ctx).Model(&model.LiveCodeQRStat{}).
		Where("qr_code_id IN (?)",
			r.db.WithContext(ctx).Model(&model.LiveCodeQR{}).
				Select("id").Where("live_code_id = ?", liveCodeID)).
		Select("COALESCE(SUM(view_count),0), COALESCE(SUM(click_count),0)").
		Row().Scan(&views, &clicks); err != nil {
		return 0, 0, err
	}
	return views, clicks, nil
}
