# HiveMtk 用户端 API 清单

来源：`user-web/src/api/*.js`（111 个文件）· 调用点 1028

约定：路径保留前端模板字面量原样（`${id}` 等为路径参数）；`query` 列为 GET/DELETE 的第二参数，`body` 列为 POST/PUT/PATCH 的第二参数。

---


## abExperiment.js  (11)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 1 | GET | `/api/ab-experiments` | `getExperimentList` | `params` | `params` |
| 2 | GET | `/api/ab-experiments/${id}` | `getExperiment` | `id` | `` |
| 3 | POST | `/api/ab-experiments` | `createExperiment` | `data` | `data` |
| 4 | PUT | `/api/ab-experiments/${id}` | `updateExperiment` | `id, data` | `data` |
| 5 | DELETE | `/api/ab-experiments/${id}` | `deleteExperiment` | `id` | `` |
| 6 | POST | `/api/ab-experiments/${id}/start` | `startExperiment` | `id` | `` |
| 7 | POST | `/api/ab-experiments/${id}/pause` | `pauseExperiment` | `id` | `` |
| 8 | POST | `/api/ab-experiments/${id}/stop` | `stopExperiment` | `id` | `` |
| 9 | GET | `/api/ab-experiments/${id}/results` | `getExperimentResults` | `id` | `` |
| 10 | GET | `/api/ab-experiments/${id}/conversion-events` | `getConversionEvents` | `id` | `` |
| 11 | GET | `/api/ab-experiments` | `getExperimentStats` | `` | `` |

## abExperimentPlus.js  (5)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 12 | GET | `/api/ab-experiments/${id}/stats` | `getExperimentWithStats` | `id, method = 'frequentist'` | `{ method }` |
| 13 | GET | `/api/ab-experiments/${id}/diagnostics` | `getExperimentDiagnostics` | `id` | `` |
| 14 | GET | `/api/ab-experiments/${id}/cuped` | `getExperimentWithCUPED` | `id` | `` |
| 15 | POST | `/api/ab-experiments/${id}/sequential-test` | `sequentialTest` | `id, alpha = 0.05` | `{ alpha }` |
| 16 | POST | `/api/ab-experiments/${id}/bayesian-test` | `bayesianTest` | `id` | `{}` |

## agentKBBinding.js  (5)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 17 | POST | `/api/agent-kb-bindings` | `bind` | `data` | `data` |
| 18 | DELETE | `/api/agent-kb-bindings/${agentId}/${kbId}` | `unbind` | `agentId, kbId` | `` |
| 19 | GET | `/api/agent-kb-bindings/by-agent/${agentId}` | `listByAgent` | `agentId` | `` |
| 20 | GET | `/api/agent-kb-bindings/by-kb/${kbId}` | `listByKB` | `kbId` | `` |
| 21 | PUT | `/api/agent-kb-bindings/by-agent/${agentId}` | `replaceAgentKBs` | `agentId, kbIds` | `{ kb_ids: kbIds }` |

## aiAgent.js  (9)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 22 | GET | `/api/ai-agents` | `listAgents` | `params` | `params` |
| 23 | GET | `/api/ai-agents-enabled` | `listEnabledAgents` | `` | `` |
| 24 | GET | `/api/ai-agents/${id}` | `getAgent` | `id` | `` |
| 25 | POST | `/api/ai-agents` | `createAgent` | `data` | `data` |
| 26 | PUT | `/api/ai-agents/${id}` | `updateAgent` | `id, data` | `data` |
| 27 | DELETE | `/api/ai-agents/${id}` | `deleteAgent` | `id` | `` |
| 28 | POST | `/api/ai-agents/${id}/toggle` | `toggleAgent` | `id, status` | `{ status }` |
| 29 | POST | `/api/ai-agents/${id}/test` | `testAgent` | `id, data` | `data` |
| 30 | GET | `/api/ai-agents/${id}/context` | `getAgentContext` | `id` | `` |

## aiProductivity.js  (2)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 31 | GET | `/api/ai-productivity/overview` | `getReport` | `params` | `params` |
| 32 | GET | `/api/ai-productivity/trend` | `getTrend` | `params` | `params` |

## aiTool.js  (7)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 33 | GET | `/api/ai-tools` | `listTools` | `params` | `params` |
| 34 | GET | `/api/ai-tools/${name}` | `getTool` | `name` | `` |
| 35 | PUT | `/api/ai-tools/${name}/status` | `updateToolStatus` | `name, enabled` | `{ is_enabled: enabled }` |
| 36 | POST | `/api/ai-tools/batch-status` | `batchUpdateToolStatus` | `tools, enabled` | `{ tools, is_enabled: enabled }` |
| 37 | GET | `/api/ai-tools/${toolName}/accounts` | `getToolAccounts` | `toolName` | `` |
| 38 | POST | `/api/ai-tools/${toolName}/accounts` | `bindToolAccount` | `toolName, accountType, accountId, isPrimary` | `{` |
| 39 | DELETE | `/api/ai-tools/${toolName}/accounts/${accountType}/${accountId}` | `unbindToolAccount` | `toolName, accountType, accountId` | `` |

## approval.js  (2)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 40 | GET | `${base}/${seg(id)}` | `get` | `id` | `` |
| 41 | POST | `${base}/${seg(id)}/decide` | `decide` | `id, verdict, note` | `{ verdict, note }` |
| | | | | opts | `{ _silent: true }` |

## assetBundle.js  (16)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 42 | POST | `/api/asset-bundle` | `createBundle` | `data` | `data` |
| 43 | PUT | `/api/asset-bundle/${id}` | `updateBundle` | `id, data` | `data` |
| 44 | GET | `/api/asset-bundle/${id}` | `getBundle` | `id` | `` |
| 45 | GET | `/api/asset-bundle/by-aid/${aid}` | `getBundleByAssetID` | `aid` | `` |
| 46 | POST | `/api/asset-bundle/list` | `listBundles` | `data` | `data` |
| 47 | POST | `/api/asset-bundle/${id}/publish` | `publishBundle` | `id` | `` |
| 48 | POST | `/api/asset-bundle/${id}/submit-platform` | `submitToPlatform` | `id` | `` |
| 49 | POST | `/api/asset-bundle/${id}/archive` | `archiveBundle` | `id` | `` |
| 50 | DELETE | `/api/asset-bundle/${id}` | `deleteBundle` | `id` | `` |
| 51 | POST | `/api/asset-bundle/weave` | `weaveBundle` | `data` | `data` |
| 52 | POST | `/api/asset-bundle/merchant-save` | `merchantSave` | `data` | `data` |
| 53 | POST | `/api/asset-bundle/merchant-parse/${aid}` | `merchantParse` | `aid` | `` |
| 54 | POST | `/api/asset-bundle/${id}/enable` | `enableBundle` | `id` | `` |
| 55 | POST | `/api/asset-bundle/${id}/disable` | `disableBundle` | `id` | `` |
| 56 | POST | `/api/asset-bundle/enabled/list` | `listEnabledBundles` | `` | `` |
| 57 | POST | `/api/upload` | `uploadCover` | `file` | `fd` |
| | | | | opts | `{` |

## assetMarket.js  (12)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 58 | GET | `/api/v1/asset-market/list` | `listAssets` | `params` | `params` |
| 59 | GET | `/api/v1/asset-market/detail/${id}` | `assetDetail` | `id` | `` |
| 60 | POST | `/api/v1/asset-market/purchase` | `purchaseAsset` | `data` | `data` |
| 61 | POST | `/api/v1/asset-market/sync` | `syncAsset` | `data` | `data` |
| 62 | POST | `/api/v1/asset-market/report-usage` | `reportUsage` | `data` | `data` |
| 63 | GET | `/api/v1/local-assets` | `listLocalAssets` | `params` | `params` |
| 64 | GET | `/api/v1/local-assets/${id}` | `getLocalAsset` | `id` | `` |
| 65 | POST | `/api/v1/local-assets` | `createLocalAsset` | `data` | `data` |
| 66 | PUT | `/api/v1/local-assets/${id}` | `updateLocalAsset` | `id, data` | `data` |
| 67 | DELETE | `/api/v1/local-assets/${id}` | `deleteLocalAsset` | `id` | `` |
| 68 | PUT | `/api/v1/local-assets/${id}/toggle-active` | `toggleLocalAsset` | `id, active` | `{ active }` |
| 69 | GET | `/api/v1/local-assets/sync-log` | `syncLog` | `params` | `params` |

## backup.js  (7)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 70 | GET | `/api/backups` | `getBackupList` | `params` | `params` |
| 71 | GET | `/api/backups/${id}` | `getBackupByID` | `id` | `` |
| 72 | POST | `/api/backups` | `createBackup` | `data` | `data` |
| 73 | DELETE | `/api/backups/${id}` | `deleteBackup` | `id` | `` |
| 74 | POST | `/api/restore` | `restoreBackup` | `data` | `data` |
| 75 | GET | `/api/restore/list` | `getRestoreList` | `params` | `params` |
| 76 | GET | `/api/restore/last` | `getLastRestore` | `` | `` |

## badCase.js  (8)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 77 | GET | `base` | `list` | `params` | `params` |
| | | | | opts | `{ paramsSerializer: toQuery }` |
| 78 | GET | `${base}/stats` | `stats` | `` | `undefined` |
| | | | | opts | `{ _silent: true }` |
| 79 | GET | `${base}/taxonomy` | `taxonomy` | `` | `undefined` |
| | | | | opts | `{ _silent: true }` |
| 80 | GET | `${base}/${seg(id)}` | `get` | `id` | `` |
| 81 | POST | `base` | `create` | `input` | `input` |
| 82 | POST | `${base}/${seg(id)}/label` | `label` | `id, label, note` | `{ label, note }` |
| 83 | POST | `${base}/${seg(id)}/dismiss` | `dismiss` | `id, reason` | `{ reason }` |
| 84 | POST | `${base}/export` | `exportSet` | `labels, limit` | `{ labels, limit }` |
| | | | | opts | `{ _silent: true }` |

## batchOperation.js  (10)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 85 | POST | `/api/batch/import` | `batchImportFile` | `data` | `data` |
| 86 | GET | `/api/batch/template` | `downloadBatchTemplate` | `` | `` |
| 87 | POST | `/api/batch/export` | `batchExport` | `data` | `data` |
| 88 | POST | `/api/batch/delete` | `batchDelete` | `data` | `data` |
| 89 | POST | `/api/batch/update` | `batchUpdate` | `data` | `data` |
| 90 | GET | `/api/batch/tools` | `getBatchTools` | `` | `` |
| 91 | GET | `/api/batch/histories` | `getBatchHistories` | `params` | `params` |
| 92 | POST | `/api/batch/histories/${id}/cancel` | `cancelBatch` | `id` | `` |
| 93 | GET | `/api/batch/histories/${id}` | `getBatchDetail` | `id` | `` |
| 94 | POST | `/api/batch/preview` | `previewBatch` | `data` | `data` |

## bridgeToken.js  (2)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 95 | GET | `/api/bridge/token/status` | `getBridgeTokenStatus` | `` | `` |
| 96 | POST | `/api/bridge/token/reset` | `resetBridgeToken` | `` | `` |

## browserAutomation.js  (31)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 97 | GET | `/api/browser-automation/tasks` | `listBrowserTasks` | `params` | `params` |
| 98 | GET | `/api/browser-automation/tasks/${id}` | `getBrowserTask` | `id` | `` |
| 99 | POST | `/api/browser-automation/tasks` | `createBrowserTask` | `data` | `data` |
| 100 | PUT | `/api/browser-automation/tasks/${id}` | `updateBrowserTask` | `id, data` | `data` |
| 101 | DELETE | `/api/browser-automation/tasks/${id}` | `deleteBrowserTask` | `id` | `` |
| 102 | POST | `/api/browser-automation/tasks/${id}/publish` | `publishBrowserTask` | `id` | `` |
| 103 | POST | `/api/browser-automation/tasks/${id}/run` | `runBrowserTask` | `id` | `{}` |
| | | | | opts | `{ _silent: true }` |
| 104 | POST | `/api/browser-automation/tasks/${id}/pause` | `pauseBrowserTask` | `id` | `` |
| 105 | POST | `/api/browser-automation/tasks/${id}/resume` | `resumeBrowserTask` | `id` | `` |
| 106 | POST | `/api/browser-automation/tasks/${id}/archive` | `archiveBrowserTask` | `id` | `` |
| 107 | PUT | `/api/browser-automation/tasks/${id}/dependency` | `setBrowserTaskDependency` | `id, data` | `data` |
| 108 | GET | `/api/browser-automation/sessions` | `listBrowserSessions` | `params` | `params` |
| 109 | GET | `/api/browser-automation/sessions/${id}` | `getBrowserSession` | `id` | `` |
| 110 | GET | `/api/browser-automation/sessions/${id}/steps` | `getBrowserSessionSteps` | `id` | `` |
| 111 | GET | `/api/browser-automation/tasks/${taskId}/sessions` | `listBrowserTaskSessions` | `taskId, params` | `params` |
| 112 | POST | `/api/browser-automation/sessions/${id}/stop` | `stopBrowserSession` | `id, reason` | `{ reason: reason \|\| '' }` |
| 113 | GET | `/api/browser-automation/sessions/${id}/confirm-gate` | `getBrowserConfirmGate` | `id` | `` |
| 114 | POST | `/api/browser-automation/sessions/${id}/confirm` | `confirmBrowserSession` | `id, payloadHash` | `{ payload_hash: hash }` |
| | | | | opts | `{ _silent: true }` |
| 115 | GET | `/api/browser-automation/sessions/${id}/logs` | `getBrowserSessionLogs` | `id, direction` | `direction ? { direction } : undefined` |
| 116 | GET | `/api/browser-automation/sessions/${id}/export` | `exportBrowserSessionAudit` | `id` | `` |
| 117 | GET | `/api/browser-automation/tasks/${taskId}/receipts` | `listBrowserTaskReceipts` | `taskId` | `` |
| 118 | GET | `/api/browser-automation/cron` | `listBrowserCron` | `` | `` |
| 119 | POST | `/api/browser-automation/cron` | `createBrowserCron` | `data` | `data` |
| 120 | PUT | `/api/browser-automation/cron/${id}` | `updateBrowserCron` | `id, data` | `data` |
| 121 | DELETE | `/api/browser-automation/cron/${id}` | `deleteBrowserCron` | `id` | `` |
| 122 | POST | `/api/browser-automation/cron/${id}/enable` | `enableBrowserCron` | `id` | `` |
| 123 | POST | `/api/browser-automation/cron/${id}/disable` | `disableBrowserCron` | `id` | `` |
| 124 | GET | `/api/browser-automation/host/status` | `getBrowserHostStatus` | `` | `` |
| 125 | POST | `/api/browser-automation/host/token/reset` | `resetBrowserHostToken` | `` | `` |
| 126 | GET | `/api/browser-automation/platforms` | `listPlatforms` | `` | `` |
| 127 | GET | `/api/browser-automation/platforms/${id}/locators` | `getPlatformLocators` | `id` | `` |

