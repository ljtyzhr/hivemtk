package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"hivemtk-user/internal/geo/model"
	"hivemtk-user/internal/geo/repository"
	"hivemtk-user/internal/pkg/timeutil"
)

// citation_drift.go —— A10 引用漂移（2026-10-24）。
//
// 规划 §4.8 A10：定时快照 CitedURLs，按域名/URL 做增删与排名变化。
// 引用是探针流水（geo_probe_runs.citations jsonb），跑过就散了——没有按日快照
// 就没有"漂移"可言。本文件三件事：
//  1. 快照：按业务日聚合当日 URL 引用计数写入 geo_citation_snapshots（cron）；
//  2. diff：两次快照对比，输出新增/消失/计数变化/排名变化；
//  3. 读端点：/geo/citations/drift?from=&to=。
//
// 与 AggregateCitationDomains 的分工：那边是"窗口内域名汇总"（现状），
// 这边是"两个时点之间的变化"（趋势）。不合并——一个是报表一个是 diff，混在一起
// 读数口径会打架。

// CitationDriftService 引用漂移服务
type CitationDriftService struct {
	probeRepo repository.GeoProbeRunRepository
	snapRepo  repository.GeoCitationSnapshotRepository
}

func NewCitationDriftService(
	probeRepo repository.GeoProbeRunRepository,
	snapRepo repository.GeoCitationSnapshotRepository,
) *CitationDriftService {
	return &CitationDriftService{probeRepo: probeRepo, snapRepo: snapRepo}
}

// TakeSnapshot 聚合当日（业务日）引用 URL 计数并 UPSERT 快照。
// 重复执行安全：同日重跑覆盖当日行（DoUpdates），幂等。
func (s *CitationDriftService) TakeSnapshot(ctx context.Context) (*model.GeoCitationSnapshot, error) {
	todayStart := timeutil.StartOfBusinessDay(time.Now())
	today := timeutil.BusinessDate(todayStart)
	runs, err := s.probeRepo.ListSince(ctx, todayStart, 0)
	if err != nil {
		return nil, fmt.Errorf("拉取今日探针记录失败: %w", err)
	}
	counts := AggregateRunCitationCounts(runs)
	total := 0
	for _, c := range counts {
		total += c
	}
	raw, err := json.Marshal(counts)
	if err != nil {
		return nil, fmt.Errorf("序列化 URL 计数失败: %w", err)
	}
	snap := &model.GeoCitationSnapshot{
		SnapshotDate:   today,
		URLCounts:      string(raw),
		TotalCitations: total,
		DistinctURLs:   len(counts),
	}
	if err := s.snapRepo.Upsert(ctx, snap); err != nil {
		return nil, fmt.Errorf("写入快照失败: %w", err)
	}
	return snap, nil
}

// AggregateRunCitationCounts 纯聚合：探针行 → URL→引用次数（跨行同 URL 累加）。
func AggregateRunCitationCounts(runs []*model.GeoProbeRun) map[string]int {
	counts := map[string]int{}
	for _, run := range runs {
		for _, u := range extractCitationURLs(run) {
			counts[u]++
		}
	}
	return counts
}

// CitationDriftResult 两次快照之间的漂移
type CitationDriftResult struct {
	FromDate  string             `json:"from_date"`
	ToDate    string             `json:"to_date"`
	Added     []CitationDrift    `json:"added"`      // 新增引用（from 无 → to 有）
	Removed   []CitationDrift    `json:"removed"`    // 消失引用（from 有 → to 无）
	Changed   []CitationChange   `json:"changed"`    // 计数变化（两侧都有）
	RankMoves []CitationRankMove `json:"rank_moves"` // 排名变化（两侧都在且计数>0）
}

// CitationDrift 单条增/删
type CitationDrift struct {
	URL    string `json:"url"`
	Domain string `json:"domain"`
	Count  int    `json:"count"` // to 侧（added）或 from 侧（removed）的计数
}

// CitationChange 计数变化
type CitationChange struct {
	URL    string `json:"url"`
	Domain string `json:"domain"`
	From   int    `json:"from"`
	To     int    `json:"to"`
	Delta  int    `json:"delta"`
}

// CitationRankMove 排名变化（按引用次数排名；1 = 最热）
type CitationRankMove struct {
	URL      string `json:"url"`
	Domain   string `json:"domain"`
	FromRank int    `json:"from_rank"`
	ToRank   int    `json:"to_rank"`
}

