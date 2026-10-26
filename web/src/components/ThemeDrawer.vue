<script setup>
import { computed } from 'vue'
import ThemePanel from './ThemePanel.vue'

const props = defineProps({ modelValue: { type: Boolean, default: false } })
const emit = defineEmits(['update:modelValue'])

const visible = computed({
  get: () => props.modelValue,
  set: (value) => emit('update:modelValue', value)
})
</script>

<template>
  <el-drawer v-model="visible" title="主题与外观" size="380px" append-to-body>
    <ThemePanel />
    <p class="waf-muted" style="font-size: 12px; line-height: 1.7; margin-top: 8px">
      设置会保存在浏览器本地，仅影响当前后台界面，不影响 WAF 防护行为。
    </p>
  </el-drawer>
</template>
