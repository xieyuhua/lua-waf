<script setup>
import LuaConfigPanel from '../components/config/LuaConfigPanel.vue'
</script>

<template>
  <div class="waf-page">
    <div class="waf-page-head">
      <div>
        <h1 class="waf-page-title"><span class="dot"></span> WAF 配置</h1>
        <p class="waf-page-sub">
          在线修改 openresty 侧 config.lua 的检测模块开关与参数，保存后可一键 reload 生效；按模块选项卡切换
        </p>
      </div>
    </div>

    <LuaConfigPanel />
  </div>
</template>
