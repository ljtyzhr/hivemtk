import { describe, it, expect, vi, beforeEach } from 'vitest'
import { createTokenRefresher } from '@/utils/tokenRefresh'

// JWT 只需 payload 段可解出 exp；签名段不参与本地过期判断
function makeJwt(expSec) {
  const payload = btoa(JSON.stringify({ exp: expSec }))
  return `h.${payload}.s`
}

function harness({ token, refreshCall, now = 1_000_000 }) {
  let current = token
  const state = {
    setToken: vi.fn((t) => { current = t }),
    onSessionInvalid: vi.fn()
  }
  const refresher = createTokenRefresher({
    getToken: () => current,
    setToken: state.setToken,
    onSessionInvalid: state.onSessionInvalid,
    refreshCall,
    now: () => now
  })
  return {
    refresher,
    state,
    get current() { return current },
    set current(v) { current = v }
  }
}

describe('tokenRefresh（续期单飞 + 401 分档）', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('剩余寿命充足且非 force：不发刷新请求，原样返回令牌', async () => {
    const token = makeJwt(1_000_000 / 1000 + 3600) // 1 小时后
    const refreshCall = vi.fn()
    const h = harness({ token, refreshCall, now: 1_000_000 })
    await expect(h.refresher.refresh()).resolves.toBe(token)
    expect(refreshCall).not.toHaveBeenCalled()
    expect(h.state.setToken).not.toHaveBeenCalled()
  })

  it('并发两次只发一次刷新，并返回同一新令牌', async () => {
    const oldToken = makeJwt(1_000_000 / 1000)
    const newToken = makeJwt(1_000_000 / 1000 + 86400)
    let resolveCall
    const refreshCall = vi.fn(() => new Promise((r) => { resolveCall = r }))
    const h = harness({ token: oldToken, refreshCall, now: 1_000_000 })

    const p1 = h.refresher.refresh()
    const p2 = h.refresher.refresh()
    expect(refreshCall).toHaveBeenCalledTimes(1)
    resolveCall(newToken)
    await expect(Promise.all([p1, p2])).resolves.toEqual([newToken, newToken])
    expect(h.state.setToken).toHaveBeenCalledTimes(1)
    expect(h.current).toBe(newToken)
  })

  it('401 且令牌未变：判会话失效一次（并发失败只通知一次）', async () => {
    const oldToken = makeJwt(1_000_000 / 1000)
    const err = Object.assign(new Error('unauthorized'), { response: { status: 401 } })
    const refreshCall = vi.fn(() => Promise.reject(err))
    const h = harness({ token: oldToken, refreshCall, now: 1_000_000 })

    const p1 = h.refresher.refresh().catch(() => 'e1')
    const p2 = h.refresher.refresh().catch(() => 'e2')
    await Promise.all([p1, p2])
    expect(refreshCall).toHaveBeenCalledTimes(1)
    expect(h.state.onSessionInvalid).toHaveBeenCalledTimes(1)
    expect(h.state.setToken).not.toHaveBeenCalled()
  })

  it('401 但期间别的入口已换到新令牌：不清会话、不覆盖新令牌', async () => {
    const oldToken = makeJwt(1_000_000 / 1000)
    const otherNew = makeJwt(1_000_000 / 1000 + 86400)
    const err = Object.assign(new Error('old blacklisted'), { response: { status: 401 } })
    let h
    const refreshCall = vi.fn(async () => {
      // 模拟并发路径先把新令牌写进 localStorage
      h.current = otherNew
      throw err
    })
    h = harness({ token: oldToken, refreshCall, now: 1_000_000 })

    await expect(h.refresher.refresh()).rejects.toBe(err)
    expect(h.state.onSessionInvalid).not.toHaveBeenCalled()
    expect(h.state.setToken).not.toHaveBeenCalled()
    expect(h.current).toBe(otherNew)
  })

  it('网络错误 / 5xx：保留会话并抛错，不跳登录', async () => {
    const oldToken = makeJwt(1_000_000 / 1000)
    for (const response of [undefined, { status: 503 }]) {
      const err = Object.assign(new Error('network'), response ? { response } : {})
      const h = harness({ token: oldToken, refreshCall: vi.fn(() => Promise.reject(err)), now: 1_000_000 })
      await expect(h.refresher.refresh()).rejects.toBe(err)
      expect(h.state.onSessionInvalid).not.toHaveBeenCalled()
      expect(h.current).toBe(oldToken)
    }
  })

  it('force=true：即使未到期也换发', async () => {
    const oldToken = makeJwt(1_000_000 / 1000 + 3600)
    const newToken = makeJwt(1_000_000 / 1000 + 7200)
    const refreshCall = vi.fn(async () => newToken)
    const h = harness({ token: oldToken, refreshCall, now: 1_000_000 })

    await expect(h.refresher.refresh({ force: true })).resolves.toBe(newToken)
    expect(refreshCall).toHaveBeenCalledTimes(1)
    expect(h.current).toBe(newToken)
  })

  it('刷新响应缺少 token：抛错但不判会话失效', async () => {
    const oldToken = makeJwt(1_000_000 / 1000)
    const refreshCall = vi.fn(async () => '')
    const h = harness({ token: oldToken, refreshCall, now: 1_000_000 })

    await expect(h.refresher.refresh()).rejects.toThrow('缺少 token')
    expect(h.state.onSessionInvalid).not.toHaveBeenCalled()
    expect(h.current).toBe(oldToken)
  })
})
