package service

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/repository"

	"gorm.io/gorm"
)

// 站内消息通知模块的三个核心不变量（spec SC-003 / SC-006 / SC-007）：
//
//	1. 幂等：同一条平台消息重复投递 ⇒ 本地只有一行；
//	2. 隔离：两个不同租户各投递 ⇒ 彼此看不到对方的行；
//	3. 按人已读：甲读了 ⇒ 乙的未读数不变。
//
// 这三条都是**静默失效**型缺陷（编译过、常规单测绿、功能却不对），
// 所以每个用例都真实连库断言行数与读面结果，不靠 mock。

func setupNoticeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	database := testutil.NewTestDB(t,
		&model.Notification{},
		&model.NotificationRead{},
	)
	return database
}

// newNoticeRepoForTest 造一对仓储（通知 + 回执），共用一个测试库。
func newNoticeRepoForTest(t *testing.T, database *gorm.DB) (*repository.NotificationRepository, *repository.NotificationReadRepository) {
	t.Helper()
	return repository.NewNotificationRepositoryWithDB(database),
		repository.NewNotificationReadRepositoryWithDB(database)
}

// TestNoticeUpsert_Idempotent 钉死幂等：同一条平台消息投递两次只有一行。
//
// 幂等键 = (tenant_id, source_msg_id)，靠**部分唯一**索引
// （WHERE source_msg_id > 0）实现 —— 普通唯一索引会让第二条本地业务通知撞键。
func TestNoticeUpsert_Idempotent(t *testing.T) {
	database := setupNoticeTestDB(t)
	repo, _ := newNoticeRepoForTest(t, database)

	ctx := tenant.WithTenantID(context.Background(), 1)

	newRow := func() *model.Notification {
		return &model.Notification{
			UserID:      0,
			Type:        model.NotificationTypePlatformNotice,
			Title:       "系统维护通知",
			Content:     "今晚 23:00 维护。",
			Level:       model.NotificationLevelWarning,
			Source:      model.NotificationSourcePlatform,
			SourceMsgID: 42,
		}
	}

	first, err := repo.UpsertFromPlatform(ctx, newRow())
	if err != nil {
		t.Fatalf("首次投递失败: %v", err)
	}
	if !first {
		t.Fatal("首次投递应当是新落地（first=true）")
	}

	second, err := repo.UpsertFromPlatform(ctx, newRow())
	if err != nil {
		t.Fatalf("重复投递不应报错: %v", err)
	}
	if second {
		t.Fatal("重复投递应当命中幂等键、不再新落地（second=false）")
	}

	var count int64
	if err := database.WithContext(ctx).Model(&model.Notification{}).
		Where("source_msg_id = ?", 42).Count(&count).Error; err != nil {
		t.Fatalf("统计失败: %v", err)
	}
	if count != 1 {
		t.Fatalf("同一条平台消息在本地应只有 1 行，实际 %d 行（幂等失效）", count)
	}
}

// TestNoticeUpsert_LocalNotificationsNotBlocked 钉死「部分唯一索引不误伤本地通知」。
//
// 本地业务通知的 source_msg_id 恒为 0。若幂等键用的是**普通**唯一索引
// (tenant_id, source_msg_id)，第二条本地通知就会撞唯一键 ⇒ 存量生产者
// （待办 SLA / 坏例 / 知识库变更）全部写失败。
func TestNoticeUpsert_LocalNotificationsNotBlocked(t *testing.T) {
	database := setupNoticeTestDB(t)
	repo, _ := newNoticeRepoForTest(t, database)

	ctx := tenant.WithTenantID(context.Background(), 1)

	// 三条本地业务通知，source_msg_id 都是 0（本地产生）。
	for i := 0; i < 3; i++ {
		n := &model.Notification{
			UserID:      0,
			Type:        model.NotificationTypeInfo,
			Title:       "本地业务通知",
			Source:      model.NotificationSourceLocal,
			SourceMsgID: 0,
		}
		if err := repo.Create(ctx, n); err != nil {
			t.Fatalf("第 %d 条本地业务通知写入失败（部分唯一索引误伤了本地通知）: %v", i+1, err)
		}
	}

	var count int64
	if err := database.WithContext(ctx).Model(&model.Notification{}).
		Where("source_msg_id = 0").Count(&count).Error; err != nil {
		t.Fatalf("统计失败: %v", err)
	}
	if count != 3 {
		t.Fatalf("应有 3 条本地业务通知，实际 %d 条", count)
	}
}

