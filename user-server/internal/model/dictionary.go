package model

import "time"

// Dictionary 业务字典。
//
// 与 config_params 的分界（这是两套机制并行以来第一次把边界写下来）：
//
//	config_params      一个 key 一个「标量值」（int/float/bool/duration/string）
//	                   → 管的是「这一个旋钮拧到几」
//	dictionaries       一个 key 一组「枚举项」+「项之间的合法跃迁」
//	                   → 管的是「有哪几种状态」「从 A 能不能到 B」
//
// 为什么不让字典寄生在 config_params 上：一个 5 状态的审批流要 5 条 string 参数 +
// 一张跃迁表（10 条有序二元组），而参数中心既没有顺序也没有"成组"的概念，
// 翻出来是 15 行平铺的记录，运维看不出它们是一套。字典表用 parent_code 分层表达
// 「选项」与「跃迁」两种行，一次查询就能拿到整张状态机。
//
// 复合唯一 (parent_code, code) 而不是 (code)：不同域完全可能各有一个 "pending"，
// 单列唯一会逼着人给键名加域前缀（approval_pending / order_pending），
// 那正是 config_params 在 v3.47.0 之前踩过的坑。
type Dictionary struct {
	ID          uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	ParentCode  string `gorm:"column:parent_code;type:varchar(100);not null;index;uniqueIndex:idx_dict_parent_code,priority:1" json:"parent_code"`
	Code        string `gorm:"type:varchar(100);not null;uniqueIndex:idx_dict_parent_code,priority:2" json:"code"`
	Name        string `gorm:"type:varchar(200);not null" json:"name"`
	Description string `gorm:"type:varchar(500)" json:"description"`
	SortOrder   int    `gorm:"default:0" json:"sort_order"`
	// Enabled=false 的项不参与新写入，但存量行仍可能是它——所以下发时必须带上，
	// 让前端能把存量值也译成人话；只挡"新建时选它"。
	Enabled      bool      `gorm:"default:true" json:"enabled"`
	SystemLocked bool      `gorm:"default:false" json:"system_locked"`
	UpdatedBy    uint      `json:"updated_by"`
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (Dictionary) TableName() string { return "dictionaries" }

// DictionaryTransition 字典项之间的合法跃迁。
//
// 为什么单独一张表而不是在 Dictionary 上加一列 allowed_targets：
// 跃迁是**二元关系**（A→B 合法不意味着 B→A 合法），塞进 JSON 列就退回了
// system_config_kv 那种"整份 blob 没有约束"的形态——加一条跃迁要重写整列，
// 也查不出"谁现在引用着这条跃迁"。
type DictionaryTransition struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	ParentCode  string    `gorm:"column:parent_code;type:varchar(100);not null;index;uniqueIndex:idx_dict_trans,priority:1" json:"parent_code"`
	FromCode    string    `gorm:"type:varchar(100);not null;uniqueIndex:idx_dict_trans,priority:2" json:"from_code"`
	ToCode      string    `gorm:"type:varchar(100);not null" json:"to_code"`
	Description string    `gorm:"type:varchar(500)" json:"description"`
	UpdatedBy   uint      `json:"updated_by"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (DictionaryTransition) TableName() string { return "dictionary_transitions" }
