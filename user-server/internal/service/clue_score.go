package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"context"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"

	"gorm.io/gorm"
)

// 线索评分域对调用方暴露的「不存在」语义，供 controller 区分 404 与真实故障。
var (
	ErrClueScoreNotFound = errors.New("未找到评分")
	ErrClueNotFound      = errors.New("线索不存在")
)

// ClueScoreService 线索评分服务
// 评分模型（5 维度加权，0-100）：
//
//	channel 25% / verify 20% / profile 20% / engagement 25% / recency 10%
type ClueScoreService struct {
	scoreRepo  repository.ClueScoreRepository
	engageRepo repository.ClueEngagementRepository
	clueRepo   repository.ClueRepository
}

// NewClueScoreService 用全局库创建线索评分服务（测试用 db.SetTestDB／testutil 换库）
func NewClueScoreService() *ClueScoreService {
	return &ClueScoreService{
		scoreRepo:  repository.NewClueScoreRepository(),
		engageRepo: repository.NewClueEngagementRepository(),
		clueRepo:   repository.NewClueRepositoryDefault(),
	}
}

// NewClueScoreServiceWithRepos 测试用 - 自定义依赖
func NewClueScoreServiceWithRepos(s repository.ClueScoreRepository, e repository.ClueEngagementRepository, c repository.ClueRepository) *ClueScoreService {
	return &ClueScoreService{scoreRepo: s, engageRepo: e, clueRepo: c}
}

// ScoreClue 评分单条线索（基础信息+互动事件）
// 群聊线索：若来自活跃群聊，engagement 维度加成 10%
func (s *ClueScoreService) ScoreClue(ctx context.Context, clue *model.Clue) (*model.ClueScore, error) {
	if clue == nil {
		return nil, errors.New("线索不能为空")
	}

	channelScore := scoreChannel(clue.Type)

	verifyScore := 0
	if clue.IsVerify == 1 {
		verifyScore = 100
	} else if clue.IsVerify == 0 {
		verifyScore = 20
	}

	profileScore := scoreProfile(clue)

	engagementScore, err := s.scoreEngagement(ctx, clue.ID)
	if err != nil {

		logger.Warnf("[clue-score] scoreEngagement failed (clue=%s): %v", clue.ID, err)
		engagementScore = 0
	}

	if clue.IsGroup {
		engagementScore = int(math.Min(100, float64(engagementScore)*1.10))
	}

	recencyScore := clueScoreRecency(clue.CreateTime)

	total := int(math.Round(
		float64(channelScore)*0.25 +
			float64(verifyScore)*0.20 +
			float64(profileScore)*0.20 +
			float64(engagementScore)*0.25 +
			float64(recencyScore)*0.10,
	))
	if total < 0 {
		total = 0
	}
	if total > 100 {
		total = 100
	}

	confidence := calcConfidence(channelScore, verifyScore, profileScore, engagementScore, recencyScore)

	factors := map[string]any{
		"channel":    channelScore,
		"verify":     verifyScore,
		"profile":    profileScore,
		"engagement": engagementScore,
		"recency":    recencyScore,
		"is_group":   clue.IsGroup,
		"weights": map[string]float64{
			"channel":    0.25,
			"verify":     0.20,
			"profile":    0.20,
			"engagement": 0.25,
			"recency":    0.10,
		},
		"clue_type": clue.Type,
		"is_verify": clue.IsVerify,
	}
	factorsJSON, _ := json.Marshal(factors)

	score := &model.ClueScore{
		ClueID:          clue.ID,
		Account:         clue.Account,
		TotalScore:      total,
		Grade:           model.CalcGradeFromScore(total),
		Confidence:      confidence,
		ChannelScore:    channelScore,
		VerifyScore:     verifyScore,
		ProfileScore:    profileScore,
		EngagementScore: engagementScore,
		RecencyScore:    recencyScore,
		FactorsJSON:     string(factorsJSON),
		ModelVersion:    "h-score-1",
		ScoredAt:        time.Now(),
	}
	if err := s.scoreRepo.Upsert(ctx, score); err != nil {
		return nil, err
	}
	s.writeBackLevel(ctx, clue.ID, total)
	return score, nil
}

