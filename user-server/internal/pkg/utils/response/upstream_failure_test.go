package response

// 契约：上游/渠道失败**不是**"记录不存在"。
//
// 坑：ErrorFromDB 按**错误文本**判定"记录不存在"（文本含 "not found"/"不存在" ⇒ 404），
// 而渠道原文常自带这些词 —— 阿里云短信回 "Specified access key is not found." ——
// 于是"密钥填错了"落成 404「记录不存在」，把操作者引去查记录而不是改密钥。
//
// 修法：错误自己声明它是上游失败（bizerr 5xxx）⇒ 走 502，且消息用 BizError 的文案
// （带渠道给的真原因），不退回 fallbackMsg。
//
// 本文件锁两条腿：
//  1. 上游失败 ⇒ 502（不是 404），且消息里能看到渠道给的原因；
//  2. 本地"记录不存在" ⇒ 仍然是 404（不能为修上游误伤本地语义）。

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bizerr "hivemtk-user/internal/domain/errors"

	"github.com/gin-gonic/gin"
)

func newUpstreamCtx(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	return ctx, rec
}

// TestErrorFromDB_UpstreamFailureIs502Not404 上游失败 ⇒ 502，且带真原因。
func TestErrorFromDB_UpstreamFailureIs502Not404(t *testing.T) {
	ctx, rec := newUpstreamCtx(t)

	err := bizerr.Wrap(bizerr.CodePlatformUnavail,
		"发送短信失败（渠道不可达或凭证无效）：Specified access key is not found.",
		errors.New("Specified access key is not found."))

	ErrorFromDB(ctx, err, "发送草稿失败")

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("上游失败应映射成 502，实际 %d，body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "凭证无效") {
		t.Errorf("响应消息必须带渠道给的真原因（凭证无效），实际 body=%s", body)
	}
	if strings.Contains(body, "记录不存在") {
		t.Errorf("上游失败不得被翻译成「记录不存在」，实际 body=%s", body)
	}
}

// TestErrorFromDB_LocalNotFoundStill404 本地"记录不存在"仍然是 404。
func TestErrorFromDB_LocalNotFoundStill404(t *testing.T) {
	ctx, rec := newUpstreamCtx(t)

	ErrorFromDB(ctx, errors.New("记录不存在"), "获取短链")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("本地记录不存在应映射成 404，实际 %d，body=%s", rec.Code, rec.Body.String())
	}
}
