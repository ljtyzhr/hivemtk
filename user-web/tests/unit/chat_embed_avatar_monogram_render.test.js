import { describe, it, expect, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'

// 头像徽标的"一个字符"不变量。
//
// 真机走查在 en 语种的访客窗口头部看到蓝方块里溢出半个词（"away" 压在白底上，
// 左半边看不见），ja 的 ゲスト 同样溢出。根因是两处把整词塞进固定尺寸徽标：
// VisitorHeader 的 28px 方块与 ChatMessages 打字气泡旁的 32px 圆头像 ——
// 后者在非中文语种整词是 "Customer Service"。
// 消息列表里别的头像走 avatarText(msg)（取首字符），只有这两处例外，
// 所以判据就是"这两个槽位渲染出的必须是一个码点"。
//
// 顺带钉一枚数据锚：en 的 '客' 曾整条译成 "away"（把"客"当"离开"译）。
// 只断言"一个字符"抓不到它——"a" 也是一个字符，所以额外认这条词值。

import i18n from '@/i18n'
import VisitorHeader from '@/views/chat/embed/components/VisitorHeader.vue'
import ChatMessages from '@/views/chat/embed/components/ChatMessages.vue'

const LOCALES = ['zh', 'en', 'ja', 'ar', 'es', 'fr', 'de', 'ru', 'pt']
const saved = i18n.global.locale.value

const codePoints = (s) => Array.from(String(s || '').trim()).length

const AGENT_MSG = {
  id: 'm1',
  sender_type: 'agent',
  sender_name: '客服小李',
  content: '在的',
  content_type: 'text',
  created_at: '2026-10-11T09:00:00+08:00'
}

let wrapper = null
afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  i18n.global.locale.value = saved
})

describe('访客窗头像徽标只放一个字符', () => {
  for (const loc of LOCALES) {
    it(`头部徽标在 ${loc} 语种渲染为单字符`, () => {
      i18n.global.locale.value = loc
      wrapper = mount(VisitorHeader, {
        props: { title: '在线客服', color: '#1989fa', isOnline: true, agentCount: 1 },
        global: { plugins: [i18n] }
      })
      const badge = wrapper.get('.title-icon').text()
      expect(codePoints(badge)).toBe(1)
    })

    it(`打字指示器头像在 ${loc} 语种渲染为单字符`, () => {
      i18n.global.locale.value = loc
      wrapper = mount(ChatMessages, {
        props: { messages: [AGENT_MSG], loading: false, typing: true },
        global: { plugins: [i18n] }
      })
      const row = wrapper.findAll('.message-row.left')
      const typingRow = row[row.length - 1]
      expect(typingRow.find('.bubble.typing').exists()).toBe(true)
      const texts = typingRow.findAll('.avatar-text').map(n => n.text())
      expect(texts).toHaveLength(1)
      expect(codePoints(texts[0])).toBe(1)
    })
  }

  it("en 的 '客' 词值不再是 away", () => {
    i18n.global.locale.value = 'en'
    expect(i18n.global.t('客')).toBe('Guest')
  })
})
