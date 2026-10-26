# 前端工程说明（wafadmin-web）

X-WAF 管理台前端：Vue 3 + Vite + Element Plus + ECharts。
构建产物 `dist/` 通过 Go 的 `//go:embed all:web/dist` 内嵌进二进制，运行时不需要单独部署静态文件。

## 技术栈

| 依赖 | 版本范围 | 用途 |
| --- | --- | --- |
| vue | ^3.5 | 组合式 API（`<script setup>`） |
| vue-router | ^4.5 | history 模式路由，由后端做 index.html 回落 |
| element-plus | ^2.9 | 组件库 + 暗色主题变量（`theme-chalk/dark/css-vars.css`） |
| @element-plus/icons-vue | ^2.3 | 图标，全局注册后按名称使用 |
| echarts | ^5.6 | 趋势/占比/地图图表 |
| vite | ^6.0 | 开发服务器与构建 |
| world-atlas + topojson-client | devDependency | 生成攻击地图所需 GeoJSON |

## 目录结构

```
web/
├── index.html              # 入口 HTML（内联 SVG favicon）
├── vite.config.js          # 构建输出 dist；dev 时 /api 代理到 127.0.0.1:5002
├── scripts/
│   └── build-world-geo.mjs # 由 world-atlas 生成 src/assets/world.geo.json
└── src/
    ├── main.js             # 装配：路由、Element Plus 中文包、图标、全局样式
    ├── App.vue             # 根组件；注入 401 未登录回调
    ├── api/
    │   ├── client.js       # fetch 封装：CSRF、统一信封、ApiError、401 回调
    │   ├── auth.js  site.js  rule.js  user.js  log.js
    │   └── slowsql.js  ai.js  dashboard.js  system.js
    ├── router/index.js     # 路由表 + 登录守卫 + 标题
    ├── stores/
    │   ├── auth.js         # 登录态（reactive + computed）
    │   ├── theme.js        # 外观模式 + 主题色 + 图表调色板
    │   └── rules.js        # 规则类型缓存（侧边栏子菜单与规则页共用）
    ├── layouts/AdminLayout.vue   # 侧栏 + 顶栏 + 内容区（移动端抽屉）
    ├── components/
    │   ├── AppSidebar.vue  AppHeader.vue
    │   ├── StatCard.vue    ChartPanel.vue
    │   ├── ThemePanel.vue  ThemeDrawer.vue
    │   └── AiAnalyzeDialog.vue
    ├── views/              # 每个菜单一个页面
    │   ├── LoginView.vue   DashboardView.vue   AttackMapView.vue
    │   ├── SitesView.vue   RulesView.vue       UsersView.vue
    │   ├── LogsView.vue    SlowSqlView.vue
    │   ├── SystemConfigView.vue  AppearanceView.vue
    │   ├── AiSettingsView.vue  AccountView.vue  ForbiddenView.vue
    ├── styles/main.css     # 设计系统（色板/卡片/导航/表格/动效/暗色）
    ├── utils/format.js     # 时间、数字、耗时、SQL 语句格式化
    └── assets/world.geo.json  # 攻击地图数据（构建期生成，已随仓库提交）
```

## 常用命令

```bash
npm install          # 安装依赖
npm run dev          # 开发服务器 http://127.0.0.1:5173（/api 代理到 5002）
npm run build        # 产物输出到 dist/，随后在仓库根执行 go build 即可内嵌
npm run preview      # 预览构建产物
npm run gen:map      # 重新生成 src/assets/world.geo.json
```

> 改了 `src/` 之后必须 `npm run build`，否则 Go 二进制里仍是旧前端。
> `dist/` 已纳入版本管理，保证 `go build` 在没有 Node 环境的机器上也能通过（embed 需要文件存在）。

## 接口约定（api/client.js）

- 基础路径 `/api`，`credentials: 'same-origin'`；非 GET 请求自动读取 Cookie `waf_csrf` 写入 `X-CSRF-Token`。
- 统一解析 `{code,message,data}`：`code !== 0` 或 HTTP 非 2xx 抛出 `ApiError(message, code, status)`，页面用 `try/catch` + `ElMessage.error(e.message)` 展示。
- HTTP 401 会触发注册的未登录回调（`App.vue` 中跳转 `/login`）。
- 查询参数为 `undefined/null/''` 时自动忽略，便于表单直接透传。

## 响应式（移动端自适应）

断点与 `styles/main.css`、`utils/responsive.js` 保持一致：

| 断点 | 变化 |
| --- | --- |
| ≤1400px | 统计卡 4 列 → 2 列 |
| ≤1200px | 三列网格 → 2 列 |
| ≤900px | 侧栏变抽屉（点击遮罩关闭）、所有网格单列、页头堆叠、筛选区单列、表格紧凑、弹窗/抽屉按视口宽度（92%/88%） |
| ≤640px | 统计卡 2 列、图表高度 220px、顶栏隐藏搜索框与用户名、字号内边距收紧 |
| ≤360px | 统计卡回到单列 |

实现方式：

- **布局**：`waf-grid--2/3/4` 随断点降列；表单里并排的两列改用 `waf-inline-grid`（窄屏自动单列）；
- **弹窗**：≤900px 时 `.el-dialog` 限高 92vh 并改为 flex 布局，正文 `overflow-y: auto`
  —— 内容再长（如 AI 研判结论）也能在弹窗内滚动，页脚按钮始终可见；
  AI 分析弹窗在结果出现后会自动滚到「研判结果」区，避免手机上"看不到结果"；
