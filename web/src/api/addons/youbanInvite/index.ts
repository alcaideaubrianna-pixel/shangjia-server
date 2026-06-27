import { http } from '@/utils/http/axios';

export function Config() {
  return http.request({
    url: '/youban_invite/invite/config',
    method: 'get',
  });
}

export function SaveConfig(params = {}) {
  return http.request({
    url: '/youban_invite/invite/saveConfig',
    method: 'POST',
    params,
  });
}

export function List(params = {}) {
  return http.request({
    url: '/youban_invite/invite/list',
    method: 'get',
    params,
  });
}

export function SaveRecord(params = {}) {
  return http.request({
    url: '/youban_invite/invite/saveRecord',
    method: 'POST',
    params,
  });
}

export function Delete(params = {}) {
  return http.request({
    url: '/youban_invite/invite/delete',
    method: 'POST',
    params,
  });
}
