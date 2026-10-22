package migrations

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// TenantOrphanRowRepairMigration v3.76.0：把 tenant_id=0 的孤儿行归入默认租户。
//
// 背景（任务三逐路由 curl 实测）：churn_predictions / customer_rfm 两张表的
// 写入曾用 `Save(整结构体)` 回写，把「调用方从未赋值的 tenant_id」（零值 0）
// 一并写进 SET 子句：
//
//   - on 模式：撞「更新不可改写 tenant_id」闸门 ⇒ 整条写入链路 500；
//   - 落库形态：行带着 tenant_id=0 —— 那不属于任何租户，于是
//     **任何租户都读不到它**（连离线单机档的默认租户 1 也读不到），
//     等于这批流失预测 / RFM 数据对所有人隐身。
//
// 写入侧已分别改为「按主键定向更新 + 显式列出业务列」（tenant_id 交给隔离
// callback），本迁移只收拾**存量**：把 tenant_id=0 的行归到默认租户 1，
// 与「存量数据升级后归入默认租户」的既有口径一致。
//
// 幂等：只处理 tenant_id=0 的行，跑第二遍无行可改。
// 表不存在（全新库尚未 AutoMigrate）时跳过而不是失败 —— 迁移不能因为
// 一张还没建的表把整个启动链卡住。
type TenantOrphanRowRepairMigration struct {
	db *gorm.DB
}

// NewTenantOrphanRowRepairMigration 创建孤儿行归位迁移
func NewTenantOrphanRowRepairMigration(db *gorm.DB) *TenantOrphanRowRepairMigration {
	return &TenantOrphanRowRepairMigration{db: db}
}

// Version 返回版本号
func (m *TenantOrphanRowRepairMigration) Version() string { return "v3.76.0" }

// Name 返回迁移名称
func (m *TenantOrphanRowRepairMigration) Name() string {
	return "孤儿行 tenant_id=0 归入默认租户"
}

// Description 返回迁移描述
func (m *TenantOrphanRowRepairMigration) Description() string {
	return "SaaS 多租户：churn_predictions / customer_rfm 存量 tenant_id=0 的行归入默认租户 1（写入侧已修，不再产生新孤儿行）"
}

// orphanTables 本迁移要收拾的表（表名 → 该表是否需要 tenant_id 列）。
var orphanTables = []string{
	"churn_predictions",
	"customer_rfm",
}

// Up 执行升级
func (m *TenantOrphanRowRepairMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	for _, table := range orphanTables {
		if !m.tableExists(ctx, table) {
			continue
		}
		// 两步走，顺序不能反：
		//
		// ① 先删「在目标租户已有同一业务键的行」的孤儿行 —— 那些是同一条业务
		//    记录的重复副本（唯一键 (tenant_id, customer_id) 撞车就是它撞的），
		//    保留目标租户那条即可。若先归位，UPDATE 必然撞唯一键 23505，
		//    整轮迁移失败，存量行原地不动。
		// ② 再把「没有对应行的孤儿行」归位到默认租户 —— 它们是真的漏了归属。
		//
		// 业务键：customer_rfm 是 customer_id，churn_predictions 是 user_id。
		del := m.db.WithContext(ctx).Exec(fmt.Sprintf(`
			DELETE FROM %q o
			WHERE o.tenant_id = 0
			  AND EXISTS (
				SELECT 1 FROM %q t
				WHERE t.tenant_id = 1 AND t.%s = o.%s
			  )`, table, table, m.businessKey(table), m.businessKey(table)))
		if del.Error != nil {
			return fmt.Errorf("清理 %s 的重复孤儿行失败: %w", table, del.Error)
		}
		if del.RowsAffected > 0 {
			m.db.Logger.Info(ctx, "[migration] 重复孤儿行已清理",
				"table", table, "rows", del.RowsAffected)
		}

		upd := m.db.WithContext(ctx).Exec(fmt.Sprintf(
			`UPDATE %q SET tenant_id = 1 WHERE tenant_id = 0`, table))
		if upd.Error != nil {
			return fmt.Errorf("归位 %s 的孤儿行失败: %w", table, upd.Error)
		}
		if upd.RowsAffected > 0 {
			m.db.Logger.Info(ctx, "[migration] 孤儿行归位完成",
				"table", table, "rows", upd.RowsAffected)
		}
	}
	return nil
}

// businessKey 该表的业务键列（同一租户内唯一的那一列）。
func (m *TenantOrphanRowRepairMigration) businessKey(table string) string {
	if table == "churn_predictions" {
		return "user_id"
	}
	return "customer_id"
}

// Down 不回滚：把行改回 tenant_id=0 等于把它们重新变成谁也读不到的孤儿行，
// 而「看不见的数据」比「多算了几行统计」坏得多。归位本身是幂等的，
// 重跑一次即可（此时已无行可改）。
func (m *TenantOrphanRowRepairMigration) Down(ctx context.Context) error {
	return nil
}

// tableExists 表是否在库里
func (m *TenantOrphanRowRepairMigration) tableExists(ctx context.Context, table string) bool {
	var n int64
	err := m.db.WithContext(ctx).Raw(
		`SELECT count(*) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = ?`,
		table,
	).Scan(&n).Error
	return err == nil && n > 0
}
