<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import StatCard from '../components/StatCard.vue'
import ChartPanel from '../components/ChartPanel.vue'
import { dashboardApi } from '../api/dashboard'
import { systemApi } from '../api/system'
import { themeStore } from '../stores/theme'

const loading = ref(true)
const summary = ref(null)

// 日志入库队列（lua 写 Redis、Go 消费入库）的实时指标
const ingest = ref(null)
// 队列深度采样，用于画 5 分钟走势
const ingestSamples = ref([])
let ingestTimer = null

/** 拉取入库队列指标（失败静默，不影响看板其它数据） */
async function loadIngest() {
  try {
    const data = await systemApi.ingest()
    ingest.value = data
    if (data?.enabled) {
      const now = new Date()
      const label = `${String(now.getHours()).padStart(2, '0')}:${String(now.getMinutes()).padStart(2, '0')}:${String(now.getSeconds()).padStart(2, '0')}`
      const samples = [...ingestSamples.value, { label, queue: data.queue_length ?? 0, processing: data.processing_length ?? 0 }]
      ingestSamples.value = samples.slice(-30)
    }
  } catch (error) {
    /* 静默：入库队列不可用不影响看板 */
  }
}

/** 加载看板数据 */
async function load() {
  loading.value = true
  try {
    summary.value = await dashboardApi.summary()
    await loadIngest()
  } catch (error) {
    ElMessage.error(error.message || '加载看板失败')
  } finally {
    loading.value = false
  }
}

const palette = computed(() => themeStore.chartPalette.value)

/** 入库队列状态徽标 */
const ingestStatus = computed(() => {
  const data = ingest.value
  if (!data) return { text: '未知', type: 'info' }
  if (!data.enabled) return { text: '未启用', type: 'info' }
  if (!data.connected) return { text: 'Redis 断开', type: 'danger' }
  if ((data.queue_length ?? 0) > 10000) return { text: '队列积压', type: 'danger' }
  if ((data.queue_length ?? 0) > 1000) return { text: '轻微积压', type: 'warning' }
  return { text: '运行中', type: 'success' }
})

/** 队列深度走势 */
const ingestTrendOption = computed(() => {
  const samples = ingestSamples.value
  return {
    grid: { left: 8, right: 12, top: 24, bottom: 4, containLabel: true },
    tooltip: { trigger: 'axis' },
    legend: { show: true, right: 0, top: 0, itemWidth: 10, itemHeight: 8, textStyle: { fontSize: 11 } },
    xAxis: {
      type: 'category',
      data: samples.map((item) => item.label),
      boundaryGap: false,
      axisLine: { lineStyle: { color: 'rgba(148,163,184,0.35)' } },
      axisLabel: { color: '#8f9bb3', fontSize: 10 }
    },
    yAxis: { type: 'value', splitLine: { lineStyle: { color: 'rgba(148,163,184,0.16)' } }, axisLabel: { color: '#8f9bb3', fontSize: 10 } },
    series: [
      {
        name: '待处理',
        type: 'line',
        smooth: true,
        showSymbol: false,
        areaStyle: { opacity: 0.18 },
        lineStyle: { width: 2, color: palette.value.accent },
        itemStyle: { color: palette.value.accent },
        data: samples.map((item) => item.queue)
      },
      {
        name: '处理中',
        type: 'line',
        smooth: true,
        showSymbol: false,
        lineStyle: { width: 2, color: '#f59e0b' },
        itemStyle: { color: '#f59e0b' },
        data: samples.map((item) => item.processing)
      }
    ]
  }
})

const trendOption = computed(() => {
  const trend = summary.value?.trend ?? []
  return {
    grid: { left: 10, right: 18, top: 30, bottom: 6, containLabel: true },
    tooltip: { trigger: 'axis', backgroundColor: 'rgba(15,23,42,0.9)', borderWidth: 0, textStyle: { color: '#e2e8f0' } },
    xAxis: {
      type: 'category',
      data: trend.map((item) => item.date.slice(5)),
      boundaryGap: false,
      axisLine: { lineStyle: { color: 'rgba(148,163,184,0.35)' } },
      axisLabel: { color: '#8f9bb3', fontSize: 11 }
    },
    yAxis: {
      type: 'value',
      splitLine: { lineStyle: { color: 'rgba(148,163,184,0.16)' } },
      axisLabel: { color: '#8f9bb3', fontSize: 11 }
    },
    series: [
      {
        name: '攻击次数',
        type: 'line',
        smooth: true,
        symbol: 'circle',
        symbolSize: 6,
        data: trend.map((item) => item.num),
        lineStyle: { width: 3, color: palette.value.primary },
        itemStyle: { color: palette.value.primary },
        areaStyle: {
          color: {
            type: 'linear',
            x: 0,
            y: 0,
            x2: 0,
            y2: 1,
            colorStops: [
              { offset: 0, color: `${palette.value.primary}59` },
              { offset: 1, color: `${palette.value.primary}05` }
            ]
          }
        }
      }
    ]
  }
})

