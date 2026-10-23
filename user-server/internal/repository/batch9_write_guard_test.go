// batch9_write_guard_test.go 钉住批 9（#152–#161）改掉的那一类写口语义。
//
// 被测的四条路径原先都是**裸 Save / 裸 Delete**，两者各有一个"安静的错误":
//
//   - 写不存在的行：`gorm.Save` 在 UPDATE 命中 0 行后会回退成
//     `INSERT ... ON CONFLICT (id) DO UPDATE`（gorm@v1.31.2 finisher_api.go），
//     于是"更新一条已经被删掉的行"不是失败，而是**按原主键插出一条新行**。
//     对本批的表尤其糟：退订记录复活 = 继续给要求停发的人发短信；回执行复活 =
//     重试计数带着旧值重来，这条短信会被永远重试。
//   - 删不存在的行：`Delete` 命中 0 行返回 nil，删除动作变成一次没有留痕的空操作。
//
// 修法统一走 `db.SaveWholeRow` / `db.EnsureAffected`（pkg/db/write_guard.go）。
// 下面的用例断言的核心不是"返回了什么错误"，而是**库里没有多出一行** ——
// 那是这两条路径唯一不可接受的后果。
package repository

import (
	"context"
	"errors"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

func assertNoResurrect(t *testing.T, count int64, what string) {
	t.Helper()
	if count != 0 {
		t.Fatalf("%s：更新一条不存在的行之后库里多出 %d 行 —— Save 回退成了 INSERT（ON CONFLICT DO UPDATE）", what, count)
	}
}

// TestShortLinkRepository_UpdateMissingDoesNotResurrect 短链：更新不存在的 id。
func TestShortLinkRepository_UpdateMissingDoesNotResurrect(t *testing.T) {
	db := testutil.NewTestDB(t, &model.ShortLink{})
	repo := NewShortLinkRepository(db)
	ctx := context.Background()

	err := repo.Update(ctx, &model.ShortLink{
		ID:          987654321,
		ShortCode:   "ghost-code",
		OriginalURL: "https://example.com/ghost",
	})
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("更新不存在的短链应回 ErrRecordNotFound，实际 %v", err)
	}
	var n int64
	if err := db.Model(&model.ShortLink{}).Count(&n).Error; err != nil {
		t.Fatalf("统计失败: %v", err)
	}
	assertNoResurrect(t, n, "short_links")
}

// TestShortLinkRepository_DeleteMissingIsNotFound 短链：删不存在的 id。
func TestShortLinkRepository_DeleteMissingIsNotFound(t *testing.T) {
	db := testutil.NewTestDB(t, &model.ShortLink{})
	repo := NewShortLinkRepository(db)

	if err := repo.Delete(context.Background(), 987654321); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("删除不存在的短链应回 ErrRecordNotFound（而不是 nil 的\"删成功了\"），实际 %v", err)
	}
}

// TestSessionTagRepository_UpdateMissingDoesNotResurrect 会话标签：更新不存在的 id。
func TestSessionTagRepository_UpdateMissingDoesNotResurrect(t *testing.T) {
	db := testutil.NewTestDB(t, &model.SessionTag{})
	repo := NewSessionTagRepository() // 无参构造器读全局句柄，testutil 已注入
	ctx := context.Background()

	err := repo.Update(ctx, &model.SessionTag{
		ID:   987654321,
		Name: "ghost-tag",
		Code: "ghost",
	})
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("更新不存在的标签应回 ErrRecordNotFound，实际 %v", err)
	}
	var n int64
	if err := db.Model(&model.SessionTag{}).Count(&n).Error; err != nil {
		t.Fatalf("统计失败: %v", err)
	}
	assertNoResurrect(t, n, "session_tags")
}

// TestSessionTagRepository_UpdateRuleConditionMissingIsNotFound 按 code 改规则条件时 code 不存在。
func TestSessionTagRepository_UpdateRuleConditionMissingIsNotFound(t *testing.T) {
	testutil.NewTestDB(t, &model.SessionTag{})
	repo := NewSessionTagRepository()

	err := repo.UpdateRuleCondition(context.Background(), "no-such-code", "amount > 100")
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("改不存在标签的规则条件应回 ErrRecordNotFound，实际 %v", err)
	}
}

// TestSmsUnsubscribeRepository_UpdateMissingDoesNotResurrect 退订记录：更新不存在的 id。
//
// 这条是四条里后果最硬的一条：退订是一条**意思表示**。把一条被人工清除的退订
// 记录按原 id 复活，等于继续给一个已经要求停发的人发短信。
func TestSmsUnsubscribeRepository_UpdateMissingDoesNotResurrect(t *testing.T) {
	db := testutil.NewTestDB(t, &model.SmsUnsubscribe{})
	repo := NewSmsUnsubscribeRepository(db)

	err := repo.Update(context.Background(), &model.SmsUnsubscribe{
		ID:    987654321,
		Phone: "13800000000",
	})
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("更新不存在的退订记录应回 ErrRecordNotFound，实际 %v", err)
	}
	var n int64
	if err := db.Model(&model.SmsUnsubscribe{}).Count(&n).Error; err != nil {
		t.Fatalf("统计失败: %v", err)
	}
	assertNoResurrect(t, n, "sms_unsubscribes")
}

