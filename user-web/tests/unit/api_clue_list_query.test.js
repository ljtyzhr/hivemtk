/**
 * 线索列表查询串的序列化契约。
 *
 * 页面那一层把条件交给 api.clue.list(page, limit, filters) 后，剩下的丢失点就在拼串：
 * 1) 用 `if (value)` 之类的真值判断筛参数会把「未验证＝0」整条丢掉 —— 用户选了未验证，
 *    看到的却是全量，比报错更难发现；
 * 2) 手工拼 `?keyword=${kw}` 时，中文之外的 `&`/`=`/`#` 会把关键字截断成前缀，
 *    搜 "a&b" 实际搜的是 "a"（库里 49 条 vs 真值 0 条，两处都不报错）。
 * 所以这里既断参数在不在，也断编码后的字节。
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'

const { http } = vi.hoisted(() => ({
  http: { get: vi.fn(), post: vi.fn(), put: vi.fn(), patch: vi.fn(), delete: vi.fn() }
}))
vi.mock('@/utils/request', () => ({ default: { get: http.get }, http }))

import { clueApi } from '@/api/clue.js'

const sentUrl = () => http.get.mock.calls.at(-1)[0]

describe('clueApi.list 查询串', () => {
  beforeEach(() => {
    http.get.mockReset()
    http.get.mockResolvedValue({ list: [], total: 0 })
  })

  it('不传筛选时只有分页两个参数', async () => {
    await clueApi.list(1, 10)
    const u = new URL(sentUrl(), 'http://x')
    expect(u.pathname).toBe('/api/clues/list')
    expect([...u.searchParams.keys()].sort()).toEqual(['limit', 'page'])
    expect(u.searchParams.get('page')).toBe('1')
    expect(u.searchParams.get('limit')).toBe('10')
  })

  it('空串/null/undefined 不算条件（不往查询串里塞空参数）', async () => {
    await clueApi.list(1, 10, { keyword: '', type: null, is_verify: undefined, start: 0 })
    expect([...new URL(sentUrl(), 'http://x').searchParams.keys()].sort()).toEqual(['limit', 'page', 'start'])
  })

  it('0 是真条件：is_verify=0（未验证）必须出现在查询串里', async () => {
    await clueApi.list(1, 10, { is_verify: 0, type: 0 })
    const u = new URL(sentUrl(), 'http://x')
    expect(u.searchParams.get('is_verify')).toBe('0')
    expect(u.searchParams.get('type')).toBe('0')
  })

  it('中文关键字按 URL 编码送出，读回仍是原词', async () => {
    await clueApi.list(1, 10, { keyword: '小红书测试' })
    expect(sentUrl()).toContain('keyword=%E5%B0%8F%E7%BA%A2%E4%B9%A6%E6%B5%8B%E8%AF%95')
    expect(new URL(sentUrl(), 'http://x').searchParams.get('keyword')).toBe('小红书测试')
  })

  it('& = # 之类的分隔符不会把关键字劈成两段，也不会注入新参数', async () => {
    for (const kw of ['a&b', 'x=1', 'y#z', 'p%q', 'r&s=t']) {
      http.get.mockClear()
      await clueApi.list(1, 10, { keyword: kw, type: '4' })
      const u = new URL(sentUrl(), 'http://x')
      expect(u.searchParams.get('keyword'), `关键字 ${JSON.stringify(kw)} 变形了`).toBe(kw)
      expect([...u.searchParams.keys()].sort(), `多出了参数：${sentUrl()}`).toEqual(['keyword', 'limit', 'page', 'type'])
      expect(sentUrl().split('?')[1].split('&')).toHaveLength(4)
    }
  })

  it('日期区间用 unix 秒，分页与筛选并存', async () => {
    await clueApi.list(3, 50, { keyword: '北京', type: '9', start: 1577808000, end: 2051279999 })
    const u = new URL(sentUrl(), 'http://x')
    expect(u.searchParams.get('page')).toBe('3')
    expect(u.searchParams.get('limit')).toBe('50')
    expect(u.searchParams.get('start')).toBe('1577808000')
    expect(u.searchParams.get('end')).toBe('2051279999')
    expect(u.searchParams.get('type')).toBe('9')
  })

  it('统计卡是另一个端点，不与列表查询串混用', async () => {
    await clueApi.list(1, 10, { type: '4' })
    http.get.mockClear()
    await clueApi.statistics()
    expect(sentUrl()).toBe('/api/clues/statistics')
  })
})
