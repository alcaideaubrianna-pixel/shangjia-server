import { http } from '@/utils/http/axios';

const mobileApiOptions = {
  urlPrefix: '/api',
};

export function ListProfiles(params) {
  return http.request({ url: '/content/profiles', method: 'get', params }, mobileApiOptions);
}

export function ViewProfile(params) {
  return http.request({ url: '/content/profile/view', method: 'get', params }, mobileApiOptions);
}

export function ListAnnouncements(params) {
  return http.request({ url: '/content/announcements', method: 'get', params }, mobileApiOptions);
}

export function MobileRegister(params) {
  return http.request({ url: '/member/register', method: 'POST', params }, mobileApiOptions);
}

export function MobileAccountLogin(params) {
  return http.request({ url: '/member/accountLogin', method: 'POST', params }, mobileApiOptions);
}
