<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import * as echarts from 'echarts'
import worldGeo from '../assets/world.geo.json'
import ChartPanel from '../components/ChartPanel.vue'
import { dashboardApi } from '../api/dashboard'
import { themeStore } from '../stores/theme'

const loading = ref(true)
const snapshot = ref(null)
const range = ref('all')
const autoRefresh = ref(true)
let timer = null
let mapRegistered = false

/** 注册世界地图（只需一次） */
function ensureMap() {
  if (mapRegistered) return
  echarts.registerMap('world', worldGeo)
  mapRegistered = true
}

/** 加载快照 */
async function load() {
  loading.value = true
  try {
    ensureMap()
    snapshot.value = await dashboardApi.attackMap(range.value)
  } catch (error) {
    ElMessage.error(error.message || '加载攻击地图失败')
  } finally {
    loading.value = false
  }
}

/** 切换时间范围 */
function switchRange(value) {
  if (range.value === value) return
  range.value = value
  load()
}

/** 地图坐标点（过滤未知地区） */
const points = computed(() =>
  (snapshot.value?.points ?? []).filter((item) => item.lat !== 0 || item.lon !== 0)
)

const palette = computed(() => themeStore.chartPalette.value)

const mapOption = computed(() => ({
  backgroundColor: 'transparent',
  tooltip: {
    trigger: 'item',
    backgroundColor: 'rgba(9,15,28,0.92)',
    borderColor: 'rgba(99,102,241,0.5)',
    textStyle: { color: '#e2e8f0', fontSize: 12 },
    formatter: (params) => {
      const value = params.value || []
      return `<strong>${params.name}</strong><br/>攻击次数：${value[2] ?? 0}<br/>来源：${params.data?.area || '-'}<br/>类型：${
        (params.data?.attack_types || []).join('、') || '-'
      }`
    }
  },
  geo: {
    map: 'world',
    roam: false,
    zoom: 1.05,
    center: [12, 18],
    itemStyle: {
      areaColor: 'rgba(23,35,58,0.9)',
      borderColor: 'rgba(99,102,241,0.35)',
      borderWidth: 0.6
    },
    emphasis: {
      itemStyle: { areaColor: 'rgba(99,102,241,0.35)' },
      label: { show: false }
    },
    silent: true
  },
  series: [
    {
      name: '攻击来源',
      type: 'effectScatter',
      coordinateSystem: 'geo',
      rippleEffect: { brushType: 'stroke', scale: 3.2, period: 4 },
      symbolSize: (value) => Math.min(6 + Math.sqrt(value[2] || 1) * 2, 26),
      itemStyle: { color: '#f97316', shadowBlur: 14, shadowColor: 'rgba(249,115,22,0.85)' },
      label: {
        show: true,
        position: 'right',
        formatter: '{b}',
        color: '#cbd5e1',
        fontSize: 10
      },
      data: points.value.map((item) => ({
        name: item.name,
        value: [item.lon, item.lat, item.count],
        area: item.area,
        attack_types: item.attack_types
      }))
    }
  ]
}))

const trendOption = computed(() => {
  const trend = snapshot.value?.trend ?? []
  return {
    grid: { left: 8, right: 12, top: 16, bottom: 4, containLabel: true },
    tooltip: { trigger: 'axis' },
    xAxis: {
      type: 'category',
      data: trend.map((item) => item.date.slice(5)),
      axisLine: { lineStyle: { color: 'rgba(148,163,184,0.3)' } },
      axisLabel: { color: '#8593ad', fontSize: 10 }
    },
    yAxis: { type: 'value', show: false },
    series: [
      {
        type: 'bar',
        data: trend.map((item) => item.num),
        barWidth: 10,
        itemStyle: {
          borderRadius: [4, 4, 0, 0],
          color: {
            type: 'linear',
            x: 0,
            y: 0,
            x2: 0,
            y2: 1,
            colorStops: [
              { offset: 0, color: palette.value.accent },
              { offset: 1, color: `${palette.value.accent}26` }
            ]
          }
        }
      }
    ]
  }
})

const topTypes = computed(() => snapshot.value?.top_types ?? [])
const topCountries = computed(() => snapshot.value?.top_countries ?? [])
const recent = computed(() => snapshot.value?.recent ?? [])
const maxTypeNum = computed(() => Math.max(1, ...topTypes.value.map((item) => item.num)))

onMounted(() => {
  load()
  timer = setInterval(() => {
    if (autoRefresh.value) load()
  }, 20000)
})

onBeforeUnmount(() => {
  if (timer) clearInterval(timer)
})
</script>

