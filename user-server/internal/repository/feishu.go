package repository

import (
	"context"
	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// FeishuAccountRepository 飞书账号仓库
type FeishuAccountRepository struct {
	db *gorm.DB
}

// NewFeishuAccountRepository 创建飞书账号仓库
func NewFeishuAccountRepository() *FeishuAccountRepository {
	return &FeishuAccountRepository{db: _db.GetDB()}
}

// SetDB 注入 db（用于测试）
func (r *FeishuAccountRepository) SetDB(ctx context.Context, db *gorm.DB) {
	if db != nil {
		r.db = db
	}
}

// GetDB 返回底层连接（未初始化时为 nil），供调用方判空防误用
func (r *FeishuAccountRepository) GetDB() *gorm.DB {
	if r == nil {
		return nil
	}
	return r.db
}

// Create 创建飞书账号
func (r *FeishuAccountRepository) Create(ctx context.Context, acc *model.FeishuAccount) error {
	return r.db.WithContext(ctx).Create(acc).Error
}

// GetByID 根据 ID 获取
func (r *FeishuAccountRepository) GetByID(ctx context.Context, id uint) (*model.FeishuAccount, error) {
	var acc model.FeishuAccount
	if r.db == nil { // DB 未装配：nil 语义收口在 repo 边界（对齐 kb_document_chunk_repo），service 不摸全局句柄
		return nil, gorm.ErrInvalidDB
	}
	if err := r.db.WithContext(ctx).First(&acc, id).Error; err != nil {
		return nil, err
	}
	return &acc, nil
}

// GetAll 获取所有飞书账号
func (r *FeishuAccountRepository) GetAll(ctx context.Context) ([]*model.FeishuAccount, error) {
	var accs []*model.FeishuAccount
	if err := r.db.WithContext(ctx).Order("id DESC").Find(&accs).Error; err != nil {
		return nil, err
	}
	return accs, nil
}

// Update 更新飞书账号
func (r *FeishuAccountRepository) Update(ctx context.Context, acc *model.FeishuAccount) error {
	return saveWholeRow(r.db.WithContext(ctx), acc)
}

// Delete 删除飞书账号
func (r *FeishuAccountRepository) Delete(ctx context.Context, id uint) error {
	res := r.db.WithContext(ctx).Delete(&model.FeishuAccount{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		// 删不存在的 id（含别租户的行）不能回"删除成功"。
		return gorm.ErrRecordNotFound
	}
	return nil
}

// UpdateToken 更新访问令牌
func (r *FeishuAccountRepository) UpdateToken(ctx context.Context, id uint, token string, expires *int64) error {
	updates := map[string]any{
		"access_token": token,
	}
	if expires != nil {
		updates["token_expires"] = expires
	}
	return r.db.WithContext(ctx).Model(&model.FeishuAccount{}).Where("id = ?", id).Updates(updates).Error
}

// FeishuCustomerRepository 飞书客户仓库
type FeishuCustomerRepository struct {
	db *gorm.DB
}

func (r *FeishuCustomerRepository) SetDB(ctx context.Context, db *gorm.DB) {
	if db != nil {
		r.db = db
	}
}

func (r *FeishuCustomerRepository) Create(ctx context.Context, c *model.FeishuCustomer) error {
	return r.db.WithContext(ctx).Create(c).Error
}

func (r *FeishuCustomerRepository) Update(ctx context.Context, c *model.FeishuCustomer) error {
	return saveWholeRow(r.db.WithContext(ctx), c)
}

// FeishuMessageRepository 飞书消息仓库
type FeishuMessageRepository struct {
	db *gorm.DB
}

func NewFeishuMessageRepository() *FeishuMessageRepository {
	return &FeishuMessageRepository{db: _db.GetDB()}
}

func (r *FeishuMessageRepository) SetDB(ctx context.Context, db *gorm.DB) {
	if db != nil {
		r.db = db
	}
}

func (r *FeishuMessageRepository) Create(ctx context.Context, m *model.FeishuMessage) error {
	return r.db.WithContext(ctx).Create(m).Error
}

func (r *FeishuMessageRepository) GetByMsgID(ctx context.Context, msgID string) (*model.FeishuMessage, error) {
	var m model.FeishuMessage
	if err := r.db.WithContext(ctx).Where("msg_id = ?", msgID).First(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

// TelegramAccountRepository Telegram 账号仓库
type TelegramAccountRepository struct {
	db *gorm.DB
}

func NewTelegramAccountRepository() *TelegramAccountRepository {
	return &TelegramAccountRepository{db: _db.GetDB()}
}

// NewTelegramAccountRepositoryWithDB 显式注入 DB（推荐）。
//
// 无参版本读全局 _db.GetDB()，在未初始化全局句柄的场景（测试、独立装配）会拿到
// nil 并在首次查询时报 gorm.ErrInvalidDB —— 调用方只能再补一次 SetDB。
// 与 NewTelegramGroupGateRepositoryWithDB / NewTelegramGroupMemberRepositoryWithDB
// 保持同一约定，避免调用点各写一套。
func NewTelegramAccountRepositoryWithDB(db *gorm.DB) *TelegramAccountRepository {
	return &TelegramAccountRepository{db: db}
}

func (r *TelegramAccountRepository) SetDB(ctx context.Context, db *gorm.DB) {
	if db != nil {
		r.db = db
	}
}

// TelegramAccountRepository Telegram 账号仓库。
//
// 每条查询/写入都必须 WithContext(ctx)：on 模式（TENANT_MODE=on）的隔离 callback 从
// stmt.Context 取租户身份，忽略 ctx 等于恒用无租户 ctx ⇒ telegram_accounts 的读写全部被
// fail-closed 拒绝（生产日志实证："[TG-Bootstrap] 列举 Telegram 账号失败: [tenant] 表
// telegram_accounts 需要租户上下文但 ctx 未绑定（query）"）。ctx 由调用方给：HTTP 路径传
// 请求 ctx（中间件已绑租户），启动期对账/平台级扫描传 tenant.WithSystemScope(ctx)。
// 与下方 QQAccountRepository 同一口径。
func (r *TelegramAccountRepository) Create(ctx context.Context, acc *model.TelegramAccount) error {
	return r.db.WithContext(ctx).Create(acc).Error
}

func (r *TelegramAccountRepository) GetByID(ctx context.Context, id uint) (*model.TelegramAccount, error) {
	var acc model.TelegramAccount
	if r.db == nil { // DB 未装配：nil 语义在 repo 边界收口（对齐 kb_document_chunk_repo），service 不摸全局句柄
		return nil, gorm.ErrInvalidDB
	}
	if err := r.db.WithContext(ctx).First(&acc, id).Error; err != nil {
		return nil, err
	}
	return &acc, nil
}

func (r *TelegramAccountRepository) GetAll(ctx context.Context) ([]*model.TelegramAccount, error) {
	var accs []*model.TelegramAccount
	if err := r.db.WithContext(ctx).Order("id DESC").Find(&accs).Error; err != nil {
		return nil, err
	}
	return accs, nil
}

func (r *TelegramAccountRepository) Update(ctx context.Context, acc *model.TelegramAccount) error {
	return saveWholeRow(r.db.WithContext(ctx), acc)
}

func (r *TelegramAccountRepository) Delete(ctx context.Context, id uint) error {
	res := r.db.WithContext(ctx).Delete(&model.TelegramAccount{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		// 删不存在的 id（含别租户的行）不能回"删除成功"。
		return gorm.ErrRecordNotFound
	}
	return nil
}

// WhatsAppCloudAccountRepository WhatsApp Cloud API 账号仓库
type WhatsAppCloudAccountRepository struct {
	db *gorm.DB
}

func NewWhatsAppCloudAccountRepository() *WhatsAppCloudAccountRepository {
	return &WhatsAppCloudAccountRepository{db: _db.GetDB()}
}

func (r *WhatsAppCloudAccountRepository) SetDB(ctx context.Context, db *gorm.DB) {
	if db != nil {
		r.db = db
	}
}

func (r *WhatsAppCloudAccountRepository) Create(ctx context.Context, acc *model.WhatsAppCloudAccount) error {
	return r.db.WithContext(ctx).Create(acc).Error
}

func (r *WhatsAppCloudAccountRepository) GetByID(ctx context.Context, id uint) (*model.WhatsAppCloudAccount, error) {
	var acc model.WhatsAppCloudAccount
	if err := r.db.WithContext(ctx).First(&acc, id).Error; err != nil {
		return nil, err
	}
	return &acc, nil
}

func (r *WhatsAppCloudAccountRepository) GetByPhoneNumberID(ctx context.Context, phoneID string) (*model.WhatsAppCloudAccount, error) {
	var acc model.WhatsAppCloudAccount
	if err := r.db.WithContext(ctx).Where("phone_number_id = ?", phoneID).First(&acc).Error; err != nil {
		return nil, err
	}
	return &acc, nil
}

func (r *WhatsAppCloudAccountRepository) GetAll(ctx context.Context) ([]*model.WhatsAppCloudAccount, error) {
	var accs []*model.WhatsAppCloudAccount
	if err := r.db.WithContext(ctx).Order("id DESC").Find(&accs).Error; err != nil {
		return nil, err
	}
	return accs, nil
}

func (r *WhatsAppCloudAccountRepository) Update(ctx context.Context, acc *model.WhatsAppCloudAccount) error {
	return saveWholeRow(r.db.WithContext(ctx), acc)
}

func (r *WhatsAppCloudAccountRepository) Delete(ctx context.Context, id uint) error {
	res := r.db.WithContext(ctx).Delete(&model.WhatsAppCloudAccount{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		// 删不存在的 id（含别租户的行）不能回"删除成功"。
		return gorm.ErrRecordNotFound
	}
	return nil
}

// GetFirstEnabled 取第一个启用的飞书账号（webhook_enabled=true 且 status=1）
// 找不到返回 gorm.ErrRecordNotFound
func (r *FeishuAccountRepository) GetFirstEnabled(ctx context.Context) (*model.FeishuAccount, error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrRecordNotFound
	}
	var acc model.FeishuAccount
	err := r.db.WithContext(ctx).
		Where("webhook_enabled = ? AND status = ?", true, 1).
		Order("id ASC").
		First(&acc).Error
	if err != nil {
		return nil, err
	}
	return &acc, nil
}

// GetFirst 取第一条飞书账号（兜底，不区分启用状态）
func (r *FeishuAccountRepository) GetFirst(ctx context.Context) (*model.FeishuAccount, error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrRecordNotFound
	}
	var acc model.FeishuAccount
	err := r.db.WithContext(ctx).Order("id ASC").First(&acc).Error
	if err != nil {
		return nil, err
	}
	return &acc, nil
}

// GetFirstEnabled 取第一个启用的 Telegram 账号（status=1）
func (r *TelegramAccountRepository) GetFirstEnabled(ctx context.Context) (*model.TelegramAccount, error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrRecordNotFound
	}
	var acc model.TelegramAccount
	err := r.db.WithContext(ctx).
		Where("status = ?", 1).
		Order("id ASC").
		First(&acc).Error
	if err != nil {
		return nil, err
	}
	return &acc, nil
}

// GetFirst 取第一条 Telegram 账号（兜底，不区分启用状态）
func (r *TelegramAccountRepository) GetFirst(ctx context.Context) (*model.TelegramAccount, error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrRecordNotFound
	}
	var acc model.TelegramAccount
	err := r.db.WithContext(ctx).Order("id ASC").First(&acc).Error
	if err != nil {
		return nil, err
	}
	return &acc, nil
}

// GetFirstEnabled 取第一个启用的 WhatsApp Cloud 账号（status=1）
func (r *WhatsAppCloudAccountRepository) GetFirstEnabled(ctx context.Context) (*model.WhatsAppCloudAccount, error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrRecordNotFound
	}
	var acc model.WhatsAppCloudAccount
	err := r.db.WithContext(ctx).
		Where("status = ?", 1).
		Order("id ASC").
		First(&acc).Error
	if err != nil {
		return nil, err
	}
	return &acc, nil
}

// GetFirst 取第一条 WhatsApp Cloud 账号（兜底，不区分启用状态）
func (r *WhatsAppCloudAccountRepository) GetFirst(ctx context.Context) (*model.WhatsAppCloudAccount, error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrRecordNotFound
	}
	var acc model.WhatsAppCloudAccount
	err := r.db.WithContext(ctx).Order("id ASC").First(&acc).Error
	if err != nil {
		return nil, err
	}
	return &acc, nil
}

