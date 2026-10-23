// @hivemtk/http-core —— OPT-FE-05（2026-10-24）第一步：抽响应信封/错误形状纯函数。
//
// 规划口径（未完成事项实施规划 §4.1）：先把重复的拦截器/错误处理抽成独立包，
// 再迁 bridge/platform-web/assetdpo；保留网关差异适配层。本轮做 user-web 单仓
// 验证的第一片：把 **零依赖、可跨前端复用** 的信封判定与错误形状抽到这里——
// 它们是各前端 request.js 里逐字重复的部分（isJsonResponse / extractServerMessage /
// SUCCESS code 判定 / bizCode 错误挂载）。
//
// 有意**不**抽的部分：i18n、ElMessage、router 跳转、localStorage 会话、令牌续期
// ——这些是应用侧差异（网关差异适配层），留在各前端的 request.js。等
// bridge/platform-web 迁移时按同一接口各配一份适配层。
//
// 形态与 @hivemtk/browser-core 相同：零依赖、ESM、相对路径直引
// （`import ... from '../../../packages/http-core/index.js'`），等三仓都消费后
// 再定稿发布方式（npm workspace / git 依赖）。

// 业务码：功能模块闸（.specify/002-modular-subscription）
export const BIZ_CODE_MODULE_NOT_ENABLED = 4301
// 业务码：订阅到期（003 T023）
export const BIZ_CODE_SUBSCRIPTION_EXPIRED = 4305

// HTTP 业务码成功的三种拼写：字符串 SUCCESS / HTTP 语义 200 / 信封 0。
// 各前端响应拦截器里逐字重复的判定，抽成单一来源。
export function isBizSuccess(code) {
  return code === 'SUCCESS' || code === 200 || code === 0
}

// 响应是否 JSON（按 content-type 判；axios headers 大小写不保证）
export function isJsonContentType(headers) {
  const ct =
    (headers && (headers['content-type'] || headers['Content-Type'])) || ''
  return String(ct).toLowerCase().includes('application/json')
}

// 从信封里取服务端消息（message / msg 兼容），取不到回落 fallback。
export function extractServerMessage(data, fallback) {
  if (data && typeof data === 'object') {
    return data.message || data.msg || fallback
  }
  return fallback
}

// createBizError 工厂：构造带机器读分流字段（status / bizCode）的 Error。
// markReported 由调用方注入（user-web 挂 errorReporting.markUserReported，
// 其它前端可传 (e)=>e）——包本体不知道"用户上报"这个应用概念。
export function createBizError(message, response, bizCode, markReported) {
  const err = new Error(message)
  err.response = response || null
  err.status = (response && response.status) || 0
  if (bizCode !== undefined) err.bizCode = bizCode
  return markReported ? markReported(err) : err
}
