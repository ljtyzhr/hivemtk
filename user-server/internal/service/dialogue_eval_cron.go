// dialogue_eval_cron.go 对话级 gold 评测每日定时（P0-1）。形状对齐 rag_eval_cron.go：
// 每日一次 → AllowWorker 模块闸 → Fanout 按租户跑 → 落 dialogue_eval_runs。
package service

import (
	"context"
	"sync"
	"time"

	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/utils/logger"
)

type DialogueEvalCron struct {
	svc *DialogueEvalService

	runFn func(ctx context.Context) error

	stop      chan struct{}
	wg        sync.WaitGroup
	startOnce sync.Once
}

// NewDialogueEvalCron 创建。badCase 导出送料默认每次 20 条（BadCaseDefaultExportLimit 量级）。
func NewDialogueEvalCron() *DialogueEvalCron {
	c := &DialogueEvalCron{
		svc:  NewDialogueEvalService(GlobalBadCaseService()),
		stop: make(chan struct{}),
	}
	c.runFn = func(ctx context.Context) error {
		_, err := tenant.Fanout(ctx, nil, func(ctx context.Context, _ uint) (int64, error) {
			_, err := c.svc.RunGoldEvaluation(ctx, "dialogue_gold_"+time.Now().Format("20060102"), 20)
			return 0, err
		})
		return err
	}
	return c
}

// Start 启动（每日 03:55 首跳——错开 rag_eval 的 03:40——过后每 24h 执行）。
func (c *DialogueEvalCron) Start(ctx context.Context) {
	c.startOnce.Do(func() {
		c.wg.Add(1)
		go c.loop(ctx)
	})
}

// Stop 优雅停止
func (c *DialogueEvalCron) Stop(_ context.Context) {
	select {
	case <-c.stop:
	default:
		close(c.stop)
	}
	c.wg.Wait()
}

func (c *DialogueEvalCron) loop(_ context.Context) {
	defer c.wg.Done()
	for {
		next := nextRagEvalRun(3, 55) // 复用同族 next-run 计算（3:55 档）
		select {
		case <-c.stop:
			return
		case <-time.After(time.Until(next)):
		}
		// 模块闸（FR-5.6，派发时刻判定）：aiagent 模块无任何租户开通则跳过。
		if !AllowWorker(context.Background(), "dialogue_eval") {
			logger.Info("[dialogue_eval_cron] aiagent 模块无任何租户开通，跳过本轮对话评测")
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		if err := c.runFn(ctx); err != nil {
			logger.Ctx(ctx).Error().Err(err).Msg("[dialogue_eval_cron] daily dialogue evaluation failed")
		} else {
			logger.Ctx(ctx).Info().Msg("[dialogue_eval_cron] daily dialogue evaluation done")
		}
		cancel()
	}
}
