<script setup>
import { onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { setUnauthorizedHandler } from './api/client'
import { authStore } from './stores/auth'
import { themeStore } from './stores/theme'

const router = useRouter()

// 主题在应用启动时应用一次（模块内部已处理，这里确保 SSR/热更新后仍生效）
themeStore.dark.value && document.documentElement.classList.add('dark')

onMounted(() => {
  // 会话失效时统一跳转登录页
  setUnauthorizedHandler(() => {
    authStore.state.user = null
    if (router.currentRoute.value.path !== '/login') {
      router.push({ path: '/login' })
    }
  })
})
</script>

<template>
  <router-view v-slot="{ Component }">
    <transition name="fade-slide" mode="out-in">
      <component :is="Component" />
    </transition>
  </router-view>
</template>
