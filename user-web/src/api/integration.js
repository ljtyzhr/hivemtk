import { http } from '@/utils/request';

export function getIntegrationAccountList(params) {
  return http.get('/api/integrations', params)
}
export function getIntegrationAccount(id) {
  return http.get(`/api/integrations/${id}`)
}
export function createIntegrationAccount(data) {
  return http.post('/api/integrations', data)
}
export function updateIntegrationAccount(id, data) {
  return http.put(`/api/integrations/${id}`, data)
}
export function deleteIntegrationAccount(id) {
  return http.delete(`/api/integrations/${id}`)
}
export function syncCustomers(id) {
  return http.post(`/api/integrations/${id}/sync-customers`)
}
export function syncProducts(id) {
  return http.post(`/api/integrations/${id}/sync-products`)
}
export function getSyncLogs(params) {
  return http.get('/api/integration/sync-logs', params)
}
export function getExternalCustomers() {
  return http.get('/api/integration/external-customers')
}
export function getExternalOrders() {
  return http.get('/api/integration/external-orders')
}
export function getExternalOrdersByCustomer(phone, name) {
  return http.get('/api/integration/external-orders-by-customer', { params: { phone, name } })
}
export function getExternalProducts() {
  return http.get('/api/integration/external-products')
}

export function getIntegrations(params) {
  return getIntegrationAccountList(params)
}
export function createIntegration(data) {
  return createIntegrationAccount(data)
}
export function updateIntegration(id, data) {
  return updateIntegrationAccount(id, data)
}
export function deleteIntegration(id) {
  return deleteIntegrationAccount(id)
}
export function toggleIntegrationStatus(id, status) {
  return updateIntegrationAccount(id, { status })
}
export function testIntegration(id) {
  return http.post(`/api/integrations/${id}/test`)
}
export function getIntegrationLogs(params) {
  // 后端 /api/integration/sync-logs 只按分页返回全平台日志，没有按账号过滤的参数，
  // 所以按账号取最近一页再由调用方按 platform 收窄。
  return getSyncLogs(params)
}
