package repository

import (
	"hivemtk-user/internal/ops/model"
	"hivemtk-user/internal/pkg/db"
	"testing"

	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

func setupDashboardTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.DashboardScreen{},
		&model.DashboardWidget{},
	)
	db.SetTestDB(database)
	return database
}

func setupDashboardRepositories(t *testing.T) (*DashboardScreenRepository, *DashboardWidgetRepository) {
	setupDashboardTestDB(t)

	return &DashboardScreenRepository{db: db.GetDB()},
		&DashboardWidgetRepository{db: db.GetDB()}
}

// TestDashboardScreenRepository_Create 测试创建大屏
func TestDashboardScreenRepository_Create(t *testing.T) {
	screenRepo, _ := setupDashboardRepositories(t)

	tests := []struct {
		name    string
		screen  *model.DashboardScreen
		wantErr bool
	}{
		{
			name: "create screen success",
			screen: &model.DashboardScreen{
				Name:      "Test Dashboard",
				Code:      "test_dashboard_001",
				Layout:    `{"grid": "12x12"}`,
				Widgets:   `[]`,
				Theme:     "dark",
				IsPublic:  false,
				CreatedBy: 1,
			},
			wantErr: false,
		},
		{
			name: "create screen with minimal fields",
			screen: &model.DashboardScreen{
				Name: "Minimal Dashboard",
				Code: "minimal_001",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := screenRepo.Create(tt.screen)

			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && tt.screen.ID == 0 {
				t.Error("Expected screen ID to be set after creation")
			}
		})
	}
}

// TestDashboardScreenRepository_GetByID 测试根据 ID 获取大屏
func TestDashboardScreenRepository_GetByID(t *testing.T) {
	screenRepo, _ := setupDashboardRepositories(t)

	screen := &model.DashboardScreen{
		Name:      "GetByID Dashboard",
		Code:      "getbyid_001",
		Theme:     "dark",
		CreatedBy: 1,
	}
	screenRepo.Create(screen)

	tests := []struct {
		name    string
		id      uint
		wantErr bool
	}{
		{
			name:    "get existing screen",
			id:      screen.ID,
			wantErr: false,
		},
		{
			name:    "get non-existing screen",
			id:      99999,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := screenRepo.GetByID(tt.id)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetByID() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if result.Name != "GetByID Dashboard" {
					t.Errorf("Expected name 'GetByID Dashboard', got '%s'", result.Name)
				}
			}
		})
	}
}

// TestDashboardScreenRepository_GetByCode 测试根据编码获取大屏
func TestDashboardScreenRepository_GetByCode(t *testing.T) {
	screenRepo, _ := setupDashboardRepositories(t)

	screen := &model.DashboardScreen{
		Name:     "Code Test Dashboard",
		Code:     "unique_code_123",
		IsPublic: true,
	}
	screenRepo.Create(screen)

	tests := []struct {
		name    string
		code    string
		wantErr bool
	}{
		{
			name:    "get existing code",
			code:    "unique_code_123",
			wantErr: false,
		},
		{
			name:    "get non-existing code",
			code:    "non_existing_code",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := screenRepo.GetByCode(tt.code)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetByCode() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if result.Code != "unique_code_123" {
					t.Errorf("Expected code 'unique_code_123', got '%s'", result.Code)
				}
			}
		})
	}
}

// TestDashboardScreenRepository_Update 测试更新大屏
func TestDashboardScreenRepository_Update(t *testing.T) {
	screenRepo, _ := setupDashboardRepositories(t)

	screen := &model.DashboardScreen{
		Name:  "Original Name",
		Code:  "original_code",
		Theme: "light",
	}
	screenRepo.Create(screen)

	screen.Name = "Updated Name"
	screen.Theme = "dark"

	err := screenRepo.Update(screen)
	if err != nil {
		t.Errorf("Update() error = %v", err)
	}

	updated, _ := screenRepo.GetByID(screen.ID)
	if updated.Name != "Updated Name" {
		t.Errorf("Expected name 'Updated Name', got '%s'", updated.Name)
	}
	if updated.Theme != "dark" {
		t.Errorf("Expected theme 'dark', got '%s'", updated.Theme)
	}
}

