package email

import (
	"context"
	"os"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/crypto"
	"hivemtk-user/internal/repository"
)

// clearSmtpEnv 清掉 resolveSmtpFromEnv 认得的全部 env，确保用例只测 DB 那一臂。
func clearSmtpEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"EMAIL_163_USER", "EMAIL_163_PASSWORD", "EMAIL_163_SMTP_HOST",
		"QQ_EMAIL", "QQ_EMAIL_PASSWORD",
		"SMTP_HOST", "SMTP_PORT", "SMTP_USER", "SMTP_PASSWORD",
	} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
}

// TestResolveSmtpFromDB_PicksFirstUsable 表里第一条 Server/Username 非空的行应被选中，
// 且密码经 crypto.Decrypt 解回明文（落库是加密的）。
func TestResolveSmtpFromDB_PicksFirstUsable(t *testing.T) {
	t.Setenv("FIELD_ENCRYPTION_KEY", "test-field-encryption-key-0123456789abcdef")
	setupEmailSendServiceTestDB(t)
	clearSmtpEnv(t)
	repo := repository.NewEmailSmtpRepository()
	enc, err := crypto.Encrypt("decrypted-pw")
	if err != nil {
		t.Fatalf("加密失败: %v", err)
	}
	if err := repo.Create(context.Background(), &model.EmailSmtp{
		Name: "db-smtp", Server: "smtp.db.example", Port: 465,
		Username: "db@example.com", Password: enc, Limit: 100,
	}); err != nil {
		t.Fatalf("建 SMTP 失败: %v", err)
	}

	svc := NewEmailSendService()
	got := svc.resolveSmtpFromDB(context.Background())
	if got == nil {
		t.Fatal("resolveSmtpFromDB 应返回表里的账号，实际 nil")
	}
	if got.Server != "smtp.db.example" || got.Username != "db@example.com" {
		t.Errorf("选中账号 = %+v, 期望 smtp.db.example / db@example.com", got)
	}
	if got.Password != "decrypted-pw" {
		t.Errorf("密码 = %q, 期望解密后的明文 decrypted-pw", got.Password)
	}
}

// TestResolveSmtpFromDB_EmptyTableReturnsNil 表空时返回 nil（让调用方回落报错，
// 与接线前「无 env 即报错」一致）。
func TestResolveSmtpFromDB_EmptyTableReturnsNil(t *testing.T) {
	setupEmailSendServiceTestDB(t)
	clearSmtpEnv(t)
	svc := NewEmailSendService()
	if got := svc.resolveSmtpFromDB(context.Background()); got != nil {
		t.Errorf("空表 resolveSmtpFromDB 应返回 nil, 实际 %+v", got)
	}
}

// TestResolveSmtpFromDB_SkipsIncomplete 行的 Server 或 Username 为空时跳过，选后面那条完整的。
func TestResolveSmtpFromDB_SkipsIncomplete(t *testing.T) {
	setupEmailSendServiceTestDB(t)
	clearSmtpEnv(t)
	repo := repository.NewEmailSmtpRepository()
	if err := repo.Create(context.Background(), &model.EmailSmtp{
		Name: "bad", Server: "", Username: "", Password: "x", Limit: 10,
	}); err != nil {
		t.Fatalf("建坏行失败: %v", err)
	}
	if err := repo.Create(context.Background(), &model.EmailSmtp{
		Name: "good", Server: "smtp.good.example", Username: "good@example.com",
		Password: "pw", Limit: 10,
	}); err != nil {
		t.Fatalf("建好行失败: %v", err)
	}
	svc := NewEmailSendService()
	got := svc.resolveSmtpFromDB(context.Background())
	if got == nil || got.Server != "smtp.good.example" {
		t.Errorf("应跳过不完整行并选中 good, 实际 %+v", got)
	}
}

// TestSendActualEmail_Priority env > DB：env 配了 SMTP_HOST 时，即便表里有账号也应走 env
// （env 是部署层应急开关，优先级最高）。
func TestSendActualEmail_PriorityEnvOverDB(t *testing.T) {
	setupEmailSendServiceTestDB(t)
	clearSmtpEnv(t)
	repo := repository.NewEmailSmtpRepository()
	if err := repo.Create(context.Background(), &model.EmailSmtp{
		Name: "db", Server: "smtp.db.example", Username: "db@example.com",
		Password: "pw", Limit: 100,
	}); err != nil {
		t.Fatalf("建 SMTP 失败: %v", err)
	}
	// 配 env：env 应压过 DB。
	t.Setenv("SMTP_HOST", "smtp.env.example")
	t.Setenv("SMTP_USER", "env@example.com")
	t.Setenv("SMTP_PASSWORD", "envpw")

	got := resolveSmtpFromEnv()
	if got == nil {
		t.Fatal("resolveSmtpFromEnv 应在 env 配置时返回账号")
	}
	if got.Server != "smtp.env.example" {
		t.Errorf("env 优先级：Server = %q, 期望 smtp.env.example（env 应压过 DB）", got.Server)
	}
}
