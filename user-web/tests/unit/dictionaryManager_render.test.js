import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import ElementPlus, { ElMessageBox } from 'element-plus'

// 业务字典管理页的渲染用例。
//
// 这一页要钉的是三件事，每一件都对应一类「运营会当场做错」的动作：
//   1. 域与项必须显示中文名 —— 后端这批字典的权威值是英文 code，
//      页面如果只显示 code，运营根本分不清 approval_status 和 bad_case_status。
//   2. 跃迁表必须补中文名 —— 后端只存 from_code / to_code，
//      运营盯着两个英文 code 看不出这条边是「待审批 → 已批准」。
//   3. 跃迁保存是「整体替换」而不是「增量 upsert」 —— 后端 ReplaceTransitions
//      会先清空再写入，页面不把这个后果讲清楚，运营删一行就等于把整张状态机改坏。

const api = vi.hoisted(() => ({
  listParents: vi.fn(),
  get: vi.fn(),
  upsertItem: vi.fn(),
  replaceTransitions: vi.fn()
}))

vi.mock('@/api/runtimeConfig', () => ({
  dictionariesApi: {
    listParents: api.listParents,
    get: api.get,
    upsertItem: api.upsertItem,
    replaceTransitions: api.replaceTransitions
  }
}))

import DictionaryManager from '@/views/system/DictionaryManager.vue'
import { settle } from './settle-helper'

const PARENTS = [
  {
    code: 'approval_status',
    label: '审批状态',
    description: '审批请求的生命周期：提交 → 裁决 → 终态。',
    item_count: 3,
    seeded: true
  },
  {
    code: 'bad_case_status',
    label: '坏案例状态',
    description: '坏案例从发现到闭环的处理阶段。',
    item_count: 1,
    seeded: true
  }
]

const ITEMS = [
  { parent_code: 'approval_status', code: 'pending', name: '待审批', description: '已提交，等待裁决', sort_order: 10, enabled: true, system_locked: true },
  { parent_code: 'approval_status', code: 'approved', name: '已批准', description: '', sort_order: 20, enabled: true, system_locked: true },
  { parent_code: 'approval_status', code: 'rejected', name: '已驳回', description: '', sort_order: 30, enabled: true, system_locked: false }
]

const TRANSITIONS = [
  { parent_code: 'approval_status', from_code: 'pending', to_code: 'approved', description: '批准' },
  { parent_code: 'approval_status', from_code: 'pending', to_code: 'rejected', description: '驳回' }
]

function mountPage() {
  return mount(DictionaryManager, { global: { plugins: [ElementPlus] } })
}

async function openParent(w, index = 0) {
  await w.findAll('.parent-item')[index].trigger('click')
  await settle(w)
}

function clickButton(w, label) {
  const btn = w.findAll('button').find((b) => b.text().replace(/\s+/g, '').includes(label))
  if (!btn) throw new Error('找不到按钮：' + label)
  return btn.trigger('click')
}

