package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"hivemtk-user/internal/config"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/repository"
	"hivemtk-user/internal/secrets"

	"gorm.io/gorm"
)

// sso_tenant_test.go 覆盖 R5-4：租户自配 IdP 的存取、校验、加密与"按租户取适配器"。
//
// 这一族用例刻意不走真库读 KV（除最后一格）：要验的是"哪个租户拿到哪套 IdP"，
// 用替身可以把租户归属写成断言，而不是靠两张表里的行恰好没串。

const ssoTestMasterKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// tenantKV 按 (租户, 键) 分格的 KV 替身。
//
// 与 ltc_config_test.go 里的 fakeKV 不是一回事：那张表只按 key 索引，证不了隔离。
// 这里 on 模式下没有租户绑定必须报错（与生产仓储同一 fail-closed 口径），
// 否则"漏绑中间件"在测试里会表现成"读到了默认租户那一格"。
type tenantKV struct {
	rows  map[string]string // key: "<tenantID>/<key>"
	calls int
}

func newTenantKV() *tenantKV { return &tenantKV{rows: map[string]string{}} }

func (k *tenantKV) Available() bool { return true }

func (k *tenantKV) slot(ctx context.Context, key string) (string, error) {
	tid, ok := tenant.FromContext(ctx)
	if !ok {
		if !tenant.IsMultiTenant() {
			tid = tenant.DefaultTenantID
		} else {
			return "", repository.ErrKVTenantContextMissing
		}
	}
	return kvSlot(tid, key), nil
}

func kvSlot(tid uint, key string) string { return fmt.Sprintf("%d/%s", tid, key) }

func (k *tenantKV) Get(ctx context.Context, key string) (string, error) {
	k.calls++
	slot, err := k.slot(ctx, key)
	if err != nil {
		return "", err
	}
	return k.rows[slot], nil
}

func (k *tenantKV) Upsert(ctx context.Context, key, value string) (string, error) {
	slot, err := k.slot(ctx, key)
	if err != nil {
		return "", err
	}
	k.rows[slot] = value
	return value, nil
}

func (k *tenantKV) EnsureTable(ctx context.Context) error { return nil }

func (k *tenantKV) raw(tenantID uint) string { return k.rows[kvSlot(tenantID, kvSSOProviders)] }

var _ repository.SystemConfigKVRepository = (*tenantKV)(nil)

// ---- 校验 ------------------------------------------------------------------

func TestValidateTenantSSOProviders(t *testing.T) {
	valid := TenantSSOProvider{
		Name:     "corp-idp",
		Issuer:   "https://idp.example.com",
		ClientID: "cid",
	}
	tests := []struct {
		name      string
		providers []TenantSSOProvider
		wantErr   string
	}{
		{"合法一条", []TenantSSOProvider{valid}, ""},
		{"回环 http 放行", []TenantSSOProvider{{Name: "local", Issuer: "http://127.0.0.1:8080", ClientID: "cid"}}, ""},
		{"缺 name", []TenantSSOProvider{{Issuer: "https://x.example.com", ClientID: "cid"}}, "缺 name"},
		{"name 非法字符", []TenantSSOProvider{{Name: "bad name", Issuer: "https://x.example.com", ClientID: "cid"}}, "不合法"},
		{"name 重复", []TenantSSOProvider{valid, valid}, "重复"},
		{"缺 client_id", []TenantSSOProvider{{Name: "a", Issuer: "https://x.example.com"}}, "缺 client_id"},
		{"缺 issuer", []TenantSSOProvider{{Name: "a", ClientID: "cid"}}, "缺 issuer"},
		{"明文 http 外网端点", []TenantSSOProvider{{Name: "a", Issuer: "https://x.example.com", ClientID: "cid", TokenEndpoint: "http://idp.example.com/token"}}, "必须是 https"},
		{"非法角色", []TenantSSOProvider{{Name: "a", Issuer: "https://x.example.com", ClientID: "cid", DefaultRole: "superuser2"}}, "不是合法角色"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateTenantSSOProviders(tc.providers)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("want nil, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("want error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q 不含 %q", err.Error(), tc.wantErr)
			}
		})
	}
}

// ---- 加密封装 --------------------------------------------------------------

