<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppConfigPanel from '../components/config/AppConfigPanel.vue'
import LuaConfigPanel from '../components/config/LuaConfigPanel.vue'

const route = useRoute()
const router = useRouter()

// 运行配置（app.ini）与 WAF 配置（config.lua）合并为一个页面，用选项卡切换；
// 选项卡状态写入 query，刷新或分享链接都能定位到同一页
const TABS = ['app', 'waf']
const active = ref(normalize(route.query.tab))

function normalize(value) {
  const tab = typeof value === 'string' ? value : ''
  return TABS.includes(tab) ? tab : 'app'
}

/** 切换选项卡并同步到地址栏 */
function switchTab(name) {
  active.value = name
  if (route.query.tab !== name) {
    router.replace({ query: { ...route.query, tab: name } })
  }
}

// 支持浏览器前进/后退
watch(
  () => route.query.tab,
  (value) => {
    const tab = normalize(value)
    if (tab !== active.value) active.value = tab
  }
)

const subtitle = computed(() =>
  active.value === 'waf'
    ? '在线修改 openresty 侧的 config.lua，保存后可一键 reload 生效'
    : '在线修改 conf/app.ini 的运行时参数，保存后立即热更新'
)

onMounted(() => {
  // 地址栏没有 tab 参数时补上，便于刷新保持
  if (!route.query.tab) router.replace({ query: { ...route.query, tab: active.value } })
})
</script>

<template>
  <div class="waf-page">
    <div class="waf-page-head">
      <div>
        <h1 class="waf-page-title"><span class="dot"></span> 系统设置</h1>
        <p class="waf-page-sub">{{ subtitle }}</p>
      </div>
    </div>

    <el-tabs :model-value="active" class="config-tabs" @update:model-value="switchTab">
      <el-tab-pane label="运行配置（app.ini）" name="app">
        <AppConfigPanel v-if="active === 'app'" />
      </el-tab-pane>
      <el-tab-pane label="WAF 配置（config.lua）" name="waf">
        <LuaConfigPanel v-if="active === 'waf'" />
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<style scoped>
.config-tabs :deep(.el-tabs__header) {
  margin-bottom: 14px;
}

.config-tabs :deep(.el-tabs__item) {
  font-size: 13.5px;
  font-weight: 600;
}
</style>
