package repository

import (
	"context"

	"gorm.io/gorm"

	"hivemtk-user/internal/model"
)

// ChatChannelRepository 客服 Web Widget 渠道仓库
//
// 五层架构 §三.5：将 service 层的 s.db.* 调用收敛到 repository，
// service 层禁止直接持有/调用 *gorm.DB。
type ChatChannelRepository struct {
	db *gorm.DB
}

// NewChatChannelRepositoryWithDB 创建指定数据库连接的渠道仓库实例（用于 service 依赖注入 / 测试）
//
// 不做 nil 兜底：隐式回落到全局句柄会把"装配点拿错了 db"变成运行期才显形的跨库读写，
// 而本仓的装配口径是显式注入（router/main 传 gormDB）。
func NewChatChannelRepositoryWithDB(db *gorm.DB) *ChatChannelRepository {
	return &ChatChannelRepository{db: db}
}

// Create 创建渠道
func (r *ChatChannelRepository) Create(ctx context.Context, channel *model.ChatChannel) error {
	return r.db.WithContext(ctx).Create(channel).Error
}

// Updates 按主键 ID 批量更新字段（map 形式，零值字段也会更新）
//
// 0 行命中回 ErrRecordNotFound：服务层的每条写路径都先读过那一行，
// 更新不到只可能是行在读与写之间被删（或被租户闸挡掉）——静默成功会让调用方
// 拿着一份"已更新"的答复去读回一行旧值。
func (r *ChatChannelRepository) Updates(ctx context.Context, id uint, updates map[string]any) error {
	res := r.db.WithContext(ctx).Model(&model.ChatChannel{}).
		Where("id = ?", id).
		Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// UpdateField 按主键 ID 更新单个字段（用于 status / app_key / app_secret_hash 等单字段切换）
//
// 0 行命中的判据与 Updates 同一条。
func (r *ChatChannelRepository) UpdateField(ctx context.Context, id uint, field string, value any) error {
	res := r.db.WithContext(ctx).Model(&model.ChatChannel{}).
		Where("id = ?", id).
		Update(field, value)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// GetByID 按主键 ID 查询渠道
func (r *ChatChannelRepository) GetByID(ctx context.Context, id uint) (*model.ChatChannel, error) {
	var channel model.ChatChannel
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&channel).Error; err != nil {
		return nil, err
	}
	return &channel, nil
}

// GetByChannelID 按 channel_id 查询渠道
func (r *ChatChannelRepository) GetByChannelID(ctx context.Context, channelID string) (*model.ChatChannel, error) {
	var channel model.ChatChannel
	if err := r.db.WithContext(ctx).Where("channel_id = ?", channelID).First(&channel).Error; err != nil {
		return nil, err
	}
	return &channel, nil
}

// GetByAppKey 按 app_key 查询渠道
func (r *ChatChannelRepository) GetByAppKey(ctx context.Context, appKey string) (*model.ChatChannel, error) {
	var channel model.ChatChannel
	if err := r.db.WithContext(ctx).Where("app_key = ?", appKey).First(&channel).Error; err != nil {
		return nil, err
	}
	return &channel, nil
}

// ChatChannelListQuery 渠道列表查询条件（与 service 层字段对齐）
type ChatChannelListQuery struct {
	Keyword  string
	Status   string
	Page     int
	PageSize int
}

// ListByQuery 按条件分页查询渠道列表
//
// 行为与原 service.List 一致：
//   - keyword 同时模糊匹配 channel_name / app_key
//   - status 等值过滤
//   - 按 id DESC 排序
func (r *ChatChannelRepository) ListByQuery(ctx context.Context, q ChatChannelListQuery) ([]model.ChatChannel, int64, error) {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 || q.PageSize > 100 {
		q.PageSize = 20
	}

	tx := r.db.WithContext(ctx).Model(&model.ChatChannel{})
	if q.Keyword != "" {
		like := "%" + q.Keyword + "%"
		tx = tx.Where("channel_name LIKE ? OR app_key LIKE ?", like, like)
	}
	if q.Status != "" {
		tx = tx.Where("status = ?", q.Status)
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var channels []model.ChatChannel
	if err := tx.Order("id DESC").
		Offset((q.Page - 1) * q.PageSize).
		Limit(q.PageSize).
		Find(&channels).Error; err != nil {
		return nil, 0, err
	}
	return channels, total, nil
}

// IncrementVisitorCount 增加访客计数（visitor_count + 1）
func (r *ChatChannelRepository) IncrementVisitorCount(ctx context.Context, channelID string) error {
	return r.db.WithContext(ctx).Model(&model.ChatChannel{}).
		Where("channel_id = ?", channelID).
		UpdateColumn("visitor_count", gorm.Expr("visitor_count + 1")).Error
}

// IncrementSessionCount 增加会话计数（session_count + 1）
func (r *ChatChannelRepository) IncrementSessionCount(ctx context.Context, channelID string) error {
	return r.db.WithContext(ctx).Model(&model.ChatChannel{}).
		Where("channel_id = ?", channelID).
		UpdateColumn("session_count", gorm.Expr("session_count + 1")).Error
}
