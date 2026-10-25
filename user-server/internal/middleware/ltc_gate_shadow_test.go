package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// E-7 shadow 闸门行为（规划 §4.7 验收第 1 条）：
//
//	① 阶段关着 ⇒ 请求**不阻断**（200 进 handler）、would-refuse 计数 +1；
//	② 阶段开着 ⇒ 放行、计数不变。
//
// 与 ltc_gate_test.go 共用 stubLTCReader / sideEffects。
func TestLTCStageGateShadow_RecordsWithoutBlocking(t *testing.T) {
	gin.SetMode(gin.TestMode)
	se := &sideEffects{}
	r := gin.New()
	// 关闭态（master off ⇒ 全阶段 inactive）
	r.GET("/ltc/shadow", ltcStageGateShadow(service.LTCStageQuote, &stubLTCReader{
		cfg: &service.LTCConfig{Enabled: false},
	}), se.handler())

	before := LTCShadowWouldRefuse()[service.LTCStageQuote]
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ltc/shadow", nil))

	// ① 不阻断：handler 必达，200
	assert.Equal(t, http.StatusOK, w.Code, "shadow 闸门不得拦截（只记录）")
	assert.Equal(t, 1, se.handlerCalls, "shadow 态请求必须进业务 handler")
	// ① 计入"本应拒绝"读数
	assert.Equal(t, before+1, LTCShadowWouldRefuse()[service.LTCStageQuote],
		"shadow 期必须记入 would-refuse 读数")

	// ② 开着 ⇒ 放行且计数不变
	se2 := &sideEffects{}
	r2 := gin.New()
	stages := service.LTCStages{Quote: true}
	r2.GET("/ltc/shadow", ltcStageGateShadow(service.LTCStageQuote, &stubLTCReader{
		cfg: &service.LTCConfig{Enabled: true, StagesEnabled: stages},
	}), se2.handler())
	w2 := httptest.NewRecorder()
	r2.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/ltc/shadow", nil))
	assert.Equal(t, http.StatusOK, w2.Code)
	assert.Equal(t, 1, se2.handlerCalls)
	assert.Equal(t, before+1, LTCShadowWouldRefuse()[service.LTCStageQuote],
		"启用阶段不得计入 would-refuse")
}

var _ = context.Background // 保留 import 对齐（stubLTCReader.Config 需要 ctx 形参）

// D-1 转强制后的装配语义锁：GatedGroup 挂的是**强制**门——
// 阶段关着 ⇒ 409 拒绝、业务 handler 不可达；阶段开着 ⇒ 放行。
// （shadow 中间件本身保留，历史观察语义见上测试。）
func TestGatedGroup_EnforcesWhenStageDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prev := service.GlobalLTCConfig()
	t.Cleanup(func() { service.SetGlobalLTCConfig(prev) })

	// 关闭态（master off ⇒ 全阶段 inactive）：必须 409 且 handler 不可达
	service.SetGlobalLTCConfig(service.NewLTCConfigServiceWithStore(&ltcStubKV{rows: map[string]string{}}))
	se := &sideEffects{}
	r := gin.New()
	auth := r.Group("/api")
	GatedGroup(auth, service.LTCStageQuote).GET("/quote", se.handler())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/quote", nil))
	assert.Equal(t, http.StatusConflict, w.Code, "强制闸在阶段关闭时必须 409")
	assert.Equal(t, 0, se.handlerCalls, "强制闸拦下时业务 handler 不可达")
	assert.Contains(t, w.Body.String(), "LTC", "409 响应体应携带 LTC 语义数据")
}

// ltcStubKV 满足 service 侧未导出存储接口的形状（同 router 包 memKV 的做法）。
type ltcStubKV struct {
	rows map[string]string
}

func (m *ltcStubKV) Available() bool { return true }
func (m *ltcStubKV) Get(ctx context.Context, key string) (string, error) {
	return m.rows[key], nil
}
func (m *ltcStubKV) Upsert(ctx context.Context, key, value string) (string, error) {
	m.rows[key] = value
	return value, nil
}
func (m *ltcStubKV) EnsureTable(ctx context.Context) error { return nil }
