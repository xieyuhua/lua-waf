import { reactive, computed } from 'vue'
import { authApi } from '../api/auth'

/** 全局登录态，供路由守卫与布局共用 */
const state = reactive({
  user: null,
  loading: false,
  ready: false
})

/** 拉取当前登录账号；失败时清空登录态 */
async function fetchProfile() {
  state.loading = true
  try {
    state.user = await authApi.profile()
    state.ready = true
    return state.user
  } catch (error) {
    state.user = null
    state.ready = true
    return null
  } finally {
    state.loading = false
  }
}

/** 登录 */
async function login(payload) {
  const data = await authApi.login(payload)
  state.user = data?.user ?? null
  state.ready = true
  return data
}

/** 注销 */
async function logout() {
  try {
    await authApi.logout()
  } finally {
    state.user = null
  }
}

export const authStore = {
  state,
  isLoggedIn: computed(() => Boolean(state.user)),
  displayName: computed(() => state.user?.username ?? '未登录'),
  fetchProfile,
  login,
  logout
}
