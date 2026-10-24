// LLM 观测报表路由（T3）
export default [
  {
    path: '/reports/llm',
    name: 'LlmUsageDashboard',
    component: () => import('@/views/reports/LlmUsageDashboard.vue'),
    meta: { title: 'LLM用量报表', group: 'analytics', icon: 'TrendCharts', requiresAuth: true }
  }
]
