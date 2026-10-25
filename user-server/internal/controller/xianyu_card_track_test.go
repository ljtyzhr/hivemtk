package controller

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
	cardpkg "hivemtk-user/internal/reach/card"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// xianyuTrackStatsStub 只记录 handler 是否鉴权通过并把埋点转发到服务层，
// 让三条安全腿不依赖 PostgreSQL；真正的落库语义由 TestXianyuTrack_ValidToken_Writes 覆盖。
type xianyuTrackStatsStub struct {
	recorded []string
	lastIP   string
	lastUA   string
	lastRef  string
}

func (s *xianyuTrackStatsStub) GetCardStats(context.Context, uint, string, string) (*dto.XianyuCardStatsResponse, error) {
	return nil, nil
}

func (s *xianyuTrackStatsStub) GetOverallStats(context.Context, string, string) (*dto.XianyuCardOverallStatsResponse, error) {
	return nil, nil
}

func (s *xianyuTrackStatsStub) GetCardStatsRaw(context.Context, uint, string, string) (*dto.CardStatsData, error) {
	return nil, nil
}

func (s *xianyuTrackStatsStub) GetOverallStatsRaw(context.Context, string, string) (*dto.OverallStatsData, error) {
	return nil, nil
}

func (s *xianyuTrackStatsStub) RecordView(_ context.Context, _ uint, ip, userAgent, referer string) error {
	return s.record("view", ip, userAgent, referer)
}

func (s *xianyuTrackStatsStub) RecordClick(_ context.Context, _ uint, ip, userAgent, referer string) error {
	return s.record("click", ip, userAgent, referer)
}

func (s *xianyuTrackStatsStub) RecordShare(_ context.Context, _ uint, ip, userAgent, referer string) error {
	return s.record("share", ip, userAgent, referer)
}

func (s *xianyuTrackStatsStub) record(kind, ip, userAgent, referer string) error {
	s.recorded = append(s.recorded, kind)
	s.lastIP, s.lastUA, s.lastRef = ip, userAgent, referer
	return nil
}

// setupXianyuTrackTestDB 让本用例文件可以单独跑（不依赖同包其他测试夹具）：
// controller 包目前有并行会话留下的旧签名用例，整包 go test 会因别人的文件编译红。
func setupXianyuTrackTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	return testutil.NewTestDB(t,
		&model.XianyuCard{},
		&model.XianyuCardActivity{},
	)
}

// newXianyuTrackTestRouter 只挂三条公网埋点写口，模拟生产里的注册面
// （生产由 router.setupXianyuCardTrackRoutes 注册，公网组 + PublicIDTenant）。
// 埋点 handler 不依赖 XianyuCardService，传 nil 即可。
func newXianyuTrackTestRouter(stats service.XianyuCardStatsService) *gin.Engine {
	ctrl := NewXianyuCardController(nil, stats)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/xianyu/:id/click", ctrl.RecordClick)
	router.POST("/api/xianyu/:id/share", ctrl.RecordShare)
	router.POST("/api/xianyu/:id/view", ctrl.RecordView)
	return router
}

func seedXianyuTrackCard(t *testing.T, database *gorm.DB) uint {
	t.Helper()
	card := &model.XianyuCard{Title: "track-test", Description: "d"}
	if err := database.Create(card).Error; err != nil {
		t.Fatalf("种卡片失败: %v", err)
	}
	return card.ID
}

func doXianyuTrackReq(router *gin.Engine, id, action, token string) *httptest.ResponseRecorder {
	body := []byte(`{"ip":"203.0.113.7","user_agent":"ua-test","referer":"https://ref.example"}`)
	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/api/xianyu/%s/%s", id, action), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set(cardpkg.XianyuReportTokenHeader, token)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func countXianyuActivity(t *testing.T, database *gorm.DB, cardID uint, activityType string) int64 {
	t.Helper()
	var n int64
	if err := database.Model(&model.XianyuCardActivity{}).
		Where("card_id = ? AND activity_type = ?", cardID, activityType).
		Count(&n).Error; err != nil {
		t.Fatalf("统计活动失败: %v", err)
	}
	return n
}

// TestXianyuTrack_NoToken_401 不带凭证 → 401，且不会把埋点转发给服务层。
func TestXianyuTrack_NoToken_401(t *testing.T) {
	stats := &xianyuTrackStatsStub{}
	router := newXianyuTrackTestRouter(stats)

	w := doXianyuTrackReq(router, "42", "click", "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("不带凭证应 401，实际 %d body=%s", w.Code, w.Body.String())
	}
	if len(stats.recorded) != 0 {
		t.Fatalf("鉴权失败不应写埋点，实际转发 %v", stats.recorded)
	}
}

