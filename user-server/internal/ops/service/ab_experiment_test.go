package service

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/ops/model"
	"hivemtk-user/internal/pkg/db"

	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

func setupABExperimentServiceTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.ABExperiment{},
		&model.ABVariant{},
		&model.ABConversionEvent{},
		&model.ABExperimentResult{},
	)
	db.SetTestDB(database)
	return database
}

func setupABExperimentService(t *testing.T) *ABExperimentService {
	setupABExperimentServiceTestDB(t)
	return NewABExperimentService()
}

// TestNewABExperimentService 测试创建服务实例
func TestNewABExperimentService(t *testing.T) {
	service := NewABExperimentService()
	if service == nil {
		t.Fatal("Expected service instance, got nil")
	}
	if service.experimentRepo == nil {
		t.Error("Expected experimentRepo to be initialized")
	}
	if service.variantRepo == nil {
		t.Error("Expected variantRepo to be initialized")
	}
	if service.conversionRepo == nil {
		t.Error("Expected conversionRepo to be initialized")
	}
	if service.resultRepo == nil {
		t.Error("Expected resultRepo to be initialized")
	}
}

// TestABExperimentService_CreateExperiment_Success 测试创建实验成功
func TestABExperimentService_CreateExperiment_Success(t *testing.T) {
	ctx := context.Background()
	service := setupABExperimentService(t)

	req := &CreateExperimentRequest{
		Name:         "Test Experiment",
		Description:  "Test Description",
		SourceType:   "page",
		SourceID:     "page_123",
		TrafficSplit: 50,
		Variants: []VariantConfig{
			{Name: "Control", IsControl: true, Weight: 50},
			{Name: "Variant A", IsControl: false, Weight: 50},
		},
	}

	experiment, err := service.CreateExperiment(ctx, req)
	if err != nil {
		t.Fatalf("CreateExperiment failed: %v", err)
	}

	if experiment.Name != "Test Experiment" {
		t.Errorf("Expected name 'Test Experiment', got '%s'", experiment.Name)
	}
	if experiment.Status != "draft" {
		t.Errorf("Expected status 'draft', got '%s'", experiment.Status)
	}
}

// TestABExperimentService_CreateExperiment_WithDates 测试创建带日期的实验
func TestABExperimentService_CreateExperiment_WithDates(t *testing.T) {
	ctx := context.Background()
	service := setupABExperimentService(t)

	startDate := time.Now().Format("2006-01-02")
	endDate := time.Now().AddDate(0, 0, 30).Format("2006-01-02")

	req := &CreateExperimentRequest{
		Name:         "Dated Experiment",
		SourceType:   "component",
		SourceID:     "comp_456",
		StartDate:    startDate,
		EndDate:      endDate,
		TrafficSplit: 60,
		Variants: []VariantConfig{
			{Name: "A", Weight: 40},
			{Name: "B", Weight: 60},
		},
	}

	experiment, err := service.CreateExperiment(ctx, req)
	if err != nil {
		t.Fatalf("CreateExperiment failed: %v", err)
	}

	if experiment.SourceType != "component" {
		t.Errorf("Expected source_type 'component', got '%s'", experiment.SourceType)
	}
}

// TestABExperimentService_CreateExperiment_WithoutVariants 测试创建无变体的实验
func TestABExperimentService_CreateExperiment_WithoutVariants(t *testing.T) {
	ctx := context.Background()
	service := setupABExperimentService(t)

	req := &CreateExperimentRequest{
		Name:       "Empty Experiment",
		SourceType: "page",
		SourceID:   "page_789",
		Variants:   []VariantConfig{},
	}

	experiment, err := service.CreateExperiment(ctx, req)
	if err != nil {
		t.Fatalf("CreateExperiment failed: %v", err)
	}

	if experiment.ID == 0 {
		t.Error("Expected experiment ID to be set")
	}
}

