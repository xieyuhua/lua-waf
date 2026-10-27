<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { systemApi } from '../../api/system'

const loading = ref(true)
const saving = ref(false)
const reloading = ref(false)
const rawVisible = ref(false)
const rawContent = ref('')
const meta = reactive({ file: '', source: '', writable: true })
const restartRequired = ref([])

/** 表单：与后端 ConfigPayload 对齐 */
const form = reactive({
  run_mode: 'demo',
  http_port: 5002,
  api_key: '',
  api_servers_text: '',
  nginx_bin: '',
  nginx_vhosts: '',
  ssl_cert_dir: '',
  rule_path: '',
  waf_log_dir: '',
  page_size: 10,
  session_secret: '',
  session_ttl: '12h',
  session_secure: false,
  captcha_ttl: '5m',
  slow_sql_ms: 200,
  db_host: '127.0.0.1',
  db_port: 3306,
  db_user: '',
  db_password: '',
  db_name: '',
  db_show_sql: false
})

const runModes = [
  { value: 'demo', label: 'demo（内存仓储 + 演示数据）' },
  { value: 'dev', label: 'dev（MySQL，详细日志）' },
  { value: 'prod', label: 'prod（MySQL，生产）' }
]

const isDemo = computed(() => form.run_mode === 'demo')

/** 加载运行配置 */
async function load() {
  loading.value = true
  try {
    applyView(await systemApi.config())
  } catch (error) {
    ElMessage.error(error.message || '读取运行配置失败')
  } finally {
    loading.value = false
  }
}

/** 把后端返回写入表单与元信息 */
function applyView(view) {
  const data = view?.config ?? {}
  Object.assign(form, {
    run_mode: data.run_mode ?? 'demo',
    http_port: data.http_port ?? 5002,
    api_key: data.api_key ?? '',
    api_servers_text: (data.api_servers ?? []).join(', '),
    nginx_bin: data.nginx_bin ?? '',
    nginx_vhosts: data.nginx_vhosts ?? '',
    ssl_cert_dir: data.ssl_cert_dir ?? '',
    rule_path: data.rule_path ?? '',
    waf_log_dir: data.waf_log_dir ?? '',
    page_size: data.page_size ?? 10,
    session_secret: data.session_secret ?? '',
    session_ttl: data.session_ttl ?? '12h',
    session_secure: Boolean(data.session_secure),
    captcha_ttl: data.captcha_ttl ?? '5m',
    slow_sql_ms: data.slow_sql_ms ?? 200,
    db_host: data.database?.host ?? '127.0.0.1',
    db_port: data.database?.port ?? 3306,
    db_user: data.database?.user ?? '',
    db_password: data.database?.password ?? '',
    db_name: data.database?.name ?? '',
    db_show_sql: Boolean(data.database?.show_sql)
  })
  meta.file = view?.file ?? ''
  meta.source = view?.source ?? ''
  meta.writable = view?.writable ?? false
  restartRequired.value = view?.restart_required ?? []
}

/** 组装保存载荷 */
function buildPayload() {
  return {
    run_mode: form.run_mode,
    http_port: Number(form.http_port),
    api_key: form.api_key,
    api_servers: String(form.api_servers_text || '')
      .split(',')
      .map((item) => item.trim())
      .filter(Boolean),
    nginx_bin: form.nginx_bin,
    nginx_vhosts: form.nginx_vhosts,
    ssl_cert_dir: form.ssl_cert_dir,
    rule_path: form.rule_path,
    waf_log_dir: form.waf_log_dir,
    page_size: Number(form.page_size),
    session_secret: form.session_secret,
    session_ttl: form.session_ttl,
    session_secure: form.session_secure,
    captcha_ttl: form.captcha_ttl,
    slow_sql_ms: Number(form.slow_sql_ms),
    database: {
      host: form.db_host,
      port: Number(form.db_port),
      user: form.db_user,
      password: form.db_password,
      name: form.db_name,
      show_sql: Boolean(form.db_show_sql)
    }
  }
}

/** 保存配置 */
async function save() {
  saving.value = true
  try {
    const view = await systemApi.saveConfig(buildPayload())
    applyView(view)
    ElMessage.success('配置已保存并写入配置文件')
  } catch (error) {
    ElMessage.error(error.message || '保存失败')
  } finally {
    saving.value = false
  }
}

