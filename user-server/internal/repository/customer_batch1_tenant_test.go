// customer_batch1_tenant_test.go 客户域第一批（#68–#78）的写面租户契约。
//
// 三处被改的写口此前都靠「整行 Save + 不看 RowsAffected」：
//   - customer_tags.Update：命中 0 行（已删/别租户）也算保存成功；
//   - recovery_queue 的「已挽回/推后重试」：先 Get 再 Save 整行，会把 worker 正在改的
//     attempts / next_attempt_at 用旧快照覆回去，且 0 行静默成功；
//   - customer_channels 的幂等绑定：FirstOrCreate 先查后写，并发会撞唯一索引，
//     且首绑时 is_primary/group_id 落在 assignVals 里、插入分支拿不到。
//
// 这里逐条钉死「只写该写的列、0 行即不存在、跨租户不可见」。
package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

const (
	b1TenantA = uint(101)
	b1TenantB = uint(102)
)

func b1SetupDB(t *testing.T, models ...any) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, models...)
	if db == nil {
		t.Fatal("测试库不可达：租户遮挡判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func b1Ctx(id uint) context.Context {
	return tenant.WithTenantID(context.Background(), id)
}

// TestCustomerTagUpdate_WhitelistAndNotFound 标签规则更新只写 name/category/rule，
// 且命中 0 行（别租户 / 已删）必须报不存在。
func TestCustomerTagUpdate_WhitelistAndNotFound(t *testing.T) {
	b1SetupDB(t, &model.CustomerTag{})
	repo := NewCustomerTagRepository()

	ctxA := b1Ctx(b1TenantA)
	tag := &model.CustomerTag{Name: "旧名", Category: model.TagCategoryBehavioral, Source: model.TagSourceAuto, Rule: `{"k":1}`}
	if err := repo.Create(ctxA, tag); err != nil {
		t.Fatalf("创建标签失败: %v", err)
	}

	// 别租户改同一条：必须报不存在，且不得改动归属
	stolen := *tag
	stolen.Name = "被别租户改了"
	if err := repo.Update(b1Ctx(b1TenantB), &stolen); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户更新应报 ErrRecordNotFound，实际 %v", err)
	}
	after, err := repo.GetByID(ctxA, tag.ID)
	if err != nil || after == nil {
		t.Fatalf("本租户读取失败: %v", err)
	}
	if after.Name != "旧名" {
		t.Fatalf("跨租户更新改动了名字: %q", after.Name)
	}

	// 本租户更新：name/category/rule 落库，source 不被调用方传的零值覆盖
	edit := *tag
	edit.Name = "新名"
	edit.Category = model.TagCategoryTransactional
	edit.Rule = `{"k":2}`
	edit.Source = model.TagSourceManual // 调用方不该能改来源，白名单要挡住
	if err := repo.Update(ctxA, &edit); err != nil {
		t.Fatalf("本租户更新失败: %v", err)
	}
	got, err := repo.GetByID(ctxA, tag.ID)
	if err != nil || got == nil {
		t.Fatalf("更新后读取失败: %v", err)
	}
	if got.Name != "新名" || got.Category != model.TagCategoryTransactional || got.Rule != `{"k":2}` {
		t.Fatalf("白名单列未按预期落库: %+v", got)
	}
	if got.Source != model.TagSourceAuto {
		t.Fatalf("source 被整行回写覆盖了: %q", got.Source)
	}

	// 不存在的 id：报不存在，而不是静默成功
	ghost := &model.CustomerTag{Name: "幽灵"}
	ghost.ID = "00000000-0000-0000-0000-000000000000"
	if err := repo.Update(ctxA, ghost); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("不存在的 id 应报 ErrRecordNotFound，实际 %v", err)
	}
}

