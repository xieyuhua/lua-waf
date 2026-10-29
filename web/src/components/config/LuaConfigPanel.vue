<script setup>
/**
 * WAF 配置面板（config.lua 在线编辑）
 *
 * 数据流：
 *   1. 进入页面 → GET /api/lua/config → 拿到按「模块」分组的配置项（groups[].items[]）
 *   2. 表单状态 form 以 key 为索引平铺保存（form[item.key]），便于整体提交
 *   3. 保存 → PUT /api/lua/config（只提交有变化的项；密钥类提交脱敏值表示不修改）
 *   4. 保存成功后可选「保存并生效」→ POST /api/lua/reload 执行 nginx -t && reload
 *
 * 控件类型（由后端 schema 决定，见 internal/service/luaconfig_schema.go）：
 *   switch=开关 / number=数字 / text=单行文本 / select=枚举 / secret=密钥 / code=多行模板
 *
 * 界面结构：模块选项卡（基础 / 检测开关 / 频率与请求体 / 自动封禁 / 目录 / 拦截页面 /
 * 日志输出 / MySQL 落库 / Redis 队列 / 未纳入表单），选项卡内两列网格，code 类型占满整行。
 */
import { computed, h, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { luaApi } from '../../api/lua'

const loading = ref(true)
const saving = ref(false)
const reloading = ref(false)
// 是否正在测试 Redis / MySQL 连通性
const testing = ref(false)
// 是否正在执行生效自检
const verifying = ref(false)
const rawVisible = ref(false)
const rawContent = ref('')

const meta = reactive({ path: '', exists: false, writable: false, updatedAt: '', unknown: [] })
const groups = ref([])
// 当前选中的配置模块（选项卡）
const activeGroup = ref('')

/** 表单值：key -> value */
const form = reactive({})
/** 表单原始值，用于「有改动才启用保存」 */
const origin = reactive({})
const dirty = computed(() => Object.keys(form).some((key) => String(form[key]) !== String(origin[key])))

/** 加载配置 */
async function load() {
  loading.value = true
  try {
    const data = await luaApi.config()
    meta.path = data.path
    meta.exists = data.exists
    meta.writable = data.writable
    meta.updatedAt = data.updated_at
    meta.unknown = data.unknown ?? []
    groups.value = data.groups ?? []
    if (groups.value.length && !groups.value.some((item) => item.name === activeGroup.value)) {
      activeGroup.value = groups.value[0].name
    }

    for (const key of Object.keys(form)) delete form[key]
    for (const key of Object.keys(origin)) delete origin[key]
    for (const group of groups.value) {
      for (const item of group.items) {
        const value = item.type === 'switch' ? item.value === 'on' : item.value
        form[item.key] = value
        origin[item.key] = value
      }
    }
  } catch (error) {
    ElMessage.error(error.message || '读取 config.lua 失败')
  } finally {
    loading.value = false
  }
}

/** 组装提交值（开关转 on/off） */
function collect() {
  const values = {}
  for (const group of groups.value) {
    for (const item of group.items) {
      const value = form[item.key]
      if (item.type === 'switch') {
        values[item.key] = value ? 'on' : 'off'
        continue
      }
      // 数字控件绑定的是 number，统一转成字符串再提交（后端按字符串写回 config.lua）
      if (item.type === 'number') {
        values[item.key] = value === null || value === undefined ? '' : String(value)
        continue
      }
      values[item.key] = value === null || value === undefined ? '' : String(value)
    }
  }
  return values
}

/** 保存（reload 为 true 时顺便让配置生效） */
async function save(reload = false) {
  saving.value = true
  try {
    await luaApi.save(collect())
    ElMessage.success('已写入 config.lua')
    await load()

    if (!reload) return
    reloading.value = true
    const result = await luaApi.reload()
    ElMessage.success(result?.message || '配置已生效')
  } catch (error) {
    ElMessage.error(error.message || '保存失败')
  } finally {
    saving.value = false
    reloading.value = false
  }
}

// 自检 / 连接测试结果的配色
const LEVEL_STYLE = {
  error: { color: '#f56c6c', mark: '✘' },
  warn: { color: '#e6a23c', mark: '!' },
  ok: { color: '#67c23a', mark: '✔' }
}

/** 把一条结果渲染成一行（label 加粗 + 说明），保证内容一行一行展示 */
function renderResultLine(level, label, message) {
  const style = LEVEL_STYLE[level] ?? LEVEL_STYLE.ok
  return h('div', { style: 'display:flex;gap:8px;align-items:flex-start;padding:5px 0' }, [
    h('span', { style: `flex:0 0 auto;font-weight:700;color:${style.color}` }, style.mark),
    h('span', { style: 'flex:1 1 auto;line-height:1.65;word-break:break-word' }, [
      h('b', { style: `color:${style.color}` }, `${label}：`),
      h('span', { style: 'color:#5c6070' }, message)
    ])
  ])
}

/** 组装弹窗内容：顶部概要 + 逐行结果 */
function renderResultPanel(meta, items) {
  const header = h(
    'div',
    {
      style:
        'margin-bottom:10px;padding-bottom:10px;border-bottom:1px solid #ebeef5;font-size:12.5px;color:#606266;line-height:1.9'
    },
    meta.map((line) => h('div', {}, line))
  )
  const body = h(
    'div',
    { style: 'max-height:56vh;overflow:auto;font-size:13px' },
    items.map((item) => renderResultLine(item.level, item.label, item.message))
  )
  return h('div', { style: 'text-align:left' }, [header, body])
}
/** 测试 Redis / MySQL 连通性（用表单当前值，可测未保存的配置） */
async function testConnections() {
  testing.value = true
  try {
    const result = await luaApi.test(collect())
    const items = []
    if (result?.redis) {
      items.push({
        level: result.redis.ok ? 'ok' : 'error',
        label: `Redis ${result.redis.address}`,
        message: result.redis.message
      })
    }
    if (result?.mysql) {
      items.push({
        level: result.mysql.ok ? 'ok' : 'error',
        label: `MySQL ${result.mysql.address}`,
        message: result.mysql.message
      })
    }
    if (!items.length) {
      items.push({ level: 'warn', label: '未配置 Redis 地址', message: '在「日志输出」里填写 Redis 后再测试' })
    }

    const failed = items.some((item) => item.level === 'error')
    ElMessageBox.alert(
      renderResultPanel([`测试对象：${items.length} 项`], items),
      failed ? '连接测试未通过' : '连接测试通过',
      {
        type: failed ? 'error' : 'success',
        confirmButtonText: '知道了',
        customStyle: { maxWidth: '720px' }
      }
    )
  } catch (error) {
    ElMessage.error(error.message || '测试连接失败')
  } finally {
    testing.value = false
  }
}

/** 生效自检：确认后台改的是不是 nginx 正在读取的那份 config.lua */
async function verifyConfig() {
  verifying.value = true
  try {
    const result = await luaApi.verify()
    const problems = result.items.filter((item) => item.level === 'error').length
    const notices = result.items.filter((item) => item.level === 'warn').length

    const meta = [
      `配置路径：${result.path}`,
      `日志模式：${result.mode || '未设置'}`,
      `检查结果：${problems} 项问题 / ${notices} 项提醒 / ${result.items.length} 项检查`
    ]

    ElMessageBox.alert(
      renderResultPanel(meta, result.items),
      result.verdict === 'error' ? '自检发现问题' : result.verdict === 'warn' ? '自检有提醒' : '自检通过',
      {
        type: result.verdict === 'error' ? 'error' : result.verdict === 'warn' ? 'warning' : 'success',
        confirmButtonText: '知道了',
        customStyle: { maxWidth: '760px' }
      }
    )
  } catch (error) {
    ElMessage.error(error.message || '自检失败')
  } finally {
    verifying.value = false
  }
}

/** 只执行 reload */
async function reloadOnly() {
  try {
    await ElMessageBox.confirm('将对 nginx 执行 nginx -t 检查并 reload，是否继续？', '让配置生效', {
      type: 'warning',
      confirmButtonText: '执行',
      cancelButtonText: '取消'
    })
  } catch (error) {
    return
  }
  reloading.value = true
  try {
    const result = await luaApi.reload()
    ElMessage.success(result?.message || '配置已生效')
  } catch (error) {
    ElMessage.error(error.message || 'reload 失败')
  } finally {
    reloading.value = false
  }
}

/** 查看原文 */
async function viewRaw() {
  try {
    const data = await luaApi.raw()
    rawContent.value = data.content
    rawVisible.value = true
  } catch (error) {
    ElMessage.error(error.message || '读取配置原文失败')
  }
}

onMounted(load)
</script>

<template>
  <div class="config-panel">
    <div class="panel-toolbar">
      <p class="panel-hint">
        openresty 侧的 config.lua；保存后点「保存并生效」执行 nginx -t 与 reload
      </p>
      <div class="panel-actions">
        <el-button :loading="testing" @click="testConnections"><el-icon><Connection /></el-icon> 测试连接</el-button>
        <el-button :loading="verifying" @click="verifyConfig"><el-icon><CircleCheck /></el-icon> 生效自检</el-button>
        <el-button text :disabled="!meta.exists" @click="viewRaw"><el-icon><Document /></el-icon> 查看原文</el-button>
        <el-button :loading="reloading" @click="reloadOnly"><el-icon><RefreshRight /></el-icon> 仅 reload</el-button>
        <el-button :disabled="!meta.exists || !dirty" :loading="saving" @click="save(false)">
          <el-icon><Check /></el-icon> 保存
        </el-button>
        <el-button type="primary" :disabled="!meta.exists || !dirty" :loading="saving || reloading" @click="save(true)">
          <el-icon><MagicStick /></el-icon> 保存并生效
        </el-button>
      </div>
    </div>

    <div class="waf-card" style="padding: 12px 16px; margin-bottom: 16px">
      <div style="display: flex; flex-wrap: wrap; gap: 18px; align-items: center; font-size: 12.5px">
        <span class="waf-muted">配置文件</span>
        <span class="waf-mono">{{ meta.path || '-' }}</span>
        <el-tag size="small" :type="meta.exists ? 'success' : 'danger'" effect="light">
          {{ meta.exists ? '已找到' : '不存在' }}
        </el-tag>
        <el-tag v-if="meta.exists" size="small" :type="meta.writable ? 'success' : 'danger'" effect="light">
          {{ meta.writable ? '可写' : '不可写（检查目录权限）' }}
        </el-tag>
        <span v-if="meta.updatedAt" class="waf-muted">最近修改 {{ meta.updatedAt }}</span>
        <el-tag v-if="dirty" size="small" type="warning" effect="light">有未保存修改</el-tag>
      </div>
    </div>

    <el-alert
      v-if="!loading && !meta.exists"
      type="error"
      show-icon
      :closable="false"
      style="margin-bottom: 16px"
      title="未找到 config.lua"
      description="请确认 lua-waf 已部署到该路径，或在 conf/app.ini 的 [waf] LUA_CONFIG 中指定正确路径（默认 /www/server/nginx/x-waf/config.lua）。"
    />

    <el-skeleton v-if="loading" :rows="10" animated />

    <template v-else-if="meta.exists">
      <div class="waf-card" style="padding: 8px 14px 14px">
        <el-tabs v-model="activeGroup" class="module-tabs">
          <el-tab-pane v-for="group in groups" :key="group.name" :name="group.name">
            <template #label>
              <span class="module-tab-label">
                {{ group.name }}
                <el-tag size="small" type="info" effect="plain" round>{{ group.items.length }}</el-tag>
              </span>
            </template>

          <div class="lua-fields lua-fields--grid">
            <div v-for="item in group.items" :key="item.key" class="lua-field" :class="{ 'is-wide': item.type === 'code' }">
              <div class="lua-label">
                <span>{{ item.label }}</span>
                <code class="lua-key">{{ item.key }}</code>
              </div>

              <el-switch
                v-if="item.type === 'switch'"
                v-model="form[item.key]"
                active-text="开启"
                inactive-text="关闭"
              />

              <el-input-number
                v-else-if="item.type === 'number'"
                v-model="form[item.key]"
                :min="item.min || 0"
                :max="item.max || undefined"
                :step="1"
                controls-position="right"
                style="width: 100%"
              />

              <el-select v-else-if="item.type === 'select'" v-model="form[item.key]" style="width: 100%">
                <el-option v-for="option in item.options" :key="option" :label="option" :value="option" />
              </el-select>

              <el-input
                v-else-if="item.type === 'secret'"
                v-model="form[item.key]"
                type="password"
                show-password
                placeholder="留空或保持脱敏值表示不修改"
              />

              <el-input
                v-else-if="item.type === 'code'"
                v-model="form[item.key]"
                type="textarea"
                :autosize="{ minRows: 12, maxRows: 28 }"
                spellcheck="false"
                class="lua-code"
                placeholder="多行模板内容，不支持包含 ]]"
              />

              <el-input v-else v-model="form[item.key]" />

              <div v-if="item.desc" class="lua-desc">{{ item.desc }}</div>
            </div>
          </div>
          </el-tab-pane>

          <el-tab-pane v-if="meta.unknown.length" name="__unknown__" label="未纳入表单">
            <p class="waf-muted" style="font-size: 12px; line-height: 1.8">
              以下键在 config.lua 中存在，但暂不支持在线编辑（例如多行 HTML 模板），修改请直接编辑文件：
            </p>
            <div style="display: flex; flex-wrap: wrap; gap: 6px">
              <el-tag v-for="key in meta.unknown" :key="key" size="small" type="info">{{ key }}</el-tag>
            </div>
          </el-tab-pane>
        </el-tabs>
      </div>
    </template>

    <el-dialog v-model="rawVisible" title="config.lua 原文" width="860px" top="6vh">
      <div class="waf-code" style="max-height: 62vh">{{ rawContent }}</div>
      <template #footer>
        <el-button @click="rawVisible = false">关闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.lua-fields {
  display: grid;
  gap: 14px;
}

.lua-field.is-wide {
  grid-column: 1 / -1;
}

.lua-code :deep(textarea) {
  font-family: 'JetBrains Mono', 'Cascadia Code', Consolas, monospace;
  font-size: 12.5px;
  line-height: 1.6;
}

.lua-fields--grid {
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px 24px;
  padding-top: 6px;
}

@media (max-width: 900px) {
  
.lua-fields--grid {
    grid-template-columns: minmax(0, 1fr);
  }
}

.module-tab-label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.module-tabs :deep(.el-tabs__header) {
  margin-bottom: 14px;
}

.module-tabs :deep(.el-tabs__item) {
  font-size: 13px;
  font-weight: 600;
}

.lua-label {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  font-size: 13px;
  font-weight: 600;
  color: var(--waf-text);
  margin-bottom: 6px;
}

.lua-key {
  font-size: 11px;
  color: var(--waf-text-dim);
  background: var(--waf-surface-2);
  border: 1px solid var(--waf-border);
  border-radius: 6px;
  padding: 1px 6px;
}

.lua-desc {
  margin-top: 5px;
  font-size: 11.5px;
  line-height: 1.6;
  color: var(--waf-text-muted);
}
</style>
