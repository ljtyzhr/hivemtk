package repository

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/testutil"
	_type "hivemtk-user/internal/pkg/utils/type"

	"gorm.io/gorm"
)

func setupAccountTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.Account{},
	)
	db.SetTestDB(database)
	return database
}

func TestAccountRepository_GetByID(t *testing.T) {
	database := setupAccountTestDB(t)

	repo := NewAccountRepositoryWithDB(database)
	ctx := context.Background()

	account := &model.Account{
		ID:         "acc-get-by-id",
		TgName:     "testtg",
		TgBotToken: "token123",
		GroupID:    12345,
		Status:     _type.AccountStatusActive,
	}
	if err := database.Create(account).Error; err != nil {
		t.Fatalf("seed account failed: %v", err)
	}

	fetchedAccount, err := repo.GetByID(ctx, account.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if fetchedAccount.TgName != "testtg" {
		t.Errorf("Expected TgName 'testtg', got %s", fetchedAccount.TgName)
	}
}

func TestAccountRepository_GetByID_NotFound(t *testing.T) {
	database := setupAccountTestDB(t)

	ctx := context.Background()
	repo := NewAccountRepositoryWithDB(database)

	_, err := repo.GetByID(ctx, "non-existent-id")
	if err == nil {
		t.Error("Expected error for non-existent account")
	}
}

func TestAccountRepository_GetAccountList(t *testing.T) {
	database := setupAccountTestDB(t)

	ctx := context.Background()
	repo := NewAccountRepositoryWithDB(database)

	for i := 0; i < 5; i++ {
		account := &model.Account{
			ID:         "acc-list-" + string(rune('0'+i)),
			TgName:     "tg" + string(rune('0'+i)),
			TgBotToken: "token" + string(rune('0'+i)),
			GroupID:    int64(10000 + i),
			Status:     _type.AccountStatusActive,
		}
		if err := database.Create(account).Error; err != nil {
			t.Fatalf("seed account %d failed: %v", i, err)
		}
	}

	accounts, err := repo.GetAccountList(ctx)
	if err != nil {
		t.Fatalf("GetAccountList failed: %v", err)
	}

	if len(accounts) != 5 {
		t.Errorf("Expected 5 accounts, got %d", len(accounts))
	}
}

// TestAccountRepository_InactiveStatusPersists 停用状态（值为 0）必须能真正落库：
// 字段曾声明 gorm:"default:1"，GORM 会把显式 0 当零值省略，落库反被默认值改成 Active。
func TestAccountRepository_InactiveStatusPersists(t *testing.T) {
	database := setupAccountTestDB(t)
	ctx := context.Background()
	repo := NewAccountRepositoryWithDB(database)

	row := &model.Account{
		ID:     "acc-inactive",
		TgName: "tg-inactive",
		Status: _type.AccountStatusInactive,
	}
	if err := database.WithContext(ctx).Create(row).Error; err != nil {
		t.Fatalf("seed inactive account failed: %v", err)
	}

	got, err := repo.GetByID(ctx, row.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Status != _type.AccountStatusInactive {
		t.Fatalf("停用状态被列默认值覆盖：期望 %d，实际 %d", _type.AccountStatusInactive, got.Status)
	}
}
