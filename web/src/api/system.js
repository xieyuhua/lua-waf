import { api } from './client'

/** 系统信息与运行配置接口 */
export const systemApi = {
  /** 运行信息（版本、模式、路径、节点） */
  info: () => api.get('/system/info'),
  /** 读取运行配置（密钥脱敏） */
  config: () => api.get('/system/config'),
  /** 保存运行配置（写回 ini 并热更新） */
  saveConfig: (payload) => api.put('/system/config', payload),
  /** 读取配置文件原文 */
  rawConfig: () => api.get('/system/config/raw'),
  /** 从磁盘重新加载配置 */
  reloadConfig: () => api.post('/system/config/reload'),
  /** 日志入库消费者状态（队列长度、消费/写入/失败计数） */
  ingest: () => api.get('/system/ingest')
}
