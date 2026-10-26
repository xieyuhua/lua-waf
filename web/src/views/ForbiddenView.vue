<script setup>
import { computed } from 'vue'
import { useRoute } from 'vue-router'

const route = useRoute()
const code = computed(() => route.meta?.code ?? 404)
const message = computed(() => (code.value === 404 ? '页面不存在' : '无访问权限'))
</script>

<template>
  <div class="waf-page" style="display: grid; place-items: center; min-height: 72vh">
    <div class="waf-card" style="text-align: center; max-width: 460px; padding: 46px 32px">
      <div class="code">{{ code }}</div>
      <p style="font-size: 15px; margin: 6px 0 22px">{{ message }}</p>
      <p class="waf-muted" style="font-size: 13px; margin-bottom: 24px">
        请检查地址是否正确，或返回控制台继续操作。
      </p>
      <router-link to="/dashboard">
        <el-button type="primary">返回控制台</el-button>
      </router-link>
    </div>
  </div>
</template>

<style scoped>
.code {
  font-size: 64px;
  font-weight: 800;
  letter-spacing: 4px;
  background: var(--waf-gradient);
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
  line-height: 1;
}
</style>
