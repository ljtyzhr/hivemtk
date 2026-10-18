package model

import "time"

// 功能模块化的本地镜像（用户端）
//
// ============================================================
// 为什么本地要存一份而不是每次问平台
//
// 平台不可达时（离线部署 / 平台宕机 / 网络抖动）系统必须照常可用，
// 这是「数据零出域 + 离线独立部署」承诺的直接推论。因此：
//   - 平台可达：每 10 分钟拉一次模块快照，落 tenant_modules
//   - 平台不可达：用上次落地的镜像继续判定模块闸
//   - 一次都没同步过：**基础模块全开、付费模块锁住**（见 GateOfflinePolicy）
// ============================================================

// TenantModuleStatus 本地镜像的开通状态
const (
	TenantModuleActive   = "active"
	TenantModuleExpired  = "expired"
	TenantModuleDisabled = "disabled"
)

// TenantModuleSource 开通来源（与平台端同名同义）
const (
	TenantModuleSourcePurchase    = "purchase"
	TenantModuleSourceDependency  = "dependency"
	TenantModuleSourceManual      = "manual"
	TenantModuleSourceDefaultBase = "default_base"
)

// TenantModule 本地已开通模块镜像（一行 = 一个模块）
//
// ⚠️ **平台端有一个同名表，结构完全不同**，排查 SQL 时先确认连的是哪个库：
//
//	platform_db.tenant_modules: id, merchant_key, module_key, status, source,
//	                            order_id, options, activated_at, expires_at, ...
//	user_db.tenant_modules:     tenant_id, id, module_key, status, source,
//	                            category, price_cents, menu_keys, ...
//
// 语义差别：平台端那张是**权威**（按 merchant_key 归属，带 order_id/options），
// 本表是**只读镜像**（按 tenant_id 归属，带 category/price_cents/menu_keys
// 供本地闸判定与展示）。同名是有意的（一眼看出是同一概念的两侧），
// 但不要把两侧的列混用。
type TenantModule struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID int64 `gorm:"primaryKey;autoIncrement" json:"id"`
	// ModuleKey 业务唯一键。**只声明业务列，不加 tenant_id** ——
	// 仓库的租户索引策略（internal/pkg/tenancyidx）会在 AutoMigrate 之后
	// 自动把租户表上的唯一索引复合化为 (tenant_id, 原列)。
	// 在这里自己写 tenant_id 会被该策略跳过（它按 `indexdef LIKE '%tenant_id%'`
	// 判幂等），于是索引形状与策略的产物不一致，ON CONFLICT 匹配不上 → 42P10。
	//
	// 曾用过 `uniqueIndex:idx_local_tenant_module,priority:1` 于一个额外的
	// tenant_slot 列上，想"为将来多租户预留"。那是多余的：多租户维度由
	// tenant_id 承担，多加一列只会让唯一键变成 (tenant_id, tenant_slot, module_key)
	// 而三列复合在语义上与 (tenant_id, module_key) 完全等价。
	ModuleKey string `gorm:"type:varchar(64);uniqueIndex:idx_local_tenant_module;not null" json:"module_key"`

	Status string `gorm:"type:varchar(16);not null;default:'active';index" json:"status"`
	Source string `gorm:"type:varchar(16);not null;default:'manual'" json:"source"`

	// Category / PriceCents 是平台下发的目录快照，本地只读不写
	Category   string `gorm:"type:varchar(16);not null;default:'paid'" json:"category"`
	PriceCents int64  `gorm:"type:bigint;not null;default:0" json:"price_cents"`

	// MenuKeys 下发的菜单节点 jsonb —— 模块闸按它反查"这条接口属于哪个模块"
	MenuKeys string `gorm:"type:jsonb;not null;default:'[]'" json:"menu_keys"`

	ActivatedAt time.Time  `json:"activated_at"`
	ExpiresAt   *time.Time `json:"expires_at"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (TenantModule) TableName() string { return "tenant_modules" }

// TenantModuleUsable 处于可放行状态
func TenantModuleUsable(t *TenantModule, now time.Time) bool {
	if t == nil || t.Status != TenantModuleActive {
		return false
	}
	if t.ExpiresAt == nil {
		return true
	}
	return t.ExpiresAt.After(now)
}

// ModuleSyncLog 模块下发同步日志
//
// 与 LocalAssetSyncLog 同一形态：下发失败必须留痕。区别在于本地资产同步失败
// 只影响一个资产，而模块同步失败会让**整棵菜单**回到上一次的状态 —— 运营需要
// 知道"用户的模块是从什么时候开始不准的"。
type ModuleSyncLog struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID          int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	Action      string `gorm:"type:varchar(32);not null" json:"action"` // sync | grant | revoke | offline_bootstrap
	Status      string `gorm:"type:varchar(16);not null" json:"status"` // success | failed | degraded
	ModuleKey   string `gorm:"type:varchar(64);index" json:"module_key"`
	ActiveCount int    `gorm:"not null;default:0" json:"active_count"`
	// Source 远端来源描述（platform / offline_bootstrap / cache）
	Source     string    `gorm:"type:varchar(32);not null;default:''" json:"source"`
	ErrorMsg   string    `gorm:"type:text" json:"error_msg"`
	DurationMs int64     `gorm:"not null;default:0" json:"duration_ms"`
	CreatedAt  time.Time `gorm:"autoCreateTime;index" json:"created_at"`
}

func (ModuleSyncLog) TableName() string { return "module_sync_logs" }

// ModuleOrderStatus 本地订单镜像状态（与平台端同名同义）
const (
	ModuleOrderPendingPayment = "pending_payment"
	ModuleOrderPaid           = "paid"
	ModuleOrderFulfilled      = "fulfilled"
	ModuleOrderCancelled      = "cancelled"
	ModuleOrderRejected       = "rejected"
)

// ModuleOrder 本地订单镜像
//
// **只读缓存**：状态以平台为准，本地不跑状态机、不发确认。
// 双份状态机是订单系统最经典的打架来源（两边都认为自己是对的）。
type ModuleOrder struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID string `gorm:"type:varchar(36);primaryKey" json:"id"`
	// OrderNo 业务唯一键（同 ModuleKey：只声明业务列，tenant_id 由策略加）
	OrderNo string `gorm:"type:varchar(32);uniqueIndex;not null" json:"order_no"`
	// ClientOrderNo 本地生成的幂等键（提交给平台的那一把）
	ClientOrderNo string `gorm:"type:varchar(64);index;not null" json:"client_order_no"`

	OrderType string `gorm:"type:varchar(16);not null" json:"order_type"`
	Status    string `gorm:"type:varchar(16);not null;default:'pending_payment';index" json:"status"`

	TotalAmountCents int64  `gorm:"type:bigint;not null;default:0" json:"total_amount_cents"`
	Currency         string `gorm:"type:varchar(8);not null;default:'CNY'" json:"currency"`

	// Items jsonb：订单行快照（模块 key + 名称 + 价格）
	Items string `gorm:"type:jsonb;not null;default:'[]'" json:"items"`
	// PaymentGuide 下单时平台返回的付款指引（含联系方式），本地留一份供订单页展示
	PaymentGuide string `gorm:"type:text" json:"payment_guide"`

	CreatedAt time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
	PaidAt    *time.Time `json:"paid_at"`
}

func (ModuleOrder) TableName() string { return "module_orders" }

// LLMWallet LLM 余额的本地单行镜像
//
// 一期不做本地扣减（权威在平台侧代理），镜像只用于平台不可达时仍能展示。
// 用 scope='default' 而不是 merchant_key 作主键：本仓单部署，merchant_key
// 在用户端根本不存在（那是平台侧概念）。
type LLMWallet struct {
	ID       uint   `gorm:"primaryKey" json:"id"`
	Scope    string `gorm:"type:varchar(16);uniqueIndex;not null;default:'default'" json:"scope"`
	TenantID uint   `gorm:"not null;default:1;index" json:"tenant_id"`

	InputTokensRemaining  int64     `gorm:"not null;default:0" json:"input_tokens_remaining"`
	OutputTokensRemaining int64     `gorm:"not null;default:0" json:"output_tokens_remaining"`
	SyncedAt              time.Time `json:"synced_at"`
	CreatedAt             time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt             time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (LLMWallet) TableName() string { return "llm_wallets" }

// LLMWalletTxn LLM 余额变动的本地流水
type LLMWalletTxn struct {
	ID       uint `gorm:"primaryKey" json:"id"`
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	Reason             string    `gorm:"type:varchar(16);not null" json:"reason"`
	DeltaInput         int64     `gorm:"not null;default:0" json:"delta_input"`
	DeltaOutput        int64     `gorm:"not null;default:0" json:"delta_output"`
	BalanceInputAfter  int64     `gorm:"not null;default:0" json:"balance_input_after"`
	BalanceOutputAfter int64     `gorm:"not null;default:0" json:"balance_output_after"`
	Note               string    `gorm:"type:varchar(255)" json:"note"`
	RefType            string    `gorm:"type:varchar(16)" json:"ref_type"`
	RefID              string    `gorm:"type:varchar(64);index" json:"ref_id"`
	CreatedAt          time.Time `gorm:"autoCreateTime;index" json:"created_at"`
}

func (LLMWalletTxn) TableName() string { return "llm_wallet_txns" }

// LLMSource provider 来源（与 platform-server 的 model.LLMSource 同名同义）
const (
	LLMSourceSelf     = "self"     // 商户自带 Key：只统计，不计费
	LLMSourcePlatform = "platform" // 平台垫资：计量 + 扣费
)

// LLMUsageRecord 平台垫资模型的用量明细（本地镜像）
//
// 三张表的分工，说清楚才不会有人去合并它们：
//   - llm_routing_logs   记每一次调用的路由与成本（自产 + 平台垫资都有）
//   - llm_usage_records  记**每一次**的 token 总量与成本合计，AI 产能看板用
//     （迁移 v3.4.0 建的那张，主键是 uuid，形状完全不同）
//   - platform_llm_usage_records（**本表**）记平台垫资的**分列**明细：
//     输入/输出 token 与成本各自一列、request_id 幂等、可归因到人
//
// 为什么不复用 llm_usage_records：那张表是聚合向的（只有总量，没有 provider /
// 幂等键 / 归因字段），塞不下明细；而把明细硬塞进去会让两套语义的列混在一张表里，
// 将来加字段时必然互相打架。代价只是多一张表，而"主键类型不同"的代价是
// **静默改表** —— AutoMigrate 会按模型把它改成 bigint PK，历史行的 uuid 值随之失效，
// 而 AI 产能看板不会报任何错。
type PlatformLLMUsageRecord struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID int64 `gorm:"primaryKey;autoIncrement" json:"id"`
	// RequestID 幂等键：有 trace_id 用 trace_id，没有则服务端兜底生成。
	// uniqueIndex 是「重试不重复记账」的物理保证 —— 网络抖动导致的一次
	// 自动重发在 dispatcher 层是两次调用，但在账上只能算一次。
	RequestID string `gorm:"type:varchar(128);uniqueIndex;not null" json:"request_id"`

	Provider string `gorm:"type:varchar(64);index;not null;default:''" json:"provider"`
	Model    string `gorm:"type:varchar(128);not null;default:''" json:"model"`
	PlanCode string `gorm:"type:varchar(32);index" json:"plan_code"`
	Scenario string `gorm:"type:varchar(64);index" json:"scenario"`
	TraceID  string `gorm:"type:varchar(64);index" json:"trace_id"`
	AgentID  string `gorm:"type:varchar(64);index" json:"agent_id"`
	UserID   string `gorm:"type:varchar(64);index" json:"user_id"`

	PromptTokens     int64 `gorm:"not null;default:0" json:"prompt_tokens"`
	CompletionTokens int64 `gorm:"not null;default:0" json:"completion_tokens"`

	// 输入/输出分列成本（分）。不是从总价按比例拆出来的 —— 那等价于单一单价。
	InputCostCents  int64 `gorm:"not null;default:0" json:"input_cost_cents"`
	OutputCostCents int64 `gorm:"not null;default:0" json:"output_cost_cents"`
	TotalCostCents  int64 `gorm:"not null;default:0" json:"total_cost_cents"`

	OccurredAt time.Time `gorm:"not null;index" json:"occurred_at"`
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (PlatformLLMUsageRecord) TableName() string { return "platform_llm_usage_records" }
