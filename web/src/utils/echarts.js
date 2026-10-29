/**
 * ECharts 按需引入
 *
 * 全量 `import * as echarts from 'echarts'` 会打进 1.03MB（gzip 343KB），
 * 而本项目只用到：折线、柱状、饼图、散点、涟漪散点、飞线 + 世界地图 geo。
 * 这里按需注册这些模块，其它地方统一从本文件导入。
 *
 * 新增图表类型时：在 CHARTS / COMPONENTS 里补一项即可。
 */
import * as echarts from 'echarts/core'
import { BarChart, LineChart, LinesChart, PieChart, ScatterChart, EffectScatterChart } from 'echarts/charts'
import {
  DatasetComponent,
  GeoComponent,
  GridComponent,
  LegendComponent,
  TitleComponent,
  ToolboxComponent,
  TooltipComponent
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'

echarts.use([
  BarChart,
  LineChart,
  LinesChart,
  PieChart,
  ScatterChart,
  EffectScatterChart,
  DatasetComponent,
  GeoComponent,
  GridComponent,
  LegendComponent,
  TitleComponent,
  ToolboxComponent,
  TooltipComponent,
  CanvasRenderer
])

/** 世界地图是否已注册（registerMap 重复调用会覆盖，加个标记省一次开销） */
let worldRegistered = false

/**
 * 注册世界地图（幂等）
 * @param {object} geoJson world.geo.json 内容
 */
export function ensureWorldMap(geoJson) {
  if (worldRegistered || !geoJson) return
  echarts.registerMap('world', geoJson)
  worldRegistered = true
}

export default echarts
export { echarts }
