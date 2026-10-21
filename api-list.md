# API 路由清单（任务三 · SaaS 多租户改造后逐路由 curl 验证）

- 来源：`user-web/src/api/*.js`（111 个文件）
- 规模：**1028 条前端 API 出口**
- 生成方式：`scripts/gen_api_list.py`（只做索引，不发请求）
- 校验口径：每条路由**逐条手工 curl**，分析 `code` 与 `data`；
  `code=0` 通过；`code!=0` 判定是调用错误还是业务错误，即时修复

## 档位说明

| 档位 | 含义 |
|---|---|
| 公开 | 无需登录（登录、模块市场、公开聊天） |
| 用户端 | 需登录的普通业务接口 `/api/*` |
| 管理端 | 管理面接口 `/api/manage/*`、`/api/platform/*` |

## 清单

| # | 状态 | 方法 | 路径 | 档位 | 源文件 | 行 |
|---|---|---|---|---|---|---|
| 1 | [x] | GET | `/api/ab-experiments` | 用户端 | `src/api/abExperiment.js` | 4 |
| 2 | [ ] | GET | `/api/ab-experiments/{id}` | 用户端 | `src/api/abExperiment.js` | 7 |
| 3 | [ ] | POST | `/api/ab-experiments` | 用户端 | `src/api/abExperiment.js` | 10 |
| 4 | [ ] | PUT | `/api/ab-experiments/{id}` | 用户端 | `src/api/abExperiment.js` | 13 |
| 5 | [ ] | DELETE | `/api/ab-experiments/{id}` | 用户端 | `src/api/abExperiment.js` | 16 |
| 6 | [ ] | POST | `/api/ab-experiments/{id}/start` | 用户端 | `src/api/abExperiment.js` | 19 |
| 7 | [ ] | POST | `/api/ab-experiments/{id}/pause` | 用户端 | `src/api/abExperiment.js` | 22 |
| 8 | [ ] | POST | `/api/ab-experiments/{id}/stop` | 用户端 | `src/api/abExperiment.js` | 25 |
| 9 | [x] | GET | `/api/ab-experiments/{id}/results` | 用户端 | `src/api/abExperiment.js` | 28 |
| 10 | [x] | GET | `/api/ab-experiments/{id}/conversion-events` | 用户端 | `src/api/abExperiment.js` | 31 |
| 11 | [x] | GET | `/api/ab-experiments` | 用户端 | `src/api/abExperiment.js` | 41 |
| 12 | [x] | GET | `/api/ab-experiments/{id}/stats` | 用户端 | `src/api/abExperimentPlus.js` | 4 |
| 13 | [x] | GET | `/api/ab-experiments/{id}/diagnostics` | 用户端 | `src/api/abExperimentPlus.js` | 7 |
| 14 | [x] | GET | `/api/ab-experiments/{id}/cuped` | 用户端 | `src/api/abExperimentPlus.js` | 10 |
| 15 | [x] | POST | `/api/ab-experiments/{id}/sequential-test` | 用户端 | `src/api/abExperimentPlus.js` | 13 |
| 16 | [x] | POST | `/api/ab-experiments/{id}/bayesian-test` | 用户端 | `src/api/abExperimentPlus.js` | 16 |
| 17 | [ ] | POST | `/api/agent-kb-bindings` | 用户端 | `src/api/agentKBBinding.js` | 4 |
| 18 | [ ] | DELETE | `/api/agent-kb-bindings/{agentId}/{kbId}` | 用户端 | `src/api/agentKBBinding.js` | 8 |
| 19 | [x] | GET | `/api/agent-kb-bindings/by-agent/{agentId}` | 用户端 | `src/api/agentKBBinding.js` | 12 |
| 20 | [x] | GET | `/api/agent-kb-bindings/by-kb/{kbId}` | 用户端 | `src/api/agentKBBinding.js` | 16 |
| 21 | [x] | PUT | `/api/agent-kb-bindings/by-agent/{agentId}` | 用户端 | `src/api/agentKBBinding.js` | 20 |
| 22 | [x] | GET | `/api/ai-agents` | 用户端 | `src/api/aiAgent.js` | 4 |
| 23 | [x] | GET | `/api/ai-agents-enabled` | 用户端 | `src/api/aiAgent.js` | 8 |
| 24 | [ ] | GET | `/api/ai-agents/{id}` | 用户端 | `src/api/aiAgent.js` | 12 |
| 25 | [ ] | POST | `/api/ai-agents` | 用户端 | `src/api/aiAgent.js` | 16 |
| 26 | [ ] | PUT | `/api/ai-agents/{id}` | 用户端 | `src/api/aiAgent.js` | 20 |
| 27 | [ ] | DELETE | `/api/ai-agents/{id}` | 用户端 | `src/api/aiAgent.js` | 24 |
| 28 | [ ] | POST | `/api/ai-agents/{id}/toggle` | 用户端 | `src/api/aiAgent.js` | 28 |
| 29 | [ ] | POST | `/api/ai-agents/{id}/test` | 用户端 | `src/api/aiAgent.js` | 32 |
| 30 | [ ] | GET | `/api/ai-agents/{id}/context` | 用户端 | `src/api/aiAgent.js` | 36 |
| 31 | [x] | GET | `/api/ai-productivity/overview` | 用户端 | `src/api/aiProductivity.js` | 5 |
| 32 | [x] | GET | `/api/ai-productivity/trend` | 用户端 | `src/api/aiProductivity.js` | 8 |
| 33 | [x] | GET | `/api/ai-tools` | 用户端 | `src/api/aiTool.js` | 4 |
| 34 | [ ] | GET | `/api/ai-tools/{name}` | 用户端 | `src/api/aiTool.js` | 8 |
| 35 | [ ] | PUT | `/api/ai-tools/{name}/status` | 用户端 | `src/api/aiTool.js` | 12 |
| 36 | [ ] | POST | `/api/ai-tools/batch-status` | 用户端 | `src/api/aiTool.js` | 16 |
| 37 | [x] | GET | `/api/ai-tools/{toolName}/accounts` | 用户端 | `src/api/aiTool.js` | 20 |
| 38 | [ ] | POST | `/api/ai-tools/{toolName}/accounts` | 用户端 | `src/api/aiTool.js` | 24 |
| 39 | [ ] | DELETE | `/api/ai-tools/{toolName}/accounts/{accountType}/{accountId}` | 用户端 | `src/api/aiTool.js` | 32 |
| 40 | [~] | GET | `/api/approvals/{id}` | 用户端 | `src/api/approval.js` | 18 |
| 41 | [~] | POST | `/api/approvals/{id}/decide` | 用户端 | `src/api/approval.js` | 27 |
| 42 | [ ] | POST | `/api/asset-bundle` | 用户端 | `src/api/assetBundle.js` | 4 |
| 43 | [ ] | PUT | `/api/asset-bundle/{id}` | 用户端 | `src/api/assetBundle.js` | 7 |
| 44 | [ ] | GET | `/api/asset-bundle/{id}` | 用户端 | `src/api/assetBundle.js` | 10 |
| 45 | [ ] | GET | `/api/asset-bundle/by-aid/{aid}` | 用户端 | `src/api/assetBundle.js` | 13 |
| 46 | [x] | POST | `/api/asset-bundle/list` | 用户端 | `src/api/assetBundle.js` | 16 |
| 47 | [ ] | POST | `/api/asset-bundle/{id}/publish` | 用户端 | `src/api/assetBundle.js` | 19 |
| 48 | [ ] | POST | `/api/asset-bundle/{id}/submit-platform` | 用户端 | `src/api/assetBundle.js` | 22 |
| 49 | [ ] | POST | `/api/asset-bundle/{id}/archive` | 用户端 | `src/api/assetBundle.js` | 25 |
| 50 | [ ] | DELETE | `/api/asset-bundle/{id}` | 用户端 | `src/api/assetBundle.js` | 28 |
| 51 | [ ] | POST | `/api/asset-bundle/weave` | 用户端 | `src/api/assetBundle.js` | 31 |
| 52 | [ ] | POST | `/api/asset-bundle/merchant-save` | 用户端 | `src/api/assetBundle.js` | 34 |
| 53 | [ ] | POST | `/api/asset-bundle/merchant-parse/{aid}` | 用户端 | `src/api/assetBundle.js` | 37 |
| 54 | [ ] | POST | `/api/asset-bundle/{id}/enable` | 用户端 | `src/api/assetBundle.js` | 40 |
| 55 | [ ] | POST | `/api/asset-bundle/{id}/disable` | 用户端 | `src/api/assetBundle.js` | 43 |
| 56 | [x] | POST | `/api/asset-bundle/enabled/list` | 用户端 | `src/api/assetBundle.js` | 46 |
| 57 | [ ] | POST | `/api/upload` | 用户端 | `src/api/assetBundle.js` | 51 |
| 58 | [x] | GET | `/api/v1/asset-market/list` | 用户端 | `src/api/assetMarket.js` | 4 |
| 59 | [x] | GET | `/api/v1/asset-market/detail/{id}` | 用户端 | `src/api/assetMarket.js` | 7 |
| 60 | [ ] | POST | `/api/v1/asset-market/purchase` | 用户端 | `src/api/assetMarket.js` | 10 |
| 61 | [ ] | POST | `/api/v1/asset-market/sync` | 用户端 | `src/api/assetMarket.js` | 13 |
| 62 | [ ] | POST | `/api/v1/asset-market/report-usage` | 用户端 | `src/api/assetMarket.js` | 16 |
| 63 | [x] | GET | `/api/v1/local-assets` | 用户端 | `src/api/assetMarket.js` | 19 |
| 64 | [ ] | GET | `/api/v1/local-assets/{id}` | 用户端 | `src/api/assetMarket.js` | 22 |
| 65 | [ ] | POST | `/api/v1/local-assets` | 用户端 | `src/api/assetMarket.js` | 25 |
| 66 | [ ] | PUT | `/api/v1/local-assets/{id}` | 用户端 | `src/api/assetMarket.js` | 28 |
| 67 | [x] | DELETE | `/api/v1/local-assets/{id}` | 用户端 | `src/api/assetMarket.js` | 31 |
| 68 | [x] | PUT | `/api/v1/local-assets/{id}/toggle-active` | 用户端 | `src/api/assetMarket.js` | 34 |
| 69 | [x] | GET | `/api/v1/local-assets/sync-log` | 用户端 | `src/api/assetMarket.js` | 37 |
| 70 | [x] | GET | `/api/backups` | 用户端 | `src/api/backup.js` | 4 |
| 71 | [ ] | GET | `/api/backups/{id}` | 用户端 | `src/api/backup.js` | 8 |
| 72 | [ ] | POST | `/api/backups` | 用户端 | `src/api/backup.js` | 12 |
| 73 | [ ] | DELETE | `/api/backups/{id}` | 用户端 | `src/api/backup.js` | 16 |
| 74 | [ ] | POST | `/api/restore` | 用户端 | `src/api/backup.js` | 20 |
| 75 | [x] | GET | `/api/restore/list` | 用户端 | `src/api/backup.js` | 24 |
| 76 | [x] | GET | `/api/restore/last` | 用户端 | `src/api/backup.js` | 28 |
| 77 | [x] | GET | `/api/bad-cases` | 用户端 | `src/api/badCase.js` | 44 |
| 78 | [x] | GET | `/api/bad-cases/stats` | 用户端 | `src/api/badCase.js` | 50 |
| 79 | [x] | GET | `/api/bad-cases/taxonomy` | 用户端 | `src/api/badCase.js` | 57 |
| 80 | [ ] | GET | `/api/bad-cases/{id}` | 用户端 | `src/api/badCase.js` | 61 |
| 81 | [ ] | POST | `/api/bad-cases` | 用户端 | `src/api/badCase.js` | 69 |
| 82 | [ ] | POST | `/api/bad-cases/{id}/label` | 用户端 | `src/api/badCase.js` | 76 |
| 83 | [ ] | POST | `/api/bad-cases/{id}/dismiss` | 用户端 | `src/api/badCase.js` | 82 |
| 84 | [ ] | POST | `/api/bad-cases/export` | 用户端 | `src/api/badCase.js` | 89 |
| 85 | [ ] | POST | `/api/batch/import` | 用户端 | `src/api/batchOperation.js` | 4 |
| 86 | [ ] | GET | `/api/batch/template` | 用户端 | `src/api/batchOperation.js` | 7 |
| 87 | [ ] | POST | `/api/batch/export` | 用户端 | `src/api/batchOperation.js` | 10 |
| 88 | [ ] | POST | `/api/batch/delete` | 用户端 | `src/api/batchOperation.js` | 13 |
| 89 | [ ] | POST | `/api/batch/update` | 用户端 | `src/api/batchOperation.js` | 16 |
| 90 | [x] | GET | `/api/batch/tools` | 用户端 | `src/api/batchOperation.js` | 20 |
| 91 | [x] | GET | `/api/batch/histories` | 用户端 | `src/api/batchOperation.js` | 26 |
| 92 | [ ] | POST | `/api/batch/histories/{id}/cancel` | 用户端 | `src/api/batchOperation.js` | 29 |
| 93 | [ ] | GET | `/api/batch/histories/{id}` | 用户端 | `src/api/batchOperation.js` | 32 |
| 94 | [ ] | POST | `/api/batch/preview` | 用户端 | `src/api/batchOperation.js` | 35 |
| 95 | [x] | GET | `/api/bridge/token/status` | 用户端 | `src/api/bridgeToken.js` | 3 |
| 96 | [x] | POST | `/api/bridge/token/reset` | 用户端 | `src/api/bridgeToken.js` | 5 |
| 97 | [x] | GET | `/api/browser-automation/tasks` | 用户端 | `src/api/browserAutomation.js` | 6 |
| 98 | [ ] | GET | `/api/browser-automation/tasks/{id}` | 用户端 | `src/api/browserAutomation.js` | 9 |
| 99 | [ ] | POST | `/api/browser-automation/tasks` | 用户端 | `src/api/browserAutomation.js` | 12 |
| 100 | [ ] | PUT | `/api/browser-automation/tasks/{id}` | 用户端 | `src/api/browserAutomation.js` | 15 |
| 101 | [ ] | DELETE | `/api/browser-automation/tasks/{id}` | 用户端 | `src/api/browserAutomation.js` | 18 |
| 102 | [ ] | POST | `/api/browser-automation/tasks/{id}/publish` | 用户端 | `src/api/browserAutomation.js` | 21 |
| 103 | [ ] | POST | `/api/browser-automation/tasks/{id}/run` | 用户端 | `src/api/browserAutomation.js` | 27 |
| 104 | [ ] | POST | `/api/browser-automation/tasks/{id}/pause` | 用户端 | `src/api/browserAutomation.js` | 30 |
| 105 | [ ] | POST | `/api/browser-automation/tasks/{id}/resume` | 用户端 | `src/api/browserAutomation.js` | 33 |
| 106 | [ ] | POST | `/api/browser-automation/tasks/{id}/archive` | 用户端 | `src/api/browserAutomation.js` | 36 |
| 107 | [ ] | PUT | `/api/browser-automation/tasks/{id}/dependency` | 用户端 | `src/api/browserAutomation.js` | 39 |
| 108 | [x] | GET | `/api/browser-automation/sessions` | 用户端 | `src/api/browserAutomation.js` | 44 |
| 109 | [ ] | GET | `/api/browser-automation/sessions/{id}` | 用户端 | `src/api/browserAutomation.js` | 47 |
| 110 | [ ] | GET | `/api/browser-automation/sessions/{id}/steps` | 用户端 | `src/api/browserAutomation.js` | 50 |
| 111 | [x] | GET | `/api/browser-automation/tasks/{taskId}/sessions` | 用户端 | `src/api/browserAutomation.js` | 54 |
| 112 | [ ] | POST | `/api/browser-automation/sessions/{id}/stop` | 用户端 | `src/api/browserAutomation.js` | 57 |
| 113 | [ ] | GET | `/api/browser-automation/sessions/{id}/confirm-gate` | 用户端 | `src/api/browserAutomation.js` | 62 |
| 114 | [ ] | POST | `/api/browser-automation/sessions/{id}/confirm` | 用户端 | `src/api/browserAutomation.js` | 72 |
| 115 | [ ] | GET | `/api/browser-automation/sessions/{id}/logs` | 用户端 | `src/api/browserAutomation.js` | 93 |
| 116 | [ ] | GET | `/api/browser-automation/sessions/{id}/export` | 用户端 | `src/api/browserAutomation.js` | 97 |
| 117 | [ ] | GET | `/api/browser-automation/tasks/{taskId}/receipts` | 用户端 | `src/api/browserAutomation.js` | 102 |
| 118 | [x] | GET | `/api/browser-automation/cron` | 用户端 | `src/api/browserAutomation.js` | 107 |
| 119 | [ ] | POST | `/api/browser-automation/cron` | 用户端 | `src/api/browserAutomation.js` | 110 |
| 120 | [ ] | PUT | `/api/browser-automation/cron/{id}` | 用户端 | `src/api/browserAutomation.js` | 113 |
| 121 | [ ] | DELETE | `/api/browser-automation/cron/{id}` | 用户端 | `src/api/browserAutomation.js` | 116 |
| 122 | [ ] | POST | `/api/browser-automation/cron/{id}/enable` | 用户端 | `src/api/browserAutomation.js` | 119 |
| 123 | [ ] | POST | `/api/browser-automation/cron/{id}/disable` | 用户端 | `src/api/browserAutomation.js` | 122 |
| 124 | [x] | GET | `/api/browser-automation/host/status` | 用户端 | `src/api/browserAutomation.js` | 127 |
| 125 | [x] | POST | `/api/browser-automation/host/token/reset` | 用户端 | `src/api/browserAutomation.js` | 130 |
| 126 | [x] | GET | `/api/browser-automation/platforms` | 用户端 | `src/api/browserAutomation.js` | 135 |
| 127 | [ ] | GET | `/api/browser-automation/platforms/{id}/locators` | 用户端 | `src/api/browserAutomation.js` | 138 |
| 128 | [x] | GET | `/api/whatsapp/templates` | 用户端 | `src/api/bulkMessaging.js` | 4 |
| 129 | [x] | POST | `/api/whatsapp/templates` | 用户端 | `src/api/bulkMessaging.js` | 8 |
| 130 | [ ] | PUT | `/api/whatsapp/templates/{id}` | 用户端 | `src/api/bulkMessaging.js` | 12 |
| 131 | [ ] | DELETE | `/api/whatsapp/templates/{id}` | 用户端 | `src/api/bulkMessaging.js` | 16 |
| 132 | [ ] | POST | `/api/whatsapp/group-messaging/send` | 用户端 | `src/api/bulkMessaging.js` | 20 |
| 133 | [x] | GET | `/api/whatsapp/group-messaging/status/{queueId}` | 用户端 | `src/api/bulkMessaging.js` | 24 |
| 134 | [x] | GET | `/api/whatsapp/group-messaging/records` | 用户端 | `src/api/bulkMessaging.js` | 28 |
| 135 | [ ] | GET | `/api/channel-agent-bindings` | 用户端 | `src/api/channelAgentBinding.js` | 4 |
| 136 | [x] | GET | `/api/channel-agent-bindings/by-agent/{agentId}` | 用户端 | `src/api/channelAgentBinding.js` | 8 |
| 137 | [ ] | POST | `/api/channel-agent-bindings` | 用户端 | `src/api/channelAgentBinding.js` | 12 |
| 138 | [ ] | PUT | `/api/channel-agent-bindings/{id}` | 用户端 | `src/api/channelAgentBinding.js` | 16 |
| 139 | [x] | DELETE | `/api/channel-agent-bindings/{id}` | 用户端 | `src/api/channelAgentBinding.js` | 20 |
| 140 | [ ] | GET | `/api/chat/public/upload-token` | 公开 | `src/api/chat.js` | 4 |
| 141 | [x] | GET | `/api/chat-channels` | 用户端 | `src/api/chatChannel.js` | 4 |
| 142 | [ ] | GET | `/api/chat-channels/{channelId}` | 用户端 | `src/api/chatChannel.js` | 8 |
| 143 | [ ] | POST | `/api/chat-channels` | 用户端 | `src/api/chatChannel.js` | 12 |
| 144 | [ ] | PUT | `/api/chat-channels/{channelId}` | 用户端 | `src/api/chatChannel.js` | 16 |
| 145 | [ ] | DELETE | `/api/chat-channels/{channelId}` | 用户端 | `src/api/chatChannel.js` | 20 |
| 146 | [ ] | POST | `/api/chat-channels/{channelId}/rotate-key` | 用户端 | `src/api/chatChannel.js` | 24 |
| 147 | [ ] | POST | `/api/chat-channels/{channelId}/reset-secret` | 用户端 | `src/api/chatChannel.js` | 28 |
| 148 | [ ] | GET | `/api/churn/prediction` | 用户端 | `src/api/churnPrediction.js` | 4 |
| 149 | [x] | GET | `/api/churn-prediction` | 用户端 | `src/api/churnPrediction.js` | 7 |
| 150 | [x] | GET | `/api/churn-prediction/users` | 用户端 | `src/api/churnPrediction.js` | 10 |
| 151 | [x] | GET | `/api/churn-prediction/warnings` | 用户端 | `src/api/churnPrediction.js` | 13 |
| 152 | [x] | GET | `/api/churn/unhandled-warnings` | 用户端 | `src/api/churnPrediction.js` | 16 |
| 153 | [x] | POST | `/api/churn/warnings/{id}/handle` | 用户端 | `src/api/churnPrediction.js` | 19 |
| 154 | [x] | GET | `/api/churn-prediction/model-config` | 用户端 | `src/api/churnPrediction.js` | 22 |
| 155 | [x] | POST | `/api/churn-prediction/model-config` | 用户端 | `src/api/churnPrediction.js` | 25 |
| 156 | [ ] | GET | `/api/churn-prediction/statistics` | 用户端 | `src/api/churnPrediction.js` | 28 |
| 157 | [x] | GET | `/api/churn/risk-distribution` | 用户端 | `src/api/churnPrediction.js` | 31 |
| 158 | [ ] | POST | `/api/churn/warnings/intervene` | 用户端 | `src/api/churnPrediction.js` | 38 |
| 159 | [x] | POST | `/api/user-segment/rfm/calculate` | 用户端 | `src/api/churnPrediction.js` | 50 |
| 160 | [x] | DELETE | `/api/clues/delete/{id}` | 用户端 | `src/api/clue.js` | 5 |
| 161 | [x] | GET | `/api/clues/list?{q}` | 用户端 | `src/api/clue.js` | 12 |
| 162 | [x] | GET | `/api/clues/statistics` | 用户端 | `src/api/clue.js` | 15 |
| 163 | [ ] | POST | `/api/clues/import` | 用户端 | `src/api/clue.js` | 18 |
| 164 | [x] | GET | `/api/system/stats` | 用户端 | `src/api/cockpit.js` | 4 |
| 165 | [x] | GET | `/api/monitor/health` | 用户端 | `src/api/cockpit.js` | 8 |
| 166 | [x] | GET | `/api/monitor/node-health` | 用户端 | `src/api/cockpit.js` | 12 |
| 167 | [x] | GET | `/api/monitor/anomalies` | 用户端 | `src/api/cockpit.js` | 16 |
| 168 | [x] | GET | `/api/ai/sales-cockpit` | 用户端 | `src/api/cockpit.js` | 24 |
| 169 | [ ] | POST | `/api/customer-sessions/{sessionId}/messages` | 用户端 | `src/api/collaboration.js` | 4 |
| 170 | [ ] | POST | `/api/customer-sessions/{sessionId}/internal-notes` | 用户端 | `src/api/collaboration.js` | 7 |
| 171 | [x] | GET | `/api/users/search` | 用户端 | `src/api/collaboration.js` | 10 |
| 172 | [x] | POST | `/api/mentions/{mentionId}/read` | 用户端 | `src/api/collaboration.js` | 13 |
| 173 | [x] | GET | `/api/mentions/mine` | 用户端 | `src/api/collaboration.js` | 16 |
| 174 | [x] | POST | `/api/customer-sessions/{sessionId}/edit-lock` | 用户端 | `src/api/collaboration.js` | 19 |
| 175 | [x] | DELETE | `/api/customer-sessions/{sessionId}/edit-lock` | 用户端 | `src/api/collaboration.js` | 22 |
| 176 | [x] | GET | `/api/customer-sessions/{sessionId}/edit-lock` | 用户端 | `src/api/collaboration.js` | 25 |
| 177 | [x] | GET | `/api/customer-sessions/{sessionId}/internal-notes` | 用户端 | `src/api/collaboration.js` | 28 |
| 178 | [x] | GET | `/api/community/groups` | 用户端 | `src/api/community.js` | 5 |
| 179 | [ ] | POST | `/api/community/groups` | 用户端 | `src/api/community.js` | 9 |
| 180 | [ ] | GET | `/api/community/groups/{id}` | 用户端 | `src/api/community.js` | 13 |
| 181 | [ ] | PUT | `/api/community/groups/{id}` | 用户端 | `src/api/community.js` | 17 |
| 182 | [ ] | DELETE | `/api/community/groups/{id}` | 用户端 | `src/api/community.js` | 21 |
| 183 | [ ] | GET | `/api/community/members` | 用户端 | `src/api/community.js` | 25 |
| 184 | [ ] | GET | `/api/community/messages` | 用户端 | `src/api/community.js` | 29 |
| 185 | [x] | GET | `/api/community/stats` | 用户端 | `src/api/community.js` | 33 |
| 186 | [ ] | POST | `/api/community/import` | 用户端 | `src/api/community.js` | 37 |
| 187 | [x] | POST | `/api/community/export` | 用户端 | `src/api/community.js` | 41 |
| 188 | [x] | GET | `/api/manage/config-params` | 管理端 | `src/api/configParams.js` | 8 |
| 189 | [x] | GET | `/api/manage/config-params/groups` | 管理端 | `src/api/configParams.js` | 17 |
| 190 | [x] | GET | `/api/manage/config-params/audit-logs` | 管理端 | `src/api/configParams.js` | 21 |
| 191 | [~] | PUT | `/api/manage/config-params/{group}/{key}` | 管理端 | `src/api/configParams.js` | 25 |
| 192 | [~] | POST | `/api/manage/config-params/{group}/{key}/reset` | 管理端 | `src/api/configParams.js` | 29 |
| 193 | [x] | POST | `/api/manage/config-params/{group}/reset` | 管理端 | `src/api/configParams.js` | 33 |
| 194 | [x] | GET | `/api/conversion-funnels` | 用户端 | `src/api/conversionFunnel.js` | 5 |
| 195 | [x] | GET | `/api/conversion-funnels/stage` | 用户端 | `src/api/conversionFunnel.js` | 8 |
| 196 | [ ] | POST | `/api/customer-sessions/{sessionId}/csat` | 用户端 | `src/api/csat.js` | 4 |
| 197 | [x] | GET | `/api/csat/stats` | 用户端 | `src/api/csat.js` | 7 |
| 198 | [x] | GET | `/api/csat/trend` | 用户端 | `src/api/csat.js` | 10 |
| 199 | [x] | GET | `/api/csat/negative` | 用户端 | `src/api/csat.js` | 13 |
| 200 | [ ] | POST | `/api/customer-sessions/{sessionId}/csat/trigger` | 用户端 | `src/api/csat.js` | 16 |
| 201 | [x] | GET | `/api/csat/template` | 用户端 | `src/api/csat.js` | 18 |
| 202 | [ ] | PUT | `/api/csat/template` | 用户端 | `src/api/csat.js` | 19 |
| 203 | [x] | GET | `/api/custom-reports` | 用户端 | `src/api/customReport.js` | 4 |
| 204 | [ ] | GET | `/api/custom-reports/{id}` | 用户端 | `src/api/customReport.js` | 7 |
| 205 | [ ] | POST | `/api/custom-reports` | 用户端 | `src/api/customReport.js` | 10 |
| 206 | [ ] | PUT | `/api/custom-reports/{id}` | 用户端 | `src/api/customReport.js` | 13 |
| 207 | [ ] | DELETE | `/api/custom-reports/{id}` | 用户端 | `src/api/customReport.js` | 16 |
| 208 | [x] | GET | `/api/custom-reports/templates` | 用户端 | `src/api/customReport.js` | 19 |
| 209 | [ ] | POST | `/api/custom-reports/templates/{id}/use` | 用户端 | `src/api/customReport.js` | 22 |
| 210 | [ ] | GET | `/api/custom-reports/{id}/data` | 用户端 | `src/api/customReport.js` | 25 |
| 211 | [ ] | GET | `/api/custom-reports/{id}/data?format=export` | 用户端 | `src/api/customReport.js` | 41 |
| 212 | [ ] | GET | `/api/custom-reports/{id}/data?format=run` | 用户端 | `src/api/customReport.js` | 44 |
| 213 | [x] | GET | `/api/customer-360/basic?user_id={id}` | 用户端 | `src/api/customer360.js` | 4 |
| 214 | [ ] | GET | `/api/customer-360/events` | 用户端 | `src/api/customer360.js` | 7 |
| 215 | [ ] | GET | `/api/customer-360/sessions` | 用户端 | `src/api/customer360.js` | 10 |
| 216 | [x] | GET | `/api/customer-360/tags?user_id={id}` | 用户端 | `src/api/customer360.js` | 13 |
| 217 | [ ] | GET | `/api/customer-360/orders` | 用户端 | `src/api/customer360.js` | 17 |
| 218 | [x] | GET | `/api/customer-360/list` | 用户端 | `src/api/customer360.js` | 43 |
| 219 | [x] | GET | `/api/customer-360/basic?user_id={id}` | 用户端 | `src/api/customer360.js` | 46 |
| 220 | [x] | GET | `/api/customer-360/tags?user_id={id}` | 用户端 | `src/api/customer360.js` | 64 |
| 221 | [ ] | PUT | `/api/customer-360/tags?user_id={id}` | 用户端 | `src/api/customer360.js` | 73 |
| 222 | [ ] | PUT | `/api/customer-360/tags?user_id={id}` | 用户端 | `src/api/customer360.js` | 78 |
| 223 | [ ] | POST | `/api/events/track` | 用户端 | `src/api/customerEvent.js` | 4 |
| 224 | [ ] | POST | `/api/events/pageview` | 用户端 | `src/api/customerEvent.js` | 7 |
| 225 | [ ] | POST | `/api/events/click` | 用户端 | `src/api/customerEvent.js` | 10 |
| 226 | [ ] | POST | `/api/events/purchase` | 用户端 | `src/api/customerEvent.js` | 13 |
| 227 | [ ] | POST | `/api/events/signup` | 用户端 | `src/api/customerEvent.js` | 16 |
| 228 | [ ] | POST | `/api/events/login` | 用户端 | `src/api/customerEvent.js` | 19 |
| 229 | [ ] | POST | `/api/events/add-to-cart` | 用户端 | `src/api/customerEvent.js` | 22 |
| 230 | [x] | GET | `/api/events/customer/{customerId}` | 用户端 | `src/api/customerEvent.js` | 25 |
| 231 | [x] | GET | `/api/events/stats` | 用户端 | `src/api/customerEvent.js` | 28 |
| 232 | [x] | DELETE | `/api/events/customer/{id}` | 用户端 | `src/api/customerEvent.js` | 40 |
| 233 | [x] | GET | `/api/customer-events/list` | 用户端 | `src/api/customerEvent.js` | 44 |
| 234 | [x] | GET | `/api/customer-journey/overview` | 用户端 | `src/api/customerJourney.js` | 4 |
| 235 | [x] | GET | `/api/customer-journey/overview?customer_id={customerId}` | 用户端 | `src/api/customerJourney.js` | 8 |
| 236 | [x] | GET | `/api/customer-journey/stages` | 用户端 | `src/api/customerJourney.js` | 12 |
| 237 | [x] | GET | `/api/customer-journey/by-stage?stage={stage}` | 用户端 | `src/api/customerJourney.js` | 16 |
| 238 | [ ] | POST | `/api/customer-journey/transition` | 用户端 | `src/api/customerJourney.js` | 20 |
| 239 | [ ] | POST | `/api/customer-journey/touch` | 用户端 | `src/api/customerJourney.js` | 24 |
| 240 | [ ] | POST | `/api/agents` | 用户端 | `src/api/customerService.js` | 4 |
| 241 | [x] | GET | `/api/agents/{id}` | 用户端 | `src/api/customerService.js` | 7 |
| 242 | [x] | GET | `/api/agents/online` | 用户端 | `src/api/customerService.js` | 10 |
| 243 | [x] | GET | `/api/agents/all` | 用户端 | `src/api/customerService.js` | 13 |
| 244 | [ ] | PUT | `/api/agents/{id}/status` | 用户端 | `src/api/customerService.js` | 16 |
| 245 | [x] | POST | `/api/agents/{id}/online` | 用户端 | `src/api/customerService.js` | 19 |
| 246 | [ ] | POST | `/api/agents/{id}/offline` | 用户端 | `src/api/customerService.js` | 22 |
| 247 | [x] | GET | `/api/agents/{id}/sessions` | 用户端 | `src/api/customerService.js` | 25 |
| 248 | [x] | GET | `/api/agents/me` | 用户端 | `src/api/customerService.js` | 28 |
| 249 | [x] | GET | `/api/quick-replies` | 用户端 | `src/api/customerService.js` | 32 |
| 250 | [x] | GET | `/api/quick-replies/categories` | 用户端 | `src/api/customerService.js` | 35 |
| 251 | [ ] | POST | `/api/quick-replies` | 用户端 | `src/api/customerService.js` | 38 |
| 252 | [ ] | PUT | `/api/quick-replies/{id}` | 用户端 | `src/api/customerService.js` | 41 |
| 253 | [ ] | DELETE | `/api/quick-replies/{id}` | 用户端 | `src/api/customerService.js` | 44 |
| 254 | [x] | GET | `/api/session-tags` | 用户端 | `src/api/customerService.js` | 48 |
| 255 | [ ] | POST | `/api/session-tags` | 用户端 | `src/api/customerService.js` | 51 |
| 256 | [ ] | PUT | `/api/session-tags/{id}` | 用户端 | `src/api/customerService.js` | 54 |
| 257 | [ ] | DELETE | `/api/session-tags/{id}` | 用户端 | `src/api/customerService.js` | 57 |
| 258 | [x] | GET | `/api/ai-suggestions/{sessionId}` | 用户端 | `src/api/customerService.js` | 61 |
| 259 | [x] | POST | `/api/ai-suggestions/{id}/use` | 用户端 | `src/api/customerService.js` | 64 |
| 260 | [ ] | POST | `/api/customer-sessions/{sessionId}/tags` | 用户端 | `src/api/customerService.js` | 68 |
| 261 | [ ] | GET | `/api/customer-service-agents` | 用户端 | `src/api/customerServiceAgent.js` | 4 |
| 262 | [x] | GET | `/api/customer-service-agents/by-ai-agent/{aiAgentId}` | 用户端 | `src/api/customerServiceAgent.js` | 8 |
| 263 | [ ] | POST | `/api/customer-service-agents` | 用户端 | `src/api/customerServiceAgent.js` | 12 |
| 264 | [ ] | PUT | `/api/customer-service-agents/{id}` | 用户端 | `src/api/customerServiceAgent.js` | 16 |
| 265 | [x] | DELETE | `/api/customer-service-agents/{id}` | 用户端 | `src/api/customerServiceAgent.js` | 20 |
| 266 | [x] | GET | `/api/customer-service-agents/by-user/{userId}` | 用户端 | `src/api/customerServiceAgent.js` | 24 |
| 267 | [ ] | POST | `/api/customer-service-agents/by-user/{userId}` | 用户端 | `src/api/customerServiceAgent.js` | 28 |
| 268 | [x] | GET | `/api/customer-sessions` | 用户端 | `src/api/customerSession.js` | 4 |
| 269 | [x] | GET | `/api/customer-sessions/{id}/messages` | 用户端 | `src/api/customerSession.js` | 7 |
| 270 | [ ] | POST | `/api/customer-sessions/{data_sessionId}/messages` | 用户端 | `src/api/customerSession.js` | 10 |
| 271 | [ ] | POST | `/api/customer-sessions` | 用户端 | `src/api/customerSession.js` | 18 |
| 272 | [x] | POST | `/api/customer-sessions/{id}/close` | 用户端 | `src/api/customerSession.js` | 21 |
| 273 | [ ] | POST | `/api/customer-sessions/{id}/transfer` | 用户端 | `src/api/customerSession.js` | 24 |
| 274 | [ ] | POST | `/api/customer-sessions/{id}/takeover` | 用户端 | `src/api/customerSession.js` | 28 |
| 275 | [ ] | POST | `/api/customer-sessions/{id}/release` | 用户端 | `src/api/customerSession.js` | 32 |
| 276 | [ ] | POST | `/api/customer-sessions/{id}/switch-handler` | 用户端 | `src/api/customerSession.js` | 36 |
| 277 | [x] | POST | `/api/customer-sessions/{id}/blacklist` | 用户端 | `src/api/customerSession.js` | 40 |
| 278 | [ ] | POST | `/api/customer-sessions/blacklist/remove` | 用户端 | `src/api/customerSession.js` | 44 |
| 279 | [ ] | GET | `/api/customer-sessions/blacklist/check` | 用户端 | `src/api/customerSession.js` | 48 |
| 280 | [x] | GET | `/api/customer-sessions/blacklist` | 用户端 | `src/api/customerSession.js` | 52 |
| 281 | [x] | GET | `/api/customer-360?user_id={id}` | 用户端 | `src/api/customerSession.js` | 56 |
| 282 | [x] | GET | `/api/customer-360/basic?user_id={id}` | 用户端 | `src/api/customerSession.js` | 59 |
| 283 | [x] | GET | `/api/customer-360/stats?user_id={id}` | 用户端 | `src/api/customerSession.js` | 62 |
| 284 | [x] | GET | `/api/customer-360/sessions?user_id={id}` | 用户端 | `src/api/customerSession.js` | 65 |
| 285 | [x] | GET | `/api/customer-360/tags?user_id={id}` | 用户端 | `src/api/customerSession.js` | 68 |
| 286 | [x] | GET | `/api/dashboards` | 用户端 | `src/api/dashboardScreen.js` | 4 |
| 287 | [ ] | GET | `/api/dashboards/{id}` | 用户端 | `src/api/dashboardScreen.js` | 7 |
| 288 | [ ] | POST | `/api/dashboards` | 用户端 | `src/api/dashboardScreen.js` | 10 |
| 289 | [ ] | PUT | `/api/dashboards/{id}` | 用户端 | `src/api/dashboardScreen.js` | 13 |
| 290 | [ ] | DELETE | `/api/dashboards/{id}` | 用户端 | `src/api/dashboardScreen.js` | 16 |
| 291 | [ ] | GET | `/api/dashboards/public/{code}` | 用户端 | `src/api/dashboardScreen.js` | 19 |
| 292 | [x] | GET | `/api/dashboards/data` | 用户端 | `src/api/dashboardScreen.js` | 23 |
| 293 | [x] | GET | `/api/dashboards/activities` | 用户端 | `src/api/dashboardScreen.js` | 26 |
| 294 | [x] | GET | `/api/dashboards/data` | 用户端 | `src/api/dashboardScreen.js` | 29 |
| 295 | [x] | GET | `/api/dashboards/data` | 用户端 | `src/api/dashboardScreen.js` | 32 |
| 296 | [x] | GET | `/api/dashboards/data` | 用户端 | `src/api/dashboardScreen.js` | 35 |
| 297 | [x] | GET | `/api/dashboards/data` | 用户端 | `src/api/dashboardScreen.js` | 38 |
| 298 | [ ] | POST | `/api/memory/messages` | 用户端 | `src/api/dialogueMemory.js` | 5 |
| 299 | [ ] | GET | `/api/memory/short` | 用户端 | `src/api/dialogueMemory.js` | 9 |
| 300 | [x] | GET | `/api/memory/long` | 用户端 | `src/api/dialogueMemory.js` | 13 |
| 301 | [ ] | POST | `/api/memory/facts` | 用户端 | `src/api/dialogueMemory.js` | 17 |
| 302 | [ ] | POST | `/api/memory/objections` | 用户端 | `src/api/dialogueMemory.js` | 21 |
| 303 | [ ] | POST | `/api/memory/purchase-intent` | 用户端 | `src/api/dialogueMemory.js` | 25 |
| 304 | [ ] | POST | `/api/memory/intent-trail` | 用户端 | `src/api/dialogueMemory.js` | 29 |
| 305 | [ ] | POST | `/api/memory/sop-history` | 用户端 | `src/api/dialogueMemory.js` | 33 |
| 306 | [x] | GET | `/api/memory/context` | 用户端 | `src/api/dialogueMemory.js` | 37 |
| 307 | [x] | GET | `/api/memory/list` | 用户端 | `src/api/dialogueMemory.js` | 41 |
| 308 | [x] | GET | `/api/dingtalk-app/accounts` | 用户端 | `src/api/dingtalkApp.js` | 6 |
| 309 | [ ] | GET | `/api/dingtalk-app/accounts/{id}` | 用户端 | `src/api/dingtalkApp.js` | 10 |
| 310 | [x] | POST | `/api/dingtalk-app/accounts` | 用户端 | `src/api/dingtalkApp.js` | 14 |
| 311 | [ ] | PUT | `/api/dingtalk-app/accounts/{id}` | 用户端 | `src/api/dingtalkApp.js` | 18 |
| 312 | [ ] | DELETE | `/api/dingtalk-app/accounts/{id}` | 用户端 | `src/api/dingtalkApp.js` | 22 |
| 313 | [ ] | POST | `/api/dingtalk-app/accounts/{id}/test` | 用户端 | `src/api/dingtalkApp.js` | 26 |
| 314 | [x] | GET | `/api/domain-pool/health` | 用户端 | `src/api/domainPool.js` | 3 |
| 315 | [x] | GET | `/api/domain-pool/{domain}/blacklist` | 用户端 | `src/api/domainPool.js` | 4 |
| 316 | [ ] | POST | `/api/domain-pool/{id}/suspend` | 用户端 | `src/api/domainPool.js` | 5 |
| 317 | [ ] | POST | `/api/domain-pool/{id}/rotate` | 用户端 | `src/api/domainPool.js` | 6 |
| 318 | [x] | GET | `/api/domain-pool/alerts` | 用户端 | `src/api/domainPool.js` | 7 |
| 319 | [ ] | POST | `/api/domain-pool/alerts/{id}/resolve` | 用户端 | `src/api/domainPool.js` | 8 |
| 320 | [x] | GET | `/api/domain-pool` | 用户端 | `src/api/domainPool.js` | 11 |
| 321 | [ ] | POST | `/api/domain-pool` | 用户端 | `src/api/domainPool.js` | 12 |
| 322 | [ ] | PUT | `/api/domain-pool/{data_id}` | 用户端 | `src/api/domainPool.js` | 13 |
| 323 | [x] | DELETE | `/api/domain-pool/{id}` | 用户端 | `src/api/domainPool.js` | 14 |
| 324 | [ ] | POST | `/api/domain-pool/{id}/check` | 用户端 | `src/api/domainPool.js` | 15 |
| 325 | [x] | POST | `/api/domain-pool/check-all` | 用户端 | `src/api/domainPool.js` | 16 |
| 326 | [x] | GET | `/api/douyin/list` | 用户端 | `src/api/douyinCard.js` | 4 |
| 327 | [ ] | GET | `/api/douyin/{id}` | 用户端 | `src/api/douyinCard.js` | 8 |
| 328 | [ ] | POST | `/api/douyin/create` | 用户端 | `src/api/douyinCard.js` | 12 |
| 329 | [ ] | PUT | `/api/douyin/update` | 用户端 | `src/api/douyinCard.js` | 16 |
| 330 | [x] | DELETE | `/api/douyin/delete/{id}` | 用户端 | `src/api/douyinCard.js` | 20 |
| 331 | [ ] | GET | `/api/douyin/view/{id}` | 用户端 | `src/api/douyinCard.js` | 24 |
| 332 | [ ] | GET | `/api/douyin/stats/card/{id}` | 用户端 | `src/api/douyinCard.js` | 28 |
| 333 | [x] | GET | `/api/douyin/stats/overall` | 用户端 | `src/api/douyinCard.js` | 32 |
| 334 | [ ] | POST | `/api/douyin/{id}/generate-short-link` | 用户端 | `src/api/douyinCard.js` | 36 |
| 335 | [x] | GET | `/api/email/smtp` | 用户端 | `src/api/email.js` | 5 |
| 336 | [ ] | POST | `/api/email/smtp` | 用户端 | `src/api/email.js` | 9 |
| 337 | [ ] | PUT | `/api/email/smtp/{id}` | 用户端 | `src/api/email.js` | 13 |
| 338 | [x] | DELETE | `/api/email/smtp/{id}` | 用户端 | `src/api/email.js` | 17 |
| 339 | [x] | GET | `/api/email/drafts` | 用户端 | `src/api/email.js` | 21 |
| 340 | [ ] | GET | `/api/email/drafts/{id}` | 用户端 | `src/api/email.js` | 25 |
| 341 | [ ] | POST | `/api/email/drafts` | 用户端 | `src/api/email.js` | 29 |
| 342 | [ ] | PUT | `/api/email/drafts/{id}` | 用户端 | `src/api/email.js` | 33 |
| 343 | [ ] | DELETE | `/api/email/drafts/{id}` | 用户端 | `src/api/email.js` | 37 |
| 344 | [ ] | POST | `/api/upload` | 用户端 | `src/api/email.js` | 41 |
| 345 | [ ] | POST | `/api/email/send` | 用户端 | `src/api/email.js` | 49 |
| 346 | [x] | GET | `/api/email/list?page={page}&limit={limit}` | 用户端 | `src/api/email.js` | 52 |
| 347 | [x] | GET | `/api/email/jobs?page={page}&limit={limit}` | 用户端 | `src/api/email.js` | 56 |
| 348 | [ ] | DELETE | `/api/email/jobs/{id}` | 用户端 | `src/api/email.js` | 59 |
| 349 | [ ] | GET | `/api/email/jobs/{id}` | 用户端 | `src/api/email.js` | 62 |
| 350 | [ ] | GET | `/api/email/lists/{id}/tracking` | 用户端 | `src/api/email.js` | 66 |
| 351 | [ ] | DELETE | `/api/email/list/{id}` | 用户端 | `src/api/email.js` | 70 |
| 352 | [x] | GET | `/api/faqs` | 用户端 | `src/api/faq.js` | 5 |
| 353 | [ ] | GET | `/api/faqs/{id}` | 用户端 | `src/api/faq.js` | 9 |
| 354 | [ ] | POST | `/api/faqs` | 用户端 | `src/api/faq.js` | 13 |
| 355 | [ ] | PUT | `/api/faqs/{id}` | 用户端 | `src/api/faq.js` | 17 |
| 356 | [ ] | PATCH | `/api/faqs/{id}/toggle` | 用户端 | `src/api/faq.js` | 21 |
| 357 | [x] | DELETE | `/api/faqs/{id}` | 用户端 | `src/api/faq.js` | 25 |
| 358 | [ ] | POST | `/api/faqs/match` | 用户端 | `src/api/faq.js` | 29 |
| 359 | [x] | GET | `/api/faqs/stats` | 用户端 | `src/api/faq.js` | 33 |
| 360 | [x] | GET | `/api/feature-flags` | 用户端 | `src/api/featureFlags.js` | 15 |
| 361 | [x] | GET | `/api/feature-flags/stale` | 用户端 | `src/api/featureFlags.js` | 21 |
| 362 | [ ] | GET | `/api/feature-flags/{id}` | 用户端 | `src/api/featureFlags.js` | 25 |
| 363 | [ ] | POST | `/api/feature-flags` | 用户端 | `src/api/featureFlags.js` | 29 |
| 364 | [ ] | PUT | `/api/feature-flags/{id}` | 用户端 | `src/api/featureFlags.js` | 33 |
| 365 | [ ] | DELETE | `/api/feature-flags/{id}` | 用户端 | `src/api/featureFlags.js` | 37 |
| 366 | [ ] | POST | `/api/feature-flags/{id}/enable` | 用户端 | `src/api/featureFlags.js` | 44 |
| 367 | [ ] | POST | `/api/feature-flags/{id}/disable` | 用户端 | `src/api/featureFlags.js` | 48 |
| 368 | [ ] | POST | `/api/feature-flags/{id}/rollout` | 用户端 | `src/api/featureFlags.js` | 54 |
| 369 | [x] | GET | `/api/feature-flags/{id}/audit` | 用户端 | `src/api/featureFlags.js` | 58 |
| 370 | [ ] | GET | `/api/feature-flags/{id}/eval-log` | 用户端 | `src/api/featureFlags.js` | 62 |
| 371 | [x] | GET | `/api/feishu/accounts` | 用户端 | `src/api/feishu.js` | 4 |
| 372 | [ ] | GET | `/api/feishu/accounts/{id}` | 用户端 | `src/api/feishu.js` | 8 |
| 373 | [ ] | POST | `/api/feishu/accounts` | 用户端 | `src/api/feishu.js` | 12 |
| 374 | [ ] | PUT | `/api/feishu/accounts/{id}` | 用户端 | `src/api/feishu.js` | 16 |
| 375 | [ ] | DELETE | `/api/feishu/accounts/{id}` | 用户端 | `src/api/feishu.js` | 20 |
| 376 | [ ] | POST | `/api/feishu/accounts/{id}/test-send` | 用户端 | `src/api/feishu.js` | 24 |
| 377 | [ ] | POST | `/api/feishu/accounts/{id}/refresh-token` | 用户端 | `src/api/feishu.js` | 28 |
| 378 | [~] | GET | `/api/followups/today` | 用户端 | `src/api/followup.js` | 28 |
| 379 | [~] | GET | `/api/followups/pending` | 用户端 | `src/api/followup.js` | 35 |
| 380 | [~] | GET | `/api/followups/overdue` | 用户端 | `src/api/followup.js` | 39 |
| 381 | [~] | POST | `/api/followups/{id}/complete` | 用户端 | `src/api/followup.js` | 46 |
| 382 | [~] | POST | `/api/followups/{id}/cancel` | 用户端 | `src/api/followup.js` | 50 |
| 383 | [~] | POST | `/api/geo/keywords/mine` | 用户端 | `src/api/geo.js` | 5 |
| 384 | [~] | POST | `/api/geo/keywords/expand` | 用户端 | `src/api/geo.js` | 9 |
| 385 | [~] | POST | `/api/geo/keywords/cluster` | 用户端 | `src/api/geo.js` | 13 |
| 386 | [~] | GET | `/api/geo/keywords/list` | 用户端 | `src/api/geo.js` | 17 |
| 387 | [~] | POST | `/api/geo/keyword-mining/crawl-suggest` | 用户端 | `src/api/geo.js` | 22 |
| 388 | [~] | POST | `/api/geo/keyword-mining/longtail` | 用户端 | `src/api/geo.js` | 26 |
| 389 | [~] | GET | `/api/geo/keyword-mining/funnel` | 用户端 | `src/api/geo.js` | 30 |
| 390 | [~] | POST | `/api/geo/push/urls` | 用户端 | `src/api/geo.js` | 35 |
| 391 | [~] | GET | `/api/geo/push/quota` | 用户端 | `src/api/geo.js` | 39 |
| 392 | [~] | GET | `/api/geo/push/sitemap` | 用户端 | `src/api/geo.js` | 43 |
| 393 | [~] | POST | `/api/geo/index-tracking/verify/{articleId}` | 用户端 | `src/api/geo.js` | 48 |
| 394 | [~] | GET | `/api/geo/index-tracking/funnel` | 用户端 | `src/api/geo.js` | 52 |
| 395 | [~] | POST | `/api/geo/index-tracking/verify-all` | 用户端 | `src/api/geo.js` | 56 |
| 396 | [~] | POST | `/api/geo/site/export` | 用户端 | `src/api/geo.js` | 61 |
| 397 | [~] | POST | `/api/geo/site/deploy` | 用户端 | `src/api/geo.js` | 65 |
| 398 | [~] | POST | `/api/geo/site/full-pipeline` | 用户端 | `src/api/geo.js` | 69 |
| 399 | [~] | GET | `/api/geo/site/llms-txt-preview` | 用户端 | `src/api/geo.js` | 73 |
| 400 | [~] | GET | `/api/geo/site/robots-preview` | 用户端 | `src/api/geo.js` | 77 |
| 401 | [~] | GET | `/api/geo/sites` | 用户端 | `src/api/geo.js` | 82 |
| 402 | [~] | POST | `/api/geo/sites` | 用户端 | `src/api/geo.js` | 86 |
| 403 | [~] | PUT | `/api/geo/sites/{id}` | 用户端 | `src/api/geo.js` | 90 |
| 404 | [~] | DELETE | `/api/geo/sites/{id}` | 用户端 | `src/api/geo.js` | 94 |
| 405 | [~] | GET | `/api/geo/pushers` | 用户端 | `src/api/geo.js` | 99 |
| 406 | [~] | POST | `/api/geo/pushers` | 用户端 | `src/api/geo.js` | 103 |
| 407 | [~] | PUT | `/api/geo/pushers/{id}` | 用户端 | `src/api/geo.js` | 107 |
| 408 | [~] | DELETE | `/api/geo/pushers/{id}` | 用户端 | `src/api/geo.js` | 111 |
| 409 | [~] | GET | `/api/geo/schema-templates` | 用户端 | `src/api/geo.js` | 116 |
| 410 | [~] | POST | `/api/geo/schema-templates` | 用户端 | `src/api/geo.js` | 120 |
| 411 | [~] | PUT | `/api/geo/schema-templates/{id}` | 用户端 | `src/api/geo.js` | 124 |
| 412 | [~] | DELETE | `/api/geo/schema-templates/{id}` | 用户端 | `src/api/geo.js` | 128 |
| 413 | [~] | DELETE | `/api/geo/keywords/{id}` | 用户端 | `src/api/geo.js` | 132 |
| 414 | [~] | POST | `/api/geo/content/generate` | 用户端 | `src/api/geo.js` | 136 |
| 415 | [~] | POST | `/api/geo/content/score` | 用户端 | `src/api/geo.js` | 140 |
| 416 | [~] | POST | `/api/geo/content/optimize` | 用户端 | `src/api/geo.js` | 144 |
| 417 | [~] | POST | `/api/geo/content/eeat` | 用户端 | `src/api/geo.js` | 148 |
| 418 | [~] | POST | `/api/geo/content/schema` | 用户端 | `src/api/geo.js` | 152 |
| 419 | [~] | POST | `/api/geo/content/uniqueness` | 用户端 | `src/api/geo.js` | 156 |
| 420 | [~] | GET | `/api/geo/content/list` | 用户端 | `src/api/geo.js` | 160 |
| 421 | [~] | GET | `/api/geo/content/{id}` | 用户端 | `src/api/geo.js` | 164 |
| 422 | [~] | POST | `/api/geo/verification/verify` | 用户端 | `src/api/geo.js` | 168 |
| 423 | [~] | POST | `/api/geo/verification/negative` | 用户端 | `src/api/geo.js` | 172 |
| 424 | [~] | GET | `/api/geo/verification/results/{articleId}` | 用户端 | `src/api/geo.js` | 176 |
| 425 | [~] | GET | `/api/geo/reports/summary` | 用户端 | `src/api/geo.js` | 180 |
| 426 | [~] | GET | `/api/geo/reports/roi` | 用户端 | `src/api/geo.js` | 184 |
| 427 | [~] | GET | `/api/geo/reports/api-costs` | 用户端 | `src/api/geo.js` | 188 |
| 428 | [~] | GET | `/api/geo/config` | 用户端 | `src/api/geo.js` | 192 |
| 429 | [~] | PUT | `/api/geo/config` | 用户端 | `src/api/geo.js` | 196 |
| 430 | [ ] | POST | `/api/geo/config/optimize` | 用户端 | `src/api/geo.js` | 200 |
| 431 | [x] | GET | `/api/geo/alerts` | 用户端 | `src/api/geoAlert.js` | 4 |
| 432 | [x] | GET | `/api/geo/alerts/unread-count` | 用户端 | `src/api/geoAlert.js` | 7 |
| 433 | [ ] | POST | `/api/geo/alerts/{id}/ack` | 用户端 | `src/api/geoAlert.js` | 10 |
| 434 | [ ] | DELETE | `/api/geo/alerts/{id}` | 用户端 | `src/api/geoAlert.js` | 13 |
| 435 | [~] | GET | `/api/geo/decision/report` | 用户端 | `src/api/geoDecision.js` | 3 |
| 436 | [~] | GET | `/api/geo/decision/tasks` | 用户端 | `src/api/geoDecision.js` | 6 |
| 437 | [~] | POST | `/api/geo/decision/tasks/{id}/done` | 用户端 | `src/api/geoDecision.js` | 8 |
| 438 | [~] | GET | `/api/geo/entity/list` | 用户端 | `src/api/geoEntity.js` | 4 |
| 439 | [~] | GET | `/api/geo/entity/{entityId}/graph` | 用户端 | `src/api/geoEntity.js` | 7 |
| 440 | [~] | POST | `/api/geo/entities/extract` | 用户端 | `src/api/geoEntity.js` | 10 |
| 441 | [~] | GET | `/api/geo/kb/documents` | 用户端 | `src/api/geoEntity.js` | 13 |
| 442 | [~] | POST | `/api/geo/kb/documents` | 用户端 | `src/api/geoEntity.js` | 16 |
| 443 | [~] | DELETE | `/api/geo/kb/documents/{id}` | 用户端 | `src/api/geoEntity.js` | 19 |
| 444 | [~] | GET | `/api/geo/kb/documents/{id}` | 用户端 | `src/api/geoEntity.js` | 22 |
| 445 | [~] | GET | `/api/geo/kb/search` | 用户端 | `src/api/geoEntity.js` | 25 |
| 446 | [~] | POST | `/api/geo/kb/ask` | 用户端 | `src/api/geoEntity.js` | 28 |
| 447 | [~] | GET | `/api/geo/workflow/workflows` | 用户端 | `src/api/geoEntity.js` | 31 |
| 448 | [~] | GET | `/api/geo/workflow/workflows/{id}` | 用户端 | `src/api/geoEntity.js` | 34 |
| 449 | [~] | POST | `/api/geo/workflow/workflows` | 用户端 | `src/api/geoEntity.js` | 37 |
| 450 | [~] | PUT | `/api/geo/workflow/workflows/{id}` | 用户端 | `src/api/geoEntity.js` | 40 |
| 451 | [~] | DELETE | `/api/geo/workflow/workflows/{id}` | 用户端 | `src/api/geoEntity.js` | 43 |
| 452 | [~] | POST | `/api/geo/workflow/workflows/{id}/run` | 用户端 | `src/api/geoEntity.js` | 46 |
| 453 | [~] | GET | `/api/geo/workflow/workflows/{workflowId}/executions` | 用户端 | `src/api/geoEntity.js` | 49 |
| 454 | [~] | GET | `/api/geo/workflow/templates` | 用户端 | `src/api/geoEntity.js` | 52 |
| 455 | [~] | POST | `/api/geo/workflow/templates` | 用户端 | `src/api/geoEntity.js` | 55 |
| 456 | [~] | GET | `/api/geo/platform/platforms` | 用户端 | `src/api/geoPlatform.js` | 4 |
| 457 | [~] | GET | `/api/geo/platform/accounts` | 用户端 | `src/api/geoPlatform.js` | 7 |
| 458 | [~] | POST | `/api/geo/platform/accounts` | 用户端 | `src/api/geoPlatform.js` | 10 |
| 459 | [ ] | DELETE | `/api/geo/platform/accounts/{id}` | 用户端 | `src/api/geoPlatform.js` | 13 |
| 460 | [ ] | POST | `/api/geo/platform/publish` | 用户端 | `src/api/geoPlatform.js` | 16 |
| 461 | [~] | GET | `/api/geo/platform/records` | 用户端 | `src/api/geoPlatform.js` | 19 |
| 462 | [~] | GET | `/api/geo/probe/engines` | 用户端 | `src/api/geoProbe.js` | 4 |
| 463 | [~] | POST | `/api/geo/probe/test` | 用户端 | `src/api/geoProbe.js` | 7 |
| 464 | [~] | GET | `/api/geo/probe/runs` | 用户端 | `src/api/geoProbe.js` | 10 |
| 465 | [~] | POST | `/api/geo/probe/run-negative` | 用户端 | `src/api/geoProbe.js` | 13 |
| 466 | [~] | POST | `/api/geo/probe/run-source-sync` | 用户端 | `src/api/geoProbe.js` | 16 |
| 467 | [~] | POST | `/api/geo/probe/all` | 用户端 | `src/api/geoProbe.js` | 19 |
| 468 | [~] | GET | `/api/geo/sov` | 用户端 | `src/api/geoProbe.js` | 22 |
| 469 | [~] | GET | `/api/geo/crawler-stats` | 用户端 | `src/api/geoProbe.js` | 25 |
| 470 | [~] | POST | `/api/geo/crawler/run` | 用户端 | `src/api/geoProbe.js` | 28 |
| 471 | [~] | POST | `/api/geo/inaccurate-claims` | 用户端 | `src/api/geoProbe.js` | 31 |
| 472 | [~] | GET | `/api/geo/source-catalog/levels` | 用户端 | `src/api/geoProbe.js` | 34 |
| 473 | [~] | GET | `/api/geo/competitors` | 用户端 | `src/api/geoProbe.js` | 37 |
| 474 | [~] | GET | `/api/geo/competitors/{id}` | 用户端 | `src/api/geoProbe.js` | 40 |
| 475 | [~] | POST | `/api/geo/competitors` | 用户端 | `src/api/geoProbe.js` | 43 |
| 476 | [~] | PUT | `/api/geo/competitors/{id}` | 用户端 | `src/api/geoProbe.js` | 46 |
| 477 | [~] | DELETE | `/api/geo/competitors/{id}` | 用户端 | `src/api/geoProbe.js` | 49 |
| 478 | [~] | GET | `/api/geo/visibility/engine-compare` | 用户端 | `src/api/geoProbe.js` | 52 |
| 479 | [~] | GET | `/api/geo/sov/trend` | 用户端 | `src/api/geoProbe.js` | 55 |
| 480 | [ ] | POST | `/api/glossaries` | 用户端 | `src/api/glossary.js` | 4 |
| 481 | [x] | GET | `/api/glossaries` | 用户端 | `src/api/glossary.js` | 8 |
| 482 | [ ] | GET | `/api/glossaries/{termId}` | 用户端 | `src/api/glossary.js` | 12 |
| 483 | [ ] | PUT | `/api/glossaries/{termId}` | 用户端 | `src/api/glossary.js` | 16 |
| 484 | [x] | DELETE | `/api/glossaries/{termId}` | 用户端 | `src/api/glossary.js` | 20 |
| 485 | [ ] | POST | `/api/glossaries/validate` | 用户端 | `src/api/glossary.js` | 24 |
| 486 | [x] | GET | `/api/human-tasks` | 用户端 | `src/api/humanTask.js` | 43 |
| 487 | [x] | GET | `/api/human-tasks/counts` | 用户端 | `src/api/humanTask.js` | 49 |
| 488 | [ ] | GET | `/api/human-tasks/{id}` | 用户端 | `src/api/humanTask.js` | 53 |
| 489 | [ ] | POST | `/api/human-tasks/{id}/claim` | 用户端 | `src/api/humanTask.js` | 58 |
| 490 | [ ] | POST | `/api/human-tasks/{id}/release` | 用户端 | `src/api/humanTask.js` | 63 |
| 491 | [ ] | POST | `/api/human-tasks/{id}/complete` | 用户端 | `src/api/humanTask.js` | 68 |
| 492 | [ ] | POST | `/api/human-tasks/{id}/cancel` | 用户端 | `src/api/humanTask.js` | 73 |
| 493 | [x] | GET | `/api/i18n/stats` | 用户端 | `src/api/i18nStats.js` | 4 |
| 494 | [x] | GET | `/api/i18n/stats/lang-dist` | 用户端 | `src/api/i18nStats.js` | 8 |
| 495 | [x] | GET | `/api/i18n/stats/cache` | 用户端 | `src/api/i18nStats.js` | 12 |
| 496 | [x] | GET | `/api/i18n/stats/glossary` | 用户端 | `src/api/i18nStats.js` | 16 |
| 497 | [x] | GET | `/api/i18n/stats/quality` | 用户端 | `src/api/i18nStats.js` | 20 |
| 498 | [x] | GET | `/api/i18n/stats/latency` | 用户端 | `src/api/i18nStats.js` | 24 |
| 499 | [x] | GET | `/api/inbox/conversations` | 用户端 | `src/api/inbox.js` | 7 |
| 500 | [ ] | GET | `/api/inbox/conversations/{id}` | 用户端 | `src/api/inbox.js` | 12 |
| 501 | [ ] | GET | `/api/inbox/conversations/{id}/messages` | 用户端 | `src/api/inbox.js` | 17 |
| 502 | [ ] | POST | `/api/inbox/conversations/{id}/read` | 用户端 | `src/api/inbox.js` | 22 |
| 503 | [ ] | POST | `/api/inbox/conversations/{id}/pin` | 用户端 | `src/api/inbox.js` | 27 |
| 504 | [ ] | POST | `/api/inbox/conversations/{id}/star` | 用户端 | `src/api/inbox.js` | 31 |
| 505 | [ ] | POST | `/api/inbox/conversations/{id}/mute` | 用户端 | `src/api/inbox.js` | 35 |
| 506 | [ ] | POST | `/api/inbox/conversations/{id}/tags` | 用户端 | `src/api/inbox.js` | 40 |
| 507 | [ ] | DELETE | `/api/inbox/conversations/{id}/tags/{tag}` | 用户端 | `src/api/inbox.js` | 44 |
| 508 | [ ] | DELETE | `/api/inbox/conversations/{id}/messages/{mid}?source={source}` | 用户端 | `src/api/inbox.js` | 49 |
| 509 | [ ] | POST | `/api/inbox/assign` | 用户端 | `src/api/inbox.js` | 54 |
| 510 | [ ] | POST | `/api/inbox/assign/auto` | 用户端 | `src/api/inbox.js` | 59 |
| 511 | [x] | GET | `/api/inbox/assignments` | 用户端 | `src/api/inbox.js` | 64 |
| 512 | [x] | GET | `/api/inbox/stats` | 用户端 | `src/api/inbox.js` | 69 |
| 513 | [ ] | POST | `/api/inbox/reconcile?mode={mode}` | 用户端 | `src/api/inbox.js` | 74 |
| 514 | [x] | GET | `/api/inbox/staff/{staff}/load` | 用户端 | `src/api/inbox.js` | 79 |
| 515 | [x] | GET | `/api/inbox/agents` | 用户端 | `src/api/inbox.js` | 84 |
| 516 | [x] | GET | `/api/integrations` | 用户端 | `src/api/integration.js` | 4 |
| 517 | [ ] | GET | `/api/integrations/{id}` | 用户端 | `src/api/integration.js` | 7 |
| 518 | [ ] | POST | `/api/integrations` | 用户端 | `src/api/integration.js` | 10 |
| 519 | [ ] | PUT | `/api/integrations/{id}` | 用户端 | `src/api/integration.js` | 13 |
| 520 | [x] | DELETE | `/api/integrations/{id}` | 用户端 | `src/api/integration.js` | 16 |
| 521 | [ ] | POST | `/api/integrations/{id}/sync-customers` | 用户端 | `src/api/integration.js` | 19 |
| 522 | [ ] | POST | `/api/integrations/{id}/sync-products` | 用户端 | `src/api/integration.js` | 22 |
| 523 | [x] | GET | `/api/integration/sync-logs` | 用户端 | `src/api/integration.js` | 25 |
| 524 | [x] | GET | `/api/integration/external-customers` | 用户端 | `src/api/integration.js` | 28 |
| 525 | [x] | GET | `/api/integration/external-orders` | 用户端 | `src/api/integration.js` | 31 |
| 526 | [ ] | GET | `/api/integration/external-orders-by-customer` | 用户端 | `src/api/integration.js` | 34 |
| 527 | [x] | GET | `/api/integration/external-products` | 用户端 | `src/api/integration.js` | 37 |
| 528 | [ ] | POST | `/api/integrations/{id}/test` | 用户端 | `src/api/integration.js` | 56 |
| 529 | [ ] | POST | `/api/intent/recognize` | 用户端 | `src/api/intentRecognition.js` | 5 |
| 530 | [ ] | POST | `/api/intent/recognize/batch` | 用户端 | `src/api/intentRecognition.js` | 9 |
| 531 | [x] | GET | `/api/intent/stats` | 用户端 | `src/api/intentRecognition.js` | 13 |
| 532 | [x] | GET | `/api/intent/recent` | 用户端 | `src/api/intentRecognition.js` | 17 |
| 533 | [x] | GET | `/api/intent/dict` | 用户端 | `src/api/intentRecognition.js` | 21 |
| 534 | [ ] | POST | `/api/intent/recognize/fine` | 用户端 | `src/api/intentRecognition.js` | 25 |
| 535 | [x] | GET | `/api/intent/logs` | 用户端 | `src/api/intentRecognition.js` | 29 |
| 536 | [x] | GET | `/api/intent/stats/fine` | 用户端 | `src/api/intentRecognition.js` | 33 |
| 537 | [x] | GET | `/api/intent/config` | 用户端 | `src/api/intentRecognition.js` | 37 |
| 538 | [ ] | PUT | `/api/intent/config` | 用户端 | `src/api/intentRecognition.js` | 41 |
| 539 | [x] | GET | `/api/intent-records/keywords-override` | 用户端 | `src/api/intentRecognition.js` | 45 |
| 540 | [x] | PUT | `/api/intent-records/keywords-override` | 用户端 | `src/api/intentRecognition.js` | 49 |
| 541 | [x] | GET | `/api/kb-connectors` | 用户端 | `src/api/kbConnector.js` | 5 |
| 542 | [ ] | POST | `/api/kb-connectors` | 用户端 | `src/api/kbConnector.js` | 6 |
| 543 | [ ] | PUT | `/api/kb-connectors/{id}` | 用户端 | `src/api/kbConnector.js` | 7 |
| 544 | [x] | DELETE | `/api/kb-connectors/{id}` | 用户端 | `src/api/kbConnector.js` | 8 |
| 545 | [ ] | POST | `/api/kb-connectors/{id}/sync` | 用户端 | `src/api/kbConnector.js` | 9 |
| 546 | [x] | GET | `/api/capabilities` | 用户端 | `src/api/kbConnector.js` | 14 |
| 547 | [ ] | POST | `/api/knowledge/import/upload` | 用户端 | `src/api/knowledge.js` | 5 |
| 548 | [ ] | POST | `/api/knowledge/import/text` | 用户端 | `src/api/knowledge.js` | 10 |
| 549 | [ ] | POST | `/api/knowledge/import/url` | 用户端 | `src/api/knowledge.js` | 13 |
| 550 | [x] | GET | `/api/knowledge/documents` | 用户端 | `src/api/knowledge.js` | 17 |
| 551 | [ ] | GET | `/api/knowledge/documents/{id}` | 用户端 | `src/api/knowledge.js` | 20 |
| 552 | [ ] | GET | `/api/knowledge/documents/{id}/progress` | 用户端 | `src/api/knowledge.js` | 23 |
| 553 | [x] | GET | `/api/knowledge/documents/{id}/chunks` | 用户端 | `src/api/knowledge.js` | 26 |
| 554 | [ ] | PUT | `/api/knowledge/documents/{id}` | 用户端 | `src/api/knowledge.js` | 29 |
| 555 | [ ] | DELETE | `/api/knowledge/documents/{id}` | 用户端 | `src/api/knowledge.js` | 32 |
| 556 | [ ] | POST | `/api/knowledge/documents/{id}/reindex` | 用户端 | `src/api/knowledge.js` | 35 |
| 557 | [x] | POST | `/api/knowledge/products/{productId}/rebuild-index` | 用户端 | `src/api/knowledge.js` | 39 |
| 558 | [x] | GET | `/api/knowledge/products/{productId}/overview` | 用户端 | `src/api/knowledge.js` | 42 |
| 559 | [ ] | POST | `/api/knowledge/search` | 用户端 | `src/api/knowledge.js` | 46 |
| 560 | [x] | GET | `/api/knowledge/import-logs` | 用户端 | `src/api/knowledge.js` | 50 |
| 561 | [x] | GET | `/api/knowledge/openapi/sources` | 用户端 | `src/api/knowledge.js` | 54 |
| 562 | [ ] | POST | `/api/knowledge/openapi/sources` | 用户端 | `src/api/knowledge.js` | 57 |
| 563 | [ ] | GET | `/api/knowledge/openapi/sources/{id}` | 用户端 | `src/api/knowledge.js` | 60 |
| 564 | [ ] | PUT | `/api/knowledge/openapi/sources/{id}` | 用户端 | `src/api/knowledge.js` | 63 |
| 565 | [x] | DELETE | `/api/knowledge/openapi/sources/{id}` | 用户端 | `src/api/knowledge.js` | 66 |
| 566 | [ ] | POST | `/api/knowledge/openapi/sources/{id}/sync` | 用户端 | `src/api/knowledge.js` | 69 |
| 567 | [x] | POST | `/api/knowledge/openapi/sources/{id}/test` | 用户端 | `src/api/knowledge.js` | 72 |
| 568 | [ ] | POST | `/api/knowledge/openapi/sources/{id}/toggle` | 用户端 | `src/api/knowledge.js` | 75 |
| 569 | [x] | GET | `/api/knowledge/stats/overview` | 用户端 | `src/api/knowledge.js` | 79 |
| 570 | [x] | GET | `/api/knowledge/stats/documents` | 用户端 | `src/api/knowledge.js` | 82 |
| 571 | [x] | GET | `/api/knowledge/stats/searches` | 用户端 | `src/api/knowledge.js` | 85 |
| 572 | [x] | GET | `/api/knowledge/stats/imports` | 用户端 | `src/api/knowledge.js` | 88 |
| 573 | [x] | GET | `/api/knowledge/stats/openapi` | 用户端 | `src/api/knowledge.js` | 91 |
| 574 | [x] | GET | `/api/knowledge-bases` | 用户端 | `src/api/knowledgeBase.js` | 4 |
| 575 | [ ] | GET | `/api/knowledge-bases/by-type/{kbType}` | 用户端 | `src/api/knowledgeBase.js` | 8 |
| 576 | [ ] | GET | `/api/knowledge-bases/{id}` | 用户端 | `src/api/knowledgeBase.js` | 12 |
| 577 | [ ] | POST | `/api/knowledge-bases` | 用户端 | `src/api/knowledgeBase.js` | 16 |
| 578 | [ ] | PUT | `/api/knowledge-bases/{id}` | 用户端 | `src/api/knowledgeBase.js` | 20 |
| 579 | [x] | DELETE | `/api/knowledge-bases/{id}` | 用户端 | `src/api/knowledgeBase.js` | 24 |
| 580 | [ ] | GET | `/api/knowledge-bases/{id}/stats` | 用户端 | `src/api/knowledgeBase.js` | 28 |
| 581 | [ ] | POST | `/api/knowledge-merchant/batch/import` | 用户端 | `src/api/knowledgeMerchant.js` | 5 |
| 582 | [ ] | POST | `/api/knowledge-merchant/batch/upload` | 用户端 | `src/api/knowledgeMerchant.js` | 8 |
| 583 | [ ] | POST | `/api/knowledge-merchant/playground` | 用户端 | `src/api/knowledgeMerchant.js` | 14 |
| 584 | [x] | GET | `/api/knowledge-merchant/documents/{documentId}/chunks` | 用户端 | `src/api/knowledgeMerchant.js` | 18 |
| 585 | [ ] | PUT | `/api/knowledge-merchant/chunks/{chunkId}` | 用户端 | `src/api/knowledgeMerchant.js` | 21 |
| 586 | [x] | DELETE | `/api/knowledge-merchant/chunks/{chunkId}` | 用户端 | `src/api/knowledgeMerchant.js` | 24 |
| 587 | [ ] | POST | `/api/knowledge-merchant/chunks/{chunkId}/split` | 用户端 | `src/api/knowledgeMerchant.js` | 27 |
| 588 | [ ] | POST | `/api/knowledge-merchant/feedback` | 用户端 | `src/api/knowledgeMerchant.js` | 31 |
| 589 | [x] | GET | `/api/knowledge-merchant/feedbacks` | 用户端 | `src/api/knowledgeMerchant.js` | 34 |
| 590 | [ ] | POST | `/api/knowledge-merchant/tokens` | 用户端 | `src/api/knowledgeMerchant.js` | 38 |
| 591 | [x] | GET | `/api/knowledge-merchant/tokens` | 用户端 | `src/api/knowledgeMerchant.js` | 41 |
| 592 | [x] | POST | `/api/knowledge-merchant/tokens/{tokenId}/revoke` | 用户端 | `src/api/knowledgeMerchant.js` | 44 |
| 593 | [ ] | POST | `/api/knowledge-merchant/external/import` | 用户端 | `src/api/knowledgeMerchant.js` | 48 |
| 594 | [x] | GET | `/api/knowledge-merchant/external/jobs` | 用户端 | `src/api/knowledgeMerchant.js` | 53 |
| 595 | [x] | GET | `/api/kuaishou/list` | 用户端 | `src/api/kuaishouCard.js` | 4 |
| 596 | [ ] | GET | `/api/kuaishou/{id}` | 用户端 | `src/api/kuaishouCard.js` | 8 |
| 597 | [ ] | POST | `/api/kuaishou/create` | 用户端 | `src/api/kuaishouCard.js` | 12 |
| 598 | [ ] | PUT | `/api/kuaishou/update` | 用户端 | `src/api/kuaishouCard.js` | 16 |
| 599 | [ ] | DELETE | `/api/kuaishou/delete/{id}` | 用户端 | `src/api/kuaishouCard.js` | 20 |
| 600 | [ ] | GET | `/api/kuaishou/view/{id}` | 用户端 | `src/api/kuaishouCard.js` | 24 |
| 601 | [ ] | POST | `/api/kuaishou/like/{id}` | 用户端 | `src/api/kuaishouCard.js` | 28 |
| 602 | [ ] | POST | `/api/kuaishou/share/{id}` | 用户端 | `src/api/kuaishouCard.js` | 32 |
| 603 | [ ] | GET | `/api/kuaishou/stats/card/{id}` | 用户端 | `src/api/kuaishouCard.js` | 36 |
| 604 | [x] | GET | `/api/kuaishou/stats/overall` | 用户端 | `src/api/kuaishouCard.js` | 40 |
| 605 | [ ] | POST | `/api/kuaishou/{id}/generate-short-link` | 用户端 | `src/api/kuaishouCard.js` | 44 |
| 606 | [x] | GET | `/api/lead-mining/config` | 用户端 | `src/api/leadMining.js` | 4 |
| 607 | [ ] | POST | `/api/lead-mining/config` | 用户端 | `src/api/leadMining.js` | 8 |
| 608 | [x] | GET | `/api/live-codes/list` | 用户端 | `src/api/livecode.js` | 4 |
| 609 | [ ] | GET | `/api/live-codes/{id}` | 用户端 | `src/api/livecode.js` | 8 |
| 610 | [ ] | POST | `/api/live-codes/create` | 用户端 | `src/api/livecode.js` | 12 |
| 611 | [ ] | PUT | `/api/live-codes/{id}/update` | 用户端 | `src/api/livecode.js` | 16 |
| 612 | [ ] | DELETE | `/api/live-codes/{id}/delete` | 用户端 | `src/api/livecode.js` | 20 |
| 613 | [ ] | GET | `/api/live-codes/{id}/stats` | 用户端 | `src/api/livecode.js` | 24 |
| 614 | [ ] | GET | `/api/live-codes/{liveCodeId}/qrcodes` | 用户端 | `src/api/livecode.js` | 28 |
| 615 | [ ] | POST | `/api/live-codes/{liveCodeId}/qrcodes/create` | 用户端 | `src/api/livecode.js` | 32 |
| 616 | [ ] | GET | `/api/live-codes/qrcodes/{qrId}/stats` | 用户端 | `src/api/livecode.js` | 36 |
| 617 | [ ] | POST | `/api/live-codes/{id}/share` | 用户端 | `src/api/livecode.js` | 40 |
| 618 | [ ] | DELETE | `/api/live-codes/qrcodes/{id}/delete` | 用户端 | `src/api/livecode.js` | 44 |
| 619 | [ ] | PUT | `/api/live-codes/qrcodes/{id}/update` | 用户端 | `src/api/livecode.js` | 48 |
| 620 | [x] | GET | `/api/llm/models` | 用户端 | `src/api/llmRouting.js` | 5 |
| 621 | [x] | GET | `/api/llm/models/{name}` | 用户端 | `src/api/llmRouting.js` | 9 |
| 622 | [~] | POST | `/api/llm/models` | 用户端 | `src/api/llmRouting.js` | 13 |
| 623 | [~] | PUT | `/api/llm/models/{name}` | 用户端 | `src/api/llmRouting.js` | 17 |
| 624 | [~] | DELETE | `/api/llm/models/{name}` | 用户端 | `src/api/llmRouting.js` | 21 |
| 625 | [~] | PUT | `/api/llm/models/{name}` | 用户端 | `src/api/llmRouting.js` | 25 |
| 626 | [~] | POST | `/api/llm/models/{name}/test` | 用户端 | `src/api/llmRouting.js` | 29 |
| 627 | [x] | GET | `/api/llm/scene-routing` | 用户端 | `src/api/llmRouting.js` | 33 |
| 628 | [x] | GET | `/api/llm/scenarios` | 用户端 | `src/api/llmRouting.js` | 37 |
| 629 | [~] | PUT | `/api/llm/scene-routing` | 用户端 | `src/api/llmRouting.js` | 42 |
| 630 | [x] | GET | `/api/llm/fallback` | 用户端 | `src/api/llmRouting.js` | 46 |
| 631 | [x] | GET | `/api/llm/audit` | 用户端 | `src/api/llmRouting.js` | 52 |
| 632 | [x] | GET | `/api/llm/stats` | 用户端 | `src/api/llmRouting.js` | 56 |
| 633 | [x] | GET | `/api/llm/usage` | 用户端 | `src/api/llmRouting.js` | 60 |
| 634 | [x] | GET | `/api/llm/cost-stats` | 用户端 | `src/api/llmRouting.js` | 64 |
| 635 | [x] | GET | `/api/llm/health` | 用户端 | `src/api/llmRouting.js` | 68 |
| 636 | [x] | GET | `/api/llm/model-type-stats` | 用户端 | `src/api/llmRouting.js` | 72 |
| 637 | [x] | GET | `/api/llm/egress-audit` | 用户端 | `src/api/llmRouting.js` | 76 |
| 638 | [x] | GET | `/api/llm/egress-alerts` | 用户端 | `src/api/llmRouting.js` | 80 |
| 639 | [x] | GET | `/api/manage/ltc/config` | 管理端 | `src/api/ltcConfig.js` | 11 |
| 640 | [ ] | PUT | `/api/manage/ltc/config` | 管理端 | `src/api/ltcConfig.js` | 21 |
| 641 | [x] | GET | `/api/ltc-rates` | 用户端 | `src/api/ltcRates.js` | 5 |
| 642 | [x] | GET | `/api/marketing-flows` | 用户端 | `src/api/marketingFlow.js` | 4 |
| 643 | [ ] | GET | `/api/marketing-flows/{id}` | 用户端 | `src/api/marketingFlow.js` | 7 |
| 644 | [ ] | POST | `/api/marketing-flows` | 用户端 | `src/api/marketingFlow.js` | 10 |
| 645 | [ ] | PUT | `/api/marketing-flows/{id}` | 用户端 | `src/api/marketingFlow.js` | 13 |
| 646 | [ ] | DELETE | `/api/marketing-flows/{id}` | 用户端 | `src/api/marketingFlow.js` | 16 |
| 647 | [ ] | POST | `/api/marketing-flows/{id}/activate` | 用户端 | `src/api/marketingFlow.js` | 19 |
| 648 | [ ] | POST | `/api/marketing-flows/{id}/pause` | 用户端 | `src/api/marketingFlow.js` | 22 |
| 649 | [ ] | POST | `/api/marketing-flows/{id}/stop` | 用户端 | `src/api/marketingFlow.js` | 25 |
| 650 | [ ] | GET | `/api/marketing-flows/{id}/executions` | 用户端 | `src/api/marketingFlow.js` | 28 |
| 651 | [ ] | GET | `/api/marketing-flows/{id}/stats` | 用户端 | `src/api/marketingFlow.js` | 31 |
| 652 | [x] | GET | `/api/material/list` | 用户端 | `src/api/material.js` | 4 |
| 653 | [x] | UPLOAD | `/api/material/upload` | 用户端 | `src/api/material.js` | 8 |
| 654 | [ ] | DELETE | `/api/material/{id}` | 用户端 | `src/api/material.js` | 12 |
| 655 | [x] | GET | `/api/material/categories` | 用户端 | `src/api/material.js` | 16 |
| 656 | [ ] | POST | `/api/material/categories` | 用户端 | `src/api/material.js` | 20 |
| 657 | [ ] | PUT | `/api/material/categories/{id}` | 用户端 | `src/api/material.js` | 24 |
| 658 | [ ] | DELETE | `/api/material/categories/{id}` | 用户端 | `src/api/material.js` | 28 |
| 659 | [x] | GET | `/api/material/selector` | 用户端 | `src/api/material.js` | 32 |
| 660 | [ ] | POST | `/api/material/{id}/usage` | 用户端 | `src/api/material.js` | 36 |
| 661 | [x] | GET | `/api/material/stats` | 用户端 | `src/api/material.js` | 40 |
| 662 | [x] | GET | `/api/mcp/credentials` | 用户端 | `src/api/mcpCredential.js` | 5 |
| 663 | [x] | POST | `/api/mcp/credentials` | 用户端 | `src/api/mcpCredential.js` | 6 |
| 664 | [ ] | PUT | `/api/mcp/credentials/{id}` | 用户端 | `src/api/mcpCredential.js` | 7 |
| 665 | [x] | DELETE | `/api/mcp/credentials/{id}` | 用户端 | `src/api/mcpCredential.js` | 8 |
| 666 | [x] | GET | `/api/menus` | 用户端 | `src/api/menu.js` | 12 |
| 667 | [x] | GET | `/api/system/menus` | 用户端 | `src/api/menu.js` | 17 |
| 668 | [ ] | GET | `/api/system/roles/{code}/menus` | 用户端 | `src/api/menu.js` | 22 |
| 669 | [ ] | PUT | `/api/system/roles/{code}/menus` | 用户端 | `src/api/menu.js` | 27 |
| 670 | [ ] | POST | `/api/message-hub/push` | 用户端 | `src/api/messageHub.js` | 5 |
| 671 | [ ] | POST | `/api/message-hub/push-batch` | 用户端 | `src/api/messageHub.js` | 9 |
| 672 | [ ] | POST | `/api/message-hub/push-from-channel` | 用户端 | `src/api/messageHub.js` | 13 |
| 673 | [x] | GET | `/api/message-hub/list` | 用户端 | `src/api/messageHub.js` | 17 |
| 674 | [x] | GET | `/api/message-hub/{id}` | 用户端 | `src/api/messageHub.js` | 21 |
| 675 | [ ] | POST | `/api/message-hub/{ids[0]}/read` | 用户端 | `src/api/messageHub.js` | 25 |
| 676 | [x] | GET | `/api/message-hub/stats` | 用户端 | `src/api/messageHub.js` | 29 |
| 677 | [x] | GET | `/api/message-hub/platforms` | 用户端 | `src/api/messageHub.js` | 33 |
| 678 | [x] | GET | `/api/module-market/list` | 公开 | `src/api/moduleMarket.js` | 17 |
| 679 | [ ] | GET | `/api/module-market/detail/{key}` | 公开 | `src/api/moduleMarket.js` | 20 |
| 680 | [x] | GET | `/api/module-market/payment-guide` | 公开 | `src/api/moduleMarket.js` | 23 |
| 681 | [x] | GET | `/api/tenant/regulatory-notice` | 公开 | `src/api/moduleMarket.js` | 26 |
| 682 | [ ] | POST | `/api/module-market/order` | 公开 | `src/api/moduleMarket.js` | 31 |
| 683 | [x] | GET | `/api/module-market/orders` | 公开 | `src/api/moduleMarket.js` | 36 |
| 684 | [ ] | GET | `/api/module-market/orders/{orderNo}` | 公开 | `src/api/moduleMarket.js` | 39 |
| 685 | [ ] | GET | `/api/module-market/orders/{orderNo}/pay-status` | 公开 | `src/api/moduleMarket.js` | 43 |
| 686 | [ ] | POST | `/api/module-market/orders/{orderNo}/pay` | 公开 | `src/api/moduleMarket.js` | 48 |
| 687 | [x] | GET | `/api/subscription/status` | 用户端 | `src/api/moduleMarket.js` | 56 |
| 688 | [ ] | POST | `/api/subscription/renew` | 用户端 | `src/api/moduleMarket.js` | 59 |
| 689 | [x] | GET | `/api/modules/my` | 用户端 | `src/api/moduleMarket.js` | 64 |
| 690 | [x] | POST | `/api/modules/sync` | 用户端 | `src/api/moduleMarket.js` | 67 |
| 691 | [x] | GET | `/api/modules/sync-logs` | 用户端 | `src/api/moduleMarket.js` | 70 |
| 692 | [x] | GET | `/api/llm/wallet` | 用户端 | `src/api/moduleMarket.js` | 78 |
| 693 | [x] | GET | `/api/llm/plans` | 用户端 | `src/api/moduleMarket.js` | 81 |
| 694 | [ ] | POST | `/api/llm/recharge` | 用户端 | `src/api/moduleMarket.js` | 84 |
| 695 | [x] | GET | `/api/monitor/trace-tree` | 用户端 | `src/api/monitor.js` | 5 |
| 696 | [x] | GET | `/api/monitor/traces` | 用户端 | `src/api/monitor.js` | 8 |
| 697 | [ ] | GET | `/api/monitor/lifecycle` | 用户端 | `src/api/monitor.js` | 11 |
| 698 | [x] | GET | `/api/monitor/health` | 用户端 | `src/api/monitor.js` | 14 |
| 699 | [x] | GET | `/api/monitor/node-health` | 用户端 | `src/api/monitor.js` | 17 |
| 700 | [x] | GET | `/api/monitor/latency` | 用户端 | `src/api/monitor.js` | 20 |
| 701 | [x] | GET | `/api/monitor/anomalies` | 用户端 | `src/api/monitor.js` | 23 |
| 702 | [x] | GET | `/api/monitor/trace-eval/logs` | 用户端 | `src/api/monitor.js` | 26 |
| 703 | [x] | GET | `/api/monitor/knowledge-weights` | 用户端 | `src/api/monitor.js` | 29 |
| 704 | [x] | POST | `/api/monitor/trace-eval/trigger` | 用户端 | `src/api/monitor.js` | 32 |
| 705 | [x] | GET | `/api/rag/recall/snapshot` | 用户端 | `src/api/monitor.js` | 36 |
| 706 | [x] | GET | `/api/rag/recall/snapshots` | 用户端 | `src/api/monitor.js` | 39 |
| 707 | [x] | POST | `/api/rag/recall/collect` | 用户端 | `src/api/monitor.js` | 42 |
| 708 | [x] | POST | `/api/rag/recall/start` | 用户端 | `src/api/monitor.js` | 45 |
| 709 | [x] | POST | `/api/rag/recall/stop` | 用户端 | `src/api/monitor.js` | 48 |
| 710 | [ ] | GET | `/api/rag/recall/metrics` | 用户端 | `src/api/monitor.js` | 51 |
| 711 | [x] | GET | `/api/rag/recall/low-recall` | 用户端 | `src/api/monitor.js` | 54 |
| 712 | [x] | POST | `/api/objection/handle` | 用户端 | `src/api/objection.js` | 4 |
| 713 | [x] | POST | `/api/objection/classify` | 用户端 | `src/api/objection.js` | 8 |
| 714 | [x] | GET | `/api/objection/categories` | 用户端 | `src/api/objection.js` | 12 |
| 715 | [x] | POST | `/api/objection/usage` | 用户端 | `src/api/objection.js` | 16 |
| 716 | [x] | GET | `/api/obs/config` | 用户端 | `src/api/obs.js` | 4 |
| 717 | [ ] | GET | `/api/obs/config/{id}` | 用户端 | `src/api/obs.js` | 8 |
| 718 | [ ] | POST | `/api/obs/config` | 用户端 | `src/api/obs.js` | 12 |
| 719 | [ ] | PUT | `/api/obs/config/{id}` | 用户端 | `src/api/obs.js` | 16 |
| 720 | [ ] | DELETE | `/api/obs/config/{id}` | 用户端 | `src/api/obs.js` | 20 |
| 721 | [ ] | POST | `/api/obs/config/{id}/test` | 用户端 | `src/api/obs.js` | 24 |
| 722 | [ ] | POST | `/api/obs/config/{id}/default` | 用户端 | `src/api/obs.js` | 28 |
| 723 | [x] | GET | `/api/obs/config/default` | 用户端 | `src/api/obs.js` | 32 |
| 724 | [x] | GET | `/api/customer/oneid/list` | 用户端 | `src/api/oneid.js` | 4 |
| 725 | [x] | GET | `/api/customer/oneid/conflicts` | 用户端 | `src/api/oneid.js` | 8 |
| 726 | [ ] | POST | `/api/customer/oneid/merge` | 用户端 | `src/api/oneid.js` | 12 |
| 727 | [ ] | POST | `/api/customer/oneid/conflicts/{id}/resolve` | 用户端 | `src/api/oneid.js` | 16 |
| 728 | [ ] | GET | `/api/customer-oneid/{customerId}/identities` | 用户端 | `src/api/oneid.js` | 20 |
| 729 | [ ] | POST | `/api/customer-oneid/{customerId}/identities` | 用户端 | `src/api/oneid.js` | 24 |
| 730 | [x] | POST | `/api/customer/oneid/resolve` | 用户端 | `src/api/oneid.js` | 28 |
| 731 | [x] | GET | `/api/customer/oneid/stats` | 用户端 | `src/api/oneid.js` | 32 |
| 732 | [x] | GET | `/api/oneid/merge-rules` | 用户端 | `src/api/oneid.js` | 36 |
| 733 | [ ] | POST | `/api/oneid/merge-rules` | 用户端 | `src/api/oneid.js` | 40 |
| 734 | [x] | GET | `/api/operation-logs` | 用户端 | `src/api/operationLog.js` | 4 |
| 735 | [x] | GET | `/api/operation-logs/{id}` | 用户端 | `src/api/operationLog.js` | 7 |
| 736 | [x] | GET | `/api/operation-logs/statistics` | 用户端 | `src/api/operationLog.js` | 10 |
| 737 | [ ] | GET | `/api/operation-logs/export` | 用户端 | `src/api/operationLog.js` | 13 |
| 738 | [x] | POST | `/api/operation-logs/clean` | 用户端 | `src/api/operationLog.js` | 16 |
| 739 | [~] | GET | `/api/manage/order-drafts` | 管理端 | `src/api/orderDraft.js` | 32 |
| 740 | [~] | GET | `/api/manage/order-drafts/{id}` | 管理端 | `src/api/orderDraft.js` | 36 |
| 741 | [~] | PATCH | `/api/manage/order-drafts/{id}` | 管理端 | `src/api/orderDraft.js` | 43 |
| 742 | [~] | POST | `/api/manage/order-drafts/{id}/confirm` | 管理端 | `src/api/orderDraft.js` | 50 |
| 743 | [~] | POST | `/api/manage/order-drafts/{id}/cancel` | 管理端 | `src/api/orderDraft.js` | 55 |
| 744 | [x] | GET | `/api/agent/order-drafts/stats` | 用户端 | `src/api/orderDraft.js` | 62 |
| 745 | [ ] | PUT | `/api/system/permissions/{id}/enabled` | 用户端 | `src/api/permission.js` | 4 |
| 746 | [ ] | PUT | `/api/system/permissions/{id}/password` | 用户端 | `src/api/permission.js` | 8 |
| 747 | [x] | GET | `/api/system/permissions/audit-logs` | 用户端 | `src/api/permission.js` | 12 |
| 748 | [x] | GET | `/api/analytics/persona/staffs` | 用户端 | `src/api/persona.js` | 4 |
| 749 | [x] | GET | `/api/analytics/persona/staffs/{staffId}` | 用户端 | `src/api/persona.js` | 8 |
| 750 | [x] | GET | `/api/platform/message/latest` | 管理端 | `src/api/platform.js` | 5 |
| 751 | [x] | POST | `/api/platform/message/{messageId}/read` | 管理端 | `src/api/platform.js` | 9 |
| 752 | [x] | GET | `/api/platform-accounts` | 用户端 | `src/api/platformAccount.js` | 5 |
| 753 | [x] | GET | `/api/platform-accounts/platforms` | 用户端 | `src/api/platformAccount.js` | 9 |
| 754 | [ ] | GET | `/api/platform-accounts/{id}` | 用户端 | `src/api/platformAccount.js` | 13 |
| 755 | [ ] | POST | `/api/platform-accounts` | 用户端 | `src/api/platformAccount.js` | 17 |
| 756 | [ ] | PUT | `/api/platform-accounts/{id}` | 用户端 | `src/api/platformAccount.js` | 21 |
| 757 | [x] | DELETE | `/api/platform-accounts/{id}` | 用户端 | `src/api/platformAccount.js` | 25 |
| 758 | [ ] | POST | `/api/platform-accounts/{id}/login` | 用户端 | `src/api/platformAccount.js` | 29 |
| 759 | [ ] | GET | `/api/platform-accounts/{id}/status` | 用户端 | `src/api/platformAccount.js` | 33 |
| 760 | [x] | GET | `/api/qq/accounts` | 用户端 | `src/api/qqBot.js` | 4 |
| 761 | [ ] | GET | `/api/qq/accounts/{id}` | 用户端 | `src/api/qqBot.js` | 8 |
| 762 | [ ] | POST | `/api/qq/accounts` | 用户端 | `src/api/qqBot.js` | 12 |
| 763 | [ ] | PUT | `/api/qq/accounts/{id}` | 用户端 | `src/api/qqBot.js` | 16 |
| 764 | [ ] | DELETE | `/api/qq/accounts/{id}` | 用户端 | `src/api/qqBot.js` | 20 |
| 765 | [ ] | POST | `/api/qq/accounts/{id}/test-send` | 用户端 | `src/api/qqBot.js` | 24 |
| 766 | [ ] | POST | `/api/qq/accounts/{id}/verify-callback` | 用户端 | `src/api/qqBot.js` | 28 |
| 767 | [ ] | POST | `/api/rag-config/products` | 用户端 | `src/api/ragProductConfig.js` | 12 |
| 768 | [x] | GET | `/api/rag-config/products` | 用户端 | `src/api/ragProductConfig.js` | 21 |
| 769 | [x] | GET | `/api/rag-config/products` | 用户端 | `src/api/ragProductConfig.js` | 25 |
| 770 | [ ] | PUT | `/api/rag-config/products/{id}` | 用户端 | `src/api/ragProductConfig.js` | 29 |
| 771 | [x] | DELETE | `/api/rag-config/products/{id}` | 用户端 | `src/api/ragProductConfig.js` | 33 |
| 772 | [x] | GET | `/api/rag-config/accounts/config` | 用户端 | `src/api/ragProductConfig.js` | 37 |
| 773 | [ ] | PUT | `/api/rag-config/accounts/config` | 用户端 | `src/api/ragProductConfig.js` | 41 |
| 774 | [ ] | POST | `/api/rag-config/process-message` | 用户端 | `src/api/ragProductConfig.js` | 45 |
| 775 | [x] | GET | `/api/reach/pipelines` | 用户端 | `src/api/reachPipeline.js` | 5 |
| 776 | [ ] | POST | `/api/reach/pipelines` | 用户端 | `src/api/reachPipeline.js` | 8 |
| 777 | [ ] | GET | `/api/reach/pipelines/{id}` | 用户端 | `src/api/reachPipeline.js` | 11 |
| 778 | [ ] | PUT | `/api/reach/pipelines/{id}` | 用户端 | `src/api/reachPipeline.js` | 14 |
| 779 | [ ] | DELETE | `/api/reach/pipelines/{id}` | 用户端 | `src/api/reachPipeline.js` | 17 |
| 780 | [x] | POST | `/api/reach/pipelines/{id}/pause` | 用户端 | `src/api/reachPipeline.js` | 20 |
| 781 | [x] | POST | `/api/reach/pipelines/{id}/resume` | 用户端 | `src/api/reachPipeline.js` | 23 |
| 782 | [x] | POST | `/api/reach/pipelines/{id}/archive` | 用户端 | `src/api/reachPipeline.js` | 26 |
| 783 | [x] | GET | `/api/reach/jobs` | 用户端 | `src/api/reachPipeline.js` | 30 |
| 784 | [ ] | POST | `/api/reach/jobs` | 用户端 | `src/api/reachPipeline.js` | 33 |
| 785 | [ ] | GET | `/api/reach/jobs/{id}` | 用户端 | `src/api/reachPipeline.js` | 36 |
| 786 | [ ] | POST | `/api/reach/jobs/{id}/cancel` | 用户端 | `src/api/reachPipeline.js` | 39 |
| 787 | [ ] | POST | `/api/reach/jobs/{id}/retry` | 用户端 | `src/api/reachPipeline.js` | 42 |
| 788 | [ ] | POST | `/api/reach/jobs/{id}/execute` | 用户端 | `src/api/reachPipeline.js` | 45 |
| 789 | [x] | GET | `/api/reach/stats` | 用户端 | `src/api/reachPipeline.js` | 49 |
| 790 | [ ] | POST | `/api/reach/rate-limit/reset` | 用户端 | `src/api/reachPipeline.js` | 52 |
| 791 | [x] | GET | `/api/system/roles` | 用户端 | `src/api/role.js` | 10 |
| 792 | [ ] | GET | `/api/system/roles/{code}` | 用户端 | `src/api/role.js` | 15 |
| 793 | [ ] | GET | `/api/system/roles/{code}/members` | 用户端 | `src/api/role.js` | 22 |
| 794 | [x] | GET | `/api/config/runtime` | 用户端 | `src/api/runtimeConfig.js` | 13 |
| 795 | [~] | GET | `/api/config/dictionaries/{parent}` | 用户端 | `src/api/runtimeConfig.js` | 23 |
| 796 | [x] | GET | `/api/manage/dictionaries` | 管理端 | `src/api/runtimeConfig.js` | 28 |
| 797 | [~] | GET | `/api/manage/dictionaries/{parent}` | 管理端 | `src/api/runtimeConfig.js` | 29 |
| 798 | [~] | PUT | `/api/manage/dictionaries/{parent}/items` | 管理端 | `src/api/runtimeConfig.js` | 30 |
| 799 | [x] | PUT | `/api/manage/dictionaries/{parent}/transitions` | 管理端 | `src/api/runtimeConfig.js` | 32 |
| 800 | [ ] | GET | `/api/sales-workbench/overview` | 用户端 | `src/api/salesWorkbench.js` | 24 |
| 801 | [x] | GET | `/api/sales-workbench/team-dashboard` | 用户端 | `src/api/salesWorkbench.js` | 30 |
| 802 | [x] | GET | `/api/sales-workbench/champion` | 用户端 | `src/api/salesWorkbench.js` | 37 |
| 803 | [x] | GET | `/api/sales-workbench/quick-actions` | 用户端 | `src/api/salesWorkbench.js` | 43 |
| 804 | [x] | GET | `/api/script-library/{scriptId}/versions` | 用户端 | `src/api/scriptAb.js` | 4 |
| 805 | [x] | POST | `/api/script-library/{scriptId}/versions` | 用户端 | `src/api/scriptAb.js` | 5 |
| 806 | [x] | PUT | `/api/script-library/{scriptId}/versions/{versionId}/activate` | 用户端 | `src/api/scriptAb.js` | 7 |
| 807 | [x] | POST | `/api/script-library/{scriptId}/expire` | 用户端 | `src/api/scriptAb.js` | 8 |
| 808 | [x] | GET | `/api/script-library/{scriptId}/ab-stats` | 用户端 | `src/api/scriptAb.js` | 9 |
| 809 | [ ] | PUT | `/api/script-library/{scriptId}/ab-config` | 用户端 | `src/api/scriptAb.js` | 10 |
| 810 | [ ] | POST | `/api/script-ab/conversion` | 用户端 | `src/api/scriptAb.js` | 11 |
| 811 | [x] | GET | `/api/scripts` | 用户端 | `src/api/scriptTemplate.js` | 4 |
| 812 | [ ] | GET | `/api/scripts/{id}` | 用户端 | `src/api/scriptTemplate.js` | 7 |
| 813 | [ ] | POST | `/api/scripts` | 用户端 | `src/api/scriptTemplate.js` | 10 |
| 814 | [ ] | PUT | `/api/scripts/{id}` | 用户端 | `src/api/scriptTemplate.js` | 13 |
| 815 | [ ] | DELETE | `/api/scripts/{id}` | 用户端 | `src/api/scriptTemplate.js` | 16 |
| 816 | [x] | GET | `/api/scripts/categories` | 用户端 | `src/api/scriptTemplate.js` | 19 |
| 817 | [x] | GET | `/api/scripts/search` | 用户端 | `src/api/scriptTemplate.js` | 22 |
| 818 | [x] | GET | `/api/scripts/public` | 用户端 | `src/api/scriptTemplate.js` | 25 |
| 819 | [ ] | POST | `/api/scripts/recommend` | 用户端 | `src/api/scriptTemplate.js` | 28 |
| 820 | [x] | POST | `/api/security/audit` | 用户端 | `src/api/securityAudit.js` | 4 |
| 821 | [x] | GET | `/api/security/audit/list` | 用户端 | `src/api/securityAudit.js` | 8 |
| 822 | [x] | GET | `/api/security/audit/{id}` | 用户端 | `src/api/securityAudit.js` | 12 |
| 823 | [x] | GET | `/api/shortlink/list` | 用户端 | `src/api/shortLink.js` | 5 |
| 824 | [ ] | GET | `/api/shortlink/{id}` | 用户端 | `src/api/shortLink.js` | 9 |
| 825 | [ ] | POST | `/api/shortlink/create` | 用户端 | `src/api/shortLink.js` | 13 |
| 826 | [ ] | PUT | `/api/shortlink/update` | 用户端 | `src/api/shortLink.js` | 17 |
| 827 | [ ] | DELETE | `/api/shortlink/delete/{id}` | 用户端 | `src/api/shortLink.js` | 21 |
| 828 | [ ] | POST | `/api/shortlink/access` | 用户端 | `src/api/shortLink.js` | 25 |
| 829 | [ ] | POST | `/api/shortlink/generate` | 用户端 | `src/api/shortLink.js` | 29 |
| 830 | [ ] | GET | `/api/shortlink/{id}/stats` | 用户端 | `src/api/shortLink.js` | 33 |
| 831 | [x] | GET | `/api/shortlink/stats/all` | 用户端 | `src/api/shortLink.js` | 37 |
| 832 | [ ] | POST | `/api/shortlink/{id}/share` | 用户端 | `src/api/shortLink.js` | 41 |
| 833 | [x] | GET | `/api/sms/config` | 用户端 | `src/api/sms.js` | 5 |
| 834 | [ ] | POST | `/api/sms/config` | 用户端 | `src/api/sms.js` | 8 |
| 835 | [x] | GET | `/api/sms/list` | 用户端 | `src/api/sms.js` | 12 |
| 836 | [x] | GET | `/api/sms/detail/{id}` | 用户端 | `src/api/sms.js` | 15 |
| 837 | [ ] | POST | `/api/sms/resend/{id}` | 用户端 | `src/api/sms.js` | 18 |
| 838 | [ ] | POST | `/api/sms/send` | 用户端 | `src/api/sms.js` | 21 |
| 839 | [x] | GET | `/api/sms/draft/list` | 用户端 | `src/api/sms.js` | 25 |
| 840 | [ ] | GET | `/api/sms/draft/{id}` | 用户端 | `src/api/sms.js` | 28 |
| 841 | [ ] | POST | `/api/sms/draft` | 用户端 | `src/api/sms.js` | 31 |
| 842 | [ ] | PUT | `/api/sms/draft/{id}` | 用户端 | `src/api/sms.js` | 34 |
| 843 | [x] | DELETE | `/api/sms/draft/{id}` | 用户端 | `src/api/sms.js` | 37 |
| 844 | [ ] | POST | `/api/sms/draft/{id}/send` | 用户端 | `src/api/sms.js` | 40 |
| 845 | [x] | GET | `/api/sms/job/list` | 用户端 | `src/api/sms.js` | 44 |
| 846 | [ ] | GET | `/api/sms/job/{id}` | 用户端 | `src/api/sms.js` | 47 |
| 847 | [ ] | POST | `/api/sms/job/{id}/pause` | 用户端 | `src/api/sms.js` | 50 |
| 848 | [ ] | POST | `/api/sms/job/{id}/resume` | 用户端 | `src/api/sms.js` | 53 |
| 849 | [ ] | POST | `/api/sms/job/{id}/stop` | 用户端 | `src/api/sms.js` | 56 |
| 850 | [ ] | POST | `/api/sms/job` | 用户端 | `src/api/sms.js` | 59 |
| 851 | [ ] | DELETE | `/api/sms/job/{id}` | 用户端 | `src/api/sms.js` | 62 |
| 852 | [ ] | GET | `/api/sms/job/{id}/records` | 用户端 | `src/api/sms.js` | 65 |
| 853 | [x] | GET | `/api/sop` | 用户端 | `src/api/sopAgent.js` | 5 |
| 854 | [ ] | POST | `/api/sop` | 用户端 | `src/api/sopAgent.js` | 9 |
| 855 | [x] | GET | `/api/sop/stats` | 用户端 | `src/api/sopAgent.js` | 13 |
| 856 | [x] | GET | `/api/sop/match` | 用户端 | `src/api/sopAgent.js` | 17 |
| 857 | [x] | GET | `/api/sop/executions` | 用户端 | `src/api/sopAgent.js` | 21 |
| 858 | [x] | GET | `/api/sop/executions/{id}` | 用户端 | `src/api/sopAgent.js` | 25 |
| 859 | [x] | POST | `/api/sop/executions/{id}/pause` | 用户端 | `src/api/sopAgent.js` | 29 |
| 860 | [x] | POST | `/api/sop/executions/{id}/resume` | 用户端 | `src/api/sopAgent.js` | 33 |
| 861 | [x] | POST | `/api/sop/executions/{id}/cancel` | 用户端 | `src/api/sopAgent.js` | 37 |
| 862 | [ ] | GET | `/api/sop/{id}` | 用户端 | `src/api/sopAgent.js` | 41 |
| 863 | [ ] | PUT | `/api/sop/{id}` | 用户端 | `src/api/sopAgent.js` | 45 |
| 864 | [ ] | DELETE | `/api/sop/{id}` | 用户端 | `src/api/sopAgent.js` | 49 |
| 865 | [ ] | POST | `/api/sop/{id}/activate` | 用户端 | `src/api/sopAgent.js` | 53 |
| 866 | [ ] | POST | `/api/sop/{id}/deactivate` | 用户端 | `src/api/sopAgent.js` | 57 |
| 867 | [ ] | POST | `/api/sop/execute` | 用户端 | `src/api/sopAgent.js` | 61 |
| 868 | [ ] | POST | `/api/sop/step` | 用户端 | `src/api/sopAgent.js` | 65 |
| 869 | [x] | GET | `/api/sop-templates` | 用户端 | `src/api/sopTemplate.js` | 5 |
| 870 | [ ] | GET | `/api/sop-templates/{id}` | 用户端 | `src/api/sopTemplate.js` | 9 |
| 871 | [ ] | POST | `/api/sop-templates` | 用户端 | `src/api/sopTemplate.js` | 13 |
| 872 | [ ] | PUT | `/api/sop-templates/{id}` | 用户端 | `src/api/sopTemplate.js` | 17 |
| 873 | [ ] | PATCH | `/api/sop-templates/{id}/toggle` | 用户端 | `src/api/sopTemplate.js` | 21 |
| 874 | [x] | DELETE | `/api/sop-templates/{id}` | 用户端 | `src/api/sopTemplate.js` | 25 |
| 875 | [ ] | POST | `/api/sop-templates/match` | 用户端 | `src/api/sopTemplate.js` | 29 |
| 876 | [x] | GET | `/api/system/info` | 用户端 | `src/api/stats.js` | 7 |
| 877 | [x] | GET | `/api/system/info` | 用户端 | `src/api/system.js` | 6 |
| 878 | [x] | GET | `/api/system/config` | 用户端 | `src/api/system.js` | 9 |
| 879 | [ ] | POST | `/api/system/config` | 用户端 | `src/api/system.js` | 12 |
| 880 | [x] | GET | `/api/system/users` | 用户端 | `src/api/systemUser.js` | 4 |
| 881 | [ ] | GET | `/api/system/users/{id}` | 用户端 | `src/api/systemUser.js` | 8 |
| 882 | [ ] | POST | `/api/system/users` | 用户端 | `src/api/systemUser.js` | 12 |
| 883 | [ ] | PUT | `/api/system/users/{id}` | 用户端 | `src/api/systemUser.js` | 16 |
| 884 | [ ] | DELETE | `/api/system/users/{id}` | 用户端 | `src/api/systemUser.js` | 20 |
| 885 | [x] | GET | `/api/session-tags` | 用户端 | `src/api/tagSegmentation.js` | 5 |
| 886 | [ ] | PUT | `/api/session-tags/{id}` | 用户端 | `src/api/tagSegmentation.js` | 12 |
| 887 | [ ] | POST | `/api/session-tags` | 用户端 | `src/api/tagSegmentation.js` | 16 |
| 888 | [ ] | DELETE | `/api/session-tags/{id}` | 用户端 | `src/api/tagSegmentation.js` | 20 |
| 889 | [x] | GET | `/api/customer-360/tag-rules` | 用户端 | `src/api/tagSegmentation.js` | 24 |
| 890 | [ ] | POST | `/api/customer-360/tag-rules` | 用户端 | `src/api/tagSegmentation.js` | 28 |
| 891 | [ ] | PUT | `/api/customer-360/tag-rules/{id}` | 用户端 | `src/api/tagSegmentation.js` | 32 |
| 892 | [x] | DELETE | `/api/customer-360/tag-rules/{id}` | 用户端 | `src/api/tagSegmentation.js` | 36 |
| 893 | [x] | GET | `/api/user-segment/layers` | 用户端 | `src/api/tagSegmentation.js` | 40 |
| 894 | [x] | GET | `/api/customer-360/tag-stats` | 用户端 | `src/api/tagSegmentation.js` | 44 |
| 895 | [x] | GET | `/api/telegram/accounts` | 用户端 | `src/api/telegram.js` | 4 |
| 896 | [ ] | GET | `/api/telegram/accounts/{id}` | 用户端 | `src/api/telegram.js` | 8 |
| 897 | [ ] | POST | `/api/telegram/accounts` | 用户端 | `src/api/telegram.js` | 12 |
| 898 | [ ] | PUT | `/api/telegram/accounts/{id}` | 用户端 | `src/api/telegram.js` | 16 |
| 899 | [ ] | DELETE | `/api/telegram/accounts/{id}` | 用户端 | `src/api/telegram.js` | 20 |
| 900 | [ ] | POST | `/api/telegram/accounts/{id}/register-webhook` | 用户端 | `src/api/telegram.js` | 24 |
| 901 | [ ] | POST | `/api/telegram/accounts/{id}/test-send` | 用户端 | `src/api/telegram.js` | 28 |
| 902 | [x] | GET | `/api/telegram/gates` | 用户端 | `src/api/telegram.js` | 34 |
| 903 | [ ] | POST | `/api/telegram/gates` | 用户端 | `src/api/telegram.js` | 38 |
| 904 | [ ] | PUT | `/api/telegram/gates/{id}` | 用户端 | `src/api/telegram.js` | 42 |
| 905 | [x] | DELETE | `/api/telegram/gates/{id}` | 用户端 | `src/api/telegram.js` | 46 |
| 906 | [ ] | GET | `/api/telegram/gates/{id}/members` | 用户端 | `src/api/telegram.js` | 50 |
| 907 | [x] | POST | `/api/telegram/gates/{id}/members/{memberId}/authorize` | 用户端 | `src/api/telegram.js` | 54 |
| 908 | [x] | GET | `/api/tiktok-card/list` | 用户端 | `src/api/tiktokCard.js` | 4 |
| 909 | [ ] | GET | `/api/tiktok-card/{id}` | 用户端 | `src/api/tiktokCard.js` | 8 |
| 910 | [ ] | POST | `/api/tiktok-card` | 用户端 | `src/api/tiktokCard.js` | 12 |
| 911 | [ ] | PUT | `/api/tiktok-card/{data_id}` | 用户端 | `src/api/tiktokCard.js` | 16 |
| 912 | [ ] | DELETE | `/api/tiktok-card/{id}` | 用户端 | `src/api/tiktokCard.js` | 20 |
| 913 | [ ] | POST | `/api/tiktok-card/generate-short-link` | 用户端 | `src/api/tiktokCard.js` | 24 |
| 914 | [x] | GET | `/api/tiktok-card/stats/overall` | 用户端 | `src/api/tiktokCard.js` | 28 |
| 915 | [ ] | GET | `/api/tiktok-card/{cardId}/stats` | 用户端 | `src/api/tiktokCard.js` | 32 |
| 916 | [x] | GET | `/api/admin/tuning/confidence/signals` | 用户端 | `src/api/tuning.js` | 4 |
| 917 | [x] | GET | `/api/admin/tuning/confidence/signals/{id}` | 用户端 | `src/api/tuning.js` | 7 |
| 918 | [x] | GET | `/api/admin/tuning/confidence/signals/stats` | 用户端 | `src/api/tuning.js` | 10 |
| 919 | [x] | GET | `/api/admin/tuning/confidence/calibrations` | 用户端 | `src/api/tuning.js` | 13 |
| 920 | [x] | GET | `/api/admin/tuning/confidence/policies` | 用户端 | `src/api/tuning.js` | 16 |
| 921 | [ ] | PUT | `/api/admin/tuning/confidence/policies` | 用户端 | `src/api/tuning.js` | 19 |
| 922 | [x] | GET | `/api/admin/tuning/humanize/scores` | 用户端 | `src/api/tuning.js` | 23 |
| 923 | [x] | GET | `/api/admin/tuning/humanize/scores/stats` | 用户端 | `src/api/tuning.js` | 26 |
| 924 | [x] | GET | `/api/admin/tuning/humanize/baselines` | 用户端 | `src/api/tuning.js` | 29 |
| 925 | [x] | GET | `/api/admin/tuning/humanize/low-quality` | 用户端 | `src/api/tuning.js` | 32 |
| 926 | [x] | GET | `/api/admin/tuning/feedback/events` | 用户端 | `src/api/tuning.js` | 36 |
| 927 | [x] | GET | `/api/admin/tuning/feedback/events/stats` | 用户端 | `src/api/tuning.js` | 39 |
| 928 | [x] | GET | `/api/admin/tuning/feedback/dialogues` | 用户端 | `src/api/tuning.js` | 42 |
| 929 | [x] | GET | `/api/admin/tuning/prompt/candidates` | 用户端 | `src/api/tuning.js` | 45 |
| 930 | [ ] | PUT | `/api/admin/tuning/prompt/candidates/{id}/status` | 用户端 | `src/api/tuning.js` | 48 |
| 931 | [x] | GET | `/api/admin/tuning/bandit/arms` | 用户端 | `src/api/tuning.js` | 51 |
| 932 | [x] | GET | `/api/unified-messages` | 用户端 | `src/api/unifiedMessage.js` | 5 |
| 933 | [x] | GET | `/api/unified-messages/{id}` | 用户端 | `src/api/unifiedMessage.js` | 9 |
| 934 | [x] | GET | `/api/user-segment/rfm/list` | 用户端 | `src/api/userSegment.js` | 4 |
| 935 | [x] | GET | `/api/user-segment/rfm/rule` | 用户端 | `src/api/userSegment.js` | 7 |
| 936 | [ ] | POST | `/api/user-segment/rfm/rule` | 用户端 | `src/api/userSegment.js` | 10 |
| 937 | [ ] | PUT | `/api/user-segment/rfm/rule/{id}` | 用户端 | `src/api/userSegment.js` | 13 |
| 938 | [ ] | GET | `/api/user-segment/rfm/user?user_id={userId}` | 用户端 | `src/api/userSegment.js` | 16 |
| 939 | [x] | GET | `/api/user-segment/rfm/stats` | 用户端 | `src/api/userSegment.js` | 19 |
| 940 | [x] | POST | `/api/user-segment/rfm/calculate` | 用户端 | `src/api/userSegment.js` | 22 |
| 941 | [x] | GET | `/api/user-segment/layers` | 用户端 | `src/api/userSegment.js` | 25 |
| 942 | [x] | GET | `/api/user-segments` | 用户端 | `src/api/userSegment.js` | 29 |
| 943 | [x] | DELETE | `/api/user-segment/rfm/rule/{id}` | 用户端 | `src/api/userSegment.js` | 41 |
| 944 | [x] | GET | `/api/user-segment/rfm/list?segment_id={id}` | 用户端 | `src/api/userSegment.js` | 44 |
| 945 | [x] | GET | `/api/users` | 用户端 | `src/api/users.js` | 5 |
| 946 | [ ] | GET | `/api/users/{id}` | 用户端 | `src/api/users.js` | 9 |
| 947 | [ ] | POST | `/api/users` | 用户端 | `src/api/users.js` | 13 |
| 948 | [ ] | PUT | `/api/users/{id}` | 用户端 | `src/api/users.js` | 17 |
| 949 | [ ] | DELETE | `/api/users/{id}` | 用户端 | `src/api/users.js` | 21 |
| 950 | [ ] | PUT | `/api/users/{id}/password` | 用户端 | `src/api/users.js` | 25 |
| 951 | [ ] | POST | `/api/auth/login` | 公开 | `src/api/users.js` | 29 |
| 952 | [ ] | POST | `/api/auth/mfa/verify` | 公开 | `src/api/users.js` | 34 |
| 953 | [x] | GET | `/api/wecom/health/accounts` | 用户端 | `src/api/wecomAccount.js` | 5 |
| 954 | [x] | GET | `/api/wecom/health/accounts/risks` | 用户端 | `src/api/wecomAccount.js` | 9 |
| 955 | [x] | GET | `/api/wecom/health/accounts/select` | 用户端 | `src/api/wecomAccount.js` | 13 |
| 956 | [x] | GET | `/api/wecom/health/accounts/summary` | 用户端 | `src/api/wecomAccount.js` | 17 |
| 957 | [ ] | GET | `/api/wecom/health/accounts/{id}` | 用户端 | `src/api/wecomAccount.js` | 21 |
| 958 | [x] | GET | `/api/wecom/health/accounts/{id}/history` | 用户端 | `src/api/wecomAccount.js` | 25 |
| 959 | [ ] | POST | `/api/wecom/health/accounts/{id}` | 用户端 | `src/api/wecomAccount.js` | 29 |
| 960 | [ ] | POST | `/api/wecom/health/accounts/{id}/status` | 用户端 | `src/api/wecomAccount.js` | 33 |
| 961 | [ ] | POST | `/api/wecom/health/accounts/{id}/quota/consume` | 用户端 | `src/api/wecomAccount.js` | 37 |
| 962 | [x] | POST | `/api/wecom/health/accounts/quota/reset` | 用户端 | `src/api/wecomAccount.js` | 41 |
| 963 | [ ] | POST | `/api/wecom/messages/ingest` | 用户端 | `src/api/wecomAccount.js` | 45 |
| 964 | [ ] | POST | `/api/wecom/messages/send` | 用户端 | `src/api/wecomAccount.js` | 49 |
| 965 | [ ] | POST | `/api/wecom/accounts` | 用户端 | `src/api/wecomAccount.js` | 53 |
| 966 | [ ] | PUT | `/api/wecom/accounts/{id}` | 用户端 | `src/api/wecomAccount.js` | 57 |
| 967 | [x] | DELETE | `/api/wecom/accounts/{id}` | 用户端 | `src/api/wecomAccount.js` | 61 |
| 968 | [x] | GET | `/api/wecom/customers` | 用户端 | `src/api/wecomAccount.js` | 65 |
| 969 | [x] | GET | `/api/wecom/groups` | 用户端 | `src/api/wecomAccount.js` | 69 |
| 970 | [x] | GET | `/api/wecom/tags` | 用户端 | `src/api/wecomAccount.js` | 73 |
| 971 | [x] | GET | `/api/wecom/messages` | 用户端 | `src/api/wecomAccount.js` | 77 |
| 972 | [ ] | POST | `/api/wecom/accounts/{id}/sync-customers` | 用户端 | `src/api/wecomAccount.js` | 81 |
| 973 | [ ] | POST | `/api/wecom/accounts/{id}/sync-groups` | 用户端 | `src/api/wecomAccount.js` | 85 |
| 974 | [ ] | POST | `/api/wecom/accounts/{id}/sync-tags` | 用户端 | `src/api/wecomAccount.js` | 89 |
| 975 | [ ] | POST | `/api/wecom/accounts/{id}/refresh` | 用户端 | `src/api/wecomAccount.js` | 93 |
| 976 | [ ] | POST | `/api/wecom/accounts/{id}/send-message` | 用户端 | `src/api/wecomAccount.js` | 97 |
| 977 | [x] | GET | `/api/whatsapp/accounts` | 用户端 | `src/api/whatsapp.js` | 3 |
| 978 | [x] | POST | `/api/whatsapp/accounts` | 用户端 | `src/api/whatsapp.js` | 5 |
| 979 | [ ] | POST | `/api/whatsapp/accounts/{id}/login/start` | 用户端 | `src/api/whatsapp.js` | 6 |
| 980 | [ ] | GET | `/api/whatsapp/accounts/{id}/login/status` | 用户端 | `src/api/whatsapp.js` | 7 |
| 981 | [ ] | PUT | `/api/whatsapp/accounts/{id}` | 用户端 | `src/api/whatsapp.js` | 8 |
| 982 | [ ] | DELETE | `/api/whatsapp/accounts/{id}` | 用户端 | `src/api/whatsapp.js` | 9 |
| 983 | [x] | GET | `/api/whatsapp/drafts` | 用户端 | `src/api/whatsapp.js` | 11 |
| 984 | [x] | POST | `/api/whatsapp/drafts` | 用户端 | `src/api/whatsapp.js` | 12 |
| 985 | [ ] | PUT | `/api/whatsapp/drafts/{id}` | 用户端 | `src/api/whatsapp.js` | 13 |
| 986 | [ ] | DELETE | `/api/whatsapp/drafts/{id}` | 用户端 | `src/api/whatsapp.js` | 14 |
| 987 | [x] | GET | `/api/whatsapp/jobs` | 用户端 | `src/api/whatsapp.js` | 16 |
| 988 | [ ] | POST | `/api/whatsapp/jobs` | 用户端 | `src/api/whatsapp.js` | 17 |
| 989 | [ ] | GET | `/api/whatsapp/jobs/{id}` | 用户端 | `src/api/whatsapp.js` | 18 |
| 990 | [ ] | DELETE | `/api/whatsapp/jobs/{id}` | 用户端 | `src/api/whatsapp.js` | 19 |
| 991 | [x] | GET | `/api/whatsapp-cloud/accounts` | 用户端 | `src/api/whatsappCloud.js` | 6 |
| 992 | [ ] | GET | `/api/whatsapp-cloud/accounts/{id}` | 用户端 | `src/api/whatsappCloud.js` | 10 |
| 993 | [ ] | POST | `/api/whatsapp-cloud/accounts` | 用户端 | `src/api/whatsappCloud.js` | 14 |
| 994 | [ ] | PUT | `/api/whatsapp-cloud/accounts/{id}` | 用户端 | `src/api/whatsappCloud.js` | 18 |
| 995 | [ ] | DELETE | `/api/whatsapp-cloud/accounts/{id}` | 用户端 | `src/api/whatsappCloud.js` | 22 |
| 996 | [ ] | POST | `/api/whatsapp-cloud/accounts/{id}/test-send` | 用户端 | `src/api/whatsappCloud.js` | 26 |
| 997 | [x] | GET | `/api/workflows/versions` | 用户端 | `src/api/workflowOrchestrator.js` | 6 |
| 998 | [x] | GET | `/api/workflows/versions` | 用户端 | `src/api/workflowOrchestrator.js` | 9 |
| 999 | [x] | GET | `/api/workflows/versions` | 用户端 | `src/api/workflowOrchestrator.js` | 17 |
| 1000 | [ ] | GET | `/api/workflows/versions/{id}` | 用户端 | `src/api/workflowOrchestrator.js` | 21 |
| 1001 | [ ] | POST | `/api/workflows/versions` | 用户端 | `src/api/workflowOrchestrator.js` | 25 |
| 1002 | [ ] | PUT | `/api/workflows/versions/{id}` | 用户端 | `src/api/workflowOrchestrator.js` | 29 |
| 1003 | [x] | DELETE | `/api/workflows/versions/{id}` | 用户端 | `src/api/workflowOrchestrator.js` | 33 |
| 1004 | [x] | POST | `/api/workflows/versions/{id}/publish` | 用户端 | `src/api/workflowOrchestrator.js` | 37 |
| 1005 | [x] | POST | `/api/workflows/versions/{id}/archive` | 用户端 | `src/api/workflowOrchestrator.js` | 41 |
| 1006 | [ ] | POST | `/api/workflows/execute` | 用户端 | `src/api/workflowOrchestrator.js` | 45 |
| 1007 | [x] | GET | `/api/workflows/executions` | 用户端 | `src/api/workflowOrchestrator.js` | 49 |
| 1008 | [x] | GET | `/api/workflows/executions/{id}` | 用户端 | `src/api/workflowOrchestrator.js` | 53 |
| 1009 | [x] | GET | `/api/workflows/executions/{executionId}/nodes` | 用户端 | `src/api/workflowOrchestrator.js` | 57 |
| 1010 | [ ] | POST | `/api/workflows/executions/{id}/stop` | 用户端 | `src/api/workflowOrchestrator.js` | 61 |
| 1011 | [x] | GET | `/api/xianyu/list` | 用户端 | `src/api/xianyuCard.js` | 4 |
| 1012 | [ ] | GET | `/api/xianyu/{id}` | 用户端 | `src/api/xianyuCard.js` | 8 |
| 1013 | [ ] | POST | `/api/xianyu/create` | 用户端 | `src/api/xianyuCard.js` | 12 |
| 1014 | [ ] | PUT | `/api/xianyu/update` | 用户端 | `src/api/xianyuCard.js` | 16 |
| 1015 | [ ] | DELETE | `/api/xianyu/delete/{id}` | 用户端 | `src/api/xianyuCard.js` | 20 |
| 1016 | [ ] | GET | `/api/xianyu/view/{id}` | 用户端 | `src/api/xianyuCard.js` | 24 |
| 1017 | [x] | GET | `/api/xianyu/stats/card/{id}` | 用户端 | `src/api/xianyuCard.js` | 28 |
| 1018 | [x] | GET | `/api/xianyu/stats/overall` | 用户端 | `src/api/xianyuCard.js` | 32 |
| 1019 | [ ] | POST | `/api/xianyu/{id}/generate-short-link` | 用户端 | `src/api/xianyuCard.js` | 36 |
| 1020 | [x] | GET | `/api/xiaohongshu/list` | 用户端 | `src/api/xiaohongshuCard.js` | 4 |
| 1021 | [ ] | GET | `/api/xiaohongshu/{id}` | 用户端 | `src/api/xiaohongshuCard.js` | 8 |
| 1022 | [ ] | POST | `/api/xiaohongshu/create` | 用户端 | `src/api/xiaohongshuCard.js` | 12 |
| 1023 | [ ] | PUT | `/api/xiaohongshu/update` | 用户端 | `src/api/xiaohongshuCard.js` | 16 |
| 1024 | [ ] | DELETE | `/api/xiaohongshu/delete/{id}` | 用户端 | `src/api/xiaohongshuCard.js` | 20 |
| 1025 | [ ] | GET | `/api/xiaohongshu/view/{id}` | 用户端 | `src/api/xiaohongshuCard.js` | 24 |
| 1026 | [ ] | GET | `/api/xiaohongshu/stats/card/{id}` | 用户端 | `src/api/xiaohongshuCard.js` | 28 |
| 1027 | [x] | GET | `/api/xiaohongshu/stats/overall` | 用户端 | `src/api/xiaohongshuCard.js` | 32 |
| 1028 | [ ] | POST | `/api/xiaohongshu/{id}/generate-short-link` | 用户端 | `src/api/xiaohongshuCard.js` | 36 |
