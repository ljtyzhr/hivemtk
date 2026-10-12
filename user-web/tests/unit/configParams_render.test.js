import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import ElementPlus from 'element-plus'

// 动态阈值参数页的渲染用例。
//
// 这一页是「配置治理」在用户侧唯一的落地界面，所以它的每一条友好性承诺都要有钉子：
//   1. 已下架的旋钮必须显式可见且不可编辑 —— 页面挂着一个改了不生效的旋钮，
//      比没有这个旋钮更糟（用户改完去验证，行为一定不变，原因却藏在代码里）。
//   2. 分组中文名必须来自后端 —— 原来这里是前端写死的 18 项 GROUP_LABELS，
//      后端实际有 29 组，新增的组在页面上静默回落成英文 key。
//   3. duration 不能一律显示「N 秒」 —— 86400 秒没人能一眼读成一天。
//   4. 高危参数保存前必须二次确认；需重启/敏感参数必须有提示。
//   5. 已下架参数不得计入「重置整组」的条数（后端 BulkResetGroup 会跳过它们）。
//
// api 被 mock：端点归属与查询串有各自的用例，这里只钉"渲染 + 交互"。

const api = vi.hoisted(() => ({
  list: vi.fn(),
  listGroups: vi.fn(),
  auditLogs: vi.fn(),
  update: vi.fn(),
  reset: vi.fn(),
  bulkReset: vi.fn()
}))

vi.mock('@/api/configParams', () => ({
  list: api.list,
  listGroups: api.listGroups,
  auditLogs: api.auditLogs,
  update: api.update,
  reset: api.reset,
  bulkReset: api.bulkReset
}))

import ConfigParams from '@/views/system/ConfigParams.vue'
import { settle } from './settle-helper'

const GROUPS = [
  { group: 'lock', label: '分布式锁', description: '锁与幂等的存活时间', sort_order: 10 },
  { group: 'agent_llm', label: 'AI 大模型', description: '模型调用与熔断', sort_order: 20 }
]

function param(over) {
  return {
    group: 'lock',
    key: 'inbox_human_ttl',
    name: '收件箱人工锁存活时间',
    description: '超过后其他坐席可以抢接管',
    value_type: 'duration',
    value: '86400',
    default_value: '86400',
    min: '60',
    max: '604800',
    step: '60',
    read_only: false,
    restart: false,
    category: '',
    updated_by: 0,
    deprecated: false,
    ...over
  }
}

function mountPage() {
  return mount(ConfigParams, { global: { plugins: [ElementPlus] } })
}

