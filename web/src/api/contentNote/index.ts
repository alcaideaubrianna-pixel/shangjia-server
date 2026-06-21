import { http } from '@/utils/http/axios';

export function List(params) {
  return http.request({
    url: '/contentNote/list',
    method: 'get',
    params,
  });
}

export function View(params) {
  return http.request({
    url: '/contentNote/view',
    method: 'GET',
    params,
  });
}

export function Edit(params) {
  return http.request({
    url: '/contentNote/edit',
    method: 'POST',
    params,
  });
}

export function MediaEdit(params) {
  return http.request({
    url: '/contentNote/mediaEdit',
    method: 'POST',
    params,
  });
}

export function BatchDelete(params) {
  return http.request({
    url: '/contentNote/batchDelete',
    method: 'POST',
    params,
  });
}

export function BatchReview(params) {
  return http.request({
    url: '/contentNote/batchReview',
    method: 'POST',
    params,
  });
}

export function BatchStatus(params) {
  return http.request({
    url: '/contentNote/batchStatus',
    method: 'POST',
    params,
  });
}

export function BatchRemark(params) {
  return http.request({
    url: '/contentNote/batchRemark',
    method: 'POST',
    params,
  });
}
