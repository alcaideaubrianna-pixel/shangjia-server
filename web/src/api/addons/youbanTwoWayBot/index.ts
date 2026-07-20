import { http } from '@/utils/http/axios';

const prefix = '/youban_two_way_bot/twoWayBot';

export function BotList(params = {}) {
  return http.request({ url: `${prefix}/list`, method: 'get', params });
}