func TestTenantSSOSecret_EncryptedWhenMasterKeyReady(t *testing.T) {
	secrets.ResetForTest()
	defer secrets.ResetForTest()
	t.Setenv("MASTER_KEY", ssoTestMasterKey)
	if err := secrets.InitFromEnv(); err != nil {
		t.Fatalf("secrets.InitFromEnv: %v", err)
	}

	kv := newTenantKV()
	ctx := context.Background() // off 模式：落默认租户那一格
	providers := []TenantSSOProvider{
		{Name: "corp", Issuer: "https://idp.example.com", ClientID: "cid", ClientSecret: "s3cr3t-plain"},
	}
	if err := saveTenantSSOProvidersWithRepo(ctx, kv, providers); err != nil {
		t.Fatalf("save: %v", err)
	}

	raw := kv.raw(tenant.DefaultTenantID)
	if strings.Contains(raw, "s3cr3t-plain") {
		t.Fatalf("库里出现明文密钥: %s", raw)
	}
	var stored []TenantSSOProvider
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		t.Fatalf("stored json 解析: %v", err)
	}
	if !secrets.IsCiphertextFormat(stored[0].ClientSecret) {
		t.Fatalf("client_secret 应为 enc:v1: 密文, got %q", stored[0].ClientSecret)
	}

	got, err := loadTenantSSOProvidersWithRepo(ctx, kv)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 1 || got[0].ClientSecret != "s3cr3t-plain" {
		t.Errorf("读取应还原明文: got %+v", got)
	}
}

// TestTenantSSOPlaintextRowUpgradedOnSave 存量明文行在 MASTER_KEY 配好后的第一次写回里升级成密文。
//
// 单独一格的原因：升级只在"写"这条路发生，读路径必须对明文宽容（否则密钥轮换期间
// 整个租户登录入口全挂），所以它的证据只能来自写侧。
func TestTenantSSOPlaintextRowUpgradedOnSave(t *testing.T) {
	secrets.ResetForTest()
	defer secrets.ResetForTest()
	t.Setenv("MASTER_KEY", ssoTestMasterKey)
	if err := secrets.InitFromEnv(); err != nil {
		t.Fatalf("secrets.InitFromEnv: %v", err)
	}

	kv := newTenantKV()
	ctx := context.Background()
	plain, _ := json.Marshal([]TenantSSOProvider{{Name: "corp", Issuer: "https://idp.example.com", ClientID: "cid", ClientSecret: "legacy-plain"}})
	kv.rows[kvSlot(tenant.DefaultTenantID, kvSSOProviders)] = string(plain)

	// 读：明文照旧可用。
	got, err := loadTenantSSOProvidersWithRepo(ctx, kv)
	if err != nil || len(got) != 1 || got[0].ClientSecret != "legacy-plain" {
		t.Fatalf("明文行读取: got %+v err %v", got, err)
	}

	// 写回（改个显示名）：密钥顺带升级成密文。
	got[0].DisplayName = "公司 IdP"
	if err := saveTenantSSOProvidersWithRepo(ctx, kv, got); err != nil {
		t.Fatalf("save: %v", err)
	}
	if strings.Contains(kv.raw(tenant.DefaultTenantID), "legacy-plain") {
		t.Errorf("写回后库里仍是明文: %s", kv.raw(tenant.DefaultTenantID))
	}
	again, err := loadTenantSSOProvidersWithRepo(ctx, kv)
	if err != nil || len(again) != 1 || again[0].ClientSecret != "legacy-plain" {
		t.Errorf("升级后读取: got %+v err %v", again, err)
	}
}

func TestTenantSSOSecret_PlaintextWhenMasterKeyMissing(t *testing.T) {
	secrets.ResetForTest()
	defer secrets.ResetForTest()
	t.Setenv("MASTER_KEY", "")
	if secrets.Ready() {
		t.Fatal("MASTER_KEY 置空后 secrets 不应就绪")
	}

	kv := newTenantKV()
	ctx := context.Background()
	providers := []TenantSSOProvider{{Name: "corp", Issuer: "https://idp.example.com", ClientID: "cid", ClientSecret: "plain-only"}}
	if err := saveTenantSSOProvidersWithRepo(ctx, kv, providers); err != nil {
		t.Fatalf("save: %v", err)
	}
	if !strings.Contains(kv.raw(tenant.DefaultTenantID), "plain-only") {
		t.Errorf("未配置 MASTER_KEY 时应按明文落库（降级可用优先于拒绝写入）: %s", kv.raw(tenant.DefaultTenantID))
	}
	got, err := loadTenantSSOProvidersWithRepo(ctx, kv)
	if err != nil || len(got) != 1 || got[0].ClientSecret != "plain-only" {
		t.Errorf("明文回读: got %+v err %v", got, err)
	}
}

