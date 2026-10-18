export default [
  {
    path: '/module-market',
    name: 'ModuleMarket',
    component: () => import('@/views/moduleMarket/Market.vue'),
    // 侧栏由**后端菜单树**驱动（GET /api/menus，见 stores/permission.js），
    // 不是由这里的 meta.title 聚合。所以本文件不需要 meta.group ——
    // 侧栏位置与可见性完全取决于 user-server internal/model/menu.go 里
    // 有没有对应节点（moduleCenterGroup）。
    // 未登录能逛市场（契约：浏览与列表是公开橱窗档，后端 list 也在 public 组）。
    // 下单/我的模块/订单仍在鉴权档 —— requiresAuth 保留在 my/orders 上。
    meta: { title: '功能模块市场', icon: 'Shop', requiresAuth: false, public: true }
  },
  {
    path: '/module-market/detail/:key',
    name: 'ModuleMarketDetail',
    component: () => import('@/views/moduleMarket/Detail.vue'),
    // 详情接口在后端鉴权档（401）：匿名访问时 Detail.vue 渲染内联登录引导，
    // 而不是被路由守卫挡在门外 —— 能逛到详情本身就是市场获客的一部分。
    meta: { title: '模块详情', requiresAuth: false, public: true, hidden: true }
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
