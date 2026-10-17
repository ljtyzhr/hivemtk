<template>
  <div class="my-modules">
    <el-row :gutter="12" class="stats-row">
      <el-col :span="8">
        <el-card shadow="never"><div class="stat">
          <div class="label">已开通</div><div class="value">{{ ownedCount }}</div>
        </div></el-card>
      </el-col>
      <el-col :span="8">
        <el-card shadow="never"><div class="stat">
          <div class="label">基础能力</div><div class="value">{{ baseCount }}</div>
        </div></el-card>
      </el-col>
      <el-col :span="8">
        <el-card shadow="never"><div class="stat">
          <div class="label">平台连接</div>
          <div class="value small">
            <el-tag size="small" :type="platformEnabled ? 'success' : 'info'">
              {{ platformEnabled ? '已连接' : '未连接' }}
            </el-tag>
          </div>
          <el-button size="small" text type="primary" :loading="syncing" @click="onSync" class="mt-4">
            立即同步
          </el-button>
        </div></el-card>
      </el-col>
    </el-row>

    <el-card shadow="never" class="mt-12">
      <template #header>
        <div class="card-header">
          <span>我的功能模块</span>
          <el-radio-group v-model="tab" size="small">
            <el-radio-button label="owned">已开通 ({{ owned.length }})</el-radio-button>
            <el-radio-button label="all">全部 ({{ list.length }})</el-radio-button>
          </el-radio-group>
        </div>
      </template>

      <el-form inline>
        <el-form-item label="关键词">
          <el-input v-model="keyword" placeholder="模块名称 / key" clearable style="width: 220px" />
        </el-form-item>
        <el-form-item>
          <el-button @click="$router.push('/module-market')">去市场</el-button>
          <el-button @click="$router.push('/module-market/orders')">我的订单</el-button>
        </el-form-item>
      </el-form>

      <el-table :data="filtered" v-loading="loading" stripe>
        <el-table-column label="模块" min-width="220">
          <template #default="{ row }">
            <div class="name">
              {{ row.name }}
              <el-tag size="small" :type="row.category === 'base' ? 'success' : 'warning'" effect="plain">
                {{ CATEGORY_LABEL[row.category] }}
              </el-tag>
              <el-tag v-if="row.owned" size="small" :type="STATUS_TAG[row.status]">
                {{ STATUS_LABEL[row.status] || row.status }}
              </el-tag>
            </div>
            <div class="key">{{ row.module_key }}</div>
          </template>
        </el-table-column>
        <el-table-column label="价值" min-width="240">
          <template #default="{ row }"><span class="muted">{{ row.summary }}</span></template>
        </el-table-column>
        <el-table-column label="价格" width="120">
          <template #default="{ row }">{{ priceLabel(row) }}</template>
        </el-table-column>
        <el-table-column label="开通来源" width="140">
          <template #default="{ row }">
            <el-tag v-if="row.owned" size="small" effect="plain">{{ SOURCE_LABEL[row.source] || row.source }}</el-tag>
            <span v-else class="muted">—</span>
          </template>
        </el-table-column>
        <el-table-column label="页面数" width="90" align="right">
          <template #default="{ row }">{{ (row.menu_keys || []).length }}</template>
        </el-table-column>
        <el-table-column label="到期" width="120">
          <template #default="{ row }">{{ row.expires_at ? row.expires_at.slice(0, 10) : '—' }}</template>
        </el-table-column>
        <el-table-column label="操作" width="110" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="$router.push(`/module-market/detail/${row.module_key}`)">详情</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-card shadow="never" class="mt-12">
      <template #header><span>同步日志</span></template>
      <el-table :data="logs" size="small" empty-text="暂无">
        <el-table-column label="时间" width="170">
          <template #default="{ row }">{{ (row.created_at || '').replace('T', ' ').slice(0, 19) }}</template>
        </el-table-column>
        <el-table-column label="动作" prop="action" width="150" />
        <el-table-column label="状态" width="110">
          <template #default="{ row }">
            <el-tag size="small" :type="row.status === 'success' ? 'success' : row.status === 'degraded' ? 'warning' : 'danger'">
              {{ row.status === 'success' ? '成功' : row.status === 'degraded' ? '降级' : '失败' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="已开通" prop="active_count" width="90" />
        <el-table-column label="来源" prop="source" width="160" />
        <el-table-column label="耗时" width="90">
          <template #default="{ row }">{{ row.duration_ms }} ms</template>
        </el-table-column>
        <el-table-column label="错误" prop="error_msg" min-width="200" show-overflow-tooltip />
      </el-table>
    </el-card>
  </div>
</template>

<script setup>
import { ref, reactive, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import {
  myModules, syncModules, moduleSyncLogs,
  priceLabel, MODULE_CATEGORY_LABEL, MODULE_SOURCE_LABEL, MODULE_STATUS_LABEL
} from '@/api/moduleMarket'

const CATEGORY_LABEL = MODULE_CATEGORY_LABEL
const SOURCE_LABEL = MODULE_SOURCE_LABEL
const STATUS_LABEL = MODULE_STATUS_LABEL
const STATUS_TAG = { active: 'success', expired: 'warning', disabled: 'info' }

const list = ref([])
const logs = ref([])
const loading = ref(false)
const syncing = ref(false)
const platformEnabled = ref(true)
const tab = ref('owned')
const keyword = ref('')
const filter = reactive({})

const owned = computed(() => list.value.filter(m => m.owned))
const ownedCount = computed(() => owned.value.length)
const baseCount = computed(() => list.value.filter(m => m.category === 'base').length)

const filtered = computed(() => {
  let out = tab.value === 'owned' ? owned.value : list.value
  if (keyword.value) {
    const k = keyword.value.trim().toLowerCase()
    out = out.filter(m =>
      (m.name || '').toLowerCase().includes(k) ||
      (m.module_key || '').toLowerCase().includes(k))
  }
  return out
})

async function fetch () {
  loading.value = true
  try {
    const res = await myModules()
    list.value = res?.list || []
    try {
      const l = await moduleSyncLogs({ limit: 20 })
      logs.value = l?.list || []
    } catch {
      logs.value = []
    }
  } finally {
    loading.value = false
  }
}

async function onSync () {
  syncing.value = true
  try {
    const res = await syncModules()
    platformEnabled.value = res?.platform_enabled !== false
    if (res?.degraded) {
      ElMessage.warning(res.message || '平台不可达，已沿用上次同步结果')
    } else {
      ElMessage.success(res.message || `已同步，当前开通 ${res?.active_count ?? 0} 个模块`)
    }
    fetch()
  } catch (e) {
    ElMessage.error(e?.message || '同步失败')
  } finally {
    syncing.value = false
  }
}

onMounted(fetch)
</script>

<style scoped>
.card-header { display: flex; align-items: center; justify-content: space-between; }
.mt-12 { margin-top: 12px; }
.mt-4 { margin-top: 4px; }
.stats-row { margin-bottom: 4px; }
.stat .label { color: var(--el-text-color-secondary); font-size: 12px; }
.stat .value { font-size: 24px; font-weight: 600; margin-top: 4px; }
.stat .value.small { font-size: 14px; }
.name { font-weight: 500; display: flex; align-items: center; gap: 6px; }
.key { color: var(--el-text-color-secondary); font-size: 12px; font-family: monospace; }
.muted { color: var(--el-text-color-secondary); font-size: 13px; }
</style>