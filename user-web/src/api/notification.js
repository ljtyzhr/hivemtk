import { http } from '@/utils/request';

// 通知中心（站内消息）。
//
// 两类消息共用一套端点：
//   - 平台下发（source=platform）：已读按人独立记录，一人点开不影响他人；
//   - 本地业务通知（source=local）：沿用原有的全局已读位。
export const notificationApi = {
  list: (params) => http.get('/api/auth/notifications', { params }),

  markRead: (id) => http.post(`/api/auth/notifications/${id}/read`),

  markAllRead: () => http.post('/api/auth/notifications/read-all'),

  unreadCount: () => http.get('/api/auth/notifications/unread-count')
};

// 消息级别取值，与后端 model.NotificationLevel* 一致。
export const NOTIFICATION_LEVELS = [
  { value: 'info', label: '普通', type: 'info' },
  { value: 'warning', label: '警告', type: 'warning' },
  { value: 'error', label: '重要', type: 'danger' },
  { value: 'success', label: '成功', type: 'success' }
];

export const notificationLevelMeta = (level) =>
  NOTIFICATION_LEVELS.find((l) => l.value === level) || NOTIFICATION_LEVELS[0];
