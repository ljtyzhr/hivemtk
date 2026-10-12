import { describe, it, expect } from 'vitest'
import { readFileSync, readdirSync } from 'node:fs'
import { resolve } from 'node:path'
import { backendMenuKeyByPath } from './menuRegistry-helper'

// 路由登记面的守护用例。
//
// 为什么要测这个：本项目的路由是**懒登记**的 —— src/router/modules/*.js 里的文件写好了
// 不等于挂上了，还得在 index.js 的 moduleNames 里出现一次；漏掉那一句的表现不是白屏报错，
// 而是"点菜单进 NotFound"，前端 review 时几乎看不出来（文件明明在）。
// 后端那边有 scripts/check-unwired-assets.sh 管这件事，前端只有这一条。

const at = (p) => resolve(process.cwd(), p)
const INDEX_SRC = readFileSync(at('src/router/index.js'), 'utf8')
const MODULE_DIR = at('src/router/modules')

// 解析不到就抛：这是一条闸门用例，找不到自己该读的东西时必须红，
// 而不是"数组为空 ⇒ 后面每条断言都真空地通过"。
const listOf = (src, decl) => {
  const block = src.match(new RegExp(`const ${decl} = \\[([\\s\\S]*?)\\n\\];`))
  if (!block) throw new Error(`index.js 里找不到 const ${decl}`)
  return [...block[1].matchAll(/['"]([A-Za-z0-9_]+)['"]/g)].map((m) => m[1])
}

const moduleNames = listOf(INDEX_SRC, 'moduleNames')
const pathToBlock = INDEX_SRC.match(/const pathToModule = \{([\s\S]*?)\n\};/)
if (!pathToBlock) throw new Error('index.js 里找不到 const pathToModule')
const pathToModuleTargets = [...pathToBlock[1].matchAll(/:\s*['"]([A-Za-z0-9_]+)['"]/g)].map((m) => m[1])

const moduleFiles = () =>
  readdirSync(MODULE_DIR)
    .filter((f) => f.endsWith('.js'))
    .map((f) => f.replace(/\.js$/, ''))

const approvalRoute = () => import('@/router/modules/approvalTask.js')

describe('待办中心的路由登记', () => {
  it('模块导出 /approvalTask/list，且组件是懒加载函数', async () => {
    const mod = await approvalRoute()
    expect(Array.isArray(mod.default)).toBe(true)
    const [route] = mod.default
    expect(route.path).toBe('/approvalTask/list')
    expect(route.name).toBe('ApprovalTaskList')
    expect(typeof route.component).toBe('function')
    expect(route.meta.group).toBe('workspace')
  })

  it('已在 moduleNames 里登记（漏了就是点菜单进 NotFound）', () => {
    expect(moduleNames).toContain('approvalTask')
  })

  it('后端菜单注册表里有这一项，且 path 与路由字面量一致（不一致时点菜单进 NotFound、面包屑也不显示）', async () => {
    const mod = await approvalRoute()
    const routePath = mod.default[0].path
    // 菜单改成后端驱动后，权威注册表是 user-server/internal/model/menu.go 的
    // SystemMenuTree，不再是 Layout.vue 里的静态常量。判据没过时，只是读取点搬了家。
    // 一并钉住匹配到的 key：只断言"有东西匹配上了"的话，注册表里将来多一条同 path
    // 的错项也会照样绿。
    expect(backendMenuKeyByPath(routePath)).toBe('approvalTask')
  })
})

describe('登记面整体不失配（后续每张前端卡的通用闸门）', () => {
  it('modules 目录里没有"写了文件却没人登记"的模块', () => {
    const reachable = new Set([...moduleNames, ...pathToModuleTargets])
    expect(moduleFiles().filter((f) => !reachable.has(f))).toEqual([])
  })

  it('登记名里没有"index.js 写了但文件不存在"的幽灵模块', () => {
    const files = new Set(moduleFiles())
    expect(moduleNames.filter((n) => !files.has(n))).toEqual([])
  })

  it('路由 name 全局唯一（重名会让后登记的那条静默顶掉前一条）', async () => {
    const routes = import.meta.glob('../../src/router/modules/*.js', { eager: true })
    const names = Object.values(routes).flatMap((mod) => {
      const list = Array.isArray(mod.default) ? mod.default : [mod.default]
      return list.map((r) => r && r.name).filter(Boolean)
    })
    expect(names.length).toBeGreaterThan(0)
    expect(names.filter((n, i) => names.indexOf(n) !== i)).toEqual([])
  })
})
