<script setup>
/**
 * 限频规则：按 URL 精细化限频
 *
 * 与「WAF 配置 → 频率与请求体」里的全局 CC/IP 频率的区别：
 *   全局只有一组阈值；这里可以给不同 URL 配不同阈值，并可按 IP / Header（如 X-API-Key）/ Cookie /
 *   URL 参数 / URI 不同维度计数，超限后可拦截或只记录（观察）。
 *
 * 规则保存后会自动下发到 nginx 侧 rules/ratelimit.rule（JSON 数组），lua 侧按顺序匹配，
 * 命中第一条规则即生效——所以更具体的 URL 应该排在前面（列表按 Id 升序下发）。
 */
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { rateLimitApi } from '../api/ratelimit'

const loading = ref(false)
const syncing = ref(false)
const items = ref([])
const ruleDir = ref('')

const dialogVisible = ref(false)
const dialogTitle = ref('新建限频规则')
const submitting = ref(false)
const editingId = ref(0)

// 表单：默认按 IP + 前缀匹配，60 次 / 60 秒
const form = reactive({
  name: '',
  pattern: '',
  match: 'prefix',
  key: 'ip',
  key_name: '',
  limit: 60,
  window: 60,
  action: 'block',
  enabled: true
})

// 需要维度名的类型（header / cookie / arg）
const needKeyName = computed(() => ['header', 'cookie', 'arg'].includes(form.key))

const matchOptions = [
  { value: 'prefix', label: '前缀匹配（如 /api/login）' },
  { value: 'regex', label: '正则匹配（如 ^/open/api/）' }
]

const keyOptions = [
  { value: 'ip', label: '来源 IP' },
  { value: 'header', label: '请求头（如 X-API-Key）' },
  { value: 'cookie', label: 'Cookie' },
  { value: 'arg', label: 'URL 参数' },
  { value: 'uri', label: '完整 URI（每个 URL 各自计数）' }
]

const keyNamePlaceholder = computed(() => {
  if (form.key === 'header') return '如 X-API-Key'
  if (form.key === 'cookie') return '如 session'
  if (form.key === 'arg') return '如 apikey'
  return ''
})

/** 加载规则列表 */
async function load() {
  loading.value = true
  try {
    const data = await rateLimitApi.list()
    items.value = data.items ?? []
    ruleDir.value = data.rule_dir ?? ''
  } catch (error) {
    ElMessage.error(error.message || '读取限频规则失败')
  } finally {
    loading.value = false
  }
}

/** 打开新建弹窗 */
function openCreate() {
  editingId.value = 0
  dialogTitle.value = '新建限频规则'
  Object.assign(form, {
    name: '',
    pattern: '',
    match: 'prefix',
    key: 'ip',
    key_name: '',
    limit: 60,
    window: 60,
    action: 'block',
    enabled: true
  })
  dialogVisible.value = true
}

/** 打开编辑弹窗 */
function openEdit(row) {
  editingId.value = row.id
  dialogTitle.value = `编辑：${row.name}`
  Object.assign(form, {
    name: row.name,
    pattern: row.pattern,
    match: row.match || 'prefix',
    key: row.key || 'ip',
    key_name: row.key_name || '',
    limit: row.limit,
    window: row.window,
    action: row.action || 'block',
    enabled: row.enabled
  })
  dialogVisible.value = true
}

/** 提交表单 */
async function submit() {
  if (!form.name.trim()) return ElMessage.warning('请填写规则名称')
  if (!form.pattern.trim()) return ElMessage.warning('请填写 URL 匹配内容')
  if (needKeyName.value && !form.key_name.trim()) return ElMessage.warning('请填写维度名')

  submitting.value = true
  try {
    const payload = { ...form }
    if (editingId.value) {
      await rateLimitApi.update(editingId.value, payload)
      ElMessage.success('规则已更新并下发')
    } else {
      await rateLimitApi.create(payload)
      ElMessage.success('规则已创建并下发')
    }
    dialogVisible.value = false
    await load()
  } catch (error) {
    ElMessage.error(error.message || '保存失败')
  } finally {
    submitting.value = false
  }
}

/** 启停 */
async function toggle(row) {
  try {
    await rateLimitApi.setEnabled(row.id, row.enabled)
    ElMessage.success(row.enabled ? '已启用并下发' : '已停用并下发')
  } catch (error) {
    row.enabled = !row.enabled
    ElMessage.error(error.message || '操作失败')
  }
}

