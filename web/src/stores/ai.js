import { reactive } from 'vue'

// AI 分析结果在浏览器侧的缓存：与「AI 设置」中的缓存时长（默认 10 分钟）保持一致，
// 用于刷新页面后仍能看到刚才的研判结论，避免重复消耗 token。
const STORAGE_KEY = 'wafadmin.aiResults'
// 场景：攻击日志 / 慢 SQL，缓存键与查询都按场景隔离
export const AI_SCENE_LOG = 'log'
export const AI_SCENE_SLOW_SQL = 'slow_sql'
export const AI_RESULT_TTL = 10 * 60 * 1000

const state = reactive({ items: {} })

/** 从 localStorage 恢复（过滤过期项） */
function restore() {
  if (typeof localStorage === 'undefined') return
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return
    const parsed = JSON.parse(raw)
    const now = Date.now()
    const items = {}
    for (const [key, item] of Object.entries(parsed || {})) {
      const content = String(item?.result?.content || '').trim()
      // 丢弃过期、没有正文或缺少场景标识的条目（旧版本可能存了空结果）
      if (item && item.scene && content && now - (item.at || 0) < AI_RESULT_TTL) items[key] = item
    }
    state.items = items
    // 把过滤后的结果写回，清掉过期与空正文的旧条目
    persist()
  } catch (error) {
    /* 忽略损坏数据 */
  }
}

/** 写回 localStorage */
function persist() {
  if (typeof localStorage === 'undefined') return
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(state.items))
  } catch (error) {
    /* 隐私模式忽略 */
  }
}

/** 清理过期项 */
function prune() {
  const now = Date.now()
  for (const [key, item] of Object.entries(state.items)) {
    if (!item || now - (item.at || 0) >= AI_RESULT_TTL) delete state.items[key]
  }
}

/** 生成缓存键：场景 + 对象 ID 集合 + 追问 */
export function resultKey(scene, ids, question) {
  const list = [...(ids || [])]
    .map((id) => Number(id))
    .sort((a, b) => a - b)
    .join(',')
  return `${scene}|${list}|${String(question || '').trim()}`
}

restore()

export const aiResultStore = {
  state,

  /** 按「场景 + ID 集合 + 追问」取缓存结果 */
  get(scene, ids, question) {
    prune()
    const item = state.items[resultKey(scene, ids, question)]
    return item && item.scene === scene ? item : null
  },

  /** 指定场景下某条记录是否已有分析结果 */
  has(scene, id) {
    return Boolean(this.find(scene, id))
  },

  /** 找到包含指定记录的最早一条结果 */
  find(scene, id) {
    prune()
    const target = Number(id)
    const list = Object.values(state.items)
      .filter(
        (item) =>
          item &&
          item.scene === scene &&
          Array.isArray(item.logIds) &&
          item.logIds.includes(target)
      )
      .sort((a, b) => (b.at || 0) - (a.at || 0))
    return list.length ? list[0] : null
  },

  /** 保存分析结果 */
  save(scene, ids, question, result) {
    if (!result || !result.content) return
    const list = [...(ids || [])].map((id) => Number(id))
    state.items[resultKey(scene, list, question)] = {
      scene,
      logIds: list,
      question: String(question || '').trim(),
      at: Date.now(),
      result: {
        content: result.content,
        model: result.model,
        elapsed_ms: result.elapsed_ms,
        usage: result.usage,
        cached: result.cached,
        cached_at: result.cached_at,
        truncated: result.truncated,
        finish_reason: result.finish_reason
      }
    }
    prune()
    persist()
  },

  /** 清空本地结果 */
  clear() {
    state.items = {}
    persist()
  },

  /** 有效期内的结果条数 */
  count() {
    prune()
    return Object.keys(state.items).length
  }
}
