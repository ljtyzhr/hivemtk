package repository

import (
	"context"
	"errors"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// ErrSSOIdentityNotFound SSO 身份记录不存在
var ErrSSOIdentityNotFound = errors.New("sso: identity not found")

type SSOIdentityRepository interface {
	Create(ctx context.Context, identity *model.SSOIdentity) error
	GetByProviderSubject(ctx context.Context, provider, subject string) (*model.SSOIdentity, error)
	ListByUserID(ctx context.Context, userID uint) ([]*model.SSOIdentity, error)
	Delete(ctx context.Context, id uint) error
}

type ssoIdentityRepo struct {
	db *gorm.DB
}

// NewSSOIdentityRepository 构造
func NewSSOIdentityRepository(db *gorm.DB) SSOIdentityRepository {
	return &ssoIdentityRepo{db: db}
}

// errSSOIdentityUnavailable 仓储句柄缺失（装配错，不是「没数据」）。
var errSSOIdentityUnavailable = errors.New("sso identity repository not initialized")

func (r *ssoIdentityRepo) Create(ctx context.Context, identity *model.SSOIdentity) error {
	if r == nil || r.db == nil {
		return errSSOIdentityUnavailable
	}
	if identity == nil {
		return errors.New("sso identity: nil 不是写入对象")
	}
	return r.db.WithContext(ctx).Create(identity).Error
}

func (r *ssoIdentityRepo) GetByProviderSubject(ctx context.Context, provider, subject string) (*model.SSOIdentity, error) {
	if r == nil || r.db == nil {
		return nil, errSSOIdentityUnavailable
	}
	var identity model.SSOIdentity
	if err := r.db.WithContext(ctx).
		Where("provider = ? AND subject = ?", provider, subject).
		First(&identity).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSSOIdentityNotFound
		}
		return nil, err
	}
	return &identity, nil
}

// ListByUserID 列出某个用户绑定的全部 SSO 身份（「账号已绑定的第三方登录」）。
//
// 刻意不加 limit：一个用户在一种 provider 上只会有一条（唯一索引
// idx_sso_provider_subject 的物理约束），全量返回不会无界增长；加个上限反而
// 会让「第 6 个绑定」从页面上消失。
func (r *ssoIdentityRepo) ListByUserID(ctx context.Context, userID uint) ([]*model.SSOIdentity, error) {
	if r == nil || r.db == nil {
		return nil, errSSOIdentityUnavailable
	}
	var list []*model.SSOIdentity
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// Delete 解绑一条 SSO 身份。
//
// 0 行仍然翻成领域哨兵 ErrSSOIdentityNotFound（`service/sso.go:403` 靠
// errors.Is 判它来决定"解绑一个已经解过的绑定"该怎么回），但判据本身不再手写：
// 走 db.EnsureAffected，与其余仓储共用同一处口径（pkg/db/write_guard.go）。
func (r *ssoIdentityRepo) Delete(ctx context.Context, id uint) error {
	if r == nil || r.db == nil {
		return errSSOIdentityUnavailable
	}
	err := _db.EnsureAffected(r.db.WithContext(ctx).Delete(&model.SSOIdentity{}, id))
	if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrSSOIdentityNotFound
	}
	return err
}

var _ SSOIdentityRepository = (*ssoIdentityRepo)(nil)
