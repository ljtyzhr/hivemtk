// Package kbcrawler 单元测试：robots / sitemap / 抽取 / 限速 / 链接发现。
// 表驱动，无网络依赖（httptest 起本地 fixture）。
package kbcrawler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseRobots(t *testing.T) {
	cases := []struct {
		name      string
		body      string
		allowPath string
		denyPath  string
		wantDelay time.Duration
	}{
		{
			name:      "基本 Disallow",
			body:      "User-agent: *\nDisallow: /admin/\n",
			allowPath: "/docs/x",
			denyPath:  "/admin/x",
		},
		{
			name:      "Crawl-delay 生效",
			body:      "User-agent: *\nCrawl-delay: 3\nDisallow:\n",
			allowPath: "/a",
			denyPath:  "",
			wantDelay: 3 * time.Second,
		},
		{
			name:      "指定 UA 组不影响星号组",
			body:      "User-agent: Googlebot\nDisallow: /\n",
			allowPath: "/anything",
			denyPath:  "",
		},
		{
			name:      "注释行忽略",
			body:      "# comment\nUser-agent: * # star\nDisallow: /private\n",
			allowPath: "/pub",
			denyPath:  "/private/x",
		},
		{
			name:      "Allow 优先于等长 Disallow",
			body:      "User-agent: *\nDisallow: /docs\nAllow: /docs/public\n",
			allowPath: "/docs/public/a",
			denyPath:  "/docs/other",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rules, err := ParseRobots(strings.NewReader(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			if tc.allowPath != "" && !rules.Allowed("https://x.com"+tc.allowPath) {
				t.Errorf("%s 应允许", tc.allowPath)
			}
			if tc.denyPath != "" && rules.Allowed("https://x.com"+tc.denyPath) {
				t.Errorf("%s 应拒绝", tc.denyPath)
			}
			if rules.Delay() != tc.wantDelay {
				t.Errorf("Delay = %v, want %v", rules.Delay(), tc.wantDelay)
			}
		})
	}
}

func TestParseRobots_CrawlDelayCapped(t *testing.T) {
	rules, _ := ParseRobots(strings.NewReader("User-agent: *\nCrawl-delay: 999\n"))
	if rules.Delay() != 60*time.Second {
		t.Errorf("Crawl-delay 应封顶 60s, got %v", rules.Delay())
	}
}

func TestNilRulesFailOpen(t *testing.T) {
	var rules *RobotsRules
	if !rules.Allowed("https://x.com/anything") {
		t.Error("nil rules 应 fail-open 全放行")
	}
}

func TestFetchRobots_403MeansDisallowAll(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	rules, err := FetchRobots(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if rules.Allowed(srv.URL + "/anything") {
		t.Error("403 应视为全站禁爬")
	}
}

func TestFetchRobots_NetworkFailFailsOpen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	rules, err := FetchRobots(context.Background(), srv.Client(), srv.URL)
	if err == nil || rules != nil {
		t.Error("5xx 应 fail-open 返回 nil rules")
	}
}

func TestParseSitemap(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantLen int
	}{
		{"urlset", `<urlset><url><loc>https://x.com/a</loc></url><url><loc>https://x.com/b</loc></url></urlset>`, 2},
		{"空 loc 跳过", `<urlset><url><loc></loc></url><url><loc>https://x.com/a</loc></url></urlset>`, 1},
		{"lastmod 保留", `<urlset><url><loc>https://x.com/a</loc><lastmod>2026-01-01T00:00:00Z</lastmod></url></urlset>`, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			entries, err := ParseSitemap(context.Background(), srv.Client(), srv.URL, map[string]bool{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != tc.wantLen {
				t.Errorf("got %d entries, want %d: %+v", len(entries), tc.wantLen, entries)
			}
		})
	}
}

func TestParseSitemap_IndexExpandsOneLevel(t *testing.T) {
	// /index → sitemapindex 指向 /sub；/sub → urlset 一条
	mux := http.NewServeMux()
	mux.HandleFunc("/index", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<sitemapindex><sitemap><loc>` + "http://" + r.Host + `/sub</loc></sitemap></sitemapindex>`))
	})
	mux.HandleFunc("/sub", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<urlset><url><loc>https://x.com/page</loc></url></urlset>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	entries, err := ParseSitemap(context.Background(), srv.Client(), srv.URL+"/index", map[string]bool{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Loc != "https://x.com/page" {
		t.Errorf("index 应展开一层得 1 条: %+v", entries)
	}
}

func TestParseSitemap_CycleGuard(t *testing.T) {
	// sitemap A 指向自己 → 防环：只解析一次
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<sitemapindex><sitemap><loc>` + "http://" + r.Host + `</loc></sitemap></sitemapindex>`))
	}))
	defer srv.Close()
	entries, err := ParseSitemap(context.Background(), srv.Client(), srv.URL, map[string]bool{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("环引用应返回 0 条, got %d", len(entries))
	}
}

