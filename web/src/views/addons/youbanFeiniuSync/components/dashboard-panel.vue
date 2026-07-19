<template>
  <n-spin :show="loading">
    <n-space vertical size="large" class="dashboard-panel">
      <n-space justify="space-between" align="center">
        <div>
          <div class="dashboard-title">监控大盘</div>
          <div class="dashboard-sub">更新时间：{{ summary.updatedAt || '-' }}</div>
        </div>
        <n-space>
          <n-select
            v-model:value="query.configId"
            :options="configOptions"
            clearable
            filterable
            placeholder="全部配置"
            class="config-select"
          />
          <n-date-picker v-model:value="dateRange" type="daterange" clearable />
          <n-button type="primary" @click="loadDashboard">刷新</n-button>
        </n-space>
      </n-space>

      <n-grid cols="1 s:2 m:4 l:4 xl:4 2xl:4" responsive="screen" :x-gap="12" :y-gap="12">
        <n-grid-item v-for="item in stats" :key="item.label">
          <n-card size="small" :bordered="true">
            <n-statistic :label="item.label" :value="item.value"
              ><template v-if="item.suffix" #suffix>{{ item.suffix }}</template></n-statistic
            >
          </n-card>
        </n-grid-item>
      </n-grid>

      <n-grid cols="1 l:3" responsive="screen" :x-gap="12" :y-gap="12">
        <n-grid-item span="1 l:2"
          ><n-card title="每日采集量趋势" :bordered="true"
            ><div ref="trendRef" class="chart"></div></n-card
        ></n-grid-item>
        <n-grid-item
          ><n-card title="同步结果分布" :bordered="true"
            ><div ref="pieRef" class="chart"></div></n-card
        ></n-grid-item>
      </n-grid>

      <n-grid cols="1 l:2" responsive="screen" :x-gap="12" :y-gap="12">
        <n-grid-item
          ><n-card title="频道采集排行" :bordered="true"
            ><div ref="rankRef" class="chart"></div></n-card
        ></n-grid-item>
        <n-grid-item>
          <n-card title="最近运行" :bordered="true">
            <n-data-table
              :columns="runColumns"
              :data="recentRuns"
              :row-key="(row) => row.id"
              size="small"
            />
          </n-card>
        </n-grid-item>
      </n-grid>
    </n-space>
  </n-spin>
</template>

