import axios from 'axios';
import { ElMessage } from 'element-plus'
import { getApiConfig } from './configManager'
import i18n from '@/i18n'
import { getStoredLocale } from '@/i18n/locale'
import { markUserReported } from './errorReporting'

// 生产构建里 vue-i18n 的预编译消息编译器对全部现存 key 抛 UNEXPECTED_RETURN_TYPE
// （spec §6：dev/prod 一致，1834/1834）。这里必须兜住：t() 的调用点全在拦截器给错误对象
// 挂 status/bizCode 之前，一抛出去整条错误分支就变成 toast「SyntaxError」，
// 调用方拿到的连 Error 都不是，分流字段自然全丢。取不到译文就退回 key 本身
// （和 vue-i18n 自己遇到缺 key 的行为一致），全站 i18n 的正解仍按 §6 单独排期。
const t = (key) => {
  try {
    return i18n.global.t(key)
  } catch {
    return key
  }
}

const createRequestInstance = () => {
  let apiBaseUrl = import.meta.env?.VITE_API_BASE_URL || '';
  // localStorage 覆盖仅允许在开发环境生效；生产构建一律走 env 配置，
  // 防止被篡改的 localStorage 把全部 API 流量指向任意地址
  if (import.meta.env?.DEV) {
    try {
      const configStr = localStorage.getItem('apiConfig')
      if (configStr) {
        const cfg = JSON.parse(configStr)
        apiBaseUrl = cfg.baseUrl || apiBaseUrl
      }
    } catch (e) { /* 忽略：清理/存储/恢复类 best-effort 操作 */ }
  }
  return axios.create({
    baseURL: apiBaseUrl,
    headers: {
      'Content-Type': 'application/json'
    }
  })
};

const INIT_REDIRECT_MAP = {
  INIT_REQUIRED: '/setup'
};

// ============================================================================
// 功能模块闸（.specify/002-modular-subscription · 业务码 4301）
//
// 为什么必须在这里拦，而不是让每个页面各判一次：
// 未开通模块的接口**返回 HTTP 200 + 体内 code 4301**（本仓错误通道约定），
// 于是它会落进上面 `code === 0` 之外的通用分支 —— 只弹一句 toast。
// 用户看到的是"操作失败"，而事实是"你没买这个功能"。他既不知道为什么，
// 也不知道该去哪买 —— 于是多半会反复重试，或者以为系统坏了。
//
// 拦在这里的好处是**全站一致**：任何深链、任何页面、任何写法都得到同一句
// 引导 + 同一个去处。加它只需在业务码表登记一行，不会有页面漏掉。
// ============================================================================
const BIZ_CODE_MODULE_NOT_ENABLED = 4301

// MODULE_NOT_ENABLED_REDIRECT 未开通模块时的去处：功能模块市场。
// 带 next 参数让市场页高亮/预选该模块，省掉用户自己找的一步。
const MODULE_NOT_ENABLED_REDIRECT = '/module-market'

function handleModuleNotEnabled (data, code) {
  if (Number(code) !== BIZ_CODE_MODULE_NOT_ENABLED) return false
  const moduleKey = data && data.data && data.data.module_key
  const target = moduleKey
    ? `${MODULE_NOT_ENABLED_REDIRECT}?highlight=${encodeURIComponent(moduleKey)}`
    : MODULE_NOT_ENABLED_REDIRECT
  // 不直接跳：用户可能是误点深链（别人发的链接），跳走会丢上下文。
  // 给 5 秒缓冲 + 可关闭提示，足够他看清原因，也给误点留了退路。
  showToast(`功能模块「${moduleKey || ''}」未开通，请在功能模块市场订阅后使用`, 'warning')
  setTimeout(() => redirectTo(target), 3000)
  return true
}

