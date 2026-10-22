import { describe, it, expect } from 'vitest'
import fs from 'node:fs'
import path from 'node:path'

// 新手引导 driver.js 接线的不变量（任务四 · SaaS 多租户改造后逐页拟人点验）。
//
// 逐页点验时在 /chat/embed/default 上抓到一条真缺陷：
//
//   Uncaught TypeError: Cannot read properties of undefined (reading 'onHighlighted')
//       at useOnboarding.ts:350
//
// 根因是 driver.js 1.x 的 hook 签名是
//
//   type DriverHook = (element: Element | undefined, step: DriveStep, opts: HookOpts) => void
//
// 第一个参数是**元素**、第二个才是步骤。本仓旧代码把第一个参数当步骤用：
//
//   onHighlighted: (step) => { step.onHighlighted?.() }
//
// 于是有两处坏：
//   1. driver.js 在步骤的 element 选择器没命中时用「dummy element」占位，
//      并把 **undefined** 作为 element 传给 hook —— `step.onHighlighted` 直接抛。
//      /chat/embed、/login、/help-center 这些不挂 Layout 的页面上，
//      全局导览的 6 个选择器一个都不存在，正好落进这条路径。
//   2. 就算 element 命中，`steps.indexOf(step)` 也拿不到正确的步号 ——
//      driver.js 传给 hook 的 step 是 B() 复制出来的副本，不是 steps 数组里那个对象；
//      权威步号在 opts.index。旧实现让「当前第几步」永远是 0。
//
// 这里把三条不变量钉死：hook 签名、空元素不炸、以及「首次登录自动触发」
// （旧实现只在 App.vue 的 onMounted 调一次 tryAutoStart，而 App.vue 在 SPA 生命周期里
//  只挂载一次 —— 从 /login 登录进来的用户，onMounted 那一刻还没登录，
//  tryAutoStart 直接跳过；登录成功后 SPA 只换路由，App.vue 不重新挂载，
//  于是全局导览**永远不出现**）。

const REPO = path.resolve(__dirname, '../../..')
const ONBOARDING = path.join(REPO, 'user-web/src/composables/useOnboarding.ts')
const APP_VUE = path.join(REPO, 'user-web/src/App.vue')

const src = fs.readFileSync(ONBOARDING, 'utf8')
const appSrc = fs.readFileSync(APP_VUE, 'utf8')

/**
 * 去掉注释再断言。
 *
 * 断言要卡的是**代码**，不是注释：注释里为了解释坑，往往会把反面写法
 * 原样写出来（比如「也不能用 steps.indexOf(step)」）。直接对原文做
 * 正则匹配，这些解释性文字会把自己的断言打红。
 *
 * 顺序很重要：**先删行注释，再删块注释**。反过来的话，
 * 行注释里出现的字面量块注释起始符（本仓源码里就有这么一处，
 * 注释文本里写了 `/*、` 这种形状）会被当成块注释的开头，
 * 一口气吞掉后面真正的代码，于是断言看着「代码没了」，
 * 其实是注释解析把代码吃掉了。
 */
