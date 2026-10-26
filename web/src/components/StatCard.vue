<script setup>
import { computed } from 'vue'

const props = defineProps({
  label: { type: String, required: true },
  value: { type: [Number, String], default: 0 },
  icon: { type: String, default: 'DataLine' },
  color: { type: String, default: 'linear-gradient(135deg,#6366f1,#8b5cf6)' },
  trend: { type: [String, Number], default: '' },
  hint: { type: String, default: '' }
})

const displayValue = computed(() => {
  if (typeof props.value === 'number') return props.value.toLocaleString('zh-CN')
  return props.value
})
</script>

<template>
  <div class="waf-card waf-card--hover waf-stat">
    <div class="waf-stat-icon" :style="{ background: color }">
      <el-icon><component :is="icon" /></el-icon>
    </div>
    <div style="min-width: 0">
      <div class="waf-stat-value">{{ displayValue }}</div>
      <div class="waf-stat-label">{{ label }}</div>
    </div>
    <div v-if="trend !== '' || hint" class="waf-stat-trend">
      <div v-if="trend !== ''">{{ trend }}</div>
      <div v-if="hint" style="margin-top: 4px">{{ hint }}</div>
    </div>
  </div>
</template>
