package repository

import (
	"context"
	"hivemtk-user/internal/model"
	"testing"
	"time"

	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/pkg/timeutil"

	"gorm.io/gorm"
)

func setupLiveCodeQRTestDB(t *testing.T) *gorm.DB {
	return testutil.NewTestDB(t,
		&model.LiveCodeQR{},
		&model.LiveCodeQRStat{},
	)
}

func setupLiveCodeQRRepository(t *testing.T) LiveCodeQRRepository {
	repo, _ := setupLiveCodeQRRepositoryWithDB(t)
	return repo
}

func setupLiveCodeQRRepositoryWithDB(t *testing.T) (LiveCodeQRRepository, *gorm.DB) {
	t.Helper()
	db := setupLiveCodeQRTestDB(t)
	return NewLiveCodeQRRepository(db), db
}

// seedQRStat 造一行日桶。生产侧唯一的写口是 incrementStat（先 UPDATE 再 INSERT），
// 仓储不再对外暴露"整行造数"方法，测试直接用句柄插入。
func seedQRStat(t *testing.T, db *gorm.DB, stat *model.LiveCodeQRStat) {
	t.Helper()
	if err := db.Create(stat).Error; err != nil {
		t.Fatalf("seed LiveCodeQRStat: %v", err)
	}
}

// TestLiveCodeQRRepository_Create 测试创建二维码
func TestLiveCodeQRRepository_Create(t *testing.T) {
	ctx := context.Background()
	repo := setupLiveCodeQRRepository(t)

	tests := []struct {
		name    string
		qrCode  *model.LiveCodeQR
		wantErr bool
	}{
		{
			name: "create qr code success",
			qrCode: &model.LiveCodeQR{
				LiveCodeID: "live-code-123",
				QRType:     "wechat",
				QRContent:  "https://example.com/qr1",
				QRTitle:    "Test QR Code",
				Priority:   1,
				DailyLimit: 200,
				ExpireDays: 7,
				Status:     1,
			},
			wantErr: false,
		},
		{
			name: "create qr code with image",
			qrCode: &model.LiveCodeQR{
				LiveCodeID: "live-code-123",
				QRType:     "alipay",
				QRContent:  "https://example.com/qr2",
				QRTitle:    "Alipay QR",
				ImageURL:   "https://example.com/image.png",
				Priority:   2,
				DailyLimit: 100,
				Status:     1,
			},
			wantErr: false,
		},
		{
			name: "create disabled qr code",
			qrCode: &model.LiveCodeQR{
				LiveCodeID: "live-code-456",
				QRType:     "wechat",
				QRContent:  "https://example.com/qr3",
				QRTitle:    "Disabled QR",
				Status:     1,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.Create(ctx, tt.qrCode)

			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && tt.qrCode.ID == "" {
				t.Error("Expected QR code ID to be set after creation")
			}
		})
	}
}

// TestLiveCodeQRRepository_Update 测试更新二维码
func TestLiveCodeQRRepository_Update(t *testing.T) {
	ctx := context.Background()
	repo := setupLiveCodeQRRepository(t)

	qrCode := &model.LiveCodeQR{
		LiveCodeID: "live-code-123",
		QRType:     "wechat",
		QRContent:  "https://example.com/original",
		QRTitle:    "Original Title",
		Priority:   1,
		Status:     1,
	}
	repo.Create(ctx, qrCode)

	qrCode.QRTitle = "Updated Title"
	qrCode.Priority = 5
	qrCode.Status = 0

	err := repo.Update(ctx, qrCode)
	if err != nil {
		t.Errorf("Update() error = %v", err)
	}

	updated, _ := repo.GetByID(ctx, qrCode.ID)
	if updated.QRTitle != "Updated Title" {
		t.Errorf("Expected QRTitle 'Updated Title', got '%s'", updated.QRTitle)
	}
	if updated.Priority != 5 {
		t.Errorf("Expected Priority 5, got %d", updated.Priority)
	}
	if updated.Status != 0 {
		t.Errorf("Expected Status 0, got %d", updated.Status)
	}
}