<template>
  <div class="screen">
    <div class="screen-head">
      <div>
        <h1 class="screen-title">攻击地图 · 实时态势</h1>
        <p class="screen-sub">
          统计范围 {{ snapshot?.range || '-' }} · 更新于 {{ snapshot?.updated_at || '-' }}
        </p>
      </div>
      <div style="display: flex; align-items: center; gap: 10px">
        <el-radio-group :model-value="range" size="small" @update:model-value="switchRange">
          <el-radio-button value="today">今日</el-radio-button>
          <el-radio-button value="all">全部</el-radio-button>
        </el-radio-group>
        <el-switch v-model="autoRefresh" active-text="自动刷新" />
        <el-button size="small" :loading="loading" @click="load">
          <el-icon><Refresh /></el-icon>
        </el-button>
      </div>
    </div>

    <div class="screen-body">
      <!-- 左侧指标 -->
      <div class="screen-col screen-col--left">
        <div class="screen-card">
          <div class="metric-row">
            <div class="metric">
              <div class="metric-value">{{ snapshot?.total ?? 0 }}</div>
              <div class="metric-label">攻击总次数</div>
            </div>
            <div class="metric">
              <div class="metric-value">{{ snapshot?.overseas ?? 0 }}</div>
              <div class="metric-label">境外来源</div>
            </div>
          </div>
          <div class="metric-row">
            <div class="metric">
              <div class="metric-value">{{ (snapshot?.foreign_rate ?? 0).toFixed(1) }}%</div>
              <div class="metric-label">境外占比</div>
            </div>
            <div class="metric">
              <div class="metric-value">{{ snapshot?.site_count ?? 0 }}</div>
              <div class="metric-label">保护站点</div>
            </div>
          </div>
        </div>

        <div class="screen-card">
          <div class="screen-card-title">攻击类型分布</div>
          <div v-for="item in topTypes" :key="item.attack_type" class="bar-item">
            <div class="bar-head">
              <span>{{ item.attack_type || '未知' }}</span>
              <span class="bar-num">{{ item.num }}</span>
            </div>
            <div class="bar-track">
              <div class="bar-fill" :style="{ width: `${Math.max(6, (item.num / maxTypeNum) * 100)}%` }"></div>
            </div>
          </div>
          <div v-if="!topTypes.length" class="waf-empty" style="color: #64748b">暂无数据</div>
        </div>
      </div>

      <!-- 中间地图 -->
      <div class="screen-col screen-col--center">
        <div class="screen-card screen-map">
          <ChartPanel v-if="points.length" :option="mapOption" height="100%" />
          <div v-else class="waf-empty" style="color: #64748b">
            <el-icon><Location /></el-icon> 暂无可定位的攻击来源
          </div>
        </div>
        <div class="screen-card" style="padding: 10px 14px 4px">
          <ChartPanel :option="trendOption" height="120px" />
        </div>
      </div>

      <!-- 右侧排行 -->
      <div class="screen-col screen-col--right">
        <div class="screen-card">
          <div class="screen-card-title">来源地区 Top10</div>
          <div class="rank-list">
            <div v-for="(item, index) in topCountries" :key="item.name" class="rank-row">
              <span class="rank-index">{{ index + 1 }}</span>
              <span class="rank-name">{{ item.name }}</span>
              <span class="rank-tag" :class="{ warn: item.count > 0 }">{{ item.count }}</span>
            </div>
            <div v-if="!topCountries.length" class="waf-empty" style="color: #64748b">暂无数据</div>
          </div>
        </div>

        <div class="screen-card" style="flex: 1; min-height: 0">
          <div class="screen-card-title">实时拦截</div>
          <div class="ticker">
            <div v-for="item in recent" :key="item.id" class="ticker-row">
              <span class="ticker-time">{{ (item.local_time || '').slice(5, 16) }}</span>
              <span class="ticker-ip waf-mono">{{ item.client_ip }}</span>
              <span class="ticker-type">{{ item.attack_type }}</span>
            </div>
            <div v-if="!recent.length" class="waf-empty" style="color: #64748b">暂无拦截记录</div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.screen {
  min-height: 100%;
  padding: 18px 22px 26px;
  background: radial-gradient(1200px 500px at 50% -10%, rgba(79, 70, 229, 0.25), transparent 65%),
    linear-gradient(160deg, #070c18 0%, #0a1120 60%, #080d18 100%);
  color: #e2e8f6;
}

.screen-head {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 16px;
  flex-wrap: wrap;
}

.screen-title {
  margin: 0;
  font-size: 20px;
  font-weight: 750;
  letter-spacing: 0.6px;
  background: linear-gradient(120deg, #c7d2fe, #a5b4fc 45%, #7dd3fc);
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
}

.screen-sub {
  margin: 6px 0 0;
  font-size: 12px;
  color: #7f8caa;
}

.screen-body {
  display: grid;
  grid-template-columns: 300px minmax(0, 1fr) 300px;
  gap: 14px;
  height: calc(100vh - 190px);
  min-height: 560px;
}

.screen-col {
  display: flex;
  flex-direction: column;
  gap: 14px;
  min-height: 0;
}

.screen-card {
  background: rgba(19, 29, 49, 0.72);
  border: 1px solid rgba(99, 102, 241, 0.22);
  border-radius: 14px;
  padding: 14px 16px;
  backdrop-filter: blur(6px);
  box-shadow: 0 20px 40px -30px rgba(0, 0, 0, 0.9);
  min-height: 0;
}

.screen-card-title {
  font-size: 13px;
  font-weight: 650;
  color: #c7d2fe;
  margin-bottom: 12px;
  padding-left: 8px;
  border-left: 3px solid #6366f1;
  line-height: 1.2;
}

.screen-map {
  flex: 1;
  min-height: 320px;
  padding: 4px;
  display: flex;
  overflow: hidden;
}

.metric-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
  margin-bottom: 10px;
}

.metric {
  background: linear-gradient(140deg, rgba(99, 102, 241, 0.18), rgba(15, 23, 42, 0.2));
  border: 1px solid rgba(99, 102, 241, 0.2);
  border-radius: 10px;
  padding: 10px 12px;
}

.metric-value {
  font-size: 22px;
  font-weight: 750;
  color: #e8edfb;
}

.metric-label {
  font-size: 11.5px;
  color: #8593ad;
  margin-top: 2px;
}

.bar-item {
  margin-bottom: 10px;
}

.bar-head {
  display: flex;
  justify-content: space-between;
  font-size: 12px;
  color: #b6c2d9;
  margin-bottom: 4px;
}

.bar-num {
  color: #a5b4fc;
  font-weight: 650;
}

.bar-track {
  height: 6px;
  border-radius: 999px;
  background: rgba(148, 163, 184, 0.15);
  overflow: hidden;
}

.bar-fill {
  height: 100%;
  border-radius: 999px;
  background: linear-gradient(90deg, #6366f1, #22d3ee);
  transition: width 0.5s ease;
}

.rank-list,
.ticker {
  display: grid;
  gap: 7px;
  overflow-y: auto;
}

.rank-row {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12.5px;
  color: #c3cddf;
  padding: 4px 6px;
  border-radius: 8px;
  background: rgba(148, 163, 184, 0.06);
}

.rank-index {
  width: 18px;
  text-align: center;
  color: #7f8caa;
  font-size: 11px;
}

.rank-name {
  flex: 1;
}

.rank-tag {
  font-family: ui-monospace, Menlo, monospace;
  font-size: 11.5px;
  color: #93a2bd;
}

.rank-tag.warn {
  color: #fb923c;
}

.ticker-row {
  display: flex;
  gap: 8px;
  align-items: center;
  font-size: 12px;
  color: #b6c2d9;
  padding: 5px 6px;
  border-bottom: 1px dashed rgba(148, 163, 184, 0.12);
}

.ticker-time {
  color: #6f7d99;
  font-family: ui-monospace, Menlo, monospace;
  font-size: 11px;
}

.ticker-ip {
  color: #a5b4fc;
}

.ticker-type {
  margin-left: auto;
  color: #f0abfc;
  font-size: 11.5px;
}

@media (max-width: 1280px) {
  .screen-body {
    grid-template-columns: minmax(0, 1fr);
    height: auto;
  }
  .screen-map {
    height: 420px;
  }
}

@media (max-width: 900px) {
  .screen {
    padding: 14px 12px 20px;
  }
  .screen-head {
    gap: 10px;
    margin-bottom: 12px;
  }
  .screen-title {
    font-size: 17px;
  }
  .screen-map {
    height: 320px;
  }
  .metric-value {
    font-size: 20px;
  }
  .screen-card {
    padding: 12px;
  }
}

@media (max-width: 640px) {
  .screen {
    padding: 12px 10px 18px;
  }
  .screen-map {
    height: 240px;
  }
  .metric-row {
    gap: 8px;
  }
  .metric-value {
    font-size: 18px;
  }
  .metric-label {
    font-size: 11px;
  }
}</style>
