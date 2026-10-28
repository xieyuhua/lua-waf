<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { apisixApi } from '../api/apisix'

const loading = ref(true)
const saving = ref(false)
const status = ref(null)
const config = ref(null)
const configVisible = ref(false)
const configForm = reactive({ enabled: false, admin_url: '', admin_key: '', timeout_ms: 10000 })
const routes = ref([])
const keyword = ref('')

// 当前操作的路由
const current = ref(null)
const mirrorVisible = ref(false)
const limitVisible = ref(false)

const mirrorForm = reactive({ host: '', path: '', sample_ratio: 1, enabled: false })
const limitForm = reactive({
  key: 'remote_addr',
  key_type: 'var',
  count: 100,
  time_window: 60,
  rejected_code: 429,
  policy: 'local',
  show_limit_quota_header: true,
  enabled: false
})

const keyTypes = [
  { value: 'var', label: 'var（Nginx 变量，如 remote_addr / http_x_api_key）' },
  { value: 'var_combination', label: 'var_combination（组合变量）' },
  { value: 'constant', label: 'constant（固定值，全局共享计数）' }
]
const policies = [
  { value: 'local', label: 'local（单机内存计数）' },
  { value: 'redis', label: 'redis（多节点共享）' },
  { value: 'redis-cluster', label: 'redis-cluster（集群）' }
]
const keyPresets = ['remote_addr', 'http_x_api_key', 'http_authorization', 'consumer_name']

/** 加载状态与路由 */
async function load() {
  loading.value = true
  try {
    config.value = await apisixApi.config()
    status.value = await apisixApi.status()
    routes.value = status.value?.connected ? await apisixApi.routes() : []
  } catch (error) {
    ElMessage.error(error.message || '读取 APISIX 路由失败')
  } finally {
    loading.value = false
  }
}

const filtered = computed(() => {
  const text = keyword.value.trim().toLowerCase()
  if (!text) return routes.value
  return routes.value.filter((item) =>
    [item.id, item.name, ...(item.uris || []), ...(item.hosts || [])]
      .filter(Boolean)
      .some((value) => String(value).toLowerCase().includes(text))
  )
})

/** 切换集成开关（启用/关闭） */
async function toggleEnabled(value) {
  saving.value = true
  try {
    config.value = await apisixApi.saveConfig({
      enabled: value,
      admin_url: config.value?.admin_url || '',
      admin_key: '',
      timeout_ms: config.value?.timeout_ms || 10000
    })
    ElMessage.success(value ? 'APISIX 集成已启用' : 'APISIX 集成已关闭')
    await load()
  } catch (error) {
    ElMessage.error(error.message || '保存失败')
  } finally {
    saving.value = false
  }
}

/** 打开连接设置 */
function openConfig() {
  configForm.enabled = Boolean(config.value?.enabled)
  configForm.admin_url = config.value?.admin_url || ''
  configForm.admin_key = config.value?.admin_key || ''
  configForm.timeout_ms = config.value?.timeout_ms || 10000
  configVisible.value = true
}

/** 保存连接设置 */
async function saveConfig() {
  saving.value = true
  try {
    config.value = await apisixApi.saveConfig({
      enabled: configForm.enabled,
      admin_url: configForm.admin_url.trim(),
      admin_key: configForm.admin_key.trim(),
      timeout_ms: configForm.timeout_ms
    })
    ElMessage.success('配置已保存并生效')
    configVisible.value = false
    await load()
  } catch (error) {
    ElMessage.error(error.message || '保存失败')
  } finally {
    saving.value = false
  }
}

/** 快捷开关：镜像复制 */
async function toggleMirror(row, value) {
  try {
    if (value) {
      const host = row.mirror?.host || ''
      if (!host) {
        openMirror(row)
        return
      }
      await apisixApi.setMirror(row.id, { host, path: row.mirror?.path || '', sample_ratio: row.mirror?.sample_ratio ?? 1 })
      ElMessage.success('镜像复制已开启')
    } else {
      await apisixApi.clearMirror(row.id)
      ElMessage.success('镜像复制已关闭')
    }
    await load()
  } catch (error) {
    ElMessage.error(error.message || '操作失败')
  }
}

