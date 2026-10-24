package middleware

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ErrOwnershipNotFound 资源不存在（或不属于任何可见租户）。
//
// 引入它是为了**区分两种"查不到"**：资源不存在应回 404（不泄漏存在性），
// 而 DB 故障应回 500（可重试、可告警）。
var ErrOwnershipNotFound = errors.New("ownership: resource not found")

// ErrOwnershipMisconfigured 归属查询器未正确装配（如 db == nil）。
//
// 此时一律返回本错误，由中间件转 500：装配错必须在请求上响亮，而不是静默放行
// （把"未装配"当"系统级资源"放行是 fail-open）。
var ErrOwnershipMisconfigured = errors.New("ownership: checker not configured")

// OwnershipChecker 资源归属查询接口。
//
// table 为数据库表名（业务侧硬编码传入，避免 GORM TableName 反射开销），
// resourceID 是 URL 路径里的资源主键。
//
// 返回值语义（三条，与中间件一一对应）：
//   - ownerID > 0：归属该用户；与当前 uid 不等 ⇒ 403
//   - ownerID == 0 且 err == nil：**系统级资源**（owner_id 列真是 0 的行）。
//     中间件默认也拒绝（404），只有显式 `WithSystemOwnedAllowed()` 才放行
//   - err != nil：必须返回 `ErrOwnershipNotFound`（⇒ 404）或其它错误（⇒ 500），
//     绝不允许用 (0, nil) 表达"查不到"
type OwnershipChecker interface {
	GetOwnerID(ctx context.Context, table string, resourceID uint) (uint, error)
}

// DBOwnershipChecker 基于 GORM 的通用归属查询实现。
//
// 通过 SELECT owner_id FROM <table> WHERE id = ? LIMIT 1 查询；
// 主键字段统一为 id（与项目现有模型约定一致）。
//
// 该实现仅供中间件使用——业务逻辑应走专用 Repository 以复用 SQL 缓存/预编译。
type DBOwnershipChecker struct {
	db *gorm.DB
}

// NewDBOwnershipChecker 构造一个 GORM 归属查询器。
// db 不可为 nil，否则 GetOwnerID 立即返回错误。
func NewDBOwnershipChecker(db *gorm.DB) *DBOwnershipChecker {
	if db == nil {
		return &DBOwnershipChecker{}
	}
	return &DBOwnershipChecker{db: db}
}

// GetOwnerID 查询资源 owner_id。
//
// 错误语义：
//   - db 为 nil / 未装配 → `ErrOwnershipMisconfigured`（调用方 500，绝不返回 (0, nil) 放行）
//   - 记录不存在（`gorm.ErrRecordNotFound`）→ `ErrOwnershipNotFound`（调用方 404）
//   - 其它 DB 错误 → 透传 error（调用方 500）
func (c *DBOwnershipChecker) GetOwnerID(ctx context.Context, table string, resourceID uint) (uint, error) {
	if c == nil {
		return 0, ErrOwnershipMisconfigured
	}
	if table == "" {
		return 0, ErrOwnershipMisconfigured
	}
	// 入参校验先于装配校验：resourceID==0 是"没有这个资源"，与 checker 装没装好无关。
	if resourceID == 0 {
		return 0, ErrOwnershipNotFound
	}
	if c.db == nil {
		return 0, ErrOwnershipMisconfigured
	}
	var ownerID uint
	err := c.db.WithContext(ctx).
		Table(table).
		Select("owner_id").
		Where("id = ?", resourceID).
		Take(&ownerID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, ErrOwnershipNotFound
	}
	if err != nil {
		return 0, err
	}
	return ownerID, nil
}

type ownershipCacheEntry struct {
	ownerID uint
	err     error
	expire  time.Time
}

type ownershipCache struct {
	mu   sync.RWMutex
	data map[string]ownershipCacheEntry
}

func newOwnershipCache() *ownershipCache {
	return &ownershipCache{data: make(map[string]ownershipCacheEntry)}
}

func (c *ownershipCache) get(key string) (ownershipCacheEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.data[key]
	if !ok {
		return ownershipCacheEntry{}, false
	}
	if time.Now().After(e.expire) {
		return ownershipCacheEntry{}, false
	}
	return e, true
}

func (c *ownershipCache) set(key string, entry ownershipCacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = entry
}

var globalOwnershipCache = newOwnershipCache()

