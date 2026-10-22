<template>
  <el-container class="app-shell">
    <MessageNotification />
    <!-- 订阅横幅（003 T023）：trial 剩 N 天提醒 / expired 红条，仅 saas 版渲染 -->
    <SubscriptionBanner />

    
    <el-header class="app-header">
      <div class="brand">
        <span class="brand-mark" />
        <span class="brand-text">{{ t('system.appName') }}</span>
      </div>

      <el-menu
        :default-active="activeTopMenu"
        mode="horizontal"
        :ellipsis="false"
        class="top-menu"
        @select="handleTopMenuSelect"
      >
        <el-menu-item
          v-for="menu in filteredTopMenus"
          :key="menu.key"
          :index="menu.key"
        >
          <el-icon><component :is="resolveRouteIcon(menu.icon)" /></el-icon>
          <span>{{ t('menu.' + menu.key) }}</span>
        </el-menu-item>
      </el-menu>

      
      <div
        v-if="userStore.isLoggedIn"
        class="notif-bell"
        role="button"
        tabindex="0"
        aria-label="通知"
        @click="router.push({ name: 'Notifications' })"
        @keydown.enter.prevent="router.push({ name: 'Notifications' })"
        @keydown.space.prevent="router.push({ name: 'Notifications' })"
      >
        <el-badge
          :value="unreadCount"
          :hidden="unreadCount === 0"
          :max="99"
        >
          <el-icon :size="20">
            <Bell />
          </el-icon>
        </el-badge>
      </div>

      <div
        v-if="userStore.isLoggedIn"
        class="notif-bell geo-alert-bell"
        role="button"
        tabindex="0"
        aria-label="GEO 告警"
        :title="`GEO 告警：${geoAlertCount} 条未确认`"
        @click="router.push('/geo-tools/alerts')"
        @keydown.enter.prevent="router.push('/geo-tools/alerts')"
        @keydown.space.prevent="router.push('/geo-tools/alerts')"
      >
        <el-badge
          :value="geoAlertCount"
          :hidden="geoAlertCount === 0"
          :max="99"
        >
          <el-icon
            :size="20"
            color="#e6a23c"
          >
            <Warning />
          </el-icon>
        </el-badge>
      </div>

      
      <div
        v-if="userStore.isLoggedIn"
        class="user-area"
      >
        <LanguageSwitcher />
        <el-dropdown
          trigger="click"
          @command="handleUserCommand"
        >
          <span class="user-dropdown">
            <el-avatar
              :size="30"
              class="user-avatar"
            >{{ userStore.username?.charAt(0) || 'U' }}</el-avatar>
            <span class="user-name">{{ userStore.username }}</span>
            <el-icon class="el-icon--right"><arrow-down /></el-icon>
          </span>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item command="profile">
                <el-icon><User /></el-icon>{{ t('layout.profile') }}
              </el-dropdown-item>
              <el-dropdown-item command="help-center">
                <el-icon><QuestionFilled /></el-icon>{{ t('layout.helpCenter') }}
              </el-dropdown-item>
              <el-dropdown-item
                command="global-tour"
                divided
              >
                <el-icon><Compass /></el-icon>{{ t('layout.globalTour') }}
              </el-dropdown-item>
              <el-dropdown-item
                command="logout"
                divided
              >
                <el-icon><SwitchButton /></el-icon>{{ t('layout.logout') }}
              </el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </div>

      
      <div
        v-else
        class="login-button"
        role="button"
        tabindex="0"
        @click="handleLogin"
        @keydown.enter.prevent="handleLogin"
        @keydown.space.prevent="handleLogin"
      >
        <el-icon><User /></el-icon>
        <span>{{ t('layout.login') }}</span>
      </div>
    </el-header>

    <el-container class="app-body">
      <el-aside
        v-if="currentTopMenu && currentTopMenu.children"
        :width="sidebarCollapsed ? '64px' : '240px'"
        class="app-aside"
        :class="{ 'is-collapsed': sidebarCollapsed }"
      >
        <div class="aside-brand">
          <el-icon v-show="!sidebarCollapsed">
            <Menu />
          </el-icon>
          <span v-show="!sidebarCollapsed">{{ t('menu.' + currentTopMenu.key) }}</span>
          <el-button
            class="collapse-btn"
            :class="{ 'collapse-btn--collapsed': sidebarCollapsed }"
            text
            :title="sidebarCollapsed ? t('layout.expandMenu') : t('layout.collapseMenu')"
            @click="toggleSidebar"
          >
            <el-icon><Fold v-if="!sidebarCollapsed" /><Expand v-else /></el-icon>
          </el-button>
        </div>
        <el-scrollbar>
          <el-menu
            :default-active="route.path"
            :unique-opened="false"
            :collapse="sidebarCollapsed"
            :collapse-transition="true"
            mode="vertical"
            @select="handleSubMenuSelect"
          >
            <template
              v-for="subMenu in currentTopMenu.children"
              :key="subMenu.key"
            >
              <sub-menu-item
                :menu="subMenu"
                :icon-components="iconComponents"
              />
            </template>
          </el-menu>
        </el-scrollbar>
      </el-aside>

      
      <el-main class="app-main">
        <div
          v-if="breadcrumb.length"
          class="breadcrumb-wrap"
        >
          <el-breadcrumb separator="/">
            <el-breadcrumb-item
              v-for="(b, i) in breadcrumb"
              :key="i"
            >
              {{ b }}
            </el-breadcrumb-item>
          </el-breadcrumb>
        </div>
        <router-view v-slot="{ Component }">
          <transition name="fade">
            <component
              :is="Component"
              :key="$route.fullPath"
            />
          </transition>
        </router-view>
      </el-main>
    </el-container>
  </el-container>
