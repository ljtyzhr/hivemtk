# UI 拟人点击测试进度（任务四）

> 逐页打开 → 点击页面所有元素/表单/交互 → 比对控制台报错与 Network 里的 API 响应。
> 进度表：`router-list.md`（244 条页面路由）。状态图例见该文件头部。
> 本轮结论：**244 条路由全部走完**（242 `[x]` 通过 + 2 `[~]` 设计内拒绝），两种部署形态都过。

## 环境（本轮实测口径）

| 形态 | 后端 | 前端 | 站点标识 |
|---|---|---|---|
| SaaS 多租户 | `127.0.0.1:8214`（`TENANT_MODE=on`） | `127.0.0.1:8290` | `tenant_mode=true` / `edition=saas` |
| 私域独立部署 | `127.0.0.1:8304`（`TENANT_MODE=off`） | `127.0.0.1:8291` | `tenant_mode=false` / `edition=opensource` |

- 后端由 `user-server/cmd/api` 源码构建（`/tmp/hv-api`），两种形态各起一份，DB `127.0.0.1:8202`
- 前端：`user-web` vite，分别用 `vite.local.config.js`（8290→8214）/ `vite.standalone.config.js`（8291→8304）
  （两个本地配置不进版本库，已写入 `.git/info/exclude`）
- 浏览器：复用主浏览器登录态（CDP `127.0.0.1:9223`，profile `chrome-main-profile`），不新建会话
- 登录账号：`uit_admin` / `Hive@Test123`（id=26，tenant_id=1，agent_id=26）

## 启动命令

```bash
cd user-server
set +H; CPW=$(cat /tmp/hv_dbpass)
nohup env PORT=8214 TENANT_MODE=on  GIN_MODE=release APP_ENV=development \
  DB_HOST=127.0.0.1 DB_PORT=8202 POSTGRES_PASSWORD="$CPW" \
  ALLOWED_WS_ORIGINS="http://127.0.0.1:8290,http://127.0.0.1:8291" /tmp/hv-api > /tmp/hv-on.log 2>&1 &
nohup env PORT=8304 TENANT_MODE=off GIN_MODE=release APP_ENV=development \
  DB_HOST=127.0.0.1 DB_PORT=8202 POSTGRES_PASSWORD="$CPW" \
  ALLOWED_WS_ORIGINS="http://127.0.0.1:8290,http://127.0.0.1:8291" /tmp/hv-api > /tmp/hv-off.log 2>&1 &

cd ../user-web
nohup npx vite --config vite.local.config.js       > /tmp/vite-8290.log 2>&1 &
nohup npx vite --config vite.standalone.config.js   > /tmp/vite-8291.log 2>&1 &
```

## 已知无害噪声（不计入缺陷）

| 现象 | 判定 |
|---|---|
| `[SW] 注册失败 (不影响功能): Unexpected token '<'` | vite dev 下 `sw.js` 拿到 index.html；仅 dev 存在 |
| `[web-vitals][debug]` | 埋点调试输出 |
| `ElementPlusError: type.text is about to be deprecated` | Element Plus 组件库弃用告警 |
| 页面切换瞬间新旧两页同时在 DOM 里 | `<transition name="fade">` 正常离场动画（300ms），非残留 |
| `GET /api/ai-agents/75` 404 | 该智能体已被软删（deleted_at），404 是正确口径 |

## 已发现问题与处置

