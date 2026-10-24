package migrations

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// MessageHubPlatformMsgIDMigration v3.82.0（D-9 存储半项，2026-10-24）：
// message_hub 加可空 platform_msg_id 列 + (platform, account_id, platform_msg_id)
// 复合索引，并把存量出站行按行键格式回填。
//
// 背景（规划 §3.9）：hub 行键 msg_id 是 ContentHash/合成键，不是平台消息号。
// Recall 反查与 echo 拦截此前只能按行键"解析"（platformMessageID 对 TG/飞书
// 取末段、企微读 Extra["msgid"]），WA 的行键本体是 wamid——合同不统一。
// 本迁移把"平台侧消息号"落成一等列，对外回执合同统一 {hub_id, platform_msg_id?}。
//
// 回填口径（只回填**能无歧义解析**的行，历史行不猜）：
//   - tg-out-{account}-{平台号} / feishu-out-{account}-{平台id}：行键内嵌平台号，
//     且 direction=outbound 才回填（入站行键是 ContentHash，末段不是平台号）；
//   - whatsapp direction=outbound：行键就是 wamid 本体；
//   - wecom：平台号在 Extra["msgid"]，发送侧（wecom_integration.go）自本批起
//     直接写 platform_msg_id 列；旧行按 Extra 回填；
//   - 其余（桥接合成键/ContentHash）：留空，Recall 反查走原有解析路径兜底。
//
// 幂等可重入：列已存在则跳过；回填语句带 WHERE platform_msg_id IS NULL。
type MessageHubPlatformMsgIDMigration struct {
	db *gorm.DB
}

var _ migration.Migration = (*MessageHubPlatformMsgIDMigration)(nil)

func NewMessageHubPlatformMsgIDMigration(db *gorm.DB) *MessageHubPlatformMsgIDMigration {
	return &MessageHubPlatformMsgIDMigration{db: db}
}

func (m *MessageHubPlatformMsgIDMigration) Version() string { return "v3.82.0" }

func (m *MessageHubPlatformMsgIDMigration) Name() string {
	return "message_hub.platform_msg_id"
}

func (m *MessageHubPlatformMsgIDMigration) Description() string {
	return "message_hub 加 platform_msg_id 列 + (platform, account_id, platform_msg_id) 索引；存量行按行键格式回填（不能无歧义解析的留空）"
}

func (m *MessageHubPlatformMsgIDMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	db := m.db.WithContext(ctx)
	if !db.Migrator().HasTable("message_hub") {
		return nil // 全新部署：AutoMigrate 建表时模型 tag 已带列与索引
	}

	if !db.Migrator().HasColumn(messageHubColumnProbe{}, "platform_msg_id") {
		if err := db.Exec(
			`ALTER TABLE message_hub ADD COLUMN platform_msg_id varchar(100)`).Error; err != nil {
			return fmt.Errorf("加 platform_msg_id 列失败: %w", err)
		}
	}

	// 回填：TG/飞书行键末段；WA 行键本体；企微 Extra["msgid"]（JSONB/text 的 Extra
	// 用 ::jsonb 取键，取不出或非字符串即 NULL——不猜）。
	// WHERE 用 IS NULL OR = ''：GORM Create 会把 Go 零值写成空串而不是 NULL，
	// 只判 IS NULL 会让所有"经 ORM 写入的行"永远绕过回填。
	if err := db.Exec(`
		UPDATE message_hub SET platform_msg_id = CASE
			WHEN platform IN ('telegram','feishu')
			     AND direction = 'outbound'
			     AND msg_id LIKE '%-out-%'
			     AND msg_id NOT LIKE '%mh:%'
				THEN ltrim((regexp_match(msg_id, '-[^-]+$'))[1], '-')
			WHEN platform = 'whatsapp'
			     AND direction = 'outbound'
			     AND msg_id NOT LIKE '%mh:%'
				THEN msg_id
			ELSE NULL
		END
		WHERE (platform_msg_id IS NULL OR platform_msg_id = '')
		  AND (
			(platform IN ('telegram','feishu') AND direction = 'outbound'
			     AND msg_id LIKE '%-out-%' AND msg_id NOT LIKE '%mh:%')
			OR (platform = 'whatsapp' AND direction = 'outbound' AND msg_id NOT LIKE '%mh:%')
		  )`).Error; err != nil {
		return fmt.Errorf("回填 TG/飞书/WA 平台号失败: %w", err)
	}

	if err := db.Exec(`
		UPDATE message_hub
		SET platform_msg_id = extra::jsonb ->> 'msgid'
		WHERE (platform_msg_id IS NULL OR platform_msg_id = '')
		  AND platform = 'wecom'
		  AND direction = 'outbound'
		  AND extra IS NOT NULL
		  AND jsonb_typeof(extra::jsonb) = 'object'
		  AND (extra::jsonb ->> 'msgid') IS NOT NULL`).Error; err != nil {
		// Extra 列类型如果不是合法 JSON（text 存了别的），这一句会报错——降级为跳过，
		// 不阻断迁移：回填是尽力而为，读路径有 platformMessageID 解析兜底。
		fmt.Printf("post-migrate: 回填 wecom Extra[msgid] 跳过（extra 非合法 JSON 或类型不符）: %v\n", err)
	}

	if err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_hub_platform_msg
		ON message_hub (platform, account_id, platform_msg_id)`).Error; err != nil {
		return fmt.Errorf("建 idx_hub_platform_msg 失败: %w", err)
	}
	return nil
}

func (m *MessageHubPlatformMsgIDMigration) Down(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	db := m.db.WithContext(ctx)
	if err := db.Exec(`DROP INDEX IF EXISTS idx_hub_platform_msg`).Error; err != nil {
		return err
	}
	// platform_msg_id 列由模型声明（AutoMigrate 持有），降级不销毁在用对象
	// （down_drop_policy.go 同一取舍：多留一个可空列不破坏功能，删掉则当前进程永不补回）。
	declineColumnDrop(m.Version(), "message_hub.platform_msg_id")
	return nil
}

// messageHubColumnProbe 只为 Migrator().HasColumn 服务（按 TableName 判列存在性）。
type messageHubColumnProbe struct{}

func (messageHubColumnProbe) TableName() string { return "message_hub" }
