# 架构违规清单

**扫描日期：** 2026-10-12
**扫描范围：** `hivemtk/user-server/internal/`

---

## 1. Router 层内联闭包

### 扫描结果

| 文件 | 行号 | 类型 | 是否违规 |
|------|------|------|----------|
| `router/files_guard.go` | 50 | 中间件返回函数 | 否（中间件模式） |
| `router/swagger.go` | 41 | Swagger 路由 | 否（特殊路由） |
| `router/swagger.go` | 49 | Swagger JSON | 否（特殊路由） |
| `router/embed_static_routes.go` | 48,56,60,64,84,90,99 | 静态文件路由 | 否（特殊路由） |
| `router/health.go` | 73,136,157 | 健康检查 | 否（特殊路由） |
| `router/router.go` | 96 | 中间件返回函数 | 否（中间件模式） |
| `router/router.go` | 174 | NoMethod 处理 | 否（特殊路由） |

### 结论
Router 层未发现违规内联闭包。所有 `func(c *gin.Context)` 均为中间件返回函数或特殊路由处理，符合规范。

---

## 2. Controller 层跨层调用

### 扫描结果

| 文件 | 行号 | 内容 | 是否违规 |
|------|------|------|----------|
| `controller/alert_rule.go` | 19 | 注释说明调用链 | 否（注释） |
| `controller/xianyu_card_stats.go` | 16 | 注释说明已移除越层调用 | 否（注释） |
| `controller/customer_oneid.go` | 18 | 注释说明已移除越层调用 | 否（注释） |
| `controller/kb_release.go` | 493 | 注释说明 | 否（注释） |
| `controller/recovery_queue.go` | 20 | 注释说明 | 否（注释） |

### 结论
Controller 层未发现实际跨层调用违规。注释表明之前存在越层调用但已修复。

---

## 3. Service 层直接操作 DB

### 扫描结果

**违规数量：** 151 个函数接受 `*gorm.DB` 参数

**主要违规模式：**

#### 3.1 构造函数接受 `*gorm.DB` 参数（约 60+ 个）

| 文件 | 函数 |
|------|------|
| `service/sso.go` | `NewSSOService(db *gorm.DB)` |
| `service/webhook.go` | `NewWebhookService(db *gorm.DB)` |
| `service/sop_outbox_dispatcher.go` | `NewSOPOutboxDispatcher(db *gorm.DB, ...)` |
| `service/sop_outbox_dispatcher.go` | `NewSOPStuckDetector(db *gorm.DB, ...)` |
| `service/sms_delivery_tracker.go` | `NewSmsDeliveryTrackerService(db *gorm.DB, ...)` |
| `service/feedback_loop_cron.go` | `NewFeedbackLoopCron(db *gorm.DB, ...)` |
| `service/memory_facade.go` | `NewMemoryFacade(db *gorm.DB, ...)` |
| `service/telegram_gate.go` | `NewTelegramGateService(db *gorm.DB)` |
| `service/knowledge_base.go` | `NewKnowledgeBaseService(db *gorm.DB)` |
| `service/sop_template.go` | `NewSOPTemplateService(db *gorm.DB, ...)` |
| `service/feedback_learning_cron.go` | `NewFeedbackLearningCron(db *gorm.DB)` |
| `service/sales_persona.go` | `NewSalesPersonaServiceWithDB(db *gorm.DB)` |
| `service/memory_system.go` | `InitMemorySystem(db *gorm.DB)` |
| `service/sop.go` | `NewSOPService(db *gorm.DB, ...)` |
| `service/config_param.go` | `NewConfigParamService(db *gorm.DB)` |
| `service/ai_agent.go` | `NewAIAgentServiceWithDB(db *gorm.DB)` |
| `service/security_rules.go` | `NewSecurityRulesService(db *gorm.DB)` |
| `service/intent_recognition.go` | `NewIntentRecognizer(db *gorm.DB, ...)` |
| `service/agent_kb_binding.go` | `NewAgentKBBindingService(db *gorm.DB)` |
| `service/feedback_learner.go` | `NewFeedbackLearner(db *gorm.DB)` |
| `service/rag_health.go` | `NewRagHealthService(db *gorm.DB, ...)` |
| `service/sop_dispatcher.go` | `NewSOPExecutionDispatcher(db *gorm.DB, ...)` |
| `service/qq_account.go` | `NewQQService(db *gorm.DB)` |
| `service/dialogue_memory.go` | `NewDialogueMemoryService(db *gorm.DB, ...)` |
| `service/order_draft.go` | `NewOrderDraftServiceWithDB(..., db *gorm.DB)` |
| `service/support_metrics.go` | `NewSupportMetricsService(db *gorm.DB)` |
| `service/sop_scheduler.go` | `NewSOPScheduler(..., db *gorm.DB, ...)` |
| `service/order.go` | `NewOrderServiceWithDB(db *gorm.DB)` |
| `service/xiaohongshu_card.go` | `NewXiaohongshuCardService(db *gorm.DB)` |
| `service/password_reset.go` | `NewPasswordResetService(db *gorm.DB)` |
| `service/chat_channel.go` | `NewChatChannelService(db *gorm.DB)` |
| `service/notification.go` | `NewNotificationService(db *gorm.DB)` |
| `service/security_audit.go` | `NewSecurityAuditService(gdb *gorm.DB)` |
| `service/douyin_card_stats.go` | `NewDouyinCardStatsService(db *gorm.DB)` |
| `service/inbox_ingress.go` | `NewInboxIngressServiceWithDB(db *gorm.DB, ...)` |
| `service/rag_metrics.go` | `NewRagMetricsService(db *gorm.DB)` |

