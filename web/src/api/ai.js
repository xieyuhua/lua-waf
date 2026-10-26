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
  analyze: (payload) => api.post('/ai/analyze', payload)
}
