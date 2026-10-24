// reflux adopter：审核采纳动作与写回 faq_entries（T1）。
//
// 状态机：draft→pending_review→adopted|ignored|merged（非法迁移拒绝）。
// adopt 时经 keyworder 去泛化硬校验，经 FAQService.Create 写回（复用 Layer1
// 关键词索引维护），全簇成员置 adopted 并回填 adopted_faq_id，写 review_log。
package reflux

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/model"
)

// ErrIllegalTransition 非法状态迁移
var ErrIllegalTransition = errors.New("非法状态迁移")

// ErrNotFound 目标不存在
var ErrNotFound = errors.New("目标不存在")

// ReviewAction 审核动作
type ReviewAction string

const (
	ActionSubmit ReviewAction = "submit"
	ActionAdopt  ReviewAction = "adopt"
	ActionIgnore ReviewAction = "ignore"
	ActionMerge  ReviewAction = "merge"
)

// AdoptRequest 审核请求
type AdoptRequest struct {
	Action         ReviewAction `json:"action" binding:"required"`
	MergedTargetID int64        `json:"merged_target_id"`
	Answer         string       `json:"answer"`
	Category       string       `json:"category"`
}

// RefluxService 审核/采纳服务（controller 唯一入口）
type RefluxService struct {
	db *gorm.DB
	// FAQCreate 复用 FAQService.Create 的函数钩子（装配层注入，避免包依赖环）。
	// 签名：create(ctx, req) → faq_id
	FAQCreate func(ctx context.Context, req FAQCreateReq) (uint, error)
}

// FAQCreateReq 写回 FAQ 的参数（字段映射见设计文档 §6.1）
type FAQCreateReq struct {
	Question  string
	Answer    string
	Keywords  []string
	Category  string
	AgentID   uint
	Intent    string
	Enabled   bool
	SourceTag string
}

// NewRefluxService 构造
func NewRefluxService(db *gorm.DB) *RefluxService { return &RefluxService{db: db} }

// SetFAQCreate 装配层注入 FAQService.Create 钩子
func (s *RefluxService) SetFAQCreate(fn func(ctx context.Context, req FAQCreateReq) (uint, error)) {
	s.FAQCreate = fn
}

// allowedTransitions 状态机白名单
var allowedTransitions = map[string]map[ReviewAction]bool{
	model.UQStatusDraft:         {ActionSubmit: true},
	model.UQStatusPendingReview: {ActionAdopt: true, ActionIgnore: true, ActionMerge: true},
}

// SubmitQuestion 单条 draft→pending_review
func (s *RefluxService) SubmitQuestion(ctx context.Context, id int64, operatorID string) error {
	return s.transition(ctx, "question", id, model.UQStatusDraft, model.UQStatusPendingReview,
		ActionSubmit, operatorID, "")
}

// ReviewQuestion 单条审核
func (s *RefluxService) ReviewQuestion(ctx context.Context, id int64, req *AdoptRequest, operatorID string) (map[string]any, error) {
	var uq model.UnknownQuestion
	if err := s.db.WithContext(ctx).First(&uq, id).Error; err != nil {
		return nil, ErrNotFound
	}
	switch req.Action {
	case ActionIgnore:
		if err := s.transition(ctx, "question", id, uq.Status, model.UQStatusIgnored, ActionIgnore, operatorID, ""); err != nil {
			return nil, err
		}
		return map[string]any{"id": id, "status": model.UQStatusIgnored}, nil
	case ActionMerge:
		if req.MergedTargetID == 0 {
			return nil, fmt.Errorf("merged_target_id 必填")
		}
		if err := s.db.WithContext(ctx).Model(&model.UnknownQuestion{}).Where("id = ?", id).Updates(map[string]any{
			"status": model.UQStatusMerged, "merged_target_id": req.MergedTargetID,
			"reviewed_by": operatorID, "reviewed_at": time.Now(),
		}).Error; err != nil {
			return nil, err
		}
		s.log(ctx, "question", id, ActionMerge, uq.Status, model.UQStatusMerged, operatorID,
			fmt.Sprintf("merged_target_id=%d", req.MergedTargetID))
		return map[string]any{"id": id, "status": model.UQStatusMerged}, nil
	case ActionAdopt:
		return s.adoptSingle(ctx, &uq, req, operatorID)
	default:
		return nil, ErrIllegalTransition
	}
}

