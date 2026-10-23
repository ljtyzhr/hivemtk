package router

// /files 的租户归属校验（T011 静态服务半边，2026-10-23 补）。
//
// 缺陷形状：on 模式下 LocalDriver 把新上传落到 `tenant_{id}/`，但 /files 此前
// 只判"危险扩展名 + 路径穿越"，**完全不看路径里的租户段** —— 任何拿到 URL 的
// 已登录租户都能读别的租户的文件（UUID 不可枚举是唯一防线，而 URL 本身会经
// 导出、日志、转发外流）。tasks.md 的 T011 写着"静态服务校验租户归属"，
// 代码里那半边从来没实现 —— 声明与实现漂移，且没有任何用例会红。
//
// 本组用例钉三件事：
//  1. on 模式：跨租户路径对已绑定租户的请求 ⇒ 404（不是 403，不做存在性预言机）
//  2. on 模式：无租户绑定（访客 <img>、分享页）⇒ 仍放行（公开外链语义不变）
//  3. off 模式：逐字节等价现状（离线单机路径里本就没有租户段，行为零变化）

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"hivemtk-user/internal/pkg/tenant"

	"github.com/gin-gonic/gin"
)

func newTenantFilesGuardServer(t *testing.T) (*gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	mustWrite := func(rel string) {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("ok"), 0o640); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("tenant_5/materials/2026/10/a.pdf")
	mustWrite("tenant_6/materials/2026/10/b.pdf")
	mustWrite("materials/legacy.pdf") // 存量根域文件（T011 只读兼容）
	r := gin.New()
	r.GET("/files/*filepath", serveUploadsGuarded(dir))
	return r, dir
}

func doFilesRequest(t *testing.T, r *gin.Engine, path string, tenantID uint) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if tenantID != 0 {
		req = req.WithContext(tenant.WithTenantID(req.Context(), tenantID))
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code
}

func TestFilesGuard_TenantScopedOnCrossTenantBlocked(t *testing.T) {
	r, _ := newTenantFilesGuardServer(t)
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	if code := doFilesRequest(t, r, "/files/tenant_5/materials/2026/10/a.pdf", 5); code != http.StatusOK {
		t.Fatalf("本租户读自己的文件应 200，实际 %d", code)
	}
	if code := doFilesRequest(t, r, "/files/tenant_6/materials/2026/10/b.pdf", 5); code != http.StatusNotFound {
		t.Fatalf("跨租户读文件应 404（存在性也不暴露），实际 %d", code)
	}
	// 反向：租户 6 读租户 5 的文件同样被拒
	if code := doFilesRequest(t, r, "/files/tenant_5/materials/2026/10/a.pdf", 6); code != http.StatusNotFound {
		t.Fatalf("反向跨租户读文件应 404，实际 %d", code)
	}
}

func TestFilesGuard_NoTenantBindingStaysPublic(t *testing.T) {
	r, _ := newTenantFilesGuardServer(t)
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	// 访客 Widget 的 <img>、分享页等无凭证外链：不因"拿不到租户"而断掉
	if code := doFilesRequest(t, r, "/files/tenant_5/materials/2026/10/a.pdf", 0); code != http.StatusOK {
		t.Fatalf("无租户绑定的公开外链应保持可读（否则访客侧图片全挂），实际 %d", code)
	}
	// 存量根域文件：on 模式下按只读兼容放行（T011 契约）
	if code := doFilesRequest(t, r, "/files/materials/legacy.pdf", 6); code != http.StatusOK {
		t.Fatalf("存量根域文件应保持可读，实际 %d", code)
	}
}

func TestFilesGuard_OffModeUnchanged(t *testing.T) {
	r, _ := newTenantFilesGuardServer(t)
	tenant.SetMode(tenant.ModeOff)

	// off 模式：请求 ctx 即便被塞了租户，也不参与判定（离线单机零回归）
	for _, tc := range []struct {
		path     string
		tenantID uint
	}{
		{"/files/tenant_5/materials/2026/10/a.pdf", 6},
		{"/files/tenant_6/materials/2026/10/b.pdf", 0},
		{"/files/materials/legacy.pdf", 5},
	} {
		if code := doFilesRequest(t, r, tc.path, tc.tenantID); code != http.StatusOK {
			t.Errorf("off 模式 %s 应 200（与历史一致），实际 %d", tc.path, code)
		}
	}
}

func TestTenantPathAllowed_EdgeCases(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	// 租户段必须整段匹配：tenant_51 不能被 tenant_5 前缀骗过，方向两边都要拒
	for _, tc := range []struct {
		rel      string
		ctxID    uint
		expected bool
	}{
		{"tenant_5/materials/a.pdf", 51, false},
		{"/tenant_5/materials/a.pdf", 51, false},
		{"tenant_51/materials/a.pdf", 5, false},
		{"tenant_5/materials/a.pdf", 5, true},
		// 首段不是租户段：存量根域 / 系统级上传 / 附件子目录一律放行
		{"materials/a.pdf", 5, true},
		{"tenant_x/a.pdf", 5, true},
		{"attachments/tenant_5/a.pdf", 5, true},
	} {
		if got := tenantPathAllowed(tenantCtx(tc.ctxID), tc.rel); got != tc.expected {
			t.Errorf("tenantPathAllowed(%q, tenant=%d) = %v，期望 %v", tc.rel, tc.ctxID, got, tc.expected)
		}
	}
}

func tenantCtx(id uint) context.Context {
	return tenant.WithTenantID(context.Background(), id)
}
