<script setup>
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { aiApi } from '../api/ai'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  logs: { type: Array, default: () => [] }
})
const emit = defineEmits(['update:modelValue'])

const loading = ref(false)
const question = ref('')
const result = ref(null)

const visible = computed({
  get: () => props.modelValue,
  set: (value) => emit('update:modelValue', value)
})

const logCount = computed(() => props.logs.length)
const logPreview = computed(() => props.logs.slice(0, 6))

/** 触发 AI 分析 */
async function run() {
  if (!props.logs.length) {
    ElMessage.warning('请先选择要分析的日志')
    return
  }
  loading.value = true
  result.value = null
  try {
    result.value = await aiApi.analyze({
      log_ids: props.logs.map((item) => item.id),
      question: question.value.trim()
    })
  } catch (error) {
    ElMessage.error(error.message || 'AI 分析失败')
  } finally {
    loading.value = false
  }
}

// 关闭时重置状态
watch(visible, (value) => {
  if (!value) {
    result.value = null
    question.value = ''
    loading.value = false
  }
})
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

    <div style="margin-top: 14px; display: flex; gap: 10px; align-items: center">
      <el-button type="primary" :loading="loading" @click="run">
        <el-icon><MagicStick /></el-icon>
        开始分析
      </el-button>
      <span class="waf-muted" style="font-size: 12px">分析需要先在「AI 设置」中配置模型</span>
    </div>

    <div v-if="result" style="margin-top: 18px">
      <div class="waf-card-head">
        <div class="waf-card-title"><span class="bar"></span> 研判结果</div>
        <div class="waf-card-extra">
          模型 {{ result.model || '-' }} · 耗时 {{ result.elapsed_ms }}ms · tokens {{ result.usage?.total_tokens ?? '-' }}
        </div>
      </div>
      <div class="waf-markdown">{{ result.content }}</div>
    </div>

    <template #footer>
      <el-button @click="visible = false">关闭</el-button>
    </template>
  </el-dialog>
</template>
