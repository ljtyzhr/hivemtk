package repository

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"

	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

func setupClueTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.Clue{},
	)
	db.SetTestDB(database)
	return database
}

func TestClueRepository_New(t *testing.T) {
	setupClueTestDB(t)

	repo := NewClueRepositoryWithDB(db.GetDB())
	if repo == nil {
		t.Fatal("Expected non-nil repository")
	}
}

func TestClueRepository_Create(t *testing.T) {
	setupClueTestDB(t)

	repo := NewClueRepositoryWithDB(db.GetDB())
	ctx := context.Background()

	clue := &model.Clue{
		SourceID: "source123",
		Account:  "test@example.com",
		Type:     1,
		IsVerify: 0,
		Name:     "Test User",
		City:     "Beijing",
		Address:  "123 Test Street",
		Desc:     "Test description",
	}

	err := repo.Create(ctx, clue)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if clue.ID == "" {
		t.Error("Expected non-empty ID after create")
	}
}

// TestClueRepository_CreateIsPureInsert 仓储不做 (type, account) 去重。
// 去重判据属业务规则，由 service.Register 先查后写；这一条把分层边界钉住，
// 避免以后有人把 Count 检查又塞回仓储。
func TestClueRepository_CreateIsPureInsert(t *testing.T) {
	setupClueTestDB(t)

	repo := NewClueRepositoryWithDB(db.GetDB())
	ctx := context.Background()

	clue := &model.Clue{
		SourceID: "source123",
		Account:  "test@example.com",
		Type:     1,
		IsVerify: 0,
		Name:     "Test User",
	}
	if err := repo.Create(ctx, clue); err != nil {
		t.Fatalf("First Create failed: %v", err)
	}

	clue2 := &model.Clue{
		SourceID: "source456",
		Account:  "test@example.com",
		Type:     1,
		IsVerify: 0,
		Name:     "Another User",
	}
	if err := repo.Create(ctx, clue2); err != nil {
		t.Fatalf("仓储应只负责写入（去重在 service 层），实际报错: %v", err)
	}
	var n int64
	if err := db.GetDB().Model(&model.Clue{}).Where("type = ? AND account = ?", 1, "test@example.com").Count(&n).Error; err != nil {
		t.Fatalf("计数失败: %v", err)
	}
	if n != 2 {
		t.Errorf("应写入 2 行（仓储不做去重），实际 %d", n)
	}
}

func TestClueRepository_GetClueList(t *testing.T) {
	setupClueTestDB(t)

	repo := NewClueRepositoryWithDB(db.GetDB())
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		clue := &model.Clue{
			SourceID: "source" + string(rune('0'+i)),
			Account:  "test" + string(rune('0'+i)) + "@example.com",
			Type:     int64(i%3 + 1),
			Name:     "User " + string(rune('0'+i)),
		}
		repo.Create(ctx, clue)
	}

	clues, total, err := repo.GetClueList(context.Background(), 1, 5)
	if err != nil {
		t.Fatalf("GetClueList failed: %v", err)
	}

	if len(clues) != 5 {
		t.Errorf("Expected 5 clues, got %d", len(clues))
	}

	if total != 10 {
		t.Errorf("Expected total 10, got %d", total)
	}
}

func TestClueRepository_GetClueList_Empty(t *testing.T) {
	setupClueTestDB(t)

	repo := NewClueRepositoryWithDB(db.GetDB())

	clues, total, err := repo.GetClueList(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("GetClueList failed: %v", err)
	}

	if len(clues) != 0 {
		t.Errorf("Expected 0 clues, got %d", len(clues))
	}

	if total != 0 {
		t.Errorf("Expected total 0, got %d", total)
	}
}

func TestClueRepository_ExistsByTypeAndAccount(t *testing.T) {
	setupClueTestDB(t)

	repo := NewClueRepositoryWithDB(db.GetDB())
	ctx := context.Background()

	clue := &model.Clue{
		SourceID: "source123",
		Account:  "test@example.com",
		Type:     1,
		Name:     "Test User",
	}
	repo.Create(ctx, clue)

	exists, err := repo.ExistsByTypeAndAccount(context.Background(), 1, "test@example.com")
	if err != nil {
		t.Fatalf("ExistsByTypeAndAccount failed: %v", err)
	}

	if !exists {
		t.Error("Expected clue to exist")
	}
}

func TestClueRepository_ExistsByTypeAndAccount_NotExists(t *testing.T) {
	setupClueTestDB(t)

	repo := NewClueRepositoryWithDB(db.GetDB())

	exists, err := repo.ExistsByTypeAndAccount(context.Background(), 1, "nonexistent@example.com")
	if err != nil {
		t.Fatalf("ExistsByTypeAndAccount failed: %v", err)
	}

	if exists {
		t.Error("Expected clue to not exist")
	}
}

