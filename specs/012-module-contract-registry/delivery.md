# Delivery Report: 可插拔模块边界（模块契约 + 注册表）

**Feature**: `012-module-contract-registry` ｜ **交付日期**: 2026-10-25 ｜ **Spec/Plan/Tasks**: 同目录

## 一句话

调研文档 P2 项 **A3（可插拔模块边界）** 的第一刀落地：新建纯函数包 `internal/modulekit/`，
把「模块契约（Module 接口）+ 进程内注册表（fail-closed）+ 两个内建示例模块」立起来；
内建模块与 Go plugin 两条实现路径收敛到同一张 `Registry`。**不迁移 auth_plugin.so、不接路由/CI。**

## 交付物清单

| 类型 | 文件 | 规模 | 说明 |
|---|---|---|---|
| 新增 | `user-server/internal/modulekit/doc.go` | 34 行 | 包文档：背景取证、两种实现挂同一张表、装配/巡检示例 |
| 新增 | `user-server/internal/modulekit/contract.go` | 74 行 | `Module` 接口 / `Descriptor` / `ModuleState` / `PluginSymbol` |
| 新增 | `user-server/internal/modulekit/registry.go` | 295 行 | `Registry` + 7 个 sentinel + fail-closed 生命周期 + 锁策略 |
| 新增 | `user-server/internal/modulekit/examples.go` | 80 行 | `NoopHealthModule` / `AuthPlaceholderModule` / `BuiltinModules()` |
| 新增 | `user-server/internal/modulekit/registry_test.go` | 385 行 | 10 个测试函数，全部纯内存无 DB |
| 新增 | `specs/012-module-contract-registry/{spec,plan,tasks,delivery}.md` | 4 件 | spec-kit 四件套 |

**未触碰任何他人文件**：`CHANGELOG.md`、`docs/开发计划-改进清单-2026-10-23.md`、`scripts/*`、
`user-server/internal/model/module_tables.go`、`specs/006*`~`011*`、`.github/workflows/**` 均未改。

## 验证读数（本机实测，2026-10-25，非推测）

| 命令 | 结果 |
|---|---|
| `go build ./internal/modulekit/` | ✅ EXIT 0（本交付包独立编译通过，零消费者、不影响任何其他包） |
| `go build ./...` | ⚠️ 被**他人 lane 在途未提交文件**挡住：`internal/repository/observability_dashboard.go`（`??` untracked，非本交付文件）第 57 行 `FunnelStageRow redeclared`，与既有 `dashboard_stats.go:30` 重名。与本交付无关，未触碰（铁律：不改他人文件） |
| `go test ./internal/modulekit/... -count=1` | ✅ ok（10 函数全过，含表驱动子用例） |
| `go test ./internal/modulekit/... -count=1 -race` | ✅ ok（并发读安全用例竞态检测通过） |
| `go vet ./internal/modulekit/` | ✅ 通过 |
| `gofmt -l internal/modulekit/` | ✅ 无输出 |

```
=== RUN   TestRegisterDuplicateFailsClosed      --- PASS
=== RUN   TestRegisterInvalid                   --- PASS（nil 模块 / 空 ID）
=== RUN   TestGetUnknownFailsClosed             --- PASS
=== RUN   TestLifecycleOrder                    --- PASS（启用序/健康/停用逆序）
=== RUN   TestEnableDependencyMissing           --- PASS（未注册/未启用/自身）
=== RUN   TestDisableWithEnabledDependents      --- PASS
=== RUN   TestEnableDisableIdempotent           --- PASS
=== RUN   TestEnableAllStopsOnFirstError        --- PASS
=== RUN   TestRegisterFromLookup                --- PASS（Module/string/int/nil）
=== RUN   TestBuiltinModulesBootstrap           --- PASS
=== RUN   TestConcurrentReadSafe                --- PASS
PASS  ok  hivemtk-user/internal/modulekit
```

本轮无 DB 依赖：注册表纯内存，不需要 `POSTGRES_TEST_*` 环境变量；未跑任何真库测试。

## 设计要点（为什么这么做）

1. **契约与实现分离**：`ID()` 是唯一事实源，能力声明放 `Descriptor` 并在 Register 时快照——
   声明是编译期的、模块方法是运行时的，二者生命周期不同，读声明不该调模块方法。
2. **fail-closed 用 sentinel**：重复注册、查未注册、启停未注册、依赖未启用、停用被依赖者、
   未启用健康检查，全部返回 `errors.Is` 可判定的 sentinel，绝不返回 nil 静默放行——
   「装配漏注册」必须当场炸，不能延迟成千里之外的空指针。
3. **模块方法锁外调用**：模块 Enable/Health 可能反向调注册表（自检时 List 依赖），
   持锁调用必死锁；读走 RWMutex 读锁，同一模块的启停用 per-entry 互斥锁串行化。
