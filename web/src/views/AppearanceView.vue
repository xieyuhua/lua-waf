<script setup>
import ThemePanel from '../components/ThemePanel.vue'
</script>

<template>
  <div class="waf-page">
    <div class="waf-page-head">
      <div>
        <h1 class="waf-page-title"><span class="dot"></span> 外观设置</h1>
        <p class="waf-page-sub">选择界面模式与主题色，设置保存在浏览器本地，仅影响当前后台界面</p>
      </div>
    </div>

    <div class="waf-grid waf-grid--2">
      <div class="waf-card">
        <div class="waf-card-head">
          <div class="waf-card-title"><span class="bar"></span> 主题切换</div>
        </div>
        <ThemePanel />
      </div>

      <div class="waf-card">
        <div class="waf-card-head">
          <div class="waf-card-title"><span class="bar"></span> 说明</div>
        </div>
        <ul class="tips">
          <li><strong>外观模式</strong>：「跟随系统」会自动读取操作系统的深色偏好并实时切换。</li>
          <li><strong>主题色</strong>：同时作用于侧边栏选中态、按钮、链接、图表主色与品牌渐变。</li>
          <li><strong>存储位置</strong>：浏览器 <code>localStorage</code> 的 <code>wafadmin.theme</code>，
            换浏览器或清理缓存后回到默认（跟随系统 + 靛蓝）。</li>
          <li><strong>快捷入口</strong>：顶栏调色板图标可随时打开面板，旁边的太阳/月亮按钮用于一键切换亮暗。</li>
        </ul>
      </div>
    </div>
  </div>
</template>

<style scoped>
.tips {
  margin: 0;
  padding-left: 18px;
  line-height: 2;
  color: var(--waf-text-muted);
  font-size: 13px;
}

.tips code {
  background: var(--waf-surface-2);
  border: 1px solid var(--waf-border);
  border-radius: 6px;
  padding: 1px 6px;
  font-size: 12px;
}
</style>
