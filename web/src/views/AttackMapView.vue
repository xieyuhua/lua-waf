<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import * as echarts from 'echarts'
import worldGeo from '../assets/world.geo.json'
import ChartPanel from '../components/ChartPanel.vue'
import { dashboardApi } from '../api/dashboard'
import { themeStore } from '../stores/theme'

const loading = ref(true)
const snapshot = ref(null)
const mapRef = ref(null)
// 地图缩放/平移状态（故意用普通对象：georoam 时不触发重渲染，数据刷新时按此还原视野）
const DEFAULT_ZOOM = 1.05
const MIN_ZOOM = 1
const MAX_ZOOM = 8
const DEFAULT_CENTER = [12, 18]
const roamState = { zoom: DEFAULT_ZOOM, center: DEFAULT_CENTER }
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

/** 攻击来源 Top N（用于绘制箭头，避免线条过多） */
const arrowPoints = computed(() => points.value.slice(0, 12))

const palette = computed(() => themeStore.chartPalette.value)
const dark = computed(() => themeStore.dark.value)

/** 状态与目标 */
const status = computed(() => snapshot.value?.status ?? { level: 'normal', message: '暂无数据', today_count: 0, window_count: 0 })
const target = computed(() => snapshot.value?.target ?? null)
const statusText = computed(() => {
  const level = status.value.level
  if (level === 'high') return '高危'
  if (level === 'elevated') return '告警'
  return '正常'
})

/** 随主题变化的配色 */
const colors = computed(() => {
  const isDark = dark.value
  return {
    area: isDark ? 'rgba(23,35,58,0.92)' : '#e9eefb',
    areaHover: isDark ? 'rgba(99,102,241,0.42)' : 'rgba(99,102,241,0.24)',
    border: isDark ? 'rgba(99,102,241,0.32)' : 'rgba(99,102,241,0.28)',
    label: isDark ? '#cbd5e1' : '#475569',
    axis: isDark ? 'rgba(148,163,184,0.35)' : 'rgba(100,116,139,0.3)',
    tooltipBg: isDark ? 'rgba(9,15,28,0.94)' : 'rgba(255,255,255,0.96)',
    tooltipText: isDark ? '#e2e8f0' : '#1f2937'
  }
})

/** 世界地图 + 攻击箭头（来源 → 防护目标） */
const mapOption = computed(() => {
  const theme = colors.value
  const accent = palette.value.primary
  const accent2 = palette.value.accent

  const targetPoint = target.value
  const arrowData = targetPoint
    ? arrowPoints.value.map((item) => ({
        name: item.name,
        coords: [
          [item.lon, item.lat],
          [targetPoint.lon, targetPoint.lat]
        ],
        value: item.count,
        area: item.area
      }))
    : []

  return {
    backgroundColor: 'transparent',
    tooltip: {
      trigger: 'item',
      backgroundColor: theme.tooltipBg,
      borderColor: 'rgba(99,102,241,0.45)',
      textStyle: { color: theme.tooltipText, fontSize: 12 },
      formatter: (params) => {
        if (params.seriesName === '攻击链路') {
          return `<strong>${params.name} → ${targetPoint?.name || '防护目标'}</strong><br/>攻击次数：${params.data?.value ?? 0}`
        }
        if (params.seriesName === '防护目标') {
          return `<strong>${targetPoint?.name}</strong><br/>位置：${targetPoint?.area || '-'}`
        }
        const value = params.value || []
        return `<strong>${params.name}</strong><br/>攻击次数：${value[2] ?? 0}<br/>来源：${params.data?.area || '-'}<br/>类型：${
          (params.data?.attack_types || []).join('、') || '-'
        }`
      }
    },
    geo: {
      map: 'world',
      // 开启缩放与拖拽：滚轮 / 双指缩放，按住拖动平移
      roam: true,
      scaleLimit: { min: MIN_ZOOM, max: MAX_ZOOM },
      zoom: roamState.zoom,
      center: roamState.center,
      itemStyle: {
        areaColor: theme.area,
        borderColor: theme.border,
        borderWidth: 0.6
      },
      emphasis: {
        itemStyle: { areaColor: theme.areaHover },
        label: { show: false }
      },
      silent: true
    },
    series: [
      {
        name: '攻击链路',
        type: 'lines',
        coordinateSystem: 'geo',
        zlevel: 1,
        effect: {
          show: true,
          period: 3.2,
          trailLength: 0.28,
          symbol: 'arrow',
          symbolSize: 7,
          color: accent2
        },
        lineStyle: {
          color: accent,
          width: 1.2,
          opacity: 0.55,
          curveness: 0.25
        },
        emphasis: { lineStyle: { width: 2.4, opacity: 0.9 } },
        data: arrowData
      },
      {
        name: '攻击来源',
        type: 'effectScatter',
        coordinateSystem: 'geo',
        zlevel: 2,
        rippleEffect: { brushType: 'stroke', scale: 3.2, period: 4 },
        symbolSize: (value) => Math.min(6 + Math.sqrt(value[2] || 1) * 2, 26),
        itemStyle: { color: '#f97316', shadowBlur: 14, shadowColor: 'rgba(249,115,22,0.85)' },
        label: {
          show: true,
          position: 'right',
          formatter: '{b}',
          color: theme.label,
          fontSize: 10
        },
        data: points.value.map((item) => ({
          name: item.name,
          value: [item.lon, item.lat, item.count],
          area: item.area,
          attack_types: item.attack_types
        }))
      },
      {
        name: '防护目标',
        type: 'scatter',
        coordinateSystem: 'geo',
        zlevel: 3,
        symbol: 'pin',
        symbolSize: 24,
        itemStyle: { color: accent, shadowBlur: 16, shadowColor: `${accent}aa` },
        label: {
          show: true,
          position: 'top',
          formatter: () => targetPoint?.name || '防护目标',
          color: theme.label,
          fontSize: 11,
          fontWeight: 600
        },
        data: targetPoint ? [{ name: targetPoint.name, value: [targetPoint.lon, targetPoint.lat, 0] }] : []
      }
    ]
  }
})