/** 快捷开关：key 限流 */
async function toggleLimit(row, value) {
  try {
    if (value) {
      const limit = row.limit
      if (!limit || !limit.key) {
        openLimit(row)
        return
      }
      await apisixApi.setLimit(row.id, limit)
      ElMessage.success('key 限流已开启')
    } else {
      await apisixApi.clearLimit(row.id)
      ElMessage.success('key 限流已关闭')
    }
    await load()
  } catch (error) {
    ElMessage.error(error.message || '操作失败')
  }
}

/** 打开镜像配置 */
function openMirror(row) {
  current.value = row
  const mirror = row.mirror
  mirrorForm.host = mirror?.host || ''
  mirrorForm.path = mirror?.path || ''
  mirrorForm.sample_ratio = mirror?.sample_ratio ?? 1
  mirrorForm.enabled = Boolean(mirror)
  mirrorVisible.value = true
}

/** 保存镜像配置 */
async function saveMirror() {
  saving.value = true
  try {
    await apisixApi.setMirror(current.value.id, {
      host: mirrorForm.host.trim(),
      path: mirrorForm.path.trim(),
      sample_ratio: mirrorForm.sample_ratio
    })
    ElMessage.success('镜像复制已下发')
    mirrorVisible.value = false
    await load()
  } catch (error) {
    ElMessage.error(error.message || '下发失败')
  } finally {
    saving.value = false
  }
}

/** 关闭镜像 */
async function disableMirror(row) {
  try {
    await ElMessageBox.confirm(`确认关闭路由「${row.name || row.id}」的流量镜像？`, '关闭镜像复制', {
      type: 'warning',
      confirmButtonText: '关闭',
      cancelButtonText: '取消'
    })
  } catch (error) {
    return
  }
  try {
    await apisixApi.clearMirror(row.id)
    ElMessage.success('镜像复制已关闭')
    await load()
  } catch (error) {
    ElMessage.error(error.message || '操作失败')
  }
}

/** 打开限流配置 */
function openLimit(row) {
  current.value = row
  const limit = row.limit
  limitForm.key = limit?.key || 'remote_addr'
  limitForm.key_type = limit?.key_type || 'var'
  limitForm.count = limit?.count ?? 100
  limitForm.time_window = limit?.time_window ?? 60
  limitForm.rejected_code = limit?.rejected_code ?? 429
  limitForm.policy = limit?.policy || 'local'
  limitForm.show_limit_quota_header = limit?.show_limit_quota_header ?? true
  limitForm.enabled = Boolean(limit)
  limitVisible.value = true
}

/** 保存限流配置 */
async function saveLimit() {
  saving.value = true
  try {
    await apisixApi.setLimit(current.value.id, {
      key: limitForm.key.trim(),
      key_type: limitForm.key_type,
      count: limitForm.count,
      time_window: limitForm.time_window,
      rejected_code: limitForm.rejected_code,
      policy: limitForm.policy,
      show_limit_quota_header: limitForm.show_limit_quota_header
    })
    ElMessage.success('key 限流已下发')
    limitVisible.value = false
    await load()
  } catch (error) {
    ElMessage.error(error.message || '下发失败')
  } finally {
    saving.value = false
  }
}

/** 关闭限流 */
async function disableLimit(row) {
  try {
    await ElMessageBox.confirm(`确认关闭路由「${row.name || row.id}」的 key 限流？`, '关闭 key 限流', {
      type: 'warning',
      confirmButtonText: '关闭',
      cancelButtonText: '取消'
    })
  } catch (error) {
    return
  }
  try {
    await apisixApi.clearLimit(row.id)
    ElMessage.success('key 限流已关闭')
    await load()
  } catch (error) {
    ElMessage.error(error.message || '操作失败')
  }
}

onMounted(load)
</script>

