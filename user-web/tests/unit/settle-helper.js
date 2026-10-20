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
// 用法：`await settle(w)`（w 是 mount 出来的 wrapper）；断言打在 document.body
// （teleport 弹层 / ElMessage）上时用 `await settleBody()`。
// 返回实际等待的轮数，便于用例在超限时排查。
// 在模块加载时捕获真实的 setTimeout。用例里装上 vi.useFakeTimers() 之后，
// 全局 setTimeout 会被换成假时钟，settle 的「让出一个宏任务」就永远等不到
// （test-utils 的 flushPromises 不受此害：它的 scheduler 在模块加载时就绑定了 setImmediate）。
// 捕获后 settle 在假时钟下退化为「排微任务 + 让出一个真实宏任务」，
// 对轮询类用例至少等价于一次 flushPromises，且不会挂死。
const realSetTimeout = setTimeout

const MAX_ROUNDS = 40

const settleOn = async (snapshot) => {
  let prev = snapshot()
  for (let round = 1; round <= MAX_ROUNDS; round++) {
    await flushPromises()
    await new Promise((resolve) => realSetTimeout(resolve, 0))
    const now = snapshot()
    if (now === prev) return round
    prev = now
  }
  return MAX_ROUNDS
}

export const settle = async (w) =>
  settleOn(() => (typeof w?.html === 'function' ? w.html() : ''))

// 弹层 teleport 到 body（el-drawer / el-select popper / ElMessage 都是）或用例只
// 断言 document.body 时，wrapper.html() 看不见那部分 DOM —— 用整页 body 当稳定判据。
export const settleBody = async () => settleOn(() => document.body.innerHTML)