func TestParseSitemap_LastmodPrefilter(t *testing.T) {
	body := `<urlset>
		<url><loc>https://x.com/old</loc><lastmod>2020-01-01T00:00:00Z</lastmod></url>
		<url><loc>https://x.com/new</loc><lastmod>2026-06-01T00:00:00Z</lastmod></url>
	</urlset>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	cutoff := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	entries, err := ParseSitemap(context.Background(), srv.Client(), srv.URL, map[string]bool{}, &cutoff)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Loc != "https://x.com/new" {
		t.Errorf("lastmod 预筛应只剩 new: %+v", entries)
	}
}

func TestGoqueryExtractor(t *testing.T) {
	cases := []struct {
		name      string
		html      string
		wantErr   error
		wantTitle string
		minLen    int
	}{
		{
			name: "main 选择器命中",
			html: `<html><head><title>帮助中心</title></head><body><nav>导航导航</nav><main><p>` +
				strings.Repeat("这是产品使用说明的正文内容，介绍各种功能与常见问题。", 30) + `</p></main></body></html>`,
			wantTitle: "帮助中心",
			minLen:    200,
		},
		{
			name:    "纯导航页判空",
			html:    `<html><body><nav><a href="/">首页</a><a href="/about">关于</a></nav></body></html>`,
			wantErr: ErrEmptyPage,
		},
		{
			name:    "JS 壳判空",
			html:    `<html><head><title>SPA</title></head><body><div id="app"></div><script>var x=1;</script></body></html>`,
			wantErr: ErrEmptyPage,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			page, err := NewGoqueryExtractor().Extract(tc.html, "https://x.com/p")
			if tc.wantErr != nil {
				if err != tc.wantErr {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if page.Title != tc.wantTitle {
				t.Errorf("title = %q, want %q", page.Title, tc.wantTitle)
			}
			if len([]rune(page.Content)) < tc.minLen {
				t.Errorf("content 长度 %d < %d", len([]rune(page.Content)), tc.minLen)
			}
		})
	}
}

func TestExtractLinks(t *testing.T) {
	html := `<html><body>
		<a href="/docs/a">A</a>
		<a href="https://other.com/x">外域</a>
		<a href="mailto:a@b.c">邮件</a>
		<a href="#anchor">锚点</a>
		<a href="/docs/b?q=1">B</a>
	</body></html>`
	links := ExtractLinks(html, "https://x.com/docs/index", nil)
	if len(links) != 2 { // 外域/mailto/锚点剔除；b 的 query 被剥
		t.Fatalf("got %v", links)
	}
	// include 白名单
	links2 := ExtractLinks(html, "https://x.com/docs/index", []string{"https://x.com/docs/"})
	if len(links2) != 2 {
		t.Fatalf("include 命中应 2 条, got %v", links2)
	}
}

func TestHostLimiter(t *testing.T) {
	l := NewHostLimiter(0) // 下限抬到 1s
	start := time.Now()
	if !l.Acquire(context.Background()) || !l.Acquire(context.Background()) {
		t.Fatal("Acquire 不应返回 false")
	}
	if elapsed := time.Since(start); elapsed < 900*time.Millisecond {
		t.Errorf("第二次 Acquire 应等待 ≥1s, 实际 %v", elapsed)
	}
}
