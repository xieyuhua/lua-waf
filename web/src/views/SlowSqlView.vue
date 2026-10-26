<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { slowSqlApi } from '../api/slowsql'
import { compactStatement, formatDuration, formatNumber, formatTime, toBackendTime } from '../utils/format'
import StatCard from '../components/StatCard.vue'

const loading = ref(false)
const rows = ref([])
const total = ref(0)
const page = ref(1)
const size = ref(20)
const stats = ref(null)

const filters = reactive({
  keyword: '',
  source: '',
  min_ms: undefined,
  range: [],
  sort_by: 'occurred_at',
  order: 'desc'
})

const presets = [
  { label: '近 1 小时', hours: 1 },
  { label: '近 6 小时', hours: 6 },
  { label: '近 24 小时', hours: 24 },
  { label: '近 7 天', hours: 168 },
  { label: '近 30 天', hours: 720 },
  { label: '全部', hours: 0 }
]
const activePreset = ref(3)

const detailVisible = ref(false)
const current = ref(null)

/** 应用预设时间范围 */
function applyPreset(preset, index) {
  activePreset.value = index
  if (!preset.hours) {
    filters.range = []
    return
  }
  const end = new Date()
  const start = new Date(end.getTime() - preset.hours * 3600 * 1000)
  filters.range = [start, end]
}

/** 组装查询参数（时间范围由后端解析） */
function queryParams() {
  const params = {
    page: page.value,
    size: size.value,
    keyword: filters.keyword,
    source: filters.source,
    min_ms: filters.min_ms,
    sort_by: filters.sort_by,
    order: filters.order
  }
  if (Array.isArray(filters.range) && filters.range.length === 2) {
    params.from = toBackendTime(filters.range[0])
    params.to = toBackendTime(filters.range[1])
  }
  return params
}

/** 加载列表与统计 */
async function load() {
  loading.value = true
  try {
    const params = queryParams()
    const [listData, statsData] = await Promise.all([slowSqlApi.list(params), slowSqlApi.stats(params)])
    rows.value = listData.items ?? []
    total.value = listData.total ?? 0
    stats.value = statsData
  } catch (error) {
    ElMessage.error(error.message || '加载慢 SQL 失败')
  } finally {
    loading.value = false
  }
}

/** 重置 */
function reset() {
  Object.assign(filters, { keyword: '', source: '', min_ms: undefined, sort_by: 'occurred_at', order: 'desc' })
  applyPreset(presets[3], 3)
  page.value = 1
  load()
}

/** 切换排序 */
function toggleSort() {
  if (filters.sort_by !== 'duration') {
    filters.sort_by = 'duration'
    filters.order = 'desc'
  } else {
    filters.order = filters.order === 'desc' ? 'asc' : 'desc'
  }
  load()
}

/** 清理历史记录 */
async function purge() {
  try {
    await ElMessageBox.confirm('将删除 30 天之前的慢 SQL 记录，是否继续？', '清理确认', {
      type: 'warning',
      confirmButtonText: '清理',
      cancelButtonText: '取消'
    })
  } catch (error) {
    return
  }
  try {
    const data = await slowSqlApi.purge(30)
    ElMessage.success(`已清理 ${data.removed} 条记录`)
    load()
  } catch (error) {
    ElMessage.error(error.message || '清理失败')
  }
}

/** 打开详情 */
function openDetail(row) {
  current.value = row
  detailVisible.value = true
}

const statCards = computed(() => [
  { label: '记录总数', value: stats.value?.total ?? 0, icon: 'Tickets', color: 'linear-gradient(135deg,#6366f1,#8b5cf6)' },
  { label: '平均耗时', value: formatDuration(stats.value?.avg_ms ?? 0), icon: 'Odometer', color: 'linear-gradient(135deg,#0ea5e9,#22d3ee)' },
  { label: 'P95 耗时', value: formatDuration(stats.value?.p95_ms ?? 0), icon: 'TrendCharts', color: 'linear-gradient(135deg,#8b5cf6,#d946ef)' },
  { label: '超过 1 秒', value: stats.value?.over_1s ?? 0, icon: 'Warning', color: 'linear-gradient(135deg,#f97316,#ef4444)' }
])

