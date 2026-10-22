package config

import (
	"net/http"
	"net/url"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

var DefaultAllowedWSOrigins = []string{
	"http://localhost:3000",
	"http://localhost:8080",
	"http://localhost:5173",
	"http://localhost:8211",
	"http://localhost:8212",
	"http://localhost:8204",
	"http://127.0.0.1:8211",
	"http://127.0.0.1:8212",
	"http://127.0.0.1:8204",
}

// WSCORSConfig WebSocket 跨域配置 (从 config.yaml platform.allowed_ws_origins 段读取)
type WSCORSConfig struct {
	AllowedOrigins []string `yaml:"allowed_ws_origins" json:"allowed_ws_origins"`
}

var allowedWSOriginsCache []string
var allowedWSOriginsLoaded bool

// GetAllowedWSOrigins 获取 WebSocket Origin 白名单
//
// 返回:
//
//	[]string 允许的 origin 列表
//	  - "*" 表示允许所有 (仅调试)
//	  - 其他值要求严格匹配
//
// 加载顺序:
//
//  1. env ALLOWED_WS_ORIGINS (优先, 逗号分隔)
//  2. config.yaml platform.allowed_ws_origins 段
//  3. DefaultAllowedWSOrigins（本文件顶部；含 dev 8211/8212 与 8204 同源档）
func GetAllowedWSOrigins() []string {
	if envOrigins := os.Getenv("ALLOWED_WS_ORIGINS"); envOrigins != "" {
		parts := strings.Split(envOrigins, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			if v := strings.TrimSpace(p); v != "" {
				out = append(out, v)
			}
		}
		if len(out) > 0 {
			return out
		}
	}

	if !allowedWSOriginsLoaded {
		allowedWSOriginsCache = loadAllowedWSOriginsFromYAML()
		allowedWSOriginsLoaded = true
	}
	if len(allowedWSOriginsCache) > 0 {
		return allowedWSOriginsCache
	}

	return append([]string{}, DefaultAllowedWSOrigins...)
}

func loadAllowedWSOriginsFromYAML() []string {
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		return nil
	}
	expanded := os.ExpandEnv(string(data))

	var cfg struct {
		Platform WSCORSConfig `yaml:"platform"`
	}
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil
	}
	if len(cfg.Platform.AllowedOrigins) == 0 {
		return nil
	}
	return cfg.Platform.AllowedOrigins
}

// ReloadAllowedWSOrigins 清空缓存, 下次 GetAllowedWSOrigins 时重新读 yaml
// 用于 SIGHUP / admin API 触发配置重载。
func ReloadAllowedWSOrigins() {
	allowedWSOriginsLoaded = false
	allowedWSOriginsCache = nil
}

// IsSameOriginWS 判定 WebSocket 的 Origin 是否与请求本身同源。
//
// 背景（任务四 UI 拟人测试实测）：前端与 user-server 由同一反向代理层同源托管
// （.env.development 注释与 VITE_API_BASE_URL=/ 的口径），浏览器发起的 WS 握手
// 必然带 Origin=<前端站点域>，例如 https://work.xapptool.cn。而 WS 白名单
// （ALLOWED_WS_ORIGINS / config.yaml platform.allowed_ws_origins / 默认表）按
// 部署域逐个维护，且所有部署脚本都没有设过该变量 ⇒ 同源部署下坐席实时通知 WS
// 被 gorilla 以 403 "Forbidden" 拒绝，前端陷入重连循环且控制台持续报错。
//
// 因此同源一律放行（与 internal/router 的 CORS 同源判定口径一致，也与
// internal/websocket/visitor_handler.go 访客 WS 的既有做法一致）：同源意味着
// 请求就是本站自己的 JS 发起的；跨源仍然必须走白名单。
//
// 判定口径：Origin 的 host 与请求 Host 相同即同源（不比较 scheme，避免反代
// 终止 TLS 后 scheme 与浏览器视角不一致导致误拒）。
func IsSameOriginWS(origin string, r *http.Request) bool {
	if origin == "" || r == nil || r.Host == "" {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}
