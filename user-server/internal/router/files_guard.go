package router

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/storage"

	"github.com/gin-gonic/gin"
)

// RegisterFilesRoute 把本地上传目录挂到 /files/*。
//
// 抽成独立函数是为了让它可测："托管哪一棵树"这件事单独就能错，而 Setup 要拉起整个应用
// （见 TestRegisterFilesRouteServesTheUploadWritersRoot；装配腿由 TestSetupRegistersFilesRoute 守着）。
//
// 托管根取自 storage.LocalSource() ——与上传写入方（controller/upload.go）、邮件外发侧三方
// 同一份解析，兜底链 STORAGE_LOCAL_BASE_DIR → ./uploads。此前这里自己读一遍 STORAGE_LOCAL_BASE_DIR，
// 而上传侧的链中间还夹着一档 UPLOAD_DIR ⇒ "只配 UPLOAD_DIR"的部署把文件写进 $UPLOAD_DIR、
// 却从 ./uploads 对外公开，公开链接逐个 404（第三十八轮把那一档连同这里的手抄一起退役）。
// 取第一个返回值：第三个（附件子目录）与第二个（公网前缀）都不是本路由的坐标系，
// 路由路径按 /files/* 硬编码，改 STORAGE_LOCAL_PUBLIC_URL 只会改外链，不会改这里挂的路径。
//
// /files 走守卫版（同源可执行扩展名 403 + nosniff/sandbox）而非裸 r.Static，
// 堵素材库/渠道媒体任意扩展名落盘后的同源直出。
func RegisterFilesRoute(r *gin.Engine) {
	uploadDir, _, _ := storage.LocalSource()
	_ = os.MkdirAll(uploadDir, 0o750)
	r.GET("/files/*filepath", serveUploadsGuarded(uploadDir))
	logger.Infof("[Router] static file server registered (guarded): /files -> %s", uploadDir)
}

// serveUploadsGuarded 以 r.Static 的替代实现发布本地上传目录（/files/*），
// 叠加三层防护：
//
//  1. 命中"同源可执行/可渲染"扩展名清单（storage.IsDangerousWebExt，与落盘
//     中和层同一清单）→ 403。落盘层（LocalDriver.generatePath）已把新文件
//     中和为 .bin，此处兜底**历史已落盘**的 .html/.svg 与绕过驱动直接放入
//     目录的文件。
//  2. X-Content-Type-Options: nosniff + Content-Security-Policy: sandbox，
//     封掉旧浏览器内容嗅探与万一命中文档类型的脚本执行/同源能力。
//  3. 租户归属校验（T011 的静态服务半边，原先只有落盘半边有实现）：
//     on 模式下 storage.LocalDriver 把新上传落到 `tenant_{id}/`，而本路由此前
//     **完全不看路径里的租户段**——拿着 URL 的任何人都能读别的租户的文件。
//     现行判据见 tenantPathAllowed。
//
// 路径处理：c.Param 取到的相对段先 filepath.Clean("/"+rel) 归一（消化 ..
// 与多重斜杠）再 Join 到 baseDir；http.ServeFile 对含 ".." 的请求段自带二次
// 拒绝——双保险防穿越，与第七轮 platform 存储驱动的修法同构。
func serveUploadsGuarded(baseDir string) gin.HandlerFunc {
	base := filepath.Clean(baseDir)
	return func(c *gin.Context) {
		rel := c.Param("filepath")
		if storage.IsDangerousWebExt(filepath.Ext(rel)) {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "file type not allowed to serve",
				"path":  rel,
			})
			return
		}
		if !tenantPathAllowed(c.Request.Context(), rel) {
			// 404 而不是 403：403 等于"这个文件存在但不给你看"，是个存在性预言机。
			c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
			return
		}
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Content-Security-Policy", "sandbox")
		c.File(filepath.Join(base, filepath.Clean("/"+rel)))
	}
}

// tenantFileSegmentRe 匹配 LocalDriver.tenantPathPrefix 产出的租户目录段。
var tenantFileSegmentRe = regexp.MustCompile(`^tenant_([0-9]+)(?:/|$)`)

// tenantPathAllowed 判定本次请求能否读取该相对路径（T011 静态服务半边）。
//
// 判据（三条，都是"不改变合法用法"的方向）：
//  1. off 模式 ⇒ 恒放行（离线单机：路径里本来就不会有租户段，行为逐字节不变）
//  2. 路径首段不是 `tenant_{id}` ⇒ 放行（存量文件仍在根域、系统级上传、附件子目录）
//  3. 路径首段是 `tenant_{id}`：
//     - 请求 ctx 已绑定租户 ⇒ 必须同一个租户，否则 404
//     - 请求 ctx 无租户 ⇒ **放行**（访客 Widget 的 <img>、分享页等无凭证的公开外链；
//     on 模式下渲染侧链路本来就靠"UUID 不可枚举"这一层，本轮不改变该前提）
//
// 第 3 条的第二种情形是**已知残留风险**，不假装解决：无凭证的公开外链拿不到租户
// 归属，要收口只能走签名 URL（预签名/短时 token），那是独立一项，见
// docs/architecture/SAAS_FULLCHAIN_AUDIT_2026-10-23.md 的 P1-2。
func tenantPathAllowed(ctx context.Context, rel string) bool {
	if !tenant.IsMultiTenant() {
		return true
	}
	m := tenantFileSegmentRe.FindStringSubmatch(strings.TrimPrefix(rel, "/"))
	if m == nil {
		return true
	}
	pathTenant, err := strconv.ParseUint(m[1], 10, 64)
	if err != nil {
		return false
	}
	ctxTenant, ok := tenant.FromContext(ctx)
	if !ok {
		return true // 无凭证公开访问：见函数注释第 3 条第二种情形
	}
	return uint64(ctxTenant) == pathTenant
}