// TestLiveCodeQRRepository_Delete 测试删除二维码
func TestLiveCodeQRRepository_Delete(t *testing.T) {
	ctx := context.Background()
	repo := setupLiveCodeQRRepository(t)

	qrCode := &model.LiveCodeQR{
		LiveCodeID: "live-code-123",
		QRType:     "wechat",
		QRContent:  "https://example.com/to-delete",
		QRTitle:    "To Delete",
		Status:     1,
	}
	repo.Create(ctx, qrCode)

	err := repo.Delete(ctx, qrCode.ID)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	_, err = repo.GetByID(ctx, qrCode.ID)
	if err == nil {
		t.Error("Expected QR code to be deleted")
	}
}

// TestLiveCodeQRRepository_GetByID 测试根据 ID 获取二维码
func TestLiveCodeQRRepository_GetByID(t *testing.T) {
	ctx := context.Background()
	repo := setupLiveCodeQRRepository(t)

	qrCode := &model.LiveCodeQR{
		LiveCodeID: "live-code-123",
		QRType:     "wechat",
		QRContent:  "https://example.com/getbyid",
		QRTitle:    "GetByID QR",
		Status:     1,
	}
	repo.Create(ctx, qrCode)

	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{
			name:    "get existing qr code",
			id:      qrCode.ID,
			wantErr: false,
		},
		{
			name:    "get non-existing qr code",
			id:      "non-existing-id",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := repo.GetByID(ctx, tt.id)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetByID() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if result.QRTitle != "GetByID QR" {
					t.Errorf("Expected QRTitle 'GetByID QR', got '%s'", result.QRTitle)
				}
			}
		})
	}
}

// TestLiveCodeQRRepository_GetByLiveCodeID 测试根据活码 ID 获取二维码列表
func TestLiveCodeQRRepository_GetByLiveCodeID(t *testing.T) {
	ctx := context.Background()
	repo := setupLiveCodeQRRepository(t)

	for i := 1; i <= 5; i++ {
		repo.Create(ctx, &model.LiveCodeQR{
			LiveCodeID: "live-code-123",
			QRType:     "wechat",
			QRContent:  "https://example.com/qr" + string(rune('0'+i)),
			QRTitle:    "QR Code " + string(rune('0'+i)),
			Priority:   i,
			Status:     1,
		})
	}

	repo.Create(ctx, &model.LiveCodeQR{
		LiveCodeID: "live-code-456",
		QRType:     "alipay",
		QRContent:  "https://example.com/other",
		QRTitle:    "Other QR",
		Status:     1,
	})

	results, err := repo.GetByLiveCodeID(context.Background(), "live-code-123")
	if err != nil {
		t.Errorf("GetByLiveCodeID() error = %v", err)
	}

	if len(results) != 5 {
		t.Errorf("Expected 5 QR codes, got %d", len(results))
	}

	if results[0].QRTitle != "QR Code 5" {
		t.Errorf("Expected first result to be 'QR Code 5', got '%s'", results[0].QRTitle)
	}
}

// TestLiveCodeQRRepository_GetStats 测试获取访问统计
func TestLiveCodeQRRepository_GetStats(t *testing.T) {
	ctx := context.Background()
	repo, db := setupLiveCodeQRRepositoryWithDB(t)

	now := time.Now()
	seedQRStat(t, db, &model.LiveCodeQRStat{
		QRCodeID:   "qr-code-123",
		Date:       now,
		ViewCount:  100,
		ClickCount: 50,
	})

	seedQRStat(t, db, &model.LiveCodeQRStat{
		QRCodeID:   "qr-code-123",
		Date:       now.AddDate(0, 0, -1),
		ViewCount:  200,
		ClickCount: 80,
	})

	seedQRStat(t, db, &model.LiveCodeQRStat{
		QRCodeID:   "qr-code-123",
		Date:       now.AddDate(0, 0, -2),
		ViewCount:  150,
		ClickCount: 60,
	})

	seedQRStat(t, db, &model.LiveCodeQRStat{
		QRCodeID:   "qr-code-456",
		Date:       now,
		ViewCount:  50,
		ClickCount: 20,
	})

	results, err := repo.GetStats(ctx, "qr-code-123")
	if err != nil {
		t.Errorf("GetStats() error = %v", err)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 stats, got %d", len(results))
	}

	if results[0].ViewCount != 100 {
		t.Errorf("Expected first result ViewCount 100, got %d", results[0].ViewCount)
	}
}

