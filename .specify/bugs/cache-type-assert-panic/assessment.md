# cache-type-assert-panic

## 问题描述

`cache/redis.go:113` 和 `cache/memory.go:133` 存在类型断言 panic 风险。

### 风险点 1: cache/redis.go:113

```go
vals, _ := res.([]any)
```

**风险**：如果 Redis 返回的 `res` 不是 `[]any` 类型（例如返回 error、nil 或其他类型），类型断言会 panic。

**触发条件**：
- Redis 返回非预期类型（虽然理论上 Eval 返回的应该是 []any，但网络异常或 Redis 配置错误可能导致返回其他类型）
- Redis 返回 error 时，`res` 可能是 nil 或其他类型

**影响**：生产环境 panic，导致服务崩溃。

### 风险点 2: cache/memory.go:133

```go
item := ele.Value.(*cacheItem)
```

**风险**：如果 `ele.Value` 不是 `*cacheItem` 类型，类型断言会 panic。

**触发条件**：
- 理论上 `ele.Value` 应该总是 `*cacheItem`，因为只有 `cacheItem` 才会被放入 list
- 但如果代码有其他路径修改了 list 中的值，或者有并发修改，可能导致类型不匹配

**影响**：生产环境 panic，导致服务崩溃。

## 修复方案

### cache/redis.go:113

```go
// 修复前
vals, _ := res.([]any)

// 修复后
vals, ok := res.([]any)
if !ok {
    return nil, fmt.Errorf("cache: unexpected type %T for key %s", res, key)
}
```

### cache/memory.go:133

```go
// 修复前
item := ele.Value.(*cacheItem)

// 修复后
item, ok := ele.Value.(*cacheItem)
if !ok {
    // 跳过非 cacheItem 元素，避免 panic
    ele = prev
    continue
}
```

## 验证方案

1. 单元测试：验证类型断言失败时的行为
2. 集成测试：验证缓存操作的正常路径
3. 编译检查：确保无编译错误
