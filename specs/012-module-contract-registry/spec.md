# Feature Specification: 可插拔模块边界（模块契约 + 注册表）

**Feature Branch**: `012-module-contract-registry`

**Created**: 2026-10-25

**Status**: Approved

**Input**: 计划文档《开发计划-改进清单-2026-10-23.md》P2 项 **A3 可插拔模块边界**。
原文取证（该文档第二节 A3 行）：「`internal/plugin/` 只有 `main/auth_plugin.so`（Go plugin 单点），
无模块契约/注册表」。本 spec 对应「把边界立起来」的第一刀：定义模块契约与进程内注册表，
并给两个内建示例模块演示注册/查询/生命周期。**本轮不重写整个插件体系，不强制迁移 auth_plugin.so。**

## 现状调研结论（Argumentation，回仓取证 2026-10-25）

### 证据 1：`internal/plugin/` 的 git 跟踪树是空的

```
$ git ls-files user-server/internal/plugin/
（空输出——目录下没有任何被 git 跟踪的文件）
```

磁盘上唯一存在的是 `user-server/internal/plugin/main/auth_plugin.so`，但它在忽略名单里：

```
.gitignore:165  *.so
.gitignore:182  /user-server/internal/*.so
.gitignore:183  /user-server/internal/plugin/main/*.so
```

`file` 实测：`Mach-O 64-bit dynamically linked shared library arm64`，12,315,730 字节。
结论：所谓「插件单点」在版本库里**没有源码**，只有一份本地构建的二进制产物。

### 证据 2：该 .so 是外部模块编译的 Go plugin

`go tool nm` 对 auth_plugin.so 的导出符号实测：

```
B plugin/unnamed-a727...AuthPlugin                       ← 宿主可 Lookup 的导出符号
R go:itab.*marketing/internal/plugin/auth.AuthPluginImpl,marketing/internal/plugin/auth.AuthPlugin
T marketing/internal/plugin/auth.(*AuthPluginImpl).CheckAuthorization
T marketing/internal/plugin/auth.(*AuthPluginImpl).RefreshAuthorization
T marketing/internal/plugin/auth.(*AuthPluginImpl).GetFirstAccessTime
T marketing/internal/plugin/auth.(*AuthPluginImpl).Init
T marketing/internal/plugin/auth.(*AuthPluginImpl).fetchLicenseFromPlatform
```

解读：它是**另一个 Go module（`marketing`）**的 `marketing/internal/plugin/auth` 包编译出来的
授权插件，导出符号名 `AuthPlugin`，接口方法含 `CheckAuthorization` / `RefreshAuthorization` /
`GetFirstAccessTime` / `Init`（License 校验向）。本仓没有它的源码。

### 证据 3：全仓没有任何装载代码

```
$ grep -rn "plugin\.Open\|plugin\.Lookup" --include="*.go" user-server/
（空输出）
```

`internal/plugin/main/auth_plugin.so` 是**孤儿产物**：没有一行代码打开它、查它、调它。
「可插拔」目前只是目录名 + 一份没人加载的二进制。

### 证据 4：model 层有「module」概念，但那是业务/数据概念，不是运行时契约

| 文件 | 定义 | 语义 |
|---|---|---|
| `internal/model/tenant_module.go` | `TenantModule`（tenant_modules）/ `ModuleSyncLog` / `ModuleOrder`（module_orders） | 本地**开通状态镜像**与订单镜像：租户买没买、开没开、有没有过期 |
| `internal/model/module_bindings.go` | `ModuleKey` / `ModuleMeta` / `ModuleCatalogMeta`（24 个模块）/ `ModuleMenuBindings` | 模块**目录元数据**与菜单归属：模块闸按它裁菜单 |

这两处回答的是「**业务上**哪些模块对哪些租户放行」（模块闸），与「**运行时**一个能力如何被加载、
启用、健康检查、停用」是两回事。A3 缺的是后者。

### 结论

A3 的边界三要素全缺：① 没有模块契约（接口）；② 没有注册表（进程内查询/生命周期）；
③ 没有 fail-closed 语义（重复注册、查未注册、依赖未启用时的行为没有定义）。
本轮补齐这三要素，并把「内建模块」与「Go plugin」两条实现路径收敛到同一张表。

## 本功能不覆盖（明确边界）

- **不迁移** auth_plugin.so：它是外部 `marketing` module 的产物，源码不在本仓；
  迁移需要先取回源码 + 解决 Go plugin 的类型同一性问题（见 plan D5），留待后续。
- 不接 router/controller/Makefile/CI：接线只写建议，不在本轮实施。
- 不建新表：注册表纯内存，无持久化诉求；`model/module_tables.go` 不动。
- 不做模块热卸载/热更新、不做跨进程注册：进程内注册表是本轮的边界。

## User Scenarios & Testing

### User Story 1 - 注册与查询的 fail-closed（Priority: P0）

作为装配代码的编写者，我要在「重复注册、查询未注册模块」时立刻拿到明确的错误，
而不是拿到 nil / 零值后继续往下跑，最后在线上才炸在某个空指针上。

**Acceptance Scenarios**:

1. Given 空注册表，When 同 ID 注册两次，Then 第二次返回 `ErrModuleAlreadyRegistered`，
   且表内该 ID 仍只有一份、状态不变。
