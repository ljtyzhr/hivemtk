package repository

import (
	"context"
	"testing"

	"hivemtk-user/internal/content/model"
	"hivemtk-user/internal/pkg/db"

	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

func setupScriptTemplateTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.ScriptTemplate{},
		&model.ScriptCategory{},
	)
	db.SetTestDB(database)
	return database
}

func setupScriptTemplateRepositories(t *testing.T) (*ScriptTemplateRepository, *ScriptCategoryRepository) {
	database := setupScriptTemplateTestDB(t)

	return NewScriptTemplateRepository(database), NewScriptCategoryRepository(database)
}

// TestScriptTemplateRepository_Create 测试创建话术模板
func TestScriptTemplateRepository_Create(t *testing.T) {
	templateRepo, _ := setupScriptTemplateRepositories(t)

	tests := []struct {
		name     string
		template *model.ScriptTemplate
		wantErr  bool
	}{
		{
			name: "create template success",
			template: &model.ScriptTemplate{
				Category:  "sales",
				Title:     "Sales Pitch Template",
				Content:   "Hello, I'd like to introduce our product...",
				Variables: "[\"customer_name\", \"product_name\"]",
				Tags:      "sales,_pitch,intro",
				IsPublic:  false,
				IsSystem:  false,
				CreatedBy: 1,
			},
			wantErr: false,
		},
		{
			name: "create public template",
			template: &model.ScriptTemplate{
				Category: "support",
				Title:    "Customer Support Template",
				Content:  "Thank you for contacting us...",
				IsPublic: true,
				IsSystem: false,
			},
			wantErr: false,
		},
		{
			name: "create system template",
			template: &model.ScriptTemplate{
				Category: "general",
				Title:    "System Template",
				Content:  "System default content...",
				IsPublic: true,
				IsSystem: true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := templateRepo.Create(context.Background(), tt.template)

			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && tt.template.ID == 0 {
				t.Error("Expected template ID to be set after creation")
			}
		})
	}
}

// TestScriptTemplateRepository_GetByID 测试根据 ID 获取话术模板
func TestScriptTemplateRepository_GetByID(t *testing.T) {
	templateRepo, _ := setupScriptTemplateRepositories(t)

	template := &model.ScriptTemplate{
		Category: "sales",
		Title:    "GetByID Template",
		Content:  "Test content...",
	}
	templateRepo.Create(context.Background(), template)

	tests := []struct {
		name    string
		id      uint
		wantErr bool
	}{
		{
			name:    "get existing template",
			id:      template.ID,
			wantErr: false,
		},
		{
			name:    "get non-existing template",
			id:      99999,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := templateRepo.GetByID(context.Background(), tt.id)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetByID() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if result.Title != "GetByID Template" {
					t.Errorf("Expected title 'GetByID Template', got '%s'", result.Title)
				}
			}
		})
	}
}

// TestScriptTemplateRepository_Update 测试更新话术模板
func TestScriptTemplateRepository_Update(t *testing.T) {
	templateRepo, _ := setupScriptTemplateRepositories(t)

	template := &model.ScriptTemplate{
		Category:   "sales",
		Title:      "Original Title",
		Content:    "Original content...",
		UsageCount: 0,
		Rating:     0,
	}
	templateRepo.Create(context.Background(), template)

	template.Title = "Updated Title"
	template.Content = "Updated content..."
	template.Rating = 4.5

	err := templateRepo.Update(context.Background(), template)
	if err != nil {
		t.Errorf("Update() error = %v", err)
	}

	updated, _ := templateRepo.GetByID(context.Background(), template.ID)
	if updated.Title != "Updated Title" {
		t.Errorf("Expected title 'Updated Title', got '%s'", updated.Title)
	}
	if updated.Rating != 4.5 {
		t.Errorf("Expected rating 4.5, got %f", updated.Rating)
	}
}

// TestScriptTemplateRepository_Delete 测试删除话术模板
func TestScriptTemplateRepository_Delete(t *testing.T) {
	templateRepo, _ := setupScriptTemplateRepositories(t)

	template := &model.ScriptTemplate{
		Category: "sales",
		Title:    "To Delete",
		Content:  "Delete content...",
	}
	templateRepo.Create(context.Background(), template)

	err := templateRepo.Delete(context.Background(), template.ID)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	_, err = templateRepo.GetByID(context.Background(), template.ID)
	if err == nil {
		t.Error("Expected template to be deleted")
	}
}

// TestScriptTemplateRepository_IncrementUsage 测试增加使用次数
func TestScriptTemplateRepository_IncrementUsage(t *testing.T) {
	templateRepo, _ := setupScriptTemplateRepositories(t)

	template := &model.ScriptTemplate{
		Category:   "sales",
		Title:      "Usage Test",
		Content:    "Content...",
		UsageCount: 0,
	}
	templateRepo.Create(context.Background(), template)

	for i := 1; i <= 3; i++ {
		err := templateRepo.IncrementUsage(context.Background(), template.ID)
		if err != nil {
			t.Errorf("IncrementUsage() error = %v", err)
		}
	}

	updated, _ := templateRepo.GetByID(context.Background(), template.ID)
	if updated.UsageCount != 3 {
		t.Errorf("Expected UsageCount 3, got %d", updated.UsageCount)
	}
}

