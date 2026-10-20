package migrations

import (
	"context"
	"fmt"

	browsermodel "hivemtk-user/internal/browser_automation/model"
	"hivemtk-user/internal/migration"
	"hivemtk-user/internal/pkg/utils/logger"

	"gorm.io/gorm"
)

// BrowserCronTriggerTombstonePruneMigration v3.67.0：摘掉 browser_cron_triggers 里的软删残留行。
//
// 这些行是「删除走软删、而唯一键位不认软删」留下的疤。触发器上有一把 (tenant_id, task_id)
// 唯一约束（uk_browser_cron_task / 模型 tag 的 uniqueIndex），它守的是「一条 cron 任务只有一个
// 触发器」；软删的行仍然占着键位，而写侧重名预检查（GetByTaskID 带 deleted_at IS NULL）读不到它，
// 于是直接放行到 INSERT——撞约束，用户看到的是 500。
//
// 为什么必须在库里删干净，而不是只修删除侧：删除侧已经改成硬删（repository/cron.go 的
// Delete/DeleteByTaskID 走 Unscoped），那只是不再制造新疤；已经躺在库里的残留会继续把任务锁死。
// 开发库实测：53 行里 52 行是软删残留，50 条在世的 cron 任务因此一个触发器都挂不上——
// RestoreAll 又只看活行，于是这些任务在界面上「配过定时」而实际永不执行，比报错更难发现。
//
// 残留行没有历史读家：触发器的执行事实记在 browser_sessions / browser_command_log，
// 全仓没有任何路径读软删触发行（唯一会读到它们的 Unscoped 查询只有本迁移与测试夹具）。
// 留着它只换来故障，删掉它不丢任何被读过的东西。
type BrowserCronTriggerTombstonePruneMigration struct {
	db *gorm.DB
}

var _ migration.Migration = (*BrowserCronTriggerTombstonePruneMigration)(nil)

func NewBrowserCronTriggerTombstonePruneMigration(db *gorm.DB) *BrowserCronTriggerTombstonePruneMigration {
	return &BrowserCronTriggerTombstonePruneMigration{db: db}
}

func (m *BrowserCronTriggerTombstonePruneMigration) Version() string { return "v3.67.0" }

func (m *BrowserCronTriggerTombstonePruneMigration) Name() string {
	return "清理浏览器定时触发器的软删残留行（释放 (tenant_id, task_id) 唯一键位）"
}

func (m *BrowserCronTriggerTombstonePruneMigration) Description() string {
	return "物理删除 browser_cron_triggers 中 deleted_at IS NOT NULL 的残留行；" +
		"这些行占着 (tenant_id, task_id) 唯一键位，让同一任务再也挂不上新触发器（删了再建必 500）"
}

func (m *BrowserCronTriggerTombstonePruneMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	if !m.db.WithContext(ctx).Migrator().HasTable(&browsermodel.BrowserCronTrigger{}) {
		// 表还没建（全新部署的迁移位）：没有残留，自然没有可摘的东西。
		return nil
	}

	var tombstones int64
	if err := m.db.WithContext(ctx).Unscoped().Model(&browsermodel.BrowserCronTrigger{}).
		Where("deleted_at IS NOT NULL").Count(&tombstones).Error; err != nil {
		return fmt.Errorf("count browser_cron_triggers tombstones failed: %w", err)
	}
	if tombstones == 0 {
		return nil // 0 行是正常收口（重复执行 / 干净库），不是失败
	}

	// 数与删同事务：避免「日志说摘了 52 行、库里其实只少了 3 行」这种对不上账的读数。
	err := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Unscoped().Where("deleted_at IS NOT NULL").
			Delete(&browsermodel.BrowserCronTrigger{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != tombstones {
			return fmt.Errorf("prune browser_cron_triggers tombstones: counted %d, deleted %d",
				tombstones, res.RowsAffected)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("prune browser_cron_triggers tombstones failed: %w", err)
	}

	logger.Infof("[migration %s] 摘除定时触发器软删残留 %d 行，其键位已归还对应任务",
		m.Version(), tombstones)
	return nil
}

// Down 不回填：残留行不是事实，而是缺陷的产物——把 deleted_at 重新写回去等于把 50 条任务
// 再次永久锁死在挂不上触发器的状态。真要恢复某个触发器，正确动作是在管理台重新配一次。
func (m *BrowserCronTriggerTombstonePruneMigration) Down(ctx context.Context) error {
	return nil
}
