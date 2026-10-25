package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"hivemtk-user/internal/model"
	_pagination "hivemtk-user/internal/pkg/pagination"

	"gorm.io/gorm"
)

type SecurityAuditRepository struct {
	db *gorm.DB
}

func NewSecurityAuditRepository() *SecurityAuditRepository {
	return &SecurityAuditRepository{}
}

func (r *SecurityAuditRepository) SetDB(ctx context.Context, db *gorm.DB) {
	if db != nil {
		r.db = db
	}
}

func (r *SecurityAuditRepository) GetDB(ctx context.Context) *gorm.DB {
	if r.db == nil {
		return nil
	}
	return r.db.WithContext(ctx)
}

// errSecurityAuditUnavailable 仓储句柄缺失（两段式构造：db 由装配层 SetDB 后注入）。
var errSecurityAuditUnavailable = errors.New("security audit repository not initialized")

func (r *SecurityAuditRepository) Create(ctx context.Context, a *model.SecurityAudit) error {
	if r == nil || r.db == nil {
		return errSecurityAuditUnavailable
	}
	return r.db.WithContext(ctx).Create(a).Error
}

func (r *SecurityAuditRepository) List(ctx context.Context, page, pageSize int) ([]model.SecurityAudit, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, errSecurityAuditUnavailable
	}
	// page = 0 ⇒ 负 offset ⇒ PG 报错；pageSize = 0 ⇒ Limit(0) 等于不加 limit。
	if page < 1 {
		page = 1
	}
	pageSize = _pagination.ClampLimit(pageSize)

	var total int64
	if err := r.db.WithContext(ctx).Model(&model.SecurityAudit{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []model.SecurityAudit
	offset := (page - 1) * pageSize
	if err := r.db.WithContext(ctx).Order("id DESC").Offset(offset).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// ListKeyset 使用 keyset（cursor）分页查询审计记录。
// 游标复用 pagination.EncodeCursor（<unix_nano>:<id>），排序 created_at DESC, id DESC，
// 多取 1 条判断是否还有下一页。
func (r *SecurityAuditRepository) ListKeyset(ctx context.Context, cursor string, pageSize int) ([]model.SecurityAudit, int64, string, error) {
	if r.db == nil {
		return nil, 0, "", errors.New("security audit repository not initialized")
	}
	// 上界要守：本口直连 HTTP 管理面，超大 page_size 会一次拖出大量带 payload 的行。
	if pageSize <= 0 {
		pageSize = 10
	}
	pageSize = _pagination.ClampLimit(pageSize)

	var total int64
	if err := r.db.WithContext(ctx).Model(&model.SecurityAudit{}).Count(&total).Error; err != nil {
		return nil, 0, "", err
	}

	q := r.db.WithContext(ctx).Model(&model.SecurityAudit{})
	if cursor != "" {
		ts, id, ok := _pagination.DecodeCursor(_pagination.Cursor(cursor))
		if !ok {
			return nil, 0, "", fmt.Errorf("invalid cursor: %s", cursor)
		}
		q = q.Where("(created_at, id) < (?, ?)", ts, id)
	}

	var list []model.SecurityAudit
	if err := q.Order("created_at DESC, id DESC").Limit(pageSize + 1).Find(&list).Error; err != nil {
		return nil, 0, "", err
	}

	nextCursor := ""
	if len(list) > pageSize {
		last := list[pageSize-1]
		list = list[:pageSize]
		nextCursor = string(_pagination.EncodeCursor(last.CreatedAt, uint64(last.ID)))
	}

	return list, total, nextCursor, nil
}

func (r *SecurityAuditRepository) GetByID(ctx context.Context, id uint) (*model.SecurityAudit, error) {
	if r == nil || r.db == nil {
		return nil, errSecurityAuditUnavailable
	}
	var a model.SecurityAudit
	if err := r.db.WithContext(ctx).Preload("Items").First(&a, id).Error; err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *SecurityAuditRepository) PingDB(ctx context.Context) error {
	if r == nil || r.db == nil {
		return errors.New("security audit repository not initialized")
	}
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

func (r *SecurityAuditRepository) CountSystemUserByRole(ctx context.Context, role string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("security audit repository not initialized")
	}
	var cnt int64
	if err := r.db.WithContext(ctx).Model(&model.SystemUser{}).
		Where("role = ?", role).Count(&cnt).Error; err != nil {
		return 0, err
	}
	return cnt, nil
}

func (r *SecurityAuditRepository) FirstSystemUserByRole(ctx context.Context, role string) (*model.SystemUser, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("security audit repository not initialized")
	}
	var u model.SystemUser
	if err := r.db.WithContext(ctx).Where("role = ?", role).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *SecurityAuditRepository) CountEnabledLLMProviders(ctx context.Context) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("security audit repository not initialized")
	}
	var cnt int64
	if err := r.db.WithContext(ctx).Model(&model.LLMProvider{}).Where("enabled = ?", true).Count(&cnt).Error; err != nil {
		return 0, err
	}
	return cnt, nil
}

