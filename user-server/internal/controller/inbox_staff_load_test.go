package controller

// 客服负载这条口的判据是"参数=归属"：URL 里的 :staff 就是被查询的那个人，
// 所以它不在行级登记表里（登记表一格是"某张表的某行属于谁"），只能在这一层判。
// 这里不连库：InboxService 的零值没有 inboxRepo，StaffLoad 会直接回 0，
// 正例/反例都在进服务之前分出。

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

func inboxStaffLoadCall(t *testing.T, role, username, staffParam string) (int, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("role", role)
		c.Set("username", username)
		c.Next()
	})
	ctrl := NewInboxController(&service.InboxService{})
	engine.GET("/api/inbox/staff/:staff/load", ctrl.StaffLoad)

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/inbox/staff/"+staffParam+"/load", nil))
	return w.Code, w.Body.String()
}

func TestInboxStaffLoad_NonAdminAskingForOtherStaffDenies(t *testing.T) {
	// 三枚反例共用一个"恒放"盲区：查别人、查一个不存在的人、读不出自己的身份，
	// 三种都必须拦；只测第一种的话，"拿 username 当判据但 username 为空时也放过"会绿。
	for _, c := range []struct{ name, role, username, staff string }{
		{"非超管查同事", "customer_service", "alice", "bob"},
		{"非超管查任意名字", "staff", "alice", "admin"},
		{"身份读不出来", "customer_service", "", "alice"},
	} {
		code, body := inboxStaffLoadCall(t, c.role, c.username, c.staff)
		if code != http.StatusForbidden {
			t.Errorf("%s：应 403，实得 %d / %s", c.name, code, strings.TrimSpace(body))
		}
		if !strings.Contains(body, "无权查看他人的负载") {
			t.Errorf("%s：403 应给出负载专属文案，实得 %s", c.name, strings.TrimSpace(body))
		}
	}
}

func TestInboxStaffLoad_OwnerAndAdminReachService(t *testing.T) {
	// 正例：查自己（非超管）与超管查任何人都要进服务层。没有这一档，上一条恒拦也能骗过。
	for _, c := range []struct{ name, role, username, staff string }{
		{"非超管查自己", "customer_service", "alice", "alice"},
		{"超管查别人", "admin", "admin", "alice"},
	} {
		code, body := inboxStaffLoadCall(t, c.role, c.username, c.staff)
		if code != http.StatusOK {
			t.Errorf("%s：应进服务层回 200，实得 %d / %s", c.name, code, strings.TrimSpace(body))
		}
		if !strings.Contains(body, `"staff":"`+c.staff+`"`) {
			t.Errorf("%s：响应体应回显被查坐席 %q，实得 %s", c.name, c.staff, strings.TrimSpace(body))
		}
	}
}
