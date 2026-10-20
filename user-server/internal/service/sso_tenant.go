package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"
	"hivemtk-user/internal/secrets"
)

// kvSSOProviders 是租户级 IdP 配置在 system_config_kv 里的键。
//
// 为什么放 KV 而不是参数中心（config_params）：这一格里有 client_secret。
// 参数中心的值列今天不落密（只有审计行走 dbencrypt），把凭证写进去等于新增一个
// 明文凭证面；而 system_config_kv + secrets 这条"整份 JSON 一把存、凭证字段逐个
// 加密封装"的形状已有先例（agent.tool_integrations）。
const kvSSOProviders = "sso.providers"

// ssoProviderNamePattern 限制 provider 名能进 URL 路径 /api/sso/login/:provider。
//
// 名称同时是 sso_identities.provider 的取值，放宽到任意字符串会把"登录方式"
// 变成可以塞斜杠/空格的字段。
var ssoProviderNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,31}$`)

// TenantSSOProvider 单个租户自配的 SSO IdP。
//
// 字段与 config.SSOProviderConfig（平台 config.yaml 那一侧）逐一对齐，
// 多出的 Name/DisplayName 只服务登录页渲染；平台侧靠 map key 表达名字，
// 租户侧是一维数组，必须自带名字。
type TenantSSOProvider struct {
	Name                  string   `json:"name"`
	DisplayName           string   `json:"display_name,omitempty"`
	Issuer                string   `json:"issuer"`
	ClientID              string   `json:"client_id"`
	ClientSecret          string   `json:"client_secret,omitempty"`
	RedirectURL           string   `json:"redirect_url,omitempty"`
	AutoProvision         bool     `json:"auto_provision,omitempty"`
	DefaultRole           string   `json:"default_role,omitempty"`
	Scopes                []string `json:"scopes,omitempty"`
	AuthorizationEndpoint string   `json:"authorization_endpoint,omitempty"`
	TokenEndpoint         string   `json:"token_endpoint,omitempty"`
	UserInfoEndpoint      string   `json:"userinfo_endpoint,omitempty"`
	JWKSURI               string   `json:"jwks_uri,omitempty"`
}

// loadTenantSSOProvidersWithRepo 读本租户的 IdP 配置；client_secret 透明解密。
//
// 返回 nil 且 err 为 nil 表示"本租户没配过"，调用方必须回落到平台 config.yaml，
// 不能当成"配了但为空"——两者对登录页的意义不同（一个是平台统一入口，一个是租户
// 自己关掉了）。
//
// 没有包级 Load/SaveTenantSSOProviders 那种"用全局句柄再包一层"的入口：读写这一格
// 的唯一消费者是 SSOService，它构造时已经拿到一把显式的 db（NewSSOService(gormDB)），
// 再走全局句柄等于同一份配置有两个出处，装配顺序错了会静默读到另一个库。
func loadTenantSSOProvidersWithRepo(ctx context.Context, repo repository.SystemConfigKVRepository) ([]TenantSSOProvider, error) {
	raw, err := repo.Get(ctx, kvSSOProviders)
	if err != nil {
		return nil, err
	}
	return decodeTenantSSOProviders(raw)
}

// decodeTenantSSOProviders 解析 KV 里的 JSON 并把凭证字段还原成明文。
//
// 密钥三态与 agent.tool_integrations 同一口径：
//   - MASTER_KEY 可用：密文解密；判定为明文的在写回时透明升级（见 saveTenantSSOProvidersWithRepo）
//   - MASTER_KEY 缺失：全部按明文处理并 WARN
//   - 解密失败：保留原值 + ERROR（存量明文行不能把登录入口打死）
func decodeTenantSSOProviders(raw string) ([]TenantSSOProvider, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var providers []TenantSSOProvider
	if err := json.Unmarshal([]byte(raw), &providers); err != nil {
		return nil, fmt.Errorf("sso: 解析 %s 失败: %w", kvSSOProviders, err)
	}
	if !secrets.Ready() {
		for i := range providers {
			if providers[i].ClientSecret != "" && !secrets.IsCiphertextFormat(providers[i].ClientSecret) {
				logger.Warnf("[SSO] MASTER_KEY 未配置，租户 IdP client_secret 以明文读取（建议配置 MASTER_KEY 启用加密存储）")
				break
			}
		}
		return providers, nil
	}
	for i := range providers {
		p := &providers[i]
		if p.ClientSecret == "" || !secrets.IsCiphertextFormat(p.ClientSecret) {
			continue
		}
		plain, derr := secrets.DecryptString(p.ClientSecret)
		if derr != nil {
			logger.Errorf("[SSO] 租户 IdP %q 的 client_secret 解密失败（密钥不匹配或密文损坏），保留原值: %v", p.Name, derr)
			continue
		}
		p.ClientSecret = plain
	}
	return providers, nil
}

// saveTenantSSOProvidersWithRepo 写入本租户的 IdP 配置；client_secret 加密后落库。
//
// 传空切片等于"本租户不再自配"——删掉这一行，读取口随即回落到平台 config.yaml。
// 读到的旧行若是明文，这次写回会顺带把它升级成密文（MASTER_KEY 可用时）。
func saveTenantSSOProvidersWithRepo(ctx context.Context, repo repository.SystemConfigKVRepository, providers []TenantSSOProvider) error {
	if len(providers) == 0 {
		_, err := repo.Upsert(ctx, kvSSOProviders, "")
		return err
	}
	if err := validateTenantSSOProviders(providers); err != nil {
		return err
	}
	store := make([]TenantSSOProvider, len(providers))
	copy(store, providers)
	if secrets.Ready() {
		for i := range store {
			if store[i].ClientSecret == "" || secrets.IsCiphertextFormat(store[i].ClientSecret) {
				continue
			}
			enc, err := secrets.EncryptString(store[i].ClientSecret)
			if err != nil {
				return fmt.Errorf("sso: 加密 client_secret 失败: %w", err)
			}
			store[i].ClientSecret = enc
		}
	} else {
		logger.Warnf("[SSO] MASTER_KEY 未配置，租户 IdP client_secret 以明文落库（建议配置 MASTER_KEY 启用加密存储）")
	}
	raw, err := json.Marshal(store)
	if err != nil {
		return fmt.Errorf("sso: 序列化租户 IdP 配置失败: %w", err)
	}
	_, err = repo.Upsert(ctx, kvSSOProviders, string(raw))
	return err
}

// validateTenantSSOProviders 写入侧静态校验。
//
// 为什么要在落库前挡住而不是读取时报错：IdP 配错的最坏后果是登录页打不开，
// 而运维是在参数页填完之后隔一段时间才发现"改完没人读"——那时审计行已经把
// 上一个可用值盖掉了。校验前置能把错值挡在写请求里，连同原因一起返回。
func validateTenantSSOProviders(providers []TenantSSOProvider) error {
	seen := map[string]bool{}
	for i, p := range providers {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			return fmt.Errorf("sso: 第 %d 个 IdP 缺 name", i+1)
		}
		if !ssoProviderNamePattern.MatchString(name) {
			return fmt.Errorf("sso: IdP %q 的 name 不合法（只允许字母数字开头、长度 ≤32、可含 - 与 _）", name)
		}
		if seen[name] {
			return fmt.Errorf("sso: IdP name %q 重复", name)
		}
		seen[name] = true
		if strings.TrimSpace(p.ClientID) == "" {
			return fmt.Errorf("sso: IdP %q 缺 client_id（没有它连授权请求都发不出去）", name)
		}
		if err := validateSSOEndpoint(name, "issuer", p.Issuer, true); err != nil {
			return err
		}
		for _, ep := range []struct{ field, value string }{
			{"redirect_url", p.RedirectURL},
			{"authorization_endpoint", p.AuthorizationEndpoint},
			{"token_endpoint", p.TokenEndpoint},
			{"userinfo_endpoint", p.UserInfoEndpoint},
			{"jwks_uri", p.JWKSURI},
		} {
			if err := validateSSOEndpoint(name, ep.field, ep.value, false); err != nil {
				return err
			}
		}
		if p.DefaultRole != "" && !model.IsValidSystemUserRole(p.DefaultRole) {
			return fmt.Errorf("sso: IdP %q 的 default_role %q 不是合法角色", name, p.DefaultRole)
		}
	}
	return nil
}

// validateSSOEndpoint 校验一个 IdP 端点。required=true 时空值即错。
//
// 只放行 https，但放开 http://localhost 与 127.0.0.1：开发态用 Keycloak 容器
// 跑 http 是常态，把它钉死成 https 会让本地联调只能去配自签证书。
func validateSSOEndpoint(name, field, value string, required bool) error {
	v := strings.TrimSpace(value)
	if v == "" {
		if required {
			return fmt.Errorf("sso: IdP %q 缺 %s", name, field)
		}
		return nil
	}
	u, err := url.Parse(v)
	if err != nil {
		return fmt.Errorf("sso: IdP %q 的 %s 解析失败: %w", name, field, err)
	}
	if u.Host == "" {
		return fmt.Errorf("sso: IdP %q 的 %s 缺 host（当前=%q）", name, field, v)
	}
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme == "http" && isLoopbackHost(u.Hostname()) {
		return nil
	}
	return fmt.Errorf("sso: IdP %q 的 %s 必须是 https（本机回环可用 http，当前=%q）", name, field, v)
}

func isLoopbackHost(host string) bool {
	h := strings.ToLower(strings.Trim(strings.TrimSpace(host), "[]"))
	return h == "localhost" || h == "127.0.0.1" || h == "::1"
}

// PreserveMaskedClientSecrets 把 GET 回显里的 "***" 还原成库里真实存过的那份密钥。
//
// 为什么必须在前端与落库之间插这一刀：回显口只能给 "***"（否则 secret 进浏览器历史），
// 而保存口收到的是同一份表单——照原样写库就等于把 "***" 当成 client_secret 存下去，
// 表现是授权能跳到 IdP、换 token 时对方回 invalid_client，运维从日志里看不出是配置被自己盖掉了。
//
// 三档语义：
//   - "***"：沿用库里同名的旧值（改名后的 IdP 查不到旧值，直接报错而不是悄悄存字面量）
//   - ""：清空（前端"清除密钥"输入框留空即为显式清除）
//   - 其他：按提交值写入（随后由 SSOService.SaveTenantProviders 加密落库）
func PreserveMaskedClientSecrets(submitted, existing []TenantSSOProvider) ([]TenantSSOProvider, error) {
	if len(submitted) == 0 {
		return submitted, nil
	}
	old := make(map[string]string, len(existing))
	for _, p := range existing {
		old[p.Name] = p.ClientSecret
	}
	out := make([]TenantSSOProvider, len(submitted))
	copy(out, submitted)
	for i := range out {
		if out[i].ClientSecret != "***" {
			continue
		}
		prev, ok := old[strings.TrimSpace(out[i].Name)]
		if !ok || prev == "" {
			return nil, fmt.Errorf("sso: IdP %q 提交了掩码值但没有可沿用的旧密钥，请填入真实 client_secret", out[i].Name)
		}
		out[i].ClientSecret = prev
	}
	return out, nil
}

// MaskTenantSSOClientSecrets 返回把 client_secret 抹掉的副本，供 GET 回显。
//
// 回显不能带原文：参数页一旦把 secret 渲染进 input，它就进了浏览器历史与前端日志，
// 而这两个地方不在这次改动的可控范围内。留一个"已配置"的标记位，前端才能区分
// "没填"与"填过但看不到"。
func MaskTenantSSOClientSecrets(providers []TenantSSOProvider) []TenantSSOProvider {
	out := make([]TenantSSOProvider, len(providers))
	copy(out, providers)
	for i := range out {
		if out[i].ClientSecret != "" {
			out[i].ClientSecret = "***"
		}
	}
	return out
}