// TestRecoveryQueue_MarkRecoveredAndDefer_NoClobber 挽回队列表的两条写口
// 只动自己那几列，且 0 行必须报不存在。
func TestRecoveryQueue_MarkRecoveredAndDefer_NoClobber(t *testing.T) {
	db := b1SetupDB(t, &model.RecoveryQueue{})
	repo := NewRecoveryQueueRepositoryWithDB(db)
	ctxA := b1Ctx(b1TenantA)

	item := &model.RecoveryQueue{
		CustomerID: "cust-b1",
		Stage:      model.RecoveryStageQueued,
		Attempts:   2,
	}
	if err := repo.Create(ctxA, item); err != nil {
		t.Fatalf("入队失败: %v", err)
	}

	if err := repo.MarkRecovered(ctxA, item.ID, 9900, time.Now()); err != nil {
		t.Fatalf("标记挽回失败: %v", err)
	}
	got, err := repo.GetByID(ctxA, item.ID)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if got.Stage != model.RecoveryStageSucceed || got.RecoveryValue != 9900 {
		t.Fatalf("挽回状态未落库: stage=%s value=%d", got.Stage, got.RecoveryValue)
	}
	if got.RecoveredAt == nil {
		t.Fatal("recovered_at 未写入")
	}
	if got.Attempts != 2 {
		t.Fatalf("attempts 被整行回写覆盖了: %d", got.Attempts)
	}

	// 别租户点同一条：不可见 ⇒ 0 行 ⇒ 报不存在
	if err := repo.MarkRecovered(b1Ctx(b1TenantB), item.ID, 1, time.Now()); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户标记挽回应报 ErrRecordNotFound，实际 %v", err)
	}

	next := time.Now().Add(time.Hour)
	if err := repo.DeferAttempt(b1Ctx(b1TenantB), item.ID, next); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户推后应报 ErrRecordNotFound，实际 %v", err)
	}

	// 本租户推后：只动 next_attempt_at，不消耗 attempts
	if err := repo.DeferAttempt(ctxA, item.ID, next); err != nil {
		t.Fatalf("推后失败: %v", err)
	}
	got2, err := repo.GetByID(ctxA, item.ID)
	if err != nil {
		t.Fatalf("推后后读取失败: %v", err)
	}
	if got2.Attempts != 2 {
		t.Fatalf("推后重试不该消耗 attempts: %d", got2.Attempts)
	}
	if got2.NextAttemptAt == nil || got2.NextAttemptAt.Before(next.Add(-time.Minute)) {
		t.Fatalf("next_attempt_at 未按期望推后: %v", got2.NextAttemptAt)
	}
}

// TestChannelOverviewUpsert_FirstBindKeepsPrimary 渠道绑定首绑必须保住 is_primary，
// 重复绑定只更新列、不新增行，且跨租户同 (one_id, channel) 各存一份。
func TestChannelOverviewUpsert_FirstBindKeepsPrimary(t *testing.T) {
	db := b1SetupDB(t, &model.CustomerChannel{}, &model.Customer{})
	repo := NewChannelOverviewRepository(db)
	ctxA := b1Ctx(b1TenantA)

	first := &model.CustomerChannel{
		OneID:         "one-batch1",
		Channel:       "telegram",
		ChannelUserID: "tg-1",
		IsPrimary:     true,
	}
	if err := repo.UpsertCustomerChannel(ctxA, first); err != nil {
		t.Fatalf("首绑失败: %v", err)
	}

	var rows []model.CustomerChannel
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).
		Where("one_id = ?", "one-batch1").Find(&rows).Error; err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("首绑应只有 1 行，实际 %d", len(rows))
	}
	if !rows[0].IsPrimary {
		t.Fatal("首绑丢了 is_primary：插入分支没拿到 assignVals 里的字段")
	}

	// 重复绑定：同一行被更新，不新增；is_primary 显式降级要生效
	second := &model.CustomerChannel{
		OneID:         "one-batch1",
		Channel:       "telegram",
		ChannelUserID: "tg-2",
		IsPrimary:     false,
	}
	if err := repo.UpsertCustomerChannel(ctxA, second); err != nil {
		t.Fatalf("重复绑定失败: %v", err)
	}
	rows = nil
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).
		Where("one_id = ?", "one-batch1").Find(&rows).Error; err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("重复绑定应只留 1 行，实际 %d", len(rows))
	}
	if rows[0].ChannelUserID != "tg-2" {
		t.Fatalf("冲突时未更新 channel_user_id: %q", rows[0].ChannelUserID)
	}
	if rows[0].IsPrimary {
		t.Fatal("显式降级 is_primary=false 没落库（零值被 ON CONFLICT 更新集漏掉）")
	}

	// 另一租户绑同一个 one_id+channel：各存一份
	other := &model.CustomerChannel{OneID: "one-batch1", Channel: "telegram", ChannelUserID: "tg-b"}
	if err := repo.UpsertCustomerChannel(b1Ctx(b1TenantB), other); err != nil {
		t.Fatalf("别租户绑定失败: %v", err)
	}
	var total int64
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).
		Model(&model.CustomerChannel{}).Where("one_id = ?", "one-batch1").Count(&total).Error; err != nil {
		t.Fatalf("计数失败: %v", err)
	}
	if total != 2 {
		t.Fatalf("跨租户同键应共存两行，实际 %d", total)
	}
}
