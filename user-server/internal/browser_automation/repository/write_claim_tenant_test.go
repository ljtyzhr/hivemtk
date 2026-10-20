package repository

// 契约锁（实体审查 #13，TENANT_MODE=on）：双发闸的键位必须是**租户内**的键位。
//
// 这张表与 #10/#11/#12 的关键差别：它的裁决权整个住在 DDL 里。
// `ClaimWriteSlot` 的 ON CONFLICT 目标写死成 `(tenant_id, task_id, text_hash)` 三列，
// 而模型标签建的是 `(task_id, text_hash)` 两列——三列的形态完全由 `tenancyidx.Normalize`
// 复合化得来。于是这一张表有两种独有的坏法：
//   - Normalize 没跑/被改坏 ⇒ 索引回到两列 ⇒ 占坑的每条 INSERT 撞 PG 42P10
//     （没有匹配该列集的约束）⇒ **所有写步判红**，两种部署形态一起坏；
//   - 索引带着 tenant_id 但租户谓词另一头漏了 ⇒ B 抢同一把键时 INSERT 被 A 的行挡下，
//     而回读 `FindWriteClaim` 在 B 的租户下查不到（A 的行被遮住），
//     于是拿到的是那句「声明在两步之间被释放」——**错误信息说的是谎，真实成因是别的租户**，
//     而后果是 B 的这条写步被永久拦死（跨租户互锁）。
// 两条都要钉在 on 模式，off 模式的同题由 `write_claim_a12_test.go` 的 A1-A8 覆盖。

import (
	"context"
	"errors"
	"strings"
	"testing"

	"hivemtk-user/internal/browser_automation/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

const (
	claimTenantA = uint(71)
	claimTenantB = uint(89)
	// 号段避开 a12*（782xxx）：同包并行时两文件的清理会互相影响行数判据。
	claimTaskA   = uint(797001)
	claimTaskB   = uint(797002)
	claimHashA   = "cafe0001"
	claimHashB   = "cafe0002"
	claimStepA   = uint(797101)
	claimStepB   = uint(797102)
	claimStepC   = uint(797103)
	claimSession = uint(797201)
)

func claimTenantDB(t *testing.T) (BrowserWriteClaimRepository, *gorm.DB, context.Context, context.Context) {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })
	db := testutil.NewTestDB(t, &model.BrowserWriteClaim{})
	if db == nil {
		t.Fatal("测试库不可达：键位归谁、约束长什么样全都判不了（不 Skip，跳过等于没锁）")
	}
	return NewBrowserWriteClaimRepositoryWithDB(db), db,
		tenant.WithTenantID(context.Background(), claimTenantA),
		tenant.WithTenantID(context.Background(), claimTenantB)
}

// claimPhysical 跨租户物理读（判断「到底是谁的行挡住了我」只能用这一条）。
func claimPhysical(t *testing.T, db *gorm.DB, taskID uint, hash string) []model.BrowserWriteClaim {
	t.Helper()
	var rows []model.BrowserWriteClaim
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).
		Where("task_id = ? AND text_hash = ?", taskID, hash).Order("tenant_id ASC").Find(&rows).Error; err != nil {
		t.Fatalf("物理读取 claim(task=%d hash=%s) 失败: %v", taskID, hash, err)
	}
	return rows
}

// TestClaimUniqueIndexLeadsWithTenant 约束的列集必须与代码里的 ON CONFLICT 目标逐字同形。
//
// 这条锁判的是 DDL 与 SQL 的契约，不是租户谓词：ON CONFLICT 的列集必须在库里真有一条
// 唯一索引与之匹配，否则占坑语句直接 42P10，写步一条都发不出去（fail-close 一侧，
// 所以它不会静默坏，但会在**两种部署形态**上同时坏——独立部署没有多租户，
// 而这句话里的 tenant_id 照样在列集里）。
func TestClaimUniqueIndexLeadsWithTenant(t *testing.T) {
	repo, db, ctxA, _ := claimTenantDB(t)
	var defs []string
	if err := db.Raw(`SELECT indexdef FROM pg_indexes WHERE tablename = 'browser_write_claims' AND indexdef ILIKE '%UNIQUE%'`).
		Scan(&defs).Error; err != nil {
		t.Fatalf("索引读数失败: %v", err)
	}
	want := "(tenant_id, task_id, text_hash)"
	found := false
	for _, d := range defs {
		if strings.Contains(d, a12UniqueIndex) {
			found = true
			if !strings.Contains(d, want) {
				t.Errorf("唯一索引列集不是 %s（got %q）：代码里的 ON CONFLICT 目标是 (tenant_id, task_id, text_hash)，"+
					"列集不匹配时每条占坑 INSERT 都会以 42P10 失败，所有写步判红", want, d)
			}
		}
	}
	if !found {
		t.Fatalf("库里没有名为 %s 的唯一索引（got %v）：双发闸的裁决权根本不在库内", a12UniqueIndex, defs)
	}
	// 这条约束必须真的接得住占坑语句（列集匹配的最直接证据就是 INSERT 不报 42P10）。
	if holder, err := repo.ClaimWriteSlot(ctxA, claimTaskA, claimSession, claimStepA, claimHashA); err != nil || holder != nil {
		t.Fatalf("占坑本身失败 holder=%+v err=%v", holder, err)
	}
}

