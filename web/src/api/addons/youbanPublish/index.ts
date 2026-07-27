import { http } from '@/utils/http/axios';

export function TenantList(params = {}) {
  return http.request({ url: '/youban_publish/publish/tenant/list', method: 'get', params });
}

export function Dashboard(params = {}) {
  return http.request({ url: '/youban_publish/publish/dashboard', method: 'get', params });
}

export function TenantSave(params = {}) {
  return http.request({ url: '/youban_publish/publish/tenant/save', method: 'POST', params });
}

export function TenantDelete(params = {}) {
  return http.request({ url: '/youban_publish/publish/tenant/delete', method: 'POST', params });
}

export function AccountList(params = {}) {
  return http.request({ url: '/youban_publish/publish/account/list', method: 'get', params });
}

export function ChannelList(params = {}) {
  return http.request({ url: '/youban_publish/publish/channel/list', method: 'get', params });
}

export function AccountSave(params = {}) {
  return http.request({ url: '/youban_publish/publish/account/save', method: 'POST', params });
}

export function AccountResetPassword(params = {}) {
  return http.request({ url: '/youban_publish/publish/account/resetPwd', method: 'POST', params });
}

export function AccountDelete(params = {}) {
  return http.request({ url: '/youban_publish/publish/account/delete', method: 'POST', params });
}

export function PublishRecordList(params = {}) {
  return http.request({ url: '/youban_publish/publish/record/list', method: 'get', params });
}

export function TgObserveQueueList(params = {}) {
  return http.request({ url: '/youban_publish/publish/observe/queue/list', method: 'get', params });
}

export function TgObserveChannelList(params = {}) {
  return http.request({
    url: '/youban_publish/publish/observe/channel/list',
    method: 'get',
    params,
  });
}

export function TgObserveBotList(params = {}) {
  return http.request({ url: '/youban_publish/publish/observe/bot/list', method: 'get', params });
}

export function ProfileList(params = {}) {
  return http.request({ url: '/youban_publish/publish/profile/list', method: 'get', params });
}

export function ProfileView(params = {}) {
  return http.request({ url: '/youban_publish/publish/profile/view', method: 'get', params });
}

export function ProfileSave(params = {}) {
  return http.request({ url: '/youban_publish/publish/profile/save', method: 'POST', params });
}

export function ProfileDelete(params = {}) {
  return http.request({ url: '/youban_publish/publish/profile/delete', method: 'POST', params });
}

export function ProfilePurgeDeleted(params = {}) {
  return http.request({
    url: '/youban_publish/publish/profile/purgeDeleted',
    method: 'POST',
    params,
  });
}

export function ProfileReview(params = {}) {
  return http.request({ url: '/youban_publish/publish/profile/review', method: 'POST', params });
}

export function ImportTaskList(params = {}) {
  return http.request({ url: '/youban_publish/publish/importTask/list', method: 'get', params });
}

export function ImportTaskCreate(params = {}) {
  return http.request({ url: '/youban_publish/publish/importTask/create', method: 'POST', params });
}

export function ImportTaskCreateForAccount(params = {}) {
  return http.request({
    url: '/youban_publish/publish/materialImport/createForAccount',
    method: 'POST',
    params,
  });
}

export function ServerTgAccountList(params = {}) {
  return http.request({
    url: '/youban_publish/publish/admin/tgAccount/list',
    method: 'get',
    params,
  });
}

export function ImportTaskView(params = {}) {
  return http.request({ url: '/youban_publish/publish/importTask/view', method: 'get', params });
}

export function ImportTaskStart(params = {}) {
  return http.request({ url: '/youban_publish/publish/importTask/start', method: 'POST', params });
}

export function ImportTaskCancel(params = {}) {
  return http.request({ url: '/youban_publish/publish/importTask/cancel', method: 'POST', params });
}

export function ImportTaskRetry(params = {}) {
  return http.request({ url: '/youban_publish/publish/importTask/retry', method: 'POST', params });
}

export function ImportTaskScan(params = {}) {
  return http.request({ url: '/youban_publish/publish/importTask/scan', method: 'POST', params });
}

export function ImportTaskRepair(params = {}) {
  return http.request({ url: '/youban_publish/publish/importTask/repair', method: 'POST', params });
}

export function ImportRunList(params = {}) {
  return http.request({ url: '/youban_publish/publish/importRun/list', method: 'get', params });
}

export function ImportRunCreate(params = {}) {
  return http.request({ url: '/youban_publish/publish/importRun/create', method: 'POST', params });
}

export function ImportRunDelete(params = {}) {
  return http.request({ url: '/youban_publish/publish/importRun/delete', method: 'POST', params });
}

export function ImportRunCancel(params = {}) {
  return http.request({ url: '/youban_publish/publish/importRun/cancel', method: 'POST', params });
}

export function ImportRunLogList(params = {}) {
  return http.request({ url: '/youban_publish/publish/importRun/logs', method: 'get', params });
}

export function ImportRunLogClear(params = {}) {
  return http.request({
    url: '/youban_publish/publish/importRun/clearLogs',
    method: 'POST',
    params,
  });
}

export function ImportRunMatchConfig(params = {}) {
  return http.request({
    url: '/youban_publish/publish/importRunMatch/config',
    method: 'get',
    params,
  });
}

export function ImportRunMatchStart(params = {}) {
  return http.request({
    url: '/youban_publish/publish/importRunMatch/start',
    method: 'POST',
    params,
  });
}

