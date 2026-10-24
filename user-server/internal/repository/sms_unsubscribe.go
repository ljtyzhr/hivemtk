package repository

import (
	"context"
	"errors"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// SmsUnsubscribeRepository 短信退订仓库接口
type SmsUnsubscribeRepository interface {
	Create(ctx context.Context, record *model.SmsUnsubscribe) error
	Update(ctx context.Context, record *model.SmsUnsubscribe) error
	GetByPhone(ctx context.Context, phone string) (*model.SmsUnsubscribe, error)
	ExistsByPhone(ctx context.Context, phone string) (bool, error)
	DeleteByPhone(ctx context.Context, phone string) error
	List(ctx context.Context, page, limit int, keyword string) ([]*model.SmsUnsubscribe, int64, error)
	ListAll(ctx context.Context) ([]*model.SmsUnsubscribe, error)
}

type smsUnsubscribeRepo struct {
	db *gorm.DB
}

// NewSmsUnsubscribeRepository 创建短信退订仓库
func NewSmsUnsubscribeRepository(db *gorm.DB) SmsUnsubscribeRepository {
	if db == nil {
		db = _db.GetDB()
	}
	return &smsUnsubscribeRepo{db: db}
}

// errSmsUnsubscribeUnavailable 仓储句柄缺失（装配错，不是「没数据」）。
var errSmsUnsubscribeUnavailable = errors.New("sms unsubscribe repository not initialized")

func (r *smsUnsubscribeRepo) Create(ctx context.Context, record *model.SmsUnsubscribe) error {
	if r == nil || r.db == nil {
		return errSmsUnsubscribeUnavailable
	}
	if record == nil {
		return errors.New("sms unsubscribe: nil 不是写入对象")
	}
	return r.db.WithContext(ctx).Create(record).Error
}

// Update 整行回写一条退订记录。
//
// 走 `db.SaveWholeRow`：裸 Save 命中 0 行会回退成 `INSERT ... ON CONFLICT (id)
// DO UPDATE` —— 把「读完记录、写回之前被删掉」的那条按原 id 复活。退订是
// 「客户说不许再发」的意思表示，复活一条被人工清除的退订记录意味着**继续给
// 一个要求停发的人发短信**，这不是显示问题。
func (r *smsUnsubscribeRepo) Update(ctx context.Context, record *model.SmsUnsubscribe) error {
	if r == nil || r.db == nil {
		return errSmsUnsubscribeUnavailable
	}
	if record == nil {
		return errors.New("sms unsubscribe: nil 不是更新对象")
	}
	return _db.SaveWholeRow(r.db.WithContext(ctx), record)
}

func (r *smsUnsubscribeRepo) GetByPhone(ctx context.Context, phone string) (*model.SmsUnsubscribe, error) {
	if r == nil || r.db == nil {
		return nil, errSmsUnsubscribeUnavailable
	}
	var record model.SmsUnsubscribe
	err := r.db.WithContext(ctx).Where("phone = ?", phone).First(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

func (r *smsUnsubscribeRepo) ExistsByPhone(ctx context.Context, phone string) (bool, error) {
	if r == nil || r.db == nil {
		return false, errSmsUnsubscribeUnavailable
	}
	var count int64
	err := r.db.WithContext(ctx).Model(&model.SmsUnsubscribe{}).Where("phone = ?", phone).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// DeleteByPhone 清除某号码的退订记录（人工复核后恢复发送）。
//
// 0 行 ⇒ ErrRecordNotFound：这个号码本来就不在退订表里。静默回成功会让
// 「我以为我清掉了」变成一次没有留痕的空操作 —— 而它的后果是下一条短信
// 仍然被拦，或者（更糟）有人据此认为拦截逻辑失效而去关掉整条检查。
func (r *smsUnsubscribeRepo) DeleteByPhone(ctx context.Context, phone string) error {
	if r == nil || r.db == nil {
		return errSmsUnsubscribeUnavailable
	}
	return _db.EnsureAffected(r.db.WithContext(ctx).Where("phone = ?", phone).Delete(&model.SmsUnsubscribe{}))
}

func (r *smsUnsubscribeRepo) List(ctx context.Context, page, limit int, keyword string) ([]*model.SmsUnsubscribe, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, errSmsUnsubscribeUnavailable
	}
	// page = 0 ⇒ offset 为负 ⇒ PG 报 "OFFSET must not be negative"；
	// limit = 0 ⇒ GORM 的 Limit(0) 等于**不加 limit**（直连 HTTP 管理面）。
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}

	// Count 与 Find 必须各起一条 query（见 base()），不能在同一条链上先 Count 再 Find。
	base := func() *gorm.DB {
		q := r.db.WithContext(ctx).Model(&model.SmsUnsubscribe{})
		if keyword != "" {
			like := "%" + keyword + "%"
			q = q.Where("phone LIKE ? OR reason LIKE ? OR keyword_matched LIKE ?", like, like, like)
		}
		return q
	}

	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var records []*model.SmsUnsubscribe
	offset := (page - 1) * limit
	if err := base().Order("unsubscribed_at DESC").Offset(offset).Limit(limit).Find(&records).Error; err != nil {
		return nil, 0, err
	}
	return records, total, nil
}

// ListAll 取全部退订号码（不分页）。
//
// 刻意保留无界读：调用方是「发送前查这个号码有没有退订」这类全量比对
// （`do_not_contact` / 发送拦截），被截断的退订名单会让本应停发的号码继续收到
// 短信 —— 这是唯一一种「读少了比读多了更危险」的清单。量的上限由退订表本身
// 兜住（真正的退订率是个位百分比，不是每天全量）。
func (r *smsUnsubscribeRepo) ListAll(ctx context.Context) ([]*model.SmsUnsubscribe, error) {
	if r == nil || r.db == nil {
		return nil, errSmsUnsubscribeUnavailable
	}
	var records []*model.SmsUnsubscribe
	if err := r.db.WithContext(ctx).Order("unsubscribed_at DESC").Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}
