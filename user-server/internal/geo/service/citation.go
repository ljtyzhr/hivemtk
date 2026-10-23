package service

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"hivemtk-user/internal/geo/model"
	"hivemtk-user/internal/geo/repository"
	"hivemtk-user/internal/reporting"
)

// CitationService 引用智能服务
//
// 对标 gego stats/urls 与 elmo 引用分析：把探针 Citations jsonb 聚合成
// 域名统计（自有/earned/竞品分类）与 keyword×domain 矩阵。
type CitationService struct {
	probeRepo      repository.GeoProbeRunRepository
	configRepo     repository.GeoConfigRepository
	competitorRepo repository.GeoCompetitorRepository
}

// NewCitationService 构造引用智能服务
func NewCitationService(
	probeRepo repository.GeoProbeRunRepository,
	configRepo repository.GeoConfigRepository,
	competitorRepo repository.GeoCompetitorRepository,
) *CitationService {
	return &CitationService{probeRepo: probeRepo, configRepo: configRepo, competitorRepo: competitorRepo}
}

// CitationDomainStat 单域名引用统计
type CitationDomainStat struct {
	Domain      string `json:"domain"`
	Category    string `json:"category"` // own | competitor | earned
	Count       int    `json:"count"`
	QueryCount  int    `json:"query_count"`
	EngineCount int    `json:"engine_count"`
}

// CitationDomainResult 域名聚合结果
type CitationDomainResult struct {
	Days    int                  `json:"days"`
	Domains []CitationDomainStat `json:"domains"`
	Total   int                  `json:"total"`

	// A9（2026-10-24）引用域名覆盖率：自有域引用 / 总引用。
	// OwnCitations=0 且 Total>0 ⇒ 覆盖率 0%，GEO 的"让 AI 引用你"目标还没兑现第一条。
	OwnCitations int            `json:"own_citations"`
	OwnCoverage  float64        `json:"own_coverage"` // 0~1，Total=0 时恒 0
	TopExternal  []CitationLink `json:"top_external"` // Top 外链（非自有域），按引用次数降序
}

// CitationLink 单条外链（A9：Top 外链明细，域名级聚合太粗，运营要看具体 URL）。
type CitationLink struct {
	URL    string `json:"url"`
	Domain string `json:"domain"`
	Count  int    `json:"count"`
}

// OwnDomainCoverage 纯函数：从域名统计里算自有域覆盖率。
// Total=0 ⇒ 0（没有引用谈何覆盖）。
func OwnDomainCoverage(stats []CitationDomainStat) (own, total int, coverage float64) {
	for _, st := range stats {
		total += st.Count
		if st.Category == "own" {
			own += st.Count
		}
	}
	if total > 0 {
		coverage = float64(own) / float64(total)
	}
	return own, total, coverage
}

// TopExternalLinks 纯函数：非自有域的外链 TopN（按 count 降序、URL 升序钉死并列序）。
// urls 为 URL→count 的原始计数（调用方从 runs 聚合）。
func TopExternalLinks(stats []CitationDomainStat, n int) []CitationLink {
	if n <= 0 {
		n = 10
	}
	var ext []CitationDomainStat
	for _, st := range stats {
		if st.Category != "own" {
			ext = append(ext, st)
		}
	}
	sort.Slice(ext, func(i, j int) bool {
		if ext[i].Count != ext[j].Count {
			return ext[i].Count > ext[j].Count
		}
		return ext[i].Domain < ext[j].Domain
	})
	out := make([]CitationLink, 0, n)
	for _, st := range ext {
		if len(out) >= n {
			break
		}
		out = append(out, CitationLink{Domain: st.Domain, Count: st.Count})
	}
	return out
}

// CitationMatrixRow keyword×domain 矩阵行
type CitationMatrixRow struct {
	Query   string         `json:"query"`
	Domains map[string]int `json:"domains"`
	Total   int            `json:"total"`
}

// CitationMatrixResult 矩阵结果
type CitationMatrixResult struct {
	Days    int                 `json:"days"`
	Domains []string            `json:"domains"`
	Rows    []CitationMatrixRow `json:"rows"`
}

// normalizeHost 归一化 host：小写、去 www. 前缀、去端口
func normalizeHost(host string) string {
	h := strings.ToLower(strings.TrimSpace(host))
	h = strings.TrimPrefix(h, "www.")
	if idx := strings.Index(h, ":"); idx >= 0 {
		h = h[:idx]
	}
	return h
}

