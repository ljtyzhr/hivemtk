# 命名违规清单

**扫描日期：** 2026-10-12
**扫描范围：** `hivemtk/user-server/internal/controller/`

---

## 1. 控制器命名规范

### 规范要求
- 控制器类型：`XxxController`（如 `CustomerController`）
- 禁止 `XxxHandler` 同义词

### 扫描结果

| 类型名 | 文件 | 是否违规 |
|--------|------|----------|
| `TypingPredictController` | typing_predict.go:15 | 否 |
| `ManageTypingPredictController` | typing_predict.go:110 | 否 |
| `ChatPublicController` | chat_public.go:45 | 否 |
| `AlertRuleController` | alert_rule.go:27 | 否 |
| `PermissionController` | permission.go:15 | 否 |
| `SSOController` | sso.go:38 | 否 |
| `WebhookController` | webhook.go:22 | 否 |
| `RagEvalController` | rag_eval.go:15 | 否 |
| `ManageRagEvalController` | rag_eval.go:76 | 否 |
| `WhatsappController` | whatsapp.go:13 | 否 |
| `SmsDeliveryTrackerController` | sms_delivery_tracker.go:15 | 否 |
| `LLMRoutingController` | llm_routing.go:15 | 否 |
| `EmailListController` | email_list.go:18 | 否 |
| `TelegramGateController` | telegram_gate.go:15 | 否 |
| `KnowledgeBaseController` | knowledge_base.go:17 | 否 |
| `SOPTemplateController` | sop_template.go:18 | 否 |
| `GroupMessagingController` | whatsapp_group_messaging.go:27 | 否 |
| `CustomerJourneyController` | customer_journey.go:15 | 否 |
| `FeatureFlagController` | feature_flag.go:15 | 否 |
| `SalesPersonaController` | sales_persona.go:15 | 否 |
| `MacroController` | r48_session_tools.go:13 | 否 |
| `SessionAIController` | r48_session_tools.go:82 | 否 |
| `OperationLogController` | operation_log.go:22 | 否 |
| `SalesWorkbenchController` | sales_workbench.go:15 | 否 |
| `MarketingFlowSyncController` | r39_extras.go:19 | 否 |
| `CustomerEventBatchController` | r39_extras.go:61 | 否 |
| `WebVitalsController` | r39_extras.go:124 | 否 |
| `BackupController` | backup.go:14 | 否 |
| `RestoreController` | backup.go:215 | 否 |
| `CardStatsFactoryController` | card_stats_factory.go:22 | 否 |
| `SmartRouterController` | smart_router.go:14 | 否 |
| `ManageSmartRouterController` | smart_router.go:46 | 否 |
| `SOPController` | sop.go:18 | 否 |
| `ConfigParamController` | config_param.go:18 | 否 |
| `CustomerEventController` | customer_event.go:16 | 否 |
| `WorkflowOrchestratorController` | workflow_orchestrator.go:17 | 否 |
| `UserController` | user.go:13 | 否 |
| `AIAgentController` | ai_agent.go:17 | 否 |
| `ChannelAgentBindingController` | ai_agent.go:453 | 否 |
| `CustomerServiceAgentController` | ai_agent.go:593 | 否 |
| `SSEDashboardController` | sse_dashboard.go:17 | 否 |
| `TraceController` | trace.go:20 | 否 |
| `AuthController` | auth.go:21 | 否 |
| `SystemUserController` | auth.go:571 | 否 |
| `SopHeatmapController` | sop_heatmap.go:14 | 否 |
| `HelpCenterController` | help_center.go:13 | 否 |
| `SessionTagController` | session_tag.go:13 | 否 |
| `KBConnectorController` | kb_connectors.go:15 | 否 |
| `SystemMonitorController` | system_monitor.go:12 | 否 |
| `BillController` | bill.go:88 | 否 |

### 结论
**未发现命名违规。** 所有控制器类型均使用 `XxxController` 命名，符合规范。

---

## 2. 其他命名检查

### 扫描结果
- 未发现 `XxxHandler` 同义词
- 未发现其他命名违规

---

## 总结

| 违规类型 | 数量 | 严重程度 |
|----------|------|----------|
| 控制器命名 | 0 | 无 |
| XxxHandler 同义词 | 0 | 无 |

**命名规范执行情况良好。**