</template>

<script setup>
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import SubMenuItem from '@/components/SubMenuItem.vue'
import MessageNotification from '@/components/MessageNotification.vue'
import SubscriptionBanner from '@/components/SubscriptionBanner.vue'
import LanguageSwitcher from '@/components/LanguageSwitcher.vue'
import i18n from '@/i18n'
import { useUserStore } from '@/stores/user'
import { usePermissionStore } from '@/stores/permission'
import { useOnboarding } from '@/composables/useOnboarding'
import { Warning, Bell, Menu, SwitchButton, Fold, Expand, QuestionFilled, Refresh, Compass } from '@element-plus/icons-vue'
import { routeIconMap, resolveRouteIcon } from '@/utils/iconMap'
void Bell; void Warning; void Menu; void SwitchButton; void Fold; void Expand; void Refresh; void Compass

const { startGlobalTour } = useOnboarding()

// 传给 SubMenuItem 的仍是整张表（它内部走 resolveRouteIcon 逐个解析），
// 顶层菜单这里也走同一个解析函数：2026-10-12 之前这里写的是
// <component :is="iconComponents[menu.icon]" />，直接下标没有兜底，
// 一个没在 iconMap 登记的图标名会得到 undefined，静默渲染成空白——
// 菜单项还在、只是少了图标，比整条路由失效更难被发现。
const iconComponents = routeIconMap

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()
const permissionStore = usePermissionStore()
const t = i18n.global.t

const activeTopMenu = ref('')
const activeSubMenu = ref(route.path)
const unreadCount = ref(0)
let unreadTimer = null
import { notificationApi } from '@/api/notification'
// 铃铛未读角标：轮询通知中心的未读数。
//
// 口径与通知中心一致：平台消息按人独立已读（一人点开不影响他人），
// 本地业务通知沿用原有口径。
const loadUnreadCount = async () => {
  if (!userStore.isLoggedIn) return
  try {
    const res = await notificationApi.unreadCount()
    const data = res?.data || res
    unreadCount.value = Number(data?.count || 0)
  } catch { /* 静默失败：铃铛角标非关键路径 */ }
}
const geoAlertCount = ref(0)
let geoAlertTimer = null
import { getGeoAlertsUnreadCount } from '@/api/geoAlert'
const loadGeoAlertCount = async () => {
  if (!userStore.isLoggedIn) return
  try {
    const res = await getGeoAlertsUnreadCount()
    const data = res?.data || res
    geoAlertCount.value = Number(data?.count || 0)
  } catch { /* 静默失败：铃铛角标非关键路径 */ }
}
const SIDEBAR_COLLAPSED_KEY = 'hivemtk_sidebar_collapsed'
const readSidebarCollapsed = () => {
  try {
    return localStorage.getItem(SIDEBAR_COLLAPSED_KEY) === '1'
  } catch {
    return false
  }
}
const sidebarCollapsed = ref(readSidebarCollapsed())
const persistSidebarCollapsed = () => {
  try {
    localStorage.setItem(SIDEBAR_COLLAPSED_KEY, sidebarCollapsed.value ? '1' : '0')
  } catch { /* 忽略：清理/存储/恢复类 best-effort 操作 */ }
}