<script setup lang="ts">
  import { computed, h, nextTick, onMounted, onUnmounted, reactive, ref } from 'vue';
  import { NTag } from 'naive-ui';
  import useEcharts from '@/hooks/useEcharts';
  import {
    ConfigList,
    DashboardChannelRank,
    DashboardRecentRuns,
    DashboardSummary,
    DashboardTrend,
  } from '@/api/addons/youbanFeiniuSync';

  const loading = ref(false);
  const configs = ref<any[]>([]);
  const trend = ref<any[]>([]);
  const channelRank = ref<any[]>([]);
  const recentRuns = ref<any[]>([]);
  const dateRange = ref<[number, number] | null>(defaultRange());
  const trendRef = ref<HTMLDivElement | null>(null);
  const pieRef = ref<HTMLDivElement | null>(null);
  const rankRef = ref<HTMLDivElement | null>(null);
  let trendChart: ReturnType<typeof useEcharts> | null = null;
  let pieChart: ReturnType<typeof useEcharts> | null = null;
  let rankChart: ReturnType<typeof useEcharts> | null = null;
  const query = reactive({ configId: null as number | null });
  const summary = reactive<any>({});
  const configOptions = computed(() =>
    configs.value.map((item) => ({ label: item.name, value: item.id }))
  );
  const stats = computed(() => [
    { label: '采集总量', value: summary.totalCount || 0 },
    { label: '新增', value: summary.createdCount || 0 },
    { label: '更新', value: summary.updatedCount || 0 },
    { label: '跳过', value: summary.skippedCount || 0 },
    { label: '失败', value: summary.failedCount || 0 },
    { label: '成功率', value: Number(summary.successRate || 0).toFixed(2), suffix: '%' },
    { label: '频道映射', value: summary.channelCount || 0 },
    { label: '已同步资料', value: summary.profileCount || 0 },
  ]);
  const runColumns = [
    { title: 'ID', key: 'id', width: 80 },
    { title: '类型', key: 'runType', width: 80 },
    {
      title: '状态',
      key: 'status',
      width: 90,
      render: (row) =>
        h(NTag, { type: tagType(row.status), bordered: false }, { default: () => row.status }),
    },
    { title: '总数', key: 'totalCount', width: 70 },
    { title: '失败', key: 'failedCount', width: 70 },
    { title: '开始时间', key: 'startedAt', width: 170 },
  ];

  onMounted(async () => {
    await loadConfigs();
    await loadDashboard();
  });
  onUnmounted(() => {
    trendChart?.dispose();
    pieChart?.dispose();
    rankChart?.dispose();
  });

  async function loadConfigs() {
    const res = await ConfigList({ page: 1, pageSize: 100 });
    configs.value = res.list || [];
  }
  function params() {
    return { configId: query.configId, ...dateParams() };
  }
  function dateParams() {
    if (!dateRange.value) return {};
    return { startDate: formatDate(dateRange.value[0]), endDate: formatDate(dateRange.value[1]) };
  }
  async function loadDashboard() {
    loading.value = true;
    try {
      const [summaryRes, trendRes, rankRes, runsRes] = await Promise.all([
        DashboardSummary(params()),
        DashboardTrend(params()),
        DashboardChannelRank({ ...params(), limit: 10 }),
        DashboardRecentRuns({ configId: query.configId, limit: 8 }),
      ]);
      Object.assign(summary, summaryRes || {});
      trend.value = trendRes.list || [];
      channelRank.value = rankRes.list || [];
      recentRuns.value = runsRes.list || [];
      await nextTick();
      renderCharts();
    } finally {
      loading.value = false;
    }
  }
  function renderCharts() {
    if (trendRef.value) {
      trendChart = useEcharts(trendRef.value);
      trendChart.setOption({
        tooltip: { trigger: 'axis' },
        legend: { bottom: 0 },
        grid: { left: 30, right: 20, top: 34, bottom: 36, containLabel: true },
        xAxis: { type: 'category', data: trend.value.map((item) => String(item.date).slice(5)) },
        yAxis: { type: 'value' },
        series: ['createdCount', 'updatedCount', 'skippedCount', 'failedCount'].map(
          (key, index) => ({
            name: ['新增', '更新', '跳过', '失败'][index],
            type: index === 0 ? 'bar' : 'line',
            smooth: true,
            data: trend.value.map((item) => item[key] || 0),
          })
        ),
      });
    }
    if (pieRef.value) {
      pieChart = useEcharts(pieRef.value);
      pieChart.setOption({
        tooltip: { trigger: 'item' },
        legend: { bottom: 0 },
        series: [
          {
            type: 'pie',
            radius: ['45%', '70%'],
            data: [
              { name: '新增', value: summary.createdCount || 0 },
              { name: '更新', value: summary.updatedCount || 0 },
              { name: '跳过', value: summary.skippedCount || 0 },
              { name: '失败', value: summary.failedCount || 0 },
            ],
          },
        ],
      });
    }
    if (rankRef.value) {
      rankChart = useEcharts(rankRef.value);
      const data = [...channelRank.value].reverse();
      rankChart.setOption({
        tooltip: { trigger: 'axis' },
        grid: { left: 20, right: 20, top: 20, bottom: 20, containLabel: true },
        xAxis: { type: 'value' },
        yAxis: {
          type: 'category',
          data: data.map((item) => item.feiniuChannelTitle || item.feiniuChannelId),
        },
        series: [{ name: '采集量', type: 'bar', data: data.map((item) => item.totalCount || 0) }],
      });
    }
  }
  function defaultRange(): [number, number] {
    const end = Date.now();
    const start = end - 6 * 24 * 60 * 60 * 1000;
    return [start, end];
  }
  function formatDate(value: number) {
    const d = new Date(value);
    return `${d.getFullYear()}-${`${d.getMonth() + 1}`.padStart(2, '0')}-${`${d.getDate()}`.padStart(2, '0')}`;
  }
  function tagType(status: string) {
    if (status === 'success') return 'success';
    if (status === 'running') return 'info';
    return 'error';
  }
</script>

<style scoped>
  .dashboard-panel {
    width: 100%;
  }
  .dashboard-title {
    font-size: 18px;
    font-weight: 600;
  }
  .dashboard-sub {
    margin-top: 4px;
    color: var(--text-color-3);
    font-size: 12px;
  }
  .config-select {
    width: 220px;
  }
  .chart {
    height: 320px;
  }
</style>
