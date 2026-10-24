export default [
  {
    path: '/observability-dashboard',
    name: 'ObservabilityDashboard',
    component: () => import('@/views/observabilityDashboard/Index.vue'),
    meta: { title: 'AI 观测与渠道健康', icon: 'Odometer', requiresAdmin: true, keepAlive: true }
  }
]