func (r *SecurityAuditRepository) ListDomainPoolDomains(ctx context.Context) ([]string, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("security audit repository not initialized")
	}
	var rows []model.DomainPool
	if err := r.db.WithContext(ctx).Model(&model.DomainPool{}).
		Select("domain").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Domain)
	}
	return out, nil
}

func (r *SecurityAuditRepository) ListShortLinkOriginalURLs(ctx context.Context) ([]string, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("security audit repository not initialized")
	}
	var rows []model.ShortLink
	if err := r.db.WithContext(ctx).Select("original_url").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.OriginalURL)
	}
	return out, nil
}

func (r *SecurityAuditRepository) ListLiveCodeOutboundURLs(ctx context.Context) ([]string, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("security audit repository not initialized")
	}
	var rows []model.LiveCode
	if err := r.db.WithContext(ctx).Select("entry_url, landing_url").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows)*2)
	for _, r := range rows {
		if r.EntryURL != "" {
			out = append(out, r.EntryURL)
		}
		if r.LandingURL != "" {
			out = append(out, r.LandingURL)
		}
	}
	return out, nil
}

// ---- C-5 安全总览 / 弱密码抽样探测的取数面 ----

// CountLoginEventsSince 统计 since 之后的登录事件总数与高危数（overview 用）。
func (r *SecurityAuditRepository) CountLoginEventsSince(ctx context.Context, since time.Time) (total, high int64, err error) {
	if r == nil || r.db == nil {
		return 0, 0, errors.New("security audit repository not initialized")
	}
	base := r.db.WithContext(ctx).Model(&model.LoginEvent{}).Where("login_at >= ?", since)
	if err := base.Count(&total).Error; err != nil {
		return 0, 0, err
	}
	err = r.db.WithContext(ctx).Model(&model.LoginEvent{}).
		Where("login_at >= ? AND risk_level IN ?", since, []model.RiskLevel{model.RiskLevelHigh, model.RiskLevelCritical}).
		Count(&high).Error
	return total, high, err
}

// CountOpenAlertsBySeverity 统计 open 状态告警按风险级分布（overview 用）。
func (r *SecurityAuditRepository) CountOpenAlertsBySeverity(ctx context.Context) (map[string]int64, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("security audit repository not initialized")
	}
	type row struct {
		RiskLevel string
		Cnt       int64
	}
	var rows []row
	err := r.db.WithContext(ctx).Model(&model.SecurityAlert{}).
		Select("risk_level, count(*) as cnt").
		Where("status = ?", model.SecurityAlertStatusOpen).
		Group("risk_level").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(rows))
	for _, rw := range rows {
		out[rw.RiskLevel] = rw.Cnt
	}
	return out, nil
}

// LatestSecurityAudit 取最近一次审计（含 Items，供总览读弱密码项结论）。
func (r *SecurityAuditRepository) LatestSecurityAudit(ctx context.Context) (*model.SecurityAudit, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("security audit repository not initialized")
	}
	var audit model.SecurityAudit
	err := r.db.WithContext(ctx).Preload("Items").Order("id DESC").First(&audit).Error
	if err != nil {
		return nil, err
	}
	return &audit, nil
}

// ListActiveUserPasswordHashes 弱密码抽样探测的取数面：只回活跃用户的哈希，
// 不回明文（库里有且只有哈希）；limit 由调用方约束在两位数——bcrypt 比对是
// 毫秒级×候选数，扫全量会把审计端点拖成 DoS 自己。
func (r *SecurityAuditRepository) ListActiveUserPasswordHashes(ctx context.Context, limit int) ([]model.SystemUser, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("security audit repository not initialized")
	}
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	var users []model.SystemUser
	err := r.db.WithContext(ctx).
		Select("id, username, password").
		Where("status = ?", 1).
		Order("id ASC").Limit(limit).Find(&users).Error
	return users, err
}
