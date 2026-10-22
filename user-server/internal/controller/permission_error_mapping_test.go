// permission_error_mapping_test.go 权限管理端点的服务层错误映射。
//
// 背景（任务三逐路由 curl 实测）：`PUT /api/system/permissions/:id/password`
// 打初始超管（id=1）时，服务层回的是一条**业务规则**
// （service.ErrInitialAdminProtected「初始超管账号的密码不允许通过此入口修改」），
// 但控制器的 writePermissionServiceError 只把 ErrInvalidInput 判成 400，
// 其余一律落 ErrorFromDB ⇒ **500**。
//
// 500 的含义是「服务端炸了」，而这里服务端好得很 —— 是这条请求越了界。
// 调用方（页面、监控）会把业务拒绝误读成故障。
//
// 判据：ErrInitialAdminProtected ⇒ 400（与其它改密入口一致：自助改密、
// 管理员重置、邮件重置三个入口都回 400，只有这一个回 500）。
package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

func TestWritePermissionServiceError_InitialAdminProtectedIs400(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{
			name: "初始超管受保护 ⇒ 400（不是 500）",
			err:  service.ErrInitialAdminProtected,
			want: http.StatusBadRequest,
		},
		{
			name: "ErrInvalidInput ⇒ 400",
			err:  service.ErrInvalidInput,
			want: http.StatusBadRequest,
		},
		{
			name: "包了一层的 ErrInvalidInput ⇒ 400",
			err:  errors.New("不能重置自己的密码: " + service.ErrInvalidInput.Error()),
			want: http.StatusBadRequest,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("PUT", "/api/system/permissions/1/password", nil)

			writePermissionServiceError(c, tc.err)

			if w.Code != tc.want {
				t.Fatalf("状态码 = %d，期望 %d，body: %s", w.Code, tc.want, w.Body.String())
			}
		})
	}
}
