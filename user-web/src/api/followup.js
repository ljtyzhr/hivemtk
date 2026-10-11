import { http } from '@/utils/request'

// 跟进提醒读写口的前端出口（对应后端 controller/followup.go，A11）。
//
// 背景：FollowUpService 的读写口此前零 HTTP 暴露，销售的"今日跟进"在前端
// 没有任何入口；本模块五条端点是它们的第一批消费方。
//
// 口径（与后端判据对齐，不在前端越权兜底）：
// - owner_id 必带（后端 400：service 层空串=不过滤全员，不能默认空）；
// - 未装配（草稿/旅程运行时 off）后端 503 —— 与"没数据"是两件事，页面按
//   状态码分别措辞，不在这里吞掉；
// - 完成/取消撞状态守卫（重复完成、终态改写）回 409，err.message 带
//   "已处理"，直接透给页面提示；
// - 提醒是进程内内存态（重启丢失）—— 这是后端既有形态，前端无从兜底。
const base = '/api/followups'
const seg = (id) => encodeURIComponent(id)

// 三条读口带 _silent，与 api/orderDraft.js 同口径：它们的失败在页面里有常驻
// 告警位（503=底座未装配、其他=describeError 给的具体文案），拦截器再弹一条会
// 用固定的「服务器内部错误」把页面那句真话盖掉。写口（完成/取消）照旧出声。
const SILENT = { _silent: true }

export const followupApi = {
  // 今日日历：date 可选（YYYY-MM-DD，缺省后端取今天）。
  today(ownerId, date) {
    const params = { owner_id: ownerId }
    if (date) params.date = date
    return http.get(`${base}/today`, params, SILENT)
  },

  // 全部待办：limit 可选（后端缺省 50，给了必须 1..500）。
  pending(ownerId, limit) {
    const params = { owner_id: ownerId }
    if (limit) params.limit = limit
    return http.get(`${base}/pending`, params, SILENT)
  },

  overdue(ownerId) {
    return http.get(`${base}/overdue`, { owner_id: ownerId }, SILENT)
  },

  // 完成跟进：result 七选一（contacted/interested/quoted/converted/rejected/
  // lost/no_response），缺省 contacted；note 自由文本。会推进旅程+记销售事件，
  // 因此后端有"已处理"守卫（重复完成 409）。
  complete(id, result, note) {
    return http.post(`${base}/${seg(id)}/complete`, { result: result || '', note: note || '' })
  },

  cancel(id) {
    return http.post(`${base}/${seg(id)}/cancel`)
  }
}

export default followupApi
