# 前端页面路由清单（任务四 · SaaS 多租户改造后逐页面拟人点击验证）

- 来源：`user-web/src/router/index.js` + `src/router/modules/*.js`
- 规模：**236 条页面路由**
- 生成方式：`scripts/gen_router_list.py`（只做索引，不开浏览器）
- 校验口径：每条路由**逐页打开**，点击页面所有元素/表单/交互，
  比对浏览器控制台报错与 Network 里的 API 响应；发现错误即修复

## 清单

| # | 状态 | 路径 | 名称 | 标题 | 需登录 | 组件 | 源文件 |
|---|---|---|---|---|---|---|---|
| 1 | [ ] | `/setup` | InitSetup | 系统初始化 | - | `views/setup/InitSetup.vue` | `src/router/index.js` |
| 2 | [ ] | `/login` | Login | 登录 | - | `views/Login.vue` | `src/router/index.js` |
| 3 | [ ] | `/help-center` | HelpCenter | 帮助中心 | 否 | `views/public/HelpCenter.vue` | `src/router/index.js` |
| 4 | [ ] | `/chat/embed/default` | ChatEmbed | 在线客服 | 否 | `views/chat/embed/Index.vue` | `src/router/index.js` |
| 5 | [ ] | `/chat/embed/:channel_ref` | ChatEmbedChannel | 在线客服 | 否 | `views/chat/embed/Index.vue` | `src/router/index.js` |
| 6 | [ ] | `profile` | Profile | 个人资料 | 是 | `views/Profile.vue` | `src/router/index.js` |
| 7 | [ ] | `notifications` | Notifications | 通知中心 | 是 | `views/Notifications.vue` | `src/router/index.js` |
| 8 | [ ] | `/:pathMatch(.*)*` | NotFound | 页面不存在 | - | `views/NotFound.vue` | `src/router/index.js` |
| 9 | [ ] | `/` | Layout |  | - | `layout/Layout.vue` | `src/router/index.js` |
| 10 | [ ] | `/chat/embed` | - |  | - | `-` | `src/router/index.js` |
| 11 | [ ] | `/chat/embed/` | - |  | - | `-` | `src/router/index.js` |
| 12 | [ ] | `/telegram/group` | - |  | - | `-` | `src/router/index.js` |
| 13 | [ ] | `/oneid` | - |  | - | `-` | `src/router/index.js` |
| 14 | [ ] | `/abExperiment/list` | AbExperimentList | A/B 实验 | 是 | `views/abExperiment/List.vue` | `src/router/modules/abExperiment.js` |
| 15 | [ ] | `/aiAgent` | AIAgent | 智能体 | - | `views/aiAgent/List.vue` | `src/router/modules/aiAgent.js` |
| 16 | [ ] | `/aiAgent/list` | AIAgentList | 智能体列表 | - | `views/aiAgent/List.vue` | `src/router/modules/aiAgent.js` |
| 17 | [ ] | `/aiAgent/create` | AIAgentCreate | 创建智能体 | - | `views/aiAgent/Edit.vue` | `src/router/modules/aiAgent.js` |
| 18 | [ ] | `/aiAgent/edit/:id` | AIAgentEdit | 编辑智能体 | - | `views/aiAgent/Edit.vue` | `src/router/modules/aiAgent.js` |
| 19 | [ ] | `/aiAgent/tools` | AIToolManagement | AI 工具管理 | - | `views/aiAgent/ToolManagement/ToolList.vue` | `src/router/modules/aiAgent.js` |
| 20 | [ ] | `/aiProductivity/list` | AiProductivityList | AI产能分析 | 是 | `views/aiProductivity/List.vue` | `src/router/modules/aiProductivity.js` |
| 21 | [ ] | `/analytics/cohort-path` | CohortPath | 留存与路径 | 是 | `views/analytics/CohortPath.vue` | `src/router/modules/analytics.js` |
| 22 | [ ] | `/approvalTask/list` | ApprovalTaskList | 待办中心 | - | `views/approvalTask/List.vue` | `src/router/modules/approvalTask.js` |
| 23 | [ ] | `/asset-bundle/list` | AssetBundleList | 资产包管理 | 是 | `views/assetBundle/List.vue` | `src/router/modules/assetBundle.js` |
| 24 | [ ] | `/asset-bundle/playground` | AssetBundlePlayground | 开发者 Playground | 是 | `views/assetBundle/Playground.vue` | `src/router/modules/assetBundle.js` |
| 25 | [ ] | `/asset-bundle/playground/:aid` | AssetBundlePlaygroundEdit | 开发者 Playground | 是 | `views/assetBundle/Playground.vue` | `src/router/modules/assetBundle.js` |
| 26 | [ ] | `/asset-bundle/merchant-new` | AssetBundleMerchantNew | 商户新建话术包 | 是 | `views/assetBundle/MerchantEditor.vue` | `src/router/modules/assetBundle.js` |
| 27 | [ ] | `/asset-bundle/merchant/:aid` | AssetBundleMerchantEdit | 商户配置话术包 | 是 | `views/assetBundle/MerchantEditor.vue` | `src/router/modules/assetBundle.js` |
| 28 | [ ] | `/asset-market` | AssetMarket | 资产市场 | 是 | `views/assetMarket/Market.vue` | `src/router/modules/assetMarket.js` |
| 29 | [ ] | `/asset-market/detail/:id` | AssetMarketDetail | 资产详情 | 是 | `views/assetMarket/Detail.vue` | `src/router/modules/assetMarket.js` |
| 30 | [ ] | `/asset-market/my-assets` | AssetMyAssets | 我的资产 | 是 | `views/assetMarket/MyAssets.vue` | `src/router/modules/assetMarket.js` |
| 31 | [ ] | `/asset-market/sync-log` | AssetSyncLog | 同步日志 | 是 | `views/assetMarket/SyncLog.vue` | `src/router/modules/assetMarket.js` |
| 32 | [ ] | `/backup/list` | BackupList | 备份恢复 | 是 | `views/backup/List.vue` | `src/router/modules/backup.js` |
| 33 | [ ] | `/backup/enhanced` | BackupEnhanced | 备份管理 | 是 | `views/backup/Enhanced.vue` | `src/router/modules/backup.js` |
| 34 | [ ] | `/badCase/list` | BadCaseList | Bad Case 队列 | - | `views/badCase/List.vue` | `src/router/modules/badCase.js` |
| 35 | [ ] | `/batchOperation/list` | BatchOperationList | 批量操作 | 是 | `views/batchOperation/List.vue` | `src/router/modules/batchOperation.js` |
| 36 | [ ] | `/bridge/token` | BridgeTokenManagement | 桥接凭证 | - | `views/bridge/TokenManagement.vue` | `src/router/modules/bridgeToken.js` |
| 37 | [ ] | `/bridge` | BridgeRoot | 渠道接入 | - | `-` | `src/router/modules/bridgeToken.js` |
| 38 | [ ] | `/token` | - |  | - | `-` | `src/router/modules/bridgeToken.js` |
| 39 | [ ] | `tasks` | BrowserAutomationList | 任务列表 | 是 | `views/browserAutomation/List.vue` | `src/router/modules/browserAutomation.js` |
| 40 | [ ] | `tasks/create` | BrowserAutomationCreate | 新建任务 | 是 | `views/browserAutomation/Editor.vue` | `src/router/modules/browserAutomation.js` |
| 41 | [ ] | `tasks/:id/edit` | BrowserAutomationEditor | 编辑任务 | 是 | `views/browserAutomation/Editor.vue` | `src/router/modules/browserAutomation.js` |
| 42 | [ ] | `tasks/:id` | BrowserAutomationDetail | 任务详情 | 是 | `views/browserAutomation/Detail.vue` | `src/router/modules/browserAutomation.js` |
| 43 | [ ] | `sessions/:id` | BrowserAutomationMonitor | 执行监控 | 是 | `views/browserAutomation/Monitor.vue` | `src/router/modules/browserAutomation.js` |
| 44 | [ ] | `cron` | BrowserAutomationCron | 定时触发器 | 是 | `views/browserAutomation/Cron.vue` | `src/router/modules/browserAutomation.js` |
| 45 | [ ] | `status` | BrowserAutomationStatus | Host 状态 | 是 | `views/browserAutomation/Status.vue` | `src/router/modules/browserAutomation.js` |
| 46 | [ ] | `/browser-automation` | BrowserAutomation | 浏览器自动化 | 是 | `-` | `src/router/modules/browserAutomation.js` |
| 47 | [ ] | `/chatChannel/list` | ChatChannelList | 客服渠道 | 是 | `views/chatChannel/List.vue` | `src/router/modules/chatChannel.js` |
| 48 | [ ] | `/chatChannel/create` | ChatChannelCreate | 新建客服渠道 | 是 | `views/chatChannel/Create.vue` | `src/router/modules/chatChannel.js` |
| 49 | [ ] | `/chatChannel/edit/:id` | ChatChannelEdit | 编辑客服渠道 | 是 | `views/chatChannel/Edit.vue` | `src/router/modules/chatChannel.js` |
| 50 | [ ] | `/chatChannel/install-guide/:id?` | ChatChannelInstallGuide | Widget 安装引导 | 是 | `views/chatChannel/InstallGuide.vue` | `src/router/modules/chatChannel.js` |
| 51 | [ ] | `/churnPrediction/list` | ChurnPredictionList | 流失预警 | 是 | `views/churnPrediction/List.vue` | `src/router/modules/churnPrediction.js` |
| 52 | [ ] | `/clue/list` | ClueList | 线索列表 | - | `views/clue/List.vue` | `src/router/modules/clue.js` |
| 53 | [ ] | `/clue/statistics` | ClueStatistics | 线索统计 | - | `views/clue/Statistics.vue` | `src/router/modules/clue.js` |
| 54 | [ ] | `/community/list` | CommunityList | 社群管理 | - | `views/community/List.vue` | `src/router/modules/community.js` |
| 55 | [ ] | `/conversionFunnel/list` | ConversionFunnelList | 转化漏斗 | 是 | `views/conversionFunnel/List.vue` | `src/router/modules/conversionFunnel.js` |
| 56 | [ ] | `/customReport/list` | CustomReportList | 自定义报表 | 是 | `views/customReport/List.vue` | `src/router/modules/customReport.js` |
| 57 | [ ] | `/customer360/list` | Customer360List | 客户 360 | 是 | `views/customer360/List.vue` | `src/router/modules/customer360.js` |
| 58 | [ ] | `/customerEvent/list` | CustomerEventList | 客户事件 | 是 | `views/customerEvent/List.vue` | `src/router/modules/customerEvent.js` |
| 59 | [ ] | `/customerJourney/dashboard` | CustomerJourneyDashboard | 客户旅程大屏 | 是 | `views/customerJourney/Dashboard.vue` | `src/router/modules/customerJourney.js` |
| 60 | [ ] | `/customerService/agentStatus` | AgentStatus | 坐席状态 | 是 | `views/customerService/AgentStatus.vue` | `src/router/modules/customerService.js` |
| 61 | [ ] | `/customerService/quickReply` | QuickReply | 快捷回复 | 是 | `views/customerService/QuickReply.vue` | `src/router/modules/customerService.js` |
| 62 | [ ] | `/customerService/sessionTag` | SessionTag | 会话标签 | 是 | `views/customerService/SessionTag.vue` | `src/router/modules/customerService.js` |
| 63 | [ ] | `/customerService/aiSuggestion` | AISuggestion | AI 建议 | 是 | `views/customerService/AISuggestion.vue` | `src/router/modules/customerService.js` |
| 64 | [ ] | `/customerService/csat` | CsatDashboard | CSAT 看板 | 是 | `views/customerService/CsatDashboard.vue` | `src/router/modules/customerService.js` |
| 65 | [ ] | `/customerSession/list` | CustomerSessionList | 客服会话 | 是 | `views/customerSession/List.vue` | `src/router/modules/customerSession.js` |
| 66 | [ ] | `/dashboardScreen/list` | DashboardScreenList | 数据大屏 | 是 | `views/dashboardScreen/List.vue` | `src/router/modules/dashboardScreen.js` |
| 67 | [ ] | `/dashboardScreen/builder` | DashboardScreenBuilder | 大屏构建 | 是 | `views/dashboardScreen/Builder.vue` | `src/router/modules/dashboardScreen.js` |
| 68 | [ ] | `/dialogueMemory/list` | DialogueMemoryList | 对话记忆 | 是 | `views/dialogueMemory/List.vue` | `src/router/modules/dialogueMemory.js` |
| 69 | [ ] | `/dingtalk-app` | DingtalkApp | 钉钉应用 | - | `views/dingtalkApp/DingtalkAppAccount.vue` | `src/router/modules/dingtalkApp.js` |
| 70 | [ ] | `/domainPool` | DomainPool | 域名池管理 | 是 | `views/domainPool/List.vue` | `src/router/modules/domainPool.js` |
| 71 | [ ] | `/douyinCard` | DouyinCard | 抖音卡片 | - | `views/douyinCard/List.vue` | `src/router/modules/douyinCard.js` |
| 72 | [ ] | `/douyin/stats` | DouyinStats | 抖音卡片统计 | - | `views/douyinCard/Stats.vue` | `src/router/modules/douyinCard.js` |
| 73 | [ ] | `/douyin-card-stats/:id` | DouyinCardStats | 抖音卡片详情统计 | - | `views/douyinCard/CardStats.vue` | `src/router/modules/douyinCard.js` |
| 74 | [ ] | `/email` | Email | 邮件列表 | - | `views/email/EmailList.vue` | `src/router/modules/email.js` |
| 75 | [ ] | `/email/drafts` | EmailDrafts | 我的草稿 | - | `views/email/Drafts.vue` | `src/router/modules/email.js` |
| 76 | [ ] | `/email/jobs` | EmailJobs | 我的任务 | - | `views/email/Jobs.vue` | `src/router/modules/email.js` |
| 77 | [ ] | `/email/smtp` | EmailSmtp | 邮件账号 | - | `views/email/Smtp.vue` | `src/router/modules/email.js` |
| 78 | [ ] | `/email/info` | EmailInfo | 邮件代理 | - | `views/email/Info.vue` | `src/router/modules/email.js` |
| 79 | [ ] | `/email/guide` | EmailGuide | 使用引导 | - | `views/email/Guide.vue` | `src/router/modules/email.js` |
| 80 | [ ] | `/email/deliverability` | EmailDeliverability | 送达分析 | 是 | `views/email/Deliverability.vue` | `src/router/modules/email.js` |
| 81 | [ ] | `/email/drag-editor` | EmailDragEditor | 拖拽编辑器 | 是 | `views/email/DragEditor.vue` | `src/router/modules/email.js` |
| 82 | [ ] | `/faq` | FAQ | FAQ 知识库 | - | `views/faq/Index.vue` | `src/router/modules/faq.js` |
| 83 | [ ] | `/faq/list` | FAQList | FAQ 列表 | - | `views/faq/List.vue` | `src/router/modules/faq.js` |
| 84 | [ ] | `/faq/editor` | FAQEditor | FAQ 编辑 | - | `views/faq/Editor.vue` | `src/router/modules/faq.js` |
| 85 | [ ] | `/faq/editor/:id` | FAQEditorEdit | FAQ 编辑 | - | `views/faq/Editor.vue` | `src/router/modules/faq.js` |
| 86 | [ ] | `/feishu` | Feishu | 飞书 | - | `views/feishu/FeishuAccount.vue` | `src/router/modules/feishu.js` |
| 87 | [ ] | `/feishu/account` | FeishuAccount | 飞书账号 | - | `views/feishu/FeishuAccount.vue` | `src/router/modules/feishu.js` |
| 88 | [ ] | `/followups/today` | FollowUpToday | 今日跟进 | 是 | `views/followups/Today.vue` | `src/router/modules/followups.js` |
| 89 | [ ] | `/followups/:id` | FollowUpDetail | 跟进详情 | 是 | `views/followups/Today.vue` | `src/router/modules/followups.js` |
| 90 | [ ] | `/geo-tools/brand-config` | GeoBrandConfig | 品牌与域名 | 是 | `views/geo/BrandConfig.vue` | `src/router/modules/geoTools.js` |
| 91 | [ ] | `/geo-tools/seo-infra` | GeoSeoInfra | SEO 基础设施 | 是 | `views/geo/SeoInfra.vue` | `src/router/modules/geoTools.js` |
| 92 | [ ] | `/geo-tools/pusher-config` | GeoPusherConfig | 蜘蛛推送配置 | 是 | `views/geo/PusherConfig.vue` | `src/router/modules/geoTools.js` |
| 93 | [ ] | `/geo-tools/site-config` | GeoSiteConfig | 静态站部署 | 是 | `views/geo/SiteConfig.vue` | `src/router/modules/geoTools.js` |
| 94 | [ ] | `/geo-tools/schema-templates` | GeoSchemaTemplates | Schema 模板 | 是 | `views/geo/SchemaTemplates.vue` | `src/router/modules/geoTools.js` |
| 95 | [ ] | `/geo-tools/competitors` | GeoCompetitors | 竞品管理 | 是 | `views/geo/CompetitorManage.vue` | `src/router/modules/geoTools.js` |
| 96 | [ ] | `/geo-tools/config` | GeoConfigOptimizer | 配置优化 | 是 | `views/geo/ConfigOptimizer.vue` | `src/router/modules/geoTools.js` |
| 97 | [ ] | `/geo-tools/keyword-mining` | GeoKeywordMining | 关键词蒸馏 | 是 | `views/geo/KeywordMining.vue` | `src/router/modules/geoTools.js` |
| 98 | [ ] | `/geo-tools/content-creation` | GeoContentCreation | 内容创作 | 是 | `views/geo/ContentCreation.vue` | `src/router/modules/geoTools.js` |
| 99 | [ ] | `/geo-tools/content-optimize` | GeoContentOptimize | 文章优化 | 是 | `views/geo/ContentOptimize.vue` | `src/router/modules/geoTools.js` |
| 100 | [ ] | `/geo-tools/site-publish` | GeoSitePublish | 发布到官网 | 是 | `views/geo/SitePublish.vue` | `src/router/modules/geoTools.js` |
| 101 | [ ] | `/geo-tools/push-center` | GeoPushCenter | 蜘蛛推送 | 是 | `views/geo/PushCenter.vue` | `src/router/modules/geoTools.js` |
| 102 | [ ] | `/geo-tools/platform-publish` | GeoPlatformPublish | 多平台发布 | 是 | `views/geo/PlatformPublish.vue` | `src/router/modules/geoTools.js` |
| 103 | [ ] | `/geo-tools/workflow` | GeoWorkflow | 工作流 | 是 | `views/geo/WorkflowEditor.vue` | `src/router/modules/geoTools.js` |
| 104 | [ ] | `/geo-tools/knowledge-base` | GeoKnowledgeBase | GEO 知识库 | 是 | `views/geo/KnowledgeBase.vue` | `src/router/modules/geoTools.js` |
| 105 | [ ] | `/geo-tools/entity-graph` | GeoEntityGraph | 实体图谱 | 是 | `views/geo/EntityGraph.vue` | `src/router/modules/geoTools.js` |
| 106 | [ ] | `/geo-tools/funnel-dashboard` | GeoFunnelDashboard | 漏斗总览 | 是 | `views/geo/FunnelDashboard.vue` | `src/router/modules/geoTools.js` |
| 107 | [ ] | `/geo-tools/index-tracking` | GeoIndexTracking | 收录与引用 | 是 | `views/geo/IndexTracking.vue` | `src/router/modules/geoTools.js` |
| 108 | [ ] | `/geo-tools/visibility` | GeoVisibilityBoard | 可见性趋势 | 是 | `views/geo/VisibilityBoard.vue` | `src/router/modules/geoTools.js` |
| 109 | [ ] | `/geo-tools/sov-board` | GeoSovBoard | 竞品 SOV | 是 | `views/geo/SovBoard.vue` | `src/router/modules/geoTools.js` |
| 110 | [ ] | `/geo-tools/crawler-stats` | GeoCrawlerStats | 爬虫统计 | 是 | `views/geo/CrawlerStats.vue` | `src/router/modules/geoTools.js` |
| 111 | [ ] | `/geo-tools/verification` | GeoVerification | 多模型验证 | 是 | `views/geo/Verification.vue` | `src/router/modules/geoTools.js` |
| 112 | [ ] | `/geo/decision-report` | GeoDecisionReport | 决策链报表 | 是 | `views/geo/DecisionReport.vue` | `src/router/modules/geoTools.js` |
| 113 | [ ] | `/geo-tools/reports` | GeoReports | 成本报表 | 是 | `views/geo/Reports.vue` | `src/router/modules/geoTools.js` |
| 114 | [ ] | `/geo-tools/alerts` | GeoAlertCenter | 告警中心 | 是 | `views/geo/AlertCenter.vue` | `src/router/modules/geoTools.js` |
| 115 | [ ] | `/glossary` | GlossaryList | 术语表管理 | 是 | `views/glossary/List.vue` | `src/router/modules/glossary.js` |
| 116 | [ ] | `/i18n/dashboard` | I18nDashboard | 多语言监控 | 是 | `views/i18n/Dashboard.vue` | `src/router/modules/i18nStats.js` |
| 117 | [ ] | `/inbox/list` | InboxList | 统一收件箱 | - | `views/inbox/List.vue` | `src/router/modules/inbox.js` |
| 118 | [ ] | `/integration/list` | IntegrationList | 第三方对接 | 是 | `views/integration/List.vue` | `src/router/modules/integration.js` |
| 119 | [ ] | `/intentRecognition/list` | IntentRecognitionList | 意图识别 | 是 | `views/intentRecognition/List.vue` | `src/router/modules/intentRecognition.js` |
| 120 | [ ] | `/kb/connectors` | KbConnectors | 知识库连接器 | 是 | `views/kb/Connectors.vue` | `src/router/modules/kbConnector.js` |
| 121 | [ ] | `/cap/matrix` | CapabilityMatrix | 能力矩阵 | 是 | `views/cap/Matrix.vue` | `src/router/modules/kbConnector.js` |
| 122 | [ ] | `/knowledge/management` | KnowledgeManagement | 知识库管理 | 是 | `views/KnowledgeWorkspace/KnowledgeManagement.vue` | `src/router/modules/knowledge.js` |
| 123 | [ ] | `/knowledge/batch-import` | KnowledgeBatchImport | 批量导入 | 是 | `views/KnowledgeWorkspace/BatchImport.vue` | `src/router/modules/knowledge.js` |
| 124 | [ ] | `/knowledge/playground` | KnowledgePlayground | 检索 Playground | 是 | `views/KnowledgeWorkspace/Playground.vue` | `src/router/modules/knowledge.js` |
| 125 | [ ] | `/knowledge/connectors` | KnowledgeConnectors | 外部连接器 | 是 | `views/KnowledgeWorkspace/Connectors.vue` | `src/router/modules/knowledge.js` |
| 126 | [ ] | `/knowledge/feedbacks` | KnowledgeFeedbacks | 反馈管理 | 是 | `views/KnowledgeWorkspace/FeedbackList.vue` | `src/router/modules/knowledge.js` |
| 127 | [ ] | `/knowledge/tokens` | KnowledgeTokens | API Token | 是 | `views/KnowledgeWorkspace/ApiToken.vue` | `src/router/modules/knowledge.js` |
| 128 | [ ] | `/knowledge/external` | KnowledgeExternal | 外部系统接入 | 是 | `views/KnowledgeWorkspace/ExternalImport.vue` | `src/router/modules/knowledge.js` |
| 129 | [ ] | `/knowledge/statistics` | KnowledgeStatistics | 知识库统计 | 是 | `views/KnowledgeWorkspace/KnowledgeStatistics.vue` | `src/router/modules/knowledge.js` |
| 130 | [ ] | `/knowledge/openapi` | KnowledgeOpenAPI | OpenAPI 集成 | 是 | `views/KnowledgeWorkspace/OpenAPIIntegration.vue` | `src/router/modules/knowledge.js` |
| 131 | [ ] | `/knowledge/rag-eval` | RagEvaluation | RAG 评测 | 是 | `views/knowledge/RagEvaluation.vue` | `src/router/modules/knowledge.js` |
| 132 | [ ] | `/knowledgeBase` | KnowledgeBase | 知识库管理 | - | `views/knowledgeBase/List.vue` | `src/router/modules/knowledgeBase.js` |
| 133 | [ ] | `/knowledgeBase/list` | KnowledgeBaseList | 知识库列表 | - | `views/knowledgeBase/List.vue` | `src/router/modules/knowledgeBase.js` |
| 134 | [ ] | `/knowledgeBase/create` | KnowledgeBaseCreate | 新建知识库 | - | `views/knowledgeBase/List.vue` | `src/router/modules/knowledgeBase.js` |
| 135 | [ ] | `/knowledgeBase/edit/:id` | KnowledgeBaseEdit | 编辑知识库 | - | `views/knowledgeBase/List.vue` | `src/router/modules/knowledgeBase.js` |
| 136 | [ ] | `/kuaishouCard` | KuaishouCard | 快手卡片 | - | `views/kuaishouCard/List.vue` | `src/router/modules/kuaishouCard.js` |
| 137 | [ ] | `/kuaishou/stats` | KuaishouStats | 快手卡片统计 | - | `views/kuaishouCard/Stats.vue` | `src/router/modules/kuaishouCard.js` |
| 138 | [ ] | `/kuaishou-card-stats/:id` | KuaishouCardStats | 快手卡片详情统计 | - | `views/kuaishouCard/CardStats.vue` | `src/router/modules/kuaishouCard.js` |
| 139 | [ ] | `/lead-mining/index` | LeadMining | 线索发掘 | - | `views/leadMining/Index.vue` | `src/router/modules/leadMining.js` |
| 140 | [ ] | `/livecode` | LiveCodeManagement | 活码管理 | - | `views/livecode/LiveCodeManagement.vue` | `src/router/modules/livecode.js` |
| 141 | [ ] | `/llmRouting/list` | LlmRoutingList | LLM路由 | 是 | `views/llmRouting/List.vue` | `src/router/modules/llmRouting.js` |
| 142 | [ ] | `/llmRouting/cost` | LlmRoutingCostDashboard | 成本看板 | 是 | `views/llmRouting/CostDashboard.vue` | `src/router/modules/llmRouting.js` |
| 143 | [ ] | `/marketingFlow/list` | MarketingFlowList | 营销自动化 | 是 | `views/marketingFlow/List.vue` | `src/router/modules/marketingFlow.js` |
| 144 | [ ] | `/mcp/credentials` | McpCredentials | MCP凭证 | 是 | `views/mcp/Credentials.vue` | `src/router/modules/mcpCredential.js` |
| 145 | [ ] | `/messageHub/list` | MessageHubList | 消息中台 | - | `views/messageHub/List.vue` | `src/router/modules/messageHub.js` |
| 146 | [ ] | `/messageHub/dashboard` | MessageHubDashboard | 消息总览 | - | `views/messageHub/Dashboard.vue` | `src/router/modules/messageHub.js` |
| 147 | [ ] | `/module-market` | ModuleMarket | 功能模块市场 | 否 | `views/moduleMarket/Market.vue` | `src/router/modules/moduleMarket.js` |
| 148 | [ ] | `/module-market/detail/:key` | ModuleMarketDetail | 模块详情 | 否 | `views/moduleMarket/Detail.vue` | `src/router/modules/moduleMarket.js` |
| 149 | [ ] | `/module-market/my` | ModuleMy | 我的模块 | 是 | `views/moduleMarket/MyModules.vue` | `src/router/modules/moduleMarket.js` |
| 150 | [ ] | `/module-market/orders` | ModuleOrders | 我的订单 | 是 | `views/moduleMarket/Orders.vue` | `src/router/modules/moduleMarket.js` |
| 151 | [ ] | `/subscription-wall` | SubscriptionWall | 订阅续费 | 是 | `views/subscription/Wall.vue` | `src/router/modules/moduleMarket.js` |
| 152 | [ ] | `/register` | Register | 商户注册 | 否 | `views/register/Register.vue` | `src/router/modules/moduleMarket.js` |
| 153 | [ ] | `/objection/list` | ObjectionList | 异议处理 | 是 | `views/objection/List.vue` | `src/router/modules/objection.js` |
| 154 | [ ] | `/oneid/list` | OneIDList | OneID 列表 | 是 | `views/oneid/List.vue` | `src/router/modules/oneid.js` |
| 155 | [ ] | `/oneid/conflicts` | OneIDConflicts | 身份冲突解决 | 是 | `views/oneid/Conflicts.vue` | `src/router/modules/oneid.js` |
| 156 | [ ] | `/oneid/merge-rules` | OneIDMergeRules | 合并规则配置 | 是 | `views/oneid/MergeRuleConfig.vue` | `src/router/modules/oneid.js` |
| 157 | [ ] | `/operationLog/list` | OperationLogList | 操作日志 | 是 | `views/operationLog/List.vue` | `src/router/modules/operationLog.js` |
| 158 | [ ] | `/operationLog/enhanced` | OperationLogEnhanced | 操作日志(增强) | 是 | `views/operationLog/Enhanced.vue` | `src/router/modules/operationLog.js` |
| 159 | [ ] | `/ops-overview` | OpsOverview | 运维总览 | - | `views/OpsOverview/Index.vue` | `src/router/modules/ops.js` |
| 160 | [ ] | `/sales-cockpit` | SalesCockpitIndex | AI 销冠驾驶舱 | - | `views/SalesCockpit/Index.vue` | `src/router/modules/ops.js` |
| 161 | [ ] | `/dashboard/drafts` | OrderDraftList | 订单草稿 | 是 | `views/orderDraft/List.vue` | `src/router/modules/orderDraft.js` |
| 162 | [ ] | `/dashboard/drafts/:id` | OrderDraftDetail | 草稿详情 | 是 | `views/orderDraft/Detail.vue` | `src/router/modules/orderDraft.js` |
| 163 | [ ] | `/system/permissions` | PermissionPanel | permission.title | 是 | `views/system/PermissionPanel.vue` | `src/router/modules/permission.js` |
| 164 | [ ] | `/persona/list` | PersonaList | 销冠画像 | 是 | `views/persona/List.vue` | `src/router/modules/persona.js` |
| 165 | [ ] | `/platformAccount/list` | PlatformAccountList | 平台账号 | 是 | `views/platformAccount/List.vue` | `src/router/modules/platformAccount.js` |
| 166 | [ ] | `/qq` | QQBot | QQ 机器人 | - | `views/qq/account.vue` | `src/router/modules/qq.js` |
| 167 | [ ] | `/qq/account` | QQAccount | 机器人账号 | - | `views/qq/account.vue` | `src/router/modules/qq.js` |
| 168 | [ ] | `/system/rag-product-config` | RagProductConfigIndex | RAG 主配置 | 是 | `views/RagProductConfig/index.vue` | `src/router/modules/ragProductConfig.js` |
| 169 | [ ] | `/system/rag-account` | RagProductAccountConfig | RAG 账号配置 | 是 | `views/RagProductConfig/AccountConfig.vue` | `src/router/modules/ragProductConfig.js` |
| 170 | [ ] | `/system/rag-product` | RagProductManagement | RAG 产品管理 | 是 | `views/RagProductConfig/RagProductManagement.vue` | `src/router/modules/ragProductConfig.js` |
| 171 | [ ] | `/reachPipeline/list` | ReachPipelineList | 触达Pipeline | - | `views/reachPipeline/List.vue` | `src/router/modules/reachPipeline.js` |
| 172 | [ ] | `/reachPipeline/editor/:id?` | ReachPipelineEditor | Pipeline 编辑 | 是 | `views/reachPipeline/Editor.vue` | `src/router/modules/reachPipeline.js` |
| 173 | [ ] | `/system/roles` | RoleList | role.title | 是 | `views/system/RoleList.vue` | `src/router/modules/role.js` |
| 174 | [ ] | `/sales-workbench` | SalesWorkbenchIndex | 销售工作台 | 是 | `views/salesWorkbench/Index.vue` | `src/router/modules/salesWorkbench.js` |
| 175 | [ ] | `/scriptTemplate/list` | ScriptTemplateList | 话术库 | 是 | `views/scriptTemplate/List.vue` | `src/router/modules/scriptTemplate.js` |
| 176 | [ ] | `/scriptTemplate/ab-stats` | ScriptTemplateAbStats | 话术AB测试 | 是 | `views/scriptTemplate/AbStats.vue` | `src/router/modules/scriptTemplate.js` |
| 177 | [ ] | `/securityAudit/list` | SecurityAuditList | 安全审计 | 是 | `views/securityAudit/List.vue` | `src/router/modules/securityAudit.js` |
| 178 | [ ] | `/shortLink` | ShortLink | 短链管理 | - | `views/shortLink/List.vue` | `src/router/modules/shortLink.js` |
| 179 | [ ] | `/shortLink/stats` | ShortLinkStats | 短链统计 | - | `views/shortLink/Stats.vue` | `src/router/modules/shortLink.js` |
| 180 | [ ] | `/sms/list` | SmsList | 短信列表 | - | `views/sms/List.vue` | `src/router/modules/sms.js` |
| 181 | [ ] | `/sms/drafts` | SmsDrafts | 短信草稿 | - | `views/sms/Drafts.vue` | `src/router/modules/sms.js` |
| 182 | [ ] | `/sms/jobs` | SmsJobs | 短信任务 | - | `views/sms/Jobs.vue` | `src/router/modules/sms.js` |
| 183 | [ ] | `/sms/config` | SmsConfig | 短信配置 | - | `views/sms/Config.vue` | `src/router/modules/sms.js` |
| 184 | [ ] | `/sopAgent/list` | SopAgentList | 销冠 SOP 智能体 | 是 | `views/sopAgent/List.vue` | `src/router/modules/sopAgent.js` |
| 185 | [ ] | `/sop-template` | SOPTemplate | SOP 模板库 | - | `views/sopTemplate/Index.vue` | `src/router/modules/sopTemplateKb.js` |
| 186 | [ ] | `/sop-template/list` | SOPTemplateList | SOP 模板列表 | - | `views/sopTemplate/List.vue` | `src/router/modules/sopTemplateKb.js` |
| 187 | [ ] | `/sop-template/market` | SOPTemplateMarket | SOP 模板市场 | - | `views/sopTemplate/Market.vue` | `src/router/modules/sopTemplateKb.js` |
| 188 | [ ] | `/sop-template/editor` | SOPTemplateEditor | SOP 模板编辑 | - | `views/sopTemplate/Editor.vue` | `src/router/modules/sopTemplateKb.js` |
| 189 | [ ] | `/sop-template/editor/:id` | SOPTemplateEditorEdit | SOP 模板编辑 | - | `views/sopTemplate/Editor.vue` | `src/router/modules/sopTemplateKb.js` |
| 190 | [ ] | `/system/config` | SystemConfig | 站点设置 | 是 | `views/system/Config.vue` | `src/router/modules/system.js` |
| 191 | [ ] | `/system/obs-config` | SystemObsConfig | 存储配置 | 是 | `views/system/ObsConfig.vue` | `src/router/modules/system.js` |
| 192 | [ ] | `/system/material-library` | SystemMaterialLibrary | 素材库 | 是 | `views/system/MaterialLibrary.vue` | `src/router/modules/system.js` |
| 193 | [ ] | `/system/monitor` | SystemMonitor | 监控 | 是 | `views/system/Monitor.vue` | `src/router/modules/system.js` |
| 194 | [ ] | `/system/trace` | TraceMonitor | 链路追踪 | 是 | `views/system/TraceMonitor.vue` | `src/router/modules/system.js` |
| 195 | [ ] | `/system/guide` | SystemGuide | 使用引导 | 是 | `views/system/Guide.vue` | `src/router/modules/system.js` |
| 196 | [ ] | `/system/rag-overview` | RagOverview | RAG概览 | 是 | `views/system/RagOverview.vue` | `src/router/modules/system.js` |
| 197 | [ ] | `/system/automation-hub` | AutomationHub | 自动化中心 | 是 | `views/system/AutomationHub.vue` | `src/router/modules/system.js` |
| 198 | [ ] | `/system/config-params` | ConfigParams | 动态阈值参数 | 是 | `views/system/ConfigParams.vue` | `src/router/modules/system.js` |
| 199 | [ ] | `/system/dictionaries` | SystemDictionaries | 业务字典 | 是 | `views/system/DictionaryManager.vue` | `src/router/modules/system.js` |
| 200 | [ ] | `/system/feature-flags` | SystemFeatureFlags | 功能开关 | 是 | `views/system/FeatureFlags.vue` | `src/router/modules/system.js` |
| 201 | [ ] | `/system/ltc-config` | LTCConfig | LTC 运营开关 | 是 | `views/system/LTCConfig.vue` | `src/router/modules/system.js` |
| 202 | [ ] | `/system/users` | SystemUserList | systemUser.title | 是 | `views/system/UserList.vue` | `src/router/modules/systemUser.js` |
| 203 | [ ] | `/tagSegmentation/list` | TagSegmentationList | 标签分层 | 是 | `views/tagSegmentation/List.vue` | `src/router/modules/tagSegmentation.js` |
| 204 | [ ] | `/telegram` | Telegram | TG 机器人 | - | `views/telegram/account.vue` | `src/router/modules/telegram.js` |
| 205 | [ ] | `/telegram/account` | TelegramAccount | 机器人账号 | - | `views/telegram/account.vue` | `src/router/modules/telegram.js` |
| 206 | [ ] | `/telegram/gate` | TelegramGate | 入群管控 | - | `views/telegram/gate.vue` | `src/router/modules/telegram.js` |
| 207 | [ ] | `/tiktok/list` | TikTokList | TikTok卡片 | 是 | `views/tiktokCard/List.vue` | `src/router/modules/tiktok.js` |
| 208 | [ ] | `/tiktok/stats` | TikTokStats | TikTok统计 | 是 | `views/tiktokCard/Stats.vue` | `src/router/modules/tiktok.js` |
| 209 | [ ] | `/tiktok-card-stats/:id` | TikTokCardStats | TikTok卡片统计 | 是 | `views/tiktokCard/CardStats.vue` | `src/router/modules/tiktok.js` |
| 210 | [ ] | `/tiktok` | TikTok | TikTok | - | `-` | `src/router/modules/tiktok.js` |
| 211 | [ ] | `/confidence/panel` | ConfidencePanel | 置信度运营 | 是 | `views/confidence/Panel.vue` | `src/router/modules/tuning.js` |
| 212 | [ ] | `/humanize/panel` | HumanizePanel | 拟人度评估 | 是 | `views/humanize/Panel.vue` | `src/router/modules/tuning.js` |
| 213 | [ ] | `/feedbackLoop/panel` | FeedbackLoopPanel | 反馈学习闭环 | 是 | `views/feedbackLoop/Panel.vue` | `src/router/modules/tuning.js` |
| 214 | [ ] | `/unifiedMessage/list` | UnifiedMessageList | 统一消息 | - | `views/unifiedMessage/List.vue` | `src/router/modules/unifiedMessage.js` |
| 215 | [ ] | `/userSegment/list` | UserSegmentList | 用户分层 RFM | 是 | `views/userSegment/List.vue` | `src/router/modules/userSegment.js` |
| 216 | [ ] | `/userSegment/rfm-matrix` | RfmMatrix | RFM 矩阵 | 是 | `views/userSegment/RfmMatrix.vue` | `src/router/modules/userSegment.js` |
| 217 | [ ] | `/userSegment/builder` | UserSegmentBuilder | 分层规则构建 | 是 | `views/userSegment/Builder.vue` | `src/router/modules/userSegment.js` |
| 218 | [ ] | `/wecomAccount/list` | WecomAccountList | 企微账号管理 | - | `views/wecomAccount/List.vue` | `src/router/modules/wecomAccount.js` |
| 219 | [ ] | `/wecomAccount/data` | WecomAccountData | 企微数据看板 | - | `views/wecomAccount/Data.vue` | `src/router/modules/wecomAccount.js` |
| 220 | [ ] | `/whatsapp` | Whatsapp | WhatsApp 社群 | - | `views/whatsapp/WhatsappAccount.vue` | `src/router/modules/whatsapp.js` |
| 221 | [ ] | `/whatsapp/account` | WhatsappAccount | 账号管理 | - | `views/whatsapp/WhatsappAccount.vue` | `src/router/modules/whatsapp.js` |
| 222 | [ ] | `/whatsapp/drafts` | WhatsappDrafts | 草稿箱 | - | `views/whatsapp/WhatsappDrafts.vue` | `src/router/modules/whatsapp.js` |
| 223 | [ ] | `/whatsapp/jobs` | WhatsappJobs | 群发 | - | `views/whatsapp/WhatsappJobs.vue` | `src/router/modules/whatsapp.js` |
| 224 | [ ] | `/whatsapp/lead-group-selection` | LeadGroupSelection | 从线索库选择群体 | 是 | `views/whatsappBot/LeadGroupSelection.vue` | `src/router/modules/whatsapp.js` |
| 225 | [ ] | `/whatsapp/group-messaging` | BulkMessaging | 批量消息发送 | 是 | `views/whatsappBot/BulkMessaging.vue` | `src/router/modules/whatsapp.js` |
| 226 | [ ] | `/whatsapp/bulk-matrix` | WhatsappBulkMatrix | 群发矩阵 | 是 | `views/whatsapp/BulkMatrix.vue` | `src/router/modules/whatsapp.js` |
| 227 | [ ] | `/whatsapp-cloud` | WhatsappCloud | WhatsApp Cloud | - | `views/whatsappCloud/WhatsappCloudAccount.vue` | `src/router/modules/whatsappCloud.js` |
| 228 | [ ] | `/workflow-orchestrator/list` | WorkflowOrchestratorList | 工作流编排 | 是 | `views/workflowOrchestrator/List.vue` | `src/router/modules/workflowOrchestrator.js` |
| 229 | [ ] | `/workflow-orchestrator/editor/:workflow_id` | WorkflowOrchestratorEditor | 工作流编辑 | 是 | `views/workflowOrchestrator/Editor.vue` | `src/router/modules/workflowOrchestrator.js` |
| 230 | [ ] | `/workflow-orchestrator/execution/:id` | WorkflowOrchestratorExecution | 执行详情 | 是 | `views/workflowOrchestrator/Execution.vue` | `src/router/modules/workflowOrchestrator.js` |
| 231 | [ ] | `/xianyuCard` | XianyuCard | 闲鱼卡片 | - | `views/xianyuCard/List.vue` | `src/router/modules/xianyuCard.js` |
| 232 | [ ] | `/xianyu/stats` | XianyuStats | 闲鱼卡片统计 | - | `views/xianyuCard/Stats.vue` | `src/router/modules/xianyuCard.js` |
| 233 | [ ] | `/xianyu-card-stats/:id` | XianyuCardStats | 闲鱼卡片详情统计 | - | `views/xianyuCard/CardStats.vue` | `src/router/modules/xianyuCard.js` |
| 234 | [ ] | `/xiaohongshuCard` | XiaohongshuCard | 小红书卡片 | - | `views/xiaohongshuCard/List.vue` | `src/router/modules/xiaohongshuCard.js` |
| 235 | [ ] | `/xiaohongshu/stats` | XiaohongshuStats | 小红书卡片统计 | - | `views/xiaohongshuCard/Stats.vue` | `src/router/modules/xiaohongshuCard.js` |
| 236 | [ ] | `/xiaohongshu-card-stats/:id` | XiaohongshuCardStats | 小红书卡片详情统计 | - | `views/xiaohongshuCard/CardStats.vue` | `src/router/modules/xiaohongshuCard.js` |
