# user-web 路由清单（共 216 条）

## 静态路由（5条）
1. /setup - 系统初始化
2. /login - 登录
3. /help-center - 帮助中心
4. /chat/embed/default - 在线客服
5. /chat/embed/:channel_ref - 在线客服(渠道)

## 动态路由（211条，来自81个模块）

### abExperiment (1条)
1. /abExperiment/list

### aiAgent (5条)
2. /aiAgent
3. /aiAgent/list
4. /aiAgent/create
5. /aiAgent/edit/:id
6. /aiAgent/tools

### aiProductivity (1条)
7. /aiProductivity/list

### analytics (1条)
8. /analytics/cohort-path

### approvalTask (1条)
9. /approvalTask/list

### assetBundle (5条)
10. /asset-bundle/list
11. /asset-bundle/playground
12. /asset-bundle/playground/:aid
13. /asset-bundle/merchant-new
14. /asset-bundle/merchant/:aid

### assetMarket (4条)
15. /asset-market
16. /asset-market/detail/:id
17. /asset-market/my-assets
18. /asset-market/sync-log

### backup (2条)
19. /backup/list
20. /backup/enhanced

### badCase (1条)
21. /badCase/list

### batchOperation (1条)
22. /batchOperation/list

### bridgeToken (3条)
23. /bridge
24. /bridge/token
25. /token

### browserAutomation (7条)
26. /browser-automation
27. /browser-automation/tasks
28. /browser-automation/tasks/create
29. /browser-automation/tasks/:id/edit
30. /browser-automation/tasks/:id
31. /browser-automation/sessions/:id
32. /browser-automation/cron
33. /browser-automation/status

### chatChannel (4条)
34. /chatChannel/list
35. /chatChannel/create
36. /chatChannel/edit/:id
37. /chatChannel/install-guide/:id?

### churnPrediction (1条)
38. /churnPrediction/list

### clue (2条)
39. /clue/list
40. /clue/statistics

### community (1条)
41. /community/list

### conversionFunnel (1条)
42. /conversionFunnel/list

### customReport (1条)
43. /customReport/list

### customer360 (1条)
44. /customer360/list

### customerEvent (1条)
45. /customerEvent/list

### customerJourney (1条)
46. /customerJourney/dashboard

### customerService (5条)
47. /customerService/agentStatus
48. /customerService/quickReply
49. /customerService/sessionTag
50. /customerService/aiSuggestion
51. /customerService/csat

### customerSession (1条)
52. /customerSession/list

### dashboardScreen (2条)
53. /dashboardScreen/list
54. /dashboardScreen/builder

### dialogueMemory (1条)
55. /dialogueMemory/list

### dingtalkApp (1条)
56. /dingtalk-app

### domainPool (1条)
57. /domainPool

### douyinCard (3条)
58. /douyinCard
59. /douyin/stats
60. /douyin-card-stats/:id

### email (8条)
61. /email
62. /email/drafts
63. /email/jobs
64. /email/smtp
65. /email/info
66. /email/guide
67. /email/deliverability
68. /email/drag-editor

### faq (4条)
69. /faq
70. /faq/list
71. /faq/editor
72. /faq/editor/:id

### feishu (2条)
73. /feishu
74. /feishu/account

### followups (2条)
75. /followups/today
76. /followups/:id

### geoTools (27条)
77. /geo-tools/brand-config
78. /geo-tools/seo-infra
79. /geo-tools/pusher-config
80. /geo-tools/site-config
81. /geo-tools/schema-templates
82. /geo-tools/competitors
83. /geo-tools/config
84. /geo-tools/keyword-mining
85. /geo-tools/content-creation
86. /geo-tools/content-optimize
87. /geo-tools/site-publish
88. /geo-tools/push-center
89. /geo-tools/platform-publish
90. /geo-tools/workflow
91. /geo-tools/knowledge-base
92. /geo-tools/entity-graph
93. /geo-tools/funnel-dashboard
94. /geo-tools/index-tracking
95. /geo-tools/visibility
96. /geo-tools/sov-board
97. /geo-tools/crawler-stats
98. /geo-tools/verification
99. /geo/decision-report
100. /geo-tools/reports
101. /geo-tools/alerts

### glossary (1条)
102. /glossary

### i18nStats (1条)
103. /i18n/dashboard

### inbox (1条)
104. /inbox/list

### integration (1条)
105. /integration/list

### intentRecognition (1条)
106. /intentRecognition/list

