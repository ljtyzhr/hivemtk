import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import ElementPlus from 'element-plus'
import { createPinia } from 'pinia'

// 收件箱「分配给坐席」下拉框的渲染用例。
//
// 这一页确实坏过两次，两次都是"页面打得开、下拉永远空"，而且都不出声：
//   1. 选择器原来读 /api/system/users —— 那是整行用户资料、仅超管可访问的口子。坐席以
//      customer_service 身份点进去，后端回 403，前端的 catch 把 403 咽成空数组，界面只剩
//      一个"暂无数据"，看不出是权限问题（后端为此加了脱敏四列的 /api/inbox/agents，
//      对应判据见 user-server/internal/router/inbox_agents_route_test.go）。
//   2. 补这条链路时 api 层的方法名和调用点对不上：视图调 inboxApi.listAgents()，模块里
//      挂的是 agents()。这类失效编译期查不出来（Vue 运行时才认），所以这里刻意**不**
//      mock @/api/inbox，只 mock 它下面唯一的出口 @/utils/request 里的 http：
//      调用点、方法名、URL 三段都走真相，断言才有权说"这条链路是通的"。
//
// 渲染这一腿另外钉的是文案：选项标签取 real_name || username。SystemUser 上没有 nickname
// 字段（上一版模板写的是 s.nickname，于是所有坐席在下拉里都显示成裸用户名），
// 后端给的回退列是 real_name。

const ctx = vi.hoisted(() => ({
  // real_name 为空串的那一行是「回落 username」这条分支的证据，删了它这条腿就没牙。
  agents: [
    { id: 3, username: 'agent-a', real_name: '一线坐席', role: 'customer_service' },
    { id: 4, username: 'agent-b', real_name: '', role: 'staff' }
  ]
}))

const mocks = vi.hoisted(() => {
  const calls = []
  const respond = (url) => {
    if (url === '/api/inbox/conversations') {
      return {
        list: [{ id: 'conv-1', customer_name: '客户甲', platform: 'web', status: 'open', unread_count: 0 }],
        total: 1
      }
    }
    if (url === '/api/inbox/agents') {
      return { list: ctx.agents, total: ctx.agents.length }
    }
    if (url === '/api/inbox/stats') {
      return { total: 1, open: 1, assigned: 0, closed: 0 }
    }
    return { list: [], total: 0 }
  }
  // http.get 的返回值按 request.js 的响应拦截器口径给"已拆封的业务数据"：
  // 拦截器对 code===0 回的是 data.data，收件箱列表类接口那一层是 {list,total}。
  const http = {
    get: vi.fn((url, params) => {
      calls.push({ method: 'GET', url, params })
      return Promise.resolve(respond(url))
    }),
    post: vi.fn((url, data) => {
      calls.push({ method: 'POST', url, data })
      return Promise.resolve({})
    }),
    put: vi.fn(() => Promise.resolve({})),
    delete: vi.fn(() => Promise.resolve({})),
    upload: vi.fn(() => Promise.resolve({}))
  }
  return { http, request: vi.fn(() => Promise.resolve({})), calls }
})

vi.mock('@/utils/request', () => ({ default: mocks.request, http: mocks.http }))
vi.mock('@/stores/user', () => ({
  useUserStore: () => ({ userInfo: { id: 7, username: 'agent-a', role: 'customer_service' } })
}))

import { inboxApi } from '@/api/inbox'
import List from '@/views/inbox/List.vue'
import i18n from '@/i18n'
import { settle, settleBody } from './settle-helper'

const getUrls = () => mocks.calls.filter((c) => c.method === 'GET').map((c) => c.url)

// 走真实点击而不是直接改组件内部状态：这条用例要证的正是"界面能走到那条判据"。
async function openAssignDialog() {
  const wrapper = mount(List, {
    attachTo: document.body,
    global: { plugins: [createPinia(), ElementPlus, i18n] }
  })
  await settle(wrapper)

  const convItem = document.querySelector('.conv-item')
  expect(convItem, '会话列表没渲染出可点的会话项，后面的分配链路根本走不到').toBeTruthy()
  convItem.dispatchEvent(new MouseEvent('click', { bubbles: true }))
  await settle(wrapper)

  const assignButton = Array.from(document.querySelectorAll('.thread-actions button')).find(
    (b) => b.textContent.trim() === '分配'
  )
  expect(assignButton, '会话工具条里没有「分配」按钮').toBeTruthy()
  assignButton.dispatchEvent(new MouseEvent('click', { bubbles: true }))
  await settle(wrapper)

  const menuItem = Array.from(document.querySelectorAll('.el-dropdown-menu__item')).find(
    (i) => i.textContent.trim() === '分配给坐席…'
  )
  expect(menuItem, '分配下拉里没有「分配给坐席…」这一项').toBeTruthy()
  menuItem.dispatchEvent(new MouseEvent('click', { bubbles: true }))
  await settle(wrapper)

  const dialog = Array.from(document.querySelectorAll('.el-dialog')).find((d) =>
    d.textContent.includes('分配会话给坐席')
  )
  expect(dialog, '点完「分配给坐席…」后没弹出分配对话框').toBeTruthy()
  return { wrapper, dialog }
}

