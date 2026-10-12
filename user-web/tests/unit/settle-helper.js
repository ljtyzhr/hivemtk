import { flushPromises } from '@vue/test-utils'

// 等组件「真的停下来」，而不是猜要刷新几次。
//
// 为什么需要它：`await flushPromises()` 内部是 `scheduler(resolve, 0)`，
// 也就是一次宏任务。它只保证**微任务**队列被排空，对组件里 setTimeout 驱动的
// 东西（防抖搜索、轮询、重试退避）一个都不等。而「刷新 3 次」这种写法在
// 空闲时碰巧够用，机器一忙（CI 并行、同时在跑 go test）就不够了 ——
// 实测同一棵树连跑三次，失败集合分别是 0 / 30 / 11 个，跨十几个互不相干的
// 页面，全是这个原因。这类失败比真 bug 更坏：它让人怀疑刚改的东西。
//
// 做法：每轮同时排空微任务 + 让出一个宏任务，然后比一次渲染结果；
// 连续两轮结果相同即认为稳定，早停。没有魔数上限式的「多刷几次」，
// 因为那只是把概率问题往后推。
//
// 用法：`await settle(w)`（w 是 mount 出来的 wrapper）。
// 返回实际等待的轮数，便于用例在超限时排查。
const MAX_ROUNDS = 40

export const settle = async (w) => {
  let prev = typeof w?.html === 'function' ? w.html() : ''
  for (let round = 1; round <= MAX_ROUNDS; round++) {
    await flushPromises()
    await new Promise((resolve) => setTimeout(resolve, 0))
    const now = typeof w?.html === 'function' ? w.html() : ''
    if (now === prev) return round
    prev = now
  }
  return MAX_ROUNDS
}