// TestNoticeUpsert_TenantIsolation 钉死跨租户隔离（spec SC-007）。
//
// **两个不同租户**各投递一条同 source_msg_id 的消息 ⇒ 各一行，互不可见。
//
// 为什么必须是不同租户：隔离 callback 在读面注入 WHERE tenant_id=?
// ⇒ 同租户两行根本造不出跨租户场景；而"同键两租户各一行"才照得出
// 归属写错（行落到了别的租户）。
func TestNoticeUpsert_TenantIsolation(t *testing.T) {
	database := setupNoticeTestDB(t)
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	repo, _ := newNoticeRepoForTest(t, database)

	ctxA := tenant.WithTenantID(context.Background(), 91)
	ctxB := tenant.WithTenantID(context.Background(), 92)

	rowA := &model.Notification{
		UserID: 0, Type: model.NotificationTypePlatformNotice,
		Title: "租户甲的消息", Source: model.NotificationSourcePlatform, SourceMsgID: 7,
	}
	rowB := &model.Notification{
		UserID: 0, Type: model.NotificationTypePlatformNotice,
		Title: "租户乙的消息", Source: model.NotificationSourcePlatform, SourceMsgID: 7,
	}
	if _, err := repo.UpsertFromPlatform(ctxA, rowA); err != nil {
		t.Fatalf("租户甲投递失败: %v", err)
	}
	if _, err := repo.UpsertFromPlatform(ctxB, rowB); err != nil {
		t.Fatalf("租户乙投递失败: %v", err)
	}

	// 甲只看得见自己的
	var listA []model.Notification
	if err := database.WithContext(ctxA).Model(&model.Notification{}).Find(&listA).Error; err != nil {
		t.Fatalf("租户甲读取失败: %v", err)
	}
	if len(listA) != 1 || listA[0].Title != "租户甲的消息" {
		t.Fatalf("租户甲应只看到自己的 1 条消息，实际 %d 条: %+v", len(listA), listA)
	}

	// 乙也只看得见自己的
	var listB []model.Notification
	if err := database.WithContext(ctxB).Model(&model.Notification{}).Find(&listB).Error; err != nil {
		t.Fatalf("租户乙读取失败: %v", err)
	}
	if len(listB) != 1 || listB[0].Title != "租户乙的消息" {
		t.Fatalf("租户乙应只看到自己的 1 条消息，实际 %d 条: %+v", len(listB), listB)
	}

	// 行自证：两行分别落在各自租户名下（读面校验防的是"读不到"，
	// 这里额外确认归属本身没写错）。
	var all []model.Notification
	if err := database.WithContext(tenant.WithSystemScope(context.Background())).
		Model(&model.Notification{}).Find(&all).Error; err != nil {
		t.Fatalf("全量读取失败: %v", err)
	}
	byTitle := map[string]uint{}
	for _, n := range all {
		byTitle[n.Title] = n.TenantID
	}
	if byTitle["租户甲的消息"] != 91 || byTitle["租户乙的消息"] != 92 {
		t.Fatalf("消息归属写错：甲=%d 乙=%d（应为 91 / 92）",
			byTitle["租户甲的消息"], byTitle["租户乙的消息"])
	}
}

