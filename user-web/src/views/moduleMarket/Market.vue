<template>
  <div class="module-market">
    <el-alert
      v-if="!platformEnabled"
      type="info"
      show-icon
      :closable="false"
      class="mb-12"
      title="平台未连接，暂不可订阅功能模块"
      description="功能模块订阅需要连接平台后开通。当前部署可正常使用全部基础能力（客户中心、会话工作台、知识库、资产与话术、自有模型接入）。"
    />

    <el-card
      shadow="never"
      class="mb-12"
    >
      <el-form inline>
        <el-form-item label="分类">
          <el-select
            v-model="filter.category"
            clearable
            placeholder="全部"
            style="width: 140px"
            @change="fetch"
          >
            <el-option
              v-for="(label, k) in CATEGORY_LABEL"
              :key="k"
              :label="label"
              :value="k"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="状态">
          <el-select
            v-model="filter.onlyUnowned"
            style="width: 140px"
            @change="fetch"
          >
            <el-option
              label="全部模块"
              :value="false"
            />
            <el-option
              label="仅未开通"
              :value="true"
            />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-input
            v-model="filter.keyword"
            placeholder="模块名称 / 关键词"
            clearable
            style="width: 220px"
            @keyup.enter="fetch"
          />
        </el-form-item>
        <el-form-item>
          <el-button
            type="primary"
            :loading="loading"
            @click="fetch"
          >
            查询
          </el-button>
          <el-button @click="fetch">
            刷新
          </el-button>
          <el-button @click="$router.push('/module-market/my')">
            我的模块
          </el-button>
          <el-button @click="$router.push('/module-market/orders')">
            我的订单
          </el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <div
      v-loading="loading"
      class="grid"
    >
      <el-card
        v-for="m in visible"
        :key="m.module_key"
        class="mod-card"
        shadow="hover"
        :class="{ highlight: m.module_key === highlightKey }"
      >
        <div class="head">
          <span class="icon">
            <el-icon><component :is="m.icon || 'Grid'" /></el-icon>
          </span>
          <div class="titles">
            <div class="name">
              {{ m.name }}
              <el-tag
                v-if="m.owned"
                size="small"
                type="success"
                effect="plain"
              >
                已开通
              </el-tag>
              <el-tag
                v-else-if="m.category === 'base'"
                size="small"
                type="success"
              >
                免费
              </el-tag>
              <el-tag
                v-else-if="m.free_to_enable"
                size="small"
                type="warning"
                effect="plain"
              >
                免费开通
              </el-tag>
            </div>
            <div class="key">
              {{ m.module_key }}
            </div>
          </div>
        </div>

        <p class="summary">
          {{ m.summary || '暂无描述' }}
        </p>

        <div class="meta">
          <span
            class="price"
            :class="{ free: m.free || m.free_to_enable }"
          >{{ priceLabel(m) }}</span>
          <span class="pages">含 {{ m.menu_count }} 个页面</span>
        </div>

        <div
          v-if="m.depends_on && m.depends_on.length"
          class="deps"
        >
          <span class="deps-label">依赖</span>
          <el-tag
            v-for="d in m.depends_on"
            :key="d"
            size="small"
            effect="plain"
            class="dep"
          >
            {{ d }}
          </el-tag>
        </div>

        <div class="actions">
          <el-button
            size="small"
            @click="$router.push(`/module-market/detail/${m.module_key}`)"
          >
            查看详情
          </el-button>
          <el-button
            v-if="!m.owned && m.category === 'paid'"
            type="primary"
            size="small"
            :disabled="!platformEnabled"
            @click="openConfirm(m)"
          >
            {{ m.free_to_enable ? '开通' : '订阅' }}
          </el-button>
          <el-button
            v-else-if="!m.owned"
            size="small"
            disabled
          >
            已包含
          </el-button>
        </div>
      </el-card>
    </div>

    <el-empty
      v-if="!loading && visible.length === 0"
      description="没有匹配的功能模块"
    />

    <!-- 订阅二次确认：必须列出依赖闭包与总价 -->
    <el-dialog
      v-model="confirmVisible"
      title="确认订阅"
      width="560px"
    >
      <div
        v-if="confirmTarget"
        class="confirm-body"
      >
        <div class="row">
          <span class="k">模块</span>
          <span class="v">{{ confirmTarget.name }}</span>
        </div>
        <div class="row">
          <span class="k">价格</span>
          <span class="v price">{{ priceLabel(confirmTarget) }}</span>
        </div>

        <el-divider content-position="left">
          将自动开通（基础能力，无需付费）
        </el-divider>
        <div
          v-if="autoEnabled.length"
          class="chips"
        >
          <el-tag
            v-for="d in autoEnabled"
            :key="d"
            size="small"
            type="success"
            effect="plain"
          >
            {{ d }}
          </el-tag>
        </div>
        <div
          v-else
          class="muted small"
        >
          无
        </div>

        <template v-if="payableDeps.length">
          <el-divider content-position="left">
            需另行订阅（不会自动开通）
          </el-divider>
          <div class="chips">
            <el-tag
              v-for="d in payableDeps"
              :key="d"
              size="small"
              type="warning"
              effect="plain"
            >
              {{ d }}
            </el-tag>
          </div>
          <el-alert
            type="info"
            :closable="false"
            class="mt-8"
            title="这些依赖不会被自动开通"
          />
        </template>

        <el-divider content-position="left">
          联系方式（线下付款必填其一；在线支付可不填）
        </el-divider>
        <el-form
          :model="form"
          label-width="80px"
          class="mt-8"
        >
          <el-form-item label="称呼">
            <el-input
              v-model="form.contact_name"
              placeholder="选填"
            />
          </el-form-item>
          <el-form-item label="电话">
            <el-input
              v-model="form.contact_phone"
              placeholder="线下付款必填其一（在线支付可不填）"
            />
          </el-form-item>
          <el-form-item label="邮箱">
            <el-input
              v-model="form.contact_email"
              placeholder="线下付款必填其一（在线支付可不填）"
            />
          </el-form-item>
          <el-form-item label="备注">
            <el-input
              v-model="form.remark"
              placeholder="选填"
            />
          </el-form-item>
        </el-form>
      </div>
      <template #footer>
        <el-button @click="confirmVisible = false">
          取消
        </el-button>
        <el-button
          type="primary"
          :loading="submitting"
          @click="submit"
        >
          提交订单
        </el-button>
      </template>
    </el-dialog>

    <!-- 收银台（003 T019）：有 payment 段 → 跳转支付 + 轮询；无 → 线下指引 -->
    <CashierDialog
      v-model="cashierVisible"
      :order-no="cashierOrderNo"
      :payment="cashierPayment"
      :guide="cashierGuide"
      :amount-cents="cashierAmountCents"
      @paid="onCashierPaid"
    />
  </div>
