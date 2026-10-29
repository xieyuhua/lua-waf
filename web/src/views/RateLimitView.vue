<script setup>
/**
 * 限频规则：按 URL 精细化限频
 *
 * 与「WAF 配置 → 基础」里的全局 CC/IP 频率的区别：
 *   全局只有一组阈值；这里可以给不同 URL 配不同阈值，并可按 IP / Header（如 X-API-Key）/ Cookie /
 *   URL 参数 / URI / 站点 / 请求方法 / Referer 等维度计数，超限后可拦截或只记录（观察）。
 *
 * 规则保存后自动下发到 nginx 侧 rules/ratelimit.rule（JSON 数组），lua 侧按顺序匹配，
 * 命中第一条规则即生效——所以更具体的 URL 应排在前面（列表按 ID 升序下发）。
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

// 概览数字
const stats = computed(() => {
  const total = items.value.length
  const enabled = items.value.filter((item) => item.enabled).length
  const blocking = items.value.filter((item) => item.action === 'block').length
  return { total, enabled, blocking, watching: total - blocking }
})

// 规则下发路径（用于页面提示）
const ruleFilePath = computed(() => `${ruleDir.value || 'rules/'}ratelimit.rule`)

const matchOptions = [
  { value: 'prefix', label: '前缀匹配（如 /api/login）' },
  { value: 'regex', label: '正则匹配（如 ^/open/api/）' }
]

const keyOptions = [
  { value: 'ip', label: '来源 IP' },
  { value: 'header', label: '请求头（如 X-API-Key）' },
  { value: 'cookie', label: 'Cookie' },
  { value: 'arg', label: 'URL 参数' },
  { value: 'uri', label: '完整 URI（每个 URL 各自计数）' },
  { value: 'site', label: '站点（Host / server_name）' },
  { value: 'method', label: '请求方法（GET/POST…）' },
  { value: 'referer', label: '来源页 Referer（防盗链）' }
]

const keyNamePlaceholder = computed(() => {
  if (form.key === 'header') return '如 X-API-Key'
  if (form.key === 'cookie') return '如 session'
  if (form.key === 'arg') return '如 apikey'
  return ''
})

// 空状态里的常见场景模板（点一下即预填表单）
const templates = {
  login: {
    title: '登录防爆破',
    payload: { name: '登录防爆破', pattern: '/api/login', match: 'prefix', key: 'ip', key_name: '', limit: 10, window: 60, action: 'block' }
  },
  apikey: {
    title: '开放 API 按密钥限流',
    payload: {
      name: '开放 API 按密钥限流',
      pattern: '^/open/api/',
      match: 'regex',
      key: 'header',
      key_name: 'X-API-Key',
      limit: 1000,
      window: 60,
      action: 'block'
    }
  },
  download: {
    title: '下载防刷',
    payload: { name: '下载防刷', pattern: '/dl/', match: 'prefix', key: 'cookie', key_name: 'session', limit: 20, window: 60, action: 'block' }
  },
  observe: {
    title: '接口观察（不拦截）',
    payload: { name: '接口观察', pattern: '/ping', match: 'prefix', key: 'uri', key_name: '', limit: 100, window: 60, action: 'log' }
  }
}

const emptyTemplates = Object.entries(templates).map(([key, item]) => ({
  key,
  title: item.title,
  summary: `${item.payload.pattern} · ${describeKey(item.payload)} · ${item.payload.limit} 次 / ${item.payload.window} 秒${item.payload.action === 'log' ? ' · 仅记录' : ''}`
}))

/** 场景模板的维度说明 */
function describeKey(payload) {
  if (payload.key === 'ip') return '按 IP'
  if (payload.key === 'uri') return '按 URI'
  if (payload.key === 'header') return `按请求头 ${payload.key_name}`
  if (payload.key === 'cookie') return `按 Cookie ${payload.key_name}`
  if (payload.key === 'arg') return `按参数 ${payload.key_name}`
  if (payload.key === 'site') return '按站点'
  if (payload.key === 'method') return '按请求方法'
  return payload.key
}

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