// 菜单树改为后端驱动（RBAC）：GET /api/menus 返回按角色过滤后的树（超管全量）。
// 结构与旧静态 topMenus 一致：{ key, title, icon, path?, children? }；标题渲染走 i18n t('menu.'+key)。
const topMenus = computed(() => permissionStore.menus)

// 后端已按角色过滤菜单树，前端不再做 roles 静态过滤
const filteredTopMenus = computed(() => topMenus.value)

const currentTopMenu = computed(() => {
  return filteredTopMenus.value.find(menu => menu.key === activeTopMenu.value)
})

const breadcrumb = computed(() => {
  const chain = []
  const build = (items, trail) => {
    for (const it of items) {
      const next = [...trail, it.title]
      if (it.path === route.path) { chain.push(...next); return true }
      if (it.children && build(it.children, next)) return true
    }
    return false
  }
  for (const top of filteredTopMenus.value) {
    if (build([top], [])) break
  }
  return chain
});

const hasActiveChild = (menu, path) => {
  if (menu.path === path) return true
  if (menu.children) {
    for (const child of menu.children) {
      if (hasActiveChild(child, path)) return true
    }
  }
  return false
}

const findFirstChild = (menu) => {
  if (menu.path) return menu
  if (menu.children) {
    for (const child of menu.children) {
      const result = findFirstChild(child)
      if (result) return result
    }
  }
  return null
}

watch(() => route.path, (newPath) => {
  activeSubMenu.value = newPath
  for (const menu of filteredTopMenus.value) {
    if (hasActiveChild(menu, newPath)) {
      activeTopMenu.value = menu.key
      break
    }
  }
}, { immediate: true })

const handleTopMenuSelect = (key) => {
  activeTopMenu.value = key
  const menu = filteredTopMenus.value.find(item => item.key === key)
  if (menu && menu.children) {
    const firstChild = findFirstChild(menu)
    if (firstChild) router.push(firstChild.path)
  }
}

const handleSubMenuSelect = (path) => {
  router.push(path)
}

const toggleSidebar = () => {
  sidebarCollapsed.value = !sidebarCollapsed.value
  persistSidebarCollapsed()
}

const handleUserCommand = (command) => {
  switch (command) {
    case 'profile':
      router.push({ name: 'Profile' })
      break
    case 'help-center':
      router.push({ name: 'HelpCenter' })
      break
    case 'global-tour':
      startGlobalTour()
      break
    case 'logout':
        userStore.logout()
        // RBAC：清掉上一个身份的菜单缓存，防止残留授权面
        permissionStore.resetMenus()
        ElMessage.success('已退出登录')
        router.replace('/login')
        break
  }
}

const handleLogin = () => {
  router.push('/login')
}

