<script setup>
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import * as echarts from 'echarts'

const props = defineProps({
  option: { type: Object, required: true },
  height: { type: String, default: '280px' }
})

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
</script>

<template>
  <div ref="container" :style="{ width: '100%', height }"></div>
</template>
