import { http } from '@/utils/request';

// ============================================================
// 功能模块市场（用户端）
//
// 对标 assetMarket.js 的交互形态（分组筛选 + 卡片网格 + 一键下单）。
// 主路径是在线收银台（易支付）：下单响应带 payment 段就直接跳收银台，
// 支付成功权益自动开通。payment 段为空（平台关通道 / 生成失败）才是例外 ——
// 此时回落 payment_guide 线下指引，那是零代码回滚路径，不是常规流程。
// ============================================================

// ── 市场浏览（public 档：未登录也能逛）──────────────────────────────

// 第二参 config 透传给 http.get（如 {_silent:true}）：MyModules 把它当闸态探针用，
// 探针失败不该弹 toast。
export const listMarketModules = (params, config) =>
  http.get('/api/module-market/list', params, config);

export const marketModuleDetail = (key) =>
  http.get(`/api/module-market/detail/${encodeURIComponent(key)}`);

export const paymentGuide = (config) =>
  http.get('/api/module-market/payment-guide', undefined, config);

export const regulatoryNotice = () =>
  http.get('/api/tenant/regulatory-notice');

// ── 下单与订单（auth 档）──────────────────────────────────────────

export const createModuleOrder = (data) =>
  http.post('/api/module-market/order', data);

// 第二参 config 透传（如 {_silent:true}）：订单页 30 秒后台轮询里的刷新要静默，
// 失败不该弹 toast。
export const listModuleOrders = (params, config) =>
  http.get('/api/module-market/orders', params, config);

export const moduleOrderDetail = (orderNo) =>
  http.get(`/api/module-market/orders/${encodeURIComponent(orderNo)}`);

// 支付状态直查（003 T018）：比 10 分钟 sync 快，收银台轮询用
export const orderPayStatus = (orderNo, config) =>
  http.get(`/api/module-market/orders/${encodeURIComponent(orderNo)}/pay-status`, undefined, config);

// 「去支付」：给已下的待支付订单重新取一次支付参数，收银台关掉后可重开。
// 平台按 out_trade_no 现算参数，不会产生第二张订单。
export const orderPayParams = (orderNo) =>
  http.post(`/api/module-market/orders/${encodeURIComponent(orderNo)}/pay`);

// ── 订阅状态与续费（003 T018/T022）────────────────────────────────
//
// 都在订阅闸白名单内（/api/subscription/*）：expired 态下墙页靠它们
// 拿状态、下单 —— 这两个接口自身永远不会 4305（防死循环，oracle P1-12）。

export const subscriptionStatus = (config) =>
  http.get('/api/subscription/status', undefined, config);

export const renewSubscription = (data) =>
  http.post('/api/subscription/renew', data);

// ── 我的模块 ───────────────────────────────────────────────────────

export const myModules = () =>
  http.get('/api/modules/my');

export const syncModules = (config) =>
  http.post('/api/modules/sync', undefined, config);

export const moduleSyncLogs = (params, config) =>
  http.get('/api/modules/sync-logs', params, config);

// ── 平台模型余额与套餐（需求五）──────────────────────────────────────
//
// 这三个此前只在后端存在、前端零调用 —— 计量做了一整套却没有任何人看得见。
// 「实时扣减」对用户是否可感知，取决于他能不能看到剩余量。

export const llmWallet = (config) =>
  http.get('/api/llm/wallet', undefined, config);

export const llmPlans = (config) =>
  http.get('/api/llm/plans', undefined, config);

export const createLLMOrder = (data) =>
  http.post('/api/llm/recharge', data);

// ── 展示辅助 ───────────────────────────────────────────────────────

/** 分 → 元。展示层换算，接口层一律用分（避免两处各自四舍五入一次导致对账差 1 分） */
export const fmtPrice = (cents) => {
  const v = Number(cents || 0) / 100
  return v.toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
}

/** token 展示：1k / 1M */
export const fmtTokens = (n) => {
  const v = Number(n || 0)
  if (v >= 1e6) return `${(v / 1e6).toFixed(2)}M`
  if (v >= 1e3) return `${(v / 1e3).toFixed(1)}K`
  return String(v)
}

export const MODULE_CATEGORY_LABEL = { base: '基础模块', paid: '收费模块' }

export const MODULE_SOURCE_LABEL = {
  purchase: '用户购买',
  dependency: '依赖自动开通',
  manual: '平台开通',
  default_base: '基础能力'
}

export const MODULE_STATUS_LABEL = {
  active: '已开通',
  expired: '已到期',
  disabled: '已停用'
}

export const ORDER_STATUS_LABEL = {
  pending_payment: '待确认收款',
  paid: '已收款',
  fulfilled: '已开通',
  cancelled: '已取消',
  rejected: '已拒绝'
}

/**
 * priceLabel 生成价格文案。
 *
 * 四档口径对应四种完全不同的用户动作，写错一档就会误导：
 *   base    → 「免费」        （本来就是基础能力，不用做任何事）
 *   0 元    → 「免费开通」    （要开通、但不收钱；点一下会走订单流程）
 *   once    → 「¥X 买断」     （一次性买断，browser-automation 等）
 *   其余    → 「¥X / 年」    （要下单 → 联系付款）
 *
 * 003 T019 修复：原先只有「/年」「按量」两档，¥99 买断的模块被标成
 * 「¥99 / 年」—— 用户以为每年还要付一次，实际是一次性买断。
 */
export const priceLabel = (item) => {
  if (item.free) return '免费'
  if (Number(item.price_cents || 0) === 0) return '免费开通'
  if (item.billing_cycle === 'once') return `¥${fmtPrice(item.price_cents)} 买断`
  return `¥${fmtPrice(item.price_cents)} / 年`
}

/** billingCycleLabel 计费周期文案（Detail 页「计费周期」行） */
export const billingCycleLabel = (cycle) => {
  if (cycle === 'once') return '一次性买断'
  if (cycle === 'metered') return '按量（token 计量）'
  if (cycle === 'monthly') return '按月'
  return '按年'
}
