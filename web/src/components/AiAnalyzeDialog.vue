<script setup>
import { computed, nextTick, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { aiApi } from '../api/ai'
import { AI_SCENE_LOG, AI_SCENE_SLOW_SQL, aiResultStore } from '../stores/ai'
import { renderMarkdown } from '../utils/markdown'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  /** 待分析记录：攻击日志或慢 SQL 记录 */
  logs: { type: Array, default: () => [] },
  /** 分析场景：log（攻击日志）/ slow_sql（慢 SQL 优化建议） */
  scene: { type: String, default: 'log' },
  /** 打开时直接展示的结果（来自本地缓存） */
  initial: { type: Object, default: null }
})
const emit = defineEmits(['update:modelValue'])

const loading = ref(false)
const question = ref('')
const result = ref(null)
// 是否存在可展示的正文（旧缓存可能是空内容）
const hasContent = computed(() => Boolean(String(result.value?.content || '').trim()))
const fromCache = ref(false)
const resultRef = ref(null)

const visible = computed({
  get: () => props.modelValue,
  set: (value) => emit('update:modelValue', value)
})

const isSlowSQL = computed(() => props.scene === AI_SCENE_SLOW_SQL)
const sceneKey = computed(() => (isSlowSQL.value ? AI_SCENE_SLOW_SQL : AI_SCENE_LOG))
const dialogTitle = computed(() => (isSlowSQL.value ? 'AI 慢 SQL 优化建议' : 'AI 日志分析'))
const countLabel = computed(() => (isSlowSQL.value ? '慢 SQL' : '日志'))
const questionPlaceholder = computed(() =>
  isSlowSQL.value
    ? '可选：补充你的问题，例如「这张表能不能加联合索引？有没有不改 SQL 的方案？」'
    : '可选：补充你的问题，例如「这些请求是否属于同一攻击源？需要封禁哪些 IP？」'
)
const resultTitle = computed(() => (isSlowSQL.value ? '优化建议' : '研判结果'))

const logCount = computed(() => props.logs.length)
const logPreview = computed(() => props.logs.slice(0, 6))

/** 预览标签文案：日志显示 IP + 类型，慢 SQL 显示耗时 + 操作 */
function previewLabel(item) {
  if (isSlowSQL.value) {
    return `${item.duration_ms ?? 0} ms · ${item.operation || 'sql'}`
  }
  return `${item.client_ip || '-'} · ${item.attack_type || '-'}`
}
const cacheMinutes = computed(() => {
  const at = result.value?.cached_at || result.value?.at
  return at || ''
})

/** 触发 AI 分析；refresh 为 true 时跳过缓存 */
async function run(refresh = false) {
  if (!props.logs.length) {
    ElMessage.warning(isSlowSQL.value ? '请先选择要分析的慢 SQL' : '请先选择要分析的日志')
    return
  }

  const ids = props.logs.map((item) => item.id)

  // 先查本地缓存（10 分钟内同参数直接复用，不发请求）
  if (!refresh) {
    const local = aiResultStore.get(sceneKey.value, ids, question.value.trim())
    if (local) {
      result.value = local.result
      fromCache.value = true
      return
    }
  }

  loading.value = true
  result.value = null
  fromCache.value = false
  try {
    const payload = { question: question.value.trim(), refresh }
    const data = isSlowSQL.value
      ? await aiApi.analyzeSlowSQL({ ...payload, slow_sql_ids: ids })
      : await aiApi.analyze({ ...payload, log_ids: ids })
    result.value = data
    fromCache.value = Boolean(data.cached)
    aiResultStore.save(sceneKey.value, ids, question.value.trim(), data)
  } catch (error) {
    ElMessage.error(error.message || 'AI 分析失败')
  } finally {
    loading.value = false
  }
}

// 结果渲染后滚到可视区（窄屏弹窗内滚动），避免"看不到结果"
watch(result, async (value) => {
  if (!value) return
  await nextTick()
  resultRef.value?.scrollIntoView({ block: 'start', behavior: 'smooth' })
})

/** 打开弹窗时优先展示已有缓存结果 */
watch(
  visible,
  (value) => {
    if (value) {
      const local = props.initial || aiResultStore.get(sceneKey.value, props.logs.map((item) => item.id), '')
      if (local && !result.value) {
        result.value = local.result
        fromCache.value = true
        question.value = local.question || ''
      }
      return
    }
    result.value = null
    question.value = ''
    loading.value = false
    fromCache.value = false
  },
  { immediate: true }
)
</script>

<template>
  <el-dialog v-model="visible" :title="dialogTitle" width="760px" top="6vh" destroy-on-close>
    <div class="waf-card" style="background: var(--waf-surface-2); box-shadow: none">
      <div style="display: flex; align-items: center; gap: 8px; margin-bottom: 10px">
        <el-icon color="var(--waf-primary)"><MagicStick /></el-icon>
        <strong>已选择 {{ logCount }} 条{{ countLabel }}</strong>
        <span class="waf-muted" style="font-size: 12px">（单次最多 20 条）</span>
      </div>
      <div style="display: flex; flex-wrap: wrap; gap: 6px">
        <el-tag v-for="item in logPreview" :key="item.id" size="small" type="info">
          {{ previewLabel(item) }}
        </el-tag>
        <el-tag v-if="logCount > logPreview.length" size="small" type="info">... 其余 {{ logCount - logPreview.length }} 条</el-tag>
      </div>
    </div>

    <el-input
      v-model="question"
      type="textarea"
      :rows="2"
      :placeholder="questionPlaceholder"
      style="margin-top: 14px"
    />

    <div style="margin-top: 14px; display: flex; gap: 10px; align-items: center; flex-wrap: wrap">
      <el-button type="primary" :loading="loading" @click="run(false)">
        <el-icon><MagicStick /></el-icon>
        {{ result ? '重新分析（用缓存）' : '开始分析' }}
      </el-button>
      <el-button v-if="result" :loading="loading" @click="run(true)">
        <el-icon><RefreshRight /></el-icon>
        忽略缓存重新请求
      </el-button>
      <span class="waf-muted" style="font-size: 12px">分析需先在「系统设置 → AI 设置」中配置模型</span>
    </div>

    <div v-if="result" ref="resultRef" style="margin-top: 18px">
      <div class="waf-card-head">
        <div class="waf-card-title"><span class="bar"></span> {{ resultTitle }}</div>
        <div class="waf-card-extra">
          <el-tag v-if="fromCache" size="small" type="success" effect="plain" style="margin-right: 8px">
            缓存结果{{ cacheMinutes ? ' · ' + cacheMinutes : '' }}
          </el-tag>
          模型 {{ result.model || '-' }} · 耗时 {{ result.elapsed_ms }}ms · tokens {{ result.usage?.total_tokens ?? '-' }}
        </div>
      </div>
      <div v-if="hasContent" class="waf-markdown" v-html="renderMarkdown(result.content)"></div>
      <el-alert
        v-else
        type="warning"
        show-icon
        :closable="false"
        title="这条记录没有正文内容"
        description="可能来自旧版本的缓存（模型只返回了思考过程或空内容）。请点「忽略缓存重新请求」重新分析，或在「AI 设置」中更换模型、调大「最大输出 tokens」。"
        style="margin-top: 10px"
      />
    </div>

    <template #footer>
      <el-button @click="visible = false">关闭</el-button>
    </template>
  </el-dialog>
</template>