func TestClueRepository_ExistsByTypeAndAccount_DifferentType(t *testing.T) {
	setupClueTestDB(t)

	repo := NewClueRepositoryWithDB(db.GetDB())
	ctx := context.Background()

	clue := &model.Clue{
		SourceID: "source123",
		Account:  "test@example.com",
		Type:     1,
		Name:     "Test User",
	}
	repo.Create(ctx, clue)

	exists, err := repo.ExistsByTypeAndAccount(context.Background(), 2, "test@example.com")
	if err != nil {
		t.Fatalf("ExistsByTypeAndAccount failed: %v", err)
	}

	if exists {
		t.Error("Expected clue to not exist for different type")
	}
}

func TestClueRepository_Create_VariousTypes(t *testing.T) {
	setupClueTestDB(t)

	repo := NewClueRepositoryWithDB(db.GetDB())
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		clue := &model.Clue{
			SourceID: "source" + string(rune('0'+i)),
			Account:  "test" + string(rune('0'+i)) + "@example.com",
			Type:     int64(i + 1),
			Name:     "Type" + string(rune('1'+i)) + " User",
		}
		repo.Create(ctx, clue)
	}

	clues, total, err := repo.GetClueList(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("GetClueList failed: %v", err)
	}

	if len(clues) != 5 {
		t.Errorf("Expected 5 clues, got %d", len(clues))
	}

	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}
}

func TestClueRepository_Create_VariousAccounts(t *testing.T) {
	setupClueTestDB(t)

	repo := NewClueRepositoryWithDB(db.GetDB())
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		clue := &model.Clue{
			SourceID: "source" + string(rune('0'+i)),
			Account:  "user" + string(rune('0'+i)) + "@example.com",
			Type:     1,
			Name:     "User " + string(rune('0'+i)),
		}
		repo.Create(ctx, clue)
	}

	clues, total, err := repo.GetClueList(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("GetClueList failed: %v", err)
	}

	if len(clues) != 5 {
		t.Errorf("Expected 5 clues, got %d", len(clues))
	}

	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}
}

func TestClueRepository_Create_WithAllFields(t *testing.T) {
	setupClueTestDB(t)

	repo := NewClueRepositoryWithDB(db.GetDB())
	ctx := context.Background()

	clue := &model.Clue{
		SourceID: "source123",
		Account:  "complete@example.com",
		Type:     1,
		IsVerify: 1,
		Name:     "Complete User",
		City:     "Shanghai",
		Address:  "456 Complete Street, District",
		Desc:     "This is a complete description with all fields filled in",
	}

	err := repo.Create(ctx, clue)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if clue.ID == "" {
		t.Error("Expected non-empty ID after create")
	}

	clues, _, _ := repo.GetClueList(context.Background(), 1, 10)
	if len(clues) != 1 {
		t.Errorf("Expected 1 clue, got %d", len(clues))
	}
}

func TestClueRepository_Create_LargeBatch(t *testing.T) {
	setupClueTestDB(t)

	repo := NewClueRepositoryWithDB(db.GetDB())
	ctx := context.Background()

	for i := 0; i < 50; i++ {
		clue := &model.Clue{
			SourceID: "source" + string(rune('0'+i%10)),
			Account:  "batch" + string(rune('0'+i)) + "@example.com",
			Type:     int64(i%3 + 1),
			Name:     "Batch User " + string(rune('0'+i)),
		}
		repo.Create(ctx, clue)
	}

	clues, total, err := repo.GetClueList(context.Background(), 1, 25)
	if err != nil {
		t.Fatalf("GetClueList failed: %v", err)
	}

	if len(clues) != 25 {
		t.Errorf("Expected 25 clues on page 1, got %d", len(clues))
	}

	if total != 50 {
		t.Errorf("Expected total 50, got %d", total)
	}
}

func TestClueRepository_GetClueList_SecondPage(t *testing.T) {
	setupClueTestDB(t)

	repo := NewClueRepositoryWithDB(db.GetDB())
	ctx := context.Background()

	for i := 0; i < 20; i++ {
		clue := &model.Clue{
			SourceID: "source" + string(rune('0'+i%10)),
			Account:  "page" + string(rune('0'+i)) + "@example.com",
			Type:     1,
			Name:     "Page User " + string(rune('0'+i)),
		}
		repo.Create(ctx, clue)
	}

	clues, total, err := repo.GetClueList(context.Background(), 2, 10)
	if err != nil {
		t.Fatalf("GetClueList failed: %v", err)
	}

	if len(clues) != 10 {
		t.Errorf("Expected 10 clues on page 2, got %d", len(clues))
	}

	if total != 20 {
		t.Errorf("Expected total 20, got %d", total)
	}
}
