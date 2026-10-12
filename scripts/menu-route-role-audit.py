#!/usr/bin/env python3
"""菜单可见性 × 路由授权档位 的错配审计器（可重跑版）。

## 它补的是哪个洞
侧栏给谁看、接口谁能调，是两份**独立写下**的授权：
- 前者在 `user-server/internal/model/menu.go` 的 `SystemMenuTree` 各节点 `Roles`
  （落库后经 `role_menu_permissions` 分配，首空表按 `DefaultMenuKeys` seed）；
- 后者在路由注册点上的守卫（`JWTAuthMiddleware` / `AdminAuthMiddleware` /
  `RoutePolicyGuard` 那张仅超管表）。

两者会各改各的，而两个方向的错配都已经在真系统里发生过：
- **菜单藏着、接口放行**＝"前端不显示＝没有权限"这个假前提：任何登录角色直接调 API
  就能读到只该超管看的数据（本轮实测到的形态：统一消息、若干运营看板）。
- **菜单给了、接口拦人**＝页面能打开但一片空白／满屏 403，症状与"没有菜单"几乎一样，
  排查时最容易先怀疑前端。

这道审计把两个方向都算成名册，而不是靠人肉对齐两张表。

## 事实源（三处，缺一就退非零）
1. 菜单树 = `user-server/internal/model/menu.go` 里的 `SystemMenuTree` 字面量，
   按花括号配对解析出 key/path/roles 与父子链（节点可见性＝自身与所有祖先都允许）。
2. 页面路径 → 视图文件 = `user-web/src/router/**/*.js` 里的 `path:` / `component:` 对
   （嵌套子路由的相对 path 按父级绝对路径拼接，与 vue-router 的口径一致）。
3. 路由档位 = `go test -run '^TestDumpLiveRouteGuardChains$' ./internal/router` 导出的 TSV
   （需测试库环境变量；`ROUTE_GUARD_DUMP_FILE` 指到哪个文件就写哪个）。
   档位是 **effective** 的：`RoutePolicyGuard` 挂在 auth 组每条链上，只有路径命中仅超管表
   才算它在拦人，所以 `super-admin` 的判据是"链上有硬超管守卫，或命中仅超管表且链上有
   RoutePolicyGuard"，不是"名字出现过"。四档：
   - `anonymous` 链上没身份守卫；
   - `identity` 登录即可调（泄露面就在这一档）；
   - `capability` 拦不拦取决于调用者有没有被授予该形状所属的菜单节点（本脚本生成的
     `menu_capability_routes.go` 在场时才会读到这一档）；
   - `super-admin` 非超管一律 403。

## 消费面口径
一个页面调了哪些接口：从视图文件出发，沿**本地模块 import**（`@/…`、`./…`、`../…`）
递归到深度 3，沿途每份文件里出现的 `/api/…` 字面量都算这个页面的调用面。
形状归一与串抽取直接复用 `scripts/route-consumer-triage.py` 的 `shape_of` /
`extract_tokens`——两处口径必须同源，否则同一个页面在两份读数里会被算成两个消费面。

除了"哪个菜单页在调"，还要问"谁在登录就会调"：pinia store、layout、router 守卫、
axios 拦截器、公共组件这些**不是菜单页**的源文件，任意角色登录都会执行。这类壳层调用
的形状即使挂在 admin-only 菜单页下也不能收进仅超管表——拦了就把整个前端打挂
（实测的第一版就漏了这层：`/api/menus` 被 permission store 全局调用，却因"只有
roleManage 页在调它"被判成 LEAK）。名册因此分六桶（LEAK／SHARED／BLOCK／ORPHAN／
GLOBAL／PENDING），降级两桶各单列 `global-demoted.tsv`、`pending-nonmenu-page.tsv`
供人工复核，不并进取口表。

## 读数的三条限制（写在这里，别让名册被拿去做过删除/放行的依据）
- 可见性按 **seed 默认分配**（`DefaultMenuKeys`）折算。管理员在界面上改过
  `role_menu_permissions` 之后，库里的分配才是真值；这份名册只描述"出厂形状"。
- 静态扫不到的调用方（老前端构建、地址栏直开、服务端模板）同样看不见 ⇒ 名册里的
  "没人调"不成立时也不构成放行依据，只构成"这一条要人看一眼"。
- 后端只有 3 档角色（admin / customer_service / staff），而菜单词表里还留着
  manager/sales/viewer 这类旧词：`Roles: ["admin","manager"]` 在两档非超管角色上
  都判不可见，即"这页只有超管能看到"。这不是 bug，是这张表的历史形状，审计按它算。

## 读数怎么变成防线（--emit-go / --check-go）
LEAK 名册不只是清单：`--emit-go REL` 把它渲染成 `user-server/internal/model/menu_capability_routes.go`
（形状 → 归属菜单 key），`RoutePolicyGuard` 拿它做第二档判定——非超管要被授予归属 key 之一
才放行。于是"菜单给谁看"和"接口谁能调"合成一处，管理员在角色菜单界面勾选即开、取消即关。

因此这份脚本和它的生成件是**闭环**，两条口径必须写死：
- 生成输入用整份 LEAK 名册（含已收口的），不是只取未收口的。否则表一生效，下一轮读到的
  档位就从 identity 变成 capability、名册归零、`--emit-go` 把真表抹成空表。
  `LEAK_TRIGGER_TIERS` 把 capability 也算进触发集，就是在守这条。
- `--check-go REL`（CI 用）查两件事：生成结果与库里那份逐字相同（防手改/防漂移），
  且未收口条数为 0（防"表还在、防线没了"——把一条注册挪到守卫快照之前时，档位退回
  identity，那份表反而成了没人读的字节，只比字节的门照样绿）。

## 自检
`--self-check` 用内存夹具验三样东西：菜单解析器能算出父子链上的可见性、
路由页解析器能把嵌套相对 path 拼成绝对 path、两向错配各自开火且有极性对
（"admin-only 菜单 + identity 接口"必须报 LEAK，"admin-only 菜单 + super-admin 接口"
必须不报；"非超管可见菜单 + super-admin 接口"必须报 BLOCK；同一形状若是壳层在调，
LEAK 必须让位给降级）。
任何一枚不成立就退非零：读数先要能否掉自己，才允许拿去判别人。
"""

import argparse
import hashlib
import importlib.util
import os
import re
import subprocess
import sys
import time

HERE = os.path.dirname(os.path.abspath(__file__))
ADMIN = "admin"
CUSTOMER_SERVICE = "customer_service"
STAFF = "staff"
NON_ADMIN_ROLES = (CUSTOMER_SERVICE, STAFF)

# 口径同源：形状归一与 /api/ 串抽取都用分诊器那一份，避免两套口径各说各话
_spec = importlib.util.spec_from_file_location(
    "route_consumer_triage", os.path.join(HERE, "route-consumer-triage.py"))
RCT = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(RCT)

MENU_GO_RE = re.compile(r"var SystemMenuTree = \[\](?:model\.)?SystemMenuNode\{")
FIELD_RE_CACHE = {}


