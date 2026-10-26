<script setup>
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import StatCard from '../components/StatCard.vue'
import ChartPanel from '../components/ChartPanel.vue'
import { dashboardApi } from '../api/dashboard'
import { themeStore } from '../stores/theme'

const loading = ref(true)
const summary = ref(null)

/** 加载看板数据 */
async function load() {
  loading.value = true
  try {
    summary.value = await dashboardApi.summary()
  } catch (error) {
    ElMessage.error(error.message || '加载看板失败')
  } finally {
    loading.value = false
  }
}

const palette = computed(() => themeStore.chartPalette.value)

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

onMounted(load)
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