// OwnershipPolicy 归属判定策略。
//
// 只影响一类判定：**查到了行、但 owner_id 列是 0**。
type OwnershipPolicy struct {
	allowSystemOwned bool
}

// OwnershipOption 构造归属中间件时的可选策略。
type OwnershipOption func(*OwnershipPolicy)

// WithSystemOwnedAllowed 允许访问 owner_id = 0 的"系统级资源"行。
//
// 默认**不允许**。理由：`owner_id = 0` 既可能是"平台内置行"，也可能是"历史脏数据没填 owner"，
// 两者在库里长得一模一样。默认拒绝 + 显式开启，是唯一不会把脏数据自动变成后门的口径。
func WithSystemOwnedAllowed() OwnershipOption {
	return func(p *OwnershipPolicy) { p.allowSystemOwned = true }
}

func newOwnershipPolicy(opts []OwnershipOption) OwnershipPolicy {
	var p OwnershipPolicy
	for _, o := range opts {
		if o != nil {
			o(&p)
		}
	}
	return p
}

// RequireOwnerWithChecker 归属校验中间件（无缓存）。
//
// **checker 必须非 nil，构造期 panic**：nil checker 若按放行处理，
// 任何路由接上它就等于"一律放行"，而它在 review 里长得像一个安全件。
//
// 判定流程：
//  1. 未登录 → 401；资源 ID 非法 → 400
//  2. checker 返回 `ErrOwnershipNotFound` → **404**（不回 500/403）
//  3. checker 返回其它错误 → 500（fail-closed）
//  4. ownerID == uid → 放行；ownerID == 0 → 404（除非 `WithSystemOwnedAllowed()`）；其它 → 403
func RequireOwnerWithChecker(resourceKey, table string, checker OwnershipChecker, opts ...OwnershipOption) gin.HandlerFunc {
	if checker == nil {
		panic("middleware: RequireOwnerWithChecker 必须注入非 nil OwnershipChecker（nil⇒放行 是 fail-open，已移除）")
	}
	pol := newOwnershipPolicy(opts)
	return func(c *gin.Context) {
		uid := utils.GetUID(c)
		if uid == 0 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    401,
				"message": "未登录或会话失效",
			})
			return
		}
		resourceID, err := strconv.ParseUint(c.Param(resourceKey), 10, 64)
		if err != nil || resourceID == 0 {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": "无效的资源 ID",
			})
			return
		}
		ownerID, err := checker.GetOwnerID(c.Request.Context(), table, uint(resourceID))
		if ownershipVerdict(c, ownerID, err, uid, pol) {
			return
		}
		c.Next()
	}
}

// ownershipVerdict 把 (ownerID, err) 折算成响应；返回 true 表示已 abort（调用方必须 return）。
//
// 无缓存路径与缓存命中路径**必须走同一个函数**：两处各写一遍会造成
// 「同一语义两个实现、其中一个漏判」的缺陷。
func ownershipVerdict(c *gin.Context, ownerID uint, err error, uid uint, pol OwnershipPolicy) bool {
	switch {
	case errors.Is(err, ErrOwnershipNotFound):
		// 404 而不是 403/500：403 等于承认"这个 ID 存在但不归你"（存在性预言机），
		// 500 会把它混进真故障里，告警与重试都失真。
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "资源不存在",
		})
		return true
	case err != nil:
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "归属校验失败",
		})
		return true
	case ownerID == 0:
		if pol.allowSystemOwned {
			return false
		}
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "资源不存在",
		})
		return true
	case ownerID != uid:
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"code":    403,
			"message": "无权访问该资源",
		})
		return true
	}
	return false
}

// defaultOwnershipCacheTTL 归属缓存默认存活时长，与参数中心
// `misc.ownership_cache_ttl` 的 DefaultValue（5 秒）一致。
//
// 5s 这个取值是有取舍的：缓存存在的目的是挡住"拿别人的资源 ID 批量扫"的 DoS，
// 而它同时是一段**越权窗口**——owner 在这 5s 内被改掉，命中缓存的旧请求仍按旧 owner 放行。
// 调长要重新评估这条，调短则缓存基本失去意义（每请求一次 DB 查询）。
const defaultOwnershipCacheTTL = 5 * time.Second

// ownershipCacheTTLProvider 由装配层注入；未注入时回落默认值。
var ownershipCacheTTLProvider = func() time.Duration { return defaultOwnershipCacheTTL }