def field_re(name):
    if name not in FIELD_RE_CACHE:
        FIELD_RE_CACHE[name] = re.compile(
            r"\b%s:\s*(\"[^\"]*\"|'[^']*'|\[\]string\{[^}]*\})" % re.escape(name))
    return FIELD_RE_CACHE[name]


def match_brace(text, open_idx):
    """从 text[open_idx]（必须是 '{'）配到闭合 '}'，返回闭合下标。

    跳过 Go/JS 字符串字面量与行注释：注释里的中文引号与代码里的花括号都不该参与配对。
    """
    depth = 0
    i = open_idx
    n = len(text)
    while i < n:
        ch = text[i]
        if ch in "\"'`":
            quote = ch
            i += 1
            while i < n:
                if text[i] == "\\":
                    i += 2
                    continue
                if text[i] == quote:
                    break
                i += 1
            i += 1
            continue
        if ch == "/" and i + 1 < n and text[i + 1] == "/":
            while i < n and text[i] != "\n":
                i += 1
            continue
        if ch == "/" and i + 1 < n and text[i + 1] == "*":
            end = text.find("*/", i + 2)
            i = n if end < 0 else end + 2
            continue
        if ch == "{":
            depth += 1
        elif ch == "}":
            depth -= 1
            if depth == 0:
                return i
        i += 1
    raise ValueError(f"花括号没配平（起始下标 {open_idx}）：解析器该修，读数不可用")


def match_bracket(text, open_idx):
    """同上，但配 '[' … ']'（children 数组用）。"""
    depth = 0
    i = open_idx
    n = len(text)
    while i < n:
        ch = text[i]
        if ch in "\"'`":
            quote = ch
            i += 1
            while i < n:
                if text[i] == "\\":
                    i += 2
                    continue
                if text[i] == quote:
                    break
                i += 1
            i += 1
            continue
        if ch == "/" and i + 1 < n and text[i + 1] == "/":
            while i < n and text[i] != "\n":
                i += 1
            continue
        if ch == "/" and i + 1 < n and text[i + 1] == "*":
            end = text.find("*/", i + 2)
            i = n if end < 0 else end + 2
            continue
        if ch == "[":
            depth += 1
        elif ch == "]":
            depth -= 1
            if depth == 0:
                return i
        i += 1
    raise ValueError(f"方括号没配平（起始下标 {open_idx}）")


def strip_nested(inner, names):
    """把 inner 里 `name:` 后面的嵌套块切掉，返回只剩本节点自身字段的那段文本。"""
    out = inner
    for name in names:
        m = re.search(r"\b%s\s*:" % re.escape(name), out)
        if not m:
            continue
        start = m.end()
        while start < len(out) and out[start] not in "[{":
            start += 1
        if start >= len(out):
            continue
        closer = match_bracket(out, start) if out[start] == "[" else match_brace(out, start)
        out = out[:m.start()] + out[closer + 1:]
    return out


def unquote(s):
    s = s.strip()
    if s and s[0] in "\"'":
        return s[1:-1]
    return s


def parse_roles(text):
    m = re.search(r"\[\]string\{([^}]*)\}", text)
    if not m:
        return []
    return [unquote(x) for x in re.findall(r"\"[^\"]*\"|'[^']*'", m.group(1))]


def parse_menu_tree(go_path):
    """解析 SystemMenuTree，返回 [{key,path,roles,children…}] 的森林（只含根层节点）。"""
    with open(go_path, encoding="utf-8") as f:
        text = f.read()
    m = MENU_GO_RE.search(text)
    if not m:
        raise SystemExit(f"在 {go_path} 里找不到 SystemMenuTree 的声明头：菜单事实源搬家了")
    open_idx = text.index("{", m.end() - 1)
    close_idx = match_brace(text, open_idx)
    return parse_node_list(text[open_idx + 1:close_idx])


def literal_open(text, from_idx):
    """从 `from_idx` 起找到**字面量**的起始花括号下标。

    Go 的切片字面量写成 `[]SystemMenuNode{ … }`：紧跟 `Children:` 的那个 `[` 是类型
    的维度括号（`[]` 自己就是开与闭），不是数据。按 `[` 配它会当场闭合成空串，
    子树就这么静默丢了——菜单解析只回 7 个根节点、叶子全没，就是这么来的。
    """
    i = from_idx
    n = len(text)
    while i < n and text[i] not in "[{":
        i += 1
    if i >= n:
        return -1, ""
    if text[i] == "{":
        return i, "brace"
    if i + 1 < n and text[i + 1] == "]":
        j = i + 2
        while j < n and text[j] != "{":
            j += 1
        if j >= n:
            return -1, ""
        return j, "brace"
    return i, "bracket"


def parse_node_list(inner):
    """inner 是 `[]SystemMenuNode{ ... }` 去掉外层花括号后的内容 ⇒ 一串以 '{' 开头的元素。"""
    nodes = []
    i = 0
    n = len(inner)
    while i < n:
        if inner[i] != "{":
            if inner[i] == "/" and i + 1 < n and inner[i + 1] == "/":
                while i < n and inner[i] != "\n":
                    i += 1
                continue
            i += 1
            continue
        close = match_brace(inner, i)
        body = inner[i + 1:close]
        children_src = ""
        cm = re.search(r"\bChildren\s*:", body)
        own = body
        if cm:
            start, kind = literal_open(body, cm.end())
            if start >= 0:
                closer = (match_brace(body, start) if kind == "brace"
                          else match_bracket(body, start))
                children_src = body[start + 1:closer]
                own = body[:cm.start()] + body[closer + 1:]
        node = {
            "key": unquote_first(own, "Key"),
            "path": unquote_first(own, "Path"),
            "roles": parse_roles(own),
            "children": parse_node_list(children_src) if children_src else [],
        }
        nodes.append(node)
        i = close + 1
    return nodes


def unquote_first(text, name):
    m = field_re(name).search(text)
    return unquote(m.group(1)) if m else ""


def role_allows(role, roles):
    """与 model.roleAllowsMenu 逐字同口径（旧静态词表折算到 3 档角色）。"""
    if not roles:
        return True
    if role == ADMIN:
        return True
    if role == CUSTOMER_SERVICE:
        return any(r in (CUSTOMER_SERVICE, STAFF) for r in roles)
    if role == STAFF:
        return any(r in (STAFF, "viewer") for r in roles)
    return False


def flatten_menu(nodes, ancestors=()):
    """展开成带祖先链的叶子记录：可见性要按"自身与所有祖先都允许"算。"""
    out = []
    for n in nodes:
        chain = ancestors + (n,)
        if n["path"]:
            out.append({"key": n["key"], "path": n["path"], "chain": chain})
        out.extend(flatten_menu(n["children"], chain))
    return out


def visible(node, role):
    return all(role_allows(role, a["roles"]) for a in node["chain"])


