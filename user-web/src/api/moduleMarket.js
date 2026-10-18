import { http } from '@/utils/request';

// ============================================================
// 功能模块市场（用户端）
//
// 对标 assetMarket.js 的交互形态（分组筛选 + 卡片网格 + 一键下单），
// 关键差别：这里**没有"立即到账"**。一期没有支付通道，提交订单后必须
// 联系运营线下付款，所以每个入口都要带出付款指引，而不是让人点了就以为买到了。
// ============================================================

// ── 市场浏览（public 档：未登录也能逛）──────────────────────────────

// 第二参 config 透传给 http.get（如 {_silent:true}）：MyModules 把它当闸态探针用，
// 探针失败不该弹 toast。
export const listMarketModules = (params, config) =>
  http.get('/api/module-market/list', params, config);

export const marketModuleDetail = (key) =>
  http.get(`/api/module-market/detail/${encodeURIComponent(key)}`);

export const paymentGuide = () =>
  http.get('/api/module-market/payment-guide');

export const regulatoryNotice = () =>
  http.get('/api/tenant/regulatory-notice');

// ── 下单与订单（auth 档）──────────────────────────────────────────

export const createModuleOrder = (data) =>
  http.post('/api/module-market/order', data);

export const listModuleOrders = (params) =>
  http.get('/api/module-market/orders', params);

export const moduleOrderDetail = (orderNo) =>
  http.get(`/api/module-market/orders/${encodeURIComponent(orderNo)}`);

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
 * 三档口径对应三种完全不同的用户动作，写错一档就会误导：
 *   base    → 「免费」        （本来就是基础能力，不用做任何事）
 *   0 元    → 「免费开通」    （要开通、但不收钱；点一下会走订单流程）
 *   > 0     → 「¥X / 年」    （要下单 → 联系付款）
 */
export const priceLabel = (item) => {
  if (item.free) return '免费'
  if (Number(item.price_cents || 0) === 0) return '免费开通'
  return `¥${fmtPrice(item.price_cents)} / 年`
}