const trendOption = computed(() => {
  const trend = snapshot.value?.trend ?? []
  const theme = colors.value
  return {
    grid: { left: 8, right: 12, top: 16, bottom: 4, containLabel: true },
    tooltip: { trigger: 'axis' },
    xAxis: {
      type: 'category',
      data: trend.map((item) => item.date.slice(5)),
      axisLine: { lineStyle: { color: theme.axis } },
      axisLabel: { color: theme.label, fontSize: 10 }
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

/** 绑定地图缩放事件，记录当前视野 */
function attachRoam() {
  const chart = mapRef.value?.getInstance()
  if (!chart) return
  chart.off('georoam')
  chart.on('georoam', () => {
    const geo = chart.getOption()?.geo?.[0]
    if (!geo) return
    roamState.zoom = geo.zoom ?? DEFAULT_ZOOM
    roamState.center = geo.center ?? DEFAULT_CENTER
  })
}

/** 按倍率缩放地图（>1 放大，<1 缩小）；以当前视野中心为基准 */
function zoomBy(factor) {
  const chart = mapRef.value?.getInstance()
  if (!chart) return

  const geo = chart.getOption()?.geo?.[0]
  const current = geo?.zoom ?? roamState.zoom
  const zoom = Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, Number((current * factor).toFixed(3))))
  roamState.zoom = zoom
  roamState.center = geo?.center ?? roamState.center

  chart.setOption({ geo: { zoom, center: roamState.center } })
}

/** 重置视野 */
function resetZoom() {
  roamState.zoom = DEFAULT_ZOOM
  roamState.center = DEFAULT_CENTER
  const chart = mapRef.value?.getInstance()
  if (!chart) return
  chart.setOption({ geo: { zoom: DEFAULT_ZOOM, center: DEFAULT_CENTER } })
}

// 数据到达后重新绑定（ChartPanel 首次渲染完成后实例才可用）
watch(points, async () => {
  await nextTick()
  attachRoam()
})

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
  <div class="screen" :class="dark ? 'is-dark' : 'is-light'">
    <div class="screen-head">
      <div>
        <h1 class="screen-title">攻击地图 · 实时态势</h1>
        <p class="screen-sub">
          统计范围 {{ snapshot?.range || '-' }} · 更新于 {{ snapshot?.updated_at || '-' }}
        </p>
      </div>
      <div style="display: flex; align-items: center; gap: 10px; flex-wrap: wrap">
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

    <!-- 攻击状态条 -->
    <div class="status-strip" :class="`is-${status.level}`">
      <span class="status-badge">
        <span class="status-dot"></span>
        {{ statusText }}
      </span>
      <span class="status-message">{{ status.message }}</span>
      <span class="status-facts">
        <span class="fact"><b>{{ status.today_count || 0 }}</b> 今日攻击</span>
        <span class="fact"><b>{{ status.window_count || 0 }}</b> 近 5 分钟</span>
        <span class="fact"><b>{{ (status.rate_per_min || 0).toFixed(1) }}</b> 次/分</span>
        <span class="fact">最近攻击 <b>{{ status.last_attack || '-' }}</b></span>
        <span class="fact target">防护目标 <b>{{ target?.name || '-' }}</b></span>
        <span v-if="snapshot?.unresolved" class="fact" title="归属地无法解析（如「保留」「内网IP」）">未定位 <b>{{ snapshot.unresolved }}</b> 个来源</span>
      </span>
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
          <div class="arrow-note">
            箭头表示「来源 → 防护目标」的攻击链路，线宽随攻击次数增加
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
          <div v-if="!topTypes.length" class="empty-hint">暂无数据</div>
        </div>
      </div>

      <!-- 中间地图 -->
      <div class="screen-col screen-col--center">
        <div class="screen-card screen-map">
          <div v-if="points.length" class="map-tools">
            <el-button-group>
              <el-button size="small" title="放大" @click="zoomBy(1.35)"><el-icon><ZoomIn /></el-icon></el-button>
              <el-button size="small" title="缩小" @click="zoomBy(1 / 1.35)"><el-icon><ZoomOut /></el-icon></el-button>
              <el-button size="small" title="重置视野" @click="resetZoom"><el-icon><RefreshLeft /></el-icon></el-button>
            </el-button-group>
            <span class="map-hint">滚轮 / 双指缩放 · 拖拽平移</span>
          </div>
          <ChartPanel v-if="points.length" ref="mapRef" :option="mapOption" height="100%" />
          <div v-else class="empty-hint">
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
            <div v-if="!topCountries.length" class="empty-hint">暂无数据</div>
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
            <div v-if="!recent.length" class="empty-hint">暂无拦截记录</div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* 主题变量：亮色为默认，暗色由 html.dark 覆盖 */
.screen {
  --map-bg: radial-gradient(1200px 500px at 50% -10%, rgba(99, 102, 241, 0.12), transparent 65%),
    linear-gradient(160deg, #f5f7fe 0%, #eef2fb 60%, #f7f9ff 100%);
  --map-card-bg: rgba(255, 255, 255, 0.86);
  --map-border: rgba(99, 102, 241, 0.18);
  --map-text: #1f2937;
  --map-text-muted: #5b6779;
  --map-title: var(--waf-primary);
  --map-bar-track: rgba(99, 102, 241, 0.12);
  --map-shadow: 0 16px 32px -28px rgba(15, 23, 42, 0.5);

  min-height: 100%;
  padding: 18px 22px 26px;
  background: var(--map-bg);
  color: var(--map-text);
}

.screen.is-dark {
  --map-bg: radial-gradient(1200px 500px at 50% -10%, rgba(79, 70, 229, 0.25), transparent 65%),
    linear-gradient(160deg, #070c18 0%, #0a1120 60%, #080d18 100%);
  --map-card-bg: rgba(19, 29, 49, 0.72);
  --map-border: rgba(99, 102, 241, 0.22);
  --map-text: #e2e8f6;
  --map-text-muted: #7f8caa;
  --map-title: #c7d2fe;
  --map-bar-track: rgba(99, 102, 241, 0.16);
  --map-shadow: 0 20px 40px -30px rgba(0, 0, 0, 0.9);
}

.screen-head {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 14px;
  flex-wrap: wrap;
}

.screen-title {
  margin: 0;
  font-size: 20px;
  font-weight: 750;
  letter-spacing: 0.6px;
  color: var(--map-title);
}

.screen-sub {
  margin: 6px 0 0;
  font-size: 12px;
  color: var(--map-text-muted);
}

/* 攻击状态条 */
.status-strip {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  padding: 10px 14px;
  margin-bottom: 14px;
  border-radius: 12px;
  border: 1px solid var(--map-border);
  background: var(--map-card-bg);
  box-shadow: var(--map-shadow);
  font-size: 12.5px;
}

.status-badge {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  padding: 3px 10px;
  border-radius: 999px;
  font-weight: 700;
  color: #fff;
  background: #22c55e;
}

.status-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: #fff;
  box-shadow: 0 0 0 0 rgba(255, 255, 255, 0.7);
  animation: pulse 1.8s infinite;
}

@keyframes pulse {
  0% {
    box-shadow: 0 0 0 0 rgba(255, 255, 255, 0.6);
  }
  70% {
    box-shadow: 0 0 0 8px rgba(255, 255, 255, 0);
  }
  100% {
    box-shadow: 0 0 0 0 rgba(255, 255, 255, 0);
  }
}

.is-elevated .status-badge {
  background: #f59e0b;
}

.is-high .status-badge {
  background: #ef4444;
}

.status-message {
  color: var(--map-text);
  font-weight: 600;
}

.status-facts {
  display: flex;
  gap: 14px;
  flex-wrap: wrap;
  margin-left: auto;
  color: var(--map-text-muted);
}

.status-facts b {
  color: var(--map-text);
  font-weight: 700;
}

.status-facts .target b {
  color: var(--waf-primary);
}

.screen-body {
  display: grid;
  grid-template-columns: 300px minmax(0, 1fr) 300px;
  gap: 14px;
  height: calc(100vh - 240px);
  min-height: 520px;
}

.screen-col {
  display: flex;
  flex-direction: column;
  gap: 14px;
  min-height: 0;
}

.screen-card {
  background: var(--map-card-bg);
  border: 1px solid var(--map-border);
  border-radius: 14px;
  padding: 14px 16px;
  backdrop-filter: blur(6px);
  box-shadow: var(--map-shadow);
  min-height: 0;
}

.screen-card-title {
  font-size: 13px;
  font-weight: 650;
  color: var(--map-title);
  margin-bottom: 12px;
  padding-left: 8px;
  border-left: 3px solid var(--waf-primary);
  line-height: 1.2;
}

.screen-map {
  position: relative;
  flex: 1;
  min-height: 320px;
  padding: 4px;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.map-tools {
  position: absolute;
  top: 10px;
  right: 12px;
  z-index: 5;
  display: flex;
  align-items: center;
  gap: 8px;
}

.map-hint {
  font-size: 11px;
  color: var(--map-text-muted);
  background: var(--map-card-bg);
  border: 1px solid var(--map-border);
  border-radius: 999px;
  padding: 2px 9px;
  white-space: nowrap;
}

@media (max-width: 640px) {
  .map-hint {
    display: none;
  }
}
.metric-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
  margin-bottom: 10px;
}

.metric {
  background: color-mix(in srgb, var(--waf-primary) 12%, transparent);
  border: 1px solid var(--map-border);
  border-radius: 10px;
  padding: 10px 12px;
}

.metric-value {
  font-size: 22px;
  font-weight: 750;
  color: var(--map-text);
}

.metric-label {
  font-size: 11.5px;
  color: var(--map-text-muted);
  margin-top: 2px;
}

.arrow-note {
  margin-top: 4px;
  font-size: 11px;
  line-height: 1.6;
  color: var(--map-text-muted);
}

.bar-item {
  margin-bottom: 10px;
}

.bar-head {
  display: flex;
  justify-content: space-between;
  font-size: 12px;
  color: var(--map-text-muted);
  margin-bottom: 4px;
}

.bar-num {
  color: var(--waf-primary);
  font-weight: 650;
}

.bar-track {
  height: 6px;
  border-radius: 999px;
  background: var(--map-bar-track);
  overflow: hidden;
}

.bar-fill {
  height: 100%;
  border-radius: 999px;
  background: linear-gradient(90deg, var(--waf-primary), var(--waf-accent));
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
  color: var(--map-text);
  padding: 4px 6px;
  border-radius: 8px;
  background: color-mix(in srgb, var(--waf-primary) 7%, transparent);
}

.rank-index {
  width: 18px;
  text-align: center;
  color: var(--map-text-muted);
  font-size: 11px;
}

.rank-name {
  flex: 1;
}

.rank-tag {
  font-family: ui-monospace, Menlo, monospace;
  font-size: 11.5px;
  color: var(--map-text-muted);
}

.rank-tag.warn {
  color: #fb923c;
}

.ticker-row {
  display: flex;
  gap: 8px;
  align-items: center;
  font-size: 12px;
  color: var(--map-text);
  padding: 5px 6px;
  border-bottom: 1px dashed var(--map-border);
}

.ticker-time {
  color: var(--map-text-muted);
  font-family: ui-monospace, Menlo, monospace;
  font-size: 11px;
}

.ticker-ip {
  color: var(--waf-primary);
}

.ticker-type {
  margin-left: auto;
  color: var(--waf-accent);
  font-size: 11.5px;
}

.empty-hint {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 14px 0;
  font-size: 12.5px;
  color: var(--map-text-muted);
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
  .status-facts {
    margin-left: 0;
    gap: 10px;
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
}
</style>