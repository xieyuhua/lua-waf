<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppConfigPanel from '../components/config/AppConfigPanel.vue'
import AppearancePanel from '../components/config/AppearancePanel.vue'
import AiConfigPanel from '../components/config/AiConfigPanel.vue'
import AccountPanel from '../components/config/AccountPanel.vue'
import AlertPanel from '../components/config/AlertPanel.vue'

const route = useRoute()
const router = useRouter()

// 全部系统设置合并为一个页面，用选项卡切换；选项卡状态写入 query，刷新/分享都能定位
const TABS = {
  app: {
    label: '运行配置',
    hint: '在线修改 conf/app.ini 的运行时参数，保存后立即热更新',
    component: AppConfigPanel
  },
  appearance: {
    label: '外观设置',
    hint: '设置后台界面的明暗模式与主题色，保存在浏览器本地',
    component: AppearancePanel
  },
  alert: {
    label: '告警通知',
    hint: '命中勾选的规则时推送到企业微信群机器人',
    component: AlertPanel
  },
  ai: {
    label: 'AI 设置',
    hint: '配置大模型接口，用于攻击日志智能研判',
    component: AiConfigPanel
  },
  account: {
    label: '账号设置',
    hint: '查看登录信息、修改口令并了解系统运行参数',
    component: AccountPanel
  }
}

const NAMES = Object.keys(TABS)
const active = ref(normalize(route.query.tab))

function normalize(value) {
  const tab = typeof value === 'string' ? value : ''
  return NAMES.includes(tab) ? tab : 'app'
}

/** 切换选项卡并同步地址栏 */
function switchTab(name) {
  active.value = normalize(name)
  if (route.query.tab !== active.value) {
    router.replace({ query: { ...route.query, tab: active.value } })
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

const subtitle = computed(() => TABS[active.value]?.hint || '')

onMounted(() => {
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
      <el-tab-pane v-for="(meta, name) in TABS" :key="name" :label="meta.label" :name="name">
        <component :is="meta.component" v-if="active === name" />
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
