<script setup>
/**
 * 告警通知：命中「勾选的规则」时推送企业微信群机器人
 *
 * 用法：
 *   1. 在企业微信群里「添加群机器人」拿到 Webhook 地址；
 *   2. 打开总开关、粘贴 Webhook、点「发送测试」确认能收到；
 *   3. 在下方规则列表里勾选需要提醒的规则（可搜索过滤、可按类型筛选）；
 *   4. 保存。之后日志入库时，命中这些规则的攻击会推送一条消息到群里。
 *
 * 说明：
 *   - 静默期内的同一条规则只提醒一次，避免刷屏；
 *   - 推送只发生在后台消费进程里，失败只记日志，不影响日志入库。
 */
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { alertApi } from '../../api/alert'
import { ruleApi } from '../../api/rule'

const loading = ref(false)
const saving = ref(false)
const testing = ref(false)

const form = reactive({
  enabled: false,
  webhook_url: '',
  interval_sec: 60,
  mention_all: false
})

// 勾选状态：id -> true
const selected = reactive(new Set())
const rules = ref([])
const types = ref([])
const keyword = ref('')
const activeType = ref('')
const status = ref(null)

const loadingRules = ref(false)

// 过滤后的规则列表
const filteredRules = computed(() => {
  const text = keyword.value.trim().toLowerCase()
  return rules.value.filter((item) => {
    if (activeType.value && item.rule_type !== activeType.value) return false
    if (!text) return true
    return (item.rule_item || '').toLowerCase().includes(text)
  })
})

const selectedCount = computed(() => selected.size)

const webhookHint = '在「企业微信群 → 群机器人 → 添加机器人」里复制 Webhook 地址（形如 https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=xxx）'

/** 加载告警配置 */
async function loadConfig() {
  loading.value = true
  try {
    const data = await alertApi.config()
    form.enabled = Boolean(data.enabled)
    form.webhook_url = data.webhook_url || ''
    form.interval_sec = data.interval_sec || 60
    form.mention_all = Boolean(data.mention_all)
    status.value = data

    selected.clear()
    for (const id of data.rule_ids || []) selected.add(id)
  } catch (error) {
    ElMessage.error(error.message || '读取告警配置失败')
  } finally {
    loading.value = false
  }
}

/** 加载规则清单（一次取全量，前端筛选） */
async function loadRules() {
  loadingRules.value = true
  try {
    const typeData = await ruleApi.types()
    types.value = (typeData || []).map((item) => ({ key: item.rule_type || item.key, name: item.name, count: item.count }))

    const data = await ruleApi.list({ type: '', keyword: '' })
    rules.value = data?.items ?? []
  } catch (error) {
    ElMessage.error(error.message || '读取规则列表失败')
  } finally {
    loadingRules.value = false
  }
}

/** 勾选/取消单条规则 */
function toggleRule(id) {
  if (selected.has(id)) selected.delete(id)
  else selected.add(id)
}

/** 全选/清空当前筛选结果 */
function selectVisible(all) {
  for (const item of filteredRules.value) {
    if (all) selected.add(item.id)
    else selected.delete(item.id)
  }
}

/** 保存配置 */
async function save() {
  if (form.enabled && !form.webhook_url.trim()) {
    return ElMessage.warning('启用告警前请先填写企业微信机器人 Webhook 地址')
  }
  if (form.enabled && selected.size === 0) {
    return ElMessage.warning('请至少勾选一条需要提醒的规则')
  }

  saving.value = true
  try {
    const data = await alertApi.save({
      enabled: form.enabled,
      webhook_url: form.webhook_url.trim(),
      interval_sec: Number(form.interval_sec) || 60,
      mention_all: form.mention_all,
      rule_ids: Array.from(selected)
    })
    status.value = data
    ElMessage.success(`已保存：共勾选 ${data.rule_count} 条规则`)
  } catch (error) {
    ElMessage.error(error.message || '保存失败')
  } finally {
    saving.value = false
  }
}

/** 发送测试消息 */
async function sendTest() {
  if (!form.webhook_url.trim()) return ElMessage.warning('请先填写 Webhook 地址')
  testing.value = true
  try {
    const result = await alertApi.test({ webhook_url: form.webhook_url.trim(), mention_all: form.mention_all })
    ElMessage.success(result?.message || '测试消息已发送')
  } catch (error) {
    ElMessage.error(error.message || '发送失败')
  } finally {
    testing.value = false
  }
}

