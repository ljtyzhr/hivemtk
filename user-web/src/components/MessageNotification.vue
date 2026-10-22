<template>
  <div
    v-if="hasNewMessage && !dismissed"
    class="message-notification"
  >
    <div class="message-content">
      <div class="message-header">
        <el-icon><Bell /></el-icon>
        <span class="message-title">{{ currentMessage.title }}</span>
      </div>
      <div class="message-body">
        <p>{{ currentMessage.content }}</p>
      </div>
      <div class="message-actions">
        <el-button
          type="primary"
          size="small"
          @click="markAsRead"
        >
          {{ $t('知道了') }}
        </el-button>
        <el-button
          type="text"
          size="small"
          @click="dismiss"
        >
          {{ $t('关闭') }}
        </el-button>
      </div>
    </div>
  </div>
</template>

<script setup>
import i18n from '@/i18n'

import { ref, onMounted, onUnmounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Bell } from '@element-plus/icons-vue'
import { notificationApi } from '@/api/notification'

const hasNewMessage = ref(false)
const dismissed = ref(false)
const currentMessage = ref({
  id: '',
  title: '',
  content: ''
})

const DISMISSED_MESSAGES_KEY = 'dismissed_messages'

let checkInterval = null

const getDismissedMessages = () => {
  const dismissed = localStorage.getItem(DISMISSED_MESSAGES_KEY)
  return dismissed ? JSON.parse(dismissed) : []
};

const addToDismissedMessages = (messageId) => {
  const dismissedMessages = getDismissedMessages()
  if (!dismissedMessages.includes(messageId)) {
    dismissedMessages.push(messageId)
    localStorage.setItem(DISMISSED_MESSAGES_KEY, JSON.stringify(dismissedMessages))
  }
};

const isMessageDismissed = (messageId) => {
  return getDismissedMessages().includes(messageId)
};

let pollingStopped = false;
// 飘条读**本地通知中心**的最新一条，而不是平台侧的 latest。
//
// 两点理由：
//   1. 同源同口径 —— 飘条与通知中心列表读的是同一份数据，不会出现
//      "列表里没有但飘条弹出来"的分裂；
//   2. 平台 latest 是**全局一条**（不分租户、不分人），已读状态记在平台侧；
//      平台消息的已读是按人独立记录的，只有本地通知中心有这个口径。
const checkNewMessage = async () => {
  if (pollingStopped) return
  try {
    const res = await notificationApi.list({ page: 1, page_size: 1 })
    const message = (res?.list || [])[0]
    if (message && message.id && message.id !== currentMessage.value.id && !isMessageDismissed(message.id)) {
      currentMessage.value = message
      hasNewMessage.value = true
      dismissed.value = false
    }
  } catch (error) {
    if (error?.response?.status === 403 || error?.bizCode === 403) {
      pollingStopped = true
      if (checkInterval) { clearInterval(checkInterval); checkInterval = null }
      return
    }
    if (import.meta.env?.DEV) console.warn('获取最新消息失败:', error)
  }
}

const markAsRead = async () => {
  try {
    if (currentMessage.value.id) {
      await notificationApi.markRead(currentMessage.value.id)
      addToDismissedMessages(currentMessage.value.id);
    }
    hasNewMessage.value = false
    dismissed.value = true
    ElMessage.success(i18n.global.t('消息已标记为已读'))
  } catch (error) {
    console.error('标记消息已读失败:', error)
    ElMessage.error(i18n.global.t('标记消息已读失败'))
  }
};

const dismiss = () => {
  hasNewMessage.value = false
  dismissed.value = true
};

onMounted(() => {
  checkNewMessage();
  
  checkInterval = setInterval(checkNewMessage, 30000);
})

onUnmounted(() => {
  if (checkInterval) {
    clearInterval(checkInterval)
  }
})
</script>

<style scoped>
.message-notification {
  position: fixed;
  top: 80px;
  right: 20px;
  z-index: 2000;
  background: #fff;
  border: 1px solid #e4e7ed;
  border-radius: 4px;
  box-shadow: 0 2px 12px 0 rgba(0, 0, 0, 0.1);
  max-width: 320px;
  animation: slideInRight 0.3s ease-out;
}

.message-content {
  padding: 16px;
}

.message-header {
  display: flex;
  align-items: center;
  margin-bottom: 8px;
  font-weight: 500;
  color: #303133;
}

.message-header .el-icon {
  margin-right: 8px;
  color: #4F46E5;
}

.message-title {
  font-size: 14px;
}

.message-body {
  margin-bottom: 12px;
  color: #606266;
  font-size: 13px;
  line-height: 1.5;
}

.message-body p {
  margin: 0;
}

.message-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

@keyframes slideInRight {
  from {
    transform: translateX(100%);
    opacity: 0;
  }
  to {
    transform: translateX(0);
    opacity: 1;
  }
}

@keyframes slideOutRight {
  from {
    transform: translateX(0);
    opacity: 1;
  }
  to {
    transform: translateX(100%);
    opacity: 0;
  }
}

.message-notification.hide {
  animation: slideOutRight 0.3s ease-out;
}
</style>
