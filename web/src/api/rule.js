import { api } from './client'

/** WAF 规则接口 */
export const ruleApi = {
  /** 规则类型（含各类型数量），用于生成子菜单 */
  types: () => api.get('/rules/types'),
  /** 按类型/关键词查询规则 */
  list: (params) => api.get('/rules', params),
  create: (payload) => api.post('/rules', payload),
  update: (id, payload) => api.put(`/rules/${id}`, payload),
  remove: (id) => api.del(`/rules/${id}`),
  validate: (payload) => api.post('/rules/validate', payload),
  sync: () => api.post('/rules/sync'),
  /** 规则文件与数据库取并集：缺少的新增、已有的保留 */
  syncFiles: () => api.post('/rules/sync-files'),
  /** WAF 检测与拦截顺序（只读说明） */
  pipeline: () => api.get('/rules/pipeline'),
  /** 规则命中统计（按 rule_tag 聚合） */
  stats: (params) => api.get('/rules/stats', params),
  /** 启用/停用规则 */
  setEnabled: (id, enabled) => api.put(/rules//enabled, { enabled })
}
