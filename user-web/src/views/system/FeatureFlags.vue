<template>
  <div class="feature-flags-page">
    <el-alert
      type="info"
      :closable="false"
      show-icon
      class="mb-16"
    >
      <template #title>
        功能开关 = 这一整块功能对谁开
      </template>
      <div class="hint">
        「阈值拧到几」去 <el-link
          type="primary"
          @click="$router.push('/system/config-params')"
        >
          动态阈值参数
        </el-link>；
        「这块功能要不要开、对多少人开」在这里。开关走 Unleash 风格的 FNV-1a 分桶，
        灰度百分比按 <code>hash(key + context_id) % 100 &lt; 百分比</code> 判定——同一个人反复评估结果稳定，
        但百分比一改，命中的人群集合就整体换一批。
      </div>
    </el-alert>

    <el-card
      shadow="never"
      class="mb-16"
    >
      <div class="toolbar">
        <el-input
          v-model="keyword"
          placeholder="搜 key / 名称 / 说明 / 标签"
          clearable
          style="width: 280px"
        />
        <el-select
          v-model="stateFilter"
          style="width: 150px"
        >
          <el-option
            label="全部状态"
            value=""
          />
          <el-option
            label="已启用"
            value="on"
          />
          <el-option
            label="已停用"
            value="off"
          />
          <el-option
            label="灰度中"
            value="gray"
          />
          <el-option
            label="疑似闲置"
            value="stale"
          />
        </el-select>
        <div class="spacer" />
        <el-button
          :loading="loading"
          @click="load"
        >
          刷新
        </el-button>
        <el-button
          type="primary"
          @click="openCreate"
        >
          新建开关
        </el-button>
      </div>

      <el-table
        v-loading="loading"
        :data="filtered"
        row-key="id"
        empty-text="还没有任何功能开关"
      >
        <el-table-column
          label="开关"
          min-width="240"
        >
          <template #default="{ row }">
            <div class="flag-key">
              <code>{{ row.key }}</code>
              <el-tag
                v-if="row.tags"
                size="small"
                type="info"
                effect="plain"
              >
                {{ row.tags }}
              </el-tag>
            </div>
            <div
              v-if="row.name"
              class="flag-name"
            >
              {{ row.name }}
            </div>
            <div
              v-if="row.description"
              class="flag-desc"
            >
              {{ row.description }}
            </div>
          </template>
        </el-table-column>

        <el-table-column
          label="状态"
          width="130"
        >
          <template #default="{ row }">
            <el-switch
              :model-value="row.enabled"
              active-text="开"
              @change="(v) => onToggle(row, v)"
            />
            <el-tag
              v-if="staleKeys.has(row.key)"
              size="small"
              type="warning"
              class="ml-4"
            >
              疑似闲置
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column
          label="灰度"
          width="210"
        >
          <template #default="{ row }">
            <el-slider
              :model-value="row.rollout_percentage"
              :min="0"
              :max="100"
              :step="5"
              :disabled="!row.enabled"
              @change="(v) => onRollout(row, v)"
            />
            <div class="rollout-text">
              {{ row.enabled ? rolloutText(row) : '已停用，不分桶' }}
            </div>
          </template>
        </el-table-column>

        <el-table-column
          label="最近被评估"
          width="160"
        >
          <template #default="{ row }">
            <span v-if="row.last_evaluated_at">{{ formatTs(row.last_evaluated_at) }}</span>
            <el-tooltip
              v-else
              content="从来没有被任何请求评估过——要么代码里没人读这个 key，要么流量还没走到它"
              placement="top"
            >
              <span class="never">从未</span>
            </el-tooltip>
          </template>
        </el-table-column>

        <el-table-column
          label="操作"
          width="180"
          fixed="right"
        >
          <template #default="{ row }">
            <el-button
              link
              type="primary"
              size="small"
              @click="openEdit(row)"
            >
              编辑
            </el-button>
            <el-button
              link
              type="primary"
              size="small"
              @click="openLogs(row)"
            >
              日志
            </el-button>
            <el-button
              link
              type="danger"
              size="small"
              @click="onDelete(row)"
            >
              删除
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 新建 / 编辑 -->
    <el-dialog
      v-model="dialogVisible"
      :title="form.id ? '编辑开关' : '新建开关'"
      width="560px"
    >
      <el-form
        :model="form"
        label-width="96px"
      >
        <el-form-item label="Key">
          <el-input
            v-model="form.key"
            :disabled="!!form.id"
            placeholder="稳定标识，代码里 Evaluate 用的就是它，一经创建不可改"
          />
          <div class="field-hint">
            建议用点分命名（<code>module.feature</code>）。改了 key 等于换了一个新开关——
            老开关的灰度人群与审计全部对不上，所以这里禁用编辑。
          </div>
        </el-form-item>
        <el-form-item label="名称">
          <el-input
            v-model="form.name"
            placeholder="给人看的名字，比如「智能体拟人化回复」"
          />
        </el-form-item>
        <el-form-item label="说明">
          <el-input
            v-model="form.description"
            type="textarea"
            :rows="3"
            placeholder="这个开关控制什么、关掉会有什么影响"
          />
        </el-form-item>
        <el-form-item label="标签">
          <el-input
            v-model="form.tags"
            placeholder="逗号分隔，用于筛选，比如 llm,beta"
          />
        </el-form-item>
        <el-form-item label="初始状态">
          <el-switch
            v-model="form.enabled"
            active-text="启用"
          />
          <span class="field-hint inline">
            新建时默认停用更安全：先建后灰度，而不是建完就对 100% 用户生效。
          </span>
        </el-form-item>
        <el-form-item label="灰度百分比">
          <el-slider
            v-model="form.rollout_percentage"
            :min="0"
            :max="100"
            :step="5"
            show-input
          />
        </el-form-item>
        <el-form-item label="Payload">
          <el-input
            v-model="form.payload"
            type="textarea"
            :rows="3"
            placeholder="可选。随开关一起下发的配置内容，一般是 JSON"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">
          取消
        </el-button>
        <el-button
          type="primary"
          :loading="saving"
          :disabled="!!formError"
          @click="submit"
        >
          {{ form.id ? '保存' : '创建' }}
        </el-button>
      </template>
      <el-alert
        v-if="formError"
        type="error"
        :closable="false"
        :title="formError"
        class="mt-8"
      />
    </el-dialog>

    <!-- 变更审计 + 评估日志 -->
    <el-drawer
      v-model="logVisible"
      :title="logFlag ? `${logFlag.key} 的日志` : '日志'"
      size="560px"
    >
      <el-tabs v-model="logTab">
        <el-tab-pane
          label="变更审计"
          name="audit"
        >
          <el-alert
            type="warning"
            :closable="false"
            class="mb-12"
            title="开关的每次启用/禁用/调灰度都会留一条审计——出问题时这是唯一能回答「当时到底开没开」的记录"
          />
          <el-timeline v-if="auditRows.length">
            <el-timeline-item
              v-for="a in auditRows"
              :key="a.id"
              :timestamp="formatTs(a.created_at)"
            >
              <b>{{ actionText(a.action) }}</b>
              <div class="log-detail">
                {{ a.detail }}
              </div>
            </el-timeline-item>
          </el-timeline>
          <el-empty
            v-else
            description="没有变更记录"
          />
        </el-tab-pane>
        <el-tab-pane
          label="评估记录"
          name="eval"
        >
          <el-alert
            type="info"
            :closable="false"
            class="mb-12"
            title="每条记录是一次 Evaluate 调用。reason=not_found 说明代码里传的 key 在库里不存在"
          />
          <el-table
            v-if="evalRows.length"
            :data="evalRows"
            size="small"
            max-height="520"
          >
            <el-table-column
              prop="context_id"
              label="评估对象"
              min-width="140"
            />
            <el-table-column
              label="结果"
              width="80"
            >
              <template #default="{ row }">
                <el-tag
                  :type="row.enabled ? 'success' : 'info'"
                  size="small"
                >
                  {{ row.enabled ? '命中' : '未命中' }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column
              prop="reason"
              label="原因"
              width="150"
            />
            <el-table-column
              label="时间"
              width="150"
            >
              <template #default="{ row }">
                {{ formatTs(row.created_at) }}
              </template>
            </el-table-column>
          </el-table>
          <el-empty
            v-else
            description="没有评估记录：这个开关还没被任何请求读到过"
          />
        </el-tab-pane>
      </el-tabs>
    </el-drawer>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  list, stale, create, update, remove, enable, disable, setRollout, auditLogs, evalLogs
} from '@/api/featureFlags'

