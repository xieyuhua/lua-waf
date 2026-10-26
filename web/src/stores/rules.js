import { reactive } from 'vue'
import { ruleApi } from '../api/rule'

/** 规则类型缓存，供侧边栏子菜单复用 */
const state = reactive({
  types: [],
  loading: false,
  loaded: false
})

/** 加载规则类型（含数量），已加载则直接返回缓存 */
async function load(force = false) {
  if (state.loaded && !force) return state.types
  state.loading = true
  try {
    state.types = await ruleApi.types()
    state.loaded = true
  } catch (error) {
    state.types = state.types ?? []
  } finally {
    state.loading = false
  }
  return state.types
}

/** 按分组归类，用于子菜单分组展示 */
function grouped() {
  const groups = new Map()
  state.types.forEach((item) => {
    const key = item.group || '其他'
    if (!groups.has(key)) groups.set(key, [])
    groups.get(key).push(item)
  })
  return Array.from(groups.entries()).map(([name, items]) => ({ name, items }))
}

/** 按 key 查找规则类型 */
function find(key) {
  return state.types.find((item) => item.key === key)
}

export const ruleStore = { state, load, grouped, find }
