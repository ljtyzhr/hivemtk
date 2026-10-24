// T5 资产市场商店化：平台审核后台路由（设计 docs/architecture/design/T5-asset-store.md §8）
export default [
  {
    path: '/asset-admin/review-queue',
    name: 'AssetAdminReviewQueue',
    component: () => import('@/views/assetAdmin/ReviewQueue.vue'),
    meta: { title: '资产上架审核', group: 'system', icon: 'Goods', requiresAuth: true }
  }
]