2. Given 注册 nil 模块或空 ID 模块，When Register，Then 返回 `ErrModuleInvalid` 且不留痕。
3. Given 空注册表，When Get 未注册 ID，Then 返回 `ErrModuleNotRegistered` 且模块为 nil。

### User Story 2 - 生命周期顺序（Priority: P0）

作为装配代码的编写者，我要依赖注册表保证：注册 → 启用（按注册序）→ 健康检查 → 停用
（按注册逆序）的顺序语义，以及「依赖未启用不得启用」「被依赖者不得先停」。

**Acceptance Scenarios**:

1. Given 模块 a、b 依次注册，When EnableAll，Then 模块 Enable 按 a→b 调用；
   DisableAll 按 b→a 调用。
2. Given 模块 x 声明依赖未注册/未启用的 y，When Enable(x)，Then 返回
   `ErrModuleDependencyMissing` 且 x 保持 disabled。
3. Given b 依赖 a 且都启用，When Disable(a)，Then 返回 `ErrModuleHasDependents` 且 a 保持 enabled；
   先 Disable(b) 再 Disable(a) 则都成功。
4. Given 已启用的模块，When 重复 Enable/Disable，Then 幂等返回 nil 且不重复调用模块方法。

### User Story 3 - 健康检查与状态（Priority: P0）

作为巡检代码的编写者，我要能对单个模块查健康、查生命周期状态；对未启用的模块做健康检查
必须得到显式的 `ErrModuleNotEnabled`，而不是「看起来健康」。

**Acceptance Scenarios**:

1. Given 已启用模块，When CheckHealth，Then 调用模块 Health 并透传结果。
2. Given 已注册未启用的模块，When CheckHealth，Then 返回 `ErrModuleNotEnabled` 且不调用模块 Health。

### User Story 4 - 并发读安全（Priority: P1）

作为运维面（健康巡检、模块列表页）的开发者，我要能在「请求并发读、装配期并发写」的
场景下安全地 Get/List/State/Has，不 panic、不竞态。

**Acceptance Scenarios**:

1. Given 8 个 goroutine 并发 Register 与 8 个 goroutine 并发 Get/List/State/Has，
   When 在 `-race` 下运行，Then 无竞态报警，数据面自洽（注册过的 ID 必可查到）。

## Requirements

- **FR-1** 模块契约接口 `Module`：`ID() string`（唯一标识）、`Descriptor() Descriptor`
  （能力声明：Name/Version/DependsOn/Capabilities）、`Enable(ctx) error`、
  `Disable(ctx) error`、`Health(ctx) error`；Enable/Disable 必须幂等，三方法可并发调用。
- **FR-2** 进程内注册表 `Registry`：`Register`（快照 Descriptor）、`Get`、`Has`、`List`
  （注册序稳定快照）、`Descriptor`、`State`、`Enable`、`Disable`、`CheckHealth`、
  `EnableAll`（注册序，任一失败即停）、`DisableAll`（注册逆序，尽力而为聚合错误）。
- **FR-3** fail-closed 语义：重复注册报错、查未注册报错、启停未注册报错、依赖未启用报错、
  停用被依赖者报错、未启用健康检查报错。全部用 sentinel 错误（`errors.Is` 判定），
  绝不返回 nil/零值静默放行。
- **FR-4** 并发安全：读走 RWMutex 读锁；状态变更走写锁；**模块方法在锁外调用**
  （防模块反向调用注册表死锁）；同一模块的 Enable/Disable 用 per-entry 互斥锁串行化。
- **FR-5** Go plugin 与内建模块挂同一注册表：约定导出符号名 `PluginSymbol="Module"`，
  提供 `RegisterFromLookup(sym any)` 断言为 `Module` 并登记；符号类型不符返回
  `ErrPluginSymbolType`。
- **FR-6** 两个内建示例模块：`NoopHealthModule`（ID `builtin.health`，no-op 健康占位）与
  `AuthPlaceholderModule`（ID `builtin.auth`，依赖 `builtin.health`，能力声明
  `auth.check`/`auth.license`），以及 `BuiltinModules()` 装配辅助。
- **FR-7** 包位置 `user-server/internal/modulekit/`：不依赖 model/repository/service 层，
  可被任意层 import 而不成环。
- **FR-8** 全部行为有纯单元测试覆盖（无 DB 依赖），`go test -race` 通过。

## Key Entities

- **Module**（接口）：模块契约本体。
- **Descriptor**（值对象）：能力声明快照。
- **ModuleState**（枚举）：unknown / disabled / enabled。
- **Registry**（结构体）：进程内注册表，RWMutex + 注册序切片 + ID 索引。
- **NoopHealthModule / AuthPlaceholderModule**：内建示例。

## Success Criteria

- **SC-1** `go test ./internal/modulekit/... -count=1` 全绿，`-race` 全绿。
- **SC-2** `go build ./...`、`go vet ./internal/modulekit/`、`gofmt -l` 全绿。
- **SC-3** 10 个测试函数覆盖：重复注册、非法注册、查未注册、生命周期顺序、依赖缺失、
  停用被依赖者、幂等、EnableAll 首错即停、RegisterFromLookup 类型断言、并发读安全。
- **SC-4** `specs/012-module-contract-registry/{spec,plan,tasks,delivery}.md` 四件套齐备，
  delivery 写清改动文件、测试读数、未迁移决策点与建议接线点。
