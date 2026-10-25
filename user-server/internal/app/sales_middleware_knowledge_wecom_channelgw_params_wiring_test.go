package app

import (
	"testing"
	"time"

	knowledgesvc "hivemtk-user/internal/aiagent/knowledge/service"
	"hivemtk-user/internal/model"
	appsvc "hivemtk-user/internal/service"

	"hivemtk-user/internal/middleware"
	"hivemtk-user/internal/pkg/testutil"

	"github.com/stretchr/testify/require"
)

var wantSalesMiddlewareKnowledgeWired = []string{
	"sales.identity_max_attempts",
	"middleware.audit_flush_interval",
	"middleware.mfa_recent_verify_ttl",
	"knowledge.max_upload_file_size",
	"knowledge.merchant_batch_max_file_size",
	"knowledge.merchant_knowledge_max_len",
}

func resetSalesMiddlewareKnowledgeProviders(t *testing.T) {
	t.Helper()
	// ⚠️ 这段 cleanup 目前是**空转**：六只 setter 的契约都是「传 nil 表示不注入」
	// （`if fn != nil`），所以这里传 nil 一个 provider 都不会被还原。
	// 真把读数兜住的是第二层回落——cleanup 把 GlobalConfigParam 置回 nil，
	// 残留的 provider 闭包再取值时拿到的是无库 stub，于是照样回编译期默认。
	// 也就是说这几条断言现在测的是「两层兜底」，不是「provider 被还原」；
	// 要测后者得给这些包加 test-only 复位口，而 `run.tests:false` 的 lint 会把
	// 生产文件里的 test-only 写侧判成死代码，所以没加。此处保留调用只为形状一致，
	// 别把它读成隔离已成立。
	t.Cleanup(func() {
		appsvc.SetIdentityMaxAttemptsProvider(nil)
		middleware.SetAuditFlushIntervalProvider(nil)
		middleware.SetMfaRecentVerifyTTLProvider(nil)
		knowledgesvc.SetMaxUploadFileSizeProvider(nil)
		knowledgesvc.SetMerchantBatchMaxFileSizeProvider(nil)
		knowledgesvc.SetMerchantKnowledgeMaxLenProvider(nil)
	})
}

func TestWireSalesMiddlewareKnowledgeWecomChannelGWConfigParamsKeys(t *testing.T) {
	resetSalesMiddlewareKnowledgeProviders(t)
	got := WireSalesMiddlewareKnowledgeWecomChannelGWConfigParams()
	require.Equal(t, wantSalesMiddlewareKnowledgeWired, got, "接线键名必须逐条相等且顺序稳定")

	seen := map[string]bool{}
	for _, k := range got {
		require.False(t, seen[k], "接线键名重复：%s", k)
		seen[k] = true
	}
}

func TestWiredSalesMiddlewareKnowledgeKeysExistInSeed(t *testing.T) {
	// 刻意写死期望值而不是从 DefaultParamDefs 里过滤：拼错 group 也算 misc 组，
	// 那样漏接一条根本没人会发现。
	inSeed := map[string]bool{}
	for _, d := range appsvc.DefaultParamDefs() {
		inSeed[d.Group+"."+d.Key] = true
	}
	for _, k := range wantSalesMiddlewareKnowledgeWired {
		require.True(t, inSeed[k], "%s 不在 DefaultParamDefs 里，接线指向了不存在的参数", k)
	}
}

func newSalesMiddlewareKnowledgeParamFixture(t *testing.T) {
	t.Helper()
	resetSalesMiddlewareKnowledgeProviders(t)
	testutil.NewTestDBOrSkip(t, &model.ConfigParam{}, &model.ConfigParamAuditLog{})
	require.NoError(t, appsvc.SeedConfigParams(t.Context()))
	appsvc.SetGlobalForTest(appsvc.NewConfigParamService())
	t.Cleanup(func() { appsvc.SetGlobalForTest(nil) })
}

func TestWireSalesMiddlewareKnowledgeWecomChannelGWConfigParamsReachesReaders(t *testing.T) {
	newSalesMiddlewareKnowledgeParamFixture(t)
	cp := appsvc.GlobalConfigParam()
	bg := t.Context()

	require.NoError(t, cp.UpdateValue(bg, "sales", "identity_max_attempts", "11", 1))
	require.NoError(t, cp.UpdateValue(bg, "middleware", "audit_flush_interval", "17", 1))
	require.NoError(t, cp.UpdateValue(bg, "middleware", "mfa_recent_verify_ttl", "1900", 1))
	require.NoError(t, cp.UpdateValue(bg, "knowledge", "max_upload_file_size", "20971520", 1))
	require.NoError(t, cp.UpdateValue(bg, "knowledge", "merchant_batch_max_file_size", "10485760", 1))
	require.NoError(t, cp.UpdateValue(bg, "knowledge", "merchant_knowledge_max_len", "1234", 1))

	WireSalesMiddlewareKnowledgeWecomChannelGWConfigParams()

	require.Equal(t, 11, appsvc.ProbeIdentityMaxAttempts())
	require.Equal(t, 17*time.Second, middleware.AuditFlushInterval())
	require.Equal(t, 1900*time.Second, middleware.MFARecentVerifyTTL())
	require.Equal(t, int64(20971520), knowledgesvc.MaxUploadFileSize())
	// 与上一条各走各的值：批量导入上限若哪天被并进文档上传那条，这一格会先红。
	require.Equal(t, int64(10485760), knowledgesvc.MerchantBatchMaxFileSize())
	require.Equal(t, 1234, knowledgesvc.MerchantKnowledgeMaxLen())
}

func TestWireSalesMiddlewareKnowledgeWecomChannelGWConfigParamsWithoutDB(t *testing.T) {
	resetSalesMiddlewareKnowledgeProviders(t)
	appsvc.SetGlobalForTest(nil)
	t.Cleanup(func() { appsvc.SetGlobalForTest(nil) })

	got := WireSalesMiddlewareKnowledgeWecomChannelGWConfigParams()
	require.Len(t, got, 6, "无库时仍应报告接了 6 条，只是取值全部回落兜底")

	require.Equal(t, appsvc.DefaultIdentityMaxAttempts, appsvc.ProbeIdentityMaxAttempts())
	require.Equal(t, middleware.DefaultAuditFlushInterval, middleware.AuditFlushInterval())
	require.Equal(t, middleware.DefaultMfaRecentVerifyTTL, middleware.MFARecentVerifyTTL())
	require.Equal(t, knowledgesvc.DefaultMaxUploadFileSize, knowledgesvc.MaxUploadFileSize())
	require.Equal(t, knowledgesvc.DefaultMerchantBatchMaxFileSize, knowledgesvc.MerchantBatchMaxFileSize())
	require.Equal(t, knowledgesvc.DefaultMerchantKnowledgeMaxLen, knowledgesvc.MerchantKnowledgeMaxLen())
}