// TestClaimKeysAreTenantLocal 两个租户各自占同一把 (task_id, text_hash) 都必须成功。
//
// 这是跨租户互锁那一格的正面锁：键位若仍是全局的，B 的 INSERT 被 A 的行挡下、
// 回读又被租户谓词遮住，B 拿到的错误是「声明在两步之间被释放」——
// 一句谎话配一次永久拦死，比报错更坏。
func TestClaimKeysAreTenantLocal(t *testing.T) {
	repo, db, ctxA, ctxB := claimTenantDB(t)
	if holder, err := repo.ClaimWriteSlot(ctxA, claimTaskA, claimSession, claimStepA, claimHashA); err != nil || holder != nil {
		t.Fatalf("A 占坑失败 holder=%+v err=%v", holder, err)
	}
	holder, err := repo.ClaimWriteSlot(ctxB, claimTaskA, claimSession, claimStepB, claimHashA)
	if err != nil {
		t.Fatalf("B 占同一把键报错（err=%v）：跨租户互锁，B 的写步永远发不出去——"+
			"若错误是「两步之间被释放」，那就是回读被租户谓词遮住了 A 的行", err)
	}
	if holder != nil {
		t.Errorf("B 被自己的另一条腿判成持有者（holder step=%d tenant=%d）", holder.StepRowID, holder.TenantID)
	}
	rows := claimPhysical(t, db, claimTaskA, claimHashA)
	if len(rows) != 2 {
		t.Fatalf("同键应有两把各自的坑（got %d 行）：说明唯一约束仍是全局的", len(rows))
	}
	for _, r := range rows {
		if r.TenantID != claimTenantA && r.TenantID != claimTenantB {
			t.Errorf("坑落在租户 %d（期望 %d/%d）：填错租户的键位会落进默认租户 1", r.TenantID, claimTenantA, claimTenantB)
		}
	}

	// 读侧与释放侧都必须只在各自租户内生效。
	if row, err := repo.FindWriteClaim(ctxA, claimTaskA, claimHashA); err != nil || row.TenantID != claimTenantA {
		t.Errorf("A 的回读拿到 %+v err=%v，期望只看自己的那把", row, err)
	}
	if n, err := repo.ReleaseWriteSlot(ctxA, claimStepB, claimHashA); err != nil || n != 0 {
		t.Errorf("A 释放 B 的坑：rows=%d err=%v，期望 0/nil（条件少了租户就成了一把跨租户万能钥匙）", n, err)
	}
	if len(claimPhysical(t, db, claimTaskA, claimHashA)) != 2 {
		t.Error("B 的坑被 A 删掉了")
	}
	// 自己的那一步在自己的 ctx 下删得掉（防「谓词太严」造成的假绿）。
	if n, err := repo.ReleaseWriteSlot(ctxB, claimStepB, claimHashA); err != nil || n != 1 {
		t.Errorf("B 释放自己的坑 rows=%d err=%v，期望 1/nil", n, err)
	}
}

// TestClaimWriteSideFillsTenantAndRefusesBareContext 占坑写侧：租户由 ctx 填，裸 ctx 必须报错。
//
// 裸 ctx 的红法与其他表不同：map 形态的 Create 走 `fillMapCreateForTenant`，
// 未绑定且 map 里没有 tenant_id ⇒ 拒绝。放行的话行落列默认值 1，
// 于是「平台默认租户」名下凭空多出一把坑——它既不归发起方，又占着键位不腾。
func TestClaimWriteSideFillsTenantAndRefusesBareContext(t *testing.T) {
	repo, db, ctxA, _ := claimTenantDB(t)
	holder, err := repo.ClaimWriteSlot(ctxA, claimTaskB, claimSession, claimStepC, claimHashB)
	if err != nil || holder != nil {
		t.Fatalf("A 占坑失败 holder=%+v err=%v", holder, err)
	}
	row := claimPhysical(t, db, claimTaskB, claimHashB)
	if len(row) != 1 || row[0].TenantID != claimTenantA {
		t.Fatalf("坑的归属 %+v（期望 1 行 tenant_id=%d）", row, claimTenantA)
	}
	if _, err := repo.ClaimWriteSlot(context.Background(), claimTaskB, claimSession, claimStepA, "cafe0003"); err == nil {
		t.Error("裸 ctx 的占坑未被拒绝")
	} else if !strings.Contains(err.Error(), "[tenant]") {
		t.Errorf("裸 ctx 报错但不是租户闸报的（got %v）：真实成因可能被别的错误盖住", err)
	}
	// 回读必须是**闸报错**而不是「查不到」：ErrRecordNotFound 在调用方那边是
	// 「没有声明」= 可以下发，裸 ctx 静默返空等于把闸门拆成摆设（ClaimWriteSlot 的
	// 占坑 0 行分支正是靠这条回读判持有者的）。
	if _, err := repo.FindWriteClaim(context.Background(), claimTaskB, claimHashB); err == nil {
		t.Error("裸 ctx 的回读未被拒绝")
	} else if errors.Is(err, gorm.ErrRecordNotFound) || !strings.Contains(err.Error(), "[tenant]") {
		t.Errorf("裸 ctx 回读 err=%v，期望 [tenant] 的 fail-closed 报错，不是「查不到」", err)
	}
}
