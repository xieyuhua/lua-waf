<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ruleApi } from '../api/rule'
import { ruleStore } from '../stores/rules'

const route = useRoute()
const router = useRouter()

const loading = ref(false)
const syncing = ref(false)
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
        <el-table-column prop="id" label="ID" width="90" align="center" />
        <el-table-column label="规则内容（正则片段）" min-width="420">
          <template #default="{ row }">
            <div class="rule-text waf-mono">{{ row.rule_item }}</div>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="150" align="right">
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