ROUTER_PATH_RE = re.compile(r"\bpath\s*:\s*(\"[^\"]*\"|'[^']*')")
ROUTER_COMP_RE = re.compile(r"\bcomponent\s*:\s*\(?\)?\s*=>\s*import\(\s*(\"[^\"]*\"|'[^']*')\s*\)")


def parse_router_pages(root):
    """返回 pagePath（绝对、已按父级拼接）→ 视图文件相对路径 的表。

    只认 `component: () => import('…')` 这种懒加载写法；`component: Layout` 这类
    直接引用变量的写法不产页面归属（它是外壳，不调业务接口）。
    """
    pages = {}
    router_dir = os.path.join(root, "user-web", "src", "router")
    if not os.path.isdir(router_dir):
        raise SystemExit(f"前端路由目录不存在：{router_dir}")
    for dirpath, _dirs, files in os.walk(router_dir):
        for fn in files:
            if not fn.endswith(".js"):
                continue
            path = os.path.join(dirpath, fn)
            with open(path, encoding="utf-8") as f:
                text = f.read()
            for rel, comp, parent in walk_route_objects(text):
                full = join_route_path(parent, rel)
                if not comp:
                    continue
                pages.setdefault(full, os.path.relpath(comp, root))
    return pages


def walk_route_objects(text):
    """递归遍历路由数组里的对象，产出 (path, componentImport, parentFullPath)。"""
    results = []

    def scan(inner, parent_path):
        i = 0
        n = len(inner)
        while i < n:
            if inner[i] != "{":
                i += 1
                continue
            try:
                close = match_brace(inner, i)
            except ValueError:
                i += 1
                continue
            body = inner[i + 1:close]
            children_src = ""
            own = body
            cm = re.search(r"\bchildren\s*:", body)
            if cm:
                start = cm.end()
                while start < len(body) and body[start] not in "[{":
                    start += 1
                if start < len(body):
                    closer = match_bracket(body, start) if body[start] == "[" else match_brace(body, start)
                    children_src = body[start + 1:closer]
                    own = body[:cm.start()] + body[closer + 1:]
            own = strip_nested(own, ["meta"])
            pm = ROUTER_PATH_RE.search(own)
            compm = ROUTER_COMP_RE.search(own)
            rel = unquote(pm.group(1)) if pm else ""
            comp = unquote(compm.group(1)) if compm else ""
            full = join_route_path(parent_path, rel)
            if rel and comp:
                results.append((rel, resolve_component(root_marker, comp), parent_path))
            if children_src:
                scan(children_src, full or parent_path)
            i = close + 1
        return results

    # 两种导出形状都要认：`export default [ {...} ]`（多数模块）和 `export default { ... }`
    # （单页模块）。只认数组时，单页模块的 path 根本进不了 pages，挂在它上面的菜单节点
    # 会被报成 ORPHAN＝"菜单点了进 NotFound"，而真相是解析器没看见这条路由。
    # ORPHAN 这一档在名册里的含义是"前端缺页"，所以它必须先把解析器自己的覆盖面交代清楚，
    # 否则假 ORPHAN 会盖住真 ORPHAN。
    arr = re.search(r"export default \[|\[\s*\{", text)
    obj = re.search(r"export default \{", text)
    if obj and (not arr or obj.start() < arr.start()):
        start = obj.end() - 1
        try:
            end = match_brace(text, start)
        except ValueError:
            return results
        return scan(text[start:end + 1], "")
    if not arr:
        return results
    start = text.find("[", arr.start())
    if start < 0:
        return results
    try:
        end = match_bracket(text, start)
    except ValueError:
        return results
    return scan(text[start + 1:end], "")


root_marker = ""


def resolve_component(base_root, spec):
    """把 `@/views/x/List.vue` 解析成仓库相对路径（找不到就回原样，由上层判缺失）。"""
    if spec.startswith("@/"):
        cand = os.path.join(base_root, "user-web", "src", spec[2:])
    elif spec.startswith("."):
        cand = os.path.normpath(os.path.join(base_root, "user-web", "src", spec))
    else:
        cand = os.path.join(base_root, spec.lstrip("/"))
    if os.path.splitext(cand)[1] == "":
        for ext in (".vue", ".js", ".ts"):
            if os.path.exists(cand + ext):
                return cand
    return cand


def join_route_path(parent, rel):
    if not rel:
        return parent
    if rel.startswith("/"):
        return rel
    if not parent:
        return "/" + rel.lstrip("/")
    return parent.rstrip("/") + "/" + rel.lstrip("/")


IMPORT_RE = re.compile(r"""(?:from|import)\s*\(?\s*['"]([^'"]+)['"]""")
API_TOKEN_MIN_SEGMENTS = 2

# 注释里提到的接口不是调用点：`src/router/modules/salesWorkbench.js` 的注释写着
# /api/sales-workbench/overview，页面/壳层文件都不真调它。不剥注释就会凭空多出
# 一批"某页在调"的形状（实测 18 个页×形状对），其中一半会伪装成 LEAK 名册里的
# 一条口子，据此往仅超管表里写规则就是在拦一个前端根本没发的请求。
BLOCK_COMMENT_RE = re.compile(r"/\*.*?\*/", re.S)
HTML_COMMENT_RE = re.compile(r"<!--.*?-->", re.S)
# 只认行首或前面是空白/;/,/*/ { 的 //，否则会把 `https://x/api/y` 从 `//` 处截断，
# 那种截断丢掉的是真调用点——方向上比"漏剥一条注释"危险得多。
LINE_COMMENT_RE = re.compile(r"(?:(?<=^)|(?<=[\s;,*{]))//")


def strip_comments(text, is_vue):
    if is_vue:
        text = HTML_COMMENT_RE.sub(" ", text)
    text = BLOCK_COMMENT_RE.sub(" ", text)
    out = []
    for line in text.splitlines():
        cut = LINE_COMMENT_RE.search(line)
        out.append(line[:cut.start()] if cut else line)
    return "\n".join(out)


def api_shapes_in_file(abs_path):
    """一份源文件里出现的 /api/… 形状（剥注释后），读不到就当没有。"""
    try:
        with open(abs_path, encoding="utf-8") as f:
            text = f.read()
    except (OSError, UnicodeDecodeError):
        return set()
    return RCT.extract_tokens(strip_comments(text, abs_path.endswith(".vue")))


def _import_child(root, rel_path, spec):
    """把一条本地 import 折成**仓库相对**路径（与 rel_path 同一坐标系）；不是本地模块就 None。

    `@/` 别名指向 `user-web/src/`，所以必须补上这段前缀：只回 "api/menu" 的话，
    visit 拿 root+"api/menu" 去 stat 必然落空，整条 import 递归在第一跳就断——
    实测这样会把 LEAK 从 246 读成 15、SHARED/GLOBAL 读成 0（一份"看起来更干净"的假名册）。
    """
    if spec.startswith("@/"):
        return os.path.join("user-web", "src", *spec[2:].split("/"))
    if spec.startswith("."):
        joined = os.path.normpath(os.path.join(os.path.dirname(rel_path), spec))
        return joined.replace(os.sep, "/")
    return None