// TestScriptTemplateRepository_GetPublicTemplates 测试获取公开话术模板
func TestScriptTemplateRepository_GetPublicTemplates(t *testing.T) {
	templateRepo, _ := setupScriptTemplateRepositories(t)

	for i := 1; i <= 3; i++ {
		templateRepo.Create(context.Background(), &model.ScriptTemplate{
			Category:   "general",
			Title:      "Public Template " + string(rune('0'+i)),
			Content:    "Public content...",
			IsPublic:   true,
			UsageCount: i * 10,
		})
	}

	templateRepo.Create(context.Background(), &model.ScriptTemplate{
		Category: "general",
		Title:    "Private Template",
		Content:  "Private content...",
		IsPublic: false,
	})

	results, total, err := templateRepo.GetPublicTemplates(context.Background(), 1, 10)
	if err != nil {
		t.Errorf("GetPublicTemplates() error = %v", err)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 public templates, got %d", len(results))
	}

	if total != 3 {
		t.Errorf("Expected total 3, got %d", total)
	}
}

// TestScriptTemplateRepository_SearchTemplates 测试搜索话术模板
func TestScriptTemplateRepository_SearchTemplates(t *testing.T) {
	templateRepo, _ := setupScriptTemplateRepositories(t)

	templateRepo.Create(context.Background(), &model.ScriptTemplate{
		Category: "sales",
		Title:    "Welcome Message",
		Content:  "Welcome to our store! How can I help you?",
		IsPublic: true,
	})

	templateRepo.Create(context.Background(), &model.ScriptTemplate{
		Category: "support",
		Title:    "Problem Resolution",
		Content:  "I understand your problem. Let me help you...",
		IsPublic: false,
	})

	templateRepo.Create(context.Background(), &model.ScriptTemplate{
		Category: "sales",
		Title:    "Product Introduction",
		Content:  "Our product features...",
		IsPublic: true,
	})

	tests := []struct {
		name       string
		merchantID string
		keyword    string
		wantCount  int
	}{
		{
			name: "search by title keyword",

			keyword:   "Welcome",
			wantCount: 1,
		},
		{
			name: "search by content keyword",

			keyword:   "help",
			wantCount: 2,
		},
		{
			name: "search with no results",

			keyword:   "nonexistent",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, _, err := templateRepo.SearchTemplates(context.Background(), tt.keyword, 1, 100)

			if err != nil {
				t.Errorf("SearchTemplates() error = %v", err)
			}

			if len(results) != tt.wantCount {
				t.Errorf("Expected %d results, got %d", tt.wantCount, len(results))
			}
		})
	}
}

// TestScriptCategoryRepository_Create 测试创建分类
func TestScriptCategoryRepository_Create(t *testing.T) {
	_, categoryRepo := setupScriptTemplateRepositories(t)

	tests := []struct {
		name     string
		category *model.ScriptCategory
		wantErr  bool
	}{
		{
			name: "create category success",
			category: &model.ScriptCategory{
				Name:      "Sales Scripts",
				ParentID:  0,
				SortOrder: 1,
			},
			wantErr: false,
		},
		{
			name: "create system category (empty merchant)",
			category: &model.ScriptCategory{
				Name:      "General Scripts",
				ParentID:  0,
				SortOrder: 0,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := categoryRepo.Create(context.Background(), tt.category)

			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && tt.category.ID == 0 {
				t.Error("Expected category ID to be set after creation")
			}
		})
	}
}

// TestScriptCategoryRepository_Update 测试更新分类
func TestScriptCategoryRepository_Update(t *testing.T) {
	_, categoryRepo := setupScriptTemplateRepositories(t)

	category := &model.ScriptCategory{
		Name:      "Original Name",
		SortOrder: 1,
	}
	categoryRepo.Create(context.Background(), category)

	category.Name = "Updated Name"
	category.SortOrder = 10

	err := categoryRepo.Update(context.Background(), category)
	if err != nil {
		t.Errorf("Update() error = %v", err)
	}

	var updated model.ScriptCategory
	db.GetDB().First(&updated, category.ID)
	if updated.Name != "Updated Name" {
		t.Errorf("Expected name 'Updated Name', got '%s'", updated.Name)
	}
}

// TestScriptCategoryRepository_Delete 测试删除分类
func TestScriptCategoryRepository_Delete(t *testing.T) {
	_, categoryRepo := setupScriptTemplateRepositories(t)

	category := &model.ScriptCategory{
		Name:      "To Delete",
		SortOrder: 1,
	}
	categoryRepo.Create(context.Background(), category)

	err := categoryRepo.Delete(context.Background(), category.ID)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	var count int64
	db.GetDB().Model(&model.ScriptCategory{}).Where("id = ?", category.ID).Count(&count)
	if count != 0 {
		t.Error("Expected category to be deleted")
	}
}
