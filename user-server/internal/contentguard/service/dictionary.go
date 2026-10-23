// dictionary.go 词库快照（进程内 AC 自动机）与热更新。
//
// 关键设计：
//   - **快照整体替换**：词库变更时编译一个新的 matcher，再原子换指针。
//     绝不在旧快照上增删词——扫描是并发的，改一个共享 map 就是数据竞争
//     （本仓 CI 跑 -race，这类竞争会直接把测试判红）。
//   - **按租户缓存**：SaaS 模式下每个租户有自己的词库；离线单机（TENANT_MODE=off）
//     只有一个默认租户，等价于全局一份。
//   - **TTL 兜底**：即便某次写入口忘了 Invalidate（新增写路径、直接改库），
//     快照也会在 ttl 后自动重建，不会出现"改了词库线上永不生效"。
//   - 词库存取失败时**失败开放**（返回空匹配器 + 错误由调用方记录）：
//     词库不可用的正确后果是"这轮查不出违规"，而不是"所有消息都被拦"。
//     拦全量用户是比漏审更大的事故。
package service

import (
	"context"
	"sync"
	"time"

	"hivemtk-user/internal/contentguard/matcher"
	"hivemtk-user/internal/contentguard/model"
	"hivemtk-user/internal/pkg/tenant"
)

// DefaultDictTTL 词库快照有效期。
const DefaultDictTTL = 30 * time.Second

type dictEntry struct {
	matcher  *matcher.Matcher
	words    map[uint]model.GuardWord
	loadedAt time.Time
	denyCnt  int
	allowCnt int
}

// Dictionary 词库快照缓存。
type Dictionary struct {
	words WordStore
	ttl   time.Duration
	now   func() time.Time

	mu      sync.RWMutex
	entries map[uint]*dictEntry
}

// NewDictionary 构造词库缓存。
func NewDictionary(words WordStore) *Dictionary {
	return &Dictionary{
		words:   words,
		ttl:     DefaultDictTTL,
		now:     time.Now,
		entries: make(map[uint]*dictEntry),
	}
}

// SetTTL 调整快照有效期（测试注入用；<=0 恢复默认）。
func (d *Dictionary) SetTTL(ttl time.Duration) {
	if ttl <= 0 {
		ttl = DefaultDictTTL
	}
	d.mu.Lock()
	d.ttl = ttl
	d.mu.Unlock()
}

// Snapshot 取当前租户的匹配快照（必要时重建）。
//
// 返回的 matcher 与 words 必须**成对**使用：matcher 给出 PatternID，
// words 把 ID 翻回词条（判定 deny/allow、严重级别、分类）。
func (d *Dictionary) Snapshot(ctx context.Context) (*matcher.Matcher, map[uint]model.GuardWord, error) {
	if d == nil {
		return matcher.New(nil), map[uint]model.GuardWord{}, nil
	}
	tenantID, ok := tenant.FromContext(ctx)
	if !ok {
		tenantID = tenant.DefaultTenantID
	}
	d.mu.RLock()
	entry := d.entries[tenantID]
	ttl := d.ttl
	d.mu.RUnlock()
	if entry != nil && len(entry.words) >= 0 && d.now().Sub(entry.loadedAt) < ttl {
		return entry.matcher, entry.words, nil
	}
	return d.reload(ctx, tenantID)
}

func (d *Dictionary) reload(ctx context.Context, tenantID uint) (*matcher.Matcher, map[uint]model.GuardWord, error) {
	words, err := d.words.ListEnabled(ctx)
	if err != nil {
		// 失败开放：返回空快照，错误上抛给调用方记录（审查记录里 sources 会缺 dict）。
		empty := matcher.New(nil)
		return empty, map[uint]model.GuardWord{}, err
	}
	patterns := make([]matcher.Pattern, 0, len(words))
	index := make(map[uint]model.GuardWord, len(words))
	deny, allow := 0, 0
	for _, w := range words {
		patterns = append(patterns, matcher.Pattern{
			ID:       w.ID,
			Word:     w.Word,
			Category: w.Category,
			Severity: w.Severity,
		})
		index[w.ID] = w
		if w.ListKind == model.ListKindAllow {
			allow++
		} else {
			deny++
		}
	}
	entry := &dictEntry{
		matcher:  matcher.New(patterns),
		words:    index,
		loadedAt: d.now(),
		denyCnt:  deny,
		allowCnt: allow,
	}
	d.mu.Lock()
	if d.entries == nil {
		d.entries = make(map[uint]*dictEntry)
	}
	d.entries[tenantID] = entry
	d.mu.Unlock()
	return entry.matcher, entry.words, nil
}

// Invalidate 让某租户的快照失效（词库写入口调用）。
//
// 只删缓存不主动重建：重建要读库，写入口不该为一次后台刷新付延迟。
func (d *Dictionary) Invalidate(tenantID uint) {
	if d == nil {
		return
	}
	d.mu.Lock()
	delete(d.entries, tenantID)
	d.mu.Unlock()
}

// InvalidateAll 全量失效。
func (d *Dictionary) InvalidateAll() {
	if d == nil {
		return
	}
	d.mu.Lock()
	d.entries = make(map[uint]*dictEntry)
	d.mu.Unlock()
}

// Stats 已缓存租户数与各租户词条规模（运维/健康检查用）。
func (d *Dictionary) Stats() map[uint][2]int {
	out := map[uint][2]int{}
	if d == nil {
		return out
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	for id, e := range d.entries {
		out[id] = [2]int{e.denyCnt, e.allowCnt}
	}
	return out
}
