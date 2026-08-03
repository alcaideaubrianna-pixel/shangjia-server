<template>
  <n-spin :show="loading">
    <n-space vertical size="large">
      <n-alert type="info" :bordered="false">
        大盘汇总与趋势仅查询全局日统计行，Top 10 查询用户每日聚合行，不读取原始调用日志。
      </n-alert>

      <n-space class="toolbar" align="center">
        <n-date-picker
          v-model:value="dateRange"
          type="daterange"
          clearable
          class="date-range"
          :shortcuts="cloudResourceDateShortcuts"
        />
        <n-select
          v-model:value="resourceType"
          :options="cloudResourceOptions"
          clearable
          placeholder="全部资源"
          class="resource-select"
        />
        <n-button type="primary" @click="loadDashboard">查询</n-button>
        <n-button quaternary @click="resetQuery">重置</n-button>
      </n-space>

      <n-grid cols="1 s:2 m:4 l:4 xl:4 2xl:4" responsive="screen" :x-gap="12" :y-gap="12">
        <n-grid-item v-for="item in summaryCards" :key="item.label">
          <n-card size="small" :bordered="true" class="summary-card">
            <n-space align="center" justify="space-between">
              <div>
                <div class="summary-card__label">{{ item.label }}</div>
                <div class="summary-card__value">{{ item.value }}</div>
              </div>
              <n-icon :component="item.icon" :size="30" :color="item.color" />
            </n-space>
            <div class="summary-card__hint">{{ item.hint }}</div>
          </n-card>
        </n-grid-item>
      </n-grid>

      <n-grid cols="1 l:3" responsive="screen" :x-gap="12" :y-gap="12">
        <n-grid-item span="1 l:2">
          <n-card title="调用趋势" size="small" :bordered="true">
            <div ref="trendChartRef" class="trend-chart"></div>
          </n-card>
        </n-grid-item>
        <n-grid-item>
          <n-card title="资源消耗" size="small" :bordered="true">
            <n-data-table
              :columns="breakdownColumns"
              :data="breakdown"
              :pagination="false"
              :row-key="(row) => row.resourceType"
              size="small"
            />
          </n-card>
        </n-grid-item>
      </n-grid>

      <n-card title="调用量 Top 10 用户" size="small" :bordered="true">
        <n-data-table
          :columns="topUserColumns"
          :data="topUsers"
          :pagination="false"
          :row-key="(row) => row.accountId"
          :scroll-x="1050"
          size="small"
        />
      </n-card>
    </n-space>
  </n-spin>
</template>

