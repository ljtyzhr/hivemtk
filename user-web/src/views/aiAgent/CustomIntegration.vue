<template>
  <div class="custom-integration">
    <el-card shadow="never">
      <template #header>
        <div class="card-header">
          <span>{{ t('customIntegration.title') }}</span>
        </div>
      </template>

      <el-tabs v-model="activeTab">
        <!-- OpenAPI 工具集（P1-2） -->
        <el-tab-pane
          :label="t('customIntegration.openapiTab')"
          name="openapi"
        >
          <el-alert
            class="mb-12"
            type="info"
            show-icon
            :closable="false"
            :title="t('customIntegration.importHint')"
          />
          <el-form
            :model="openapiForm"
            label-width="120px"
            class="import-form"
          >
            <el-form-item :label="t('customIntegration.name')">
              <el-input
                v-model="openapiForm.name"
                placeholder="erp-tools"
              />
            </el-form-item>
            <el-form-item :label="t('customIntegration.baseUrl')">
              <el-input
                v-model="openapiForm.base_url"
                placeholder="https://erp.example.com"
              />
            </el-form-item>
            <el-form-item :label="t('customIntegration.authHeader')">
              <el-input
                v-model="openapiForm.auth_header"
                placeholder="Authorization"
              />
            </el-form-item>
            <el-form-item :label="t('customIntegration.authValue')">
              <el-input
                v-model="openapiForm.auth_value"
                type="password"
                show-password
                placeholder="Bearer xxx"
              />
            </el-form-item>
            <el-form-item :label="t('customIntegration.schemaJson')">
              <el-input
                v-model="openapiForm.schema_json"
                type="textarea"
                :rows="8"
                placeholder="{&quot;openapi&quot;:&quot;3.0.0&quot;, ...}"
              />
            </el-form-item>
            <el-form-item>
              <el-button
                type="primary"
                :loading="importing"
                @click="doImport"
              >
                {{ t('customIntegration.import') }}
              </el-button>
            </el-form-item>
          </el-form>

          <el-table
            v-loading="loadingTools"
            :data="tools"
            stripe
          >
            <el-table-column
              prop="name"
              :label="t('customIntegration.name')"
              min-width="140"
            />
            <el-table-column
              prop="base_url"
              :label="t('customIntegration.baseUrl')"
              min-width="220"
            />
            <el-table-column
              prop="tool_count"
              :label="t('customIntegration.tools')"
              width="100"
            />
            <el-table-column
              :label="t('customIntegration.delete')"
              width="100"
            >
              <template #default="{ row }">
                <el-button
                  type="danger"
                  link
                  @click="removeTool(row)"
                >
                  {{ t('customIntegration.delete') }}
                </el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>

        <!-- 外部 MCP 服务（P1-3） -->
        <el-tab-pane
          :label="t('customIntegration.mcpTab')"
          name="mcp"
        >
          <el-alert
            class="mb-12"
            type="info"
            show-icon
            :closable="false"
            :title="t('customIntegration.mcpHint')"
          />
          <el-form
            :model="mcpForm"
            label-width="120px"
            class="import-form"
          >
            <el-form-item :label="t('customIntegration.name')">
              <el-input
                v-model="mcpForm.name"
                placeholder="filesystem"
              />
            </el-form-item>
            <el-form-item :label="t('customIntegration.endpoint')">
              <el-input
                v-model="mcpForm.endpoint"
                placeholder="https://mcp.example.com/rpc"
              />
            </el-form-item>
            <el-form-item :label="t('customIntegration.apiKey')">
              <el-input
                v-model="mcpForm.api_key"
                type="password"
                show-password
              />
            </el-form-item>
            <el-form-item>
              <el-button
                type="primary"
                :loading="creating"
                @click="doCreateMcp"
              >
                {{ t('customIntegration.save') }}
              </el-button>
            </el-form-item>
          </el-form>

          <el-table
            v-loading="loadingMcp"
            :data="mcpServers"
            stripe
          >
            <el-table-column
              prop="name"
              :label="t('customIntegration.name')"
              min-width="140"
            />
            <el-table-column
              prop="endpoint"
              :label="t('customIntegration.endpoint')"
              min-width="220"
            />
            <el-table-column
              prop="tool_count"
              :label="t('customIntegration.tools')"
              width="100"
            />
            <el-table-column
              prop="last_status"
              :label="t('customIntegration.status')"
              width="110"
            />
            <el-table-column
              prop="last_error"
              :label="t('customIntegration.lastError')"
              min-width="200"
              show-overflow-tooltip
            />
            <el-table-column
              :label="t('customIntegration.enabled')"
              width="110"
            >
              <template #default="{ row }">
                <el-switch
                  :model-value="row.enabled"
                  @change="(v) => toggleMcp(row, v)"
                />
              </template>
            </el-table-column>
            <el-table-column width="200">
              <template #default="{ row }">
                <el-button
                  link
                  type="primary"
                  @click="testMcp(row)"
                >
                  {{ t('customIntegration.test') }}
                </el-button>
                <el-button
                  link
                  type="danger"
                  @click="removeMcp(row)"
                >
                  {{ t('customIntegration.delete') }}
                </el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>
      </el-tabs>
    </el-card>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  listCustomTools,
  importCustomTool,
  deleteCustomTool,
  listMcpServers,
  createMcpServer,
  deleteMcpServer,
  setMcpServerEnabled,
  testMcpServer
} from '@/api/customIntegration'

