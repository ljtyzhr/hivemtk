// 模板/技能市场（F2 最小可用版）路由模块。
//
// 本文件是「接线建议」的落地件：路由懒装配机制（src/router/index.js 的
// import.meta.glob('./modules/*.js')）会自动捡到这个文件，但还需在
// router/index.js 两处登记（见 specs/013 的 delivery.md「建议接线点」）：
//   1) moduleNames 数组追加 "templateMarket"；
//   2) pathToModule 增加 'template-market': 'templateMarket'。
// 未登记前本路由不会被装配，页面仍可通过单测直接挂载验证。
export default [
  {
    path: '/template-market',
    name: 'TemplateMarket',
    component: () => import('@/views/templateMarket/Market.vue'),
    // 市场浏览是公开橱窗档：未登录也能逛（对齐 module-market 的 list 公开档）。
    meta: { title: '模板/技能市场', icon: 'Shop', requiresAuth: false, public: true }
  }
]
