package dto

import (
	"errors"
	"time"
)

// FeedbackSignalKey 反馈信号类型
type FeedbackSignalKey string

const (
	FBSignalLike         FeedbackSignalKey = "like"
	FBSignalDislike      FeedbackSignalKey = "dislike"
	FBSignalRating       FeedbackSignalKey = "rating"
	FBSignalComplaint    FeedbackSignalKey = "complaint"
	FBSignalConversion   FeedbackSignalKey = "conversion"
	FBSignalReplyRate    FeedbackSignalKey = "reply_rate"
	FBSignalDuration     FeedbackSignalKey = "duration"
	FBSignalTransfer     FeedbackSignalKey = "transfer"
	FBSignalChampionMark FeedbackSignalKey = "champion_mark"
	FBSignalScriptAdopt  FeedbackSignalKey = "script_adopt"
	FBSignalToolCall     FeedbackSignalKey = "tool_call"
	FBSignalIntentMatch  FeedbackSignalKey = "intent_match"
	// FBSignalCollection 回款金额信号（T-P8-02）：value = 本次结算后累计 settled 金额，
	// reward 按 log10 缩放（weight 作缩放系数），封顶见 computeReward。
	FBSignalCollection FeedbackSignalKey = "collection"
	// FBSignalCollectionLost 丢单负样本（T-P8-02）：MarkLost 真跃迁时 emission，
	// flat 负 reward 走 default 分支（weight 即 reward）。
	FBSignalCollectionLost FeedbackSignalKey = "collection_lost"
)

// FeedbackEventType 反馈事件类型
type FeedbackEventType string

const (
	FBEventTypeExplicit FeedbackEventType = "explicit"
	FBEventTypeImplicit FeedbackEventType = "implicit"
)

// CollectRequest 反馈采集请求（外部 → FeedbackCollector）
//
// 用法：SalesEngine / API handler 构造此请求提交给 FeedbackCollector.Collect
type CollectRequest struct {
	SessionID         string            `json:"session_id"`
	CustomerID        string            `json:"customer_id"`
	SOPID             uint              `json:"sop_id"`
	ExecutionID       uint              `json:"execution_id"`
	Variant           string            `json:"variant"`
	PromptCandidateID uint              `json:"prompt_candidate_id"`
	EventType         FeedbackEventType `json:"event_type"`
	SignalKey         FeedbackSignalKey `json:"signal_key"`
	SignalValue       any               `json:"signal_value"`
	AIReply           string            `json:"ai_reply"`
	CustomerMsg       string            `json:"customer_msg"`
	Metadata          map[string]any    `json:"metadata"`
	CreatedBy         uint              `json:"created_by"`
}

// ChampionAnalysisReport 销冠对话分析报告（service → 外部）
//
// 由 ChampionDialogueAnalyzer.AnalyzePipeline 返回
type ChampionAnalysisReport struct {
	RunAt            time.Time            `json:"run_at"`
	Since            time.Time            `json:"since"`
	CandidateCount   int                  `json:"candidate_count"`
	ClusterCount     int                  `json:"cluster_count"`
	PersistedCount   int                  `json:"persisted_count"`
	ExtractedScripts []ExtractedScriptDTO `json:"extracted_scripts"`
	Errors           []string             `json:"errors"`
}

// ExtractedScriptDTO LLM 提取的话术（service → script_templates 入库）
type ExtractedScriptDTO struct {
	Title              string   `json:"title"`
	Content            string   `json:"content"`
	Scenario           string   `json:"scenario"`
	TriggerKeywords    []string `json:"trigger_keywords"`
	JourneyStage       string   `json:"journey_stage"`
	EffectivenessScore float64  `json:"effectiveness_score"`
	ClusterID          uint     `json:"cluster_id"`
}

// OptimizationReport SOP 优化报告（service → 外部）
//
// 由 SOPAutoOptimizer.ProcessPendingSuggestions 返回
type OptimizationReport struct {
	RunAt           time.Time `json:"run_at"`
	PendingCount    int       `json:"pending_count"`
	AppliedCount    int       `json:"applied_count"`
	FailedCount     int       `json:"failed_count"`
	RolledBackCount int       `json:"rolled_back_count"`
	PromotedCount   int       `json:"promoted_count"`
	Errors          []string  `json:"errors"`
}

var (
	ErrFeedbackRequestNil     = errors.New("feedback_loop: collect request is nil")
	ErrFeedbackSessionEmpty   = errors.New("feedback_loop: session_id is empty")
	ErrFeedbackCustomerEmpty  = errors.New("feedback_loop: customer_id is empty")
	ErrFeedbackEventTypeEmpty = errors.New("feedback_loop: event_type is empty")
	ErrFeedbackSignalKeyEmpty = errors.New("feedback_loop: signal_key is empty")
)
