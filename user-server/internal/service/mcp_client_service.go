package service

// mcp_client_service.go P1-3：外部 MCP server 配置面（运营增删/测试/发现）。
//
// 保存即发现：与 OpenAPI 工具集同口径（导入即注册），避免「配好了但要重启才生效」。
// 工具进同一个 ToolRegistry ⇒ 自动过装饰器链；server 不可达只把该行标 error。

import (
	"context"
	"fmt"
	"strings"
	"time"

	"hivemtk-user/internal/aiagent/agent/tooluse"
	"hivemtk-user/internal/aiagent/mcp"
	"hivemtk-user/internal/model"
	dbpkg "hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/dbencrypt"
	"hivemtk-user/internal/pkg/utils/logger"
)

// MCPClientService 外部 MCP server 管理。
type MCPClientService struct{}

// NewMCPClientService 构造。
func NewMCPClientService() *MCPClientService { return &MCPClientService{} }

// MCPImportInput 新增/更新配置。
type MCPImportInput struct {
	Name            string `json:"name"`
	Endpoint        string `json:"endpoint"`
	APIKey          string `json:"api_key"`
	Enabled         *bool  `json:"enabled"`
	TimeoutSeconds  int    `json:"timeout_seconds"`
	FailLimit       int    `json:"fail_limit"`
	CooldownSeconds int    `json:"cooldown_seconds"`
}

// Create 落库并立即发现注册。
func (s *MCPClientService) Create(ctx context.Context, in MCPImportInput) (*model.MCPServer, int, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, 0, fmt.Errorf("name 不能为空")
	}
	if strings.TrimSpace(in.Endpoint) == "" {
		return nil, 0, fmt.Errorf("endpoint 不能为空")
	}
	var existing model.MCPServer
	if err := dbpkg.GetDB().WithContext(ctx).Where("name = ?", in.Name).First(&existing).Error; err == nil {
		return nil, 0, fmt.Errorf("MCP server %q 已存在，请先删除", in.Name)
	}
	row := &model.MCPServer{
		Name: in.Name, Endpoint: in.Endpoint,
		APIKeyEnc:      dbencrypt.Encrypt(in.APIKey),
		TimeoutSeconds: in.TimeoutSeconds, FailLimit: in.FailLimit, CooldownSeconds: in.CooldownSeconds,
	}
	row.Enabled = in.Enabled == nil || *in.Enabled
	if err := dbpkg.GetDB().WithContext(ctx).Create(row).Error; err != nil {
		return nil, 0, fmt.Errorf("落库失败: %w", err)
	}
	if row.Enabled {
		n, err := mcp.DiscoverAndRegisterRow(ctx, dbpkg.GetDB(), row, tooluse.GetGlobalRegistry())
		if err != nil {
			// 不当作失败：配置已存下，失败留痕在 last_status/last_error，运维可见。
			logger.Ctx(ctx).Warn().Err(err).Str("name", in.Name).Msg("[mcp-client] 加入时发现失败（配置已保存）")
			return row, 0, nil
		}
		row.ToolCount = n
	}
	return row, row.ToolCount, nil
}

// List 配置列表（不含密钥）。
//
// 隔离靠 tenancyidx 的全局 callback（模型读路径自动注入 WHERE tenant_id = ?，
// 写路径自动打戳）——这里**不要**再手写过滤：重复过滤会掩盖"登记表漏了新表"，
// 让真正该红的那次静默通过。守这条的是 mcp_client_service_test 的多租户用例。
func (s *MCPClientService) List(ctx context.Context) ([]*model.MCPServer, error) {
	var out []*model.MCPServer
	err := dbpkg.GetDB().WithContext(ctx).Order("id DESC").Find(&out).Error
	return out, err
}

// Delete 删除配置。
//
// 已注册的工具在重启后自然消失（启动发现只拉现存行）；进程内工具名保留到重启，
// 与 custom_tools 的注销口径一致（tooluse.UnregisterProvider 注释：注销不等于删工具）。
func (s *MCPClientService) Delete(ctx context.Context, id uint) error {
	return dbpkg.GetDB().WithContext(ctx).Delete(&model.MCPServer{}, id).Error
}

// SetEnabled 启停。
func (s *MCPClientService) SetEnabled(ctx context.Context, id uint, enabled bool) (*model.MCPServer, int, error) {
	var row model.MCPServer
	if err := dbpkg.GetDB().WithContext(ctx).First(&row, id).Error; err != nil {
		return nil, 0, fmt.Errorf("配置不存在")
	}
	if err := dbpkg.GetDB().WithContext(ctx).Model(&row).Update("enabled", enabled).Error; err != nil {
		return nil, 0, err
	}
	row.Enabled = enabled
	if !enabled {
		return &row, 0, nil
	}
	n, err := mcp.DiscoverAndRegisterRow(ctx, dbpkg.GetDB(), &row, tooluse.GetGlobalRegistry())
	if err != nil {
		return &row, 0, nil
	}
	row.ToolCount = n
	return &row, n, nil
}

// Test 连通性/发现自检（不注册，仅回工具名清单供运营确认）。
func (s *MCPClientService) Test(ctx context.Context, id uint) ([]string, error) {
	var row model.MCPServer
	if err := dbpkg.GetDB().WithContext(ctx).First(&row, id).Error; err != nil {
		return nil, fmt.Errorf("配置不存在")
	}
	client := mcp.NewClient(mcp.ServerConfigFromRow(&row))
	tools, err := client.ListTools(ctx)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		names = append(names, t.Name)
	}
	now := time.Now()
	_ = dbpkg.GetDB().WithContext(ctx).Model(&row).Updates(map[string]any{
		"last_status": "ok", "last_error": "", "tool_count": len(names), "last_synced_at": &now,
	}).Error
	return names, nil
}