const typeOption = computed(() => {
  const items = summary.value?.today_types ?? []
  return {
    tooltip: { trigger: 'item' },
    legend: { bottom: 0, icon: 'circle', textStyle: { color: '#8f9bb3', fontSize: 11 }, itemWidth: 8, itemHeight: 8 },
    color: palette.value.series,
    series: [
      {
        name: '今日攻击类型',
        type: 'pie',
        radius: ['52%', '74%'],
        center: ['50%', '44%'],
        avoidLabelOverlap: true,
        itemStyle: { borderColor: 'transparent', borderWidth: 3, borderRadius: 6 },
        label: { show: false },
        data: items.map((item) => ({ name: item.attack_type, value: item.num }))
      }
    ]
  }
})

const topIPs = computed(() => (summary.value?.today_ips ?? []).slice(0, 5))
const recentLogs = computed(() => summary.value?.recent_logs ?? [])
const recentSlow = computed(() => summary.value?.recent_slow_sql ?? [])

onMounted(() => {
  load()
  ingestTimer = setInterval(loadIngest, 10000)
})

onBeforeUnmount(() => {
  if (ingestTimer) clearInterval(ingestTimer)
})
</script>

<template>
  <div class="waf-page">
    <div class="waf-page-head">
      <div>
        <h1 class="waf-page-title"><span class="dot"></span> 安全概览</h1>
        <p class="waf-page-sub">
          数据截至 {{ summary?.generated_at || '-' }} · 统计口径：openresty 侧写入的 waf_log
        </p>
      </div>
      <el-button :loading="loading" @click="load">
        <el-icon><Refresh /></el-icon> 刷新
      </el-button>
    </div>

    <el-skeleton v-if="loading && !summary" :rows="8" animated />

    <template v-else>
      <div class="waf-grid waf-grid--4" style="margin-bottom: 16px">
        <StatCard label="今日拦截" :value="summary?.logs_today ?? 0" icon="Warning" color="linear-gradient(135deg,#f97316,#ef4444)" hint="当日命中规则数" />
        <StatCard label="累计攻击" :value="summary?.logs_total ?? 0" icon="DataLine" color="linear-gradient(135deg,#6366f1,#8b5cf6)" hint="全部历史记录" />
        <StatCard label="保护站点" :value="summary?.sites ?? 0" icon="Cloudy" color="linear-gradient(135deg,#0ea5e9,#22d3ee)" hint="反向代理后端" />
        <StatCard label="WAF 规则" :value="summary?.rules ?? 0" icon="Lock" color="linear-gradient(135deg,#10b981,#34d399)" hint="启用中的规则条目" />
      </div>

      <div class="waf-grid waf-grid--2" style="margin-bottom: 16px">
        <div class="waf-card">
          <div class="waf-card-head">
            <div class="waf-card-title"><span class="bar"></span> 近 15 天攻击趋势</div>
            <div class="waf-card-extra">单位：次</div>
          </div>
          <ChartPanel :option="trendOption" height="260px" />
        </div>

        <div class="waf-card">
          <div class="waf-card-head">
            <div class="waf-card-title"><span class="bar"></span> 今日攻击类型分布</div>
            <div class="waf-card-extra">今日 {{ summary?.logs_today ?? 0 }} 次</div>
          </div>
          <ChartPanel v-if="(summary?.today_types ?? []).length" :option="typeOption" height="260px" />
          <div v-else class="waf-empty">今日暂无攻击记录</div>
        </div>
      </div>

      <!-- 日志入库队列：lua 写 Redis、Go 消费入库 -->
      <div class="waf-card" style="margin-bottom: 16px">
        <div class="waf-card-head">
          <div class="waf-card-title"><span class="bar"></span> 日志入库队列</div>
          <div style="display: flex; align-items: center; gap: 10px">
            <el-tag size="small" :type="ingestStatus.type" effect="light">{{ ingestStatus.text }}</el-tag>
            <span class="waf-muted" style="font-size: 11.5px">每 10 秒刷新</span>
          </div>
        </div>

        <div class="ingest-metrics">
          <div class="ingest-metric">
            <div class="ingest-value">{{ ingest?.queue_length ?? 0 }}</div>
            <div class="ingest-label">待处理（Redis 队列）</div>
          </div>
          <div class="ingest-metric">
            <div class="ingest-value">{{ ingest?.processing_length ?? 0 }}</div>
            <div class="ingest-label">处理中（入库中）</div>
          </div>
          <div class="ingest-metric">
            <div class="ingest-value">{{ ingest?.written ?? 0 }}</div>
            <div class="ingest-label">本次启动已入库</div>
          </div>
          <div class="ingest-metric">
            <div class="ingest-value" :style="{ color: (ingest?.failed ?? 0) > 0 ? 'var(--waf-danger)' : 'inherit' }">
              {{ ingest?.failed ?? 0 }}
            </div>
            <div class="ingest-label">失败 / 丢弃</div>
          </div>
        </div>

        <div class="ingest-meta">
          <span>队列键 <code class="waf-mono">{{ ingest?.source || '-' }}</code></span>
          <span v-if="ingest?.last_flush_at">最近入库 {{ ingest.last_flush_at }}</span>
          <span v-if="ingest?.last_error" class="ingest-error" :title="ingest.last_error">
            最近错误：{{ ingest.last_error.slice(0, 80) }}
          </span>
        </div>

        <ChartPanel
          v-if="ingest?.enabled && ingestSamples.length > 1"
          :option="ingestTrendOption"
          height="150px"
        />

        <el-alert
          v-if="ingest && !ingest.enabled"
          type="info"
          show-icon
          :closable="false"
          style="margin-top: 10px"
          title="未启用「lua 写 Redis → Go 消费入库」"
          description="启用需要三处配套：① config.lua 的 config_log_mode = redis；② config.lua 的 config_redis_consume = off（避免与 nginx 抢队列）；③ conf/app.ini 的 [log] INGEST_ENABLED = on 并填 REDIS_ADDR。当前使用 file / nginx 直连 MySQL 模式时该卡片为 0 属正常。"
        />
      </div>
      <div class="waf-grid waf-grid--3">
        <div class="waf-card">
          <div class="waf-card-head">
            <div class="waf-card-title"><span class="bar"></span> 今日攻击来源 Top5</div>
          </div>
          <div v-if="topIPs.length" class="ip-list">
            <div v-for="(item, index) in topIPs" :key="item.client_ip" class="ip-item">
              <span class="ip-rank" :class="`rank-${index + 1}`">{{ index + 1 }}</span>
              <div style="min-width: 0; flex: 1">
                <div class="waf-mono">{{ item.client_ip }}</div>
                <div class="waf-muted" style="font-size: 11.5px">{{ item.area || '未知地区' }}</div>
              </div>
              <el-tag size="small" type="danger" effect="dark" round>{{ item.num }} 次</el-tag>
            </div>
          </div>
          <div v-else class="waf-empty">今日暂无来源数据</div>
        </div>

        <div class="waf-card">
          <div class="waf-card-head">
            <div class="waf-card-title"><span class="bar"></span> 最近慢 SQL</div>
            <div class="waf-card-extra">共 {{ summary?.slow_sql_total ?? 0 }} 条</div>
          </div>
          <div v-if="recentSlow.length" class="slow-list">
            <div v-for="item in recentSlow" :key="item.id" class="slow-item">
              <el-tag size="small" :type="item.duration_ms >= 1000 ? 'danger' : 'warning'" effect="light" round>
                {{ item.duration_ms }}ms
              </el-tag>
              <span class="waf-mono slow-text">{{ item.statement }}</span>
            </div>
          </div>
          <div v-else class="waf-empty">暂无慢 SQL 记录</div>
        </div>

        <div class="waf-card">
          <div class="waf-card-head">
            <div class="waf-card-title"><span class="bar"></span> 最新拦截</div>
            <router-link to="/logs" style="font-size: 12px">查看全部</router-link>
          </div>
          <div v-if="recentLogs.length" class="log-list">
            <div v-for="item in recentLogs" :key="item.id" class="log-item">
              <el-tag size="small" type="info">{{ item.attack_type || '未知' }}</el-tag>
              <span class="waf-mono" style="flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap">
                {{ item.client_ip }} → {{ item.req_url }}
              </span>
              <span class="waf-muted" style="font-size: 11.5px">{{ (item.local_time || '').slice(11) }}</span>
            </div>
          </div>
          <div v-else class="waf-empty">暂无拦截记录</div>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.ingest-metrics {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 10px;
}

