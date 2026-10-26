// mcp_server.go 外部 MCP server 配置面仓储（P1-3，2026-10-25 自 service 层下沉）。
//
// service 不得取 DB 句柄（架构文档 L3）；发现注册（DiscoverAndRegisterRow）需要
// 句柄落库 tool_count/last_status，因此连同包装一起收在本层——句柄不出 repository。
package repository

import (
	"context"
	"time"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

// MCPServerRepository 外部 MCP server 配置 CRUD + 发现注册。
type MCPServerRepository struct {
	db *gorm.DB
}

// NewMCPServerRepository 默认构造（全局句柄，仓库既有惯例）。
func NewMCPServerRepository() *MCPServerRepository {
	return &MCPServerRepository{db: GetDB()}
}

// FindByName 按名字查配置；不存在返回 gorm.ErrRecordNotFound。
func (r *MCPServerRepository) FindByName(ctx context.Context, name string) (*model.MCPServer, error) {
	var row model.MCPServer
	err := r.db.WithContext(ctx).Where("name = ?", name).First(&row).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// Create 落库。
func (r *MCPServerRepository) Create(ctx context.Context, row *model.MCPServer) error {
	return r.db.WithContext(ctx).Create(row).Error
}

// List 全量列表（租户隔离由 tenancyidx 全局 callback 负责，不手写过滤——
// 重复过滤会掩盖登记表漏新表，守这条的是 mcp_client_test 的多租户用例）。
func (r *MCPServerRepository) List(ctx context.Context) ([]*model.MCPServer, error) {
	var out []*model.MCPServer
	err := r.db.WithContext(ctx).Order("id DESC").Find(&out).Error
	return out, err
}

// Delete 按 id 删除。
func (r *MCPServerRepository) Delete(ctx context.Context, id uint) error {
	res := r.db.WithContext(ctx).Delete(&model.MCPServer{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// GetByID 单条；不存在返回 gorm.ErrRecordNotFound。
func (r *MCPServerRepository) GetByID(ctx context.Context, id uint) (*model.MCPServer, error) {
	var row model.MCPServer
	if err := r.db.WithContext(ctx).First(&row, id).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

// SetEnabled 启停。
func (r *MCPServerRepository) SetEnabled(ctx context.Context, id uint, enabled bool) error {
	return r.db.WithContext(ctx).Model(&model.MCPServer{}).Where("id = ?", id).
		Update("enabled", enabled).Error
}

// UpdateSyncResult 回写自检结果（Test 连通性探测后）。
func (r *MCPServerRepository) UpdateSyncResult(ctx context.Context, id uint, status, errMsg string, toolCount int, syncedAt *time.Time) error {
	return r.db.WithContext(ctx).Model(&model.MCPServer{}).Where("id = ?", id).
		Updates(map[string]any{
			"last_status": status, "last_error": errMsg,
			"tool_count": toolCount, "last_synced_at": syncedAt,
		}).Error
}
