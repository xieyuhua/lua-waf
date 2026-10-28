<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ruleApi } from '../api/rule'
import { viewport } from '../utils/responsive'
import { ruleStore } from '../stores/rules'

const route = useRoute()
const router = useRouter()

const loading = ref(false)
const syncing = ref(false)
const syncingFiles = ref(false)
const rows = ref([])
const keyword = ref('')

const dialogVisible = ref(false)
const dialogTitle = ref('新增规则')
const submitting = ref(false)
const editingId = ref(0)
const warning = ref('')

const form = reactive({ rule_item: '' })

const currentKey = computed(() => String(route.params.type || 'args'))
const currentType = computed(() => ruleStore.find(currentKey.value) || { name: currentKey.value, desc: '', file: '' })
const ruleGroups = computed(() => ruleStore.grouped())

/** 加载当前类型的规则 */
// 规则命中统计（最近 N 天，按 rule_tag 聚合）
const ruleStats = ref([])
const statsDays = ref(7)

// 规则内容 → 命中次数 映射
const hitMap = computed(() => {
  const map = {}
  for (const item of ruleStats.value) {
    map[item.rule_tag] = (map[item.rule_tag] || 0) + item.count
  }
  return map
})

/** 某条规则的命中次数 */
function hitCount(ruleItem) {
  return hitMap.value[ruleItem] || 0
}

/** 加载命中统计 */
async function loadStats() {
  try {
    const data = await ruleApi.stats({ days: statsDays.value, limit: 12 })
    ruleStats.value = data.items || []
  } catch (error) {
    ruleStats.value = []
  }
}

async function load() {
  loading.value = true
  try {
    const data = await ruleApi.list({ type: currentKey.value, keyword: keyword.value })
    rows.value = data.items ?? []
  } catch (error) {
    ElMessage.error(error.message || '加载规则失败')
  } finally {
    loading.value = false
  }
}

/** 打开新增弹窗 */
function openCreate() {
  editingId.value = 0
  dialogTitle.value = `新增${currentType.value.name}`
  form.rule_item = ''
  warning.value = ''
  dialogVisible.value = true
}

/** 打开编辑弹窗 */
function openEdit(row) {
  editingId.value = row.id
  dialogTitle.value = `编辑规则 #${row.id}`
  form.rule_item = row.rule_item
  warning.value = ''
  dialogVisible.value = true
}

/** 从规则文件同步：与数据库取并集（缺少的新增，已有的保留不变） */
async function syncFromFiles() {
  syncingFiles.value = true
  try {
    const result = await ruleApi.syncFiles()
    const parts = [`新增 ${result.added} 条`, `保留 ${result.kept} 条`]
    if (result.skipped) parts.push(`跳过 ${result.skipped} 条`)
    ElMessage.success(`规则文件同步完成：${parts.join('，')}`)
    if (result.message) ElMessage.info(result.message)
    await load()
    await loadTypes()
  } catch (error) {
    ElMessage.error(error.message || '同步失败')
  } finally {
    syncingFiles.value = false
  }
}

/** 启用/停用规则（停用后不再下发到 lua，对线上立即不生效） */
async function toggleRule(row, enabled) {
  try {
    await ruleApi.setEnabled(row.id, enabled)
    row.enabled = enabled
    ElMessage.success(enabled ? '规则已启用（需下发后生效）' : '规则已停用（需下发后生效）')
  } catch (error) {
    ElMessage.error(error.message || '操作失败')
  }
}

/** 校验正则表达式 */
async function validate() {
  try {
    const data = await ruleApi.validate({ rule_type: currentKey.value, rule_item: form.rule_item })
    warning.value = data?.warning || ''
    ElMessage.success(warning.value ? '格式可用，但存在兼容性提示' : '规则内容格式正确')
  } catch (error) {
    ElMessage.error(error.message || '校验失败')
  }
}

