//go:build unix

package main

import (
	"time"

	"github.com/fvbock/endless"
	"github.com/gin-gonic/gin"
)

func serveHTTP(addr string, r *gin.Engine) error {
	// 优雅停机由 endless 管（SIGTERM/SIGINT → 关监听 + 等在途请求收敛）。被 Hijack 的
	// 长连接（Host WS、SSE）永不 WaitGroup.Done，旧进程必须活满 DefaultHammerTime
	// （库默认 60s）才被强制收掉；期间端口已释放、旧实例 cron/outbox 仍在跑（双 worker
	// 并存窗口）。压到 15s：普通请求收敛够用，双跑窗口 60s→15s。
	// 运维即时停用 `kill -USR2 <pid>`（endless.hammerTime(0) 立即强退）；
	// `kill -HUP` 是 endless.fork() 会并存第二个 server，不是重启。
	endless.DefaultHammerTime = 15 * time.Second

	// 只设 ReadHeaderTimeout/IdleTimeout:不设 ReadTimeout(知识库大文件上传)与
	// WriteTimeout(SSE 长连接会被强制断开);endless.Server 内嵌 http.Server,
	// 热重启能力与超时配置并存
	srv := endless.NewServer(addr, r)
	srv.ReadHeaderTimeout = 10 * time.Second
	srv.IdleTimeout = 120 * time.Second
	// ListenAndServe 会创建 EndlessListener 后调 Serve;
	// 直接调 Serve 时 EndlessListener 为 nil → Accept 空指针 panic
	return srv.ListenAndServe()
}