def collect_shapes(root, view_rel, max_depth=4, forbidden=()):
    """沿本地 import 递归收 /api/… 形状。

    forbidden 用来在某个目录边界停住：页面闭包不许顺路由表爬进别的页面，
    壳层闭包不许顺 `@/router` 把 216 个页面的调用面全收进来（那样 LEAK 恒空）。
    """
    shapes = set()
    seen = set()

    def visit(rel_path, depth):
        if depth > max_depth or rel_path in seen:
            return
        seen.add(rel_path)
        if any(rel_path == f or rel_path.startswith(f) for f in forbidden):
            return
        abs_path = os.path.join(root, rel_path)
        if os.path.isdir(abs_path):
            abs_path = os.path.join(abs_path, "index.js")
        if not os.path.exists(abs_path):
            for cand in (abs_path + ".vue", abs_path + ".js", abs_path + ".ts",
                         os.path.join(abs_path, "index.js")):
                if os.path.exists(cand):
                    abs_path = cand
                    break
            else:
                return
        shapes.update(api_shapes_in_file(abs_path))
        try:
            with open(abs_path, encoding="utf-8") as f:
                text = f.read()
        except (OSError, UnicodeDecodeError):
            return
        for spec in IMPORT_RE.findall(text):
            child = _import_child(root, rel_path, spec)
            if child is None:
                continue
            visit(child, depth + 1)

    visit(view_rel, 1)
    return shapes


def page_api_shapes(root, view_rel, max_depth=4):
    """视图文件沿本地 import 递归，收所有 /api/… 字面量的形状（页面闭包不进路由表）。"""
    return collect_shapes(root, view_rel, max_depth, forbidden=("user-web/src/router",))


# 登录即执行的前端代码：store、壳层布局、请求封装、启动入口、全局配置。
# 不含 components/——`src/components/WeComSendDialog.vue` 只被一页挂载，把它当全局
# 执行体会把整族 /api/wecom/* 从名册里抹掉（实测踩过）。
SHELL_ENTRY_DIRS = ("stores", "layout", "utils", "config", "constants", "i18n")
SHELL_ENTRY_FILES = ("main.js", "App.vue")


def global_api_shapes(root):
    """任意角色登录就会打的接口形状 ⇒ 形状 → 首个命中的壳层入口。

    这些形状即使挂在 admin-only 菜单页下也不能收进仅超管表：拦了就把整个前端打挂
    （`/api/menus` 由 layout/permission store 全局调用，却因"只有 roleManage 页在调"
    被判成 LEAK，就是这一类）。
    """
    src = os.path.join(root, "user-web", "src")
    entries = list(SHELL_ENTRY_FILES)
    for d in SHELL_ENTRY_DIRS:
        base = os.path.join(src, d)
        if not os.path.isdir(base):
            continue
        for dirpath, dirnames, filenames in os.walk(base):
            dirnames[:] = [x for x in dirnames if x != "node_modules"]
            for fn in filenames:
                if fn.endswith((".vue", ".js", ".ts")):
                    rel = os.path.relpath(os.path.join(dirpath, fn), src)
                    entries.append(rel.replace(os.sep, "/"))
    hits = {}
    for rel in entries:
        for shape in collect_shapes(root, os.path.join("user-web", "src", rel),
                                    forbidden=("user-web/src/router",
                                               "user-web/src/views")):
            hits.setdefault(shape, rel)
    return hits



def load_guard_tiers(tsv):
    """METHOD PATH TIER GUARDS → {(shape): tier}，并保留 (方法,路径) 的名册。"""
    tiers = {}
    rows = []
    with open(tsv, encoding="utf-8") as f:
        header = f.readline()
        if not header.startswith("METHOD\tPATH\tTIER"):
            raise SystemExit(f"{tsv} 的表头不是 METHOD/PATH/TIER[/GUARDS]：这不是守卫档位导出件")
        for line in f:
            parts = line.rstrip("\n").split("\t")
            if len(parts) < 3:
                continue
            method, path, tier = parts[0], parts[1], parts[2]
            shape = RCT.shape_of(path)
            tiers.setdefault(shape, set()).add(tier)
            rows.append((method, path, shape, tier))
    if not rows:
        raise SystemExit(f"{tsv} 是空的：档位读不出来，不能据此判'没有错配'")
    return tiers, rows


def audit(root, tiers):
    menu_nodes = flatten_menu(parse_menu_tree(
        os.path.join(root, "user-server", "internal", "model", "menu.go")))
    pages = parse_router_pages(root)
    globals_ = global_api_shapes(root)

    # 先建"每个形状被哪些页面用"的反向索引：LEAK 里同一形状可能被另一个**非超管也能看**
    # 的页面用着，那时把它收进仅超管表＝把那个角色的页面打成满屏 403。
    # 没有这层反向索引，名册只能读成"345 条都该收口"，而真值是能收口的只是其中一部分。
    shape_users = {}
    leaves, leaks, blocks, orphans, shared, demoted, unknowns = [], [], [], [], [], [], []
    node_apis = {}
    menu_used_views = set()
    for node in menu_nodes:
        view = pages.get(node["path"])
        if view is None:
            orphans.append((node["key"], node["path"]))
            continue
        menu_used_views.add(view)
        shapes = page_api_shapes(root, view)
        node_apis[node["key"]] = shapes
        leaves.append((node["key"], node["path"], view, len(shapes)))
        vis = {r: visible(node, r) for r in NON_ADMIN_ROLES}
        for shape in shapes:
            shape_users.setdefault(shape, set()).add(
                (node["key"], bool(vis[CUSTOMER_SERVICE] or vis[STAFF])))

    # 前端有路由、菜单里没有的页面（详情页／帮助页／嵌入页）：角色可见性算不出来，
    # 但它们在 app 内被任何角色点开的概率不为零 ⇒ 不许直接收口，单列待人工判。
    extra_pages = {}
    for path, view in pages.items():
        if view in menu_used_views:
            continue
        for shape in collect_shapes(root, view, forbidden=("user-web/src/router",)):
            extra_pages.setdefault(shape, []).append(path)

    for node in menu_nodes:
        vis = {r: visible(node, r) for r in NON_ADMIN_ROLES}
        shapes = node_apis.get(node["key"])
        if shapes is None:
            continue
        for shape in sorted(shapes):
            got = tiers.get(shape)
            if not got:
                continue
            users = shape_users.get(shape, ())
            others = sorted(k for k, _ in users if k != node["key"])
            non_admin_users = sorted(k for k, na in users if na and k != node["key"])
            verdict = classify_fixture(
                vis_cs=vis[CUSTOMER_SERVICE], vis_staff=vis[STAFF],
                tiers_of_shape=got,
                has_non_admin_page=bool(non_admin_users),
                in_shell=shape in globals_,
                in_extra_page=shape in extra_pages)
            if verdict == "LEAK":
                leaks.append((node["key"], node["path"], shape, sorted(got),
                              ",".join(others)))
            elif verdict == "SHARED":
                shared.append((node["key"], node["path"], shape,
                               ",".join(non_admin_users)))
            elif verdict == "GLOBAL":
                demoted.append((node["key"], node["path"], shape, globals_[shape]))
            elif verdict == "PENDING":
                unknowns.append((node["key"], node["path"], shape,
                                 ",".join(sorted(set(extra_pages[shape]))[:4])))
            elif verdict == "BLOCK":
                blocks.append((node["key"], node["path"], shape,
                               vis[CUSTOMER_SERVICE], vis[STAFF]))
    return (menu_nodes, pages, leaves, leaks, blocks, orphans, shared,
            demoted, unknowns, globals_, shape_users)


