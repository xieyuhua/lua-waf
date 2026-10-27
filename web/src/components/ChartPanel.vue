<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { viewport } from '../utils/responsive'
import * as echarts from 'echarts'

const props = defineProps({
  option: { type: Object, required: true },
  height: { type: String, default: '280px' }
})

// 移动端统一降到 220px，避免图表挤占首屏
const resolvedHeight = computed(() => (viewport.isMobile.value ? '220px' : props.height))

const container = ref(null)
let chart = null
let observer = null

/** 渲染图表 */
function render() {
  if (!container.value) return
  if (!chart) {
    chart = echarts.init(container.value)
  }
  chart.setOption(props.option, true)
}

onMounted(() => {
  render()
  if (typeof ResizeObserver !== 'undefined') {
    observer = new ResizeObserver(() => chart?.resize())
    observer.observe(container.value)
  }
})

onBeforeUnmount(() => {
  observer?.disconnect()
  chart?.dispose()
  chart = null
})

watch(() => props.option, render, { deep: true })

// 暴露实例，便于父组件做缩放、导出等交互（地图缩放会用到）
defineExpose({
  /** 返回 ECharts 实例（尚未挂载时为 null） */
  getInstance: () => chart
})
</script>

<template>
  <div ref="container" :style="{ width: '100%', height: resolvedHeight }"></div>
</template>
