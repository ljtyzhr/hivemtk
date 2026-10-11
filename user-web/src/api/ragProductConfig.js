import { http } from '@/utils/request'

function normalizeList(res) {
  if (Array.isArray(res)) return res
  if (Array.isArray(res?.list)) return res.list
  if (Array.isArray(res?.items)) return res.items
  return res
}

export const ragProductConfigAPI = {
  createRagProduct(data) {
    return http.post('/api/rag-config/products', data)
  },

  // 这个口子必须把 {list,page,page_size,total} 信封原样交给调用方。
  // 之前它挂了 normalizeList 把信封压成裸数组，而四个消费方读的都是 response.items /
  // response.list ⇒ 数组上取不到这些键，于是 RagProductManagement 的表格恒「暂无数据」
  // （库里 14 条）、分页恒「共 0 条」、AccountConfig 与 aiAgent/Edit 的产品下拉恒空、
  // RagOverview 的产品计数恒 0。listProducts 保持压平：它的消费方全部按数组写。
  getRagProducts(params) {
    return http.get('/api/rag-config/products', params)
  },

  listProducts(params) {
    return http.get('/api/rag-config/products', { params }).then(normalizeList)
  },

  updateRagProduct(id, data) {
    return http.put(`/api/rag-config/products/${id}`, data)
  },

  deleteRagProduct(id) {
    return http.delete(`/api/rag-config/products/${id}`)
  },

  getAccountConfig(params) {
    return http.get('/api/rag-config/accounts/config', { params })
  },

  updateAccountConfig(data) {
    return http.put('/api/rag-config/accounts/config', data)
  },

  processMessage(data) {
    return http.post('/api/rag-config/process-message', data)
  }
};

export default ragProductConfigAPI
