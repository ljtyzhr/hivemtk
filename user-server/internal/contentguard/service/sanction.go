// sanction.go 处置服务：冻结 / 黑名单的施加、解除与生效判定。
//
// 三条口径（都是线上最容易搞错的地方）：
//  1. **生效判定只看时间**：Level=permanent 恒生效；temp 看 ExpiresAt 是否在未来。
//     不依赖任何定时任务——定时任务挂了、进程重启了、副本没选上主，时间判定照样正确。
//  2. **幂等升不降**：同一主体重复命中的处置只做"续期或升级"（temp→永久），
//     绝不因为一条新记录把已有永久冻结降回临时（那等于给违规者自动减刑）。
//  3. **缓存带失效**：热路径（每条消息判断"这个用户还能不能发"）不能每次都查库，
//     但施加/解除必须立刻生效，否则运营点了"解除"而用户还在被拦，会被当成系统坏了。
package service

import (
	"context"
	"strings"
	"sync"
	"time"

	"hivemtk-user/internal/contentguard/model"
	"hivemtk-user/internal/pkg/tenant"
)

// DefaultSanctionCacheTTL 处置缓存有效期。
const DefaultSanctionCacheTTL = 5 * time.Second

type sanctionCacheEntry struct {
	sanction *model.GuardSanction
	at       time.Time
}

// SanctionService 处置服务。
type SanctionService struct {
	store SanctionStore
	ttl   time.Duration
	now   func() time.Time

	mu    sync.Mutex
	cache map[string]sanctionCacheEntry
}

// NewSanctionService 构造处置服务。
func NewSanctionService(store SanctionStore) *SanctionService {
	return &SanctionService{
		store: store,
		ttl:   DefaultSanctionCacheTTL,
		now:   time.Now,
		cache: make(map[string]sanctionCacheEntry),
	}
}

// SetCacheTTL 调整缓存有效期（测试注入；<=0 恢复默认）。
func (s *SanctionService) SetCacheTTL(ttl time.Duration) {
	if ttl <= 0 {
		ttl = DefaultSanctionCacheTTL
	}
	s.mu.Lock()
	s.ttl = ttl
	s.mu.Unlock()
}

func (s *SanctionService) cacheKey(ctx context.Context, subjectType, subjectID string) string {
	id, _ := tenant.FromContext(ctx)
	return strings.Join([]string{
		itoa(uint64(id)), subjectType, subjectID,
	}, "|")
}

// IsBlocked 查询主体当前是否被拦（供热路径调用，命中缓存时不查库）。
func (s *SanctionService) IsBlocked(ctx context.Context, subjectType, subjectID string) (*model.GuardSanction, bool) {
	if s == nil || s.store == nil || subjectID == "" {
		return nil, false
	}
	now := s.now()
	key := s.cacheKey(ctx, subjectType, subjectID)
	s.mu.Lock()
	entry, ok := s.cache[key]
	ttl := s.ttl
	s.mu.Unlock()
	if ok && now.Sub(entry.at) < ttl {
		if entry.sanction != nil && entry.sanction.Effective(now) {
			return entry.sanction, true
		}
		// 缓存里是"无处置"或已过期：过期数据一律回源，避免缓存把解除/到期挡住。
		if entry.sanction == nil {
			return nil, false
		}
	}

	cur, err := s.store.FindActive(ctx, subjectType, subjectID, now)
	if err != nil {
		// 查不到就按"未处置"处理并**不写缓存**：把库抖动缓存成"永久放行"是事故级错误。
		return nil, false
	}
	s.mu.Lock()
	if cur == nil {
		s.cache[key] = sanctionCacheEntry{sanction: nil, at: now}
	} else {
		s.cache[key] = sanctionCacheEntry{sanction: cur, at: now}
	}
	s.mu.Unlock()
	if cur == nil || !cur.Effective(now) {
		return nil, false
	}
	return cur, true
}

// ApplyInput 施加处置的入参。
type ApplyInput struct {
	SubjectType  string
	SubjectID    string
	Kind         string
	Level        string
	Reason       string
	Source       string
	RecordID     uint64
	TraceID      string
	OperatorID   uint
	OperatorName string
	// DurationMinutes temp 档的有效期（<=0 取 24h）。
	DurationMinutes int
}

