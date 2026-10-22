package platform

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"hivemtk-user/internal/config"
)

// TestNoticeClient_TreatsCode200AsSuccess 钉死「平台成功码是 200 而不是 0」。
//
// 平台侧 response.Success 写的是 `{"code":200,"msg":"ok",...}`
// （platform-server/internal/utils/response/response.go:18），
// 而仓内既有的 envelopeRefusal 把 `0` 与 `200` **都**当成功。
//
// 本用例钉住这个口径：只认 0 会把每一次成功读成"平台拒绝"——
// 生产日志实证过：
//
//	[notice-sync] 拉取平台消息失败(tenant=tz3rmdj): 平台拒绝通知接口: code=200 msg=ok
//
// 这类误判的特征是"每次都失败且失败原因自相矛盾"（code=200 却说拒绝）。
//
// 用 httptest 起真服务器，让请求真的走一遍 Do → 信封解读，
// 而不是把判定式抄一遍（那只是同义反复，测不出任何东西）。
func TestNoticeClient_TreatsCode200AsSuccess(t *testing.T) {
	cases := []struct {
		name    string
		code    int
		wantErr bool
	}{
		{name: "平台成功码 200", code: 200, wantErr: false},
		{name: "兼容 0", code: 0, wantErr: false},
		{name: "参数错误 400", code: 400, wantErr: true},
		{name: "签名错误 401", code: 401, wantErr: true},
		{name: "权限错误 403", code: 403, wantErr: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"code": c.code,
					"msg":  "ok",
					"data": map[string]any{"list": []any{}},
				})
			}))
			defer srv.Close()

			withPlatformConfig(t, &config.PlatformConfig{APIURL: srv.URL, Secret: "s"})

			nc := &NoticeClient{client: NewPlatformClient("mk")}
			_, err := nc.List(t.Context(), "abc1234", 0)

			if c.wantErr && err == nil {
				t.Fatalf("信封码 %d 应当判为失败，实际成功", c.code)
			}
			if !c.wantErr && err != nil {
				t.Fatalf("信封码 %d 应当判为成功，实际报错: %v", c.code, err)
			}
		})
	}
}
