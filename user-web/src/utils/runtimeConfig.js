// 运行时配置的 nil-safe 读取层。
//
// 为什么要有这一层：`useRuntimeConfigStore()` 在没有 active Pinia 的环境里会直接抛
// `getActivePinia() was called but there was no active Pinia`——单测里 mount 组件但没
// `app.use(pinia)` 就是这种环境（仓内既有的 customerSession_deeplink /
// browser_monitor_resilience 等测试文件正是如此）。读取口抛错会把**调用方的整条路径**
// 干掉：深链扫描不再发那一次宽列表请求，监控页的退避判定直接中断。
//
// 所以这里把「读不到 store」统一收敛成「回落 fallback」：
//   - Pinia 没装 / store 尚未 ensureLoaded → 回落调用点给的兜底常量
//   - 兜底常量就是改造前的硬编码值，所以这两种环境下行为与改造前逐字节一致
//
// 代价是「静默回落」：配置面真出事时调用方看不到。这个代价是故意的——
// 这些阈值全是前端自己的行为旋钮，拉不到配置时站点必须还能跑；而 store 内部已经
// 把失败记进了 failed 标志，需要时可查。
//
// 单位约定：后端 duration 参数存的是**秒**（service.parseDurationSeconds 的双口径），
// 这里的 rtDur 负责乘 1000 转成前端 setTimeout/setInterval 用的毫秒。

import { useRuntimeConfigStore } from '@/stores/runtimeConfig'

let warnOnce = false

function store() {
  try {
    return useRuntimeConfigStore()
  } catch {
    // 没有 active Pinia（单测 / 极早期的启动窗口）。只提醒一次，
    // 避免每个调用点每次读取都打一行，把控制台刷满。
    if (!warnOnce) {
      warnOnce = true
      if (typeof console !== 'undefined') {
        console.warn('[runtimeConfig] 无 active Pinia，全部阈值回落代码兜底值')
      }
    }
    return null
  }
}

/** 读字符串原样；缺失/空回落 fallback。 */
export function rtStr(group, key, fallback) {
  const s = store()
  return s ? s.str(group, key, fallback) : fallback
}

/** 读整数；非数字/非正数（<1）一律回落 fallback。 */
export function rtNum(group, key, fallback) {
  const s = store()
  return s ? s.num(group, key, fallback) : fallback
}

/** 读后端秒数并转成毫秒；读不到回落 fallback（毫秒）。 */
export function rtDur(group, key, fallbackMs) {
  const s = store()
  return s ? s.durSeconds(group, key, fallbackMs) : fallbackMs
}

/** 读布尔；没配过才回落 fallback，配了 false 就是 false。 */
export function rtBool(group, key, fallback) {
  const s = store()
  return s ? s.bool(group, key, fallback) : fallback
}

/** 供启动链调用：预热运行时配置。同样 nil-safe。 */
export async function rtEnsureLoaded(force = false) {
  const s = store()
  return s ? s.ensureLoaded(force) : {}
}
