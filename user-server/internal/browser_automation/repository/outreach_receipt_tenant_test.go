package repository

// 契约锁（实体审查 #8，TENANT_MODE=on）：触达回执行落在自己租户名下，且读得到谁也读不到别人。
//
// 为什么这一格要单独钉：回执表是**交付物**而不是闸门，它的全部意义在于能把
// 「内容已发出但列表里没有回执」和「这步根本没执行过」分开。两件事会毁掉这个意义：
//   - 读侧：回执行没有 user_id 列，归属只能经 task_id 落到 browser_tasks 上判；而 task_id
//     是小整数、可猜。租户 callback 是这道猜号唯一的遮挡——一旦漏掉，别人的触达正文与
//     截图链接（CopySnapshot/ScreenshotURL）就随任务详情接口漏出去。
//   - 写侧：有人把去重表那套 OnConflict{DoNothing}「统一」过来（或给回执补唯一键），
//     重复的交付证据就被静默吞掉，回执表退化成闸门，上面那条区分永久消失。
//     唯一键在 on 模式下会被 tenancyidx.Normalize 补成租户内键位，所以「B 也能落同四元组」
//     这一腿还顺带钉住了键位本身：跨租户吞行与租户内吞行都能被这组断言抓住。
//
// 种子刻意让两个租户的回执用**同一个** task_id：那样断言的才是 tenant_id 谓词本身，
// 而不是「两个租户的任务号恰好不撞」这种随时会被 PK 复用拆穿的巧合。

import (
	"context"
	"testing"

	"hivemtk-user/internal/browser_automation/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

const (
	receiptTenantA = uint(17)
	receiptTenantB = uint(38)
	// 号段不与 dedupeTaskA/B（outreach_dedupe_tenant_test.go）、llmPlanTaskA/B 重叠：
	// 同包并行时共用行号会让对方的清理改变本方计数。
	receiptSharedTask = uint(792801)
	receiptUserA      = uint(792811)
	receiptUserB      = uint(792812)
	receiptOtherTask  = uint(792821)
	// 四元组与去重表同构：故意两租户共用，用来验回执不是闸门。
	receiptPlatform  = "xiaohongshu"
	receiptTargetURL = "https://www.xiaohongshu.com/explore/792801"
	receiptAction    = "post_comment"
	receiptCopyHash  = "same-copy-hash-both-tenants"
)

func receiptTenantDB(t *testing.T) (*gorm.DB, context.Context, context.Context) {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })
	db := testutil.NewTestDB(t,
		&model.BrowserOutreachReceipt{}, &model.BrowserTask{}, &model.BrowserSession{})
	if db == nil {
		t.Fatal("测试库不可达：回执行的租户归属无法判定（不 Skip，跳过等于没锁）")
	}
	return db,
		tenant.WithTenantID(context.Background(), receiptTenantA),
		tenant.WithTenantID(context.Background(), receiptTenantB)
}

// receiptSeedTask 建一行归属任务。ID 显式给定：归属子查询按 task_id 精确匹配，
// 两个租户各建自己的任务时用不同号，供「B 读 A 的任务」那一腿交叉验证。
func receiptSeedTask(t *testing.T, db *gorm.DB, ctx context.Context, id, userID uint) {
	t.Helper()
	task := &model.BrowserTask{ID: id, Name: "回执租户锁", TaskType: "one_shot", Status: "ready",
		Url: "https://www.xiaohongshu.com/explore", Platform: receiptPlatform,
		UserID: userID, LoopCount: 1, TimeoutSec: 60}
	if err := db.WithContext(ctx).Create(task).Error; err != nil {
		t.Fatalf("任务 %d 落库失败: %v", id, err)
	}
}