// classifyDomain 域名分类：自有 / 竞品 / earned
func classifyDomain(host, ownDomain string, competitorDomains map[string]bool) string {
	h := normalizeHost(host)
	if h == "" {
		return ""
	}
	own := normalizeHost(ownDomain)
	if own != "" && (h == own || strings.HasSuffix(h, "."+own)) {
		return "own"
	}
	if competitorDomains[h] {
		return "competitor"
	}
	return "earned"
}

// extractCitationURLs 从探针行解析引用 URL 列表
func extractCitationURLs(run *model.GeoProbeRun) []string {
	if run == nil || len(run.Citations) == 0 {
		return nil
	}
	var cites []Citation
	if err := json.Unmarshal(run.Citations, &cites); err != nil {
		return nil
	}
	out := make([]string, 0, len(cites))
	for _, c := range cites {
		if strings.TrimSpace(c.URL) != "" {
			out = append(out, c.URL)
		}
	}
	return out
}

// AggregateCitationDomains 纯聚合：runs → 按域名统计（含分类）
// ownDomain 为品牌官网域，competitorDomains 为竞品域集合（均做归一化）。
func AggregateCitationDomains(runs []*model.GeoProbeRun, ownDomain string, competitorDomains []string) []CitationDomainStat {
	compSet := make(map[string]bool, len(competitorDomains))
	for _, d := range competitorDomains {
		if h := normalizeHost(d); h != "" {
			compSet[h] = true
		}
	}
	type bucket struct {
		count   int
		queries map[string]bool
		engines map[string]bool
		cat     string
	}
	buckets := map[string]*bucket{}
	for _, run := range runs {
		for _, rawURL := range extractCitationURLs(run) {
			host := normalizeHost(ExtractDomain(rawURL))
			if host == "" {
				continue
			}
			b, ok := buckets[host]
			if !ok {
				b = &bucket{queries: map[string]bool{}, engines: map[string]bool{}, cat: classifyDomain(host, ownDomain, compSet)}
				buckets[host] = b
			}
			b.count++
			b.queries[run.Query] = true
			b.engines[run.Engine] = true
		}
	}
	out := make([]CitationDomainStat, 0, len(buckets))
	for host, b := range buckets {
		out = append(out, CitationDomainStat{
			Domain: host, Category: b.cat, Count: b.count,
			QueryCount: len(b.queries), EngineCount: len(b.engines),
		})
	}
	// 只按 Count 排时并列项先后取自 buckets 的迭代序（Go 每轮随机）⇒ 同数据两次请求给出不同榜单；
	// 域名升序兜底把顺序钉死。
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Domain < out[j].Domain
	})
	return out
}

// AggregateKeywordDomainMatrix 纯聚合：top 关键词 × top 域名矩阵
func AggregateKeywordDomainMatrix(runs []*model.GeoProbeRun, topQueries, topDomains int) ([]string, []CitationMatrixRow) {
	if topQueries <= 0 {
		topQueries = 20
	}
	if topDomains <= 0 {
		topDomains = 10
	}
	qCount := map[string]int{}
	dCount := map[string]int{}
	cells := map[string]map[string]int{}
	for _, run := range runs {
		urls := extractCitationURLs(run)
		if len(urls) == 0 {
			continue
		}
		qCount[run.Query]++
		for _, rawURL := range urls {
			host := normalizeHost(ExtractDomain(rawURL))
			if host == "" {
				continue
			}
			dCount[host]++
			m, ok := cells[run.Query]
			if !ok {
				m = map[string]int{}
				cells[run.Query] = m
			}
			m[host]++
		}
	}
	queries := topKeys(qCount, topQueries)
	domains := topKeys(dCount, topDomains)
	rows := make([]CitationMatrixRow, 0, len(queries))
	for _, q := range queries {
		row := CitationMatrixRow{Query: q, Domains: map[string]int{}}
		for _, d := range domains {
			if v := cells[q][d]; v > 0 {
				row.Domains[d] = v
				row.Total += v
			}
		}
		rows = append(rows, row)
	}
	return domains, rows
}

