import { http } from '@/utils/http/axios';

export function ConversationList(params = {}) {
  return http.request({
    url: '/youban_chat/chat/list',
    method: 'get',
    params,
  });
}

export function ConversationView(params = {}) {
  return http.request({
    url: '/youban_chat/chat/view',
    method: 'get',
    params,
  });
}

export function MessageList(params = {}) {
  return http.request({
    url: '/youban_chat/chat/messages',
    method: 'get',
    params,
  });
}

export function ClearConversation(params = {}) {
  return http.request({
    url: '/youban_chat/chat/clear',
    method: 'POST',
    params,
  });
}

export function MarkRead(params = {}) {
  return http.request({
    url: '/youban_chat/chat/read',
    method: 'POST',
    params,
  });
}

export function Unread(params = {}) {
  return http.request({
    url: '/youban_chat/chat/unread',
    method: 'get',
    params,
  });
}

export function BotList(params = {}) {
  return http.request({
    url: '/youban_chat/chat/botList',
    method: 'get',
    params,
  });
}

export function SaveBot(params = {}) {
  return http.request({
    url: '/youban_chat/chat/saveBot',
    method: 'POST',
    params,
  });
}

export function BindingList(params = {}) {
  return http.request({
    url: '/youban_chat/chat/bindingList',
    method: 'get',
    params,
  });
}

export function SaveBinding(params = {}) {
  return http.request({
    url: '/youban_chat/chat/saveBinding',
    method: 'POST',
    params,
  });
}

export function ChannelOptions(params = {}) {
  return http.request({
    url: '/youban_chat/chat/channelOptions',
    method: 'get',
    params,
  });
}

export function OperatorList(params = {}) {
  return http.request({
    url: '/youban_chat/chat/operatorList',
    method: 'get',
    params,
  });
}

export function SaveOperator(params = {}) {
  return http.request({
    url: '/youban_chat/chat/saveOperator',
    method: 'POST',
    params,
  });
}

export function FeatureList(params = {}) {
  return http.request({
    url: '/youban_chat/chat/featureList',
    method: 'get',
    params,
  });
}

export function SaveFeature(params = {}) {
  return http.request({
    url: '/youban_chat/chat/saveFeature',
    method: 'POST',
    params,
  });
}
