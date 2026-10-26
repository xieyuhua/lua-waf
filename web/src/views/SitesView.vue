<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { siteApi } from '../api/site'
import { formatTime } from '../utils/format'

const loading = ref(false)
const syncing = ref(false)
const rows = ref([])
const total = ref(0)
const page = ref(1)
const size = ref(10)
const keyword = ref('')

const drawerVisible = ref(false)
const drawerTitle = ref('新增站点')
const submitting = ref(false)
const editingId = ref(0)

const previewVisible = ref(false)
const previewContent = ref('')

const form = reactive({
  site_name: '',
  port: 80,
  backend: '',
  unreal: '',
  ssl: 'off',
  debug_level: 'error'
})

const debugLevels = ['debug', 'info', 'notice', 'warn', 'error', 'crit', 'alert', 'emerg']

const sslTag = (value) => (value === 'on' ? 'success' : 'info')

/** 加载站点列表 */
async function load() {
  loading.value = true
  try {
    const data = await siteApi.list({ page: page.value, size: size.value, keyword: keyword.value })
    rows.value = data.items ?? []
    total.value = data.total ?? 0
  } catch (error) {
    ElMessage.error(error.message || '加载站点失败')
  } finally {
    loading.value = false
  }
}

/** 打开新增抽屉 */
function openCreate() {
  editingId.value = 0
  drawerTitle.value = '新增站点'
  Object.assign(form, { site_name: '', port: 80, backend: '', unreal: '', ssl: 'off', debug_level: 'error' })
  drawerVisible.value = true
}

/** 打开编辑抽屉 */
async function openEdit(row) {
  editingId.value = row.id
  drawerTitle.value = `编辑站点 · ${row.site_name}`
  Object.assign(form, {
    site_name: row.site_name,
    port: row.port,
    backend: (row.backend_addr ?? []).join('\n'),
    unreal: (row.unreal_addr ?? []).join('\n'),
    ssl: row.ssl || 'off',
    debug_level: row.debug_level || 'error'
  })
  drawerVisible.value = true
}

/** 提交表单 */
async function submit() {
  const payload = {
    site_name: form.site_name.trim(),
    port: Number(form.port),
    backend_addr: splitLines(form.backend),
    unreal_addr: splitLines(form.unreal),
    ssl: form.ssl,
    debug_level: form.debug_level
  }
  submitting.value = true
  try {
    if (editingId.value) {
      await siteApi.update(editingId.value, payload)
      ElMessage.success('站点已更新')
    } else {
      await siteApi.create(payload)
      ElMessage.success('站点已创建')
    }
    drawerVisible.value = false
    load()
  } catch (error) {
    ElMessage.error(error.message || '保存失败')
  } finally {
    submitting.value = false
  }
}

/** 删除站点 */
async function remove(row) {
  try {
    await ElMessageBox.confirm(`确认删除站点 ${row.site_name}？该操作会同时移除对应 vhost 配置。`, '删除确认', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消'
    })
  } catch (error) {
    return
  }
  try {
    await siteApi.remove(row.id)
    ElMessage.success('站点已删除')
    load()
  } catch (error) {
    ElMessage.error(error.message || '删除失败')
  }
}

/** 查看渲染后的 nginx 配置 */
async function preview(row) {
  try {
    const data = await siteApi.preview(row.id)
    previewContent.value = data.content
    previewVisible.value = true
  } catch (error) {
    ElMessage.error(error.message || '渲染配置失败')
  }
}

/** 同步单个站点 */
async function syncOne(row) {
  syncing.value = true
  try {
    const data = await siteApi.syncOne(row.id)
    showSyncResult(data)
  } catch (error) {
    ElMessage.error(error.message || '同步失败')
  } finally {
    syncing.value = false
  }
}

/** 同步全部站点 */
async function syncAll() {
  syncing.value = true
  try {
    const data = await siteApi.syncAll()
    showSyncResult(data)
  } catch (error) {
    ElMessage.error(error.message || '同步失败')
  } finally {
    syncing.value = false
  }
}

/** 展示同步结果 */
function showSyncResult(data) {
  const nodes = data?.nodes ?? []
  if (nodes.length === 0) {
    ElMessage.success(`${data?.message || '配置已生效'}（已应用 ${data?.applied ?? 0} 项）`)
    return
  }
  const failed = nodes.filter((item) => item.status !== 'success')
  if (failed.length === 0) {
    ElMessage.success(`${data?.message || '配置已生效'}，${nodes.length} 个节点同步成功`)
  } else {
    ElMessage.warning(`${failed.length} 个节点同步失败：${failed.map((item) => item.server).join(', ')}`)
  }
}

