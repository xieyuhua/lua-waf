import { api } from './client'

/** 慢 SQL 统计接口，from/to 支持日期或日期时间 */
export const slowSqlApi = {
  list: (params) => api.get('/slow-sql', params),
  stats: (params) => api.get('/slow-sql/stats', params),
  purge: (days) => api.post('/slow-sql/purge', {}, { params: { days } })
}