4. **Go plugin 与内建挂同一张表的唯一前提是类型同一性**：插件必须 import
   `hivemtk-user/internal/modulekit` 并 `var Module modulekit.Module = ...`，宿主
   `plugin.Lookup(PluginSymbol)` → `RegisterFromLookup`。接口侧契约本轮已用类型断言测试钉死。

## 决策点清单（本轮拍板 / 留待后续）

| # | 决策 | 结论 | 理由 |
|---|---|---|---|
| D1 | 包位置 | `internal/modulekit/`（新包，不并入 `internal/plugin/`） | `internal/plugin/` 是 gitignore 产物目录、无源码；`main/` 曾因裸 `.gitignore` 模式吞过源码目录 |
| D2 | 未注册查询 | 返回 `ErrModuleNotRegistered`，不返回 nil | fail-closed 总纲 |
| D3 | 重复注册 | 报 `ErrModuleAlreadyRegistered`，零副作用 | 同 ID 双注册是装配 bug，必须当场炸 |
| D4 | 依赖语义 | Enable 校验依赖已启用；Disable 校验无已启用依赖者；自依赖/成环被拒 | 不拆承重墙、不造半启用集合 |
| D5 | 健康检查口径 | 仅对 `StateEnabled` 透传；未启用返回 `ErrModuleNotEnabled` | 「未启用」不等于「健康」，显式区分 |
| D6 | **auth_plugin.so 不迁移**（留待后续） | 本轮不迁 | 它是外部 `marketing` module 的产物、本仓无源码；迁移=取回源码+改 import+同版本重编+补 loader，是独立切片，不能与「立边界」搅在一起 |
| D7 | 运行时模块 ID 命名空间 | 示例用 `builtin.health` / `builtin.auth` 前缀，不强制对齐 `model.ModuleKey` | 进程内能力名 ≠ 业务模块 key（计费/菜单闸），两个命名空间 |
| D8 | 生命周期方法幂等 | Enable/Disable 重复调用返回 nil，不重复调模块 | 装配期多处接线不会互相踩 |

## 未迁移 / 留待后续

1. **auth_plugin.so 迁移**：需先取回 `marketing/internal/plugin/auth` 源码，改为 import 本包的
   `Module` 契约、导出 `var Module modulekit.Module`，与宿主同 Go 版本 / 同依赖重编（Go plugin
   类型同一性的硬约束），再补一个 `plugin.Open` + `plugin.Lookup(PluginSymbol)` 的 loader。
2. **真 .so 加载验证**：本轮的 `RegisterFromLookup` 只验证接口侧（符号断言）；真实加载需在
   Linux/macOS + CGO 下编一个最小 plugin 做端到端验证。
3. **持久化 / 跨进程**：注册表纯内存；多实例部署下「每实例各有一张表」是当前边界，
   若有全局一致诉求再评估外置目录（etcd/Redis）。
4. **热卸载 / 热更新**：`Disable` 之后重新 `Enable` 已支持（幂等 + 状态翻转），但 Go plugin
   本身不可卸载（`plugin` 包无 Unload），热更新是另一张卡。

## 建议接线点（本轮只建议，不实施）

- **装配**（`cmd/api/main.go` 或 `internal/app/` 某 wiring）：进程启动时
  `r := modulekit.NewRegistry(); for _, m := range modulekit.BuiltinModules() { r.Register(m) }`，
  `r.EnableAll(ctx)` 失败即 `log.Fatal`（fail-closed 落到底：启动起不来，而不是半活）。
- **巡检**：`/healthz` 或 `internal/monitor/` 巡检任务里对 `r.List()` 逐个 `CheckHealth`，
  有 `ModuleState` 可报「模块级」健康，不再只有进程级 up/down。
- **loader（未来）**：新文件 `internal/app/plugin_loader.go`，扫描配置目录里的 `.so`，
  `plugin.Open` → `plugin.Lookup(modulekit.PluginSymbol)` → `r.RegisterFromLookup`。
- **与业务模块闸的关系（建议，非本轮）**：`model.ModuleKey` 与 `modulekit` 模块 ID 是两套
  命名空间，若要做「租户模块开通 ⇒ 运行时能力启停」联动，建议在 service 层加一个映射
  adapter，而不是让 modulekit 依赖 model。

## 已知边界（写清不做什么）

- 不强制对齐 `model.ModuleKey` 词表（D7）。
- `Descriptor` 是 Register 时快照，之后实现若改声明，注册表不感知（契约要求声明不可变）。
- `EnableAll` 半启用集合不会自动回滚已启用的部分（fail-closed 是「停在那并报错」，
  回滚语义留给调用方决定）。
