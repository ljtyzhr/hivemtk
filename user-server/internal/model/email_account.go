package model

import "time"

// EmailAccount 邮件账号配置（`email_accounts`，一租户多账号）。
//
// 为什么搬到这里：它原先声明在 `internal/service/email.go`，靠 `init()` 里的
// `db.RegisterExtraModels` 挤进 AutoMigrate；而 `internal/repository/email_account_repo.go`
// 为了"gorm 扫描"另写了 `EmailAccountRow` —— 同一张表两份结构体，且两份并不一致
// （repo 那份没有 tenant_id，也没有 created_at/updated_at）。两份结构体的漂移不会报错，
// 只会让"读出来的账号"缺列，或者让"建表"与"写入"描述的列集不同。
//
// 声明在 leaf 包（model 只依赖 gorm）就不再需要绕环登记，正常进 allModels 即可。
type EmailAccount struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID         uint      `gorm:"primaryKey" json:"id"`
	Name       string    `gorm:"type:varchar(100)" json:"name"`
	Host       string    `gorm:"type:varchar(255);not null" json:"host"`
	Port       int       `gorm:"default:465" json:"port"`
	Username   string    `gorm:"type:varchar(255)" json:"username"`
	Password   string    `gorm:"type:varchar(255)" json:"-"`
	FromAddr   string    `gorm:"type:varchar(255);not null" json:"from_addr"`
	FromName   string    `gorm:"type:varchar(100)" json:"from_name"`
	UseSSL     bool      `gorm:"default:true" json:"use_ssl"`
	DailyQuota int       `gorm:"default:500" json:"daily_quota"`
	DailyUsed  int       `gorm:"default:0" json:"daily_used"`
	Status     string    `gorm:"type:varchar(20);default:'active'" json:"status"`
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt  time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (EmailAccount) TableName() string { return "email_accounts" }
