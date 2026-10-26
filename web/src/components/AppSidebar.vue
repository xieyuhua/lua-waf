<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ruleStore } from '../stores/rules'

const props = defineProps({
  collapsed: { type: Boolean, default: false },
  mobileOpen: { type: Boolean, default: false }
})
const emit = defineEmits(['navigate'])

const route = useRoute()
const router = useRouter()

/** 基础导航分组 */
const groups = [
  {
    name: '概览',
    items: [
      { label: '安全概览', icon: 'DataBoard', to: '/dashboard' },
      { label: '攻击地图', icon: 'Location', to: '/attack-map' }
    ]
  },
  {
    name: '防护配置',
    items: [
      { label: '站点管理', icon: 'Cloudy', to: '/sites' },
      { label: 'WAF 规则', icon: 'Lock', to: '/rules', children: true },
      { label: '账号管理', icon: 'User', to: '/users' }
    ]
  },
  {
    name: '审计分析',
    items: [
      { label: '攻击日志', icon: 'Document', to: '/logs' },
      { label: '慢 SQL 统计', icon: 'Timer', to: '/slow-sql' }
    ]
  },
  {
    name: '系统设置',
    items: [
      { label: 'AI 设置', icon: 'MagicStick', to: '/settings/ai' },
      { label: '账号设置', icon: 'Setting', to: '/settings/account' }
    ]
  }
]

const rulesExpanded = ref(true)
const ruleGroups = computed(() => ruleStore.grouped())
const totalRules = computed(() => ruleStore.state.types.reduce((sum, item) => sum + (item.count || 0), 0))

/** 判断菜单是否激活 */
function isActive(to) {
  if (to === '/rules') return route.path.startsWith('/rules')
  return route.path === to
}

/** 子规则是否激活 */
function isChildActive(key) {
  return route.path === `/rules/${key}`
}

/** 点击菜单后关闭移动端抽屉 */
function handleNavigate() {
  emit('navigate')
}

/** 展开/收起规则子菜单 */
function toggleRules() {
  if (props.collapsed) {
    router.push('/rules')
    return
  }
  rulesExpanded.value = !rulesExpanded.value
}

onMounted(() => {
  ruleStore.load()
})
</script>

<template>
  <aside class="waf-sidebar" :class="{ 'is-collapsed': collapsed, 'is-open': mobileOpen }">
    <div class="waf-brand">
      <div class="waf-brand-logo">X</div>
      <div v-show="!collapsed" class="waf-brand-text">
        <span class="waf-brand-name">X-WAF 管理台</span>
        <span class="waf-brand-sub">云防火墙控制中心</span>
      </div>
    </div>

    <nav class="waf-nav">
      <template v-for="group in groups" :key="group.name">
        <div v-show="!collapsed" class="waf-nav-group">{{ group.name }}</div>

        <template v-for="item in group.items" :key="item.to">
          <!-- 带子菜单的规则入口 -->
          <template v-if="item.children">
            <a
              class="waf-nav-item"
              :class="{ 'is-active': isActive(item.to) }"
              @click="toggleRules"
            >
              <span class="waf-nav-icon"><el-icon><Lock /></el-icon></span>
              <template v-if="!collapsed">
                <span>{{ item.label }}</span>
                <span class="waf-nav-badge">{{ totalRules }}</span>
                <el-icon style="margin-left: 6px; font-size: 12px">
                  <component :is="rulesExpanded ? 'ArrowUp' : 'ArrowDown'" />
                </el-icon>
              </template>
            </a>
            <div v-if="!collapsed && rulesExpanded" class="waf-nav-children">
              <template v-for="ruleGroup in ruleGroups" :key="ruleGroup.name">
                <div class="waf-nav-group" style="margin: 8px 6px 4px">{{ ruleGroup.name }}</div>
                <router-link
                  v-for="rule in ruleGroup.items"
                  :key="rule.key"
                  :to="`/rules/${rule.key}`"
                  class="waf-nav-item waf-nav-child"
                  :class="{ 'is-active': isChildActive(rule.key) }"
                  @click="handleNavigate"
                >
                  <span>{{ rule.name }}</span>
                  <span class="waf-nav-badge">{{ rule.count }}</span>
                </router-link>
              </template>
            </div>
          </template>

          <!-- 普通菜单 -->
          <router-link
            v-else
            :to="item.to"
            class="waf-nav-item"
            :class="{ 'is-active': isActive(item.to) }"
            @click="handleNavigate"
          >
            <span class="waf-nav-icon">
              <el-icon><component :is="item.icon" /></el-icon>
            </span>
            <span v-if="!collapsed">{{ item.label }}</span>
          </router-link>
        </template>
      </template>
    </nav>

    <div v-show="!collapsed" class="waf-sidebar-foot">X-WAF · 分层架构 v2</div>
  </aside>
</template>
