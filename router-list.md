# 前端页面路由清单（任务四逐页点验 + 任务六新手引导文案）

- 来源：`user-web/src/router/index.js` + `src/router/modules/*.js`（82 个模块文件）
- 规模：**237 条页面路由**
- 生成方式：`scripts/gen_router_list.py`（只做索引，不开浏览器；重跑保留既有勾选状态）
- 校验口径：本地起 user-server + user-web，逐条路由**拟人点击**页面所有元素/表单/交互，
  比对**浏览器控制台报错**与 **Network 里的非 2xx API 响应**；发现错误即修复

## 测试环境（本轮）

| 形态 | 后端 | 前端 | 标识 |
|---|---|---|---|
| SaaS 多租户 | `127.0.0.1:8214`（`TENANT_MODE=on`） | `127.0.0.1:8290` | `tenant_mode=true` / `edition=saas` |
| 私域独立部署 | `127.0.0.1:8304`（`TENANT_MODE=off`） | `127.0.0.1:8291` | `tenant_mode=false` / `edition=opensource` |

- 浏览器：复用主浏览器登录态（CDP `127.0.0.1:9223`，profile `chrome-main-profile`）
- 登录账号：`uit_admin` / `Hive@Test123`（id=26，tenant_id=1）
- 两种形态**都要过**：SaaS 页出现的租户/商务属性，独立部署页必须不出现且不报错

## 状态图例

| 标记 | 含义 |
|---|---|
| `[x]` | 已逐页打开并点击验证：控制台无报错、API 无非 2xx |
| `[~]` | 已开页，响应属设计内拒绝（如参数必填校验、无权限），非缺陷 |
| `[!]` | 已点出缺陷，待修复 |
| `[ ]` | 未测 |

## 任务六 · 新手引导（页面导览）状态图例

「引导」列记的是**这一页的新手引导文案与步骤**写没写、写没写对：
读懂该页的功能/业务/使用/配置逻辑 → 写这一页的定位说明与关键操作说明 →
把说明挂到该页**真实存在**的 DOM 元素上 → 浏览器里点开确认高亮正确、× 能关掉。

| 标记 | 含义 |
|---|---|
| `G` | 已写导览：文案讲的是这一页的真实功能/操作/配置，高亮元素在浏览器里点过，关闭按钮生效 |
| `-` | 本页不需要导览（公共页/重定向/无业务语义），或尚未开写 |

## 清单

