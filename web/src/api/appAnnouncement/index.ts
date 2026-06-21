import { http } from '@/utils/http/axios';

export function List(params) {
  return http.request({ url: '/appAnnouncement/list', method: 'get', params });
}

export function Edit(params) {
  return http.request({ url: '/appAnnouncement/edit', method: 'POST', params });
}

export function Delete(params) {
  return http.request({ url: '/appAnnouncement/delete', method: 'POST', params });
}

export function Status(params) {
  return http.request({ url: '/appAnnouncement/status', method: 'POST', params });
}

export function MaxSort(params = {}) {
  return http.request({ url: '/appAnnouncement/maxSort', method: 'get', params });
}