// TestLiveCodeQRRepository_GetStats_Empty 测试获取空统计
func TestLiveCodeQRRepository_GetStats_Empty(t *testing.T) {
	ctx := context.Background()
	repo := setupLiveCodeQRRepository(t)

	results, err := repo.GetStats(ctx, "non-existing-qr")
	if err != nil {
		t.Errorf("GetStats() error = %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected 0 stats, got %d", len(results))
	}
}

// TestLiveCodeQRRepository_SumLiveCodeStats 按活码汇总二维码展示/点击。
//
// 这条用例钉的是一个曾经必然报错的子查询：原实现从 live_code_qrs 里选 qr_code_id
// （那张表的列是 id），PG 直接 column does not exist；而唯一调用方
// service/live_code.go 把它包在 `if ...; err == nil` 里，错误被吞掉、统计恒为 0。
func TestLiveCodeQRRepository_SumLiveCodeStats(t *testing.T) {
	ctx := context.Background()
	repo, db := setupLiveCodeQRRepositoryWithDB(t)

	for _, qr := range []*model.LiveCodeQR{
		{ID: "qr-a", LiveCodeID: "lc-1", QRType: "wechat", QRContent: "https://e/a", Status: 1},
		{ID: "qr-b", LiveCodeID: "lc-1", QRType: "wechat", QRContent: "https://e/b", Status: 1},
		{ID: "qr-other", LiveCodeID: "lc-2", QRType: "wechat", QRContent: "https://e/c", Status: 1},
	} {
		if err := db.Create(qr).Error; err != nil {
			t.Fatalf("seed LiveCodeQR: %v", err)
		}
	}

	day := timeutil.StartOfBusinessDay(time.Now())
	for _, stat := range []*model.LiveCodeQRStat{
		{QRCodeID: "qr-a", Date: day, ViewCount: 3, ClickCount: 1},
		{QRCodeID: "qr-b", Date: day, ViewCount: 4, ClickCount: 2},
		{QRCodeID: "qr-other", Date: day, ViewCount: 100, ClickCount: 100},
	} {
		seedQRStat(t, db, stat)
	}

	views, clicks, err := repo.SumLiveCodeStats(ctx, "lc-1")
	if err != nil {
		t.Fatalf("SumLiveCodeStats: %v", err)
	}
	if views != 7 || clicks != 3 {
		t.Fatalf("expected views=7 clicks=3, got views=%d clicks=%d", views, clicks)
	}

	emptyViews, emptyClicks, err := repo.SumLiveCodeStats(ctx, "lc-none")
	if err != nil {
		t.Fatalf("SumLiveCodeStats(不存在的活码): %v", err)
	}
	if emptyViews != 0 || emptyClicks != 0 {
		t.Fatalf("expected 0/0, got views=%d clicks=%d", emptyViews, emptyClicks)
	}
}

// TestLiveCodeQRRepository_IncrementStat 当日首点造日桶、再点累加同一行
func TestLiveCodeQRRepository_IncrementStat(t *testing.T) {
	ctx := context.Background()
	repo, db := setupLiveCodeQRRepositoryWithDB(t)

	if err := repo.IncrementViewStat(ctx, "qr-a"); err != nil {
		t.Fatalf("IncrementViewStat: %v", err)
	}
	if err := repo.IncrementViewStat(ctx, "qr-a"); err != nil {
		t.Fatalf("IncrementViewStat(第二次): %v", err)
	}
	if err := repo.IncrementClickStat(ctx, "qr-a"); err != nil {
		t.Fatalf("IncrementClickStat: %v", err)
	}

	var stats []model.LiveCodeQRStat
	if err := db.Where("qr_code_id = ?", "qr-a").Find(&stats).Error; err != nil {
		t.Fatalf("读回统计行: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("同一天应只有一行日桶，实际 %d 行", len(stats))
	}
	if stats[0].ViewCount != 2 || stats[0].ClickCount != 1 {
		t.Fatalf("expected view=2 click=1, got view=%d click=%d", stats[0].ViewCount, stats[0].ClickCount)
	}

	views, clicks, err := repo.SumStats(ctx, "qr-a")
	if err != nil {
		t.Fatalf("SumStats: %v", err)
	}
	if views != 2 || clicks != 1 {
		t.Fatalf("expected 2/1, got %d/%d", views, clicks)
	}
}
