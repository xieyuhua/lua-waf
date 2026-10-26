import { computed, reactive, ref, watch } from 'vue'

const STORAGE_KEY = 'wafadmin.theme'

/** 可选主题色（同时驱动品牌色、渐变与 Element Plus 主色） */
export const accentPresets = [
  { id: 'indigo', name: '靛蓝', primary: '#6366f1', accent: '#8b5cf6' },
  { id: 'violet', name: '紫罗兰', primary: '#8b5cf6', accent: '#d946ef' },
  { id: 'sky', name: '晴空', primary: '#0ea5e9', accent: '#22d3ee' },
  { id: 'emerald', name: '翡翠', primary: '#10b981', accent: '#34d399' },
  { id: 'amber', name: '琥珀', primary: '#f59e0b', accent: '#f97316' },
  { id: 'rose', name: '玫瑰', primary: '#f43f5e', accent: '#fb7185' }
]

/** 模式选项 */
export const modeOptions = [
  { value: 'light', label: '亮色', icon: 'Sunny' },
  { value: 'dark', label: '暗色', icon: 'Moon' },
  { value: 'system', label: '跟随系统', icon: 'Monitor' }
]

const state = reactive({ mode: 'system', accent: 'indigo' })
const systemDark = ref(false)

/** 读取系统偏好 */
function initSystemWatcher() {
  if (typeof window === 'undefined' || !window.matchMedia) return
  const query = window.matchMedia('(prefers-color-scheme: dark)')
  systemDark.value = query.matches
  const handler = (event) => {
    systemDark.value = event.matches
  }
  if (query.addEventListener) query.addEventListener('change', handler)
  else if (query.addListener) query.addListener(handler)
}

/** 从本地存储恢复（兼容早期只存 "dark"/"light" 的格式） */
function restore() {
  if (typeof localStorage === 'undefined') return
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return
    if (raw === 'dark' || raw === 'light') {
      state.mode = raw
      return
    }
    const parsed = JSON.parse(raw)
    if (['light', 'dark', 'system'].includes(parsed?.mode)) state.mode = parsed.mode
    if (accentPresets.some((item) => item.id === parsed?.accent)) state.accent = parsed.accent
  } catch (error) {
    /* 忽略损坏的存储值 */
  }
}

/** 是否为暗色 */
const dark = computed(() => state.mode === 'dark' || (state.mode === 'system' && systemDark.value))

/** 当前主题色预设 */
const currentPreset = computed(() => accentPresets.find((item) => item.id === state.accent) ?? accentPresets[0])

/** 图表调色板（随主题色变化，供 ECharts 使用） */
const chartPalette = computed(() => {
  const preset = currentPreset.value
  return {
    primary: preset.primary,
    accent: preset.accent,
    series: [
      preset.primary,
      preset.accent,
      '#38bdf8',
      '#10b981',
      '#f59e0b',
      '#ef4444',
      '#ec4899',
      '#14b8a6'
    ]
  }
})

/** 十六进制转 rgb */
function hexToRgb(hex) {
  const value = hex.replace('#', '')
  const normalized =
    value.length === 3
      ? value
          .split('')
          .map((char) => char + char)
          .join('')
      : value
  return {
    r: parseInt(normalized.slice(0, 2), 16),
    g: parseInt(normalized.slice(2, 4), 16),
    b: parseInt(normalized.slice(4, 6), 16)
  }
}

/** 把颜色与目标色混合，ratio 为混合比例 */
function mix(hex, target, ratio) {
  const from = hexToRgb(hex)
  const to = hexToRgb(target)
  const r = Math.round(from.r + (to.r - from.r) * ratio)
  const g = Math.round(from.g + (to.g - from.g) * ratio)
  const b = Math.round(from.b + (to.b - from.b) * ratio)
  return `#${[r, g, b].map((value) => value.toString(16).padStart(2, '0')).join('')}`
}

/** 计算 Element Plus 需要的色阶 */
function shades(primary) {
  return {
    base: primary,
    strong: mix(primary, '#000000', 0.14),
    light3: mix(primary, '#ffffff', 0.3),
    light5: mix(primary, '#ffffff', 0.5),
    light7: mix(primary, '#ffffff', 0.7),
    light8: mix(primary, '#ffffff', 0.8),
    light9: mix(primary, '#ffffff', 0.92),
    dark2: mix(primary, '#000000', 0.2)
  }
}

/** 应用主题到文档根节点 */
function apply() {
  if (typeof document === 'undefined') return
  const root = document.documentElement
  root.classList.toggle('dark', dark.value)

  const preset = currentPreset.value
  const color = shades(preset.primary)
  const accent = preset.accent || mix(preset.primary, '#8b5cf6', 0.35)

  root.style.setProperty('--waf-primary', color.base)
  root.style.setProperty('--waf-primary-strong', color.strong)
  root.style.setProperty('--waf-accent', accent)
  root.style.setProperty('--waf-gradient', `linear-gradient(135deg, ${color.base} 0%, ${accent} 100%)`)

  root.style.setProperty('--el-color-primary', color.base)
  root.style.setProperty('--el-color-primary-light-3', color.light3)
  root.style.setProperty('--el-color-primary-light-5', color.light5)
  root.style.setProperty('--el-color-primary-light-7', color.light7)
  root.style.setProperty('--el-color-primary-light-8', color.light8)
  root.style.setProperty('--el-color-primary-light-9', color.light9)
  root.style.setProperty('--el-color-primary-dark-2', color.dark2)

  root.dataset.accent = preset.id
}

/** 持久化 */
function persist() {
  if (typeof localStorage === 'undefined') return
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ mode: state.mode, accent: state.accent }))
  } catch (error) {
    /* 隐私模式下忽略 */
  }
}

/** 设置模式 */
function setMode(mode) {
  if (['light', 'dark', 'system'].includes(mode)) state.mode = mode
}

/** 设置主题色 */
function setAccent(id) {
  if (accentPresets.some((item) => item.id === id)) state.accent = id
}

/** 恢复默认（跟随系统 + 靛蓝） */
function reset() {
  state.mode = 'system'
  state.accent = 'indigo'
}

initSystemWatcher()
restore()
apply()

watch([() => state.mode, () => state.accent, dark], () => {
  apply()
  persist()
})

export const themeStore = {
  state,
  dark,
  presets: accentPresets,
  modeOptions,
  currentPreset,
  chartPalette,
  /** 当前模式的中文名 */
  modeLabel: computed(() => modeOptions.find((item) => item.value === state.mode)?.label ?? '跟随系统'),
  setMode,
  setAccent,
  reset,
  /** 亮暗快速切换（header 快捷按钮使用） */
  toggle: () => {
    state.mode = dark.value ? 'light' : 'dark'
  }
}