def synthetic_controls():
    """夹具验判据本身：两向错配各自开火，且各自的"不该报"那一极真的没报。"""
    checks = 0
    failed = 0

    menu_src = '''
var SystemMenuTree = []model.SystemMenuNode{
\t{
\t\tKey: "ops", Title: "运营",
\t\tRoles: []string{"admin", "manager", "sales", "viewer", "staff"},
\t\tChildren: []SystemMenuNode{
\t\t\t{Key: "boardA", Title: "A", Path: "/ops/a", Roles: []string{"admin", "manager"}},
\t\t\t{Key: "boardB", Title: "B", Path: "/ops/b"},
\t\t\t{Key: "boardC", Title: "C", Path: "/ops/c", Roles: []string{"admin"}},
\t\t},
\t},
}
'''
    nodes = parse_menu_tree_from_text(menu_src)
    flat = {n["key"]: n for n in flatten_menu(nodes)}
    checks += 1
    if (visible(flat["boardA"], CUSTOMER_SERVICE) is False
            and visible(flat["boardB"], CUSTOMER_SERVICE) is True
            and visible(flat["boardC"], STAFF) is False):
        print("  ✓ 菜单解析：父子链可见性算对（admin/manager 节点对客服不可见、"
              "无 Roles 节点对客服可见、纯 admin 节点对 staff 不可见）")
    else:
        print("  ✗ 菜单解析：父子链可见性算错")
        failed += 1

    checks += 1
    router_src = """
export default [
  {
    path: '/ops',
    component: Layout,
    children: [
      { path: 'a', component: () => import('@/views/ops/A.vue'), meta: { title: 'A 页' } },
      { path: '/ops/c', component: () => import('@/views/ops/C.vue') }
    ]
  }
]
"""
    found = dict(_route_pairs_from_text(router_src, "/tmp"))
    if found.get("/ops/a") == "@/views/ops/A.vue".replace("@/views/ops/A.vue", "views/ops/A.vue") \
            or "/ops/a" in found:
        print(f"  ✓ 前端路由解析：相对子 path 拼成绝对 /ops/a（实得键 {sorted(found)}）")
    else:
        print(f"  ✗ 前端路由解析：没把相对子 path 拼成绝对路径，实得 {sorted(found)}")
        failed += 1

    checks += 1
    verdict = classify_fixture(
        vis_cs=False, vis_staff=False, tiers_of_shape={"identity"})
    if verdict == "LEAK":
        print("  ✓ 极性对之一：admin-only 菜单 + identity 接口 ⇒ 判 LEAK")
    else:
        print(f"  ✗ 极性对之一：admin-only 菜单 + identity 接口判成 {verdict}，LEAK 没开火")
        failed += 1

    checks += 1
    verdict = classify_fixture(
        vis_cs=False, vis_staff=False, tiers_of_shape={"capability"})
    if verdict == "LEAK":
        print("  ✓ 极性对之一之二：admin-only 菜单 + capability 接口 ⇒ 仍判 LEAK"
              "（这一档就是本脚本生成的表造出来的，把它请出名册＝下一轮把表自己抹掉）")
    else:
        print(f"  ✗ 极性对之一之二：已收口的形状判成 {verdict!r} ⇒ 名册会随防线生效而归零，"
              "--emit-go 下一轮生成空表覆盖真表")
        failed += 1

    checks += 1
    verdict = classify_fixture(
        vis_cs=False, vis_staff=False, tiers_of_shape={"identity"}, in_shell=True)
    if verdict == "GLOBAL":
        print("  ✓ 降级腿之一：同一形状被登录即执行的壳层代码调用 ⇒ LEAK 让位给 GLOBAL"
              "（收它就是把前端打挂）")
    else:
        print(f"  ✗ 降级腿之一：壳层在调的形状仍被判 {verdict!r} ⇒ 会把 /api/menus 这类"
              "全局口写进仅超管表")
        failed += 1

    checks += 1
    verdict = classify_fixture(
        vis_cs=False, vis_staff=False, tiers_of_shape={"identity"},
        has_non_admin_page=True)
    if verdict == "SHARED":
        print("  ✓ 降级腿之二：同一形状被角色可见页面共用 ⇒ 判 SHARED 而不是 LEAK")
    else:
        print(f"  ✗ 降级腿之二：共用形状被判 {verdict!r} ⇒ 收口会打坏那个角色的页面")
        failed += 1

    checks += 1
    verdict = classify_fixture(
        vis_cs=False, vis_staff=False, tiers_of_shape={"identity"},
        in_extra_page=True)
    if verdict == "PENDING":
        print("  ✓ 降级腿之三：形状只被菜单外的路由页调用 ⇒ 判 PENDING 待人工定角色")
    else:
        print(f"  ✗ 降级腿之三：菜单外页面在用的形状被判 {verdict!r} ⇒ 角色可见性"
              "算不出来却直接收口")
        failed += 1

    checks += 1
    verdict = classify_fixture(
        vis_cs=False, vis_staff=False, tiers_of_shape={"super-admin"})
    if verdict == "":
        print("  ✓ 极性对之二：admin-only 菜单 + super-admin 接口 ⇒ 不报（授权本来就一致）")
    else:
        print(f"  ✗ 极性对之二：一致的那种也被报了（{verdict}）⇒ 判据过宽")
        failed += 1

    checks += 1
    verdict = classify_fixture(
        vis_cs=True, vis_staff=True, tiers_of_shape={"super-admin"})
    if verdict == "BLOCK":
        print("  ✓ 另一向：非超管可见菜单 + super-admin 接口 ⇒ 判 BLOCK（页面能开却满屏 403）")
    else:
        print(f"  ✗ 另一向没开火：判成 {verdict!r}")
        failed += 1

    checks += 1
    comment_only = "// 这一行只是注释：GET /api/x 由别的模块调用\nconst q = 1\n"
    if "/api/x" not in RCT.extract_tokens(strip_comments(comment_only, False)):
        print("  ✓ 注释剥离：只出现在 // 注释里的接口串不计为调用点")
    else:
        print("  ✗ 注释没剥：/api/x 在注释里也被算成页面调用面 ⇒ 名册会多出假 LEAK")
        failed += 1

    checks += 1
    url_line = "const hook = 'https://example.com/api/push-hook'\n"
    if RCT.extract_tokens(url_line) == RCT.extract_tokens(strip_comments(url_line, False)):
        print("  ✓ 注释剥离不误伤：scheme:// 开头的 URL 字面量原样保留（真调用点没被截掉）")
    else:
        print("  ✗ 注释剥离过宽：URL 字面量被 // 截断，真调用点丢了（会把该收口的漏掉）")
        failed += 1

    return (1 if failed else 0), checks, failed

    return (1 if failed else 0), checks, failed


