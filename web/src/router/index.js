import { authStore } from '../stores/auth'

/** 应用路由表 */
export const routes = [
  {
    path: '/login',
    name: 'login',
    component: () => import('../views/LoginView.vue'),
    meta: { public: true, title: '登录' }
  },
  {
    path: '/',
    component: () => import('../layouts/AdminLayout.vue'),
    children: [
      { path: '', redirect: '/dashboard' },
      {
        path: 'dashboard',
        name: 'dashboard',
        component: () => import('../views/DashboardView.vue'),
        meta: { title: '安全概览', crumb: '概览' }
      },
      {
        path: 'attack-map',
        name: 'attack-map',
        component: () => import('../views/AttackMapView.vue'),
        meta: { title: '攻击地图大屏', crumb: '攻击地图' }
      },
      {
        path: 'sites',
        name: 'sites',
        component: () => import('../views/SitesView.vue'),
        meta: { title: '站点管理', crumb: '防护配置 / 站点管理' }
      },
      { path: 'rules', redirect: '/rules/args' },
      {
        path: 'rules/:type',
        name: 'rules',
        component: () => import('../views/RulesView.vue'),
        meta: { title: 'WAF 规则', crumb: '防护配置 / WAF 规则' }
      },
      {
        path: 'users',
        name: 'users',
        component: () => import('../views/UsersView.vue'),
        meta: { title: '账号管理', crumb: '防护配置 / 账号管理' }
      },
      {
        path: 'logs',
        name: 'logs',
        component: () => import('../views/LogsView.vue'),
        meta: { title: '攻击日志', crumb: '审计分析 / 攻击日志' }
      },
      {
        path: 'slow-sql',
        name: 'slow-sql',
        component: () => import('../views/SlowSqlView.vue'),
        meta: { title: '慢 SQL 统计', crumb: '审计分析 / 慢 SQL' }
      },
      {
        path: 'apisix',
        name: 'apisix',
        component: () => import('../views/ApisixView.vue'),
        meta: { title: 'APISIX 网关', crumb: '防护配置 / APISIX 网关' }
      },
      {
        path: 'settings/system',
        name: 'system-config',
        component: () => import('../views/SystemSettingsView.vue'),
        meta: { title: '系统设置', crumb: '系统设置' }
      },
      // 兼容旧地址：各设置页已并入「系统设置」的选项卡
      { path: 'settings/lua', redirect: { path: 'settings/system', query: { tab: 'waf' } } },
      { path: 'settings/appearance', redirect: { path: 'settings/system', query: { tab: 'appearance' } } },
      { path: 'settings/ai', redirect: { path: 'settings/system', query: { tab: 'ai' } } },
      { path: 'settings/account', redirect: { path: 'settings/system', query: { tab: 'account' } } },
      {
        path: 'forbidden',
        name: 'forbidden',
        component: () => import('../views/ForbiddenView.vue'),
        meta: { title: '无访问权限', public: true }
      }
    ]
  },
  {
    path: '/:pathMatch(.*)*',
    name: 'not-found',
    component: () => import('../views/ForbiddenView.vue'),
    meta: { public: true, title: '页面不存在', code: 404 }
  }
]

/** 安装路由守卫：未登录跳转登录页，已登录访问登录页跳转首页 */
export function installGuards(router) {
  router.beforeEach(async (to) => {
    if (to.meta.public) return true
    if (authStore.state.user) return true

    const user = await authStore.fetchProfile()
    if (user) return true
    return { path: '/login', query: { redirect: to.fullPath } }
  })

  router.afterEach((to) => {
    const title = to.meta?.title
    document.title = title ? `${title} · X-WAF 管理台` : 'X-WAF 云防火墙管理台'
  })
}
