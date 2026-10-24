<template>
  <div class="asset-review-queue">
    <el-card shadow="never">
      <template #header>
        <div class="card-header">
          <span>资产上架审核</span>
          <div>
            <el-button @click="fetchReports">举报处理</el-button>
            <el-button type="primary" @click="refresh">刷新</el-button>
          </div>
        </div>
      </template>

      <el-form inline class="mb-12">
        <el-form-item label="类型">
          <el-select v-model="filter.asset_type" clearable placeholder="全部" style="width: 180px" @change="fetchQueue">
            <el-option label="FAQ 包" value="faq_pack" />
            <el-option label="智能体角色" value="agent_persona" />
            <el-option label="话术包" value="script_pack" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="fetchQueue">查询</el-button>
        </el-form-item>
      </el-form>

      <el-table v-loading="loading" :data="rows" border>
        <el-table-column prop="id" label="ID" width="70" />
        <el-table-column prop="asset_code" label="资产编码" min-width="140" />
        <el-table-column prop="name" label="名称" min-width="160" />
        <el-table-column prop="asset_type" label="类型" width="120" />
        <el-table-column prop="status" label="状态" width="120">
          <template #default="{ row }">
            <el-tag :type="statusTag(row.status)">{{ statusText(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="contributor_id" label="贡献者" width="90" />
        <el-table-column prop="created_at" label="提交时间" width="170">
          <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="220" fixed="right">
          <template #default="{ row }">
            <el-button
              size="small" type="success"
              :disabled="row.status !== 'manual_review'"
              @click="review(row, true)"
            >通过</el-button>
            <el-button
              size="small" type="danger"
              :disabled="!['submitted', 'auto_check', 'manual_review'].includes(row.status)"
              @click="review(row, false)"
            >驳回</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-pagination
        v-model:current-page="page"
        :page-size="pageSize"
        :total="total"
        layout="prev, pager, next, total"
        class="mt-12"
        @current-change="fetchQueue"
      />
    </el-card>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import assetStoreApi from '@/api/assetStore'

const rows = ref([])
const total = ref(0)
const page = ref(1)
const pageSize = 20
const loading = ref(false)
const filter = ref({ asset_type: '' })

const statusText = (s) => ({
  draft: '草稿', submitted: '已提交', auto_check: '自动检查',
  manual_review: '人工复核', published: '已发布',
  rejected: '已驳回', suspended: '已下架'
}[s] || s)
const statusTag = (s) => ({
  published: 'success', rejected: 'danger', suspended: 'danger',
  manual_review: 'warning', auto_check: 'info'
}[s] || 'info')
const fmtTime = (t) => (t ? new Date(t).toLocaleString('zh-CN', { hour12: false }) : '-')

async function fetchQueue() {
  loading.value = true
  try {
    const d = await assetStoreApi.reviewQueue({
      asset_type: filter.value.asset_type || undefined,
      page: page.value, page_size: pageSize
    }) || {}
    rows.value = d.items || []
    total.value = Number(d.total || 0)
  } catch (e) {
    ElMessage.error('加载失败：' + (e?.message || e))
  } finally {
    loading.value = false
  }
}

function refresh() {
  fetchQueue()
}

async function review(row, pass) {
  if (!pass) {
    const { value } = await ElMessageBox.prompt('请输入驳回理由', `驳回 #${row.id} ${row.name}`, {
      inputValidator: (v) => (v && v.trim() ? true : '理由必填')
    })
    await assetStoreApi.reject(row.id, value.trim())
    ElMessage.success('已驳回')
  } else {
    await ElMessageBox.confirm(`确认通过 #${row.id} ${row.name} 并发布？`, '审核通过')
    await assetStoreApi.approve(row.id)
    ElMessage.success('已发布')
  }
  fetchQueue()
}

async function fetchReports() {
  // 举报处理窗口：列出 open 举报并处理
  try {
    const d = await assetStoreApi.reports({ status: 'open', page: 1, page_size: 50 }) || {}
    const items = d.items || []
    if (!items.length) {
      ElMessage.info('暂无待处理举报')
      return
    }
    const { value } = await ElMessageBox.prompt(
      `待处理举报 ${items.length} 条：\n` +
      items.map((r) => `#${r.id} listing=${r.listing_id} [${r.category}] ${r.detail || ''}`).join('\n') +
      '\n\n输入首条举报编号进行处理，留空取消',
      '举报处理'
    )
    const id = Number((value || '').trim())
    if (!id) return
    const { value: reason } = await ElMessageBox.prompt('处理说明（确认成立则下架，不成立则驳回举报）', '处理方式', {
      inputValidator: () => true
    })
    const confirm = await ElMessageBox.confirm('点击「确定」= 举报成立（下架该 listing）；「取消」= 不成立', '处理判定')
      .then(() => true).catch(() => false)
    await assetStoreApi.handleReport(id, { confirm, reason: reason || '' })
    ElMessage.success('已处理')
  } catch (e) {
    if (e !== 'cancel' && e?.message) ElMessage.error('处理失败：' + (e?.message || e))
  }
}

onMounted(fetchQueue)
</script>

<style scoped>
.card-header { display: flex; justify-content: space-between; align-items: center; }
.mb-12 { margin-bottom: 12px; }
.mt-12 { margin-top: 12px; }
</style>
