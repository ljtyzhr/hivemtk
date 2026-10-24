// Package portcontract 提供 aiagent 工具层（tooluse）与业务层（service）之间的
// 端口契约（Port interfaces）与数据传输对象（DTO）。
//
// 依赖方向：service ──→ portcontract ←── tooluse；两侧不相互 import，
// 由本包接口契约解耦（切断 import cycle）。所有跨层 DTO 都在本包定义，
// 避免 tooluse 引用 service 的私有请求结构体。
//
// 装配：service 包的 tool_ports_adapter.go 提供所有 Port 接口实现，
// 在 main/cmd/api 启动期通过 setter 注入到 tooluse.Registry 或工具 Deps。
package portcontract