## bulkMessaging.js  (7)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 128 | GET | `/api/whatsapp/templates` | `getTemplates` | `params = {}` | `params` |
| 129 | POST | `/api/whatsapp/templates` | `createTemplate` | `data` | `data` |
| 130 | PUT | `/api/whatsapp/templates/${id}` | `updateTemplate` | `id, data` | `data` |
| 131 | DELETE | `/api/whatsapp/templates/${id}` | `deleteTemplate` | `id` | `` |
| 132 | POST | `/api/whatsapp/group-messaging/send` | `sendBulkMessage` | `data` | `data` |
| 133 | GET | `/api/whatsapp/group-messaging/status/${queueId}` | `getMessageStatus` | `queueId` | `` |
| 134 | GET | `/api/whatsapp/group-messaging/records` | `getSendRecords` | `params = {}` | `params` |

## channelAgentBinding.js  (5)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 135 | GET | `/api/channel-agent-bindings` | `listBindings` | `params` | `params` |
| 136 | GET | `/api/channel-agent-bindings/by-agent/${agentId}` | `listBindingsByAgent` | `agentId` | `` |
| 137 | POST | `/api/channel-agent-bindings` | `createBinding` | `data` | `data` |
| 138 | PUT | `/api/channel-agent-bindings/${id}` | `updateBinding` | `id, data` | `data` |
| 139 | DELETE | `/api/channel-agent-bindings/${id}` | `deleteBinding` | `id` | `` |

## chat.js  (1)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 140 | GET | `/api/chat/public/upload-token` | `getUploadToken` | `params` | `{ params }` |

## chatChannel.js  (7)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 141 | GET | `/api/chat-channels` | `listChannels` | `params` | `params` |
| 142 | GET | `/api/chat-channels/${channelId}` | `getChannel` | `channelId` | `` |
| 143 | POST | `/api/chat-channels` | `createChannel` | `data` | `data` |
| 144 | PUT | `/api/chat-channels/${channelId}` | `updateChannel` | `channelId, data` | `data` |
| 145 | DELETE | `/api/chat-channels/${channelId}` | `deleteChannel` | `channelId` | `` |
| 146 | POST | `/api/chat-channels/${channelId}/rotate-key` | `rotateAppKey` | `channelId` | `` |
| 147 | POST | `/api/chat-channels/${channelId}/reset-secret` | `resetAppSecret` | `channelId` | `` |

## churnPrediction.js  (12)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 148 | GET | `/api/churn/prediction` | `getChurnPrediction` | `params` | `params` |
| 149 | GET | `/api/churn-prediction` | `getChurnPredictions` | `params` | `params` |
| 150 | GET | `/api/churn-prediction/users` | `getHighRiskUsers` | `params` | `params` |
| 151 | GET | `/api/churn-prediction/warnings` | `getChurnWarnings` | `params` | `params` |
| 152 | GET | `/api/churn/unhandled-warnings` | `getUnhandledWarnings` | `params` | `params` |
| 153 | POST | `/api/churn/warnings/${id}/handle` | `markWarningHandled` | `id, data` | `data` |
| 154 | GET | `/api/churn-prediction/model-config` | `getChurnModelConfig` | `` | `` |
| 155 | POST | `/api/churn-prediction/model-config` | `saveChurnModelConfig` | `data` | `data` |
| 156 | GET | `/api/churn-prediction/statistics` | `getChurnStatistics` | `params` | `params` |
| 157 | GET | `/api/churn/risk-distribution` | `getRiskDistribution` | `` | `` |
| 158 | POST | `/api/churn/warnings/intervene` | `interveneUser` | `data` | `data` |
| 159 | POST | `/api/user-segment/rfm/calculate` | `calculateRFM` | `data` | `data` |

## clue.js  (4)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 160 | DELETE | `/api/clues/delete/${id}` | `?` | `` | `` |
| 161 | GET | `/api/clues/list?${q}` | `?` | `` | `` |
| 162 | GET | `/api/clues/statistics` | `?` | `` | `` |
| 163 | POST | `/api/clues/import` | `?` | `` | `data` |

## cockpit.js  (5)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 164 | GET | `/api/system/stats` | `getSystemStatus` | `` | `` |
| 165 | GET | `/api/monitor/health` | `getOpsStats` | `` | `` |
| 166 | GET | `/api/monitor/node-health` | `getModuleStatus` | `` | `` |
| 167 | GET | `/api/monitor/anomalies` | `getRecentOperations` | `limit = 20` | `{ limit }` |
| 168 | GET | `/api/ai/sales-cockpit` | `getSalesCockpit` | `` | `` |

## collaboration.js  (9)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 169 | POST | `/api/customer-sessions/${sessionId}/messages` | `sendCollaborativeMessage` | `sessionId, data` | `data` |
| 170 | POST | `/api/customer-sessions/${sessionId}/internal-notes` | `addInternalNote` | `sessionId, data` | `data` |
| 171 | GET | `/api/users/search` | `searchMentionUsers` | `params` | `params` |
| 172 | POST | `/api/mentions/${mentionId}/read` | `markMentionRead` | `mentionId` | `{}` |
| 173 | GET | `/api/mentions/mine` | `getMyMentions` | `params` | `params` |
| 174 | POST | `/api/customer-sessions/${sessionId}/edit-lock` | `acquireEditLock` | `sessionId` | `{ holder: 'me' }` |
| | | | | opts | `{ _silent: true }` |
| 175 | DELETE | `/api/customer-sessions/${sessionId}/edit-lock` | `releaseEditLock` | `sessionId` | `{ _silent: true }` |
| 176 | GET | `/api/customer-sessions/${sessionId}/edit-lock` | `getEditLock` | `sessionId` | `{ _silent: true }` |
| 177 | GET | `/api/customer-sessions/${sessionId}/internal-notes` | `listInternalNotes` | `sessionId, params` | `params` |

## community.js  (10)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 178 | GET | `/api/community/groups` | `getGroups` | `params` | `{ params }` |
| 179 | POST | `/api/community/groups` | `createGroup` | `data` | `data` |
| 180 | GET | `/api/community/groups/${id}` | `getGroupById` | `id` | `` |
| 181 | PUT | `/api/community/groups/${id}` | `updateGroup` | `id, data` | `data` |
| 182 | DELETE | `/api/community/groups/${id}` | `deleteGroup` | `id` | `` |
| 183 | GET | `/api/community/members` | `getMembers` | `params` | `{ params }` |
| 184 | GET | `/api/community/messages` | `getMessages` | `params` | `{ params }` |
| 185 | GET | `/api/community/stats` | `getStats` | `params` | `{ params }` |
| 186 | POST | `/api/community/import` | `importData` | `data` | `data` |
| 187 | POST | `/api/community/export` | `exportData` | `data` | `data` |

## configParams.js  (6)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 188 | GET | `/api/manage/config-params` | `list` | `group, includeDeprecated = false` | `{` |
| 189 | GET | `/api/manage/config-params/groups` | `listGroups` | `` | `` |
| 190 | GET | `/api/manage/config-params/audit-logs` | `auditLogs` | `limit = 100` | `{ params: { limit } }` |
| 191 | PUT | `/api/manage/config-params/${group}/${key}` | `update` | `group, key, value` | `{ value }` |
| 192 | POST | `/api/manage/config-params/${group}/${key}/reset` | `reset` | `group, key` | `` |
| 193 | POST | `/api/manage/config-params/${group}/reset` | `bulkReset` | `group` | `` |

## conversionFunnel.js  (2)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 194 | GET | `/api/conversion-funnels` | `getFunnel` | `params` | `params` |
| 195 | GET | `/api/conversion-funnels/stage` | `getStageDetails` | `stage, params` | `{ params: { stage, ...params } }` |

## csat.js  (7)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 196 | POST | `/api/customer-sessions/${sessionId}/csat` | `submitCSAT` | `sessionId, data` | `data` |
| 197 | GET | `/api/csat/stats` | `getCSATStats` | `params` | `params` |
| 198 | GET | `/api/csat/trend` | `getCSATTrend` | `params` | `params` |
| 199 | GET | `/api/csat/negative` | `getNegativeCSAT` | `params` | `params` |
| 200 | POST | `/api/customer-sessions/${sessionId}/csat/trigger` | `triggerCSATSurvey` | `sessionId` | `{}` |
| 201 | GET | `/api/csat/template` | `getCSATTemplate` | `` | `` |
| 202 | PUT | `/api/csat/template` | `saveCSATTemplate` | `data` | `data` |

## customReport.js  (10)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 203 | GET | `/api/custom-reports` | `getReportList` | `params` | `params` |
| 204 | GET | `/api/custom-reports/${id}` | `getReport` | `id` | `` |
| 205 | POST | `/api/custom-reports` | `createReport` | `data` | `data` |
| 206 | PUT | `/api/custom-reports/${id}` | `updateReport` | `id, data` | `data` |
| 207 | DELETE | `/api/custom-reports/${id}` | `deleteReport` | `id` | `` |
| 208 | GET | `/api/custom-reports/templates` | `getPublicTemplates` | `` | `` |
| 209 | POST | `/api/custom-reports/templates/${id}/use` | `useReportTemplate` | `id` | `` |
| 210 | GET | `/api/custom-reports/${id}/data` | `queryReportData` | `id, params` | `params` |
| 211 | GET | `/api/custom-reports/${id}/data?format=export` | `exportCustomReport` | `id` | `` |
| 212 | GET | `/api/custom-reports/${id}/data?format=run` | `runCustomReport` | `id` | `` |

## customer360.js  (10)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 213 | GET | `/api/customer-360/basic?user_id=${id}` | `getCustomerBasic` | `id, signal` | `undefined` |
| | | | | opts | `{ signal }` |
| 214 | GET | `/api/customer-360/events` | `getCustomerEvents` | `id, params, signal` | `{ user_id: id, ...params }` |
| | | | | opts | `{ signal }` |
| 215 | GET | `/api/customer-360/sessions` | `getCustomerSessions` | `id, params, signal` | `{ user_id: id, ...params }` |
| | | | | opts | `{ signal }` |
| 216 | GET | `/api/customer-360/tags?user_id=${id}` | `getCustomerTags` | `id, signal` | `undefined` |
| | | | | opts | `{ signal }` |
| 217 | GET | `/api/customer-360/orders` | `getCustomerOrders` | `id, params, signal` | `{ user_id: id, ...params }` |
| | | | | opts | `{ signal }` |
| 218 | GET | `/api/customer-360/list` | `getCustomerList` | `params` | `params` |
| 219 | GET | `/api/customer-360/basic?user_id=${encodeURIComponent(id)}` | `getCustomerBasicById` | `id` | `` |
| 220 | GET | `/api/customer-360/tags?user_id=${encodeURIComponent(id)}` | `fetchTagsRaw` | `id` | `` |
| 221 | PUT | `/api/customer-360/tags?user_id=${encodeURIComponent(id)}` | `addCustomerTag` | `id, tag` | `{ tags }` |
| 222 | PUT | `/api/customer-360/tags?user_id=${encodeURIComponent(id)}` | `removeCustomerTag` | `id, tag` | `{ tags }` |

## customerEvent.js  (11)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 223 | POST | `/api/events/track` | `trackEvent` | `data` | `data` |
| 224 | POST | `/api/events/pageview` | `trackPageView` | `data` | `data` |
| 225 | POST | `/api/events/click` | `trackClick` | `data` | `data` |
| 226 | POST | `/api/events/purchase` | `trackPurchase` | `data` | `data` |
| 227 | POST | `/api/events/signup` | `trackSignup` | `data` | `data` |
| 228 | POST | `/api/events/login` | `trackLogin` | `data` | `data` |
| 229 | POST | `/api/events/add-to-cart` | `trackAddToCart` | `data` | `data` |
| 230 | GET | `/api/events/customer/${encodeURIComponent(customerId)}` | `getCustomerEventHistory` | `customerId, params` | `params` |
| 231 | GET | `/api/events/stats` | `getEventStats` | `params` | `params` |
| 232 | DELETE | `/api/events/customer/${encodeURIComponent(id)}` | `deleteEvent` | `id` | `` |
| 233 | GET | `/api/customer-events/list` | `getGlobalEvents` | `params` | `params` |

## customerJourney.js  (6)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 234 | GET | `/api/customer-journey/overview` | `getJourneyOverview` | `` | `` |
| 235 | GET | `/api/customer-journey/overview?customer_id=${customerId}` | `getJourneyState` | `customerId` | `` |
| 236 | GET | `/api/customer-journey/stages` | `listJourneyStages` | `` | `` |
| 237 | GET | `/api/customer-journey/by-stage?stage=${stage}` | `listByStage` | `stage` | `` |
| 238 | POST | `/api/customer-journey/transition` | `transitionJourney` | `data` | `data` |
| 239 | POST | `/api/customer-journey/touch` | `touchCustomer` | `data` | `data` |

