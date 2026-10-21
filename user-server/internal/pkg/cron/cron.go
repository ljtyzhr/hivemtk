package cron

import (
	"context"
	"fmt"
	geoservice "hivemtk-user/internal/geo/service"
	opsservice "hivemtk-user/internal/ops/service"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/utils"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/service"
	"sync"

	"github.com/robfig/cron/v3"
)

var (
	taskManager *TaskManager
	once        sync.Once
)

type TaskManager struct {
	cron *cron.Cron
	mu   sync.Mutex
}

func GetTaskManager() *TaskManager {
	once.Do(func() {
		taskManager = &TaskManager{
			cron: cron.New(
				cron.WithSeconds(),
				// panic 不击穿进程；上一轮未跑完的任务本轮跳过，防重叠执行
				cron.WithChain(cron.Recover(cron.PrintfLogger(logger.StdLogger())), cron.SkipIfStillRunning(cron.PrintfLogger(logger.StdLogger()))),
			),
		}
		taskManager.cron.Start()
	})
	return taskManager
}

// goTask 在 cron 触发器内部再以 SafeGo 起异步执行：
// SkipIfStillRunning 只对 Job 函数本身生效，Job 内立即返回的异步任务需自行收敛 recover。
func (tm *TaskManager) goTask(name string, fn func(ctx context.Context)) func() {
	return func() {
		utils.SafeGo(context.Background(), "cron."+name, fn)
	}
}

func (tm *TaskManager) AddTask(spec string, cmd func()) (cron.EntryID, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return tm.cron.AddFunc(spec, cmd)
}

func (tm *TaskManager) RemoveTask(id cron.EntryID) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.cron.Remove(id)
}

// fanoutTask 按活跃租户逐个执行后台任务。
//
// cron 交来的 ctx 不含租户，而带 tenant_id 列的表在 on 模式下要求绑定租户，
// 未绑定即被隔离守卫拒绝（活码、令牌、升级链规则都属此类）。
// Fanout 只在全部租户都失败时才把错误汇总回调用方，单租户失败由它就地出声并继续其余租户。
func fanoutTask(ctx context.Context, run func(ctx context.Context) error) error {
	_, err := tenant.Fanout(ctx, nil, func(ctx context.Context, _ uint) (int64, error) {
		return 0, run(ctx)
	})
	return err
}

func InitCron() {
	mgr := GetTaskManager()
	_, err := mgr.AddTask("0 * * * * *", func() {
		EmailListCron()
	})
	if err != nil {
		logger.Info(fmt.Sprintf("添加email列表定时任务失败 %s", err.Error()))
		panic(err)
	}

	_, err = mgr.AddTask("0 0 * * * *", func() {
		LiveCodeRotateCron()
	})
	if err != nil {
		logger.Info(fmt.Sprintf("添加活码轮询定时任务失败 %s", err.Error()))
		panic(err)
	}

	_, err = mgr.AddTask("0 30 3 * * *", func() {
		ChurnCalculationCron()
	})
	if err != nil {
		logger.Info(fmt.Sprintf("添加流失预测计算任务失败 %s", err.Error()))
		panic(err)
	}

	_, err = mgr.AddTask("0 0 4 * * *", func() {
		PasswordResetTokenCleanupCron()
	})
	if err != nil {
		logger.Info(fmt.Sprintf("添加密码重置令牌清理任务失败 %s", err.Error()))
		panic(err)
	}

	_, err = mgr.AddTask("0 */5 * * * *", mgr.goTask("bridge_offline_replay", func(ctx context.Context) {
		service.NewBridgeOfflineReplayService().RunOnce(ctx)
	}))
	if err != nil {
		logger.Info(fmt.Sprintf("添加离线消息回扫定时任务失败 %s", err.Error()))
		panic(err)
	}

	_, err = mgr.AddTask("5 */5 * * * *", mgr.goTask("handoff_chain", func(ctx context.Context) {
		// 升级链规则存放在租户维的参数表里：不逐租户注入 ctx 则每轮每条会话都被守卫拒于门外。
		cerr := fanoutTask(ctx, func(ctx context.Context) error {
			_, nerr := service.NewHandoffChainService().RunCron(ctx, 200)
			return nerr
		})
		if cerr != nil {
			logger.Info(fmt.Sprintf("工单升级链定时任务执行失败 %s", cerr.Error()))
		}
	}))
	if err != nil {
		logger.Info(fmt.Sprintf("添加工单升级链定时任务失败 %s", err.Error()))
		panic(err)
	}

	// GEO 任务的模块闸（FR-5.6）：每次 tick 由 service.AllowWorker 判
	// 「任一租户开通 GEO 才跑」。worker 名 → 模块 key 的映射在
	// service.workerModuleKey；必须在 SetupGeoJobs 之前注入。
	geoservice.SetGeoJobModuleGate(service.AllowWorker)
	geoservice.SetupGeoJobs(mgr)
	geoservice.GetGeoJobManager().SetJobManagerDB(db.GetDB())

	_, err = mgr.AddTask("0 5 3 * * *", mgr.goTask("daily_backup", func(ctx context.Context) {
		service.RunDailyBackup()
	}))
	if err != nil {
		logger.Info(fmt.Sprintf("添加每日自动备份定时任务失败 %s", err.Error()))
		panic(err)
	}
	logger.Info("[Backup] 每日自动备份 cron 已注册 (03:05:00)")
}

// ChurnCalculationCron 流失预测定时任务：从 customer_events 真实聚合全部客户行为并执行流失计算。
// 解决此前 churn_predictions 表永远为空（流失页面无真实数据）的断头功能问题。
func ChurnCalculationCron() {
	logger.Info("开始执行流失预测计算定时任务...")
	churnService := opsservice.NewChurnPredictionService()
	n, err := churnService.RunChurnCalculationForAllCustomers(context.Background())
	if err != nil {
		logger.Error(err, "流失预测计算定时任务执行失败")
		return
	}
	logger.Info(fmt.Sprintf("流失预测计算定时任务执行完成，覆盖客户数=%d", n))
}

// LiveCodeRotateCron 活码轮询定时任务
func LiveCodeRotateCron() {
	logger.Info("开始执行活码轮询定时任务...")

	liveCodeService := service.NewLiveCodeService(db.GetDB())

	if err := fanoutTask(context.Background(), liveCodeService.RotateLiveCodes); err != nil {
		logger.Error(err, "活码轮询定时任务执行失败")
		return
	}

	logger.Info("活码轮询定时任务执行完成")
}

// PasswordResetTokenCleanupCron 密码重置令牌清理定时任务
func PasswordResetTokenCleanupCron() {
	logger.Info("开始执行密码重置令牌清理定时任务...")
	resetService := service.NewPasswordResetService(db.GetDB())
	if err := fanoutTask(context.Background(), resetService.CleanupExpiredTokens); err != nil {
		logger.Error(err, "密码重置令牌清理定时任务执行失败")
		return
	}
	logger.Info("密码重置令牌清理定时任务执行完成")
}
