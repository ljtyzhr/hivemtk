import { http } from '@/utils/request'

// includeDeprecated 默认 false：管理端默认不列已下架参数。
// 那 11 条是"代码里没有消费点"的旋钮（服务端没实现长轮询、/ws/chat 路由从未注册、
// NewResultCache 生产链路从未构造 …）。列出来用户改完去验证，行为一定不变，
// 而原因藏在代码里——所以宁可默认不列，需要查历史值时再显式打开。
export function list(group, includeDeprecated = false) {
  return http.get('/api/manage/config-params', {
    params: { group, include_deprecated: includeDeprecated ? 1 : 0 }
  })
}

// listGroups 返回分组元数据（中文名 / 说明 / 排序）。这是侧栏那列中文名的事实源，
// 曾经在 user-web 里硬编码成一张 GROUP_LABELS 表——新增参数组时前端会静默回落显示
// 英文 group key，而页面上看不出哪里不对。
export function listGroups() {
  return http.get('/api/manage/config-params/groups')
}

export function auditLogs(limit = 100) {
  return http.get('/api/manage/config-params/audit-logs', { params: { limit } })
}

export function update(group, key, value) {
  return http.put(`/api/manage/config-params/${group}/${key}`, { value })
}

export function reset(group, key) {
  return http.post(`/api/manage/config-params/${group}/${key}/reset`)
}

export function bulkReset(group) {
  return http.post(`/api/manage/config-params/${group}/reset`)
}

// 平台端地址（platform.api_host）的专用读写端点。
//
// 为什么不用上面的 update('platform', 'api_host', ...)：
// 那条路在 SaaS 模式下会落进**租户覆盖行**（后端 UpdateValue 见到请求 ctx 带租户
// 绑定就写 tenant_config_params），而平台地址是部署级事实 —— 覆盖行对它不生效。
// 结果是页面回「保存成功」而地址没变，是最难查的一类不一致。
//
// 专用端点直接写平台默认行（config_params 本体），且挂在平台运营方守卫下。
// _silent：403 由卡片自己渲染成「无权限」状态（并附后端原话），
// 通用拦截器再弹一句「拒绝访问」只会把精确原因盖掉。
export function setPlatformHost(address) {
  return http.put('/api/manage/platform-host', { address }, { _silent: true })
}

// testPlatformConnectivity 探测平台端连通性。
//
// 不传 address 时探当前生效的地址；传了则探指定的地址（用于「还没保存，先试一下」）。
// 探针是 GET {host}/health，刻意不走鉴权链 —— 把连通性探针绑到签名/登录上，
// 会把「签名没配好」「管理员密码不对」都读成「平台挂了」。
export function testPlatformConnectivity(address) {
  return http.post('/api/manage/platform-host/connectivity', { address: address || '' }, { _silent: true })
}
