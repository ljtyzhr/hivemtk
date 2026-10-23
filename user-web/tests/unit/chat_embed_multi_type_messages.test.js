import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'

// 002-multi-type-messages：媒体消息渲染契约。
//
// GetMessagesByConversation（hub）与访客会话（session）两条消息源都输出
// content_type + media_url；此前两个渲染面（工作台 inbox List.vue、聊天窗
// ChatMessages.vue）只有 image 一个分支，video/audio/file 全部退化成一行
// 占位文字。这里锁渲染分派：
//   1) ChatMessages.vue：image → <img>；video → <video controls>；audio → <audio>；file → 下载链接。
//   2) 文本消息渲染不变（回归）。

import ChatMessages from '@/views/chat/embed/components/ChatMessages.vue'

const mountComp = (messages) => mount(ChatMessages, {
  props: { messages, loading: false, typing: false },
  global: { stubs: { RichCard: true } },
})

beforeEach(() => {
  vi.useFakeTimers()
})
afterEach(() => {
  vi.useRealTimers()
})

describe('002 多类型消息：ChatMessages.vue 媒体渲染', () => {
  it('content_type=image → <img>，媒体在上', () => {
    const w = mountComp([
      { id: 1, sender_type: 'user', content_type: 'image', media_url: 'https://f/i.png', content: '看这张', created_at: '2026-10-24T10:00:00Z' },
    ])
    expect(w.find('img.media-image').exists()).toBe(true)
    expect(w.find('img.media-image').attributes('src')).toBe('https://f/i.png')
    // 文字与媒体共存
    expect(w.find('.bubble-content').text()).toContain('看这张')
  })

  it('content_type=video → <video controls>', () => {
    const w = mountComp([
      { id: 2, sender_type: 'user', content_type: 'video', media_url: 'https://f/v.mp4', content: '', created_at: '2026-10-24T10:00:00Z' },
    ])
    const v = w.find('video.media-video')
    expect(v.exists()).toBe(true)
    expect(v.attributes('controls')).toBeDefined()
    expect(v.attributes('src')).toBe('https://f/v.mp4')
  })

  it('content_type=audio → <audio controls>', () => {
    const w = mountComp([
      { id: 3, sender_type: 'ai', content_type: 'audio', media_url: 'https://f/a.mp3', content: '', created_at: '2026-10-24T10:00:00Z' },
    ])
    const a = w.find('audio.media-audio')
    expect(a.exists()).toBe(true)
    expect(a.attributes('src')).toBe('https://f/a.mp3')
  })

  it('content_type=file → 文件名下载链接', () => {
    const w = mountComp([
      { id: 4, sender_type: 'user', content_type: 'file', media_url: 'https://f/files/报价单.pdf', content: '', created_at: '2026-10-24T10:00:00Z' },
    ])
    const link = w.find('.media-file-link')
    expect(link.exists()).toBe(true)
    expect(link.attributes('href')).toBe('https://f/files/报价单.pdf')
    expect(link.text()).toContain('报价单.pdf')
  })

  it('content_type 缺省（旧数据）→ 纯文本气泡（回归）', () => {
    const w = mountComp([
      { id: 5, sender_type: 'ai', content: '你好', created_at: '2026-10-24T10:00:00Z' },
    ])
    expect(w.find('img.media-image').exists()).toBe(false)
    expect(w.find('.bubble-content').text()).toContain('你好')
  })

  it('content_type=card → 走 RichCard 分支（回归，不进媒体分支）', () => {
    const w = mountComp([
      { id: 6, sender_type: 'ai', content_type: 'card', card: { title: '卡片' }, content: '', created_at: '2026-10-24T10:00:00Z' },
    ])
    expect(w.find('.card-wrap').exists()).toBe(true)
    expect(w.find('img.media-image').exists()).toBe(false)
  })
})
