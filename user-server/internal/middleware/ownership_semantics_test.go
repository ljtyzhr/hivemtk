package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"hivemtk-user/internal/pkg/testutil"

	"github.com/gin-gonic/gin"
)

// 本文件钉死 A2（`未完成任务.md` §9）收口后的归属中间件语义。
//
// 修前形态（2026-10-23 前，逐条对应一个 fail-open / 语义漂移点）：
//  1. `RequireOwner(table, key)` 与 `RequireOwnerWithCache` 不带 checker ⇒ 请求期 `c.Next()` 放行；
//  2. `DBOwnershipChecker` 在 db==nil / table=="" / resourceID==0 时返回 `(0, nil)`，落进"系统级放行"；
//  3. 记录不存在返回 `(0, gorm.ErrRecordNotFound)` ⇒ 中间件 500（不是 404）；
//  4. 函数注释写"记录不存在 → 0 + nil，视为系统级资源"，与实现方向相反。

// ownershipProbeRow 仅用于本文件：验证 DB 侧的 owner_id 读取与"查不到"的错误映射。
type ownershipProbeRow struct {
	ID      uint `gorm:"primaryKey"`
	OwnerID uint
}

func (ownershipProbeRow) TableName() string { return "ownership_probe_rows" }

// TestDBOwnershipChecker_RealDBMapping 真库验证：查到 ⇒ 返回 owner_id；查不到 ⇒ ErrOwnershipNotFound。
//
// 为什么必须接真库：`gorm.ErrRecordNotFound` 的判定与错误包装是 GORM 驱动的行为，
// 用 fake checker 测不到——而这条映射正是"404 vs 500"的唯一分界。
func TestDBOwnershipChecker_RealDBMapping(t *testing.T) {
	database := testutil.NewTestDBOrSkip(t, &ownershipProbeRow{})
	if database == nil {
		return
	}
	if err := database.Create(&ownershipProbeRow{ID: 1, OwnerID: 42}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	checker := NewDBOwnershipChecker(database)

	id, err := checker.GetOwnerID(context.Background(), "ownership_probe_rows", 1)
	if err != nil || id != 42 {
		t.Fatalf("want (42,nil), got (%d,%v)", id, err)
	}

	id, err = checker.GetOwnerID(context.Background(), "ownership_probe_rows", 404404)
	if !errors.Is(err, ErrOwnershipNotFound) {
		t.Fatalf("查不到必须返回 ErrOwnershipNotFound（⇒404），实际 (%d,%v)", id, err)
	}
}

// TestOwnershipVerdictMatrix 判定矩阵：一个函数覆盖四个出口，缓存路径与非缓存路径共用它。
func TestOwnershipVerdictMatrix(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name      string
		ownerID   uint
		err       error
		allowSys  bool
		wantCode  int
		wantAbort bool
	}{
		{"同租户同用户", 42, nil, false, http.StatusOK, false},
		{"他人资源", 99, nil, false, http.StatusForbidden, true},
		{"不存在", 0, ErrOwnershipNotFound, false, http.StatusNotFound, true},
		{"不存在 + 允许系统行（仍 404）", 0, ErrOwnershipNotFound, true, http.StatusNotFound, true},
		{"DB 故障", 0, errors.New("db down"), false, http.StatusInternalServerError, true},
		{"系统级行 默认拒绝", 0, nil, false, http.StatusNotFound, true},
		{"系统级行 显式放行", 0, nil, true, http.StatusOK, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/x/1", nil)

			pol := OwnershipPolicy{allowSystemOwned: tc.allowSys}
			aborted := ownershipVerdict(c, tc.ownerID, tc.err, 42, pol)

			if aborted != tc.wantAbort {
				t.Fatalf("aborted=%v want %v", aborted, tc.wantAbort)
			}
			if tc.wantAbort && w.Code != tc.wantCode {
				t.Fatalf("code=%d want %d body=%s", w.Code, tc.wantCode, w.Body.String())
			}
		})
	}
}

// TestRequireOwner_NotFoundNotCachedAs500 缓存路径下"不存在"也必须是 404（修前缓存的是 500）
func TestRequireOwner_NotFoundNotCachedAs500(t *testing.T) {
	checker := &fakeOwnershipChecker{err: ErrOwnershipNotFound}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", uint(42)); c.Next() })
	r.GET("/c/:id", RequireOwnerWithCacheAndChecker("id", "cached_notfound_table", 5*time.Second, checker), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/c/777", nil))
		if w.Code != http.StatusNotFound {
			t.Fatalf("iter=%d want 404, got %d body=%s", i, w.Code, w.Body.String())
		}
	}
	if got := checker.calls.Load(); got != 1 {
		t.Fatalf("404 应被缓存（避免拿不存在的 ID 批量扫表），checker 调用次数 = %d", got)
	}
}
