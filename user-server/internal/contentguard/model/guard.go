// Package model 内容安全闸（ContentGuard）的持久化模型。
//
// 表清单（五张，全部为租户表，tenant_id 走 GORM 隔离 callback）：
//
//	guard_words      词库（deny 拦截词 / allow 白名单词）
//	guard_policies   审查策略（按 subject_type 各一行：user / merchant）
//	guard_tasks      审查任务队列（异步、可重放、幂等键唯一）
//	guard_records    审查结论（判定 + 命中证据 + 人工复核状态）
//	guard_sanctions  处置（用户/商户的冻结与黑名单，临时 / 永久）
//
// 建表登记：internal/pkg/db/migrate.go 的 allModels()（本仓生产建表只跑 GORM
// AutoMigrate）+ internal/pkg/db/migrate_test.go 的 mustCover（有生产写入路径即须登记）。
package model

import "time"

// 词条类型。
const (
	// ListKindDeny 拦截词：命中即按策略处置。
	ListKindDeny = "deny"
	// ListKindAllow 白名单词：命中区间与 deny 命中重叠时抑制该次命中（误杀的第一道闸）。
	ListKindAllow = "allow"
)

// 审查主体类型。用户与商户是两条不同的规则线（阈值/动作/冻结时长都不同）。
const (
	SubjectUser     = "user"
	SubjectMerchant = "merchant"
)

// 审查场景。用户侧只审入站消息；商户侧审其对外发布与落库的内容资产。
const (
	SceneInboundMessage = "inbound_message"
	SceneOutboundReply  = "outbound_reply"
	SceneMaterial       = "material"
	SceneKBDocument     = "kb_document"
	SceneMarketingCopy  = "marketing_copy"
)

// 判定结果。
const (
	VerdictPass   = "pass"
	VerdictReview = "review"
	VerdictBlock  = "block"
)

// 处置动作（写进 guard_records.action，供前端/运营按动作分流）。
const (
	ActionNone         = "none"
	ActionReview       = "review"        // 进人工复核队列，不处置
	ActionReject       = "reject"        // 拒收该条用户消息
	ActionBlockPublish = "block_publish" // 商户内容不予发布/下发
	ActionFreezeTemp   = "freeze_temp"   // 临时冻结
	ActionFreezePerm   = "freeze_perm"   // 永久冻结（人工复核后方可解）
)

// 处置类型。
const (
	SanctionKindFreeze    = "freeze"
	SanctionKindBlacklist = "blacklist"
)

// 处置时长档位。
const (
	SanctionLevelTemp      = "temp"
	SanctionLevelPermanent = "permanent"
)

// 任务状态。
const (
	TaskStatusPending = "pending"
	TaskStatusRunning = "running"
	TaskStatusDone    = "done"
	TaskStatusFailed  = "failed"
)

// 人工复核状态。
const (
	ManualStatusNone       = "none" // 自动处置，无需复核
	ManualStatusPending    = "pending"
	ManualStatusConfirmed  = "confirmed"  // 维持原判
	ManualStatusOverturned = "overturned" // 误杀，撤销
)

// GuardHit 一条命中证据（存进 guard_records.hits 的 JSON）。
type GuardHit struct {
	PatternID uint   `json:"pattern_id"`
	Word      string `json:"word"`
	Category  string `json:"category"`
	Severity  int    `json:"severity"`
	// Start/End 原文（rune）区间 [Start, End)；已由归一化索引回映射。
	Start int `json:"start"`
	End   int `json:"end"`
}

