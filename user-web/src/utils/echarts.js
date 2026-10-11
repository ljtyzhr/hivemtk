import * as echarts from 'echarts';

// 图表容器会变宽：侧边栏折叠、窗口缩放、抽屉展开都不会让 echarts 自己重算画布尺寸，
// 图就僵在 init 那一刻的宽度上（卡片右侧留一条空白，或溢出卡片被裁掉）。
// 这里给所有实例挂一个共用的 ResizeObserver：
//   · 观察者只建一次 —— 规范里观察目标持弱引用，元素离开文档后实例才跟着可回收；
//     每个图各挂一个观察者的话，回调闭包会把已卸载的实例一直拽在内存里。
//   · 尺寸变化按帧合并，折叠动画的连续回调不会变成几十次重绘。
//   · 宽高为 0 的容器（未激活页签、收起的面板）跳过：把图画成 0×0 之后就不会自己回来了。
const instances = new WeakMap()
const pending = new Set()
let observer = null
let frameId = null

function flush() {
  frameId = null
  const els = Array.from(pending)
  pending.clear()
  for (const el of els) {
    const inst = instances.get(el)
    if (!inst || inst.isDisposed()) continue
    if (!el.clientWidth || !el.clientHeight) continue
    inst.resize()
  }
}

function schedule(el) {
  pending.add(el)
  if (frameId === null) frameId = requestAnimationFrame(flush)
}

function ensureObserver() {
  if (observer || typeof ResizeObserver === 'undefined') return
  observer = new ResizeObserver((entries) => {
    for (const entry of entries) schedule(entry.target)
  })
}

export function safeInit(el, theme, opts) {
  if (!el) return null
  const existing = echarts.getInstanceByDom(el)
  if (existing) return existing
  const inst = echarts.init(el, theme, opts)
  instances.set(el, inst)
  ensureObserver()
  observer?.observe(el)
  return inst
}

export function safeDispose(inst) {
  if (!inst || typeof inst.dispose !== 'function') return
  try {
    if (!inst.isDisposed()) {
      const el = typeof inst.getDom === 'function' ? inst.getDom() : null
      if (el) {
        instances.delete(el)
        observer?.unobserve(el)
        pending.delete(el)
      }
      inst.dispose()
    }
  } catch { /* 忽略：清理/存储/恢复类 best-effort 操作 */ }
}

export { echarts }
export default { safeInit, safeDispose, echarts }
