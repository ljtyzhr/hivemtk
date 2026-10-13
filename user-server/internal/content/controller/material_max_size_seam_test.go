package controller

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hivemtk-user/internal/content/dto"
	"hivemtk-user/internal/content/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// resetMaterialMaxSizeMBProvider 把读取口复位到「未注入」。
//
// ⚠️ 不能写成 SetMaterialMaxSizeMBProvider(nil)：setter 的契约是 nil 表示「不注入」，
// 而不是「复位」，传 nil 是空操作（与全仓 provider setter 同一语义）。
// 复位只能重新注入兜底常量本身。
func resetMaterialMaxSizeMBProvider() {
	materialMaxSizeMBProvider = func() int { return DefaultMaterialMaxSizeMB }
}

// TestMaterialMaxSizeMBProviderSeam 逐格钉死 frontend_ui.material_max_size_mb 的读取口。
func TestMaterialMaxSizeMBProviderSeam(t *testing.T) {
	t.Cleanup(resetMaterialMaxSizeMBProvider)
	resetMaterialMaxSizeMBProvider()

	// 兜底 = 接线前写死的那个数，改它等于改「读不到库时线上上传口放多大」，是行为变更。
	maxBytes, maxMB := materialMaxUploadSize()
	require.Equal(t, 10, maxMB)
	require.Equal(t, int64(10*1024*1024), maxBytes)

	SetMaterialMaxSizeMBProvider(func() int { return 20 })
	maxBytes, maxMB = materialMaxUploadSize()
	require.Equal(t, 20, maxMB, "注入值没进读取口")
	require.Equal(t, int64(20*1024*1024), maxBytes, "MB→字节的换算不是 1024×1024")

	// <=0 是坏数据而不是「不限」：这条的 Min 是 1，界面配不出 0，
	// 真读到 0 时把上限换成 0 会让所有素材上传全 400，比按 10MB 放行更难排查。
	for _, bad := range []int{0, -5} {
		SetMaterialMaxSizeMBProvider(func() int { return bad })
		maxBytes, maxMB = materialMaxUploadSize()
		require.Equal(t, DefaultMaterialMaxSizeMB, maxMB, "注入 %d 时应回落兜底", bad)
		require.Equal(t, int64(DefaultMaterialMaxSizeMB*1024*1024), maxBytes)
	}

	// setter 的 nil 语义＝「不注入」，所以传 nil 后仍是被注入的那个值。
	// 这条不是期望行为，是把现状钉住：别处的 t.Cleanup(SetXxx(nil)）都是空转。
	SetMaterialMaxSizeMBProvider(func() int { return 32 })
	SetMaterialMaxSizeMBProvider(nil)
	_, maxMB = materialMaxUploadSize()
	require.Equal(t, 32, maxMB, "传 nil 应是空操作，不该把注入值顶掉")
}

// stubMaterialService 只为了把 HTTP binding 这条路跑到 size 判断之后的那一行。
// 除 UploadMaterial 外全部返回错误：本文件不断这些方法的行为。
type stubMaterialService struct {
	uploadCalled bool
	uploadedName string
}

var errStubNotUsed = errors.New("stub: 本用例不该走到这里")

func (*stubMaterialService) GetCategoryList(string, string, string, int, int) (*dto.GetMaterialCategoryListResponse, error) {
	return nil, errStubNotUsed
}
func (*stubMaterialService) GetCategoryTree(string, string) ([]*dto.MaterialCategoryResponse, error) {
	return nil, errStubNotUsed
}
func (*stubMaterialService) GetCategory(string) (*dto.MaterialCategoryResponse, error) {
	return nil, errStubNotUsed
}
func (*stubMaterialService) CreateCategory(*dto.CreateMaterialCategoryRequest) (*dto.MaterialCategoryResponse, error) {
	return nil, errStubNotUsed
}
func (*stubMaterialService) UpdateCategory(string, *dto.UpdateMaterialCategoryRequest) (*dto.MaterialCategoryResponse, error) {
	return nil, errStubNotUsed
}
func (*stubMaterialService) DeleteCategory(string) error { return errStubNotUsed }
func (*stubMaterialService) GetMaterialList(string, string, string, string, int, int) (*dto.GetMaterialListResponse, error) {
	return nil, errStubNotUsed
}
func (*stubMaterialService) GetMaterial(string) (*dto.MaterialResponse, error) {
	return nil, errStubNotUsed
}
func (*stubMaterialService) CreateMaterial(*dto.CreateMaterialRequest) (*dto.MaterialResponse, error) {
	return nil, errStubNotUsed
}
func (*stubMaterialService) UpdateMaterial(string, *dto.UpdateMaterialRequest) (*dto.MaterialResponse, error) {
	return nil, errStubNotUsed
}
func (*stubMaterialService) DeleteMaterial(string) error { return errStubNotUsed }
func (s *stubMaterialService) UploadMaterial(_ multipart.File, header *multipart.FileHeader, req *dto.UploadMaterialRequest) (*dto.MaterialResponse, error) {
	s.uploadCalled = true
	s.uploadedName = req.Name
	return &dto.MaterialResponse{Name: header.Filename}, nil
}
func (*stubMaterialService) GetMaterialSelector(string, string) (*dto.MaterialSelectorResponse, error) {
	return nil, errStubNotUsed
}
func (*stubMaterialService) UpdateMaterialUsage(string) error { return errStubNotUsed }
func (*stubMaterialService) GetMaterialStats(string) (*dto.MaterialStatsResponse, error) {
	return nil, errStubNotUsed
}

var _ service.MaterialService = (*stubMaterialService)(nil)