onMounted(async () => {
  try {
    const { updateRequestConfig } = await import('@/utils/request')
    await updateRequestConfig()
  } catch (error) {
    console.error('初始化请求配置失败:', error)
  }
  // RBAC：拉取当前用户可见菜单树（路由守卫通常已加载，此处幂等兜底）
  permissionStore.ensureMenus().catch(() => {})
  loadGeoAlertCount()
  geoAlertTimer = setInterval(() => {
    if (document.visibilityState === 'visible') loadGeoAlertCount()
  }, 60000)

  loadUnreadCount()
  // 轮询**不按可见性跳过**。
  //
  // 照抄 geoAlertCount 的 `document.visibilityState === 'visible'` 守卫是错的：
  // 后台标签页里可见性为 hidden ⇒ 回调被跳过 ⇒ 用户切回前台时角标是旧的，
  // 要再等一个周期才更新 —— 而"切回来就看到新消息"恰恰是铃铛存在的意义。
  // 顺带一提：无头浏览器里 visibilityState 恒为 hidden，照抄会让这段代码
  // 在自动化验证中**完全不跑**，角标永远是初次加载时的值。
  //
  // 未读数是 30 秒一次的一次 GET，代价可忽略；真正该省的是后台轮询，
  // 那是另一个量级的事（见 platform 侧 module-sync 的 10 分钟周期）。
  unreadTimer = setInterval(loadUnreadCount, 30000)
})

onUnmounted(() => {
  if (geoAlertTimer) { clearInterval(geoAlertTimer); geoAlertTimer = null }
  if (unreadTimer) { clearInterval(unreadTimer); unreadTimer = null }
})
</script>

<style lang="scss" scoped>
.app-shell {
  height: 100vh;
  background: $bg-color-page;
}

/* ===== 顶栏 ===== */
.app-header {
  height: 64px;
  background: $bg-color;
  border-bottom: 1px solid $border-base;
  box-shadow: 0 1px 3px rgba(15, 23, 42, 0.04);
  display: flex;
  align-items: center;
  padding: 0 20px;
  position: relative;
  z-index: 10;
}
.brand {
  display: flex;
  align-items: center;
  gap: $spacing-md;
  margin-right: $spacing-lg;
}
.brand-mark {
  width: 30px;
  height: 30px;
  border-radius: $border-radius-base;
  background: linear-gradient(135deg, $primary-color 0%, $primary-color 100%);
  box-shadow: 0 4px 12px rgba(79, 70, 229, 0.35);
  position: relative;
  flex-shrink: 0;
}
.brand-mark::after {
  content: '';
  position: absolute;
  inset: 7px;
  border: 2px solid rgba(255, 255, 255, 0.92);
  border-radius: $border-radius-small;
  border-bottom-color: transparent;
  border-left-color: transparent;
}
.brand-text {
  font-size: $font-size-medium;
  font-weight: 700;
  color: $text-primary;
  letter-spacing: 0.3px;
  white-space: nowrap;
}
.top-menu {
  flex: 1;
  min-width: 0; /* 允许 flex 子项收缩，配合下方 overflow 让菜单可横向滚动 */
  border-bottom: none;
  background: transparent;
  overflow-x: auto;
  overflow-y: hidden;
  scrollbar-width: thin;
}
/* 强制 el-menu 内部容器按内容宽度展开，避免 el-menu 自行将溢出项 hidden 掉 */
.top-menu.el-menu,
.top-menu > .el-menu,
.top-menu .el-menu--horizontal {
  min-width: max-content;
}
.top-menu::-webkit-scrollbar { height: 4px; }
.top-menu::-webkit-scrollbar-thumb { background: $border-base; border-radius: 2px; }
.top-menu .el-menu-item {
  height: 64px;
  line-height: 64px;
  border-bottom: none;
  font-weight: 500;
  color: $text-regular;
}
.top-menu .el-menu-item:hover {
  color: $primary-color;
  background: transparent;
}
.top-menu .el-menu-item.is-active {
  color: $primary-color;
  font-weight: 600;
  border-bottom: 2px solid $primary-color;
}

/* ===== 通知铃铛 ===== */
.notif-bell {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 38px;
  height: 38px;
  margin: $spacing-sm;
  border-radius: $border-radius-base;
  color: $text-regular;
  cursor: pointer;
  transition: background-color 0.2s ease, color 0.2s ease;
}
.notif-bell:hover {
  background: $border-extra-light;
  color: $primary-color;
}
.geo-alert-bell {
  margin-left: 0;
}

