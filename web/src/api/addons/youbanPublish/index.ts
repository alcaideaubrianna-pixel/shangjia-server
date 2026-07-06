import { http } from '@/utils/http/axios';

export function TenantList(params = {}) {
  return http.request({ url: '/youban_publish/publish/tenant/list', method: 'get', params });
}

export function TenantSave(params = {}) {
  return http.request({ url: '/youban_publish/publish/tenant/save', method: 'POST', params });
}

export function TenantDelete(params = {}) {
  return http.request({ url: '/youban_publish/publish/tenant/delete', method: 'POST', params });
}

export function AccountList(params = {}) {
  return http.request({ url: '/youban_publish/publish/account/list', method: 'get', params });
}

export function AccountSave(params = {}) {
  return http.request({ url: '/youban_publish/publish/account/save', method: 'POST', params });
}

export function AccountResetPassword(params = {}) {
  return http.request({ url: '/youban_publish/publish/account/resetPwd', method: 'POST', params });
}

export function AccountDelete(params = {}) {
  return http.request({ url: '/youban_publish/publish/account/delete', method: 'POST', params });
}

export function TaskList(params = {}) {
  return http.request({ url: '/youban_publish/publish/task/list', method: 'get', params });
}

export function TaskSubmit(params = {}) {
  return http.request({ url: '/youban_publish/publish/task/submit', method: 'POST', params });
}

export function TaskCancel(params = {}) {
  return http.request({ url: '/youban_publish/publish/task/cancel', method: 'POST', params });
}

export function ImportTaskList(params = {}) {
  return http.request({ url: '/youban_publish/publish/importTask/list', method: 'get', params });
}

export function ImportTaskCreate(params = {}) {
  return http.request({ url: '/youban_publish/publish/importTask/create', method: 'POST', params });
}

export function ImportTaskView(params = {}) {
  return http.request({ url: '/youban_publish/publish/importTask/view', method: 'get', params });
}

export function ImportTaskStart(params = {}) {
  return http.request({ url: '/youban_publish/publish/importTask/start', method: 'POST', params });
}

export function ImportTaskCancel(params = {}) {
  return http.request({ url: '/youban_publish/publish/importTask/cancel', method: 'POST', params });
}

export function ImportTaskRetry(params = {}) {
  return http.request({ url: '/youban_publish/publish/importTask/retry', method: 'POST', params });
}

export function TagList(params = {}) {
  return http.request({ url: '/youban_publish/publish/tag/list', method: 'get', params });
}

export function TagSave(params = {}) {
  return http.request({ url: '/youban_publish/publish/tag/save', method: 'POST', params });
}

export function TagDelete(params = {}) {
  return http.request({ url: '/youban_publish/publish/tag/delete', method: 'POST', params });
}

export function BotList(params = {}) {
  return http.request({ url: '/youban_publish/publish/bot/list', method: 'get', params });
}

export function BotSave(params = {}) {
  return http.request({ url: '/youban_publish/publish/bot/save', method: 'POST', params });
}

export function BotDelete(params = {}) {
  return http.request({ url: '/youban_publish/publish/bot/delete', method: 'POST', params });
}

export function ConfigGet(params = {}) {
  return http.request({ url: '/youban_publish/publish/config/get', method: 'get', params });
}

export function ConfigUpdate(params = {}) {
  return http.request({ url: '/youban_publish/publish/config/update', method: 'POST', params });
}
