package repository

import (
	"context"
	"time"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ChampionDialogueRow 销冠对话候选行（feedback_signals + feedback_events 聚合快照）
//
// 由 FetchChampionDialogueCandidates 通过 Raw SQL 填充，
// 字段顺序与 SQL SELECT 列对应。
type ChampionDialogueRow struct {
	SessionID   string
	CustomerID  string
	CustomerMsg string
	AIReply     string
	Reward      float64
	IsChampion  bool
	Scenario    string
	CreatedAt   time.Time
}

// FetchChampionDialogueCandidates 从 feedback_signals 拉取高价值候选对话
//
// SQL：聚合 reward + 关联最近一条 feedback_events 取 customer_msg / ai_reply 快照
// 按 aggregated_reward DESC 排序，取最多 maxDialogues 条
func (r *FeedbackLoopRepository) FetchChampionDialogueCandidates(ctx context.Context, scenario string, minReward float64, since time.Time, maxDialogues int) ([]ChampionDialogueRow, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	var rows []ChampionDialogueRow
	err := r.db.WithContext(ctx).Table("feedback_signals fs").
		Select("fs.session_id, fs.customer_id, "+
			"(SELECT customer_msg FROM feedback_events WHERE session_id = fs.session_id ORDER BY created_at DESC LIMIT 1) AS customer_msg, "+
			"(SELECT ai_reply FROM feedback_events WHERE session_id = fs.session_id ORDER BY created_at DESC LIMIT 1) AS ai_reply, "+
			"fs.aggregated_reward AS reward, fs.is_champion, "+
			"COALESCE(fs.signal_breakdown->>'scenario', ?) AS scenario, fs.created_at", scenario).
		Where("fs.aggregated_reward >= ? AND fs.created_at >= ?", minReward, since).
		Order("fs.aggregated_reward DESC").
		Limit(maxDialogues).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// ChampionDialoguePersist 销冠对话持久化参数
//
// EmbeddingLiteral 为 pgvector 字面量 '[v1,v2,...]'，由调用方格式化
type ChampionDialoguePersist struct {
	Fingerprint        string
	SessionID          string
	CustomerID         string
	Scenario           string
	CustomerMsg        string
	ChampionReply      string
	EmbeddingLiteral   string
	ClusterID          uint
	Reward             float64
	ConversionAchieved bool
}

// PersistChampionDialogue 持久化销冠对话到 champion_dialogues
//
// 用原生 SQL 写入（embedding vector 类型 GORM 不支持，用 ?::vector 强转）
// ON CONFLICT (dialogue_fingerprint) DO UPDATE：重复对话按 reward / conversion_achieved 覆盖
func (r *FeedbackLoopRepository) PersistChampionDialogue(ctx context.Context, p ChampionDialoguePersist) error {
	if r == nil || r.db == nil {
		return nil
	}
	// embedding vector 类型以 ?::vector 显式强转（gorm.Expr 内联，参数仍走绑定）
	return r.db.WithContext(ctx).Table("champion_dialogues").
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "tenant_id"}, {Name: "dialogue_fingerprint"}},
			DoUpdates: clause.Assignments(map[string]any{
				"reward":              p.Reward,
				"conversion_achieved": p.ConversionAchieved,
			}),
		}).
		Create(map[string]any{
			"dialogue_fingerprint": p.Fingerprint,
			"session_id":           p.SessionID,
			"customer_id":          p.CustomerID,
			"staff_id":             0,
			"staff_name":           "",
			"scenario":             p.Scenario,
			"journey_stage":        "",
			"customer_msg":         p.CustomerMsg,
			"champion_reply":       p.ChampionReply,
			"context_msgs":         "{}",
			"embedding":            gorm.Expr("?::vector", p.EmbeddingLiteral),
			"cluster_id":           p.ClusterID,
			"reward":               p.Reward,
			"conversion_achieved":  p.ConversionAchieved,
			"extracted_scripts":    "[]",
			"created_at":           time.Now(),
		}).Error
}

// GetChampionDialogueIDByCluster 反查 cluster_id 对应的最新 champion_dialogue.id
//
// 用于 saveScriptsToTemplate 回流话术时关联销冠对话 ID
// 未找到时返回 (0, nil)（不视为错误，调用方按需处理）
func (r *FeedbackLoopRepository) GetChampionDialogueIDByCluster(ctx context.Context, clusterID uint) (uint, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	var dialogueID uint
	err := r.db.WithContext(ctx).Model(&model.ChampionDialogue{}).
		Where("cluster_id = ?", clusterID).
		Order("id DESC").
		Limit(1).
		Pluck("id", &dialogueID).Error
	return dialogueID, err
}

// InsertScriptTemplate 写入话术库 script_templates（原生 SQL）
//
// 用原生 SQL 写入：script_templates 模型在 model 包，本包不直接依赖该子包避免循环依赖
// Source 固定为 'champion_extract'，is_system=TRUE
//
// 参数：
//   - category, title, content, tags    - 话术元信息
//   - effectivenessScore                 - LLM 评分（0-1，调用方已做边界裁剪）
//   - journeyStage                       - 客户旅程阶段
//   - championDialogueID                 - 关联销冠对话 ID
func (r *FeedbackLoopRepository) InsertScriptTemplate(ctx context.Context, category, title, content, tags string, effectivenessScore float64, journeyStage string, championDialogueID uint) error {
	if r == nil || r.db == nil {
		return nil
	}
	now := time.Now()
	return r.db.WithContext(ctx).Table("script_templates").
		Create(map[string]any{
			"category":             category,
			"title":                title,
			"content":              content,
			"tags":                 tags,
			"rating":               effectivenessScore,
			"is_system":            true,
			"variables":            "[]",
			"source":               "champion_extract",
			"effectiveness_score":  effectivenessScore,
			"trigger_keywords":     tags,
			"journey_stage":        journeyStage,
			"champion_dialogue_id": championDialogueID,
			"created_at":           now,
			"updated_at":           now,
		}).Error
}
