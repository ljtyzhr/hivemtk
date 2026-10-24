// Package kbcrawler T4 知识库冷启动：robots.txt 解析与同域限速（§4.3）。
//
// robots.txt 获取失败 fail-open（仓库对外请求的通用惯例）；401/403 视为全站禁爬。
package kbcrawler

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// UserAgent 爬虫 UA（租户自提交域名，标明用途）
const UserAgent = "hivemtk-kb-crawler/1.0"

// RobotsRules 单 host 的 robots 规则（仅解析 User-agent: * 组，够用原则）。
// nil *RobotsRules = fail-open，Allowed 恒允许。
type RobotsRules struct {
	disallow   []string
	allow      []string
	crawlDelay time.Duration
}

// Allowed 判 URL 是否允许爬取
func (r *RobotsRules) Allowed(rawURL string) bool {
	if r == nil {
		return true
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	path := u.Path
	if path == "" {
		path = "/"
	}
	// 最长前缀匹配：Allow 与 Disallow 同长时 Allow 优先（Google 风格）
	best := -1
	allowed := true
	for _, d := range r.disallow {
		if strings.HasPrefix(path, d) && len(d) > best {
			best = len(d)
			allowed = false
		}
	}
	for _, a := range r.allow {
		if strings.HasPrefix(path, a) && len(a) >= best {
			best = len(a)
			allowed = true
		}
	}
	return allowed
}

// Delay 返回 Crawl-delay（与全局最小间隔 1s 取大者由调用方做）
func (r *RobotsRules) Delay() time.Duration {
	if r == nil {
		return 0
	}
	return r.crawlDelay
}

// FetchRobots 拉取并解析 <scheme>://<host>/robots.txt。
//   - 200 → 解析规则；
//   - 401/403 → 全站禁爬（disallow:["/"]）；
//   - 其他失败（网络错/4xx/5xx）→ fail-open，返回 (nil, err)。
func FetchRobots(ctx context.Context, client *http.Client, siteURL string) (*RobotsRules, error) {
	u, err := url.Parse(siteURL)
	if err != nil {
		return nil, fmt.Errorf("解析站点 URL 失败: %w", err)
	}
	robotsURL := u.Scheme + "://" + u.Host + "/robots.txt"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, robotsURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err // fail-open
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		// 明确拒绝：全站禁爬
		return &RobotsRules{disallow: []string{"/"}}, nil
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("robots.txt HTTP %d", resp.StatusCode) // fail-open
	}
	return ParseRobots(resp.Body)
}

// ParseRobots 解析 robots.txt 文本（仅 User-agent: * 组）
func ParseRobots(r io.Reader) (*RobotsRules, error) {
	rules := &RobotsRules{}
	sc := bufio.NewScanner(r)
	inStarGroup := false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if i := strings.Index(line, "#"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		if line == "" {
			continue
		}
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		val = strings.TrimSpace(val)
		switch key {
		case "user-agent":
			inStarGroup = val == "*"
		case "crawl-delay":
			if !inStarGroup {
				continue
			}
			if secs, err := strconv.Atoi(strings.Split(val, ".")[0]); err == nil && secs > 0 {
				if secs > 60 {
					secs = 60 // Crawl-delay 上限 60s，防恶意配置把爬虫冻死
				}
				rules.crawlDelay = time.Duration(secs) * time.Second
			}
		case "disallow":
			if !inStarGroup {
				continue
			}
			if val != "" {
				rules.disallow = append(rules.disallow, val)
			}
		case "allow":
			if !inStarGroup {
				continue
			}
			if val != "" {
				rules.allow = append(rules.allow, val)
			}
		}
	}
	return rules, sc.Err()
}

// HostLimiter 同 host 串行限速器：Acquire 阻塞到可请求（间隔 ≥ interval，下限 1s）。
type HostLimiter struct {
	last     time.Time
	interval time.Duration
}

// NewHostLimiter 构造（interval < 1s 时抬到 1s：§4.3 请求间隔 ≥ 1s/请求·host）
func NewHostLimiter(interval time.Duration) *HostLimiter {
	if interval < time.Second {
		interval = time.Second
	}
	return &HostLimiter{interval: interval}
}

// Acquire 阻塞等待到下次可请求时刻；ctx 取消则返回 false。
func (l *HostLimiter) Acquire(ctx context.Context) bool {
	now := time.Now()
	if wait := l.interval - now.Sub(l.last); wait > 0 {
		t := time.NewTimer(wait)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return false
		case <-t.C:
		}
	}
	l.last = time.Now()
	return true
}
