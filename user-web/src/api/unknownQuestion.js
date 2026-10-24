import { http } from '@/utils/request'

// 未知问题运营 API（T1 低置信度回流，见 design/T1-confidence-reflux.md）
export const unknownQuestionApi = {
  // 未知问题列表：status/source/keyword/agent_id/page/page_size
  list(params) {
    return http.get('/api/unknown-questions', params)
  },
  stats() {
    return http.get('/api/unknown-questions/stats')
  },
  get(id) {
    return http.get(`/api/unknown-questions/${id}`)
  },
  submit(id) {
    return http.post(`/api/unknown-questions/${id}/submit`)
  },
  review(id, data) {
    return http.post(`/api/unknown-questions/${id}/review`, data)
  },
  // 聚类簇
  clusterList(params) {
    return http.get('/api/unknown-questions/clusters', params)
  },
  clusterGet(id) {
    return http.get(`/api/unknown-questions/clusters/${id}`)
  },
  clusterReview(id, data) {
    return http.post(`/api/unknown-questions/clusters/${id}/review`, data)
  }
}
