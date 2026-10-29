<script setup>
/**
 * 规则自检对话框
 *
 * 两个用途：
 *   1. 全量自检：把每条启用规则跑一遍内置攻击样例，列出「死规则」（连按规则自己生成的样例
 *      都命中不了，永远不会生效）与正则写错的规则；
 *   2. 载荷验证：贴一条参数或 URL，看它会命中哪些规则（按 lua 侧检查顺序），
 *      用于上线前确认「这条会不会被拦」，或排查「为什么没拦住」。
 */
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { ruleApi } from '../api/rule'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  // 预填的载荷（从规则页进来时通常为空）
  initialPayload: { type: String, default: '' }
})

const emit = defineEmits(['update:modelValue'])

const mode = ref('scan')
const payload = ref(props.initialPayload)
const onlyType = ref('')
const includeDisabled = ref(false)
const loading = ref(false)
const report = ref(null)

const visible = computed({
  get: () => props.modelValue,
  set: (value) => emit('update:modelValue', value)
})

const TYPES = [
  { value: '', label: '全部类型' },
  { value: 'args', label: 'URL 参数' },
  { value: 'post', label: 'POST 参数' },
  { value: 'url', label: 'URL 规则' },
  { value: 'cookie', label: 'Cookie' },
  { value: 'header', label: '请求头' },
  { value: 'useragent', label: 'User-Agent' },
  { value: 'blackip', label: 'IP 黑名单' },
  { value: 'whiteip', label: 'IP 白名单' },
  { value: 'whiteUrl', label: 'URL 白名单' }
]

// 需要人工处理的规则（死规则 / 正则错误 / RE2 不支持）
const problems = computed(() => {
  const items = report.value?.results ?? []
  return items.filter((item) => ['dead', 'no-match', 'bad-regex', 'go-incompat'].includes(item.status))
})

// 载荷模式下命中的规则
const hits = computed(() => {
  const items = report.value?.results ?? []
  return items.filter((item) => item.status === 'hit')
})

const STATUS_META = {
  dead: { text: '死规则', type: 'danger' },
  'no-match': { text: '未命中', type: 'warning' },
  'bad-regex': { text: '正则错误', type: 'danger' },
  'go-incompat': { text: 'RE2 不支持', type: 'info' },
  narrow: { text: '有效·样例未覆盖', type: 'success' },
  hit: { text: '命中', type: 'success' },
  skipped: { text: '跳过', type: 'info' }
}

/** 执行自检 */
async function run() {
  if (mode.value === 'payload' && !payload.value.trim()) {
    return ElMessage.warning('请先填写要验证的参数或 URL')
  }

  loading.value = true
  try {
    report.value = await ruleApi.selfTest({
      payload: mode.value === 'payload' ? payload.value.trim() : '',
      type: onlyType.value,
      all: includeDisabled.value
    })
  } catch (error) {
    ElMessage.error(error.message || '自检失败')
  } finally {
    loading.value = false
  }
}

// 打开时自动跑一次全量自检
watch(visible, (open) => {
  if (open && !report.value) run()
})
</script>

