// 引导状态：整个前端生命周期只拉取一次，合并两个「未登录也可取」的端点：
//   - /api/system/init-status：**权威**的系统初始化状态（后端读的是 install.lock）
//   - /api/public/config：公开配置，取其中的 tenant_mode（SaaS/多租户开关）
//
// 为什么要有这一层：路由守卫此前只读 localStorage 的 system_initialized 判断
// “是否已初始化”。干净浏览器（首次访问）localStorage 为空 → isInitialized() 恒为
// false → 任何非公开路由都被推进 /setup 单机安装向导，哪怕后端早已初始化——尤其在
// SaaS（tenant_mode=on）多租户实例里，根本不存在“单机首次安装”场景，首屏应是
// /login（注册入口在登录页的 enable_register 开关）。App.vue 虽也拉了 init-status，
// 但它在路由首次判定**之后**才跑（存在竞态），且不会把已初始化的访客从 /setup 带走。
//
// ensureBootstrap() 用一个共享 Promise 去重：第一次调用发起请求，之后所有调用都
// 复用同一份已缓存结果（不会逐次网络往返）。拉取失败回落 localStorage
// （system_initialized），tenantMode 默认 false——启动链上的一环不能因一次可失败
// 的请求把站点卡死。
import { http } from '@/utils/request'
import { INIT_STATUS_KEY } from '@/utils/initHelper'

let bootstrapPromise = null

const state = {
  initialized: false,
  tenantMode: false,
}

// 拉一次 init-status 并同步 localStorage（保持与旧 App.vue 相同的落盘语义：
// 已初始化写 true、未初始化移除）。失败/返回体异常时回落 localStorage。
async function fetchInitStatus() {
  try {
    const resp = await http.get('/api/system/init-status')
    const status = resp?.data ?? resp
    if (status && typeof status === 'object' && 'initialized' in status) {
      const initialized = !!status.initialized
      if (initialized) localStorage.setItem(INIT_STATUS_KEY, 'true')
      else localStorage.removeItem(INIT_STATUS_KEY)
      return initialized
    }
  } catch { /* 落到下面的 localStorage 回落 */ }
  return localStorage.getItem(INIT_STATUS_KEY) === 'true'
}

// 拉一次公开配置取 tenant_mode；失败默认 false（单机口径）。
async function fetchTenantMode() {
  try {
    const resp = await http.get('/api/public/config')
    const cfg = resp?.data ?? resp
    if (cfg && typeof cfg === 'object') return !!cfg.tenant_mode
  } catch { /* 默认 false */ }
  return false
}

// 幂等：首次调用发起一次性并行拉取，之后复用缓存的共享 Promise。
// 返回 { initialized, tenantMode }。永不 reject。
//
// tenantMode=true 时 initialized 强制为 true：SaaS 多租户实例没有"单机首次
// 安装"场景（后端 init-status 同口径），哪怕 init-status 请求失败/滞后，
// 干净浏览器首屏也应是 /login（注册入口）而非 /setup 安装向导。
export function ensureBootstrap() {
  if (bootstrapPromise) return bootstrapPromise
  bootstrapPromise = Promise.all([fetchInitStatus(), fetchTenantMode()])
    .then(([initialized, tenantMode]) => {
      state.tenantMode = tenantMode
      state.initialized = tenantMode ? true : initialized
      return { ...state }
    })
    .catch(() => ({ ...state }))
  return bootstrapPromise
}

// 同步读已缓存的引导状态（供已过首屏、无需 await 的调用方使用）。
export function getBootstrap() {
  return { ...state }
}
