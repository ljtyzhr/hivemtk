package router

import (
	"net/http"
	"os"
	"path/filepath"
	"time"

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
// /files 走守卫版（同源可执行扩展名 403 + 签名凭证 + nosniff/sandbox）而非裸 r.Static，
// 堵素材库/渠道媒体任意扩展名落盘后的同源直出。
func RegisterFilesRoute(r *gin.Engine) {
	uploadDir, _, _ := storage.LocalSource()
	_ = os.MkdirAll(uploadDir, 0o750)
	r.GET("/files/*filepath", serveUploadsGuarded(uploadDir))
	logger.Infof("[Router] static file server registered (signed): /files -> %s", uploadDir)
}

// serveUploadsGuarded 以 r.Static 的替代实现发布本地上传目录（/files/*），叠加三层防护：
//
//  1. 命中"同源可执行/可渲染"扩展名清单（storage.IsDangerousWebExt，与落盘
//     中和层同一清单）→ 403。落盘层（LocalDriver.generatePath）已把新文件
//     中和为 .bin，此处兜底**历史已落盘**的 .html/.svg 与绕过驱动直接放入
//     目录的文件。
//  2. X-Content-Type-Options: nosniff + Content-Security-Policy: sandbox，
//     封掉旧浏览器内容嗅探与万一命中文档类型的脚本执行/同源能力。
//  3. 访问凭证（SaaS 审计 A1 / A12，2026-10-23）：
//     on 模式下，租户归属与过期时间只写在 `?s=` 的签名 token 里，**路径里没有租户段**。
//     验不过（缺失/篡改/过期）一律 404。判据见下面的 tenantPathAllowed 退役说明。
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

		// off 模式：逐字节等价历史行为（离线单机不签名，路径里本就没有租户段）
		if !tenant.IsMultiTenant() {
			serveFile(c, base, filepath.Clean("/"+rel))
			return
		}

		// on 模式：签名是**唯一**凭证。三种失败一律 404（不做"验不过就放行"的分支，
		// 那正是 A1 要清掉的形状；也不用 403 —— 避免变成存在性预言机）。
		tenantID, path, err := storage.VerifyFileURL(rel, c.Query("s"), time.Now())
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
			return
		}
		serveFile(c, base, filepath.Join(storage.TenantDir(tenantID), filepath.Clean("/"+path)))
	}
}

func serveFile(c *gin.Context, base, rel string) {
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "sandbox")
	c.File(filepath.Join(base, rel))
}

// 退役记录：tenantPathAllowed（2026-10-23 之前的那版 E0 修法）。
//
// 上一轮（P0-2）加了"路径首段 `tenant_{id}` 与请求 ctx 里的租户必须相同"的判据，
// 并配了四态用例。实测（`TestZZFilesChainProbe` 形态的探针，见下方注释里的复现）
// 证明**这条判据在生产上从未生效**：`/files` 是在 `Setup` 里**最先**注册的一条路由，
// 而 gin 的 `r.Use` 只对之后登记的路由生效 ⇒ 它的 handler 链上只有一个 handler，
// 没有 JWTAuth/TenantMiddleware ⇒ 请求 ctx 里永远没有租户绑定 ⇒ 那段 `if` 恒走
// "无绑定 ⇒ 放行"分支。
//
// 复现（本轮实测，改前）：带「租户 6」JWT 请求 `/files/tenant_5/x.pdf` ⇒ **200**；
// 不带 token 请求同一条 ⇒ **200**。也就是说上一轮的用例之所以绿，是因为它在进程里
// 手动 `req.WithContext(tenant.WithTenantID(...))` 注入了一个生产路径上不存在的状态。
//
// 这正是本仓最值得记的失效模式：**判定存在，但没有任何一条生产路径能走到它**。
// 所以本轮把判据整体换掉，而不是给它打补丁：凭证只剩签名一种（`storage.SignStoredPath`
// 在 URL 里签发，`storage.VerifyFileURL` 在这里验），跨租户访问在结构上不可表达 ——
// 签名自指它自己的租户目录，路径段改一点就验不过。
//
// 行为差异（有意为之，登记在案）：on 模式下**任何**未签名的 /files 请求（包括存量
// 明文 URL `/files/tenant_5/…`）现在都是 404。存量 URL 由
// `internal/migration/migrations/v3_80_0_file_url_sign_migration.go` 重签。