onMounted(async () => {
  await Promise.all([loadConfig(), loadRules()])
})
</script>

<template>
  <div class="alert-panel" v-loading="loading">
    <div class="waf-card" style="padding: 18px 20px; margin-bottom: 16px">
      <div class="alert-head">
        <div>
          <h3>企业微信机器人提醒</h3>
          <p class="waf-muted">
            命中下方勾选的规则时，向企业微信群推送一条消息。静默期内的同一条规则只提醒一次。
          </p>
        </div>
        <el-switch v-model="form.enabled" active-text="启用" inactive-text="关闭" inline-prompt />
      </div>

      <el-form label-width="120px" style="margin-top: 8px">
        <el-form-item label="Webhook 地址">
          <el-input v-model="form.webhook_url" :placeholder="webhookHint" clearable />
        </el-form-item>
        <el-form-item label="静默期（秒）">
          <el-input-number v-model="form.interval_sec" :min="10" :max="3600" :step="10" />
          <span class="waf-muted" style="margin-left: 10px; font-size: 12px">
            同一规则在该时间内只提醒一次，避免刷屏
          </span>
        </el-form-item>
        <el-form-item label="@所有人">
          <el-switch v-model="form.mention_all" />
          <span class="waf-muted" style="margin-left: 10px; font-size: 12px">开启后消息末尾会附加 @all</span>
        </el-form-item>
        <el-form-item label="操作">
          <el-button :loading="testing" @click="sendTest">发送测试</el-button>
          <el-button type="primary" :loading="saving" @click="save">保存</el-button>
          <span v-if="status?.last_sent_at" class="waf-muted" style="margin-left: 12px; font-size: 12px">
            最近推送：{{ status.last_sent_at }}
          </span>
          <span v-if="status?.last_error" class="alert-error" style="margin-left: 12px">
            最近错误：{{ status.last_error }}
          </span>
        </el-form-item>
      </el-form>
    </div>

    <div class="waf-card" style="padding: 18px 20px">
      <div class="alert-head">
        <div>
          <h3>提醒的规则（已勾选 {{ selectedCount }} 条）</h3>
          <p class="waf-muted">只有命中这些规则的攻击才会推送；规则内容即日志里的「命中规则」标签。</p>
        </div>
        <div style="display: flex; gap: 8px; align-items: center">
          <el-input v-model="keyword" placeholder="搜索规则内容" clearable style="width: 220px">
            <template #prefix><el-icon><Search /></el-icon></template>
          </el-input>
          <el-select v-model="activeType" placeholder="全部类型" clearable style="width: 160px">
            <el-option v-for="item in types" :key="item.key" :label="`${item.name}（${item.count}）`" :value="item.key" />
          </el-select>
        </div>
      </div>

      <div style="display: flex; gap: 8px; margin: 10px 0 8px">
        <el-button text @click="selectVisible(true)">勾选当前筛选（{{ filteredRules.length }}）</el-button>
        <el-button text @click="selectVisible(false)">取消当前筛选</el-button>
      </div>

      <el-table :data="filteredRules" v-loading="loadingRules" height="420" stripe size="small">
        <el-table-column width="60" label="选择">
          <template #default="{ row }">
            <el-checkbox :model-value="selected.has(row.id)" @change="toggleRule(row.id)" />
          </template>
        </el-table-column>
        <el-table-column prop="id" label="ID" width="70" />
        <el-table-column prop="rule_type" label="类型" width="130" />
        <el-table-column label="规则内容" min-width="360">
          <template #default="{ row }">
            <span class="waf-mono" style="font-size: 12px">{{ row.rule_item }}</span>
          </template>
        </el-table-column>
        <template #empty>
          <p class="waf-muted" style="padding: 20px 0">没有匹配的规则</p>
        </template>
      </el-table>
    </div>
  </div>
</template>

<style scoped>
.alert-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 16px;
  flex-wrap: wrap;
}

.alert-head h3 {
  margin: 0 0 4px;
  font-size: 15px;
}

.alert-head p {
  margin: 0;
  font-size: 12.5px;
  line-height: 1.7;
}

.alert-error {
  color: #f56c6c;
  font-size: 12px;
}
</style>