- **AI 结论渲染**：`utils/markdown.js` 先把文本整体转义再转换标题/列表/代码/加粗，
  既安全（防 XSS）又比纯文本易读；长单词与链接强制换行，不撑破卡片；
- **表格**：次要列用 `v-if="!viewport.isMobile.value"` 隐藏（如日志的地区/站点、站点的后端/日志级别/最后变更），
  操作列 `fixed="right"` 固定并收窄，保证手机上始终能点到按钮；其余列可横向滑动；
- **图表**：`ChartPanel` 在移动端把高度压到 220px，`ResizeObserver` 自动重绘；
- **入口**：`AppLayout` ≤900px 用抽屉式侧栏，点击菜单后自动收起；触屏设备加大按钮点击区域。

页面级横向溢出的回归检查（无头浏览器遍历全部路由）：

```bash
node mobile-check.mjs http://127.0.0.1:5002 "$COOKIE" ./shots 390 844
# 全部 11 个页面在 390px 下无横向溢出
```

## 状态管理

不使用 Pinia，三处轻量 `reactive` 即可覆盖：

| store | 内容 | 使用方 |
| --- | --- | --- |
| `stores/auth.js` | `user`、`isLoggedIn`、`login/logout/fetchProfile` | 路由守卫、顶栏、账号页 |
| `stores/theme.js` | 外观模式、主题色、图表调色板 | 顶栏、登录页、外观设置页、图表 |
| `stores/rules.js` | 规则类型与数量（`load/grouped/find`） | 侧边栏子菜单、规则页类型切换 |
| `stores/ai.js` | AI 分析结果本地缓存（10 分钟，localStorage） | 日志页、AI 分析弹窗 |

## 主题系统

`stores/theme.js` 负责主题状态与落地：

| 能力 | 说明 |
| --- | --- |
| 外观模式 | `light` / `dark` / `system`，`system` 通过 `matchMedia('(prefers-color-scheme: dark)')` 跟随系统并监听变化 |
| 主题色 | 6 套预设（靛蓝/紫罗兰/晴空/翡翠/琥珀/玫瑰），同时驱动 `--waf-primary`、`--waf-gradient` 与 Element Plus 的 `--el-color-primary` 及 light-3/5/7/8/9、dark-2 色阶 |
| 持久化 | `localStorage['wafadmin.theme'] = {"mode":"dark","accent":"violet"}`，兼容早期只存 `"dark"/"light"` 的格式 |
| 图表联动 | `themeStore.chartPalette` 暴露 `primary/accent/series`，页面在 `computed` 中使用，切换主题色时图表自动重绘 |
| 快捷入口 | 顶栏调色板图标打开 `components/ThemeDrawer.vue`；太阳/月亮按钮一键切换亮暗 |

实现要点：主题色由 JS 在 `documentElement` 上以内联 CSS 变量写入（优先级高于 Element Plus 暗色主题的定义），
色阶用 `mix(primary, #fff/#000, ratio)` 计算，避免为每个色板手写一套变量。
`html.dark` 负责暗色，`html[data-accent]` 便于个别样式挂载。

## 路由与菜单

| 路径 | 页面 | 菜单 |
| --- | --- | --- |
| `/login` | 登录（公开） | — |
| `/dashboard` | 安全概览 | 概览 |
| `/attack-map` | 攻击地图大屏 | 概览 |
| `/sites` | 站点管理 | 防护配置 |
| `/rules/:type` | WAF 规则（`type` 为规则类型 key） | 防护配置 › WAF 规则（子菜单） |
| `/users` | 账号管理 | 防护配置 |
| `/logs` | 攻击日志 | 审计分析 |
| `/slow-sql` | 慢 SQL 统计 | 审计分析 |
| `/settings/system` | 运行配置（编辑 conf/app.ini） | 系统设置 |
| `/settings/appearance` | 外观设置（主题切换） | 系统设置 |
| `/settings/ai` | AI 设置 | 系统设置 |
| `/settings/account` | 账号设置 | 系统设置 |

菜单结构定义在 `components/AppSidebar.vue` 的 `groups` 常量；规则子菜单由 `stores/rules.js` 按
`group` 字段（拦截策略 / 访问控制）自动分组，新增规则类型时后端 `model.Catalog` 增加一条即可，前端无需改动。

## 设计系统要点

- 品牌渐变 `--waf-gradient: #6366f1 → #8b5cf6`；卡片圆角 14px，阴影轻量（`--waf-shadow-sm`）。
- 颜色、圆角、阴影、背景等全部通过 `styles/main.css` 的 CSS 变量定义；暗色主题在 `html.dark` 作用域覆盖。
- 常用类：`waf-page`（页面容器）、`waf-page-head`、`waf-card`、`waf-grid--4/3/2`、`waf-mono`、
  `waf-code`、`waf-empty`、`waf-markdown`。
- 攻击地图大屏使用独立深色皮肤（`.screen*`），不随主题切换。
- 表格内文本按钮做了紧凑化处理，操作列避免换行。

## 新增一个页面

1. `src/api/xxx.js`：按业务域新增接口方法（复用 `api.get/post/put/del`）。
2. `src/views/XxxView.vue`：沿用 `waf-page` + `waf-page-head` + `waf-card` 结构。
3. `src/router/index.js`：在 `AdminLayout` 的 children 增加路由，填写 `meta.title`（顶栏标题）与 `meta.crumb`（面包屑）。
4. `src/components/AppSidebar.vue`：在对应分组的 `items` 增加一条（`icon` 用 Element Plus 图标名）。
5. 若有多个页面共享数据，放到 `src/stores/` 而不是各页面重复请求。
