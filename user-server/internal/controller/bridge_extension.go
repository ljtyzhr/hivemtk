package controller

import (
	"errors"
	"net/http"

	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// BridgeExtensionController 桥接扩展安装包下载入口
//
// 浏览器扩展不上架 Chrome Web Store：平台运营在平台端上传安装包，
// 租户在本端下载后自行「加载已解压的扩展程序」。
// 本 controller 只回答一个问题："当前部署能拿到哪一版扩展包、下载地址是什么"。
type BridgeExtensionController struct {
	svc *service.BridgeExtensionService
}

// NewBridgeExtensionController 构造
func NewBridgeExtensionController(svc *service.BridgeExtensionService) *BridgeExtensionController {
	return &BridgeExtensionController{svc: svc}
}

// RegisterRoutes 挂载路由（挂在已鉴权组下）
func (c *BridgeExtensionController) RegisterRoutes(auth *gin.RouterGroup) {
	auth.GET("/bridge/extension/package", c.Package)
}

// @Summary      当前部署未接入平台，或平台尚未上架桥接扩展包
// @Tags      bridge-extension
// @Router    /api/bridge/extension/package [get]
// Package GET /api/bridge/extension/package
//
// 返回平台上最新一版桥接扩展安装包信息（含绝对下载地址与 sha256）。
//
// 状态码口径：
//
//	200 —— 有可下载的包
//	404 —— 当前部署未接入平台，或平台上还没上架扩展包（两种都是"现在拿不到"，
//	       但文案不同：前者提示部署形态，后者提示平台侧待上传）
//	503 —— 平台侧调用失败（网络/签名），不是"没有这个东西"
func (c *BridgeExtensionController) Package(ctx *gin.Context) {
	pkg, err := c.svc.Latest(ctx.Request.Context())
	if err != nil {
		if errors.Is(err, service.ErrBridgeExtensionOffline) {
			response.Error(ctx, http.StatusNotFound, "当前部署未接入平台，或平台尚未上架桥接扩展包")
			return
		}
		response.Error(ctx, http.StatusServiceUnavailable, "获取扩展包信息失败："+err.Error())
		return
	}
	response.Success(ctx, pkg, "ok")
}
