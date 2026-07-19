import { http } from '@/utils/http/axios';

const prefix = '/youban_feiniu_sync/sync';

export function TenantOptions(params = {}) {
  return http.request({ url: `${prefix}/options/tenants`, method: 'get', params });
}
export function AdminAccountOptions(params = {}) {
  return http.request({ url: `${prefix}/options/adminAccounts`, method: 'get', params });
}

export function Dashboard(params = {}) {
  return http.request({ url: `${prefix}/dashboard`, method: 'get', params });
}
export function DashboardSummary(params = {}) {
  return http.request({ url: `${prefix}/dashboard/summary`, method: 'get', params });
}
export function DashboardTrend(params = {}) {
  return http.request({ url: `${prefix}/dashboard/trend`, method: 'get', params });
}
export function DashboardChannelRank(params = {}) {
  return http.request({ url: `${prefix}/dashboard/channelRank`, method: 'get', params });
}
export function DashboardRecentRuns(params = {}) {
  return http.request({ url: `${prefix}/dashboard/recentRuns`, method: 'get', params });
}

export function ConfigList(params = {}) {
  return http.request({ url: `${prefix}/config/list`, method: 'get', params });
}
export function ConfigSave(params = {}) {
  return http.request({ url: `${prefix}/config/save`, method: 'POST', params });
}
export function ConfigDelete(params = {}) {
  return http.request({ url: `${prefix}/config/delete`, method: 'POST', params });
}
export function ConfigAutoSync(params = {}) {
  return http.request({ url: `${prefix}/config/autoSync`, method: 'POST', params });
}
export function ConfigTest(params = {}) {
  return http.request({ url: `${prefix}/config/test`, method: 'POST', params });
}

export function ChannelMapList(params = {}) {
  return http.request({ url: `${prefix}/channel/list`, method: 'get', params });
}
export function ChannelClear(params = {}) {
  return http.request({ url: `${prefix}/channel/clear`, method: 'POST', params });
}

export function RunList(params = {}) {
  return http.request({ url: `${prefix}/run/list`, method: 'get', params });
}
export function RunView(params = {}) {
  return http.request({ url: `${prefix}/run/view`, method: 'get', params });
}
export function RunItems(params = {}) {
  return http.request({ url: `${prefix}/run/items`, method: 'get', params });
}
export function RunStart(params = {}) {
  return http.request({ url: `${prefix}/run/start`, method: 'POST', params });
}
