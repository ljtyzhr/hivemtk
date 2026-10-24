import { http } from '@/utils/request'

// 知识库爬取源 / 批次 / QA 候选 API（T4 知识库冷启动，见 design/T4-kb-coldstart.md）
export const kbCrawlerApi = {
  // 源列表：kb_id
  listSources(params) {
    return http.get('/api/kb/crawler/sources', params)
  },
  // 创建源：{kb_id, source_type:'url'|'sitemap', start_url, include_patterns?, max_pages?, interval_hours?, qa_split_enabled?}
  createSource(data) {
    return http.post('/api/kb/crawler/sources', data)
  },
  updateSource(id, data) {
    return http.put(`/api/kb/crawler/sources/${id}`, data)
  },
  pauseSource(id) {
    return http.post(`/api/kb/crawler/sources/${id}/pause`)
  },
  resumeSource(id) {
    return http.post(`/api/kb/crawler/sources/${id}/resume`)
  },
  verifySite(id) {
    return http.post(`/api/kb/crawler/sources/${id}/verify`)
  },
  deleteSource(id) {
    return http.delete(`/api/kb/crawler/sources/${id}`)
  },
  // 手动同步（等价 Chatbase "Resync now"）：返回 {batch_no, status:'pending'}
  syncSource(id) {
    return http.post(`/api/kb/crawler/sources/${id}/sync`)
  },
  // 批次列表：source_id / page
  listBatches(params) {
    return http.get('/api/kb/crawler/batches', params)
  },
  retryBatch(id) {
    return http.post(`/api/kb/crawler/batches/${id}/retry`)
  }
}

export const kbQaCandidateApi = {
  // 审核队列：source_type=crawl|feedback / status / page
  list(params) {
    return http.get('/api/kb/qa/candidates', params)
  },
  approve(id) {
    return http.post(`/api/kb/qa/candidates/${id}/approve`)
  },
  reject(id) {
    return http.post(`/api/kb/qa/candidates/${id}/reject`)
  },
  publish(id) {
    return http.post(`/api/kb/qa/candidates/${id}/publish`)
  }
}
