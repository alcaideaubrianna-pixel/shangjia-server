import { http } from '@/utils/http/axios';

export function Overview(params = {}) {
  return http.request({
    url: '/contentImport/overview',
    method: 'get',
    params,
  });
}

export function RunList(params = {}) {
  return http.request({
    url: '/contentImport/runList',
    method: 'get',
    params,
  });
}

export function RunFeiNiu(params = {}) {
  return http.request({
    url: '/contentImport/runFeiNiu',
    method: 'POST',
    params,
  });
}