<script lang="ts" setup>
  import type { Ref } from 'vue';
  import { computed, h, onMounted, ref } from 'vue';
  import {
    CheckmarkCircleOutline,
    CloudOutline,
    PeopleOutline,
    TimeOutline,
  } from '@vicons/ionicons5';
  import { NTag } from 'naive-ui';
  import { CloudResourceUsageDashboard } from '@/api/addons/youbanPublish';
  import { useECharts } from '@/hooks/web/useECharts';
  import {
    cloudResourceDateShortcuts,
    cloudResourceLabel,
    cloudResourceOptions,
    cloudResourceSuccessRate,
    currentMonthRange,
    formatCloudResourceDuration,
    formatCloudResourceNumber,
    normalizeCloudResourceDateRange,
  } from './cloud-resource-usage-shared';

  const emptySummary = () => ({
    activeUserCount: 0,
    avgDurationMs: 0,
    backgroundMattingCount: 0,
    faceDetectionCount: 0,
    failureCount: 0,
    requestCount: 0,
    successCount: 0,
    validationCount: 0,
  });

  const loading = ref(false);
  const dateRange = ref<[number, number] | null>(currentMonthRange());
  const resourceType = ref('');
  const summary = ref<any>(emptySummary());
  const trend = ref<any[]>([]);
  const topUsers = ref<any[]>([]);
  const breakdown = ref<any[]>([]);
  const trendChartRef = ref<HTMLDivElement | null>(null);
  const trendChart = useECharts(trendChartRef as Ref<HTMLDivElement>);

  const summaryCards = computed(() => [
    {
      label: '调用总量',
      value: formatCloudResourceNumber(summary.value.requestCount),
      hint: `失败 ${formatCloudResourceNumber(summary.value.failureCount)} 次`,
      icon: CloudOutline,
      color: '#2080f0',
    },
    {
      label: '调用用户',
      value: formatCloudResourceNumber(summary.value.activeUserCount),
      hint: `配置验证 ${formatCloudResourceNumber(summary.value.validationCount)} 次`,
      icon: PeopleOutline,
      color: '#8a2be2',
    },
    {
      label: '成功率',
      value: `${cloudResourceSuccessRate(summary.value).toFixed(1)}%`,
      hint: `成功 ${formatCloudResourceNumber(summary.value.successCount)} 次`,
      icon: CheckmarkCircleOutline,
      color: '#18a058',
    },
    {
      label: '平均耗时',
      value: formatCloudResourceDuration(summary.value.avgDurationMs),
      hint: '按真实云端请求平均计算',
      icon: TimeOutline,
      color: '#f0a020',
    },
  ]);

  const breakdownColumns = [
    {
      title: '资源',
      key: 'resourceType',
      render: (row) => cloudResourceLabel(row.resourceType),
    },
    {
      title: '调用',
      key: 'requestCount',
      width: 90,
      render: (row) => formatCloudResourceNumber(row.requestCount),
    },
    {
      title: '耗时',
      key: 'avgDurationMs',
      width: 100,
      render: (row) => formatCloudResourceDuration(row.avgDurationMs),
    },
  ];

  const topUserColumns = [
    {
      title: '排名',
      key: 'rank',
      width: 70,
      render: (_row, index) => `#${index + 1}`,
    },
    {
      title: '用户',
      key: 'username',
      width: 190,
      render: (row) => row.nickname || row.username || `账号 ${row.accountId}`,
    },
    { title: '账号归属', key: 'tenantName', width: 160, render: (row) => row.tenantName || '-' },
    { title: '会员', key: 'vipLevel', width: 100, render: (row) => renderVip(row) },
    {
      title: '调用量',
      key: 'requestCount',
      width: 110,
      render: (row) => formatCloudResourceNumber(row.requestCount),
    },
    {
      title: '成功率',
      key: 'successRate',
      width: 100,
      render: (row) => `${cloudResourceSuccessRate(row).toFixed(1)}%`,
    },
    {
      title: '平均耗时',
      key: 'avgDurationMs',
      width: 120,
      render: (row) => formatCloudResourceDuration(row.avgDurationMs),
    },
    {
      title: '最后调用',
      key: 'lastCalledAt',
      width: 170,
      render: (row) => row.lastCalledAt || '-',
    },
  ];

  onMounted(loadDashboard);

  async function loadDashboard() {
    loading.value = true;
    try {
      const [startDate, endDate] = normalizeCloudResourceDateRange(dateRange.value);
      const res: any = await CloudResourceUsageDashboard({
        startDate,
        endDate,
        resourceType: resourceType.value,
      });
      summary.value = { ...emptySummary(), ...(res?.summary || {}) };
      trend.value = res?.trend || [];
      topUsers.value = res?.topUsers || [];
      breakdown.value = res?.breakdown || [];
      renderTrendChart();
    } finally {
      loading.value = false;
    }
  }

  function resetQuery() {
    dateRange.value = currentMonthRange();
    resourceType.value = '';
    loadDashboard();
  }

  function renderTrendChart() {
    trendChart.setOptions({
      tooltip: { trigger: 'axis' },
      legend: { data: ['调用量', '失败量', '平均耗时'] },
      grid: { left: 56, right: 58, top: 48, bottom: 36 },
      xAxis: {
        type: 'category',
        boundaryGap: true,
        data: trend.value.map((item) => String(item.usageDate || '').slice(5)),
      },
      yAxis: [
        { type: 'value', name: '次数', minInterval: 1 },
        { type: 'value', name: '耗时(ms)', min: 0 },
      ],
      series: [
        {
          name: '调用量',
          type: 'bar',
          barMaxWidth: 28,
          data: trend.value.map((item) => item.requestCount || 0),
          itemStyle: { color: '#2080f0' },
        },
        {
          name: '失败量',
          type: 'bar',
          barMaxWidth: 28,
          data: trend.value.map((item) => item.failureCount || 0),
          itemStyle: { color: '#d03050' },
        },
        {
          name: '平均耗时',
          type: 'line',
          yAxisIndex: 1,
          smooth: true,
          data: trend.value.map((item) => item.avgDurationMs || 0),
          itemStyle: { color: '#f0a020' },
        },
      ],
    });
  }

  function renderVip(row: any) {
    const enabled = Number(row.vipStatus) === 1 && Number(row.vipLevel) > 0;
    return h(
      NTag,
      { type: enabled ? 'success' : 'default', bordered: false },
      { default: () => (enabled ? `VIP ${row.vipLevel}` : '非会员') }
    );
  }
</script>

<style scoped>
  .toolbar {
    margin-top: 4px;
  }

  .date-range {
    width: 270px;
  }

  .resource-select {
    width: 160px;
  }

  .summary-card {
    min-height: 124px;
  }

  .summary-card__label {
    color: var(--n-text-color-3);
    font-size: 13px;
  }

  .summary-card__value {
    margin-top: 8px;
    font-size: 27px;
    font-weight: 600;
    line-height: 1.2;
  }

  .summary-card__hint {
    margin-top: 12px;
    color: var(--n-text-color-3);
    font-size: 12px;
  }

  .trend-chart {
    height: 330px;
  }
</style>
