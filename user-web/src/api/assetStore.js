import { http } from '@/utils/request'

// T5 资产市场商店化（商户侧 /asset-store + 平台审核后台 /asset-admin）
export const assetStoreApi = {
  // 商户侧
  createListing: (data) => http.post('/api/v1/asset-store/listings', data),
  submit: (id) => http.post(`/api/v1/asset-store/listings/${id}/submit`),
  transition: (id, to) => http.post(`/api/v1/asset-store/listings/${id}/transition`, { to }),
  versions: (id) => http.get(`/api/v1/asset-store/listings/${id}/versions`),
  status: (id) => http.get(`/api/v1/asset-store/listings/${id}/status`),
  rate: (data) => http.post('/api/v1/asset-store/rate', data),
  report: (data) => http.post('/api/v1/asset-store/report', data),
  // 平台审核后台
  reviewQueue: (params) => http.get('/api/v1/asset-admin/review-queue', params),
  approve: (id) => http.post(`/api/v1/asset-admin/listings/${id}/approve`),
  reject: (id, reason) => http.post(`/api/v1/asset-admin/listings/${id}/reject`, { reason }),
  suspend: (id, reason) => http.post(`/api/v1/asset-admin/listings/${id}/suspend`, { reason }),
  reports: (params) => http.get('/api/v1/asset-admin/reports', params),
  handleReport: (id, data) => http.post(`/api/v1/asset-admin/reports/${id}/handle`, data),
  attribution: (params) => http.get('/api/v1/asset-admin/attribution', params)
}

export default assetStoreApi
