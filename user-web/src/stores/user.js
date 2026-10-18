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
      tenant_id: info.tenant_id || '',
      tenant_code: info.tenant_code || ''
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