## customerService.js  (21)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 240 | POST | `/api/agents` | `createAgent` | `data` | `data` |
| 241 | GET | `/api/agents/${id}` | `getAgentStatus` | `id` | `` |
| 242 | GET | `/api/agents/online` | `getOnlineAgents` | `` | `` |
| 243 | GET | `/api/agents/all` | `listAllAgents` | `` | `` |
| 244 | PUT | `/api/agents/${id}/status` | `updateAgentStatus` | `id, data` | `data` |
| 245 | POST | `/api/agents/${id}/online` | `goOnline` | `id` | `` |
| 246 | POST | `/api/agents/${id}/offline` | `goOffline` | `id` | `` |
| 247 | GET | `/api/agents/${id}/sessions` | `getAgentSessions` | `id` | `` |
| 248 | GET | `/api/agents/me` | `getMyAgent` | `` | `` |
| 249 | GET | `/api/quick-replies` | `getQuickReplies` | `params` | `params` |
| 250 | GET | `/api/quick-replies/categories` | `getQuickReplyCategories` | `` | `` |
| 251 | POST | `/api/quick-replies` | `createQuickReply` | `data` | `data` |
| 252 | PUT | `/api/quick-replies/${id}` | `updateQuickReply` | `id, data` | `data` |
| 253 | DELETE | `/api/quick-replies/${id}` | `deleteQuickReply` | `id` | `` |
| 254 | GET | `/api/session-tags` | `getSessionTags` | `` | `` |
| 255 | POST | `/api/session-tags` | `createSessionTag` | `data` | `data` |
| 256 | PUT | `/api/session-tags/${id}` | `updateSessionTag` | `id, data` | `data` |
| 257 | DELETE | `/api/session-tags/${id}` | `deleteSessionTag` | `id` | `` |
| 258 | GET | `/api/ai-suggestions/${sessionId}` | `getAISuggestions` | `sessionId` | `` |
| 259 | POST | `/api/ai-suggestions/${id}/use` | `useAISuggestion` | `id` | `` |
| 260 | POST | `/api/customer-sessions/${sessionId}/tags` | `tagSession` | `sessionId, data` | `data` |

## customerServiceAgent.js  (7)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 261 | GET | `/api/customer-service-agents` | `listMounts` | `params` | `params` |
| 262 | GET | `/api/customer-service-agents/by-ai-agent/${aiAgentId}` | `listMountsByAIAgent` | `aiAgentId` | `` |
| 263 | POST | `/api/customer-service-agents` | `createMount` | `data` | `data` |
| 264 | PUT | `/api/customer-service-agents/${id}` | `updateMount` | `id, data` | `data` |
| 265 | DELETE | `/api/customer-service-agents/${id}` | `deleteMount` | `id` | `` |
| 266 | GET | `/api/customer-service-agents/by-user/${userId}` | `listMountsByUser` | `userId` | `` |
| 267 | POST | `/api/customer-service-agents/by-user/${userId}` | `createMountByUser` | `userId, data` | `data` |

## customerSession.js  (18)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 268 | GET | `/api/customer-sessions` | `getSessions` | `params` | `params` |
| 269 | GET | `/api/customer-sessions/${id}/messages` | `getSessionMessages` | `id, params` | `params` |
| 270 | POST | `/api/customer-sessions/${data.sessionId}/messages` | `sendMessage` | `data` | `{` |
| 271 | POST | `/api/customer-sessions` | `createSession` | `data` | `data` |
| 272 | POST | `/api/customer-sessions/${id}/close` | `closeSession` | `id` | `` |
| 273 | POST | `/api/customer-sessions/${id}/transfer` | `transferSession` | `id, data` | `data` |
| 274 | POST | `/api/customer-sessions/${id}/takeover` | `takeoverSession` | `id, reason = ''` | `{ reason }` |
| 275 | POST | `/api/customer-sessions/${id}/release` | `releaseSession` | `id` | `` |
| 276 | POST | `/api/customer-sessions/${id}/switch-handler` | `switchSessionHandler` | `id, handlerType, reason = ''` | `{ handler_type: handlerType, reason }` |
| 277 | POST | `/api/customer-sessions/${id}/blacklist` | `blacklistSession` | `id, reason = '', ttlHours = 0` | `{ reason, ttl_hours: ttlHours }` |
| 278 | POST | `/api/customer-sessions/blacklist/remove` | `unblacklistUser` | `userId, platform = 'web'` | `{ user_id: userId, platform }` |
| 279 | GET | `/api/customer-sessions/blacklist/check` | `checkBlacklist` | `userId, platform = 'web'` | `{ params: { user_id: userId, platform } }` |
| 280 | GET | `/api/customer-sessions/blacklist` | `listBlacklist` | `params = {}` | `params` |
| 281 | GET | `/api/customer-360?user_id=${id}` | `getCustomer360` | `id` | `` |
| 282 | GET | `/api/customer-360/basic?user_id=${id}` | `getCustomerBasic` | `id` | `` |
| 283 | GET | `/api/customer-360/stats?user_id=${id}` | `getCustomerStats` | `id` | `` |
| 284 | GET | `/api/customer-360/sessions?user_id=${id}` | `getCustomerSessions` | `id` | `` |
| 285 | GET | `/api/customer-360/tags?user_id=${id}` | `getCustomerTags` | `id` | `` |

## dashboardScreen.js  (12)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 286 | GET | `/api/dashboards` | `getScreenList` | `params` | `params` |
| 287 | GET | `/api/dashboards/${id}` | `getScreenByID` | `id` | `` |
| 288 | POST | `/api/dashboards` | `createScreen` | `data` | `data` |
| 289 | PUT | `/api/dashboards/${id}` | `updateScreen` | `id, data` | `data` |
| 290 | DELETE | `/api/dashboards/${id}` | `deleteScreen` | `id` | `` |
| 291 | GET | `/api/dashboards/public/${code}` | `publicViewScreen` | `code` | `` |
| 292 | GET | `/api/dashboards/data` | `getDashboardData` | `` | `` |
| 293 | GET | `/api/dashboards/activities` | `getRealtimeActivities` | `` | `` |
| 294 | GET | `/api/dashboards/data` | `getKPIs` | `` | `` |
| 295 | GET | `/api/dashboards/data` | `getTrends` | `params` | `params` |
| 296 | GET | `/api/dashboards/data` | `getChannels` | `` | `` |
| 297 | GET | `/api/dashboards/data` | `getRegions` | `` | `` |

## dialogueMemory.js  (10)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 298 | POST | `/api/memory/messages` | `appendMessage` | `data` | `data` |
| 299 | GET | `/api/memory/short` | `getShortTerm` | `params` | `params` |
| 300 | GET | `/api/memory/long` | `getLongTerm` | `params` | `params` |
| 301 | POST | `/api/memory/facts` | `updateKeyFacts` | `data` | `data` |
| 302 | POST | `/api/memory/objections` | `recordObjection` | `data` | `data` |
| 303 | POST | `/api/memory/purchase-intent` | `updatePurchaseIntent` | `data` | `data` |
| 304 | POST | `/api/memory/intent-trail` | `recordIntent` | `data` | `data` |
| 305 | POST | `/api/memory/sop-history` | `recordSOP` | `data` | `data` |
| 306 | GET | `/api/memory/context` | `buildContext` | `params` | `params` |
| 307 | GET | `/api/memory/list` | `list` | `params` | `params` |

## dingtalkApp.js  (6)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 308 | GET | `BASE` | `listDingtalkApp` | `params` | `params` |
| 309 | GET | `${BASE}/${id}` | `getDingtalkApp` | `id` | `` |
| 310 | POST | `BASE` | `createDingtalkApp` | `data` | `data` |
| 311 | PUT | `${BASE}/${id}` | `updateDingtalkApp` | `id, data` | `data` |
| 312 | DELETE | `${BASE}/${id}` | `deleteDingtalkApp` | `id` | `` |
| 313 | POST | `${BASE}/${id}/test` | `testDingtalkApp` | `id` | `` |

## domainPool.js  (12)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 314 | GET | `/api/domain-pool/health` | `getDomainHealth` | `` | `` |
| 315 | GET | `/api/domain-pool/${domain}/blacklist` | `checkBlacklist` | `domain` | `` |
| 316 | POST | `/api/domain-pool/${id}/suspend` | `suspendDomain` | `id` | `{}` |
| 317 | POST | `/api/domain-pool/${id}/rotate` | `rotateToBackup` | `id` | `{}` |
| 318 | GET | `/api/domain-pool/alerts` | `listAlerts` | `params` | `params` |
| 319 | POST | `/api/domain-pool/alerts/${id}/resolve` | `resolveAlert` | `id` | `{}` |
| 320 | GET | `/api/domain-pool` | `resolveAlert` | `id` | `params),` |
| 321 | POST | `/api/domain-pool` | `resolveAlert` | `id` | `data),` |
| 322 | PUT | `/api/domain-pool/${data.id}` | `resolveAlert` | `id` | `data),` |
| 323 | DELETE | ``/api/domain-pool/${id}`),` | `resolveAlert` | `id` | `` |
| 324 | POST | `/api/domain-pool/${id}/check` | `resolveAlert` | `id` | `{}),` |
| 325 | POST | `/api/domain-pool/check-all` | `resolveAlert` | `id` | `{}),` |

## douyinCard.js  (9)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 326 | GET | `/api/douyin/list` | `getDouyinCardList` | `params` | `params` |
| 327 | GET | `/api/douyin/${id}` | `getDouyinCard` | `id` | `` |
| 328 | POST | `/api/douyin/create` | `createDouyinCard` | `data` | `data` |
| 329 | PUT | `/api/douyin/update` | `updateDouyinCard` | `data` | `data` |
| 330 | DELETE | `/api/douyin/delete/${id}` | `deleteDouyinCard` | `id` | `` |
| 331 | GET | `/api/douyin/view/${id}` | `viewDouyinCard` | `id` | `` |
| 332 | GET | `/api/douyin/stats/card/${id}` | `getDouyinCardStats` | `id, params` | `params` |
| 333 | GET | `/api/douyin/stats/overall` | `getDouyinCardOverallStats` | `params` | `params` |
| 334 | POST | `/api/douyin/${id}/generate-short-link` | `generateShortLink` | `id` | `` |

## email.js  (17)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 335 | GET | `/api/email/smtp` | `getEmailSmtpList` | `` | `` |
| 336 | POST | `/api/email/smtp` | `addEmailSmtp` | `data` | `data` |
| 337 | PUT | `/api/email/smtp/${id}` | `updateEmailSmtp` | `id, data` | `data` |
| 338 | DELETE | `/api/email/smtp/${id}` | `deleteEmailSmtp` | `id` | `` |
| 339 | GET | `/api/email/drafts` | `getDrafts` | `` | `` |
| 340 | GET | `/api/email/drafts/${id}` | `getDraftDetail` | `id` | `` |
| 341 | POST | `/api/email/drafts` | `createDraft` | `data` | `data` |
| 342 | PUT | `/api/email/drafts/${id}` | `updateDraft` | `id, data` | `data` |
| 343 | DELETE | `/api/email/drafts/${id}` | `deleteDraft` | `id` | `` |
| 344 | POST | `/api/upload` | `uploadImage` | `formData` | `formData` |
| | | | | opts | `{` |
| 345 | POST | `/api/email/send` | `sendEmail` | `data` | `data` |
| 346 | GET | `/api/email/list?page=${page}&limit=${limit}` | `sendEmail` | `data` | `` |
| 347 | GET | `/api/email/jobs?page=${page}&limit=${limit}` | `getJobsList` | `page, limit` | `` |
| 348 | DELETE | `/api/email/jobs/${id}` | `deleteJob` | `id` | `` |
| 349 | GET | `/api/email/jobs/${id}` | `getJobDetail` | `id` | `` |
| 350 | GET | `/api/email/lists/${id}/tracking` | `getEmailTrace` | `id` | `` |
| 351 | DELETE | `/api/email/list/${id}` | `deleteEmailList` | `id` | `` |

## faq.js  (8)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 352 | GET | `/api/faqs` | `list` | `params` | `params` |
| 353 | GET | `/api/faqs/${id}` | `get` | `id` | `` |
| 354 | POST | `/api/faqs` | `create` | `data` | `data` |
| 355 | PUT | `/api/faqs/${id}` | `update` | `id, data` | `data` |
| 356 | PATCH | `/api/faqs/${id}/toggle` | `toggle` | `id, enabled` | `{ enabled }` |
| 357 | DELETE | `/api/faqs/${id}` | `remove` | `id` | `` |
| 358 | POST | `/api/faqs/match` | `match` | `data` | `data` |
| 359 | GET | `/api/faqs/stats` | `stats` | `` | `` |

## featureFlags.js  (12)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 360 | GET | `/api/feature-flags` | `list` | `params = {}` | `{ params }` |
| 361 | GET | `/api/feature-flags/stale` | `stale` | `` | `` |
| 362 | GET | `/api/feature-flags/${id}` | `detail` | `id` | `` |
| 363 | POST | `/api/feature-flags` | `create` | `data` | `data` |
| 364 | PUT | `/api/feature-flags/${id}` | `update` | `id, data` | `data` |
| 365 | DELETE | `/api/feature-flags/${id}` | `remove` | `id` | `` |
| 366 | POST | `/api/feature-flags/${id}/enable` | `enable` | `id` | `` |
| 367 | POST | `/api/feature-flags/${id}/disable` | `disable` | `id` | `` |
| 368 | POST | `/api/feature-flags/${id}/rollout` | `setRollout` | `id, percentage` | `{ percentage }` |
| 369 | GET | `/api/feature-flags/${id}/audit` | `auditLogs` | `id` | `` |
| 370 | GET | `/api/feature-flags/${id}/eval-log` | `evalLogs` | `id` | `` |
| 371 | GET | `/api/feature-flags/${id}/code-references` | `codeReferences` | `id` | `` |

## feishu.js  (7)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 372 | GET | `/api/feishu/accounts` | `listAccounts` | `params = {}` | `params` |
| 373 | GET | `/api/feishu/accounts/${id}` | `getAccount` | `id` | `` |
| 374 | POST | `/api/feishu/accounts` | `createAccount` | `data` | `data` |
| 375 | PUT | `/api/feishu/accounts/${id}` | `updateAccount` | `id, data` | `data` |
| 376 | DELETE | `/api/feishu/accounts/${id}` | `deleteAccount` | `id` | `` |
| 377 | POST | `/api/feishu/accounts/${id}/test-send` | `testSend` | `id, data` | `data` |
| 378 | POST | `/api/feishu/accounts/${id}/refresh-token` | `refreshToken` | `id` | `` |

