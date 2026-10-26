<script setup>
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessageBox } from 'element-plus'
import { authStore } from '../stores/auth'
import { themeStore } from '../stores/theme'

defineProps({ collapsed: { type: Boolean, default: false } })
const emit = defineEmits(['toggle', 'refresh'])

const route = useRoute()
const router = useRouter()

const keyword = ref('')
const title = computed(() => route.meta.title ?? 'X-WAF 管理台')
const crumb = computed(() => route.meta.crumb ?? '')
const initial = computed(() => (authStore.displayName.value || 'A').slice(0, 1).toUpperCase())

/** 全局搜索：跳转到攻击日志并按关键词过滤 */
function search() {
  const value = keyword.value.trim()
  if (!value) return
  router.push({ path: '/logs', query: { keyword: value } })
  keyword.value = ''
}

/** 用户菜单 */
async function handleCommand(command) {
  if (command === 'account') {
    router.push('/settings/account')
    return
  }
  if (command === 'logout') {
    try {
      await ElMessageBox.confirm('确认退出当前登录？', '退出登录', {
        confirmButtonText: '退出',
        cancelButtonText: '取消',
        type: 'warning'
      })
    } catch (error) {
      return
    }
    await authStore.logout()
    router.push('/login')
  }
}
</script>

<template>
  <header class="waf-header">
    <button class="waf-icon-btn" title="折叠菜单" @click="emit('toggle')">
      <el-icon><Fold /></el-icon>
    </button>

    <div>
      <div class="waf-header-title">{{ title }}</div>
      <div v-if="crumb" class="waf-header-crumb">{{ crumb }}</div>
    </div>

    <div style="flex: 1"></div>

    <el-input
      v-model="keyword"
      class="waf-header-search"
      placeholder="搜索 IP / URL / 攻击类型"
      clearable
      style="max-width: 260px"
      @keyup.enter="search"
    >
      <template #prefix>
        <el-icon><Search /></el-icon>
      </template>
    </el-input>

    <button class="waf-icon-btn" title="刷新数据" @click="emit('refresh')">
      <el-icon><Refresh /></el-icon>
    </button>

    <button class="waf-icon-btn" :title="themeStore.dark.value ? '切换亮色' : '切换暗色'" @click="themeStore.toggle()">
      <el-icon>
        <component :is="themeStore.dark.value ? 'Sunny' : 'Moon'" />
      </el-icon>
    </button>

    <el-dropdown trigger="click" @command="handleCommand">
      <div class="waf-user-chip">
        <span class="waf-avatar">{{ initial }}</span>
        <span style="font-size: 13px; font-weight: 600">{{ authStore.displayName.value }}</span>
        <el-icon style="font-size: 12px; color: var(--waf-text-dim)"><ArrowDown /></el-icon>
      </div>
      <template #dropdown>
        <el-dropdown-menu>
          <el-dropdown-item command="account">
            <el-icon><Setting /></el-icon> 账号设置
          </el-dropdown-item>
          <el-dropdown-item command="logout" divided>
            <el-icon><SwitchButton /></el-icon> 退出登录
          </el-dropdown-item>
        </el-dropdown-menu>
      </template>
    </el-dropdown>
  </header>
</template>

<style scoped>
@media (max-width: 900px) {
  .waf-header-search {
    display: none;
  }
}
</style>