// uploadToMaterial 造一条真实 multipart 请求打到 UploadMaterial。
// bodySize 是文件字节数；返回状态码、信封里的 message，以及未解析的原始响应体。
func uploadToMaterial(t *testing.T, svc *stubMaterialService, bodySize int) (int, string, string) {
	return uploadToMaterialNamed(t, svc, "payload.bin", bodySize)
}

func uploadToMaterialNamed(t *testing.T, svc *stubMaterialService, filename string, bodySize int) (int, string, string) {
	t.Helper()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	require.NoError(t, w.WriteField("category_id", "cat-1"))
	fw, err := w.CreateFormFile("file", filename)
	require.NoError(t, err)
	// 全零字节：这里只关心体积，不关心内容。
	_, err = io.Copy(fw, bytes.NewReader(make([]byte, bodySize)))
	require.NoError(t, err)
	require.NoError(t, w.Close())

	gin.SetMode(gin.TestMode)
	router := gin.New()
	c := &MaterialController{service: svc}
	router.POST("/api/materials/upload", func(ctx *gin.Context) {
		ctx.Set("license_id", "license-test")
		ctx.Set("user_id", uint(1))
		c.UploadMaterial(ctx)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/materials/upload", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	var body struct {
		Code    any    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), "响应体不是信封 JSON：%q", rec.Body.String())
	return rec.Code, body.Message, rec.Body.String()
}

// TestUploadMaterialLimitComesFromParameter 证明上限真的来自读取口，而不是那句写死的 10MB。
//
// 只在 seam 层断言不够：materialMaxUploadSize() 有值不代表 UploadMaterial 用了它。
// 这里注入 1MB 上限（seam 的 Min 是 1，压不到 0），传 1.5MB，
// 服务端必须拒、且不能把文件交给下游 service。
func TestUploadMaterialLimitComesFromParameter(t *testing.T) {
	t.Cleanup(resetMaterialMaxSizeMBProvider)
	SetMaterialMaxSizeMBProvider(func() int { return 1 })

	svc := &stubMaterialService{}
	status, message, _ := uploadToMaterial(t, svc, 1536*1024)
	require.Equal(t, http.StatusBadRequest, status, "超上限应 400，实际 %d：%s", status, message)
	require.Contains(t, message, "1MB", "报错文案要跟着参数走，读到的却是：%q", message)
	require.False(t, svc.uploadCalled, "越界文件不该交给素材服务")
	require.NotContains(t, message, "10MB", "文案里再出现 10MB 说明还是写死的那句")
}

// TestUploadMaterialUnderLimitReachesService 上一格的正控制：同一套装配下未越界必须放行。
// 少了这一格，「handler 永远 400」这种坏法也能骗过上一条。
func TestUploadMaterialUnderLimitReachesService(t *testing.T) {
	t.Cleanup(resetMaterialMaxSizeMBProvider)
	SetMaterialMaxSizeMBProvider(func() int { return 1 })

	svc := &stubMaterialService{}
	status, message, _ := uploadToMaterial(t, svc, 1024)
	require.Equal(t, http.StatusOK, status, "未越界应放行，实际 %d：%s", status, message)
	require.True(t, svc.uploadCalled, "放行却没调素材服务")
	require.Equal(t, "payload.bin", svc.uploadedName)
}

// TestUploadMaterialFallsBackToTenMBWithoutParameter 未接参数中心（装配缺失/读库失败）时
// 的行为必须与接线前逐字一致：按 10MB 拦，文案也是 10MB。
func TestUploadMaterialFallsBackToTenMBWithoutParameter(t *testing.T) {
	t.Cleanup(resetMaterialMaxSizeMBProvider)
	resetMaterialMaxSizeMBProvider()

	svc := &stubMaterialService{}
	status, message, _ := uploadToMaterial(t, svc, 11*1024*1024)
	require.Equal(t, http.StatusBadRequest, status)
	require.Contains(t, message, "10MB", "兜底行为变了：读到的文案是 %q", message)
	require.False(t, svc.uploadCalled)

	// 兜底下 1MB 的素材照常放行，证明上一条不是「一律拒」。
	svc2 := &stubMaterialService{}
	status2, message2, _ := uploadToMaterial(t, svc2, 1024*1024)
	require.Equal(t, http.StatusOK, status2, "兜底放行失败：%d %s", status2, message2)
	require.True(t, svc2.uploadCalled)
}

// TestUploadMaterialRejectionDoesNotEchoRequest 体积拒绝的文案里只能出现配置数字：
// 这条链路收的是任意外部文件，文件名与内容都不该被回显到响应里。
//
// 注：只覆盖「超上限被拒」这一条出口。放行后下游 UploadMaterial 失败时，handler 会把
// err.Error() 拼进 500 文案（"上传失败: "+err.Error()），那条路的内容回显不归本参数管，
// 属素材服务与下游实现的独立问题。
func TestUploadMaterialRejectionDoesNotEchoRequest(t *testing.T) {
	t.Cleanup(resetMaterialMaxSizeMBProvider)
	SetMaterialMaxSizeMBProvider(func() int { return 1 })

	svc := &stubMaterialService{}
	marker := strings.Repeat("Z", 64)
	status, _, raw := uploadToMaterialNamed(t, svc, marker+".bin", 1536*1024)
	require.Equal(t, http.StatusBadRequest, status, "越界必须拒，实际 %d", status)
	require.False(t, svc.uploadCalled, "越界文件不该交给素材服务")
	require.NotContains(t, raw, marker, "响应体里出现了上传文件名")
	require.Contains(t, raw, "1MB", "拒绝文案没带上配置的上限：%s", raw)
}