## followup.js  (5)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 379 | GET | `${base}/today` | `today` | `ownerId, date` | `params` |
| | | | | opts | `SILENT` |
| 380 | GET | `${base}/pending` | `pending` | `ownerId, limit` | `params` |
| | | | | opts | `SILENT` |
| 381 | GET | `${base}/overdue` | `overdue` | `ownerId` | `{ owner_id: ownerId }` |
| | | | | opts | `SILENT` |
| 382 | POST | `${base}/${seg(id)}/complete` | `complete` | `id, result, note` | `{ result: result \|\| '', note: note \|\| '' }` |
| 383 | POST | `${base}/${seg(id)}/cancel` | `cancel` | `id` | `` |

## geo.js  (48)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 384 | POST | `/api/geo/keywords/mine` | `mineKeywords` | `data` | `data` |
| 385 | POST | `/api/geo/keywords/expand` | `semanticExpand` | `data` | `data` |
| 386 | POST | `/api/geo/keywords/cluster` | `topicCluster` | `data` | `data` |
| 387 | GET | `/api/geo/keywords/list` | `getKeywordList` | `params` | `params` |
| 388 | POST | `/api/geo/keyword-mining/crawl-suggest` | `crawlSuggest` | `data` | `data` |
| 389 | POST | `/api/geo/keyword-mining/longtail` | `combineLongtail` | `data` | `data` |
| 390 | GET | `/api/geo/keyword-mining/funnel` | `getKeywordFunnel` | `` | `` |
| 391 | POST | `/api/geo/push/urls` | `pushUrls` | `data` | `data` |
| 392 | GET | `/api/geo/push/quota` | `getPushQuota` | `` | `` |
| 393 | GET | `/api/geo/push/sitemap` | `getSitemapPreview` | `` | `` |
| 394 | POST | `/api/geo/index-tracking/verify/${articleId}` | `verifyIndexFull` | `articleId` | `` |
| 395 | GET | `/api/geo/index-tracking/funnel` | `getIndexFunnel` | `` | `` |
| 396 | POST | `/api/geo/index-tracking/verify-all` | `verifyAllIndex` | `` | `` |
| 397 | POST | `/api/geo/site/export` | `exportSite` | `` | `` |
| 398 | POST | `/api/geo/site/deploy` | `deploySite` | `` | `` |
| 399 | POST | `/api/geo/site/full-pipeline` | `siteFullPipeline` | `` | `` |
| 400 | GET | `/api/geo/site/llms-txt-preview` | `getLlmsTxtPreview` | `domain` | `domain ? { domain } : {}` |
| 401 | GET | `/api/geo/site/robots-preview` | `getRobotsTxtPreview` | `` | `` |
| 402 | GET | `/api/geo/sites` | `listSites` | `params` | `params` |
| 403 | POST | `/api/geo/sites` | `createSite` | `data` | `data` |
| 404 | PUT | `/api/geo/sites/${id}` | `updateSite` | `id, data` | `data` |
| 405 | DELETE | `/api/geo/sites/${id}` | `deleteSite` | `id` | `` |
| 406 | GET | `/api/geo/pushers` | `listPushers` | `params` | `params` |
| 407 | POST | `/api/geo/pushers` | `createPusher` | `data` | `data` |
| 408 | PUT | `/api/geo/pushers/${id}` | `updatePusher` | `id, data` | `data` |
| 409 | DELETE | `/api/geo/pushers/${id}` | `deletePusher` | `id` | `` |
| 410 | GET | `/api/geo/schema-templates` | `listSchemaTemplates` | `params` | `params` |
| 411 | POST | `/api/geo/schema-templates` | `createSchemaTemplate` | `data` | `data` |
| 412 | PUT | `/api/geo/schema-templates/${id}` | `updateSchemaTemplate` | `id, data` | `data` |
| 413 | DELETE | `/api/geo/schema-templates/${id}` | `deleteSchemaTemplate` | `id` | `` |
| 414 | DELETE | `/api/geo/keywords/${id}` | `deleteKeyword` | `id` | `` |
| 415 | POST | `/api/geo/content/generate` | `generateContent` | `data` | `data` |
| 416 | POST | `/api/geo/content/score` | `scoreContent` | `data` | `data` |
| 417 | POST | `/api/geo/content/optimize` | `optimizeContent` | `data` | `data` |
| 418 | POST | `/api/geo/content/eeat` | `enhanceEEAT` | `data` | `data` |
| 419 | POST | `/api/geo/content/schema` | `generateSchema` | `data` | `data` |
| 420 | POST | `/api/geo/content/uniqueness` | `checkUniqueness` | `data` | `data` |
| 421 | GET | `/api/geo/content/list` | `getArticleList` | `params` | `params` |
| 422 | GET | `/api/geo/content/${id}` | `getArticleByID` | `id` | `` |
| 423 | POST | `/api/geo/verification/verify` | `verifyArticle` | `data` | `data` |
| 424 | POST | `/api/geo/verification/negative` | `monitorNegative` | `data` | `data` |
| 425 | GET | `/api/geo/verification/results/${articleId}` | `getVerifyResults` | `articleId` | `` |
| 426 | GET | `/api/geo/reports/summary` | `getReport` | `params` | `params` |
| 427 | GET | `/api/geo/reports/roi` | `getROI` | `params` | `params` |
| 428 | GET | `/api/geo/reports/api-costs` | `getAPICosts` | `params` | `params` |
| 429 | GET | `/api/geo/config` | `getConfig` | `` | `` |
| 430 | PUT | `/api/geo/config` | `updateConfig` | `data` | `data` |
| 431 | POST | `/api/geo/config/optimize` | `optimizeConfig` | `data` | `data` |

## geoAlert.js  (4)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 432 | GET | `/api/geo/alerts` | `listGeoAlerts` | `params` | `params` |
| 433 | GET | `/api/geo/alerts/unread-count` | `getGeoAlertsUnreadCount` | `` | `` |
| 434 | POST | `/api/geo/alerts/${id}/ack` | `ackGeoAlert` | `id` | `{}` |
| 435 | DELETE | `/api/geo/alerts/${id}` | `deleteGeoAlert` | `id` | `` |

## geoDecision.js  (3)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 436 | GET | `/api/geo/decision/report` | `getGeoDecisionReport` | `` | `` |
| 437 | GET | `/api/geo/decision/tasks` | `getGeoGapTasks` | `limit = 50` | `{ limit }` |
| 438 | POST | `/api/geo/decision/tasks/${id}/done` | `markGeoTaskDone` | `id` | `` |

## geoEntity.js  (18)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 439 | GET | `/api/geo/entity/list` | `listEntities` | `type, keyword` | `{ type: type \|\| '', keyword: keyword \|\| '' }` |
| 440 | GET | `/api/geo/entity/${entityId}/graph` | `getEntityRelations` | `entityId` | `` |
| 441 | POST | `/api/geo/entities/extract` | `extractEntities` | `docId` | `{ doc_id: docId }` |
| 442 | GET | `/api/geo/kb/documents` | `listKBDocuments` | `keyword, sourceLevel` | `{ keyword: keyword \|\| '', source_level: sourceLevel \|\| '' }` |
| 443 | POST | `/api/geo/kb/documents` | `saveKBDocument` | `data` | `data` |
| 444 | DELETE | `/api/geo/kb/documents/${id}` | `deleteKBDocument` | `id` | `` |
| 445 | GET | `/api/geo/kb/documents/${id}` | `getKBDocument` | `id` | `` |
| 446 | GET | `/api/geo/kb/search` | `searchKB` | `q, limit = 10` | `{ q, limit }` |
| 447 | POST | `/api/geo/kb/ask` | `askKB` | `question` | `{ question }` |
| 448 | GET | `/api/geo/workflow/workflows` | `listWorkflows` | `` | `` |
| 449 | GET | `/api/geo/workflow/workflows/${id}` | `getWorkflow` | `id` | `` |
| 450 | POST | `/api/geo/workflow/workflows` | `saveWorkflow` | `data` | `data` |
| 451 | PUT | `/api/geo/workflow/workflows/${id}` | `updateWorkflow` | `id, data` | `data` |
| 452 | DELETE | `/api/geo/workflow/workflows/${id}` | `deleteWorkflow` | `id` | `` |
| 453 | POST | `/api/geo/workflow/workflows/${id}/run` | `runWorkflow` | `id, params` | `params \|\| {}` |
| 454 | GET | `/api/geo/workflow/workflows/${workflowId}/executions` | `listWorkflowExecutions` | `workflowId` | `` |
| 455 | GET | `/api/geo/workflow/templates` | `listWorkflowTemplates` | `` | `` |
| 456 | POST | `/api/geo/workflow/templates` | `saveWorkflowTemplate` | `data` | `data` |

## geoPlatform.js  (6)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 457 | GET | `/api/geo/platform/platforms` | `listPlatforms` | `` | `` |
| 458 | GET | `/api/geo/platform/accounts` | `listPlatformAccounts` | `` | `` |
| 459 | POST | `/api/geo/platform/accounts` | `savePlatformAccount` | `data` | `data` |
| 460 | DELETE | `/api/geo/platform/accounts/${id}` | `deletePlatformAccount` | `id` | `` |
| 461 | POST | `/api/geo/platform/publish` | `publishToPlatform` | `articleId, platform, opts = {}` | `{ article_id: articleId, platform, ...opts }` |
| 462 | GET | `/api/geo/platform/records` | `listPlatformPublishRecords` | `articleId` | `{ article_id: articleId \|\| '' }` |

## geoProbe.js  (18)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 463 | GET | `/api/geo/probe/engines` | `listProbeEngines` | `` | `` |
| 464 | POST | `/api/geo/probe/test` | `testEngineProbe` | `engine, query` | `{ engine, query }` |
| 465 | GET | `/api/geo/probe/runs` | `listProbeRuns` | `engine, limit = 100` | `{ engine, limit }` |
| 466 | POST | `/api/geo/probe/run-negative` | `runNegativeMonitor` | `` | `` |
| 467 | POST | `/api/geo/probe/run-source-sync` | `runSourceCatalogSync` | `` | `` |
| 468 | POST | `/api/geo/probe/all` | `probeAllEngines` | `engines, query` | `{ engines, query }` |
| 469 | GET | `/api/geo/sov` | `getSOV` | `` | `` |
| 470 | GET | `/api/geo/crawler-stats` | `getCrawlerStats` | `` | `` |
| 471 | POST | `/api/geo/crawler/run` | `runCrawler` | `` | `` |
| 472 | POST | `/api/geo/inaccurate-claims` | `detectInaccurateClaims` | `brandName` | `{ brand_name: brandName }` |
| 473 | GET | `/api/geo/source-catalog/levels` | `lookupSourceLevel` | `url` | `{ url }` |
| 474 | GET | `/api/geo/competitors` | `listCompetitors` | `` | `` |
| 475 | GET | `/api/geo/competitors/${id}` | `getCompetitor` | `id` | `` |
| 476 | POST | `/api/geo/competitors` | `createCompetitor` | `data` | `data` |
| 477 | PUT | `/api/geo/competitors/${id}` | `updateCompetitor` | `id, data` | `data` |
| 478 | DELETE | `/api/geo/competitors/${id}` | `deleteCompetitor` | `id` | `` |
| 479 | GET | `/api/geo/visibility/engine-compare` | `getEngineCompare` | `days = 30, intent = ''` | `{ days, intent: intent \|\| '' }` |
| 480 | GET | `/api/geo/sov/trend` | `getSOVTrend` | `days = 30, intent = ''` | `{ days, intent: intent \|\| '' }` |

## glossary.js  (6)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 481 | POST | `/api/glossaries` | `createGlossary` | `data` | `data` |
| 482 | GET | `/api/glossaries` | `listGlossaries` | `params` | `params` |
| 483 | GET | `/api/glossaries/${termId}` | `getGlossary` | `termId` | `` |
| 484 | PUT | `/api/glossaries/${termId}` | `updateGlossary` | `termId, data` | `data` |
| 485 | DELETE | `/api/glossaries/${termId}` | `deleteGlossary` | `termId` | `` |
| 486 | POST | `/api/glossaries/validate` | `validateGlossary` | `data` | `data` |

## humanTask.js  (7)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 487 | GET | `base` | `list` | `params` | `params` |
| | | | | opts | `{ paramsSerializer: toQuery }` |
| 488 | GET | `${base}/counts` | `counts` | `` | `undefined` |
| | | | | opts | `{ _silent: true }` |
| 489 | GET | `${base}/${seg(id)}` | `get` | `id` | `` |
| 490 | POST | `${base}/${seg(id)}/claim` | `claim` | `id` | `` |
| 491 | POST | `${base}/${seg(id)}/release` | `release` | `id` | `` |
| 492 | POST | `${base}/${seg(id)}/complete` | `complete` | `id` | `` |
| 493 | POST | `${base}/${seg(id)}/cancel` | `cancel` | `id, reason` | `{ reason }` |

## i18nStats.js  (6)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 494 | GET | `/api/i18n/stats` | `getI18nStats` | `` | `` |
| 495 | GET | `/api/i18n/stats/lang-dist` | `getLangDistribution` | `days = 7` | `{ params: { days } }` |
| 496 | GET | `/api/i18n/stats/cache` | `getCacheHitRate` | `days = 7` | `{ params: { days } }` |
| 497 | GET | `/api/i18n/stats/glossary` | `getGlossaryCoverage` | `` | `` |
| 498 | GET | `/api/i18n/stats/quality` | `getQualityTrend` | `days = 30` | `{ params: { days } }` |
| 499 | GET | `/api/i18n/stats/latency` | `getLatencyStats` | `days = 7` | `{ params: { days } }` |

