import { http } from '@/utils/http/axios';

export function getConfig(params) {
  return http.request({
    url: '/lazysheep_tggo/config/get',
    method: 'get',
    params,
  });
}

export function updateConfig(params) {
  return http.request({
    url: '/lazysheep_tggo/config/update',
    method: 'post',
    params,
  });
}

export function upsertBot(params) {
  return http.request({
    url: '/lazysheep_tggo/config/upsertBot',
    method: 'post',
    params,
  });
}

export function bots(params = {}) {
  return http.request({
    url: '/lazysheep_tggo/config/bots',
    method: 'get',
    params,
  });
}

export function channelList(params = {}) {
  return http.request({
    url: '/lazysheep_tggo/config/channelList',
    method: 'get',
    params,
  });
}

export function inspectBot(params) {
  return http.request({
    url: '/lazysheep_tggo/config/inspectBot',
    method: 'post',
    params,
  });
}

export function deleteBot(params) {
  return http.request({
    url: '/lazysheep_tggo/config/deleteBot',
    method: 'post',
    params,
  });
}

export function startBot(params) {
  return http.request({
    url: '/lazysheep_tggo/config/startBot',
    method: 'post',
    params,
  });
}

export function botUsers(params) {
  return http.request({
    url: '/lazysheep_tggo/config/botUsers',
    method: 'get',
    params,
  });
}

export function updateBotUser(params) {
  return http.request({
    url: '/lazysheep_tggo/config/updateBotUser',
    method: 'post',
    params,
  });
}

export function testTelegramProxy(params) {
  return http.request({
    url: '/lazysheep_tggo/config/testTelegramProxy',
    method: 'post',
    params,
  });
}

export function pullMonitor(params = {}) {
  return http.request({
    url: '/lazysheep_tggo/config/pullMonitor',
    method: 'get',
    params,
  });
}

export function pullMonitorOverview(params = {}) {
  return http.request({
    url: '/lazysheep_tggo/config/pullMonitorOverview',
    method: 'get',
    params,
  });
}

export function pullMonitorBindings(params = {}) {
  return http.request({
    url: '/lazysheep_tggo/config/pullMonitorBindings',
    method: 'get',
    params,
  });
}

export function pullMonitorRecent(params = {}) {
  return http.request({
    url: '/lazysheep_tggo/config/pullMonitorRecent',
    method: 'get',
    params,
  });
}

export function pushQueueMonitor(params = {}) {
  return http.request({
    url: '/lazysheep_tggo/config/pushQueueMonitor',
    method: 'get',
    params,
  });
}

export function pushQueueControl(params) {
  return http.request({
    url: '/lazysheep_tggo/config/pushQueueControl',
    method: 'post',
    params,
  });
}

export function bindingAutoPullControl(params) {
  return http.request({
    url: '/lazysheep_tggo/config/bindingAutoPullControl',
    method: 'post',
    params,
  });
}