/** 拆分多行文本 */
function splitLines(text) {
  return String(text || '')
    .split('\n')
    .map((item) => item.trim())
    .filter(Boolean)
}

const backendCount = computed(() => splitLines(form.backend).length)

onMounted(load)
</script>

<template>
  <div class="waf-page">
    <div class="waf-page-head">
      <div>
        <h1 class="waf-page-title"><span class="dot"></span> 站点管理</h1>
        <p class="waf-page-sub">配置受保护域名与后端应用服务器，同步后由 openresty 生效</p>
      </div>
      <div style="display: flex; gap: 10px">
        <el-button :loading="syncing" @click="syncAll">
          <el-icon><Refresh /></el-icon> 同步全部
        </el-button>
        <el-button type="primary" @click="openCreate">
          <el-icon><Plus /></el-icon> 新增站点
        </el-button>
      </div>
    </div>

    <div class="waf-card">
      <div class="waf-card-head">
        <el-input v-model="keyword" placeholder="搜索域名" clearable style="max-width: 240px" @keyup.enter="load" @clear="load">
          <template #prefix><el-icon><Search /></el-icon></template>
        </el-input>
        <el-button text @click="load"><el-icon><RefreshRight /></el-icon> 刷新</el-button>
      </div>

      <el-table v-loading="loading" :data="rows" stripe style="width: 100%">
        <el-table-column prop="site_name" label="域名" min-width="200">
          <template #default="{ row }">
            <strong>{{ row.site_name }}</strong>
            <span class="waf-muted" style="margin-left: 6px">:{{ row.port }}</span>
          </template>
        </el-table-column>
        <el-table-column label="后端服务器" min-width="240">
          <template #default="{ row }">
            <div style="display: flex; flex-wrap: wrap; gap: 4px">
              <el-tag v-for="addr in row.backend_addr" :key="addr" size="small" effect="plain">{{ addr }}</el-tag>
              <span v-if="!(row.backend_addr || []).length" class="waf-muted">-</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="SSL" width="90" align="center">
          <template #default="{ row }">
            <el-tag :type="sslTag(row.ssl)" size="small" effect="light">{{ row.ssl === 'on' ? '已启用' : '关闭' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="debug_level" label="日志级别" width="110" align="center" />
        <el-table-column label="最后变更" min-width="160">
          <template #default="{ row }">
            <span class="waf-muted" style="font-size: 12.5px">{{ formatTime(row.last_change) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="230" align="right">
          <template #default="{ row }">
            <el-button text type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button text type="primary" @click="preview(row)">配置</el-button>
            <el-button text type="primary" @click="syncOne(row)">同步</el-button>
            <el-button text type="danger" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
        <template #empty>
          <div class="waf-empty">暂无站点，点击右上角「新增站点」开始配置</div>
        </template>
      </el-table>

      <div style="display: flex; justify-content: flex-end; margin-top: 14px">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="size"
          :total="total"
          :page-sizes="[10, 20, 50]"
          layout="total, sizes, prev, pager, next"
          @current-change="load"
          @size-change="load"
        />
      </div>
    </div>

    <!-- 新增 / 编辑抽屉 -->
    <el-drawer v-model="drawerVisible" :title="drawerTitle" size="480px" destroy-on-close>
      <el-form label-position="top">
        <el-form-item label="站点域名" required>
          <el-input v-model="form.site_name" placeholder="例如 www.example.com" />
        </el-form-item>
        <el-form-item label="监听端口" required>
          <el-input-number v-model="form.port" :min="1" :max="65535" style="width: 100%" />
        </el-form-item>
        <el-form-item :label="`后端服务器（每行一个，当前 ${backendCount} 个）`" required>
          <el-input v-model="form.backend" type="textarea" :rows="4" placeholder="10.0.0.11:8080&#10;10.0.0.12:8080" />
        </el-form-item>
        <el-form-item label="诱饵地址（命中后直接返回 404，每行一个）">
          <el-input v-model="form.unreal" type="textarea" :rows="2" placeholder="1.1.1.1" />
        </el-form-item>
        <el-form-item label="SSL 状态">
          <el-radio-group v-model="form.ssl">
            <el-radio-button value="off">关闭</el-radio-button>
            <el-radio-button value="on">启用</el-radio-button>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="日志级别">
          <el-select v-model="form.debug_level" style="width: 100%">
            <el-option v-for="level in debugLevels" :key="level" :label="level" :value="level" />
          </el-select>
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="drawerVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="submit">保存</el-button>
      </template>
    </el-drawer>

    <!-- 配置预览 -->
    <el-dialog v-model="previewVisible" title="生成的 nginx 配置" width="760px" top="6vh">
      <div class="waf-code">{{ previewContent }}</div>
      <template #footer>
        <el-button @click="previewVisible = false">关闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>