// TestABExperimentService_GetExperiment_Success 测试获取实验详情
func TestABExperimentService_GetExperiment_Success(t *testing.T) {
	ctx := context.Background()
	service := setupABExperimentService(t)

	createReq := &CreateExperimentRequest{
		Name:       "Get Test",
		SourceType: "page",
		SourceID:   "page_get",
		Variants:   []VariantConfig{{Name: "A"}},
	}
	created, _ := service.CreateExperiment(ctx, createReq)

	retrieved, err := service.GetExperiment(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetExperiment failed: %v", err)
	}

	if retrieved.Name != "Get Test" {
		t.Errorf("Expected name 'Get Test', got '%s'", retrieved.Name)
	}
}

// TestABExperimentService_GetExperiment_NotFound 测试获取不存在的实验
func TestABExperimentService_GetExperiment_NotFound(t *testing.T) {
	ctx := context.Background()
	service := setupABExperimentService(t)

	_, err := service.GetExperiment(ctx, 999999)
	if err == nil {
		t.Error("Expected error for non-existent experiment")
	}
}

// TestABExperimentService_GetExperimentList_Success 测试获取实验列表
func TestABExperimentService_GetExperimentList_Success(t *testing.T) {
	ctx := context.Background()
	service := setupABExperimentService(t)

	for i := 1; i <= 5; i++ {
		createReq := &CreateExperimentRequest{
			Name:       "Experiment " + string(rune('0'+i)),
			SourceType: "page",
			SourceID:   "page_" + string(rune('0'+i)),
			Variants:   []VariantConfig{{Name: "A"}},
		}
		service.CreateExperiment(ctx, createReq)
	}

	experiments, total, err := service.GetExperimentList(ctx, 1, 10)
	if err != nil {
		t.Fatalf("GetExperimentList failed: %v", err)
	}

	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}
	if len(experiments) != 5 {
		t.Errorf("Expected 5 experiments, got %d", len(experiments))
	}
}

// TestABExperimentService_GetExperimentList_WithPagination 测试分页获取实验列表
func TestABExperimentService_GetExperimentList_WithPagination(t *testing.T) {
	ctx := context.Background()
	service := setupABExperimentService(t)

	for i := 0; i < 15; i++ {
		createReq := &CreateExperimentRequest{
			Name:       "Experiment" + string(rune('A'+i)),
			SourceType: "page",
			SourceID:   "page_" + string(rune('A'+i)),
			Variants:   []VariantConfig{{Name: "A"}},
		}
		service.CreateExperiment(ctx, createReq)
	}

	experiments, total, err := service.GetExperimentList(ctx, 1, 10)
	if err != nil {
		t.Fatalf("GetExperimentList failed: %v", err)
	}

	if total != 15 {
		t.Errorf("Expected total 15, got %d", total)
	}
	if len(experiments) != 10 {
		t.Errorf("Expected 10 experiments on page 1, got %d", len(experiments))
	}

	experiments2, _, err := service.GetExperimentList(ctx, 2, 10)
	if err != nil {
		t.Fatalf("GetExperimentList page 2 failed: %v", err)
	}
	if len(experiments2) != 5 {
		t.Errorf("Expected 5 experiments on page 2, got %d", len(experiments2))
	}
}

// TestABExperimentService_GetExperimentList_EmptyList 测试空实验列表
func TestABExperimentService_GetExperimentList_EmptyList(t *testing.T) {
	ctx := context.Background()
	service := setupABExperimentService(t)

	experiments, total, err := service.GetExperimentList(ctx, 1, 10)
	if err != nil {
		t.Fatalf("GetExperimentList failed: %v", err)
	}

	if total != 0 {
		t.Errorf("Expected total 0, got %d", total)
	}
	if len(experiments) != 0 {
		t.Errorf("Expected 0 experiments, got %d", len(experiments))
	}
}

