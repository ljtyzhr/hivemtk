import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import ElementPlus from 'element-plus'

// 同期群路径分析页：两张图共用一个 el-tabs，未激活的那页是 v-show 藏着的（宽高 0）。
//
// 原来两个页签的图都在 onMounted 里画。echarts 在 0 宽容器上 init 会把画布宽度回退成
// 100px，而 safeInit 拿到的是同一个已存在实例、setOption 不会重算尺寸 ⇒ 切到「行为路径」
// 看到的是一条挤在卡片左侧、标签互相压着读的桑基图（控制台还会留一行 ECharts 警告）。
// 这里守的是"给哪张图画"：只有当前页签的容器该被 init，换页签时才补画另一张。
//
// 两个 .chart 容器在结构上一模一样，任何按内容比较（toEqual / toHaveBeenCalledWith）
// 都分不开它们，所以本文件一律用元素引用本身（===）认人。

const http = vi.hoisted(() => ({ get: vi.fn() }))
const safeInit = vi.hoisted(() => vi.fn())

vi.mock('@/utils/request', () => ({ http }))
vi.mock('@/utils/echarts', () => ({ safeInit }))

import CohortPath from '@/views/analytics/CohortPath.vue'

const COHORT = {
  periods: ['W1', 'W2'],
  cohorts: [{ label: '2026-09-01', size: 12, retention: [100, 42] }]
}
const PATH = {
  nodes: [{ name: 'landing' }, { name: 'view' }, { name: 'call' }],
  links: [{ source: 'landing', target: 'view', value: 9 }, { source: 'view', target: 'call', value: 3 }]
}

function mountPage(body = {}) {
  // 照 safeInit 的真实契约打桩：同一个 DOM 元素第二次进来拿回同一个实例
  const instances = new Map()
  safeInit.mockReset()
  safeInit.mockImplementation((el) => {
    if (!instances.has(el)) {
      instances.set(el, { setOption: vi.fn(), resize: vi.fn(), dispose: vi.fn() })
    }
    return instances.get(el)
  })
  http.get.mockReset()
  http.get.mockImplementation(async (url) => {
    if (url.includes('/cohort')) return body.cohort ?? COHORT
    if (url.includes('/path')) return body.path ?? PATH
    return {}
  })
  const wrapper = mount(CohortPath, { attachTo: document.body, global: { plugins: [ElementPlus] } })
  return { wrapper, instances }
}

// 模板里两个 .chart 的顺序即页签顺序：[0]=Cohort 留存热力图，[1]=行为路径桑基图
function chartEls(wrapper) {
  return wrapper.findAll('.chart').map(n => n.element)
}

function initedEls() {
  return safeInit.mock.calls.map(([el]) => el)
}

function inited(el) {
  return initedEls().some((c) => c === el)
}

async function openPathTab(wrapper) {
  await wrapper.findAll('.el-tabs__item')[1].trigger('click')
  await flushPromises()
}

describe('CohortPath 的图表按当前页签绘制', () => {
  beforeEach(() => {
    document.body.textContent = ''
  })

  it('进入页面时只画当前页签那张图，不去 0 宽的隐藏容器里 init', async () => {
    const { wrapper } = mountPage()
    await flushPromises()
    const [cohortEl, pathEl] = chartEls(wrapper)
    expect(inited(cohortEl)).toBe(true)
    expect(inited(pathEl)).toBe(false)
    expect(safeInit).toHaveBeenCalledTimes(1)
  })

  it('当前页签的图画在真实容器上，并校正过一次尺寸', async () => {
    const { wrapper, instances } = mountPage()
    await flushPromises()
    const [cohortEl] = chartEls(wrapper)
    expect(initedEls()[0]).toBe(cohortEl)
    const chart = instances.get(cohortEl)
    expect(chart.setOption.mock.calls[0][0].series[0].type).toBe('heatmap')
    expect(chart.resize).toHaveBeenCalledTimes(1)
  })

  it('切到「行为路径」才画第二张，画的是桑基图', async () => {
    const { wrapper, instances } = mountPage()
    await flushPromises()
    const [, pathEl] = chartEls(wrapper)
    expect(inited(pathEl)).toBe(false)

    await openPathTab(wrapper)

    expect(inited(pathEl)).toBe(true)
    const chart = instances.get(pathEl)
    expect(chart.setOption.mock.calls[0][0].series[0].type).toBe('sankey')
    expect(chart.resize).toHaveBeenCalledTimes(1)
  })

  it('来回切换：每张容器只有一个实例，不会越切越多', async () => {
    const { wrapper, instances } = mountPage()
    await flushPromises()
    const items = wrapper.findAll('.el-tabs__item')
    await items[1].trigger('click')
    await flushPromises()
    await items[0].trigger('click')
    await flushPromises()
    await items[1].trigger('click')
    await flushPromises()
    expect(instances.size).toBe(2)
    expect(new Set(initedEls()).size).toBe(2)
  })

  it('接口回空数据时显示空状态，不给它画一张空图', async () => {
    const { wrapper } = mountPage({
      cohort: { periods: [], cohorts: [{ label: 'x', size: 0, retention: [] }] },
      path: { nodes: [], links: [] }
    })
    await flushPromises()
    expect(safeInit).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('暂无留存数据')

    await openPathTab(wrapper)

    expect(wrapper.text()).toContain('暂无行为路径数据')
    expect(safeInit).not.toHaveBeenCalled()
  })
})