// receiptWrite 走被测的 RecordOutreachReceipt 落一条回执（不是夹具直插 db）：
// 写侧的租户归属填充与被审的那件事是同一条路径。
func receiptWrite(t *testing.T, db *gorm.DB, ctx context.Context, taskID uint, snapshot string) {
	t.Helper()
	if err := NewBrowserOutreachReceiptRepositoryWithDB(db).RecordOutreachReceipt(ctx,
		&model.BrowserOutreachReceipt{
			TaskID: taskID, SessionID: taskID + 1, Platform: receiptPlatform, Action: receiptAction,
			TargetURL: receiptTargetURL, CopyHash: receiptCopyHash, CopySnapshot: snapshot,
			Verified: true,
		}); err != nil {
		t.Fatalf("落回执失败: %v", err)
	}
}

// receiptPhysicalRows 全租户物理行数（system scope）：租户内计数看不出「静默吞行」，
// 只有跨租户总量才对 OnConflict{DoNothing} 这类收口敏感。
func receiptPhysicalRows(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	ctx := tenant.WithSystemScope(context.Background())
	if err := db.WithContext(ctx).Model(&model.BrowserOutreachReceipt{}).Count(&n).Error; err != nil {
		t.Fatalf("统计物理回执行失败: %v", err)
	}
	return n
}

// TestReceiptRecordFillsTenantAndRejectsBareContext 写侧两格：绑租户的 ctx 落自己的号，裸 ctx 被拒。
func TestReceiptRecordFillsTenantAndRejectsBareContext(t *testing.T) {
	db, ctxA, _ := receiptTenantDB(t)
	receiptSeedTask(t, db, ctxA, receiptSharedTask, receiptUserA)
	receiptWrite(t, db, ctxA, receiptSharedTask, "A 的文案")

	var got model.BrowserOutreachReceipt
	if err := db.WithContext(ctxA).Where("task_id = ?", receiptSharedTask).First(&got).Error; err != nil {
		t.Fatalf("租户 %d 读不回执行: %v", receiptTenantA, err)
	}
	if got.TenantID != receiptTenantA {
		t.Errorf("回执落库后 TenantID got=%d want=%d：隔离 callback 没填发起租户，"+
			"这条交付证据会挂到默认租户名下，owner 的任务详情读不到它", got.TenantID, receiptTenantA)
	}

	// 裸 ctx：on 模式必须 fail-closed。放行等于允许把别人的触达证据写进任意租户。
	err := NewBrowserOutreachReceiptRepositoryWithDB(db).RecordOutreachReceipt(context.Background(),
		&model.BrowserOutreachReceipt{TaskID: receiptSharedTask, SessionID: 1, Platform: receiptPlatform,
			Action: receiptAction, TargetURL: receiptTargetURL, CopyHash: "unbound"})
	if err == nil {
		t.Error("裸 ctx 的落回执未被拒绝")
	}
	if n := receiptPhysicalRows(t, db); n != 1 {
		t.Errorf("被拒的回执仍落了行：物理计数 got=%d want=1", n)
	}
}

