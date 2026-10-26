import { computed, ref } from 'vue'

// 视口宽度（模块级单例，避免每个组件各自监听 resize）
const width = ref(typeof window === 'undefined' ? 1440 : window.innerWidth)

if (typeof window !== 'undefined') {
  window.addEventListener('resize', () => {
    width.value = window.innerWidth
  })
}

/** 响应式断点：与 main.css 中的 media query 保持一致 */
export const viewport = {
  width,
  /** 手机（≤768px） */
  isMobile: computed(() => width.value <= 768),
  /** 平板及以下（≤1024px） */
  isTablet: computed(() => width.value <= 1024)
}

/** 组件内取用断点 */
export function useBreakpoint() {
  return viewport
}