// TestNotificationRead_PerUserIsolation 钉死「按人已读」（spec SC-003）。
//
// 甲标记已读后：甲的未读数减 1，**乙的未读数不变**。
// 这正是 messages 上单一 is_read 布尔位做不到的事。
func TestNotificationRead_PerUserIsolation(t *testing.T) {
	database := setupNoticeTestDB(t)
	repo, readRepo := newNoticeRepoForTest(t, database)

	ctx := tenant.WithTenantID(context.Background(), 1)

	// 一条广播给全体的平台消息
	n := &model.Notification{
		UserID: 0, Type: model.NotificationTypePlatformNotice,
		Title: "全体公告", Source: model.NotificationSourcePlatform, SourceMsgID: 99,
	}
	if _, err := repo.UpsertFromPlatform(ctx, n); err != nil {
		t.Fatalf("投递失败: %v", err)
	}
	var msg model.Notification
	if err := database.WithContext(ctx).Where("source_msg_id = ?", 99).First(&msg).Error; err != nil {
		t.Fatalf("读取消息失败: %v", err)
	}

	// 初始：甲乙都未读
	for _, uid := range []uint{11, 22} {
		got, err := readRepo.CountUnread(ctx, 1, uid)
		if err != nil {
			t.Fatalf("统计用户 %d 未读失败: %v", uid, err)
		}
		if got != 1 {
			t.Fatalf("用户 %d 初始未读数应为 1，实际 %d", uid, got)
		}
	}

	// 甲读了
	if err := readRepo.MarkRead(ctx, 1, msg.ID, 11); err != nil {
		t.Fatalf("标记已读失败: %v", err)
	}

	gotA, err := readRepo.CountUnread(ctx, 1, 11)
	if err != nil {
		t.Fatalf("统计甲未读失败: %v", err)
	}
	if gotA != 0 {
		t.Fatalf("甲已读后未读数应为 0，实际 %d", gotA)
	}

	gotB, err := readRepo.CountUnread(ctx, 1, 22)
	if err != nil {
		t.Fatalf("统计乙未读失败: %v", err)
	}
	if gotB != 1 {
		t.Fatalf("甲已读不影响乙：乙未读数应仍为 1，实际 %d（按人已读失效）", gotB)
	}

	// 重复标记已读是幂等的，不产生第二行
	if err := readRepo.MarkRead(ctx, 1, msg.ID, 11); err != nil {
		t.Fatalf("重复标记已读不应报错: %v", err)
	}
	var readCount int64
	if err := database.WithContext(ctx).Model(&model.NotificationRead{}).Count(&readCount).Error; err != nil {
		t.Fatalf("统计回执失败: %v", err)
	}
	if readCount != 1 {
		t.Fatalf("重复标记已读后应只有 1 条回执，实际 %d 条", readCount)
	}
}

// TestNotificationService_CountUnread_NoDoubleCount 钉死未读数**不重复计平台消息**。
//
// 生产实测：铃铛角标 11、接口未读数 10，差值正是那条平台消息。
//
// 根因：service 层 CountUnread = 平台消息未读（回执口径）+ 本地通知未读，
// 而仓储的本地未读查询把**平台消息也算进去了** ⇒ 同一条平台消息被数两次。
//
// 修法：仓储的本地口径排除 type = platform_notice（平台消息的已读由回执表
// 负责），两边各算各的、互不重叠。
func TestNotificationService_CountUnread_NoDoubleCount(t *testing.T) {
	database := setupNoticeTestDB(t)
	repo, _ := newNoticeRepoForTest(t, database)
	svc := NewNotificationService(database)

	ctx := tenant.WithTenantID(context.Background(), 1)

	// 一条平台消息 + 两条本地业务通知
	platform := &model.Notification{
		UserID: 0, Type: model.NotificationTypePlatformNotice,
		Title: "平台公告", Source: model.NotificationSourcePlatform, SourceMsgID: 500,
	}
	if _, err := repo.UpsertFromPlatform(ctx, platform); err != nil {
		t.Fatalf("投递平台消息失败: %v", err)
	}
	for i := 0; i < 2; i++ {
		local := &model.Notification{
			UserID: 0, Type: model.NotificationTypeInfo,
			Title: "本地通知", Source: model.NotificationSourceLocal,
		}
		if err := repo.Create(ctx, local); err != nil {
			t.Fatalf("写本地通知失败: %v", err)
		}
	}

	got, err := svc.CountUnread(ctx, 11)
	if err != nil {
		t.Fatalf("统计未读失败: %v", err)
	}
	// 1 条平台消息（无回执）+ 2 条本地通知 = 3，不该是 4。
	if got != 3 {
		t.Fatalf("未读数应为 3（1 平台 + 2 本地），实际 %d（平台消息被重复计数）", got)
	}

	// 平台消息标记已读后，未读数减 1（走回执，不动行上 is_read）
	var msg model.Notification
	if err := database.WithContext(ctx).Where("source_msg_id = ?", 500).First(&msg).Error; err != nil {
		t.Fatalf("读取平台消息失败: %v", err)
	}
	if err := svc.MarkRead(ctx, 11, msg.ID); err != nil {
		t.Fatalf("标记已读失败: %v", err)
	}
	got2, err := svc.CountUnread(ctx, 11)
	if err != nil {
		t.Fatalf("统计未读失败: %v", err)
	}
	if got2 != 2 {
		t.Fatalf("平台消息已读后未读数应为 2，实际 %d", got2)
	}

	// 平台消息行上的 is_read 必须仍是 false：已读归回执管，不归行上布尔位管。
	if err := database.WithContext(ctx).Where("id = ?", msg.ID).First(&msg).Error; err != nil {
		t.Fatalf("回读平台消息失败: %v", err)
	}
	if msg.IsRead {
		t.Fatal("平台消息行的 is_read 不该被置上（已读归回执表管）")
	}
}

