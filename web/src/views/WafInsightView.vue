<script setup>
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ruleApi } from '../api/rule'
import { viewport } from '../utils/responsive'

// 命中统计
const statsLoading = ref(true)
const ruleStats = ref([])
const statsDays = ref(7)
const totalHits = computed(() => ruleStats.value.reduce((sum, item) => sum + item.count, 0))

// 命中样例抽屉
const sampleVisible = ref(false)
const sampleLoading = ref(false)
const sample = ref(null)
const sampleDays = ref(7)
const currentTag = ref('')

/** 加载命中排行 */
async function loadStats() {
  statsLoading.value = true
  try {
    const data = await ruleApi.stats({ days: statsDays.value, limit: 20 })
    ruleStats.value = data.items || []
  } catch (error) {
    ElMessage.error(error.message || '读取命中统计失败')
  } finally {
    statsLoading.value = false
  }
}

/** 查看某条规则的命中样例（判断是否误杀） */
async function openSamples(tag) {
  currentTag.value = tag
  sampleDays.value = statsDays.value
  sampleVisible.value = true
  await loadSamples()
}

/** 加载样例 */
async function loadSamples() {
  sampleLoading.value = true
  try {
    sample.value = await ruleApi.samples({ rule_tag: currentTag.value, days: sampleDays.value, limit: 8 })
  } catch (error) {
    ElMessage.error(error.message || '读取命中样例失败')
    sample.value = null
  } finally {
    sampleLoading.value = false
  }
}

/** 停用/启用该规则 */
async function toggleRule() {
  if (!sample.value?.rule_id) return
  const next = !sample.value.rule_enabled
  try {
    await ElMessageBox.confirm(
      next
        ? '确认重新启用这条规则？启用后需「下发全部规则」才会在 lua 生效。'
        : '确认停用这条规则？停用后不再下发到 lua，需要重新下发才对线上生效，规则不会被删除。',
      next ? '启用规则' : '停用规则',
      { type: 'warning', confirmButtonText: '确认', cancelButtonText: '取消' }
    )
  } catch (error) {
    return
  }
  try {
    await ruleApi.setEnabled(sample.value.rule_id, next)
    sample.value.rule_enabled = next
    ElMessage.success(next ? '规则已启用（需下发后生效）' : '规则已停用（需下发后生效）')
    await loadStats()
  } catch (error) {
    ElMessage.error(error.message || '操作失败')
  }
}

onMounted(loadStats)
</script>

