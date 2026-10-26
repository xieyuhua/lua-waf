<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { logApi } from '../api/log'
import { ruleApi } from '../api/rule'
import { ruleStore } from '../stores/rules'
import { toBackendTime } from '../utils/format'
import AiAnalyzeDialog from '../components/AiAnalyzeDialog.vue'
import StatCard from '../components/StatCard.vue'

const route = useRoute()
const router = useRouter()

const loading = ref(false)
const rows = ref([])
const total = ref(0)
const page = ref(1)
const size = ref(20)

const filters = reactive({
  keyword: '',
  client_ip: '',
  server_name: '',
  attack_type: '',
  range: []
})

const options = reactive({ attack_types: [], sites: [] })
const overview = ref(null)

const selection = ref([])
const aiVisible = ref(false)
const detailVisible = ref(false)
const current = ref(null)

/** 组装查询参数 */
function queryParams() {
  const params = {
    page: page.value,
    size: size.value,
    keyword: filters.keyword,
    client_ip: filters.client_ip,
    server_name: filters.server_name,
    attack_type: filters.attack_type
  }
  if (Array.isArray(filters.range) && filters.range.length === 2) {
    params.from = toBackendTime(filters.range[0])
    params.to = toBackendTime(filters.range[1])
  }
  return params
}

/** 加载日志列表 */
async function load() {
  loading.value = true
  try {
    const data = await logApi.list(queryParams())
    rows.value = data.items ?? []
    total.value = data.total ?? 0
  } catch (error) {
    ElMessage.error(error.message || '加载日志失败')
  } finally {
    loading.value = false
  }
}

/** 加载筛选项与统计 */
async function loadMeta() {
  try {
    const [filtersData, overviewData] = await Promise.all([logApi.filters(), logApi.overview({ days: 15 })])
    options.attack_types = filtersData.attack_types ?? []
    options.sites = filtersData.sites ?? []
    overview.value = overviewData
  } catch (error) {
    /* 首屏失败不阻断日志列表 */
  }
}

/** 重置筛选 */
function reset() {
  Object.assign(filters, { keyword: '', client_ip: '', server_name: '', attack_type: '', range: [] })
  page.value = 1
  load()
}

/** 选中变化 */
function handleSelectionChange(value) {
  selection.value = value
}

/** 打开详情 */
function openDetail(row) {
  current.value = row
  detailVisible.value = true
}

/** 打开 AI 分析（单条） */
function analyzeOne(row) {
  selection.value = [row]
  aiVisible.value = true
}

/** 打开 AI 分析（选中多条） */
function analyzeSelected() {
  if (selection.value.length === 0) {
    ElMessage.warning('请先勾选要分析的日志')
    return
  }
  aiVisible.value = true
}

/** 把来源 IP 加入黑名单并下发（转义点号，避免正则通配） */
async function banIp(row) {
  const ip = String(row.client_ip || '').trim()
  if (!ip) {
    ElMessage.warning('该日志没有可封禁的来源 IP')
    return
  }
  try {
    await ElMessageBox.confirm(
      `将 ${ip} 写入「IP 黑名单」并立即下发到本机与集群节点，是否继续？`,
      '封禁 IP',
      { type: 'warning', confirmButtonText: '封禁', cancelButtonText: '取消' }
    )
  } catch (error) {
    return
  }

  try {
    await ruleApi.create({ rule_type: 'blackip', rule_item: ip.replace(/\./g, '\\.') })
    await ruleApi.sync()
    ElMessage.success(`${ip} 已加入 IP 黑名单并下发`)
    ruleStore.load(true)
  } catch (error) {
    ElMessage.error(error.message || '封禁失败（可能已存在同名规则）')
  }
}

const attackTagType = (type) => {
  if (!type) return 'info'
  if (type.includes('SQL') || type.includes('命令')) return 'danger'
  if (type.includes('XSS')) return 'warning'
  if (type.includes('穿越') || type.includes('包含')) return 'warning'
  return 'info'
}

const statCards = computed(() => {
  const data = overview.value
  return [
    { label: '累计攻击', value: data?.total ?? 0, icon: 'DataLine', color: 'linear-gradient(135deg,#6366f1,#8b5cf6)' },
    { label: '今日拦截', value: data?.today ?? 0, icon: 'Warning', color: 'linear-gradient(135deg,#f97316,#ef4444)' },
    { label: '攻击类型', value: (data?.types ?? []).length, icon: 'Files', color: 'linear-gradient(135deg,#0ea5e9,#22d3ee)' },
    { label: '攻击来源', value: (data?.ips ?? []).length, icon: 'Aim', color: 'linear-gradient(135deg,#10b981,#34d399)' }
  ]
})

// 支持从顶栏搜索跳转时带关键词
watch(
  () => route.query.keyword,
  (value) => {
    if (typeof value === 'string' && value) {
      filters.keyword = value
      page.value = 1
      load()
    }
  }
)

onMounted(() => {
  if (typeof route.query.keyword === 'string') filters.keyword = route.query.keyword
  loadMeta()
  load()
})
</script>

