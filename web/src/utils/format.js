/** 通用格式化工具 */

/** 时间格式化：兼容后端零值时间 */
export function formatTime(value) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime()) || date.getFullYear() < 2000) return '-'
  const pad = (num) => String(num).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(
    date.getMinutes()
  )}`
}

/** 仅日期 */
/** 把 Date 转为后端可解析的 "YYYY-MM-DD HH:mm:ss" */
export function toBackendTime(value) {
  if (!value) return ''
  const date = value instanceof Date ? value : new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  const pad = (num) => String(num).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(
    date.getMinutes()
  )}:${pad(date.getSeconds())}`
}

/** 数字千分位 */
export function formatNumber(value) {
  if (value === null || value === undefined || value === '') return '-'
  const num = Number(value)
  return Number.isFinite(num) ? num.toLocaleString('zh-CN') : String(value)
}

/** 毫秒耗时格式化 */
export function formatDuration(ms) {
  const value = Number(ms) || 0
  if (value < 1000) return `${Math.round(value)}ms`
  return `${(value / 1000).toFixed(2)}s`
}

/** SQL 语句单行预览 */
export function compactStatement(statement, limit = 90) {
  const text = String(statement || '').replace(/\s+/g, ' ').trim()
  if (text.length <= limit) return text
  return `${text.slice(0, limit)}...`
}