// TestDashboardScreenRepository_Delete 测试删除大屏
func TestDashboardScreenRepository_Delete(t *testing.T) {
	screenRepo, _ := setupDashboardRepositories(t)

	screen := &model.DashboardScreen{
		Name: "To Delete",
		Code: "delete_code",
	}
	screenRepo.Create(screen)

	err := screenRepo.Delete(screen.ID)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	_, err = screenRepo.GetByID(screen.ID)
	if err == nil {
		t.Error("Expected screen to be deleted")
	}
}

// TestDashboardScreenRepository_IncrementViewCount 测试增加访问次数
func TestDashboardScreenRepository_IncrementViewCount(t *testing.T) {
	screenRepo, _ := setupDashboardRepositories(t)

	screen := &model.DashboardScreen{
		Name:      "View Count Test",
		Code:      "view_test",
		ViewCount: 0,
	}
	screenRepo.Create(screen)

	for i := 1; i <= 3; i++ {
		err := screenRepo.IncrementViewCount(screen.ID)
		if err != nil {
			t.Errorf("IncrementViewCount() error = %v", err)
		}
	}

	updated, _ := screenRepo.GetByID(screen.ID)
	if updated.ViewCount != 3 {
		t.Errorf("Expected ViewCount 3, got %d", updated.ViewCount)
	}
}

// TestDashboardWidgetRepository_Create 测试创建 Widget
func TestDashboardWidgetRepository_Create(t *testing.T) {
	_, widgetRepo := setupDashboardRepositories(t)

	screenRepo := &DashboardScreenRepository{db: db.GetDB()}
	screen := &model.DashboardScreen{
		Name: "Widget Test Dashboard",
		Code: "widget_test",
	}
	screenRepo.Create(screen)

	widget := &model.DashboardWidget{
		ScreenID:   screen.ID,
		WidgetType: "chart",
		Title:      "Test Widget",
		Config:     `{"type": "line"}`,
		SortOrder:  1,
		X:          0,
		Y:          0,
		Width:      6,
		Height:     4,
	}

	err := widgetRepo.Create(widget)
	if err != nil {
		t.Errorf("Create() error = %v", err)
	}

	if widget.ID == 0 {
		t.Error("Expected widget ID to be set after creation")
	}
}

// TestDashboardWidgetRepository_GetByScreenID 测试根据大屏 ID 获取 Widgets
func TestDashboardWidgetRepository_GetByScreenID(t *testing.T) {
	_, widgetRepo := setupDashboardRepositories(t)

	screenRepo := &DashboardScreenRepository{db: db.GetDB()}
	screen := &model.DashboardScreen{
		Name: "GetWidgets Dashboard",
		Code: "getwidgets_test",
	}
	screenRepo.Create(screen)

	for i := 1; i <= 3; i++ {
		widgetRepo.Create(&model.DashboardWidget{
			ScreenID:   screen.ID,
			WidgetType: "chart",
			Title:      "Widget " + string(rune('0'+i)),
			SortOrder:  i,
		})
	}

	results, err := widgetRepo.GetByScreenID(screen.ID)
	if err != nil {
		t.Errorf("GetByScreenID() error = %v", err)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 widgets, got %d", len(results))
	}
}

// TestDashboardWidgetRepository_DeleteByScreenID 测试删除大屏下所有 Widgets
func TestDashboardWidgetRepository_DeleteByScreenID(t *testing.T) {
	_, widgetRepo := setupDashboardRepositories(t)

	screenRepo := &DashboardScreenRepository{db: db.GetDB()}
	screen := &model.DashboardScreen{
		Name: "Delete Widgets Dashboard",
		Code: "delete_widgets_test",
	}
	screenRepo.Create(screen)

	for i := 1; i <= 3; i++ {
		widgetRepo.Create(&model.DashboardWidget{
			ScreenID:   screen.ID,
			WidgetType: "chart",
			Title:      "Widget " + string(rune('0'+i)),
		})
	}

	err := widgetRepo.DeleteByScreenID(screen.ID)
	if err != nil {
		t.Errorf("DeleteByScreenID() error = %v", err)
	}

	results, _ := widgetRepo.GetByScreenID(screen.ID)
	if len(results) != 0 {
		t.Errorf("Expected 0 widgets after deletion, got %d", len(results))
	}
}
