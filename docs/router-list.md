# user-web 页面路由清单（router-list）

> 来源：`user-web/src/router/index.js` + `user-web/src/router/modules/*.js`（82 个模块文件）。
> 用途：SaaS 多租户改造后的人机点击测试（拟人测试）进度表 —— 逐页打开、点击、核对控制台与 API 返回。
> 路由总数：**235**。最后一列「UI 测试」由测试者逐页更新，不允许脚本批量勾选。

## 状态图例

| 标记 | 含义 |
| --- | --- |
| ⬜ | 未测 |
| 🟡 | 测试中 / 有未修问题 |
| ✅ | 通过（无控制台错误、无 API 报错） |
| ⚠️ | 通过，但存在已记录的差异/告警（备注列说明） |
| ❌ | 未通过（阻塞） |

## 模块索引

| 模块 | 路由数 |
| --- | --- |
| `abExperiment` | 1 |
| `aiAgent` | 5 |
| `aiProductivity` | 1 |
| `analytics` | 1 |
| `approvalTask` | 1 |
| `assetBundle` | 5 |
| `assetMarket` | 4 |
| `backup` | 2 |
| `badCase` | 1 |
| `batchOperation` | 1 |
| `bridgeToken` | 3 |
| `browserAutomation` | 7 |
| `chatChannel` | 4 |
| `churnPrediction` | 1 |
| `clue` | 2 |
| `community` | 1 |
| `conversionFunnel` | 1 |
| `customReport` | 1 |
| `customer360` | 1 |
| `customerEvent` | 1 |
| `customerJourney` | 1 |
| `customerService` | 5 |
| `customerSession` | 1 |
| `dashboardScreen` | 2 |
| `dialogueMemory` | 1 |
| `dingtalkApp` | 1 |
| `domainPool` | 1 |
| `douyinCard` | 3 |
| `email` | 8 |
| `faq` | 4 |
| `feishu` | 2 |
| `followups` | 2 |
| `geoTools` | 25 |
| `glossary` | 1 |
| `i18nStats` | 1 |
| `inbox` | 1 |
| `index(固定路由)` | 13 |
| `integration` | 1 |
| `intentRecognition` | 1 |
| `kbConnector` | 2 |
| `knowledge` | 10 |
| `knowledgeBase` | 4 |
| `kuaishouCard` | 3 |
| `leadMining` | 1 |
| `livecode` | 1 |
| `llmRouting` | 2 |
| `marketingFlow` | 1 |
| `mcpCredential` | 1 |
| `messageHub` | 2 |
| `moduleMarket` | 6 |
| `objection` | 1 |
| `oneid` | 3 |
| `operationLog` | 2 |
| `ops` | 2 |
| `orderDraft` | 2 |
| `permission` | 1 |
| `persona` | 1 |
| `platformAccount` | 1 |
| `qq` | 2 |
| `ragProductConfig` | 3 |
| `reachPipeline` | 2 |
| `role` | 1 |
| `salesWorkbench` | 1 |
| `scriptTemplate` | 2 |
| `securityAudit` | 1 |
| `shortLink` | 2 |
| `sms` | 4 |
| `sopAgent` | 1 |
| `sopTemplateKb` | 5 |
| `system` | 12 |
| `systemUser` | 1 |
| `tagSegmentation` | 1 |
| `telegram` | 3 |
| `tiktok` | 4 |
| `tuning` | 3 |
| `unifiedMessage` | 1 |
| `userSegment` | 3 |
| `wecomAccount` | 2 |
| `whatsapp` | 7 |
| `whatsappCloud` | 1 |
| `workflowOrchestrator` | 3 |
| `xianyuCard` | 3 |
| `xiaohongshuCard` | 3 |

## 明细