export function ImportRunTgSyncStart(params = {}) {
  return http.request({
    url: '/youban_publish/publish/importRunTgSync/start',
    method: 'POST',
    params,
  });
}

export function ImportRunMatchView(params = {}) {
  return http.request({
    url: '/youban_publish/publish/importRunMatch/view',
    method: 'get',
    params,
  });
}

export function ImportRunMatchItemList(params = {}) {
  return http.request({
    url: '/youban_publish/publish/importRunMatch/items',
    method: 'get',
    params,
  });
}

export function ImportRunMatchCandidateList(params = {}) {
  return http.request({
    url: '/youban_publish/publish/importRunMatch/candidates',
    method: 'get',
    params,
  });
}

export function ImportRunMatchCandidateSearch(params = {}) {
  return http.request({
    url: '/youban_publish/publish/importRunMatch/candidateSearch',
    method: 'get',
    params,
  });
}

export function ImportRunMatchSaveDraft(params = {}) {
  return http.request({
    url: '/youban_publish/publish/importRunMatch/saveDraft',
    method: 'POST',
    params,
  });
}

export function ImportRunMatchConfirm(params = {}) {
  return http.request({
    url: '/youban_publish/publish/importRunMatch/confirm',
    method: 'POST',
    params,
  });
}

export function ImportRunMatchBatchConfirm(params = {}) {
  return http.request({
    url: '/youban_publish/publish/importRunMatch/batchConfirm',
    method: 'POST',
    params,
  });
}

export function ImportRunMatchSkip(params = {}) {
  return http.request({
    url: '/youban_publish/publish/importRunMatch/skip',
    method: 'POST',
    params,
  });
}

export function ImportRunMatchUnbind(params = {}) {
  return http.request({
    url: '/youban_publish/publish/importRunMatch/unbind',
    method: 'POST',
    params,
  });
}

export function TagList(params = {}) {
  return http.request({ url: '/youban_publish/publish/tag/list', method: 'get', params });
}

export function TagSave(params = {}) {
  return http.request({ url: '/youban_publish/publish/tag/save', method: 'POST', params });
}

export function TagDelete(params = {}) {
  return http.request({ url: '/youban_publish/publish/tag/delete', method: 'POST', params });
}

export function BotList(params = {}) {
  return http.request({ url: '/youban_publish/publish/bot/list', method: 'get', params });
}

export function BotSave(params = {}) {
  return http.request({ url: '/youban_publish/publish/bot/save', method: 'POST', params });
}

export function BotDelete(params = {}) {
  return http.request({ url: '/youban_publish/publish/bot/delete', method: 'POST', params });
}

export function BotRefresh(params = {}) {
  return http.request({ url: '/youban_publish/publish/bot/refresh', method: 'POST', params });
}

export function ConfigGet(params = {}) {
  return http.request({ url: '/youban_publish/publish/config/get', method: 'get', params });
}

export function ConfigUpdate(params = {}) {
  return http.request({ url: '/youban_publish/publish/config/update', method: 'POST', params });
}

export function AdminInviteList(params = {}) {
  return http.request({ url: '/youban_publish/publish/admin/invite/list', method: 'get', params });
}

export function AdminTgAccountList(params = {}) {
  return http.request({
    url: '/youban_publish/publish/admin/tgAccount/list',
    method: 'get',
    params,
  });
}

export function AdminChannelCacheList(params = {}) {
  return http.request({
    url: '/youban_publish/publish/admin/channel/cache/list',
    method: 'get',
    params,
  });
}

export function ChannelMemberSyncStart(params = {}) {
  return http.request({
    url: '/youban_publish/publish/admin/channel/member/sync/start',
    method: 'post',
    params,
  });
}

export function ChannelMemberSyncView(params = {}) {
  return http.request({
    url: '/youban_publish/publish/admin/channel/member/sync/view',
    method: 'get',
    params,
  });
}

export function ChannelMemberSyncCancel(params = {}) {
  return http.request({
    url: '/youban_publish/publish/admin/channel/member/sync/cancel',
    method: 'post',
    params,
  });
}

export function ChannelMemberList(params = {}) {
  return http.request({
    url: '/youban_publish/publish/admin/channel/member/list',
    method: 'get',
    params,
  });
}

export function ChannelMemberExport(params = {}) {
  return http.request({
    url: '/youban_publish/publish/admin/channel/member/export',
    method: 'get',
    params,
  });
}

export function VipConfigView(params = {}) {
  return http.request({ url: '/youban_publish/publish/vip/config/view', method: 'get', params });
}

export function VipConfigSave(params = {}) {
  return http.request({ url: '/youban_publish/publish/vip/config/save', method: 'POST', params });
}

export function VipTenantSave(params = {}) {
  return http.request({ url: '/youban_publish/publish/vip/tenant/save', method: 'POST', params });
}

export function VipOrderList(params = {}) {
  return http.request({ url: '/youban_publish/publish/vip/order/list', method: 'get', params });
}

export function VipLogList(params = {}) {
  return http.request({ url: '/youban_publish/publish/vip/log/list', method: 'get', params });
}

export function VipCouponList(params = {}) {
  return http.request({ url: '/youban_publish/publish/vip/coupon/list', method: 'get', params });
}

export function VipCouponSave(params = {}) {
  return http.request({ url: '/youban_publish/publish/vip/coupon/save', method: 'POST', params });
}

export function VipCouponStatus(params = {}) {
  return http.request({ url: '/youban_publish/publish/vip/coupon/status', method: 'POST', params });
}