/** 删除 */
async function remove(row) {
  try {
    await ElMessageBox.confirm(`确定删除规则「${row.name}」？`, '删除限频规则', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消'
    })
  } catch (error) {
    return
  }

  try {
    await rateLimitApi.remove(row.id)
    ElMessage.success('已删除并下发')
    await load()
  } catch (error) {
    ElMessage.error(error.message || '删除失败')
  }
}

/** 手动下发 */
async function sync() {
  syncing.value = true
  try {
    const result = await rateLimitApi.sync()
    ElMessage.success(result?.message || '已下发')
    await load()
  } catch (error) {
    ElMessage.error(error.message || '下发失败')
  } finally {
    syncing.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="ratelimit-page">
    <div class="page-head">
      <div>
        <h2>限频规则</h2>
        <p class="waf-muted">
          按 URL 单独设置频率阈值，可分别按 IP / 请求头 / Cookie / 参数维度计数；命中即按动作拦截或仅记录。
          规则下发到 <span class="waf-mono">{{ ruleDir || 'rules/' }}ratelimit.rule</span>，lua 侧按顺序匹配（第一条命中生效）。
        </p>
      </div>
      <div class="page-actions">
        <el-button :loading="syncing" @click="sync">重新下发</el-button>
        <el-button type="primary" @click="openCreate">新建规则</el-button>
      </div>
    </div>

    <div class="waf-card">
      <el-table :data="items" v-loading="loading" stripe style="width: 100%">
        <el-table-column prop="id" label="ID" width="70" />
        <el-table-column prop="name" label="名称" min-width="140" show-overflow-tooltip />
        <el-table-column label="URL 匹配" min-width="220">
          <template #default="{ row }">
            <span class="waf-mono">{{ row.pattern }}</span>
            <el-tag size="small" effect="plain" style="margin-left: 6px">
              {{ row.match === 'regex' ? '正则' : '前缀' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="key_label" label="计数维度" min-width="150" show-overflow-tooltip />
        <el-table-column prop="rate_text" label="阈值" width="130" />
        <el-table-column label="超限动作" width="110">
          <template #default="{ row }">
            <el-tag :type="row.action === 'block' ? 'danger' : 'warning'" size="small" effect="light">
              {{ row.action === 'block' ? '拦截' : '仅记录' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="启用" width="90">
          <template #default="{ row }">
            <el-switch v-model="row.enabled" @change="toggle(row)" />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="150" fixed="right">
          <template #default="{ row }">
            <el-button text type="primary" size="small" @click="openEdit(row)">编辑</el-button>
            <el-button text type="danger" size="small" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
        <template #empty>
          <p class="waf-muted" style="padding: 24px 0">
            还没有规则。示例：登录接口按 IP 限制 10 次 / 60 秒；开放 API 按 X-API-Key 限制 1000 次 / 60 秒。
          </p>
        </template>
      </el-table>
    </div>

    <el-dialog v-model="dialogVisible" :title="dialogTitle" width="620px" destroy-on-close>
      <el-form label-width="110px">
        <el-form-item label="规则名称">
          <el-input v-model="form.name" maxlength="64" placeholder="如：登录接口" />
        </el-form-item>
        <el-form-item label="URL 匹配">
          <el-input v-model="form.pattern" maxlength="255" placeholder="/api/login 或 ^/open/api/" />
        </el-form-item>
        <el-form-item label="匹配方式">
          <el-select v-model="form.match" style="width: 100%">
            <el-option v-for="item in matchOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
        <el-form-item label="计数维度">
          <el-select v-model="form.key" style="width: 100%">
            <el-option v-for="item in keyOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="needKeyName" label="维度名">
          <el-input v-model="form.key_name" maxlength="64" :placeholder="keyNamePlaceholder" />
        </el-form-item>
        <el-form-item label="阈值">
          <div style="display: flex; gap: 8px; align-items: center">
            <el-input-number v-model="form.limit" :min="1" :max="1000000" controls-position="right" />
            <span class="waf-muted">次 /</span>
            <el-input-number v-model="form.window" :min="1" :max="86400" controls-position="right" />
            <span class="waf-muted">秒</span>
          </div>
        </el-form-item>
        <el-form-item label="超限动作">
          <el-radio-group v-model="form.action">
            <el-radio value="block">拦截</el-radio>
            <el-radio value="log">仅记录（观察）</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="form.enabled" />
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="submit">保存并下发</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.ratelimit-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.page-head {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  align-items: flex-start;
  flex-wrap: wrap;
}

.page-head h2 {
  margin: 0 0 6px;
  font-size: 18px;
}

.page-head p {
  margin: 0;
  max-width: 900px;
  font-size: 12.5px;
  line-height: 1.7;
}

.page-actions {
  display: flex;
  gap: 8px;
}
</style>
