import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

export const useUserStore = defineStore('user', () => {
  const userInfo = ref({
    id: '',
    username: '',
    email: '',
    role: '',
    tenant_id: '',
    tenant_code: ''
  })
  
  const token = ref('')
  
  const isLoggedIn = computed(() => !!token.value)
  
  const username = computed(() => userInfo.value.username)
  const role = computed(() => userInfo.value.role || '')
  const isAdmin = computed(() => role.value === 'admin');
  
  const setUserInfo = (info) => {
    userInfo.value = {
      id: info.id || '',
      username: info.username || '',
      email: info.email || '',
      role: info.role || '',
      // 租户字段按「缺失就沿用旧值」合并，不是「缺失就清空」。
      // setUserInfo 有两类调用方：登录落库（带完整租户字段）和个人资料页刷新
      // （只带 id/username/email/role）。后者若按 || '' 覆盖，登录时落下的
      // tenant_code 会被抹成空串 —— 于是「登录后记录租户代码」在打开一次个人
      // 中心后就消失了（生产 localStorage 里 tenant_code 恒为空即此因），
      // 连带 HelpCenter 拼 tenant_code 查询参数也变空。
      // 换账号残留由 logout() 显式清空兜住，不靠这里兜。
      tenant_id: info.tenant_id ?? userInfo.value.tenant_id ?? '',
      tenant_code: info.tenant_code ?? userInfo.value.tenant_code ?? ''
    }
    localStorage.setItem('user_info', JSON.stringify(userInfo.value));
  }
  
  const setToken = (newToken) => {
    token.value = newToken
    if (newToken) {
      localStorage.setItem('token', newToken)
    } else {
      localStorage.removeItem('token')
    }
  }
  
  const login = (info, userToken) => {
    setUserInfo(info)
    setToken(userToken)
  }
  
  const logout = () => {
    userInfo.value = {
      id: '',
      username: '',
      email: '',
      role: '',
      tenant_id: '',
      tenant_code: ''
    }
    token.value = ''
    localStorage.removeItem('token')
    localStorage.removeItem('user_info')
  }
  
  const initAuth = () => {
    const savedToken = localStorage.getItem('token')
    if (savedToken) {
      token.value = savedToken
    }
    const savedUserInfo = localStorage.getItem('user_info')
    if (savedUserInfo) {
      try {
        const parsed = JSON.parse(savedUserInfo)
        userInfo.value = {
          id: parsed.id || '',
          username: parsed.username || '',
          email: parsed.email || '',
          role: parsed.role || '',
          tenant_id: parsed.tenant_id || '',
          tenant_code: parsed.tenant_code || ''
        }
      } catch (e) {
        localStorage.removeItem('user_info')
      }
    }
  };
  
  initAuth();
  
  return {
    userInfo,
    token,
    isLoggedIn,
    username,
    role,
    isAdmin,
    setUserInfo,
    setToken,
    login,
    logout
  }
})