/* ===== 用户区 ===== */
.user-area {
  display: flex;
  align-items: center;
  margin-left: $spacing-md;
}
.user-dropdown {
  display: flex;
  align-items: center;
  gap: $spacing-sm;
  cursor: pointer;
  padding: $spacing-xs 10px;
  border-radius: $border-radius-base;
  transition: background-color 0.2s ease;
}
.user-dropdown:hover {
  background: $border-extra-light;
}
.user-avatar {
  background: linear-gradient(135deg, $primary-color, $primary-color);
  font-weight: 600;
  font-size: $font-size-small;
}
.user-name {
  color: $text-primary;
  font-weight: 500;
  font-size: $font-size-base;
}
.login-button {
  display: flex;
  align-items: center;
  gap: $spacing-sm;
  color: $bg-color;
  background: $primary-color;
  cursor: pointer;
  padding: $spacing-sm 18px;
  border-radius: $border-radius-base;
  font-weight: 500;
  font-size: $font-size-base;
  margin-left: $spacing-md;
  transition: background 0.2s ease;
}
.login-button:hover {
  background: $primary-color;
}

/* ===== 侧边栏（品牌深靛蓝） ===== */
.app-body {
  height: calc(100vh - 64px);
}
.app-aside {
  background: linear-gradient(180deg, $primary-color 0%, $primary-color 100%);
  border-right: none;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  transition: width 0.25s ease;
}
.aside-brand {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: $spacing-md 20px;
  color: $primary-light-3;
  font-size: $font-size-base;
  font-weight: 600;
  letter-spacing: 0.5px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.07);
}
.aside-brand .el-icon {
  font-size: $font-size-large;
  color: $primary-light-2;
}
.collapse-btn {
  margin-left: auto;
  color: $primary-light-3;
}
.app-aside.is-collapsed .aside-brand {
  justify-content: center;
  padding: $spacing-md 0;
  gap: 0;
}
.app-aside.is-collapsed .collapse-btn {
  margin-left: 0;
}
.collapse-btn:hover {
  color: $bg-color;
  background: rgba(255, 255, 255, 0.06);
}
.app-aside :deep(.el-scrollbar) {
  flex: 1;
}
.app-aside :deep(.el-menu) {
  background: transparent;
  border-right: none;
  padding: $spacing-sm;
}
.app-aside :deep(.el-menu-item),
.app-aside :deep(.el-sub-menu__title) {
  color: $primary-light-3;
  border-radius: $border-radius-base;
  margin-bottom: $spacing-xs;
  height: 44px;
  line-height: 44px;
}
.app-aside :deep(.el-menu-item .el-icon),
.app-aside :deep(.el-sub-menu__title .el-icon) {
  color: $primary-light-4;
}
.app-aside :deep(.el-menu-item:hover),
.app-aside :deep(.el-sub-menu__title:hover) {
  background: rgba(255, 255, 255, 0.06);
  color: $bg-color;
}
.app-aside :deep(.el-menu-item.is-active) {
  background: rgba(99, 102, 241, 0.22);
  color: $bg-color;
  font-weight: 600;
  position: relative;
}
.app-aside :deep(.el-menu-item.is-active::before) {
  content: '';
  position: absolute;
  left: 0;
  top: 8px;
  bottom: 8px;
  width: 3px;
  border-radius: 0 3px 3px 0;
  background: $primary-light-2;
}
.app-aside :deep(.el-sub-menu.is-active > .el-sub-menu__title) {
  color: $bg-color;
}


/* ===== 主内容区 ===== */
.app-main {
  background: $bg-color-page;
  padding: 0;
  overflow-y: auto;
}
.breadcrumb-wrap {
  padding: $spacing-md 24px 0;
}
.breadcrumb-wrap :deep(.el-breadcrumb__inner) {
  color: $text-secondary;
  font-weight: 500;
}
.breadcrumb-wrap :deep(.el-breadcrumb__item:last-child .el-breadcrumb__inner) {
  color: $text-primary;
  font-weight: 600;
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.25s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
