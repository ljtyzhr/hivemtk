package service

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/repository"

	"gorm.io/gorm"
)

func setupAccountServiceTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.Account{},
	)
	db.SetTestDB(database)
	return database
}

func newAccountServiceForTest(t *testing.T) (*AccountService, *gorm.DB) {
	t.Helper()
	database := setupAccountServiceTestDB(t)
	return NewAccountService(repository.NewAccountRepositoryWithDB(database)), database
}

func TestAccountService_GetAccount(t *testing.T) {
	service, database := newAccountServiceForTest(t)

	account := model.Account{
		ID:         "svc-get-account",
		TgName:     "test_account",
		TgBotToken: "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
		GroupID:    123456789,
		Price:      "100.00",
		URL:        "http://example.com",
	}
	if err := database.Create(&account).Error; err != nil {
		t.Fatalf("seed account failed: %v", err)
	}

	result, err := service.GetAccount(context.Background(), account.ID)
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}

	if result.TgName != "test_account" {
		t.Errorf("Expected TgName 'test_account', got %s", result.TgName)
	}
}

func TestAccountService_GetAccount_NotFound(t *testing.T) {
	service, _ := newAccountServiceForTest(t)

	_, err := service.GetAccount(context.Background(), "non-existent-id")
	if err == nil {
		t.Error("Expected error for non-existent account")
	}
}

func TestAccountService_GetAccountList(t *testing.T) {
	service, database := newAccountServiceForTest(t)

	for i := 0; i < 5; i++ {
		account := model.Account{
			ID:         "svc-list-" + string(rune('0'+i)),
			TgName:     "account_" + string(rune('0'+i)),
			TgBotToken: "token_" + string(rune('0'+i)),
			GroupID:    int64(100000 + i),
			Price:      "100.00",
			URL:        "http://example" + string(rune('0'+i)) + ".com",
		}
		if err := database.Create(&account).Error; err != nil {
			t.Fatalf("seed account %d failed: %v", i, err)
		}
	}

	accounts, err := service.GetAccountList(context.Background())
	if err != nil {
		t.Fatalf("GetAccountList failed: %v", err)
	}

	if len(accounts) != 5 {
		t.Errorf("Expected 5 accounts, got %d", len(accounts))
	}
}

func TestAccountService_GetAccountList_Empty(t *testing.T) {
	service, _ := newAccountServiceForTest(t)

	accounts, err := service.GetAccountList(context.Background())
	if err != nil {
		t.Fatalf("GetAccountList failed: %v", err)
	}

	if len(accounts) != 0 {
		t.Errorf("Expected 0 accounts, got %d", len(accounts))
	}
}
