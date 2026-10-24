// Package service — custom_tool_service.go P1-2：OpenAPI 导入服务层。
// 职责：校验（SSRF 防线在 tooluse.ValidateOpenAPIBaseURL）→ 加密落凭据 →
// 注册 Provider 进全局注册中心（自动进入装饰链接线）→ 列表/删除。
package service

import (
	"context"
	"fmt"
	"strings"

	"hivemtk-user/internal/aiagent/agent/tooluse"
	"hivemtk-user/internal/model"
	dbpkg "hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/dbencrypt"
	"hivemtk-user/internal/pkg/utils/logger"
)

// CustomToolService OpenAPI 自定义工具服务。
type CustomToolService struct{}

// NewCustomToolService 构造。
func NewCustomToolService() *CustomToolService { return &CustomToolService{} }

// ImportInput 导入请求。
type ImportInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	BaseURL     string `json:"base_url"`
	SchemaJSON  string `json:"schema_json"`
	AuthHeader  string `json:"auth_header"`
	AuthValue   string `json:"auth_value"`
	TimeoutMS   int    `json:"timeout_ms"`
}

// Import 解析 schema、SSRF 校验、落库并注册 Provider。返回导入的工具数。
func (s *CustomToolService) Import(ctx context.Context, in ImportInput) (int, *model.CustomTool, error) {
	if strings.TrimSpace(in.Name) == "" {
		return 0, nil, fmt.Errorf("name 不能为空")
	}
	provider, err := tooluse.NewOpenAPIToolProvider(tooluse.OpenAPIImportSpec{
		Name: in.Name, Description: in.Description, BaseURL: in.BaseURL,
		SchemaJSON: in.SchemaJSON, AuthHeader: in.AuthHeader, AuthValue: in.AuthValue,
		TimeoutMS: in.TimeoutMS,
	})
	if err != nil {
		return 0, nil, err
	}
	// SSRF：导入时校验一次（运行时转发由 client 直连该基址，导入即锁定）
	if err := tooluse.ValidateOpenAPIBaseURL(in.BaseURL); err != nil {
		return 0, nil, err
	}

	// 去重
	var existing model.CustomTool
	err = dbpkg.GetDB().WithContext(ctx).Where("name = ?", in.Name).First(&existing).Error
	if err == nil {
		return 0, nil, fmt.Errorf("工具集 %q 已存在，请先删除再导入", in.Name)
	}

	rec := &model.CustomTool{
		Name: in.Name, Description: in.Description, BaseURL: in.BaseURL,
		AuthHeader: in.AuthHeader, SchemaJSON: in.SchemaJSON,
		TimeoutMS: in.TimeoutMS, IsActive: true,
	}
	if in.AuthValue != "" {
		rec.AuthValueEnc = dbencrypt.Encrypt(in.AuthValue)
	}
	if err := dbpkg.GetDB().WithContext(ctx).Create(rec).Error; err != nil {
		return 0, nil, fmt.Errorf("落库失败: %w", err)
	}

	// 经 tooluse 层自注册通道挂 Provider：既覆盖「导入时已装配」（下一节即时注册），
	// 也覆盖「先导入后重启」（registerAllAgentToolsViaProviders 会拉 GetAutoRegisteredProviders）。
	tooluse.RegisterToolProvider(provider)
	if tr := tooluse.GetGlobalRegistry(); tr != nil {
		tools, perr := provider.Provide(tooluse.ProviderContext{})
		if perr == nil {
			for _, t := range tools {
				_ = tr.Register(t)
			}
		}
	}
	n := countOps(in.SchemaJSON)
	dbpkg.GetDB().WithContext(ctx).Model(rec).Update("tool_count", n)
	rec.ToolCount = n
	logger.Ctx(ctx).Info().Str("name", in.Name).Int("tools", n).Msg("[custom-tool] OpenAPI 工具集已导入并注册")
	return n, rec, nil
}

// List 导入记录列表（不含凭据明文/密文）。
func (s *CustomToolService) List(ctx context.Context) ([]*model.CustomTool, error) {
	var out []*model.CustomTool
	err := dbpkg.GetDB().WithContext(ctx).Order("id DESC").Find(&out).Error
	return out, err
}

// Delete 删除记录。已注册工具的卸载交给重启装配（tooluse.UnregisterProvider
// 注释口径：注销 Provider 不删除已注册工具，同步清理由调用方处理；重启后
// auto_register 列表已无此项，自然收敛）。
func (s *CustomToolService) Delete(ctx context.Context, id uint) error {
	var rec model.CustomTool
	if err := dbpkg.GetDB().WithContext(ctx).First(&rec, id).Error; err != nil {
		return err
	}
	return dbpkg.GetDB().WithContext(ctx).Delete(&model.CustomTool{}, id).Error
}

// countOps 统计 schema 中的操作数（ToolCount 展示用）。
func countOps(schemaJSON string) int {
	ops, err := tooluse.ParseOpenAPISchema(schemaJSON)
	if err != nil {
		return 0
	}
	return len(ops)
}
