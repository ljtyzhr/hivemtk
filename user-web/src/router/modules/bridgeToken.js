export default [{
  path: '/bridge',
  name: 'BridgeRoot',
  redirect: '/bridge/token',
  meta: { title: '渠道接入', icon: 'Link', requiresAdmin: true }
}, {
  path: '/bridge/token',
  name: 'BridgeTokenManagement',
  component: () => import('@/views/bridge/TokenManagement.vue'),
  meta: { title: '桥接凭证', requiresAdmin: true }
}, {
  path: '/bridge/extension',
  name: 'BridgeExtensionPackage',
  component: () => import('@/views/bridge/ExtensionPackage.vue'),
  meta: { title: '扩展安装包', icon: 'Download', requiresAuth: true }
}, {
  path: '/token',
  redirect: '/bridge/token'
}];
