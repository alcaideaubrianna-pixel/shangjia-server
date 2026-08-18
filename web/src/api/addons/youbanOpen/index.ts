import { http } from '@/utils/http/axios';

export function BindingList(params = {}) {
  return http.request({ url: '/youban_open/binding/list', method: 'get', params });
}

export function BindingStatus(params = {}) {
  return http.request({ url: '/youban_open/binding/status', method: 'post', params });
}
