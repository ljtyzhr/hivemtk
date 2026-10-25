package repository

import (
	"context"
	"errors"
	"hivemtk-user/internal/model"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ClueScoreRepository 线索评分仓库
type ClueScoreRepository interface {
	Upsert(ctx context.Context, score *model.ClueScore) error
	GetByClueID(ctx context.Context, clueID string) (*model.ClueScore, error)
	ListByGrade(ctx context.Context, grade string, page, pageSize int) ([]*model.ClueScore, int64, error)
}

type clueScoreRepo struct {
	db *gorm.DB
}

// NewClueScoreRepository 默认构造（全局句柄，仓库既有惯例）。
func NewClueScoreRepository() ClueScoreRepository {
	return &clueScoreRepo{db: GetDB()}
}

// NewClueScoreRepositoryWithDB 用装配点注入的连接创建线索评分仓库
func NewClueScoreRepositoryWithDB(db *gorm.DB) ClueScoreRepository {
	return &clueScoreRepo{db: db}
}

// Upsert 写入评分：先按唯一键插入，命中已有行则更新。
// 唯一索引在租户模式下被 tenancyidx 复合化为 (tenant_id, clue_id)，单租户模式下仍可能是
// 单列 clue_id，因此 ON CONFLICT 不带目标列（DoNothing），两种索引形态都成立。
func (r *clueScoreRepo) Upsert(ctx context.Context, score *model.ClueScore) error {
	if score.ClueID == "" {
		return errors.New("clue_id 不能为空")
	}
	score.ScoredAt = time.Now()
	res := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(score)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected > 0 {
		return nil
	}
	upd := r.db.WithContext(ctx).Model(&model.ClueScore{}).
		Where("clue_id = ?", score.ClueID).
		Updates(map[string]any{
			"account":          score.Account,
			"total_score":      score.TotalScore,
			"grade":            score.Grade,
			"confidence":       score.Confidence,
			"channel_score":    score.ChannelScore,
			"verify_score":     score.VerifyScore,
			"profile_score":    score.ProfileScore,
			"engagement_score": score.EngagementScore,
			"recency_score":    score.RecencyScore,
			"factors_json":     score.FactorsJSON,
			"model_version":    score.ModelVersion,
			"scored_at":        score.ScoredAt,
		})
	if upd.Error != nil {
		return upd.Error
	}
	if upd.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *clueScoreRepo) GetByClueID(ctx context.Context, clueID string) (*model.ClueScore, error) {
	var score model.ClueScore
	if err := r.db.WithContext(ctx).Where("clue_id = ?", clueID).First(&score).Error; err != nil {
		return nil, err
	}
	return &score, nil
}

func (r *clueScoreRepo) ListByGrade(ctx context.Context, grade string, page, pageSize int) ([]*model.ClueScore, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	var total int64
	countTx := r.db.WithContext(ctx).Model(&model.ClueScore{})
	if grade != "" {
		countTx = countTx.Where("grade = ?", grade)
	}
	if err := countTx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var scores []*model.ClueScore
	listTx := r.db.WithContext(ctx).Model(&model.ClueScore{})
	if grade != "" {
		listTx = listTx.Where("grade = ?", grade)
	}
	if err := listTx.Order("total_score DESC, scored_at DESC, id DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&scores).Error; err != nil {
		return nil, 0, err
	}
	return scores, total, nil
}

// ClueEngagementRepository 线索互动事件仓库
type ClueEngagementRepository interface {
	Create(ctx context.Context, evt *model.ClueEngagementEvent) error
	CountByClueID(ctx context.Context, clueID string, since time.Time) (int64, error)
	CountByClueIDsBatch(ctx context.Context, clueIDs []string, since time.Time) (map[string]int64, error)
}

type clueEngagementRepo struct {
	db *gorm.DB
}

// NewClueEngagementRepository 默认构造（全局句柄，仓库既有惯例）。
func NewClueEngagementRepository() ClueEngagementRepository {
	return &clueEngagementRepo{db: GetDB()}
}

// NewClueEngagementRepositoryWithDB 用装配点注入的连接创建线索互动事件仓库
func NewClueEngagementRepositoryWithDB(db *gorm.DB) ClueEngagementRepository {
	return &clueEngagementRepo{db: db}
}

func (r *clueEngagementRepo) Create(ctx context.Context, evt *model.ClueEngagementEvent) error {
	if evt.ClueID == "" {
		return errors.New("clue_id 不能为空")
	}
	return r.db.WithContext(ctx).Create(evt).Error
}

func (r *clueEngagementRepo) CountByClueID(ctx context.Context, clueID string, since time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.ClueEngagementEvent{}).
		Where("clue_id = ? AND created_at >= ?", clueID, since).Count(&count).Error
	return count, err
}

func (r *clueEngagementRepo) CountByClueIDsBatch(ctx context.Context, clueIDs []string, since time.Time) (map[string]int64, error) {
	result := make(map[string]int64, len(clueIDs))
	if len(clueIDs) == 0 {
		return result, nil
	}
	unique := make([]string, 0, len(clueIDs))
	seen := make(map[string]struct{}, len(clueIDs))
	for _, id := range clueIDs {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	if len(unique) == 0 {
		return result, nil
	}
	type row struct {
		ClueID string
		Cnt    int64
	}
	var rows []row
	err := r.db.WithContext(ctx).
		Model(&model.ClueEngagementEvent{}).
		Select("clue_id, COUNT(*) as cnt").
		Where("clue_id IN ? AND created_at >= ?", unique, since).
		Group("clue_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		result[r.ClueID] = r.Cnt
	}
	return result, nil
}
