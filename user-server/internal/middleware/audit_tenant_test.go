package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"hivemtk-user/internal/pkg/tenant"

	"github.com/gin-gonic/gin"
)

type captureAuditSink struct {
	entry     *AuditEntry
	ctxTenant uint
}

func (s *captureAuditSink) Save(ctx context.Context, entry *AuditEntry) error {
	s.entry = entry
	s.ctxTenant, _ = tenant.FromContext(ctx)
	return nil
}

// 审计条目入队后不再持有 request ctx，落库时必须按条目自带租户重建绑定；
// on 模式下无租户 ctx 会被隔离 callback fail-closed，整批日志静默丢失。
func TestAuditManager_SaveKeepsTenantBinding(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	sink := &captureAuditSink{}
	SetAuditSink(sink)
	t.Cleanup(func() { SetAuditSink(nil) })

	m := NewAuditManager()
	m.saveAuditBatch([]*AuditEntry{{TenantID: 42, UserID: 7, Action: "create", Module: "x"}})

	if sink.entry == nil {
		t.Fatal("审计条目未送达 sink")
	}
	if sink.ctxTenant != 42 {
		t.Fatalf("sink 收到的 ctx 租户 = %d，期望 42", sink.ctxTenant)
	}

	// 无归属条目保持无绑定：不得静默落到默认租户（off 模式外一律 fail-closed）。
	sink2 := &captureAuditSink{}
	SetAuditSink(sink2)
	m.saveAuditBatch([]*AuditEntry{{UserID: 7, Action: "create", Module: "x"}})
	if sink2.ctxTenant != 0 {
		t.Fatalf("无归属条目被绑定到租户 %d，期望 0", sink2.ctxTenant)
	}
}

func TestAuditMiddleware_CapturesTenantFromRequest(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/test", nil).
		WithContext(tenant.WithTenantID(context.Background(), 42))

	if got := auditTenantID(c); got != 42 {
		t.Fatalf("auditTenantID = %d，期望 42", got)
	}

	tenant.SetMode(tenant.ModeOff)
	c2, _ := gin.CreateTestContext(httptest.NewRecorder())
	c2.Request = httptest.NewRequest(http.MethodPost, "/api/test", nil)
	if got := auditTenantID(c2); got != tenant.DefaultTenantID {
		t.Fatalf("off 模式 auditTenantID = %d，期望默认租户 %d", got, tenant.DefaultTenantID)
	}
}