const loading = ref(false)
const saving = ref(false)
const rows = ref([])
const staleKeys = ref(new Set())
const keyword = ref('')
const stateFilter = ref('')

const dialogVisible = ref(false)
const form = reactive({ id: null, key: '', name: '', description: '', tags: '', enabled: false, rollout_percentage: 0, payload: '' })
const formError = ref('')

const logVisible = ref(false)
const logTab = ref('audit')
const logFlag = ref(null)
const auditRows = ref([])
const evalRows = ref([])

// 灰度百分比 <100 且 >0 时才叫「灰度中」；=0 或 =100 都是确定的（全不放 / 全放）。
const filtered = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  return rows.value.filter((r) => {
    if (staleKeys.value.has(r.key) && stateFilter.value === 'stale') return true
    if (stateFilter.value === 'stale') return false
    if (stateFilter.value === 'on' && !r.enabled) return false
    if (stateFilter.value === 'off' && r.enabled) return false
    if (stateFilter.value === 'gray') {
      const p = r.rollout_percentage
      if (!r.enabled || p <= 0 || p >= 100) return false
    }
    if (!kw) return true
    return [r.key, r.name, r.description, r.tags].some((v) => String(v || '').toLowerCase().includes(kw))
  })
})

const rolloutText = (row) => {
  const p = row.rollout_percentage
  if (p >= 100) return '全量（100%）'
  if (p <= 0) return '0%：对所有人关闭'
  return `${p}% 用户命中`
}