| # | 状态 | 路径 | 名称 | 标题 | 需登录 | 组件 | 源文件 | 引导 |
|---|---|---|---|---|---|---|---|---|
| 1 | [x] | `/setup` | InitSetup | 系统初始化 | - | `views/setup/InitSetup.vue` | `src/router/index.js` | - |
| 2 | [x] | `/login` | Login | 登录 | - | `views/Login.vue` | `src/router/index.js` | - |
| 3 | [x] | `/help-center` | HelpCenter | 帮助中心 | 否 | `views/public/HelpCenter.vue` | `src/router/index.js` | - |
| 4 | [x] | `/chat/embed/default` | ChatEmbed | 在线客服 | 否 | `views/chat/embed/Index.vue` | `src/router/index.js` | - |
| 5 | [x] | `/chat/embed/:channel_ref` | ChatEmbedChannel | 在线客服 | 否 | `views/chat/embed/Index.vue` | `src/router/index.js` | - |
| 6 | [x] | `profile` | Profile | 个人资料 | 是 | `views/Profile.vue` | `src/router/index.js` | - |
| 7 | [x] | `notifications` | Notifications | 通知中心 | 是 | `views/Notifications.vue` | `src/router/index.js` | - |
| 8 | [x] | `/:pathMatch(.*)*` | NotFound | 页面不存在 | - | `views/NotFound.vue` | `src/router/index.js` | - |
| 9 | [x] | `/` | Layout |  | - | `layout/Layout.vue` | `src/router/index.js` | - |
| 10 | [x] | `/chat/embed` | - |  | - | `-` | `src/router/index.js` | - |
| 11 | [x] | `/chat/embed/` | - |  | - | `-` | `src/router/index.js` | - |
| 12 | [x] | `/telegram/group` | - |  | - | `-` | `src/router/index.js` | - |
| 13 | [x] | `/oneid` | - |  | - | `-` | `src/router/index.js` | - |
| 14 | [x] | `/abExperiment/list` | AbExperimentList | A/B 实验 | 是 | `views/abExperiment/List.vue` | `src/router/modules/abExperiment.js` | G |
| 15 | [x] | `/aiAgent` | AIAgent | 智能体 | - | `views/aiAgent/List.vue` | `src/router/modules/aiAgent.js` | - |
| 16 | [x] | `/aiAgent/list` | AIAgentList | 智能体列表 | - | `views/aiAgent/List.vue` | `src/router/modules/aiAgent.js` | G |
| 17 | [x] | `/aiAgent/create` | AIAgentCreate | 创建智能体 | - | `views/aiAgent/Edit.vue` | `src/router/modules/aiAgent.js` | G |
| 18 | [x] | `/aiAgent/edit/:id` | AIAgentEdit | 编辑智能体 | - | `views/aiAgent/Edit.vue` | `src/router/modules/aiAgent.js` | - |
| 19 | [x] | `/aiAgent/tools` | AIToolManagement | AI 工具管理 | - | `views/aiAgent/ToolManagement/ToolList.vue` | `src/router/modules/aiAgent.js` | - |
| 20 | [x] | `/aiProductivity/list` | AiProductivityList | AI产能分析 | 是 | `views/aiProductivity/List.vue` | `src/router/modules/aiProductivity.js` | G |
| 21 | [x] | `/analytics/cohort-path` | CohortPath | 留存与路径 | 是 | `views/analytics/CohortPath.vue` | `src/router/modules/analytics.js` | G |
| 22 | [x] | `/approvalTask/list` | ApprovalTaskList | 待办中心 | - | `views/approvalTask/List.vue` | `src/router/modules/approvalTask.js` | G |
| 23 | [x] | `/asset-bundle/list` | AssetBundleList | 资产包管理 | 是 | `views/assetBundle/List.vue` | `src/router/modules/assetBundle.js` | G |
| 24 | [x] | `/asset-bundle/playground` | AssetBundlePlayground | 开发者 Playground | 是 | `views/assetBundle/Playground.vue` | `src/router/modules/assetBundle.js` | G |
| 25 | [x] | `/asset-bundle/playground/:aid` | AssetBundlePlaygroundEdit | 开发者 Playground | 是 | `views/assetBundle/Playground.vue` | `src/router/modules/assetBundle.js` | - |
| 26 | [x] | `/asset-bundle/merchant-new` | AssetBundleMerchantNew | 商户新建话术包 | 是 | `views/assetBundle/MerchantEditor.vue` | `src/router/modules/assetBundle.js` | G |
| 27 | [x] | `/asset-bundle/merchant/:aid` | AssetBundleMerchantEdit | 商户配置话术包 | 是 | `views/assetBundle/MerchantEditor.vue` | `src/router/modules/assetBundle.js` | - |
| 28 | [x] | `/asset-market` | AssetMarket | 资产市场 | 是 | `views/assetMarket/Market.vue` | `src/router/modules/assetMarket.js` | G |
| 29 | [x] | `/asset-market/detail/:id` | AssetMarketDetail | 资产详情 | 是 | `views/assetMarket/Detail.vue` | `src/router/modules/assetMarket.js` | - |
| 30 | [x] | `/asset-market/my-assets` | AssetMyAssets | 我的资产 | 是 | `views/assetMarket/MyAssets.vue` | `src/router/modules/assetMarket.js` | G |
| 31 | [x] | `/asset-market/sync-log` | AssetSyncLog | 同步日志 | 是 | `views/assetMarket/SyncLog.vue` | `src/router/modules/assetMarket.js` | G |
| 32 | [x] | `/backup/list` | BackupList | 备份恢复 | 是 | `views/backup/List.vue` | `src/router/modules/backup.js` | G |
| 33 | [x] | `/backup/enhanced` | BackupEnhanced | 备份管理 | 是 | `views/backup/Enhanced.vue` | `src/router/modules/backup.js` | - |
| 34 | [x] | `/badCase/list` | BadCaseList | Bad Case 队列 | - | `views/badCase/List.vue` | `src/router/modules/badCase.js` | G |
| 35 | [x] | `/batchOperation/list` | BatchOperationList | 批量操作 | 是 | `views/batchOperation/List.vue` | `src/router/modules/batchOperation.js` | G |
| 36 | [x] | `/bridge/token` | BridgeTokenManagement | 桥接凭证 | - | `views/bridge/TokenManagement.vue` | `src/router/modules/bridgeToken.js` | G |
| 37 | [~] | `/bridge/extension` | BridgeExtensionPackage | 扩展安装包 | 是 | `views/bridge/ExtensionPackage.vue` | `src/router/modules/bridgeToken.js` | - |
| 38 | [x] | `/bridge` | BridgeRoot | 渠道接入 | - | `-` | `src/router/modules/bridgeToken.js` | - |
| 39 | [x] | `/token` | - |  | - | `-` | `src/router/modules/bridgeToken.js` | - |
| 40 | [x] | `/browser-automation` | BrowserAutomation | 浏览器自动化 | 是 | `-` | `src/router/modules/browserAutomation.js` | - |
| 41 | [x] | `/browser-automation/tasks` | BrowserAutomationList | 任务列表 | 是 | `views/browserAutomation/List.vue` | `src/router/modules/browserAutomation.js` | G |
| 42 | [x] | `/browser-automation/tasks/create` | BrowserAutomationCreate | 新建任务 | 是 | `views/browserAutomation/Editor.vue` | `src/router/modules/browserAutomation.js` | G |
| 43 | [x] | `/browser-automation/tasks/:id/edit` | BrowserAutomationEditor | 编辑任务 | 是 | `views/browserAutomation/Editor.vue` | `src/router/modules/browserAutomation.js` | - |
| 44 | [x] | `/browser-automation/tasks/:id` | BrowserAutomationDetail | 任务详情 | 是 | `views/browserAutomation/Detail.vue` | `src/router/modules/browserAutomation.js` | - |
| 45 | [x] | `/browser-automation/sessions/:id` | BrowserAutomationMonitor | 执行监控 | 是 | `views/browserAutomation/Monitor.vue` | `src/router/modules/browserAutomation.js` | - |
| 46 | [x] | `/browser-automation/cron` | BrowserAutomationCron | 定时触发器 | 是 | `views/browserAutomation/Cron.vue` | `src/router/modules/browserAutomation.js` | G |
| 47 | [x] | `/browser-automation/status` | BrowserAutomationStatus | Host 状态 | 是 | `views/browserAutomation/Status.vue` | `src/router/modules/browserAutomation.js` | - |
| 48 | [x] | `/chatChannel/list` | ChatChannelList | 客服渠道 | 是 | `views/chatChannel/List.vue` | `src/router/modules/chatChannel.js` | - |
| 49 | [x] | `/chatChannel/create` | ChatChannelCreate | 新建客服渠道 | 是 | `views/chatChannel/Create.vue` | `src/router/modules/chatChannel.js` | - |
| 50 | [x] | `/chatChannel/edit/:id` | ChatChannelEdit | 编辑客服渠道 | 是 | `views/chatChannel/Edit.vue` | `src/router/modules/chatChannel.js` | - |
| 51 | [x] | `/chatChannel/install-guide/:id?` | ChatChannelInstallGuide | Widget 安装引导 | 是 | `views/chatChannel/InstallGuide.vue` | `src/router/modules/chatChannel.js` | - |
| 52 | [x] | `/churnPrediction/list` | ChurnPredictionList | 流失预警 | 是 | `views/churnPrediction/List.vue` | `src/router/modules/churnPrediction.js` | G |
| 53 | [x] | `/clue/list` | ClueList | 线索列表 | - | `views/clue/List.vue` | `src/router/modules/clue.js` | G |
| 54 | [x] | `/clue/statistics` | ClueStatistics | 线索统计 | - | `views/clue/Statistics.vue` | `src/router/modules/clue.js` | - |
| 55 | [x] | `/community/list` | CommunityList | 社群管理 | - | `views/community/List.vue` | `src/router/modules/community.js` | G |
| 56 | [x] | `/conversionFunnel/list` | ConversionFunnelList | 转化漏斗 | 是 | `views/conversionFunnel/List.vue` | `src/router/modules/conversionFunnel.js` | G |
| 57 | [x] | `/customReport/list` | CustomReportList | 自定义报表 | 是 | `views/customReport/List.vue` | `src/router/modules/customReport.js` | - |
| 58 | [x] | `/customer360/list` | Customer360List | 客户 360 | 是 | `views/customer360/List.vue` | `src/router/modules/customer360.js` | G |
| 59 | [x] | `/customerEvent/list` | CustomerEventList | 客户事件 | 是 | `views/customerEvent/List.vue` | `src/router/modules/customerEvent.js` | - |
| 60 | [x] | `/customerJourney/dashboard` | CustomerJourneyDashboard | 客户旅程大屏 | 是 | `views/customerJourney/Dashboard.vue` | `src/router/modules/customerJourney.js` | - |
| 61 | [x] | `/customerService/agentStatus` | AgentStatus | 坐席状态 | 是 | `views/customerService/AgentStatus.vue` | `src/router/modules/customerService.js` | G |
| 62 | [x] | `/customerService/quickReply` | QuickReply | 快捷回复 | 是 | `views/customerService/QuickReply.vue` | `src/router/modules/customerService.js` | G |
| 63 | [x] | `/customerService/sessionTag` | SessionTag | 会话标签 | 是 | `views/customerService/SessionTag.vue` | `src/router/modules/customerService.js` | G |
| 64 | [x] | `/customerService/aiSuggestion` | AISuggestion | AI 建议 | 是 | `views/customerService/AISuggestion.vue` | `src/router/modules/customerService.js` | G |
| 65 | [x] | `/customerService/csat` | CsatDashboard | CSAT 看板 | 是 | `views/customerService/CsatDashboard.vue` | `src/router/modules/customerService.js` | G |
| 66 | [x] | `/customerSession/list` | CustomerSessionList | 客服会话 | 是 | `views/customerSession/List.vue` | `src/router/modules/customerSession.js` | - |
| 67 | [x] | `/dashboardScreen/list` | DashboardScreenList | 数据大屏 | 是 | `views/dashboardScreen/List.vue` | `src/router/modules/dashboardScreen.js` | - |
| 68 | [x] | `/dashboardScreen/builder` | DashboardScreenBuilder | 大屏构建 | 是 | `views/dashboardScreen/Builder.vue` | `src/router/modules/dashboardScreen.js` | - |
| 69 | [x] | `/dialogueMemory/list` | DialogueMemoryList | 对话记忆 | 是 | `views/dialogueMemory/List.vue` | `src/router/modules/dialogueMemory.js` | - |
| 70 | [x] | `/dingtalk-app` | DingtalkApp | 钉钉应用 | - | `views/dingtalkApp/DingtalkAppAccount.vue` | `src/router/modules/dingtalkApp.js` | - |
| 71 | [x] | `/domainPool` | DomainPool | 域名池管理 | 是 | `views/domainPool/List.vue` | `src/router/modules/domainPool.js` | - |
| 72 | [x] | `/douyinCard` | DouyinCard | 抖音卡片 | - | `views/douyinCard/List.vue` | `src/router/modules/douyinCard.js` | - |
| 73 | [x] | `/douyin/stats` | DouyinStats | 抖音卡片统计 | - | `views/douyinCard/Stats.vue` | `src/router/modules/douyinCard.js` | - |
| 74 | [x] | `/douyin-card-stats/:id` | DouyinCardStats | 抖音卡片详情统计 | - | `views/douyinCard/CardStats.vue` | `src/router/modules/douyinCard.js` | - |
| 75 | [x] | `/email` | Email | 邮件列表 | - | `views/email/EmailList.vue` | `src/router/modules/email.js` | G |
| 76 | [x] | `/email/drafts` | EmailDrafts | 我的草稿 | - | `views/email/Drafts.vue` | `src/router/modules/email.js` | G |
| 77 | [x] | `/email/jobs` | EmailJobs | 我的任务 | - | `views/email/Jobs.vue` | `src/router/modules/email.js` | G |
| 78 | [x] | `/email/smtp` | EmailSmtp | 邮件账号 | - | `views/email/Smtp.vue` | `src/router/modules/email.js` | G |
| 79 | [x] | `/email/info` | EmailInfo | 邮件代理 | - | `views/email/Info.vue` | `src/router/modules/email.js` | G |
| 80 | [x] | `/email/guide` | EmailGuide | 使用引导 | - | `views/email/Guide.vue` | `src/router/modules/email.js` | - |
| 81 | [x] | `/email/deliverability` | EmailDeliverability | 送达分析 | 是 | `views/email/Deliverability.vue` | `src/router/modules/email.js` | G |
| 82 | [x] | `/email/drag-editor` | EmailDragEditor | 拖拽编辑器 | 是 | `views/email/DragEditor.vue` | `src/router/modules/email.js` | G |
| 83 | [x] | `/faq` | FAQ | FAQ 知识库 | - | `views/faq/Index.vue` | `src/router/modules/faq.js` | - |
| 84 | [x] | `/faq/list` | FAQList | FAQ 列表 | - | `views/faq/List.vue` | `src/router/modules/faq.js` | - |
| 85 | [x] | `/faq/editor` | FAQEditor | FAQ 编辑 | - | `views/faq/Editor.vue` | `src/router/modules/faq.js` | - |
| 86 | [x] | `/faq/editor/:id` | FAQEditorEdit | FAQ 编辑 | - | `views/faq/Editor.vue` | `src/router/modules/faq.js` | - |
| 87 | [x] | `/feishu` | Feishu | 飞书 | - | `views/feishu/FeishuAccount.vue` | `src/router/modules/feishu.js` | - |
| 88 | [x] | `/feishu/account` | FeishuAccount | 飞书账号 | - | `views/feishu/FeishuAccount.vue` | `src/router/modules/feishu.js` | - |
| 89 | [x] | `/followups/today` | FollowUpToday | 今日跟进 | 是 | `views/followups/Today.vue` | `src/router/modules/followups.js` | - |
| 90 | [~] | `/followups/:id` | FollowUpDetail | 跟进详情 | 是 | `views/followups/Today.vue` | `src/router/modules/followups.js` | - |
| 91 | [x] | `/geo-tools/brand-config` | GeoBrandConfig | 品牌与域名 | 是 | `views/geo/BrandConfig.vue` | `src/router/modules/geoTools.js` | G |
| 92 | [x] | `/geo-tools/seo-infra` | GeoSeoInfra | SEO 基础设施 | 是 | `views/geo/SeoInfra.vue` | `src/router/modules/geoTools.js` | G |
| 93 | [x] | `/geo-tools/pusher-config` | GeoPusherConfig | 蜘蛛推送配置 | 是 | `views/geo/PusherConfig.vue` | `src/router/modules/geoTools.js` | G |
| 94 | [x] | `/geo-tools/site-config` | GeoSiteConfig | 静态站部署 | 是 | `views/geo/SiteConfig.vue` | `src/router/modules/geoTools.js` | G |
| 95 | [x] | `/geo-tools/schema-templates` | GeoSchemaTemplates | Schema 模板 | 是 | `views/geo/SchemaTemplates.vue` | `src/router/modules/geoTools.js` | G |
| 96 | [x] | `/geo-tools/competitors` | GeoCompetitors | 竞品管理 | 是 | `views/geo/CompetitorManage.vue` | `src/router/modules/geoTools.js` | G |
| 97 | [x] | `/geo-tools/config` | GeoConfigOptimizer | 配置优化 | 是 | `views/geo/ConfigOptimizer.vue` | `src/router/modules/geoTools.js` | G |
| 98 | [x] | `/geo-tools/keyword-mining` | GeoKeywordMining | 关键词蒸馏 | 是 | `views/geo/KeywordMining.vue` | `src/router/modules/geoTools.js` | G |
| 99 | [x] | `/geo-tools/content-creation` | GeoContentCreation | 内容创作 | 是 | `views/geo/ContentCreation.vue` | `src/router/modules/geoTools.js` | G |
| 100 | [x] | `/geo-tools/content-optimize` | GeoContentOptimize | 文章优化 | 是 | `views/geo/ContentOptimize.vue` | `src/router/modules/geoTools.js` | G |
| 101 | [x] | `/geo-tools/site-publish` | GeoSitePublish | 发布到官网 | 是 | `views/geo/SitePublish.vue` | `src/router/modules/geoTools.js` | G |
| 102 | [x] | `/geo-tools/push-center` | GeoPushCenter | 蜘蛛推送 | 是 | `views/geo/PushCenter.vue` | `src/router/modules/geoTools.js` | G |
| 103 | [x] | `/geo-tools/platform-publish` | GeoPlatformPublish | 多平台发布 | 是 | `views/geo/PlatformPublish.vue` | `src/router/modules/geoTools.js` | G |
| 104 | [x] | `/geo-tools/workflow` | GeoWorkflow | 工作流 | 是 | `views/geo/WorkflowEditor.vue` | `src/router/modules/geoTools.js` | G |
| 105 | [x] | `/geo-tools/knowledge-base` | GeoKnowledgeBase | GEO 知识库 | 是 | `views/geo/KnowledgeBase.vue` | `src/router/modules/geoTools.js` | G |
| 106 | [x] | `/geo-tools/entity-graph` | GeoEntityGraph | 实体图谱 | 是 | `views/geo/EntityGraph.vue` | `src/router/modules/geoTools.js` | G |
| 107 | [x] | `/geo-tools/funnel-dashboard` | GeoFunnelDashboard | 漏斗总览 | 是 | `views/geo/FunnelDashboard.vue` | `src/router/modules/geoTools.js` | G |
| 108 | [x] | `/geo-tools/index-tracking` | GeoIndexTracking | 收录与引用 | 是 | `views/geo/IndexTracking.vue` | `src/router/modules/geoTools.js` | G |
| 109 | [x] | `/geo-tools/visibility` | GeoVisibilityBoard | 可见性趋势 | 是 | `views/geo/VisibilityBoard.vue` | `src/router/modules/geoTools.js` | G |
| 110 | [x] | `/geo-tools/sov-board` | GeoSovBoard | 竞品 SOV | 是 | `views/geo/SovBoard.vue` | `src/router/modules/geoTools.js` | G |
| 111 | [x] | `/geo-tools/crawler-stats` | GeoCrawlerStats | 爬虫统计 | 是 | `views/geo/CrawlerStats.vue` | `src/router/modules/geoTools.js` | G |
| 112 | [x] | `/geo-tools/verification` | GeoVerification | 多模型验证 | 是 | `views/geo/Verification.vue` | `src/router/modules/geoTools.js` | G |
| 113 | [x] | `/geo/decision-report` | GeoDecisionReport | 决策链报表 | 是 | `views/geo/DecisionReport.vue` | `src/router/modules/geoTools.js` | G |
| 114 | [x] | `/geo-tools/reports` | GeoReports | 成本报表 | 是 | `views/geo/Reports.vue` | `src/router/modules/geoTools.js` | G |
| 115 | [x] | `/geo-tools/alerts` | GeoAlertCenter | 告警中心 | 是 | `views/geo/AlertCenter.vue` | `src/router/modules/geoTools.js` | G |
| 116 | [x] | `/glossary` | GlossaryList | 术语表管理 | 是 | `views/glossary/List.vue` | `src/router/modules/glossary.js` | - |
| 117 | [x] | `/i18n/dashboard` | I18nDashboard | 多语言监控 | 是 | `views/i18n/Dashboard.vue` | `src/router/modules/i18nStats.js` | - |
| 118 | [x] | `/inbox/list` | InboxList | 统一收件箱 | - | `views/inbox/List.vue` | `src/router/modules/inbox.js` | - |
| 119 | [x] | `/integration/list` | IntegrationList | 第三方对接 | 是 | `views/integration/List.vue` | `src/router/modules/integration.js` | - |
| 120 | [x] | `/intentRecognition/list` | IntentRecognitionList | 意图识别 | 是 | `views/intentRecognition/List.vue` | `src/router/modules/intentRecognition.js` | - |
| 121 | [x] | `/kb/connectors` | KbConnectors | 知识库连接器 | 是 | `views/kb/Connectors.vue` | `src/router/modules/kbConnector.js` | - |
| 122 | [x] | `/cap/matrix` | CapabilityMatrix | 能力矩阵 | 是 | `views/cap/Matrix.vue` | `src/router/modules/kbConnector.js` | - |
| 123 | [x] | `/knowledge/management` | KnowledgeManagement | 知识库管理 | 是 | `views/KnowledgeWorkspace/KnowledgeManagement.vue` | `src/router/modules/knowledge.js` | G |
| 124 | [x] | `/knowledge/batch-import` | KnowledgeBatchImport | 批量导入 | 是 | `views/KnowledgeWorkspace/BatchImport.vue` | `src/router/modules/knowledge.js` | G |
| 125 | [x] | `/knowledge/playground` | KnowledgePlayground | 检索 Playground | 是 | `views/KnowledgeWorkspace/Playground.vue` | `src/router/modules/knowledge.js` | G |
| 126 | [x] | `/knowledge/connectors` | KnowledgeConnectors | 外部连接器 | 是 | `views/KnowledgeWorkspace/Connectors.vue` | `src/router/modules/knowledge.js` | G |
| 127 | [x] | `/knowledge/feedbacks` | KnowledgeFeedbacks | 反馈管理 | 是 | `views/KnowledgeWorkspace/FeedbackList.vue` | `src/router/modules/knowledge.js` | G |
| 128 | [x] | `/knowledge/tokens` | KnowledgeTokens | API Token | 是 | `views/KnowledgeWorkspace/ApiToken.vue` | `src/router/modules/knowledge.js` | G |
| 129 | [x] | `/knowledge/external` | KnowledgeExternal | 外部系统接入 | 是 | `views/KnowledgeWorkspace/ExternalImport.vue` | `src/router/modules/knowledge.js` | G |
| 130 | [x] | `/knowledge/statistics` | KnowledgeStatistics | 知识库统计 | 是 | `views/KnowledgeWorkspace/KnowledgeStatistics.vue` | `src/router/modules/knowledge.js` | G |
| 131 | [x] | `/knowledge/openapi` | KnowledgeOpenAPI | OpenAPI 集成 | 是 | `views/KnowledgeWorkspace/OpenAPIIntegration.vue` | `src/router/modules/knowledge.js` | G |
| 132 | [x] | `/knowledge/rag-eval` | RagEvaluation | RAG 评测 | 是 | `views/knowledge/RagEvaluation.vue` | `src/router/modules/knowledge.js` | G |
| 133 | [x] | `/knowledgeBase` | KnowledgeBase | 知识库管理 | - | `views/knowledgeBase/List.vue` | `src/router/modules/knowledgeBase.js` | - |
| 134 | [x] | `/knowledgeBase/list` | KnowledgeBaseList | 知识库列表 | - | `views/knowledgeBase/List.vue` | `src/router/modules/knowledgeBase.js` | - |
| 135 | [x] | `/knowledgeBase/create` | KnowledgeBaseCreate | 新建知识库 | - | `views/knowledgeBase/List.vue` | `src/router/modules/knowledgeBase.js` | - |
| 136 | [x] | `/knowledgeBase/edit/:id` | KnowledgeBaseEdit | 编辑知识库 | - | `views/knowledgeBase/List.vue` | `src/router/modules/knowledgeBase.js` | - |
| 137 | [x] | `/kuaishouCard` | KuaishouCard | 快手卡片 | - | `views/kuaishouCard/List.vue` | `src/router/modules/kuaishouCard.js` | - |
| 138 | [x] | `/kuaishou/stats` | KuaishouStats | 快手卡片统计 | - | `views/kuaishouCard/Stats.vue` | `src/router/modules/kuaishouCard.js` | - |
| 139 | [x] | `/kuaishou-card-stats/:id` | KuaishouCardStats | 快手卡片详情统计 | - | `views/kuaishouCard/CardStats.vue` | `src/router/modules/kuaishouCard.js` | - |
| 140 | [x] | `/lead-mining/index` | LeadMining | 线索发掘 | - | `views/leadMining/Index.vue` | `src/router/modules/leadMining.js` | - |
| 141 | [x] | `/livecode` | LiveCodeManagement | 活码管理 | - | `views/livecode/LiveCodeManagement.vue` | `src/router/modules/livecode.js` | - |
| 142 | [x] | `/llmRouting/list` | LlmRoutingList | LLM路由 | 是 | `views/llmRouting/List.vue` | `src/router/modules/llmRouting.js` | - |
| 143 | [x] | `/llmRouting/cost` | LlmRoutingCostDashboard | 成本看板 | 是 | `views/llmRouting/CostDashboard.vue` | `src/router/modules/llmRouting.js` | - |
| 144 | [x] | `/marketingFlow/list` | MarketingFlowList | 营销自动化 | 是 | `views/marketingFlow/List.vue` | `src/router/modules/marketingFlow.js` | - |
| 145 | [x] | `/mcp/credentials` | McpCredentials | MCP凭证 | 是 | `views/mcp/Credentials.vue` | `src/router/modules/mcpCredential.js` | - |
| 146 | [x] | `/messageHub/list` | MessageHubList | 消息中台 | - | `views/messageHub/List.vue` | `src/router/modules/messageHub.js` | G |
| 147 | [x] | `/messageHub/dashboard` | MessageHubDashboard | 消息总览 | - | `views/messageHub/Dashboard.vue` | `src/router/modules/messageHub.js` | - |
| 148 | [x] | `/module-market` | ModuleMarket | 功能模块市场 | 否 | `views/moduleMarket/Market.vue` | `src/router/modules/moduleMarket.js` | - |
| 149 | [x] | `/module-market/detail/:key` | ModuleMarketDetail | 模块详情 | 否 | `views/moduleMarket/Detail.vue` | `src/router/modules/moduleMarket.js` | - |
| 150 | [x] | `/module-market/my` | ModuleMy | 我的模块 | 是 | `views/moduleMarket/MyModules.vue` | `src/router/modules/moduleMarket.js` | - |
| 151 | [x] | `/module-market/orders` | ModuleOrders | 我的订单 | 是 | `views/moduleMarket/Orders.vue` | `src/router/modules/moduleMarket.js` | - |
| 152 | [x] | `/subscription-wall` | SubscriptionWall | 订阅续费 | 是 | `views/subscription/Wall.vue` | `src/router/modules/moduleMarket.js` | - |
| 153 | [x] | `/register` | Register | 商户注册 | 否 | `views/register/Register.vue` | `src/router/modules/moduleMarket.js` | - |
| 154 | [x] | `/objection/list` | ObjectionList | 异议处理 | 是 | `views/objection/List.vue` | `src/router/modules/objection.js` | - |
| 155 | [x] | `/oneid/list` | OneIDList | OneID 列表 | 是 | `views/oneid/List.vue` | `src/router/modules/oneid.js` | - |
| 156 | [x] | `/oneid/conflicts` | OneIDConflicts | 身份冲突解决 | 是 | `views/oneid/Conflicts.vue` | `src/router/modules/oneid.js` | - |
| 157 | [x] | `/oneid/merge-rules` | OneIDMergeRules | 合并规则配置 | 是 | `views/oneid/MergeRuleConfig.vue` | `src/router/modules/oneid.js` | - |
| 158 | [x] | `/operationLog/list` | OperationLogList | 操作日志 | 是 | `views/operationLog/List.vue` | `src/router/modules/operationLog.js` | - |
| 159 | [x] | `/operationLog/enhanced` | OperationLogEnhanced | 操作日志(增强) | 是 | `views/operationLog/Enhanced.vue` | `src/router/modules/operationLog.js` | - |
| 160 | [x] | `/ops-overview` | OpsOverview | 运维总览 | - | `views/OpsOverview/Index.vue` | `src/router/modules/ops.js` | - |
| 161 | [x] | `/sales-cockpit` | SalesCockpitIndex | AI 销冠驾驶舱 | - | `views/SalesCockpit/Index.vue` | `src/router/modules/ops.js` | - |
| 162 | [x] | `/dashboard/drafts` | OrderDraftList | 订单草稿 | 是 | `views/orderDraft/List.vue` | `src/router/modules/orderDraft.js` | - |
| 163 | [~] | `/dashboard/drafts/:id` | OrderDraftDetail | 草稿详情 | 是 | `views/orderDraft/Detail.vue` | `src/router/modules/orderDraft.js` | - |
| 164 | [x] | `/system/permissions` | PermissionPanel | permission.title | 是 | `views/system/PermissionPanel.vue` | `src/router/modules/permission.js` | - |
| 165 | [x] | `/persona/list` | PersonaList | 销冠画像 | 是 | `views/persona/List.vue` | `src/router/modules/persona.js` | - |
| 166 | [x] | `/platformAccount/list` | PlatformAccountList | 平台账号 | 是 | `views/platformAccount/List.vue` | `src/router/modules/platformAccount.js` | - |
| 167 | [x] | `/qq` | QQBot | QQ 机器人 | - | `views/qq/account.vue` | `src/router/modules/qq.js` | - |
| 168 | [x] | `/qq/account` | QQAccount | 机器人账号 | - | `views/qq/account.vue` | `src/router/modules/qq.js` | G |
| 169 | [x] | `/system/rag-product-config` | RagProductConfigIndex | RAG 主配置 | 是 | `views/RagProductConfig/index.vue` | `src/router/modules/ragProductConfig.js` | - |
| 170 | [x] | `/system/rag-account` | RagProductAccountConfig | RAG 账号配置 | 是 | `views/RagProductConfig/AccountConfig.vue` | `src/router/modules/ragProductConfig.js` | - |
| 171 | [x] | `/system/rag-product` | RagProductManagement | RAG 产品管理 | 是 | `views/RagProductConfig/RagProductManagement.vue` | `src/router/modules/ragProductConfig.js` | - |
| 172 | [x] | `/reachPipeline/list` | ReachPipelineList | 触达Pipeline | - | `views/reachPipeline/List.vue` | `src/router/modules/reachPipeline.js` | - |
| 173 | [x] | `/reachPipeline/editor/:id?` | ReachPipelineEditor | Pipeline 编辑 | 是 | `views/reachPipeline/Editor.vue` | `src/router/modules/reachPipeline.js` | - |
| 174 | [x] | `/system/roles` | RoleList | role.title | 是 | `views/system/RoleList.vue` | `src/router/modules/role.js` | - |
| 175 | [x] | `/sales-workbench` | SalesWorkbenchIndex | 销售工作台 | 是 | `views/salesWorkbench/Index.vue` | `src/router/modules/salesWorkbench.js` | - |
| 176 | [x] | `/scriptTemplate/list` | ScriptTemplateList | 话术库 | 是 | `views/scriptTemplate/List.vue` | `src/router/modules/scriptTemplate.js` | - |
| 177 | [x] | `/scriptTemplate/ab-stats` | ScriptTemplateAbStats | 话术AB测试 | 是 | `views/scriptTemplate/AbStats.vue` | `src/router/modules/scriptTemplate.js` | - |
| 178 | [x] | `/securityAudit/list` | SecurityAuditList | 安全审计 | 是 | `views/securityAudit/List.vue` | `src/router/modules/securityAudit.js` | - |
| 179 | [x] | `/shortLink` | ShortLink | 短链管理 | - | `views/shortLink/List.vue` | `src/router/modules/shortLink.js` | - |
| 180 | [x] | `/shortLink/stats` | ShortLinkStats | 短链统计 | - | `views/shortLink/Stats.vue` | `src/router/modules/shortLink.js` | - |
| 181 | [x] | `/sms/list` | SmsList | 短信列表 | - | `views/sms/List.vue` | `src/router/modules/sms.js` | G |
| 182 | [x] | `/sms/drafts` | SmsDrafts | 短信草稿 | - | `views/sms/Drafts.vue` | `src/router/modules/sms.js` | G |
| 183 | [x] | `/sms/jobs` | SmsJobs | 短信任务 | - | `views/sms/Jobs.vue` | `src/router/modules/sms.js` | G |
| 184 | [x] | `/sms/config` | SmsConfig | 短信配置 | - | `views/sms/Config.vue` | `src/router/modules/sms.js` | G |
| 185 | [x] | `/sopAgent/list` | SopAgentList | 销冠 SOP 智能体 | 是 | `views/sopAgent/List.vue` | `src/router/modules/sopAgent.js` | - |
| 186 | [x] | `/sop-template` | SOPTemplate | SOP 模板库 | - | `views/sopTemplate/Index.vue` | `src/router/modules/sopTemplateKb.js` | - |
| 187 | [x] | `/sop-template/list` | SOPTemplateList | SOP 模板列表 | - | `views/sopTemplate/List.vue` | `src/router/modules/sopTemplateKb.js` | - |
| 188 | [x] | `/sop-template/market` | SOPTemplateMarket | SOP 模板市场 | - | `views/sopTemplate/Market.vue` | `src/router/modules/sopTemplateKb.js` | - |
| 189 | [x] | `/sop-template/editor` | SOPTemplateEditor | SOP 模板编辑 | - | `views/sopTemplate/Editor.vue` | `src/router/modules/sopTemplateKb.js` | - |
| 190 | [x] | `/sop-template/editor/:id` | SOPTemplateEditorEdit | SOP 模板编辑 | - | `views/sopTemplate/Editor.vue` | `src/router/modules/sopTemplateKb.js` | - |
| 191 | [x] | `/system/config` | SystemConfig | 站点设置 | 是 | `views/system/Config.vue` | `src/router/modules/system.js` | - |
| 192 | [x] | `/system/obs-config` | SystemObsConfig | 存储配置 | 是 | `views/system/ObsConfig.vue` | `src/router/modules/system.js` | - |
| 193 | [x] | `/system/material-library` | SystemMaterialLibrary | 素材库 | 是 | `views/system/MaterialLibrary.vue` | `src/router/modules/system.js` | - |
| 194 | [x] | `/system/monitor` | SystemMonitor | 监控 | 是 | `views/system/Monitor.vue` | `src/router/modules/system.js` | - |
| 195 | [x] | `/system/trace` | TraceMonitor | 链路追踪 | 是 | `views/system/TraceMonitor.vue` | `src/router/modules/system.js` | - |
| 196 | [x] | `/system/guide` | SystemGuide | 使用引导 | 是 | `views/system/Guide.vue` | `src/router/modules/system.js` | - |
| 197 | [x] | `/system/rag-overview` | RagOverview | RAG概览 | 是 | `views/system/RagOverview.vue` | `src/router/modules/system.js` | - |
| 198 | [x] | `/system/automation-hub` | AutomationHub | 自动化中心 | 是 | `views/system/AutomationHub.vue` | `src/router/modules/system.js` | - |
| 199 | [x] | `/system/config-params` | ConfigParams | 动态阈值参数 | 是 | `views/system/ConfigParams.vue` | `src/router/modules/system.js` | - |
| 200 | [x] | `/system/dictionaries` | SystemDictionaries | 业务字典 | 是 | `views/system/DictionaryManager.vue` | `src/router/modules/system.js` | - |
| 201 | [x] | `/system/feature-flags` | SystemFeatureFlags | 功能开关 | 是 | `views/system/FeatureFlags.vue` | `src/router/modules/system.js` | - |
| 202 | [x] | `/system/ltc-config` | LTCConfig | LTC 运营开关 | 是 | `views/system/LTCConfig.vue` | `src/router/modules/system.js` | - |
| 203 | [x] | `/system/users` | SystemUserList | systemUser.title | 是 | `views/system/UserList.vue` | `src/router/modules/systemUser.js` | - |
| 204 | [x] | `/tagSegmentation/list` | TagSegmentationList | 标签分层 | 是 | `views/tagSegmentation/List.vue` | `src/router/modules/tagSegmentation.js` | - |
| 205 | [x] | `/telegram` | Telegram | TG 机器人 | - | `views/telegram/account.vue` | `src/router/modules/telegram.js` | - |
| 206 | [x] | `/telegram/account` | TelegramAccount | 机器人账号 | - | `views/telegram/account.vue` | `src/router/modules/telegram.js` | G |
| 207 | [x] | `/telegram/gate` | TelegramGate | 入群管控 | - | `views/telegram/gate.vue` | `src/router/modules/telegram.js` | G |
| 208 | [x] | `/tiktok/list` | TikTokList | TikTok卡片 | 是 | `views/tiktokCard/List.vue` | `src/router/modules/tiktok.js` | - |
| 209 | [x] | `/tiktok/stats` | TikTokStats | TikTok统计 | 是 | `views/tiktokCard/Stats.vue` | `src/router/modules/tiktok.js` | - |
| 210 | [x] | `/tiktok-card-stats/:id` | TikTokCardStats | TikTok卡片统计 | 是 | `views/tiktokCard/CardStats.vue` | `src/router/modules/tiktok.js` | - |
| 211 | [x] | `/tiktok` | TikTok | TikTok | - | `-` | `src/router/modules/tiktok.js` | - |
| 212 | [x] | `/confidence/panel` | ConfidencePanel | 置信度运营 | 是 | `views/confidence/Panel.vue` | `src/router/modules/tuning.js` | - |
| 213 | [x] | `/humanize/panel` | HumanizePanel | 拟人度评估 | 是 | `views/humanize/Panel.vue` | `src/router/modules/tuning.js` | - |
| 214 | [x] | `/feedbackLoop/panel` | FeedbackLoopPanel | 反馈学习闭环 | 是 | `views/feedbackLoop/Panel.vue` | `src/router/modules/tuning.js` | - |
| 215 | [x] | `/unifiedMessage/list` | UnifiedMessageList | 统一消息 | - | `views/unifiedMessage/List.vue` | `src/router/modules/unifiedMessage.js` | - |
| 216 | [x] | `/userSegment/list` | UserSegmentList | 用户分层 RFM | 是 | `views/userSegment/List.vue` | `src/router/modules/userSegment.js` | - |
| 217 | [x] | `/userSegment/rfm-matrix` | RfmMatrix | RFM 矩阵 | 是 | `views/userSegment/RfmMatrix.vue` | `src/router/modules/userSegment.js` | - |
| 218 | [x] | `/userSegment/builder` | UserSegmentBuilder | 分层规则构建 | 是 | `views/userSegment/Builder.vue` | `src/router/modules/userSegment.js` | - |
| 219 | [x] | `/wecomAccount/list` | WecomAccountList | 企微账号管理 | - | `views/wecomAccount/List.vue` | `src/router/modules/wecomAccount.js` | - |
| 220 | [x] | `/wecomAccount/data` | WecomAccountData | 企微数据看板 | - | `views/wecomAccount/Data.vue` | `src/router/modules/wecomAccount.js` | - |
| 221 | [x] | `/whatsapp` | Whatsapp | WhatsApp 社群 | - | `views/whatsapp/WhatsappAccount.vue` | `src/router/modules/whatsapp.js` | - |
| 222 | [x] | `/whatsapp/account` | WhatsappAccount | 账号管理 | - | `views/whatsapp/WhatsappAccount.vue` | `src/router/modules/whatsapp.js` | G |
| 223 | [x] | `/whatsapp/drafts` | WhatsappDrafts | 草稿箱 | - | `views/whatsapp/WhatsappDrafts.vue` | `src/router/modules/whatsapp.js` | G |
| 224 | [x] | `/whatsapp/jobs` | WhatsappJobs | 群发 | - | `views/whatsapp/WhatsappJobs.vue` | `src/router/modules/whatsapp.js` | G |
| 225 | [x] | `/whatsapp/lead-group-selection` | LeadGroupSelection | 从线索库选择群体 | 是 | `views/whatsappBot/LeadGroupSelection.vue` | `src/router/modules/whatsapp.js` | G |
| 226 | [x] | `/whatsapp/group-messaging` | BulkMessaging | 批量消息发送 | 是 | `views/whatsappBot/BulkMessaging.vue` | `src/router/modules/whatsapp.js` | G |
| 227 | [x] | `/whatsapp/bulk-matrix` | WhatsappBulkMatrix | 群发矩阵 | 是 | `views/whatsapp/BulkMatrix.vue` | `src/router/modules/whatsapp.js` | G |
| 228 | [x] | `/whatsapp-cloud` | WhatsappCloud | WhatsApp Cloud | - | `views/whatsappCloud/WhatsappCloudAccount.vue` | `src/router/modules/whatsappCloud.js` | - |
| 229 | [x] | `/workflow-orchestrator/list` | WorkflowOrchestratorList | 工作流编排 | 是 | `views/workflowOrchestrator/List.vue` | `src/router/modules/workflowOrchestrator.js` | - |
| 230 | [x] | `/workflow-orchestrator/editor/:workflow_id` | WorkflowOrchestratorEditor | 工作流编辑 | 是 | `views/workflowOrchestrator/Editor.vue` | `src/router/modules/workflowOrchestrator.js` | - |
| 231 | [x] | `/workflow-orchestrator/execution/:id` | WorkflowOrchestratorExecution | 执行详情 | 是 | `views/workflowOrchestrator/Execution.vue` | `src/router/modules/workflowOrchestrator.js` | - |
| 232 | [x] | `/xianyuCard` | XianyuCard | 闲鱼卡片 | - | `views/xianyuCard/List.vue` | `src/router/modules/xianyuCard.js` | - |
| 233 | [x] | `/xianyu/stats` | XianyuStats | 闲鱼卡片统计 | - | `views/xianyuCard/Stats.vue` | `src/router/modules/xianyuCard.js` | - |
| 234 | [x] | `/xianyu-card-stats/:id` | XianyuCardStats | 闲鱼卡片详情统计 | - | `views/xianyuCard/CardStats.vue` | `src/router/modules/xianyuCard.js` | - |
| 235 | [x] | `/xiaohongshuCard` | XiaohongshuCard | 小红书卡片 | - | `views/xiaohongshuCard/List.vue` | `src/router/modules/xiaohongshuCard.js` | - |
| 236 | [x] | `/xiaohongshu/stats` | XiaohongshuStats | 小红书卡片统计 | - | `views/xiaohongshuCard/Stats.vue` | `src/router/modules/xiaohongshuCard.js` | - |
| 237 | [x] | `/xiaohongshu-card-stats/:id` | XiaohongshuCardStats | 小红书卡片详情统计 | - | `views/xiaohongshuCard/CardStats.vue` | `src/router/modules/xiaohongshuCard.js` | - |

## 进度

- 已验证无错误 `[x]`：**234** 条
- 已验证·设计内拒绝 `[~]`：**3** 条
- 待点验 `[ ]`：**0** 条
- 已点出缺陷 `[!]`：**0** 条
- 任务六已写页面导览 `G`：**85** 条
