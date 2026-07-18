<template>
  <n-spin :show="pageLoading">
    <n-space vertical :size="16">
      <n-card :bordered="false">
        <n-space justify="space-between" align="center">
          <n-form inline :show-feedback="false">
            <n-form-item label="时间范围">
              <n-date-picker
                v-model:value="timeRange"
                type="datetimerange"
                clearable
                :actions="['clear', 'confirm']"
                :is-date-disabled="disableDate"
                start-placeholder="开始时间"
                end-placeholder="结束时间"
              />
            </n-form-item>
            <n-form-item label="自动刷新">
              <n-switch v-model:value="autoRefresh" />
            </n-form-item>
            <n-form-item label="间隔">
              <n-select
                v-model:value="refreshInterval"
                :options="refreshOptions"
                :disabled="!autoRefresh"
              />
            </n-form-item>
          </n-form>
          <n-space>
            <n-button @click="resetRange">最近30分钟</n-button>
            <n-button type="primary" :loading="pageLoading" @click="load">刷新</n-button>
          </n-space>
        </n-space>
      </n-card>

      <n-tabs v-model:value="activeTab" type="line" animated @update:value="handleTabChange">
        <n-tab-pane name="overview" tab="概览" display-directive="show:lazy">
          <n-space vertical :size="16">
            <n-grid :cols="5" :x-gap="12" responsive="screen">
              <n-gi>
                <n-card :bordered="false">
                  <n-statistic label="拉取次数" :value="summary.total || 0" />
                </n-card>
              </n-gi>
              <n-gi>
                <n-card :bordered="false">
                  <n-statistic label="成功" :value="summary.success || 0" />
                </n-card>
              </n-gi>
              <n-gi>
                <n-card :bordered="false">
                  <n-statistic label="失败" :value="summary.failed || 0" />
                </n-card>
              </n-gi>
              <n-gi>
                <n-card :bordered="false">
                  <n-statistic label="推送失败" :value="pushFailedTotal" />
                </n-card>
              </n-gi>
              <n-gi>
                <n-card :bordered="false">
                  <n-statistic label="平均耗时(ms)" :value="summary.avgElapsedMs || 0" />
                </n-card>
              </n-gi>
            </n-grid>

            <n-grid :cols="2" :x-gap="12" :y-gap="12" responsive="screen">
              <n-gi>
                <n-card :bordered="false" title="采集稳定性">
                  <div ref="stabilityChartRef" class="monitor-chart"></div>
                </n-card>
              </n-gi>
              <n-gi>
                <n-card :bordered="false" title="链路耗时">
                  <div ref="latencyChartRef" class="monitor-chart"></div>
                </n-card>
              </n-gi>
            </n-grid>
          </n-space>
        </n-tab-pane>

        <n-tab-pane name="bindings" tab="频道分析" display-directive="show:lazy">
          <n-space vertical :size="16">
            <n-card :bordered="false" title="频道稳定性">
              <div ref="bindingChartRef" class="monitor-chart"></div>
            </n-card>

            <n-card :bordered="false" title="频道汇总">
              <n-data-table
                :columns="bindingColumns"
                :data="bindings"
                :pagination="{ pageSize: 10 }"
                :scroll-x="1720"
              />
            </n-card>
          </n-space>
        </n-tab-pane>

        <n-tab-pane name="queue" tab="推送队列" display-directive="show:lazy">
          <n-card :bordered="false">
            <n-space vertical :size="12">
              <n-space justify="space-between" align="center">
                <n-space align="center">
                  <n-tag :type="pushQueue.paused ? 'warning' : 'success'" bordered>
                    {{ pushQueue.paused ? '已暂停' : '运行中' }}
                  </n-tag>
                  <n-text depth="3"
                    >频道内每 20 秒最多推送 1 条，遇到 Telegram 限流自动退避。</n-text
                  >
                </n-space>
                <n-space>
                  <n-button :loading="queueLoading" @click="loadQueue(true)">刷新队列</n-button>
                  <n-button
                    :type="pushQueue.paused ? 'primary' : 'warning'"
                    :loading="queueLoading"
                    @click="togglePushQueue"
                  >
                    {{ pushQueue.paused ? '恢复队列' : '暂停队列' }}
                  </n-button>
                </n-space>
              </n-space>
              <n-grid :cols="5" :x-gap="12" responsive="screen">
                <n-gi v-for="item in pushQueue.summary" :key="item.status">
                  <n-statistic :label="item.label" :value="item.count || 0" />
                </n-gi>
              </n-grid>
              <n-data-table
                :columns="queueChannelColumns"
                :data="pushQueue.channels"
                :pagination="{ pageSize: 8 }"
                :scroll-x="1180"
              />
              <n-data-table
                :columns="queueTaskColumns"
                :data="pushQueue.recent"
                :pagination="{ pageSize: 8 }"
                :scroll-x="1480"
              />
              <n-data-table
                :columns="queueLogColumns"
                :data="pushQueue.failedLogs"
                :pagination="{ pageSize: 8 }"
                :scroll-x="1280"
              />
            </n-space>
          </n-card>
        </n-tab-pane>

        <n-tab-pane name="recent" tab="最近明细" display-directive="show:lazy">
          <n-card :bordered="false">
            <n-data-table
              :columns="columns"
              :data="recent"
              :pagination="{ pageSize: 10 }"
              :scroll-x="1280"
            />
          </n-card>
        </n-tab-pane>
      </n-tabs>
    </n-space>
  </n-spin>
