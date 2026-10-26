import { api } from './client'

/** 看板与攻击地图接口 */
export const dashboardApi = {
  summary: () => api.get('/dashboard/summary'),
  /** 攻击地图快照，range 取 today / all */
  attackMap: (range = 'all') => api.get('/attack-map', { range })
}
