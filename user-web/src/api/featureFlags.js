import { http } from '@/utils/request'

// 功能开关（feature_flags）管理端 API。
//
// 与 config_params 的分工：config_params 管「旋钮拧到几」（阈值/上限/超时），
// feature_flags 管「这一整块功能对谁开」（布尔 + 灰度百分比）。两者都归管理员在 UI 上
// 调，但语义不同——一个是调参，一个是开关，混在一页里改的人分不清「调小」和「关掉」
// 的后果差别。
//
// 下面 update / remove / enable / disable / setRollout 五个写动作在 2026-10-12 之前
// 后端只挂了 create，也就是说开关建出来之后没有任何途径启用、调灰度或删除。
// 这批端点是随 FeatureFlags.vue 一起补上的。

export function list(params = {}) {
  return http.get('/api/feature-flags', { params })
}

// stale 返回超过 FeatureFlagStaleDays（14 天）没有评估记录也没有改动过的开关。
// 这类开关通常意味着「上线后没人用过」或「代码里其实没人读它」，是清理的候选。
export function stale() {
  return http.get('/api/feature-flags/stale')
}

export function detail(id) {
  return http.get(`/api/feature-flags/${id}`)
}

export function create(data) {
  return http.post('/api/feature-flags', data)
}

export function update(id, data) {
  return http.put(`/api/feature-flags/${id}`, data)
}

export function remove(id) {
  return http.delete(`/api/feature-flags/${id}`)
}

// enable / disable 走各自独立端点而不是 update({enabled})：
// 后端 SetEnabled 会单独写一条审计（谁在什么时候把开关关了），而 Update 的
// 请求体里根本没有 enabled 字段，混用会绕开这条审计。
export function enable(id) {
  return http.post(`/api/feature-flags/${id}/enable`)
}

export function disable(id) {
  return http.post(`/api/feature-flags/${id}/disable`)
}

// setRollout 调整灰度百分比（0-100）。后端按 FNV-1a 分桶，percentage 调一次
// 命中的人群集合就变一次，所以这里不做本地乐观更新，等服务端返回再刷新列表。
export function setRollout(id, percentage) {
  return http.post(`/api/feature-flags/${id}/rollout`, { percentage })
}

export function auditLogs(id) {
  return http.get(`/api/feature-flags/${id}/audit`)
}

export function evalLogs(id) {
  return http.get(`/api/feature-flags/${id}/eval-log`)
}