## inbox.js  (17)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 500 | GET | `/api/inbox/conversations` | `listConversations` | `params` | `params` |
| 501 | GET | `/api/inbox/conversations/${id}` | `getConversation` | `id` | `` |
| 502 | GET | `/api/inbox/conversations/${id}/messages` | `getMessages` | `id, params` | `params` |
| 503 | POST | `/api/inbox/conversations/${id}/read` | `markRead` | `id` | `` |
| 504 | POST | `/api/inbox/conversations/${id}/pin` | `setPin` | `id, pinned` | `{ pinned }` |
| 505 | POST | `/api/inbox/conversations/${id}/star` | `setStar` | `id, starred` | `{ starred }` |
| 506 | POST | `/api/inbox/conversations/${id}/mute` | `setMute` | `id, muted` | `{ muted }` |
| 507 | POST | `/api/inbox/conversations/${id}/tags` | `addTag` | `id, tag` | `{ tag }` |
| 508 | DELETE | `/api/inbox/conversations/${id}/tags/${encodeURIComponent(tag)}` | `removeTag` | `id, tag` | `` |
| 509 | DELETE | `/api/inbox/conversations/${id}/messages/${mid}?source=${source}` | `deleteMessage` | `id, mid, source` | `` |
| 510 | POST | `/api/inbox/assign` | `assign` | `data` | `data` |
| 511 | POST | `/api/inbox/assign/auto` | `autoAssign` | `data` | `data` |
| 512 | GET | `/api/inbox/assignments` | `listAssignments` | `params` | `params` |
| 513 | GET | `/api/inbox/stats` | `getStats` | `` | `` |
| 514 | POST | `/api/inbox/reconcile?mode=${mode}` | `reconcile` | `mode` | `` |
| 515 | GET | `/api/inbox/staff/${encodeURIComponent(staff)}/load` | `staffLoad` | `staff` | `` |
| 516 | GET | `/api/inbox/agents` | `listAgents` | `` | `` |

## integration.js  (13)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 517 | GET | `/api/integrations` | `getIntegrationAccountList` | `params` | `params` |
| 518 | GET | `/api/integrations/${id}` | `getIntegrationAccount` | `id` | `` |
| 519 | POST | `/api/integrations` | `createIntegrationAccount` | `data` | `data` |
| 520 | PUT | `/api/integrations/${id}` | `updateIntegrationAccount` | `id, data` | `data` |
| 521 | DELETE | `/api/integrations/${id}` | `deleteIntegrationAccount` | `id` | `` |
| 522 | POST | `/api/integrations/${id}/sync-customers` | `syncCustomers` | `id` | `` |
| 523 | POST | `/api/integrations/${id}/sync-products` | `syncProducts` | `id` | `` |
| 524 | GET | `/api/integration/sync-logs` | `getSyncLogs` | `params` | `params` |
| 525 | GET | `/api/integration/external-customers` | `getExternalCustomers` | `` | `` |
| 526 | GET | `/api/integration/external-orders` | `getExternalOrders` | `` | `` |
| 527 | GET | `/api/integration/external-orders-by-customer` | `getExternalOrdersByCustomer` | `phone, name` | `{ params: { phone, name } }` |
| 528 | GET | `/api/integration/external-products` | `getExternalProducts` | `` | `` |
| 529 | POST | `/api/integrations/${id}/test` | `testIntegration` | `id` | `` |

## intentRecognition.js  (12)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 530 | POST | `/api/intent/recognize` | `recognize` | `data` | `data` |
| 531 | POST | `/api/intent/recognize/batch` | `batchRecognize` | `data` | `data` |
| 532 | GET | `/api/intent/stats` | `getStats` | `params` | `params` |
| 533 | GET | `/api/intent/recent` | `getRecent` | `params` | `params` |
| 534 | GET | `/api/intent/dict` | `getDict` | `` | `` |
| 535 | POST | `/api/intent/recognize/fine` | `recognizeFine` | `data` | `data` |
| 536 | GET | `/api/intent/logs` | `getLogs` | `params` | `params` |
| 537 | GET | `/api/intent/stats/fine` | `getStatsFine` | `params` | `params` |
| 538 | GET | `/api/intent/config` | `getConfig` | `` | `` |
| 539 | PUT | `/api/intent/config` | `updateConfig` | `data` | `data` |
| 540 | GET | `/api/intent-records/keywords-override` | `getKeywordOverride` | `` | `` |
| 541 | PUT | `/api/intent-records/keywords-override` | `updateKeywordOverride` | `data` | `data` |

## kbConnector.js  (6)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 542 | GET | `'/api/kb-connectors'),` | `?` | `` | `` |
| 543 | POST | `/api/kb-connectors` | `?` | `` | `data),` |
| 544 | PUT | `/api/kb-connectors/${id}` | `?` | `` | `data),` |
| 545 | DELETE | ``/api/kb-connectors/${id}`),` | `?` | `` | `` |
| 546 | POST | `/api/kb-connectors/${id}/sync` | `?` | `` | `` |
| 547 | GET | `/api/capabilities` | `?` | `` | `` |

## knowledge.js  (27)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 548 | POST | `/api/knowledge/import/upload` | `importUpload` | `formData` | `formData` |
| | | | | opts | `{` |
| 549 | POST | `/api/knowledge/import/text` | `importText` | `data` | `data` |
| 550 | POST | `/api/knowledge/import/url` | `importURL` | `data` | `data` |
| 551 | GET | `/api/knowledge/documents` | `listDocuments` | `params` | `{ params }` |
| 552 | GET | `/api/knowledge/documents/${id}` | `getDocument` | `id, params` | `{ params }` |
| 553 | GET | `/api/knowledge/documents/${id}/progress` | `getDocumentProgress` | `id` | `` |
| 554 | GET | `/api/knowledge/documents/${id}/chunks` | `getDocumentChunks` | `id` | `` |
| 555 | PUT | `/api/knowledge/documents/${id}` | `updateDocument` | `id, data` | `data` |
| 556 | DELETE | `/api/knowledge/documents/${id}` | `deleteDocument` | `id, params` | `{ params }` |
| 557 | POST | `/api/knowledge/documents/${id}/reindex` | `reindexDocument` | `id, params` | `null` |
| | | | | opts | `{ params }` |
| 558 | POST | `/api/knowledge/products/${productId}/rebuild-index` | `rebuildProductIndex` | `productId` | `` |
| 559 | GET | `/api/knowledge/products/${productId}/overview` | `getProductOverview` | `productId` | `` |
| 560 | POST | `/api/knowledge/search` | `search` | `data` | `data` |
| 561 | GET | `/api/knowledge/import-logs` | `listImportLogs` | `params` | `{ params }` |
| 562 | GET | `/api/knowledge/openapi/sources` | `listOpenAPISources` | `params` | `{ params }` |
| 563 | POST | `/api/knowledge/openapi/sources` | `createOpenAPISource` | `data` | `data` |
| 564 | GET | `/api/knowledge/openapi/sources/${id}` | `getOpenAPISource` | `id, params` | `{ params }` |
| 565 | PUT | `/api/knowledge/openapi/sources/${id}` | `updateOpenAPISource` | `id, data` | `data` |
| 566 | DELETE | `/api/knowledge/openapi/sources/${id}` | `deleteOpenAPISource` | `id, params` | `{ params }` |
| 567 | POST | `/api/knowledge/openapi/sources/${id}/sync` | `syncOpenAPISource` | `id, params` | `null` |
| | | | | opts | `{ params }` |
| 568 | POST | `/api/knowledge/openapi/sources/${id}/test` | `testOpenAPISource` | `id, data` | `data` |
| 569 | POST | `/api/knowledge/openapi/sources/${id}/toggle` | `toggleOpenAPISource` | `id, data, params` | `data` |
| | | | | opts | `{ params }` |
| 570 | GET | `/api/knowledge/stats/overview` | `getOverviewStats` | `params` | `{ params }` |
| 571 | GET | `/api/knowledge/stats/documents` | `getDocumentStats` | `params` | `{ params }` |
| 572 | GET | `/api/knowledge/stats/searches` | `getSearchStats` | `params` | `{ params }` |
| 573 | GET | `/api/knowledge/stats/imports` | `getImportStats` | `params` | `{ params }` |
| 574 | GET | `/api/knowledge/stats/openapi` | `getOpenAPIStats` | `params` | `{ params }` |

## knowledgeBase.js  (7)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 575 | GET | `/api/knowledge-bases` | `listKBs` | `params` | `params` |
| 576 | GET | `/api/knowledge-bases/by-type/${kbType}` | `listByType` | `kbType, params = {}` | `params` |
| 577 | GET | `/api/knowledge-bases/${id}` | `getKB` | `id` | `` |
| 578 | POST | `/api/knowledge-bases` | `createKB` | `data` | `data` |
| 579 | PUT | `/api/knowledge-bases/${id}` | `updateKB` | `id, data` | `data` |
| 580 | DELETE | `/api/knowledge-bases/${id}` | `deleteKB` | `id` | `` |
| 581 | GET | `/api/knowledge-bases/${id}/stats` | `getKBStats` | `id` | `` |

## knowledgeMerchant.js  (14)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 582 | POST | `/api/knowledge-merchant/batch/import` | `batchImport` | `data` | `data` |
| 583 | POST | `/api/knowledge-merchant/batch/upload` | `batchUpload` | `formData` | `formData` |
| | | | | opts | `{` |
| 584 | POST | `/api/knowledge-merchant/playground` | `playground` | `data` | `data` |
| 585 | GET | `/api/knowledge-merchant/documents/${documentId}/chunks` | `listDocumentChunks` | `documentId, params` | `{ params }` |
| 586 | PUT | `/api/knowledge-merchant/chunks/${chunkId}` | `updateChunk` | `chunkId, data` | `data` |
| 587 | DELETE | `/api/knowledge-merchant/chunks/${chunkId}` | `deleteChunk` | `chunkId` | `` |
| 588 | POST | `/api/knowledge-merchant/chunks/${chunkId}/split` | `splitChunk` | `chunkId, data` | `data` |
| 589 | POST | `/api/knowledge-merchant/feedback` | `submitFeedback` | `data` | `data` |
| 590 | GET | `/api/knowledge-merchant/feedbacks` | `listFeedbacks` | `params` | `{ params }` |
| 591 | POST | `/api/knowledge-merchant/tokens` | `createToken` | `data` | `data` |
| 592 | GET | `/api/knowledge-merchant/tokens` | `listTokens` | `params` | `{ params }` |
| 593 | POST | `/api/knowledge-merchant/tokens/${tokenId}/revoke` | `revokeToken` | `tokenId` | `` |
| 594 | POST | `/api/knowledge-merchant/external/import` | `externalImport` | `data, token` | `data` |
| | | | | opts | `{` |
| 595 | GET | `/api/knowledge-merchant/external/jobs` | `listExternalJobs` | `params` | `{ params }` |

## kuaishouCard.js  (11)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 596 | GET | `/api/kuaishou/list` | `getKuaishouCardList` | `params` | `params` |
| 597 | GET | `/api/kuaishou/${id}` | `getKuaishouCard` | `id` | `` |
| 598 | POST | `/api/kuaishou/create` | `createKuaishouCard` | `data` | `data` |
| 599 | PUT | `/api/kuaishou/update` | `updateKuaishouCard` | `data` | `data` |
| 600 | DELETE | `/api/kuaishou/delete/${id}` | `deleteKuaishouCard` | `id` | `` |
| 601 | GET | `/api/kuaishou/view/${id}` | `viewKuaishouCard` | `id` | `` |
| 602 | POST | `/api/kuaishou/like/${id}` | `likeKuaishouCard` | `id` | `` |
| 603 | POST | `/api/kuaishou/share/${id}` | `shareKuaishouCard` | `id` | `` |
| 604 | GET | `/api/kuaishou/stats/card/${id}` | `getKuaishouCardStats` | `id, params` | `params` |
| 605 | GET | `/api/kuaishou/stats/overall` | `getKuaishouCardOverallStats` | `params` | `params` |
| 606 | POST | `/api/kuaishou/${id}/generate-short-link` | `generateShortLink` | `id` | `` |

## leadMining.js  (2)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 607 | GET | `/api/lead-mining/config` | `getLeadMiningConfig` | `` | `` |
| 608 | POST | `/api/lead-mining/config` | `saveLeadMiningConfig` | `data` | `data` |

## livecode.js  (12)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 609 | GET | `/api/live-codes/list` | `getLiveCodes` | `params` | `params` |
| 610 | GET | `/api/live-codes/${id}` | `getLiveCode` | `id` | `` |
| 611 | POST | `/api/live-codes/create` | `createLiveCode` | `data` | `data` |
| 612 | PUT | `/api/live-codes/${id}/update` | `updateLiveCode` | `id, data` | `data` |
| 613 | DELETE | `/api/live-codes/${id}/delete` | `deleteLiveCode` | `id` | `` |
| 614 | GET | `/api/live-codes/${id}/stats` | `getLiveCodeStats` | `id` | `` |
| 615 | GET | `/api/live-codes/${liveCodeId}/qrcodes` | `getLiveCodeQRs` | `liveCodeId` | `` |
| 616 | POST | `/api/live-codes/${liveCodeId}/qrcodes/create` | `generateLiveCodeQR` | `liveCodeId, data` | `data` |
| 617 | GET | `/api/live-codes/qrcodes/${qrId}/stats` | `getLiveCodeQRStats` | `qrId` | `` |
| 618 | POST | `/api/live-codes/${id}/share` | `shareLiveCode` | `id, data` | `data` |
| 619 | DELETE | `/api/live-codes/qrcodes/${id}/delete` | `deleteLiveCodeQR` | `id` | `` |
| 620 | PUT | `/api/live-codes/qrcodes/${id}/update` | `updateLiveCodeQR` | `id, data` | `data` |

## llmRouting.js  (19)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 621 | GET | `/api/llm/models` | `?` | `` | `params` |
| 622 | GET | `/api/llm/models/${encodeURIComponent(name)}` | `?` | `` | `` |
| 623 | POST | `/api/llm/models` | `?` | `` | `data` |
| 624 | PUT | `/api/llm/models/${encodeURIComponent(name)}` | `?` | `` | `data` |
| 625 | DELETE | `/api/llm/models/${encodeURIComponent(name)}` | `?` | `` | `` |
| 626 | PUT | `/api/llm/models/${encodeURIComponent(name)}` | `?` | `` | `{ enabled: status }` |
| 627 | POST | `/api/llm/models/${encodeURIComponent(name)}/test` | `?` | `` | `body` |
| 628 | GET | `/api/llm/scene-routing` | `?` | `` | `` |
| 629 | GET | `/api/llm/scenarios` | `?` | `` | `` |
| 630 | PUT | `/api/llm/scene-routing` | `?` | `` | `payload` |
| 631 | GET | `/api/llm/fallback` | `?` | `` | `` |
| 632 | GET | `/api/llm/audit` | `?` | `` | `params` |
| 633 | GET | `/api/llm/stats` | `?` | `` | `` |
| 634 | GET | `/api/llm/usage` | `?` | `` | `{ window }` |
| 635 | GET | `/api/llm/cost-stats` | `?` | `` | `{ window }` |
| 636 | GET | `/api/llm/health` | `?` | `` | `` |
| 637 | GET | `/api/llm/model-type-stats` | `?` | `` | `{ window }` |
| 638 | GET | `/api/llm/egress-audit` | `?` | `` | `params` |
| 639 | GET | `/api/llm/egress-alerts` | `?` | `` | `params` |

