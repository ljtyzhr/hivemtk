# HiveMTK 官网 / 营销落地页（website）— 菜单页面规格清单

> 生成日期：2026-07-24 ｜ 最后更新：2026-10-18 ｜ 技术栈：Vue 3 + Vue Router(history) + Vite 8 + vue-i18n 9（中 / 英 / 日 / 阿语）
>
> 站点品牌：`HiveMTK · 私域 AI 营销操作系统`（`src/config/content.js` `brand`）。导航配置：`content.js` 的 `nav`；页面文案集中在 `content.js`，改文案无需改组件。
>
> 定位（2026-10-18 修订，与产品实况对齐）：**AGPL-3.0 开源 + 私有化部署**——代码与 7 个基础功能模块免费；17 个增值功能模块在系统内**按需订阅**（年费 / 按量预付费）；支持离线单机与 SaaS 多租户双形态。官网 CTA 仍只用「立即部署 / 克隆仓库 / 获取源码」，不提供注册入口。事实源见 `TERMINOLOGY.md` 与 `src/i18n/modules/contentExtra6.js`。
>
> 每个路由带 SEO `meta`（title / description / keywords），由 `router/index.js` `afterEach` 动态注入 `<head>`。

---

## 一、顶部导航结构（`content.js` → `nav`）

| 导航项 | 路径 | 视图组件 | 类型 |
| --- | --- | --- | --- |
| 首页（品牌 Logo `HiveMTK`） | `/` | `views/HomePage.vue` | route |
| 核心功能 | `/features` | `views/FeaturesPage.vue` | route |
| 功能架构 | `/modules` | `views/ModulesPage.vue` | route |
| 按需订阅 | `/pricing` | `views/PricingPage.vue` | route |
| 工程能力 | `/toolchain` | `views/ToolchainPage.vue` | route |
| 工作流 | `/workflow` | `views/WorkflowPage.vue` | route |
| 常见问题 | `/faq` | `views/FaqPage.vue` | route |
| 部署指南 | `/deploy` | `views/DeployPage.vue` | route |
| 安装文档 | `/docs` | `views/DocsPage.vue` | route |
| 右侧 CTA「立即部署」 | `/deploy` | `views/DeployPage.vue` | route |

> 导航共 8 项 `nav.links` + 1 项 `nav.cta`，全部为 `type: 'route'`。

---

## 二、各页面内容规格

### 1. 首页 `/` — `views/HomePage.vue`

按顺序拼装 5 个区块组件（数据均来自 `content.js`）：

1. **HeroSection**（`hero`）
2. **FeaturesSection**（`featuresSection`）
3. **ArchitectureDiagram**（系统架构示意图）
4. **ToolchainSection**（`toolchainSection`）
5. **WorkflowSection**（`workflowSection`）

#### Hero 区域（`content.js` → `hero`）

- **eyebrow 标签（2 个）**：
  - `HiveMTK · 私域 AI 营销操作系统`（type: `primary`）
  - `AGPL-3.0 开源 · 私有化部署`（type: `accent`）
- **主标题**（两段拼接，`gradientIndex: 1` 表示第二段渐变高亮）：
  - 第一段：`七端打透 · AI 真自主 · `
  - 第二段：`数据封死在域内`
  - 完整文案：**七端打透 · AI 真自主 · 数据封死在域内**
- **副描述**：HiveMTK 是把七端社媒（抖音 / 快手 / 小红书 / 闲鱼 / TikTok / 企微 / 邮件）、ReAct 自主智能体（42 工具）、零出域数据安全三件事同时做透的私域营销操作系统。
- **主 CTA**：「立即部署」→ `/deploy`
- **次 CTA**：「了解核心功能」→ `/features`
- **4 个数据指标**（`stats`）：
  | 值 | 标签 |
  | --- | --- |
  | `85%+` | AI 自动承接率 |
  | `<3s` | 多账号消息时延 |
  | `200%` | 人均产能提升 |
  | `9` | 全渠道触达适配 |
