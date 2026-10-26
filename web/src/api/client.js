/**
 * 统一的接口客户端：
 * - 自动读取 CSRF Cookie 并放入请求头（非安全方法）
 * - 统一解析 { code, message, data } 响应结构
 * - 401 时触发未登录回调，由路由层跳转登录页
 */

const BASE = '/api'

/** 未登录时的回调，由 main.js 注入 */
let unauthorizedHandler = () => {}

export function setUnauthorizedHandler(handler) {
  unauthorizedHandler = handler
}

/** 业务错误 */
export class ApiError extends Error {
  constructor(message, code, status) {
    super(message)
    this.name = 'ApiError'
    this.code = code
    this.status = status
  }
}

/** 读取 Cookie */
function readCookie(name) {
  const target = document.cookie.split(';').map((item) => item.trim())
  for (const item of target) {
    if (item.startsWith(name + '=')) {
      return decodeURIComponent(item.slice(name.length + 1))
    }
  }
  return ''
}

/** 拼接查询串，忽略空值 */
function buildQuery(params) {
  if (!params) return ''
  const search = new URLSearchParams()
  Object.entries(params).forEach(([key, value]) => {
    if (value === undefined || value === null || value === '') return
    search.append(key, value)
  })
  const text = search.toString()
  return text ? `?${text}` : ''
}

/** 底层请求 */
async function request(path, { method = 'GET', body, params } = {}) {
  const headers = { Accept: 'application/json' }
  if (body !== undefined) headers['Content-Type'] = 'application/json'

  const token = readCookie('waf_csrf')
  if (token && method !== 'GET') headers['X-CSRF-Token'] = token

  let response
  try {
    response = await fetch(BASE + path + buildQuery(params), {
      method,
      headers,
      credentials: 'same-origin',
      body: body === undefined ? undefined : JSON.stringify(body)
    })
  } catch (error) {
    throw new ApiError('网络异常，请检查服务是否正常运行', -1, 0)
  }

  let payload = null
  try {
    payload = await response.json()
  } catch (error) {
    payload = null
  }

  if (response.status === 401) {
    unauthorizedHandler()
    throw new ApiError(payload?.message || '登录状态已失效', payload?.code, 401)
  }
  if (!response.ok || (payload && payload.code !== 0)) {
    throw new ApiError(payload?.message || `请求失败（HTTP ${response.status}）`, payload?.code, response.status)
  }
  return payload ? payload.data : null
}

export const api = {
  get: (path, params) => request(path, { method: 'GET', params }),
  post: (path, body, params) => request(path, { method: 'POST', body: body ?? {}, params }),
  put: (path, body) => request(path, { method: 'PUT', body: body ?? {} }),
  del: (path, params) => request(path, { method: 'DELETE', params })
}