| # | 页面/路由 | 现象 | 根因 | 处置 |
|---|---|---|---|---|
| 1 | 全站（坐席实时通知） | 控制台持续 `WebSocket connection to 'ws://…/api/ws/agent?…' failed`，后端 `GET /api/ws/agent` 记 **403 Forbidden**（gorilla `CheckOrigin` 拒绝），前端陷入重连循环 | WS Origin 白名单（`ALLOWED_WS_ORIGINS` / `config.yaml platform.allowed_ws_origins` / 默认表）按部署域逐个维护，**所有部署脚本都没有设过该变量**；而同源部署（`VITE_API_BASE_URL=/`，前端与 user-server 同一反代层）下浏览器握手必然带 `Origin=<前端站点域>`（如 `https://work.xapptool.cn`），白名单漏配 ⇒ 同源被拒。访客 WS 早已有同源放行（`visitor_handler.go`），坐席 WS 与聊天流式 WS 却没有 —— 口径不一致 | **已修**：新增 `config.IsSameOriginWS`，同源一律放行；坐席 WS（`internal/websocket/handler.go`）与聊天流式 WS（`internal/controller/chat_ws.go`）统一口径；跨源仍走白名单，冒牌域名仍拒。3 个测试文件覆盖（`config/ws_same_origin_test.go`、`websocket/agent_origin_test.go`、`controller/chat_ws_origin_test.go`） |
| 2 | `/chat/embed/:channel_ref` | 访客客服挂件点「关闭」后被推进 **/login**（登录表单嵌在第三方站点的 iframe 里，且访客本就没有账号） | `onClose` 里 `router.replace({name:'NotFound'})`，而 NotFound 路由没有 `public: true`，守卫把未登录访客推进 /login | **已修**：NotFound 路由加 `meta.public = true`（404 是静态页、无租户数据；公开标记只影响未登录时是否放行） |
| 3 | `/asset-bundle/playground/:aid` | 「🧪 本地模型沙箱」恒失败：控制台 `Connecting to 'http://localhost:11434/v1/chat/completions' violates … Content Security Policy "connect-src 'self' https: wss:"` | 首页 CSP 的 `connect-src` 不含 `http:`（DEVELOPMENT.md 记载为**有意**收紧：明文跨源一律拦掉），而沙箱是**开发者本机**直连 Ollama/vLLM 的调试工具 ⇒ 功能与 CSP 口径天生冲突 | **已修**：`connect-src` 增补 `http://localhost:* http://127.0.0.1:*`（**只放行回环地址**，对外明文 http 仍被拦）。实测控制台从「violates CSP」变为 `ERR_CONNECTION_REFUSED`（本机没起 Ollama，浏览器已放行） |
| 4 | `/knowledgeBase/edit/:id` | 路由名/标题都是「编辑知识库」，但深链进去只看到一屏列表，编辑框不打开（`route.params.id` 被静默丢弃） | `views/knowledgeBase/List.vue` 的 `onMounted` 只处理了 `KnowledgeBaseCreate` 分支，漏了 `KnowledgeBaseEdit` | **已修**：`onMounted` 增加 Edit 分支 → `openEditByRouteId()`：按路由 id 单条读取（`getKB`）并打开编辑框，必要时把列表切到该 KB 的类型分页 |
| 5 | `/bridge/extension` | 私有部署（`TENANT_MODE=off`）也显示「租户短码必填」并要求「填入租户短码」—— 私域部署根本没有租户短码这个概念 | 页面无条件渲染 SaaS 专属提示 | **已修**：按 `/api/public/config` 的 `tenant_mode` 分形态显示（与 Login.vue 同一口径）。实测：SaaS 显示租户短码指引，私域不显示 |
| 6 | `/bridge/extension` | 404 是**设计内状态**（后端契约：200=有包 / 404=未接入平台或未上架 / 503=平台侧失败），页面已渲染常驻说明，拦截器却再弹一条「请求地址不存在」 | 读口没带 `_silent` | **已修**：`getBridgeExtensionPackage` 改用 `http.get(url, undefined, { _silent: true })`（与 followup/orderDraft 同口径） |
| 7 | `/knowledgeBase/list`（三个类型标签页） | **类型过滤从未生效**：RAG / FAQ / SOP 三个标签页拿到的都是同一份全量列表（切标签看不到任何变化，用户只会以为"库里就这些"） | 前端发 `?kb_type=faq`，后端读的是 `?type=`（`controller/knowledge_base.go` 的 `@Param type` 也是这个口径）⇒ 参数名对不上，过滤条件一直是空 | **已修**：后端同时接受 `type`（文档口径）与 `kb_type`（兼容旧前端）；前端改回发 `type`。实测 RAG 标签 2 条、FAQ 标签 4 条，互不串味 |
| 8 | 全站（点击劫持口径） | `index.html` 注释宣称 `frame-ancestors 'none'` 防护点击劫持，实际 meta CSP 里**没有** frame-ancestors 指令；且按 CSP 规范 frame-ancestors 在 **meta 交付**的 CSP 中被浏览器忽略，meta 只能收紧不能放宽 ⇒ `main.js` 里那段按路由改写 meta 的代码一直是**空转** | 既有实现（本次未引入）。点击劫持防护要靠**响应头**，且 `/chat/embed` 需单独放行 | **注释已改真**（不再宣称有防护），`main.js` 的空转代码删除并留说明。**响应头层面的防护仍是缺口**，见文末「遗留缺口」 |

## 遗留缺口（不在本任务范围，已记录）

| 缺口 | 说明 | 建议 |
|---|---|---|
| 点击劫持防护（frame-ancestors） | `index.html` 的 meta CSP 里没有 frame-ancestors 指令，浏览器也忽略 meta 里的这一条 ⇒ 实际**没有**点击劫持防护。`main.js` 那段按路由改写 meta 的代码是空转（已删）。真要防护必须由服务端按路由下发响应头 `Content-Security-Policy: frame-ancestors …`（`/chat/embed` 需单独放行） | 独立任务：服务端加响应头 + 回归「第三方站点能嵌 /chat/embed、其余页面嵌不进去」 |
| `/api/ws/chat`（聊天流式 WS） | `internal/router/ws.go` 的 `RegisterWSRoutes` **全树零调用方** ⇒ 该端点从未注册进引擎（落到 SPA NoRoute 兜底）。本次只把它的 Origin 口径补齐（与坐席 WS 对齐），**没有**把它变成可用端点 | 要上线需先补鉴权（`HandleChatWS` 目前只校验 session_id/customer_id 非空） |

## 设计内拒绝（`[~]`，非缺陷）