// SetOwnershipCacheTTLProvider 注入读取函数（装配层调用；测试可注入桩）。nil 视为不注入。
func SetOwnershipCacheTTLProvider(fn func() time.Duration) {
	if fn != nil {
		ownershipCacheTTLProvider = fn
	}
}

// ownershipCacheTTL 取当前 TTL（非正数一律回落默认值）。
func ownershipCacheTTL() time.Duration {
	if d := ownershipCacheTTLProvider(); d > 0 {
		return d
	}
	return defaultOwnershipCacheTTL
}

// ttl 推荐 5s——业务可调，但需评估：过长放大越权窗口，过短失去缓存意义。
// 缓存 key = (租户, table, resourceID)（见 ownershipCacheKey）。
func RequireOwnerWithCacheAndChecker(resourceKey, table string, ttl time.Duration, checker OwnershipChecker, opts ...OwnershipOption) gin.HandlerFunc {
	if checker == nil {
		panic("middleware: RequireOwnerWithCacheAndChecker 必须注入非 nil OwnershipChecker（nil⇒放行 是 fail-open，已移除）")
	}
	if ttl <= 0 {
		ttl = ownershipCacheTTL()
	}
	pol := newOwnershipPolicy(opts)
	return func(c *gin.Context) {
		uid := utils.GetUID(c)
		if uid == 0 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    401,
				"message": "未登录或会话失效",
			})
			return
		}
		resourceID, err := strconv.ParseUint(c.Param(resourceKey), 10, 64)
		if err != nil || resourceID == 0 {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": "无效的资源 ID",
			})
			return
		}
		key := ownershipCacheKey(c.Request.Context(), table, uint(resourceID))
		if entry, ok := globalOwnershipCache.get(key); ok {
			if ownershipVerdict(c, entry.ownerID, entry.err, uid, pol) {
				return
			}
			c.Next()
			return
		}
		ownerID, err := checker.GetOwnerID(c.Request.Context(), table, uint(resourceID))
		// 「查不到」也缓存：否则拿不存在的 ID 批量扫就是一次不漏的 DB 查询。
		// 代价是新建资源在其 ID 被缓存 404 的 ttl 内可能短暂 404，与既有 5s 越权窗口同一取舍。
		globalOwnershipCache.set(key, ownershipCacheEntry{
			ownerID: ownerID,
			err:     err,
			expire:  time.Now().Add(ttl),
		})
		if ownershipVerdict(c, ownerID, err, uid, pol) {
			return
		}
		c.Next()
	}
}

// ownershipCacheKey 归属缓存键 = (租户, 表, 资源 ID)。
//
// 租户段是**必须**的：缓存是进程级全局 map，而在 on 模式下
// 同一条 `SELECT owner_id FROM t WHERE id=?` 在不同租户 ctx 下结果不同
// （隔离 callback 注入 WHERE tenant_id），键里不带租户就等于让一个租户的查询
// 结果直接命中另一个租户的判定 —— 而且 record-not-found 语义是
// "0 = 系统级资源，放行"，缓存串租户的后果是 **fail-open 一个 TTL 窗口**。
// off 模式租户段恒为空（单机部署无租户隔离，键形退化为裸键）。
func ownershipCacheKey(ctx context.Context, table string, resourceID uint) string {
	prefix := ""
	if id, ok := tenant.FromContext(ctx); ok && tenant.IsMultiTenant() {
		prefix = strconv.FormatUint(uint64(id), 10) + "|"
	}
	return prefix + table + ":" + strconv.FormatUint(uint64(resourceID), 10)
}

// InvalidateOwnershipCache 清除单条缓存（用于资源 owner 变更/删除场景）。
// 当前未在 main.go 接入；暴露此入口便于业务层主动失效。
func InvalidateOwnershipCache(table string, resourceID uint) {
	// 两种键形态都要清：off 模式裸键 + on 模式各租户前缀键（前缀未知 ⇒ 遍历匹配）
	legacy := table + ":" + strconv.FormatUint(uint64(resourceID), 10)
	suffix := "|" + legacy
	globalOwnershipCache.mu.Lock()
	defer globalOwnershipCache.mu.Unlock()
	for k := range globalOwnershipCache.data {
		if k == legacy || strings.HasSuffix(k, suffix) {
			delete(globalOwnershipCache.data, k)
		}
	}
}
