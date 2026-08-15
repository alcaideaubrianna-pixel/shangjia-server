import fs from 'node:fs';

const path = 'deploy/observability/dashboards/tg-operations.json';
const dashboard = JSON.parse(fs.readFileSync(path, 'utf8'));
const panel = (id, title, description, query, stream, legend, x, y, w, h, i) => ({
  id,
  type: 'line',
  title,
  description,
  config: {
    show_legends: true,
    legends_position: 'bottom',
    unit: 'short',
    connect_nulls: true,
    line_thickness: 2,
  },
  queryType: 'promql',
  queries: [{
    query,
    customQuery: true,
    fields: {
      stream,
      stream_type: 'metrics',
      x: [], y: [], z: [], breakdown: [],
      filter: {filterType: 'group', logicalOperator: 'AND', conditions: []},
    },
    config: {promql_legend: legend, layer_type: 'line', weight_fixed: 1, limit: 0, time_shift: []},
  }],
  layout: {x, y, w, h, i},
});

const throughputTab = {
  tabId: 'tg-business-throughput',
  name: '业务吞吐与排行',
  panels: [
    panel('tg-bot-send-ranking', 'Bot 发送消息排行', '最近 1 小时各 Bot 成功发送的消息量 Top 20。', 'topk(20, xiaohuiji_publish_bot_messages_1h)', 'xiaohuiji_publish_bot_messages_1h', '{bot} (ID: {bot_id})', 0, 0, 96, 16, 1),
    panel('tg-bot-collect-ranking', 'Bot 采集消息排行', '最近 1 小时各 Bot 接收的采集消息量 Top 20。', 'topk(20, xiaohuiji_collect_bot_messages_1h)', 'xiaohuiji_collect_bot_messages_1h', '{bot} (ID: {bot_id})', 96, 0, 96, 16, 2),
    panel('tg-account-collect-ranking', '协议账号采集数量', '最近 1 小时各常驻协议账号采集的消息量。', 'topk(20, xiaohuiji_collect_account_messages_1h)', 'xiaohuiji_collect_account_messages_1h', '{tg_account} (ID: {tg_account_id})', 0, 16, 96, 16, 3),
    panel('tg-account-throughput', '协议账号采集吞吐', '最近 1 小时折算的每分钟平均采集消息数。', 'topk(20, xiaohuiji_collect_account_messages_per_minute)', 'xiaohuiji_collect_account_messages_per_minute', '{tg_account} (ID: {tg_account_id})', 96, 16, 96, 16, 4),
    panel('tg-publish-account-latency', '业务账号平均推送时间', '最近 1 小时从 Job 创建到 TG 发送成功的平均耗时，数值越高越需要排查。', 'topk(20, xiaohuiji_publish_account_average_seconds)', 'xiaohuiji_publish_account_average_seconds', '{account} (ID: {account_id})', 0, 32, 96, 16, 5),
    panel('tg-publish-account-volume', '业务账号推送数量', '最近 1 小时各业务账号成功推送的 Job 数量。', 'topk(20, xiaohuiji_publish_account_messages_1h)', 'xiaohuiji_publish_account_messages_1h', '{account} (ID: {account_id})', 96, 32, 96, 16, 6),
    panel('tg-media-download-volume', '媒体下载数量', '最近 15 分钟下载完成的图片和视频数量。', 'xiaohuiji_collect_media_download_count_15m', 'xiaohuiji_collect_media_download_count_15m', '{media_type}', 0, 48, 64, 16, 7),
    panel('tg-media-download-speed', '媒体下载吞吐速度', '最近 15 分钟媒体下载聚合吞吐，单位 Mbps。', 'xiaohuiji_collect_media_download_throughput_mbps', 'xiaohuiji_collect_media_download_throughput_mbps', '{media_type}', 64, 48, 64, 16, 8),
    panel('tg-media-download-latency', '媒体平均下载耗时', '最近 15 分钟单个媒体平均下载耗时，单位毫秒。', 'xiaohuiji_collect_media_download_average_ms', 'xiaohuiji_collect_media_download_average_ms', '{media_type}', 128, 48, 64, 16, 9),
  ],
};

dashboard.tabs = dashboard.tabs.filter((tab) => tab.tabId !== throughputTab.tabId);
dashboard.tabs.push(throughputTab);