// ---- 掩码沿用 --------------------------------------------------------------

func TestPreserveMaskedClientSecrets(t *testing.T) {
	existing := []TenantSSOProvider{
		{Name: "corp", Issuer: "https://a.example.com", ClientID: "cid", ClientSecret: "stored-a"},
		{Name: "other", Issuer: "https://b.example.com", ClientID: "cid2", ClientSecret: ""},
	}
	submitted := []TenantSSOProvider{
		{Name: "corp", Issuer: "https://a.example.com", ClientID: "cid", ClientSecret: "***"},  // 沿用
		{Name: "new", Issuer: "https://c.example.com", ClientID: "cid3", ClientSecret: "real"}, // 新值
	}
	got, err := PreserveMaskedClientSecrets(submitted, existing)
	if err != nil {
		t.Fatalf("preserve: %v", err)
	}
	if got[0].ClientSecret != "stored-a" {
		t.Errorf("掩码应沿用旧密钥, got %q", got[0].ClientSecret)
	}
	if got[1].ClientSecret != "real" {
		t.Errorf("非掩码值应原样保留, got %q", got[1].ClientSecret)
	}
	// 输入不被改写：调用方拿的是同一份 slice 时，就地改会让"上一次提交"的残影像留在表单里。
	if submitted[0].ClientSecret != "***" {
		t.Errorf("submitted 入参被就地改写: %q", submitted[0].ClientSecret)
	}

	// 只对掩码动手：旧值为空的那条即便提交掩码也报错（无值可沿用）。
	bad := []TenantSSOProvider{{Name: "other", Issuer: "https://b.example.com", ClientID: "cid2", ClientSecret: "***"}}
	if _, err := PreserveMaskedClientSecrets(bad, existing); err == nil {
		t.Error("没有可沿用的旧密钥时提交掩码必须报错，否则字面量 *** 会被存成密钥")
	}

	// 显式空串是"清除"，不是"沿用"。
	cleared := []TenantSSOProvider{{Name: "corp", Issuer: "https://a.example.com", ClientID: "cid", ClientSecret: ""}}
	out, err := PreserveMaskedClientSecrets(cleared, existing)
	if err != nil || out[0].ClientSecret != "" {
		t.Errorf("空串应视为清除: got %+v err %v", out, err)
	}
}

func TestMaskTenantSSOClientSecrets(t *testing.T) {
	in := []TenantSSOProvider{
		{Name: "a", ClientSecret: "s1"},
		{Name: "b", ClientSecret: ""},
	}
	out := MaskTenantSSOClientSecrets(in)
	if out[0].ClientSecret != "***" {
		t.Errorf("已配置的密钥应打码, got %q", out[0].ClientSecret)
	}
	if out[1].ClientSecret != "" {
		t.Errorf("没配的密钥应保持空(前端据此区分没填与填过), got %q", out[1].ClientSecret)
	}
	if in[0].ClientSecret != "s1" {
		t.Error("打码不许改原切片：原值还要用于签发令牌")
	}
}

// ---- 按租户取适配器 --------------------------------------------------------

func ssoPlatformCfg(enabled bool) config.SSOConfig {
	return config.SSOConfig{
		Enabled: enabled,
		Providers: map[string]config.SSOProviderConfig{
			"feishu": {Issuer: "https://feishu.example.com", ClientID: "platform-cid"},
		},
	}
}

