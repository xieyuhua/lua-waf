// 按 URL 的精细化限频规则接口
import { api } from './client'

/** 限频规则接口 */
export const rateLimitApi = {
  /** 规则列表 */
  list: () => api.get('/ratelimit'),
  /** 新建规则 */
  create: (payload) => api.post('/ratelimit', payload),
  /** 更新规则 */
  update: (id, payload) => api.put(`/ratelimit/${id}`, payload),
  /** 启停规则 */
  setEnabled: (id, enabled) => api.put(`/ratelimit/${id}/enabled`, { enabled }),
  /** 删除规则 */
  remove: (id) => api.del(`/ratelimit/${id}`),
  /** 重新下发规则文件（rules/ratelimit.rule） */
  sync: () => api.post('/ratelimit/sync')
}