let lastToastMsg = '';
let lastToastTs = 0
function showToast(message, type = 'error') {
  const msg = message || t('http.requestFailed')
  const now = Date.now()
  if (msg === lastToastMsg && now - lastToastTs < 2500) return
  lastToastMsg = msg
  lastToastTs = now
  if (type === 'warning') ElMessage.warning(msg)
  else if (type === 'info') ElMessage.info(msg)
  else ElMessage.error(msg)
}

function isJsonResponse(resp) {
  const ct =
    (resp && resp.headers && (resp.headers['content-type'] || resp.headers['Content-Type'])) || ''
  return String(ct).toLowerCase().includes('application/json')
}

function extractServerMessage(data, fallback) {
  if (data && typeof data === 'object') {
    return data.message || data.msg || fallback
  }
  return fallback
}

function buildRequestError(message, response, bizCode) {
  const err = new Error(message || t('http.requestFailed'))
  err.response = response || null
  // status / bizCode 都是给调用方分流用的机器读数字段：只靠 message 文案判分支，
  // 服务端换一次措辞（或切了语言）就把逻辑判错——浏览器任务的「忙」与「Host 离线」都栽过这条。
  err.status = (response && response.status) || 0
  if (bizCode !== undefined) err.bizCode = bizCode
  return markUserReported(err)
}

