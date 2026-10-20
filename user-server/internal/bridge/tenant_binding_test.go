package bridge

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/dbstate"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// 桥接扩展只带一枚实例级凭证，归属只能由上报的 (channel, account_id) 自证。
// 这组用例钉住五种结局的边界：行自证、首次注册、跨租户歧义、显式短码、反查故障。

func bindingCtx(t *testing.T, rawQuery string, headers map[string]string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest(http.MethodGet, "/api/bridge/outbox?"+rawQuery, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	c.Request = req
	return c
}

func seedBridgeAccount(t *testing.T, db *gorm.DB, tid uint, channel, accountID string) {
	t.Helper()
	// 写入用"只带租户、不带 SystemScope"的 ctx：带 SystemScope 时隔离 callback
	// 跳过 Create 填列，行会落到列默认值 1，夹具就不是我以为的那个租户了。
	if err := db.WithContext(tenant.WithTenantID(context.Background(), tid)).
		Create(&model.BridgeAccount{Channel: channel, AccountID: accountID, UserID: 1}).Error; err != nil {
		t.Fatalf("写入桥接账号夹具失败 tenant=%d %s/%s: %v", tid, channel, accountID, err)
	}
}

// seedTenant 建一枚租户并返回其 ID（短码解析要读 tenants 表）。
func seedTenant(t *testing.T, db *gorm.DB, code, name string, status int16) uint {
	t.Helper()
	tr := &model.Tenant{Code: code, Name: name, Status: status, CompanyName: name}
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).Create(tr).Error; err != nil {
		t.Fatalf("写入租户夹具失败 code=%s: %v", code, err)
	}
	return tr.ID
}

func modeOn(t *testing.T) {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })
}

// off 模式必须逐字节等价于改造前：不解析、不绑租户、不改写 c.Request。
// 这条是整个桥接改动的回归底线——离线单机部署上线的每一天都靠它。
func TestBindBridgeTenant_OffModeUntouched(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.BridgeAccount{}, &model.Tenant{})
	tenant.SetMode(tenant.ModeOff)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	seedBridgeAccount(t, db, tenant.DefaultTenantID, "douyin", "acc-off")
	c := bindingCtx(t, "channel=douyin&account_id=acc-off", nil)
	before := c.Request

	ctx, err := bindBridgeTenant(c, "douyin", "acc-off")
	if err != nil {
		t.Fatalf("off 模式不该有任何失败: %v", err)
	}
	if ctx != before.Context() {
		t.Errorf("off 模式应原样返回请求 ctx，实际换成了另一条 ctx")
	}
	if c.Request != before {
		t.Errorf("off 模式不该改写 c.Request")
	}
	if id, bound := tenant.FromContext(ctx); bound {
		t.Errorf("off 模式不该绑进租户，实际 bound=%d", id)
	}
}

func TestBindBridgeTenant_RowSelfCertification(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.BridgeAccount{}, &model.Tenant{})
	modeOn(t)

	seedBridgeAccount(t, db, 11, "douyin", "acc-own")
	c := bindingCtx(t, "channel=douyin&account_id=acc-own", nil)

	ctx, err := bindBridgeTenant(c, "douyin", "acc-own")
	if err != nil {
		t.Fatalf("已注册账号应自证成功: %v", err)
	}
	if id, bound := tenant.FromContext(ctx); !bound || id != 11 {
		t.Errorf("返回值 ctx 应绑到 11，实际 bound=%v id=%d", bound, id)
	}
	// handler 里既有"用返回值"的链路，也有直接取 c.Request.Context() 的链路，
	// 两处必须看到同一个租户，否则消息落进 A、在线位刷到 B。
	if id, bound := tenant.FromContext(c.Request.Context()); !bound || id != 11 {
		t.Errorf("c.Request 未同步改写（应绑到 11），实际 bound=%v id=%d", bound, id)
	}
}

// 首次上报（库里还没有账号行）不带短码 ⇒ 拒。放行只会让后续每一次读写被
// 隔离 callback 判错，而 ingest 仍回 200 —— 扩展看见成功，消息一条都没落库。
func TestBindBridgeTenant_FirstReportWithoutCodeIsRejected(t *testing.T) {
	testutil.NewTestDBOrSkip(t, &model.BridgeAccount{}, &model.Tenant{})
	modeOn(t)

	c := bindingCtx(t, "channel=douyin&account_id=brand-new", nil)
	if _, err := bindBridgeTenant(c, "douyin", "brand-new"); !errors.Is(err, ErrBridgeTenantUnresolved) {
		t.Errorf("期望 ErrBridgeTenantUnresolved，实际 %v", err)
	}
	if got := bridgeTenantHTTPStatus(ErrBridgeTenantUnresolved); got != http.StatusNotFound {
		t.Errorf("首次注册缺归属应回 404（补上 tenant_code 就能重试），实际 %d", got)
	}
}

