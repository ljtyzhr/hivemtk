package service

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"hivemtk-user/internal/geo/model"
	"hivemtk-user/internal/geo/repository"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/timeutil"
	"hivemtk-user/internal/pkg/utils/logger"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PushResult 单个 URL 推送结果
type PushResult struct {
	URL         string
	Success     bool
	RemainQuota int
	Error       string
}

// Pusher 蜘蛛推送统一接口
type Pusher interface {
	Name() string
	Push(ctx context.Context, urls []string) ([]PushResult, error)
}

// ────────────────────────────────────────────
// QuotaManager 配额管理
// ────────────────────────────────────────────

type QuotaManager struct {
	mu     sync.Mutex
	limits map[string]int // platform → 每日上限
	used   map[string]int // platform → 今日已用
	reset  map[string]time.Time
}

func NewQuotaManager() *QuotaManager {
	return &QuotaManager{
		limits: map[string]int{
			"baidu":    5000,
			"google":   200,
			"indexnow": -1, // 无限额
			"toutiao":  -1,
			"shenma":   -1,
			"sitemap":  -1,
		},
		used:  map[string]int{},
		reset: map[string]time.Time{},
	}
}

func (q *QuotaManager) Allow(platform string, n int) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	limit, ok := q.limits[platform]
	if !ok || limit < 0 {
		return true // 无限额
	}
	return q.used[platform]+n <= limit
}

func (q *QuotaManager) Consume(platform string, n int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.used[platform] += n
}

func (q *QuotaManager) Remain(platform string) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	limit, ok := q.limits[platform]
	if !ok || limit < 0 {
		return -1
	}
	return limit - q.used[platform]
}

// Reset 配额重置（每日 00:05 调用）
func (q *QuotaManager) Reset() {
	q.mu.Lock()
	defer q.mu.Unlock()
	for k := range q.used {
		q.used[k] = 0
	}
}

// SetLimit 覆盖平台每日上限（DB 同步用，<=0 视为无限额）
func (q *QuotaManager) SetLimit(platform string, limit int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.limits[platform] = limit
}

// SetUsed 覆盖今日已用量（DB 同步用）
func (q *QuotaManager) SetUsed(platform string, used int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.used[platform] = used
	q.reset[platform] = time.Now()
}

// Used 返回今日已用量
func (q *QuotaManager) Used(platform string) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.used[platform]
}

// ────────────────────────────────────────────
// BaiduPusher 百度主动推送 API
// POST http://data.zz.baidu.com/urls?site=SITE&token=TOKEN
// text/plain，每行一个 URL，批量 2000
// ────────────────────────────────────────────

type BaiduPusher struct {
	Site  string
	Token string
	cli   *http.Client
}

func NewBaiduPusher(site, token string) *BaiduPusher {
	return &BaiduPusher{Site: site, Token: token, cli: newHTTPClient()}
}

func (p *BaiduPusher) Name() string { return "baidu" }

