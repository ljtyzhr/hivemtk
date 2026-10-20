package repository

import (
	"context"
	"hivemtk-user/internal/content/model"
	"testing"

	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

func setupAIContentTestDB(t *testing.T) *gorm.DB {
	return testutil.NewTestDB(t,
		&model.AIGenerationRecord{},
		&model.PromptTemplate{},
	)
}

func setupAIContentRepositories(t *testing.T) (AIGenerationRecordRepository, PromptTemplateRepository) {
	database := setupAIContentTestDB(t)

	aiRepo := NewAIGenerationRecordRepository(database)
	promptRepo := NewPromptTemplateRepository(database)

	return aiRepo, promptRepo
}

// aiTestCtx 本文件全部用例跑在 off 模式（隔离 callback 直通），
// ctx 在这里只是把参数补齐、不承担租户语义；on 模式的真实读数由 ai_content_tenant_test.go 钉。
var aiTestCtx = context.Background()

// TestAIGenerationRecordRepository_Create 测试创建生成记录
func TestAIGenerationRecordRepository_Create(t *testing.T) {
	aiRepo, _ := setupAIContentRepositories(t)

	tests := []struct {
		name    string
		record  *model.AIGenerationRecord
		wantErr bool
	}{
		{
			name: "create record success",
			record: &model.AIGenerationRecord{
				UserID:     1,
				Type:       model.AIGenerationTypeCopywriting,
				Input:      "Test input",
				Output:     "Test output",
				Model:      "gpt-4",
				TokensUsed: 100,
			},
			wantErr: false,
		},
		{
			name: "create record with rating",
			record: &model.AIGenerationRecord{
				UserID: 2,
				Type:   model.AIGenerationTypeTitle,
				Input:  "Generate title",
				Output: "Amazing Title",
				Rating: 5,
			},
			wantErr: false,
		},
		{
			name: "create record with saved and favorite",
			record: &model.AIGenerationRecord{
				UserID:     3,
				Type:       model.AIGenerationTypeReply,
				Input:      "Customer question",
				Output:     "Professional reply",
				IsSaved:    true,
				IsFavorite: true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := aiRepo.Create(aiTestCtx, tt.record)

			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && tt.record.ID == 0 {
				t.Error("Expected record ID to be set after creation")
			}
		})
	}
}

// TestAIGenerationRecordRepository_GetByID 测试根据 ID 获取记录
func TestAIGenerationRecordRepository_GetByID(t *testing.T) {
	aiRepo, _ := setupAIContentRepositories(t)

	record := &model.AIGenerationRecord{
		UserID: 1,
		Type:   model.AIGenerationTypeCopywriting,
		Input:  "GetByID test input",
		Output: "GetByID test output",
	}
	aiRepo.Create(aiTestCtx, record)

	tests := []struct {
		name    string
		id      uint
		wantErr bool
	}{
		{
			name:    "get existing record",
			id:      record.ID,
			wantErr: false,
		},
		{
			name:    "get non-existing record",
			id:      99999,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := aiRepo.GetByID(aiTestCtx, tt.id, 1)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetByID() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if result.ID != tt.id {
					t.Errorf("Expected ID %d, got %d", tt.id, result.ID)
				}
				if result.Input != "GetByID test input" {
					t.Errorf("Expected input 'GetByID test input', got '%s'", result.Input)
				}
			}
		})
	}
}