// newSSOServiceWithKV 建一个"平台档 + 指定 KV 替身"的服务实例，并把它用的库一起返回
// （回调那几格要拿库建 mock、读回落后的用户行）。
func newSSOServiceWithKV(t *testing.T, cfg config.SSOConfig, kv repository.SystemConfigKVRepository) (*SSOService, *gorm.DB) {
	t.Helper()
	database := testutil.NewTestDB(t, &model.SystemUser{}, &model.SSOIdentity{})
	db.SetTestDB(database)
	t.Cleanup(func() { db.SetTestDB(nil) })
	return NewSSOServiceWithKV(cfg, repository.NewSystemUserRepository(), repository.NewSSOIdentityRepository(database), kv), database
}

// TestSSOService_TenantProvidersVisibleOnlyToTheirTenant 两条腿：
// A 自配了一个 IdP，B 没配。断言"看到什么"按 ctx 的租户走，而不是全局第一份。
func TestSSOService_TenantProvidersVisibleOnlyToTheirTenant(t *testing.T) {
	kv := newTenantKV()
	svc, _ := newSSOServiceWithKV(t, ssoPlatformCfg(true), kv)

	tenantA := tenant.WithTenantID(context.Background(), 81)
	tenantB := tenant.WithTenantID(context.Background(), 82)

	// 两条腿共用一个 service 实例，正是生产形态（router 只建一份）。
	if err := svc.SaveTenantProviders(tenantA, []TenantSSOProvider{
		{Name: "corp-a", Issuer: "https://a-idp.example.com", ClientID: "cid-a", ClientSecret: "sk-a"},
	}); err != nil {
		t.Fatalf("save tenant 81: %v", err)
	}

	listA := svc.ListProvidersFor(tenantA)
	if len(listA) != 1 || listA[0].Name != "corp-a" {
		t.Fatalf("租户 81 应只看到自配的 corp-a, got %+v", listA)
	}
	if _, ok := svc.AdapterFor(tenantA, "feishu"); ok {
		t.Error("租户 81 自配后仍能看到平台 feishu：登录页会多出一个不属于该租户的入口")
	}

	listB := svc.ListProvidersFor(tenantB)
	if len(listB) != 1 || listB[0].Name != "feishu" {
		t.Fatalf("租户 82 未自配，应回落到平台 config.yaml, got %+v", listB)
	}
	if _, ok := svc.AdapterFor(tenantB, "corp-a"); ok {
		t.Error("租户 82 看到了租户 81 的 IdP：跨租户泄漏")
	}

	// 没自配 = 平台开关说话；自配了 = 即便平台关着也开。
	offSvc, _ := newSSOServiceWithKV(t, ssoPlatformCfg(false), newTenantKV())
	if offSvc.EnabledFor(tenantB) {
		t.Error("平台开关关且未自配时 EnabledFor 应为 false")
	}
	if err := offSvc.SaveTenantProviders(tenantA, []TenantSSOProvider{
		{Name: "corp-a", Issuer: "https://a-idp.example.com", ClientID: "cid-a"},
	}); err != nil {
		t.Fatalf("save on disabled-platform service: %v", err)
	}
	if !offSvc.EnabledFor(tenantA) {
		t.Error("租户自配了 IdP 就应可用，平台开关不该把它按住")
	}
}

// TestSSOService_BadTenantJSONFallsBackToPlatform 坏 JSON 不能把登录页打死。
func TestSSOService_BadTenantJSONFallsBackToPlatform(t *testing.T) {
	kv := newTenantKV()
	svc, _ := newSSOServiceWithKV(t, ssoPlatformCfg(true), kv)
	ctx := tenant.WithTenantID(context.Background(), 91)
	kv.rows[kvSlot(91, kvSSOProviders)] = "{not json"

	if _, ok := svc.AdapterFor(ctx, "feishu"); !ok {
		t.Error("租户配置解析失败时应回落平台那一档")
	}
	list := svc.ListProvidersFor(ctx)
	if len(list) != 1 || list[0].Name != "feishu" {
		t.Errorf("回落后的列表: got %+v", list)
	}
}

