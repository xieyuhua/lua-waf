import { api } from './client'

/** 站点管理接口 */
export const siteApi = {
  list: (params) => api.get('/sites', params),
  all: () => api.get('/sites/all'),
  detail: (id) => api.get(`/sites/${id}`),
  create: (payload) => api.post('/sites', payload),
  update: (id, payload) => api.put(`/sites/${id}`, payload),
  remove: (id) => api.del(`/sites/${id}`),
  preview: (id) => api.get(`/sites/${id}/preview`),
  syncAll: () => api.post('/sites/sync'),
  syncOne: (id) => api.post(`/sites/${id}/sync`)
}
