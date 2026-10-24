# Tasks: 可插拔模块边界（模块契约 + 注册表）

**Feature**: `012-module-contract-registry` ｜ 勾选即以「已验证」为准（无验证不算完成）

## Phase 1 契约（`internal/modulekit/contract.go`，新建）

- [x] T001 `Package modulekit` 包文档：背景取证、两种实现挂同一张表、装配/巡检示例（doc.go）
- [x] T002 契约接口 `Module`：ID / Descriptor / Enable / Disable / Health，含幂等与并发约定注释
- [x] T003 `Descriptor` 能力声明（Name/Version/DependsOn/Capabilities）与 `ModuleState` 枚举 + `String()`
- [x] T004 `PluginSymbol = "Module"` 常量与注释（宿主/插件两侧约定）

## Phase 2 注册表（`internal/modulekit/registry.go`，新建）

- [x] T005 七个 sentinel 错误（Invalid/AlreadyRegistered/NotRegistered/DependencyMissing/HasDependents/NotEnabled/PluginSymbolType）
- [x] T006 `Registry` 结构 + `NewRegistry`（RWMutex + 注册序切片 + ID 索引；entry 快照 Descriptor）
- [x] T007 `Register`：nil/空 ID 报 `ErrModuleInvalid`，重复报 `ErrModuleAlreadyRegistered`，失败零副作用
- [x] T008 `RegisterFromLookup`：断言 `Module`，类型不符报 `ErrPluginSymbolType`
- [x] T009 `Get`/`Has`/`List`/`Descriptor`/`State`：读锁、未注册 fail-closed、返回快照拷贝
- [x] T010 `Enable`：幂等 + 依赖校验（`ErrModuleDependencyMissing`）+ 模块方法锁外调用 + 失败保持 disabled
- [x] T011 `Disable`：幂等 + 反依赖校验（`ErrModuleHasDependents`）+ 失败保持 enabled
- [x] T012 `CheckHealth`：未注册/未启用拦截（`ErrModuleNotEnabled`），已启用透传模块 Health
- [x] T013 `EnableAll`（注册序、首错即停）/ `DisableAll`（注册逆序、errors.Join 聚合）

## Phase 3 内建示例（`internal/modulekit/examples.go`，新建）

- [x] T014 `NoopHealthModule`（ID `builtin.health`，能力 `health.ping`）
- [x] T015 `AuthPlaceholderModule`（ID `builtin.auth`，依赖 `builtin.health`，能力 `auth.check`/`auth.license`，注释说明对应 auth_plugin.so 的位置）
- [x] T016 `BuiltinModules()` 按依赖序返回两个示例模块

## Phase 4 测试（`internal/modulekit/registry_test.go`，新建，全部无 DB）

- [x] T017 `TestRegisterDuplicateFailsClosed`：重复注册报错、表内唯一、状态不变
- [x] T018 `TestRegisterInvalid`：nil / 空 ID（表驱动）
- [x] T019 `TestGetUnknownFailsClosed`：查未注册报错且模块为 nil
- [x] T020 `TestLifecycleOrder`：启用按注册序、健康检查在启用后、停用按逆序、未启用健康检查被拦
- [x] T021 `TestEnableDependencyMissing`：依赖未注册/未启用/自身（表驱动）
- [x] T022 `TestDisableWithEnabledDependents`：被依赖者不可先停、依赖者先停后成功
- [x] T023 `TestEnableDisableIdempotent`：重复启停幂等且不重复调用模块
- [x] T024 `TestEnableAllStopsOnFirstError`：首错即停、透传原错误、后续模块不被启用
- [x] T025 `TestRegisterFromLookup`：Module 成功登记；string/int/nil 报 `ErrPluginSymbolType`（表驱动）
- [x] T026 `TestBuiltinModulesBootstrap`：两内建注册/启用/健康/反依赖/逆序停用闭环
- [x] T027 `TestConcurrentReadSafe`：并发 Register + 并发 Get/List/State/Has（`-race` 验证）

## Phase 5 验证与交付

- [x] T028 `gofmt -l internal/modulekit/` 无输出；`go vet ./internal/modulekit/` 绿
- [x] T029 `go test ./internal/modulekit/... -count=1` 与 `-race` 全绿
- [x] T030 `go build ./...` 全绿（全仓不受影响）
- [x] T031 回写 `specs/012-module-contract-registry/{spec,plan,tasks,delivery}.md`
