<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import * as echarts from 'echarts'
import worldGeo from '../assets/world.geo.json'
import ChartPanel from '../components/ChartPanel.vue'
import { dashboardApi } from '../api/dashboard'
import { buildColors, buildMapOption, buildTrendOption } from '../composables/useAttackMapCharts'
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
// 默认展示今日数据（可切换为全部）
const range = ref('today')
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
/** 随主题变化的配色 */
const colors = computed(() => buildColors(dark.value))

/** 世界地图 + 攻击箭头（来源 → 防护目标） */
const mapOption = computed(() =>
  buildMapOption({
    points: points.value,
    arrowPoints: arrowPoints.value,
    target: target.value,
    colors: colors.value,
    palette: palette.value,
    roamState,
    minZoom: MIN_ZOOM,
    maxZoom: MAX_ZOOM
  })
)

/** 近 N 日攻击趋势 */
const trendOption = computed(() =>
  buildTrendOption({ trend: snapshot.value?.trend ?? [], colors: colors.value, palette: palette.value })
)
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

<style scoped src="./attackmap.css"></style>