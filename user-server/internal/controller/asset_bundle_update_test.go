package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// W-5 回归：PUT /api/asset-bundle/:id 的回显必须来自库内记录。
//
// 坑：handler 用请求 DTO 新建了一个不带 TenantID 的 model.AssetBundle，
// UpdateBundle 返回后直接把这个对象回显，于是响应里 tenant_id 恒为 0
// （落库是对的——tenant_id 不在更新白名单里，库内原值没被覆盖）。
//
// 本用例的 fixture 里库内记录 tenant_id=42，请求体不携带 tenant_id；
// 判据是响应 data.tenant_id 必须是 42，而不是 0，也不是"请求对象自己"。

// fakeUpdatedBundleRepo 模拟 asset_bundles 的真实更新语义：
//   - Update 只回写白名单列（tenant_id 不在其中，保持库内原值）
//   - FindByID 返回库内快照（而不是调用方传进来的指针）
type fakeUpdatedBundleRepo struct {
	repository.AssetBundleRepository

	stored        *model.AssetBundle
	updateCalls   int
	findByIDCalls int
}

func (f *fakeUpdatedBundleRepo) FindByID(_ context.Context, id int64) (*model.AssetBundle, error) {
	f.findByIDCalls++
	if f.stored == nil || f.stored.ID != id {
		return nil, fmt.Errorf("record not found")
	}
	cp := *f.stored
	cp.Messages = append(model.AssetBundleMessages(nil), f.stored.Messages...)
	return &cp, nil
}

func (f *fakeUpdatedBundleRepo) Update(_ context.Context, m *model.AssetBundle) error {
	if f.stored == nil || f.stored.ID != m.ID {
		return fmt.Errorf("record not found")
	}
	f.updateCalls++
	// 对应 repository.assetBundleUpdateColumns：不含 tenant_id。
	f.stored.Title = m.Title
	f.stored.Description = m.Description
	f.stored.Author = m.Author
	f.stored.Version = m.Version
	f.stored.Scope = m.Scope
	f.stored.Status = m.Status
	f.stored.Industry = m.Industry
	f.stored.Language = m.Language
	f.stored.Tags = m.Tags
	f.stored.Messages = m.Messages
	f.stored.CoverImage = m.CoverImage
	return nil
}

func TestAssetBundleUpdateEchoesTenantIDFromStore(t *testing.T) {
	repo := &fakeUpdatedBundleRepo{stored: &model.AssetBundle{
		TenantID: 42,
		ID:       7,
		AssetID:  "starter-bundle",
		Title:    "旧标题",
		Version:  "1.0.0",
		Scope:    model.AssetBundleScopePrivate,
		Status:   model.AssetBundleStatusDraft,
		Messages: model.AssetBundleMessages{{Role: "system", Content: "欢迎语"}},
	}}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	ctrl := NewAssetBundleController(service.NewAssetBundleService(repo, nil))
	r.PUT("/api/asset-bundle/:id", ctrl.Update)

	body := `{"id":7,"title":"新标题"}`
	req := httptest.NewRequest(http.MethodPut, "/api/asset-bundle/7", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("更新应返回 200，实际 %d，body=%s", w.Code, w.Body.String())
	}
	if repo.updateCalls != 1 {
		t.Fatalf("应落库更新 1 次，实际 %d 次", repo.updateCalls)
	}

	var resp struct {
		Code int `json:"code"`
		Data struct {
			ID       int64  `json:"id"`
			TenantID uint   `json:"tenant_id"`
			Title    string `json:"title"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v，body=%s", err, w.Body.String())
	}
	if resp.Data.TenantID == 0 {
		t.Errorf("回显 tenant_id 不得为 0，body=%s", w.Body.String())
	}
	if resp.Data.TenantID != 42 {
		t.Errorf("回显 tenant_id 应取库内真相 42，实际 %d，body=%s", resp.Data.TenantID, w.Body.String())
	}
	if resp.Data.Title != "新标题" {
		t.Errorf("回显 title 应反映更新后的值，实际 %q", resp.Data.Title)
	}
}
