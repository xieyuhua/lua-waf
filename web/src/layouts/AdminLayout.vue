<script setup>
import { onMounted, onUnmounted, ref } from 'vue'
import AppSidebar from '../components/AppSidebar.vue'
import AppHeader from '../components/AppHeader.vue'
import { authStore } from '../stores/auth'

const collapsed = ref(false)
const mobileOpen = ref(false)
const reloadKey = ref(0)

/** 折叠/展开侧边栏（窄屏切换抽屉） */
function toggleSidebar() {
  if (window.innerWidth <= 900) {
    mobileOpen.value = !mobileOpen.value
    return
  }
  collapsed.value = !collapsed.value
}

/** 刷新当前页面数据 */
function refresh() {
  reloadKey.value += 1
}

/** 窄屏下点击菜单后关闭抽屉 */
function closeMobile() {
  mobileOpen.value = false
}

/** 窗口尺寸变化时复位移动端状态 */
function handleResize() {
  if (window.innerWidth > 900) mobileOpen.value = false
}

onMounted(() => {
  window.addEventListener('resize', handleResize)
  if (!authStore.state.ready) authStore.fetchProfile()
})
onUnmounted(() => window.removeEventListener('resize', handleResize))
</script>

<template>
  <div class="waf-shell">
    <AppSidebar :collapsed="collapsed" :mobile-open="mobileOpen" @navigate="closeMobile" />

    <div v-if="mobileOpen" class="waf-mask" @click="closeMobile"></div>

    <div class="waf-main">
      <AppHeader :collapsed="collapsed" @toggle="toggleSidebar" @refresh="refresh" />

      <main class="waf-content">
        <router-view v-slot="{ Component }">
          <transition name="fade-slide" mode="out-in">
            <component :is="Component" :key="$route.fullPath + '-' + reloadKey" />
          </transition>
        </router-view>
      </main>
    </div>
  </div>
</template>

<style scoped>
.waf-shell {
  display: flex;
  height: 100vh;
  overflow: hidden;
  background: var(--waf-bg);
}

.waf-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

.waf-content {
  flex: 1;
  overflow-y: auto;
  background:
    radial-gradient(1200px 400px at 100% -10%, rgba(99, 102, 241, 0.08), transparent 60%),
    radial-gradient(900px 300px at -10% 110%, rgba(139, 92, 246, 0.07), transparent 60%),
    var(--waf-bg);
}

.waf-mask {
  position: fixed;
  inset: 0;
  background: rgba(9, 15, 28, 0.5);
  z-index: 19;
}
</style>
