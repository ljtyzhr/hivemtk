package repository

import (
	"context"

	_db "hivemtk-user/internal/pkg/db"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

// ExternalKBConnectorRepo 第三方知识库连接器仓储（L3 controller 禁直连 DB）
type ExternalKBConnectorRepo struct{ db *gorm.DB }

func NewExternalKBConnectorRepo() *ExternalKBConnectorRepo {
	return &ExternalKBConnectorRepo{db: _db.GetDB()}
}

func (r *ExternalKBConnectorRepo) List(ctx context.Context, limit int) ([]model.ExternalKBConnector, error) {
	var rows []model.ExternalKBConnector
	err := r.db.WithContext(ctx).Order("id DESC").Limit(limit).Find(&rows).Error
	return rows, err
}

func (r *ExternalKBConnectorRepo) GetByID(ctx context.Context, id uint) (*model.ExternalKBConnector, error) {
	var row model.ExternalKBConnector
	err := r.db.WithContext(ctx).First(&row, id).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *ExternalKBConnectorRepo) Create(ctx context.Context, row *model.ExternalKBConnector) error {
	return r.db.WithContext(ctx).Create(row).Error
}

// Update 更新连接器。
//
// 0 行受影响即「不存在」（`db.EnsureAffected`）：GORM 的 Updates 对没有匹配行的
// Where 不返回错误，于是「改一个不存在的连接器」会回 200「已更新」。
func (r *ExternalKBConnectorRepo) Update(ctx context.Context, id uint, updates map[string]any) error {
	return _db.EnsureAffected(r.db.WithContext(ctx).
		Model(&model.ExternalKBConnector{}).Where("id = ?", id).Updates(updates))
}

// Delete 删除连接器（同 Update：0 行 ⇒ 不存在，而不是静默成功）。
func (r *ExternalKBConnectorRepo) Delete(ctx context.Context, id uint) error {
	return _db.EnsureAffected(r.db.WithContext(ctx).Delete(&model.ExternalKBConnector{}, id))
}

// MCPCredentialRepo MCP 凭证对仓储
type MCPCredentialRepo struct{ db *gorm.DB }

func NewMCPCredentialRepo() *MCPCredentialRepo {
	return &MCPCredentialRepo{db: _db.GetDB()}
}

func (r *MCPCredentialRepo) List(ctx context.Context, limit int) ([]model.MCPCredential, error) {
	var rows []model.MCPCredential
	err := r.db.WithContext(ctx).Order("id DESC").Limit(limit).Find(&rows).Error
	return rows, err
}

func (r *MCPCredentialRepo) Create(ctx context.Context, row *model.MCPCredential) error {
	return r.db.WithContext(ctx).Create(row).Error
}

func (r *MCPCredentialRepo) Update(ctx context.Context, id uint, updates map[string]any) error {
	return _db.EnsureAffected(r.db.WithContext(ctx).
		Model(&model.MCPCredential{}).Where("id = ?", id).Updates(updates))
}

func (r *MCPCredentialRepo) Delete(ctx context.Context, id uint) error {
	return _db.EnsureAffected(r.db.WithContext(ctx).Delete(&model.MCPCredential{}, id))
}
