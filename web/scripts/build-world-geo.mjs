/**
 * 生成攻击地图所需的 GeoJSON。
 * 数据来源：world-atlas（Natural Earth 110m），构建期一次性生成，运行时不依赖外网。
 *
 * 用法：npm run gen:map
 */
import { createRequire } from 'node:module'
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const require = createRequire(import.meta.url)
const here = dirname(fileURLToPath(import.meta.url))
const projectRoot = resolve(here, '..')

/** 读取 TopoJSON 数据 */
function loadTopology() {
  const candidates = [
    () => require.resolve('world-atlas/countries-110m.json'),
    () => join(projectRoot, 'node_modules/world-atlas/countries-110m.json')
  ]
  for (const candidate of candidates) {
    try {
      const file = candidate()
      if (file && existsSync(file)) {
        return JSON.parse(readFileSync(file, 'utf8'))
      }
    } catch (error) {
      /* 尝试下一个路径 */
    }
  }
  throw new Error('未找到 world-atlas/countries-110m.json，请先执行 npm install')
}

/** 递归压缩坐标精度，减小打包体积 */
function roundCoordinates(value, precision = 2) {
  const factor = 10 ** precision
  if (typeof value === 'number') return Math.round(value * factor) / factor
  if (Array.isArray(value)) return value.map((item) => roundCoordinates(item, precision))
  return value
}

const topojson = require('topojson-client')
const topology = loadTopology()
const collection = topojson.feature(topology, topology.objects.countries)

const output = {
  type: 'FeatureCollection',
  features: collection.features.map((feature) => ({
    type: 'Feature',
    properties: { name: feature.properties?.name ?? '' },
    geometry: {
      type: feature.geometry.type,
      coordinates: roundCoordinates(feature.geometry.coordinates)
    }
  }))
}

const targetDir = join(projectRoot, 'src', 'assets')
mkdirSync(targetDir, { recursive: true })
const target = join(targetDir, 'world.geo.json')
writeFileSync(target, JSON.stringify(output))

const sizeKb = (Buffer.byteLength(JSON.stringify(output)) / 1024).toFixed(1)
console.log(`[gen:map] 已生成 ${target}（${output.features.length} 个国家/地区，${sizeKb} KB）`)