### kbConnector (2条)
107. /kb/connectors
108. /cap/matrix

### knowledge (10条)
109. /knowledge/management
110. /knowledge/batch-import
111. /knowledge/playground
112. /knowledge/connectors
113. /knowledge/feedbacks
114. /knowledge/tokens
115. /knowledge/external
116. /knowledge/statistics
117. /knowledge/openapi
118. /knowledge/rag-eval

### knowledgeBase (4条)
119. /knowledgeBase
120. /knowledgeBase/list
121. /knowledgeBase/create
122. /knowledgeBase/edit/:id

### kuaishouCard (3条)
123. /kuaishouCard
124. /kuaishou/stats
125. /kuaishou-card-stats/:id

### leadMining (1条)
126. /lead-mining/index

### livecode (1条)
127. /livecode

### llmRouting (2条)
128. /llmRouting/list
129. /llmRouting/cost

### marketingFlow (1条)
130. /marketingFlow/list

### mcpCredential (1条)
131. /mcp/credentials

### messageHub (2条)
132. /messageHub/list
133. /messageHub/dashboard

### objection (1条)
134. /objection/list

### oneid (3条)
135. /oneid/list
136. /oneid/conflicts
137. /oneid/merge-rules

### operationLog (2条)
138. /operationLog/list
139. /operationLog/enhanced

### ops (2条)
140. /ops-overview
141. /sales-cockpit

### orderDraft (2条)
142. /dashboard/drafts
143. /dashboard/drafts/:id

### permission (1条)
144. /system/permissions

### persona (1条)
145. /persona/list

### platformAccount (1条)
146. /platformAccount/list

### qq (2条)
147. /qq
148. /qq/account

### ragProductConfig (3条)
149. /system/rag-product-config
150. /system/rag-account
151. /system/rag-product

### reachPipeline (2条)
152. /reachPipeline/list
153. /reachPipeline/editor/:id?

### role (1条)
154. /system/roles

### salesWorkbench (1条)
155. /sales-workbench

### scriptTemplate (2条)
156. /scriptTemplate/list
157. /scriptTemplate/ab-stats

### securityAudit (1条)
158. /securityAudit/list

### shortLink (2条)
159. /shortLink
160. /shortLink/stats

### sms (4条)
161. /sms/list
162. /sms/drafts
163. /sms/jobs
164. /sms/config

### sopAgent (1条)
165. /sopAgent/list

### sopTemplateKb (5条)
166. /sop-template
167. /sop-template/list
168. /sop-template/market
169. /sop-template/editor
170. /sop-template/editor/:id

### system (10条)
171. /system/config
172. /system/obs-config
173. /system/material-library
174. /system/monitor
175. /system/trace
176. /system/guide
177. /system/rag-overview
178. /system/automation-hub
179. /system/config-params
180. /system/ltc-config

### systemUser (1条)
181. /system/users

### tagSegmentation (1条)
182. /tagSegmentation/list

### telegram (3条)
183. /telegram
184. /telegram/account
185. /telegram/gate

### tiktok (4条)
186. /tiktok
187. /tiktok/list
188. /tiktok/stats
189. /tiktok-card-stats/:id

### tuning (3条)
190. /confidence/panel
191. /humanize/panel
192. /feedbackLoop/panel

### unifiedMessage (1条)
193. /unifiedMessage/list

### userSegment (3条)
194. /userSegment/list
195. /userSegment/rfm-matrix
196. /userSegment/builder

### wecomAccount (2条)
197. /wecomAccount/list
198. /wecomAccount/data

### whatsapp (7条)
199. /whatsapp
200. /whatsapp/account
201. /whatsapp/drafts
202. /whatsapp/jobs
203. /whatsapp/lead-group-selection
204. /whatsapp/group-messaging
205. /whatsapp/bulk-matrix

### whatsappCloud (1条)
206. /whatsapp-cloud

### workflowOrchestrator (3条)
207. /workflow-orchestrator/list
208. /workflow-orchestrator/editor/:workflow_id
209. /workflow-orchestrator/execution/:id

### xianyuCard (3条)
210. /xianyuCard
211. /xianyu/stats
212. /xianyu-card-stats/:id

### xiaohongshuCard (3条)
213. /xiaohongshuCard
214. /xiaohongshu/stats
215. /xiaohongshu-card-stats/:id

### 其他
216. /profile - 个人资料
217. /notifications - 通知中心
