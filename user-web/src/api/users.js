import { http } from '@/utils/request'

export const usersApi = {
  list: (params) => {
    return http.get('/api/users', { params })
  },
  
  get: (id) => {
    return http.get(`/api/users/${id}`)
  },
  
  create: (data) => {
    return http.post('/api/users', data)
  },
  
  update: (id, data) => {
    return http.put(`/api/users/${id}`, data)
  },
  
  delete: (id) => {
    return http.delete(`/api/users/${id}`)
  },
  
  updatePassword: (id, data) => {
    return http.put(`/api/users/${id}/password`, data)
  },
  
  login: (data) => {
    return http.post('/api/auth/login', data)
  },

  // MFA 登录第二步：公开端点（无 JWT），凭 temp_token + 6 位 TOTP/恢复码换取终令牌
  mfaVerify: (data) => {
    return http.post('/api/auth/mfa/verify', data)
  }
}