## ltcConfig.js  (2)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 640 | GET | `/api/manage/ltc/config` | `getLTCConfig` | `` | `` |
| 641 | PUT | `/api/manage/ltc/config` | `saveLTCConfig` | `cfg` | `cfg` |

## ltcRates.js  (1)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 642 | GET | `/api/ltc-rates` | `getRates` | `` | `` |

## marketingFlow.js  (10)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 643 | GET | `/api/marketing-flows` | `getMarketingFlowList` | `params` | `params` |
| 644 | GET | `/api/marketing-flows/${id}` | `getMarketingFlow` | `id` | `` |
| 645 | POST | `/api/marketing-flows` | `createMarketingFlow` | `data` | `data` |
| 646 | PUT | `/api/marketing-flows/${id}` | `updateMarketingFlow` | `id, data` | `data` |
| 647 | DELETE | `/api/marketing-flows/${id}` | `deleteMarketingFlow` | `id` | `` |
| 648 | POST | `/api/marketing-flows/${id}/activate` | `activateFlow` | `id` | `` |
| 649 | POST | `/api/marketing-flows/${id}/pause` | `pauseMarketingFlow` | `id` | `` |
| 650 | POST | `/api/marketing-flows/${id}/stop` | `stopMarketingFlow` | `id` | `` |
| 651 | GET | `/api/marketing-flows/${id}/executions` | `getFlowExecutions` | `id` | `` |
| 652 | GET | `/api/marketing-flows/${id}/stats` | `getFlowStats` | `id` | `` |

## material.js  (10)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 653 | GET | `/api/material/list` | `getMaterialList` | `params = {}` | `params` |
| 654 | UPLOAD | `/api/material/upload` | `uploadMaterial` | `data` | `data` |
| 655 | DELETE | `/api/material/${id}` | `deleteMaterial` | `id` | `` |
| 656 | GET | `/api/material/categories` | `getMaterialCategories` | `` | `` |
| 657 | POST | `/api/material/categories` | `createMaterialCategory` | `data` | `data` |
| 658 | PUT | `/api/material/categories/${id}` | `updateMaterialCategory` | `id, data` | `data` |
| 659 | DELETE | `/api/material/categories/${id}` | `deleteMaterialCategory` | `id` | `` |
| 660 | GET | `/api/material/selector` | `getMaterialSelector` | `params = {}` | `params` |
| 661 | POST | `/api/material/${id}/usage` | `updateMaterialUsage` | `id` | `` |
| 662 | GET | `/api/material/stats` | `getMaterialStats` | `` | `` |

## mcpCredential.js  (4)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 663 | GET | `'/api/mcp/credentials'),` | `?` | `` | `` |
| 664 | POST | `/api/mcp/credentials` | `?` | `` | `data),` |
| 665 | PUT | `/api/mcp/credentials/${id}` | `?` | `` | `data),` |
| 666 | DELETE | `/api/mcp/credentials/${id}` | `?` | `` | `` |

## menu.js  (4)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 667 | GET | `/api/menus` | `getUserMenus` | `` | `` |
| 668 | GET | `/api/system/menus` | `getAllMenus` | `` | `` |
| 669 | GET | `/api/system/roles/${code}/menus` | `getRoleMenuKeys` | `code` | `` |
| 670 | PUT | `/api/system/roles/${code}/menus` | `updateRoleMenuKeys` | `code, menuKeys` | `{ menu_keys: menuKeys }` |

## messageHub.js  (8)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 671 | POST | `/api/message-hub/push` | `pushMessage` | `data` | `data` |
| 672 | POST | `/api/message-hub/push-batch` | `pushBatch` | `data` | `data` |
| 673 | POST | `/api/message-hub/push-from-channel` | `pushFromChannel` | `data` | `data` |
| 674 | GET | `/api/message-hub/list` | `getMessages` | `params` | `params` |
| 675 | GET | `/api/message-hub/${id}` | `getMessageById` | `id` | `` |
| 676 | POST | `/api/message-hub/${ids[0]}/read` | `markRead` | `ids` | `{ ids }` |
| 677 | GET | `/api/message-hub/stats` | `getStats` | `params` | `params` |
| 678 | GET | `/api/message-hub/platforms` | `getPlatforms` | `` | `` |

## moduleMarket.js  (16)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 679 | GET | `/api/module-market/list` | `listMarketModules` | `params, config` | `params` |
| | | | | opts | `config` |
| 680 | GET | `/api/module-market/detail/${encodeURIComponent(key)}` | `marketModuleDetail` | `key` | `` |
| 681 | GET | `/api/module-market/payment-guide` | `paymentGuide` | `config` | `undefined` |
| | | | | opts | `config` |
| 682 | GET | `/api/tenant/regulatory-notice` | `regulatoryNotice` | `` | `` |
| 683 | POST | `/api/module-market/order` | `createModuleOrder` | `data` | `data` |
| 684 | GET | `/api/module-market/orders` | `listModuleOrders` | `params, config` | `params` |
| | | | | opts | `config` |
| 685 | GET | `/api/module-market/orders/${encodeURIComponent(orderNo)}` | `moduleOrderDetail` | `orderNo` | `` |
| 686 | GET | `/api/module-market/orders/${encodeURIComponent(orderNo)}/pay-status` | `orderPayStatus` | `orderNo, config` | `undefined` |
| | | | | opts | `config` |
| 687 | GET | `/api/subscription/status` | `subscriptionStatus` | `config` | `undefined` |
| | | | | opts | `config` |
| 688 | POST | `/api/subscription/renew` | `renewSubscription` | `data` | `data` |
| 689 | GET | `/api/modules/my` | `myModules` | `` | `` |
| 690 | POST | `/api/modules/sync` | `syncModules` | `config` | `undefined` |
| | | | | opts | `config` |
| 691 | GET | `/api/modules/sync-logs` | `moduleSyncLogs` | `params, config` | `params` |
| | | | | opts | `config` |
| 692 | GET | `/api/llm/wallet` | `llmWallet` | `config` | `undefined` |
| | | | | opts | `config` |
| 693 | GET | `/api/llm/plans` | `llmPlans` | `config` | `undefined` |
| | | | | opts | `config` |
| 694 | POST | `/api/llm/recharge` | `createLLMOrder` | `data` | `data` |

## monitor.js  (17)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 695 | GET | `/api/monitor/trace-tree` | `traceTree` | `params` | `params` |
| 696 | GET | `/api/monitor/traces` | `traces` | `params` | `params` |
| 697 | GET | `/api/monitor/lifecycle` | `lifecycle` | `params` | `params` |
| 698 | GET | `/api/monitor/health` | `health` | `` | `` |
| 699 | GET | `/api/monitor/node-health` | `nodeHealth` | `` | `` |
| 700 | GET | `/api/monitor/latency` | `latency` | `` | `` |
| 701 | GET | `/api/monitor/anomalies` | `anomalies` | `` | `` |
| 702 | GET | `/api/monitor/trace-eval/logs` | `evalLogs` | `params` | `params` |
| 703 | GET | `/api/monitor/knowledge-weights` | `knowledgeWeights` | `params` | `params` |
| 704 | POST | `/api/monitor/trace-eval/trigger` | `triggerEval` | `params` | `null` |
| | | | | opts | `{ params }` |
| 705 | GET | `/api/rag/recall/snapshot` | `ragRecallSnapshot` | `` | `` |
| 706 | GET | `/api/rag/recall/snapshots` | `ragRecallSnapshots` | `params` | `params` |
| 707 | POST | `/api/rag/recall/collect` | `ragRecallCollect` | `params` | `params` |
| 708 | POST | `/api/rag/recall/start` | `ragRecallStart` | `` | `` |
| 709 | POST | `/api/rag/recall/stop` | `ragRecallStop` | `` | `` |
| 710 | GET | `/api/rag/recall/metrics` | `ragRecallMetrics` | `params` | `params` |
| 711 | GET | `/api/rag/recall/low-recall` | `ragLowRecallQueries` | `params` | `params` |

## objection.js  (4)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 712 | POST | `/api/objection/handle` | `handleObjection` | `data` | `data` |
| 713 | POST | `/api/objection/classify` | `classifyObjection` | `data` | `data` |
| 714 | GET | `/api/objection/categories` | `listObjectionCategories` | `` | `` |
| 715 | POST | `/api/objection/usage` | `recordObjectionUsage` | `data` | `data` |

## obs.js  (8)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 716 | GET | `/api/obs/config` | `getObsConfigList` | `params = {}` | `params` |
| 717 | GET | `/api/obs/config/${id}` | `getObsConfig` | `id` | `` |
| 718 | POST | `/api/obs/config` | `createObsConfig` | `data` | `data` |
| 719 | PUT | `/api/obs/config/${id}` | `updateObsConfig` | `id, data` | `data` |
| 720 | DELETE | `/api/obs/config/${id}` | `deleteObsConfig` | `id` | `` |
| 721 | POST | `/api/obs/config/${id}/test` | `testObsConnection` | `id` | `` |
| 722 | POST | `/api/obs/config/${id}/default` | `setDefaultObsConfig` | `id` | `` |
| 723 | GET | `/api/obs/config/default` | `getDefaultObsConfig` | `` | `` |

## oneid.js  (10)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 724 | GET | `/api/customer/oneid/list` | `listOneID` | `params` | `params` |
| 725 | GET | `/api/customer/oneid/conflicts` | `listConflicts` | `params` | `params` |
| 726 | POST | `/api/customer/oneid/merge` | `mergeOneID` | `data` | `data` |
| 727 | POST | `/api/customer/oneid/conflicts/${id}/resolve` | `resolveConflict` | `id, data` | `data` |
| 728 | GET | `/api/customer-oneid/${customerId}/identities` | `getIdentityMappings` | `customerId` | `` |
| 729 | POST | `/api/customer-oneid/${customerId}/identities` | `linkIdentity` | `customerId, data` | `data` |
| 730 | POST | `/api/customer/oneid/resolve` | `resolveIdentity` | `data` | `data` |
| 731 | GET | `/api/customer/oneid/stats` | `getOneIDStats` | `` | `` |
| 732 | GET | `/api/oneid/merge-rules` | `getMergeRules` | `` | `` |
| 733 | POST | `/api/oneid/merge-rules` | `saveMergeRules` | `data` | `data` |

## operationLog.js  (5)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 734 | GET | `/api/operation-logs` | `getOperationLogs` | `params` | `params` |
| 735 | GET | `/api/operation-logs/${id}` | `getOperationLogDetail` | `id` | `` |
| 736 | GET | `/api/operation-logs/statistics` | `getOperationLogStatistics` | `params` | `params` |
| 737 | GET | `/api/operation-logs/export` | `exportOperationLogs` | `params` | `params` |
| | | | | opts | `{ responseType: 'blob' }` |
| 738 | POST | `/api/operation-logs/clean` | `cleanOperationLogs` | `beforeDate` | `{ beforeDate }` |

## orderDraft.js  (6)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 739 | GET | `base` | `list` | `params` | `params` |
| | | | | opts | `SILENT` |
| 740 | GET | `${base}/${seg(id)}` | `get` | `id` | `undefined` |
| | | | | opts | `SILENT` |
| 741 | PATCH | `${base}/${seg(id)}` | `edit` | `id, updates` | `updates` |
| 742 | POST | `${base}/${seg(id)}/confirm` | `confirm` | `id` | `` |
| 743 | POST | `${base}/${seg(id)}/cancel` | `cancel` | `id, reason` | `{ reason }` |
| 744 | GET | `/api/agent/order-drafts/stats` | `stats` | `` | `undefined` |
| | | | | opts | `SILENT` |

## permission.js  (3)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 745 | PUT | `/api/system/permissions/${id}/enabled` | `setUserEnabled` | `id, enabled` | `{ enabled }` |
| 746 | PUT | `/api/system/permissions/${id}/password` | `resetUserPassword` | `id, password` | `{ password }` |
| 747 | GET | `/api/system/permissions/audit-logs` | `listAuditLogs` | `params` | `{ params }` |

## persona.js  (2)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 748 | GET | `/api/analytics/persona/staffs` | `listStaffs` | `` | `` |
| 749 | GET | `/api/analytics/persona/staffs/${staffId}` | `getPersonaReport` | `staffId` | `` |

## platform.js  (2)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 750 | GET | `/api/platform/message/latest` | `getLatestMessage` | `` | `{ _silent: true }` |
| 751 | POST | `/api/platform/message/${messageId}/read` | `markMessageRead` | `messageId` | `` |

## platformAccount.js  (8)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 752 | GET | `/api/platform-accounts` | `getAccounts` | `params` | `{ params }` |
| 753 | GET | `/api/platform-accounts/platforms` | `getPlatforms` | `` | `` |
| 754 | GET | `/api/platform-accounts/${id}` | `getAccountById` | `id` | `` |
| 755 | POST | `/api/platform-accounts` | `createAccount` | `data` | `data` |
| 756 | PUT | `/api/platform-accounts/${id}` | `updateAccount` | `id, data` | `data` |
| 757 | DELETE | `/api/platform-accounts/${id}` | `deleteAccount` | `id` | `` |
| 758 | POST | `/api/platform-accounts/${id}/login` | `loginAccount` | `id, data` | `data` |
| 759 | GET | `/api/platform-accounts/${id}/status` | `checkStatus` | `id` | `` |

## qqBot.js  (7)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 760 | GET | `/api/qq/accounts` | `listAccounts` | `params = {}` | `params` |
| 761 | GET | `/api/qq/accounts/${id}` | `getAccount` | `id` | `` |
| 762 | POST | `/api/qq/accounts` | `createAccount` | `data` | `data` |
| 763 | PUT | `/api/qq/accounts/${id}` | `updateAccount` | `id, data` | `data` |
| 764 | DELETE | `/api/qq/accounts/${id}` | `deleteAccount` | `id` | `` |
| 765 | POST | `/api/qq/accounts/${id}/test-send` | `testSend` | `id, data` | `data` |
| 766 | POST | `/api/qq/accounts/${id}/verify-callback` | `verifyCallback` | `id` | `{}` |

