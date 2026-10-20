package model

import "time"

// TenantConfigParam 租户级参数覆盖。
//
// 分工：
//   - config_params 是定义目录 + 平台级默认值（实例级，运维/平台维护）
//   - 本表只存「这个租户把哪些键改成了什么」，未覆盖的键读 config_params.value
//
// 为什么不在 config_params 上直接加 tenant_id 并逐租户复制全量定义：
// 种子会随版本增删，逐租户复制意味着每次升级都要给每个租户补行、下架标记也要
// 逐租户同步；一旦漏一个租户，该租户读到的就是代码兜底值而不是平台默认值。
// 覆盖表把「定义」与「取值」分开，租户只承担自己改过的那几行。
type TenantConfigParam struct {
	ID       uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID uint   `gorm:"column:tenant_id;not null;index;uniqueIndex:idx_tenant_param_group_key,priority:1" json:"tenant_id"`
	Group    string `gorm:"column:param_group;type:varchar(50);not null;uniqueIndex:idx_tenant_param_group_key,priority:2" json:"group"`
	Key      string `gorm:"type:varchar(100);not null;uniqueIndex:idx_tenant_param_group_key,priority:3" json:"key"`
	Value    string `gorm:"column:param_value;type:text;not null" json:"value"`
	// UpdatedBy 记录改这个租户值的管理员；与平台级默认值的 updated_by 分开追溯。
	UpdatedBy uint      `json:"updated_by"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (TenantConfigParam) TableName() string { return "tenant_config_params" }

// TenantConfigParamAuditLog 租户参数变更审计。
//
// 不复用 config_param_audit_logs：那张表记的是平台级默认值的变更，租户管理员
// 的每一次调参都写进去会互相污染——A 租户的运维打开审计页会看到 B 租户改了什么。
// 改前/改后值同样经 dbencrypt 加密落库（部分参数含 API Key）。
type TenantConfigParamAuditLog struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID   uint      `gorm:"column:tenant_id;not null;index" json:"tenant_id"`
	ParamGroup string    `gorm:"column:param_group;type:varchar(50);index" json:"param_group"`
	ParamKey   string    `gorm:"type:varchar(100);index;not null" json:"param_key"`
	OldValue   string    `gorm:"type:text" json:"old_value"`
	NewValue   string    `gorm:"type:text" json:"new_value"`
	Action     string    `gorm:"type:varchar(20);not null" json:"action"`
	ActorID    uint      `json:"actor_id"`
	CreatedAt  time.Time `gorm:"autoCreateTime;index" json:"created_at"`
}

func (TenantConfigParamAuditLog) TableName() string { return "tenant_config_param_audit_logs" }