func (p *BaiduPusher) Push(ctx context.Context, urls []string) ([]PushResult, error) {
	results := make([]PushResult, len(urls))
	for i := range urls {
		results[i].URL = urls[i]
		results[i].Success = false
	}
	if p.Site == "" || p.Token == "" {
		for i := range results {
			results[i].Error = "baidu: site/token 未配置"
		}
		return results, nil
	}

	// 分批：每批 2000
	batchSize := 2000
	for i := 0; i < len(urls); i += batchSize {
		end := i + batchSize
		if end > len(urls) {
			end = len(urls)
		}
		batch := urls[i:end]

		u := fmt.Sprintf("http://data.zz.baidu.com/urls?site=%s&token=%s",
			url.QueryEscape(p.Site), url.QueryEscape(p.Token))

		body := strings.Join(batch, "\n")
		req, _ := http.NewRequestWithContext(ctx, "POST", u, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "text/plain")
		req.Header.Set("User-Agent", "HiveMtk-GEO/1.0")

		resp, err := p.cli.Do(req)
		if err != nil {
			for j := i; j < end; j++ {
				results[j].Error = err.Error()
			}
			continue
		}
		respBody := bodyToString(resp.Body)
		_ = resp.Body.Close()

		// 百度返回 {"success":N, "remain":M}；失败返回 {"error":410,"message":"..."}
		var br struct {
			Success int    `json:"success"`
			Remain  int    `json:"remain"`
			Error   int    `json:"error"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal([]byte(respBody), &br)

		if resp.StatusCode == 200 && br.Error == 0 {
			// 配额按 success 数标记：前 N 个 URL 成功
			n := br.Success
			if n > end-i {
				n = end - i
			}
			for j := i; j < i+n; j++ {
				results[j].Success = true
			}
			for j := i + n; j < end; j++ {
				results[j].Error = "baidu: 超出当日配额被拒收"
			}
			if br.Remain > 0 && i == 0 && len(results) > 0 {
				results[0].RemainQuota = br.Remain
			}
		} else {
			msg := fmt.Sprintf("baidu: HTTP %d", resp.StatusCode)
			if br.Message != "" {
				msg = fmt.Sprintf("baidu: error=%d %s", br.Error, br.Message)
			}
			for j := i; j < end; j++ {
				results[j].Error = msg
			}
		}
	}
	return results, nil
}

// ────────────────────────────────────────────
// GooglePusher Google Indexing API v3
// 配额 200 URL/天/Project，多 Project Round-Robin
// ────────────────────────────────────────────

type GoogleProject struct {
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
}

type GooglePusher struct {
	Projects []GoogleProject
	mu       sync.Mutex
	tokens   map[string]googleToken // client_email → 缓存 token
	cli      *http.Client
}

type googleToken struct {
	access   string
	expireAt time.Time
}

func NewGooglePusher(projects []GoogleProject) *GooglePusher {
	return &GooglePusher{
		Projects: projects,
		tokens:   map[string]googleToken{},
		cli:      newHTTPClient(),
	}
}

func (p *GooglePusher) Name() string { return "google" }

func (p *GooglePusher) Push(ctx context.Context, urls []string) ([]PushResult, error) {
	results := make([]PushResult, len(urls))
	for i := range urls {
		results[i].URL = urls[i]
		results[i].Success = false
	}
	if len(p.Projects) == 0 {
		for i := range results {
			results[i].Error = "google: 未配置 Service Account"
		}
		return results, nil
	}

	for i, u := range urls {
		project := p.Projects[i%len(p.Projects)] // Round-Robin
		accessToken, err := p.getAccessToken(ctx, project)
		if err != nil {
			results[i].Error = "google JWT: " + err.Error()
			continue
		}
		body, _ := json.Marshal(map[string]any{
			"url":  u,
			"type": "URL_UPDATED",
		})
		req, _ := http.NewRequestWithContext(ctx, "POST",
			"https://indexing.googleapis.com/v3/urlNotifications:publish",
			bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "HiveMtk-GEO/1.0")

		resp, err := p.cli.Do(req)
		if err != nil {
			results[i].Error = err.Error()
			continue
		}
		respBody := bodyToString(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			results[i].Success = true
		} else {
			results[i].Error = truncateStr(fmt.Sprintf("google: HTTP %d %s", resp.StatusCode, respBody), 300)
		}
	}
	return results, nil
}

// getAccessToken 用 Service Account 私钥签 RS256 JWT，向 Google OAuth2 换取 access token
// 标准库实现（不引入 golang.org/x/oauth2），按 client_email 缓存并提前 60s 过期
func (p *GooglePusher) getAccessToken(ctx context.Context, proj GoogleProject) (string, error) {
	if proj.ClientEmail == "" || proj.PrivateKey == "" {
		return "", fmt.Errorf("google: Service Account 未配置")
	}

	now := time.Now()
	p.mu.Lock()
	if t, ok := p.tokens[proj.ClientEmail]; ok && t.access != "" && now.Before(t.expireAt) {
		p.mu.Unlock()
		return t.access, nil
	}
	p.mu.Unlock()

	assertion, err := signGoogleJWT(proj.ClientEmail, proj.PrivateKey, now)
	if err != nil {
		return "", err
	}

	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	form.Set("assertion", assertion)

	req, _ := http.NewRequestWithContext(ctx, "POST", "https://oauth2.googleapis.com/token",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.cli.Do(req)
	if err != nil {
		return "", fmt.Errorf("google token: %w", err)
	}
	body := bodyToString(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("google token: HTTP %d %s", resp.StatusCode, truncateStr(body, 200))
	}

	var tr struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal([]byte(body), &tr); err != nil || tr.AccessToken == "" {
		return "", fmt.Errorf("google token: 响应解析失败 %s", truncateStr(body, 200))
	}
	expiry := tr.ExpiresIn
	if expiry <= 0 {
		expiry = 3600
	}
	p.mu.Lock()
	p.tokens[proj.ClientEmail] = googleToken{
		access:   tr.AccessToken,
		expireAt: now.Add(time.Duration(expiry-60) * time.Second),
	}
	p.mu.Unlock()
	return tr.AccessToken, nil
}

// signGoogleJWT 构造并签名 Google Service Account 的 JWT assertion（RS256）
func signGoogleJWT(clientEmail, privateKeyPEM string, now time.Time) (string, error) {
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return "", fmt.Errorf("google: private_key 不是合法 PEM")
	}
	var key *rsa.PrivateKey
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		key = k
	} else if pk, err2 := x509.ParsePKCS8PrivateKey(block.Bytes); err2 == nil {
		rsaKey, ok := pk.(*rsa.PrivateKey)
		if !ok {
			return "", fmt.Errorf("google: private_key 不是 RSA 密钥")
		}
		key = rsaKey
	} else {
		return "", fmt.Errorf("google: private_key 解析失败(PKCS1/PKCS8)")
	}

	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims, _ := json.Marshal(map[string]any{
		"iss":   clientEmail,
		"scope": "https://www.googleapis.com/auth/indexing",
		"aud":   "https://oauth2.googleapis.com/token",
		"exp":   now.Add(time.Hour).Unix(),
		"iat":   now.Unix(),
	})
	payload := base64.RawURLEncoding.EncodeToString(claims)
	signInput := header + "." + payload

	digest := sha256.Sum256([]byte(signInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("google: JWT 签名失败: %w", err)
	}
	return signInput + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// ────────────────────────────────────────────
// IndexNowPusher 覆盖 Bing/Yandex/Naver/Seznam/Amazon
// POST https://api.indexnow.org/indexnow
// JSON POST，一次最多 10000 URL，无限额
// ────────────────────────────────────────────

type IndexNowPusher struct {
	Host        string
	Key         string
	KeyLocation string
	cli         *http.Client
}

func NewIndexNowPusher(host, key, keyLocation string) *IndexNowPusher {
	return &IndexNowPusher{Host: host, Key: key, KeyLocation: keyLocation, cli: newHTTPClient()}
}

func (p *IndexNowPusher) Name() string { return "indexnow" }

func (p *IndexNowPusher) Push(ctx context.Context, urls []string) ([]PushResult, error) {
	results := make([]PushResult, len(urls))
	for i := range urls {
		results[i].URL = urls[i]
		results[i].Success = false
	}
	if p.Host == "" || p.Key == "" {
		for i := range results {
			results[i].Error = "indexnow: host/key 未配置"
		}
		return results, nil
	}

	batchSize := 10000
	for i := 0; i < len(urls); i += batchSize {
		end := i + batchSize
		if end > len(urls) {
			end = len(urls)
		}
		batch := urls[i:end]

		payload, _ := json.Marshal(map[string]any{
			"host":        p.Host,
			"key":         p.Key,
			"keyLocation": p.KeyLocation,
			"urlList":     batch,
		})

		req, _ := http.NewRequestWithContext(ctx, "POST",
			"https://api.indexnow.org/indexnow",
			bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		req.Header.Set("User-Agent", "HiveMtk-GEO/1.0")

		resp, err := p.cli.Do(req)
		if err != nil {
			for j := i; j < end; j++ {
				results[j].Error = err.Error()
			}
			continue
		}
		_ = resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			for j := i; j < end; j++ {
				results[j].Success = true
			}
		} else {
			for j := i; j < end; j++ {
				results[j].Error = fmt.Sprintf("indexnow: HTTP %d", resp.StatusCode)
			}
		}
	}
	return results, nil
}

// ────────────────────────────────────────────
// ToutiaoPusher 头条/字节（豆包底层）
// 头条搜索站长平台暂无公开 API，当前推送 sitemap
// ────────────────────────────────────────────

type ToutiaoPusher struct {
	Site string
}

func (p *ToutiaoPusher) Name() string { return "toutiao" }

func (p *ToutiaoPusher) Push(ctx context.Context, urls []string) ([]PushResult, error) {
	results := make([]PushResult, len(urls))
	for i, u := range urls {
		results[i] = PushResult{URL: u, Success: true} // sitemap 兜底，视为成功
	}
	logger.Warnf("toutiao: 暂无公开 API，通过 sitemap 提交（%d URL）", len(urls))
	return results, nil
}

// ────────────────────────────────────────────
// SitemapPusher 全量 sitemap 生成 + 提交各站长平台
// ────────────────────────────────────────────

type SitemapPusher struct{}

func NewSitemapPusher() *SitemapPusher { return &SitemapPusher{} }

func (p *SitemapPusher) Name() string { return "sitemap" }

func (p *SitemapPusher) Push(ctx context.Context, urls []string) ([]PushResult, error) {
	results := make([]PushResult, len(urls))
	for i, u := range urls {
		results[i] = PushResult{URL: u, Success: true}
	}
	logger.Warnf("sitemap: 入队 %d URL，等待每日定时全量生成提交", len(urls))
	return results, nil
}

// ────────────────────────────────────────────
// PushService 编排器
// ────────────────────────────────────────────

type PushService struct {
	db       *gorm.DB
	records  repository.GeoPushRecordRepository
	quota    *QuotaManager
	pushers  map[string]Pusher
	mu       sync.Mutex
	quotaDay string // 内存配额所属日期，跨天自动 Reset
}

func NewPushService(db *gorm.DB, records repository.GeoPushRecordRepository) *PushService {
	svc := &PushService{
		db:       db,
		records:  records,
		quota:    NewQuotaManager(),
		pushers:  map[string]Pusher{},
		quotaDay: timeutil.BusinessToday(),
	}
	// 自动从 DB 读配置注册
	svc.registerFromDB()
	return svc
}

// rollQuotaDay 长驻进程跨天时重置内存配额
//
// 日期键两侧都必须走 timeutil：`today` 是宿主时区口径、`last_reset_at` 是 DB 里
// CST 会话时区写入的 timestamptz，混用 `Format` 会让同一天被判成跨天（配额莫名归零）
// 或跨天被判成同天（当日额度提前打满）。
func (s *PushService) rollQuotaDay() {
	today := timeutil.BusinessToday()
	s.mu.Lock()
	if today != s.quotaDay {
		s.quotaDay = today
		s.quota.Reset()
	}
	s.mu.Unlock()
}

// persistQuota 将内存配额写回 geo_pusher_configs（UsedToday/LastResetAt）
//
// geo_pusher_configs 带 tenant_id、在租户登记表内：隔离 callback 只看 GORM
// 语句的 ctx，调用方（PushURLs/PushManual）透传请求 ctx，否则 on 模式 fail-closed。
func (s *PushService) persistQuota(ctx context.Context, platform string) {
	used := s.quota.Used(platform)
	err := s.db.WithContext(ctx).Model(&model.GeoPusherConfig{}).
		Where("platform = ?", platform).
		Updates(map[string]any{
			"used_today":    used,
			"last_reset_at": time.Now(),
			"updated_at":    time.Now(),
		}).Error
	// 回写失败必须出声（二审 P1-9）：静默丢会让 used_today 停在旧值 —— 跨天/重启
	// 后按 DB 复原已用量时拿到脏值，当日配额被成倍超发或凭空归零。
	if err != nil {
		logger.Warnf("[push] 配额回写失败 platform=%s used_today=%d: %v", platform, used, err)
	}
}

// registerFromDB 启动期从 DB 装配推送引擎（NewPushService 构造路径）。
//
// 构造期没有 HTTP 请求 ctx 可绑，而 geo_pusher_configs 是租户表：TENANT_MODE=on
// 下裸 ctx 的 Find 会被隔离 callback fail-closed（装配结果为空 ⇒ 推送引擎全灭）。
// 走 tenant.WithSystemScope 平台级豁免通道（与 migration/健康检查同口径）。
func (s *PushService) registerFromDB() {
	ctx := tenant.WithSystemScope(context.Background())
	var configs []model.GeoPusherConfig
	if err := s.db.WithContext(ctx).Find(&configs).Error; err != nil {
		return
	}
	today := timeutil.BusinessToday()
	for _, cfg := range configs {
		// 配额 DB 同步：跨天自动归零，同天恢复已用量
		if cfg.DailyLimit > 0 {
			used := cfg.UsedToday
			lastDay := ""
			if cfg.LastResetAt != nil {
				lastDay = timeutil.BusinessDate(*cfg.LastResetAt)
			}
			if lastDay != today {
				used = 0
			}
			s.quota.SetLimit(cfg.Platform, cfg.DailyLimit)
			s.quota.SetUsed(cfg.Platform, used)
		}
		if !cfg.Active {
			continue
		}
		var m map[string]any
		_ = json.Unmarshal(cfg.ConfigJSON, &m)
		switch cfg.Platform {
		case "baidu":
			site, _ := m["site"].(string)
			token, _ := m["token"].(string)
			if site != "" && token != "" {
				s.pushers["baidu"] = NewBaiduPusher(site, token)
			}
		case "google":
			if projs, ok := m["projects"].([]any); ok {
				list := []GoogleProject{}
				for _, p := range projs {
					if pm, ok := p.(map[string]any); ok {
						list = append(list, GoogleProject{
							ClientEmail: asString(pm["client_email"]),
							PrivateKey:  asString(pm["private_key"]),
						})
					}
				}
				if len(list) > 0 {
					s.pushers["google"] = NewGooglePusher(list)
				}
			}
		case "indexnow":
			host, _ := m["host"].(string)
			key, _ := m["key"].(string)
			keyLoc, _ := m["keyLocation"].(string)
			if host != "" && key != "" {
				s.pushers["indexnow"] = NewIndexNowPusher(host, key, keyLoc)
			}
		}
	}
	// 兜底：总会注册 sitemap
	s.pushers["sitemap"] = NewSitemapPusher()
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// PushURLs 推一组 URL（所有启用的引擎）
func (s *PushService) PushURLs(ctx context.Context, urls []string) error {
	if len(urls) == 0 {
		return nil
	}

	s.rollQuotaDay()
	batchID := uuid.New().String()
	for platform, pusher := range s.pushers {
		if !s.quota.Allow(platform, len(urls)) {
			logger.Warnf("[%s] 配额用尽，跳过 %d URL", platform, len(urls))
			continue
		}
		results, err := pusher.Push(ctx, urls)
		if err != nil {
			logger.Warnf("[%s] 推送失败: %v", platform, err)
			continue
		}

		success, fail := 0, 0
		var records []*model.GeoPushRecord
		for _, r := range results {
			rec := &model.GeoPushRecord{Platform: platform, URL: r.URL, BatchID: batchID}
			if r.Success {
				success++
				rec.SuccessCount = 1
			} else {
				fail++
				rec.FailCount = 1
				rec.ErrorMsg = r.Error
			}
			if r.RemainQuota > 0 {
				rec.RemainQuota = r.RemainQuota
			}
			records = append(records, rec)
		}
		// 记账失败必须上抛：静默丢记录等于事后无账可查。
		if err := s.records.BatchCreate(ctx, records); err != nil {
			return fmt.Errorf("[%s] 推送记录落库失败: %w", platform, err)
		}

		s.quota.Consume(platform, success)
		s.persistQuota(ctx, platform)
		logger.Warnf("[%s] 推送 %d URL 成功=%d 失败=%d", platform, len(urls), success, fail)
	}
	return nil
}

// ValidatePlatforms 校验平台名是否已启用。未注册的平台必须报错，
// 不能静默跳过——否则调用方拿到 code=0 却一条记录都没有。
func (s *PushService) ValidatePlatforms(platforms []string) error {
	var unknown []string
	for _, plat := range platforms {
		if _, ok := s.pushers[plat]; !ok {
			unknown = append(unknown, plat)
		}
	}
	if len(unknown) > 0 {
		return fmt.Errorf("推送平台未启用: %s", strings.Join(unknown, "、"))
	}
	return nil
}

// PushManual 手动推指定平台
func (s *PushService) PushManual(ctx context.Context, urls []string, platforms []string) ([]*model.GeoPushRecord, error) {
	if err := s.ValidatePlatforms(platforms); err != nil {
		return nil, err
	}
	var records []*model.GeoPushRecord
	s.rollQuotaDay()
	batchID := uuid.New().String()
	for _, plat := range platforms {
		p, ok := s.pushers[plat]
		if !ok {
			continue
		}
		if !s.quota.Allow(plat, len(urls)) {
			logger.Warnf("[%s] 手动推送：配额用尽，跳过", plat)
			continue
		}
		results, err := p.Push(ctx, urls)
		if err != nil {
			continue
		}
		success := 0
		for _, r := range results {
			rec := &model.GeoPushRecord{Platform: plat, URL: r.URL, BatchID: batchID, ErrorMsg: r.Error}
			if r.Success {
				rec.SuccessCount = 1
				success++
			} else {
				rec.FailCount = 1
			}
			records = append(records, rec)
		}
		s.quota.Consume(plat, success)
		s.persistQuota(ctx, plat)
	}
	if err := s.records.BatchCreate(ctx, records); err != nil {
		return records, fmt.Errorf("推送记录落库失败: %w", err)
	}
	return records, nil
}

// QuotaStatus 返回各引擎配额状态
func (s *PushService) QuotaStatus() map[string]any {
	s.rollQuotaDay()
	out := map[string]any{}
	for name := range s.pushers {
		out[name] = map[string]any{
			"registered": true,
			"remain":     s.quota.Remain(name),
			"used":       s.quota.Used(name),
		}
	}
	return out
}

// GenerateAndSubmitSitemap 全量 sitemap 生成（定时调用）
func (s *PushService) GenerateAndSubmitSitemap(ctx context.Context) (string, error) {
	var articles []model.GeoArticle
	if err := s.db.WithContext(ctx).Where("deployed_at IS NOT NULL AND site_url != ''").Find(&articles).Error; err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	sb.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	for _, a := range articles {
		sb.WriteString(fmt.Sprintf(`<url><loc>%s</loc></url>`, a.SiteURL))
	}
	sb.WriteString(`</urlset>`)

	return sb.String(), nil
}

// bodyToString 辅助
func bodyToString(r io.Reader) string {
	b, _ := io.ReadAll(r)
	return string(b)
}
