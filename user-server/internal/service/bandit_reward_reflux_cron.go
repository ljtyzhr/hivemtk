package service

import (
	"context"
	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/utils"
	"hivemtk-user/internal/pkg/utils/logger"
	feedbackloop "hivemtk-user/internal/service/feedback_loop"
)

// runBanditRewardReflux 每小时按租户回流一次奖励
//
// 游标按租户分别保存。早期实现把游标放在外层、在 Fanout 回调里再套一个永久
// 循环，导致只有第一个租户被处理且 Fanout 永不返回；游标也全局共享，租户
// 之间会互相推进时间窗。
func (c *FeedbackLoopCron) runBanditRewardReflux(ctx context.Context, db *gorm.DB) {
	defer c.wg.Done()
	cursors := make(map[uint]time.Time)
	for {
		select {
		case <-c.stopCh:
			return
		case <-time.After(time.Hour):
		}
		now := time.Now()
		runTenant := func(tctx context.Context, tenantID uint) {
			if c.components == nil || c.components.Bandit == nil || db == nil {
				return
			}
			since, ok := cursors[tenantID]
			if !ok {
				since = now.Add(-time.Hour)
			}
			wctx, cancel := context.WithTimeout(tctx, utils.CronShortTimeout)
			stats, err := feedbackloop.NewBanditRewardReflux(db, c.components.Bandit).RefluxOnce(wctx, since, now)
			cancel()
			if err != nil {
				logger.Ctx(tctx).Error().Err(err).Msg("[cron] bandit reward reflux failed")
				return
			}
			// 游标只在成功后推进：失败窗口留待下一轮重扫，重复入账由台账唯一键兜住。
			cursors[tenantID] = now
			logger.Ctx(tctx).Info().
				Int("scanned", stats.Scanned).
				Int("refluxed", stats.Refluxed).
				Int("skipped", stats.Skipped).
				Int("duped", stats.Duped).
				Int("failed", stats.Failed).
				Msg("[cron] bandit reward reflux done")
		}
		if tenant.IsMultiTenant() {
			_, _ = tenant.Fanout(ctx, db, func(tctx context.Context, tenantID uint) (int64, error) {
				runTenant(tctx, tenantID)
				return 0, nil
			})
			continue
		}
		runTenant(ctx, 0)
	}
}