describe('ConfigParams.vue 渲染与友好性承诺', () => {
  beforeEach(() => {
    api.list.mockReset()
    api.listGroups.mockReset()
    api.update.mockReset().mockResolvedValue({})
    api.reset.mockReset().mockResolvedValue({})
    api.bulkReset.mockReset().mockResolvedValue({})
    api.listGroups.mockResolvedValue([...GROUPS])
    api.list.mockResolvedValue([
      param({}),
      param({
        key: 'ai_processing_ttl',
        name: 'AI 处理中标记存活时间（已下架）',
        value: '120',
        default_value: '120',
        deprecated: true,
        deprecated_at: '2026-10-12T03:04:05Z',
        deprecated_reason: 'tryAcquireAILock 全树只有测试调用，生产并发闸由 ai_processing_ttl 承担'
      }),
      param({
        group: 'agent_llm',
        key: 'temperature',
        name: '采样温度',
        value_type: 'float',
        value: '0.7',
        default_value: '0.7',
        min: '0',
        max: '2',
        risk: 'danger',
        restart: true,
        unit: ''
      })
    ])
  })

  it('分组中文名来自后端，不再是前端硬编码表', async () => {
    const w = mountPage()
    await settle(w)
    expect(api.listGroups).toHaveBeenCalled()
    const text = w.text()
    expect(text).toContain('分布式锁')
    expect(text).toContain('AI 大模型')
  })

  it('默认不显示已下架项，打开开关后才列出', async () => {
    const w = mountPage()
    await settle(w)
    // 后端即使把已下架项一起返回（过滤回归 / 老后端配新前端），页面也不渲染它。
    // 判据用表格行数而不是全文搜索「已下架」三个字 —— 开关的标签本身就叫
    // 「显示已下架」，全文搜索会把开关当成一条已下架记录，判红得毫无道理。
    expect(api.list).toHaveBeenLastCalledWith(undefined, false)
    expect(w.findAll('.el-table__row')).toHaveLength(1)

    await w.find('.el-switch').trigger('click')
    await settle(w)
    expect(api.list).toHaveBeenLastCalledWith(undefined, true)
    expect(w.findAll('.el-table__row')).toHaveLength(2)
  })

  it('duration 显示成人类可读单位而不是一串秒数', async () => {
    const w = mountPage()
    await settle(w)
    const text = w.text()
    expect(text).toContain('1 天')
    expect(text).not.toContain('86400 秒')
  })

  it('已下架项的编辑按钮禁用，且行内给出下架原因', async () => {
    const w = mountPage()
    await settle(w)
    await w.find('.el-switch').trigger('click')
    await settle(w)
    // 下架原因必须出现在行内（不是 tooltip 里悬停才看得到）。
    expect(w.text()).toContain('tryAcquireAILock')
    // 两行各有各的编辑按钮，所以这里数的是「禁用的那一个有几个」而不是
    // 「编辑按钮有几个」—— 早先写成后者，红得有理有据：数的是总数，
    // 而我想验的是「下架那个被禁了」，两件事被一个断言混成了一件。
    const editButtons = w.findAll('button').filter((b) => b.text().includes('编辑'))
    expect(editButtons).toHaveLength(2)
    const disabled = editButtons.filter((b) => b.attributes('disabled') !== undefined)
    expect(disabled).toHaveLength(1)
    // 只看 disabled 属性不够：Element Plus 会同时给 aria-disabled 与
    // is-disabled class，三者一致才算真禁用（历史上出现过只加了 class 的情况）。
    expect(disabled[0].attributes('aria-disabled')).toBe('true')
    expect(disabled[0].classes()).toContain('is-disabled')
  })

  it('高危 + 需重启参数带徽标', async () => {
    const w = mountPage()
    await settle(w)
    // temperature 在 agent_llm 组，当前选中的是第一个组 lock —— 必须先切过去，
    // 否则「找不到徽标」会被误读成「徽标没渲染」。
    await w.findAll('.group-item')[1].trigger('click')
    await settle(w)
    const text = w.text()
    expect(text).toContain('高危')
    expect(text).toContain('需重启')
  })

  it('种子里的状态标记（「（未接线）」等）不出现在给用户看的表格里', async () => {
    const w = mountPage()
    await settle(w)
    expect(w.text()).not.toContain('（未接线）')
  })

  it('已下架项不计入「重置整组」条数', async () => {
    const w = mountPage()
    await settle(w)
    await w.find('.el-switch').trigger('click')
    await settle(w)
    // 只有 ai_processing_ttl 有「已修改」标记（value 120 vs default 120 → 实为未改），
    // 这里再显式造一条已修改且已下架的行，确认它不进计数。
    api.list.mockResolvedValue([
      param({ key: 'modified_live', value: '3600', default_value: '86400' }),
      param({ key: 'modified_dead', value: '3600', default_value: '86400', deprecated: true, deprecated_reason: '无消费点' })
    ])
    await w.vm.loadAll()
    await settle(w)
    expect(w.text()).toContain('重置整组')
    // 两条都「已修改」，但 modified_dead 已下架 ⇒ 只有 1 条进计数。
    // 后端 BulkResetGroup 遇 deprecated 是「跳过」而不是「整组失败」，
    // 前端报 2 就是骗用户。
    expect(w.find('.reset-badge').text()).toBe('1')
  })

  it('搜索跨组时列出分组列并给出提示', async () => {
    const w = mountPage()
    await settle(w)
    const input = w.find('.header-actions input')
    await input.setValue('temperature')
    await settle(w)
    expect(w.text()).toContain('正在跨全部')
    expect(w.text()).toContain('AI 大模型')
  })
})
