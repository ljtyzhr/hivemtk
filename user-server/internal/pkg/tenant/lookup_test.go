package tenant_test

import (
	"context"
	"errors"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

// T008 把业务唯一索引统一复合成 (tenant_id, 业务列...)，业务键从此不再全局唯一。
// "按键反查归属"因此有三种结局，必须由命中行**跨了几个租户**决定，而不是取第一行：
// 取第一行会把 A 租户的行当成 B 租户的归属证据，写进错误租户。
func TestSingleTenantByKey_ThreeStates(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.BridgeAccount{})
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	sys := tenant.WithSystemScope(context.Background())
	// 夹具写入必须用"只带租户、不带 SystemScope"的 ctx：隔离 callback 见到
	// SystemScope 会跳过 Create 填列，两行都落到列默认值 1，跨租户同键根本建不出来。
	insert := func(tid uint, channel, accountID string) {
		if err := db.WithContext(tenant.WithTenantID(context.Background(), tid)).Create(&model.BridgeAccount{
			Channel:   channel,
			AccountID: accountID,
			UserID:    1,
		}).Error; err != nil {
			t.Fatalf("写入夹具失败 tenant=%d %s/%s: %v", tid, channel, accountID, err)
		}
	}

	// ① 单一归属：日常流量的唯一形态
	insert(11, "douyin", "only-one")
	// ② 歧义：同一业务键在两个租户各有一行
	insert(11, "douyin", "shared-key")
	insert(22, "douyin", "shared-key")

	tid, err := tenant.SingleTenantByKey(sys, db, "bridge_accounts", "channel = ? AND account_id = ?", "douyin", "only-one")
	if err != nil || tid != 11 {
		t.Errorf("单一归属应得 11，实际 tid=%d err=%v", tid, err)
	}

	tid, err = tenant.SingleTenantByKey(sys, db, "bridge_accounts", "channel = ? AND account_id = ?", "douyin", "no-such-account")
	if err != nil || tid != 0 {
		t.Errorf("查无此行应得「无归属」(0,nil)，实际 tid=%d err=%v", tid, err)
	}

	tid, err = tenant.SingleTenantByKey(sys, db, "bridge_accounts", "channel = ? AND account_id = ?", "douyin", "shared-key")
	if !errors.Is(err, tenant.ErrAmbiguousTenantKey) || tid != 0 {
		t.Errorf("跨租户同键必须判歧义，实际 tid=%d err=%v", tid, err)
	}
}

// 反查走系统作用域是这条 helper 成立的前提：调用方 ctx 带着自己的租户时，
// 它仍然要能看见别的租户有没有同一枚键——否则歧义永远查不出来，
// "本租户有行就绑本租户"会把跨租户冲突读成唯一归属。
// 这段用同一条 bound ctx 的普通查询做负控制，证明登记表真的在生效、夹具真的是跨租户的。
func TestSingleTenantByKey_SeesAcrossCallerTenant(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.BridgeAccount{})
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	sys := tenant.WithSystemScope(context.Background())
	for _, tid := range []uint{11, 22} {
		if err := db.WithContext(tenant.WithTenantID(context.Background(), tid)).Create(&model.BridgeAccount{
			Channel: "douyin", AccountID: "both", UserID: 1,
		}).Error; err != nil {
			t.Fatalf("写入夹具失败 tenant=%d: %v", tid, err)
		}
	}

	callerCtx := tenant.WithTenantID(context.Background(), 11)

	// 夹具自证：同键在两个租户各有一行（T008 复合索引后这是允许的形态）。
	// 这一步不成立时后面的"歧义"断言就是在测一个从未存在过的状态。
	var span []uint
	if err := db.WithContext(sys).Model(&model.BridgeAccount{}).
		Where("channel = ? AND account_id = ?", "douyin", "both").
		Distinct("tenant_id").Pluck("tenant_id", &span).Error; err != nil {
		t.Fatalf("夹具跨租户自证失败: %v", err)
	}
	if len(span) != 2 {
		t.Fatalf("夹具应让同一业务键在两个租户各有一行，实际租户集合 %v", span)
	}

	var own []uint
	if err := db.WithContext(callerCtx).Model(&model.BridgeAccount{}).
		Where("channel = ? AND account_id = ?", "douyin", "both").
		Pluck("tenant_id", &own).Error; err != nil {
		t.Fatalf("本租户可见性检查失败: %v", err)
	}
	if len(own) != 1 || own[0] != 11 {
		t.Fatalf("负控制不成立：租户 11 的普通查询应只看到自己那行，实际 %v（隔离 callback 没挂上，本用例判据为空谈）", own)
	}

	tid, err := tenant.SingleTenantByKey(callerCtx, db, "bridge_accounts", "channel = ? AND account_id = ?", "douyin", "both")
	if !errors.Is(err, tenant.ErrAmbiguousTenantKey) || tid != 0 {
		t.Errorf("调用方租户为 11 时仍须发现 22 的同键冲突，实际 tid=%d err=%v", tid, err)
	}
}

// tenant_id = 0 是平台公共池的列默认值语义（见 T007 加列迁移），它不是一门"租户"，
// 把它当归属证据回绑，等于把公共池行读成某个租户私有——后续写入会落进 0 号以外的租户。
func TestSingleTenantByKey_PlatformPoolIsNotAnOwner(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.BridgeAccount{})
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	// 只有系统作用域的 Exec 能写出 tenant_id=0 的行（on 模式 Create 会按 ctx 填列）。
	sql := `INSERT INTO bridge_accounts (tenant_id, channel, account_id, user_id) VALUES (0, 'douyin', 'pool-row', 1)`
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).Exec(sql).Error; err != nil {
		t.Fatalf("写入公共池夹具失败: %v", err)
	}

	tid, err := tenant.SingleTenantByKey(context.Background(), db, "bridge_accounts",
		"channel = ? AND account_id = ?", "douyin", "pool-row")
	if err != nil || tid != 0 {
		t.Errorf("公共池行应判「无归属」(0,nil)，实际 tid=%d err=%v", tid, err)
	}
}

// 反查本身坏掉必须与"查无归属"分成两类返回：把 SQL 错误读成 0 行，
// 调用方会以为账号还没注册，回一句"请携带 tenant_code"，而真因是库连不上。
func TestSingleTenantByKey_LookupFailureIsNotUnresolved(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.BridgeAccount{})

	tid, err := tenant.SingleTenantByKey(context.Background(), db, "bridge_accounts", "no_such_column = ?", 1)
	if err == nil || tid != 0 {
		t.Errorf("坏条件应返回错误，实际 tid=%d err=%v", tid, err)
	}

	tid, err = tenant.SingleTenantByKey(context.Background(), db, "no_such_table", "id = ?", 1)
	if err == nil || tid != 0 {
		t.Errorf("不存在的表应返回错误，实际 tid=%d err=%v", tid, err)
	}

	if _, err := tenant.SingleTenantByKey(context.Background(), (*gorm.DB)(nil), "bridge_accounts", "id = ?", 1); err == nil {
		t.Error("db 未初始化必须报错")
	}
	if _, err := tenant.SingleTenantByKey(context.Background(), db, "", "id = ?", 1); err == nil {
		t.Error("表名为空必须报错")
	}
}