// Apply 施加处置（幂等：已有生效处置时只续期或升级）。
func (s *SanctionService) Apply(ctx context.Context, in ApplyInput) (*model.GuardSanction, error) {
	if s == nil || s.store == nil {
		return nil, ErrSanctionStoreUnavailable
	}
	if err := ValidateSubjectType(in.SubjectType); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.SubjectID) == "" {
		return nil, ErrSanctionSubjectRequired
	}
	if in.Kind == "" {
		in.Kind = model.SanctionKindFreeze
	}
	if in.Level == "" {
		in.Level = model.SanctionLevelTemp
	}
	if in.Source == "" {
		in.Source = "auto"
	}
	if in.Kind != model.SanctionKindFreeze && in.Kind != model.SanctionKindBlacklist {
		return nil, ErrSanctionKindInvalid
	}
	if in.Level != model.SanctionLevelTemp && in.Level != model.SanctionLevelPermanent {
		return nil, ErrSanctionLevelInvalid
	}
	now := s.now()
	expires := expiryFor(in.Level, in.DurationMinutes, now)

	cur, err := s.store.FindActive(ctx, in.SubjectType, in.SubjectID, now)
	if err != nil {
		return nil, err
	}
	if cur != nil && cur.Effective(now) {
		updated := *cur
		if cur.Level == model.SanctionLevelPermanent {
			// 已永久：不再改档，但把最后一次证据挂上去（审计要能追到"最后一次为什么"）。
			updated.RecordID = in.RecordID
			updated.TraceID = in.TraceID
			updated.Reason = in.Reason
			if err := s.store.Update(ctx, &updated); err != nil {
				return nil, err
			}
			s.invalidateKey(ctx, in.SubjectType, in.SubjectID)
			return &updated, nil
		}
		if in.Level == model.SanctionLevelPermanent {
			updated.Level = model.SanctionLevelPermanent
			updated.ExpiresAt = nil
		} else if expires != nil {
			if updated.ExpiresAt == nil || updated.ExpiresAt.Before(*expires) {
				updated.ExpiresAt = expires
			}
		}
		updated.RecordID = in.RecordID
		updated.TraceID = in.TraceID
		updated.Reason = in.Reason
		updated.Kind = in.Kind
		if err := s.store.Update(ctx, &updated); err != nil {
			return nil, err
		}
		s.invalidateKey(ctx, in.SubjectType, in.SubjectID)
		return &updated, nil
	}

	row := &model.GuardSanction{
		SubjectType:  in.SubjectType,
		SubjectID:    in.SubjectID,
		Kind:         in.Kind,
		Level:        in.Level,
		Reason:       in.Reason,
		Source:       in.Source,
		RecordID:     in.RecordID,
		TraceID:      in.TraceID,
		OperatorID:   in.OperatorID,
		OperatorName: in.OperatorName,
		ExpiresAt:    expires,
		Active:       true,
	}
	if err := s.store.Create(ctx, row); err != nil {
		return nil, err
	}
	s.invalidateKey(ctx, in.SubjectType, in.SubjectID)
	return row, nil
}

// Release 解除处置（人工操作）。
func (s *SanctionService) Release(ctx context.Context, id uint64, operatorID uint, reason string) (*model.GuardSanction, error) {
	if s == nil || s.store == nil {
		return nil, ErrSanctionStoreUnavailable
	}
	row, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, ErrSanctionNotFound
	}
	now := s.now()
	row.Active = false
	row.ReleasedAt = &now
	row.ReleaseReason = reason
	row.OperatorID = operatorID
	if err := s.store.Update(ctx, row); err != nil {
		return nil, err
	}
	s.invalidateKey(ctx, row.SubjectType, row.SubjectID)
	return row, nil
}

// Get 处置详情。
func (s *SanctionService) Get(ctx context.Context, id uint64) (*model.GuardSanction, error) {
	if s == nil || s.store == nil {
		return nil, ErrSanctionStoreUnavailable
	}
	return s.store.Get(ctx, id)
}

// List 处置列表。
func (s *SanctionService) List(ctx context.Context, q SanctionQuery) ([]model.GuardSanction, int64, error) {
	if s == nil || s.store == nil {
		return nil, 0, ErrSanctionStoreUnavailable
	}
	return s.store.List(ctx, q)
}

// InvalidateAll 清空缓存（配置变更/批量解除后调用）。
func (s *SanctionService) InvalidateAll() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.cache = make(map[string]sanctionCacheEntry)
	s.mu.Unlock()
}

func (s *SanctionService) invalidateKey(ctx context.Context, subjectType, subjectID string) {
	s.mu.Lock()
	delete(s.cache, s.cacheKey(ctx, subjectType, subjectID))
	s.mu.Unlock()
}

// expiryFor 依档位算到期时间：permanent → nil；temp → now+duration（缺省 24h）。
func expiryFor(level string, durationMinutes int, now time.Time) *time.Time {
	if level == model.SanctionLevelPermanent {
		return nil
	}
	if durationMinutes <= 0 {
		durationMinutes = 24 * 60
	}
	t := now.Add(time.Duration(durationMinutes) * time.Minute)
	return &t
}

func itoa(v uint64) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
