package repository

// 行为回归（TENANT_MODE=off，本地独立部署形态）。
// 这批用例从 internal/ops/repository/dashboard_screen_test.go 随仓储一起迁到 content 域，
// 只按新签名补 ctx 形参，断言口径与迁前逐条一致。

import (
	"context"
	"testing"

	contentmodel "hivemtk-user/internal/content/model"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

func mtOffDB(t *testing.T) (*gorm.DB, *MarketTemplateRepository, *MarketTemplateDownloadRepository) {
	t.Helper()
	gdb := testutil.NewTestDB(t, &contentmodel.MarketTemplate{}, &contentmodel.MarketTemplateDownload{})
	if gdb == nil {
		t.Skip("测试库不可达")
	}
	return gdb, NewMarketTemplateRepository(gdb), NewMarketTemplateDownloadRepository(gdb)
}

func TestMarketTemplateRepository_GetList(t *testing.T) {
	gdb, templateRepo, _ := mtOffDB(t)

	for i := 1; i <= 5; i++ {
		gdb.Create(&contentmodel.MarketTemplate{
			Name:          "Template " + string(rune('0'+i)),
			Category:      "dashboard",
			Type:          "free",
			DownloadCount: i * 10,
		})
	}

	tests := []struct {
		name         string
		category     string
		templateType string
		page         int
		pageSize     int
		wantCount    int
		wantTotal    int64
	}{
		{"get all templates", "", "", 1, 10, 5, 5},
		{"filter by category", "dashboard", "", 1, 10, 5, 5},
		{"filter by type", "", "free", 1, 10, 5, 5},
		{"pagination first page", "", "", 1, 3, 3, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, total, err := templateRepo.GetList(context.Background(), tt.category, tt.templateType, tt.page, tt.pageSize)
			if err != nil {
				t.Errorf("GetList() error = %v", err)
			}
			if len(results) != tt.wantCount {
				t.Errorf("Expected %d results, got %d", tt.wantCount, len(results))
			}
			if total != tt.wantTotal {
				t.Errorf("Expected total %d, got %d", tt.wantTotal, total)
			}
		})
	}
}

func TestMarketTemplateRepository_GetByID(t *testing.T) {
	gdb, templateRepo, _ := mtOffDB(t)

	template := &contentmodel.MarketTemplate{
		Name:     "GetByID Template",
		Category: "dashboard",
		Type:     "free",
	}
	gdb.Create(template)

	tests := []struct {
		name    string
		id      uint
		wantErr bool
	}{
		{"get existing template", template.ID, false},
		{"get non-existing template", 99999, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := templateRepo.GetByID(context.Background(), tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetByID() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && result.Name != "GetByID Template" {
				t.Errorf("Expected name 'GetByID Template', got '%s'", result.Name)
			}
		})
	}
}

func TestMarketTemplateRepository_IncrementDownload(t *testing.T) {
	gdb, templateRepo, _ := mtOffDB(t)

	template := &contentmodel.MarketTemplate{
		Name:          "Download Test",
		Category:      "dashboard",
		DownloadCount: 0,
	}
	gdb.Create(template)

	for i := 1; i <= 5; i++ {
		if err := templateRepo.IncrementDownload(context.Background(), template.ID); err != nil {
			t.Errorf("IncrementDownload() error = %v", err)
		}
	}

	updated, err := templateRepo.GetByID(context.Background(), template.ID)
	if err != nil {
		t.Fatalf("回读失败: %v", err)
	}
	if updated.DownloadCount != 5 {
		t.Errorf("Expected DownloadCount 5, got %d", updated.DownloadCount)
	}
}

func TestMarketTemplateRepository_GetOfficialTemplates(t *testing.T) {
	gdb, templateRepo, _ := mtOffDB(t)

	for i := 1; i <= 3; i++ {
		gdb.Create(&contentmodel.MarketTemplate{
			Name:       "Official Template " + string(rune('0'+i)),
			Category:   "dashboard",
			IsOfficial: true,
		})
	}
	gdb.Create(&contentmodel.MarketTemplate{
		Name:       "Community Template",
		Category:   "dashboard",
		IsOfficial: false,
	})

	results, total, err := templateRepo.GetOfficialTemplates(context.Background(), 1, 10)
	if err != nil {
		t.Errorf("GetOfficialTemplates() error = %v", err)
	}
	if len(results) != 3 {
		t.Errorf("Expected 3 official templates, got %d", len(results))
	}
	if total != 3 {
		t.Errorf("Expected total 3, got %d", total)
	}
}

func TestMarketTemplateRepository_SearchTemplates(t *testing.T) {
	gdb, templateRepo, _ := mtOffDB(t)

	gdb.Create(&contentmodel.MarketTemplate{
		Name:        "Sales Dashboard",
		Description: "Dashboard for sales tracking",
		Category:    "dashboard",
	})
	gdb.Create(&contentmodel.MarketTemplate{
		Name:        "Marketing Report",
		Description: "Marketing analytics dashboard",
		Category:    "dashboard",
	})
	gdb.Create(&contentmodel.MarketTemplate{
		Name:        "Inventory Manager",
		Description: "Inventory tracking system",
		Category:    "inventory",
	})

	tests := []struct {
		name      string
		keyword   string
		wantCount int
	}{
		{"search by name", "Sales", 1},
		{"search by description", "dashboard", 2},
		{"search with no results", "nonexistent", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, _, err := templateRepo.SearchTemplates(context.Background(), tt.keyword, 1, 10)
			if err != nil {
				t.Errorf("SearchTemplates() error = %v", err)
			}
			if len(results) != tt.wantCount {
				t.Errorf("Expected %d results, got %d", tt.wantCount, len(results))
			}
		})
	}
}

func TestMarketTemplateDownloadRepository_Create(t *testing.T) {
	_, _, downloadRepo := mtOffDB(t)

	record := &contentmodel.MarketTemplateDownload{
		UserID:       7,
		TemplateID:   1,
		TemplateType: "dashboard",
	}

	if err := downloadRepo.Create(context.Background(), record); err != nil {
		t.Errorf("Create() error = %v", err)
	}
	if record.ID == 0 {
		t.Error("Expected download record ID to be set after creation")
	}
}

func TestMarketTemplateDownloadRepository_GetByUserPaging(t *testing.T) {
	gdb, _, downloadRepo := mtOffDB(t)

	for i := 1; i <= 5; i++ {
		gdb.Create(&contentmodel.MarketTemplateDownload{UserID: 7, TemplateID: uint(i), TemplateType: "flow"})
	}
	gdb.Create(&contentmodel.MarketTemplateDownload{UserID: 8, TemplateID: 99, TemplateType: "flow"})

	records, total, err := downloadRepo.GetByUser(context.Background(), 7, 1, 3)
	if err != nil {
		t.Fatalf("GetByUser() error = %v", err)
	}
	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}
	if len(records) != 3 {
		t.Errorf("Expected 3 records on page 1, got %d", len(records))
	}
}