- **4 张可视化卡**（`visualCards`）：
  1. AI 自动谈单 — 意图识别 · 异议处理 · 逼单邀约
  2. 多账号聚合 — 企微 + 个微统一收件箱
  3. 销冠 SOP 智能体 — 可视化编排 · 自主决策
  4. 客户 CDP — 360 画像 · 意向打分 · 旅程地图

#### 首页其余区块

- **FeaturesSection**：13 项核心功能卡（详见 `/features`）
- **ArchitectureDiagram**：系统架构示意图（用户端 vs 平台端）
- **ToolchainSection**：6 项工程能力（详见 `/toolchain`）
- **WorkflowSection**：6 步工作流（详见 `/workflow`）

---

### 2. 核心功能 `/features` — `views/FeaturesPage.vue`

- 复用 `FeaturesSection` 组件 + `WhyHiveMTKSection` + **页尾功能架构 / 按需订阅引流带**（`content.js` → `featuresBand`）。
- **区块头部**（`content.js` → `featuresSection`）：
  - tag：`核心功能`
  - title（两段，`gradientIndex: 1`）：`不是又一个 SCRM，` / `是真正会卖货的 AI`
  - subtitle：覆盖从七端多账号聚合、AI 谈单、销冠 SOP、客户 CDP、全渠道触达到数据驾驶舱的完整链路，94 个业务模块 + 45 个智能体工具。
- **13 项功能卡**（`features`，每项含 `icon` / `title` / `industries` / `pain` / `solution` / `special`）：

  | # | 功能名 | 适用行业 |
  | --- | --- | --- |
  | 1 | 多账号聚合中枢 | 招商加盟 / 医美连锁 / 教育培训 |
  | 2 | AI 自动谈单引擎 | 高客单服务 / B2B 销售 / 本地生活 |
  | 3 | 销冠 SOP 智能体 | 保险经纪 / 房产中介 / 家居定制 |
  | 4 | 客户 CDP / 360° 画像 | 耐消零售 / 汽车后市场 / 健康管理 |
  | 5 | 销售意向识别与打分 | 在线教培 / 家装建材 / 金融理财 |
  | 6 | 全渠道触达引擎 | 电商品牌 / 内容创作者 / 跨境出海 |
  | 7 | 私域自动化运营 | 快消品牌 / 母婴亲子 / 美业门店 |
  | 8 | 沉睡客户激活引擎 | 会员制零售 / 医疗美容 / 健身教育 |
  | 9 | 统一话术与素材中心 | 连锁门店 / 教培机构 / 医美集团 |
  | 10 | 数据驾驶舱 | 集团总部 / 区域代理 / 品牌方 |
  | 11 | 权限控制与合规 | 金融 / 医疗 / 政企 |
  | 12 | GEO 智能优化引擎 | 出海品牌 / B2B 获客 / 内容营销 |
  | 13 | Telegram 群增长引擎 | 跨境出海 / 外贸获客 / 加密 Web3 / 社群运营 |

- **页尾引流带**（`featuresBand`）：标题两段 `不止 13 张功能卡，` / `还有模块化的功能架构`；两颗按钮 → `/features` 旁的 `/modules` 与 `/pricing`。

---

### 2a. 功能架构 `/modules` — `views/ModulesPage.vue`（2026-10-18 新增）

> 事实源：`user-server/internal/model/module_bindings.go`（24 模块 × 212 菜单节点），与源码核对日期 2026-10-18。数据定义在 `content.js` → `moduleCatalog` / `modulesPage`。

页面区块（自上而下）：

