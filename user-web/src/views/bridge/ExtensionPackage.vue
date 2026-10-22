<template>
  <div class="p-4">
    <el-card>
      <template #header>
        <div class="flex items-center justify-between">
          <span>桥接扩展安装包</span>
          <el-button
            size="small"
            :loading="loading"
            @click="load"
          >
            刷新
          </el-button>
        </div>
      </template>

      <el-alert
        type="info"
        :closable="false"
        show-icon
        class="mb-3"
        title="扩展不上架 Chrome Web Store：由平台运营上传安装包，租户在此下载后自行安装。"
      />

      <el-alert
        v-if="error"
        type="warning"
        :closable="false"
        show-icon
        class="mb-3"
        :title="error"
      />

      <template v-if="pkg">
        <el-descriptions
          :column="1"
          border
          class="mb-3"
        >
          <el-descriptions-item label="扩展名称">
            {{ pkg.name || 'HiveBridge 蜂桥' }}
          </el-descriptions-item>
          <el-descriptions-item label="版本">
            {{ pkg.version || '-' }}
          </el-descriptions-item>
          <el-descriptions-item label="文件大小">
            {{ formatSize(pkg.size_bytes) }}
          </el-descriptions-item>
          <el-descriptions-item label="SHA256">
            <span class="font-mono text-xs break-all">{{ pkg.file_sha256 || '-' }}</span>
          </el-descriptions-item>
        </el-descriptions>

        <el-button
          type="primary"
          :loading="downloading"
          @click="download"
        >
          下载安装包
        </el-button>
      </template>

      <el-empty
        v-else-if="!loading && !error"
        description="暂无安装包信息"
      />
    </el-card>

    <el-card class="mt-4">
      <template #header>
        <span>安装步骤</span>
      </template>
      <ol class="text-sm leading-7 text-gray-600 pl-4">
        <li>下载上方安装包（.zip），解压到任意目录（解压后根目录应直接是 manifest.json）。</li>
        <li>打开 Chrome，地址栏输入 <code>chrome://extensions</code>。</li>
        <li>右上角开启「开发者模式」。</li>
        <li>点「加载已解压的扩展程序」，选择刚才解压出来的目录。</li>
        <li>点扩展图标，填写本系统的服务端地址与「桥接凭证」，并填入租户短码后保存。</li>
      </ol>
      <el-alert
        type="warning"
        :closable="false"
        show-icon
        title="租户短码必填"
        description="SaaS 多租户部署下，服务端靠租户短码（或账号行自证）确定私信归属；首次上报缺短码会被拒绝。"
      />
    </el-card>
  </div>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { getBridgeExtensionPackage } from '@/api/bridgeExtension.js'

const pkg = ref(null)
const loading = ref(false)
const downloading = ref(false)
const error = ref('')

const load = async () => {
  loading.value = true
  error.value = ''
  try {
    pkg.value = await getBridgeExtensionPackage() || null
    if (!pkg.value) error.value = '当前部署未接入平台，或平台尚未上架桥接扩展包'
  } catch (e) {
    pkg.value = null
    error.value = (e && (e.message || e.msg)) || '获取安装包信息失败'
  } finally {
    loading.value = false
  }
}

const download = async () => {
  if (!pkg.value || !pkg.value.download_url) return
  downloading.value = true
  try {
    // 直接开新窗口下载：安装包是平台托管的静态文件，不需要带本系统的鉴权。
    // 用 <a download> 而不是 fetch —— 后者要处理二进制流与对象 URL 的释放，
    // 而这里只是把浏览器现成的下载能力接上。
    const a = document.createElement('a')
    a.href = pkg.value.download_url
    a.download = pkg.value.filename || 'hivemtk-bridge.zip'
    a.rel = 'noopener'
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    ElMessage.success('已开始下载，下载后解压安装')
  } catch (e) {
    ElMessage.error('下载失败：' + ((e && e.message) || e))
  } finally {
    downloading.value = false
  }
}

const formatSize = (bytes) => {
  const n = Number(bytes)
  if (!Number.isFinite(n) || n <= 0) return '-'
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / 1024 / 1024).toFixed(2)} MB`
}

onMounted(load)
</script>