<template>
  <div class="waf-page">
    <div class="waf-page-head">
      <div>
        <h1 class="waf-page-title"><span class="dot"></span> 攻击日志</h1>
        <p class="waf-page-sub">支持关键词（IP / URL / UA / 规则）与多条件组合筛选，可勾选日志交由 AI 研判</p>
      </div>
      <div style="display: flex; gap: 10px">
        <el-button type="primary" plain :disabled="!selection.length" @click="analyzeSelected">
          <el-icon><MagicStick /></el-icon> AI 分析（{{ selection.length }}）
        </el-button>
        <el-button :loading="loading" @click="load"><el-icon><Refresh /></el-icon> 刷新</el-button>
      </div>
    </div>

    <div class="waf-grid waf-grid--4" style="margin-bottom: 16px">
      <StatCard
        v-for="card in statCards"
        :key="card.label"
        :label="card.label"
        :value="card.value"
        :icon="card.icon"
        :color="card.color"
      />
    </div>

    <div class="waf-card" style="margin-bottom: 16px">
      <div class="filter-bar">
        <el-input v-model="filters.keyword" placeholder="关键词：IP / URL / UA / 规则" clearable style="width: 240px" @keyup.enter="load">
          <template #prefix><el-icon><Search /></el-icon></template>
        </el-input>
        <el-input v-model="filters.client_ip" placeholder="来源 IP" clearable style="width: 160px" @keyup.enter="load" />
        <el-select v-model="filters.server_name" placeholder="站点" clearable style="width: 180px">
          <el-option v-for="site in options.sites" :key="site" :label="site" :value="site" />
        </el-select>
        <el-select v-model="filters.attack_type" placeholder="攻击类型" clearable style="width: 160px">
          <el-option v-for="type in options.attack_types" :key="type" :label="type" :value="type" />
        </el-select>
        <el-date-picker
          v-model="filters.range"
          type="datetimerange"
          start-placeholder="开始时间"
          end-placeholder="结束时间"
          range-separator="→"
          style="width: 360px"
        />
        <el-button type="primary" @click="(page = 1), load()">查询</el-button>
        <el-button text @click="reset">重置</el-button>
      </div>
    </div>

    <div class="waf-card">
      <el-table
        v-loading="loading"
        :data="rows"
        stripe
        row-key="id"
        @selection-change="handleSelectionChange"
      >
        <el-table-column type="selection" width="46" />
        <el-table-column prop="local_time" label="时间" width="160" />
        <el-table-column prop="client_ip" label="来源 IP" width="128">
          <template #default="{ row }">
            <span class="waf-mono">{{ row.client_ip }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="area" label="地区" width="120" show-overflow-tooltip />
        <el-table-column prop="server_name" label="站点" width="150" show-overflow-tooltip />
        <el-table-column label="攻击类型" width="110">
          <template #default="{ row }">
            <el-tag :type="attackTagType(row.attack_type)" size="small" effect="light">{{ row.attack_type || '未知' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="请求路径" min-width="200" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="waf-mono">{{ row.req_url }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="180" align="right" fixed="right">
          <template #default="{ row }">
            <el-button text type="primary" @click="openDetail(row)">详情</el-button>
            <el-button text type="primary" @click="analyzeOne(row)">AI</el-button>
            <el-button text type="danger" @click="banIp(row)">封禁</el-button>
          </template>
        </el-table-column>
        <template #empty><div class="waf-empty">没有符合条件的日志</div></template>
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

    <AiAnalyzeDialog v-model="aiVisible" :logs="selection" />

    <el-drawer v-model="detailVisible" title="日志详情" size="520px">
      <template v-if="current">
        <el-descriptions :column="1" border>
          <el-descriptions-item label="时间">{{ current.local_time }}</el-descriptions-item>
          <el-descriptions-item label="来源 IP">{{ current.client_ip }}</el-descriptions-item>
          <el-descriptions-item label="地区">{{ current.area || '-' }}</el-descriptions-item>
          <el-descriptions-item label="站点">{{ current.server_name }}</el-descriptions-item>
          <el-descriptions-item label="攻击类型">{{ current.attack_type }}</el-descriptions-item>
          <el-descriptions-item label="命中规则">{{ current.rule_tag || '-' }}</el-descriptions-item>
          <el-descriptions-item label="请求路径">
            <span class="waf-mono" style="word-break: break-all">{{ current.req_url }}</span>
          </el-descriptions-item>
          <el-descriptions-item label="User-Agent">
            <span class="waf-mono" style="word-break: break-all">{{ current.user_agent || '-' }}</span>
          </el-descriptions-item>
          <el-descriptions-item label="请求体">
            <span class="waf-mono" style="word-break: break-all">{{ current.req_data || '-' }}</span>
          </el-descriptions-item>
        </el-descriptions>

        <el-button type="primary" plain style="margin-top: 16px; width: 100%" @click="analyzeOne(current)">
          <el-icon><MagicStick /></el-icon> 交给 AI 分析这条日志
        </el-button>
        <el-button type="danger" plain style="margin-top: 10px; width: 100%" @click="banIp(current)">
          <el-icon><Lock /></el-icon> 将该 IP 加入黑名单并下发
        </el-button>
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
</style>