// TestXianyuTrack_TokenIDMismatch_403 凭证与 URL 里的 id 不匹配 → 403。
func TestXianyuTrack_TokenIDMismatch_403(t *testing.T) {
	token, err := cardpkg.GenerateXianyuReportToken(42, 0)
	if err != nil {
		t.Fatalf("签发凭证失败: %v", err)
	}
	stats := &xianyuTrackStatsStub{}
	router := newXianyuTrackTestRouter(stats)

	w := doXianyuTrackReq(router, "43", "click", token)
	if w.Code != http.StatusForbidden {
		t.Fatalf("凭证与 id 不匹配应 403，实际 %d body=%s", w.Code, w.Body.String())
	}
	if len(stats.recorded) != 0 {
		t.Fatalf("鉴权失败不应写埋点，实际转发 %v", stats.recorded)
	}
}

// TestXianyuTrack_ValidToken_Dispatches 正确凭证 → 200，且按 URL 动作转发到对应埋点。
func TestXianyuTrack_ValidToken_Dispatches(t *testing.T) {
	token, err := cardpkg.GenerateXianyuReportToken(42, 0)
	if err != nil {
		t.Fatalf("签发凭证失败: %v", err)
	}
	stats := &xianyuTrackStatsStub{}
	router := newXianyuTrackTestRouter(stats)

	for _, action := range []string{"click", "share", "view"} {
		w := doXianyuTrackReq(router, "42", action, token)
		if w.Code != http.StatusOK {
			t.Fatalf("%s 带正确凭证应 200，实际 %d body=%s", action, w.Code, w.Body.String())
		}
	}

	want := []string{"click", "share", "view"}
	if len(stats.recorded) != len(want) {
		t.Fatalf("应转发 %d 条埋点，实际 %v", len(want), stats.recorded)
	}
	for i := range want {
		if stats.recorded[i] != want[i] {
			t.Fatalf("第 %d 条埋点应转发 %q，实际 %q", i, want[i], stats.recorded[i])
		}
	}
	if stats.lastIP != "203.0.113.7" || stats.lastUA != "ua-test" || stats.lastRef != "https://ref.example" {
		t.Errorf("请求体字段应原样下传，实际 ip=%q ua=%q referer=%q", stats.lastIP, stats.lastUA, stats.lastRef)
	}
}

// TestXianyuTrack_ValidToken_Writes 正确凭证 → 200，埋点按原语义落库。
func TestXianyuTrack_ValidToken_Writes(t *testing.T) {
	database := setupXianyuTrackTestDB(t)
	cardID := seedXianyuTrackCard(t, database)
	router := newXianyuTrackTestRouter(service.NewXianyuCardStatsService(database))

	token, err := cardpkg.GenerateXianyuReportToken(cardID, 0)
	if err != nil {
		t.Fatalf("签发凭证失败: %v", err)
	}

	for _, action := range []string{"click", "share"} {
		w := doXianyuTrackReq(router, fmt.Sprint(cardID), action, token)
		if w.Code != http.StatusOK {
			t.Fatalf("%s 带正确凭证应 200，实际 %d body=%s", action, w.Code, w.Body.String())
		}
		if n := countXianyuActivity(t, database, cardID, action); n != 1 {
			t.Errorf("%s 应落 1 行活动，实际 %d", action, n)
		}
	}

	w := doXianyuTrackReq(router, fmt.Sprint(cardID), "view", token)
	if w.Code != http.StatusOK {
		t.Fatalf("view 带正确凭证应 200，实际 %d body=%s", w.Code, w.Body.String())
	}
	if n := countXianyuActivity(t, database, cardID, "view"); n != 1 {
		t.Errorf("view 应落 1 行活动，实际 %d", n)
	}
	var got model.XianyuCard
	if err := database.First(&got, cardID).Error; err != nil {
		t.Fatalf("读回卡片失败: %v", err)
	}
	if got.ViewCount != 1 {
		t.Errorf("view 应保持原有语义把 view_count 加 1，实际 %d", got.ViewCount)
	}
	var act model.XianyuCardActivity
	if err := database.Where("card_id = ? AND activity_type = ?", cardID, "view").First(&act).Error; err != nil {
		t.Fatalf("读回活动失败: %v", err)
	}
	if act.IP != "203.0.113.7" || act.UserAgent != "ua-test" || act.Referer != "https://ref.example" {
		t.Errorf("请求体字段应原样落库，实际 ip=%q ua=%q referer=%q", act.IP, act.UserAgent, act.Referer)
	}
}
