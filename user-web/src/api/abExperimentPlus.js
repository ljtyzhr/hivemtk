import { http } from '@/utils/request'

export const getExperimentWithStats = (id, method = 'frequentist') =>
  // method 是查询参数（后端 ctx.DefaultQuery("method")）。写进 axios 配置顶层会被
  // axios.get 的 mergeConfig 覆盖回 "get"，参数静默丢失 ⇒ 永远只拿到 frequentist。
  http.get(`/api/ab-experiments/${id}/stats`, { params: { method } });

export const getExperimentDiagnostics = (id) =>
  http.get(`/api/ab-experiments/${id}/diagnostics`)

export const getExperimentWithCUPED = (id) =>
  http.get(`/api/ab-experiments/${id}/cuped`);

export const sequentialTest = (id, alpha = 0.05) =>
  http.post(`/api/ab-experiments/${id}/sequential-test`, { alpha });

export const bayesianTest = (id) =>
  http.post(`/api/ab-experiments/${id}/bayesian-test`, {});