.ingest-metric {
  padding: 10px 12px;
  border-radius: 10px;
  border: 1px solid var(--waf-border);
  background: var(--waf-surface-2);
}

.ingest-value {
  font-size: 22px;
  font-weight: 750;
  color: var(--waf-text);
  line-height: 1.2;
}

.ingest-label {
  font-size: 11.5px;
  color: var(--waf-text-muted);
  margin-top: 2px;
}

.ingest-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 16px;
  font-size: 11.5px;
  color: var(--waf-text-muted);
  margin-bottom: 6px;
}

.ingest-meta code {
  font-size: 11px;
}

.ingest-error {
  color: var(--waf-danger);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 100%;
}

@media (max-width: 900px) {
  .ingest-metrics {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
.ip-list,
.slow-list,
.log-list {
  display: grid;
  gap: 10px;
}

.ip-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 10px;
  border-radius: 10px;
  background: var(--waf-surface-2);
}

.ip-rank {
  width: 22px;
  height: 22px;
  border-radius: 7px;
  display: grid;
  place-items: center;
  font-size: 11px;
  font-weight: 700;
  background: rgba(148, 163, 184, 0.2);
  color: var(--waf-text-muted);
}

.rank-1 {
  background: linear-gradient(135deg, #f97316, #ef4444);
  color: #fff;
}
.rank-2 {
  background: linear-gradient(135deg, #f59e0b, #fbbf24);
  color: #fff;
}
.rank-3 {
  background: linear-gradient(135deg, #38bdf8, #22d3ee);
  color: #fff;
}

.slow-item {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.slow-text {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--waf-text-muted);
}

.log-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 7px 0;
  border-bottom: 1px dashed var(--waf-border);
  min-width: 0;
}

.log-item:last-child {
  border-bottom: none;
}
</style>
