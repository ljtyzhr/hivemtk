<template>
  <div
    v-loading="loading"
    class="module-detail"
  >
    <template v-if="detail">
      <el-page-header
        :content="detail.name"
        style="margin-bottom: 16px"
        @back="$router.back()"
      />

      <el-row :gutter="16">
        <el-col :span="16">
          <el-card shadow="never">
            <div class="head">
              <span class="icon"><el-icon><component :is="detail.icon || 'Grid'" /></el-icon></span>
              <div>
                <div class="name">
                  {{ detail.name }}
                  <el-tag
                    v-if="detail.owned"
                    size="small"
                    type="success"
                    effect="plain"
                  >
                    已开通
                  </el-tag>
                  <el-tag
                    size="small"
                    :type="detail.category === 'base' ? 'success' : 'warning'"
                    effect="plain"
                  >
                    {{ CATEGORY_LABEL[detail.category] }}
                  </el-tag>
                </div>
                <div class="key">
                  {{ detail.module_key }}
                </div>
              </div>
            </div>

            <p class="summary">
              {{ detail.summary }}
            </p>
            <p class="desc">
              {{ detail.description || '该模块暂未提供详细说明。' }}
            </p>

            <el-descriptions
              :column="2"
              border
              size="small"
              class="mt-12"
            >
              <el-descriptions-item label="价格">
                {{ priceLabel(detail) }}
              </el-descriptions-item>
              <el-descriptions-item label="计费周期">
                {{ detail.billing_cycle === 'metered' ? '按量（token 计量）' : '按年' }}
              </el-descriptions-item>
              <el-descriptions-item label="包含页面">
                {{ detail.menu_count }} 个
              </el-descriptions-item>
              <el-descriptions-item label="模块标识">
                {{ detail.module_key }}
              </el-descriptions-item>
            </el-descriptions>
          </el-card>

          <el-card
            shadow="never"
            class="mt-12"
          >
            <template #header>
              <span>包含的页面</span>
            </template>
            <el-alert
              v-if="!menuPages.length"
              type="info"
              :closable="false"
              title="该模块暂无独立页面"
              description="它的能力以接口与配置项的形式提供，开通后在相关页面中生效。"
            />
            <div
              v-else
              class="pages"
            >
              <div
                v-for="p in menuPages"
                :key="p.key"
                class="page-row"
              >
                <span class="p-key">{{ p.key }}</span>
                <span class="p-title">{{ p.title }}</span>
                <span class="p-path">{{ p.path || '（目录节点）' }}</span>
              </div>
            </div>
            <el-alert
              type="info"
              :closable="false"
              class="mt-12"
              title="为什么要看页面清单"
              description="「买了能干什么」是最贵的信息。用户在下单前最想知道的不是价格，而是打开哪个菜单能看到它 —— 列出来能显著降低退单与投诉。"
            />
          </el-card>
        </el-col>

        <el-col :span="8">
          <el-card shadow="never">
            <template #header>
              <span>订阅</span>
            </template>
            <div
              class="big-price"
              :class="{ free: detail.free || detail.free_to_enable }"
            >
              {{ priceLabel(detail) }}
            </div>

            <template v-if="detail.auto_enabled?.length">
              <div class="side-label">
                将自动开通（免费）
              </div>
              <el-tag
                v-for="d in detail.auto_enabled"
                :key="d"
                size="small"
                type="success"
                effect="plain"
                class="tag-item"
              >
                {{ d }}
              </el-tag>
            </template>

            <template v-if="detail.payable_dependencies?.length">
              <div class="side-label mt-12">
                需另行订阅
              </div>
              <el-tag
                v-for="d in detail.payable_dependencies"
                :key="d"
                size="small"
                type="warning"
                effect="plain"
                class="tag-item"
              >
                {{ d }}
              </el-tag>
              <div class="muted small mt-4">
                付费依赖不会自动开通，需单独订阅。
              </div>
            </template>

            <el-divider />
            <el-button
              v-if="!detail.owned && detail.category === 'paid'"
              type="primary"
              style="width: 100%"
              :disabled="!platformEnabled"
              @click="$router.push('/module-market')"
            >
              {{ detail.free_to_enable ? '立即开通' : '去订阅' }}
            </el-button>
            <el-button
              v-else-if="!detail.owned"
              style="width: 100%"
              disabled
            >
              已包含在基础能力中
            </el-button>
            <el-button
              v-else
              type="success"
              style="width: 100%"
              disabled
            >
              已开通
            </el-button>

            <div class="muted small mt-12">
              当前未连接平台，暂不可订阅。基础能力不受影响。
            </div>
          </el-card>
        </el-col>
      </el-row>
    </template>

    <!-- 匿名撞上鉴权档详情接口（401/403）时的内联登录引导：
         拦截器对无令牌请求不清会话、不弹 toast（见 request.js expireSession），
         页面也不该用一条错误 toast 打发人 —— 给一个明确的去处。 -->
    <template v-else-if="authRequired">
      <el-page-header
        content="模块详情"
        style="margin-bottom: 16px"
        @back="$router.back()"
      />
      <el-alert
        type="info"
        show-icon
        :closable="false"
        title="登录后查看完整详情并订阅"
      >
        <div class="auth-gate">
          <span class="muted">市场浏览无需登录；完整说明、依赖清单与订阅入口需要登录后查看。</span>
          <el-button
            type="primary"
            size="small"
            @click="goLogin"
          >
            登录 / 注册
          </el-button>
        </div>
      </el-alert>
    </template>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { getUserMenus } from '@/api/menu'