/** 保存规则 */
async function submit() {
  submitting.value = true
  try {
    if (editingId.value) {
      await ruleApi.update(editingId.value, { rule_type: currentKey.value, rule_item: form.rule_item })
      ElMessage.success('规则已更新')
    } else {
      await ruleApi.create({ rule_type: currentKey.value, rule_item: form.rule_item })
      ElMessage.success('规则已新增')
    }
    dialogVisible.value = false
    load()
    ruleStore.load(true)
  } catch (error) {
    ElMessage.error(error.message || '保存失败')
  } finally {
    submitting.value = false
  }
}

/** 删除规则 */
async function remove(row) {
  try {
    await ElMessageBox.confirm('确认删除该规则？删除后需重新同步才会在 openresty 生效。', '删除确认', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消'
    })
  } catch (error) {
    return
  }
  try {
    await ruleApi.remove(row.id)
    ElMessage.success('规则已删除')
    load()
    ruleStore.load(true)
  } catch (error) {
    ElMessage.error(error.message || '删除失败')
  }
}

/** 下发全部规则 */
async function sync() {
  syncing.value = true
  try {
    const data = await ruleApi.sync()
    const nodes = data?.nodes ?? []
    ElMessage.success(`${data?.message || '规则已下发'}（生成 ${data?.applied ?? 0} 个规则文件，节点 ${nodes.length} 个）`)
  } catch (error) {
    ElMessage.error(error.message || '同步失败')
  } finally {
    syncing.value = false
  }
}

/** 切换子菜单 */
function goType(key) {
  if (key !== currentKey.value) router.push(`/rules/${key}`)
}

watch(currentKey, () => {
  keyword.value = ''
  load()
})

onMounted(async () => {
  await ruleStore.load()
  load()
  loadStats()
})
</script>

<template>
  <div class="waf-page">
    <div class="waf-page-head">
      <div>
        <h1 class="waf-page-title"><span class="dot"></span> {{ currentType.name }}</h1>
        <p class="waf-page-sub">{{ currentType.desc }} · 规则文件 <code class="waf-mono">{{ currentType.file }}</code></p>
      </div>
      <div style="display: flex; gap: 10px">
        <el-button :loading="syncingFiles" @click="syncFromFiles"><el-icon><Refresh /></el-icon> 从规则文件同步</el-button>
      <el-button :loading="syncing" @click="sync"><el-icon><Upload /></el-icon> 下发全部规则</el-button>
        <el-button type="primary" @click="openCreate"><el-icon><Plus /></el-icon> 新增规则</el-button>
      </div>
    </div>

    <!-- 规则类型切换（等同侧边栏子菜单） -->
    <div class="waf-card" style="padding: 12px 14px; margin-bottom: 16px">
      <div class="type-chips">
        <div v-for="group in ruleGroups" :key="group.name" class="type-group">
          <span class="waf-muted type-group-label">{{ group.name }}</span>
          <el-button
            v-for="item in group.items"
            :key="item.key"
            size="small"
            :type="item.key === currentKey ? 'primary' : 'default'"
            :plain="item.key !== currentKey"
            round
            @click="goType(item.key)"
          >
            {{ item.name }}
            <span style="opacity: 0.7; margin-left: 4px">{{ item.count }}</span>
          </el-button>
        </div>
      </div>
    </div>

    <div class="waf-card">
      <div class="waf-card-head">
        <el-input
          v-model="keyword"
          placeholder="搜索规则内容"
          clearable
          style="max-width: 260px"
          @keyup.enter="load"
          @clear="load"
        >
          <template #prefix><el-icon><Search /></el-icon></template>
        </el-input>
        <div class="waf-card-extra">共 {{ rows.length }} 条</div>
      </div>

      <el-table v-loading="loading" :data="rows" stripe>
        <el-table-column v-if="!viewport.isMobile.value" prop="id" label="ID" width="90" align="center" />
        <el-table-column label="规则内容（正则片段）" :min-width="viewport.isMobile.value ? 220 : 420">
          <template #default="{ row }">
            <div class="rule-text waf-mono">{{ row.rule_item }}</div>
          </template>
        </el-table-column>
        <el-table-column label="启用" width="90" align="center">
        <template #default="{ row }">
          <el-switch :model-value="row.enabled !== false" @update:model-value="(value) => toggleRule(row, value)" />
        </template>
      </el-table-column>
      <el-table-column
        v-if="!viewport.isMobile.value"
        label="命中(7天)"
        width="110"
        align="right"
      >
        <template #default="{ row }">
          <span :class="hitCount(row.rule_item) ? 'hit-count' : 'waf-muted'">{{ hitCount(row.rule_item) || '-' }}</span>
        </template>
      </el-table-column>
      <el-table-column label="操作" :width="viewport.isMobile.value ? 120 : 150" align="right" fixed="right">
          <template #default="{ row }">
            <el-button text type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button text type="danger" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
        <template #empty>
          <div class="waf-empty">该类型下暂无规则</div>
        </template>
      </el-table>
    </div>

    <el-dialog v-model="dialogVisible" :title="dialogTitle" width="620px" destroy-on-close>
      <el-form label-position="top">
        <el-form-item label="规则内容（PCRE 正则片段，命中即拦截）" required>
          <el-input v-model="form.rule_item" type="textarea" :rows="4" placeholder="例如：select.+(from|limit)" />
        </el-form-item>
      </el-form>
      <el-alert v-if="warning" :title="warning" type="warning" :closable="false" show-icon style="margin-bottom: 8px" />
      <template #footer>
        <el-button @click="validate">格式校验</el-button>
        <div style="flex: 1"></div>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="submit">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.stat-list {
  display: grid;
  gap: 6px;
}

