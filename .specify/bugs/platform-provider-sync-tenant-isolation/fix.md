# Fix Report: 平台 provider 同步落库被多租户隔离 fail-closed 拦截

- **Slug**: platform-provider-sync-tenant-isolation
- **Fixed**: 2026-10-22
- **Assessment**: [assessment.md](assessment.md)
- **Status**: applied

## Summary

修复 `TENANT_MODE=on` 下「平台 provider 同步」永远落库失败的缺陷。
生产后果：平台下发的 provider 同步不到商户端 ⇒ `source=platform` 的 provider
不存在 ⇒ 平台垫资的 LLM 通道对所有租户不可用（新商户开户赠送的 token 无法消耗）。

根因是**两处独立**的拦路虎，缺一个就照样失败：

1. **无显式 tenant_id（生产日志里那一条）**：落库行不写 `TenantID`，
   隔离 callback 的「行自证」通道无从自证 ⇒ fail-closed，报
   `[tenant] 表 llm_providers 需要租户上下文但 ctx 未绑定（create）`。
2. **ON CONFLICT 目标与实际索引形状不符**：框架层 `tenancyidx.Normalize`
   把租户表唯一索引原位复合化为 `(tenant_id, name)`，而原代码写
   `ON CONFLICT (name)` ⇒ 42P10
   `there is no unique or exclusion constraint matching the ON CONFLICT specification`。

第 2 条在生产上被第 1 条**掩盖**了（先在 callback 阶段就报错，压根到不了 SQL），
是本次排查中顺带挖出来的第二层缺陷 —— 只修第 1 条，同步仍然失败。

## Changes

| 文件 | 改动 | 依据 |
|------|------|------|
| `user-server/internal/service/llm_platform_usage.go` | 抽出 `savePlatformProviders(gormDB, rows)`；落库行显式写 `TenantID: tenant.DefaultTenantID`（行自证）；写库 ctx 走 `tenant.WithSystemScope`（装配层豁免）；`ON CONFLICT` 冲突目标改 `(tenant_id, name)` | assessment「Proposed Remediation」 |
| `user-server/internal/service/llm_platform_provider_sync_test.go` | 新增回归 4 例（装配期落库 / 行自证不变量 / 冲突目标形状 / 不动商户 api_key） | assessment「Tests to add or update」 |

改动面 ⊆ assessment 的文件清单（`llm_platform_usage.go` + 同名测试文件）。

## Diff Highlights

`user-server/internal/service/llm_platform_usage.go`：

```go
// 装配层写库的唯一豁免通道（与 dispatcher_persistence.go:systemScopeCtx 同一口径）
sysCtx := tenant.WithSystemScope(context.Background())
...
row := model.LLMProvider{
    TenantID:    tenant.DefaultTenantID,   // ← 行自证：让隔离 callback 认得这行的归属
    Name:        r.ProviderName,
    ...
}
if err := gormDB.WithContext(sysCtx).Clauses(clause.OnConflict{
    // 冲突目标必须与库里实际索引形状一致：(tenant_id, name)。
    Columns: []clause.Column{{Name: "tenant_id"}, {Name: "name"}},
    DoUpdates: clause.AssignmentColumns([]string{...}),
}).Create(&row).Error; err != nil {
```

## Guard Reverse Verification

守卫 = 三处：显式 `tenant_id` / `system scope` / 复合冲突目标。

逐项摘除（每次只动一项，测试文件签名同步调整）后的读数：

| 摘除的守卫 | 读数 |
|---|---|
| 全部三条（= 完整还原生产现场） | **红**，日志原文：`平台 provider 落库失败 platform-sensenova: [tenant] 表 llm_providers 需要租户上下文但 ctx 未绑定（create）`；`平台下发 provider 同步完成：0 条`；断言 `应同步 1 条平台 provider，实际 0` |
| 仅摘 system scope（保留显式 tenant_id） | 绿 —— 显式 tenant_id 走「行自证」通道即可放行 |
| 仅摘显式 tenant_id（保留 system scope） | 绿 —— system scope 是装配层豁免通道，create 回调直接 return |
| 仅摘复合冲突目标 | **红**，日志原文：`ERROR: there is no unique or exclusion constraint matching the ON CONFLICT specification (SQLSTATE 42P10)` |

两条独立守卫生效路径（显式 tenant_id 的行自证 / system scope 的装配豁免），
任一存在即可让 create 放行；**两条同时缺失**才复现生产红因。
测试断言的是「落库行必须带显式 tenant_id」与「冲突目标必须匹配 (tenant_id, name)」
这两个不变量，不是某条具体实现。

md5 双向对账（共享工作区口径）：

| 对象 | md5 |
|---|---|
| 修复后工作区（本次最终态） | `85a9f8572cf56203ad4ecbc31e68702e` |
| HEAD blob（未改前） | `2e08039dc1f3048bf35cef758593f9dd` |
| 注码前基线（注码时记录的修复态） | `a50352a8d4dd40ae3435a67a4c6523ba` |

`a50352a8` → `85a9f857` 的差异**只有注释**（把「两个条件」更正为「三个条件」
并写清行自证机制）：`diff <(grep -v "^//" A) <(grep -v "^//" B)` 输出为空，
`diff A B` 的注释差异经核对不含任何代码行。

## Deviations

- assessment 只列了「system scope + 复合冲突目标」两处，实际改动多了
  **显式 `TenantID`**。原因：行自证（`tenancyidx.fillOneForTenant` 的 `!bound`
  分支）是隔离框架提供的**第三条**放行通道，只写 system scope 会让「行归属
  默认租户」这件事依赖豁免通道的副作用，而不是显式声明 —— 显式写出来才
  与 `LoadProvidersFromDB` 的 `Where("tenant_id = ?", DefaultTenantID)` 读取
  口径对齐，也才让「写到别的租户 ⇒ 写进去但读不出来」这个失败模式在代码里
  显形。
- assessment 未提第 2 条缺陷（`ON CONFLICT (name)` 与复合索引不匹配）。
  它是排查第 1 条时用生产库实测挖出来的：直接对生产库跑一次
  `INSERT ... ON CONFLICT (name) DO UPDATE` 得到
  `ERROR: there is no unique or exclusion constraint matching the ON CONFLICT
  specification`。**未在生产库执行任何写操作**（该 INSERT 会因索引不匹配直接
  报错、不落任何行；事后已核对 `llm_providers` 行数未变）。assessment 属契约
  不改，此处记为偏差。

## Production Deployment

- 平台端 `llm_provider_bindings` 已新增 `platform-sensenova`
  （`base_url=https://token.sensenova.cn/v1`、`model=deepseek-v4-flash`、
  `plan_code=starter`、`deliver=true`、`enabled=true`）。
- 平台端代理链路已实测通：`POST /llm-api/v1/chat/completions` 转发 SenseNova
  成功、按 starter 费率扣减赠送额度（2283 输入 token → 2 分，64 输出 token → 0 分，
  整数分向下取整）。
- 商户侧同步需本修复上线后才生效（`SyncPlatformProviders` 在 user-server 启动期
  调用），待发版后复核 `llm_providers` 出现 `platform-sensenova` 行。