1. **PageHeader**：kicker `功能架构`；标题两段 `24 个功能模块，` / `212 个菜单节点`。
2. **指标条**（`modulesPage.stats`）：24 功能模块 / 212 菜单节点 / 45 智能体工具 / 2 部署形态。
3. **双形态**（`modesTitle` + `modes`）：离线单机（`TENANT_MODE=off`，默认，注册返回 4104、`PLATFORM_ENABLED=false` 行为）与 SaaS 多租户（`TENANT_MODE=on`，注册即开户 + 行级隔离）。
4. **基础模块**（7 张卡）：系统基座 / 客户中心 / 会话工作台 / 知识库 / 资产与话术 / 自有模型接入 / 平台对接——含摘要、覆盖页面数、依赖。
5. **增值模块**（17 张卡）：AI 销冠 ¥199/年 至 短链活码 ¥9/年 + 平台模型算力（0 元开通 · 按量预付费）——含摘要、价格、计费周期、覆盖页面数、依赖；16 个年费模块合计 ¥999/年（平价路线，2026-10-18 调整）。
6. **运行机制**（`mechanics` 4 条）：模块闸（登录态 → 模块闸 → 角色权限，403 + `module_not_enabled`）/ 菜单与任务过滤 / 依赖闭包 / 同步与降级（10 分钟心跳、本地镜像降级）。
7. **事实源注记**（`sourceNote`）+ 页尾 CTA（→ `/features`、`/pricing`）。

---

### 2b. 按需订阅 `/pricing` — `views/PricingPage.vue`（2026-10-18 新增）

> 事实源：`.specify/002-modular-subscription/spec.md` + `user-server/internal/service/module_market.go` + `model/module_bindings.go`。文案定义在 `content.js` → `pricingPage`。

页面区块（自上而下）：

1. **PageHeader**：kicker `按需订阅`；标题两段 `开源基座免费，` / `增值能力按需订阅`。
2. **三层模型卡**（`models` 3 张）：开源基座（AGPL-3.0 免费）/ 基础功能模块（7 个免费常开）/ 增值功能模块（17 个按年订阅 + 算力按量）。
3. **增值模块价目表**（`moduleCatalog` 中 17 个 `paid` 模块）：模块 / 能力 / 订阅价 / 计费（年付 · 按量预付费）/ 覆盖页面。
4. **平价路线**（`budgetTitle` + `budgetPaths` 3 条 + `budgetPoints` 4 条 + `budgetNote`，2026-10-18 新增）：
   - ¥0 起步线：离线 + 7 基础模块 + 自带 Key，订阅成本为零；
   - ¥128/年 轻量 AI 线：AI 智能体单模块（依赖全为免费基础模块，无付费依赖补齐），可选 +短链活码 ¥9/年 → ¥137/年；
   - ¥999/年 全量线：16 个年费模块算术合计（非套餐价，封顶在 ¥1,000 以内），平台算力按量另计；中间档销冠双件套 ¥327/年。
   - 论证 4 条：基座永远免费 / 单买不捆绑 / 模型成本自控 / 二开不加钱；`budgetNote` 注明金额口径与核对日期。
5. **订阅流程**（`flow` 5 步，一期口径）：浏览市场 → 选择并确认（依赖闭包弹窗）→ 提交订单（幂等 + 联系方式）→ 线下付款（离线时空态）→ 自动开通。
6. **模型算力双轨**（`llmTracks` 2 张）：自有模型接入（免费只统计）/ 平台模型算力（0 元开通 · token 预付费 · 输入输出分别计价）+ **离线部署口径**卡。
7. **SaaS 功能**（`saasTitle` + `saasPoints` 3 卡）：注册即开户 / 数据行级隔离 / 双形态一套代码；说明平台端（可选本地组件）管理面。
8. **订阅常见问题**（`faqs` 5 条，含预算有限最小路线问答）+ 页尾 CTA（→ `/modules`、`/deploy`）。

---

### 3. 工程能力 `/toolchain` — `views/ToolchainPage.vue`

- 拼装 `ArchitectureDiagram` + `ToolchainSection` 两个组件。
- **区块头部**（`content.js` → `toolchainSection`）：
  - tag：`工程能力`
  - title（两段，`gradientIndex: 1`）：`不是玩具 Demo，` / `是能扛生产的工程`
  - subtitle：从消息中台、大模型路由、私域自动化到可观测性，每一层都为真实业务量设计，支持私有化部署与水平扩展。