# LEAK_TRIGGER_TIERS 触发 LEAK 的档位集合。
#
# "capability" 是**这张表生效后**的档位（RoutePolicyGuard 查生成表拦非超管），
# 它也必须算触发集，理由有两个：
#   - 名册是生成表的输入。收了口就把 capability 排除出 LEAK，下一轮跑 --emit-go
#     会把真表抹成空表（能力表一生效，名册就查不到 identity，rules 归零）；
#   - 收口前后的差别不体现在"该不该进表"，而体现在"进了表没有"，所以名册要分两列读：
#     未收口（tiers 只有 identity）= 真泄露，已收口（tiers 含 capability）= 防线在场。
# super-admin 不在集合里：那一档由硬守卫裁决，不需要菜单授予。
LEAK_TRIGGER_TIERS = {"identity", "capability"}


def classify_fixture(vis_cs, vis_staff, tiers_of_shape,
                     has_non_admin_page=False, in_shell=False, in_extra_page=False):
    """错配判据的唯一实现：audit() 与 --self-check 夹具都走这里。

    写成两份会出问题：夹具验的是那份副本，名册是另一份算出来的，控制绿了不等于名册对。
    降级顺序固定为 壳层 > 角色可见页面 > 菜单外页面 > 判 LEAK：
    前三种都意味着"收口会打到非超管真在用的东西"，只有三种都不命中才允许写进口表。
    """
    admin_only = not (vis_cs or vis_staff)
    if admin_only and tiers_of_shape & LEAK_TRIGGER_TIERS:
        if in_shell:
            return "GLOBAL"
        if has_non_admin_page:
            return "SHARED"
        if in_extra_page:
            return "PENDING"
        return "LEAK"
    if not admin_only and tiers_of_shape == {"super-admin"}:
        return "BLOCK"
    return ""


def parse_menu_tree_from_text(src):
    m = MENU_GO_RE.search(src)
    open_idx = src.index("{", m.end() - 1)
    close_idx = match_brace(src, open_idx)
    return parse_node_list(src[open_idx + 1:close_idx])


def _route_pairs_from_text(src, base_root):
    global root_marker
    root_marker = base_root
    m = re.search(r"\[\s*\{", src)
    start = src.find("[", m.start())
    end = match_bracket(src, start)
    out = []

    def scan(inner, parent_path):
        i, n = 0, len(inner)
        while i < n:
            if inner[i] != "{":
                i += 1
                continue
            close = match_brace(inner, i)
            body = inner[i + 1:close]
            cm = re.search(r"\bchildren\s*:", body)
            own, children_src = body, ""
            if cm:
                s = cm.end()
                while s < len(body) and body[s] not in "[{":
                    s += 1
                closer = match_bracket(body, s) if body[s] == "[" else match_brace(body, s)
                children_src = body[s + 1:closer]
                own = body[:cm.start()] + body[closer + 1:]
            own = strip_nested(own, ["meta"])
            pm = ROUTER_PATH_RE.search(own)
            compm = ROUTER_COMP_RE.search(own)
            rel = unquote(pm.group(1)) if pm else ""
            comp = unquote(compm.group(1)) if compm else ""
            full = join_route_path(parent_path, rel)
            if rel and comp:
                out.append((full, comp))
            if children_src:
                scan(children_src, full or parent_path)
            i = close + 1

    scan(src[start + 1:end], "")
    return out


def live_repo_controls(root, globals_):
    """拿真仓库验"递归走得动、且没走到哪都是"——夹具碰不到这一段。

    三枚控制。前两枚是一对极性：前一枚断 import 递归真的跨过了 `@/api` 注册表（断在
    第一跳时 LEAK 会读成 15、GLOBAL 读成 0，是一份看起来更干净的假名册）；后一枚断壳层
    闭包没有把页面调用面整片吸进来（边界失效时 GLOBAL 会吞掉一切，LEAK 恒 0＝橡皮章）。
    第三枚管的是另一份事实源：路由表解析器对 `export default {…}` 这种单页模块的覆盖面。
    它失效时不会报错，只会凭空多出 ORPHAN 行——"菜单指向的页在前端路由里找不到"，
    而真相是解析器没看见这条路由；假 ORPHAN 会盖住真 ORPHAN，这一档必须自己交代覆盖面。
    """
    checks = 0
    failed = 0

    checks += 1
    store_closure = collect_shapes(
        root, "user-web/src/stores/permission.js",
        forbidden=("user-web/src/router", "user-web/src/views"))
    if "/api/menus" in store_closure:
        print("  ✓ 真仓库递归：permission store 沿 @/api/menu 跨到 /api/menus"
              f"（本闭包 {len(store_closure)} 个形状）")
    else:
        print(f"  ✗ 真仓库递归：从 stores/permission.js 收不到 /api/menus，"
              f"闭包={sorted(store_closure)} ⇒ import 递归断了，名册是假的")
        failed += 1

    checks += 1
    page_only = "/api/glossaries"
    if page_only not in globals_:
        print(f"  ✓ 壳层边界：{page_only} 只在术语页调用，没被算成登录即执行"
              f"（壳层形状共 {len(globals_)} 个）")
    else:
        print(f"  ✗ 壳层边界失效：{page_only} 被算成全局调用面（经 "
              f"{globals_[page_only]}）⇒ 壳层闭包在吞页面，LEAK 会恒 0")
        failed += 1

    # 单对象导出的路由模块（`export default { path: … }`）：菜单里真有节点指向它们，
    # 解析器一旦只认数组，这两页就会双双变成假 ORPHAN。
    checks += 1
    pages = parse_router_pages(root)
    single_object_pages = ("/dingtalk-app", "/whatsapp-cloud")
    missing = [p for p in single_object_pages if p not in pages]
    if not missing:
        print(f"  ✓ 路由解析覆盖面：单对象导出的 {len(single_object_pages)} 页都解析到了"
              f"（前端路由共 {len(pages)} 个页面 path）")
    else:
        print(f"  ✗ 路由解析覆盖面：{missing} 没被解析到 ⇒ 挂在其上的菜单节点会被误报成 "
              f"ORPHAN（假『前端缺页』），真缺页也会一起被这份名册掩盖")
        failed += 1

    return checks, failed


def self_check():
    print("自检：")
    rc, total, failed = synthetic_controls()
    print(f"自检：{total} 枚控制，失败 {failed} 枚")
    return rc, total, failed


