package router

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

func setupEmbedStaticRoutes(r *gin.Engine) {
	userWebDist := os.Getenv("USER_WEB_DIST")
	if userWebDist == "" {
		candidates := []string{
			"../user-web/dist",
			"./user-web-dist",
			"./dist",
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				userWebDist, _ = filepath.Abs(c)
				break
			}
		}
	}

	embedDist := os.Getenv("EMBED_SDK_DIST")
	if embedDist == "" {
		candidates := []string{
			"../embed-sdk/dist",
			"./embed-sdk-dist",
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				embedDist, _ = filepath.Abs(c)
				break
			}
		}
	}

	if userWebDist != "" {
		if _, err := os.Stat(userWebDist); err == nil {
			r.Static("/assets", filepath.Join(userWebDist, "assets"))
			r.StaticFile("/favicon.ico", filepath.Join(userWebDist, "favicon.ico"))
			r.StaticFile("/logo.png", filepath.Join(userWebDist, "logo.png"))

			r.GET("/chat/embed/*path", func(c *gin.Context) {
				path := c.Param("path")
				if path != "" && !strings.HasPrefix(path, "#") {
					// embed=1 是**服务端能看见的**放行标记：302 之后浏览器加载的是
					// 路径 "/"（hash 只到客户端），而点击劫持护栏是挂在文档响应上的
					// ——不把这个标记带过去，浮标所在的第三方 iframe 会被 SAMEORIGIN 挡掉。
					c.Redirect(http.StatusFound, "/?embed=1#/chat/embed/"+strings.TrimPrefix(path, "/"))
					return
				}
				serveSPAWithFramePolicy(c, userWebDist, true)
			})
			r.GET("/chat/embed", func(c *gin.Context) {
				c.Redirect(http.StatusFound, "/?embed=1#/chat/embed/default")
			})

			r.GET("/", func(c *gin.Context) {
				serveSPAWithFramePolicy(c, userWebDist, c.Query("embed") == "1")
			})

			r.NoRoute(func(c *gin.Context) {
				path := c.Request.URL.Path
				if strings.HasPrefix(path, "/api/") {
					c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
					return
				}

				if strings.HasPrefix(path, "/files/") {
					c.JSON(http.StatusNotFound, gin.H{"error": "file not found", "path": path})
					return
				}
				if strings.HasPrefix(path, "/assets/") ||
					strings.HasPrefix(path, "/embed/") ||
					strings.HasPrefix(path, "/static/") {
					c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
					return
				}
				serveSPAWithFramePolicy(c, userWebDist, c.Query("embed") == "1")
			})

			r.GET("/index.html", func(c *gin.Context) {
				serveSPAWithFramePolicy(c, userWebDist, c.Query("embed") == "1")
			})

			_ = userWebDist
		} else {
			r.GET("/chat/embed/*path", func(c *gin.Context) {
				c.JSON(http.StatusNotFound, gin.H{
					"error":   "user-web dist not found",
					"message": "请先构建 user-web: cd ../user-web && npm run build，并把 dist 放到能被 user-server 找到的位置。可通过环境变量 USER_WEB_DIST 指定。",
					"path":    c.Param("path"),
				})
			})
		}
	} else {
		r.GET("/chat/embed/*path", func(c *gin.Context) {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "user-web dist not configured",
			})
		})
	}

	if embedDist != "" {
		if _, err := os.Stat(embedDist); err == nil {
			r.Static("/embed", embedDist)
		}
	}
}

func serveSPA(c *gin.Context, distDir string) {
	indexPath := filepath.Join(distDir, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "index.html not found", "path": indexPath})
		return
	}
	c.File(indexPath)
}

// serveSPAWithFramePolicy 交付 SPA 文档时下发点击劫持护栏（A13）。
//
// 为什么必须是**响应头**：`index.html` 的 meta CSP 里不能带 frame-ancestors
// （浏览器按规范忽略 meta 交付的这一条，见 user-web/index.html:20-26 的注释），
// 因此"meta 里写了"从不等于"有防护"。
//
// 两档策略 —— 只有客服浮标（/chat/embed 的落地页）需要对第三方放行：
//   - 普通页面：`X-Frame-Options: SAMEORIGIN` + `Content-Security-Policy: frame-ancestors 'self'`
//     （两条都发：XFO 给老浏览器，CSP 是现代实现；两者同时出现时浏览器取交集，
//     都要求同源，语义一致）；
//   - embeddable（`?embed=1`）：只发 `frame-ancestors *`，**不发 XFO**——
//     XFO 没有"全放行"的标准值（ALLOWALL 是 Chrome 私有），发了就等于把
//     /chat/embed 又关回去。
//
// `?embed=1` 由 /chat/embed 的 302 自己带上（见该 handler）：那段 302 之后浏览器
// 加载的是路径 `/`，服务端看不到原始 hash ⇒ 不把标记带过去，浮标在第三方站点里
// 会被 SAMEORIGIN 挡掉。安全边界：任何页面都能用 `?embed=1` 嵌我们的 SPA。
// 接受这条的理由是 embed 模式本来就是公开可嵌的访客界面；要收得更紧，就得给
// embed 单独出一份 shell 文档（独立任务，属路由语义改动）。
func serveSPAWithFramePolicy(c *gin.Context, distDir string, embeddable bool) {
	if embeddable {
		c.Header("Content-Security-Policy", "frame-ancestors *")
	} else {
		c.Header("X-Frame-Options", "SAMEORIGIN")
		c.Header("Content-Security-Policy", "frame-ancestors 'self'")
	}
	serveSPA(c, distDir)
}
