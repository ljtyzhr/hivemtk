<template>
  <div class="module-orders">
    <el-card shadow="never">
      <template #header>
        <div class="card-header">
          <span>我的订单</span>
          <el-button size="small" @click="fetch">刷新</el-button>
        </div>
      </template>

      <el-alert type="info" show-icon :closable="false" class="mb-12"
        title="付款流程"
        description="提交订单后请按下方指引联系平台运营完成线下付款；运营在后台确认收款后，权益会自动开通，订单状态随之变为「已开通」。" />

      <div v-if="guide" class="guide">
        <span class="guide-label">付款指引</span>
        <span>{{ guide }}</span>
      </div>

      <el-table :data="list" v-loading="loading" stripe>
        <el-table-column label="订单号" min-width="190">
          <template #default="{ row }"><span class="mono">{{ row.order_no }}</span></template>
        </el-table-column>
        <el-table-column label="购买内容" min-width="220">
          <template #default="{ row }">
            <div v-for="(it, i) in parseItems(row)" :key="i">
              {{ it.item_name || it.item_key }}
              <span v-if="it.amount_cents" class="muted">（¥{{ fmtPrice(it.amount_cents) }}）</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="金额" width="110" align="right">
          <template #default="{ row }">¥{{ fmtPrice(row.total_amount_cents) }}</template>
        </el-table-column>
        <el-table-column label="状态" width="130">
          <template #default="{ row }">
            <el-tag size="small" :type="STATUS_TAG[row.status]">
              {{ STATUS_LABEL[row.status] || row.status }}
            </el-tag>
            <div v-if="row.status === 'pending_payment' && waitingText(row)" class="wait">
              {{ waitingText(row) }}
            </div>
          </template>
        </el-table-column>
        <el-table-column label="下单时间" width="170">
          <template #default="{ row }">{{ (row.created_at || '').replace('T', ' ').slice(0, 19) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="120" fixed="right">
          <template #default="{ row }">
            <el-button v-if="row.payment_guide" link type="primary" @click="openGuide(row)">付款指引</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-pagination class="mt-12"
        v-model:current-page="page" v-model:page-size="pageSize" :total="total"
        layout="total, prev, pager, next" @current-change="fetch" />
    </el-card>

    <el-dialog v-model="guideVisible" title="付款指引" width="480px">
      <p class="guide-text">{{ currentGuide }}</p>
      <el-alert v-if="currentGuide" type="warning" :closable="false" title="说明"
        description="完成付款后，请告知平台运营订单号；运营确认后模块会自动开通，无需重复下单。" />
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import {
  listModuleOrders, paymentGuide, fmtPrice,
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

async function fetch () {
  loading.value = true
  try {
    const res = await listModuleOrders({ page: page.value, page_size: pageSize.value })
    list.value = res?.list || []
    total.value = res?.total || 0
  } finally {
    loading.value = false
  }
  try {
    const g = await paymentGuide()
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

onMounted(fetch)
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