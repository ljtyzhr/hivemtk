package migrations

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// PaymentReasonMigration v3.84.0（E-10 部分退款行补"原因"格，2026-10-24）：
// payments 加可空 reason 列。
//
// 背景（规划 §4.10）：退款行的验收四格是"金额、原因、渠道引用、状态"。
// 并行批 c9b2f265 收编部分退款时落了三格，"原因"格缺失——渠道给的退款
// 原因（如"仅退款不退货"/"商品退货"）没处放，对账时只能靠翻渠道后台。
// 本迁移把这一格补成一等列：refunded 行填渠道原因（空=渠道没说），
// 其余状态行恒空。
//
// 无回填：列是新增的语义（历史行没有"原因"这个事实可恢复），不猜。
// 幂等可重入：列已存在则跳过。
type PaymentReasonMigration struct {
	db *gorm.DB
}

var _ migration.Migration = (*PaymentReasonMigration)(nil)

func NewPaymentReasonMigration(db *gorm.DB) *PaymentReasonMigration {
	return &PaymentReasonMigration{db: db}
}

func (m *PaymentReasonMigration) Version() string { return "v3.84.0" }

func (m *PaymentReasonMigration) Name() string {
	return "payments.reason"
}

func (m *PaymentReasonMigration) Description() string {
	return "payments 加可空 reason 列（E-10 退款行四格之「原因」：refunded 行填渠道退款原因，其余状态恒空）"
}

func (m *PaymentReasonMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	db := m.db.WithContext(ctx)
	if !db.Migrator().HasTable("payments") {
		return nil // 全新部署：AutoMigrate 建表时模型 tag 已带列
	}
	if db.Migrator().HasColumn(paymentColumnProbe{}, "reason") {
		return nil // 幂等重入
	}
	if err := db.Exec(
		`ALTER TABLE payments ADD COLUMN reason varchar(255)`).Error; err != nil {
		return fmt.Errorf("加 payments.reason 列失败: %w", err)
	}
	return nil
}

func (m *PaymentReasonMigration) Down(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	// reason 列由模型声明（AutoMigrate 持有），降级不销毁在用对象
	// （与 v3.82.0 declineColumnDrop 同一取舍：删掉列会把已落库的退款原因
	// 变成不可恢复的数据丢失，多留一个可空列不破坏任何功能）。
	declineColumnDrop(m.Version(), "payments.reason")
	return nil
}

// paymentColumnProbe 只为 Migrator().HasColumn 服务（按 TableName 判列存在性）。
type paymentColumnProbe struct{}

func (paymentColumnProbe) TableName() string { return "payments" }
