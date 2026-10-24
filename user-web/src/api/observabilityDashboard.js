import { http } from '@/utils/request'

// 统一看板（D2 AI 全链路可观测性 + B2 渠道健康，specs/007）。
//
// window 只收 24h / 7d / 30d，非法由后端回落 7d（口径见后端 service.windowDuration）。
// 空态（成本 0 / 送达 0 / 无渠道）是**合法读数**不是错误：本地模型成本本来就是 0，
// 新装实例还没有消息量，页面必须显式渲染「暂无数据」而不是把它当接口故障。
export function getObservabilityDashboard(window = '7d') {
  return http.get('/api/observability/dashboard', { window })
}
