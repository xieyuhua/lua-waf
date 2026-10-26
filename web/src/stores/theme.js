import { ref, watch } from 'vue'

const STORAGE_KEY = 'wafadmin.theme'
const stored = typeof localStorage !== 'undefined' ? localStorage.getItem(STORAGE_KEY) : null

/** 暗色主题开关，默认跟随系统 */
const prefersDark = typeof window !== 'undefined' && window.matchMedia?.('(prefers-color-scheme: dark)').matches
const dark = ref(stored ? stored === 'dark' : Boolean(prefersDark))

/** 应用主题到 html 根节点 */
function apply(value) {
  if (typeof document === 'undefined') return
  document.documentElement.classList.toggle('dark', value)
}

apply(dark.value)

watch(dark, (value) => {
  apply(value)
  try {
    localStorage.setItem(STORAGE_KEY, value ? 'dark' : 'light')
  } catch (error) {
    /* 忽略隐私模式下的存储异常 */
  }
})

export const themeStore = {
  dark,
  toggle: () => {
    dark.value = !dark.value
  }
}