<template>
  <div class="waf-page">
    <div class="waf-page-head">
      <div>
        <h1 class="waf-page-title"><span class="dot"></span> 防护分析</h1>
        <p class="waf-page-sub">规则命中排行与命中样例，用于评估规则效果和排查误杀</p>
      </div>
      <div style="display: flex; gap: 10px; align-items: center">
        <el-radio-group v-model="statsDays" size="small" @change="loadStats">
          <el-radio-button :value="1">今天</el-radio-button>
          <el-radio-button :value="7">7 天</el-radio-button>
          <el-radio-button :value="30">30 天</el-radio-button>
        </el-radio-group>
        <el-button type="primary" :loading="statsLoading" @click="loadStats">
          <el-icon><Refresh /></el-icon> 刷新
        </el-button>
      </div>
    </div>

    <div class="waf-card">
      <div class="waf-card-head">
        <div class="waf-card-title"><span class="bar"></span> 规则命中 Top</div>
        <div class="waf-card-extra">
          共 {{ totalHits }} 次命中 · 点击「看样例」查看该规则的真实命中日志，用于判断是否误杀
        </div>
      </div>

      <el-table v-loading="statsLoading" :data="ruleStats" stripe>
        <el-table-column label="排名" width="70" align="center">
          <template #default="{ $index }">{{ $index + 1 }}</template>
        </el-table-column>
        <el-table-column label="命中次数" width="120" align="right">
          <template #default="{ row }">
            <span class="hit-count">{{ row.count }}</span>
          </template>
        </el-table-column>
        <el-table-column label="占比" width="160">
          <template #default="{ row }">
            <div class="ratio-track">
              <div
                class="ratio-bar"
                :style="{ width: `${Math.max(3, (row.count / (ruleStats[0]?.count || 1)) * 100)}%` }"
              ></div>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="规则内容" :min-width="viewport.isMobile.value ? 200 : 460">
          <template #default="{ row }">
            <code class="waf-mono rule-tag">{{ row.rule_tag }}</code>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="110" align="right" fixed="right">
          <template #default="{ row }">
            <el-button text type="primary" @click="openSamples(row.rule_tag)">看样例</el-button>
          </template>
        </el-table-column>
        <template #empty>
          <div class="waf-empty">所选时间段内没有命中记录</div>
        </template>
      </el-table>
    </div>

    <!-- 命中样例抽屉 -->
    <el-drawer v-model="sampleVisible" :size="viewport.isMobile.value ? '92%' : '680px'" title="命中样例">
      <div v-loading="sampleLoading">
        <div class="sample-head">
          <code class="waf-mono rule-tag">{{ currentTag }}</code>
          <div style="display: flex; align-items: center; gap: 10px; margin-top: 10px; flex-wrap: wrap">
            <el-tag size="small" type="warning" effect="light">{{ sample?.total || 0 }} 次命中</el-tag>
            <el-tag v-if="sample?.rule_id" size="small" :type="sample.rule_enabled ? 'success' : 'info'" effect="light">
              {{ sample.rule_enabled ? '规则已启用' : '规则已停用' }}
            </el-tag>
            <el-tag v-else size="small" effect="plain">库中无此规则（内置/未同步）</el-tag>
            <el-radio-group v-model="sampleDays" size="small" @change="loadSamples">
              <el-radio-button :value="1">今天</el-radio-button>
              <el-radio-button :value="7">7 天</el-radio-button>
              <el-radio-button :value="30">30 天</el-radio-button>
            </el-radio-group>
            <el-button
              v-if="sample?.rule_id"
              size="small"
              :type="sample.rule_enabled ? 'danger' : 'success'"
              plain
              @click="toggleRule"
            >
              {{ sample.rule_enabled ? '停用该规则' : '启用该规则' }}
            </el-button>
          </div>
        </div>

        <div v-if="!sample?.items?.length" class="waf-empty" style="margin-top: 18px">没有样例</div>
        <div v-for="item in sample?.items || []" :key="item.id" class="sample-item">
          <div class="sample-meta">
            <span class="waf-mono">{{ item.client_ip }}</span>
            <span class="waf-muted">{{ item.local_time }}</span>
            <span class="waf-muted">{{ item.server_name || '-' }}</span>
            <el-tag size="small" effect="plain">{{ item.attack_type }}</el-tag>
          </div>
          <div class="sample-line"><span class="waf-muted">URL</span> <code class="waf-mono">{{ item.req_url }}</code></div>
          <div v-if="item.req_data" class="sample-line">
            <span class="waf-muted">Body</span> <code class="waf-mono">{{ item.req_data }}</code>
          </div>
        </div>

        <div class="waf-muted" style="margin-top: 14px; font-size: 11.5px; line-height: 1.7">
          判断要点：若样例是你们的正常业务请求（内部拨测、健康检查、特殊参数名），说明该规则误杀，
          可直接停用；若确为攻击，则保持启用并考虑收窄规则。
        </div>
      </div>
    </el-drawer>
  </div>
</template>

<style scoped>
.hit-count {
  font-variant-numeric: tabular-nums;
  font-weight: 700;
  color: var(--waf-primary);
}

.ratio-track {
  height: 8px;
  border-radius: 999px;
  background: var(--waf-surface-2);
  overflow: hidden;
}

.ratio-bar {
  height: 100%;
  border-radius: 999px;
  background: linear-gradient(90deg, rgba(99, 102, 241, 0.9), rgba(139, 92, 246, 0.6));
}

.rule-tag {
  font-size: 11.5px;
  color: var(--waf-text-muted);
  word-break: break-all;
}

.sample-head {
  padding-bottom: 12px;
  border-bottom: 1px solid var(--waf-border);
}

.sample-item {
  margin-top: 12px;
  padding: 10px 12px;
  border: 1px solid var(--waf-border);
  border-radius: 10px;
  background: var(--waf-surface-2);
}

.sample-meta {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
  align-items: center;
  font-size: 12px;
}

.sample-line {
  margin-top: 6px;
  font-size: 12px;
  line-height: 1.6;
  word-break: break-all;
}
</style>