- **6 项工程能力卡**（`toolchain`，每项含 `icon` / `name` / `desc`）：

  | # | 能力名 | 描述 |
  | --- | --- | --- |
  | 1 | 消息中台 MQ | 自研消息中间件，聚合多账号消息，聚合时延 < 3s |
  | 2 | 大模型路由网关 | 按场景动态路由 DeepSeek / 通义 / GPT-4o / 智谱 GLM |
  | 3 | 私域自动化引擎 | SOP 编排、定时触达、条件分支、A/B 测试 |
  | 4 | 客户 CDP | OneID 归并、360° 画像、RFM 分层与意向打分 |
  | 5 | 数据驾驶舱 | 全链路指标看板与异常预警，决策周期从周级缩短到分钟级 |
  | 6 | 可观测 Pipeline | 触达全链路 TraceID，限流 / 重试 / 降级 / 审计 / 计费 9 步保障 |

---

### 4. 工作流 `/workflow` — `views/WorkflowPage.vue`

- 复用 `WorkflowSection` 组件。
- **区块头部**（`content.js` → `workflowSection`）：
  - tag：`工作流`
  - title（两段，`gradientIndex: 1`）：`一条线索，` / `从接住到成交的自动化`
  - subtitle：AI 在每一步自动承接、判断与推进，最终把高意向客户交给真人销售。
- **6 步流程**（`steps`，每步含 `icon` / `title` / `desc`）：

  | # | 步骤 | 描述 |
  | --- | --- | --- |
  | 1 | 线索接入与识别 | 多渠道线索统一接入，AI 实时识别意向等级并自动打标签、更新画像 |
  | 2 | AI 自动谈单 | 智能体 7×24 承接咨询，完成寒暄、探需、异议处理与逼单邀约 |
  | 3 | SOP 智能推进 | 按客户阶段自动执行对应 SOP 分支，把普通销售带入销冠节奏 |
  | 4 | 内容自动生成 | 销冠人设的朋友圈文案、海报、短视频脚本一键生成并定时发布 |
  | 5 | 人工介入成交 | 高意向客户由真人销售 1v1 推进成交，中低意向客户进入自动培育池 |
  | 6 | 复购与激活 | 沉睡客户触发自动激活 SOP，成交客户进入复购旅程，形成增长闭环 |

---

### 5. 常见问题 `/faq` — `views/FaqPage.vue`

- 头部标签 + 标题 + 副标题（`content.js` → `faqSection`）；下方 `glass-card` 列表渲染 `faqSection.faqs`。
- **区块头部**：
  - tag：`常见问题`
  - title（两段，`gradientIndex: 1`）：`你可能关心的` / `几个问题`
  - subtitle：HiveMTK 七端打透 + AI 真自主 + 数据封死在域内，落地私域最常被问到的问题一次说清。
- **12 条 FAQ**（`faqs`，每条含 `q` / `a`）：

  | # | 问题 |
  | --- | --- |
  | 1 | 和传统 SCRM / 企微 SaaS 有什么区别？ |
  | 2 | 支持哪些大模型？ |
  | 3 | 什么是 GEO？和 SEO 有什么区别？ |
  | 4 | 数据安全如何保障？ |
  | 5 | 需要技术团队才能部署吗？ |
  | 6 | 七端是哪七端？ |
  | 7 | 和现有 CRM / 订单系统能打通吗？ |
  | 8 | AI 自动回复会不会被平台封号？ |
  | 9 | 开源协议是什么？商用有什么限制？ |
  | 10 | 如何开始使用？ |
  | 11 | 支持多语言 / 出海业务吗？ |
  | 12 | 后续会持续更新吗？ |

  > 订阅相关 4 条问答不在此页，见 `/pricing` 页 `pricingPage.faqs`。

---

### 6. 部署指南 `/deploy` — `views/DeployPage.vue`（页内交互，非仅静态）

页面数据由组件内部定义（`deployMethods` / `dockerSnippet` / `sourceSnippet`），仅联系微信号通过 `useSiteContact` 动态加载。