// adoptSingle 单条采纳（非簇成员）
func (s *RefluxService) adoptSingle(ctx context.Context, uq *model.UnknownQuestion, req *AdoptRequest, operatorID string) (map[string]any, error) {
	if uq.Status != model.UQStatusPendingReview {
		return nil, ErrIllegalTransition
	}
	if req.Answer == "" {
		return nil, fmt.Errorf("answer 必填")
	}
	faqID, err := s.writeFAQ(ctx, uq.Question, req, uq.AgentID, uq.IntentType)
	if err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&model.UnknownQuestion{}).Where("id = ?", uq.ID).Updates(map[string]any{
		"status": model.UQStatusAdopted, "adopted_faq_id": faqID,
		"reviewed_by": operatorID, "reviewed_at": time.Now(),
	}).Error; err != nil {
		return nil, err
	}
	s.log(ctx, "question", uq.ID, ActionAdopt, uq.Status, model.UQStatusAdopted, operatorID,
		fmt.Sprintf("faq_id=%d", faqID))
	return map[string]any{"id": uq.ID, "faq_id": faqID, "status": model.UQStatusAdopted}, nil
}

// ReviewCluster 簇级审核（adopt 时对全簇生效，生成一条 FAQ）
func (s *RefluxService) ReviewCluster(ctx context.Context, clusterID int64, req *AdoptRequest, operatorID string) (map[string]any, error) {
	var cluster model.UnknownQuestionCluster
	if err := s.db.WithContext(ctx).First(&cluster, clusterID).Error; err != nil {
		return nil, ErrNotFound
	}
	switch req.Action {
	case ActionIgnore:
		if err := s.db.WithContext(ctx).Model(&model.UnknownQuestionCluster{}).Where("id = ?", clusterID).Updates(map[string]any{
			"status": model.UQStatusIgnored, "reviewed_by": operatorID, "reviewed_at": time.Now(),
		}).Error; err != nil {
			return nil, err
		}
		s.log(ctx, "cluster", clusterID, ActionIgnore, cluster.Status, model.UQStatusIgnored, operatorID, "")
		return map[string]any{"cluster_id": clusterID, "status": model.UQStatusIgnored}, nil
	case ActionAdopt:
		if cluster.Status != model.UQStatusPendingReview {
			return nil, ErrIllegalTransition
		}
		if req.Answer == "" {
			return nil, fmt.Errorf("answer 必填")
		}
		faqID, err := s.writeFAQ(ctx, cluster.Title, req, cluster.AgentID, "")
		if err != nil {
			return nil, err
		}
		var adoptedCount int64
		err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&model.UnknownQuestion{}).
				Where("cluster_id = ? AND status = ?", clusterID, model.UQStatusPendingReview).
				Updates(map[string]any{
					"status": model.UQStatusAdopted, "adopted_faq_id": faqID,
					"reviewed_by": operatorID, "reviewed_at": time.Now(),
				}).Error; err != nil {
				return err
			}
			adoptedCount = tx.Statement.RowsAffected
			return tx.Model(&model.UnknownQuestionCluster{}).Where("id = ?", clusterID).Updates(map[string]any{
				"status": model.UQStatusAdopted, "adopted_faq_id": faqID,
				"reviewed_by": operatorID, "reviewed_at": time.Now(),
			}).Error
		})
		if err != nil {
			return nil, err
		}
		s.log(ctx, "cluster", clusterID, ActionAdopt, cluster.Status, model.UQStatusAdopted, operatorID,
			fmt.Sprintf("faq_id=%d adopted_count=%d total_frequency=%d", faqID, adoptedCount, cluster.TotalFrequency))
		return map[string]any{"cluster_id": clusterID, "faq_id": faqID, "adopted_count": adoptedCount}, nil
	default:
		return nil, ErrIllegalTransition
	}
}

// writeFAQ 关键词去泛化硬校验 + 经 FAQCreate 钩子写回
func (s *RefluxService) writeFAQ(ctx context.Context, question string, req *AdoptRequest, agentID uint, intent string) (uint, error) {
	keywords := GenerateKeywords(question)
	// 既有 FAQ 关键词全集（区分性校验用）
	existing := map[string]int{}
	rows := []struct{ Keywords []string }{}
	if err := s.db.WithContext(ctx).Table("faq_entries").
		Select("keywords").Where("keywords IS NOT NULL").Limit(5000).Scan(&rows).Error; err == nil {
		for _, r := range rows {
			for _, k := range r.Keywords {
				existing[k]++
			}
		}
	}
	if err := ValidateKeywords(keywords, existing); err != nil {
		return 0, err
	}
	if s.FAQCreate == nil {
		return 0, fmt.Errorf("FAQCreate 未装配")
	}
	category := req.Category
	if category == "" {
		category = "reflux"
	}
	return s.FAQCreate(ctx, FAQCreateReq{
		Question: question, Answer: req.Answer, Keywords: keywords,
		Category: category, AgentID: agentID, Intent: intent, Enabled: true, SourceTag: "reflux",
	})
}

// transition 通用状态迁移（白名单校验 + 流水）
func (s *RefluxService) transition(ctx context.Context, targetType string, id int64, from, to string, action ReviewAction, operatorID, detail string) error {
	allowed, ok := allowedTransitions[from][action]
	if !ok || !allowed {
		return ErrIllegalTransition
	}
	table := "unknown_questions"
	if targetType == "cluster" {
		table = "unknown_question_clusters"
	}
	if err := s.db.WithContext(ctx).Table(table).Where("id = ? AND status = ?", id, from).
		Update("status", to).Error; err != nil {
		return err
	}
	s.log(ctx, targetType, id, action, from, to, operatorID, detail)
	return nil
}

