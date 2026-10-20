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

// ── F3：备用通道（163/QQ）DB 化 ──────────────────────────────────────────────

// TestResolveSmtpFromDB_NamedBackupChannelFirst 表里有 Name="env:163" 的行时，
// 它优先于任意首条可用行被选中（与 env 链 163 → QQ 同序），且密码解回明文。
func TestResolveSmtpFromDB_NamedBackupChannelFirst(t *testing.T) {
	t.Setenv("FIELD_ENCRYPTION_KEY", "test-field-encryption-key-0123456789abcdef")
	setupEmailSendServiceTestDB(t)
	clearSmtpEnv(t)
	repo := repository.NewEmailSmtpRepository()
	// 先插一条「普通」行：没有命名通道时它会被选中；有了 env:163 就该让位。
	if err := repo.Create(context.Background(), &model.EmailSmtp{
		Name: "plain", Server: "smtp.plain.example", Port: 465,
		Username: "plain@example.com", Password: "plainpw", Limit: 100,
	}); err != nil {
		t.Fatalf("建普通行失败: %v", err)
	}
	enc, err := crypto.Encrypt("backup-163-pw")
	if err != nil {
		t.Fatalf("加密失败: %v", err)
	}
	if err := repo.Create(context.Background(), &model.EmailSmtp{
		Name: "env:163", Server: "smtp.163.com", Port: 465,
		Username: "backup163@163.com", Password: enc, Limit: 100,
	}); err != nil {
		t.Fatalf("建 env:163 行失败: %v", err)
	}

	svc := NewEmailSendService()
	got := svc.resolveSmtpFromDB(context.Background())
	if got == nil {
		t.Fatal("resolveSmtpFromDB 应选中 env:163 行，实际 nil")
	}
	if got.Name != "env:163" || got.Server != "smtp.163.com" {
		t.Errorf("选中行 = %s/%s, 期望 env:163/smtp.163.com（命名备用通道应压过首条可用行）", got.Name, got.Server)
	}
	if got.Password != "backup-163-pw" {
		t.Errorf("密码 = %q, 期望解密后的明文 backup-163-pw", got.Password)
	}
}

// TestResolveSmtpFromDB_163BeatsQQ 同时有 env:163 与 env:qq 行时按 env 链同序选 163。
func TestResolveSmtpFromDB_163BeatsQQ(t *testing.T) {
	setupEmailSendServiceTestDB(t)
	clearSmtpEnv(t)
	repo := repository.NewEmailSmtpRepository()
	// qq 先插入（物理顺序在前），163 后插入：选取顺序不应跟着插入顺序走。
	if err := repo.Create(context.Background(), &model.EmailSmtp{
		Name: "env:qq", Server: "smtp.qq.com", Port: 465,
		Username: "backup@qq.com", Password: "qqpw", Limit: 100,
	}); err != nil {
		t.Fatalf("建 env:qq 行失败: %v", err)
	}
	if err := repo.Create(context.Background(), &model.EmailSmtp{
		Name: "env:163", Server: "smtp.163.com", Port: 465,
		Username: "backup163@163.com", Password: "163pw", Limit: 100,
	}); err != nil {
		t.Fatalf("建 env:163 行失败: %v", err)
	}

	svc := NewEmailSendService()
	got := svc.resolveSmtpFromDB(context.Background())
	if got == nil || got.Name != "env:163" {
		t.Errorf("应选 env:163（与 env 链同序），实际 %+v", got)
	}
}

// TestResolveSmtpFromDB_NamedRowIncompleteFallsThrough env:163 行缺 Username 时
// 跳过它，继续找 qq 命名行——单条坏数据不应把备用通道整条打死。
func TestResolveSmtpFromDB_NamedRowIncompleteFallsThrough(t *testing.T) {
	setupEmailSendServiceTestDB(t)
	clearSmtpEnv(t)
	repo := repository.NewEmailSmtpRepository()
	if err := repo.Create(context.Background(), &model.EmailSmtp{
		Name: "env:163", Server: "smtp.163.com", Port: 465,
		Username: "", Password: "pw", Limit: 100,
	}); err != nil {
		t.Fatalf("建残缺 env:163 行失败: %v", err)
	}
	if err := repo.Create(context.Background(), &model.EmailSmtp{
		Name: "env:qq", Server: "smtp.qq.com", Port: 465,
		Username: "backup@qq.com", Password: "qqpw", Limit: 100,
	}); err != nil {
		t.Fatalf("建 env:qq 行失败: %v", err)
	}

	svc := NewEmailSendService()
	got := svc.resolveSmtpFromDB(context.Background())
	if got == nil || got.Name != "env:qq" {
		t.Errorf("残缺 163 行应跳过并选 env:qq，实际 %+v", got)
	}
}

// TestResolveSmtpFromEnv_BackedUpByDBChannel env 的 163 通道配齐时仍走 env
// （env 优先级最高，DB 命名行只是备用）；env 缺 163 时 163 命名行才顶上。
func TestResolveSmtpFromEnv_BackedUpByDBChannel(t *testing.T) {
	setupEmailSendServiceTestDB(t)
	clearSmtpEnv(t)
	repo := repository.NewEmailSmtpRepository()
	if err := repo.Create(context.Background(), &model.EmailSmtp{
		Name: "env:163", Server: "smtp.163.com", Port: 465,
		Username: "backup163@163.com", Password: "163pw", Limit: 100,
	}); err != nil {
		t.Fatalf("建 env:163 行失败: %v", err)
	}

	// env 配齐 163：resolveSmtpFromEnv 返回非 nil，DB 命名行不该被走到。
	t.Setenv("EMAIL_163_USER", "env163@163.com")
	t.Setenv("EMAIL_163_PASSWORD", "envpw")
	got := resolveSmtpFromEnv()
	if got == nil {
		t.Fatal("env 配齐 163 时 resolveSmtpFromEnv 不应为 nil")
	}
	if got.Username != "env163@163.com" || got.Password != "envpw" {
		t.Errorf("env 优先级：got %+v, 期望 env 凭据（env 必须压过 DB 命名行）", got)
	}
}