- **Hero 区域**：
  - 标签：`部署指南`
  - 标题（两段，第二段渐变）：`4 步完成部署` / `开箱即用`
  - 副标题：基于 AGPL-3.0 开源协议，无授权码、无版本下载：代码与 7 个基础功能模块免费，17 个增值功能模块可在系统内按需订阅。
  - 澄清提示：所有源码与配置均在 GitHub / Gitee 仓库直接克隆，AGPL-3.0 要求修改后的网络服务代码也必须开源。
- **部署方式卡（3 张，可点选切换 `activeTab`）**：

  | key | 标题 | 描述 | CTA | 链接 |
  | --- | --- | --- | --- | --- |
  | `docker` | Docker 一键部署 | 推荐方式：环境隔离、可一键启停、无需手动配置依赖 | 前往部署文档 | `/docs` |
  | `source` | 源码部署 | 直接 clone 仓库，本地启动后端 + 前端，适合二次开发 | 查看源码 | `https://gitee.com/xhpmayun/hivemtk`（外链） |
  | `doc` | 完整安装文档 | Docker 部署、源码部署、FRP 私域穿透、数据库迁移与配置说明 | 阅读安装文档 | `/docs` |

- **命令片段区**：Tab 切换 Docker / 源码，展示对应 shell 片段，「复制」按钮写剪贴板。
  - **Docker 片段**：克隆仓库 → `cp .env-example .env` 改密钥 → `make install`（拉起 PG/Redis 容器与推理栈）→ `make dev` 起 user-server → 访问 `http://localhost:8204`；超管账号在首次 `/setup` 向导自设（用 `scripts/bootstrap.sh` 安装时由 `SEED_PASSWORD` 写入，用户名 `admin`）。
  - **源码片段**：克隆仓库 → 后端 `user-server`（Go 1.25，端口 `8204`）→ 前端 `user-web`（Node 18+，`npm run dev` 端口 `8211`，取自 `user-web/vite.config.js`）。
- **公开提示卡**：AGPL-3.0 开源，零授权门槛——无授权码、无版本下载：源码全量免费，基础功能模块免费常开，增值功能模块按需订阅（详见 `/pricing`）；源码与文档在 GitHub / Gitee 公开托管。
- **联系作者卡**：展示微信号（`useSiteContact` 动态加载，回退占位「暂未配置」）+「复制微信号」按钮，提供 1v1 部署指导。

---

### 7. 安装文档 `/docs` — `views/DocsPage.vue`（长文档 + 锚点侧边栏）

- **顶部**：标题「安装使用文档」+ 更新信息「最后更新：2026-07-22 · 适用版本：v1.0.0+」。
- **左侧目录**：按 4 个分组渲染 12 个锚点；滚动高亮 `activeSection`，移动端抽屉式展开。

  | 分组 | 锚点 |
  | --- | --- |
  | 入门 | 快速开始 / 架构与分工 / 系统要求 |
  | 部署 | Docker 部署 / 源码部署 / FRP 私域穿透 / 配置说明 |
  | 使用 | 功能模块 / 自动回复配置 / RAG 知识库 |
  | 运维 | 故障排查 / 常见问题 |