func (s *ClueScoreService) writeBackLevel(ctx context.Context, clueID string, score int) {
	if s.clueRepo == nil || clueID == "" {
		return
	}
	level := ClueLevelFromScore(score)
	if err := s.clueRepo.UpdateByID(ctx, clueID, map[string]any{"level": level}); err != nil {
		logger.Warnf("[clue-score] writeBackLevel failed (clue=%s, level=%s): %v", clueID, level, err)
	}
}

// ClueLevelFromScore 线索评分 → 温度等级映射（P-9）。
func ClueLevelFromScore(score int) string {
	switch {
	case score >= 70:
		return "hot"
	case score >= 40:
		return "warm"
	default:
		return "cold"
	}
}

// ScoreAll 对所有线索进行评分（受 limit 限制）
func (s *ClueScoreService) ScoreAll(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 1000 {
		limit = 200
	}
	clues, _, err := s.clueRepo.GetClueList(ctx, 1, limit)
	if err != nil {
		return 0, err
	}
	if len(clues) == 0 {
		return 0, nil
	}

	since := time.Now().Add(-7 * 24 * time.Hour)
	clueIDs := make([]string, 0, len(clues))
	for _, c := range clues {
		clueIDs = append(clueIDs, c.ID)
	}
	engagementMap, err := s.engageRepo.CountByClueIDsBatch(ctx, clueIDs, since)
	if err != nil {
		engagementMap = make(map[string]int64, len(clues))
	}

	success := 0
	for _, c := range clues {
		if _, err := s.scoreClueWithEngagement(ctx, c, engagementMap[c.ID]); err != nil {
			return success, fmt.Errorf("评分失败 (clue_id=%s): %w", c.ID, err)
		}
		success++
	}
	return success, nil
}

func (s *ClueScoreService) scoreClueWithEngagement(ctx context.Context, clue *model.Clue, engagementCount int64) (*model.ClueScore, error) {
	if clue == nil {
		return nil, errors.New("线索不能为空")
	}

	channelScore := scoreChannel(clue.Type)

	verifyScore := 0
	if clue.IsVerify == 1 {
		verifyScore = 100
	} else if clue.IsVerify == 0 {
		verifyScore = 20
	}

	profileScore := scoreProfile(clue)

	c := engagementCount
	if c > 8 {
		c = 8
	}
	engagementScore := int(c) * 12

	if clue.IsGroup {
		engagementScore = int(math.Min(100, float64(engagementScore)*1.10))
	}

	recencyScore := clueScoreRecency(clue.CreateTime)

	total := int(math.Round(
		float64(channelScore)*0.25 +
			float64(verifyScore)*0.20 +
			float64(profileScore)*0.20 +
			float64(engagementScore)*0.25 +
			float64(recencyScore)*0.10,
	))
	if total < 0 {
		total = 0
	}
	if total > 100 {
		total = 100
	}

	confidence := calcConfidence(channelScore, verifyScore, profileScore, engagementScore, recencyScore)

	factors := map[string]any{
		"channel":    channelScore,
		"verify":     verifyScore,
		"profile":    profileScore,
		"engagement": engagementScore,
		"recency":    recencyScore,
		"is_group":   clue.IsGroup,
		"weights": map[string]float64{
			"channel":    0.25,
			"verify":     0.20,
			"profile":    0.20,
			"engagement": 0.25,
			"recency":    0.10,
		},
		"clue_type": clue.Type,
		"is_verify": clue.IsVerify,
	}
	factorsJSON, _ := json.Marshal(factors)

	score := &model.ClueScore{
		ClueID:          clue.ID,
		Account:         clue.Account,
		TotalScore:      total,
		Grade:           model.CalcGradeFromScore(total),
		Confidence:      confidence,
		ChannelScore:    channelScore,
		VerifyScore:     verifyScore,
		ProfileScore:    profileScore,
		EngagementScore: engagementScore,
		RecencyScore:    recencyScore,
		FactorsJSON:     string(factorsJSON),
		ModelVersion:    "h-score-1",
		ScoredAt:        time.Now(),
	}
	if err := s.scoreRepo.Upsert(ctx, score); err != nil {
		return nil, err
	}
	s.writeBackLevel(ctx, clue.ID, total)
	return score, nil
}