const actionText = (a) => ({
  create: '创建', update: '更新', enable: '启用', disable: '禁用', rollout: '调整灰度', delete: '删除'
}[a] || a)

const formatTs = (v) => (v ? String(v).replace('T', ' ').slice(0, 19) : '-')

async function load() {
  loading.value = true
  try {
    const [listRes, staleRes] = await Promise.all([list({ page: 1, page_size: 200 }), stale()])
    rows.value = listRes?.list || []
    staleKeys.value = new Set((staleRes?.list || []).map((f) => f.key))
  } catch (e) {
    ElMessage.error('加载功能开关失败：' + (e?.message || e))
  } finally {
    loading.value = false
  }
}

// 开关是「运行时立刻生效」的东西，所以停用和删除都要二次确认：
// 一个手滑的开关可能让整块功能当场从线上消失。
async function onToggle(row, val) {
  try {
    await ElMessageBox.confirm(
      val
        ? `启用「${row.key}」后，命中灰度的用户会立刻开始看到它。确定启用？`
        : `停用「${row.key}」后，正在命中灰度的用户会立刻看不到它。确定停用？`,
      val ? '启用功能开关' : '停用功能开关',
      { type: val ? 'warning' : 'error' }
    )
  } catch { return } // 用户取消
  try {
    if (val) await enable(row.id)
    else await disable(row.id)
    ElMessage.success(val ? '已启用' : '已停用')
    await load()
  } catch (e) {
    ElMessage.error('切换失败：' + (e?.message || e))
    await load() // 回读服务端真实状态，别让开关留在一个本地认为成立、实际没改成的态
  }
}

async function onRollout(row, val) {
  try {
    await setRollout(row.id, val)
    ElMessage.success(`灰度已调到 ${val}%`)
    await load()
  } catch (e) {
    ElMessage.error('调整灰度失败：' + (e?.message || e))
    await load()
  }
}

function openCreate() {
  Object.assign(form, { id: null, key: '', name: '', description: '', tags: '', enabled: false, rollout_percentage: 0, payload: '' })
  formError.value = ''
  dialogVisible.value = true
}

