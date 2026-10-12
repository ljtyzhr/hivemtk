import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

// 后端菜单权威注册表的读取器。
//
// 为什么这个文件存在：菜单原本是 Layout.vue 里的一份静态常量，后来改成后端驱动
// （Layout.vue 顶部注释写明「菜单树改为后端驱动（RBAC）：GET /api/menus 返回按角色
// 过滤后的树」），权威注册表搬到了 user-server/internal/model/menu.go 的
// `var SystemMenuTree`。于是"菜单 path 与前端路由字面量必须一致"这条判据的
// 读取点也跟着搬家——不是判据过时了，是判据指错了文件。
//
// 为什么不能顺手把这条判据删掉：path 对不上时菜单点击会进 NotFound、面包屑不显示，
// 而后端注册表与前端路由分处两个目录、两个语言，没有任何编译器会替你发现不一致。
// 这条跨栈判据现在是唯一守着这道缝的东西。
//
// 为什么读 Go 源码而不是调接口：单元测试不能依赖服务在跑、不能依赖数据库里有种子数据。
// 注册表是常量（model.SystemMenuTree），源码即事实。

const MENU_GO = resolve(process.cwd(), '../user-server/internal/model/menu.go')

const SRC = readFileSync(MENU_GO, 'utf8')

// 只取 `var SystemMenuTree = ...` 那一段。取不到就抛：这是一条闸门用例，
// 自己该读的东西读不到必须红，不能退化成"列表为空 ⇒ 后面断言真空通过"。
const block = SRC.match(/var\s+SystemMenuTree\s*=\s*\[\]SystemMenuNode\{([\s\S]*?)\n\}/)
if (!block) {
  throw new Error(`menu.go 里找不到 var SystemMenuTree（读的是 ${MENU_GO}）`)
}

// 为什么逐行解析而不是匹配 \{[^{}]*\}：
// 叶子节点经常写成一行、并且带 Roles 限定，例如
//   {Key: "dynamicThreshold", Title: "动态阈值参数", Icon: "DataLine",
//    Path: "/system/config-params", Roles: []string{"admin"}},
// 这一行里有嵌套花括号（Roles: []string{"admin"}），\{[^{}]*\} 跨不过去，
// 会整个节点都匹配不上、只匹到里层的 {"admin"}（没有 Key 于是被丢掉）。
// 后果是所有带 Roles 限定的菜单项被静默丢弃——2026-10-12 就是这样漏掉了
// /system/config-params、/system/dictionaries、/system/feature-flags 三条，
// 而"菜单 path 都有路由"那条断言因为列表本身少了几条，依然显示通过。
//
// 逐行取 Key/Title/Icon/Path 不受嵌套花括号影响（它们都是 `字段: "字面量"` 形态）。
// 父节点只有 Children 没有 Path，取到的 path 是空串，由调用方按 path 过滤掉。
const lines = block[1].split('\n')

/** 后端注册表里的全部菜单节点。带 path 的是叶子，path 为空的是分组父节点。 */
export const backendMenuItems = () =>
  lines
    .filter((line) => /Key:\s*"/.test(line))
    .map((line) => {
      const pick = (field) => {
        const m = line.match(new RegExp(`${field}:\\s*"([^"]*)"`))
        return m ? m[1] : ''
      }
      // 叶子节点写成一行且自带闭合括号；分组节点的 Children 落在下一行、
      // 自己这行不闭合。判断前先把 Roles: []string{"admin"} 这种嵌套字面量抹掉，
      // 否则带 Roles 的分组会因为那半个括号被误判成叶子。
      const stripped = line.replace(/Roles:\s*\[\]string\{[^}]*\}/g, '')
      return {
        key: pick('Key'),
        title: pick('Title'),
        icon: pick('Icon'),
        path: pick('Path'),
        isGroup: !stripped.includes('}')
      }
    })

/** 只带 Path 的叶子菜单项（分组节点由 isGroup 单独区分，不混进这里）。 */
export const backendLeafMenuItems = () =>
  backendMenuItems().filter((it) => it.path && !it.isGroup)

// 自检：解析出来的 path 条数必须等于注册表里 Path: 字面量的个数。
// 少一条就说明解析器又把某些节点吞了——这类失败是静默的（列表变短、断言更容易通过），
// 所以必须在这里显式拦一道，而不是等到某个 path 消失时去猜为什么。
const parsedPaths = backendMenuItems().filter((it) => it.path).length
const literalPaths = (block[1].match(/Path:\s*"/g) || []).length
if (parsedPaths !== literalPaths) {
  throw new Error(
    `menuRegistry-helper 解析出 ${parsedPaths} 个带 Path 的节点，` +
      `但注册表里有 ${literalPaths} 个 Path: 字面量。解析器漏掉了节点，` +
      `这类失败是静默的（节点变少、断言更容易通过），必须先修解析器。`
  )
}

/**
 * 后端菜单里有没有 path 恰好等于给定值的项。
 *
 * 返回匹配到的 key，找不到返回 null —— 调用方可以据此断言「匹配到的确实是这一项」，
 * 而不只是断言「有某个东西匹配上了」，后者在注册表里有多条同 path 时会掩盖问题。
 */
export const backendMenuKeyByPath = (path) => {
  const hit = backendMenuItems().find((it) => it.path === path)
  return hit ? hit.key : null
}