// TestSSOService_SaveInvalidatesCache 写完立刻读必须换代（"改完刷新页面没变"最难自查）。
func TestSSOService_SaveInvalidatesCache(t *testing.T) {
	kv := newTenantKV()
	svc, _ := newSSOServiceWithKV(t, ssoPlatformCfg(true), kv)
	ctx := tenant.WithTenantID(context.Background(), 95)

	if err := svc.SaveTenantProviders(ctx, []TenantSSOProvider{
		{Name: "first", Issuer: "https://one.example.com", ClientID: "c1"},
	}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, ok := svc.AdapterFor(ctx, "first"); !ok {
		t.Fatal("第一次保存后取不到适配器")
	}

	if err := svc.SaveTenantProviders(ctx, []TenantSSOProvider{
		{Name: "second", Issuer: "https://two.example.com", ClientID: "c2"},
	}); err != nil {
		t.Fatalf("save 2: %v", err)
	}
	if _, ok := svc.AdapterFor(ctx, "second"); !ok {
		t.Error("第二次保存后应看到 second")
	}
	if _, ok := svc.AdapterFor(ctx, "first"); ok {
		t.Error("缓存没换代：旧的 first 仍然可用")
	}

	// 空切片 = 不再自配，回落到平台。
	if err := svc.SaveTenantProviders(ctx, nil); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if _, ok := svc.AdapterFor(ctx, "feishu"); !ok {
		t.Error("清空后应回落平台 config.yaml")
	}
	if _, ok := svc.AdapterFor(ctx, "second"); ok {
		t.Error("清空后租户自配的 second 仍可用")
	}
}

// TestSSOService_OnModeRequiresTenantContext on 模式下没绑租户的 ctx 必须读平台档，
// 而不是"谁先写谁生效"——KV 仓储本身 fail-closed 报错，服务侧把它当回落。
func TestSSOService_OnModeRequiresTenantContext(t *testing.T) {
	old := tenant.CurrentMode()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(old) })

	kv := newTenantKV()
	svc, _ := newSSOServiceWithKV(t, ssoPlatformCfg(true), kv)

	// 先证明这张表里确实写进过租户 99 的自配行。
	if err := svc.SaveTenantProviders(tenant.WithTenantID(context.Background(), 99), []TenantSSOProvider{
		{Name: "corp-99", Issuer: "https://99.example.com", ClientID: "c99"},
	}); err != nil {
		t.Fatalf("save tenant 99: %v", err)
	}
	if _, ok := svc.AdapterFor(tenant.WithTenantID(context.Background(), 99), "corp-99"); !ok {
		t.Fatal("租户 99 应能取到自己的 IdP")
	}

	if _, ok := svc.AdapterFor(context.Background(), "corp-99"); ok {
		t.Error("没有租户绑定的 ctx 读到了某个具体租户的 IdP")
	}
	if _, ok := svc.AdapterFor(context.Background(), "feishu"); !ok {
		t.Error("没有租户绑定的 ctx 应回落到平台档")
	}
}

// ---- 句柄缺失 --------------------------------------------------------------

func TestSSOService_NilKVRejectsWrite(t *testing.T) {
	svc, _ := newSSOServiceWithKV(t, ssoPlatformCfg(true), nil)
	err := svc.SaveTenantProviders(context.Background(), []TenantSSOProvider{
		{Name: "corp", Issuer: "https://x.example.com", ClientID: "c"},
	})
	if !errors.Is(err, repository.ErrKVStoreUnavailable) {
		t.Errorf("未注入 KV 时写入应报 ErrKVStoreUnavailable，实际 %v", err)
	}
	if _, err := svc.TenantProviders(context.Background()); !errors.Is(err, repository.ErrKVStoreUnavailable) {
		t.Errorf("未注入 KV 时读取应报 ErrKVStoreUnavailable，实际 %v", err)
	}
	// 读登录侧那一档仍要能用平台配置：装配缺 KV 不该连平台统一入口一起干掉。
	if _, ok := svc.AdapterFor(context.Background(), "feishu"); !ok {
		t.Error("kv 为 nil 时 AdapterFor 应回落平台 adapters")
	}
}

// ---- 回调链路：租户自配的 IdP 必须能走完整个登录 ----------------------------

// readUserTenantID 从库里读回用户行的 tenant_id。
//
// 断言打落库那一行而不是响应体：登录响应的 DTO 里没有租户列，拿内存视图断言租户
// 归属等于什么都没断。每读一次都新建零值 struct（复用已填充的 struct 再 First()
// 会把旧字段并进 WHERE）。
func readUserTenantID(t *testing.T, database *gorm.DB, ctx context.Context, userID uint) uint {
	t.Helper()
	var u model.SystemUser
	if err := database.WithContext(ctx).First(&u, userID).Error; err != nil {
		t.Fatalf("读回用户 %d: %v", userID, err)
	}
	return u.TenantID
}