function openEdit(row) {
  Object.assign(form, {
    id: row.id, key: row.key, name: row.name || '', description: row.description || '',
    tags: row.tags || '', enabled: row.enabled, rollout_percentage: row.rollout_percentage ?? 100, payload: row.payload || ''
  })
  formError.value = ''
  dialogVisible.value = true
}

async function submit() {
  if (!form.key.trim()) { formError.value = 'Key 必填——代码里 Evaluate 用的就是它'; return }
  if (form.payload && !looksLikeJson(form.payload)) { formError.value = 'Payload 看着像 JSON 但解析不过，检查引号与逗号'; return }
  formError.value = ''
  saving.value = true
  try {
    if (form.id) {
      // Update 的请求体里没有 enabled 字段（开关状态只走 enable/disable 端点，
      // 那样才会单独留一条「谁把开关关了」的审计），所以这里不传 enabled。
      await update(form.id, {
        name: form.name, description: form.description,
        rollout_percentage: Number(form.rollout_percentage), payload: form.payload, tags: form.tags
      })
      if (form.enabled && !rows.value.find((r) => r.id === form.id)?.enabled) await enable(form.id)
      if (!form.enabled && rows.value.find((r) => r.id === form.id)?.enabled) await disable(form.id)
    } else {
      await create({
        key: form.key.trim(), name: form.name, description: form.description,
        enabled: form.enabled, rollout_percentage: Number(form.rollout_percentage),
        payload: form.payload, tags: form.tags
      })
    }
    ElMessage.success(form.id ? '已保存' : '已创建')
    dialogVisible.value = false
    await load()
  } catch (e) {
    formError.value = e?.message || String(e)
  } finally {
    saving.value = false
  }
}

const looksLikeJson = (s) => {
  const t = String(s).trim()
  if (!t.startsWith('{') && !t.startsWith('[')) return true // 不是 JSON 就当普通文本
  try { JSON.parse(t); return true } catch { return false }
}

async function onDelete(row) {
  try {
    await ElMessageBox.confirm(
      `删除「${row.key}」会连同它的审计与评估记录一起消失，且不可恢复。` +
      `如果只是想让它别生效，用「停用」就够了——那样随时能开回来。确定删除？`,
      '删除功能开关',
      { type: 'error', confirmButtonText: '仍然删除' }
    )
  } catch { return }
  try {
    await remove(row.id)
    ElMessage.success('已删除')
    await load()
  } catch (e) {
    ElMessage.error('删除失败：' + (e?.message || e))
  }
}

async function openLogs(row) {
  logFlag.value = row
  logTab.value = 'audit'
  auditRows.value = []
  evalRows.value = []
  logVisible.value = true
  try {
    const [a, e] = await Promise.all([auditLogs(row.id), evalLogs(row.id)])
    auditRows.value = a?.list || []
    evalRows.value = e?.list || []
  } catch (err) {
    ElMessage.error('加载日志失败：' + (err?.message || err))
  }
}

onMounted(load)
</script>

<style scoped>
.mb-16 { margin-bottom: 16px; }
.mb-12 { margin-bottom: 12px; }
.mt-8 { margin-top: 8px; }
.ml-4 { margin-left: 4px; }
.toolbar { display: flex; gap: 12px; align-items: center; margin-bottom: 14px; }
.spacer { flex: 1; }
.hint { font-size: 13px; line-height: 1.7; }
.field-hint { font-size: 12px; color: var(--el-text-color-secondary); line-height: 1.6; margin-top: 4px; }
.field-hint.inline { margin-left: 12px; }
.flag-key { display: flex; align-items: center; gap: 8px; }
.flag-name { font-size: 13px; margin-top: 2px; }
.flag-desc { font-size: 12px; color: var(--el-text-color-secondary); margin-top: 2px; line-height: 1.5; }
.rollout-text { font-size: 12px; color: var(--el-text-color-secondary); margin-top: -6px; }
.never { color: var(--el-text-color-secondary); border-bottom: 1px dotted var(--el-text-color-secondary); cursor: help; }
.log-detail { font-size: 12px; color: var(--el-text-color-secondary); margin-top: 2px; }
code { font-family: ui-monospace, Menlo, Consolas, monospace; font-size: 12px; }
</style>