func TestAIGenerationRecordRepository_GetByMerchantAndUser(t *testing.T) {
	aiRepo, _ := setupAIContentRepositories(t)

	records := []*model.AIGenerationRecord{
		{UserID: 1, Type: model.AIGenerationTypeCopywriting, Input: "u1-input-1", Output: "u1-output-1"},
		{UserID: 1, Type: model.AIGenerationTypeCopywriting, Input: "u1-input-2", Output: "u1-output-2"},
		{UserID: 1, Type: model.AIGenerationTypeTitle, Input: "u1-input-3", Output: "u1-output-3"},
		{UserID: 2, Type: model.AIGenerationTypeCopywriting, Input: "u2-input-1", Output: "u2-output-1"},
	}
	for _, r := range records {
		if err := aiRepo.Create(aiTestCtx, r); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	list, total, err := aiRepo.GetByMerchantAndUser(aiTestCtx, 1, 1, 10, nil)
	if err != nil {
		t.Fatalf("GetByMerchantAndUser() error = %v", err)
	}
	if total != 3 {
		t.Errorf("Expected total 3 for user 1, got %d", total)
	}
	if len(list) != 3 {
		t.Errorf("Expected 3 records for user 1, got %d", len(list))
	}

	_, total2, err := aiRepo.GetByMerchantAndUser(aiTestCtx, 2, 1, 10, nil)
	if err != nil {
		t.Fatalf("GetByMerchantAndUser() user 2 error = %v", err)
	}
	if total2 != 1 {
		t.Errorf("Expected total 1 for user 2, got %d", total2)
	}

	_, totalWithType, err := aiRepo.GetByMerchantAndUser(aiTestCtx, 1, 1, 10, map[string]any{
		"type": model.AIGenerationTypeCopywriting,
	})
	if err != nil {
		t.Fatalf("GetByMerchantAndUser() with type filter error = %v", err)
	}
	if totalWithType != 2 {
		t.Errorf("Expected total 2 for user 1 with type=copywriting, got %d", totalWithType)
	}

	page1, _, err := aiRepo.GetByMerchantAndUser(aiTestCtx, 1, 1, 2, nil)
	if err != nil {
		t.Fatalf("GetByMerchantAndUser() page 1 error = %v", err)
	}
	if len(page1) != 2 {
		t.Errorf("Expected 2 records on page 1, got %d", len(page1))
	}
	page2, _, err := aiRepo.GetByMerchantAndUser(aiTestCtx, 1, 2, 2, nil)
	if err != nil {
		t.Fatalf("GetByMerchantAndUser() page 2 error = %v", err)
	}
	if len(page2) != 1 {
		t.Errorf("Expected 1 record on page 2, got %d", len(page2))
	}

	empty, totalEmpty, err := aiRepo.GetByMerchantAndUser(aiTestCtx, 999, 1, 10, nil)
	if err != nil {
		t.Fatalf("GetByMerchantAndUser() empty user error = %v", err)
	}
	if totalEmpty != 0 || len(empty) != 0 {
		t.Errorf("Expected empty result for non-existent user, got total=%d len=%d", totalEmpty, len(empty))
	}
}

// TestAIGenerationRecordRepository_UpdateSaved 管理口：只按租户收窄，不认 user_id。
func TestAIGenerationRecordRepository_UpdateSaved(t *testing.T) {
	aiRepo, _ := setupAIContentRepositories(t)

	record := &model.AIGenerationRecord{
		UserID:  1,
		Type:    model.AIGenerationTypeCopywriting,
		Input:   "Save test",
		Output:  "Save output",
		IsSaved: false,
	}
	aiRepo.Create(aiTestCtx, record)

	tests := []struct {
		name    string
		isSaved bool
		wantErr bool
	}{
		{
			name:    "mark as saved",
			isSaved: true,
			wantErr: false,
		},
		{
			name:    "mark as unsaved",
			isSaved: false,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := aiRepo.UpdateSaved(aiTestCtx, record.ID, tt.isSaved)

			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateSaved() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				updated, _ := aiRepo.GetByID(aiTestCtx, record.ID, 1)
				if updated.IsSaved != tt.isSaved {
					t.Errorf("Expected IsSaved=%v, got %v", tt.isSaved, updated.IsSaved)
				}
			}
		})
	}
}

