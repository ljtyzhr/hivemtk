# Implementation Plan: 可插拔模块边界（模块契约 + 注册表）

**Feature**: `012-module-contract-registry` ｜ **Created**: 2026-10-25 ｜ **Spec**: `./spec.md`

## 技术路线（一句话）

新建纯函数包 `user-server/internal/modulekit/`：`Module` 接口 + `Descriptor` 能力声明 +
`Registry` 内存注册表（fail-closed）+ 两个内建示例模块。内建模块与未来 Go plugin 都收敛到
同一个 `Module` 接口、同一张 `Registry`。不迁移 auth_plugin.so，不接路由/CI。

## 分层落点（五层架构核对）

| 层 | 是否改动 | 说明 |
|----|---------|------|
| Model | ❌ 不改 | 注册表纯内存，无表需求；`module_tables.go` 不动 |
| Repository | ❌ 不改 | 无持久化 |
| Service | ❌ 不改 | 本轮不接业务 |
| Controller / Router | ❌ 不改 | 接线只写建议 |
| Pkg（新增） | ✅ `internal/modulekit/` | 契约 + 注册表 + 示例模块，零业务层依赖 |

**为什么放 `internal/modulekit/` 而不是并入 `internal/plugin/`**：
`internal/plugin/` 现状是 gitignore 的产物目录（见 spec 证据 1），连包名、模块路径都不存在；
把契约放进去等于把「契约」与「外部二进制」混居，且 `internal/plugin/main/` 已被
user-server/.gitignore 的裸 `main` 注释点名（曾被吞掉源码目录）。独立包名 `modulekit`
（module toolkit）语义清晰、可被 model 之外任意层 import 而不成环。

## 关键设计决策（含被否决的方案）

### D1 契约形状：接口 + Descriptor 分离（否决：单一接口塞满字段）

`ID()` 是唯一事实源，能力声明放 `Descriptor` 结构体。否决方案是把 Name/Version/依赖直接做成
接口方法——那会让每个实现复制粘贴一堆 Getter，且注册表读声明时频繁调用模块方法。
`Descriptor` 在 Register 时**快照**进 entry，之后读声明不再碰模块对象（模块方法是运行时的，
声明是编译期的，二者生命周期不同）。

### D2 fail-closed 用 sentinel 错误（否决：返回 nil / 零值）

七个 sentinel：`ErrModuleInvalid` / `ErrModuleAlreadyRegistered` / `ErrModuleNotRegistered` /
`ErrModuleDependencyMissing` / `ErrModuleHasDependents` / `ErrModuleNotEnabled` /
`ErrPluginSymbolType`。全部 `errors.New` + 调用点 `%w` 包装，`errors.Is` 判定。
否决「Get 未注册返回 nil」：那会把「装配漏注册」延迟成千里之外的空指针。

### D3 生命周期语义（幂等 + 依赖 + 反依赖）

- 注册后初始 `StateDisabled`；`Enable` 已启用幂等返回 nil（不重复调模块）；`Disable` 已停用同理。
- `Enable` 校验 `Descriptor.DependsOn` 全部 `StateEnabled`，否则 `ErrModuleDependencyMissing`
  （含依赖未注册、依赖自身两个退化情形）。
- `Disable` 校验不存在**已启用**模块声明依赖它，否则 `ErrModuleHasDependents`
  （fail-closed：不拆承重墙）。
- `CheckHealth` 只对 `StateEnabled` 透传给模块 `Health`；未启用 `ErrModuleNotEnabled`。
- `EnableAll` 按注册序、任一失败即停（半启用集合不得被当成「全活了」）；
  `DisableAll` 按注册逆序、尽力而为、`errors.Join` 聚合（shutdown 不做一半停手）。

### D4 锁策略（锁外调用模块方法 + per-entry 串行化）

读（Get/List/State/Has/Descriptor）走 RWMutex 读锁；状态变更走写锁。**模块方法一律锁外调用**
——模块 Enable/Health 可能反向调注册表（比如模块自检时 List 依赖），持锁调用必死锁。
同一模块的 Enable/Disable 用 per-entry `sync.Mutex` 串行化，避免并发启停踩状态。
锁顺序恒为 `entry.mu → Registry.mu`，无反向路径，无死锁。

### D5 Go plugin 与内建模块如何挂进同一张表（论证，本轮只实现接口侧）

