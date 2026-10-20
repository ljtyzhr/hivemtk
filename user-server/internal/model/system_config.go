package model

// SystemConfig 站点配置模型（每个租户一行）
//
// 站点名/主题色/注册开关/上传上限等都是商户自己的运营口径，SaaS 下必须按租户
// 各存一份。tenant_id 唯一索引保证"一租户一站点配置"，读取由租户隔离 callback
// 自动注入 WHERE tenant_id，写入由仓储显式继承既有行归属（见 SaveConfig）。
type SystemConfig struct {
	ID                   uint   `gorm:"primaryKey" json:"id"`
	TenantID             uint   `gorm:"not null;default:1;uniqueIndex:uni_system_config_tenant" json:"tenant_id,omitempty"`
	Name                 string `gorm:"size:255;not null" json:"site_name"`
	WebsiteURL           string `gorm:"size:255" json:"website_url"`
	LogoURL              string `gorm:"size:500" json:"logo_url"`
	ThemeColor           string `gorm:"size:20" json:"theme_color"`
	SEOKeywords          string `gorm:"size:500" json:"seo_keywords"`
	SEODescription       string `gorm:"type:text" json:"seo_description"`
	ServicePhone         string `gorm:"size:50" json:"service_phone"`
	ServiceEmail         string `gorm:"size:100" json:"service_email"`
	ICPRecord            string `gorm:"size:100" json:"icp_record"`
	PoliceRecord         string `gorm:"size:100" json:"police_record"`
	EnableRegister       bool   `gorm:"default:true" json:"enable_register"`
	EnableEmailMarketing bool   `gorm:"default:true" json:"enable_email_marketing"`
	EnableRAG            bool   `gorm:"default:true" json:"enable_rag"`
	MaintenanceMode      bool   `gorm:"default:false" json:"maintenance_mode"`
	MaxUsers             int    `gorm:"default:0" json:"max_users"`
	MaxUploadSizeMB      int    `gorm:"default:50" json:"max_upload_size_mb"`
}

func (*SystemConfig) TableName() string {
	return "system_config"
}