// GetDriftBetween diff [fromDate, toDate] 两次快照。from 为空 ⇒ 取 to 之前最近的一次快照。
// 两次快照都不足 ⇒ 返回带日期的错误形状（不是 panic、不是空 diff 冒充"没有漂移"）。
func (s *CitationDriftService) GetDriftBetween(ctx context.Context, fromDate, toDate string) (*CitationDriftResult, error) {
	if strings.TrimSpace(toDate) == "" {
		toDate = timeutil.BusinessDate(timeutil.StartOfBusinessDay(time.Now()))
	}
	toSnap, err := s.snapRepo.GetByDate(ctx, toDate)
	if err != nil {
		return nil, fmt.Errorf("目标日期 %s 无快照: %w", toDate, err)
	}
	var fromSnap *model.GeoCitationSnapshot
	if strings.TrimSpace(fromDate) != "" {
		fromSnap, err = s.snapRepo.GetByDate(ctx, fromDate)
		if err != nil {
			return nil, fmt.Errorf("起始日期 %s 无快照: %w", fromDate, err)
		}
	} else {
		// 取 to 之前最近的一张（升序列表里找 < to 的最后一张）
		list, lerr := s.snapRepo.ListSince(ctx, "2000-01-01", 366)
		if lerr != nil {
			return nil, lerr
		}
		for i := len(list) - 1; i >= 0; i-- {
			if list[i].SnapshotDate < toSnap.SnapshotDate {
				fromSnap = list[i]
				break
			}
		}
	}
	if fromSnap == nil {
		return nil, fmt.Errorf("没有早于 %s 的快照可比对（快照任务至少要跑两个业务日）", toSnap.SnapshotDate)
	}

	fromCounts, err := decodeURLCounts(fromSnap.URLCounts)
	if err != nil {
		return nil, fmt.Errorf("解析 %s 快照失败: %w", fromSnap.SnapshotDate, err)
	}
	toCounts, err := decodeURLCounts(toSnap.URLCounts)
	if err != nil {
		return nil, fmt.Errorf("解析 %s 快照失败: %w", toSnap.SnapshotDate, err)
	}

	return buildDrift(fromSnap.SnapshotDate, toSnap.SnapshotDate, fromCounts, toCounts), nil
}

// buildDrift 纯 diff（可测性：日期与计数全部入参）。
func buildDrift(fromDate, toDate string, from, to map[string]int) *CitationDriftResult {
	res := &CitationDriftResult{FromDate: fromDate, ToDate: toDate}
	// 排名：按 count 降序、URL 升序钉死并列序
	rank := func(m map[string]int) map[string]int {
		type uc struct {
			url   string
			count int
		}
		var list []uc
		for u, c := range m {
			list = append(list, uc{u, c})
		}
		sort.Slice(list, func(i, j int) bool {
			if list[i].count != list[j].count {
				return list[i].count > list[j].count
			}
			return list[i].url < list[j].url
		})
		r := map[string]int{}
		for i, e := range list {
			r[e.url] = i + 1
		}
		return r
	}
	fromRank := rank(from)
	toRank := rank(to)

	for u, tc := range to {
		fc, ok := from[u]
		switch {
		case !ok:
			res.Added = append(res.Added, CitationDrift{URL: u, Domain: normalizeHost(ExtractDomain(u)), Count: tc})
		case fc != tc:
			res.Changed = append(res.Changed, CitationChange{URL: u, Domain: normalizeHost(ExtractDomain(u)), From: fc, To: tc, Delta: tc - fc})
		default: // 计数相同：不进 changed
		}
		if ok && fromRank[u] != toRank[u] {
			// 排名变化独立于计数变化记账：别的 URL 新增/暴涨会把"计数没变"的
			// URL 挤下排名——那同样是运营要看的漂移（A10 验收点名排名变化）。
			res.RankMoves = append(res.RankMoves, CitationRankMove{URL: u, Domain: normalizeHost(ExtractDomain(u)), FromRank: fromRank[u], ToRank: toRank[u]})
		}
	}
	for u, fc := range from {
		if _, ok := to[u]; !ok {
			res.Removed = append(res.Removed, CitationDrift{URL: u, Domain: normalizeHost(ExtractDomain(u)), Count: fc})
		}
	}
	// 各段内部钉序：URL 升序（diff 结果必须可复现，两次请求同一输出）
	sortDrift := func(d []CitationDrift) {
		sort.Slice(d, func(i, j int) bool { return d[i].URL < d[j].URL })
	}
	sortDrift(res.Added)
	sortDrift(res.Removed)
	sort.Slice(res.Changed, func(i, j int) bool { return res.Changed[i].URL < res.Changed[j].URL })
	sort.Slice(res.RankMoves, func(i, j int) bool { return res.RankMoves[i].URL < res.RankMoves[j].URL })
	return res
}

func decodeURLCounts(raw string) (map[string]int, error) {
	if strings.TrimSpace(raw) == "" {
		return map[string]int{}, nil
	}
	out := map[string]int{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}
