<template>
  <div class="module-orders">
    <el-card shadow="never">
      <template #header>
        <div class="card-header">
          <span>我的订单</span>
          <el-button
            size="small"
            @click="fetch"
          >
            刷新
          </el-button>
        </div>
      </template>

      <el-alert
        type="info"
        show-icon
        :closable="false"
        class="mb-12"
        title="付款流程"
        description="提交订单后请按下方指引联系平台运营完成线下付款；运营在后台确认收款后，权益会自动开通，订单状态随之变为「已开通」。"
      />

      <div
        v-if="guide"
        class="guide"
      >
        <span class="guide-label">付款指引</span>
        <span>{{ guide }}</span>
      </div>

      <el-table
        v-loading="loading"
        :data="list"
        stripe
      >
        <el-table-column
          label="订单号"
          min-width="190"
        >
          <template #default="{ row }">
            <span class="mono">{{ row.order_no }}</span>
          </template>
        </el-table-column>
        <el-table-column
          label="购买内容"
          min-width="220"
        >
          <template #default="{ row }">
            <div
              v-for="(it, i) in parseItems(row)"
              :key="i"
            >
              {{ it.item_name || it.item_key }}
              <span
                v-if="it.amount_cents"
                class="muted"
              >（¥{{ fmtPrice(it.amount_cents) }}）</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column
          label="金额"
          width="110"
          align="right"
        >
          <template #default="{ row }">
            ¥{{ fmtPrice(row.total_amount_cents) }}
          </template>
        </el-table-column>
        <el-table-column
          label="状态"
          width="130"
        >
          <template #default="{ row }">
            <el-tag
              size="small"
              :type="STATUS_TAG[row.status]"
            >
              {{ STATUS_LABEL[row.status] || row.status }}
            </el-tag>
            <div
              v-if="row.status === 'pending_payment' && waitingText(row)"
              class="wait"
            >
              {{ waitingText(row) }}
            </div>
          </template>
        </el-table-column>
        <el-table-column
          label="下单时间"
          width="170"
        >
          <template #default="{ row }">
            {{ (row.created_at || '').replace('T', ' ').slice(0, 19) }}
          </template>
        </el-table-column>
        <el-table-column
          label="操作"
          width="120"
          fixed="right"
        >
          <template #default="{ row }">
            <el-button
              v-if="row.payment_guide"
              link
              type="primary"
              @click="openGuide(row)"
            >
              付款指引
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-pagination
        v-model:current-page="page"
        v-model:page-size="pageSize"
        class="mt-12"
        :total="total"
        layout="total, prev, pager, next"
        @current-change="fetch"
      />
    </el-card>

    <el-dialog
      v-model="guideVisible"
      title="付款指引"
      width="480px"
    >
      <p class="guide-text">
        {{ currentGuide }}
      </p>
      <el-alert
        v-if="currentGuide"
        type="warning"
        :closable="false"
        title="说明"
        description="完成付款后，请告知平台运营订单号；运营确认后模块会自动开通，无需重复下单。"
      />
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted, onBeforeUnmount, computed } from 'vue'
import { ElMessage } from 'element-plus'
import {
  listModuleOrders, paymentGuide, syncModules, fmtPrice,
  ORDER_STATUS_LABEL
} from '@/api/moduleMarket'

const STATUS_LABEL = ORDER_STATUS_LABEL
const STATUS_TAG = {
  pending_payment: 'warning', paid: 'primary', fulfilled: 'success',
  cancelled: 'info', rejected: 'danger'
}

const list = ref([])
const total = ref(0)
const loading = ref(false)
const guide = ref('')
const guideVisible = ref(false)
const currentGuide = ref('')
const page = ref(1)
const pageSize = ref(20)
const syncing = ref(false)

// 有未完成的订单才轮询 —— 没有待确认订单时空转只是白耗请求。
const hasPending = computed(() => list.value.some(o => o.status === 'pending_payment'))
let pollTimer = null

