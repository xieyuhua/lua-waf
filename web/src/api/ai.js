import { api } from './client'

/** AI 大模型配置与分析接口 */
export const aiApi = {
  /** 读取配置（密钥脱敏） */
  status: () => api.get('/ai/config'),
  /** 保存配置 */
  save: (payload) => api.put('/ai/config', payload),
  /** 连通性测试 */
  test: () => api.post('/ai/test'),
  /** 分析指定日志 */
  analyze: (payload) => api.post('/ai/analyze', payload),
  /** 查询某条日志在服务端缓存的研判结果（跨设备查看） */
  result: (logId) => api.get(`/ai/results/${logId}`),
  /** 慢 SQL 优化分析 */
  analyzeSlowSQL: (payload) => api.post('/ai/slow-sql/analyze', payload),
  /** 查询某条慢 SQL 在服务端缓存的分析结果 */
  slowSQLResult: (id) => api.get(`/ai/slow-sql/results/${id}`)
}
