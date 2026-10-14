package app

import (
	"strings"
	"testing"
)

// TestFeishuRecallAPIError 钉住飞书撤回接口"成败只看业务码"的判定。
//
// 背景：飞书开放平台成功与失败都回 HTTP 200，业务码在 body 的 `code`。原先 recallFeishu
// 只判 StatusCode，于是 `{"code":230002,"msg":"Bad Request"}` 这类"平台没撤掉"的响应
// 会一路 return nil，被 Recall 的调用方当成撤回成功报出去——撤回这个动作最不能接受的
// 就是谎报成功。本测试锁死"code != 0 ⇒ 返回 error"这条。
func TestFeishuRecallAPIError(t *testing.T) {
	t.Run("code=0 视为成功", func(t *testing.T) {
		if err := feishuRecallAPIError([]byte(`{"code":0,"msg":"success","data":{}}`)); err != nil {
			t.Fatalf("code=0 应判成功, got err = %v", err)
		}
	})

	t.Run("code!=0 且 HTTP 200 时必须报错", func(t *testing.T) {
		// 这条就是原 bug：HTTP 200 + 非 0 业务码 = 平台拒绝了撤回。
		err := feishuRecallAPIError([]byte(`{"code":230002,"msg":"Bad Request"}`))
		if err == nil {
			t.Fatal("code!=0 必须返回 error（修复前此处会静默判成功）")
		}
		if !strings.Contains(err.Error(), "230002") {
			t.Errorf("错误信息应带上业务码, got %q", err.Error())
		}
		if !strings.Contains(err.Error(), "Bad Request") {
			t.Errorf("错误信息应带上 errmsg, got %q", err.Error())
		}
	})

	t.Run("常见撤回拒绝码都判失败", func(t *testing.T) {
		// 230002 参数错误 / 230013 无权限 / 230020 已撤回 等，均为 HTTP 200 下的失败。
		for _, body := range []string{
			`{"code":230013,"msg":"no permission"}`,
			`{"code":230020,"msg":"message already recalled"}`,
			`{"code":230002,"msg":"message not found"}`,
		} {
			if err := feishuRecallAPIError([]byte(body)); err == nil {
				t.Errorf("%s 应判失败, 但返回了 nil", body)
			}
		}
	})

	t.Run("响应体不是合法 JSON 时报错而非放行", func(t *testing.T) {
		// 网关兜底回前端页面这类非 JSON 响应，不能当作撤回成功。
		if err := feishuRecallAPIError([]byte(`<html>502</html>`)); err == nil {
			t.Fatal("非 JSON 响应体必须报错, 不能放行")
		}
	})
}
