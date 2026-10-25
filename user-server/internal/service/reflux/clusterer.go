// reflux clusterer：指纹 + pg_trgm 相似归簇（T1）。
//
// 在线单条归簇：新问题入库时一次 similarity 查询找已有簇（阈值 0.55），
// 命中即归属（cluster_id 回写 + 簇计数累加）；未命中建新簇。
// 簇代表 = 首条目（cluster_hash），样本问句维护 ≤10 条。
// 2026-10-25：DB 访问下沉 repository/reflux.go；构造器保持 NewClusterer(db) 签名。
package reflux

import (
	"context"
	"fmt"

	"hivemtk-user/internal/repository"
)

// similarityThreshold trgm 归簇阈值（设计文档 §4.3）
const similarityThreshold = 0.55

// maxSamples 簇样本问句上限
const maxSamples = 10

// Clusterer 归簇器
type Clusterer struct {
	repo *repository.RefluxRepository
}

// NewClusterer 构造（db 为瞬时参量：即建 repo，不存句柄）
func NewClusterer(repo *repository.RefluxRepository) *Clusterer {
	return &Clusterer{repo: repo}
}

// Assign 把 (hash, question) 归入最相似的已有簇或新建簇。
// 幂等：同 hash 重复调用不重复计数（unknown_questions 已 ON CONFLICT 去重，
// 这里以 question_hash 未归属簇为前提做一次归属）。
func (cl *Clusterer) Assign(ctx context.Context, hash, question string) error {
	// 1) 该问题已归属簇则只刷新频次合计
	clusterID, err := cl.repo.FindClusterIDByHash(ctx, hash)
	if err != nil {
		return fmt.Errorf("lookup cluster_id: %w", err)
	}
	if clusterID > 0 {
		if err := cl.repo.RefreshClusterFrequency(ctx, clusterID); err != nil {
			return fmt.Errorf("refresh cluster freq: %w", err)
		}
		return nil
	}

	// 2) trgm 相似找簇（只找 pending_review 簇）
	hitID, err := cl.repo.FindSimilarPendingCluster(ctx, question, similarityThreshold)
	if err != nil {
		return fmt.Errorf("similarity search: %w", err)
	}
	if hitID > 0 {
		return cl.repo.JoinCluster(ctx, hitID, hash, maxSamples)
	}
	return cl.repo.CreateClusterForHash(ctx, hash, truncate(question, 255), []string{truncate(question, 255)})
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