/** 从磁盘重新加载 */
async function reloadFromDisk() {
  try {
    await ElMessageBox.confirm('将丢弃当前表单修改，并从配置文件重新读取，是否继续？', '重新加载', {
      type: 'warning',
      confirmButtonText: '重新加载',
      cancelButtonText: '取消'
    })
  } catch (error) {
    return
  }
  reloading.value = true
  try {
    const view = await systemApi.reloadConfig()
    applyView(view)
    ElMessage.success('已从磁盘重新加载配置')
  } catch (error) {
    ElMessage.error(error.message || '重新加载失败')
  } finally {
    reloading.value = false
  }
}

/** 查看配置文件原文 */
async function viewRaw() {
  try {
    const data = await systemApi.rawConfig()
    rawContent.value = data.content
    rawVisible.value = true
  } catch (error) {
    ElMessage.error(error.message || '读取配置文件失败')
  }
}

onMounted(load)
</script>

<template>
  <div class="config-panel">
    <div class="panel-toolbar">
      <p class="panel-hint">
        conf/app.ini 运行时参数；保存即写回文件并热更新，标注「需重启」的项需重启进程才生效
      </p>
      <div class="panel-actions">
        <el-button text @click="viewRaw"><el-icon><Document /></el-icon> 查看原文</el-button>
        <el-button :loading="reloading" @click="reloadFromDisk"><el-icon><RefreshLeft /></el-icon> 从磁盘重载</el-button>
        <el-button type="primary" :loading="saving" @click="save"><el-icon><Check /></el-icon> 保存配置</el-button>
      </div>
    </div>

    <div class="waf-card" style="padding: 12px 16px; margin-bottom: 16px">
      <div style="display: flex; flex-wrap: wrap; gap: 18px; align-items: center; font-size: 12.5px">
        <span class="waf-muted">配置文件</span>
        <span class="waf-mono">{{ meta.file || '-' }}</span>
        <el-tag size="small" effect="plain">来源 {{ meta.source || '-' }}</el-tag>
        <el-tag size="small" :type="meta.writable ? 'success' : 'danger'" effect="light">
          {{ meta.writable ? '可写' : '不可写（请检查目录权限）' }}
        </el-tag>
        <el-tag v-if="isDemo" size="small" type="warning" effect="light">演示模式：配置下发只渲染不落盘</el-tag>
      </div>
    </div>

    <el-alert
      v-if="restartRequired.length"
      type="warning"
      show-icon
      :closable="false"
      style="margin-bottom: 16px"
      title="以下配置已写入文件，但需要重启进程才生效"
      :description="restartRequired.join('、')"
    />

    <el-skeleton v-if="loading" :rows="10" animated />

    <template v-else>
      <div class="waf-grid waf-grid--2">
        <!-- 基础与集群 -->
        <div class="waf-card">
          <div class="waf-card-head">
            <div class="waf-card-title"><span class="bar"></span> 基本信息与集群</div>
          </div>

          <el-form label-position="top">
            <el-form-item label="运行模式（需重启）">
              <el-select v-model="form.run_mode" style="width: 100%">
                <el-option v-for="mode in runModes" :key="mode.value" :label="mode.label" :value="mode.value" />
              </el-select>
            </el-form-item>

            <el-form-item label="监听端口 HTTP_PORT（需重启）">
              <el-input-number v-model="form.http_port" :min="1" :max="65535" style="width: 100%" />
            </el-form-item>

            <el-form-item label="同步密钥 API_KEY（多台 WAF 必须一致）">
              <el-input v-model="form.api_key" placeholder="留空表示不修改" />
            </el-form-item>

            <el-form-item label="集群节点 API_SERVERS（逗号分隔）">
              <el-input v-model="form.api_servers_text" placeholder="127.0.0.1,10.0.0.9" />
            </el-form-item>

            <div class="waf-inline-grid">
              <el-form-item label="分页大小 PAGEMax">
                <el-input-number v-model="form.page_size" :min="1" :max="200" style="width: 100%" />
              </el-form-item>
              <el-form-item label="慢 SQL 阈值（毫秒）">
                <el-input-number v-model="form.slow_sql_ms" :min="10" :max="60000" :step="50" style="width: 100%" />
              </el-form-item>
            </div>
          </el-form>
        </div>

        <!-- nginx 与路径 -->
        <div class="waf-card">
          <div class="waf-card-head">
            <div class="waf-card-title"><span class="bar"></span> nginx 与目录</div>
            <div class="waf-card-extra">保存后立即热更新</div>
          </div>

          <el-form label-position="top">
            <el-form-item label="nginx 可执行文件 NGINX_BIN">
              <el-input v-model="form.nginx_bin" placeholder="/www/server/nginx/sbin/nginx" />
            </el-form-item>
            <el-form-item label="vhost 目录 NGINX_VHOSTS">
              <el-input v-model="form.nginx_vhosts" placeholder="/www/server/panel/vhost/nginx/" />
            </el-form-item>
            <el-form-item label="证书目录 SSL_CERT_DIR">
              <el-input v-model="form.ssl_cert_dir" placeholder="/www/server/panel/vhost/cert" />
            </el-form-item>
            <el-form-item label="规则目录 RULE_PATH（须与 lua config_rule_dir 一致）">
              <el-input v-model="form.rule_path" placeholder="/www/server/nginx/x-waf/rules/" />
            </el-form-item>
            <el-form-item label="WAF 日志目录 LOG_DIR">
              <el-input v-model="form.waf_log_dir" placeholder="/www/wwwlogs/waf" />
            </el-form-item>
          </el-form>
        </div>

        <!-- 安全 -->
        <div class="waf-card">
          <div class="waf-card-head">
            <div class="waf-card-title"><span class="bar"></span> 安全与会话</div>
          </div>

          <el-form label-position="top">
            <el-form-item label="会话密钥 SESSION_SECRET（需重启；留空表示不修改）">
              <el-input v-model="form.session_secret" placeholder="不少于 16 位随机字符串" />
            </el-form-item>
            <div class="waf-inline-grid">
              <el-form-item label="会话有效期 SESSION_TTL">
                <el-input v-model="form.session_ttl" placeholder="12h / 30m" />
              </el-form-item>
              <el-form-item label="验证码有效期 CAPTCHA_TTL">
                <el-input v-model="form.captcha_ttl" placeholder="5m" />
              </el-form-item>
            </div>
            <el-form-item label="仅 HTTPS 下发 Cookie（SESSION_SECURE）">
              <el-switch v-model="form.session_secure" active-text="启用" inactive-text="关闭" />
            </el-form-item>
          </el-form>
        </div>

        <!-- 数据库 -->
        <div class="waf-card">
          <div class="waf-card-head">
            <div class="waf-card-title"><span class="bar"></span> 数据库（需重启）</div>
            <div class="waf-card-extra">demo 模式下不生效</div>
          </div>

          <el-form label-position="top">
            <div class="waf-inline-grid waf-inline-grid--wide">
              <el-form-item label="主机 HOST">
                <el-input v-model="form.db_host" placeholder="127.0.0.1" />
              </el-form-item>
              <el-form-item label="端口 PORT">
                <el-input-number v-model="form.db_port" :min="1" :max="65535" style="width: 100%" />
              </el-form-item>
            </div>
            <div class="waf-inline-grid">
              <el-form-item label="账号 USER">
                <el-input v-model="form.db_user" placeholder="waf" />
              </el-form-item>
              <el-form-item label="口令 PASSWD（留空表示不修改）">
                <el-input v-model="form.db_password" type="password" show-password placeholder="留空表示不修改" />
              </el-form-item>
            </div>
            <el-form-item label="库名 NAME">
              <el-input v-model="form.db_name" placeholder="waf" />
            </el-form-item>
            <el-form-item label="打印执行的 SQL（database.SHOW_SQL，热更新）">
              <el-switch v-model="form.db_show_sql" active-text="开启" inactive-text="关闭" />
            </el-form-item>
          </el-form>

          <el-alert
            type="info"
            :closable="false"
            show-icon
            title="修改数据库连接后需重启进程；重启前请确认新库已初始化（会自动建表并写入初始账号与内置规则）"
          />
          <p class="waf-muted" style="font-size: 12px; margin-top: 10px; line-height: 1.7">
            开启 SQL 日志会记录全部查询参数，生产环境排障后建议关闭，避免日志量过大与敏感数据外泄。
          </p>
        </div>
      </div>
    </template>

    <el-dialog v-model="rawVisible" title="conf/app.ini 原文" width="820px" top="6vh">
      <div class="waf-code" style="max-height: 60vh">{{ rawContent }}</div>
      <template #footer>
        <el-button @click="rawVisible = false">关闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>