/** 打开新建弹窗（可带场景模板） */
function openCreate(templateKey) {
  const template = templateKey ? templates[templateKey] : null
  editingId.value = 0
  dialogTitle.value = template ? `新建限频规则：${template.title}` : '新建限频规则'
  Object.assign(form, {
    name: '',
    pattern: '',
    match: 'prefix',
    key: 'ip',
    key_name: '',
    limit: 60,
    window: 60,
    action: 'block',
    enabled: true,
    ...(template?.payload ?? {})
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
  <div class="waf-page">
    <div class="waf-page-head">
      <div>
        <h1 class="waf-page-title"><span class="dot"></span> 限频规则</h1>
        <p class="waf-page-sub">
          按 URL 单独设置频率阈值，可分别按 IP / 请求头 / Cookie / 参数 / 站点 / 请求方法 / Referer 计数；
          超限后可拦截或仅记录。规则下发到
          <code class="waf-mono">{{ ruleFilePath }}</code>，lua 按顺序匹配（第一条命中生效，更具体的放前面）
        </p>
      </div>
      <div style="display: flex; gap: 10px">
        <el-button :loading="syncing" @click="sync">
          <el-icon><Upload /></el-icon> 重新下发
        </el-button>
        <el-button type="primary" @click="openCreate()">
          <el-icon><Plus /></el-icon> 新建规则
        </el-button>
      </div>
    </div>

    <!-- 概览 -->
    <div class="rl-stats">
      <div class="rl-stat">
        <span class="rl-stat-label">规则总数</span>
        <span class="rl-stat-value">{{ stats.total }}</span>
      </div>
      <div class="rl-stat">
        <span class="rl-stat-label">启用中</span>
        <span class="rl-stat-value">{{ stats.enabled }}</span>
      </div>
      <div class="rl-stat">
        <span class="rl-stat-label">拦截</span>
        <span class="rl-stat-value is-danger">{{ stats.blocking }}</span>
      </div>
      <div class="rl-stat">
        <span class="rl-stat-label">仅记录</span>
        <span class="rl-stat-value is-muted">{{ stats.watching }}</span>
      </div>
      <div class="rl-stat rl-stat--hint">
        <el-icon><InfoFilled /></el-icon>
        <span>
          命中日志：拦截记 <b>限频攻击</b>、仅记录记 <b>限频观察</b>，标签为
          <code class="waf-mono">ratelimit:&lt;规则ID&gt;</code>，可在「攻击日志 / 防护分析」按标签查
        </span>
      </div>
    </div>

    <!-- 规则列表 -->
    <div class="waf-card">
      <div class="waf-card-head">
        <span class="waf-card-title"><span class="bar"></span> 规则列表</span>
        <div class="waf-card-extra">
          <el-button text @click="load"><el-icon><RefreshRight /></el-icon> 刷新</el-button>
        </div>
      </div>

      <el-table v-if="items.length" :data="items" v-loading="loading" stripe style="width: 100%">
        <el-table-column prop="id" label="ID" width="64" />
        <el-table-column prop="name" label="名称" min-width="150" show-overflow-tooltip />
        <el-table-column label="URL 匹配" min-width="240">
          <template #default="{ row }">
            <code class="waf-mono rl-pattern">{{ row.pattern }}</code>
            <el-tag size="small" effect="plain" round>{{ row.match === 'regex' ? '正则' : '前缀' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="key_label" label="计数维度" min-width="150" show-overflow-tooltip />
        <el-table-column label="阈值" width="130">
          <template #default="{ row }">
            <span class="rl-rate"><b>{{ row.limit }}</b> 次 / {{ row.window }} 秒</span>
          </template>
        </el-table-column>
        <el-table-column label="超限动作" width="106">
          <template #default="{ row }">
            <el-tag :type="row.action === 'block' ? 'danger' : 'warning'" size="small" effect="light">
              {{ row.action === 'block' ? '拦截' : '仅记录' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="启用" width="86">
          <template #default="{ row }">
            <el-switch v-model="row.enabled" @change="toggle(row)" />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="130" fixed="right">
          <template #default="{ row }">
            <el-button text type="primary" size="small" @click="openEdit(row)">编辑</el-button>
            <el-button text type="danger" size="small" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <!-- 空状态：直接给出常见场景，一键预填 -->
      <div v-else-if="!loading" class="rl-empty">
        <el-empty description="还没有限频规则，先从一个常见场景开始" :image-size="72">
          <div class="rl-empty-grid">
            <button v-for="item in emptyTemplates" :key="item.key" class="rl-empty-item" type="button" @click="openCreate(item.key)">
              <b>{{ item.title }}</b>
              <span>{{ item.summary }}</span>
            </button>
          </div>
        </el-empty>
      </div>
      <div v-else class="rl-loading">
        <el-icon class="is-loading"><Loading /></el-icon>
      </div>
    </div>

    <el-dialog v-model="dialogVisible" :title="dialogTitle" width="640px" destroy-on-close>
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
        <span class="rl-dialog-hint">保存后自动下发到 {{ ruleFilePath }}</span>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="submit">保存并下发</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.rl-stats {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr)) minmax(0, 2.2fr);
  gap: 12px;
}

.rl-stat {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 12px 14px;
  background: var(--waf-card-bg, #fff);
  border: 1px solid var(--waf-border, #e6e8f0);
  border-radius: 12px;
}

.rl-stat-label {
  font-size: 12px;
  color: var(--waf-muted-strong, #8a90a6);
}

.rl-stat-value {
  font-size: 22px;
  font-weight: 600;
  line-height: 1.2;
}

.rl-stat-value.is-danger {
  color: #f56c6c;
}

.rl-stat-value.is-muted {
  color: #e6a23c;
}

.rl-stat--hint {
  flex-direction: row;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  line-height: 1.6;
  color: var(--waf-muted-strong, #8a90a6);
}

.rl-pattern {
  margin-right: 6px;
  font-size: 12px;
  word-break: break-all;
}

.rl-rate b {
  font-size: 14px;
}

.rl-empty {
  padding: 28px 16px 34px;
}

.rl-empty-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
  width: 100%;
  max-width: 680px;
  margin: 0 auto;
}

.rl-empty-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 10px 12px;
  text-align: left;
  border: 1px dashed var(--waf-border, #dcdfe6);
  border-radius: 10px;
  background: transparent;
  cursor: pointer;
  transition: all 0.15s ease;
  font: inherit;
}

.rl-empty-item:hover {
  border-color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
}

.rl-empty-item b {
  font-size: 13px;
}

.rl-empty-item span {
  font-size: 11.5px;
  color: var(--waf-muted-strong, #8a90a6);
}

.rl-loading {
  display: flex;
  justify-content: center;
  padding: 48px 0;
  font-size: 22px;
  color: var(--waf-muted-strong, #8a90a6);
}

.rl-dialog-hint {
  margin-right: auto;
  font-size: 12px;
  color: var(--waf-muted-strong, #8a90a6);
}

@media (max-width: 1200px) {
  .rl-stats {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .rl-stat--hint {
    grid-column: 1 / -1;
  }
}

@media (max-width: 720px) {
  .rl-empty-grid {
    grid-template-columns: 1fr;
  }
}
</style>
