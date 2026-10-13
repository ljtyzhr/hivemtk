# cache-type-assert-panic 修复记录

## 修复内容

### 1. cache/redis.go:113

**修复前**：
```go
vals, _ := res.([]any)
```

**修复后**：
```go
vals, ok := res.([]any)
if !ok {
    return nil, fmt.Errorf("cache: unexpected type %T for key %s", res, key)
}
```

**原因**：类型断言失败时返回错误而非 panic。

### 2. cache/memory.go:133

**修复前**：
```go
item := ele.Value.(*cacheItem)
```

**修复后**：
```go
item, ok := ele.Value.(*cacheItem)
if !ok {
    ele = prev
    continue
}
```

**原因**：类型断言失败时跳过该元素而非 panic。

## 提交信息

```
fix(cache): 类型断言添加 ok 检查，避免 panic

- cache/redis.go:113 — res.([]any) 添加 ok 检查
- cache/memory.go:133 — ele.Value.(*cacheItem) 添加 ok 检查
```

## 影响范围

- `internal/cache/redis.go` — PopAll 方法
- `internal/cache/memory.go` — deleteExpired 方法
