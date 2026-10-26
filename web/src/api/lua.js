import { api } from './client'

/** lua-waf（config.lua）配置接口 */
export const luaApi = {
  /** 读取配置项（密钥脱敏） */
  config: () => api.get('/lua/config'),
  /** 保存配置项 */
  save: (values) => api.put('/lua/config', { values }),
  /** 读取 config.lua 原文 */
  raw: () => api.get('/lua/config/raw'),
  /** 让修改生效（nginx -t && nginx -s reload） */
  reload: () => api.post('/lua/reload')
}
