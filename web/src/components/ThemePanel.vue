<script setup>
import { themeStore } from '../stores/theme'
</script>

<template>
  <div class="theme-panel">
    <div class="theme-block">
      <div class="theme-label">外观模式</div>
      <div class="mode-grid">
        <button
          v-for="mode in themeStore.modeOptions"
          :key="mode.value"
          type="button"
          class="mode-card"
          :class="{ 'is-active': themeStore.state.mode === mode.value }"
          @click="themeStore.setMode(mode.value)"
        >
          <el-icon class="mode-icon"><component :is="mode.icon" /></el-icon>
          <span>{{ mode.label }}</span>
        </button>
      </div>
    </div>

    <div class="theme-block">
      <div class="theme-label">主题色</div>
      <div class="accent-grid">
        <button
          v-for="preset in themeStore.presets"
          :key="preset.id"
          type="button"
          class="accent-card"
          :class="{ 'is-active': themeStore.state.accent === preset.id }"
          @click="themeStore.setAccent(preset.id)"
        >
          <span
            class="accent-dot"
            :style="{ background: `linear-gradient(135deg, ${preset.primary}, ${preset.accent})` }"
          >
            <el-icon v-if="themeStore.state.accent === preset.id"><Check /></el-icon>
          </span>
          <span class="accent-name">{{ preset.name }}</span>
        </button>
      </div>
    </div>

    <div class="theme-current">
      <span>当前：{{ themeStore.modeLabel.value }} · {{ themeStore.currentPreset.value.name }}</span>
      <el-button text type="primary" size="small" @click="themeStore.reset()">恢复默认</el-button>
    </div>
  </div>
</template>

<style scoped>
.theme-block {
  margin-bottom: 20px;
}

.theme-label {
  font-size: 12.5px;
  color: var(--waf-text-muted);
  margin-bottom: 10px;
}

.mode-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
}

.mode-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  padding: 12px 6px;
  border-radius: 12px;
  border: 1px solid var(--waf-border);
  background: var(--waf-surface-2);
  color: var(--waf-text-muted);
  font-size: 12.5px;
  cursor: pointer;
  transition: all 0.18s ease;
}

.mode-card:hover {
  border-color: color-mix(in srgb, var(--waf-primary) 45%, transparent);
  color: var(--waf-text);
}

.mode-card.is-active {
  border-color: var(--waf-primary);
  color: var(--waf-primary);
  background: color-mix(in srgb, var(--waf-primary) 10%, transparent);
  font-weight: 600;
}

.mode-icon {
  font-size: 18px;
}

.accent-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;
}

.accent-card {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  border-radius: 10px;
  border: 1px solid var(--waf-border);
  background: var(--waf-surface-2);
  color: var(--waf-text-muted);
  font-size: 12.5px;
  cursor: pointer;
  transition: all 0.18s ease;
}

.accent-card:hover {
  color: var(--waf-text);
}

.accent-card.is-active {
  border-color: var(--waf-primary);
  color: var(--waf-text);
  font-weight: 600;
}

.accent-dot {
  width: 20px;
  height: 20px;
  border-radius: 50%;
  display: grid;
  place-items: center;
  color: #fff;
  font-size: 11px;
  flex: 0 0 auto;
  box-shadow: 0 3px 8px -4px rgba(15, 23, 42, 0.6);
}

.theme-current {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding-top: 12px;
  border-top: 1px solid var(--waf-border);
  font-size: 12.5px;
  color: var(--waf-text-muted);
}
</style>
