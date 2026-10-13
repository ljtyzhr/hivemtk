package controller

import (
	"fmt"
	"hivemtk-user/internal/pkg/errhttp"
	"net/http"
	"strconv"

	"hivemtk-user/internal/content/dto"
	"hivemtk-user/internal/content/service"
	"hivemtk-user/internal/pkg/utils/response"

	"github.com/gin-gonic/gin"
)

// DefaultMaterialMaxSizeMB 是参数中心 frontend_ui.material_max_size_mb 的兜底值。
//
// 取 10 = 接线前这里写死的那个数：兜底必须与接线前的实际行为逐字一致，
// 否则「读不到库」会变成一次静默的行为变更。
const DefaultMaterialMaxSizeMB = 10

// materialMaxSizeMBProvider 由 internal/app 的参数装配层注入。
// 形状与 internal/content/service 的 maxFlowDelaySecondsProvider 一致：
// 本子模块一贯走「装配层注入读取口」，不在 controller 里直接摸全局参数服务。
var materialMaxSizeMBProvider = func() int { return DefaultMaterialMaxSizeMB }

// SetMaterialMaxSizeMBProvider 注入素材库单文件大小上限（MB）的读取口，仅装配层调用。
// 传 nil 是「不注入」而不是「复位」（与全仓 setter 同一语义）。
func SetMaterialMaxSizeMBProvider(fn func() int) {
	if fn != nil {
		materialMaxSizeMBProvider = fn
	}
}

// materialMaxUploadSize 返回生效中的素材库上传上限（字节与 MB，报错文案要用后者）。
//
// 接线前这里写死 10MB，而前端弹窗的提示与本地拦截早就改成读参数中心的
// frontend_ui.material_max_size_mb 了（MaterialLibrary.vue 的 rtNum）：
// 运维把上限调到 20MB 后，前端放行 20MB、这里仍按 10MB 拦，用户看到的是一条
// 没有任何解释的 400——「前端放过、后端拦死」正是那条参数的 Description 里
// 自己承认的错位。
//
// 没有接进 misc.upload_max_size_mb 那条链：后者的兼容列
// （system_config.max_upload_size_mb，gorm default 50）在存量站点上基本都是 50，
// 接过去等于把这一个端点的上限从 10MB 静默放宽 5 倍。
//
// 读到 <=0 视为「未配置」回落到兜底：这条的 Min 是 1，正常界面配不出 0；
// 库里真出现 0 只可能是坏数据，此时把上传全打死比按 10MB 放行更难排查。
// 与 internal/controller/upload.go 的 resolveUploadParamMaxBytes 把 0 当「未读到」同源。
func materialMaxUploadSize() (int64, int) {
	mb := materialMaxSizeMBProvider()
	if mb <= 0 {
		mb = DefaultMaterialMaxSizeMB
	}
	return int64(mb) * 1024 * 1024, mb
}

// ProbeMaterialMaxUploadSize 暴露读取口当前值，供装配层测试断言接线确实生效。
func ProbeMaterialMaxUploadSize() (int64, int) { return materialMaxUploadSize() }

type MaterialController struct {
	service service.MaterialService
}

func NewMaterialController(service service.MaterialService) *MaterialController {
	return &MaterialController{service: service}
}

// GetMaterialList 获取素材列表
func (c *MaterialController) GetMaterialList(ctx *gin.Context) {
	categoryID := ctx.Query("category_id")
	if categoryID == "" {
		categoryID = ctx.Query("categoryId")
	}
	materialType := ctx.Query("type")
	search := ctx.Query("search")
	if search == "" {
		search = ctx.Query("keyword")
	}
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(ctx.DefaultQuery("limit", "20"))

	licenseID := ctx.GetString("license_id")
	if licenseID == "" {
		response.Error(ctx, http.StatusUnauthorized, "未授权访问")
		return
	}

	resp, err := c.service.GetMaterialList(licenseID, categoryID, materialType, search, page, limit)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "获取素材列表失败")
		return
	}
	response.Success(ctx, resp, "success")
}

// GetMaterialByID 获取素材详情
func (c *MaterialController) GetMaterialByID(ctx *gin.Context) {
	materialID := ctx.Param("id")
	if materialID == "" {
		response.Error(ctx, http.StatusBadRequest, "素材ID不能为空")
		return
	}

	material, err := c.service.GetMaterial(materialID)
	if errhttp.HandleDBError(ctx, err, "获取素材详情") {
		return
	}
	response.Success(ctx, material, "success")
}

// GetMaterialCategoryByID 获取素材分类详情
func (c *MaterialController) GetMaterialCategoryByID(ctx *gin.Context) {
	categoryID := ctx.Param("id")
	if categoryID == "" {
		response.Error(ctx, http.StatusBadRequest, "分类ID不能为空")
		return
	}

	category, err := c.service.GetCategory(categoryID)
	if errhttp.HandleDBError(ctx, err, "获取素材分类详情") {
		return
	}
	response.Success(ctx, category, "success")
}

// UpdateMaterialUsage 更新素材使用次数
func (c *MaterialController) UpdateMaterialUsage(ctx *gin.Context) {
	materialID := ctx.Param("id")
	if materialID == "" {
		response.Error(ctx, http.StatusBadRequest, "素材ID不能为空")
		return
	}

	licenseID := ctx.GetString("license_id")
	if licenseID == "" {
		response.Error(ctx, http.StatusUnauthorized, "未授权访问")
		return
	}

	err := c.service.UpdateMaterialUsage(materialID)
	if errhttp.HandleDBError(ctx, err, "更新素材使用次数") {
		return
	}
	response.Success(ctx, nil, "更新成功")
}

