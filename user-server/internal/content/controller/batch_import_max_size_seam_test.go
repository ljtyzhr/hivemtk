package controller

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"hivemtk-user/internal/content/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// resetBatchImportMaxSizeMBProvider 把读取口复位到「未注入」。
// setter 的 nil 语义是「不注入」而不是「复位」，所以只能重新注入兜底常量。
func resetBatchImportMaxSizeMBProvider() {
	batchImportMaxSizeMBProvider = func() int { return DefaultBatchImportMaxSizeMB }
}

// TestBatchImportMaxSizeMBProviderSeam 逐格钉死 misc.batch_import_max_file_mb 的读取口。
func TestBatchImportMaxSizeMBProviderSeam(t *testing.T) {
	t.Cleanup(resetBatchImportMaxSizeMBProvider)
	resetBatchImportMaxSizeMBProvider()

	maxBytes, maxMB := batchImportMaxUploadSize()
	require.Equal(t, 50, maxMB, "兜底必须等于接线前写死的那个数")
	require.Equal(t, int64(50*1024*1024), maxBytes)

	SetBatchImportMaxSizeMBProvider(func() int { return 20 })
	maxBytes, maxMB = batchImportMaxUploadSize()
	require.Equal(t, 20, maxMB, "注入值没进读取口")
	require.Equal(t, int64(20*1024*1024), maxBytes, "MB→字节的换算不是 1024×1024")

	for _, bad := range []int{0, -1} {
		SetBatchImportMaxSizeMBProvider(func() int { return bad })
		maxBytes, maxMB = batchImportMaxUploadSize()
		require.Equal(t, DefaultBatchImportMaxSizeMB, maxMB, "注入 %d 时应回落兜底", bad)
		require.Positive(t, maxBytes, "上限压成 0 会让任何带字节的文件都被拒")
	}

	SetBatchImportMaxSizeMBProvider(func() int { return 64 })
	SetBatchImportMaxSizeMBProvider(nil)
	_, maxMB = batchImportMaxUploadSize()
	require.Equal(t, 64, maxMB, "传 nil 应是空操作，不该把注入值顶掉")
}

// importCSV posts a multipart file to ImportFile and returns status + envelope message.
// svc 允许为 nil：两条用例都在体积判定处收尾或走零行 CSV，不碰仓储。
func importCSV(t *testing.T, svc *service.BatchOperationService, filename string, content []byte) (int, string) {
	t.Helper()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	require.NoError(t, w.WriteField("type", "clue"))
	fw, err := w.CreateFormFile("file", filename)
	require.NoError(t, err)
	_, err = io.Copy(fw, bytes.NewReader(content))
	require.NoError(t, err)
	require.NoError(t, w.Close())

	gin.SetMode(gin.TestMode)
	router := gin.New()
	c := &BatchImportController{svc: svc}
	router.POST("/api/batch/import", c.ImportFile)

	req := httptest.NewRequest(http.MethodPost, "/api/batch/import", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	var body struct {
		Code    any    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), "响应体不是信封 JSON：%q", rec.Body.String())
	return rec.Code, body.Message
}

// TestImportFileLimitComesFromParameter 上限来自参数中心而不是那句写死的 50MB：
// 注入 1MB 后传 1.5MB，必须 400，且文案里的数字跟着配置走。
func TestImportFileLimitComesFromParameter(t *testing.T) {
	t.Cleanup(resetBatchImportMaxSizeMBProvider)
	SetBatchImportMaxSizeMBProvider(func() int { return 1 })

	status, message := importCSV(t, nil, "clues.csv", make([]byte, 1536*1024))
	require.Equal(t, http.StatusBadRequest, status, "超上限应 400，实际 %d：%s", status, message)
	require.Contains(t, message, "1MB", "文案没跟着参数走：%q", message)
	require.NotContains(t, message, "50MB", "文案里再出现 50MB 说明还是写死的那句")
}

// TestImportFileUnderLimitReachesParser 上一格的正控制：同一档位下未越界必须走进 CSV 解析。
// 只有一枚表头的合法线索文件零行数据，不碰仓储就能走完，因此 svc 传零值实例。
func TestImportFileUnderLimitReachesParser(t *testing.T) {
	t.Cleanup(resetBatchImportMaxSizeMBProvider)
	SetBatchImportMaxSizeMBProvider(func() int { return 1 })

	status, message := importCSV(t, &service.BatchOperationService{}, "clues.csv",
		[]byte("姓名,账户,类型,城市,地址,描述,来源 ID\n"))
	require.NotEqual(t, http.StatusBadRequest, status, "未越界却在体积处被拦：%s", message)
	require.Equal(t, http.StatusOK, status, "零行导入应正常收尾：%s", message)
}

// TestImportFileFallsBackTo50MBWithoutParameter 未接参数中心时逐字复现接线前的行为。
func TestImportFileFallsBackTo50MBWithoutParameter(t *testing.T) {
	t.Cleanup(resetBatchImportMaxSizeMBProvider)
	resetBatchImportMaxSizeMBProvider()

	status, message := importCSV(t, nil, "clues.csv", make([]byte, 51*1024*1024))
	require.Equal(t, http.StatusBadRequest, status)
	require.Contains(t, message, "50MB", "兜底行为变了，读到的文案是 %q", message)
}
