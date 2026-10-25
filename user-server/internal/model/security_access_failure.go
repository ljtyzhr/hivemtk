// security_access_failure.go 未授权访问尝试的部署级聚合载荷。
//
// 存储载体是 platform_config_kv（部署级 KV，无租户维度）：未授权访问的"归属人"
// 是攻击者而不是任何租户，落租户表要么串库要么 fail-closed 整批拒写（R-8 同一学费）。
// 遥测按键 `security_access_failures_v1` 聚合，写入方是 middleware/security_recorder.go
// （JWT 401 五类），读取方是 service/security_overview.go（安全总览端点）。
package model

import (
	"encoding/json"
	"sort"
)

// SecurityAccessFailureKVKey platform_config_kv 里的聚合键。
const SecurityAccessFailureKVKey = "security_access_failures_v1"

// JWT 401 的五类失败形态（与 middleware/jwt.go 的五个 abort 点一一对应）。
const (
	SecurityAccessKindTokenMissing  = "token_missing"
	SecurityAccessKindTokenFormat   = "token_format"
	SecurityAccessKindTokenInvalid  = "token_invalid"
	SecurityAccessKindTokenExpired  = "token_expired"
	SecurityAccessKindStatusChanged = "status_changed"
)

// SecurityAccessFailureDay 一天的失败聚合计数。
type SecurityAccessFailureDay struct {
	Kinds map[string]int64 `json:"kinds"` // 失败形态 → 次数
	Paths map[string]int64 `json:"paths"` // 路径前缀（两段）→ 次数
}

// SecurityAccessFailurePayload kv 里存的整包：日期 → 当日聚合。只保留最近 7 天。
type SecurityAccessFailurePayload struct {
	Days map[string]*SecurityAccessFailureDay `json:"days"`
}

// NewSecurityAccessFailurePayload 空载荷。
func NewSecurityAccessFailurePayload() *SecurityAccessFailurePayload {
	return &SecurityAccessFailurePayload{Days: map[string]*SecurityAccessFailureDay{}}
}

// ParseSecurityAccessFailurePayload 宽松解析：空串/坏 JSON 回空载荷（遥测读侧
// 不因历史脏数据 500），只有调用方自己区分"没数据"与"坏了"。
func ParseSecurityAccessFailurePayload(raw string) *SecurityAccessFailurePayload {
	p := NewSecurityAccessFailurePayload()
	if raw == "" {
		return p
	}
	_ = json.Unmarshal([]byte(raw), p)
	if p.Days == nil {
		p.Days = map[string]*SecurityAccessFailureDay{}
	}
	return p
}

// Add 计入一次失败（date=YYYY-MM-DD，pathPrefix 已截好的两段前缀）。
func (p *SecurityAccessFailurePayload) Add(date, kind, pathPrefix string) {
	day := p.Days[date]
	if day == nil {
		day = &SecurityAccessFailureDay{Kinds: map[string]int64{}, Paths: map[string]int64{}}
		p.Days[date] = day
	}
	if kind != "" {
		day.Kinds[kind]++
	}
	if pathPrefix != "" {
		day.Paths[pathPrefix]++
	}
}

// Merge 把另一份载荷（通常是本进程待刷增量）并进当前载荷。
func (p *SecurityAccessFailurePayload) Merge(other *SecurityAccessFailurePayload) {
	if other == nil {
		return
	}
	for date, day := range other.Days {
		for kind, n := range day.Kinds {
			for i := int64(0); i < n; i++ {
				p.Add(date, kind, "")
			}
		}
		for path, n := range day.Paths {
			for i := int64(0); i < n; i++ {
				p.Add(date, "", path)
			}
		}
	}
}

// Prune 只保留最近 keepDays 天（按日期字符串倒序保留；日期串即字典序）。
func (p *SecurityAccessFailurePayload) Prune(keepDays int) {
	if keepDays <= 0 || len(p.Days) <= keepDays {
		return
	}
	dates := make([]string, 0, len(p.Days))
	for d := range p.Days {
		dates = append(dates, d)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(dates)))
	for _, d := range dates[keepDays:] {
		delete(p.Days, d)
	}
}

// TopPaths 返回按次数降序的前 n 个路径前缀。
func (p *SecurityAccessFailurePayload) TopPaths(n int) []string {
	type kv struct {
		k string
		v int64
	}
	all := map[string]int64{}
	for _, day := range p.Days {
		for path, cnt := range day.Paths {
			all[path] += cnt
		}
	}
	list := make([]kv, 0, len(all))
	for k, v := range all {
		list = append(list, kv{k, v})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].v != list[j].v {
			return list[i].v > list[j].v
		}
		return list[i].k < list[j].k
	})
	out := make([]string, 0, n)
	for i, item := range list {
		if i >= n {
			break
		}
		out = append(out, item.k)
	}
	return out
}