function stripComments(code) {
  return code
    .replace(/^[ \t]*\/\/.*$/gm, '')   // 先删整行行注释
    .replace(/\/\*[\s\S]*?\*\//g, '')   // 再删块注释
}

const code = stripComments(src)
const appCode = stripComments(appSrc)

describe('新手引导 · driver.js hook 接线', () => {
  it('hook 的第一个形参必须命名为 element（不能是 step）', () => {
    // driver.js 的签名固定是 (element, step, opts)。把 element 命名成 step
    // 就是当年那个 bug 的根，必须拦住。
    expect(code).toMatch(/onHighlightStarted:\s*\(\s*_?e(?:lement)?\s*,/)
    expect(code).toMatch(/onHighlighted:\s*\(\s*_?e(?:lement)?\s*,/)
    expect(code).toMatch(/onDeselected:\s*\(\s*_?e(?:lement)?\s*,/)
    // 反向断言：不允许再出现「第一个形参叫 step」的写法
    expect(code).not.toMatch(/onHighlighted:\s*\(\s*step\s*\)/)
    expect(code).not.toMatch(/onDeselected:\s*\(\s*step\s*\)/)
    expect(code).not.toMatch(/onHighlightStarted:\s*\(\s*step\s*\)/)
  })

  it('onHighlighted / onDeselected 必须对 undefined 步骤做保护', () => {
    // step 为 undefined 时不能直接取属性 —— 这正是「Cannot read properties of
    // undefined (reading 'onHighlighted')」的来源。
    expect(code).toMatch(/onHighlighted:[^}]*?step\?\.onHighlighted\?\.\(\)/)
    expect(code).toMatch(/onDeselected:[^}]*?step\?\.onDeselected\?\.\(\)/)
    // 不允许出现裸的 step.onHighlighted / step.onDeselected
    expect(code).not.toMatch(/\bstep\.onHighlighted\b/)
    expect(code).not.toMatch(/\bstep\.onDeselected\b/)
  })

  it('步号取自 opts.index，不能用 steps.indexOf(step)', () => {
    // driver.js 传给 hook 的 step 是复制品，indexOf 恒为 -1 ⇒ 步号永远是 0。
    expect(code).toMatch(/opts\?\.index/)
    expect(code).not.toMatch(/steps\.indexOf\(step\)/)
  })
})

describe('新手引导 · 触发时机', () => {
  it('App.vue 必须监听登录态跃迁（首次登录自动触发）', () => {
    // App.vue 在 SPA 里只挂载一次，只在 onMounted 调 tryAutoStart 的话，
    // 从 /login 登录进来的用户永远看不到全局导览。
    expect(appCode).toMatch(/watch\s*\(\s*\(\s*\)\s*=>\s*userStore\.isLoggedIn/)
    expect(appCode).toMatch(/if\s*\(\s*now\s*&&\s*!prev\s*\)/)
  })

  it('App.vue 的 onMounted 仍然保留 tryAutoStart（刷新进已登录态也能触发）', () => {
    expect(appCode).toMatch(/tryAutoStart\(\)/)
  })

  it('无布局外壳的页面不启动全局导览（否则 driver.js 退化成 dummy element）', () => {
    expect(code).toMatch(/hasLayoutChrome/)
    expect(code).toMatch(/if\s*\(\s*!hasLayoutChrome\(steps\)\s*\)/)
  })

  it('hasLayoutChrome 对非法选择器必须兜底，不能让导览流程炸掉', () => {
    expect(code).toMatch(/try\s*\{[\s\S]*?querySelector[\s\S]*?\}\s*catch/)
  })
})

describe('新手引导 · 「完成」按钮必须真的关掉导览', () => {
  // 复现：逐页拟人点验时，走完 6 步点「完成」→ 标记已落盘（不再弹），
  // 但遮罩 + 气泡留在原地不动（实测 popover display:block、300×186、overlay 可见）。
  //
  // 根因：driver.js 构造 popover 时 onNextClick 取「步骤级 || 默认」，
  // 步骤级优先。createDriver 把最后一步的 popover.onNextClick 换成了自己的
  // 包装函数，于是 driver 自己「最后一页 y('done') 销毁」的逻辑被顶掉；
  // 包装函数只调 onComplete()，没人 destroy ⇒ 导览永不关闭。
  it('最后一步的 onNextClick 包装里必须显式 destroy driver', () => {
    const m = code.match(/last\.popover\.onNextClick\s*=\s*\(\)\s*=>\s*\{([\s\S]*?)\n\s*\}/)
    expect(m, '找不到最后一步 onNextClick 的包装函数').not.toBeNull()
    const body = m[1]
    expect(body, '包装函数里必须调用 destroy()，否则「完成」关不掉').toMatch(/\.destroy\(\)/)
    // 且必须是调 driver 实例的 destroy，而不是别的空函数
    expect(body).toMatch(/instance\?\.destroy\(\)|driverInstance\?\.destroy\(\)/)
  })

  it('destroy 之前必须先跑 onComplete（标记不能因提前销毁而丢）', () => {
    const m = code.match(/last\.popover\.onNextClick\s*=\s*\(\)\s*=>\s*\{([\s\S]*?)\n\s*\}/)
    const body = m[1]
    const onCompleteAt = body.indexOf('onComplete()')
    const destroyAt = body.indexOf('.destroy(')
    expect(onCompleteAt, '包装里缺 onComplete()').toBeGreaterThan(-1)
    expect(destroyAt).toBeGreaterThan(onCompleteAt)
  })
})
