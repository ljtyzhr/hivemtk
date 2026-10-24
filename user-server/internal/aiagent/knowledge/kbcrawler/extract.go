// kbcrawler T4：sitemap 解析（§4.5）与正文抽取（§4.4）。
//
// sitemap 用 encoding/xml 标准库；sitemapindex 递归展开一层（防环：已见集合去重）。
// 抽取器封装为 Extractor 接口（MinerU/Unstructured 外部解析服务插槽，§1.3），
// 默认实现 goqueryExtractor + 选择器/文本密度兜底；正文 < 200 字符判空页跳过。
package kbcrawler

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// SitemapEntry sitemap 解析出的一个 URL
type SitemapEntry struct {
	Loc     string
	LastMod string
}

// ParseSitemap 抓取 sitemapURL 并解析。
//   - urlset → 返回 URL 列表；
//   - sitemapindex → 递归展开一层（visited 防环）。
//
// lastmodAfter 非 nil 时预筛：有 lastmod 且早于它的 URL 直接跳过（§4.5，最终仍以内容哈希为准）。
func ParseSitemap(ctx context.Context, client *http.Client, sitemapURL string, visited map[string]bool, lastmodAfter *time.Time) ([]SitemapEntry, error) {
	if visited == nil {
		visited = map[string]bool{}
	}
	if visited[sitemapURL] {
		return nil, nil
	}
	visited[sitemapURL] = true

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sitemapURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("抓取 sitemap 失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sitemap HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, err
	}

	var raw struct {
		XMLName xml.Name
		Locs    []struct {
			Loc     string `xml:"loc"`
			LastMod string `xml:"lastmod"`
		} `xml:",any"`
	}
	if err := xml.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("sitemap XML 解析失败: %w", err)
	}

	var out []SitemapEntry
	isIndex := raw.XMLName.Local == "sitemapindex"
	for _, l := range raw.Locs {
		loc := strings.TrimSpace(l.Loc)
		if loc == "" {
			continue
		}
		if isIndex {
			sub, err := ParseSitemap(ctx, client, loc, visited, lastmodAfter)
			if err != nil {
				continue // 单个子 sitemap 失败不拖垮整批
			}
			out = append(out, sub...)
			continue
		}
		if lastmodAfter != nil && l.LastMod != "" {
			if t, err := time.Parse(time.RFC3339, l.LastMod); err == nil && t.Before(*lastmodAfter) {
				continue // lastmod 预筛
			}
		}
		out = append(out, SitemapEntry{Loc: loc, LastMod: l.LastMod})
	}
	return out, nil
}

// Page 正文抽取结果
type Page struct {
	Title   string
	Content string // 纯文本正文（规范化空白后）
}

// ErrEmptyPage 空页（正文不足 / 纯导航 / JS 渲染壳）
var ErrEmptyPage = errors.New("页面无可提取正文（疑似空壳或导航页）")

// Extractor 正文抽取接口（§1.3 外部解析服务插槽：将来接 MinerU/Unstructured
// 只需新增一个实现，接入模式 = HTTP 调用 + 兜底本地）。
type Extractor interface {
	Extract(html string, pageURL string) (*Page, error)
}

// minContentLen 正文最小长度：低于此值判空页跳过（§4.4 兜底 200 字符）
const minContentLen = 200

// HTMLContentSelectors 优先命中的正文选择器（企业文档站/FAQ 页常见容器，按序尝试）
var HTMLContentSelectors = []string{
	"main", "article", "#content", ".content", ".doc-content", ".markdown-body",
	".article-content", ".faq-content", "[role=main]", ".prose", ".entry-content",
}

// GoqueryExtractor 默认抽取器：选择器优先 → body 文本密度兜底
type GoqueryExtractor struct{}

// NewGoqueryExtractor 构造
func NewGoqueryExtractor() *GoqueryExtractor { return &GoqueryExtractor{} }

// Extract 抽取标题与正文。正文 < minContentLen 时返回 ErrEmptyPage。
func (e *GoqueryExtractor) Extract(html, pageURL string) (*Page, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("HTML 解析失败: %w", err)
	}
	// 剥离噪声标签
	doc.Find("script,style,nav,header,footer,aside,noscript,iframe,form").Remove()

	title := strings.TrimSpace(doc.Find("title").First().Text())
	if h1 := strings.TrimSpace(doc.Find("h1").First().Text()); title == "" && h1 != "" {
		title = h1
	}

	var content string
	for _, sel := range HTMLContentSelectors {
		s := doc.Find(sel).First()
		if s.Length() == 0 {
			continue
		}
		if txt := NormalizeText(s.Text()); len([]rune(txt)) >= minContentLen {
			content = txt
			break
		}
	}
	if content == "" {
		// 兜底：body 全文密度启发式（含 minContentLen 判定）
		content = NormalizeText(doc.Find("body").Text())
	}
	if len([]rune(content)) < minContentLen {
		return nil, ErrEmptyPage
	}
	return &Page{Title: title, Content: content}, nil
}

// NormalizeText 规范化空白：连续空白折叠为单个空格
func NormalizeText(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// ExtractLinks 抽取页面内同域、命中 includePatterns 的 <a href>（BFS 一步）。
// includePatterns 为空 = 同域全量；重定向跨 host 由调用方抓取时校验。
func ExtractLinks(html, pageURL string, includePatterns []string) []string {
	base, err := url.Parse(pageURL)
	if err != nil {
		return nil
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		href, ok := s.Attr("href")
		if !ok || href == "" || strings.HasPrefix(href, "#") ||
			strings.HasPrefix(href, "mailto:") || strings.HasPrefix(href, "javascript:") {
			return
		}
		u, err := url.Parse(href)
		if err != nil {
			return
		}
		abs := base.ResolveReference(u)
		if abs.Host != base.Host { // 仅同域
			return
		}
		abs.Fragment = ""
		abs.RawQuery = "" // query 变体视为同一页
		absStr := abs.String()
		if seen[absStr] {
			return
		}
		if !MatchInclude(absStr, includePatterns) {
			return
		}
		seen[absStr] = true
		out = append(out, absStr)
	})
	return out
}

// MatchInclude URL 是否命中 include 前缀白名单（空 = 全放行）
func MatchInclude(rawURL string, patterns []string) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p != "" && strings.HasPrefix(rawURL, p) {
			return true
		}
	}
	return false
}