// TestABExperimentService_UpdateExperiment_Success 测试更新实验
func TestABExperimentService_UpdateExperiment_Success(t *testing.T) {
	ctx := context.Background()
	service := setupABExperimentService(t)

	createReq := &CreateExperimentRequest{
		Name:       "Original",
		SourceType: "page",
		SourceID:   "page_orig",
		Variants:   []VariantConfig{{Name: "A"}},
	}
	created, _ := service.CreateExperiment(ctx, createReq)

	updateReq := &CreateExperimentRequest{
		Name:         "Updated",
		Description:  "New Description",
		SourceType:   "component",
		SourceID:     "comp_new",
		TrafficSplit: 70,
		Variants: []VariantConfig{
			{Name: "B", Weight: 30},
			{Name: "C", Weight: 70},
		},
	}

	updated, err := service.UpdateExperiment(ctx, created.ID, updateReq)
	if err != nil {
		t.Fatalf("UpdateExperiment failed: %v", err)
	}

	if updated.Name != "Updated" {
		t.Errorf("Expected name 'Updated', got '%s'", updated.Name)
	}
	if updated.SourceType != "component" {
		t.Errorf("Expected source_type 'component', got '%s'", updated.SourceType)
	}
	if updated.TrafficSplit != 70 {
		t.Errorf("Expected traffic_split 70, got %d", updated.TrafficSplit)
	}
}

// TestABExperimentService_UpdateExperiment_NotFound 测试更新不存在的实验
func TestABExperimentService_UpdateExperiment_NotFound(t *testing.T) {
	ctx := context.Background()
	service := setupABExperimentService(t)

	updateReq := &CreateExperimentRequest{
		Name: "Updated",
	}

	_, err := service.UpdateExperiment(ctx, 999999, updateReq)
	if err == nil {
		t.Error("Expected error for non-existent experiment")
	}
}

// TestABExperimentService_DeleteExperiment_Success 测试删除实验
func TestABExperimentService_DeleteExperiment_Success(t *testing.T) {
	ctx := context.Background()
	service := setupABExperimentService(t)

	createReq := &CreateExperimentRequest{
		Name:       "Delete Me",
		SourceType: "page",
		SourceID:   "page_del",
		Variants:   []VariantConfig{{Name: "A"}},
	}
	created, _ := service.CreateExperiment(ctx, createReq)

	err := service.DeleteExperiment(ctx, created.ID)
	if err != nil {
		t.Fatalf("DeleteExperiment failed: %v", err)
	}

	_, err = service.GetExperiment(ctx, created.ID)
	if err == nil {
		t.Error("Expected error after deletion")
	}
}

// TestABExperimentService_StartExperiment_Success 测试启动实验
func TestABExperimentService_StartExperiment_Success(t *testing.T) {
	ctx := context.Background()
	service := setupABExperimentService(t)

	createReq := &CreateExperimentRequest{
		Name:       "Start Test",
		SourceType: "page",
		SourceID:   "page_start",
		Variants:   []VariantConfig{{Name: "A"}},
	}
	created, _ := service.CreateExperiment(ctx, createReq)

	err := service.StartExperiment(ctx, created.ID)
	if err != nil {
		t.Fatalf("StartExperiment failed: %v", err)
	}

	updated, _ := service.GetExperiment(ctx, created.ID)
	if updated.Status != "running" {
		t.Errorf("Expected status 'running', got '%s'", updated.Status)
	}
}

// TestABExperimentService_PauseExperiment_Success 测试暂停实验
func TestABExperimentService_PauseExperiment_Success(t *testing.T) {
	ctx := context.Background()
	service := setupABExperimentService(t)

	createReq := &CreateExperimentRequest{
		Name:       "Pause Test",
		SourceType: "page",
		SourceID:   "page_pause",
		Variants:   []VariantConfig{{Name: "A"}},
	}
	created, _ := service.CreateExperiment(ctx, createReq)
	service.StartExperiment(ctx, created.ID)

	err := service.PauseExperiment(ctx, created.ID)
	if err != nil {
		t.Fatalf("PauseExperiment failed: %v", err)
	}

	updated, _ := service.GetExperiment(ctx, created.ID)
	if updated.Status != "paused" {
		t.Errorf("Expected status 'paused', got '%s'", updated.Status)
	}
}

