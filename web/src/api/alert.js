// 告警通知（企业微信机器人）接口
import { api } from './client'

/** 告警配置接口 */
export const alertApi = {
  /** 读取配置与运行状态 */
  config: () => api.get('/alert/config'),
  /** 保存配置 */
  save: (payload) => api.put('/alert/config', payload),
  /** 发送测试消息（可传临时 webhook） */
  test: (payload) => api.post('/alert/test', payload ?? {})
}
