// user-server 管理员配置：仅承载非敏感的 UI 行为配置。
//
// 安全基线：超管账号/密码完全由 InitAdmin 接口入库（POST /api/system/init-admin），
// 不读取任何 .env / 配置文件中的密码；超管密码唯一来源是
// system_users.password（bcrypt 哈希），任何代码路径都不应持有「默认密码」概念。
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
)

// AdminConfig 管理员配置（仅承载非敏感的 UI 行为配置，不再含密码字段）
type AdminConfig struct {
	DefaultAdmin DefaultAdminConfig `json:"default_admin" yaml:"default_admin"`
	AutoLogin    AutoLoginConfig    `json:"auto_login" yaml:"auto_login"`
	Login        LoginConfig        `json:"login" yaml:"login"`
}

// DefaultAdminConfig 默认管理员展示信息（无密码字段，见包注释的安全基线）
type DefaultAdminConfig struct {
	Username string `json:"username" yaml:"username"`
	Email    string `json:"email" yaml:"email"`
	RealName string `json:"real_name" yaml:"real_name"`
}

// AutoLoginConfig 自动登录配置
type AutoLoginConfig struct {
	Enabled         bool `json:"enabled" yaml:"enabled"`
	UseDefaultAdmin bool `json:"use_default_admin" yaml:"use_default_admin"`
}

// LoginConfig 登录页提示配置
type LoginConfig struct {
	ShowDefaultCredentials bool   `json:"show_default_credentials" yaml:"show_default_credentials"`
	DefaultCredentialsHint string `json:"default_credentials_hint" yaml:"default_credentials_hint"`
}

var defaultAdminConfig = AdminConfig{
	DefaultAdmin: DefaultAdminConfig{
		Username: "admin",
		Email:    "admin@example.com",
		RealName: "系统管理员",
	},
	AutoLogin: AutoLoginConfig{
		Enabled:         false,
		UseDefaultAdmin: false,
	},
	Login: LoginConfig{
		ShowDefaultCredentials: false,
		DefaultCredentialsHint: "",
	},
}

// GetAdminConfig 获取管理员配置（仅 UI 行为，不返回任何形式的密码字段）
func GetAdminConfig() *AdminConfig {
	config := defaultAdminConfig

	cfgPath := os.Getenv("ADMIN_CONFIG_FILE")
	if cfgPath == "" {
		cfgPath = filepath.Join(".env", "admin.json")
	}
	if b, err := os.ReadFile(cfgPath); err == nil && len(b) > 0 {
		var fileCfg AdminConfig
		if jsonErr := json.Unmarshal(b, &fileCfg); jsonErr == nil {
			if fileCfg.DefaultAdmin.Username != "" {
				config.DefaultAdmin.Username = fileCfg.DefaultAdmin.Username
			}
			if fileCfg.DefaultAdmin.Email != "" {
				config.DefaultAdmin.Email = fileCfg.DefaultAdmin.Email
			}
			if fileCfg.DefaultAdmin.RealName != "" {
				config.DefaultAdmin.RealName = fileCfg.DefaultAdmin.RealName
			}

			config.AutoLogin.Enabled = fileCfg.AutoLogin.Enabled
			config.AutoLogin.UseDefaultAdmin = fileCfg.AutoLogin.UseDefaultAdmin

			if fileCfg.Login.DefaultCredentialsHint != "" {
				config.Login.DefaultCredentialsHint = fileCfg.Login.DefaultCredentialsHint
			}
			config.Login.ShowDefaultCredentials = fileCfg.Login.ShowDefaultCredentials
		}
	}

	if username := os.Getenv("ADMIN_USERNAME"); username != "" {
		config.DefaultAdmin.Username = username
	}
	if email := os.Getenv("ADMIN_EMAIL"); email != "" {
		config.DefaultAdmin.Email = email
	}
	if realName := os.Getenv("ADMIN_REAL_NAME"); realName != "" {
		config.DefaultAdmin.RealName = realName
	}

	if autoLogin := os.Getenv("AUTO_LOGIN_ENABLED"); autoLogin != "" {
		if enabled, err := strconv.ParseBool(autoLogin); err == nil {
			config.AutoLogin.Enabled = enabled
		}
	}
	if useDefault := os.Getenv("USE_DEFAULT_ADMIN"); useDefault != "" {
		if use, err := strconv.ParseBool(useDefault); err == nil {
			config.AutoLogin.UseDefaultAdmin = use
		}
	}

	if showHint := os.Getenv("SHOW_DEFAULT_CREDENTIALS"); showHint != "" {
		if show, err := strconv.ParseBool(showHint); err == nil {
			config.Login.ShowDefaultCredentials = show
		}
	}
	if hint := os.Getenv("DEFAULT_CREDENTIALS_HINT"); hint != "" {
		config.Login.DefaultCredentialsHint = hint
	}

	return &config
}