| 路由 | 现象 | 判定 |
|---|---|---|
| `/followups/:id` | `GET /api/followups/{today,pending,overdue}` → **503**「跟进服务未装配（草稿/旅程运行时未启用），非"没有跟进"」 | 特性开关未装配，页面按 503 与"没数据"分别措辞（api/followup.js 注释明确） |
| `/dashboard/drafts/:id` | 页面渲染「订单草稿底座不可用（`FF_LTC_ORDER_DRAFT_DB` 为 off）」 | 同上，档位只在装配期读一次 |

## 本地环境限制（非产品缺陷）

| 现象 | 说明 |
|---|---|
| 沙箱/聊天 LLM 报「AI 服务暂时不可用」 | 本机 LLM 推理端 `127.0.0.1:8207` 未启动（provider `local-mlx` connection refused），`sensenova` 在本网络被熔断（`reason=circuit_open`）。**两种形态表现一致**，不是形态差异。SaaS 与独立部署用**同一句**文本实测得到**同一条**秒回（FAQ 命中），排除"形态差异"误判 |
| 渠道卡片统计页请求 `https://example.com/` | 卡片图片地址本身是示例站，属数据不是缺陷 |

## 逐页记录（节选）

| # | 页面路由 | 形态 | 结论 |
|---|---|---|---|
| 1 | `/login` | 两形态 | 通过（登录 → 跳 `messageHub/list`） |
| 2 | `/messageHub/list` | 两形态 | 通过 |
| 3 | `/customerSession/list` | 两形态 | 通过 |
| 4 | `/chat/embed/:channel_ref` | 两形态 | 通过（建会话 → 发消息 → AI 回复 → 历史会话 → 关闭落 404 页） |
| 5 | `/:pathMatch(.*)*` | 两形态 | 通过（404 页两个按钮都点过：返回上一页 / 回到首页） |
| 6 | `/aiAgent/edit/:id` | 两形态 | 通过（改描述 → 保存 → PUT 200 → 跳列表） |
| 7 | `/asset-bundle/playground/:aid` | SaaS | 通过（加 Few-Shot、校验、织布 200、沙箱推理按预期提示） |
| 8 | `/asset-bundle/merchant/:aid` | SaaS | 通过 |
| 9 | `/asset-market/detail/:id` | SaaS | 通过（空数据降级） |
| 10 | `/bridge/extension` | 两形态 | 通过（修 #5 #6 后复测） |
| 11 | `/browser-automation/tasks/:id` | SaaS | 通过 |
| 12 | `/browser-automation/sessions/:id` | SaaS | 通过（导出审计包 → `/export` 200 + toast「审计包已导出」） |
| 13 | `/chatChannel/edit/:id` | SaaS | 通过 |
| 14 | `/chatChannel/install-guide/:id?` | SaaS | 通过（安装代码块完整，含 AppKey 与服务端地址） |
| 15 | `/douyin-card-stats/:id`、`/kuaishou-card-stats/:id`、`/tiktok-card-stats/:id`、`/xianyu-card-stats/:id`、`/xiaohongshu-card-stats/:id` | SaaS | 通过（5 张卡片统计页逐个开） |
| 16 | `/faq/editor/:id` | SaaS | 通过 |
| 17 | `/knowledgeBase/edit/:id` | 两形态 | 通过（修 #4 后复测：深链直开编辑框） |
| 18 | `/module-market/detail/:key` | SaaS | 通过（模块详情、包含页面清单） |
| 19 | `/reachPipeline/editor/:id?` | SaaS | 通过（节点库 / 画布） |
| 20 | `/sop-template/editor/:id` | SaaS | 通过 |
| 21 | `/workflow-orchestrator/editor/:workflow_id`、`/execution/:id` | SaaS | 通过 |
| 22 | `/followups/:id`、`/dashboard/drafts/:id` | SaaS | 设计内 503，页面措辞正确（`[~]`） |

## 测试纪律（踩过的坑，勿重复）

1. **a11y 快照会滞后**：`<component :is>` 异步组件未 resolve 时，`take_snapshot` 可能还是上一页的内容。**判断渲染结果一律用 `evaluate_script` 读 DOM**（`document.querySelector('.el-main').innerText` + 子节点 className），不要只信快照。
2. **弹窗会吃掉点击**：页面顶部常驻「新增待归因坏例」弹层（`el-overlay`）会盖住主区，先点「知道了」再点页面元素；且点完要**重新取快照**再取 uid（旧 uid 会指向已卸载的节点）。
3. **过渡动画期新旧同屏**：`<transition name="fade">` 未设 `mode="out-in"`，切页后 ~300ms 内两页同在 DOM，属正常；确认渲染结果要等动画结束（读 `.el-main` 下**不带** `fade-` 前缀的子节点）。
4. **元素必须能交互**：`evaluate_script` 里的 `el.click()` 不会触发 Vue 的绑定（那是 DOM 事件，Vue 监听的是真实事件序列）——用 `click` 工具；`fill` 之前先确认输入框当前 uid。
5. **grep 别扫 dist/**：`user-web/dist`、`embed-sdk/dist` 里的构建产物会让输出爆炸，一律限定 `src/`。
