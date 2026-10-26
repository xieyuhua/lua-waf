<script setup>
import { onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { authApi } from '../api/auth'
import { systemApi } from '../api/system'
import { authStore } from '../stores/auth'
import { formatTime } from '../utils/format'

const saving = ref(false)
const info = ref(null)
const profile = ref(null)
const form = reactive({ old_password: '', new_password: '', confirm: '' })

/** 加载账号与系统信息 */
async function load() {
  try {
    const [profileData, infoData] = await Promise.all([authApi.profile(), systemApi.info()])
    profile.value = profileData
    info.value = infoData
    authStore.state.user = profileData
  } catch (error) {
    ElMessage.error(error.message || '加载信息失败')
  }
}

/** 修改口令 */
async function changePassword() {
  if (!form.old_password || !form.new_password) {
    ElMessage.warning('请填写原口令与新口令')
    return
  }
  if (form.new_password !== form.confirm) {
    ElMessage.warning('两次输入的新口令不一致')
    return
  }
  saving.value = true
  try {
    await authApi.changePassword({ old_password: form.old_password, new_password: form.new_password })
    ElMessage.success('口令已更新')
    Object.assign(form, { old_password: '', new_password: '', confirm: '' })
  } catch (error) {
    ElMessage.error(error.message || '修改失败')
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="waf-page">
    <div class="waf-page-head">
      <div>
        <h1 class="waf-page-title"><span class="dot"></span> 账号设置</h1>
        <p class="waf-page-sub">查看当前登录信息、修改口令并了解系统运行参数</p>
      </div>
    </div>

    <div class="waf-grid waf-grid--2">
      <div class="waf-card">
        <div class="waf-card-head">
          <div class="waf-card-title"><span class="bar"></span> 当前账号</div>
        </div>

        <div class="profile-row">
          <span class="waf-avatar" style="width: 46px; height: 46px; font-size: 18px">
            {{ (profile?.username || 'A').slice(0, 1).toUpperCase() }}
          </span>
          <div>
            <div style="font-size: 16px; font-weight: 700">{{ profile?.username || '-' }}</div>
            <div class="waf-muted" style="font-size: 12.5px; margin-top: 2px">
              最近登录：{{ formatTime(profile?.last_login) }}
            </div>
          </div>
        </div>

        <el-divider />

        <el-form label-position="top" @submit.prevent="changePassword">
          <el-form-item label="原口令">
            <el-input v-model="form.old_password" type="password" show-password placeholder="请输入当前口令" />
          </el-form-item>
          <el-form-item label="新口令">
            <el-input v-model="form.new_password" type="password" show-password placeholder="至少 8 位" />
          </el-form-item>
          <el-form-item label="确认新口令">
            <el-input v-model="form.confirm" type="password" show-password placeholder="再次输入新口令" />
          </el-form-item>
          <el-button type="primary" :loading="saving" @click="changePassword">更新口令</el-button>
        </el-form>
      </div>

      <div class="waf-card">
        <div class="waf-card-head">
          <div class="waf-card-title"><span class="bar"></span> 系统运行信息</div>
        </div>

        <el-descriptions :column="1" border>
          <el-descriptions-item label="版本">{{ info?.version || '-' }}</el-descriptions-item>
          <el-descriptions-item label="运行模式">
            <el-tag size="small" :type="info?.demo ? 'warning' : 'success'" effect="light">
              {{ info?.run_mode || '-' }}{{ info?.demo ? '（演示数据）' : '' }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="监听端口">{{ info?.http_port ?? '-' }}</el-descriptions-item>
          <el-descriptions-item label="分页大小">{{ info?.page_size ?? '-' }}</el-descriptions-item>
          <el-descriptions-item label="nginx 可执行文件">
            <span class="waf-mono">{{ info?.nginx_bin || '-' }}</span>
          </el-descriptions-item>
          <el-descriptions-item label="vhost 目录">
            <span class="waf-mono">{{ info?.vhosts || '-' }}</span>
          </el-descriptions-item>
          <el-descriptions-item label="规则目录">
            <span class="waf-mono">{{ info?.rule_path || '-' }}</span>
          </el-descriptions-item>
          <el-descriptions-item label="集群节点">
            <el-tag v-for="server in info?.api_servers || []" :key="server" size="small" effect="plain" style="margin-right: 6px">
              {{ server }}
            </el-tag>
          </el-descriptions-item>
        </el-descriptions>

        <el-alert
          v-if="info?.nginx_dry_run"
          title="当前为演示模式：站点与规则配置只做渲染，不会写入文件或重载 nginx"
          type="info"
          :closable="false"
          show-icon
          style="margin-top: 14px"
        />
      </div>
    </div>
  </div>
</template>

<style scoped>
.profile-row {
  display: flex;
  align-items: center;
  gap: 14px;
}
</style>