// 坐席下拉的选项，且只取这一只下拉的。
//
// 必须按 aria-controls 定位：页头筛选器另有三只 el-select（渠道/状态/置顶），Element Plus
// 的 popper 是 persistent 的，它们的选项从一开始就全量挂在 body 上。上一版直接
// document.querySelectorAll('.el-select-dropdown__item') 把渠道列表那十几项也算了进来，
// "名单为空"那条用例因此能靠别人家的选项假装自己测到了东西。
async function openAgentOptions(dialog) {
  const select = dialog.querySelector('.el-select__wrapper')
  expect(select, '分配对话框里没有坐席下拉框').toBeTruthy()
  select.dispatchEvent(new MouseEvent('click', { bubbles: true }))
  // 选项 popper 挂在 body 上，等整页 DOM 稳定（settleBody），不再赌「一次宏任务够不够」
  await settleBody()
  const input = dialog.querySelector('.el-select input')
  expect(input, '坐席下拉框没渲染出可交互的 input').toBeTruthy()
  const popperId = input.getAttribute('aria-controls')
  expect(popperId, '坐席下拉框没有 aria-controls，定位不到自己那只列表').toBeTruthy()
  const listbox = document.getElementById(popperId)
  expect(listbox, `aria-controls 指向的 ${popperId} 不在文档里`).toBeTruthy()
  return Array.from(listbox.querySelectorAll('.el-select-dropdown__item')).map((o) => o.textContent.trim())
}

beforeEach(() => {
  // 本用例按中文文案定位控件（'分配'、'分配给坐席…'、'分配会话给坐席'）。
  // jsdom 的 navigator.language 是 en-US，@/i18n 启动时因此落在 en；这些键在 en 里
  // 现在有正式译文（Assign / Assign to agent…），早先"缺键裸吐中文、恰好能匹配"的
  // 巧合不再成立。钉住 zh：这几个键 zh 取值即键名本身，断言与环境语言解耦。
  i18n.global.locale.value = 'zh'
  vi.clearAllMocks()
  mocks.calls.length = 0
  ctx.agents = [
    { id: 3, username: 'agent-a', real_name: '一线坐席', role: 'customer_service' },
    { id: 4, username: 'agent-b', real_name: '', role: 'staff' }
  ]
  document.body.innerHTML = ''
})

describe('收件箱坐席选择器', () => {
  // 走 UI 的三条各带 30000 上限：settle() 每轮要让出一个宏任务，机器被并发的
  // go test 占满时，vitest 默认的 5s 单用例上限会先把"渲染终于稳了"挤掉，
  // 红成 `Test timed out`（实测同一条用例 16s 才走完）。
  it('api 层挂的名字就是视图调用的那个，且指向 /api/inbox/agents', async () => {
    // 改名式失效（视图调 listAgents、模块挂 agents）在这一句直接 TypeError。
    expect(typeof inboxApi.listAgents).toBe('function')
    await inboxApi.listAgents()
    expect(getUrls()).toContain('/api/inbox/agents')
  })

  it('坐席名单读的是脱敏口，绝不再是仅超管的用户清单口', async () => {
    const { wrapper } = await openAssignDialog()
    expect(getUrls()).toContain('/api/inbox/agents')
    expect(getUrls().some((u) => u.startsWith('/api/system/users'))).toBe(false)
    await wrapper.unmount()
  }, 30000)

  it('下拉里每一行显示 real_name，没填 real_name 的回落成 username', async () => {
    const { wrapper, dialog } = await openAssignDialog()
    const texts = await openAgentOptions(dialog)
    expect(texts).toEqual(['一线坐席', 'agent-b'])
    // 「undefined」是模板列名写错时最典型的形态（上一版的 s.nickname 就是这个形状）。
    expect(texts.some((t) => t === 'undefined' || t === '')).toBe(false)
    await wrapper.unmount()
  }, 30000)

  it('后端名单为空时下拉就是空：不拿别人的选项冒充', async () => {
    // 403/500 时前端同样渲染空列表，所以"空"本身不是 bug；这条钉的是渲染下界 +
    // 定位口径：页头那三只筛选下拉的选项必须不算在这里，否则下一条用例改坏了也看不出来。
    ctx.agents = []
    const { wrapper, dialog } = await openAssignDialog()
    expect(await openAgentOptions(dialog)).toEqual([])
    await wrapper.unmount()
  }, 30000)
})