## ragProductConfig.js  (8)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 767 | POST | `/api/rag-config/products` | `createRagProduct` | `data` | `data` |
| 768 | GET | `/api/rag-config/products` | `getRagProducts` | `params` | `params` |
| 769 | GET | `/api/rag-config/products` | `listProducts` | `params` | `{ params }).then(normalizeList` |
| 770 | PUT | `/api/rag-config/products/${id}` | `updateRagProduct` | `id, data` | `data` |
| 771 | DELETE | `/api/rag-config/products/${id}` | `deleteRagProduct` | `id` | `` |
| 772 | GET | `/api/rag-config/accounts/config` | `getAccountConfig` | `params` | `{ params }` |
| 773 | PUT | `/api/rag-config/accounts/config` | `updateAccountConfig` | `data` | `data` |
| 774 | POST | `/api/rag-config/process-message` | `processMessage` | `data` | `data` |

## reachPipeline.js  (16)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 775 | GET | `/api/reach/pipelines` | `getPipelines` | `params` | `params` |
| 776 | POST | `/api/reach/pipelines` | `createPipeline` | `data` | `data` |
| 777 | GET | `/api/reach/pipelines/${id}` | `getPipeline` | `id` | `` |
| 778 | PUT | `/api/reach/pipelines/${id}` | `updatePipeline` | `id, data` | `data` |
| 779 | DELETE | `/api/reach/pipelines/${id}` | `deletePipeline` | `id` | `` |
| 780 | POST | `/api/reach/pipelines/${id}/pause` | `pausePipeline` | `id` | `{}` |
| 781 | POST | `/api/reach/pipelines/${id}/resume` | `resumePipeline` | `id` | `{}` |
| 782 | POST | `/api/reach/pipelines/${id}/archive` | `archivePipeline` | `id` | `{}` |
| 783 | GET | `/api/reach/jobs` | `getJobs` | `params` | `params` |
| 784 | POST | `/api/reach/jobs` | `enqueueJob` | `data` | `data` |
| 785 | GET | `/api/reach/jobs/${id}` | `getJob` | `id` | `` |
| 786 | POST | `/api/reach/jobs/${id}/cancel` | `cancelJob` | `id` | `{}` |
| 787 | POST | `/api/reach/jobs/${id}/retry` | `retryJob` | `id` | `{}` |
| 788 | POST | `/api/reach/jobs/${id}/execute` | `executeJob` | `id` | `{}` |
| 789 | GET | `/api/reach/stats` | `getStats` | `params` | `params` |
| 790 | POST | `/api/reach/rate-limit/reset` | `resetRateLimit` | `channel` | `{}` |
| | | | | opts | `{ params: { channel } }` |

## role.js  (3)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 791 | GET | `/api/system/roles` | `listRoles` | `` | `` |
| 792 | GET | `/api/system/roles/${code}` | `getRole` | `code` | `` |
| 793 | GET | `/api/system/roles/${code}/members` | `listRoleMembers` | `code, params` | `{ params }` |

## runtimeConfig.js  (6)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 794 | GET | `/api/config/runtime` | `runtimeConfig` | `groups` | `{ params: groups ? { groups } : {} }` |
| 795 | GET | `/api/config/dictionaries/${encodeURIComponent(parent)}` | `dictionary` | `parent` | `` |
| 796 | GET | `'/api/manage/dictionaries'),` | `dictionary` | `parent` | `` |
| 797 | GET | ``/api/manage/dictionaries/${encodeURIComponent(parent)}`),` | `dictionary` | `parent` | `` |
| 798 | PUT | `/api/manage/dictionaries/${encodeURIComponent(parent)}/items` | `dictionary` | `parent` | `item),` |
| 799 | PUT | `/api/manage/dictionaries/${encodeURIComponent(parent)}/transitions` | `dictionary` | `parent` | `{ transitions }),` |

## salesWorkbench.js  (4)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 800 | GET | `${base}/overview` | `overview` | `salesId` | `{ sales_id: salesId }` |
| 801 | GET | `${base}/team-dashboard` | `teamDashboard` | `days` | `{ days }` |
| 802 | GET | `${base}/champion` | `champion` | `days` | `{ days }` |
| 803 | GET | `${base}/quick-actions` | `quickActions` | `` | `` |

## scriptAb.js  (7)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 804 | GET | ``/api/script-library/${scriptId}/versions`),` | `?` | `` | `` |
| 805 | POST | `/api/script-library/${scriptId}/versions` | `?` | `` | `data),` |
| 806 | PUT | ``/api/script-library/${scriptId}/versions/${versionId}/activate`),` | `?` | `` | `` |
| 807 | POST | `/api/script-library/${scriptId}/expire` | `?` | `` | `data),` |
| 808 | GET | `/api/script-library/${scriptId}/ab-stats` | `?` | `` | `params),` |
| 809 | PUT | `/api/script-library/${scriptId}/ab-config` | `?` | `` | `data),` |
| 810 | POST | `/api/script-ab/conversion` | `?` | `` | `data),` |

## scriptTemplate.js  (9)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 811 | GET | `/api/scripts` | `getScriptTemplateList` | `params` | `params` |
| 812 | GET | `/api/scripts/${id}` | `getScriptTemplate` | `id` | `` |
| 813 | POST | `/api/scripts` | `createScriptTemplate` | `data` | `data` |
| 814 | PUT | `/api/scripts/${id}` | `updateScriptTemplate` | `id, data` | `data` |
| 815 | DELETE | `/api/scripts/${id}` | `deleteScriptTemplate` | `id` | `` |
| 816 | GET | `/api/scripts/categories` | `getScriptCategories` | `` | `` |
| 817 | GET | `/api/scripts/search` | `searchScriptTemplates` | `params` | `params` |
| 818 | GET | `/api/scripts/public` | `getPublicScriptTemplates` | `` | `` |
| 819 | POST | `/api/scripts/recommend` | `recommendScript` | `data` | `data` |

## securityAudit.js  (3)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 820 | POST | `/api/security/audit` | `runSecurityAudit` | `data` | `data` |
| 821 | GET | `/api/security/audit/list` | `getSecurityAuditList` | `params` | `params` |
| 822 | GET | `/api/security/audit/${id}` | `getSecurityAuditDetail` | `id` | `` |

## shortLink.js  (10)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 823 | GET | `/api/shortlink/list` | `getList` | `params` | `{ params }` |
| 824 | GET | `/api/shortlink/${id}` | `getById` | `id` | `` |
| 825 | POST | `/api/shortlink/create` | `create` | `data` | `data` |
| 826 | PUT | `/api/shortlink/update` | `update` | `data` | `data` |
| 827 | DELETE | `/api/shortlink/delete/${id}` | `delete` | `id` | `` |
| 828 | POST | `/api/shortlink/access` | `access` | `data` | `data` |
| 829 | POST | `/api/shortlink/generate` | `generateShortCode` | `data` | `data` |
| 830 | GET | `/api/shortlink/${id}/stats` | `getStats` | `id, params` | `{ params }` |
| 831 | GET | `/api/shortlink/stats/all` | `getAllStats` | `params` | `{ params }` |
| 832 | POST | `/api/shortlink/${id}/share` | `share` | `id, data` | `data` |

## sms.js  (20)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 833 | GET | `/api/sms/config` | `getConfig` | `` | `` |
| 834 | POST | `/api/sms/config` | `saveConfig` | `data` | `data` |
| 835 | GET | `/api/sms/list` | `getSmsList` | `params` | `params` |
| 836 | GET | `/api/sms/detail/${id}` | `getSmsDetail` | `id` | `` |
| 837 | POST | `/api/sms/resend/${id}` | `resendSms` | `id` | `` |
| 838 | POST | `/api/sms/send` | `sendSms` | `data` | `data` |
| 839 | GET | `/api/sms/draft/list` | `getDraftList` | `params` | `params` |
| 840 | GET | `/api/sms/draft/${id}` | `getDraftDetail` | `id` | `` |
| 841 | POST | `/api/sms/draft` | `createDraft` | `data` | `data` |
| 842 | PUT | `/api/sms/draft/${id}` | `updateDraft` | `id, data` | `data` |
| 843 | DELETE | `/api/sms/draft/${id}` | `deleteDraft` | `id` | `` |
| 844 | POST | `/api/sms/draft/${id}/send` | `sendDraft` | `id, data` | `data` |
| 845 | GET | `/api/sms/job/list` | `getJobList` | `params` | `params` |
| 846 | GET | `/api/sms/job/${id}` | `getJobDetail` | `id` | `` |
| 847 | POST | `/api/sms/job/${id}/pause` | `pauseJob` | `id` | `` |
| 848 | POST | `/api/sms/job/${id}/resume` | `resumeJob` | `id` | `` |
| 849 | POST | `/api/sms/job/${id}/stop` | `stopJob` | `id` | `` |
| 850 | POST | `/api/sms/job` | `createJob` | `data` | `data` |
| 851 | DELETE | `/api/sms/job/${id}` | `deleteJob` | `id` | `` |
| 852 | GET | `/api/sms/job/${id}/records` | `getJobRecords` | `id, params` | `params` |

## sopAgent.js  (16)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 853 | GET | `/api/sop` | `list` | `params` | `params` |
| 854 | POST | `/api/sop` | `create` | `data` | `data` |
| 855 | GET | `/api/sop/stats` | `getStats` | `` | `` |
| 856 | GET | `/api/sop/match` | `matchByIntent` | `params` | `params` |
| 857 | GET | `/api/sop/executions` | `listExecutions` | `params` | `params` |
| 858 | GET | `/api/sop/executions/${id}` | `getExecution` | `id` | `` |
| 859 | POST | `/api/sop/executions/${id}/pause` | `pauseExecution` | `id` | `` |
| 860 | POST | `/api/sop/executions/${id}/resume` | `resumeExecution` | `id` | `` |
| 861 | POST | `/api/sop/executions/${id}/cancel` | `cancelExecution` | `id` | `` |
| 862 | GET | `/api/sop/${id}` | `get` | `id` | `` |
| 863 | PUT | `/api/sop/${id}` | `update` | `id, data` | `data` |
| 864 | DELETE | `/api/sop/${id}` | `remove` | `id` | `` |
| 865 | POST | `/api/sop/${id}/activate` | `activate` | `id` | `` |
| 866 | POST | `/api/sop/${id}/deactivate` | `deactivate` | `id` | `` |
| 867 | POST | `/api/sop/execute` | `execute` | `data` | `data` |
| 868 | POST | `/api/sop/step` | `step` | `data` | `data` |

## sopTemplate.js  (7)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 869 | GET | `/api/sop-templates` | `list` | `params` | `params` |
| 870 | GET | `/api/sop-templates/${id}` | `get` | `id` | `` |
| 871 | POST | `/api/sop-templates` | `create` | `data` | `data` |
| 872 | PUT | `/api/sop-templates/${id}` | `update` | `id, data` | `data` |
| 873 | PATCH | `/api/sop-templates/${id}/toggle` | `toggle` | `id, enabled` | `{ enabled }` |
| 874 | DELETE | `/api/sop-templates/${id}` | `remove` | `id` | `` |
| 875 | POST | `/api/sop-templates/match` | `match` | `data` | `data` |

## stats.js  (1)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 876 | GET | `/api/system/info` | `?` | `` | `` |

## system.js  (3)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 877 | GET | `/api/system/info` | `getInfo` | `config` | `config` |
| 878 | GET | `/api/system/config` | `getConfig` | `` | `` |
| 879 | POST | `/api/system/config` | `saveConfig` | `data` | `data` |

## systemUser.js  (5)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 880 | GET | `/api/system/users` | `listSystemUsers` | `params` | `{ params }` |
| 881 | GET | `/api/system/users/${id}` | `getSystemUser` | `id` | `` |
| 882 | POST | `/api/system/users` | `createSystemUser` | `data` | `data` |
| 883 | PUT | `/api/system/users/${id}` | `updateSystemUser` | `id, data` | `data` |
| 884 | DELETE | `/api/system/users/${id}` | `deleteSystemUser` | `id` | `` |

## tagSegmentation.js  (10)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 885 | GET | `/api/session-tags` | `?` | `` | `params` |
| 886 | PUT | `/api/session-tags/${id}` | `?` | `` | `body` |
| 887 | POST | `/api/session-tags` | `?` | `` | `data` |
| 888 | DELETE | `/api/session-tags/${id}` | `?` | `` | `` |
| 889 | GET | `/api/customer-360/tag-rules` | `?` | `` | `params` |
| 890 | POST | `/api/customer-360/tag-rules` | `?` | `` | `data` |
| 891 | PUT | `/api/customer-360/tag-rules/${id}` | `?` | `` | `data` |
| 892 | DELETE | `/api/customer-360/tag-rules/${id}` | `?` | `` | `` |
| 893 | GET | `/api/user-segment/layers` | `?` | `` | `` |
| 894 | GET | `/api/customer-360/tag-stats` | `?` | `` | `params` |

## telegram.js  (13)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 895 | GET | `/api/telegram/accounts` | `listAccounts` | `params = {}` | `params` |
| 896 | GET | `/api/telegram/accounts/${id}` | `getAccount` | `id` | `` |
| 897 | POST | `/api/telegram/accounts` | `createAccount` | `data` | `data` |
| 898 | PUT | `/api/telegram/accounts/${id}` | `updateAccount` | `id, data` | `data` |
| 899 | DELETE | `/api/telegram/accounts/${id}` | `deleteAccount` | `id` | `` |
| 900 | POST | `/api/telegram/accounts/${id}/register-webhook` | `registerWebhook` | `id, data = {}` | `data` |
| 901 | POST | `/api/telegram/accounts/${id}/test-send` | `testSend` | `id, data` | `data` |
| 902 | GET | `/api/telegram/gates` | `listGates` | `params = {}` | `params` |
| 903 | POST | `/api/telegram/gates` | `createGate` | `data` | `data` |
| 904 | PUT | `/api/telegram/gates/${id}` | `updateGate` | `id, data` | `data` |
| 905 | DELETE | `/api/telegram/gates/${id}` | `deleteGate` | `id` | `` |
| 906 | GET | `/api/telegram/gates/${id}/members` | `listGateMembers` | `id, params = {}` | `params` |
| 907 | POST | `/api/telegram/gates/${id}/members/${memberId}/authorize` | `authorizeGateMember` | `id, memberId` | `` |

