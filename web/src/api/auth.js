import { api } from './client'

/** 认证相关接口 */
export const authApi = {
  /** 获取图形验证码 */
  captcha: () => api.get('/auth/captcha'),
  /** 登录 */
  login: (payload) => api.post('/auth/login', payload),
  /** 注销 */
  logout: () => api.post('/auth/logout'),
  /** 当前登录账号 */
  profile: () => api.get('/auth/profile'),
  /** 修改自身口令 */
  changePassword: (payload) => api.post('/users/password', payload)
}