// TestNotificationRead_MarkAllRead 只影响当前用户。
func TestNotificationRead_MarkAllRead(t *testing.T) {
	database := setupNoticeTestDB(t)
	repo, readRepo := newNoticeRepoForTest(t, database)

	ctx := tenant.WithTenantID(context.Background(), 1)

	for _, id := range []int64{101, 102, 103} {
		n := &model.Notification{
			UserID: 0, Type: model.NotificationTypePlatformNotice,
			Title: "公告", Source: model.NotificationSourcePlatform, SourceMsgID: id,
		}
		if _, err := repo.UpsertFromPlatform(ctx, n); err != nil {
			t.Fatalf("投递 %d 失败: %v", id, err)
		}
	}

	marked, err := readRepo.MarkAllRead(ctx, 1, 11)
	if err != nil {
		t.Fatalf("全部标记已读失败: %v", err)
	}
	if marked != 3 {
		t.Fatalf("甲全部标记已读应写入 3 条回执，实际 %d", marked)
	}

	gotB, err := readRepo.CountUnread(ctx, 1, 22)
	if err != nil {
		t.Fatalf("统计乙未读失败: %v", err)
	}
	if gotB != 3 {
		t.Fatalf("甲全部已读不影响乙：乙未读数应仍为 3，实际 %d", gotB)
	}
}

// TestNoticeSync_PlatformDisabledSkips 钉死离线形态不发起平台请求。
//
// PLATFORM_ENABLED=false 时整个同步跳过 —— 离线单机部署不该在日志里
// 留平台不可达的噪音，更不该产生任何出站请求。
func TestNoticeSync_PlatformDisabledSkips(t *testing.T) {
	database := setupNoticeTestDB(t)

	svc := &NoticeSyncService{
		notifRepo: repository.NewNotificationRepositoryWithDB(database),
		tenantRep: repository.NewTenantRepository(),
		client:    nil, // 跳过路径下不该被用到
	}

	t.Setenv("PLATFORM_ENABLED", "false")

	res, err := svc.Sync(context.Background())
	if err != nil {
		t.Fatalf("平台联动关闭时同步不应报错: %v", err)
	}
	if !res.Skipped {
		t.Fatal("平台联动关闭时 res.Skipped 应为 true")
	}
	if res.Fetched != 0 || res.Delivered != 0 {
		t.Fatalf("跳过时不应有拉取/投递计数，实际 fetched=%d delivered=%d", res.Fetched, res.Delivered)
	}
}