const { t } = useI18n()
const activeTab = ref('openapi')

const tools = ref([])
const loadingTools = ref(false)
const importing = ref(false)
const openapiForm = reactive({ name: '', base_url: '', auth_header: '', auth_value: '', schema_json: '' })

const mcpServers = ref([])
const loadingMcp = ref(false)
const creating = ref(false)
const mcpForm = reactive({ name: '', endpoint: '', api_key: '' })

async function loadTools() {
  loadingTools.value = true
  try {
    const res = await listCustomTools()
    tools.value = res?.data ?? res ?? []
  } catch (e) {
    ElMessage.error(e.message)
  } finally {
    loadingTools.value = false
  }
}

async function doImport() {
  if (!openapiForm.name || !openapiForm.base_url || !openapiForm.schema_json) {
    ElMessage.warning(t('customIntegration.required'))
    return
  }
  importing.value = true
  try {
    await importCustomTool({ ...openapiForm })
    ElMessage.success(t('customIntegration.saved'))
    Object.assign(openapiForm, { name: '', base_url: '', auth_header: '', auth_value: '', schema_json: '' })
    await loadTools()
  } catch (e) {
    ElMessage.error(e.message)
  } finally {
    importing.value = false
  }
}

async function removeTool(row) {
  try {
    await ElMessageBox.confirm(t('customIntegration.confirmDelete'), t('customIntegration.delete'))
  } catch {
    return
  }
  try {
    await deleteCustomTool(row.id)
    ElMessage.success(t('customIntegration.deleted'))
    await loadTools()
  } catch (e) {
    ElMessage.error(e.message)
  }
}

async function loadMcp() {
  loadingMcp.value = true
  try {
    const res = await listMcpServers()
    mcpServers.value = res?.data ?? res ?? []
  } catch (e) {
    ElMessage.error(e.message)
  } finally {
    loadingMcp.value = false
  }
}

async function doCreateMcp() {
  if (!mcpForm.name || !mcpForm.endpoint) {
    ElMessage.warning(t('customIntegration.required'))
    return
  }
  creating.value = true
  try {
    await createMcpServer({ ...mcpForm })
    ElMessage.success(t('customIntegration.saved'))
    Object.assign(mcpForm, { name: '', endpoint: '', api_key: '' })
    await loadMcp()
  } catch (e) {
    ElMessage.error(e.message)
  } finally {
    creating.value = false
  }
}

async function toggleMcp(row, enabled) {
  try {
    await setMcpServerEnabled(row.id, enabled)
    await loadMcp()
  } catch (e) {
    ElMessage.error(e.message)
  }
}

async function testMcp(row) {
  try {
    const res = await testMcpServer(row.id)
    const count = res?.tool_count ?? 0
    ElMessage.success(t('customIntegration.testOk') + ' (' + count + ')')
    await loadMcp()
  } catch (e) {
    ElMessage.error(e.message)
  }
}

async function removeMcp(row) {
  try {
    await ElMessageBox.confirm(t('customIntegration.confirmDelete'), t('customIntegration.delete'))
  } catch {
    return
  }
  try {
    await deleteMcpServer(row.id)
    ElMessage.success(t('customIntegration.deleted'))
    await loadMcp()
  } catch (e) {
    ElMessage.error(e.message)
  }
}

onMounted(() => {
  loadTools()
  loadMcp()
})
</script>

<style scoped>
.custom-integration {
  padding: 16px;
}
.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.import-form {
  max-width: 760px;
  margin-bottom: 16px;
}
.mb-12 {
  margin-bottom: 12px;
}
</style>