// TestABExperimentService_StopExperiment_Success 测试停止实验
func TestABExperimentService_StopExperiment_Success(t *testing.T) {
	ctx := context.Background()
	service := setupABExperimentService(t)

	createReq := &CreateExperimentRequest{
		Name:       "Stop Test",
		SourceType: "page",
		SourceID:   "page_stop",
		Variants:   []VariantConfig{{Name: "A"}},
	}
	created, _ := service.CreateExperiment(ctx, createReq)
	service.StartExperiment(ctx, created.ID)

	err := service.StopExperiment(ctx, created.ID)
	if err != nil {
		t.Fatalf("StopExperiment failed: %v", err)
	}

	updated, _ := service.GetExperiment(ctx, created.ID)
	if updated.Status != "completed" {
		t.Errorf("Expected status 'completed', got '%s'", updated.Status)
	}
}

// TestABExperimentService_GetConversionEvents_Success 测试获取转化事件
func TestABExperimentService_GetConversionEvents_Success(t *testing.T) {
	ctx := context.Background()
	service := setupABExperimentService(t)

	createReq := &CreateExperimentRequest{
		Name:       "Get Events Test",
		SourceType: "page",
		SourceID:   "page_getevents",
		Variants:   []VariantConfig{{Name: "A"}},
	}
	experiment, _ := service.CreateExperiment(ctx, createReq)
	variants, _ := service.variantRepo.GetByExperiment(ctx, experiment.ID)

	event := &model.ABConversionEvent{
		ExperimentID: experiment.ID,
		VariantID:    variants[0].ID,
		EventName:    "purchase",
		EventType:    "click",
		EventValue:   9999,
		UserID:       "user_123",
	}
	if err := db.GetDB().Create(event).Error; err != nil {
		t.Fatalf("seed conversion event failed: %v", err)
	}

	events, total, err := service.GetConversionEvents(ctx, experiment.ID, 1, 10)
	if err != nil {
		t.Fatalf("GetConversionEvents failed: %v", err)
	}

	if total != 1 {
		t.Errorf("Expected total 1, got %d", total)
	}
	if len(events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(events))
	}
}