</template>

<script setup>
import { ref, reactive, computed, onMounted, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { useUserStore } from '@/stores/user'
import {
  listMarketModules, marketModuleDetail, createModuleOrder, priceLabel,
  MODULE_CATEGORY_LABEL
} from '@/api/moduleMarket'
import CashierDialog from '@/components/CashierDialog.vue'

const CATEGORY_LABEL = MODULE_CATEGORY_LABEL
const route = useRoute()
const router = useRouter()
const userStore = useUserStore()

// highlightKey 来自请求拦截器跳转时带的 ?highlight=<module_key>。
// 用户点深链撞到 4301 之后被带到这里，指的是"我缺的就是这个"，
// 所以直接把那张卡圈出来并滚进视野，而不是让他在几十张卡里自己找。
const highlightKey = computed(() => String(route.query.highlight || ''))

const list = ref([])
const loading = ref(false)
const submitting = ref(false)
const platformEnabled = ref(true)
const confirmVisible = ref(false)
const confirmTarget = ref(null)
const detailCache = ref({})

const filter = reactive({ category: '', onlyUnowned: false, keyword: '' })
const form = reactive({ contact_name: '', contact_phone: '', contact_email: '', remark: '' })

const visible = computed(() => {
  let out = list.value
  if (filter.category) out = out.filter(m => m.category === filter.category)
  if (filter.onlyUnowned) out = out.filter(m => !m.owned)
  if (filter.keyword) {
    const k = filter.keyword.trim().toLowerCase()
    out = out.filter(m =>
      (m.name || '').toLowerCase().includes(k) ||
      (m.module_key || '').toLowerCase().includes(k) ||
      (m.summary || '').toLowerCase().includes(k))
  }
  return out
})

// 依赖闭包拆分：基础自动开、收费需另购
const autoEnabled = ref([])
const payableDeps = ref([])

// 下单幂等键：后端 MarketOrderInput.ClientOrderNo（可选）会拿它去重。
// 生成时机必须是**开窗时**而不是提交时 —— 提交失败后用户重试要复用同一个键，
// 才挡得住双击/重试造成的重复平台订单；上一单真发出去之后再开窗才换新键。
const clientOrderNo = ref('')

// 收银台状态（003 T019）
const cashierVisible = ref(false)
const cashierOrderNo = ref('')
const cashierPayment = ref(null)
const cashierGuide = ref('')
const cashierAmountCents = ref(0)

function genClientOrderNo () {
  try {
    if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
      return crypto.randomUUID()
    }
  } catch { /* 走下面的兜底 */ }
  return Date.now() + '-' + Math.random().toString(36).slice(2)
}

async function openConfirm (m) {
  // 市场可匿名逛，但下单在鉴权档（401）。与其让人填完联系方式再撞墙，
  // 不如先带 redirect 去登录 —— 登录后回到本页接着下单。
  if (!userStore.isLoggedIn) {
    router.push({ path: '/login', query: { redirect: route.fullPath } })
    return
  }
  confirmTarget.value = m
  Object.assign(form, { contact_name: '', contact_phone: '', contact_email: '', remark: '' })
  autoEnabled.value = []
  payableDeps.value = []
  clientOrderNo.value = genClientOrderNo()
  confirmVisible.value = true
  try {
    const d = await marketModuleDetail(m.module_key)
    detailCache.value[m.module_key] = d
    autoEnabled.value = d?.auto_enabled || []
    payableDeps.value = d?.payable_dependencies || []
  } catch {
    // 详情拉不到不阻断下单：闭包为空时按"无自动补齐"处理，
    // 平台端开通时仍会按真实依赖补齐基础模块。
  }
}

