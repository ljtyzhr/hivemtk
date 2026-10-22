package config

import (
	"net/http"
	"net/url"
	"os"
	"strconv"
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

// RequestScheme 取「浏览器视角」的请求 scheme。
//
// 口径与 internal/router 的 CORS 同源判定一致：
//  1. X-Forwarded-Proto（反代终止 TLS 时由反代写入，浏览器视角的 https）
//  2. r.TLS != nil ⇒ https
//  3. 否则 http
//
// known=false 表示「这个请求的 scheme 不可知」——直连 Go 的明文部署既没有
// X-Forwarded-Proto 也没有 TLS，上游看到的一律是 http，而浏览器那边的 Origin
// 可能是 https。
func RequestScheme(r *http.Request) (scheme string, known bool) {
	if r == nil {
		return "http", false
	}
	if proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); proto != "" {
		// 反代可能给 "https,http" 这种链式值，取第一段。
		if i := strings.IndexByte(proto, ','); i >= 0 {
			proto = strings.TrimSpace(proto[:i])
		}
		if proto != "" {
			return strings.ToLower(proto), true
		}
	}
	if r.TLS != nil {
		return "https", true
	}
	return "http", false
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
// 因此同源一律放行（同源意味着请求就是本站自己的 JS 发起的；跨源仍然必须走
// 白名单）。访客 WS 此前就有同源放行（visitor_handler.go），这里是把坐席 WS 与
// 聊天流式 WS 补齐到同一口径。
//
// 与 internal/router 的 CORS 同源判定（origin == scheme://Host 整串相等）的
// 两点差异，都是部署现实逼出来的，**不是**放松：
//
//  1. 端口：反代模板（docs/operations/reverse-proxy/nginx.conf.template）写的是
//     `proxy_set_header Host $host;` —— nginx 的 $host 是**去掉端口**的小写主机名，
//     而浏览器发的 Origin 带端口（https://host:8443）。整串比较在这种部署下必然
//     不等 ⇒ 正是要修的那个 403。故这里比**主机名**，端口不比。
//  2. scheme：反代终止 TLS 时上游请求是 http，只有 X-Forwarded-Proto 能还原
//     浏览器视角的 https。能取到就要求 scheme 相同（挡住 http→https 降级冒充）；
//     取不到（直连 Go 的明文部署）就只比主机名 —— 那时整站本就是明文，不存在
//     「降级冒充一个更安全的同站」这层风险。
func IsSameOriginWS(origin string, r *http.Request) bool {
	if origin == "" || r == nil || r.Host == "" {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	// 主机名（不含端口）必须相同
	if !strings.EqualFold(u.Hostname(), hostWithoutPort(r.Host)) {
		return false
	}
	// 端口：请求 Host 头带端口时可比（直连 Go 的形态），必须一致；
	// 反代用 $host 抹掉端口时无从比较，只能不比（见函数注释）。
	if p := portOf(r.Host); p != "" {
		if p == "80" || p == "443" {
			// 默认端口与 Origin 省略端口的写法等价
			if op := u.Port(); op != "" && op != p {
				return false
			}
		} else if u.Port() != p {
			return false
		}
	}
	// scheme 已知时必须相同
	if scheme, known := RequestScheme(r); known {
		return strings.EqualFold(u.Scheme, scheme)
	}
	return true
}

// hostWithoutPort 去掉 host 里的端口部分（"h:8443" ⇒ "h"）。
func hostWithoutPort(host string) string {
	if i := strings.LastIndexByte(host, ':'); i >= 0 {
		// 只有冒号后全是数字才当端口，避免误切 IPv6 字面量 "[::1]"
		if _, err := strconv.Atoi(host[i+1:]); err == nil {
			return host[:i]
		}
	}
	return host
}

// portOf 取 host 里的端口（没有则空串）。
func portOf(host string) string {
	if i := strings.LastIndexByte(host, ':'); i >= 0 {
		if _, err := strconv.Atoi(host[i+1:]); err == nil {
			return host[i+1:]
		}
	}
	return ""
}