.stat-item {
  position: relative;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 10px;
  border-radius: 8px;
  background: var(--waf-surface-2);
  overflow: hidden;
}

.stat-bar {
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  background: linear-gradient(90deg, rgba(99, 102, 241, 0.22), rgba(99, 102, 241, 0.06));
}

.stat-count {
  position: relative;
  flex: 0 0 64px;
  font-variant-numeric: tabular-nums;
  font-weight: 700;
  font-size: 12.5px;
  color: var(--waf-primary);
}

.stat-tag {
  position: relative;
  font-size: 11.5px;
  color: var(--waf-text-muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.hit-count {
  font-variant-numeric: tabular-nums;
  font-weight: 600;
  color: var(--waf-primary);
}
.pipeline-list {
  display: grid;
  gap: 8px;
}

.pipeline-item {
  display: flex;
  gap: 10px;
  padding: 8px 10px;
  border: 1px solid var(--waf-border);
  border-radius: 10px;
  background: var(--waf-surface-2);
}

.pipeline-order {
  flex: 0 0 22px;
  height: 22px;
  line-height: 22px;
  text-align: center;
  border-radius: 50%;
  background: var(--waf-primary);
  color: #fff;
  font-size: 11.5px;
  font-weight: 700;
}

.pipeline-main {
  min-width: 0;
  flex: 1;
}

.pipeline-title {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  font-size: 13px;
  font-weight: 650;
  color: var(--waf-text);
}

.pipeline-file,
.pipeline-switch {
  font-size: 11px;
  font-weight: 500;
  color: var(--waf-text-muted);
  background: var(--waf-surface);
  border: 1px solid var(--waf-border);
  border-radius: 6px;
  padding: 1px 6px;
}

.pipeline-desc {
  margin-top: 4px;
  font-size: 12px;
  line-height: 1.65;
  color: var(--waf-text-muted);
}

.pipeline-notes {
  margin: 10px 0 0;
  padding-left: 18px;
  font-size: 12px;
  line-height: 1.8;
  color: var(--waf-text-muted);
}
.rule-text {
  background: var(--waf-surface-2);
  border: 1px solid var(--waf-border);
  border-radius: 8px;
  padding: 6px 10px;
  word-break: break-all;
  white-space: pre-wrap;
}

.type-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 14px 18px;
  align-items: center;
}

.type-group {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
}

.type-group-label {
  font-size: 12px;
  padding-right: 6px;
  border-right: 1px solid var(--waf-border);
  line-height: 1;
}
</style>
