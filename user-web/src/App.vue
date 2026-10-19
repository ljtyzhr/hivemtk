<template>
  <el-config-provider :locale="elLocale">
    <router-view />
  </el-config-provider>
</template>

<script setup>
import { computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import en from 'element-plus/es/locale/lang/en'
import ja from 'element-plus/es/locale/lang/ja'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import i18n from '@/i18n'
import { elLocaleCode, applyDirection } from '@/i18n/locale'
import { ensureBootstrap } from '@/utils/bootstrapState'
import { useOnboardingAutoStart } from '@/composables/useOnboarding'

const elMap = { 'zh-cn': zhCn, en, ja }
const elLocale = computed(() => elMap[elLocaleCode(i18n.global.locale.value)] || zhCn)

const router = useRouter()
const t = i18n.global.t

const { tryAutoStart } = useOnboardingAutoStart()

onMounted(async () => {
  applyDirection(i18n.global.locale.value)
  try {
    // 与路由守卫共用同一份引导状态（ensureBootstrap 内部去重，不会二次拉取）。
    // 未初始化时的 localStorage 落盘由 bootstrapState 负责；这里只保留“未初始化且
    // 不在公开页”时的提示 + 跳转，以及“已初始化”时自动启动新手引导的副作用。
    const { initialized } = await ensureBootstrap()

    const currentPath = router.currentRoute.value.path

    // /register 也在豁免名单：SaaS 上深链 #/register 直达注册页时，引导检查失败
    // 不该把它 replace 成 /setup（注册页本来就在初始化流程之外）。
    const isPublicEmbedPath =
      currentPath.startsWith('/chat/embed') ||
      currentPath === '/setup' ||
      currentPath === '/login' ||
      currentPath === '/register';
    if (!initialized) {
      if (!isPublicEmbedPath) {
        ElMessage.info(t('system.completeInitFirst'))
        router.replace('/setup')
      }
    } else {
      // 系统已初始化，尝试自动启动新手引导（仅首次登录）
      tryAutoStart()
    }
  } catch (e) {
    console.warn('[App] bootstrap check failed:', e);
  }
})
</script>

<style>
#app {
  font-family: Avenir, Helvetica, Arial, sans-serif;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
  color: #2c3e50;
  height: 100vh;
  margin: 0;
  padding: 0;
}

* {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
}

html, body {
  height: 100%;
  margin: 0;
  padding: 0;
}
</style>
