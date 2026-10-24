import { http } from '@/utils/request';

// ============================================================
// 模板/技能市场（F2 最小可用版）
//
// 只读浏览 + 元数据 + 分类，不含支付/发布/审核闭环。
// 后端是进程内注册的目录数据源（internal/repository/template_market_repo.go），
// 因此 list/categories 都是公开橱窗档：未登录也能逛（对齐 module-market/list）。
// ============================================================

// ── 市场浏览（public 档）──────────────────────────────────────────

// 第二参 config 透传（如 {_silent:true}），静默失败不弹 toast。
export const listMarketEntries = (params, config) =>
  http.get('/api/template-market/list', params, config);

export const marketCategories = (config) =>
  http.get('/api/template-market/categories', undefined, config);

// 详情目前用列表页内的抽屉展示；此函数为后续独立详情页预留。
export const marketEntryDetail = (id) =>
  http.get(`/api/template-market/detail/${encodeURIComponent(id)}`);

// ── 展示辅助 ─────────────────────────────────────────────────────

export const ENTRY_KIND_LABEL = { template: '模板', skill: '技能' };

export const AVAILABILITY_LABEL = {
  builtin: '内置可用',
  needs_module: '需开通模块',
  coming_soon: '即将上线'
};

export const AVAILABILITY_TAG_TYPE = {
  builtin: 'success',
  needs_module: 'warning',
  coming_soon: 'info'
};
