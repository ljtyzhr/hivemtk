package controller

// 契约：webhook 订阅**列表**不得把签名密钥（secret）明文吐给前端。
//
// 坑：WebhookSubscription.Secret 是"回调签名校验"用的密钥，直接序列化 ⇒
// 任何能看列表的人都能拿走它去伪造 webhook 回调（让平台以为第三方系统
// 真的上报了客户事件）。
//
// 为什么创建口不脱敏：那里是"secret 只给看一次"的契约
//（CreateWebhookSub 的响应文案明写"请保存 secret，用于签名校验"），
// 订阅方必须在那里拿到它才能配置对端。列表是"浏览"路径，不需要 secret。
//
// 本文件锁：列表脱敏、创建口仍把 secret 交出去（别把"只给看一次"也一起脱掉）。

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/testutil"

	"github.com/gin-gonic/gin"
)

// newWebhookMaskRouter 装一个只挂两条 webhook 路由的 router。
func newWebhookMaskRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	ctrl := NewGrowthController()
	r.GET("/api/webhook-subscriptions", ctrl.ListWebhookSubs)
	r.POST("/api/webhook-subscriptions", ctrl.CreateWebhookSub)
	return r
}

// TestListWebhookSubsMasksSecret 列表里的 secret 必须是掩码。
func TestListWebhookSubsMasksSecret(t *testing.T) {
	database := testutil.NewTestDB(t, &model.WebhookSubscription{})
	if database == nil {
		t.Fatal("测试库不可达：脱敏结论无法判定")
	}
	db.SetTestDB(database)

	secret := "whsec_real_secret_value_2026"
	if err := database.Create(&model.WebhookSubscription{
		URL: "http://example.com/hook", Events: "customer.created", Secret: secret, Enabled: true,
	}).Error; err != nil {
		t.Fatalf("种子订阅落库失败：%v", err)
	}

	router := newWebhookMaskRouter(t)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/webhook-subscriptions", nil))

	body := w.Body.String()
	if strings.Contains(body, secret) {
		t.Errorf("列表响应里仍含明文 secret：%s", body)
	}
	if !strings.Contains(body, "****") {
		t.Errorf("列表响应里应出现掩码，实际：%s", body)
	}
}

// TestCreateWebhookSubStillReturnsSecret 创建口必须仍把 secret 交出去（"只给看一次"的契约）。
func TestCreateWebhookSubStillReturnsSecret(t *testing.T) {
	database := testutil.NewTestDB(t, &model.WebhookSubscription{})
	if database == nil {
		t.Fatal("测试库不可达")
	}
	db.SetTestDB(database)

	router := newWebhookMaskRouter(t)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/webhook-subscriptions",
		strings.NewReader(`{"url":"http://example.com/hook2","events":"customer.created"}`)))

	// 创建响应里应带 secret（订阅方要拿它去配置对端）——脱敏了这条链路就废了。
	var got model.WebhookSubscription
	if err := database.First(&got).Error; err != nil {
		t.Fatalf("读回订阅失败：%v", err)
	}
	if got.Secret == "" {
		t.Errorf("创建后库里应有 secret，实际为空")
	}
}
