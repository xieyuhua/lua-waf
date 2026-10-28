import { api } from './client'

/** lua-waf（config.lua）配置接口 */
export const luaApi = {
  /** 读取配置项（密钥脱敏） */
  config: () => api.get('/lua/config'),
  /** 保存配置项 */
  save: (values) => api.put('/lua/config', { values }),
  /** 保存前测试 Redis / MySQL 连通性（用表单当前值） */
  test: (values) => api.post('/lua/config/test', { values }),
  /** 读取 config.lua 原文 */
  raw: () => api.get('/lua/config/raw'),
  /** 让修改生效（nginx -t && nginx -s reload） */
  reload: () => api.post('/lua/reload')
}