import { marketModuleDetail, priceLabel, MODULE_CATEGORY_LABEL } from '@/api/moduleMarket'

const CATEGORY_LABEL = MODULE_CATEGORY_LABEL
const route = useRoute()
const router = useRouter()
const loading = ref(false)
const detail = ref(null)
const authRequired = ref(false)
const platformEnabled = ref(true)
const menuIndex = ref({})

// 菜单 key → { title, path }。从 /api/menus 拿不到已关闭模块的节点，
// 所以这里用市场列表的 menu_count + 本地兜底：拿不到路径时只显示 key。
// 刻意不做「路径不可点」的假链接 —— 点了没反应比不显示更让人困惑。
const menuPages = computed(() => {
  const idx = menuIndex.value
  return (detail.value?.menu_keys || []).map(k => ({
    key: k,
    title: idx[k]?.title || k,
    path: idx[k]?.path || ''
  }))
})

async function load () {
  loading.value = true
  try {
    // 走详情接口而不是从列表里 find：
    // 列表项不含 description / auto_enabled / payable_dependencies，
    // 用列表等于让详情页的「详细说明」与「依赖闭包」两段永远显示占位文案。
    detail.value = await marketModuleDetail(route.params.key)
    authRequired.value = false
    platformEnabled.value = detail.value?.platform_enabled !== false
  } catch (e) {
    detail.value = null
    // 详情接口在后端鉴权档：匿名 401/403 走内联登录引导，其余照旧报错。
    if (e?.status === 401 || e?.status === 403) {
      authRequired.value = true
    } else {
      ElMessage.error(e?.message || '加载模块详情失败')
    }
  } finally {
    loading.value = false
  }
  if (!authRequired.value) await loadMenuIndex()
}

// 带 redirect 去登录：登录后回到当前详情页，而不是被丢去首页。
const goLogin = () => {
  router.push({ path: '/login', query: { redirect: route.fullPath } })
}

// 菜单索引用来把 menu_key 翻成「标题 + 路径」给人看。
// 必须走 api/menu 的封装而不是裸 fetch('/api/menus')：裸 fetch 不带 Authorization 头，
// 未登录/令牌未注入时恒 401，而 catch 又是空的 —— 页面不报错，只是「页面清单」
// 一栏永远只有一串 key。
async function loadMenuIndex () {
  try {
    const res = await getUserMenus()
    const flat = {}
    const walk = ns => (ns || []).forEach(n => {
      flat[n.key] = { title: n.title, path: n.path }
      if (n.children) walk(n.children)
    })
    walk(res?.tree)
    menuIndex.value = flat
  } catch {
    menuIndex.value = {}
  }
}

onMounted(load)
</script>

<style scoped>
.mt-12 { margin-top: 12px; }
.mt-4 { margin-top: 4px; }
.tag-item { margin: 0 6px 6px 0; }
.head { display: flex; gap: 12px; align-items: center; }
.icon {
  width: 40px; height: 40px; border-radius: 8px;
  display: inline-flex; align-items: center; justify-content: center;
  background: var(--el-fill-color-light); font-size: 20px;
}
.name { font-size: 18px; font-weight: 600; display: flex; align-items: center; gap: 8px; }
.key { color: var(--el-text-color-secondary); font-size: 12px; font-family: monospace; }
.summary { margin: 12px 0 4px; font-size: 14px; }
.desc { color: var(--el-text-color-secondary); line-height: 1.7; white-space: pre-wrap; }
.big-price { font-size: 24px; font-weight: 700; color: var(--el-color-primary); margin-bottom: 12px; }
.big-price.free { color: var(--el-color-success); }
.side-label { color: var(--el-text-color-secondary); font-size: 12px; margin-bottom: 6px; }
.pages { max-height: 420px; overflow: auto; }
.page-row {
  display: grid; grid-template-columns: 1fr 1.2fr 1fr; gap: 8px;
  padding: 6px 0; border-bottom: 1px solid var(--el-border-color-lighter);
  font-size: 13px;
}
.p-key { font-family: monospace; color: var(--el-text-color-secondary); }
.p-path { font-family: monospace; font-size: 12px; color: var(--el-text-color-secondary); }
.muted { color: var(--el-text-color-secondary); }
.small { font-size: 12px; }
.auth-gate { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
</style>