- **正文 12 节**（每节 `<section id="...">`，路由 hash 可直接定位）：

  | # | id | 标题 | 主要内容 |
  | --- | --- | --- | --- |
  | 1 | `quickstart` | 快速开始 | 4 步部署：获取源码 / 配置密钥（`cp .env-example .env`）/ 装依赖与起服务（`make install` + `make dev`）/ 访问 `8204` |
  | 2 | `architecture` | 架构与分工 | 用户端 vs 平台端对照表；平台端为**可选本地组件**（`PLATFORM_ENABLED=false` 即不连接），自建时配 `PLATFORM_API_HOST` / `MERCHANT_API_SECRET` |
  | 3 | `requirements` | 系统要求 | 硬件要求表（开发 / 小型生产 / 中大型生产）+ 软件要求表（Docker / Go / Node / PostgreSQL / Redis） |
  | 4 | `docker-deploy` | Docker 部署（推荐） | 克隆并生成配置 / 一键安装 / 关键环境变量 / 端口对照表（8202 / 8203 / 8204 / 8207 / 8208 / 8209） |
  | 5 | `source-deploy` | 源码部署 | 准备运行环境 / 构建后端 `user-server` / 构建前端 `user-web` |
  | 6 | `frp-deploy` | FRP 私域穿透 | 方案 B（nginx终止 TLS + frpc=http）；frps / nginx / frpc 配置；WebSocket 关键参数；docker-compose 集成 |
  | 7 | `config` | 配置说明 | 用户端 `.env` 字段；环境变量优先级（docker-compose `environment` > `config.yaml`） |
  | 8 | `modules` | 功能模块 | 94 业务模块 + 45 智能体工具，按业务域划分为 25 张模块卡（邮件 / 短信 / 卡片 / 社群 / 短链 / 线索 / 数据分析 / 内容创作 / 系统管理 / 团队协作 / AI Agent / LLM 路由 / 客服会话 / 坐席看板 / 销冠 SOP / 客户 CDP / 标签分层 / 触达运营 / RAG 知识库 / 模型计量 / 资产包市场 / 用户黑名单 / 心跳与安装 / 嵌入式聊天窗 / 数据驾驶舱） |
  | 9 | `auto-reply` | 自动回复配置 | Chrome Headless 检查 / 自动回复规则 / 启动与监控；浏览器自动化封号风险提示 |
  | 10 | `rag` | RAG 知识库 | 基于 pgvector；产品管理 / 文档导入（解析 / 分块 / Embedding / 入库）/ 三层决策（规则匹配 → 语义检索 → LLM 生成）/ 知识库维护 |
  | 11 | `troubleshoot` | 故障排查 | 服务无法启动 / 数据库连接失败 / Chrome 自动回复失效 / 前端访问白屏 |
  | 12 | `faq` | 常见问题 | 7 条部署相关 FAQ：数据保存位置 / SaaS 模式 / 自动回复封号 / 数据库选择 / 数据备份 / 升级版本 / 获取帮助 |

- **底部**：文档持续更新提示 + 「查看部署指南」按钮 → `/deploy`。

---

### 8. 404 页面 `/:pathMatch(.*)*` — `views/NotFoundPage.vue`

- 大字号 `404` 渐变文字 + 标题「页面未找到」。
- 描述：您访问的页面不存在或已迁移。HiveMTK 官网可能已更新内容结构。
- 3 个操作按钮：
  - 「返回首页」（primary）→ `/`
  - 「查看文档」（ghost）→ `/docs`
  - 「部署指南」（ghost）→ `/deploy`

---

## 三、Footer 结构（`components/FooterSection.vue`）

> **注意**：FooterSection 组件内部定义了自身的链接数组，**不读取** `content.js` 的 `footer` 字段。仅品牌名 / 微信号 / 邮箱 / 电话 / 二维码通过 `useSiteContact` 与 `brand` 动态注入。

### Footer 4 列网格（`footer-grid`）

| 列 | 标题 | 内容 |
| --- | --- | --- |
| 品牌（`footer-brand`） | — | 品牌标记 + `HiveMTK` 文本 + 描述（七端打透 · ReAct 智能体 · 零出域数据安全 · AGPL-3.0 · 4 步私有化部署） |
| 第 2 列 | `产品功能` | 4 个 `productLinks`：七端多账号聚合 / AI 自动谈单 / 销冠 SOP 智能体 / 客户 CDP（均跳转 `/#features` hash） |
| 第 3 列 | `快速导航` | 8 个 `resourceLinks`：部署指南（`/deploy`）/ 安装文档（`/docs`）/ 业务流程（`/#workflow`）/ 核心功能（`/features`）/ 功能架构（`/modules`）/ 按需订阅（`/pricing`）/ 工程能力（`/toolchain`）/ 常见问题（`/faq`） |
| 第 4 列 | `开源项目` | 4 个 `openSourceLinks`：AGPL-3.0 开源协议 / 贡献指南 / 更新日志 / 免责声明（均外链 Gitee）+ 「AGPL-3.0 开源，克隆仓库即可私有化部署」提示 + 微信二维码（条件渲染）+ 微信号 + 邮箱 / 电话 / 服务时间（条件渲染） |

