import { api } from './client'

/** 攻击日志接口 */
export const logApi = {
  /** 分页查询（支持关键词、IP、站点、类型、时间范围） */
  list: (params) => api.get('/logs', params),
  /** 统计摘要 */
  overview: (params) => api.get('/logs/overview', params),
  /** 筛选项：攻击类型 + 站点 */
  filters: () => api.get('/logs/filters'),
  /** 来源 IP 排行 */
  ips: (params) => api.get('/logs/ips', params),
  /** 攻击类型统计 */
  types: (params) => api.get('/logs/types', params),
  /** 按天趋势 */
  dates: (params) => api.get('/logs/dates', params),
  /** 详情 */
  detail: (id) => api.get(`/logs/${id}`)
}