// TestABExperimentService_CalculateResults_MultiVariant 多变异实验每个变体各留一条结果。
// 唯一键位只能是 (tenant_id, experiment_id, variant_id)：若 ExperimentID 单列唯一，
// 第二个变体的 Upsert 会撞约束，CalculateResults 直接失败。
func TestABExperimentService_CalculateResults_MultiVariant(t *testing.T) {
	ctx := context.Background()
	service := setupABExperimentService(t)

	createReq := &CreateExperimentRequest{
		Name:       "Multi Variant Results",
		SourceType: "page",
		SourceID:   "page_multi_results",
		Variants: []VariantConfig{
			{Name: "Control", IsControl: true, Weight: 50},
			{Name: "Treatment", Weight: 50},
		},
	}
	experiment, err := service.CreateExperiment(ctx, createReq)
	if err != nil {
		t.Fatalf("CreateExperiment failed: %v", err)
	}

	if err := service.CalculateResults(ctx, experiment.ID); err != nil {
		t.Fatalf("CalculateResults failed: %v", err)
	}

	results, err := service.resultRepo.GetByExperiment(ctx, experiment.ID)
	if err != nil {
		t.Fatalf("GetByExperiment failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("Expected 2 variant results, got %d", len(results))
	}
}

// TestABExperimentService_CalculateResults_Success 测试计算实验结果
func TestABExperimentService_CalculateResults_Success(t *testing.T) {
	ctx := context.Background()
	service := setupABExperimentService(t)

	createReq := &CreateExperimentRequest{
		Name:       "Results Test",
		SourceType: "page",
		SourceID:   "page_results",
		Variants: []VariantConfig{
			{Name: "Control", IsControl: true, Weight: 50},
		},
	}
	experiment, _ := service.CreateExperiment(ctx, createReq)

	variants, _ := service.variantRepo.GetByExperiment(ctx, experiment.ID)
	if len(variants) == 0 {
		t.Fatal("No variants created")
	}
	variant := variants[0]

	if err := db.GetDB().WithContext(ctx).Model(variant).Updates(map[string]any{
		"traffic_count":    2,
		"conversion_count": 1,
	}).Error; err != nil {
		t.Fatalf("seed variant counts failed: %v", err)
	}

	err := service.CalculateResults(ctx, experiment.ID)
	if err != nil {
		t.Fatalf("CalculateResults failed: %v", err)
	}

	results, _ := service.resultRepo.GetByExperiment(ctx, experiment.ID)
	if len(results) < 1 {
		t.Errorf("Expected at least 1 result, got %d", len(results))
	}
}

// TestABExperimentService_calculateConfidence 测试置信度计算
func TestABExperimentService_calculateConfidence(t *testing.T) {
	service := setupABExperimentService(t)

	result1 := &model.ABExperimentResult{
		TrafficCount:   100,
		ConversionRate: 70.0,
	}
	conf1 := service.calculateConfidence(result1)
	if conf1 <= 0.5 {
		t.Errorf("Expected confidence > 0.5 for 70%% conversion, got %f", conf1)
	}

	result2 := &model.ABExperimentResult{
		TrafficCount:   100,
		ConversionRate: 50.0,
	}
	conf2 := service.calculateConfidence(result2)
	if conf2 < 0.3 || conf2 > 0.7 {
		t.Logf("Confidence for 50%% conversion: %f (expected around 0.5)", conf2)
	}

	result3 := &model.ABExperimentResult{
		TrafficCount:   0,
		ConversionRate: 0.0,
	}
	conf3 := service.calculateConfidence(result3)
	if conf3 != 0 {
		t.Errorf("Expected 0 confidence for zero traffic, got %f", conf3)
	}
}

// TestABExperimentService_calculateWinner 测试获胜者计算
func TestABExperimentService_calculateWinner(t *testing.T) {
	ctx := context.Background()
	service := setupABExperimentService(t)

	createReq := &CreateExperimentRequest{
		Name:       "Winner Test",
		SourceType: "page",
		SourceID:   "page_winner",
		Variants:   []VariantConfig{{Name: "A", IsControl: true}},
	}
	experiment, _ := service.CreateExperiment(ctx, createReq)
	variants, _ := service.variantRepo.GetByExperiment(ctx, experiment.ID)

	resultA := &model.ABExperimentResult{
		ExperimentID:    experiment.ID,
		VariantID:       variants[0].ID,
		VariantName:     "A",
		IsControl:       true,
		TrafficCount:    100,
		ConversionCount: 20,
		ConversionRate:  20.0,
	}

	err := service.resultRepo.Upsert(ctx, resultA)
	if err != nil {
		t.Fatalf("Failed to create result: %v", err)
	}

	err = service.calculateWinner(ctx, experiment.ID)
	if err != nil {
		t.Fatalf("calculateWinner failed: %v", err)
	}

	results, _ := service.resultRepo.GetByExperiment(ctx, experiment.ID)
	if len(results) < 1 {
		t.Error("Expected at least 1 result")
	}
}

// TestABExperimentService_GetExperimentResults_Success 测试获取实验结果
func TestABExperimentService_GetExperimentResults_Success(t *testing.T) {
	ctx := context.Background()
	service := setupABExperimentService(t)

	createReq := &CreateExperimentRequest{
		Name:       "Get Results Test",
		SourceType: "page",
		SourceID:   "page_getresults",
		Variants:   []VariantConfig{{Name: "A"}},
	}
	experiment, _ := service.CreateExperiment(ctx, createReq)

	result := &model.ABExperimentResult{
		ExperimentID:    experiment.ID,
		VariantID:       1,
		VariantName:     "A",
		IsControl:       true,
		TrafficCount:    100,
		ConversionCount: 10,
		ConversionRate:  10.0,
	}
	service.resultRepo.Upsert(ctx, result)

	results, err := service.GetExperimentResults(ctx, experiment.ID)
	if err != nil {
		t.Fatalf("GetExperimentResults failed: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}
}