def capability_rules(leaks, shared, shape_users):
    """把名册折成"接口面 → 拥有它的菜单 key"的能力规则表。

    只收 LEAK 那一类（消费它的所有菜单页都对两档非超管角色藏着）。SHARED 不收：
    那条形状的另一个消费页本来就是给坐席看的，收口＝先把那页打挂再让人来报障。
    """
    by_pattern = {}
    for r in leaks:
        keys = sorted({k for k, _ in shape_users.get(r[2], ())})
        by_pattern[r[2]] = sorted(set(by_pattern.get(r[2], [])) | set(keys))
    return [(p, by_pattern[p]) for p in sorted(by_pattern)]


GO_TABLE_HEADER = '''// 能力面 ↔ 菜单 key 对照表：RoutePolicyGuard 用它把"这条接口只有该能力可用"
// 交给菜单授权来决定，而不是再维护一份和侧栏各写各的静态表。
//
// 语义：调用方（非超管）被授予 MenuKeys 中**任意一个**菜单节点才放行；
// 一个都没授 ⇒ 403。超管恒放行；库里 role_menu_permissions 整表为空时按
// DefaultMenuKeys 的出厂分配判（与 GET /api/menus 首屏看到的集合一致）。
//
// 入选口径（三关全过才进表，缺一就留在登录档）：
//  1. 消费该形状的菜单页在注册表里对 customer_service / staff 都不可见；
//  2. 没有任何"角色可见的菜单页"共用该形状；
//  3. 该形状不是登录即执行的壳层调用（store/layout/请求封装），也不是
//     菜单外路由页（详情页/嵌入页）在用——这两类一旦收口，打到的是
//     算不出归属角色的请求，只能先修前端归属再说。
//
// 生成命令：python3 scripts/menu-route-role-audit.py --emit-go @@GO_REL@@
// 事实源：菜单注册表 user-server/internal/model/menu.go + live 路由守卫档位导出 + user-web 调用图。
// 本文件不许手改：改了会在下一次跑生成命令时被覆盖，且
// TestRouteGuardChain_CapabilityRulesAreNotDeadLines 会逐条核对它命中的 live 路由。
package model

// MenuCapability 一条能力面规则。Pattern 按段匹配真实请求路径，
// `*` 吃掉恰好一段（与 middleware 里仅超管表用的是同一套段匹配）。
type MenuCapability struct {
	Pattern  string
	MenuKeys []string
}

// MenuCapabilities 生成件：共 @@N@@ 条。
var MenuCapabilities = []MenuCapability{
'''


def render_go_table(rules, go_rel):
    text = GO_TABLE_HEADER.replace("@@GO_REL@@", go_rel).replace("@@N@@", str(len(rules)))
    body = []
    for pattern, keys in rules:
        key_list = ", ".join(f'"{k}"' for k in keys)
        body.append(f'\t{{Pattern: "{pattern}", MenuKeys: []string{{{key_list}}}}},\n')
    return text + "".join(body) + "}\n"


def emit_or_check_go(root, rules, go_rel, check_only):
    """写出生成件；--check-go 模式下只比对，不一致就退非零（生成件漂移必须出声）。"""
    go_path = os.path.join(root, go_rel)
    want = render_go_table(rules, go_rel)
    have = None
    if os.path.exists(go_path):
        with open(go_path, encoding="utf-8") as f:
            have = f.read()
    if check_only:
        if have is None:
            print(f"✗ 能力表缺失：{go_rel} 不在树里，守卫无表可查")
            return 1
        if have != want:
            print(f"✗ 能力表漂移：{go_rel} 与生成结果不一致"
                  f"（菜单/前端/路由改过却没重跑生成命令）")
            return 1
        print(f"✓ 能力表与生成结果逐字一致：{go_rel}（{len(rules)} 条）")
        return 0
    os.makedirs(os.path.dirname(go_path), exist_ok=True)
    with open(go_path, "w", encoding="utf-8") as f:
        f.write(want)
    print(f"已写出能力表：{go_rel}（{len(rules)} 条规则）")
    return 0


def dump_guards_via_go(root, out_path):
    env = dict(os.environ)
    env["ROUTE_GUARD_DUMP_FILE"] = out_path
    env.setdefault("CGO_ENABLED", "0")
    cmd = ["go", "test", "-count=1", "-timeout", "300s",
           "-run", "^TestDumpLiveRouteGuardChains$", "./internal/router"]
    proc = subprocess.run(cmd, cwd=os.path.join(root, "user-server"), env=env,
                          capture_output=True, text=True)
    if proc.returncode != 0 or not os.path.exists(out_path):
        sys.stderr.write("红因（go test 输出尾部）：\n")
        sys.stderr.write("\n".join(proc.stdout.splitlines()[-15:]) + "\n")
        sys.stderr.write("\n".join(proc.stderr.splitlines()[-15:]) + "\n")
        raise SystemExit("守卫档位导出失败：缺产物就退非零，不能把'没读数'读成'没错配'")


def file_md5(path):
    digest = hashlib.md5()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 16), b""):
            digest.update(chunk)
    return digest.hexdigest()


