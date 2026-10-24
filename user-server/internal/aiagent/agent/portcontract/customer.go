package portcontract

import (
	"errors"

	"hivemtk-user/internal/model"
)

// ErrCustomerNotFound 客户不存在错误（sentinel）。
// 工具层依赖此 sentinel 区分"客户不存在"与其他错误，避免 import service 包；
// CustomerPort 实现方在底层返回 service.ErrCustomerNotFound 时应映射为本 sentinel。
var ErrCustomerNotFound = errors.New("客户不存在")

// CustomerIdentity 创建/更新客户的最小可识别身份子集（手机/邮箱/各平台 openID），
// 让工具层无需 import service 包即可传入身份字段。
type CustomerIdentity struct {
	Phone         string
	Email         string
	WechatOpenID  string
	DouyinOpenID  string
	XiaohongshuID string
}

// CustomerProfileView 客户 360° 视图（工具层投影）：
// Customer 为客户主表记录，RecentEvents 为最近客户行为事件（用于上下文），Tags 为长期画像标签。
type CustomerProfileView struct {
	Customer     *model.Customer        `json:"customer"`
	RecentEvents []*model.CustomerEvent `json:"recent_events,omitempty"`
	Tags         []string               `json:"tags,omitempty"`
}

// CustomerPort 客户领域能力端口
// （实现方 service.CustomerService，消费方 tooluse/customer_tools.go 等客户相关工具）
type CustomerPort interface {
	GetCustomerProfile(customerID string) (*CustomerProfileView, error)
	CreateOrUpdate(identity *CustomerIdentity) (*model.Customer, error)
	MergeCustomers(primaryID, secondaryID string) error
	AddTags(customerID string, tags []string) error
	RemoveTags(customerID string, tags []string) error
}