// TestAIGenerationRecordRepository_UpdateFavorite 测试更新收藏状态
func TestAIGenerationRecordRepository_UpdateFavorite(t *testing.T) {
	aiRepo, _ := setupAIContentRepositories(t)

	record := &model.AIGenerationRecord{
		UserID: 1,
		Type:   model.AIGenerationTypeTitle,
		Input:  "Favorite test",
		Output: "Favorite output",
	}
	aiRepo.Create(aiTestCtx, record)

	err := aiRepo.UpdateFavorite(aiTestCtx, record.ID, 1, true)
	if err != nil {
		t.Errorf("UpdateFavorite() error = %v", err)
	}

	updated, _ := aiRepo.GetByID(aiTestCtx, record.ID, 1)
	if !updated.IsFavorite {
		t.Error("Expected IsFavorite to be true")
	}
}

// TestAIGenerationRecordRepository_UpdateRating 测试更新评分
func TestAIGenerationRecordRepository_UpdateRating(t *testing.T) {
	aiRepo, _ := setupAIContentRepositories(t)

	record := &model.AIGenerationRecord{
		UserID: 1,
		Type:   model.AIGenerationTypeReply,
		Input:  "Rating test",
		Output: "Rating output",
	}
	aiRepo.Create(aiTestCtx, record)

	tests := []struct {
		name    string
		rating  int
		wantErr bool
	}{
		{
			name:    "update rating to 5",
			rating:  5,
			wantErr: false,
		},
		{
			name:    "update rating to 3",
			rating:  3,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := aiRepo.UpdateRating(aiTestCtx, record.ID, 1, tt.rating)

			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateRating() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				updated, _ := aiRepo.GetByID(aiTestCtx, record.ID, 1)
				if updated.Rating != tt.rating {
					t.Errorf("Expected Rating=%d, got %d", tt.rating, updated.Rating)
				}
			}
		})
	}
}

// TestAIGenerationRecordRepository_Delete 测试删除记录
func TestAIGenerationRecordRepository_Delete(t *testing.T) {
	aiRepo, _ := setupAIContentRepositories(t)

	record := &model.AIGenerationRecord{
		UserID: 1,
		Type:   model.AIGenerationTypeCopywriting,
		Input:  "Delete test",
		Output: "Delete output",
	}
	aiRepo.Create(aiTestCtx, record)

	err := aiRepo.Delete(aiTestCtx, record.ID)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	_, err = aiRepo.GetByID(aiTestCtx, record.ID, 1)
	if err == nil {
		t.Error("Expected record to be deleted")
	}
}

// TestPromptTemplateRepository_Create 测试创建提示词模板
func TestPromptTemplateRepository_Create(t *testing.T) {
	_, promptRepo := setupAIContentRepositories(t)

	tests := []struct {
		name     string
		template *model.PromptTemplate
		wantErr  bool
	}{
		{
			name: "create template success",
			template: &model.PromptTemplate{
				Name:        "Test Template",
				Type:        model.AIGenerationTypeCopywriting,
				Template:    "Test template content",
				Description: "Test description",
			},
			wantErr: false,
		},
		{
			name: "create system template",
			template: &model.PromptTemplate{
				Name:     "System Template",
				Type:     model.AIGenerationTypeTitle,
				Template: "System template content",
				IsSystem: true,
				Status:   1,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := promptRepo.Create(aiTestCtx, tt.template)

			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && tt.template.ID == 0 {
				t.Error("Expected template ID to be set after creation")
			}
		})
	}
}

// TestPromptTemplateRepository_GetByID 测试根据 ID 获取模板
func TestPromptTemplateRepository_GetByID(t *testing.T) {
	_, promptRepo := setupAIContentRepositories(t)

	template := &model.PromptTemplate{
		Name:        "GetByID Template",
		Type:        model.AIGenerationTypeCopywriting,
		Template:    "GetByID content",
		Description: "GetByID description",
	}
	promptRepo.Create(aiTestCtx, template)

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
			result, err := promptRepo.GetByID(aiTestCtx, tt.id)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetByID() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if result.Name != "GetByID Template" {
					t.Errorf("Expected name 'GetByID Template', got '%s'", result.Name)
				}
			}
		})
	}
}