// GetMaterialStats 获取素材统计信息
func (c *MaterialController) GetMaterialStats(ctx *gin.Context) {
	licenseID := ctx.GetString("license_id")
	if licenseID == "" {
		response.Error(ctx, http.StatusUnauthorized, "未授权访问")
		return
	}

	stats, err := c.service.GetMaterialStats(licenseID)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "获取素材统计失败")
		return
	}
	response.Success(ctx, stats, "success")
}

// UploadMaterial 上传素材
func (c *MaterialController) UploadMaterial(ctx *gin.Context) {
	licenseID := ctx.GetString("license_id")
	if licenseID == "" {
		response.Error(ctx, http.StatusUnauthorized, "未授权访问")
		return
	}

	categoryID := ctx.PostForm("category_id")
	if categoryID == "" {
		response.Error(ctx, http.StatusBadRequest, "请选择素材分类")
		return
	}

	file, header, err := ctx.Request.FormFile("file")
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "请上传文件")
		return
	}
	defer func() { _ = file.Close() }()

	maxBytes, maxMB := materialMaxUploadSize()
	if header.Size > maxBytes {
		response.Error(ctx, http.StatusBadRequest, fmt.Sprintf("文件大小不能超过%dMB", maxMB))
		return
	}

	userID := ""
	if uid, exists := ctx.Get("user_id"); exists {
		if v, ok := uid.(uint); ok {
			userID = strconv.FormatUint(uint64(v), 10)
		} else if v, ok := uid.(float64); ok {
			userID = strconv.FormatUint(uint64(v), 10)
		}
	}
	uploadReq := &dto.UploadMaterialRequest{
		CategoryID: categoryID,
		Name:       header.Filename,
		LicenseID:  licenseID,
		UserID:     userID,
	}

	material, err := c.service.UploadMaterial(file, header, uploadReq)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "上传失败: "+err.Error())
		return
	}
	response.Success(ctx, material, "上传成功")
}

// DeleteMaterial 删除素材
func (c *MaterialController) DeleteMaterial(ctx *gin.Context) {
	materialID := ctx.Param("id")
	if materialID == "" {
		response.Error(ctx, http.StatusBadRequest, "素材ID不能为空")
		return
	}

	err := c.service.DeleteMaterial(materialID)
	if errhttp.HandleDBError(ctx, err, "删除素材") {
		return
	}
	response.Success(ctx, gin.H{"deleted": true}, "删除成功")
}

// GetMaterialCategories 获取素材分类列表
func (c *MaterialController) GetMaterialCategories(ctx *gin.Context) {
	licenseID := ctx.GetString("license_id")
	if licenseID == "" {
		response.Success(ctx, []any{}, "success")
		return
	}

	parentID := ctx.Query("parent_id")
	materialType := ctx.Query("type")
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(ctx.DefaultQuery("limit", "50"))

	categories, err := c.service.GetCategoryList(licenseID, parentID, materialType, page, limit)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "获取分类失败")
		return
	}
	response.Success(ctx, categories, "success")
}

// CreateMaterialCategory 创建素材分类
func (c *MaterialController) CreateMaterialCategory(ctx *gin.Context) {
	var req dto.CreateMaterialCategoryRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "参数错误")
		return
	}

	licenseID := ctx.GetString("license_id")
	if licenseID == "" {
		response.Error(ctx, http.StatusUnauthorized, "未授权访问")
		return
	}

	userID := ""
	if uid, exists := ctx.Get("user_id"); exists {
		if v, ok := uid.(uint); ok {
			userID = strconv.FormatUint(uint64(v), 10)
		} else if v, ok := uid.(float64); ok {
			userID = strconv.FormatUint(uint64(v), 10)
		}
	}
	req.LicenseID = licenseID
	req.UserID = userID

	category, err := c.service.CreateCategory(&req)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "创建失败")
		return
	}
	response.Success(ctx, category, "创建成功")
}

// UpdateMaterialCategory 更新素材分类
func (c *MaterialController) UpdateMaterialCategory(ctx *gin.Context) {
	categoryID := ctx.Param("id")
	if categoryID == "" {
		response.Error(ctx, http.StatusBadRequest, "分类ID不能为空")
		return
	}

	var req dto.UpdateMaterialCategoryRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "参数错误")
		return
	}

	category, err := c.service.UpdateCategory(categoryID, &req)
	if errhttp.HandleDBError(ctx, err, "更新素材分类") {
		return
	}
	response.Success(ctx, category, "更新成功")
}

// DeleteMaterialCategory 删除素材分类
func (c *MaterialController) DeleteMaterialCategory(ctx *gin.Context) {
	categoryID := ctx.Param("id")
	if categoryID == "" {
		response.Error(ctx, http.StatusBadRequest, "分类ID不能为空")
		return
	}

	err := c.service.DeleteCategory(categoryID)
	if errhttp.HandleDBError(ctx, err, "删除素材分类") {
		return
	}
	response.Success(ctx, gin.H{"deleted": true}, "删除成功")
}

// GetMaterialSelector 获取素材选择器数据（用于前端选择素材）
func (c *MaterialController) GetMaterialSelector(ctx *gin.Context) {
	licenseID := ctx.GetString("license_id")
	if licenseID == "" {
		response.Error(ctx, http.StatusUnauthorized, "未授权访问")
		return
	}

	materialType := ctx.Query("type")
	resp, err := c.service.GetMaterialSelector(licenseID, materialType)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "获取素材选择器失败")
		return
	}
	response.Success(ctx, resp, "success")
}
