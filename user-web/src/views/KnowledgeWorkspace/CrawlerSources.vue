<template>
  <div class="crawler-sources-page">
    <el-card class="toolbar-card">
      <div class="toolbar">
        <div class="toolbar-left">
          <el-select
            v-model="filter.kb_id"
            :placeholder="$t('选择知识库')"
            clearable
            filterable
            style="width: 220px"
            @change="loadSources"
          >
            <el-option
              v-for="kb in kbList"
              :key="kb.id"
              :label="kb.name"
              :value="kb.id"
            />
          </el-select>
          <el-select
            v-model="filter.status"
            :placeholder="$t('状态')"
            clearable
            style="width: 120px"
            @change="loadSources"
          >
            <el-option
              :label="$t('启用')"
              value="active"
            />
            <el-option
              :label="$t('已暂停')"
              value="paused"
            />
            <el-option
              :label="$t('异常')"
              value="error"
            />
          </el-select>
        </div>
        <div class="toolbar-right">
          <el-button
            type="primary"
            @click="showCreate = true"
          >
            {{ $t('新建爬取源') }}
          </el-button>
          <el-button @click="loadSources">
            {{ $t('刷新') }}
          </el-button>
        </div>
      </div>
    </el-card>

    <el-card v-loading="loading">
      <el-table
        :data="sources"
        stripe
      >
        <el-table-column
          prop="id"
          label="ID"
          width="70"
        />
        <el-table-column
          prop="kb_id"
          :label="$t('知识库')"
          width="90"
        />
        <el-table-column
          :label="$t('类型')"
          width="100"
        >
          <template #default="{ row }">
            <el-tag :type="row.source_type === 'sitemap' ? 'warning' : 'primary'">
              {{ row.source_type === 'sitemap' ? 'Sitemap' : 'URL' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column
          prop="start_url"
          :label="$t('起始地址')"
          min-width="220"
          show-overflow-tooltip
        />
        <el-table-column
          :label="$t('站点验证')"
          width="110"
        >
          <template #default="{ row }">
            <el-tag :type="row.site_verified ? 'success' : 'danger'">
              {{ row.site_verified ? $t('已验证') : $t('未验证') }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column
          :label="$t('状态')"
          width="100"
        >
          <template #default="{ row }">
            <el-tag :type="statusTag(row.status)">
              {{ statusLabel(row.status) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column
          :label="$t('同步周期')"
          width="100"
        >
          <template #default="{ row }">
            {{ row.interval_hours }}h
          </template>
        </el-table-column>
        <el-table-column
          :label="$t('上次同步')"
          width="160"
        >
          <template #default="{ row }">
            {{ formatDate(row.last_crawl_at) }}
          </template>
        </el-table-column>
        <el-table-column
          :label="$t('操作')"
          width="300"
          fixed="right"
        >
          <template #default="{ row }">
            <el-button
              size="small"
              type="primary"
              :disabled="!row.site_verified"
              @click="handleSync(row)"
            >
              {{ $t('立即同步') }}
            </el-button>
            <el-button
              size="small"
              @click="handleVerify(row)"
            >
              {{ $t('验证站点') }}
            </el-button>
            <el-button
              size="small"
              @click="openBatches(row)"
            >
              {{ $t('批次') }}
            </el-button>
            <el-button
              v-if="row.status === 'active'"
              size="small"
              type="warning"
              @click="handlePause(row)"
            >
              {{ $t('暂停') }}
            </el-button>
            <el-button
              v-else
              size="small"
              type="success"
              @click="handleResume(row)"
            >
              {{ $t('恢复') }}
            </el-button>
            <el-button
              size="small"
              type="danger"
              @click="handleDelete(row)"
            >
              {{ $t('删除') }}
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 新建源 -->
    <el-dialog
      v-model="showCreate"
      :title="$t('新建爬取源')"
      width="560px"
    >
      <el-form
        :model="form"
        label-width="110px"
      >
        <el-form-item :label="$t('知识库')">
          <el-select
            v-model="form.kb_id"
            filterable
            style="width: 100%"
          >
            <el-option
              v-for="kb in kbList"
              :key="kb.id"
              :label="kb.name"
              :value="kb.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item :label="$t('来源类型')">
          <el-radio-group v-model="form.source_type">
            <el-radio value="url">
              URL
            </el-radio>
            <el-radio value="sitemap">
              Sitemap
            </el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item :label="$t('起始地址')">
          <el-input
            v-model="form.start_url"
            :placeholder="form.source_type === 'sitemap' ? 'https://example.com/sitemap.xml' : 'https://example.com/docs'"
          />
        </el-form-item>
        <el-form-item :label="$t('URL 前缀白名单')">
          <el-input
            v-model="form.include_patterns"
            :placeholder="$t('逗号分隔，如 https://example.com/docs/,https://example.com/faq/')"
          />
        </el-form-item>
        <el-form-item :label="$t('单次页数上限')">
          <el-input-number
            v-model="form.max_pages"
            :min="10"
            :max="2000"
          />
        </el-form-item>
        <el-form-item :label="$t('同步周期(小时)')">
          <el-input-number
            v-model="form.interval_hours"
            :min="1"
            :max="720"
          />
        </el-form-item>
        <el-form-item :label="$t('QA 自动拆分')">
          <el-switch v-model="form.qa_split_enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showCreate = false">
          {{ $t('取消') }}
        </el-button>
        <el-button
          type="primary"
          :loading="creating"
          @click="handleCreate"
        >
          {{ $t('创建') }}
        </el-button>
      </template>
    </el-dialog>

    <!-- 批次抽屉 -->
    <el-drawer
      v-model="batchDrawer"
      :title="$t('批次历史') + (currentSource ? ' #' + currentSource.id : '')"
      size="640px"
    >
      <el-table
        v-loading="batchLoading"
        :data="batches"
        stripe
      >
        <el-table-column
          prop="id"
          label="ID"
          width="70"
        />
        <el-table-column
          prop="trigger"
          :label="$t('触发')"
          width="80"
        />
        <el-table-column
          :label="$t('状态')"
          width="110"
        >
          <template #default="{ row }">
            <el-tag :type="batchTag(row.status)">
              {{ batchLabel(row.status) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column
          :label="$t('发现/新增/变更/未变/失败')"
          width="190"
        >
          <template #default="{ row }">
            {{ row.pages_found }} / {{ row.pages_new }} / {{ row.pages_changed }} / {{ row.pages_unchanged }} / {{ row.pages_failed }}
          </template>
        </el-table-column>
        <el-table-column
          prop="error_msg"
          :label="$t('错误')"
          min-width="100"
          show-overflow-tooltip
        />
        <el-table-column
          :label="$t('操作')"
          width="90"
        >
          <template #default="{ row }">
            <el-button
              v-if="row.status === 'failed'"
              size="small"
              type="primary"
              :disabled="row.retry_count >= 3"
              @click="handleRetry(row)"
            >
              {{ $t('重试') }}
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-drawer>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { kbCrawlerApi } from '@/api/kbCrawler'
import { listKBs } from '@/api/knowledgeBase'

const loading = ref(false)
const creating = ref(false)
const sources = ref([])
const kbList = ref([])
const filter = reactive({ kb_id: '', status: '' })
const showCreate = ref(false)
const form = reactive({
  kb_id: '',
  source_type: 'url',
  start_url: '',
  include_patterns: '',
  max_pages: 200,
  interval_hours: 168,
  qa_split_enabled: true
})

// 批次抽屉
const batchDrawer = ref(false)
const batchLoading = ref(false)
const batches = ref([])
const currentSource = ref(null)

const loadKBs = async () => {
  const res = await listKBs({ page: 1, page_size: 100 })
  kbList.value = res?.items || res || []
}

const loadSources = async () => {
  loading.value = true
  try {
    const params = {}
    if (filter.kb_id) params.kb_id = filter.kb_id
    if (filter.status) params.status = filter.status
    const res = await kbCrawlerApi.listSources(params)
    sources.value = res?.items || res || []
  } catch (e) {
    ElMessage.error($t('加载爬取源失败') + ': ' + (e.message || ''))
  } finally {
    loading.value = false
  }
}

const handleCreate = async () => {
  if (!form.kb_id || !form.start_url) {
    ElMessage.warning($t('请选择知识库并填写起始地址'))
    return
  }
  creating.value = true
  try {
    const data = { ...form }
    if (!data.include_patterns) delete data.include_patterns
    await kbCrawlerApi.createSource(data)
    ElMessage.success($t('已创建，请先完成站点验证后同步'))
    showCreate.value = false
    await loadSources()
  } catch (e) {
    ElMessage.error($t('创建失败') + ': ' + (e.message || ''))
  } finally {
    creating.value = false
  }
}

const handleVerify = async (row) => {
  try {
    await kbCrawlerApi.verifySite(row.id)
    ElMessage.success($t('站点验证通过'))
  } catch (e) {
    ElMessage.error($t('验证未通过') + ': ' + (e.message || ''))
  }
  await loadSources()
}

const handleSync = async (row) => {
  try {
    const res = await kbCrawlerApi.syncSource(row.id)
    ElMessage.success($t('已创建同步批次') + `: ${res?.batch_no || ''}`)
  } catch (e) {
    ElMessage.error($t('触发同步失败') + ': ' + (e.message || ''))
  }
}

const handlePause = async (row) => {
  await kbCrawlerApi.pauseSource(row.id)
  await loadSources()
}

const handleResume = async (row) => {
  await kbCrawlerApi.resumeSource(row.id)
  await loadSources()
}

const handleDelete = (row) => {
  ElMessageBox.confirm($t('删除后不再同步该源，已入库文档保留。确认删除？'), $t('确认'), { type: 'warning' })
    .then(async () => {
      await kbCrawlerApi.deleteSource(row.id)
      await loadSources()
    })
    .catch(() => {})
}

const openBatches = async (row) => {
  currentSource.value = row
  batchDrawer.value = true
  batchLoading.value = true
  try {
    const res = await kbCrawlerApi.listBatches({ source_id: row.id, page: 1, page_size: 50 })
    batches.value = res?.items || res || []
  } finally {
    batchLoading.value = false
  }
}

const handleRetry = async (row) => {
  try {
    await kbCrawlerApi.retryBatch(row.id)
    ElMessage.success($t('已创建重试批次'))
    if (currentSource.value) await openBatches(currentSource.value)
  } catch (e) {
    ElMessage.error($t('重试失败') + ': ' + (e.message || ''))
  }
}

const statusTag = (s) => ({ active: 'success', paused: 'info', error: 'danger' }[s] || 'info')
const statusLabel = (s) => ({ active: '启用', paused: '已暂停', error: '异常' }[s] || s)
const batchTag = (s) => ({ pending: 'info', running: 'warning', succeeded: 'success', partially_failed: 'warning', failed: 'danger' }[s] || 'info')
const batchLabel = (s) => ({ pending: '待执行', running: '执行中', succeeded: '成功', partially_failed: '部分失败', failed: '失败' }[s] || s)
const formatDate = (d) => d ? new Date(d).toLocaleString('zh-CN') : '-'

onMounted(async () => {
  await loadKBs()
  await loadSources()
})
</script>

<style scoped lang="scss">
.crawler-sources-page {
  padding: 16px;
}

.toolbar-card {
  margin-bottom: 16px;
}

.toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}

.toolbar-left,
.toolbar-right {
  display: flex;
  gap: 8px;
  align-items: center;
}
</style>
