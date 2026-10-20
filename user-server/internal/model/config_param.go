package model

import (
	"time"
)

// ConfigParam 动态阈值参数表（替代分散的 const 硬编码）。
//
// 设计原则：
//  1. 单一表通存所有类型（int/float/bool/string/json/duration），value 存字符串按需解析
//     （value_type 的取值集合由 service.validateValue 穷举把关：新增一个类型却没在
//     校验里加分支，写侧会直接报「unknown value_type」而不是静默放行）
//  2. group + key 联合唯一，key 采用点号命名（bridge.polling_max_timeout）
//  3. default_value 保证每次启动/重置都有合法值
//  4. min/max/value_type 约束 + UI 渲染提示
//  5. read_only=true 标记由系统自动推导/锁死（暂不开放给用户改）
type ConfigParam struct {
	ID           uint    `gorm:"primaryKey;autoIncrement" json:"id"`
	Group        string  `gorm:"column:param_group;type:varchar(50);not null;index;uniqueIndex:idx_group_key,priority:1" json:"group"`
	Key          string  `gorm:"type:varchar(100);not null;uniqueIndex:idx_group_key,priority:2" json:"key"`
	Name         string  `gorm:"type:varchar(200);not null" json:"name"`
	Description  string  `gorm:"type:varchar(500)" json:"description"`
	ValueType    string  `gorm:"type:varchar(20);not null;default:int" json:"value_type"`
	Value        string  `gorm:"column:param_value;type:text;not null" json:"value"`
	DefaultValue string  `gorm:"column:default_value;type:text;not null" json:"default_value"`
	Min          *string `gorm:"type:varchar(50)" json:"min,omitempty"`
	Max          *string `gorm:"type:varchar(50)" json:"max,omitempty"`
	Step         *string `gorm:"type:varchar(20)" json:"step,omitempty"`
	// Unit 只回答「填数字时用户怎么理解这个数字」，不参与任何计算：
	// 86400 后面挂「秒」读起来是噪音，「1 天」才读得懂。UI 靠它决定输入框后面挂什么。
	Unit string `gorm:"type:varchar(20)" json:"unit,omitempty"`
	// Risk 回答「填错了会怎样」，三档：""（普通）/ "warning"（行为会变但不会挂）/
	// "danger"（会挂：锁 TTL 填 0 变成永不过期、上传上限配错会把上传全打死）。
	// UI 靠它决定要不要弹二次确认。空串是有意义的「普通」，所以存串不存 bool。
	Risk string `gorm:"type:varchar(10)" json:"risk,omitempty"`
	// Deprecated 标记这个旋钮已下架。
	//
	// 「下架」与「删行」的区别是本页最重要的一处设计：历史值与审计留痕必须能查，
	// 一条去年被改成 300 的锁 TTL 不能因为旋钮下线就查不到了。但下架的行默认不
	// 出现在管理端列表里——页面上挂着一个改了不生效的旋钮，比没有这个旋钮更糟。
	// DeprecatedAt 记下架时刻（只在 false→true 那次写入，之后不再刷新，
	// 否则每次启动都把「什么时候下架的」冲成启动时间），DeprecatedReason 记为什么。
	Deprecated       bool       `gorm:"default:false;index" json:"deprecated"`
	DeprecatedAt     *time.Time `json:"deprecated_at,omitempty"`
	DeprecatedReason string     `gorm:"type:varchar(500)" json:"deprecated_reason,omitempty"`
	ReadOnly         bool       `gorm:"default:false" json:"read_only"`
	Restart          bool       `gorm:"default:false" json:"restart"`
	Category         string     `gorm:"type:varchar(100)" json:"category"`
	UpdatedBy        uint       `json:"updated_by"`
	CreatedAt        time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt        time.Time  `gorm:"autoUpdateTime" json:"updated_at"`

	// TenantScope／TenantScopeReason 不是库里的列，而是每次读出时按代码现算的结论：
	// 「这个键能不能按租户改」取决于读取点有没有把请求的租户带进参数中心，
	// 事实源在 internal/service/config_param_tenant_scope.go。
	//
	// 做成列会让库里能声明一个代码并不具备的能力——运维填了 platform 而读点其实带 ctx，
	// 或反过来填了 inherit 而读点用的是 context.Background()，两处各说各话且谁都不报错。
	// 与管理端已有的 ReadOnly／Restart 那类「这行诚实说明自己」的列同一个思路，
	// 只是这一条的答案在代码里，不在数据里。
	TenantScope       string `gorm:"-" json:"tenant_scope"`
	TenantScopeReason string `gorm:"-" json:"tenant_scope_reason,omitempty"`
}

func (ConfigParam) TableName() string { return "config_params" }

// ConfigParamAuditLog 变更审计
//
// ParamGroup 是补上的：config_params 的唯一键已经是 (param_group, key) 复合
// （migration v3.47.0 + 模型标签一致），所以「bridge.max_tokens」和
// 「misc.max_tokens」是两条合法的、不同的参数行。只记 ParamKey 的审计行
// 在这种库里是歧义的——看到 `max_tokens` 被改成 800，无法判断改的是哪一组的那个。
// 补这一列让审计重新可追；AutoMigrate 加可空列，旧行留空即「历史数据，组别未知」。
type ConfigParamAuditLog struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	ParamGroup string    `gorm:"column:param_group;type:varchar(50);index" json:"param_group"`
	ParamKey   string    `gorm:"type:varchar(100);index;not null" json:"param_key"`
	OldValue   string    `gorm:"type:text" json:"old_value"`
	NewValue   string    `gorm:"type:text" json:"new_value"`
	Action     string    `gorm:"type:varchar(20);not null" json:"action"`
	ActorID    uint      `json:"actor_id"`
	CreatedAt  time.Time `gorm:"autoCreateTime;index" json:"created_at"`
}

func (ConfigParamAuditLog) TableName() string { return "config_param_audit_logs" }