// GuardWord 词库词条。
//
// ListKind 让同一张表同时承载拦截词与白名单词：白名单是"这个词组里出现该词不算违规"
// 的唯一表达方式，独立一张表会让抑制逻辑在两处维护。
type GuardWord struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID       uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	ListKind string `gorm:"type:varchar(16);not null;default:'deny';uniqueIndex:uni_guard_word,priority:2" json:"list_kind"`
	Word     string `gorm:"type:varchar(200);not null;uniqueIndex:uni_guard_word,priority:3" json:"word"`
	Category string `gorm:"type:varchar(32);not null;default:'custom'" json:"category"`
	Severity int    `gorm:"not null;default:1" json:"severity"`
	Enabled  bool   `gorm:"not null;default:true;index" json:"enabled"`
	// Source 来源：builtin（内置）/ manual（手工录入）/ import（批量导入）/ api（外部审核回写）
	Source string `gorm:"type:varchar(16);not null;default:'manual'" json:"source"`
	Remark string `gorm:"type:varchar(255)" json:"remark"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 表名。
func (GuardWord) TableName() string { return "guard_words" }

// GuardPolicy 审查策略：**一个主体类型一行**（user / merchant 各一行）。
//
// 为什么按主体类型分表行而不做一张"全局策略 + 主体覆盖"：用户与商户的风险模型、
// 处置手段、法律后果都不同（用户可拒收其消息并冻结会话；商户只能拦其发布内容、
// 冻结其经营能力）。合成一张表后每个字段都要问"这一列对另一类主体是什么语义"，
// 而语义分叉正是线上误处置的源头。
type GuardPolicy struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID          uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	SubjectType string `gorm:"type:varchar(16);not null;uniqueIndex:uni_guard_policy,priority:2" json:"subject_type"`
	Enabled     bool   `gorm:"not null;default:true" json:"enabled"`
	// Scenes 生效场景，逗号分隔。不在其中的场景一律 pass（并记 skipped 原因）。
	Scenes string `gorm:"type:varchar(255);not null;default:''" json:"scenes"`
	// BlockSeverity 命中即拦截的严重级别（severity >= 该值 ⇒ block）
	BlockSeverity int `gorm:"not null;default:3" json:"block_severity"`
	// ReviewSeverity 命中即转人工的严重级别（severity >= 该值 ⇒ review）
	ReviewSeverity int `gorm:"not null;default:2" json:"review_severity"`
	// BlockAction 拦截时的基础动作（user=reject / merchant=block_publish）
	BlockAction string `gorm:"type:varchar(32);not null;default:'reject'" json:"block_action"`
	// EscalatePermanent 窗口内第 N 次拦截 ⇒ 永久冻结（0 = 不自动升级）
	EscalatePermanent int `gorm:"not null;default:0" json:"escalate_permanent"`
	// TempFreezeMinutes 首次拦截的临时冻结时长（0 = 不冻结，只拦当次内容）
	TempFreezeMinutes int `gorm:"not null;default:0" json:"temp_freeze_minutes"`
	// WindowMinutes 累计窗口（计数从这个时间窗内的历史拦截算起）
	WindowMinutes int `gorm:"not null;default:1440" json:"window_minutes"`
	// APISampleRate 外部审核 API 抽样比例（百分比 0..100；0 = 不调用，数据零出域默认）
	APISampleRate int `gorm:"not null;default:0" json:"api_sample_rate"`
	// APIEnabled 外部审核总开关（与 APISampleRate 取与：任一为 0/关即不调用）
	APIEnabled bool `gorm:"not null;default:false" json:"api_enabled"`

	UpdatedBy uint      `gorm:"not null;default:0" json:"updated_by"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 表名。
func (GuardPolicy) TableName() string { return "guard_policies" }

// GuardTask 异步审查任务。
//
// 这是"不阻塞前端任务"的实现载体：业务侧只做一次**单行插入**（毫秒级、无外部调用），
// 真正的扫描/外部 API/处置全在 worker 里跑。TaskKey 是幂等键
// （tenant + scene + subject + 内容摘要），重复提交不产生第二条任务——
// 重试、上游重复投递、前端重发都不会把同一条内容审成两遍、罚成两次。
type GuardTask struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID            uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	TaskKey       string    `gorm:"type:varchar(120);not null;uniqueIndex:uni_guard_task,priority:2" json:"task_key"`
	TraceID       string    `gorm:"type:varchar(64);index" json:"trace_id"`
	SubjectType   string    `gorm:"type:varchar(16);not null;index:idx_guard_task_subject,priority:1" json:"subject_type"`
	SubjectID     string    `gorm:"type:varchar(120);not null;index:idx_guard_task_subject,priority:2" json:"subject_id"`
	Scene         string    `gorm:"type:varchar(32);not null;index" json:"scene"`
	Content       string    `gorm:"type:text" json:"content"`
	ContentDigest string    `gorm:"type:varchar(64);not null" json:"content_digest"`
	Status        string    `gorm:"type:varchar(16);not null;default:'pending';index:idx_guard_task_due,priority:1" json:"status"`
	Attempts      int       `gorm:"not null;default:0" json:"attempts"`
	NextRunAt     time.Time `gorm:"index:idx_guard_task_due,priority:2" json:"next_run_at"`
	LastError     string    `gorm:"type:varchar(500)" json:"last_error"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 表名。
func (GuardTask) TableName() string { return "guard_tasks" }

// GuardRecord 审查结论。
//
// ContentSnippet 只存**脱敏后**的前缀片段（PII 由 safeprompt.MaskPII 处理），
// 全文留在 guard_tasks.content（到期清理），避免把用户原文搬进"运营随手能导出"的表。
type GuardRecord struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID          uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	TaskID      uint64     `gorm:"not null;default:0;index" json:"task_id"`
	TraceID     string     `gorm:"type:varchar(64);index" json:"trace_id"`
	SubjectType string     `gorm:"type:varchar(16);not null;index:idx_guard_record_subject,priority:1" json:"subject_type"`
	SubjectID   string     `gorm:"type:varchar(120);not null;index:idx_guard_record_subject,priority:2" json:"subject_id"`
	Scene       string     `gorm:"type:varchar(32);not null;index" json:"scene"`
	Verdict     string     `gorm:"type:varchar(16);not null;index" json:"verdict"`
	Action      string     `gorm:"type:varchar(32);not null;default:'none'" json:"action"`
	MaxSeverity int        `gorm:"not null;default:0" json:"max_severity"`
	Hits        []GuardHit `gorm:"type:text;serializer:json" json:"hits"`
	// Sources 参与的审查源，逗号分隔：dict / api / manual
	Sources        string `gorm:"type:varchar(64);not null;default:'dict'" json:"sources"`
	APILabels      string `gorm:"type:varchar(255)" json:"api_labels"`
	ContentDigest  string `gorm:"type:varchar(64);not null" json:"content_digest"`
	ContentSnippet string `gorm:"type:varchar(500)" json:"content_snippet"`
	LatencyMs      int    `gorm:"not null;default:0" json:"latency_ms"`
	// SanctionID 本次处置产生的处置行（0 = 未处置）
	SanctionID   uint64     `gorm:"not null;default:0;index" json:"sanction_id"`
	ManualStatus string     `gorm:"type:varchar(16);not null;default:'none';index" json:"manual_status"`
	OperatorID   uint       `gorm:"not null;default:0" json:"operator_id"`
	ReviewedAt   *time.Time `json:"reviewed_at"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 表名。
