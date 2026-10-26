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
    │   └── slowsql.js  ai.js  dashboard.js
    ├── router/index.js     # 路由表 + 登录守卫 + 标题
    ├── stores/
    │   ├── auth.js         # 登录态（reactive + computed）
    │   ├── theme.js        # 亮/暗主题，持久化 localStorage
    │   └── rules.js        # 规则类型缓存（侧边栏子菜单与规则页共用）
    ├── layouts/AdminLayout.vue   # 侧栏 + 顶栏 + 内容区（移动端抽屉）
    ├── components/
    │   ├── AppSidebar.vue  AppHeader.vue
    │   ├── StatCard.vue    ChartPanel.vue
    │   └── AiAnalyzeDialog.vue
    ├── views/              # 每个菜单一个页面
    │   ├── LoginView.vue   DashboardView.vue   AttackMapView.vue
    │   ├── SitesView.vue   RulesView.vue       UsersView.vue
    │   ├── LogsView.vue    SlowSqlView.vue
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

## 状态管理

不使用 Pinia，三处轻量 `reactive` 即可覆盖：

| store | 内容 | 使用方 |
| --- | --- | --- |
| `stores/auth.js` | `user`、`isLoggedIn`、`login/logout/fetchProfile` | 路由守卫、顶栏、账号页 |
| `stores/theme.js` | `dark`，切换时写入 `html.dark` 与 localStorage | 顶栏、登录页 |
| `stores/rules.js` | 规则类型与数量（`load/grouped/find`） | 侧边栏子菜单、规则页类型切换 |

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
