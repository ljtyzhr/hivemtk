import { describe, it, expect } from 'vitest'
import fs from 'node:fs'
import path from 'node:path'
import { baseCompile } from '@intlify/message-compiler'

// 9 个语言包里的**每一条**词条都必须能被 vue-i18n 编译通过。
//
// 为什么这条门这么严：vue-i18n 1.x 在 dev 下用**运行时**编译器（vite 的
// hivemtk-i18n-functions 插件只在 build 阶段预编译，dev 走原始 JSON），
// 运行时编译靠 new Function —— 撞上 CSP 的 script-src 'self'（没有 unsafe-eval）
// 就直接 EvalError 抛出来。而词条编译是**按需**的：只要有一条词条里有
// vue-i18n 认不出来的语法，这条词条被渲染的瞬间就报错，页面当场白屏。
//
// 已经踩过的两种坑：
//   1. 「变量写成 {{name}} 这样的形式」→ Not allowed nest placeholder
//   2. 「用户 @机器人 就能触发」     → Invalid linked format
// 两条都把 user-web 顶成了白屏（任务六实测）。
//
// 词条里想显示花括号或 @ 符号，得用 vue-i18n 的字面量转义写法：
//   {'{'}  表示字面的左花括号，{'}'} 表示字面的右花括号。
//   @ 符号在词条里**不能**直接出现（@:key 会被当成链接消息引用）。
//
// 附带效果：这条门还守住了一个**存量**缺陷——词条
// 「JSON 格式，例如 {"order_id": 123}」里的双引号让 vue-i18n 把它当占位符解析，
// 编译必失败。它一直没爆是因为没有任何 t() 引用它（孤儿词条），
// 但只要有人用一次，那一页就白屏。

const REPO = path.resolve(__dirname, '../../..')
const LOCALES_DIR = path.join(REPO, 'user-web/src/i18n/locales')
const LANGS = ['zh', 'en', 'ja', 'ar', 'es', 'fr', 'de', 'ru', 'pt']

function collectMessages(node, prefix = '', out = []) {
  if (node && typeof node === 'object' && !Array.isArray(node)) {
    Object.entries(node).forEach(([k, v]) => {
      const key = prefix ? `${prefix}.${k}` : k
      if (typeof v === 'string') out.push([key, v])
      else collectMessages(v, key, out)
    })
  }
  return out
}

describe('语言包词条 · vue-i18n 可编译性', () => {
  LANGS.forEach((lang) => {
    it(`${lang}.json 的每条词条都能被 vue-i18n 编译（编译失败 = 渲染即白屏）`, () => {
      const file = path.join(LOCALES_DIR, `${lang}.json`)
      const data = JSON.parse(fs.readFileSync(file, 'utf8'))
      const messages = collectMessages(data)
      expect(messages.length).toBeGreaterThan(0)

      const broken = []
      messages.forEach(([key, text]) => {
        try {
          baseCompile(text, {
            onError: (err) => {
              throw err
            }
          })
        } catch (err) {
          broken.push(`${key} :: ${err.message}`)
        }
      })
      expect(broken, `${lang}.json 有词条编译不过：\n${broken.join('\n')}`).toEqual([])
    })
  })

  it('引导文案里不出现 {{ }}（占位符）与 @（链接消息标记）', () => {
    LANGS.forEach((lang) => {
      const file = path.join(LOCALES_DIR, `${lang}.json`)
      const data = JSON.parse(fs.readFileSync(file, 'utf8'))
      const page = data?.setup?.onboarding?.page || {}
      Object.entries(page).forEach(([pageKey, entry]) => {
        if (pageKey === 'common' || !entry || typeof entry !== 'object') return
        const texts = []
        const collect = (node) => {
          if (typeof node === 'string') texts.push(node)
          else if (node && typeof node === 'object') Object.values(node).forEach(collect)
        }
        collect(entry)
        texts.forEach((text) => {
          expect(text, `${lang} 的 ${pageKey} 文案里出现了 {{ }}`).not.toMatch(/\{\{|\}\}/)
          expect(text, `${lang} 的 ${pageKey} 文案里出现了 @`).not.toContain('@')
        })
      })
    })
  })
})
