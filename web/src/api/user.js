import { api } from './client'

/** 账号管理接口 */
export const userApi = {
  list: (params) => api.get('/users', params),
  create: (payload) => api.post('/users', payload),
  update: (id, payload) => api.put(`/users/${id}`, payload),
  remove: (id) => api.del(`/users/${id}`)
}
