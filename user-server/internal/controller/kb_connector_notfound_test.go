// kb_connector_notfound_test.go 知识库连接器的「不存在」语义回归。
//
// 背景（任务三逐路由 curl 实测）：三个端点在资源不存在时的行为都不对：
//
//	PUT    /api/kb-connectors/:id        → 200「已更新」   （GORM Updates 0 行不报错）
//	DELETE /api/kb-connectors/:id        → 200「已删除」   （同上）
//	POST   /api/kb-connectors/:id/sync   → 500「连接器不存在」
//
// 前两条是**静默成功**：页面提示「已更新」，实际什么都没改；后一条把
// 「资源没找到」报成「服务端炸了」。三者都应当是 404。
//
// 判据：不存在的 id ⇒ 404；存在的 id ⇒ 仍按原语义成功（回归保护）。
package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"hivemtk-user/internal/model"
	pkgdb "hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

func setupKBConnectorNotFoundDB(t *testing.T) *gorm.DB {
	t.Helper()
	database := testutil.NewTestDB(t, &model.ExternalKBConnector{})
	if database == nil {
		t.Fatal("测试库不可达：连接器不存在语义判不了（不 Skip，跳过等于没锁）")
	}
	return database
}

func TestKBConnector_NotFoundIs404(t *testing.T) {
	database := setupKBConnectorNotFoundDB(t)
	// 连接器仓储是 NewExternalKBConnectorRepo() ⇒ 取 db.GetDB()；
	// 测试库要通过 db.SetTestDB 注入（与同包其它用例同一手法）。
	prev := pkgdb.GetDB()
	pkgdb.SetTestDB(database)
	t.Cleanup(func() { pkgdb.SetTestDB(prev) })

	row := &model.ExternalKBConnector{Type: "webhttp", Name: "存在性用例", Config: "{}", Enabled: true}
	if err := database.WithContext(context.Background()).Create(row).Error; err != nil {
		t.Fatalf("seed 连接器失败: %v", err)
	}

	router := setupGinEngine()
	router.PUT("/api/kb-connectors/:id", UpdateKBConnector)
	router.DELETE("/api/kb-connectors/:id", DeleteKBConnector)
	router.POST("/api/kb-connectors/:id/sync", SyncKBConnector)

	body, _ := json.Marshal(map[string]any{"name": "x"})

	cases := []struct {
		name   string
		method string
		path   string
		body   []byte
	}{
		{"更新不存在的连接器", "PUT", "/api/kb-connectors/999999", body},
		{"删除不存在的连接器", "DELETE", "/api/kb-connectors/999999", nil},
		{"同步不存在的连接器", "POST", "/api/kb-connectors/999999/sync", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var req *http.Request
			if len(tc.body) > 0 {
				req, _ = http.NewRequest(tc.method, tc.path, bytes.NewReader(tc.body))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req, _ = http.NewRequest(tc.method, tc.path, nil)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != http.StatusNotFound {
				t.Fatalf("资源不存在应回 404，实际 %d，body: %s", w.Code, w.Body.String())
			}
		})
	}
}