### Footer 开源仓库入口（`footer-repos`）

胶囊形容器「HiveMTK 开源仓库」+ 2 个 `repoLinks`：

| 仓库 | 链接 |
| --- | --- |
| GitHub · xiaofang142/hivemtk | `https://github.com/xiaofang142/hivemtk` |
| Gitee · xhpmayun/hivemtk | `https://gitee.com/xhpmayun/hivemtk` |

### Footer 底部（`footer-bottom`）

- 版权：`© 2026 HiveMTK · 私域 AI 营销操作系统（AGPL-3.0 开源）`
- 免责声明：通过 `$t('页脚免责声明')` 注入。

---

## 四、完整路由清单

| 路径 | name | 页面 | 视图组件 | 在导航 | SEO title | 备注 |
| --- | --- | --- | --- | --- | --- | --- |
| `/` | Home | 首页 | `views/HomePage.vue` | 是（Logo） | 首页 | 5 区块聚合页（Hero + Features + Architecture + Toolchain + Workflow） |
| `/features` | Features | 核心功能 | `views/FeaturesPage.vue` | 是 | 核心功能 | 13 项功能卡 + 架构/订阅引流带 |
| `/modules` | Modules | 功能架构 | `views/ModulesPage.vue` | 是 | 功能架构 | 24 模块 × 212 菜单节点清单 + 双形态 + 运行机制 |
| `/pricing` | Pricing | 按需订阅 | `views/PricingPage.vue` | 是 | 按需订阅 | 三层模型 + 价目表 + 订阅流程 + LLM 双轨 + SaaS 功能 |
| `/toolchain` | Toolchain | 工程能力 | `views/ToolchainPage.vue` | 是 | 工程能力 | Architecture + 6 项能力卡 |
| `/workflow` | Workflow | 业务流程 | `views/WorkflowPage.vue` | 是 | 业务流程 | 6 步流程 |
| `/faq` | Faq | 常见问题 | `views/FaqPage.vue` | 是 | 常见问题 | 12 条 FAQ |
| `/deploy` | Deploy | 部署指南 | `views/DeployPage.vue` | 是（含 CTA） | 部署指南 | 3 部署方式 + 命令 Tab + 公开提示 + 联系作者 |
| `/docs` | Docs | 安装使用文档 | `views/DocsPage.vue` | 是 | 安装使用文档 | 12 节长文档 + 锚点侧边栏 |
| `/download` | — | — | — | 否 | — | 302 重定向到 `/deploy`（历史兼容，开源版无版本下载） |
| `/:pathMatch(.*)*` | NotFound | 404 | `views/NotFoundPage.vue` | 否 | 页面未找到 | 大字号 404 + 3 个跳转按钮 |

> SEO `meta`（title / description / keywords）由 `router/index.js` `afterEach` 客户端动态注入 `<head>`，弥补无 SSR 的 SEO 短板。`document.title` 格式：`${meta.title} — HiveMTK · 私域 AI 营销操作系统`。

---

## 五、术语与合规约束（参见 `TERMINOLOGY.md`）

- 主标题统一为「**七端打透 · AI 真自主 · 数据封死在域内**」，禁用「把销冠的谈单能力装进系统」。
- 品牌名统一为 `HiveMTK`，禁用「AI 私域销冠 / 营销智能体套件」。
- 部署方式统一为「**4 步完成部署**」，禁用「一键部署 / 3 分钟安装」（`content.js` 与 4 语言译文均为 4 步）。
- CTA 按钮仅可使用「立即部署 / 克隆仓库 / 获取源码」，禁用「立即试用 / 免费注册 / 开通账号 / 获取授权码」。
- 开源协议统一为「基于 AGPL-3.0 开源」，禁用「无 License / 无授权限制」。
- Footer 入口仅可使用「产品功能 / 快速导航 / 开源项目 / 开源仓库 / 联系作者」，禁用「版本下载 / 帮助中心 / 关于我们 / 博客」。
