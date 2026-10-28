import { api } from './client'

/** APISIX 路由插件（镜像复制 / key 限流）接口 */
export const apisixApi = {
  /** 集成配置（启用开关、管理地址，密钥脱敏） */
  config: () => api.get('/apisix/config'),
  /** 保存集成配置（启用/关闭、管理地址与密钥） */
  saveConfig: (payload) => api.put('/apisix/config', payload),
  /** 管理接口连通状态 */
  status: () => api.get('/apisix/status'),
  /** 路由列表（含镜像与限流配置） */
  routes: () => api.get('/apisix/routes'),
  /** 开启/更新镜像复制 */
  setMirror: (id, payload) => api.put(`/apisix/routes/${id}/mirror`, payload),
  /** 关闭镜像复制 */
  clearMirror: (id) => api.del(`/apisix/routes/${id}/mirror`),
  /** 开启/更新 key 限流 */
  setLimit: (id, payload) => api.put(`/apisix/routes/${id}/limit`, payload),
  /** 关闭 key 限流 */
  clearLimit: (id) => api.del(`/apisix/routes/${id}/limit`)
}