func TestBindBridgeTenant_AmbiguousKeyNeedsExplicitCode(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.BridgeAccount{}, &model.Tenant{})
	modeOn(t)

	seedBridgeAccount(t, db, 11, "douyin", "acc-both")
	seedBridgeAccount(t, db, 22, "douyin", "acc-both")

	c := bindingCtx(t, "channel=douyin&account_id=acc-both", nil)
	if _, err := bindBridgeTenant(c, "douyin", "acc-both"); !errors.Is(err, ErrBridgeTenantAmbiguous) {
		t.Errorf("跨租户同键应判歧义，实际 %v", err)
	}

	// 带短码后可判清：绑定的是短码指定的那一枚，不是"随便第一行"。
	c2 := bindingCtx(t, "channel=douyin&account_id=acc-both&tenant_code=t22", nil)
	tid := seedTenant(t, db, "t22", "二十三", model.TenantStatusActive)
	ctx, err := bindBridgeTenant(c2, "douyin", "acc-both")
	if err != nil {
		t.Fatalf("歧义 + 有效短码应解析成功: %v", err)
	}
	if id, _ := tenant.FromContext(ctx); id != tid {
		t.Errorf("期望绑到短码租户 %d，实际 %d", tid, id)
	}
}

// 行自证优先于短码：否则任何一台持有实例凭证的机器，只要在 URL 上加一枚别人的
// tenant_code，就能把上报写进别的租户。
func TestBindBridgeTenant_RowCertificationBeatsTenantCode(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.BridgeAccount{}, &model.Tenant{})
	modeOn(t)

	seedBridgeAccount(t, db, 11, "douyin", "acc-fixed")
	other := seedTenant(t, db, "t-intruder", "越权租户", model.TenantStatusActive)

	c := bindingCtx(t, "channel=douyin&account_id=acc-fixed&tenant_code=t-intruder", nil)
	ctx, err := bindBridgeTenant(c, "douyin", "acc-fixed")
	if err != nil {
		t.Fatalf("已注册账号带短码不应失败: %v", err)
	}
	if id, _ := tenant.FromContext(ctx); id != 11 {
		t.Errorf("归属被短码改写了：期望 11，实际 %d（intruder 租户 %d）", id, other)
	}
}

func TestBindBridgeTenant_InvalidTenantCode(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.BridgeAccount{}, &model.Tenant{})
	modeOn(t)

	// 存在但已停用的租户：短码不能把停用租户变成可写入的归属
	seedTenant(t, db, "t-suspended", "停用租户", model.TenantStatusSuspended)
	c := bindingCtx(t, "channel=douyin&account_id=new-one&tenant_code=t-suspended", nil)
	if _, err := bindBridgeTenant(c, "douyin", "new-one"); !errors.Is(err, ErrBridgeTenantCodeInvalid) {
		t.Errorf("期望 ErrBridgeTenantCodeInvalid，实际 %v", err)
	}

	c2 := bindingCtx(t, "channel=douyin&account_id=new-one&tenant_code=no-such-code", nil)
	if _, err := bindBridgeTenant(c2, "douyin", "new-one"); !errors.Is(err, ErrBridgeTenantCodeInvalid) {
		t.Errorf("查无此短码应判短码无效，实际 %v", err)
	}
}

// 上游中间件已经绑好时不得重复解析，也不得覆写既有归属。
func TestBindBridgeTenant_AlreadyBoundIsNoop(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.BridgeAccount{}, &model.Tenant{})
	modeOn(t)

	seedBridgeAccount(t, db, 11, "douyin", "acc-b11")
	c := bindingCtx(t, "channel=douyin&account_id=acc-b11", nil)
	c.Request = c.Request.WithContext(tenant.WithTenantID(c.Request.Context(), 99))

	ctx, err := bindBridgeTenant(c, "douyin", "acc-b11")
	if err != nil {
		t.Fatalf("已绑定应直通: %v", err)
	}
	if id, _ := tenant.FromContext(ctx); id != 99 {
		t.Errorf("既有绑定被改写：期望 99，实际 %d", id)
	}
}