// TestSmsUnsubscribeRepository_DeleteByPhoneMissingIsNotFound 清除一个本来就不在名单里的号码。
func TestSmsUnsubscribeRepository_DeleteByPhoneMissingIsNotFound(t *testing.T) {
	testutil.NewTestDB(t, &model.SmsUnsubscribe{})
	repo := NewSmsUnsubscribeRepository(nil)

	err := repo.DeleteByPhone(context.Background(), "13900000000")
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("清除不存在的退订应回 ErrRecordNotFound，实际 %v", err)
	}
}

// TestSmsTrackingRepository_UpdateStatusMissingDoesNotResurrect 短信回执：更新不存在的 id。
//
// 复活一条回执行 = retry_count 带着旧值重来，重试上限再也到不了 ⇒ 这条短信
// 会被每 5 分钟重试一次，永远。
func TestSmsTrackingRepository_UpdateStatusMissingDoesNotResurrect(t *testing.T) {
	db := testutil.NewTestDB(t, &model.SmsDeliveryStatus{})
	repo := NewSmsTrackingRepository(db)

	err := repo.UpdateStatus(context.Background(), &model.SmsDeliveryStatus{
		ID:        987654321,
		MessageID: "ghost-msg",
		Phone:     "13800000000",
	})
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("更新不存在的回执应回 ErrRecordNotFound，实际 %v", err)
	}
	var n int64
	if err := db.Model(&model.SmsDeliveryStatus{}).Count(&n).Error; err != nil {
		t.Fatalf("统计失败: %v", err)
	}
	assertNoResurrect(t, n, "sms_delivery_statuses")
}

// TestSessionMessageRepository_DeleteMissingIsNotFound 会话消息：删不存在的 id。
func TestSessionMessageRepository_DeleteMissingIsNotFound(t *testing.T) {
	db := testutil.NewTestDB(t, &model.SessionMessage{})
	repo := NewSessionMessageRepositoryWithDB(db)

	if err := repo.Delete(context.Background(), 987654321); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("删除不存在的会话消息应回 ErrRecordNotFound，实际 %v", err)
	}
}

// TestSessionMessageRepository_GetBySessionIDClampsPaging 会话消息分页夹取。
//
// 两个参数各有真实后果：page=0 ⇒ offset 为负 ⇒ PG 直接报
// "OFFSET must not be negative"；pageSize=0 ⇒ GORM 的 Limit(0) 等于**不加 limit**。
func TestSessionMessageRepository_GetBySessionIDClampsPaging(t *testing.T) {
	db := testutil.NewTestDB(t, &model.SessionMessage{})
	repo := NewSessionMessageRepositoryWithDB(db)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if err := repo.Create(ctx, &model.SessionMessage{SessionID: "s-clamp", Content: "hi"}); err != nil {
			t.Fatalf("准备数据失败: %v", err)
		}
	}

	// page=0 / pageSize=0 原先分别会报 SQL 错、读整表；夹取之后应当拿到第一页。
	rows, total, err := repo.GetBySessionID(ctx, "s-clamp", 0, 0)
	if err != nil {
		t.Fatalf("page/pageSize 传 0 不应报错，实际 %v", err)
	}
	if total != 3 {
		t.Errorf("total = %d，期望 3", total)
	}
	if len(rows) != 3 {
		t.Errorf("返回 %d 行，期望 3", len(rows))
	}
}

// TestFindRecentDuplicate_NotFoundIsNotAnError 「没有重复」必须回 (nil, nil)。
//
// 三个调用方都写成 `existing, _ := ...; if existing != nil`：把 NotFound 当错误
// 返回时，它们的 `_` 会让「查库失败」和「没有重复」变成同一件事，去重形同不存在。
func TestFindRecentDuplicate_NotFoundIsNotAnError(t *testing.T) {
	db := testutil.NewTestDB(t, &model.SessionMessage{})
	repo := NewSessionMessageRepositoryWithDB(db)

	got, err := repo.FindRecentDuplicate(context.Background(), "s-none", "ai", "ai_assistant", "hello", 5)
	if err != nil {
		t.Fatalf("没有重复时不应返回错误，实际 %v", err)
	}
	if got != nil {
		t.Fatalf("没有重复时应回 nil 行，实际 %+v", got)
	}
}