function redirectTo(path) {
  if (!path) return
  const target = path.startsWith('/') ? path : '/' + path
  const current = (window.location.hash || '').replace(/^#/, '') || '/'
  if (current === target) return
  import('@/router')
    .then((m) => {
      const r = m.default
      if (r && typeof r.push === 'function') r.push(target)
      else window.location.href = '#' + target
    })
    .catch(() => {
      window.location.href = '#' + target
    })
}

function clearAuthAndGoLogin() {
  localStorage.removeItem('token')
  localStorage.removeItem('refreshToken')
  redirectTo('/login')
}

// 取这条请求实际带出去的令牌（axios v1 的 config.headers 是 AxiosHeaders 实例）
function bearerSentWith(config) {
  const headers = config && config.headers
  if (!headers) return ''
  let raw = ''
  if (typeof headers.get === 'function') raw = headers.get('Authorization') || ''
  if (!raw) raw = headers.Authorization || headers.authorization || ''
  raw = String(raw)
  return raw.startsWith('Bearer ') ? raw.slice(7).trim() : ''
}

// 401 说的是"这把令牌不被认"，不是"此刻 localStorage 里这把不被认"。
// 迟到的响应（用户在这条请求返回之前已经重新登录）会把刚换到的新令牌连带会话一起抹掉，
// 表现为"登录成功后过一会儿又被踢回登录页"；没带令牌的请求（访客端点自己回 401）
// 更不该动站点的登录态。第一个 401 清完令牌后 localStorage 就空了，
// 并发的其余 401 因此既不再重复清、也不再弹第二条"登录已过期"。
function expireSession(sentWith) {
  const current = localStorage.getItem('token')
  if (!current) return false
  if (!sentWith || sentWith !== current) return false
  clearAuthAndGoLogin()
  return true
}

const addInterceptors = () => {
  request.interceptors.request.use(
    (config) => {
      const token = localStorage.getItem('token');
      if (token) {
        config.headers.Authorization = `Bearer ${token}`
      }
      config.headers['Accept-Language'] = getStoredLocale();
      return config
    },
    (error) => {
      console.error('请求错误:', error)
      return Promise.reject(error)
    }
  );

  request.interceptors.response.use(
    (response) => {
      const config = response.config
      let data = response.data
      const silent = !!(config && config._silent)

      if (config && config.responseType === 'blob') {
        return response.data
      }

      let body = data;
      if (!isJsonResponse(response) && typeof data === 'string') {
        try {
          const parsed = JSON.parse(data)
          if (parsed && typeof parsed === 'object') {
            body = parsed
          }
        } catch { /* 忽略：清理/存储/恢复类 best-effort 操作 */ }
      }
      if (!isJsonResponse(response) && typeof body !== 'object') {
        if (import.meta.env?.DEV) {
          console.error('[request] 收到非 JSON 响应，疑似网关/ 反向代理层 兜底到前端页面：', data)
        }
        const msg = t('http.unexpectedResponse')
        if (!silent) showToast(msg)
        return Promise.reject(buildRequestError(msg, response))
      }
      data = body

      if (data == null) {
        const msg = t('http.emptyResponse')
        if (!silent) showToast(msg)
        return Promise.reject(buildRequestError(msg, response))
      }

      const code = data.code

      if (code === 'SUCCESS' || code === 200 || code === 0) {
        return data.data
      }

      if (INIT_REDIRECT_MAP[code]) {
        const target = data.redirect || INIT_REDIRECT_MAP[code]
        redirectTo(target)
        const msg = data.message || t('http.initRequired')
        if (!silent) showToast(msg, 'warning')
        return Promise.reject(buildRequestError(msg, response, code))
      }

      const msg = extractServerMessage(data, t('http.requestFailed'));
      // 4301：未开通模块。处理函数已自行提示并安排跳转，这里**不再**弹通用 toast
      // （两句提示叠在一起只会让人更困惑）。
      if (handleModuleNotEnabled(data, code)) {
        return Promise.reject(buildRequestError(msg, response, code))
      }
      if (!silent) showToast(msg)
      return Promise.reject(buildRequestError(msg, response, code))
    },
    (error) => {
      if (error.response) {
        const { status, data, config } = error.response
        const silent = !!(config && config._silent)

        if (!isJsonResponse(error.response) || typeof data !== 'object') {
          if (import.meta.env?.DEV) {
            console.error('[request] 网关返回非 JSON 响应：', data)
          }
          const msg = t('http.unexpectedResponse')
          if (!silent) showToast(msg)
          return Promise.reject(buildRequestError(msg, error.response))
        }

        const msg = extractServerMessage(data, t('http.requestFailed'))
        const bizCode = data && data.code

        switch (status) {
          case 401: {
            // 只有"存储里这把令牌正是被打回的那把"才清凭据跳登录。
            // silent 请求同样要清——token 已过期时留着它只会让后续请求反复失败。
            if (expireSession(bearerSentWith(config)) && !silent) {
              showToast(t('http.loginExpired'))
            }
            break
          }
          case 403:
            if (!silent) showToast(t('http.accessDenied'))
            break
          case 404:
            if (!silent) showToast(t('http.notFound'))
            break
          case 429:
            if (!silent) showToast(msg || t('http.tooManyRequests'))
            break
          case 500:
          case 502:
          case 503:
          case 504:
            if (!silent) showToast(t('http.serverError'))
            break
          default:
            if (!silent) showToast(msg)
        }
        return Promise.reject(buildRequestError(msg, error.response, bizCode))
      }

      if (error.request) {
        if (!error.config || !error.config._silent)
          showToast(t('http.networkTimeout'));
        return Promise.reject(markUserReported(error))
      }
      if (!error.config || !error.config._silent)
        showToast(t('http.configError'));
      return Promise.reject(markUserReported(error))
    }
  );
};

let request = createRequestInstance();
addInterceptors()

export function getRequestInstance() {
  return request
}

export const updateRequestConfig = async () => {
  try {
  const apiConfig = getApiConfig()
  const apiBaseUrl = apiConfig.baseUrl || import.meta.env?.VITE_API_BASE_URL || '';
    
    if (request && request.defaults && request.defaults.baseURL === apiBaseUrl) {
      return { baseURL: apiBaseUrl }
    }
    
    request = axios.create({
      baseURL: apiBaseUrl,
      headers: {
        'Content-Type': 'application/json'
      }
    })
    addInterceptors()
    return { baseURL: apiBaseUrl }
  } catch (error) {
    console.error('更新请求配置失败:', error)
    throw error
  }
};

export { http } from './http';

export default request