// ---------------------------------------------------------------------------
// QQAccountRepository QQ 机器人账号仓库
// ---------------------------------------------------------------------------

// QQAccountRepository QQ 账号仓库
type QQAccountRepository struct {
	db *gorm.DB
}

// NewQQAccountRepository 创建 QQ 账号仓库
func NewQQAccountRepository() *QQAccountRepository {
	return &QQAccountRepository{db: _db.GetDB()}
}

// SetDB 注入 db（用于测试）
func (r *QQAccountRepository) SetDB(ctx context.Context, db *gorm.DB) {
	if db != nil {
		r.db = db
	}
}

// Create 创建 QQ 账号
func (r *QQAccountRepository) Create(ctx context.Context, acc *model.QQAccount) error {
	return r.db.WithContext(ctx).Create(acc).Error
}

// GetByID 按 ID 查询
func (r *QQAccountRepository) GetByID(ctx context.Context, id uint) (*model.QQAccount, error) {
	var acc model.QQAccount
	if r.db == nil { // DB 未装配：nil 语义收口在 repo 边界（对齐 kb_document_chunk_repo），service 不摸全局句柄
		return nil, gorm.ErrInvalidDB
	}
	if err := r.db.WithContext(ctx).First(&acc, id).Error; err != nil {
		return nil, err
	}
	return &acc, nil
}

// GetAll 全量列表（按 ID 倒序）
func (r *QQAccountRepository) GetAll(ctx context.Context) ([]*model.QQAccount, error) {
	var accs []*model.QQAccount
	if err := r.db.WithContext(ctx).Order("id DESC").Find(&accs).Error; err != nil {
		return nil, err
	}
	return accs, nil
}

// Update 更新
func (r *QQAccountRepository) Update(ctx context.Context, acc *model.QQAccount) error {
	return saveWholeRow(r.db.WithContext(ctx), acc)
}

// Delete 删除
func (r *QQAccountRepository) Delete(ctx context.Context, id uint) error {
	res := r.db.WithContext(ctx).Delete(&model.QQAccount{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		// 删不存在的 id（含别租户的行）不能回"删除成功"。
		return gorm.ErrRecordNotFound
	}
	return nil
}
