// 令牌续期的单一实现（request 拦截器与 chatSocket 共用同一份单飞状态）。
//
// 为什么单独成模块：旧实现有两处各自发 `/api/auth/refresh-token`（请求拦截器
// 的 ensureFreshToken 与 chatSocket 的断线重连），并发时后端会轮换两次令牌，
// 后一次拿已拉黑的旧令牌打回 401；而失败分支无条件清凭据，可能把另一次刚换到
// 的新令牌一并抹掉 —— 表现为「用着用着被踢回登录页」。
//
// 这里用可注入依赖的工厂，便于单测覆盖「并发只发一次 / 401 分档 / 不覆盖新令牌」。

export const TOKEN_REFRESH_WINDOW_MS = 15 * 60 * 1000

export function parseJwtExp(token) {
  try {
    const payload = String(token).split('.')[1]
    if (!payload) return null
    const base64 = payload.replace(/-/g, '+').replace(/_/g, '/')
    const padded = base64 + '='.repeat((4 - (base64.length % 4)) % 4)
    const decoded = JSON.parse(atob(padded))
    const exp = decoded && decoded.exp
    return typeof exp === 'number' ? exp : null
  } catch {
    return null
  }
}

/**
 * @param {object} deps
 * @param {() => string|null} deps.getToken
 * @param {(t: string) => void} deps.setToken
 * @param {(reason: string) => void} deps.onSessionInvalid 仅在“确定这把令牌已失效”时调用
 * @param {(oldToken: string) => Promise<string>} deps.refreshCall 返回新令牌
 * @param {number} [deps.windowMs]
 * @param {() => number} [deps.now]
 */
export function createTokenRefresher({
  getToken,
  setToken,
  onSessionInvalid,
  refreshCall,
  windowMs = TOKEN_REFRESH_WINDOW_MS,
  now = () => Date.now()
}) {
  let inFlight = null
  let notifiedInvalid = false

  async function refresh({ force = false } = {}) {
    const token = getToken()
    if (!token) return null

    if (!force) {
      const exp = parseJwtExp(token)
      // 非 JWT / 读不出 exp：不在这里判死，交给服务端 401 收口
      if (exp === null) return token
      if (exp * 1000 - now() > windowMs) return token
    }

    if (inFlight) return inFlight

    inFlight = (async () => {
      try {
        const newToken = await refreshCall(token)
        if (!newToken) throw new Error('refresh-token 响应缺少 token')
        // 只有仓储里仍是发起时那把旧令牌才落盘；并发中别处已换到新令牌时不覆盖。
        if (getToken() === token) {
          setToken(newToken)
          notifiedInvalid = false
          return newToken
        }
        return getToken()
      } catch (err) {
        const status = err?.response?.status
        // 只有“明确 401 + 当前仍是发起刷新时那把令牌”才判会话失效。
        // 网络错误 / 5xx / 并发轮换期间别人已换到新令牌，都不清会话。
        if (status === 401 && getToken() === token && !notifiedInvalid) {
          notifiedInvalid = true
          onSessionInvalid?.('refresh_401')
        }
        throw err
      } finally {
        inFlight = null
      }
    })()

    return inFlight
  }

  return { refresh }
}
