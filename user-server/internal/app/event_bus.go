package app

import (
	"hivemtk-user/internal/event"
	"hivemtk-user/internal/repository"
	"hivemtk-user/internal/service"
)

// initEventBus 初始化全局事件总线并注册订阅者
//
// 调用时机：router.Setup() 阶段，在 Service 构造之前
// 设计要点：
//   - 创建 EventBus 实例（2 worker, 1024 队列容量）
//   - 注册 OperationLogSubscriber 订阅 operation.log 主题
//   - 设置为全局单例（event.SetGlobalBus）
//   - main.go 退出时通过 defer event.StopGlobal() 优雅关闭
func InitEventBus() {
	bus := event.New(2, 1024)

	logSubscriber := NewOperationLogSubscriber(repository.NewOperationLogRepository())
	// 模块闸包装（FR-5.6）：moduleKey="" = 纯结构透传（operation.log 不归属
	// 任何模块，恒放行）—— 挂点先行，未来有可归属 topic 只需把 "" 换成
	// 模块 key，订阅代码不用动。判定语义见 service.WrapSubscriber。
	bus.Subscribe(event.TopicOperationLog, service.WrapSubscriber("", logSubscriber.Handle))

	event.SetGlobalBus(bus)
}