describe('DictionaryManager.vue 渲染与危险操作提示', () => {
  beforeEach(() => {
    api.listParents.mockReset().mockResolvedValue([...PARENTS])
    api.get.mockReset().mockImplementation(async (parent) => {
      if (parent !== 'approval_status') {
        return { parent: { code: parent, label: parent }, items: [], transitions: [] }
      }
      return {
        parent: { code: 'approval_status', label: '审批状态' },
        items: ITEMS.map((i) => ({ ...i })),
        transitions: TRANSITIONS.map((t) => ({ ...t }))
      }
    })
    api.upsertItem.mockReset().mockResolvedValue({})
    api.replaceTransitions.mockReset().mockResolvedValue({})
    vi.restoreAllMocks()
  })

  it('字典域列表显示后端给的中文名与项数，而不是英文 code', async () => {
    const w = mountPage()
    await settle(w)
    expect(api.listParents).toHaveBeenCalled()
    const text = w.text()
    expect(text).toContain('审批状态')
    expect(text).toContain('坏案例状态')
    // 项数徽标要显示真实条数，否则运营不知道这个域还剩几条没登记。
    expect(w.findAll('.parent-count').map((n) => n.text())).toContain('3')
  })

  it('选中域后列出字典项并给 system_locked 项打「锁定」标记', async () => {
    const w = mountPage()
    await settle(w)
    await openParent(w)
    expect(api.get).toHaveBeenCalledWith('approval_status')
    const text = w.text()
    expect(text).toContain('待审批')
    expect(text).toContain('已驳回')
    expect(w.findAll('.locked-tag').length).toBe(2)
    expect(text).toContain('锁定')
  })

  it('跃迁表把 from/to 的 code 补成中文名', async () => {
    const w = mountPage()
    await settle(w)
    await openParent(w)
    const rows = w.findAll('.trans-name').map((n) => n.text())
    // 后端只给 from_code / to_code；页面上必须同时能看到中文名，
    // 否则运营只能对着 pending → approved 猜含义。
    expect(rows).toContain('待审批')
    expect(rows).toContain('已批准')
    expect(rows).toContain('已驳回')
  })

  it('Code 非法时拒绝提交并给出可读的规则说明', async () => {
    const w = mountPage()
    await settle(w)
    await openParent(w)
    await clickButton(w, '新增字典项')
    await settle(w)
    // 走新增路径：parent_code 已经被预填，code 是空的 —— 空值也必须被规则拦住。
    const inputs = w.findAll('.el-dialog input')
    await inputs[0].setValue('Bad-Code')
    await clickButton(w, '保存')
    await settle(w)
    expect(w.find('.form-error').text()).toContain('[a-z0-9_]')
    expect(api.upsertItem).not.toHaveBeenCalled()
  })

  it('新增字典项成功后带着完整的父域与字段提交', async () => {
    const w = mountPage()
    await settle(w)
    await openParent(w)
    await clickButton(w, '新增字典项')
    await settle(w)
    const inputs = w.findAll('.el-dialog input')
    await inputs[0].setValue('escalated')
    await inputs[1].setValue('  已升级  ')
    await clickButton(w, '保存')
    await settle(w)
    expect(api.upsertItem).toHaveBeenCalledTimes(1)
    const [parent, payload] = api.upsertItem.mock.calls[0]
    expect(parent).toBe('approval_status')
    expect(payload.code).toBe('escalated')
    // 名称两侧空白必须被裁掉：它会写进库里当展示名，
    // 带空格的 key 一旦进了运营的筛选条件就会对不上。
    expect(payload.name).toBe('已升级')
    expect(payload.enabled).toBe(true)
  })

  it('替换跃迁前必须整表确认，并把全量行交给后端', async () => {
    // confirm 必须 stub 成立即 resolve：真弹层在 jsdom 里不会有人去点确认，
    // submitTransitions 会永远卡在 await 上，整条用例变成测超时而不是测行为。
    const confirm = vi.spyOn(ElMessageBox, 'confirm').mockResolvedValue('confirm')
    const w = mountPage()
    await settle(w)
    await openParent(w)
    await clickButton(w, '编辑跃迁')
    await settle(w)
    // 打开就写明「整体替换」的后果：后端 ReplaceTransitions 先清空再写。
    expect(w.text()).toContain('整体替换')
    await clickButton(w, '整体替换')
    await settle(w)
    expect(confirm).toHaveBeenCalled()
    const [msg] = confirm.mock.calls[0]
    expect(msg).toContain('整体替换')
    expect(msg).toContain('不可撤销')
    expect(api.replaceTransitions).toHaveBeenCalledWith('approval_status', [
      { from_code: 'pending', to_code: 'approved', description: '批准' },
      { from_code: 'pending', to_code: 'rejected', description: '驳回' }
    ])
  })

  it('有半截行时直接拦下，不弹确认框也不提交', async () => {
    const confirm = vi.spyOn(ElMessageBox, 'confirm').mockResolvedValue('confirm')
    const w = mountPage()
    await settle(w)
    await openParent(w)
    await clickButton(w, '编辑跃迁')
    await settle(w)
    // 加一条没填完的跃迁再保存：后端拿到的是「被过滤掉的行」，
    // 页面若不拦就是「用户以为加了、其实没加」——而且这条行会静默消失。
    await clickButton(w, '加一条跃迁')
    await settle(w)
    await clickButton(w, '整体替换')
    await settle(w)
    expect(confirm).not.toHaveBeenCalled()
    expect(api.replaceTransitions).not.toHaveBeenCalled()
    expect(w.find('.form-error').text()).toContain('它们会被丢掉')
  })
})
