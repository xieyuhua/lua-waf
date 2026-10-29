/**
 * 攻击地图的图表配置构建（纯函数，便于复用与单测）
 *
 * 从 AttackMapView.vue 抽出来：视图只负责数据与交互，option 的细节留在这里。
 * 所有入参都是普通值（不是 ref），返回值是可直接交给 ECharts 的对象。
 */

/** 随主题变化的配色 */
export function buildColors(dark) {
  return {
    area: dark ? 'rgba(23,35,58,0.92)' : '#e9eefb',
    areaHover: dark ? 'rgba(99,102,241,0.42)' : 'rgba(99,102,241,0.24)',
    border: dark ? 'rgba(99,102,241,0.32)' : 'rgba(99,102,241,0.28)',
    label: dark ? '#cbd5e1' : '#475569',
    axis: dark ? 'rgba(148,163,184,0.35)' : 'rgba(100,116,139,0.3)',
    tooltipBg: dark ? 'rgba(9,15,28,0.94)' : 'rgba(255,255,255,0.96)',
    tooltipText: dark ? '#e2e8f0' : '#1f2937'
  }
}

/**
 * 世界地图 + 攻击箭头（来源 → 防护目标）
 *
 * @param {object} options
 * @param {Array} options.points 攻击来源点（含 lon/lat/count/area/attack_types）
 * @param {Array} options.arrowPoints 参与画箭头的前 N 个点
 * @param {object|null} options.target 防护目标（name/lon/lat/area）
 * @param {object} options.colors 主题配色
 * @param {object} options.palette 图表调色板（primary/accent）
 * @param {object} options.roamState 当前缩放/中心（拖动后保持视口）
 * @param {number} options.minZoom
 * @param {number} options.maxZoom
 */
export function buildMapOption({ points, arrowPoints, target, colors, palette, roamState, minZoom, maxZoom }) {
  const theme = colors
  const accent = palette.primary
  const accent2 = palette.accent
  const targetPoint = target

  const arrowData = targetPoint
    ? arrowPoints.map((item) => ({
        name: item.name,
        coords: [
          [item.lon, item.lat],
          [targetPoint.lon, targetPoint.lat]
        ],
        value: item.count,
        area: item.area
      }))
    : []

  return {
    backgroundColor: 'transparent',
    tooltip: {
      trigger: 'item',
      backgroundColor: theme.tooltipBg,
      borderColor: 'rgba(99,102,241,0.45)',
      textStyle: { color: theme.tooltipText, fontSize: 12 },
      formatter: (params) => {
        if (params.seriesName === '攻击链路') {
          return `<strong>${params.name} → ${targetPoint?.name || '防护目标'}</strong><br/>攻击次数：${params.data?.value ?? 0}`
        }
        if (params.seriesName === '防护目标') {
          return `<strong>${targetPoint?.name}</strong><br/>位置：${targetPoint?.area || '-'}`
        }
        const value = params.value || []
        return `<strong>${params.name}</strong><br/>攻击次数：${value[2] ?? 0}<br/>来源：${params.data?.area || '-'}<br/>类型：${
          (params.data?.attack_types || []).join('、') || '-'
        }`
      }
    },
    geo: {
      map: 'world',
      // 开启缩放与拖拽：滚轮 / 双指缩放，按住拖动平移
      roam: true,
      scaleLimit: { min: minZoom, max: maxZoom },
      zoom: roamState.zoom,
      center: roamState.center,
      itemStyle: {
        areaColor: theme.area,
        borderColor: theme.border,
        borderWidth: 0.6
      },
      emphasis: {
        itemStyle: { areaColor: theme.areaHover },
        label: { show: false }
      },
      silent: true
    },
    series: [
      {
        name: '攻击链路',
        type: 'lines',
        coordinateSystem: 'geo',
        zlevel: 1,
        effect: {
          show: true,
          period: 3.2,
          trailLength: 0.28,
          symbol: 'arrow',
          symbolSize: 7,
          color: accent2
        },
        lineStyle: {
          color: accent,
          width: 1.2,
          opacity: 0.55,
          curveness: 0.25
        },
        emphasis: { lineStyle: { width: 2.4, opacity: 0.9 } },
        data: arrowData
      },
      {
        name: '攻击来源',
        type: 'effectScatter',
        coordinateSystem: 'geo',
        zlevel: 2,
        rippleEffect: { brushType: 'stroke', scale: 3.2, period: 4 },
        symbolSize: (value) => Math.min(6 + Math.sqrt(value[2] || 1) * 2, 26),
        itemStyle: { color: '#f97316', shadowBlur: 14, shadowColor: 'rgba(249,115,22,0.85)' },
        label: {
          show: true,
          position: 'right',
          formatter: '{b}',
          color: theme.label,
          fontSize: 10
        },
        data: points.map((item) => ({
          name: item.name,
          value: [item.lon, item.lat, item.count],
          area: item.area,
          attack_types: item.attack_types
        }))
      },
      {
        name: '防护目标',
        type: 'scatter',
        coordinateSystem: 'geo',
        zlevel: 3,
        symbol: 'pin',
        symbolSize: 24,
        itemStyle: { color: accent, shadowBlur: 16, shadowColor: `${accent}aa` },
        label: {
          show: true,
          position: 'top',
          formatter: () => targetPoint?.name || '防护目标',
          color: theme.label,
          fontSize: 11,
          fontWeight: 600
        },
        data: targetPoint ? [{ name: targetPoint.name, value: [targetPoint.lon, targetPoint.lat, 0] }] : []
      }
    ]
  }
}

/** 近 N 日攻击趋势柱状图 */
export function buildTrendOption({ trend, colors, palette }) {
  const theme = colors
  return {
    grid: { left: 8, right: 12, top: 16, bottom: 4, containLabel: true },
    tooltip: { trigger: 'axis' },
    xAxis: {
      type: 'category',
      data: trend.map((item) => item.date.slice(5)),
      axisLine: { lineStyle: { color: theme.axis } },
      axisLabel: { color: theme.label, fontSize: 10 }
    },
    yAxis: { type: 'value', show: false },
    series: [
      {
        type: 'bar',
        data: trend.map((item) => item.num),
        barWidth: 10,
        itemStyle: {
          borderRadius: [4, 4, 0, 0],
          color: {
            type: 'linear',
            x: 0,
            y: 0,
            x2: 0,
            y2: 1,
            colorStops: [
              { offset: 0, color: palette.accent },
              { offset: 1, color: `${palette.accent}26` }
            ]
          }
        }
      }
    ]
  }
}