// RecordEngagement 记录一次互动事件
func (s *ClueScoreService) RecordEngagement(ctx context.Context, clueID, eventType, channel string, payload any) error {
	if clueID == "" {
		return errors.New("clue_id 不能为空")
	}
	if eventType == "" {
		return errors.New("event_type 不能为空")
	}
	payloadBytes, _ := json.Marshal(payload)
	evt := &model.ClueEngagementEvent{
		ClueID:    clueID,
		EventType: eventType,
		Channel:   channel,
		Payload:   string(payloadBytes),
	}
	return s.engageRepo.Create(ctx, evt)
}

// GetByClueID 查询线索评分
func (s *ClueScoreService) GetByClueID(ctx context.Context, clueID string) (*model.ClueScore, error) {
	score, err := s.scoreRepo.GetByClueID(ctx, clueID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrClueScoreNotFound
		}
		return nil, err
	}
	return score, nil
}

// ListByGrade 按等级分页查询
func (s *ClueScoreService) ListByGrade(ctx context.Context, grade string, page, pageSize int) ([]*model.ClueScore, int64, error) {
	return s.scoreRepo.ListByGrade(ctx, grade, page, pageSize)
}

// LoadClueForScoring 加载线索对象（用于评分）。
// 直接按主键取一行：此前是先拉前 500 条再在内存里找，线索超过 500 条后
// 排在后面的线索会被判成「线索不存在」。
func (s *ClueScoreService) LoadClueForScoring(ctx context.Context, clueID string) (*model.Clue, error) {
	if clueID == "" {
		return nil, errors.New("clue_id 不能为空")
	}
	clue, err := s.clueRepo.GetByID(ctx, clueID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrClueNotFound
		}
		return nil, err
	}
	return clue, nil
}

func scoreChannel(clueType int64) int {
	switch clueType {
	case ClueTypeQQ:
		return 60
	case ClueTypeWeChat:
		return 85
	case ClueTypePhone:
		return 95
	case ClueTypeTelegram:
		return 80
	case ClueTypeWhatsapp:
		return 90
	case ClueTypeTwitter:
		return 55
	case ClueTypeWeCom:
		return 85
	case ClueTypeDouyin:
		return 65
	case ClueTypeKuaishou:
		return 65
	case ClueTypeXiaohongshu:
		return 60
	case ClueTypeXianyu:
		return 55
	case ClueTypeFeishu:
		return 80
	case ClueTypeTikTok:
		return 60
	case ClueTypeWebWidget:
		return 75
	case ClueTypeEmail:
		return 55
	case ClueTypeSMS:
		return 70
	case ClueTypeLeadMining:
		return 60
	// GEO 决策链捕获在撞号期间一直吃的是抖音档（65）。这次只把"它是谁"分开，
	// 不动"它值多少"——评分档位属产品口径，真要单开一档得连着线索温度阈值一起看，
	// 不在一次身份修复里顺手改排序。
	case ClueTypeGeoCapture:
		return 65
	default:
		return 50
	}
}

func scoreProfile(clue *model.Clue) int {
	score := 0
	if clue.Name != "" {
		score += 25
	}
	if clue.City != "" {
		score += 25
	}
	if clue.Address != "" {
		score += 25
	}
	if clue.Desc != "" {
		score += 25
	}
	return score
}

func (s *ClueScoreService) scoreEngagement(ctx context.Context, clueID string) (int, error) {
	if clueID == "" {
		return 0, nil
	}
	since := time.Now().Add(-7 * 24 * time.Hour)
	count, err := s.engageRepo.CountByClueID(ctx, clueID, since)
	if err != nil {
		return 0, err
	}
	if count > 8 {
		count = 8
	}
	return int(count) * 12, nil
}

func clueScoreRecency(createTime int64) int {
	if createTime <= 0 {
		return 50
	}
	t := time.Unix(createTime, 0)
	days := int(time.Since(t).Hours() / 24)
	switch {
	case days <= 1:
		return 100
	case days <= 7:
		return 70
	case days <= 30:
		return 40
	default:
		return 0
	}
}

func calcConfidence(channel, verify, profile, engagement, recency int) int {
	score := 0
	if channel > 0 {
		score += 20
	}
	if verify > 0 {
		score += 20
	}
	if profile > 0 {
		score += 20
	}
	if engagement > 0 {
		score += 20
	}
	if recency > 0 {
		score += 20
	}
	return score
}
