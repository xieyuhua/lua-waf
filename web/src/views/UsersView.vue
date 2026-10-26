<script setup>
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { userApi } from '../api/user'
import { viewport } from '../utils/responsive'
import { formatTime } from '../utils/format'

const loading = ref(false)
const rows = ref([])
const total = ref(0)
const page = ref(1)
const size = ref(10)
const keyword = ref('')

const dialogVisible = ref(false)
const dialogTitle = ref('新增账号')
const submitting = ref(false)
const editingId = ref(0)
const form = reactive({ username: '', password: '' })

/** 加载账号列表 */
async function load() {
  loading.value = true
  try {
    const data = await userApi.list({ page: page.value, size: size.value, keyword: keyword.value })
    rows.value = data.items ?? []
    total.value = data.total ?? 0
  } catch (error) {
    ElMessage.error(error.message || '加载账号失败')
  } finally {
    loading.value = false
  }
}

/** 打开新增弹窗 */
function openCreate() {
  editingId.value = 0
  dialogTitle.value = '新增账号'
  Object.assign(form, { username: '', password: '' })
  dialogVisible.value = true
}

/** 打开编辑弹窗 */
function openEdit(row) {
  editingId.value = row.id
  dialogTitle.value = `编辑账号 · ${row.username}`
  Object.assign(form, { username: row.username, password: '' })
  dialogVisible.value = true
}

/** 保存 */
async function submit() {
  submitting.value = true
  try {
    if (editingId.value) {
      await userApi.update(editingId.value, { username: form.username, password: form.password })
      ElMessage.success('账号已更新')
    } else {
      await userApi.create({ username: form.username, password: form.password })
      ElMessage.success('账号已创建')
    }
    dialogVisible.value = false
    load()
  } catch (error) {
    ElMessage.error(error.message || '保存失败')
  } finally {
    submitting.value = false
  }
}

/** 删除 */
async function remove(row) {
  try {
    await ElMessageBox.confirm(`确认删除账号 ${row.username}？`, '删除确认', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消'
    })
  } catch (error) {
    return
  }
  try {
    await userApi.remove(row.id)
    ElMessage.success('账号已删除')
    load()
  } catch (error) {
    ElMessage.error(error.message || '删除失败')
  }
}

onMounted(load)
</script>

<template>
  <div class="waf-page">
    <div class="waf-page-head">
      <div>
        <h1 class="waf-page-title"><span class="dot"></span> 账号管理</h1>
        <p class="waf-page-sub">管理后台登录账号，口令采用历史兼容摘要存储，建议定期更换</p>
      </div>
      <el-button type="primary" @click="openCreate"><el-icon><Plus /></el-icon> 新增账号</el-button>
    </div>

    <div class="waf-card">
      <div class="waf-card-head">
        <el-input v-model="keyword" placeholder="搜索账号" clearable style="max-width: 220px" @keyup.enter="load" @clear="load">
          <template #prefix><el-icon><Search /></el-icon></template>
        </el-input>
        <el-button text @click="load"><el-icon><RefreshRight /></el-icon> 刷新</el-button>
      </div>

      <el-table v-loading="loading" :data="rows" stripe>
        <el-table-column v-if="!viewport.isMobile.value" prop="id" label="ID" width="80" align="center" />
        <el-table-column label="用户名" min-width="160">
          <template #default="{ row }">
            <div style="display: flex; align-items: center; gap: 8px">
              <span class="waf-avatar" style="width: 26px; height: 26px; font-size: 11px">
                {{ (row.username || '?').slice(0, 1).toUpperCase() }}
              </span>
              <strong>{{ row.username }}</strong>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="最近登录" min-width="170">
          <template #default="{ row }">
            <span class="waf-muted">{{ formatTime(row.last_login) }}</span>
          </template>
        </el-table-column>
        <el-table-column v-if="!viewport.isMobile.value" label="创建时间" min-width="170">
          <template #default="{ row }">
            <span class="waf-muted">{{ formatTime(row.created) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" :width="viewport.isMobile.value ? 120 : 150" align="right" fixed="right">
          <template #default="{ row }">
            <el-button text type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button text type="danger" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
        <template #empty><div class="waf-empty">暂无账号</div></template>
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

    <el-dialog v-model="dialogVisible" :title="dialogTitle" width="440px" destroy-on-close>
      <el-form label-position="top">
        <el-form-item label="用户名" required>
          <el-input v-model="form.username" placeholder="3-32 位字母、数字、下划线" />
        </el-form-item>
        <el-form-item :label="editingId ? '新口令（留空表示不修改）' : '口令'" :required="!editingId">
          <el-input v-model="form.password" type="password" show-password placeholder="至少 8 位" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="submit">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>
