<script setup>
import { onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { authApi } from '../api/auth'
import { authStore } from '../stores/auth'
import { themeStore } from '../stores/theme'

const router = useRouter()
const route = useRoute()

const form = reactive({ username: '', password: '', captcha: '' })
const captcha = reactive({ id: '', image: '', loading: false })
const loading = ref(false)
const errorText = ref('')

/** 拉取验证码 */
async function loadCaptcha() {
  captcha.loading = true
  try {
    const data = await authApi.captcha()
    captcha.id = data.captcha_id
    captcha.image = data.image
    form.captcha = ''
  } catch (error) {
    errorText.value = error.message || '验证码获取失败'
  } finally {
    captcha.loading = false
  }
}

/** 提交登录 */
async function submit() {
  errorText.value = ''
  if (!form.username || !form.password) {
    errorText.value = '请输入用户名与口令'
    return
  }
  if (!form.captcha) {
    errorText.value = '请输入验证码'
    return
  }

  loading.value = true
  try {
    await authStore.login({
      username: form.username,
      password: form.password,
      captcha_id: captcha.id,
      captcha_code: form.captcha
    })
    ElMessage.success('登录成功')
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/dashboard'
    router.push(redirect)
  } catch (error) {
    errorText.value = error.message || '登录失败'
    loadCaptcha()
  } finally {
    loading.value = false
  }
}

onMounted(loadCaptcha)
</script>

<template>
  <div class="login-shell">
    <!-- 左侧品牌区 -->
    <section class="login-brand">
      <div class="orb orb-1"></div>
      <div class="orb orb-2"></div>
      <div class="orb orb-3"></div>

      <div class="login-brand-inner">
        <div class="login-logo">
          <div class="waf-brand-logo" style="width: 44px; height: 44px; font-size: 18px">X</div>
          <div>
            <div style="font-size: 18px; font-weight: 750; letter-spacing: 0.5px">X-WAF 管理台</div>
            <div style="font-size: 12px; color: rgba(226, 232, 240, 0.65)">云防火墙控制中心 · v2</div>
          </div>
        </div>

        <h1 class="login-title">
          让每一次请求<br />
          都经过<span class="grad">检测与研判</span>
        </h1>
        <p class="login-desc">
          统一管理站点反向代理、WAF 规则与攻击审计，配合 AI 研判与攻击地图大屏，快速定位风险来源。
        </p>

        <ul class="login-points">
          <li><el-icon><Lock /></el-icon> 注入 / XSS / 路径穿越 / CC 攻击实时拦截</li>
          <li><el-icon><Document /></el-icon> 全量攻击日志检索与多条件筛选</li>
          <li><el-icon><MagicStick /></el-icon> 大模型辅助研判攻击日志</li>
          <li><el-icon><Timer /></el-icon> 慢 SQL 统计与性能问题追溯</li>
        </ul>
      </div>
    </section>

    <!-- 右侧登录表单 -->
    <section class="login-panel">
      <button class="login-theme" :title="themeStore.dark.value ? '切换亮色' : '切换暗色'" @click="themeStore.toggle()">
        <el-icon><component :is="themeStore.dark.value ? 'Sunny' : 'Moon'" /></el-icon>
      </button>

      <div class="login-card">
        <h2>管理员登录</h2>
        <p class="waf-muted" style="margin: 6px 0 22px; font-size: 13px">请使用后台账号登录，默认账号为 admin</p>

        <el-form label-position="top" @submit.prevent="submit">
          <el-form-item label="用户名">
            <el-input v-model="form.username" size="large" placeholder="请输入用户名" autocomplete="username">
              <template #prefix><el-icon><User /></el-icon></template>
            </el-input>
          </el-form-item>

          <el-form-item label="口令">
            <el-input
              v-model="form.password"
              type="password"
              size="large"
              show-password
              placeholder="请输入口令"
              autocomplete="current-password"
              @keyup.enter="submit"
            >
              <template #prefix><el-icon><Lock /></el-icon></template>
            </el-input>
          </el-form-item>

          <el-form-item label="验证码">
            <div style="display: flex; gap: 10px; width: 100%">
              <el-input
                v-model="form.captcha"
                size="large"
                maxlength="6"
                placeholder="请输入右侧字符"
                @keyup.enter="submit"
              >
                <template #prefix><el-icon><Key /></el-icon></template>
              </el-input>
              <div class="login-captcha" title="点击刷新" @click="loadCaptcha">
                <img v-if="captcha.image" :src="captcha.image" alt="验证码" />
                <el-icon v-else class="is-loading"><Loading /></el-icon>
              </div>
            </div>
          </el-form-item>

          <el-alert v-if="errorText" :title="errorText" type="error" :closable="false" show-icon style="margin-bottom: 14px" />

          <el-button type="primary" size="large" style="width: 100%" :loading="loading" @click="submit">
            登录控制台
          </el-button>
        </el-form>

        <div class="login-foot">© {{ new Date().getFullYear() }} X-WAF · 仅限授权管理员访问</div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.login-shell {
  display: flex;
  min-height: 100vh;
  background: var(--waf-bg);
}

.login-brand {
  position: relative;
  flex: 1.15;
  padding: 56px 60px;
  overflow: hidden;
  background: linear-gradient(150deg, #101a2f 0%, #16224a 45%, #1b1d4d 100%);
  color: #e2e8f6;
  display: flex;
  align-items: center;
}

.orb {
  position: absolute;
  border-radius: 50%;
  filter: blur(60px);
  opacity: 0.55;
  animation: float 12s ease-in-out infinite;
}
.orb-1 {
  width: 320px;
  height: 320px;
  background: radial-gradient(circle, rgba(99, 102, 241, 0.85), transparent 70%);
  top: -60px;
  right: -40px;
}
.orb-2 {
  width: 260px;
  height: 260px;
  background: radial-gradient(circle, rgba(139, 92, 246, 0.75), transparent 70%);
  bottom: -80px;
  left: -40px;
  animation-delay: -4s;
}
.orb-3 {
  width: 180px;
  height: 180px;
  background: radial-gradient(circle, rgba(56, 189, 248, 0.6), transparent 70%);
  top: 55%;
  right: 22%;
  animation-delay: -8s;
}

@keyframes float {
  0%,
  100% {
    transform: translate3d(0, 0, 0);
  }
  50% {
    transform: translate3d(0, -22px, 0);
  }
}

.login-brand-inner {
  position: relative;
  z-index: 2;
  max-width: 520px;
}

.login-logo {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 42px;
}

.login-title {
  font-size: 34px;
  line-height: 1.35;
  font-weight: 780;
  letter-spacing: 0.5px;
  margin: 0 0 16px;
}

.grad {
  background: linear-gradient(120deg, #a5b4fc, #c4b5fd 60%, #7dd3fc);
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
}

.login-desc {
  color: rgba(203, 213, 225, 0.85);
  line-height: 1.8;
  font-size: 14px;
  margin: 0 0 30px;
}

.login-points {
  list-style: none;
  padding: 0;
  margin: 0;
  display: grid;
  gap: 12px;
}

.login-points li {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 13.5px;
  color: rgba(226, 232, 240, 0.9);
}

.login-points .el-icon {
  color: #a5b4fc;
  font-size: 16px;
}

.login-panel {
  flex: 1;
  display: grid;
  place-items: center;
  padding: 40px 24px;
  position: relative;
}

.login-theme {
  position: absolute;
  top: 22px;
  right: 26px;
  width: 38px;
  height: 38px;
  border-radius: 12px;
  border: 1px solid var(--waf-border);
  background: var(--waf-surface);
  color: var(--waf-text-muted);
  cursor: pointer;
  display: grid;
  place-items: center;
}

.login-card {
  width: 100%;
  max-width: 400px;
  background: var(--waf-surface);
  border: 1px solid var(--waf-border);
  border-radius: 18px;
  padding: 34px 32px 26px;
  box-shadow: 0 30px 60px -40px rgba(15, 23, 42, 0.55);
}

.login-card h2 {
  margin: 0;
  font-size: 22px;
  font-weight: 720;
}

.login-captcha {
  flex: 0 0 132px;
  height: 40px;
  border-radius: 10px;
  border: 1px dashed var(--waf-border);
  display: grid;
  place-items: center;
  overflow: hidden;
  cursor: pointer;
  background: var(--waf-surface-2);
}

.login-captcha img {
  width: 100%;
  height: 100%;
  object-fit: contain;
}

.login-foot {
  margin-top: 22px;
  text-align: center;
  font-size: 12px;
  color: var(--waf-text-dim);
}

@media (max-width: 980px) {
  .login-brand {
    display: none;
  }
}
</style>