// refreshGrantState 触发一次模块下发同步并刷新列表。
// 同步成功后后端会即刻刷新闸的缓存（见 service/module_catalog.go 的 Sync 尾部），
// 所以这次调用之后菜单与接口权限就是最新的。
// silent=true 专供下面的 30 秒后台轮询：轮到 4301（同步口被闸住）或 403 时，
// _silent 已经拦掉全局 toast 与跳转，这里的业务提示也一并吞掉 ——
// 后台轮询的失败没有任何用户动作可归因，弹出来只会像系统坏了。
async function refreshGrantState (silent = false) {
  syncing.value = true
  try {
    const res = await syncModules(silent ? { _silent: true } : undefined)
    if (res?.degraded) {
      ElMessage.warning(res.message || '平台暂不可达，已沿用上次同步结果')
    } else {
      ElMessage.success(res?.message || `已同步，当前开通 ${res?.active_count ?? 0} 个模块`)
    }
    await fetch(silent)
  } catch (e) {
    if (!silent) ElMessage.error(e?.message || '同步失败')
  } finally {
    syncing.value = false
  }
}

// 页面停留在订单页且存在待确认订单时，每 30 秒自动同步一次。
// 30 秒是权衡：比 10 分钟的后台周期快得多（用户"看着它开通"），
// 又不至于像 3 秒那样把同步打得太频。
function startPolling () {
  stopPolling()
  pollTimer = setInterval(() => {
    if (!hasPending.value) return
    refreshGrantState(true)
  }, 30000)
}

function stopPolling () {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

// silent 专供 30 秒后台轮询：轮询触发的刷新失败（401/403/超时）没有用户动作
// 可归因，不该弹 toast —— 两条取数都带 _silent 走静默通道。
//
// 003 T019：刷新时对比上一轮状态，pending_payment → paid/fulfilled 的迁移
// 弹「支付完成 + 模块激活」提示（用户盯着订单页等的就是这一刻）。
let prevStatuses = {}

async function fetch (silent = false) {
  const cfg = silent ? { _silent: true } : undefined
  loading.value = true
  try {
    const res = await listModuleOrders({ page: page.value, page_size: pageSize.value }, cfg)
    const next = res?.list || []
    // 状态迁移检测：上一轮 pending、这一轮 paid/fulfilled → 支付完成
    for (const o of next) {
      const prev = prevStatuses[o.order_no]
      if (prev === 'pending_payment' && (o.status === 'paid' || o.status === 'fulfilled')) {
        ElMessage.success(`订单 ${o.order_no} 支付完成，模块正在开通 —— 可在「我的模块」查看。`)
      }
    }
    prevStatuses = {}
    for (const o of next) prevStatuses[o.order_no] = o.status
    list.value = next
    total.value = res?.total || 0
  } finally {
    loading.value = false
  }
  try {
    const g = await paymentGuide(cfg)
    guide.value = g?.payment_guide || ''
  } catch {
    guide.value = ''
  }
}

function openGuide (row) {
  currentGuide.value = row.payment_guide || guide.value
  guideVisible.value = true
}

function parseItems (row) {
  try {
    const v = JSON.parse(row.items || '[]')
    return Array.isArray(v) ? v : []
  } catch {
    return []
  }
}

function waitingText (row) {
  if (!row.created_at) return ''
  const mins = Math.floor((Date.now() - new Date(row.created_at).getTime()) / 60000)
  if (mins < 60) return `已等待 ${mins} 分钟`
  const hours = Math.floor(mins / 60)
  if (hours < 24) return `已等待 ${hours} 小时`
  return `已等待 ${Math.floor(hours / 24)} 天`
}

onMounted(async () => {
  await fetch()
  startPolling()
})

onBeforeUnmount(stopPolling)
</script>

<style scoped>
.card-header { display: flex; align-items: center; justify-content: space-between; }
.mb-12 { margin-bottom: 12px; }
.mt-12 { margin-top: 12px; }
.mono { font-family: monospace; font-size: 12px; }
.muted { color: var(--el-text-color-secondary); font-size: 12px; }
.wait { color: var(--el-color-warning); font-size: 12px; margin-top: 2px; }
.guide {
  background: var(--el-fill-color-light); border-radius: 6px;
  padding: 10px 12px; margin-bottom: 12px; font-size: 13px; line-height: 1.6;
}
.guide-label { font-weight: 600; margin-right: 8px; }
.guide-text { line-height: 1.8; }
</style>