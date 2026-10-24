// dialogue_eval.go 对话级 gold 评测 HTTP 面（P0-1）。形状对齐 controller/rag_eval.go：
// run/list/get 三端点 + manage 面。
package controller

import (
	"net/http"
	"strconv"

	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// DialogueEvalController 对话级评测控制器
type DialogueEvalController struct {
	svc *service.DialogueEvalService
}

// NewDialogueEvalController 构造
func NewDialogueEvalController() *DialogueEvalController {
	return &DialogueEvalController{svc: service.NewDialogueEvalService(service.GlobalBadCaseService())}
}

// RunRequest 运行参数
type DialogueEvalRunRequest struct {
	Name          string `json:"name"`
	BadCaseLimit  int    `json:"bad_case_limit"`
}

// @Summary     运行对话级 gold 评测（基线+退化双相）
// @Tags      dialogue-eval
// @Router    /api/dialogue-eval/run [post]
// Run POST /api/dialogue-eval/run
// 请求体: {"name": "my_eval", "bad_case_limit": 20}
func (c *DialogueEvalController) Run(ctx *gin.Context) {
	var req DialogueEvalRunRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		req = DialogueEvalRunRequest{}
	}
	runs, err := c.svc.RunGoldEvaluation(ctx.Request.Context(), req.Name, req.BadCaseLimit)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, runs, "评测完成")
}

// @Summary      查询对话评测运行列表
// @Tags      dialogue-eval
// @Router    /api/dialogue-eval/runs [get]
// List GET /api/dialogue-eval/runs?limit=20
func (c *DialogueEvalController) List(ctx *gin.Context) {
	limit := 20
	if v := ctx.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	runs, err := c.svc.ListDialogueEvalRuns(ctx.Request.Context(), limit)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, runs, "获取成功")
}

// @Summary      按 ID 查询对话评测运行详情
// @Tags      dialogue-eval
// @Router    /api/dialogue-eval/runs/{id} [get]
// Get GET /api/dialogue-eval/runs/:id
func (c *DialogueEvalController) Get(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的 id")
		return
	}
	run, err := c.svc.GetDialogueEvalRun(ctx.Request.Context(), uint(id))
	if err != nil {
		response.Error(ctx, http.StatusNotFound, "运行不存在")
		return
	}
	response.Success(ctx, run, "获取成功")
}