func (GuardRecord) TableName() string { return "guard_records" }

// GuardSanction 处置（冻结 / 黑名单）。
//
// 与 model.UserBlacklist（坐席手动拉黑单个访客）分开的理由：那张表是**会话运营**动作
// （坐席不想再看到这个人），本表是**内容合规**动作（命中违规内容后的自动/人工处置），
// 两者的解除条件、审计口径、对上下游的影响都不同；合并会让"客服手动拉黑"
// 与"平台内容处罚"在同一行上互相覆盖。
//
// 生效判定唯一依据：Active=true 且（Level=permanent 或 ExpiresAt 在未来）。
// 只要 ExpiresAt 过了就当失效，不依赖任何定时任务（时间判定永远比定时任务可靠）。
type GuardSanction struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID          uint64 `gorm:"primaryKey;autoIncrement" json:"id"`
	SubjectType string `gorm:"type:varchar(16);not null;index:idx_guard_sanction_subject,priority:1" json:"subject_type"`
	SubjectID   string `gorm:"type:varchar(120);not null;index:idx_guard_sanction_subject,priority:2" json:"subject_id"`
	Kind        string `gorm:"type:varchar(16);not null;default:'freeze';index" json:"kind"`
	Level       string `gorm:"type:varchar(16);not null;default:'temp'" json:"level"`
	Reason      string `gorm:"type:varchar(500)" json:"reason"`
	// Source auto（自动处置）/ manual（人工）
	Source        string     `gorm:"type:varchar(16);not null;default:'auto'" json:"source"`
	RecordID      uint64     `gorm:"not null;default:0;index" json:"record_id"`
	TraceID       string     `gorm:"type:varchar(64)" json:"trace_id"`
	OperatorID    uint       `gorm:"not null;default:0" json:"operator_id"`
	OperatorName  string     `gorm:"type:varchar(100)" json:"operator_name"`
	ExpiresAt     *time.Time `gorm:"index" json:"expires_at"`
	Active        bool       `gorm:"not null;default:true;index:idx_guard_sanction_subject,priority:3" json:"active"`
	ReleasedAt    *time.Time `json:"released_at"`
	ReleaseReason string     `gorm:"type:varchar(255)" json:"release_reason"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 表名。
func (GuardSanction) TableName() string { return "guard_sanctions" }

// Effective 处置在 now 时刻是否生效（唯一判定口径，服务层与仓储层共用）。
func (s *GuardSanction) Effective(now time.Time) bool {
	if s == nil || !s.Active {
		return false
	}
	if s.Level == SanctionLevelPermanent {
		return true
	}
	if s.ExpiresAt == nil {
		// temp 档没有到期时间 = 数据缺陷，按永久生效处理（fail-closed：
		// 宁可让人工来解，也不让一条本该生效的处罚因为字段缺失静默失效）。
		return true
	}
	return s.ExpiresAt.After(now)
}
