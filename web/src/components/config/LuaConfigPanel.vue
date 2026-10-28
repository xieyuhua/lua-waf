<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { luaApi } from '../../api/lua'

const loading = ref(true)
const saving = ref(false)
const reloading = ref(false)
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
      values[item.key] = item.type === 'switch' ? (value ? 'on' : 'off') : value
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
            <div v-for="item in group.items" :key="item.key" class="lua-field">
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
