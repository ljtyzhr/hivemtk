package model

import (
	_type "hivemtk-user/internal/pkg/utils/type"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Account struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID               string `gorm:"type:varchar(36);primaryKey" json:"id"`
	TgName           string `gorm:"tg_name" json:"tg_name"`
	TgBotToken       string `gorm:"tg_bot_token" json:"tg_bot_token"`
	GroupID          int64  `gorm:"group_id" json:"group_id"`
	Price            string `gorm:"price" json:"price"`
	ProxyEnableProxy bool   `gorm:"default:false" json:"proxy_enable_proxy"`
	ProxyProtoclo    string `gorm:"default:'http'" json:"proxy_protoclo"`
	ProxyHost        string `gorm:"default:'127.0.0.1'" json:"proxy_host"`
	ProxyPort        int    `gorm:"default:1080" json:"proxy_port"`

	// Status 不声明 default：AccountStatusInactive==0 是合法状态，带 default:1 时
	// GORM 会把显式 0 当零值省略，落库反被 DB 默认值改成 Active。
	Status     _type.AccountStatusType `gorm:"index" json:"status"`
	CreateTime int64                   `gorm:"autoCreateTime" json:"create_time"`
	Msg        string                  `gorm:"msg" json:"msg"`
	URL        string                  `gorm:"url" json:"url"`
}

func (a *Account) TableName() string {
	return "account"
}

// BeforeCreate 生成主键 ID（仅当调用方没给时）。
//
// 为什么必须留着：Account.ID 是 `varchar(36)` 的**字符串**主键，GORM 的
// 自增/默认机制对它无效 ⇒ 主键值只能由模型自己填。没有这个钩子，
// `db.Create(&model.Account{})` 会以**空串**作主键落库 ⇒ 第二行就撞
// 主键冲突。2026-10-21 的一次"零生产消费者清理"把这个钩子与仓储的
// Create 一起删了（理由是仓储 Create 没有调用方），但模型的**主键契约**
// 还在——只读调用方不代表这张表不能被写。集成测试
// TestIntegration_AccountManagement 立刻红在这里（"Expected account ID
// to be generated"），是它把这个问题挡在了提交前。
//
// **只在 ID 为空时生成**：调用方显式给了 ID（种子/数据订正/跨表搬行）
// 就要原样保留，否则"预置一行 acc-tenant-a"会被换成随机 UUID，
// 后续按那个 ID 断言的用例全红。与 WhatsappAccount.BeforeCreate 同一口径。
func (a *Account) BeforeCreate(tx *gorm.DB) error {
	if a.ID == "" {
		a.ID = uuid.New().String()
	}
	return nil
}