Go 的 `plugin` 包要求**类型同一性**：宿主与插件必须 import **同一个包**的**同一个类型**，
`plugin.Lookup` 出来的 `interface{}` 才能断言成宿主的 `Module`。因此：

1. 插件侧必须 `import "hivemtk-user/internal/modulekit"` 并导出
   `var Module modulekit.Module = &authPlugin{...}`；
2. 宿主侧 `plugin.Open(path)` → `plugin.Lookup(modulekit.PluginSymbol)` →
   `registry.RegisterFromLookup(sym)`；
3. 前提是宿主与插件 **go.mod 模块路径、Go 版本、所有依赖版本逐字节一致**，且 plugin
   只能在 Linux/macOS 上以 CGO 模式构建。

内建模块不需要上述任何一步——它编译期内联，直接 `Register(m)`。两者的**公共面就是
`Module` 接口 + `Registry`**，这也是为什么「内建/插件挂同一张表」成立。本轮交付
`RegisterFromLookup` + 类型断言测试钉死接口侧契约；真正的 `.so` 加载留待后续
（还需取回 `marketing` module 源码并同版本重编）。

### D6 不迁移 auth_plugin.so（论证留待后续）

spec 证据 2 表明 auth_plugin.so 是外部 `marketing` module 编译的产物，本仓无源码。
迁移 = 取回源码 + 改 import 到 `hivemtk-user/internal/modulekit` + 与宿主同版本重编 +
补 loader。这是一次独立切片，不能在本轮「立边界」里顺手做——顺手做的迁移会把
「契约是否正确」与「老插件是否还工作」两个问题搅在一起。

## 语义细则（实现判据）

- ID 非空即可，**不强制**与 `model.ModuleKey` 同词表：运行时模块 ID 与业务模块 key
  是两个命名空间（一个是进程内能力名，一个是计费/菜单闸 key）。示例模块用
  `builtin.health` / `builtin.auth` 前缀表明「内建」命名空间。
- `List()` / `idsSnapshot()` 返回**拷贝**：调用方可安全持有快照，不受后续 Register 影响。
- `Descriptor()` 返回的切片可能被调用方改写，Register 快照时按引用存；对外的
  `Descriptor(id)` 原样返回快照——实现模块必须把 Descriptor 当作不可变声明。
- `Enable` 失败保持 `StateDisabled`、`Disable` 失败保持 `StateEnabled`（状态只在模块方法
  成功返回后翻转）。

## 实施阶段

| 阶段 | 内容 | 产物 |
|---|---|---|
| P1 | 契约：`Module` / `Descriptor` / `ModuleState` / `PluginSymbol` | `contract.go` |
| P2 | 注册表 + sentinel + fail-closed + 锁策略 | `registry.go` |
| P3 | 内建示例模块 + 装配辅助 | `examples.go` |
| P4 | 纯单元测试（10 函数，表驱动） | `registry_test.go` |
| P5 | 验证 + 四件套文档 | `specs/012-module-contract-registry/` |

## 验证口径（与 CLAUDE.md 3.5/4.4 同表）

| 改动范围 | 命令 |
|---|---|
| Go 任意层 | `cd user-server && go build ./...` |
| Go modulekit 包 | `go test ./internal/modulekit/... -count=1` |
| 竞态 | `go test ./internal/modulekit/... -count=1 -race` |
| 静态 | `go vet ./internal/modulekit/`、`gofmt -l internal/modulekit/` |

（本轮无 DB 依赖，不需要 POSTGRES_TEST_* 环境变量。）

## 风险与回滚

| 风险 | 处置 |
|---|---|
| Go plugin 类型同一性没人遵守 | 契约写进 doc.go + `PluginSymbol` 常量 + `RegisterFromLookup` 类型断言测试；未来 loader 接线时再做真 .so 验证 |
| 依赖声明成环 | Enable 校验依赖需已启用，自依赖/成环都会因「依赖未启用」被拒（fail-closed），不会死循环 |
| 模块方法持锁反向调用注册表 | 模块方法一律锁外调用（D4）；per-entry 锁与 RWMutex 无交叉持有 |
| 并发注册同 ID | Register 在写锁内判重，恰好一个成功，其余 `ErrModuleAlreadyRegistered`（并发测试钉住） |
| 回滚 | 本包零业务引用，删除 `internal/modulekit/` 即完全回滚，无迁移无数据 |
