package controller

import (
	"strconv"

	"hivemtk-user/internal/geo/service"
	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/reporting"

	"github.com/gin-gonic/gin"
)

// CitationController 引用智能控制器
//
// A10（2026-10-24）：driftSvc 注入引用漂移读端（/geo/citations/drift）。
type CitationController struct {
	citeSvc   *service.CitationService
	driftSvc  *service.CitationDriftService
}

func NewCitationController(citeSvc *service.CitationService) *CitationController {
	return &CitationController{citeSvc: citeSvc}
}

func NewCitationControllerWithDrift(citeSvc *service.CitationService, driftSvc *service.CitationDriftService) *CitationController {
	return &CitationController{citeSvc: citeSvc, driftSvc: driftSvc}
}

// Drift 引用漂移 diff（A10）
// GET /geo/citations/drift?from=2026-10-23&to=2026-10-24
// from 缺省取 to 之前最近一次快照；任一侧快照缺失返回明确错误（不冒充"没有漂移"）。
func (c *CitationController) Drift(ctx *gin.Context) {
	if c.driftSvc == nil {
		response.Error(ctx, 503, "drift service 未初始化")
		return
	}
	res, err := c.driftSvc.GetDriftBetween(ctx.Request.Context(), ctx.Query("from"), ctx.Query("to"))
	if err != nil {
		response.Error(ctx, 404, err.Error())
		return
	}
	response.Success(ctx, res, "ok")
}

// Domains 引用域名聚合（自有/竞品/earned 分类）
// GET /geo/citations/domains?days=30&format=json|csv
func (c *CitationController) Domains(ctx *gin.Context) {
	days, _ := strconv.Atoi(ctx.DefaultQuery("days", strconv.Itoa(reporting.DefaultWindowDays(ctx.Request.Context()))))
	res, err := c.citeSvc.DomainStats(ctx.Request.Context(), days)
	if err != nil {
		response.ErrorFromDB(ctx, err, "引用域名聚合失败")
		return
	}
	if ctx.DefaultQuery("format", "json") == "csv" {
		writeCSV(ctx, "citation-domains.csv", service.BuildDomainsCSV(res))
		return
	}
	response.Success(ctx, res, "ok")
}

// Matrix keyword×domain 矩阵
// GET /geo/citations/matrix?days=30&top_queries=20&top_domains=10&format=json|csv
func (c *CitationController) Matrix(ctx *gin.Context) {
	days, _ := strconv.Atoi(ctx.DefaultQuery("days", strconv.Itoa(reporting.DefaultWindowDays(ctx.Request.Context()))))
	topQ, _ := strconv.Atoi(ctx.DefaultQuery("top_queries", "20"))
	topD, _ := strconv.Atoi(ctx.DefaultQuery("top_domains", "10"))
	res, err := c.citeSvc.KeywordDomainMatrix(ctx.Request.Context(), days, topQ, topD)
	if err != nil {
		response.ErrorFromDB(ctx, err, "引用矩阵聚合失败")
		return
	}
	if ctx.DefaultQuery("format", "json") == "csv" {
		writeCSV(ctx, "citation-matrix.csv", service.BuildMatrixCSV(res))
		return
	}
	response.Success(ctx, res, "ok")
}

// writeCSV 写 CSV 下载响应（BOM 供 Excel 中文不乱码）
func writeCSV(ctx *gin.Context, filename, body string) {
	ctx.Header("Content-Type", "text/csv; charset=utf-8")
	ctx.Header("Content-Disposition", "attachment; filename="+filename)
	_, _ = ctx.Writer.WriteString("\xef\xbb\xbf" + body)
}
