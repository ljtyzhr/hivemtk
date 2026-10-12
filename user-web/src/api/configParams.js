import { http } from '@/utils/request'

// includeDeprecated 默认 false：管理端默认不列已下架参数。
// 那 11 条是"代码里没有消费点"的旋钮（服务端没实现长轮询、/ws/chat 路由从未注册、
// NewResultCache 生产链路从未构造 …）。列出来用户改完去验证，行为一定不变，
// 而原因藏在代码里——所以宁可默认不列，需要查历史值时再显式打开。
export function list(group, includeDeprecated = false) {
  return http.get('/api/manage/config-params', {
    params: { group, include_deprecated: includeDeprecated ? 1 : 0 }
  })
}

// listGroups 返回分组元数据（中文名 / 说明 / 排序）。这是侧栏那列中文名的事实源，
// 曾经在 user-web 里硬编码成一张 GROUP_LABELS 表——新增参数组时前端会静默回落显示
// 英文 group key，而页面上看不出哪里不对。
export function listGroups() {
  return http.get('/api/manage/config-params/groups')
}

export function auditLogs(limit = 100) {
  return http.get('/api/manage/config-params/audit-logs', { params: { limit } })
}

export function update(group, key, value) {
  return http.put(`/api/manage/config-params/${group}/${key}`, { value })
}

export function reset(group, key) {
  return http.post(`/api/manage/config-params/${group}/${key}/reset`)
}

export function bulkReset(group) {
  return http.post(`/api/manage/config-params/${group}/reset`)
}
