package controller

import (
	"net/http"

	"hivemtk-user/internal/geo/service"
	"hivemtk-user/internal/pkg/utils/response"

	"github.com/gin-gonic/gin"
)

// SeedTermController 关键词种子词条（数据库驱动，独立部署可维护）
type SeedTermController struct {
	svc *service.SeedTermService
}

func NewSeedTermController(svc *service.SeedTermService) *SeedTermController {
	return &SeedTermController{svc: svc}
}

// [AUTO] List
// @Tags      seed-term
// @Router    /api/geo/seed-terms [get]
// List GET /geo/seed-terms
func (c *SeedTermController) List(ctx *gin.Context) {
	terms, err := c.svc.ListTerms(ctx.Request.Context())
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, gin.H{"total": len(terms), "terms": terms}, "ok")
}

// [AUTO] Reseed
// @Tags      seed-term
// @Router    /api/geo/seed-terms/reseed [post]
// Reseed POST /geo/seed-terms/reseed
// 幂等重播内置缺省包（新增缺省词上架，已有词条保留）
func (c *SeedTermController) Reseed(ctx *gin.Context) {
	n, err := c.svc.ReseedDefaults(ctx.Request.Context())
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, gin.H{"upserted": n}, "ok")
}
