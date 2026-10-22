package service

import (
	"context"
	"time"

	"hivemtk-user/internal/pkg/utils"
	"hivemtk-user/internal/pkg/utils/logger"
)

// 平台消息同步周期。
//
// 取 3 分钟的判据：平台心跳上报周期同为 3 分钟，说明平台↔用户端链路在
// 这个尺度上已验证稳定；而通知的可见性要求是"发布后 5 分钟内送达"
// （spec SC-002），3 分钟留有余量。再短会把 HMAC 往返放大到每天 480 次
// 以上，对当前规模（十几个租户）没有收益。
const noticeSyncInterval = 3 * time.Minute

// noticeSyncRunner 平台消息同步循环。
//
// 抽成结构体是为了让 Stop 有确切归属，且便于单测用短周期驱动它
// （否则测试要么真等 3 分钟，要么只能测单次调用）。
type noticeSyncRunner struct {
	cancel    context.CancelFunc
	done      chan struct{}
	firstDone chan struct{}
}

// StartNoticeSync 启动平台消息同步后台任务。
//
// 首轮**同步执行**（不是丢进 goroutine）：服务刚起来的时刻通知中心里
// 应该已经有平台此前发过的消息，异步的话会留一段空白窗口。
//
// 首轮失败不阻断启动 —— 平台不可达是常态（离线部署），此时用户端一切
// 照常可用，只是新消息延迟到达。
//
// 返回的 runner 可被 StopNoticeSync 停掉（测试与优雅退出用）。
func StartNoticeSync(parent context.Context) *noticeSyncRunner {
	ctx, cancel := context.WithCancel(parent)
	r := &noticeSyncRunner{
		cancel:    cancel,
		done:      make(chan struct{}),
		firstDone: make(chan struct{}),
	}
	svc := NewNoticeSyncService()

	res, err := svc.Sync(ctx)
	if err != nil {
		logger.Warnf("[notice-sync] 首轮同步失败（按降级处理）: %v", err)
	} else {
		logger.Infof("[notice-sync] 首轮同步完成：拉取 %d 条，新投递 %d 条（租户 %d 个，跳过=%v，降级=%v）",
			res.Fetched, res.Delivered, res.Tenants, res.Skipped, res.Degraded)
	}
	close(r.firstDone)

	utils.SafeGo(ctx, "notice.sync.loop", func(ctx context.Context) {
		defer close(r.done)
		ticker := time.NewTicker(noticeSyncInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				res, err := svc.Sync(ctx)
				if err != nil {
					logger.Warnf("[notice-sync] 周期同步失败: %v", err)
					continue
				}
				if res.Delivered > 0 {
					logger.Infof("[notice-sync] 周期同步投递了 %d 条新消息", res.Delivered)
				}
			}
		}
	})

	logger.Infof("[notice-sync] 平台消息同步已启动，周期 %s", noticeSyncInterval)
	return r
}

// Stop 停止同步循环并等待退出。
func (r *noticeSyncRunner) Stop() {
	if r == nil {
		return
	}
	r.cancel()
	select {
	case <-r.done:
	case <-time.After(3 * time.Second):
		logger.Warn("[notice-sync] 停止等待超时（3s），协程可能仍在收尾")
	}
}

// WaitFirstSync 等待首轮同步完成（测试用）。
func (r *noticeSyncRunner) WaitFirstSync() { <-r.firstDone }