func (s *RefluxService) log(ctx context.Context, targetType string, targetID int64, action ReviewAction, from, to, operatorID, detail string) {
	_ = s.db.WithContext(ctx).Create(&model.UnknownQuestionReviewLog{
		TargetType: targetType, TargetID: targetID, Action: string(action),
		FromStatus: from, ToStatus: to, OperatorID: operatorID, Detail: detail,
	}).Error
}

// ============================================================
// 查询侧（运营列表 / 详情 / 统计）
// ============================================================

// ListQuestions 未知问题分页（filter: status/source/agent_id/question_like）
func (s *RefluxService) ListQuestions(ctx context.Context, filter map[string]any, page, pageSize int) ([]model.UnknownQuestion, int64, error) {
	q := s.db.WithContext(ctx).Model(&model.UnknownQuestion{})
	if v, ok := filter["status"]; ok {
		q = q.Where("status = ?", v)
	}
	if v, ok := filter["source"]; ok {
		q = q.Where("source = ?", v)
	}
	if v, ok := filter["agent_id"]; ok {
		q = q.Where("agent_id = ?", v)
	}
	if v, ok := filter["question_like"]; ok {
		q = q.Where("question ILIKE ?", "%"+fmt.Sprint(v)+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.UnknownQuestion{}
	if err := q.Order("last_seen_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// ListClusters 簇分页（按 total_frequency 降序）
func (s *RefluxService) ListClusters(ctx context.Context, filter map[string]any, page, pageSize int) ([]model.UnknownQuestionCluster, int64, error) {
	q := s.db.WithContext(ctx).Model(&model.UnknownQuestionCluster{})
	if v, ok := filter["status"]; ok {
		q = q.Where("status = ?", v)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.UnknownQuestionCluster{}
	if err := q.Order("total_frequency DESC").Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// Stats 统计：总数 / 待审 / 已采纳 / 采纳率
func (s *RefluxService) Stats(ctx context.Context) (map[string]any, error) {
	row := struct {
		Total   int64 `gorm:"column:total"`
		Pending int64 `gorm:"column:pending"`
		Adopted int64 `gorm:"column:adopted"`
	}{}
	if err := s.db.WithContext(ctx).Table("unknown_questions").
		Select(`COUNT(*) AS total,
			COUNT(*) FILTER (WHERE status = 'pending_review') AS pending,
			COUNT(*) FILTER (WHERE status = 'adopted') AS adopted`).
		Scan(&row).Error; err != nil {
		return nil, err
	}
	rate := 0.0
	if row.Total > 0 {
		rate = float64(row.Adopted) / float64(row.Total)
	}
	return map[string]any{
		"total": row.Total, "pending": row.Pending,
		"adopted": row.Adopted, "adopt_rate": rate,
	}, nil
}

// QuestionDetail 详情：问题 + 审核流水 + 簇成员
func (s *RefluxService) QuestionDetail(ctx context.Context, id int64) (map[string]any, error) {
	var uq model.UnknownQuestion
	if err := s.db.WithContext(ctx).First(&uq, id).Error; err != nil {
		return nil, ErrNotFound
	}
	logs := []model.UnknownQuestionReviewLog{}
	_ = s.db.WithContext(ctx).Where("target_type = 'question' AND target_id = ?", id).
		Order("created_at ASC").Find(&logs).Error
	var cluster *model.UnknownQuestionCluster
	if uq.ClusterID > 0 {
		var c model.UnknownQuestionCluster
		if err := s.db.WithContext(ctx).First(&c, uq.ClusterID).Error; err == nil {
			cluster = &c
		}
	}
	return map[string]any{"question": uq, "review_logs": logs, "cluster": cluster}, nil
}

// ClusterDetail 簇详情：簇 + 成员 + 流水
func (s *RefluxService) ClusterDetail(ctx context.Context, id int64) (map[string]any, error) {
	var cluster model.UnknownQuestionCluster
	if err := s.db.WithContext(ctx).First(&cluster, id).Error; err != nil {
		return nil, ErrNotFound
	}
	members := []model.UnknownQuestion{}
	if err := s.db.WithContext(ctx).Where("cluster_id = ?", id).
		Order("frequency DESC").Find(&members).Error; err != nil {
		return nil, err
	}
	logs := []model.UnknownQuestionReviewLog{}
	_ = s.db.WithContext(ctx).Where("target_type = 'cluster' AND target_id = ?", id).
		Order("created_at ASC").Find(&logs).Error
	return map[string]any{"cluster": cluster, "members": members, "review_logs": logs}, nil
}
