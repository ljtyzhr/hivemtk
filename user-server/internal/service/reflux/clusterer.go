// reflux clusterer：指纹 + pg_trgm 相似归簇（T1）。
//
// 在线单条归簇：新问题入库时一次 similarity 查询找已有簇（阈值 0.55），
// 命中即归属（cluster_id 回写 + 簇计数累加）；未命中建新簇。
// 簇代表 = 首条目（cluster_hash），样本问句维护 ≤10 条。
package reflux

import (
	"context"
	"fmt"

	"gorm.io/gorm"
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
	if err := cl.db.WithContext(ctx).Raw(
		`SELECT COALESCE(cluster_id,0) FROM unknown_questions WHERE question_hash = ? LIMIT 1`, hash).
		Scan(&clusterID).Error; err != nil {
		return fmt.Errorf("lookup cluster_id: %w", err)
	}
	if clusterID > 0 {
		// 刷新簇频次合计（问题频次可能 +1 了）
		if err := cl.db.WithContext(ctx).Exec(`UPDATE unknown_question_clusters c
			SET total_frequency = (SELECT COALESCE(SUM(frequency),0) FROM unknown_questions q
				WHERE q.cluster_id = c.id AND q.tenant_id = c.tenant_id)
			WHERE c.id = ?`, clusterID).Error; err != nil {
			return fmt.Errorf("refresh cluster freq: %w", err)
		}
		return nil
	}

	// 2) trgm 相似找簇（只找 pending_review 簇）
	var hit struct {
		ID    int64  `gorm:"column:id"`
		Title string `gorm:"column:title"`
	}
	err := cl.db.WithContext(ctx).Raw(`SELECT id, title FROM unknown_question_clusters
		WHERE status = 'pending_review'
		  AND similarity(title, ?) > ?
		ORDER BY similarity(title, ?) DESC
		LIMIT 1`, question, similarityThreshold, question).Scan(&hit).Error
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
		if err := tx.Exec(`UPDATE unknown_questions SET cluster_id = ? WHERE question_hash = ? AND cluster_id = 0`,
			clusterID, hash).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE unknown_question_clusters
			SET question_count = (SELECT COUNT(*) FROM unknown_questions q WHERE q.cluster_id = ?),
			    total_frequency = (SELECT COALESCE(SUM(frequency),0) FROM unknown_questions q WHERE q.cluster_id = ?),
			    sample_questions = (
			      SELECT COALESCE(array_agg(q), sample_questions) FROM (
			        SELECT question AS q FROM unknown_questions WHERE cluster_id = ? ORDER BY last_seen_at DESC LIMIT ?
			      ) s)
			WHERE id = ?`, clusterID, clusterID, clusterID, maxSamples, clusterID).Error; err != nil {
			return err
		}
		return nil
	})
}

func (cl *Clusterer) createCluster(ctx context.Context, hash, question string) error {
	return cl.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var q struct {
			TenantID uint   `gorm:"column:tenant_id"`
			AgentID  uint   `gorm:"column:agent_id"`
			ID       int64  `gorm:"column:id"`
		}
		if err := tx.Raw(`SELECT tenant_id, COALESCE(agent_id,0) AS agent_id, id
			FROM unknown_questions WHERE question_hash = ? LIMIT 1`, hash).Scan(&q).Error; err != nil {
			return err
		}
		res := tx.Exec(`INSERT INTO unknown_question_clusters
			(tenant_id, cluster_hash, title, agent_id, status, question_count, total_frequency,
			 sample_questions, created_at, updated_at)
			VALUES ((?),(?),(?),(?),'pending_review',1,1,ARRAY[?]::text[],NOW(),NOW())
			ON CONFLICT (tenant_id, cluster_hash) DO NOTHING`,
			q.TenantID, hash, truncate(question, 255), q.AgentID, truncate(question, 255))
		if res.Error != nil {
			return res.Error
		}
		var clusterID int64
		if err := tx.Raw(`SELECT id FROM unknown_question_clusters WHERE cluster_hash = ? LIMIT 1`, hash).
			Scan(&clusterID).Error; err != nil {
			return err
		}
		return tx.Exec(`UPDATE unknown_questions SET cluster_id = ? WHERE question_hash = ? AND cluster_id = 0`,
			clusterID, hash).Error
	})
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