<template>
  <div class="waf-page">
    <div class="waf-page-head">
      <div>
        <h1 class="waf-page-title"><span class="dot"></span> APISIX 网关</h1>
        <p class="waf-page-sub">
          通过 APISIX Admin API 给路由下发插件：流量镜像复制（proxy-mirror）与按 key 限流（limit-count）
        </p>
      </div>
      <div style="display: flex; gap: 10px; align-items: center">
        <el-input v-model="keyword" placeholder="搜索路由 / URI / 域名" clearable style="width: 220px">
          <template #prefix><el-icon><Search /></el-icon></template>
        </el-input>
        <el-button type="primary" :loading="loading" @click="load"><el-icon><Refresh /></el-icon> 刷新</el-button>
      </div>
    </div>

    <div class="waf-card" style="padding: 12px 16px; margin-bottom: 16px">
      <div style="display: flex; flex-wrap: wrap; gap: 18px; align-items: center; font-size: 12.5px">
        <span class="waf-muted">管理接口</span>
        <span class="waf-mono">{{ status?.admin_url || '未配置' }}</span>
        <el-tag size="small" :type="status?.connected ? 'success' : status?.enabled ? 'danger' : 'info'" effect="light">
          {{ status?.connected ? '连接正常' : status?.enabled ? '连接失败' : '未配置' }}
        </el-tag>
        <el-tag v-if="status?.connected" size="small" effect="plain">路由 {{ status.route_count }} 条</el-tag>
        <span v-if="status?.message" class="waf-muted">{{ status.message }}</span>
        <div style="margin-left: auto; display: flex; align-items: center; gap: 10px">
          <el-switch
            :model-value="Boolean(config?.enabled)"
            :loading="saving"
            active-text="已启用"
            inactive-text="已关闭"
            @update:model-value="toggleEnabled"
          />
          <el-button text @click="openConfig"><el-icon><Setting /></el-icon> 连接设置</el-button>
        </div>
      </div>
    </div>

    <el-alert
      v-if="!loading && !status?.connected"
      type="warning"
      show-icon
      :closable="false"
      style="margin-bottom: 16px"
      title="尚未连接 APISIX 管理接口"
      description="在 conf/app.ini 增加 [apisix] 段：ADMIN_URL = http://127.0.0.1:9180/apisix/admin、ADMIN_KEY = 你的 Admin Key，重启后即可在此给路由下发插件。"
    />

    <div v-else class="waf-card">
      <div class="waf-card-head">
        <div class="waf-card-title"><span class="bar"></span> 路由列表</div>
        <div class="waf-card-extra">共 {{ filtered.length }} 条</div>
      </div>

      <el-table v-loading="loading" :data="filtered" stripe>
        <el-table-column prop="id" label="ID" width="90" />
        <el-table-column label="名称" min-width="140">
          <template #default="{ row }">{{ row.name || '-' }}</template>
        </el-table-column>
        <el-table-column label="URI" min-width="200">
          <template #default="{ row }">
            <span class="waf-mono">{{ (row.uris || []).join(' , ') || '-' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="域名" min-width="160">
          <template #default="{ row }">{{ (row.hosts || []).join(' , ') || '-' }}</template>
        </el-table-column>
        <el-table-column label="镜像复制" min-width="240">
          <template #default="{ row }">
            <div style="display: flex; align-items: center; gap: 8px">
              <el-switch :model-value="Boolean(row.mirror)" size="small" @update:model-value="(value) => toggleMirror(row, value)" />
              <template v-if="row.mirror">
                <el-tag size="small" type="success" effect="light">{{ Math.round((row.mirror.sample_ratio ?? 1) * 100) }}%</el-tag>
                <span class="waf-mono" style="font-size: 11.5px; overflow: hidden; text-overflow: ellipsis">{{ row.mirror.host }}</span>
              </template>
              <span v-else class="waf-muted">未开启</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="key 限流" min-width="240">
          <template #default="{ row }">
            <div style="display: flex; align-items: center; gap: 8px">
              <el-switch :model-value="Boolean(row.limit)" size="small" @update:model-value="(value) => toggleLimit(row, value)" />
              <template v-if="row.limit">
                <el-tag size="small" type="warning" effect="light">{{ row.limit.count }} 次 / {{ row.limit.time_window }}s</el-tag>
                <span class="waf-mono" style="font-size: 11.5px">{{ row.limit.key }}</span>
              </template>
              <span v-else class="waf-muted">未开启</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="插件数" width="80" align="center">
          <template #default="{ row }">{{ row.plugin_count }}</template>
        </el-table-column>
        <el-table-column label="操作" width="190" align="right" fixed="right">
          <template #default="{ row }">
            <el-button text type="primary" @click="openMirror(row)">镜像</el-button>
            <el-button text type="primary" @click="openLimit(row)">限流</el-button>
            <el-dropdown trigger="click" style="margin-left: 6px">
              <el-button text>更多<el-icon><ArrowDown /></el-icon></el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item :disabled="!row.mirror" @click="disableMirror(row)">关闭镜像复制</el-dropdown-item>
                  <el-dropdown-item :disabled="!row.limit" @click="disableLimit(row)">关闭 key 限流</el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </template>
        </el-table-column>
        <template #empty><div class="waf-empty">暂无路由</div></template>
      </el-table>
    </div>

    <!-- 连接设置 -->
    <el-dialog v-model="configVisible" title="APISIX 连接设置" width="560px" top="12vh">
      <el-form label-width="110px" label-position="left">
        <el-form-item label="启用集成">
          <el-switch v-model="configForm.enabled" active-text="启用" inactive-text="关闭" />
        </el-form-item>
        <el-form-item label="管理地址">
          <el-input v-model="configForm.admin_url" placeholder="http://127.0.0.1:9180/apisix/admin" />
        </el-form-item>
        <el-form-item label="Admin Key">
          <el-input v-model="configForm.admin_key" placeholder="留空表示不修改已保存的 Key" show-password />
          <div class="waf-muted" style="font-size: 11.5px; margin-top: 4px">
            保存后写入 conf/app.ini 的 [apisix] 段并立即生效，无需重启
          </div>
        </el-form-item>
        <el-form-item label="超时(毫秒)">
          <el-input-number v-model="configForm.timeout_ms" :min="1000" :max="60000" :step="1000" controls-position="right" style="width: 100%" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="configVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="saveConfig">保存</el-button>
      </template>
    </el-dialog>

    <!-- 镜像复制 -->
    <el-dialog v-model="mirrorVisible" :title="`流量镜像复制 · ${current?.name || current?.id || ''}`" width="560px" top="8vh">
      <el-form label-width="110px" label-position="left">
        <el-form-item label="镜像地址" required>
          <el-input v-model="mirrorForm.host" placeholder="http://mirror.internal:8080" />
          <div class="waf-muted" style="font-size: 11.5px; margin-top: 4px">
            需带 http:// 或 https://；APISIX 会把命中的请求体原样复制到该地址（异步、不影响主链路）
          </div>
        </el-form-item>
        <el-form-item label="路径前缀">
          <el-input v-model="mirrorForm.path" placeholder="可选，例如 /mirror" />
        </el-form-item>
        <el-form-item label="采样比例">
          <el-slider v-model="mirrorForm.sample_ratio" :min="0.01" :max="1" :step="0.01" show-input />
          <div class="waf-muted" style="font-size: 11.5px; margin-top: 4px">
            1 = 全量镜像；0.1 = 只镜像 10% 流量，建议压测/灰度时按比例采样
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="mirrorVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="saveMirror">下发</el-button>
      </template>
    </el-dialog>

    <!-- key 限流 -->
    <el-dialog v-model="limitVisible" :title="`key 限流 · ${current?.name || current?.id || ''}`" width="620px" top="8vh">
      <el-form label-width="110px" label-position="left">
        <el-form-item label="限流 key" required>
          <el-input v-model="limitForm.key" placeholder="remote_addr" />
          <div style="display: flex; gap: 6px; margin-top: 6px; flex-wrap: wrap">
            <el-tag
              v-for="preset in keyPresets"
              :key="preset"
              size="small"
              effect="plain"
              style="cursor: pointer"
              @click="limitForm.key = preset"
            >
              {{ preset }}
            </el-tag>
          </div>
        </el-form-item>
        <el-form-item label="key 类型">
          <el-select v-model="limitForm.key_type" style="width: 100%">
            <el-option v-for="item in keyTypes" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
        <div class="waf-inline-grid">
          <el-form-item label="次数">
            <el-input-number v-model="limitForm.count" :min="1" :step="10" controls-position="right" style="width: 100%" />
          </el-form-item>
          <el-form-item label="窗口(秒)">
            <el-input-number v-model="limitForm.time_window" :min="1" :step="10" controls-position="right" style="width: 100%" />
          </el-form-item>
          <el-form-item label="拒绝状态码">
            <el-input-number v-model="limitForm.rejected_code" :min="200" :max="599" :step="1" controls-position="right" style="width: 100%" />
          </el-form-item>
          <el-form-item label="计数策略">
            <el-select v-model="limitForm.policy" style="width: 100%">
              <el-option v-for="item in policies" :key="item.value" :label="item.label" :value="item.value" />
            </el-select>
          </el-form-item>
        </div>
        <el-form-item label="返回配额头">
          <el-switch v-model="limitForm.show_limit_quota_header" active-text="开启" inactive-text="关闭" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="limitVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="saveLimit">下发</el-button>
      </template>
    </el-dialog>
  </div>
</template>