// TestReceiptListByTaskIDIsTenantScoped 同 task_id 的回执行跨租户不可读（含 userID 归属子查询那腿）。
func TestReceiptListByTaskIDIsTenantScoped(t *testing.T) {
	db, ctxA, ctxB := receiptTenantDB(t)
	receiptSeedTask(t, db, ctxA, receiptSharedTask, receiptUserA)
	receiptSeedTask(t, db, ctxB, receiptOtherTask, receiptUserB)
	receiptWrite(t, db, ctxA, receiptSharedTask, "A 的私密文案")
	receiptWrite(t, db, ctxB, receiptSharedTask, "B 的私密文案")

	repo := NewBrowserOutreachReceiptRepositoryWithDB(db)

	// 关键腿：同一 task_id 在两租户各有一行，各自只读得到自己那条。
	// 这一腿判的不是 user 归属而是租户隔离——回执行没有 user_id 列，租户谓词是唯一的遮挡。
	// 断言写成「1 行且是本租户」而不是「0 行」：B 本来就有自己的一行，
	// 真正要防的是租户谓词失效时多出来的那第 2 行。
	for _, tc := range []struct {
		name       string
		ctx        context.Context
		wantTenant uint
		wantCopy   string
	}{
		{"tenantA", ctxA, receiptTenantA, "A 的私密文案"},
		{"tenantB", ctxB, receiptTenantB, "B 的私密文案"},
	} {
		list, err := repo.ListByTaskID(tc.ctx, receiptSharedTask, 0, 50)
		if err != nil {
			t.Fatalf("%s 读取失败: %v", tc.name, err)
		}
		if len(list) != 1 {
			t.Errorf("%s 读到 %d 行 want=1（多出来的是别人的触达正文与截图链接）", tc.name, len(list))
			continue
		}
		if list[0].TenantID != tc.wantTenant || list[0].CopySnapshot != tc.wantCopy {
			t.Errorf("%s 读到的回执 got=(tenant=%d copy=%q) want=(tenant=%d copy=%q)",
				tc.name, list[0].TenantID, list[0].CopySnapshot, tc.wantTenant, tc.wantCopy)
		}
	}

	// 归属子查询同受租户谓词遮挡：B 拿 A 的用户号去过滤也该读到空。
	// 这一腿同时钉住子查询的 ctx（曾漏绑：on 模式下子会话被 fail-closed ⇒ SQL 渲染成
	// `task_id IN ()` ⇒ PG 42601，任务详情的回执列表整条读路径 500）。
	if byUser, err := repo.ListByTaskID(ctxB, receiptSharedTask, receiptUserA, 50); err != nil {
		t.Fatalf("带 userID 的读取失败（on 模式下子查询必须带 ctx）: %v", err)
	} else if len(byUser) != 0 {
		t.Errorf("B 按 A 的 user_id 读到了 %d 行", len(byUser))
	}
	// A 用自己的 user_id 过滤应正常读到（锁住 userID!=0 那条腿没被租户改动做坏）。
	if byUser, err := repo.ListByTaskID(ctxA, receiptSharedTask, receiptUserA, 50); err != nil {
		t.Fatalf("owner 带 userID 的读取失败: %v", err)
	} else if len(byUser) != 1 {
		t.Errorf("owner 按自己 user_id 读到 %d 行 want=1", len(byUser))
	}

	if _, err := repo.ListByTaskID(context.Background(), receiptSharedTask, 0, 50); err == nil {
		t.Error("裸 ctx 的 ListByTaskID 未被拒绝：放行等于把「谁都能按号读触达内容」写回库里")
	}
}

// TestReceiptKeepsDuplicateRowsAcrossTenants 回执表不是闸门：同四元组重复落库必须都留下。
func TestReceiptKeepsDuplicateRowsAcrossTenants(t *testing.T) {
	db, ctxA, ctxB := receiptTenantDB(t)
	receiptSeedTask(t, db, ctxA, receiptSharedTask, receiptUserA)
	repo := NewBrowserOutreachReceiptRepositoryWithDB(db)

	for i := 0; i < 3; i++ {
		receiptWrite(t, db, ctxA, receiptSharedTask, "重复交付留证")
	}
	list, err := repo.ListByTaskID(ctxA, receiptSharedTask, 0, 50)
	if err != nil {
		t.Fatalf("列出回执失败: %v", err)
	}
	if len(list) != 3 {
		t.Errorf("同四元组回执行数 got=%d want=3：吞掉重复行就吞掉了「已发出但无回执」这个区分", len(list))
	}

	// 另一租户落同四元组：既不能被 A 的行顶掉（全局唯一键），也不能顶掉 A 的行。
	receiptWrite(t, db, ctxB, receiptSharedTask, "B 的同款文案")
	if n := receiptPhysicalRows(t, db); n != 4 {
		t.Errorf("物理回执行数 got=%d want=4（跨租户吞行＝拿别人的交付证据给自己的重复让路）", n)
	}
	if a, err := repo.ListByTaskID(ctxA, receiptSharedTask, 0, 50); err != nil || len(a) != 3 {
		t.Errorf("A 的行数被 B 的写入改变 got=%d want=3 err=%v", len(a), err)
	}
}