## tiktokCard.js  (8)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 908 | GET | `/api/tiktok-card/list` | `getTikTokCardList` | `params` | `params` |
| 909 | GET | `/api/tiktok-card/${id}` | `getTikTokCard` | `id` | `` |
| 910 | POST | `/api/tiktok-card` | `createTikTokCard` | `data` | `data` |
| 911 | PUT | `/api/tiktok-card/${data.id}` | `updateTikTokCard` | `data` | `data` |
| 912 | DELETE | `/api/tiktok-card/${id}` | `deleteTikTokCard` | `id` | `` |
| 913 | POST | `/api/tiktok-card/generate-short-link` | `generateShortLink` | `cardId` | `{ card_id: cardId }` |
| 914 | GET | `/api/tiktok-card/stats/overall` | `getTikTokCardOverallStats` | `params` | `params` |
| 915 | GET | `/api/tiktok-card/${cardId}/stats` | `getTikTokCardStats` | `cardId, params` | `params` |

## tuning.js  (16)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 916 | GET | `/api/admin/tuning/confidence/signals` | `getConfidenceSignals` | `params` | `params` |
| 917 | GET | `/api/admin/tuning/confidence/signals/${id}` | `getConfidenceSignal` | `id` | `` |
| 918 | GET | `/api/admin/tuning/confidence/signals/stats` | `getConfidenceSignalStats` | `params` | `params` |
| 919 | GET | `/api/admin/tuning/confidence/calibrations` | `getConfidenceCalibrations` | `params` | `params` |
| 920 | GET | `/api/admin/tuning/confidence/policies` | `getThresholdPolicies` | `params` | `params` |
| 921 | PUT | `/api/admin/tuning/confidence/policies` | `upsertThresholdPolicy` | `data` | `data` |
| 922 | GET | `/api/admin/tuning/humanize/scores` | `getHumanizeScores` | `params` | `params` |
| 923 | GET | `/api/admin/tuning/humanize/scores/stats` | `getHumanizeScoreStats` | `params` | `params` |
| 924 | GET | `/api/admin/tuning/humanize/baselines` | `getChampionBaselines` | `params` | `params` |
| 925 | GET | `/api/admin/tuning/humanize/low-quality` | `getLowQualitySamples` | `params` | `params` |
| 926 | GET | `/api/admin/tuning/feedback/events` | `getFeedbackEvents` | `params` | `params` |
| 927 | GET | `/api/admin/tuning/feedback/events/stats` | `getFeedbackEventStats` | `params` | `params` |
| 928 | GET | `/api/admin/tuning/feedback/dialogues` | `getChampionDialogues` | `params` | `params` |
| 929 | GET | `/api/admin/tuning/prompt/candidates` | `getPromptCandidates` | `params` | `params` |
| 930 | PUT | `/api/admin/tuning/prompt/candidates/${id}/status` | `updatePromptCandidateStatus` | `id, data` | `data` |
| 931 | GET | `/api/admin/tuning/bandit/arms` | `getBanditArms` | `params` | `params` |

## unifiedMessage.js  (2)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 932 | GET | `/api/unified-messages` | `getMessages` | `params` | `params` |
| 933 | GET | `/api/unified-messages/${id}` | `getMessageById` | `id` | `` |

## userSegment.js  (11)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 934 | GET | `/api/user-segment/rfm/list` | `getRFMList` | `params` | `params` |
| 935 | GET | `/api/user-segment/rfm/rule` | `getRFMRule` | `` | `` |
| 936 | POST | `/api/user-segment/rfm/rule` | `saveRFMRule` | `data` | `data` |
| 937 | PUT | `/api/user-segment/rfm/rule/${id}` | `updateRFMRule` | `id, data` | `data` |
| 938 | GET | `/api/user-segment/rfm/user?user_id=${userId}` | `getUserRFM` | `userId` | `` |
| 939 | GET | `/api/user-segment/rfm/stats` | `getRFMStats` | `` | `` |
| 940 | POST | `/api/user-segment/rfm/calculate` | `calculateRFM` | `data` | `data` |
| 941 | GET | `/api/user-segment/layers` | `getLayerDescription` | `` | `` |
| 942 | GET | `/api/user-segments` | `getUserSegments` | `params` | `params` |
| 943 | DELETE | `/api/user-segment/rfm/rule/${id}` | `deleteUserSegment` | `id` | `` |
| 944 | GET | `/api/user-segment/rfm/list?segment_id=${id}` | `getSegmentUsers` | `id, params` | `params` |

## users.js  (8)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 945 | GET | `/api/users` | `?` | `` | `{ params }` |
| 946 | GET | `/api/users/${id}` | `?` | `` | `` |
| 947 | POST | `/api/users` | `?` | `` | `data` |
| 948 | PUT | `/api/users/${id}` | `?` | `` | `data` |
| 949 | DELETE | `/api/users/${id}` | `?` | `` | `` |
| 950 | PUT | `/api/users/${id}/password` | `?` | `` | `data` |
| 951 | POST | `/api/auth/login` | `?` | `` | `data` |
| 952 | POST | `/api/auth/mfa/verify` | `?` | `` | `data` |

## wecomAccount.js  (24)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 953 | GET | `/api/wecom/health/accounts` | `listAccounts` | `` | `` |
| 954 | GET | `/api/wecom/health/accounts/risks` | `listRiskAccounts` | `` | `` |
| 955 | GET | `/api/wecom/health/accounts/select` | `selectHealthyAccount` | `` | `` |
| 956 | GET | `/api/wecom/health/accounts/summary` | `getHealthSummary` | `` | `` |
| 957 | GET | `/api/wecom/health/accounts/${id}` | `getLatestHealth` | `id` | `` |
| 958 | GET | `/api/wecom/health/accounts/${id}/history` | `listHealthHistory` | `id, params` | `params` |
| 959 | POST | `/api/wecom/health/accounts/${id}` | `reportHealth` | `id, data` | `data` |
| 960 | POST | `/api/wecom/health/accounts/${id}/status` | `updateAccountStatus` | `id, data` | `data` |
| 961 | POST | `/api/wecom/health/accounts/${id}/quota/consume` | `consumeQuota` | `id, count` | `{ count }` |
| 962 | POST | `/api/wecom/health/accounts/quota/reset` | `resetDailyQuota` | `` | `` |
| 963 | POST | `/api/wecom/messages/ingest` | `ingestMessage` | `data` | `data` |
| 964 | POST | `/api/wecom/messages/send` | `sendMessage` | `data` | `data` |
| 965 | POST | `/api/wecom/accounts` | `createAccount` | `data` | `data` |
| 966 | PUT | `/api/wecom/accounts/${id}` | `updateAccount` | `id, data` | `data` |
| 967 | DELETE | `/api/wecom/accounts/${id}` | `deleteAccount` | `id` | `` |
| 968 | GET | `/api/wecom/customers` | `getCustomers` | `params` | `{ params }` |
| 969 | GET | `/api/wecom/groups` | `getGroups` | `params` | `{ params }` |
| 970 | GET | `/api/wecom/tags` | `getTags` | `` | `` |
| 971 | GET | `/api/wecom/messages` | `getMessages` | `params` | `{ params }` |
| 972 | POST | `/api/wecom/accounts/${id}/sync-customers` | `syncCustomers` | `id` | `` |
| 973 | POST | `/api/wecom/accounts/${id}/sync-groups` | `syncGroups` | `id` | `` |
| 974 | POST | `/api/wecom/accounts/${id}/sync-tags` | `syncTags` | `id` | `` |
| 975 | POST | `/api/wecom/accounts/${id}/refresh` | `refreshAccount` | `id` | `` |
| 976 | POST | `/api/wecom/accounts/${id}/send-message` | `sendMessageById` | `id, data` | `data` |

## whatsapp.js  (14)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 977 | GET | `/api/whatsapp/accounts` | `listAccounts` | `params` | `{ params }) }` |
| 978 | POST | `/api/whatsapp/accounts` | `createAccount` | `data` | `data) }` |
| 979 | POST | ``/api/whatsapp/accounts/${id}/login/start`) }` | `startLogin` | `id` | `` |
| 980 | GET | ``/api/whatsapp/accounts/${id}/login/status`) }` | `loginStatus` | `id` | `` |
| 981 | PUT | `/api/whatsapp/accounts/${id}` | `updateAccount` | `id, data` | `data) }` |
| 982 | DELETE | ``/api/whatsapp/accounts/${id}`) }` | `deleteAccount` | `id` | `` |
| 983 | GET | `/api/whatsapp/drafts` | `listDrafts` | `params` | `{ params }) }` |
| 984 | POST | `/api/whatsapp/drafts` | `createDraft` | `data` | `data) }` |
| 985 | PUT | `/api/whatsapp/drafts/${id}` | `updateDraft` | `id, data` | `data) }` |
| 986 | DELETE | ``/api/whatsapp/drafts/${id}`) }` | `deleteDraft` | `id` | `` |
| 987 | GET | `/api/whatsapp/jobs` | `listJobs` | `params` | `{ params }) }` |
| 988 | POST | `/api/whatsapp/jobs` | `createJob` | `data` | `data) }` |
| 989 | GET | ``/api/whatsapp/jobs/${id}`) }` | `getJob` | `id` | `` |
| 990 | DELETE | ``/api/whatsapp/jobs/${id}`) }` | `deleteJob` | `id` | `` |

## whatsappCloud.js  (6)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 991 | GET | `BASE` | `listWhatsappCloud` | `params` | `params` |
| 992 | GET | `${BASE}/${id}` | `getWhatsappCloud` | `id` | `` |
| 993 | POST | `BASE` | `createWhatsappCloud` | `data` | `data` |
| 994 | PUT | `${BASE}/${id}` | `updateWhatsappCloud` | `id, data` | `data` |
| 995 | DELETE | `${BASE}/${id}` | `deleteWhatsappCloud` | `id` | `` |
| 996 | POST | `${BASE}/${id}/test-send` | `testSendWhatsappCloud` | `id, data` | `data` |

## workflowOrchestrator.js  (14)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 997 | GET | `/api/workflows/versions` | `listVersions` | `workflowIdOrParams` | `{}` |
| 998 | GET | `/api/workflows/versions` | `listVersions` | `workflowIdOrParams` | `{ workflow_id: workflowIdOrParams }` |
| 999 | GET | `/api/workflows/versions` | `listVersions` | `workflowIdOrParams` | `params` |
| 1000 | GET | `/api/workflows/versions/${id}` | `getVersion` | `id` | `` |
| 1001 | POST | `/api/workflows/versions` | `createVersion` | `data` | `data` |
| 1002 | PUT | `/api/workflows/versions/${id}` | `updateVersion` | `id, data` | `data` |
| 1003 | DELETE | `/api/workflows/versions/${id}` | `deleteVersion` | `id` | `` |
| 1004 | POST | `/api/workflows/versions/${id}/publish` | `publishVersion` | `id` | `` |
| 1005 | POST | `/api/workflows/versions/${id}/archive` | `archiveVersion` | `id` | `` |
| 1006 | POST | `/api/workflows/execute` | `execute` | `data` | `data` |
| 1007 | GET | `/api/workflows/executions` | `listExecutions` | `params` | `params` |
| 1008 | GET | `/api/workflows/executions/${id}` | `getExecution` | `id` | `` |
| 1009 | GET | `/api/workflows/executions/${executionId}/nodes` | `getNodeExecutions` | `executionId` | `` |
| 1010 | POST | `/api/workflows/executions/${id}/stop` | `stopExecution` | `id` | `` |

## xianyuCard.js  (9)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 1011 | GET | `/api/xianyu/list` | `getXianyuCardList` | `params` | `params` |
| 1012 | GET | `/api/xianyu/${id}` | `getXianyuCard` | `id` | `` |
| 1013 | POST | `/api/xianyu/create` | `createXianyuCard` | `data` | `data` |
| 1014 | PUT | `/api/xianyu/update` | `updateXianyuCard` | `data` | `data` |
| 1015 | DELETE | `/api/xianyu/delete/${id}` | `deleteXianyuCard` | `id` | `` |
| 1016 | GET | `/api/xianyu/view/${id}` | `viewXianyuCard` | `id` | `` |
| 1017 | GET | `/api/xianyu/stats/card/${id}` | `getXianyuCardStats` | `id, params` | `params` |
| 1018 | GET | `/api/xianyu/stats/overall` | `getXianyuCardOverallStats` | `params` | `params` |
| 1019 | POST | `/api/xianyu/${id}/generate-short-link` | `generateXianyuShortLink` | `id` | `` |

## xiaohongshuCard.js  (9)

| # | Method | Path | 前端函数 | 函数入参 | query / body |
|---|---|---|---|---|---|
| 1020 | GET | `/api/xiaohongshu/list` | `getXiaohongshuCardList` | `params` | `params` |
| 1021 | GET | `/api/xiaohongshu/${id}` | `getXiaohongshuCard` | `id` | `` |
| 1022 | POST | `/api/xiaohongshu/create` | `createXiaohongshuCard` | `data` | `data` |
| 1023 | PUT | `/api/xiaohongshu/update` | `updateXiaohongshuCard` | `data` | `data` |
| 1024 | DELETE | `/api/xiaohongshu/delete/${id}` | `deleteXiaohongshuCard` | `id` | `` |
| 1025 | GET | `/api/xiaohongshu/view/${id}` | `viewXiaohongshuCard` | `id` | `` |
| 1026 | GET | `/api/xiaohongshu/stats/card/${id}` | `getXiaohongshuCardStats` | `id, params` | `params` |
| 1027 | GET | `/api/xiaohongshu/stats/overall` | `getXiaohongshuCardOverallStats` | `params` | `params` |
| 1028 | POST | `/api/xiaohongshu/${id}/generate-short-link` | `generateShortLink` | `id` | `` |

---

合计 1028 条调用点。