| # | 路由路径 | 路由名 | 页面组件 | 标题 | 分组 | 管理员 | UI 测试 | 备注 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| | | | | | | | | |
| **模块 `abExperiment`** | | | | | | | | |
| 1 | `/abExperiment/list` | AbExperimentList | `@/views/abExperiment/List.vue` | A/B 实验 | dataAnalysis |  | ⬜ |  |
| | | | | | | | | |
| **模块 `aiAgent`** | | | | | | | | |
| 2 | `/aiAgent` | AIAgent | `@/views/aiAgent/List.vue` | 智能体 | aiAgent |  | ⬜ |  |
| 3 | `/aiAgent/list` | AIAgentList | `@/views/aiAgent/List.vue` | 智能体列表 | aiAgent |  | ⬜ |  |
| 4 | `/aiAgent/create` | AIAgentCreate | `@/views/aiAgent/Edit.vue` | 创建智能体 | aiAgent |  | ⬜ |  |
| 5 | `/aiAgent/edit/:id` | AIAgentEdit | `@/views/aiAgent/Edit.vue` | 编辑智能体 | aiAgent |  | ⬜ |  |
| 6 | `/aiAgent/tools` | AIToolManagement | `@/views/aiAgent/ToolManagement/ToolList.vue` | AI 工具管理 | aiAgent |  | ⬜ |  |
| | | | | | | | | |
| **模块 `aiProductivity`** | | | | | | | | |
| 7 | `/aiProductivity/list` | AiProductivityList | `@/views/aiProductivity/List.vue` | AI产能分析 | analytics |  | ⬜ |  |
| | | | | | | | | |
| **模块 `analytics`** | | | | | | | | |
| 8 | `/analytics/cohort-path` | CohortPath | `@/views/analytics/CohortPath.vue` | 留存与路径 | analytics |  | ⬜ |  |
| | | | | | | | | |
| **模块 `approvalTask`** | | | | | | | | |
| 9 | `/approvalTask/list` | ApprovalTaskList | `@/views/approvalTask/List.vue` | 待办中心 | workspace |  | ⬜ |  |
| | | | | | | | | |
| **模块 `assetBundle`** | | | | | | | | |
| 10 | `/asset-bundle/list` | AssetBundleList | `@/views/assetBundle/List.vue` | 资产包管理 | aiAgent |  | ⬜ |  |
| 11 | `/asset-bundle/playground` | AssetBundlePlayground | `@/views/assetBundle/Playground.vue` | 开发者 Playground | aiAgent |  | ⬜ |  |
| 12 | `/asset-bundle/playground/:aid` | AssetBundlePlaygroundEdit | `@/views/assetBundle/Playground.vue` | 开发者 Playground | aiAgent |  | ⬜ |  |
| 13 | `/asset-bundle/merchant-new` | AssetBundleMerchantNew | `@/views/assetBundle/MerchantEditor.vue` | 商户新建话术包 | aiAgent |  | ⬜ |  |
| 14 | `/asset-bundle/merchant/:aid` | AssetBundleMerchantEdit | `@/views/assetBundle/MerchantEditor.vue` | 商户配置话术包 | aiAgent |  | ⬜ |  |
| | | | | | | | | |
| **模块 `assetMarket`** | | | | | | | | |
| 15 | `/asset-market` | AssetMarket | `@/views/assetMarket/Market.vue` | 资产市场 | aiAgent |  | ⬜ |  |
| 16 | `/asset-market/detail/:id` | AssetMarketDetail | `@/views/assetMarket/Detail.vue` | 资产详情 | aiAgent |  | ⬜ |  |
| 17 | `/asset-market/my-assets` | AssetMyAssets | `@/views/assetMarket/MyAssets.vue` | 我的资产 | aiAgent |  | ⬜ |  |
| 18 | `/asset-market/sync-log` | AssetSyncLog | `@/views/assetMarket/SyncLog.vue` | 同步日志 | aiAgent |  | ⬜ |  |
| | | | | | | | | |
| **模块 `backup`** | | | | | | | | |
| 19 | `/backup/list` | BackupList | `@/views/backup/List.vue` | 备份恢复 | system | 是 | ⬜ |  |
| 20 | `/backup/enhanced` | BackupEnhanced | `@/views/backup/Enhanced.vue` | 备份管理 | system | 是 | ⬜ |  |
| | | | | | | | | |
| **模块 `badCase`** | | | | | | | | |
| 21 | `/badCase/list` | BadCaseList | `@/views/badCase/List.vue` | Bad Case 队列 | workspace |  | ⬜ |  |
| | | | | | | | | |
| **模块 `batchOperation`** | | | | | | | | |
| 22 | `/batchOperation/list` | BatchOperationList | `@/views/batchOperation/List.vue` | 批量操作 | reach |  | ⬜ |  |
| | | | | | | | | |
| **模块 `bridgeToken`** | | | | | | | | |
| 23 | `/bridge` | BridgeRoot | `` | 渠道接入 ↪ 重定向 → /bridge/token |  | 是 | ⬜ |  |
| 24 | `/bridge/token` | BridgeTokenManagement | `@/views/bridge/TokenManagement.vue` | 桥接凭证 |  | 是 | ⬜ |  |
| 25 | `/token` |  | `` |  ↪ 重定向 → /bridge/token |  |  | ⬜ |  |
| | | | | | | | | |
| **模块 `browserAutomation`** | | | | | | | | |
| 26 | `/browser-automation/tasks` | BrowserAutomationList | `@/views/browserAutomation/List.vue` | 任务列表 |  |  | ⬜ |  |
| 27 | `/browser-automation/tasks/create` | BrowserAutomationCreate | `@/views/browserAutomation/Editor.vue` | 新建任务 |  |  | ⬜ |  |
| 28 | `/browser-automation/tasks/:id/edit` | BrowserAutomationEditor | `@/views/browserAutomation/Editor.vue` | 编辑任务 |  |  | ⬜ |  |
| 29 | `/browser-automation/tasks/:id` | BrowserAutomationDetail | `@/views/browserAutomation/Detail.vue` | 任务详情 |  |  | ⬜ |  |
| 30 | `/browser-automation/sessions/:id` | BrowserAutomationMonitor | `@/views/browserAutomation/Monitor.vue` | 执行监控 |  |  | ⬜ |  |
| 31 | `/browser-automation/cron` | BrowserAutomationCron | `@/views/browserAutomation/Cron.vue` | 定时触发器 |  |  | ⬜ |  |
| 32 | `/browser-automation/status` | BrowserAutomationStatus | `@/views/browserAutomation/Status.vue` | Host 状态 |  |  | ⬜ |  |
| | | | | | | | | |
| **模块 `chatChannel`** | | | | | | | | |
| 33 | `/chatChannel/list` | ChatChannelList | `@/views/chatChannel/List.vue` | 客服渠道 | customer |  | ⬜ |  |
| 34 | `/chatChannel/create` | ChatChannelCreate | `@/views/chatChannel/Create.vue` | 新建客服渠道 | customer |  | ⬜ |  |
| 35 | `/chatChannel/edit/:id` | ChatChannelEdit | `@/views/chatChannel/Edit.vue` | 编辑客服渠道 | customer |  | ⬜ |  |
| 36 | `/chatChannel/install-guide/:id?` | ChatChannelInstallGuide | `@/views/chatChannel/InstallGuide.vue` | Widget 安装引导 | customer |  | ⬜ |  |
| | | | | | | | | |
| **模块 `churnPrediction`** | | | | | | | | |
| 37 | `/churnPrediction/list` | ChurnPredictionList | `@/views/churnPrediction/List.vue` | 流失预警 | dataAnalysis |  | ⬜ |  |
| | | | | | | | | |
| **模块 `clue`** | | | | | | | | |
| 38 | `/clue/list` | ClueList | `@/views/clue/List.vue` | 线索列表 | customer |  | ⬜ |  |
| 39 | `/clue/statistics` | ClueStatistics | `@/views/clue/Statistics.vue` | 线索统计 | customer |  | ⬜ |  |
| | | | | | | | | |
| **模块 `community`** | | | | | | | | |
| 40 | `/community/list` | CommunityList | `@/views/community/List.vue` | 社群管理 | community |  | ⬜ |  |
| | | | | | | | | |
| **模块 `conversionFunnel`** | | | | | | | | |
| 41 | `/conversionFunnel/list` | ConversionFunnelList | `@/views/conversionFunnel/List.vue` | 转化漏斗 | analytics |  | ⬜ |  |
| | | | | | | | | |
| **模块 `customReport`** | | | | | | | | |
| 42 | `/customReport/list` | CustomReportList | `@/views/customReport/List.vue` | 自定义报表 | dataAnalysis |  | ⬜ |  |
| | | | | | | | | |
| **模块 `customer360`** | | | | | | | | |
| 43 | `/customer360/list` | Customer360List | `@/views/customer360/List.vue` | 客户 360 | customer |  | ⬜ |  |
| | | | | | | | | |
| **模块 `customerEvent`** | | | | | | | | |
| 44 | `/customerEvent/list` | CustomerEventList | `@/views/customerEvent/List.vue` | 客户事件 | customer |  | ⬜ |  |
| | | | | | | | | |
| **模块 `customerJourney`** | | | | | | | | |
| 45 | `/customerJourney/dashboard` | CustomerJourneyDashboard | `@/views/customerJourney/Dashboard.vue` | 客户旅程大屏 | analytics |  | ⬜ |  |
| | | | | | | | | |
| **模块 `customerService`** | | | | | | | | |
| 46 | `/customerService/agentStatus` | AgentStatus | `@/views/customerService/AgentStatus.vue` | 坐席状态 | community |  | ⬜ |  |
| 47 | `/customerService/quickReply` | QuickReply | `@/views/customerService/QuickReply.vue` | 快捷回复 | community |  | ⬜ |  |
| 48 | `/customerService/sessionTag` | SessionTag | `@/views/customerService/SessionTag.vue` | 会话标签 | community |  | ⬜ |  |
| 49 | `/customerService/aiSuggestion` | AISuggestion | `@/views/customerService/AISuggestion.vue` | AI 建议 | community |  | ⬜ |  |
| 50 | `/customerService/csat` | CsatDashboard | `@/views/customerService/CsatDashboard.vue` | CSAT 看板 | community |  | ⬜ |  |
| | | | | | | | | |
| **模块 `customerSession`** | | | | | | | | |
| 51 | `/customerSession/list` | CustomerSessionList | `@/views/customerSession/List.vue` | 客服会话 | customer |  | ⬜ |  |
| | | | | | | | | |
| **模块 `dashboardScreen`** | | | | | | | | |
| 52 | `/dashboardScreen/list` | DashboardScreenList | `@/views/dashboardScreen/List.vue` | 数据大屏 | dataAnalysis |  | ⬜ |  |
| 53 | `/dashboardScreen/builder` | DashboardScreenBuilder | `@/views/dashboardScreen/Builder.vue` | 大屏构建 | dataAnalysis |  | ⬜ |  |
| | | | | | | | | |
| **模块 `dialogueMemory`** | | | | | | | | |
| 54 | `/dialogueMemory/list` | DialogueMemoryList | `@/views/dialogueMemory/List.vue` | 对话记忆 | aiAgent |  | ⬜ |  |
| | | | | | | | | |
| **模块 `dingtalkApp`** | | | | | | | | |
| 55 | `/dingtalk-app` | DingtalkApp | `@/views/dingtalkApp/DingtalkAppAccount.vue` | 钉钉应用 | community |  | ⬜ |  |
| | | | | | | | | |
| **模块 `domainPool`** | | | | | | | | |
| 56 | `/domainPool` | DomainPool | `@/views/domainPool/List.vue` | 域名池管理 | system | 是 | ⬜ |  |
| | | | | | | | | |
| **模块 `douyinCard`** | | | | | | | | |
| 57 | `/douyinCard` | DouyinCard | `@/views/douyinCard/List.vue` | 抖音卡片 | community |  | ⬜ |  |
| 58 | `/douyin/stats` | DouyinStats | `@/views/douyinCard/Stats.vue` | 抖音卡片统计 | community |  | ⬜ |  |
| 59 | `/douyin-card-stats/:id` | DouyinCardStats | `@/views/douyinCard/CardStats.vue` | 抖音卡片详情统计 | community |  | ⬜ |  |
| | | | | | | | | |
| **模块 `email`** | | | | | | | | |
| 60 | `/email` | Email | `@/views/email/EmailList.vue` | 邮件列表 | reach |  | ⬜ |  |
| 61 | `/email/drafts` | EmailDrafts | `@/views/email/Drafts.vue` | 我的草稿 | reach |  | ⬜ |  |
| 62 | `/email/jobs` | EmailJobs | `@/views/email/Jobs.vue` | 我的任务 | reach |  | ⬜ |  |
| 63 | `/email/smtp` | EmailSmtp | `@/views/email/Smtp.vue` | 邮件账号 | reach |  | ⬜ |  |
| 64 | `/email/info` | EmailInfo | `@/views/email/Info.vue` | 邮件代理 | reach |  | ⬜ |  |
| 65 | `/email/guide` | EmailGuide | `@/views/email/Guide.vue` | 使用引导 | reach |  | ⬜ |  |
| 66 | `/email/deliverability` | EmailDeliverability | `@/views/email/Deliverability.vue` | 送达分析 | email |  | ⬜ |  |
| 67 | `/email/drag-editor` | EmailDragEditor | `@/views/email/DragEditor.vue` | 拖拽编辑器 | reach |  | ⬜ |  |
| | | | | | | | | |
| **模块 `faq`** | | | | | | | | |
| 68 | `/faq` | FAQ | `@/views/faq/Index.vue` | FAQ 知识库 | aiAgent |  | ⬜ |  |
| 69 | `/faq/list` | FAQList | `@/views/faq/List.vue` | FAQ 列表 | aiAgent |  | ⬜ |  |
| 70 | `/faq/editor` | FAQEditor | `@/views/faq/Editor.vue` | FAQ 编辑 | aiAgent |  | ⬜ |  |
| 71 | `/faq/editor/:id` | FAQEditorEdit | `@/views/faq/Editor.vue` | FAQ 编辑 | aiAgent |  | ⬜ |  |
| | | | | | | | | |
| **模块 `feishu`** | | | | | | | | |
| 72 | `/feishu` | Feishu | `@/views/feishu/FeishuAccount.vue` | 飞书 | community |  | ⬜ |  |
| 73 | `/feishu/account` | FeishuAccount | `@/views/feishu/FeishuAccount.vue` | 飞书账号 | community |  | ⬜ |  |
| | | | | | | | | |
| **模块 `followups`** | | | | | | | | |
| 74 | `/followups/today` | FollowUpToday | `@/views/followups/Today.vue` | 今日跟进 | workspace |  | ⬜ |  |
| 75 | `/followups/:id` | FollowUpDetail | `@/views/followups/Today.vue` | 跟进详情 | workspace |  | ⬜ |  |
| | | | | | | | | |
| **模块 `geoTools`** | | | | | | | | |
| 76 | `/geo-tools/brand-config` | GeoBrandConfig | `@/views/geo/BrandConfig.vue` | 品牌与域名 | config |  | ⬜ |  |
| 77 | `/geo-tools/seo-infra` | GeoSeoInfra | `@/views/geo/SeoInfra.vue` | SEO 基础设施 | config |  | ⬜ |  |
| 78 | `/geo-tools/pusher-config` | GeoPusherConfig | `@/views/geo/PusherConfig.vue` | 蜘蛛推送配置 | config |  | ⬜ |  |
| 79 | `/geo-tools/site-config` | GeoSiteConfig | `@/views/geo/SiteConfig.vue` | 静态站部署 | config |  | ⬜ |  |
| 80 | `/geo-tools/schema-templates` | GeoSchemaTemplates | `@/views/geo/SchemaTemplates.vue` | Schema 模板 | config |  | ⬜ |  |
| 81 | `/geo-tools/competitors` | GeoCompetitors | `@/views/geo/CompetitorManage.vue` | 竞品管理 | config |  | ⬜ |  |
| 82 | `/geo-tools/config` | GeoConfigOptimizer | `@/views/geo/ConfigOptimizer.vue` | 配置优化 | config |  | ⬜ |  |
| 83 | `/geo-tools/keyword-mining` | GeoKeywordMining | `@/views/geo/KeywordMining.vue` | 关键词蒸馏 | execute |  | ⬜ |  |
| 84 | `/geo-tools/content-creation` | GeoContentCreation | `@/views/geo/ContentCreation.vue` | 内容创作 | execute |  | ⬜ |  |
| 85 | `/geo-tools/content-optimize` | GeoContentOptimize | `@/views/geo/ContentOptimize.vue` | 文章优化 | execute |  | ⬜ |  |
| 86 | `/geo-tools/site-publish` | GeoSitePublish | `@/views/geo/SitePublish.vue` | 发布到官网 | execute |  | ⬜ |  |
| 87 | `/geo-tools/push-center` | GeoPushCenter | `@/views/geo/PushCenter.vue` | 蜘蛛推送 | execute |  | ⬜ |  |
| 88 | `/geo-tools/platform-publish` | GeoPlatformPublish | `@/views/geo/PlatformPublish.vue` | 多平台发布 | execute |  | ⬜ |  |
| 89 | `/geo-tools/workflow` | GeoWorkflow | `@/views/geo/WorkflowEditor.vue` | 工作流 | execute |  | ⬜ |  |
| 90 | `/geo-tools/knowledge-base` | GeoKnowledgeBase | `@/views/geo/KnowledgeBase.vue` | GEO 知识库 | execute |  | ⬜ |  |
| 91 | `/geo-tools/entity-graph` | GeoEntityGraph | `@/views/geo/EntityGraph.vue` | 实体图谱 | execute |  | ⬜ |  |
| 92 | `/geo-tools/funnel-dashboard` | GeoFunnelDashboard | `@/views/geo/FunnelDashboard.vue` | 漏斗总览 | observe |  | ⬜ |  |
| 93 | `/geo-tools/index-tracking` | GeoIndexTracking | `@/views/geo/IndexTracking.vue` | 收录与引用 | observe |  | ⬜ |  |
| 94 | `/geo-tools/visibility` | GeoVisibilityBoard | `@/views/geo/VisibilityBoard.vue` | 可见性趋势 | observe |  | ⬜ |  |
| 95 | `/geo-tools/sov-board` | GeoSovBoard | `@/views/geo/SovBoard.vue` | 竞品 SOV | observe |  | ⬜ |  |
| 96 | `/geo-tools/crawler-stats` | GeoCrawlerStats | `@/views/geo/CrawlerStats.vue` | 爬虫统计 | observe |  | ⬜ |  |
| 97 | `/geo-tools/verification` | GeoVerification | `@/views/geo/Verification.vue` | 多模型验证 | observe |  | ⬜ |  |
| 98 | `/geo/decision-report` | GeoDecisionReport | `@/views/geo/DecisionReport.vue` | 决策链报表 | observe |  | ⬜ |  |
| 99 | `/geo-tools/reports` | GeoReports | `@/views/geo/Reports.vue` | 成本报表 | observe |  | ⬜ |  |
| 100 | `/geo-tools/alerts` | GeoAlertCenter | `@/views/geo/AlertCenter.vue` | 告警中心 | observe |  | ⬜ |  |
| | | | | | | | | |
| **模块 `glossary`** | | | | | | | | |
| 101 | `/glossary` | GlossaryList | `@/views/glossary/List.vue` | 术语表管理 | i18n |  | ⬜ |  |
| | | | | | | | | |
| **模块 `i18nStats`** | | | | | | | | |
| 102 | `/i18n/dashboard` | I18nDashboard | `@/views/i18n/Dashboard.vue` | 多语言监控 | i18n |  | ⬜ |  |
| | | | | | | | | |
| **模块 `inbox`** | | | | | | | | |
| 103 | `/inbox/list` | InboxList | `@/views/inbox/List.vue` | 统一收件箱 | workspace |  | ⬜ |  |
| | | | | | | | | |
| **模块 `index(固定路由)`** | | | | | | | | |
| 104 | `/setup` | InitSetup | `@/views/setup/InitSetup.vue` | 系统初始化 |  |  | ⬜ |  |
| 105 | `/login` | Login | `@/views/Login.vue` | 登录 |  |  | ⬜ |  |
| 106 | `/help-center` | HelpCenter | `@/views/public/HelpCenter.vue` | 帮助中心 |  |  | ⬜ |  |
| 107 | `/chat/embed/default` | ChatEmbed | `@/views/chat/embed/Index.vue` | 在线客服 |  |  | ⬜ |  |
| 108 | `/chat/embed/:channel_ref` | ChatEmbedChannel | `@/views/chat/embed/Index.vue` | 在线客服 |  |  | ⬜ |  |
| 109 | `/chat/embed` |  | `` |  ↪ 重定向 → /chat/embed/default |  |  | ⬜ |  |
| 110 | `/chat/embed/` |  | `` |  ↪ 重定向 → /chat/embed/default |  |  | ⬜ |  |
| 111 | `/` | Layout | `` |  ↪ 重定向 → /messageHub/list |  |  | ⬜ |  |
| 112 | `/profile` | Profile | `@/views/Profile.vue` | 个人资料 |  |  | ⬜ |  |
| 113 | `/notifications` | Notifications | `@/views/Notifications.vue` | 通知中心 |  |  | ⬜ |  |
| 114 | `/telegram/group` |  | `` |  ↪ 重定向 → /telegram/account |  |  | ⬜ |  |
| 115 | `/oneid` |  | `` |  ↪ 重定向 → /oneid/list |  |  | ⬜ |  |
| 116 | `/:pathMatch(.*)*` | NotFound | `@/views/NotFound.vue` | 页面不存在 |  |  | ⬜ |  |
| | | | | | | | | |
| **模块 `integration`** | | | | | | | | |
| 117 | `/integration/list` | IntegrationList | `@/views/integration/List.vue` | 第三方对接 | system | 是 | ⬜ |  |
| | | | | | | | | |
| **模块 `intentRecognition`** | | | | | | | | |
| 118 | `/intentRecognition/list` | IntentRecognitionList | `@/views/intentRecognition/List.vue` | 意图识别 | aiAgent |  | ⬜ |  |
| | | | | | | | | |
| **模块 `kbConnector`** | | | | | | | | |
| 119 | `/kb/connectors` | KbConnectors | `@/views/kb/Connectors.vue` | 知识库连接器 | knowledge |  | ⬜ |  |
| 120 | `/cap/matrix` | CapabilityMatrix | `@/views/cap/Matrix.vue` | 能力矩阵 | system |  | ⬜ |  |
| | | | | | | | | |
| **模块 `knowledge`** | | | | | | | | |
| 121 | `/knowledge/management` | KnowledgeManagement | `@/views/KnowledgeWorkspace/KnowledgeManagement.vue` | 知识库管理 | knowledge |  | ⬜ |  |
| 122 | `/knowledge/batch-import` | KnowledgeBatchImport | `@/views/KnowledgeWorkspace/BatchImport.vue` | 批量导入 | knowledge |  | ⬜ |  |
| 123 | `/knowledge/playground` | KnowledgePlayground | `@/views/KnowledgeWorkspace/Playground.vue` | 检索 Playground | knowledge |  | ⬜ |  |
| 124 | `/knowledge/connectors` | KnowledgeConnectors | `@/views/KnowledgeWorkspace/Connectors.vue` | 外部连接器 | knowledge |  | ⬜ |  |
| 125 | `/knowledge/feedbacks` | KnowledgeFeedbacks | `@/views/KnowledgeWorkspace/FeedbackList.vue` | 反馈管理 | knowledge |  | ⬜ |  |
| 126 | `/knowledge/tokens` | KnowledgeTokens | `@/views/KnowledgeWorkspace/ApiToken.vue` | API Token | knowledge |  | ⬜ |  |
| 127 | `/knowledge/external` | KnowledgeExternal | `@/views/KnowledgeWorkspace/ExternalImport.vue` | 外部系统接入 | knowledge |  | ⬜ |  |
| 128 | `/knowledge/statistics` | KnowledgeStatistics | `@/views/KnowledgeWorkspace/KnowledgeStatistics.vue` | 知识库统计 | knowledge |  | ⬜ |  |
| 129 | `/knowledge/openapi` | KnowledgeOpenAPI | `@/views/KnowledgeWorkspace/OpenAPIIntegration.vue` | OpenAPI 集成 | knowledge |  | ⬜ |  |
| 130 | `/knowledge/rag-eval` | RagEvaluation | `@/views/knowledge/RagEvaluation.vue` | RAG 评测 | knowledge |  | ⬜ |  |
| | | | | | | | | |
| **模块 `knowledgeBase`** | | | | | | | | |
| 131 | `/knowledgeBase` | KnowledgeBase | `@/views/knowledgeBase/List.vue` | 知识库管理 | knowledge |  | ⬜ |  |
| 132 | `/knowledgeBase/list` | KnowledgeBaseList | `@/views/knowledgeBase/List.vue` | 知识库列表 | knowledge |  | ⬜ |  |
| 133 | `/knowledgeBase/create` | KnowledgeBaseCreate | `@/views/knowledgeBase/List.vue` | 新建知识库 | knowledge |  | ⬜ |  |
| 134 | `/knowledgeBase/edit/:id` | KnowledgeBaseEdit | `@/views/knowledgeBase/List.vue` | 编辑知识库 | knowledge |  | ⬜ |  |
| | | | | | | | | |
| **模块 `kuaishouCard`** | | | | | | | | |
| 135 | `/kuaishouCard` | KuaishouCard | `@/views/kuaishouCard/List.vue` | 快手卡片 | community |  | ⬜ |  |
| 136 | `/kuaishou/stats` | KuaishouStats | `@/views/kuaishouCard/Stats.vue` | 快手卡片统计 | community |  | ⬜ |  |
| 137 | `/kuaishou-card-stats/:id` | KuaishouCardStats | `@/views/kuaishouCard/CardStats.vue` | 快手卡片详情统计 | community |  | ⬜ |  |
| | | | | | | | | |
| **模块 `leadMining`** | | | | | | | | |
| 138 | `/lead-mining/index` | LeadMining | `@/views/leadMining/Index.vue` | 线索发掘 | 线索发掘 |  | ⬜ |  |
| | | | | | | | | |
| **模块 `livecode`** | | | | | | | | |
| 139 | `/livecode` | LiveCodeManagement | `@/views/livecode/LiveCodeManagement.vue` | 活码管理 |  |  | ⬜ |  |
| | | | | | | | | |
| **模块 `llmRouting`** | | | | | | | | |
| 140 | `/llmRouting/list` | LlmRoutingList | `@/views/llmRouting/List.vue` | LLM路由 | aiAgent |  | ⬜ |  |
| 141 | `/llmRouting/cost` | LlmRoutingCostDashboard | `@/views/llmRouting/CostDashboard.vue` | 成本看板 | aiAgent |  | ⬜ |  |
| | | | | | | | | |
| **模块 `marketingFlow`** | | | | | | | | |
| 142 | `/marketingFlow/list` | MarketingFlowList | `@/views/marketingFlow/List.vue` | 营销自动化 | reach |  | ⬜ |  |
| | | | | | | | | |
| **模块 `mcpCredential`** | | | | | | | | |
| 143 | `/mcp/credentials` | McpCredentials | `@/views/mcp/Credentials.vue` | MCP凭证 | aiAgent |  | ⬜ |  |
| | | | | | | | | |
| **模块 `messageHub`** | | | | | | | | |
| 144 | `/messageHub/list` | MessageHubList | `@/views/messageHub/List.vue` | 消息中台 | workspace |  | ⬜ |  |
| 145 | `/messageHub/dashboard` | MessageHubDashboard | `@/views/messageHub/Dashboard.vue` | 消息总览 | workspace |  | ⬜ |  |
| | | | | | | | | |
| **模块 `moduleMarket`** | | | | | | | | |
| 146 | `/module-market` | ModuleMarket | `@/views/moduleMarket/Market.vue` | 功能模块市场 |  |  | ⬜ |  |
| 147 | `/module-market/detail/:key` | ModuleMarketDetail | `@/views/moduleMarket/Detail.vue` | 模块详情 |  |  | ⬜ |  |
| 148 | `/module-market/my` | ModuleMy | `@/views/moduleMarket/MyModules.vue` | 我的模块 |  |  | ⬜ |  |
| 149 | `/module-market/orders` | ModuleOrders | `@/views/moduleMarket/Orders.vue` | 我的订单 |  |  | ⬜ |  |
| 150 | `/subscription-wall` | SubscriptionWall | `@/views/subscription/Wall.vue` | 订阅续费 |  |  | ⬜ |  |
| 151 | `/register` | Register | `@/views/register/Register.vue` | 商户注册 |  |  | ⬜ |  |
| | | | | | | | | |
| **模块 `objection`** | | | | | | | | |
| 152 | `/objection/list` | ObjectionList | `@/views/objection/List.vue` | 异议处理 | sales |  | ⬜ |  |
| | | | | | | | | |
| **模块 `oneid`** | | | | | | | | |
| 153 | `/oneid/list` | OneIDList | `@/views/oneid/List.vue` | OneID 列表 |  |  | ⬜ |  |
| 154 | `/oneid/conflicts` | OneIDConflicts | `@/views/oneid/Conflicts.vue` | 身份冲突解决 |  |  | ⬜ |  |
| 155 | `/oneid/merge-rules` | OneIDMergeRules | `@/views/oneid/MergeRuleConfig.vue` | 合并规则配置 |  |  | ⬜ |  |
| | | | | | | | | |
| **模块 `operationLog`** | | | | | | | | |
| 156 | `/operationLog/list` | OperationLogList | `@/views/operationLog/List.vue` | 操作日志 | system | 是 | ⬜ |  |
| 157 | `/operationLog/enhanced` | OperationLogEnhanced | `@/views/operationLog/Enhanced.vue` | 操作日志(增强) | system | 是 | ⬜ |  |
| | | | | | | | | |
| **模块 `ops`** | | | | | | | | |
| 158 | `/ops-overview` | OpsOverview | `@/views/OpsOverview/Index.vue` | 运维总览 |  | 是 | ⬜ |  |
| 159 | `/sales-cockpit` | SalesCockpitIndex | `@/views/SalesCockpit/Index.vue` | AI 销冠驾驶舱 |  |  | ⬜ |  |
| | | | | | | | | |
| **模块 `orderDraft`** | | | | | | | | |
| 160 | `/dashboard/drafts` | OrderDraftList | `@/views/orderDraft/List.vue` | 订单草稿 | workspace |  | ⬜ |  |
| 161 | `/dashboard/drafts/:id` | OrderDraftDetail | `@/views/orderDraft/Detail.vue` | 草稿详情 | workspace |  | ⬜ |  |
| | | | | | | | | |
| **模块 `permission`** | | | | | | | | |
| 162 | `/system/permissions` | PermissionPanel | `@/views/system/PermissionPanel.vue` | permission.title | system | 是 | ⬜ |  |
| | | | | | | | | |
| **模块 `persona`** | | | | | | | | |
| 163 | `/persona/list` | PersonaList | `@/views/persona/List.vue` | 销冠画像 | sales |  | ⬜ |  |
| | | | | | | | | |
| **模块 `platformAccount`** | | | | | | | | |
| 164 | `/platformAccount/list` | PlatformAccountList | `@/views/platformAccount/List.vue` | 平台账号 | system | 是 | ⬜ |  |
| | | | | | | | | |
| **模块 `qq`** | | | | | | | | |
| 165 | `/qq` | QQBot | `@/views/qq/account.vue` | QQ 机器人 | community |  | ⬜ |  |
| 166 | `/qq/account` | QQAccount | `@/views/qq/account.vue` | 机器人账号 | community |  | ⬜ |  |
| | | | | | | | | |
| **模块 `ragProductConfig`** | | | | | | | | |
| 167 | `/system/rag-product-config` | RagProductConfigIndex | `@/views/RagProductConfig/index.vue` | RAG 主配置 | knowledge |  | ⬜ |  |
| 168 | `/system/rag-account` | RagProductAccountConfig | `@/views/RagProductConfig/AccountConfig.vue` | RAG 账号配置 | knowledge |  | ⬜ |  |
| 169 | `/system/rag-product` | RagProductManagement | `@/views/RagProductConfig/RagProductManagement.vue` | RAG 产品管理 | knowledge |  | ⬜ |  |
| | | | | | | | | |
| **模块 `reachPipeline`** | | | | | | | | |
| 170 | `/reachPipeline/list` | ReachPipelineList | `@/views/reachPipeline/List.vue` | 触达Pipeline | reach |  | ⬜ |  |
| 171 | `/reachPipeline/editor/:id?` | ReachPipelineEditor | `@/views/reachPipeline/Editor.vue` | Pipeline 编辑 | reach |  | ⬜ |  |
| | | | | | | | | |
| **模块 `role`** | | | | | | | | |
| 172 | `/system/roles` | RoleList | `@/views/system/RoleList.vue` | role.title | system | 是 | ⬜ |  |
| | | | | | | | | |
| **模块 `salesWorkbench`** | | | | | | | | |
| 173 | `/sales-workbench` | SalesWorkbenchIndex | `@/views/salesWorkbench/Index.vue` | 销售工作台 | workspace |  | ⬜ |  |
| | | | | | | | | |
| **模块 `scriptTemplate`** | | | | | | | | |
| 174 | `/scriptTemplate/list` | ScriptTemplateList | `@/views/scriptTemplate/List.vue` | 话术库 | aiAgent |  | ⬜ |  |
| 175 | `/scriptTemplate/ab-stats` | ScriptTemplateAbStats | `@/views/scriptTemplate/AbStats.vue` | 话术AB测试 | aiAgent |  | ⬜ |  |
| | | | | | | | | |
| **模块 `securityAudit`** | | | | | | | | |
| 176 | `/securityAudit/list` | SecurityAuditList | `@/views/securityAudit/List.vue` | 安全审计 | system | 是 | ⬜ |  |
| | | | | | | | | |
| **模块 `shortLink`** | | | | | | | | |
| 177 | `/shortLink` | ShortLink | `@/views/shortLink/List.vue` | 短链管理 | community |  | ⬜ |  |
| 178 | `/shortLink/stats` | ShortLinkStats | `@/views/shortLink/Stats.vue` | 短链统计 | community |  | ⬜ |  |
| | | | | | | | | |
| **模块 `sms`** | | | | | | | | |
| 179 | `/sms/list` | SmsList | `@/views/sms/List.vue` | 短信列表 | reach |  | ⬜ |  |
| 180 | `/sms/drafts` | SmsDrafts | `@/views/sms/Drafts.vue` | 短信草稿 | reach |  | ⬜ |  |
| 181 | `/sms/jobs` | SmsJobs | `@/views/sms/Jobs.vue` | 短信任务 | reach |  | ⬜ |  |
| 182 | `/sms/config` | SmsConfig | `@/views/sms/Config.vue` | 短信配置 | reach |  | ⬜ |  |
| | | | | | | | | |
| **模块 `sopAgent`** | | | | | | | | |
| 183 | `/sopAgent/list` | SopAgentList | `@/views/sopAgent/List.vue` | 销冠 SOP 智能体 | aiAgent |  | ⬜ |  |
| | | | | | | | | |
| **模块 `sopTemplateKb`** | | | | | | | | |
| 184 | `/sop-template` | SOPTemplate | `@/views/sopTemplate/Index.vue` | SOP 模板库 | aiAgent |  | ⬜ |  |
| 185 | `/sop-template/list` | SOPTemplateList | `@/views/sopTemplate/List.vue` | SOP 模板列表 | aiAgent |  | ⬜ |  |
| 186 | `/sop-template/market` | SOPTemplateMarket | `@/views/sopTemplate/Market.vue` | SOP 模板市场 | aiAgent |  | ⬜ |  |
| 187 | `/sop-template/editor` | SOPTemplateEditor | `@/views/sopTemplate/Editor.vue` | SOP 模板编辑 | aiAgent |  | ⬜ |  |
| 188 | `/sop-template/editor/:id` | SOPTemplateEditorEdit | `@/views/sopTemplate/Editor.vue` | SOP 模板编辑 | aiAgent |  | ⬜ |  |
| | | | | | | | | |
| **模块 `system`** | | | | | | | | |
| 189 | `/system/config` | SystemConfig | `@/views/system/Config.vue` | 站点设置 | system | 是 | ⬜ |  |
| 190 | `/system/obs-config` | SystemObsConfig | `@/views/system/ObsConfig.vue` | 存储配置 | system | 是 | ⬜ |  |
| 191 | `/system/material-library` | SystemMaterialLibrary | `@/views/system/MaterialLibrary.vue` | 素材库 | system | 是 | ⬜ |  |
| 192 | `/system/monitor` | SystemMonitor | `@/views/system/Monitor.vue` | 监控 | system | 是 | ⬜ |  |
| 193 | `/system/trace` | TraceMonitor | `@/views/system/TraceMonitor.vue` | 链路追踪 | system | 是 | ⬜ |  |
| 194 | `/system/guide` | SystemGuide | `@/views/system/Guide.vue` | 使用引导 | system | 是 | ⬜ |  |
| 195 | `/system/rag-overview` | RagOverview | `@/views/system/RagOverview.vue` | RAG概览 | knowledge |  | ⬜ |  |
| 196 | `/system/automation-hub` | AutomationHub | `@/views/system/AutomationHub.vue` | 自动化中心 | system | 是 | ⬜ |  |
| 197 | `/system/config-params` | ConfigParams | `@/views/system/ConfigParams.vue` | 动态阈值参数 | system | 是 | ⬜ |  |
| 198 | `/system/dictionaries` | SystemDictionaries | `@/views/system/DictionaryManager.vue` | 业务字典 | system | 是 | ⬜ |  |
| 199 | `/system/feature-flags` | SystemFeatureFlags | `@/views/system/FeatureFlags.vue` | 功能开关 | system | 是 | ⬜ |  |
| 200 | `/system/ltc-config` | LTCConfig | `@/views/system/LTCConfig.vue` | LTC 运营开关 | system | 是 | ⬜ |  |
| | | | | | | | | |
| **模块 `systemUser`** | | | | | | | | |
| 201 | `/system/users` | SystemUserList | `@/views/system/UserList.vue` | systemUser.title | system | 是 | ⬜ |  |
| | | | | | | | | |
| **模块 `tagSegmentation`** | | | | | | | | |
| 202 | `/tagSegmentation/list` | TagSegmentationList | `@/views/tagSegmentation/List.vue` | 标签分层 | customer |  | ⬜ |  |
| | | | | | | | | |
| **模块 `telegram`** | | | | | | | | |
| 203 | `/telegram` | Telegram | `@/views/telegram/account.vue` | TG 机器人 | community |  | ⬜ |  |
| 204 | `/telegram/account` | TelegramAccount | `@/views/telegram/account.vue` | 机器人账号 | community |  | ⬜ |  |
| 205 | `/telegram/gate` | TelegramGate | `@/views/telegram/gate.vue` | 入群管控 | community |  | ⬜ |  |
| | | | | | | | | |
| **模块 `tiktok`** | | | | | | | | |
| 206 | `/tiktok` | TikTok | `` | TikTok ↪ 重定向 → /tiktok/list | reach |  | ⬜ |  |
| 207 | `/tiktok/list` | TikTokList | `@/views/tiktokCard/List.vue` | TikTok卡片 | reach |  | ⬜ |  |
| 208 | `/tiktok/stats` | TikTokStats | `@/views/tiktokCard/Stats.vue` | TikTok统计 | reach |  | ⬜ |  |
| 209 | `/tiktok-card-stats/:id` | TikTokCardStats | `@/views/tiktokCard/CardStats.vue` | TikTok卡片统计 | reach |  | ⬜ |  |
| | | | | | | | | |
| **模块 `tuning`** | | | | | | | | |
| 210 | `/confidence/panel` | ConfidencePanel | `@/views/confidence/Panel.vue` | 置信度运营 | aiAgent |  | ⬜ |  |
| 211 | `/humanize/panel` | HumanizePanel | `@/views/humanize/Panel.vue` | 拟人度评估 | aiAgent |  | ⬜ |  |
| 212 | `/feedbackLoop/panel` | FeedbackLoopPanel | `@/views/feedbackLoop/Panel.vue` | 反馈学习闭环 | aiAgent |  | ⬜ |  |
| | | | | | | | | |
| **模块 `unifiedMessage`** | | | | | | | | |
| 213 | `/unifiedMessage/list` | UnifiedMessageList | `@/views/unifiedMessage/List.vue` | 统一消息 | customer |  | ⬜ |  |
| | | | | | | | | |
| **模块 `userSegment`** | | | | | | | | |
| 214 | `/userSegment/list` | UserSegmentList | `@/views/userSegment/List.vue` | 用户分层 RFM | customer |  | ⬜ |  |
| 215 | `/userSegment/rfm-matrix` | RfmMatrix | `@/views/userSegment/RfmMatrix.vue` | RFM 矩阵 | analytics |  | ⬜ |  |
| 216 | `/userSegment/builder` | UserSegmentBuilder | `@/views/userSegment/Builder.vue` | 分层规则构建 | customer |  | ⬜ |  |
| | | | | | | | | |
| **模块 `wecomAccount`** | | | | | | | | |
| 217 | `/wecomAccount/list` | WecomAccountList | `@/views/wecomAccount/List.vue` | 企微账号管理 | workspace |  | ⬜ |  |
| 218 | `/wecomAccount/data` | WecomAccountData | `@/views/wecomAccount/Data.vue` | 企微数据看板 | workspace |  | ⬜ |  |
| | | | | | | | | |
| **模块 `whatsapp`** | | | | | | | | |
| 219 | `/whatsapp` | Whatsapp | `@/views/whatsapp/WhatsappAccount.vue` | WhatsApp 社群 | community |  | ⬜ |  |
| 220 | `/whatsapp/account` | WhatsappAccount | `@/views/whatsapp/WhatsappAccount.vue` | 账号管理 | community |  | ⬜ |  |
| 221 | `/whatsapp/drafts` | WhatsappDrafts | `@/views/whatsapp/WhatsappDrafts.vue` | 草稿箱 | community |  | ⬜ |  |
| 222 | `/whatsapp/jobs` | WhatsappJobs | `@/views/whatsapp/WhatsappJobs.vue` | 群发 | community |  | ⬜ |  |
| 223 | `/whatsapp/lead-group-selection` | LeadGroupSelection | `@/views/whatsappBot/LeadGroupSelection.vue` | 从线索库选择群体 | community |  | ⬜ |  |
| 224 | `/whatsapp/group-messaging` | BulkMessaging | `@/views/whatsappBot/BulkMessaging.vue` | 批量消息发送 | community |  | ⬜ |  |
| 225 | `/whatsapp/bulk-matrix` | WhatsappBulkMatrix | `@/views/whatsapp/BulkMatrix.vue` | 群发矩阵 | community |  | ⬜ |  |
| | | | | | | | | |
| **模块 `whatsappCloud`** | | | | | | | | |
| 226 | `/whatsapp-cloud` | WhatsappCloud | `@/views/whatsappCloud/WhatsappCloudAccount.vue` | WhatsApp Cloud | community |  | ⬜ |  |
| | | | | | | | | |
| **模块 `workflowOrchestrator`** | | | | | | | | |
| 227 | `/workflow-orchestrator/list` | WorkflowOrchestratorList | `@/views/workflowOrchestrator/List.vue` | 工作流编排 | reach |  | ⬜ |  |
| 228 | `/workflow-orchestrator/editor/:workflow_id` | WorkflowOrchestratorEditor | `@/views/workflowOrchestrator/Editor.vue` | 工作流编辑 | reach |  | ⬜ |  |
| 229 | `/workflow-orchestrator/execution/:id` | WorkflowOrchestratorExecution | `@/views/workflowOrchestrator/Execution.vue` | 执行详情 | reach |  | ⬜ |  |
| | | | | | | | | |
| **模块 `xianyuCard`** | | | | | | | | |
| 230 | `/xianyuCard` | XianyuCard | `@/views/xianyuCard/List.vue` | 闲鱼卡片 | community |  | ⬜ |  |
| 231 | `/xianyu/stats` | XianyuStats | `@/views/xianyuCard/Stats.vue` | 闲鱼卡片统计 | community |  | ⬜ |  |
| 232 | `/xianyu-card-stats/:id` | XianyuCardStats | `@/views/xianyuCard/CardStats.vue` | 闲鱼卡片详情统计 | community |  | ⬜ |  |
| | | | | | | | | |
| **模块 `xiaohongshuCard`** | | | | | | | | |
| 233 | `/xiaohongshuCard` | XiaohongshuCard | `@/views/xiaohongshuCard/List.vue` | 小红书卡片 | community |  | ⬜ |  |
| 234 | `/xiaohongshu/stats` | XiaohongshuStats | `@/views/xiaohongshuCard/Stats.vue` | 小红书卡片统计 | community |  | ⬜ |  |
| 235 | `/xiaohongshu-card-stats/:id` | XiaohongshuCardStats | `@/views/xiaohongshuCard/CardStats.vue` | 小红书卡片详情统计 | community |  | ⬜ |  |