func topKeys(m map[string]int, n int) []string {
	type kv struct {
		k string
		v int
	}
	all := make([]kv, 0, len(m))
	for k, v := range m {
		all = append(all, kv{k, v})
	}
	// 同分必须按 key 升序兜底：只按 v 排时，并列项的先后取自 map 迭代序（Go 每轮随机），
	// 而下面又要截断到 n —— 于是同一份数据，两次请求能给出不同的 Top-N 榜单。
	sort.Slice(all, func(i, j int) bool {
		if all[i].v != all[j].v {
			return all[i].v > all[j].v
		}
		return all[i].k < all[j].k
	})
	if len(all) > n {
		all = all[:n]
	}
	out := make([]string, 0, len(all))
	for _, kv := range all {
		out = append(out, kv.k)
	}
	return out
}

// loadClassifyContext 读取分类上下文：官网域 + 竞品域（配置 Competitors 文本 + 竞品表）
func (s *CitationService) loadClassifyContext(ctx context.Context) (string, []string) {
	var own string
	var comp []string
	if cfg, err := s.configRepo.Get(ctx); err == nil && cfg != nil {
		own = ExtractDomain(cfg.Domain)
		for _, c := range strings.Split(cfg.Competitors, ",") {
			if d := ExtractDomain(strings.TrimSpace(c)); d != "" {
				comp = append(comp, d)
			}
		}
	}
	if comps, err := s.competitorRepo.ListActive(ctx); err == nil {
		for _, c := range comps {
			if d := normalizeHost(c.Domain); d != "" {
				comp = append(comp, d)
			}
		}
	}
	return own, comp
}

// DomainStats 聚合近 N 天探针引用域名
//
// A9（2026-10-24）：结果附带自有域覆盖率（OwnCoverage）与 Top 外链明细——
// 只有"own/competitor/earned 分类"读不出"GEO 做得怎么样"这一句话，覆盖率才是判据。
func (s *CitationService) DomainStats(ctx context.Context, days int) (*CitationDomainResult, error) {
	if days <= 0 || days > 90 {
		days = reporting.DefaultWindowDays(ctx)
	}
	runs, err := s.probeRepo.ListSince(ctx, time.Now().AddDate(0, 0, -days), 20000)
	if err != nil {
		return nil, err
	}
	own, comp := s.loadClassifyContext(ctx)
	stats := AggregateCitationDomains(runs, own, comp)
	total := 0
	for _, st := range stats {
		total += st.Count
	}
	res := &CitationDomainResult{Days: days, Domains: stats, Total: total}
	res.OwnCitations, res.Total, res.OwnCoverage = OwnDomainCoverage(stats)
	res.TopExternal = topExternalLinksWithURLs(runs, own, comp, 10)
	return res, nil
}

// topExternalLinksWithURLs 在域名分类之上聚合 URL 级明细（A9 的 Top 外链要能给到具体 URL）。
func topExternalLinksWithURLs(runs []*model.GeoProbeRun, ownDomain string, competitorDomains []string, n int) []CitationLink {
	compSet := make(map[string]bool, len(competitorDomains))
	for _, d := range competitorDomains {
		if h := normalizeHost(d); h != "" {
			compSet[h] = true
		}
	}
	own := normalizeHost(ownDomain)
	urlCount := map[string]int{}
	for _, run := range runs {
		for _, raw := range extractCitationURLs(run) {
			host := normalizeHost(ExtractDomain(raw))
			if host == "" || host == own || strings.HasSuffix(host, "."+own) || compSet[host] {
				continue
			}
			urlCount[raw]++
		}
	}
	type uc struct {
		url   string
		count int
	}
	var list []uc
	for u, c := range urlCount {
		list = append(list, uc{u, c})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].count != list[j].count {
			return list[i].count > list[j].count
		}
		return list[i].url < list[j].url
	})
	out := make([]CitationLink, 0, n)
	for i, e := range list {
		if i >= n {
			break
		}
		out = append(out, CitationLink{URL: e.url, Domain: normalizeHost(ExtractDomain(e.url)), Count: e.count})
	}
	return out
}

// KeywordDomainMatrix 近 N 天 keyword×domain 矩阵
func (s *CitationService) KeywordDomainMatrix(ctx context.Context, days, topQueries, topDomains int) (*CitationMatrixResult, error) {
	if days <= 0 || days > 90 {
		days = reporting.DefaultWindowDays(ctx)
	}
	runs, err := s.probeRepo.ListSince(ctx, time.Now().AddDate(0, 0, -days), 20000)
	if err != nil {
		return nil, err
	}
	domains, rows := AggregateKeywordDomainMatrix(runs, topQueries, topDomains)
	return &CitationMatrixResult{Days: days, Domains: domains, Rows: rows}, nil
}