func TestPromptTemplateRepository_ListByType(t *testing.T) {
	_, promptRepo := setupAIContentRepositories(t)

	promptRepo.Create(aiTestCtx, &model.PromptTemplate{
		Name:     "System Template 1",
		Type:     model.AIGenerationTypeCopywriting,
		Template: "System content 1",
		IsSystem: true,
		Status:   1,
	})

	promptRepo.Create(aiTestCtx, &model.PromptTemplate{
		Name:     "Merchant Template 1",
		Type:     model.AIGenerationTypeCopywriting,
		Template: "Merchant content 1",
		Status:   1,
	})

	promptRepo.Create(aiTestCtx, &model.PromptTemplate{
		Name:     "Merchant Template 2",
		Type:     model.AIGenerationTypeTitle,
		Template: "Merchant content 2",
		Status:   1,
	})

	tests := []struct {
		name         string
		templateType string
		wantCount    int
	}{
		{
			name:         "copywriting type",
			templateType: string(model.AIGenerationTypeCopywriting),
			wantCount:    2,
		},
		{
			name:         "title type",
			templateType: string(model.AIGenerationTypeTitle),
			wantCount:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := promptRepo.ListByType(aiTestCtx, tt.templateType)
			if err != nil {
				t.Errorf("ListByType() error = %v", err)
			}
			if len(results) != tt.wantCount {
				t.Errorf("Expected %d templates, got %d", tt.wantCount, len(results))
			}
		})
	}
}

// TestPromptTemplateRepository_GetByTypeAndName 测试根据类型和名称获取模板
func TestPromptTemplateRepository_GetByTypeAndName(t *testing.T) {
	_, promptRepo := setupAIContentRepositories(t)

	template := &model.PromptTemplate{
		Name:     "Test Template",
		Type:     model.AIGenerationTypeCopywriting,
		Template: "Test content",
	}
	promptRepo.Create(aiTestCtx, template)

	tests := []struct {
		name         string
		tp           model.AIGenerationType
		templateName string
		wantErr      bool
	}{
		{
			name:         "get existing template",
			tp:           model.AIGenerationTypeCopywriting,
			templateName: "Test Template",
			wantErr:      false,
		},
		{
			name:         "get non-existing template",
			tp:           model.AIGenerationTypeCopywriting,
			templateName: "Non-existing",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := promptRepo.GetByTypeAndName(aiTestCtx, tt.tp, tt.templateName)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetByTypeAndName() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if result.Name != tt.templateName {
					t.Errorf("Expected name '%s', got '%s'", tt.templateName, result.Name)
				}
			}
		})
	}
}

// TestPromptTemplateRepository_Update 测试更新模板
func TestPromptTemplateRepository_Update(t *testing.T) {
	_, promptRepo := setupAIContentRepositories(t)

	template := &model.PromptTemplate{
		Name:        "Original Name",
		Type:        model.AIGenerationTypeCopywriting,
		Template:    "Original content",
		Description: "Original description",
	}
	promptRepo.Create(aiTestCtx, template)

	template.Name = "Updated Name"
	template.Template = "Updated content"

	err := promptRepo.Update(aiTestCtx, template)
	if err != nil {
		t.Errorf("Update() error = %v", err)
	}

	updated, _ := promptRepo.GetByID(aiTestCtx, template.ID)
	if updated.Name != "Updated Name" {
		t.Errorf("Expected name 'Updated Name', got '%s'", updated.Name)
	}
}

// TestPromptTemplateRepository_Delete 测试删除模板
func TestPromptTemplateRepository_Delete(t *testing.T) {
	_, promptRepo := setupAIContentRepositories(t)

	template := &model.PromptTemplate{
		Name:     "To Delete",
		Type:     model.AIGenerationTypeTitle,
		Template: "Delete content",
	}
	promptRepo.Create(aiTestCtx, template)

	err := promptRepo.Delete(aiTestCtx, template.ID)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	_, err = promptRepo.GetByID(aiTestCtx, template.ID)
	if err == nil {
		t.Error("Expected template to be deleted")
	}
}
