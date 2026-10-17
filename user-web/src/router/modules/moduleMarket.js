export default [
  {
    path: '/module-market',
    name: 'ModuleMarket',
    component: () => import('@/views/moduleMarket/Market.vue'),
    // meta.group 留空：功能模块市场是**顶层入口**，不放进任何顶级分组。
    // 放进分组等于说"它属于某个已有业务域"—— 而它横跨智能体/触达/IM/分析，
    // 塞进任何一组都是错的，菜单上会让人以为它只管那一块。
    meta: { title: '功能模块市场', icon: 'Shop', requiresAuth: true }
  },
  {
    path: '/module-market/detail/:key',
    name: 'ModuleMarketDetail',
    component: () => import('@/views/moduleMarket/Detail.vue'),
    meta: { title: '模块详情', requiresAuth: true, hidden: true }
  },
  {
    path: '/module-market/my',
    name: 'ModuleMy',
    component: () => import('@/views/moduleMarket/MyModules.vue'),
    meta: { title: '我的模块', icon: 'FolderOpened', requiresAuth: true }
  },
  {
    path: '/module-market/orders',
    name: 'ModuleOrders',
    component: () => import('@/views/moduleMarket/Orders.vue'),
    meta: { title: '我的订单', icon: 'Tickets', requiresAuth: true }
  },
  {
    path: '/register',
    name: 'Register',
    component: () => import('@/views/register/Register.vue'),
    // 未登录可进：注册页本来就在登录之前
    meta: { title: '商户注册', requiresAuth: false, public: true, hideLayout: true }
  }
]
