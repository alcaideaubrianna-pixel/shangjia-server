import { http } from '@/utils/http/axios';

export function BotList(params = {}) {
  return http.request({ url: '/youban_bot/bot/list', method: 'get', params });
}

export function BotSave(params = {}) {
  return http.request({ url: '/youban_bot/bot/save', method: 'POST', params });
}

export function BotDelete(params = {}) {
  return http.request({ url: '/youban_bot/bot/delete', method: 'POST', params });
}

export function BotRefresh(params = {}) {
  return http.request({ url: '/youban_bot/bot/refresh', method: 'POST', params });
}

export function BotRestart(params = {}) {
  return http.request({ url: '/youban_bot/bot/restart', method: 'POST', params });
}

export function FeatureList(params = {}) {
  return http.request({ url: '/youban_bot/bot/feature/list', method: 'get', params });
}

export function FeatureSave(params = {}) {
  return http.request({ url: '/youban_bot/bot/feature/save', method: 'POST', params });
}

export function UserList(params = {}) {
  return http.request({ url: '/youban_bot/bot/user/list', method: 'get', params });
}

export function AccountBindList(params = {}) {
  return http.request({ url: '/youban_bot/bot/binding/list', method: 'get', params });
}

export function AccountBindUnbind(params = {}) {
  return http.request({ url: '/youban_bot/bot/binding/unbind', method: 'POST', params });
}

export function MessageList(params = {}) {
  return http.request({ url: '/youban_bot/bot/message/list', method: 'get', params });
}

export function UserSwitchSuperAdmin(params = {}) {
  return http.request({ url: '/youban_bot/bot/user/superAdmin', method: 'POST', params });
}

export function SendMessage(params = {}) {
  return http.request({ url: '/youban_bot/bot/message/send', method: 'POST', params });
}

export function BroadcastCreate(params = {}) {
  return http.request({ url: '/youban_bot/bot/broadcast/create', method: 'POST', params });
}

export function BroadcastTask(params = {}) {
  return http.request({ url: '/youban_bot/bot/broadcast/task', method: 'get', params });
}

export function BroadcastList(params = {}) {
  return http.request({ url: '/youban_bot/bot/broadcast/list', method: 'get', params });
}

export function BroadcastRecipientList(params = {}) {
  return http.request({ url: '/youban_bot/bot/broadcast/recipient/list', method: 'get', params });
}