</template>

<script lang="ts" setup>
  import type { Ref } from 'vue';
  import { computed, h, nextTick, onMounted, onUnmounted, ref, watch } from 'vue';
  import { NButton, NTag, useMessage } from 'naive-ui';
  import { useECharts } from '@/hooks/web/useECharts';
  import {
    bindingAutoPullControl,
    pullMonitorBindings,
    pullMonitorOverview,
    pullMonitorRecent,
    pushQueueControl,
    pushQueueMonitor,
  } from '@/api/addons/lazysheep_tggo/config';

  const message = useMessage();
  const loading = ref(false);
  const queueLoading = ref(false);
  const bindingActionLoading = ref('');
  const activeTab = ref('overview');
  const loadedTabs = ref<Record<string, boolean>>({});
  const summary = ref<any>({});
  const bindings = ref<any[]>([]);
  const recent = ref<any[]>([]);
  const buckets = ref<any[]>([]);
  const pushQueue = ref<any>({
    paused: false,
    summary: [],
    channels: [],
    recent: [],
    failedLogs: [],
  });
  const timeRange = ref<[number, number] | null>(defaultRange());
  const autoRefresh = ref(true);
  const refreshInterval = ref(30000);
  const stabilityChartRef = ref<HTMLDivElement | null>(null);
  const latencyChartRef = ref<HTMLDivElement | null>(null);
  const bindingChartRef = ref<HTMLDivElement | null>(null);
  const stabilityChart = useECharts(stabilityChartRef as Ref<HTMLDivElement>);
  const latencyChart = useECharts(latencyChartRef as Ref<HTMLDivElement>);
  const bindingChart = useECharts(bindingChartRef as Ref<HTMLDivElement>);
  let refreshTimer: number | undefined;

  const pageLoading = computed(() => loading.value || queueLoading.value);

  const refreshOptions = [
    { label: '15秒', value: 15000 },
    { label: '30秒', value: 30000 },
    { label: '60秒', value: 60000 },
  ];

  const rangeParams = computed(() => {
    const range = normalizeRange(timeRange.value);
    if (!range) {
      return {};
    }
    return {
      startAt: formatDateTime(range[0]),
      endAt: formatDateTime(range[1]),
    };
  });

  const bindingColumns = [
    { title: '机器人', key: 'botName', width: 140 },
    { title: '绑定', key: 'bindingKey', width: 180 },
    { title: '频道', key: 'chatLabel', width: 220, ellipsis: { tooltip: true } },
    { title: '次数', key: 'total', width: 80 },
    { title: '成功', key: 'success', width: 80 },
    { title: '失败', key: 'failed', width: 80 },
    { title: '获取', key: 'fetched', width: 80 },
    { title: '入库', key: 'stored', width: 80 },
    { title: '推送', key: 'pushed', width: 80 },
    { title: '推送失败', key: 'pushFailed', width: 90 },
    {
      title: '自动拉取',
      key: 'autoPull',
      width: 100,
      render(row) {
        return h(
          NTag,
          { type: row.autoPull ? 'success' : 'warning', bordered: false },
          { default: () => (row.autoPull ? '开启' : '关闭') }
        );
      },
    },
    { title: '关闭时间', key: 'autoPullStoppedAt', width: 160 },
    { title: '关闭原因', key: 'autoPullStopReason', width: 180, ellipsis: { tooltip: true } },
    {
      title: '操作',
      key: 'action',
      width: 110,
      render(row) {
        return h(
          NButton,
          {
            size: 'small',
            type: row.autoPull ? 'warning' : 'primary',
            loading: bindingActionLoading.value === row.bindingKey,
            onClick: () => toggleBindingAutoPull(row),
          },
          { default: () => (row.autoPull ? '关闭' : '开启') }
        );
      },
    },
    { title: '平均耗时(ms)', key: 'avgElapsedMs', width: 120 },
    {
      title: '最近状态',
      key: 'lastStatus',
      width: 100,
      render(row) {
        return h(
          NTag,
          { type: row.lastStatus ? 'success' : 'error', bordered: false },
          { default: () => (row.lastStatus ? '成功' : '失败') }
        );
      },
    },
    { title: '最近时间', key: 'lastAt', width: 160 },
    { title: '最近错误', key: 'lastError', ellipsis: { tooltip: true } },
  ];

  const columns = [
    { title: '时间', key: 'createdAt', width: 160 },
    { title: '机器人', key: 'botName', width: 140 },
    { title: '绑定', key: 'bindingKey', width: 180 },
    { title: '频道', key: 'chatLabel', width: 240, ellipsis: { tooltip: true } },
    {
      title: '类型',
      key: 'auto',
      width: 90,
      render(row) {
        return row.auto ? '自动' : '手动';
      },
    },
    {
      title: '状态',
      key: 'success',
      width: 90,
      render(row) {
        return h(
          NTag,
          { type: row.success ? 'success' : 'error', bordered: false },
          { default: () => (row.success ? '成功' : '失败') }
        );
      },
    },
    { title: '获取', key: 'fetched', width: 80 },
    { title: '入库', key: 'stored', width: 80 },
    { title: '推送', key: 'pushed', width: 80 },
    { title: '去重', key: 'deduped', width: 80 },
    { title: '跳过', key: 'skipped', width: 80 },
    { title: '失败数', key: 'failedCount', width: 90 },
    { title: '推送失败', key: 'pushFailed', width: 90 },
    { title: '耗时(ms)', key: 'elapsedMs', width: 100 },
    { title: '错误', key: 'error', ellipsis: { tooltip: true } },
  ];

  const queueChannelColumns = [
    { title: '机器人', key: 'botName', width: 140 },
    { title: '绑定', key: 'bindingKey', width: 220 },
    { title: '频道', key: 'chatLabel', width: 240, ellipsis: { tooltip: true } },
    { title: '待推送', key: 'ready', width: 90 },
    { title: '推送中', key: 'doing', width: 90 },
    { title: '重试中', key: 'retry', width: 90 },
    { title: '待确认', key: 'unknown', width: 90 },
    { title: '失败', key: 'dead', width: 80 },
    { title: '积压', key: 'backlog', width: 80 },
    { title: '最早任务', key: 'oldestAt', width: 160 },
    { title: '最近错误', key: 'lastError', ellipsis: { tooltip: true } },
  ];

  const queueTaskColumns = [
    { title: '任务ID', key: 'id', width: 90 },
    { title: '机器人', key: 'botName', width: 140 },
    { title: '频道', key: 'chatLabel', width: 240, ellipsis: { tooltip: true } },
    { title: '内容ID', key: 'contentId', width: 150 },
    {
      title: '状态',
      key: 'statusLabel',
      width: 100,
      render(row) {
        return h(
          NTag,
          { type: pushQueueStatusTag(row.status), bordered: false },
          { default: () => row.statusLabel || '未知' }
        );
      },
    },
    { title: '次数', key: 'attempts', width: 70 },
    { title: '下次重试', key: 'nextRetryAt', width: 160 },
    { title: '创建时间', key: 'createdAt', width: 160 },
    { title: '错误', key: 'lastError', ellipsis: { tooltip: true } },
  ];

  const queueLogColumns = [
    { title: '时间', key: 'createdAt', width: 160 },
    { title: '任务ID', key: 'taskId', width: 90 },
    { title: '机器人', key: 'botName', width: 140 },
    { title: '频道', key: 'chatLabel', width: 240, ellipsis: { tooltip: true } },
    { title: '尝试', key: 'attempt', width: 70 },
    { title: '耗时(ms)', key: 'elapsedMs', width: 100 },
    { title: '错误', key: 'error', ellipsis: { tooltip: true } },
  ];

  const pushFailedTotal = computed(() =>
    buckets.value.reduce((total, item) => total + Number(item?.pushFailed || 0), 0)
  );
  const stepNames = computed(() => {
    const set = new Set<string>();
    buckets.value.forEach((bucket) => {
      (bucket?.steps || []).forEach((step) => {
        if (step?.name) {
          set.add(step.name);
        }
      });
    });
    return Array.from(set);
  });

  function load() {
    return loadActiveTab(true);
  }

  function handleTabChange(tab: string) {
    activeTab.value = tab;
    loadActiveTab(false);
  }

  function loadActiveTab(force: boolean) {
    const tab = activeTab.value;
    if (!force && loadedTabs.value[tab]) {
      return Promise.resolve();
    }
    if (tab === 'overview') {
      return loadOverview();
    }
    if (tab === 'bindings') {
      return loadBindings();
    }
    if (tab === 'queue') {
      return loadQueue(true);
    }
    if (tab === 'recent') {
      return loadRecent();
    }
    return Promise.resolve();
  }

  function loadOverview() {
    loading.value = true;
    return pullMonitorOverview(rangeParams.value)
      .then((res) => {
        summary.value = res?.summary || {};
        buckets.value = res?.buckets || [];
        loadedTabs.value.overview = true;
        nextTick(renderOverviewCharts);
      })
      .finally(() => {
        loading.value = false;
      });
  }

  function loadBindings() {
    loading.value = true;
    return pullMonitorBindings(rangeParams.value)
      .then((res) => {
        bindings.value = res?.bindings || [];
        loadedTabs.value.bindings = true;
        nextTick(renderBindingChart);
      })
      .finally(() => {
        loading.value = false;
      });
  }

  function loadRecent() {
    loading.value = true;
    return pullMonitorRecent(rangeParams.value)
      .then((res) => {
        recent.value = res?.recent || [];
        loadedTabs.value.recent = true;
      })
      .finally(() => {
        loading.value = false;
      });
  }

  function loadQueue(force: boolean) {
    if (!force && loadedTabs.value.queue) {
      return Promise.resolve();
    }
    queueLoading.value = true;
    return pushQueueMonitor({ limit: 80 })
      .then((res) => {
        pushQueue.value = {
          paused: !!res?.paused,
          summary: res?.summary || [],
          channels: res?.channels || [],
          recent: res?.recent || [],
          failedLogs: res?.failedLogs || [],
        };
        loadedTabs.value.queue = true;
      })
      .finally(() => {
        queueLoading.value = false;
      });
  }

  function togglePushQueue() {
    queueLoading.value = true;
    pushQueueControl({ paused: !pushQueue.value.paused })
      .then(() => {
        message.success(pushQueue.value.paused ? '推送队列已恢复' : '推送队列已暂停');
        return loadQueue(true);
      })
      .finally(() => {
        queueLoading.value = false;
      });
  }

  function toggleBindingAutoPull(row) {
    if (!row?.bindingKey) {
      return;
    }
    bindingActionLoading.value = row.bindingKey;
    bindingAutoPullControl({ bindingKey: row.bindingKey, autoPull: !row.autoPull })
      .then(() => {
        message.success(row.autoPull ? '自动拉取已关闭' : '自动拉取已开启');
        loadedTabs.value.bindings = false;
        return loadBindings();
      })
      .finally(() => {
        bindingActionLoading.value = '';
      });
  }

  function renderOverviewCharts() {
    const labels = buckets.value.map((item) => item.time);
    stabilityChart.setOptions({
      tooltip: { trigger: 'axis' },
      legend: { data: ['成功', '失败', '推送失败', '总次数'] },
      grid: { left: 36, right: 16, top: 42, bottom: 28 },
      xAxis: { type: 'category', boundaryGap: false, data: labels },
      yAxis: { type: 'value', minInterval: 1 },
      series: [
        {
          name: '成功',
          type: 'line',
          smooth: true,
          data: buckets.value.map((item) => item.success),
        },
        {
          name: '失败',
          type: 'line',
          smooth: true,
          data: buckets.value.map((item) => item.failed),
        },
        {
          name: '推送失败',
          type: 'line',
          smooth: true,
          data: buckets.value.map((item) => item.pushFailed || 0),
        },
        {
          name: '总次数',
          type: 'line',
          smooth: true,
          data: buckets.value.map((item) => item.total),
        },
      ],
    });
    latencyChart.setOptions({
      tooltip: { trigger: 'axis' },
      legend: { data: ['平均耗时(ms)', ...stepNames.value] },
      grid: { left: 50, right: 16, top: 42, bottom: 28 },
      xAxis: { type: 'category', boundaryGap: false, data: labels },
      yAxis: { type: 'value' },
      series: [
        {
          name: '平均耗时(ms)',
          type: 'line',
          smooth: true,
          data: buckets.value.map((item) => item.avgElapsedMs),
        },
        ...stepNames.value.map((name) => ({
          name,
          type: 'line',
          smooth: true,
          data: buckets.value.map((item) => {
            const step = (item.steps || []).find((entry) => entry.name === name);
            return step?.avgMs || 0;
          }),
        })),
      ],
    });
  }

  function renderBindingChart() {
    const topBindings = bindings.value.slice(0, 12).reverse();
    bindingChart.setOptions({
      tooltip: { trigger: 'axis' },
      legend: { data: ['平均耗时(ms)', '失败次数', '推送失败'] },
      grid: { left: 120, right: 16, top: 42, bottom: 28 },
      xAxis: { type: 'value' },
      yAxis: {
        type: 'category',
        data: topBindings.map((item) =>
          shortLabel(item.chatLabel || item.bindingKey || item.chatId)
        ),
      },
      series: [
        {
          name: '平均耗时(ms)',
          type: 'line',
          smooth: true,
          data: topBindings.map((item) => item.avgElapsedMs || 0),
        },
        {
          name: '失败次数',
          type: 'line',
          smooth: true,
          data: topBindings.map((item) => item.failed || 0),
        },
        {
          name: '推送失败',
          type: 'line',
          smooth: true,
          data: topBindings.map((item) => item.pushFailed || 0),
        },
      ],
    });
  }

  function defaultRange(): [number, number] {
    const end = Date.now();
    return [end - 30 * 60 * 1000, end];
  }

  function resetRange() {
    timeRange.value = defaultRange();
  }

  function normalizeRange(range: [number, number] | null) {
    if (!range || range.length !== 2) {
      return null;
    }
    const maxRange = 3 * 24 * 60 * 60 * 1000;
    if (range[1] - range[0] > maxRange) {
      message.warning('最多只能查询 3 天内的数据');
      return [range[1] - maxRange, range[1]] as [number, number];
    }
    return range;
  }

  function disableDate(ts: number) {
    return ts > Date.now() || ts < Date.now() - 7 * 24 * 60 * 60 * 1000;
  }

  function formatDateTime(value: number) {
    const date = new Date(value);
    const pad = (num: number) => String(num).padStart(2, '0');
    return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(
      date.getHours()
    )}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
  }

  function shortLabel(value: string | number) {
    const text = String(value || '');
    return text.length > 18 ? `${text.slice(0, 8)}...${text.slice(-6)}` : text;
  }

  function pushQueueStatusTag(status: number) {
    if (status === 3) {
      return 'success';
    }
    if (status === 4 || status === 2) {
      return 'warning';
    }
    if (status === 5) {
      return 'error';
    }
    return 'info';
  }

  function startAutoRefresh() {
    stopAutoRefresh();
    if (!autoRefresh.value) {
      return;
    }
    refreshTimer = window.setInterval(() => loadActiveTab(true), refreshInterval.value);
  }

  function stopAutoRefresh() {
    if (refreshTimer) {
      window.clearInterval(refreshTimer);
      refreshTimer = undefined;
    }
  }

  watch(timeRange, () => {
    loadedTabs.value = {};
    loadActiveTab(true);
  });
  watch([autoRefresh, refreshInterval], startAutoRefresh);

  onMounted(() => {
    loadActiveTab(false);
    startAutoRefresh();
  });

  onUnmounted(stopAutoRefresh);
</script>

<style scoped>
  .monitor-chart {
    width: 100%;
    height: 320px;
  }
</style>