#### 3.2 方法接受 `*gorm.DB` 参数（约 90+ 个）

| 文件 | 函数 |
|------|------|
| `service/webhook.go` | `lazyDB() *gorm.DB` |
| `service/channel_media.go` | `dbForStorage(_ context.Context) *gorm.DB` |
| `service/memory_decay.go` | `RunMemoryDecayJob(ctx, db *gorm.DB, ...)` |
| `service/memory_decay.go` | `StartMemoryDecayLoop(ctx, db *gorm.DB, ...)` |
| `service/memory_decay.go` | `RunMemoryDecayOnce(db *gorm.DB)` |
| `service/memory_decay.go` | `runMemoryDecayOnce(ctx, db *gorm.DB)` |
| `service/telegram_gate.go` | `StartGateSweeper(db *gorm.DB)` |
| `service/knowledge_update.go` | `WithDB(d *gorm.DB)` |
| `service/config_param.go` | `SeedConfigParams(ctx, db *gorm.DB)` |
| `service/config_param.go` | `SeedParamGroups(ctx, db *gorm.DB)` |
| `service/config_param.go` | `syncParamDef(ctx, db *gorm.DB, ...)` |
| `service/security_audit.go` | `withDB(ctx) *gorm.DB` |
| `service/agent_kb_binding.go` | `withTxBeginner(ctx) *gorm.DB` |

### 结论
**严重违规**：Service 层大量函数直接接受 `*gorm.DB` 参数，违反五层架构规范。Service 层应通过 Repository 层访问数据库，不应直接持有或传递 `*gorm.DB`。

---

## 4. Repository 层业务判断

### 扫描结果

Repository 层文件数量：284 个

**需要进一步扫描确认是否包含业务判断逻辑。**

---

## 5. Model 层外部依赖

### 扫描结果

Model 层文件数量：215 个

**需要进一步扫描确认是否有外部依赖。**

---

## 总结

| 违规类型 | 数量 | 严重程度 |
|----------|------|----------|
| Router 内联闭包 | 0 | 无 |
| Controller 跨层调用 | 0 | 无 |
| Service 直接操作 DB | 151 | 严重 |
| Repository 业务判断 | 待确认 | 待确认 |
| Model 外部依赖 | 待确认 | 待确认 |

**主要问题：** Service 层大量函数直接接受 `*gorm.DB` 参数，严重违反五层架构规范。需要重构为通过 Repository 层访问数据库。