<template>
  <el-dialog v-model="visible" title="规则自检" width="920px" top="6vh">
    <div class="selftest-head">
      <el-radio-group v-model="mode">
        <el-radio-button label="scan">全量自检</el-radio-button>
        <el-radio-button label="payload">载荷验证</el-radio-button>
      </el-radio-group>

      <el-select v-model="onlyType" style="width: 150px">
        <el-option v-for="item in TYPES" :key="item.value" :label="item.label" :value="item.value" />
      </el-select>

      <el-checkbox v-model="includeDisabled">包含停用规则</el-checkbox>

      <el-button type="primary" :loading="loading" @click="run">开始检测</el-button>
    </div>

    <el-input
      v-if="mode === 'payload'"
      v-model="payload"
      type="textarea"
      :rows="3"
      placeholder="粘贴要验证的请求参数或 URL，例如：id=1 union select 1&path=../../etc/passwd"
      style="margin-bottom: 12px"
    />

    <template v-if="report">
      <!-- 全量自检结果 -->
      <template v-if="mode === 'scan'">
        <div class="selftest-stats">
          <span>共 <b>{{ report.total }}</b> 条</span>
          <span class="ok">命中攻击样例 <b>{{ report.hit }}</b></span>
          <span :class="{ bad: report.dead > 0 }">死规则 <b>{{ report.dead }}</b></span>
          <span>未覆盖 <b>{{ report.narrow }}</b></span>
          <span :class="{ bad: report.bad_regex > 0 }">正则错误 <b>{{ report.bad_regex }}</b></span>
          <span :class="{ bad: report.go_incompat > 0 }">RE2 不支持 <b>{{ report.go_incompat }}</b></span>
          <span>跳过 <b>{{ report.skipped }}</b></span>
        </div>

        <el-alert
          v-if="problems.length === 0"
          type="success"
          :closable="false"
          show-icon
          title="没有发现死规则：所有启用的规则都能命中"
          description="「死规则」= 连按规则自己生成的样例都命中不了，永远不会生效；「未覆盖」只是内置攻击样例没覆盖它的形态，属正常。"
          style="margin-bottom: 12px"
        />
        <el-alert
          v-else
          type="warning"
          :closable="false"
          show-icon
          :title="`发现 ${problems.length} 条需要人工确认的规则`"
          style="margin-bottom: 12px"
        />

        <el-table :data="problems" height="320" size="small" stripe>
          <el-table-column prop="rule_id" label="ID" width="70" />
          <el-table-column prop="rule_type" label="类型" width="110" />
          <el-table-column label="状态" width="120">
            <template #default="{ row }">
              <el-tag :type="STATUS_META[row.status]?.type || 'info'" size="small">
                {{ STATUS_META[row.status]?.text || row.status }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="规则内容" min-width="300">
            <template #default="{ row }">
              <span class="waf-mono" style="font-size: 12px">{{ row.rule_item }}</span>
            </template>
          </el-table-column>
          <el-table-column label="说明" min-width="180">
            <template #default="{ row }">
              <span style="font-size: 12px">{{ row.error || '-' }}</span>
            </template>
          </el-table-column>
          <template #empty>
            <p class="waf-muted" style="padding: 16px 0">没有需要处理的规则</p>
          </template>
        </el-table>
      </template>

      <!-- 载荷验证结果 -->
      <template v-else>
        <el-alert
          v-if="hits.length === 0"
          type="warning"
          :closable="false"
          show-icon
          title="没有任何规则命中这条载荷"
          description="说明当前规则拦不住它，建议补一条规则（或确认该类型规则是否已启用）。"
          style="margin-bottom: 12px"
        />
        <el-alert
          v-else
          type="success"
          :closable="false"
          show-icon
          :title="`命中 ${hits.length} 条规则（按 lua 检查顺序排列）`"
          style="margin-bottom: 12px"
        />

        <el-table :data="hits" height="320" size="small" stripe>
          <el-table-column prop="rule_id" label="ID" width="70" />
          <el-table-column prop="rule_type" label="类型" width="110" />
          <el-table-column label="规则内容" min-width="320">
            <template #default="{ row }">
              <span class="waf-mono" style="font-size: 12px">{{ row.rule_item }}</span>
            </template>
          </el-table-column>
          <el-table-column label="命中片段" min-width="200">
            <template #default="{ row }">
              <span class="waf-mono" style="font-size: 12px; color: #e6a23c">{{ row.sample }}</span>
            </template>
          </el-table-column>
          <template #empty>
            <p class="waf-muted" style="padding: 16px 0">无命中</p>
          </template>
        </el-table>
      </template>
    </template>

    <template #footer>
      <el-button @click="visible = false">关闭</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.selftest-head {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 12px;
}

.selftest-stats {
  display: flex;
  gap: 18px;
  flex-wrap: wrap;
  font-size: 13px;
  padding: 10px 14px;
  margin-bottom: 12px;
  border-radius: 10px;
  background: var(--waf-surface-alt, rgba(148, 163, 184, 0.1));
}

.selftest-stats .ok {
  color: #67c23a;
}

.selftest-stats .bad {
  color: #f56c6c;
  font-weight: 600;
}
</style>
