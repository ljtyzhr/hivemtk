import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

// 图表画布跟着容器变宽（src/utils/echarts.js 的 safeInit）。
//
// 症状：把侧边栏折叠起来（或缩放窗口、展开抽屉）之后，图还停在 init 那一刻的宽度上，
// 卡片右边空出一条、或者图溢出被裁。全站 29 个视图用 safeInit，只有 16 个自己挂了
// window resize 监听，侧边栏折叠那条路谁都没接（它不改窗口宽，只改容器宽）。
//
// 这里守的是共用观察者那层的四件事：观察建了、按帧合并、隐藏容器不动手、
// dispose 之后不再回调。jsdom 没有 ResizeObserver，所以自己塞一个假的进去；
// 每格重新 import 一次模块，免得上一格留下的观察者把这格的计数搅浑。

const reg = vi.hoisted(() => ({ byDom: new Map() }))

vi.mock('echarts', () => ({
  getInstanceByDom: (el) => reg.byDom.get(el),
  init: (el) => {
    const inst = {
      resize: vi.fn(),
      isDisposed: vi.fn(() => false),
      dispose: vi.fn(),
      getDom: () => el
    }
    reg.byDom.set(el, inst)
    return inst
  }
}))

class FakeResizeObserver {
  static created = 0
  constructor(cb) {
    this.cb = cb
    this.observed = new Set()
    this.observeCalls = []
    FakeResizeObserver.created += 1
  }
  observe(el) {
    this.observed.add(el)
    this.observeCalls.push(el)
  }
  unobserve(el) { this.observed.delete(el) }
  disconnect() { this.observed.clear() }
  fire(el) { this.cb([{ target: el }], this) }
}

function box(el, w, h) {
  Object.defineProperty(el, 'clientWidth', { value: w, configurable: true })
  Object.defineProperty(el, 'clientHeight', { value: h, configurable: true })
}

async function loadModule() {
  vi.resetModules()
  FakeResizeObserver.created = 0
  reg.byDom.clear()
  return import('@/utils/echarts')
}

function onlyObserver() {
  expect(FakeResizeObserver.created, 'safeInit 应当只建一个共用观察者').toBe(1)
  // 观察者实例藏在模块闭包里，从被观察元素反查它
  return observedBy
}

let observedBy

beforeEach(() => {
  observedBy = null
  global.ResizeObserver = class extends FakeResizeObserver {
    constructor(cb) { super(cb); observedBy = this }
  }
  vi.useFakeTimers()
})

afterEach(() => {
  vi.useRealTimers()
  delete global.ResizeObserver
})

describe('safeInit 让图表跟着容器重算尺寸', () => {
  it('init 之后容器变宽，画布按新宽度重画一次', async () => {
    const { safeInit } = await loadModule()
    const el = document.createElement('div')
    box(el, 800, 400)
    const inst = safeInit(el)
    expect(onlyObserver().observed.has(el)).toBe(true)

    box(el, 1200, 400)
    observedBy.fire(el)
    vi.advanceTimersByTime(20)

    expect(inst.resize).toHaveBeenCalledTimes(1)
  })

  it('同一帧里连续变宽只重画一次', async () => {
    const { safeInit } = await loadModule()
    const el = document.createElement('div')
    box(el, 800, 400)
    const inst = safeInit(el)

    for (const w of [900, 1000, 1100, 1200]) {
      box(el, w, 400)
      observedBy.fire(el)
    }
    vi.advanceTimersByTime(20)

    expect(inst.resize).toHaveBeenCalledTimes(1)
  })

  it('容器宽高为 0（未激活页签、收起的面板）时不动手，免得把图画成 0×0', async () => {
    const { safeInit } = await loadModule()
    const el = document.createElement('div')
    box(el, 800, 400)
    const inst = safeInit(el)

    box(el, 0, 0)
    observedBy.fire(el)
    vi.advanceTimersByTime(20)
    expect(inst.resize).not.toHaveBeenCalled()

    // 重新露出来：这一次要补上
    box(el, 1000, 400)
    observedBy.fire(el)
    vi.advanceTimersByTime(20)
    expect(inst.resize).toHaveBeenCalledTimes(1)
  })

  it('两张图共用一个观察者，重复 init 同一容器不再多挂', async () => {
    const { safeInit } = await loadModule()
    const a = document.createElement('div')
    const b = document.createElement('div')
    box(a, 800, 400); box(b, 800, 400)

    const ia = safeInit(a)
    const ib = safeInit(b)
    expect(FakeResizeObserver.created).toBe(1)
    // 空 div 之间内容比较分不开，这里按引用认
    expect(observedBy.observeCalls.length).toBe(2)
    expect(observedBy.observeCalls[0]).toBe(a)
    expect(observedBy.observeCalls[1]).toBe(b)

    expect(safeInit(a)).toBe(ia)
    expect(safeInit(b)).toBe(ib)
    expect(observedBy.observeCalls).toHaveLength(2)
  })

  it('dispose 之后不再观察，容器再变宽也不会碰已销毁的实例', async () => {
    const { safeInit, safeDispose } = await loadModule()
    const el = document.createElement('div')
    box(el, 800, 400)
    const inst = safeInit(el)

    safeDispose(inst)
    expect(observedBy.observed.has(el)).toBe(false)

    box(el, 1200, 400)
    observedBy.fire(el)
    vi.advanceTimersByTime(20)
    expect(inst.dispose).toHaveBeenCalledTimes(1)
    expect(inst.resize).not.toHaveBeenCalled()
  })

  it('环境没有 ResizeObserver 时照常出图，只是不自适应', async () => {
    global.ResizeObserver = undefined
    const { safeInit } = await loadModule()
    const el = document.createElement('div')
    box(el, 800, 400)
    const inst = safeInit(el)
    expect(inst).toBeTruthy()
    expect(inst.resize).not.toHaveBeenCalled()
  })
})
