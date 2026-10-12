package model

import (
	"time"
)

// ConfigParamGroup 参数分组元数据表。
//
// 为什么单独一张表而不是在 config_params 上加两列：
//
//  1. 冗余：35 个组 × 每个组几十条参数，组名与中文标签会重复几百遍。
//     一处改标签要改几百行，或者干脆忘了改。
//  2. 顺序是组级属性：参数表里没有「这个组排第几」的位置，而分组侧边栏要排序。
//  3. 组的说明是组级文案（「什么时候该动这几项」），挂在单条参数上是错位。
//
// 建表走启动期 AutoMigrate（见 internal/pkg/db/migrate.go 的 RegisterExtraModels），
// 与 ConfigParam / Dictionary 同一口径。本项目的表由模型持有，迁移只改索引——
// 自建表迁移会被 TestFullMigrationChainUpThenRollback 的「Down 不删不属于它的表」判红。
type ConfigParamGroup struct {
	ID          uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	Group       string `gorm:"column:param_group;type:varchar(50);not null;uniqueIndex" json:"group"`
	Label       string `gorm:"type:varchar(100);not null" json:"label"`
	Description string `gorm:"type:varchar(500)" json:"description"`
	// SortOrder 越小越靠前。0 是「没排过」，UI 把它排到末尾而不是排最前——
	// 排最前会让一个刚建的组顶掉所有老组的位置。
	SortOrder int       `gorm:"default:0" json:"sort_order"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (ConfigParamGroup) TableName() string { return "config_param_groups" }