onMounted(() => {
  applyPreset(presets[3], 3)
  load()
})
</script>

<template>
  <div class="waf-page">
    <div class="waf-page-head">
      <div>
        <h1 class="waf-page-title"><span class="dot"></span> 慢 SQL 统计</h1>
        <p class="waf-page-sub">采集后台自身数据库访问与外部上报的慢查询，支持按记录时间范围查询</p>
      </div>
      <div style="display: flex; gap: 10px">
        <el-button @click="purge"><el-icon><Delete /></el-icon> 清理 30 天前</el-button>
        <el-button type="primary" :loading="loading" @click="load"><el-icon><Refresh /></el-icon> 刷新</el-button>
      </div>
    </div>

    <div class="waf-grid waf-grid--4" style="margin-bottom: 16px">
      <StatCard v-for="card in statCards" :key="card.label" :label="card.label" :value="card.value" :icon="card.icon" :color="card.color" />
    </div>

    <div class="waf-card" style="margin-bottom: 16px">
      <div class="filter-bar">
        <el-button-group>
          <el-button
            v-for="(preset, index) in presets"
            :key="preset.label"
            size="small"
            :type="activePreset === index ? 'primary' : 'default'"
            @click="applyPreset(preset, index), (page = 1), load()"
          >
            {{ preset.label }}
          </el-button>
        </el-button-group>

        <el-date-picker
          v-model="filters.range"
          type="datetimerange"
          start-placeholder="开始时间"
          end-placeholder="结束时间"
          range-separator="→"
          style="width: 360px"
          @change="(activePreset = -1), (page = 1), load()"
        />

        <el-input v-model="filters.keyword" placeholder="搜索 SQL 语句" clearable style="width: 200px" @keyup.enter="load">
          <template #prefix><el-icon><Search /></el-icon></template>
        </el-input>

        <el-select v-model="filters.source" placeholder="来源" clearable style="width: 130px">
          <el-option label="后台自身" value="admin" />
          <el-option label="外部上报" value="agent" />
        </el-select>

        <el-input-number v-model="filters.min_ms" :min="0" :step="100" placeholder="耗时 ≥" style="width: 170px" />

        <el-button type="primary" @click="(page = 1), load()">查询</el-button>
        <el-button text @click="reset">重置</el-button>
      </div>
    </div>

    <div class="waf-grid waf-grid--2" style="margin-bottom: 16px">
      <div class="waf-card">
        <div class="waf-card-head">
          <div class="waf-card-title"><span class="bar"></span> 耗时排行 Top10</div>
          <div class="waf-card-extra">按最大耗时排序</div>
        </div>
        <el-table :data="stats?.top_items ?? []" size="small" max-height="280">
          <el-table-column label="SQL" min-width="260">
            <template #default="{ row }">
              <span class="waf-mono">{{ compactStatement(row.statement, 60) }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="count" label="次数" width="70" align="center" />
          <el-table-column label="平均" width="90" align="center">
            <template #default="{ row }">{{ formatDuration(row.avg_ms) }}</template>
          </el-table-column>
          <el-table-column label="最大" width="90" align="center">
            <template #default="{ row }">
              <span :style="{ color: row.max_ms >= 1000 ? 'var(--waf-danger)' : 'inherit' }">{{ formatDuration(row.max_ms) }}</span>
            </template>
          </el-table-column>
          <template #empty><div class="waf-empty">当前范围暂无慢 SQL</div></template>
        </el-table>
      </div>

      <div class="waf-card">
        <div class="waf-card-head">
          <div class="waf-card-title"><span class="bar"></span> 采集口径说明</div>
        </div>
        <ul class="hint-list">
          <li><strong>采集阈值</strong>：后台自身数据库访问耗时超过配置项 <code>monitor.SLOW_SQL_MS</code>（默认 200ms）时自动登记。</li>
          <li><strong>外部上报</strong>：openresty / 采集器可调用签名接口 <code>POST /api/sync/slow-sql</code> 上报业务库慢查询。</li>
          <li><strong>时间查询</strong>：<code>from</code> / <code>to</code> 支持 <code>YYYY-MM-DD</code> 或 <code>YYYY-MM-DD HH:mm:ss</code>，仅传日期时按整天计算。</li>
          <li><strong>统计口径</strong>：P95 为耗时升序 95 分位，排行按同语句归一化聚合。</li>
        </ul>
      </div>
    </div>

    <div class="waf-card">
      <div class="waf-card-head">
        <div class="waf-card-title"><span class="bar"></span> 慢 SQL 记录</div>
        <div style="display: flex; gap: 8px; align-items: center">
          <el-tag size="small" type="info">共 {{ formatNumber(total) }} 条</el-tag>
          <el-button size="small" text @click="toggleSort">
            <el-icon><Sort /></el-icon> 按耗时{{ filters.order === 'desc' ? '降序' : '升序' }}
          </el-button>
        </div>
      </div>

      <el-table v-loading="loading" :data="rows" stripe>
        <el-table-column label="记录时间" width="170">
          <template #default="{ row }">{{ formatTime(row.occurred_at) }}</template>
        </el-table-column>
        <el-table-column label="耗时" width="110" align="center">
          <template #default="{ row }">
            <el-tag :type="row.duration_ms >= 1000 ? 'danger' : 'warning'" size="small" effect="light">
              {{ row.duration_ms }} ms
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="operation" label="操作" width="100" align="center" />
        <el-table-column prop="source" label="来源" width="110" align="center">
          <template #default="{ row }">
            <el-tag size="small" :type="row.source === 'agent' ? 'success' : 'info'" effect="plain">
              {{ row.source === 'agent' ? '外部上报' : '后台自身' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="SQL 语句" min-width="360">
          <template #default="{ row }">
            <span class="waf-mono">{{ compactStatement(row.statement, 110) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="100" align="right">
          <template #default="{ row }">
            <el-button text type="primary" @click="openDetail(row)">详情</el-button>
          </template>
        </el-table-column>
        <template #empty><div class="waf-empty">当前时间范围内暂无慢 SQL 记录</div></template>
      </el-table>

      <div style="display: flex; justify-content: flex-end; margin-top: 14px">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="size"
          :total="total"
          :page-sizes="[20, 50, 100]"
          layout="total, sizes, prev, pager, next, jumper"
          @current-change="load"
          @size-change="load"
        />
      </div>
    </div>

    <el-drawer v-model="detailVisible" title="慢 SQL 详情" size="620px">
      <template v-if="current">
        <el-descriptions :column="2" border>
          <el-descriptions-item label="记录时间">{{ formatTime(current.occurred_at) }}</el-descriptions-item>
          <el-descriptions-item label="耗时">{{ current.duration_ms }} ms</el-descriptions-item>
          <el-descriptions-item label="库名">{{ current.database || '-' }}</el-descriptions-item>
          <el-descriptions-item label="操作类型">{{ current.operation || '-' }}</el-descriptions-item>
          <el-descriptions-item label="来源">{{ current.source === 'agent' ? '外部上报' : '后台自身' }}</el-descriptions-item>
          <el-descriptions-item label="影响行数">{{ current.rows ?? '-' }}</el-descriptions-item>
          <el-descriptions-item label="客户端 IP">{{ current.client_ip || '-' }}</el-descriptions-item>
          <el-descriptions-item label="Trace">{{ current.trace_id || '-' }}</el-descriptions-item>
        </el-descriptions>
        <div class="waf-code" style="margin-top: 16px">{{ current.statement }}</div>
      </template>
    </el-drawer>
  </div>
</template>

<style scoped>
.filter-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  align-items: center;
}

.hint-list {
  margin: 0;
  padding-left: 18px;
  line-height: 2;
  color: var(--waf-text-muted);
  font-size: 13px;
}

.hint-list code {
  background: var(--waf-surface-2);
  border: 1px solid var(--waf-border);
  border-radius: 6px;
  padding: 1px 6px;
  font-size: 12px;
}
</style>
