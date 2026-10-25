package migrations

import (
	"context"
	"fmt"
	"log"

	"hivemtk-user/internal/migration"
	"hivemtk-user/internal/pkg/crypto"

	"gorm.io/gorm"
)

type EmailSmtpPasswordEncryptMigration struct {
	db *gorm.DB
}

var _ migration.Migration = (*EmailSmtpPasswordEncryptMigration)(nil)

func NewEmailSmtpPasswordEncryptMigration(db *gorm.DB) *EmailSmtpPasswordEncryptMigration {
	return &EmailSmtpPasswordEncryptMigration{db: db}
}

func (m *EmailSmtpPasswordEncryptMigration) Version() string { return "v3.28.0" }

func (m *EmailSmtpPasswordEncryptMigration) Name() string {
	return "email_smtp 存量明文密码一次性 AES-GCM 加密"
}

func (m *EmailSmtpPasswordEncryptMigration) Description() string {
	return "R50 fail-closed 配套: 把 FIELD_ENCRYPTION_KEY 缺失期间明文落库的 SMTP 密码批量加密"
}

func (m *EmailSmtpPasswordEncryptMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}

	if err := crypto.Init(); err != nil {
		return fmt.Errorf("FIELD_ENCRYPTION_KEY 不可用, 拒绝执行明文密码加密迁移: %w", err)
	}

	type row struct {
		ID       string
		Password string
	}
	var rows []row
	if err := m.db.WithContext(ctx).
		Table("email_smtp").
		Select("id, password").
		Where("password IS NOT NULL AND password != ''").
		Find(&rows).Error; err != nil {
		return fmt.Errorf("查询 email_smtp 失败: %w", err)
	}

	for _, r := range rows {

		if plain, derr := crypto.Decrypt(r.Password); derr == nil && plain != "" {
			continue
		}
		enc, err := crypto.Encrypt(r.Password)
		if err != nil {
			return fmt.Errorf("加密 email_smtp %s 密码失败: %w", r.ID, err)
		}
		if enc == r.Password {
			return fmt.Errorf("email_smtp %s 加密输出与输入一致, 异常终止", r.ID)
		}
		if err := m.db.WithContext(ctx).
			Table("email_smtp").
			Where("id = ?", r.ID).
			Update("password", enc).Error; err != nil {
			return fmt.Errorf("回写 email_smtp %s 密文失败: %w", r.ID, err)
		}
	}
	return nil
}

func (m *EmailSmtpPasswordEncryptMigration) Down(ctx context.Context) error {
	// 显式 no-op：把密码解密回明文是安全倒退，所以"撤销"这一步什么都不做
	// （数据继续保持密文，安全方向上的默认值）。
	//
	// 为什么不是 `return error`：Down 报错会让 POST /api/migration/rollback 整链
	// 中止在这条上（后续迁移一条都不退），而这条本来就不需要退任何对象——
	// 用失败表达"没什么可退"会把降级能力整体锁死。口径同 down_drop_policy.go：
	// 降级只撤自己 Up 新建的对象，撤不了就不动。
	log.Printf("[migration v3.28.0] 放弃回滚：密码解密回明文是安全倒退（Down 为显式 no-op，密文保持）")
	return nil
}
