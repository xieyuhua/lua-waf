<script setup>
import { computed, nextTick, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { aiApi } from '../api/ai'
import { aiResultStore } from '../stores/ai'
import { renderMarkdown } from '../utils/markdown'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  logs: { type: Array, default: () => [] },
  /** 打开时直接展示的结果（来自本地缓存） */
  initial: { type: Object, default: null }
})
const emit = defineEmits(['update:modelValue'])

const loading = ref(false)
const question = ref('')
const result = ref(null)
const fromCache = ref(false)
const resultRef = ref(null)

const visible = computed({
  get: () => props.modelValue,
  set: (value) => emit('update:modelValue', value)
})

const logCount = computed(() => props.logs.length)
const logPreview = computed(() => props.logs.slice(0, 6))
const cacheMinutes = computed(() => {
  const at = result.value?.cached_at || result.value?.at
  return at || ''
})

/** 触发 AI 分析；refresh 为 true 时跳过缓存 */
async function run(refresh = false) {
  if (!props.logs.length) {
    ElMessage.warning('请先选择要分析的日志')
    return
  }

  const ids = props.logs.map((item) => item.id)

  // 先查本地缓存（10 分钟内同参数直接复用，不发请求）
  if (!refresh) {
    const local = aiResultStore.get(ids, question.value.trim())
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
    const data = await aiApi.analyze({
      log_ids: ids,
      question: question.value.trim(),
      refresh
    })
    result.value = data
    fromCache.value = Boolean(data.cached)
    aiResultStore.save(ids, question.value.trim(), data)
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
      const local = props.initial || aiResultStore.get(props.logs.map((item) => item.id), '')
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
  <el-dialog v-model="visible" title="AI 日志分析" width="760px" top="6vh" destroy-on-close>
    <div class="waf-card" style="background: var(--waf-surface-2); box-shadow: none">
      <div style="display: flex; align-items: center; gap: 8px; margin-bottom: 10px">
        <el-icon color="var(--waf-primary)"><MagicStick /></el-icon>
        <strong>已选择 {{ logCount }} 条日志</strong>
        <span class="waf-muted" style="font-size: 12px">（单次最多 20 条）</span>
      </div>
      <div style="display: flex; flex-wrap: wrap; gap: 6px">
        <el-tag v-for="item in logPreview" :key="item.id" size="small" type="info">
          {{ item.client_ip }} · {{ item.attack_type }}
        </el-tag>
        <el-tag v-if="logCount > logPreview.length" size="small" type="info">... 其余 {{ logCount - logPreview.length }} 条</el-tag>
      </div>
    </div>

    <el-input
      v-model="question"
      type="textarea"
      :rows="2"
      placeholder="可选：补充你的问题，例如「这些请求是否属于同一攻击源？需要封禁哪些 IP？」"
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
      <span class="waf-muted" style="font-size: 12px">分析需先在「AI 设置」中配置模型</span>
    </div>

    <div v-if="result" ref="resultRef" style="margin-top: 18px">
      <div class="waf-card-head">
        <div class="waf-card-title"><span class="bar"></span> 研判结果</div>
        <div class="waf-card-extra">
          <el-tag v-if="fromCache" size="small" type="success" effect="plain" style="margin-right: 8px">
            缓存结果{{ cacheMinutes ? ' · ' + cacheMinutes : '' }}
          </el-tag>
          模型 {{ result.model || '-' }} · 耗时 {{ result.elapsed_ms }}ms · tokens {{ result.usage?.total_tokens ?? '-' }}
        </div>
      </div>
      <div class="waf-markdown" v-html="renderMarkdown(result.content)"></div>
    </div>

    <template #footer>
      <el-button @click="visible = false">关闭</el-button>
    </template>
  </el-dialog>
</template>
