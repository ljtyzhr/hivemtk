package migrations

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// LLMSeparatePricingMigration v3.62.0：llm_providers 增「输入/输出分列单价 + 来源 + 套餐」五列。
//
// 为什么必须是迁移而不是只靠 AutoMigrate：
// ① AutoMigrate 只**新增**列，改列类型/约束的历史路径上不可靠；
// ② 新列带 NOT NULL DEFAULT，AutoMigrate 在大表上会全表重写，
//
//	而 llm_providers 行数虽小，但这条迁移的价值恰恰在于「可回滚、可审计」；
//
// ③ 未来若有人把 Source 的 DEFAULT 改错，要有版本号能指出「哪一版改的」。
//
// 五列的语义（缺一不可）：
//   - cost_per_1k_input / cost_per_1k_output：需求原文「输入和输出分别计价」的落点。
//     **只配 cost_per_1k 的老 provider 行为完全不变**（两列回落成同价），
//     这是「不破坏存量成本统计」的硬要求。
//   - source：self=商户自带 Key（不计费）/ platform=平台垫资（走平台代理 + 计量扣费）。
//     DEFAULT 'self' —— 漏填的绝大多数是用户自己加的 provider，
//     落成 platform 会凭空记一笔欠平台的账。
//   - plan_code：平台套餐号，platform provider 的扣费单价来源。
//
// 幂等：全部 ADD COLUMN IF NOT EXISTS，重复执行无副作用。
type LLMSeparatePricingMigration struct {
	db *gorm.DB
}

// NewLLMSeparatePricingMigration 创建迁移实例
func NewLLMSeparatePricingMigration(db *gorm.DB) *LLMSeparatePricingMigration {
	return &LLMSeparatePricingMigration{db: db}
}

// Version 返回版本号
func (m *LLMSeparatePricingMigration) Version() string { return "v3.62.0" }

// Name 返回迁移名称
func (m *LLMSeparatePricingMigration) Name() string {
	return "LLM 输入/输出分列计价与 provider 来源"
}

// Up 执行迁移
func (m *LLMSeparatePricingMigration) Up(ctx context.Context) error {
	ddls := []string{
		`ALTER TABLE llm_providers ADD COLUMN IF NOT EXISTS cost_per_1k_input DOUBLE PRECISION NOT NULL DEFAULT 0`,
		`ALTER TABLE llm_providers ADD COLUMN IF NOT EXISTS cost_per_1k_output DOUBLE PRECISION NOT NULL DEFAULT 0`,
		`ALTER TABLE llm_providers ADD COLUMN IF NOT EXISTS source VARCHAR(16) NOT NULL DEFAULT 'self'`,
		`ALTER TABLE llm_providers ADD COLUMN IF NOT EXISTS plan_code VARCHAR(32)`,
	}
	for _, ddl := range ddls {
		if err := m.db.WithContext(ctx).Exec(ddl).Error; err != nil {
			return fmt.Errorf("执行 DDL 失败 (%s): %w", ddl, err)
		}
	}
	// source 建索引：模块闸与"哪些 provider 是平台垫资"的查询要按它筛
	if err := m.db.WithContext(ctx).
		Exec(`CREATE INDEX IF NOT EXISTS idx_llm_providers_source ON llm_providers (source)`).Error; err != nil {
		return fmt.Errorf("创建 source 索引失败: %w", err)
	}
	// 回填：存量行显式标成 self。
	// 理论上 DEFAULT 已覆盖，这一步是为了把「历史上有没有被别的路径写成 NULL」
	// 的行也一并归位 —— NULL 会让 ProviderSource() 的判空逻辑走到
	// "不是 platform 也不是 self"的分支，而不是干净地回落 self。
	if err := m.db.WithContext(ctx).
		Exec(`UPDATE llm_providers SET source = 'self' WHERE source IS NULL OR source = ''`).Error; err != nil {
		return fmt.Errorf("回填 source 失败: %w", err)
	}
	return nil
}

// Down 回滚
//
// 刻意**不删列**：drop 掉会让所有存量成本统计失去分列口径，
// 而本迁移没有任何数据变换（只加列 + 标默认值），回滚留着无害。
func (m *LLMSeparatePricingMigration) Down(ctx context.Context) error {
	return nil
}

// Description 返回迁移描述
func (m *LLMSeparatePricingMigration) Description() string {
	return "为 llm_providers 增加 cost_per_1k_input / cost_per_1k_output（输入输出分列单价）、" +
		"source（self 自带 Key / platform 平台垫资）、plan_code（平台套餐号）。" +
		"只配 cost_per_1k 的存量 provider 行为不变（两列回落成同价），" +
		"因此本迁移不触碰任何历史成本数据。"
}
