package service

import (
	"context"
	"hivemtk-user/internal/pkg/tenant"
	"sync"
	"time"

	"hivemtk-user/internal/pkg/utils/logger"
)

type RagEvalCron struct {
	svc *RagEvalAutoService

	runFn func(ctx context.Context) error

	stop      chan struct{}
	wg        sync.WaitGroup
	startOnce sync.Once
}

// NewRagEvalCron 创建评测 cron
func NewRagEvalCron() *RagEvalCron {
	c := &RagEvalCron{
		svc:  NewRagEvalAutoService(),
		stop: make(chan struct{}),
	}
	c.runFn = func(ctx context.Context) error {
		_, err := tenant.Fanout(ctx, nil, func(ctx context.Context, _ uint) (int64, error) {
			_, err := c.svc.RunAutoEvaluation(ctx, &RagEvalConfig{
				Name:         "daily_auto_" + time.Now().Format("20060102"),
				MaxQuestions: 30,
			})
			return 0, err
		})
		return err
	}
	return c
}

// Start 启动（每日 03:40 首跳过后每 24h 执行）
func (c *RagEvalCron) Start(ctx context.Context) {
	c.startOnce.Do(func() {
		c.wg.Add(1)
		go c.loop(ctx)
	})
}

// Stop 优雅停止
func (c *RagEvalCron) Stop(_ context.Context) {
	select {
	case <-c.stop:
	default:
		close(c.stop)
	}
	c.wg.Wait()
}

func (c *RagEvalCron) loop(_ context.Context) {
	defer c.wg.Done()
	for {
		next := nextRagEvalRun(3, 40)
		select {
		case <-c.stop:
			return
		case <-time.After(time.Until(next)):
		}
		// 模块闸（FR-5.6，派发时刻判定）：knowledge 模块无任何租户开通则
		// 本轮评测跳过；未映射/DB 错误放行（fail-open）见 module_runtime_filter.go。
		if !AllowWorker(context.Background(), "rag_eval") {
			logger.Info("[rag_eval_cron] knowledge 模块无任何租户开通，跳过本轮每日评测")
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		if err := c.runFn(ctx); err != nil {
			logger.Ctx(ctx).Error().Err(err).Msg("[rag_eval_cron] daily evaluation failed")
		} else {
			logger.Ctx(ctx).Info().Msg("[rag_eval_cron] daily evaluation done")
		}
		cancel()
	}
}

func nextRagEvalRun(hour, minute int) time.Time {
	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}
