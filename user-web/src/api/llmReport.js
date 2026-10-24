import { http } from '@/utils/request';

// LLM 观测报表 API（T3，见 user-server/docs/architecture/design/T3-observability-reports.md）
const LlmReportApi = {
  // 总览 KPI：from/to(YYYY-MM-DD)、channel、source
  getOverview(params) {
    return http.get('/api/reports/llm/overview', params)
  },
  // 时间序列：from/to/model/scenario/channel/source
  getTimeseries(params) {
    return http.get('/api/reports/llm/timeseries', params)
  },
  // 维度分解：by=model|scenario|channel|error_class|provider
  getBreakdown(params) {
    return http.get('/api/reports/llm/breakdown', params)
  },
  // 错误聚合+明细：error_class/page/page_size
  getErrors(params) {
    return http.get('/api/reports/llm/errors', params)
  }
};

export default LlmReportApi