// 反查故障（库没起来）必须与"账号还没注册"分成两类：后者能靠补 tenant_code 自救，
// 前者回 404 提示补短码只会把运维引到错误的方向上。
func TestBindBridgeTenant_LookupFailureIsNotUnresolved(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.BridgeAccount{}, &model.Tenant{})
	modeOn(t)
	_ = db

	dbstate.Set(nil)
	t.Cleanup(func() { dbstate.Set(db) })

	c := bindingCtx(t, "channel=douyin&account_id=acc-x", nil)
	_, err := bindBridgeTenant(c, "douyin", "acc-x")
	if err == nil {
		t.Fatal("库未初始化的反查必须报错")
	}
	if errors.Is(err, ErrBridgeTenantUnresolved) {
		t.Errorf("反查故障被误判成首次注册：%v", err)
	}
	if got := bridgeTenantHTTPStatus(err); got != http.StatusInternalServerError {
		t.Errorf("反查故障应回 500（重试帮不上忙），实际 %d", got)
	}
}

// 渠道别名是这条链路上最容易静默失效的一处：bridge_accounts 只存规范名，
// 传 douyin_web 会让自证查不到行，把已注册账号误判成首次注册。
// 这条用例把该契约钉住——收口归一化之前，入口必须先 NormalizeBridgeChannel。
func TestBindBridgeTenant_AliasChannelDoesNotSelfCertify(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.BridgeAccount{}, &model.Tenant{})
	modeOn(t)

	seedBridgeAccount(t, db, 11, "douyin", "acc-alias")
	c := bindingCtx(t, "channel=douyin_web&account_id=acc-alias", nil)
	if _, err := bindBridgeTenant(c, "douyin_web", "acc-alias"); !errors.Is(err, ErrBridgeTenantUnresolved) {
		t.Errorf("别名渠道应判无归属（提醒入口必须归一化），实际 %v", err)
	}
}

// WS 传输口的同一份判定：channel/account_id 来自 register 帧，短码来自握手 URL。
func TestBridgeTenantForRequest(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.BridgeAccount{}, &model.Tenant{})
	modeOn(t)

	seedBridgeAccount(t, db, 11, "douyin", "acc-ws")

	newReq := func(rawQuery string) *http.Request {
		return httptest.NewRequest(http.MethodGet, "/api/ws/channel?"+rawQuery, nil)
	}

	ctx, err := BridgeTenantForRequest(newReq(""), "douyin", "acc-ws")
	if err != nil {
		t.Fatalf("已注册账号的 WS 握手应自证成功: %v", err)
	}
	if id, bound := tenant.FromContext(ctx); !bound || id != 11 {
		t.Errorf("WS 基准 ctx 应绑到 11，实际 bound=%v id=%d", bound, id)
	}

	if _, err := BridgeTenantForRequest(newReq(""), "douyin", "acc-new"); !errors.Is(err, ErrBridgeTenantUnresolved) {
		t.Errorf("首次 WS 注册无短码应拒绝，实际 %v", err)
	}

	wsTenant := seedTenant(t, db, "t-ws", "WS 租户", model.TenantStatusActive)
	ctx, err = BridgeTenantForRequest(newReq("tenant_code=t-ws"), "douyin", "acc-new")
	if err != nil {
		t.Fatalf("首次 WS 注册带短码应成功: %v", err)
	}
	if id, _ := tenant.FromContext(ctx); id != wsTenant {
		t.Errorf("期望绑到 %d，实际 %d", wsTenant, id)
	}

	// 短码走握手头与走 query 是同一件事
	hdrReq := newReq("")
	hdrReq.Header.Set("X-Tenant-Code", "t-ws")
	ctx, err = BridgeTenantForRequest(hdrReq, "douyin", "acc-new-2")
	if err != nil {
		t.Fatalf("X-Tenant-Code 头应被识别: %v", err)
	}
	if id, _ := tenant.FromContext(ctx); id != wsTenant {
		t.Errorf("头短码解析结果不符：期望 %d，实际 %d", wsTenant, id)
	}
}

func TestBridgeTenantForRequest_OffModePassesThrough(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.BridgeAccount{}, &model.Tenant{})
	tenant.SetMode(tenant.ModeOff)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })
	_ = db

	req := httptest.NewRequest(http.MethodGet, "/api/ws/channel", nil)
	ctx, err := BridgeTenantForRequest(req, "douyin", "acc-any")
	if err != nil {
		t.Fatalf("off 模式不该失败: %v", err)
	}
	if ctx != req.Context() {
		t.Errorf("off 模式应原样返回请求 ctx（不绑定、不改写）")
	}
}