async function submit () {
  // 联系方式不再前端硬拦（003 T018）：在线支付不强制、线下路径由后端
  // 按 payment 段有无判定（ErrNoContactForPayment 仅线下路径触发）。
  // 前端只在两者皆空时给一次软提醒，不阻断提交。
  if (!form.contact_phone && !form.contact_email) {
    ElMessage.info('若走线下付款，平台运营需要电话或邮箱联系你；在线支付可直接提交')
  }
  submitting.value = true
  try {
    const res = await createModuleOrder({
      module_keys: [confirmTarget.value.module_key],
      options: {},
      client_order_no: clientOrderNo.value,
      ...form
    })
    confirmVisible.value = false
    // 003 T019：有 payment 段（在线支付）→ 弹收银台；无 → 线下指引 toast
    if (res?.payment?.pay_url) {
      cashierOrderNo.value = res.order_no
      cashierPayment.value = res.payment
      cashierGuide.value = res?.payment_guide || ''
      cashierAmountCents.value = Number(res?.total_amount_cents || 0)
      cashierVisible.value = true
    } else {
      ElMessageBoxAlert(res?.payment_guide)
    }
    fetch()
  } catch (e) {
    ElMessage.error(e?.message || '下单失败')
  } finally {
    submitting.value = false
  }
}

// 收银台支付确认：提示 + 刷市场（已购标记变化）
function onCashierPaid () {
  ElMessage.success('支付成功！模块开通中，稍后自动刷新（也可手动刷新页面查看）。')
  fetch()
}

function ElMessageBoxAlert (guide) {
  ElMessage({
    message: `订单已提交。${guide || '请联系平台运营完成付款'} 付款确认后模块自动开通。`,
    type: 'success',
    duration: 8000,
    showClose: true
  })
}

async function fetch () {
  loading.value = true
  try {
    const res = await listMarketModules({ page: 1, page_size: 200 })
    list.value = res?.list || []
    platformEnabled.value = res?.platform_enabled !== false
    // 深链带过来的模块可能被当前筛选条件过滤掉了 —— 先放开筛选再滚动，
    // 否则"跳过来了却什么都没看到"比不跳更让人以为功能坏了。
    if (highlightKey.value && !visible.value.some(m => m.module_key === highlightKey.value)) {
      filter.category = ''
      filter.onlyUnowned = false
      filter.keyword = ''
    }
  } finally {
    loading.value = false
  }
  await nextTick()
  scrollToHighlight()
}

function scrollToHighlight () {
  const k = highlightKey.value
  if (!k) return
  const el = document.querySelector('.mod-card.highlight')
  if (el && el.scrollIntoView) el.scrollIntoView({ behavior: 'smooth', block: 'center' })
}

onMounted(fetch)
</script>

<style scoped>
.mb-12 { margin-bottom: 12px; }
.mt-8 { margin-top: 8px; }
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 16px;
}
.mod-card { display: flex; flex-direction: column; }
/* 深链高亮：与普通卡片的区分用"描边 + 轻微提亮"而不是变色 ——
   付费模块本来就有多种分类配色，再叠一个语义色会互相打架。 */
.mod-card.highlight {
  border-color: var(--el-color-primary);
  box-shadow: 0 0 0 2px var(--el-color-primary-light-5);
}
.head { display: flex; gap: 10px; align-items: flex-start; }
.icon {
  width: 36px; height: 36px; border-radius: 8px; flex: 0 0 36px;
  display: inline-flex; align-items: center; justify-content: center;
  background: var(--el-fill-color-light); font-size: 18px;
}
.titles { min-width: 0; }
.name { font-weight: 600; display: flex; align-items: center; gap: 6px; }
.key { color: var(--el-text-color-secondary); font-size: 12px; font-family: monospace; }
.summary { color: var(--el-text-color-secondary); font-size: 13px; line-height: 1.6; margin: 10px 0; min-height: 42px; }
.meta { display: flex; justify-content: space-between; align-items: baseline; margin-bottom: 8px; }
.price { font-size: 16px; font-weight: 600; color: var(--el-color-primary); }
.price.free { color: var(--el-color-success); }
.pages { color: var(--el-text-color-secondary); font-size: 12px; }
.deps { margin-bottom: 10px; }
.deps-label { color: var(--el-text-color-secondary); font-size: 12px; margin-right: 6px; }
.dep { margin-right: 4px; }
.actions { margin-top: auto; display: flex; gap: 8px; }
.confirm-body .row { display: flex; gap: 12px; margin-bottom: 8px; }
.confirm-body .k { width: 70px; color: var(--el-text-color-secondary); }
.confirm-body .v.price { font-weight: 600; }
.chips { display: flex; flex-wrap: wrap; gap: 6px; }
.muted { color: var(--el-text-color-secondary); }
.small { font-size: 12px; }
</style>