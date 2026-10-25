// reflux clusterer：指纹 + pg_trgm 相似归簇（T1）。
//
// 在线单条归簇：新问题入库时一次 similarity 查询找已有簇（阈值 0.55），
// 命中即归属（cluster_id 回写 + 簇计数累加）；未命中建新簇。
// 簇代表 = 首条目（cluster_hash），样本问句维护 ≤10 条。
package reflux

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"hivemtk-user/internal/model"
)

// similarityThreshold trgm 归簇阈值（设计文档 §4.3）
const similarityThreshold = 0.55

// maxSamples 簇样本问句上限
const maxSamples = 10

// Clusterer 归簇器
type Clusterer struct {
	db *gorm.DB
}

// NewClusterer 构造
func NewClusterer(db *gorm.DB) *Clusterer { return &Clusterer{db: db} }

// Assign 把 (hash, question) 归入最相似的已有簇或新建簇。
// 幂等：同 hash 重复调用不重复计数（unknown_questions 已 ON CONFLICT 去重，
// 这里以 question_hash 未归属簇为前提做一次归属）。
func (cl *Clusterer) Assign(ctx context.Context, hash, question string) error {
	// 1) 该问题已归属簇则跳过
	var clusterID int64
	if err := cl.db.WithContext(ctx).
		Model(&model.UnknownQuestion{}).
		Select("COALESCE(cluster_id, 0)").
		Where("question_hash = ?", hash).
		Limit(1).
		Scan(&clusterID).Error; err != nil {
		return fmt.Errorf("lookup cluster_id: %w", err)
	}
	if clusterID > 0 {
		// 刷新簇频次合计（问题频次可能 +1 了）
		if err := cl.db.WithContext(ctx).Model(&model.UnknownQuestionCluster{}).
			Where("id = ?", clusterID).
			Update("total_frequency", gorm.Expr(`(SELECT COALESCE(SUM(frequency), 0)
				FROM unknown_questions q
				WHERE q.cluster_id = unknown_question_clusters.id
				  AND q.tenant_id = unknown_question_clusters.tenant_id)`)).Error; err != nil {
			return fmt.Errorf("refresh cluster freq: %w", err)
		}
		return nil
	}

	// 2) trgm 相似找簇（只找 pending_review 簇）
	var hit struct {
		ID    int64  `gorm:"column:id"`
		Title string `gorm:"column:title"`
	}
	err := cl.db.WithContext(ctx).
		Model(&model.UnknownQuestionCluster{}).
		Select("id", "title").
		Where("status = ?", model.UQStatusPendingReview).
		Where("similarity(title, ?) > ?", question, similarityThreshold).
		Order(clause.OrderBy{
			Expression: clause.Expr{SQL: "similarity(title, ?) DESC", Vars: []interface{}{question}},
		}).
		Limit(1).
		Scan(&hit).Error
	if err != nil {
		return fmt.Errorf("similarity search: %w", err)
	}

	if hit.ID > 0 {
		return cl.joinCluster(ctx, hit.ID, hash, question)
	}
	return cl.createCluster(ctx, hash, question)
}

func (cl *Clusterer) joinCluster(ctx context.Context, clusterID int64, hash, question string) error {
	return cl.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.UnknownQuestion{}).
			Where("question_hash = ? AND cluster_id = 0", hash).
			Update("cluster_id", clusterID).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.UnknownQuestionCluster{}).
			Where("id = ?", clusterID).
			Updates(map[string]any{
				"question_count": gorm.Expr(
					`(SELECT COUNT(*) FROM unknown_questions q WHERE q.cluster_id = ?)`, clusterID),
				"total_frequency": gorm.Expr(
					`(SELECT COALESCE(SUM(frequency), 0) FROM unknown_questions q WHERE q.cluster_id = ?)`, clusterID),
				"sample_questions": gorm.Expr(
					`(SELECT COALESCE(array_agg(q), unknown_question_clusters.sample_questions) FROM (
						SELECT question AS q FROM unknown_questions
						WHERE cluster_id = ? ORDER BY last_seen_at DESC LIMIT ?
					) s)`, clusterID, maxSamples),
			}).Error; err != nil {
			return err
		}
		return nil
	})
}

func (cl *Clusterer) createCluster(ctx context.Context, hash, question string) error {
	return cl.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var q struct {
			TenantID uint  `gorm:"column:tenant_id"`
			AgentID  uint  `gorm:"column:agent_id"`
			ID       int64 `gorm:"column:id"`
		}
		if err := tx.Model(&model.UnknownQuestion{}).
			Select("tenant_id", "COALESCE(agent_id, 0) AS agent_id", "id").
			Where("question_hash = ?", hash).
			Limit(1).
			Scan(&q).Error; err != nil {
			return err
		}
		now := time.Now()
		cluster := model.UnknownQuestionCluster{
			TenantID:        q.TenantID,
			ClusterHash:     hash,
			Title:           truncate(question, 255),
			AgentID:         q.AgentID,
			Status:          model.UQStatusPendingReview,
			QuestionCount:   1,
			TotalFrequency:  1,
			SampleQuestions: []string{truncate(question, 255)},
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		res := tx.Select(
			"TenantID", "ClusterHash", "Title", "AgentID", "Status", "QuestionCount",
			"TotalFrequency", "SampleQuestions", "CreatedAt", "UpdatedAt",
		).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "cluster_hash"}},
			DoNothing: true,
		}).Create(&cluster)
		if res.Error != nil {
			return res.Error
		}
		var clusterID int64
		if err := tx.Model(&model.UnknownQuestionCluster{}).
			Select("id").
			Where("cluster_hash = ?", hash).
			Limit(1).
			Scan(&clusterID).Error; err != nil {
			return err
		}
		return tx.Model(&model.UnknownQuestion{}).
			Where("question_hash = ? AND cluster_id = 0", hash).
			Update("cluster_id", clusterID).Error
	})
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