// TestSSOService_HandleCallback_TenantScopedIdentity 三个租户面：
//   - 平台 SSO 关着、租户自配了 IdP ⇒ 该租户能登录，别的租户不能借用；
//   - 同一个 IdP 主体在两个租户各开一个账号，用户行按发起租户归属（SaaS 的核心口径）；
//   - 第二次登录复用同一身份行，不重复建号。
//
// 这一格同时是 resolveUser 不再回查 s.adapters 的证据：改成按下标取平台那一份，
// "corp-idp" 在那张表里不存在，取到零值接口后 .OIDC() 当场 panic。
func TestSSOService_HandleCallback_TenantScopedIdentity(t *testing.T) {
	old := tenant.CurrentMode()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(old) })

	kv := newTenantKV()
	svc, database := newSSOServiceWithKV(t, ssoPlatformCfg(false), kv)
	mt := setupSSOServiceTest(t, database)
	// mock 客户端必须在建适配器之前挂上服务实例：租户适配器是懒构建的，
	// 挂晚了 OIDC 发现会去打真网络。
	svc.SetProviderHTTPClient(&http.Client{Transport: mt})

	ctxA := tenant.WithTenantID(context.Background(), 77)
	ctxB := tenant.WithTenantID(context.Background(), 78)

	if _, err := svc.HandleCallback(ctxA, "corp-idp", "test-code", "v"); !errors.Is(err, ErrSSONotEnabled) {
		t.Fatalf("未自配时应报 ErrSSONotEnabled，实际 %v", err)
	}

	provider := TenantSSOProvider{
		Name:          "corp-idp",
		Issuer:        testOIDCIssuer,
		ClientID:      "test-client",
		ClientSecret:  "test-secret",
		RedirectURL:   "https://hivemtk.example.com/api/sso/callback/corp-idp",
		AutoProvision: true,
		DefaultRole:   "user",
		Scopes:        []string{"openid", "profile", "email"},
	}
	for _, ctx := range []context.Context{ctxA, ctxB} {
		if err := svc.SaveTenantProviders(ctx, []TenantSSOProvider{provider}); err != nil {
			t.Fatalf("save providers: %v", err)
		}
	}

	resA, err := svc.HandleCallback(ctxA, "corp-idp", "test-code", "v")
	if err != nil {
		t.Fatalf("HandleCallback tenant 77: %v", err)
	}
	if resA.Provider != "corp-idp" || !resA.IsNewUser || resA.User == nil {
		t.Fatalf("首次登录结果: %+v", resA)
	}
	if tid := readUserTenantID(t, database, ctxA, resA.User.ID); tid != 77 {
		t.Errorf("落库的用户行 tenant_id: got %d want 77（自动建号必须落发起方租户）", tid)
	}

	resB, err := svc.HandleCallback(ctxB, "corp-idp", "test-code", "v")
	if err != nil {
		t.Fatalf("HandleCallback tenant 78: %v", err)
	}
	if resB.User.ID == resA.User.ID {
		t.Error("两个租户共用了同一条本地用户行：SSO 主体被跨租户复用")
	}
	if tid := readUserTenantID(t, database, ctxB, resB.User.ID); tid != 78 {
		t.Errorf("落库的用户行 tenant_id: got %d want 78", tid)
	}

	// 同一租户第二次登录：身份行命中，不再建号。
	resA2, err := svc.HandleCallback(ctxA, "corp-idp", "other-code", "v")
	if err != nil {
		t.Fatalf("HandleCallback 第二次: %v", err)
	}
	if resA2.IsNewUser || resA2.User.ID != resA.User.ID {
		t.Errorf("应复用已有身份, got id=%d isNew=%v", resA2.User.ID, resA2.IsNewUser)
	}

	// B 租户不能凭自己的 ctx 读到 A 的用户行（身份表按租户过滤）。
	var count int64
	if err := database.WithContext(ctxB).Model(&model.SystemUser{}).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("租户 78 可见用户数: got %d want 1（应只看到自己那一条）", count)
	}
}
