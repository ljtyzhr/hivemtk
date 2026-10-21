package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/repository"
	"hivemtk-user/internal/service"

	"hivemtk-user/internal/pkg/testutil"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func setupTestControllerDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.SystemUser{},
		&model.User{},
		&model.Account{},
		&model.DouyinCard{},
		&model.ShortLink{},
		&model.DomainPool{},
		// 登录链路会读 MFA 状态与密码策略：任一查询失败即 fail-closed 拒登，
		// 缺表会让用例红在 42P01 而不是红在认证逻辑上。
		&model.UserMFA{},
		&model.PasswordHistory{},
		&model.SystemConfigKV{},
		// 登录风险检测异步写登录事件，缺表会把真实失败淹没在 42P01 噪声里。
		&model.LoginEvent{},
		// on 模式登录要查租户（停用拦截 / 租户短码），缺表登录直接 401。
		&model.Tenant{},
	)
	db.SetTestDB(database)
	return database
}

// TestAccountController_GetAccounts_Success 测试获取账户列表成功
func TestAccountController_GetAccounts_Success(t *testing.T) {
	database := setupTestControllerDB(t)
	ctrl := NewAccountController(service.NewAccountService(repository.NewAccountRepositoryWithDB(database)))
	router := setupGinEngine()
	router.GET("/accounts", ctrl.GetAccounts)

	account := model.Account{
		ID:         "test-account-id",
		TgBotToken: "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
		Price:      "100.0",
		GroupID:    123,
		Status:     1,
	}
	database.Create(&account)

	req, _ := http.NewRequest("GET", "/accounts", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}

	var response map[string]any
	json.Unmarshal(w.Body.Bytes(), &response)
	if response["code"] != float64(0) {
		t.Errorf("Expected code SUCCESS, got %v", response["code"])
	}
	// Bot Token 是可接管机器人的凭据，列表接口只能回脱敏值。
	if strings.Contains(w.Body.String(), account.TgBotToken) {
		t.Errorf("接口下发了 Bot Token 原文: %s", w.Body.String())
	}
}

// TestAccountController_GetAccounts_EmptyList 测试空账户列表
func TestAccountController_GetAccounts_EmptyList(t *testing.T) {
	database := setupTestControllerDB(t)
	ctrl := NewAccountController(service.NewAccountService(repository.NewAccountRepositoryWithDB(database)))
	router := setupGinEngine()
	router.GET("/accounts", ctrl.GetAccounts)

	req, _ := http.NewRequest("GET", "/accounts", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}
}

// TestAccountController_GetAccounts_MultipleAccounts 测试获取多个账户
func TestAccountController_GetAccounts_MultipleAccounts(t *testing.T) {
	database := setupTestControllerDB(t)
	ctrl := NewAccountController(service.NewAccountService(repository.NewAccountRepositoryWithDB(database)))
	router := setupGinEngine()
	router.GET("/accounts", ctrl.GetAccounts)

	accounts := []model.Account{
		{ID: uuid.NewString(), TgName: "account1", Status: 1},
		{ID: uuid.NewString(), TgName: "account2", Status: 1},
		{ID: uuid.NewString(), TgName: "account3", Status: 1},
	}
	for i := range accounts {
		database.Create(&accounts[i])
	}

	req, _ := http.NewRequest("GET", "/accounts", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}

	var response map[string]any
	json.Unmarshal(w.Body.Bytes(), &response)
	data := response["data"].(map[string]any)
	list := data["list"].([]any)
	if len(list) != 3 {
		t.Errorf("Expected 3 accounts, got %d", len(list))
	}
}
