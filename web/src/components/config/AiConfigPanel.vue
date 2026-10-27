<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { aiApi } from '../../api/ai'

const loading = ref(true)
const saving = ref(false)
const testing = ref(false)
const testResult = ref(null)
const status = ref(null)

const form = reactive({
  enabled: false,
  base_url: '',
  api_key: '',
  model: '',
  temperature: 0.2,
  max_tokens: 4096,
  timeout_seconds: 60,
  system_prompt: '',
  cache_minutes: 10
})

const configured = computed(() => status.value?.configured ?? false)

/** 加载配置 */
async function load() {
  loading.value = true
  try {
    const data = await aiApi.status()
    status.value = data
    Object.assign(form, {
      enabled: data.config?.enabled ?? false,
      base_url: data.config?.base_url ?? '',
      api_key: data.config?.api_key ?? '',
      model: data.config?.model ?? '',
      temperature: data.config?.temperature ?? 0.2,
      max_tokens: data.config?.max_tokens ?? 4096,
      timeout_seconds: data.config?.timeout_seconds ?? 60,
      system_prompt: data.config?.system_prompt ?? '',
      cache_minutes: data.config?.cache_minutes ?? 10
    })
  } catch (error) {
    ElMessage.error(error.message || '加载 AI 配置失败')
  } finally {
    loading.value = false
  }
}

/** 保存配置 */
async function save() {
  saving.value = true
  try {
    const data = await aiApi.save(form)
    ElMessage.success(data?.message || '配置已保存')
    await load()
  } catch (error) {
    ElMessage.error(error.message || '保存失败')
  } finally {
    saving.value = false
  }
}

/** 连通性测试 */
async function test() {
  testing.value = true
  testResult.value = null
  try {
    testResult.value = await aiApi.test()
    ElMessage.success('接口连通正常')
  } catch (error) {
    ElMessage.error(error.message || '测试失败')
  } finally {
    testing.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="config-panel">
    <div class="panel-toolbar">
      <p class="panel-hint">配置 OpenAI 兼容接口的大模型，用于攻击日志智能研判</p>
      <div class="panel-actions">
        <el-button :loading="testing" @click="test"><el-icon><Connection /></el-icon> 连通性测试</el-button>
        <el-button type="primary" :loading="saving" @click="save"><el-icon><Check /></el-icon> 保存配置</el-button>
      </div>
    </div>

    <el-alert
      v-if="!loading && !configured"
      title="AI 尚未启用"
      type="warning"
      :closable="false"
      show-icon
      description="填写接口地址与模型名称并打开开关后，攻击日志页即可使用「AI 分析」功能。"
      style="margin-bottom: 16px"
    />

    <div class="waf-grid waf-grid--2">
      <div class="waf-card" v-loading="loading">
        <div class="waf-card-head">
          <div class="waf-card-title"><span class="bar"></span> 模型接入</div>
          <el-switch v-model="form.enabled" active-text="启用" inactive-text="停用" />
        </div>

        <el-form label-position="top">
          <el-form-item label="接口地址（OpenAI 兼容）">
            <el-input v-model="form.base_url" placeholder="例如 https://api.openai.com/v1" />
            <div class="waf-muted" style="font-size: 12px; margin-top: 6px">
              实际请求地址为 <code class="waf-mono">{{ form.base_url || 'https://api.openai.com/v1' }}/chat/completions</code>
            </div>
          </el-form-item>

          <el-form-item label="API Key">
            <el-input v-model="form.api_key" type="password" show-password placeholder="sk-..." />
            <div class="waf-muted" style="font-size: 12px; margin-top: 6px">保存后会脱敏展示，留空表示不修改已保存的密钥</div>
          </el-form-item>

          <el-form-item label="模型名称">
            <el-input v-model="form.model" placeholder="例如 gpt-4o-mini / deepseek-chat" />
          </el-form-item>

          <div class="waf-inline-grid">
            <el-form-item label="Temperature">
              <el-slider v-model="form.temperature" :min="0" :max="1" :step="0.1" show-input />
            </el-form-item>
            <el-form-item label="最大输出 tokens">
              <el-input-number v-model="form.max_tokens" :min="-1" :max="32768" :step="256" style="width: 100%" />
            <div class="waf-muted" style="font-size: 11.5px; margin-top: 4px; line-height: 1.6">
              输出上限越大越不容易被截断（默认 4096，最大 32768）；填 <code>-1</code> 表示不限制，由模型自行决定。
              即使被截断，后台也会自动续写最多 2 轮。
            </div>
            </el-form-item>
          </div>

          <div class="waf-inline-grid">
            <el-form-item label="请求超时（秒）">
              <el-input-number v-model="form.timeout_seconds" :min="10" :max="300" :step="10" style="width: 100%" />
            </el-form-item>
            <el-form-item label="结果缓存（分钟）">
              <el-input-number v-model="form.cache_minutes" :min="1" :max="120" :step="1" style="width: 100%" />
            </el-form-item>
          </div>
          <div class="waf-muted" style="font-size: 12px; margin-bottom: 10px; line-height: 1.7">
            相同日志 + 相同提问在缓存期内直接复用上次研判结果，不再消耗 token；保存配置会立即清空旧缓存。
          </div>
        </el-form>
      </div>

      <div class="waf-card">
        <div class="waf-card-head">
          <div class="waf-card-title"><span class="bar"></span> 研判提示词</div>
        </div>
        <el-form label-position="top">
          <el-form-item label="System Prompt">
            <el-input v-model="form.system_prompt" type="textarea" :rows="12" placeholder="留空则使用内置安全分析师提示词" />
          </el-form-item>
        </el-form>

        <div v-if="testResult" class="test-result">
          <div class="waf-card-head" style="margin-bottom: 8px">
            <div class="waf-card-title"><span class="bar"></span> 测试返回</div>
            <div class="waf-card-extra">
              {{ testResult.model || '-' }} · {{ testResult.elapsed_ms }}ms · tokens {{ testResult.usage?.total_tokens ?? '-' }}
            </div>
          </div>
          <div class="waf-code">{{ testResult.content }}</div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.test-result {
  margin-top: 16px;
  border-top: 1px dashed var(--waf-border);
  padding-top: 14px;
}
</style>