def main():
    ap = argparse.ArgumentParser(description="菜单可见性 × 路由授权档位 错配审计器")
    ap.add_argument("--repo", default=None, help="仓库根（默认取 scripts/ 的上一级）")
    ap.add_argument("--guards", default=None, help="已有的守卫档位 TSV；不给就现跑导出（需测试库）")
    ap.add_argument("--out", default=None, help="名册输出目录")
    ap.add_argument("--emit-go", default=None, metavar="REL",
                    help="把能力面表写到该仓库相对路径（生成件，供 RoutePolicyGuard 查）")
    ap.add_argument("--check-go", default=None, metavar="REL",
                    help="只比对已存在的生成件与本次生成结果，漂移就退非零（CI 用）")
    ap.add_argument("--self-check", action="store_true", help="只验判据与解析器，不读仓库")
    args = ap.parse_args()

    if args.self_check:
        return self_check()[0]

    global root_marker
    root = os.path.abspath(args.repo or RCT.repo_root())
    root_marker = root

    outdir = args.out or os.path.join(root, "docs", "superpowers", "specs", "ledger",
                                      "logs", "MenuRouteAudit",
                                      time.strftime("%Y%m%d-%H%M%S"))
    os.makedirs(outdir, exist_ok=True)
    tsv = args.guards
    if not tsv:
        tsv = os.path.join(outdir, "guard_tiers.tsv")
        dump_guards_via_go(root, tsv)

    tiers, rows = load_guard_tiers(tsv)
    (menu_nodes, pages, leaves, leaks, blocks, orphans, shared,
     demoted, unknowns, globals_, shape_users) = audit(root, tiers)

    # 同一份 LEAK 名册按"这条形状现在拦不拦得住"分两列：形状档位里还有 identity，
    # 说明它至少有一条 live 路由是任何登录角色都能调的（要么没进表，要么进了表却
    # 注册在守卫快照之外）；只剩 capability 才说明生成表真的在裁决它。
    open_leaks = [r for r in leaks if "identity" in r[3]]
    enforced_leaks = [r for r in leaks if "identity" not in r[3]]

    print(f"事实源：路由档位 {len(rows)} 行 · 菜单带 path 节点 {len(menu_nodes)} 个 · "
          f"前端页面 {len(pages)} 条 · 审计到页面归属的菜单节点 {len(leaves)} 个")
    print(f"错配读数：LEAK 共 {len(leaks)} 条 = 未收口 {len(open_leaks)} 条 + "
          f"已由能力表裁决 {len(enforced_leaks)} 条（档位只剩 capability）· "
          f"SHARED（同样是菜单藏着，但接口被别的角色可见页面共用，收口会打坏那页）"
          f"{len(shared)} 条 · "
          f"BLOCK（菜单给了非超管、接口只让超管调）{len(blocks)} 条 · "
          f"ORPHAN（菜单指向的 path 在前端路由里找不到）{len(orphans)} 条 · "
          f"GLOBAL（接口被登录即执行的壳层代码调用，收口会打挂整个前端，禁改）"
          f"{len(demoted)} 条 · "
          f"PENDING（接口被菜单外的路由页调用，角色可见性算不出来，先人工判）"
          f"{len(unknowns)} 条")

    rc, control_total, control_failed = self_check()
    lc_total, lc_failed = live_repo_controls(root, globals_)
    control_total += lc_total
    control_failed += lc_failed
    print(f"自检合计：{control_total} 枚控制，失败 {control_failed} 枚")
    if rc or lc_failed:
        print("❌ 口径自检未过：解析器或判据坏了，名册不写出")
        return 1

    with open(os.path.join(outdir, "leak.tsv"), "w", encoding="utf-8") as f:
        f.write("MENU_KEY\tPAGE_PATH\tAPI_SHAPE\tTIERS_ON_THIS_SHAPE\tOTHER_ADMIN_ONLY_PAGES\n")
        for r in sorted(open_leaks):
            f.write("\t".join([r[0], r[1], r[2], ",".join(r[3]), r[4]]) + "\n")
    with open(os.path.join(outdir, "leak-enforced.tsv"), "w", encoding="utf-8") as f:
        f.write("MENU_KEY\tPAGE_PATH\tAPI_SHAPE\tTIERS_ON_THIS_SHAPE\tOWNING_MENU_KEYS\n")
        for r in sorted(enforced_leaks):
            owners = sorted(k for k, _ in shape_users.get(r[2], ()))
            f.write("\t".join([r[0], r[1], r[2], ",".join(r[3]), ",".join(owners)]) + "\n")
    with open(os.path.join(outdir, "leak-shared.tsv"), "w", encoding="utf-8") as f:
        f.write("MENU_KEY\tPAGE_PATH\tAPI_SHAPE\tUSED_BY_NON_ADMIN_PAGES\n")
        for r in sorted(shared):
            f.write("\t".join(r) + "\n")
    with open(os.path.join(outdir, "block.tsv"), "w", encoding="utf-8") as f:
        f.write("MENU_KEY\tPAGE_PATH\tAPI_SHAPE\tVISIBLE_TO_CS\tVISIBLE_TO_STAFF\n")
        for r in sorted(blocks):
            f.write("\t".join([r[0], r[1], r[2], str(r[3]), str(r[4])]) + "\n")
    with open(os.path.join(outdir, "orphan.tsv"), "w", encoding="utf-8") as f:
        f.write("MENU_KEY\tPAGE_PATH\n")
        for r in sorted(orphans):
            f.write("\t".join(r) + "\n")
    with open(os.path.join(outdir, "global-demoted.tsv"), "w", encoding="utf-8") as f:
        f.write("MENU_KEY\tPAGE_PATH\tAPI_SHAPE\tSHELL_ENTRY_FILE\n")
        for r in sorted(demoted):
            f.write("\t".join(r) + "\n")
    with open(os.path.join(outdir, "pending-nonmenu-page.tsv"), "w", encoding="utf-8") as f:
        f.write("MENU_KEY\tPAGE_PATH\tAPI_SHAPE\tNON_MENU_PAGE_PATHS\n")
        for r in sorted(unknowns):
            f.write("\t".join(r) + "\n")

    head = subprocess.run(["git", "-C", root, "rev-parse", "HEAD"],
                          capture_output=True, text=True).stdout.strip() or "UNKNOWN"
    tool = os.path.abspath(__file__)
    with open(os.path.join(outdir, "00-provenance.log"), "w", encoding="utf-8") as f:
        f.write("\n".join([
            f"基线字节：仓库 HEAD {head}",
            f"取证器 {os.path.relpath(tool, root)} md5 {file_md5(tool)}",
            f"事实源 {os.path.relpath(os.path.abspath(tsv), root)} md5 {file_md5(tsv)}",
            f"复现命令：python3 {' '.join([os.path.relpath(tool, root)] + sys.argv[1:])}",
            f"读数：档位 {len(rows)} 行 · 菜单 path 节点 {len(menu_nodes)} · 页面 {len(pages)} · "
            f"归属节点 {len(leaves)} · LEAK {len(leaks)}（未收口 {len(open_leaks)}／"
            f"已收口 {len(enforced_leaks)}）· SHARED {len(shared)} · "
            f"BLOCK {len(blocks)} · ORPHAN {len(orphans)} · GLOBAL {len(demoted)} · "
            f"PENDING {len(unknowns)}",
            f"壳层形状 {len(global_api_shapes(root))} 个 · 菜单外路由页形状另计",
            f"口径自检：{control_total} 枚控制，失败 {control_failed} 枚",
            "限制：可见性按首空表 seed 折算，库里 role_menu_permissions 被人工改过则真值在库里",
            f"落盘时间：{time.strftime('%Y%m%d-%H%M%S%z')}",
        ]) + "\n")
    print(f"名册输出目录：{outdir}")

    rules = capability_rules(leaks, shared, shape_users)
    if args.emit_go:
        return emit_or_check_go(root, rules, args.emit_go, check_only=False)
    if args.check_go:
        drift_rc = emit_or_check_go(root, rules, args.check_go, check_only=True)
        if open_leaks:
            # CI 门的第二半：表没漂移还不够，还得证明**每一条**泄露面都在这张表里被裁决过。
            # 只查漂移的门会这样骗人：把一条注册挪到守卫快照之前 ⇒ 那条口的档位从
            # capability 退回 identity ⇒ 它仍留在名册里、表里也仍有这一行 ⇒ cmp 逐字相同，
            # 门照样绿，而这条口的防线已经没了。
            print(f"✗ 还有 {len(open_leaks)} 条泄露面没被能力档裁决（档位里仍有 identity）："
                  f"要么补生成表，要么它注册在 RoutePolicyGuard 快照之外")
            for r in sorted(open_leaks)[:20]:
                print(f"    {r[2]}（菜单节点 {r[0]}，档位 {','.join(r[3])}）")
            return max(1, drift_rc)
        return drift_rc
    if rules:
        print(f"能力表本次可生成 {len(rules)} 条（未带 --emit-go/--check-go，只报数不写盘）")
    return 0


if __name__ == "__main__":
    sys.exit(main())