const deleteFallbackTab = {
  tabId: 'tg-delete-fallback',
  name: '协议号删除兜底',
  panels: [
    panel('tg-delete-fallback-events', '删除兜底事件趋势', '按结果查看排队、执行、成功、重试、限流和永久失败。flood_wait、dead 或 permanent_failed 大于 0 时需要排查。', 'sum by (result) (increase(xiaohuiji_tg_delete_fallback_events[15m]))', 'xiaohuiji_tg_delete_fallback_events', '{result}', 0, 0, 96, 16, 1),
    panel('tg-delete-fallback-flood-accounts', '限流协议号 Top 10', '最近 6 小时触发 FloodWait 的协议号。结合日志中的 taskId、jobId 和 wait 定位具体任务。', 'topk(10, sum by (tg_account_id) (increase(xiaohuiji_tg_delete_fallback_events{result="flood_wait"}[6h])))', 'xiaohuiji_tg_delete_fallback_events', '账号 {tg_account_id}', 96, 0, 96, 16, 2),
    panel('tg-delete-fallback-wait-p95', '等待时间 P95', 'queue 表示主动节流排队时间；flood_wait 表示 Telegram 返回的限流等待时间，单位秒。', 'histogram_quantile(0.95, sum by (le, kind) (rate(xiaohuiji_tg_delete_fallback_wait_seconds_bucket[15m])))', 'xiaohuiji_tg_delete_fallback_wait_seconds_bucket', '{kind}', 0, 16, 96, 16, 3),
    panel('tg-delete-fallback-messages', '删除消息数量', '最近 15 分钟排队和实际成功删除的消息数量。success 持续为 0 且 queued 增长表示任务积压。', 'sum by (result) (increase(xiaohuiji_tg_delete_fallback_messages[15m]))', 'xiaohuiji_tg_delete_fallback_messages', '{result}', 96, 16, 96, 16, 4),
  ],
};

dashboard.tabs = dashboard.tabs.filter((tab) => tab.tabId !== deleteFallbackTab.tabId);
dashboard.tabs.push(deleteFallbackTab);

const accountStatusPanel = dashboard.tabs
  .flatMap((tab) => tab.panels)
  .find((item) => item.id === 'tg-account-status');
if (accountStatusPanel) {
  accountStatusPanel.type = 'bar';
  accountStatusPanel.title = 'TG 账号状态分布';
  accountStatusPanel.description = '按当前授权状态汇总 TG 协议账号数量。authorized 为已授权；其他状态表示登录失效、异常或处理中。';
  accountStatusPanel.config = {
    show_legends: true,
    legends_position: 'bottom',
    unit: 'short',
  };
  accountStatusPanel.queries[0].query = 'sum by (status) (xiaohuiji_tg_accounts) or label_replace(vector(0), "status", "无数据", "", "")';
  accountStatusPanel.queries[0].config.layer_type = 'bar';
  accountStatusPanel.queries[0].config.promql_legend = '{status}';
}

for (const item of dashboard.tabs.flatMap((tab) => tab.panels)) {
  for (const query of item.queries ?? []) {
    if (query.query && !query.query.includes('vector(0)')) {
      query.query = `(${query.query}) or vector(0)`;
    }
  }
}

const overview = dashboard.tabs.find((tab) => tab.tabId === 'tg-incident-overview');
const fixedPanelIds = new Set([
  'tg-observability-heartbeat',
  'tg-media-purpose-violations',
]);
overview.panels = overview.panels.filter((item) => !fixedPanelIds.has(item.id));
const minimumOverviewY = Math.min(...overview.panels.map((item) => item.layout.y));
overview.panels.forEach((item) => {
  item.layout.y = item.layout.y - minimumOverviewY + 14;
});
overview.panels.unshift(
  panel(
    'tg-observability-heartbeat',
    '监控数据是否正常上报',
    '各 Railway 角色最近一分钟心跳。全部为 0 时优先检查 OTel Collector 到 OpenObserve 的链路，不要误判为业务无异常。',
    'sum by (role) (increase(xiaohuiji_runtime_heartbeats[2m])) or vector(0)',
    'xiaohuiji_runtime_heartbeats',
    '{role}', 0, 0, 96, 14, 1,
  ),
  panel(
    'tg-media-purpose-violations',
    '展示与验证资料混组拦截',
    '数值大于 0 表示发送层或落库层发现 display/verify 混组并已阻止发送，必须立即排查调用链。',
    'sum by (stage) (increase(xiaohuiji_tg_media_purpose_violations[15m])) or vector(0)',
    'xiaohuiji_tg_media_purpose_violations',
    '{stage}', 96, 0, 96, 14, 2,
  ),
);

overview.panels.forEach((item, index) => {
  item.layout.i = index + 1;
});

dashboard.version = 8;
dashboard.title = '小灰机 Telegram 运行状态';
dashboard.description = '先看监控心跳，再看异常、积压、账号租约、Webhook、采集媒体和频道推送。无异常显示 0，无数据不再显示空白。';
dashboard.tabs.forEach((tab) => {
  tab.name = tab.name.replace('详情：', '');
  for (const item of tab.panels) {
    for (const query of item.queries ?? []) {
      if (query.config?.promql_legend?.includes('{channel_id}')) {
        query.config.promql_legend = query.config.promql_legend.replaceAll('channel {channel_id}', '{channel}').replaceAll('{channel_id}', '{channel}');
      }
    }
  }
});

fs.writeFileSync(path, `${JSON.stringify(dashboard, null, 2)}\n`);
