<template>
  <n-spin :show="loading">
    <n-space vertical size="large" class="dashboard-panel">
      <n-space justify="space-between" align="center" wrap>
        <div>
          <div class="dashboard-title-main">发布监控大盘</div>
          <div class="dashboard-title-sub">
            统计周期：{{ dashboard.startDate || '-' }} 至 {{ dashboard.endDate || '-' }} ·
            更新时间：{{ dashboard.updatedAt || '-' }}
          </div>
        </div>
        <n-space align="center" wrap>
          <n-select
            v-model:value="rangeMode"
            :options="rangeOptions"
            class="range-select"
            @update:value="handleRangeModeChange"
          />
          <n-date-picker
            v-if="rangeMode === 'custom'"
            v-model:value="customRange"
            type="daterange"
            :clearable="false"
            class="date-range"
          />
          <n-button type="primary" @click="loadDashboard">刷新</n-button>
        </n-space>
      </n-space>

      <n-grid cols="1 s:2 m:4 l:4 xl:4" responsive="screen" :x-gap="12" :y-gap="12">
        <n-grid-item v-for="item in dashboard.stats" :key="item.key">
          <n-card size="small" :bordered="true" class="stat-card">
            <n-statistic :label="item.title" :value="item.value">
              <template v-if="item.suffix" #suffix>{{ item.suffix }}</template>
            </n-statistic>
          </n-card>
        </n-grid-item>
      </n-grid>

      <n-grid cols="1 l:2" responsive="screen" :x-gap="12" :y-gap="12">
        <n-grid-item>
          <n-card title="任务趋势" :bordered="true">
            <div ref="taskTrendChartRef" class="chart"></div>
          </n-card>
        </n-grid-item>
        <n-grid-item>
          <n-card title="资料上架趋势" :bordered="true">
            <div ref="profileTrendChartRef" class="chart"></div>
          </n-card>
        </n-grid-item>
        <n-grid-item>
          <n-card title="发布失败 Top 10" :bordered="true">
            <div
              v-if="dashboard.publishFailureTop.length"
              ref="failureChartRef"
              class="rank-chart"
            ></div>
            <n-empty v-else description="当前周期暂无发布失败" class="empty-block" />
          </n-card>
        </n-grid-item>
        <n-grid-item>
          <n-card title="资料发布 Top 10" :bordered="true">
            <div
              v-if="dashboard.profilePublishTop.length"
              ref="profileTopChartRef"
              class="rank-chart"
            ></div>
            <n-empty v-else description="当前周期暂无资料发布" class="empty-block" />
          </n-card>
        </n-grid-item>
      </n-grid>

      <n-grid cols="1 l:2" responsive="screen" :x-gap="12" :y-gap="12">
        <n-grid-item>
          <n-card title="系统健康" :bordered="true">
            <n-space vertical>
              <div v-for="item in dashboard.health" :key="item.key" class="health-item">
                <div>
                  <div class="health-title">{{ item.title }}</div>
                  <div class="health-message">{{ item.message }}</div>
                </div>
                <n-tag :type="tagType(item.status)">{{ item.value }}</n-tag>
              </div>
            </n-space>
          </n-card>
        </n-grid-item>
        <n-grid-item>
          <n-card title="待处理事项" :bordered="true">
            <n-empty v-if="!dashboard.todos.length" description="暂无待处理事项" />
            <n-list v-else>
              <n-list-item v-for="item in dashboard.todos" :key="item.key">
                <n-thing :title="item.title" :description="item.desc">
                  <template #header-extra>
                    <n-tag :type="tagType(item.status)">{{ statusText(item.status) }}</n-tag>
                  </template>
                  <template #footer>{{ item.updatedAt }}</template>
                </n-thing>
              </n-list-item>
            </n-list>
          </n-card>
        </n-grid-item>
      </n-grid>
    </n-space>
  </n-spin>
</template>

<script lang="ts" setup>
  import { nextTick, onMounted, ref } from 'vue';
  import { Dashboard } from '@/api/addons/youbanPublish';
  import { useECharts } from '@/hooks/web/useECharts';

  interface RankItem {
    key: string;
    name: string;
    value: number;
  }

  interface DashboardData {
    stats: any[];
    taskTrend: any[];
    profileTrend: any[];
    health: any[];
    todos: any[];
    publishFailureTop: RankItem[];
    profilePublishTop: RankItem[];
    startDate: string;
    endDate: string;
    updatedAt: string;
  }

  const loading = ref(false);
  const rangeMode = ref<number | 'custom'>(7);
  const customRange = ref<[number, number]>(defaultDateRange(7));
  const dashboard = ref<DashboardData>(emptyDashboard());
  const taskTrendChartRef = ref<HTMLDivElement>();
  const profileTrendChartRef = ref<HTMLDivElement>();
  const failureChartRef = ref<HTMLDivElement>();
  const profileTopChartRef = ref<HTMLDivElement>();
  const taskTrendChart = useECharts(taskTrendChartRef);
  const profileTrendChart = useECharts(profileTrendChartRef);
  const failureChart = useECharts(failureChartRef);
  const profileTopChart = useECharts(profileTopChartRef);

  const rangeOptions = [
    { label: '近 7 天', value: 7 },
    { label: '近 30 天', value: 30 },
    { label: '近 90 天', value: 90 },
    { label: '自定义', value: 'custom' },
  ];

  onMounted(loadDashboard);

  async function loadDashboard() {
    loading.value = true;
    try {
      const res: any = await Dashboard(buildRangeParams());
      dashboard.value = {
        ...emptyDashboard(),
        ...res,
        stats: res?.stats || [],
        taskTrend: res?.taskTrend || [],
        profileTrend: res?.profileTrend || [],
        health: res?.health || [],
        todos: res?.todos || [],
        publishFailureTop: res?.publishFailureTop || [],
        profilePublishTop: res?.profilePublishTop || [],
      };
      await nextTick();
      renderCharts();
    } finally {
      loading.value = false;
    }
  }

  function handleRangeModeChange(value: number | 'custom') {
    if (value !== 'custom') {
      customRange.value = defaultDateRange(value);
      loadDashboard();
    }
  }

  function buildRangeParams() {
    if (rangeMode.value !== 'custom') return { days: rangeMode.value };
    return {
      startDate: formatDate(customRange.value[0]),
      endDate: formatDate(customRange.value[1]),
    };
  }

  function renderCharts() {
    const taskTrend = dashboard.value.taskTrend;
    taskTrendChart.setOptions({
      color: ['#5b8ff9', '#18a058', '#d03050'],
      grid: chartGrid(),
      legend: { data: ['新增任务', '发布成功', '发布失败'], bottom: 0 },
      tooltip: { trigger: 'axis' },
      xAxis: { type: 'category', data: taskTrend.map((item) => shortDate(item.date)) },
      yAxis: valueAxis(),
      series: [
        {
          name: '新增任务',
          type: 'bar',
          barMaxWidth: 18,
          data: taskTrend.map((item) => item.created || 0),
        },
        {
          name: '发布成功',
          type: 'line',
          smooth: true,
          data: taskTrend.map((item) => item.published || 0),
        },
        {
          name: '发布失败',
          type: 'line',
          smooth: true,
          data: taskTrend.map((item) => item.failed || 0),
        },
      ],
    });

    const profileTrend = dashboard.value.profileTrend;
    profileTrendChart.setOptions({
      color: ['#5b8ff9', '#18a058', '#f0a020'],
      grid: chartGrid(),
      legend: { data: ['新增资料', '上架资料', '下架资料'], bottom: 0 },
      tooltip: { trigger: 'axis' },
      xAxis: { type: 'category', data: profileTrend.map((item) => shortDate(item.date)) },
      yAxis: valueAxis(),
      series: [
        {
          name: '新增资料',
          type: 'bar',
          barMaxWidth: 18,
          data: profileTrend.map((item) => item.created || 0),
        },
        {
          name: '上架资料',
          type: 'line',
          smooth: true,
          data: profileTrend.map((item) => item.published || 0),
        },
        {
          name: '下架资料',
          type: 'line',
          smooth: true,
          data: profileTrend.map((item) => item.down || 0),
        },
      ],
    });

    renderRankChart(failureChart, dashboard.value.publishFailureTop, '#d03050');
    renderRankChart(profileTopChart, dashboard.value.profilePublishTop, '#18a058');
  }

  function renderRankChart(chart: ReturnType<typeof useECharts>, items: RankItem[], color: string) {
    const rows = [...items].reverse();
    chart.setOptions({
      color: [color],
      grid: { left: 12, right: 32, top: 8, bottom: 8, containLabel: true },
      tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
      xAxis: valueAxis(),
      yAxis: {
        type: 'category',
        data: rows.map((item) => item.name),
        axisLabel: { width: 220, overflow: 'truncate' },
      },
      series: [
        {
          type: 'bar',
          barMaxWidth: 16,
          data: rows.map((item) => item.value),
          label: { show: true, position: 'right' },
        },
      ],
    });
  }

  function chartGrid() {
    return { left: 20, right: 20, top: 28, bottom: 36, containLabel: true };
  }

  function valueAxis() {
    return { type: 'value', minInterval: 1, splitLine: { lineStyle: { type: 'dashed' } } };
  }

  function emptyDashboard(): DashboardData {
    return {
      stats: [],
      taskTrend: [],
      profileTrend: [],
      health: [],
      todos: [],
      publishFailureTop: [],
      profilePublishTop: [],
      startDate: '',
      endDate: '',
      updatedAt: '',
    };
  }

  function defaultDateRange(days: number): [number, number] {
    const end = new Date();
    const start = new Date(end);
    start.setDate(end.getDate() - days + 1);
    return [start.getTime(), end.getTime()];
  }

  function formatDate(timestamp: number) {
    const date = new Date(timestamp);
    const month = String(date.getMonth() + 1).padStart(2, '0');
    const day = String(date.getDate()).padStart(2, '0');
    return `${date.getFullYear()}-${month}-${day}`;
  }

  function shortDate(value: string) {
    return String(value || '').slice(5);
  }

  function tagType(status: string) {
    if (status === 'success' || status === 'published') return 'success';
    if (status === 'error' || status === 'failed' || status === 'expired') return 'error';
    return 'warning';
  }

  function statusText(status: string) {
    const map: Record<string, string> = {
      failed: '失败',
      pending: '待发布',
      publishing: '发布中',
      success: '正常',
      warning: '警告',
      error: '异常',
    };
    return map[status] || status;
  }
</script>

<style scoped>
  .dashboard-panel {
    width: 100%;
  }

  .dashboard-title-main {
    font-size: 20px;
    font-weight: 600;
  }

  .dashboard-title-sub,
  .health-message {
    margin-top: 4px;
    color: var(--text-color-3);
    font-size: 12px;
  }

  .range-select {
    width: 120px;
  }

  .date-range {
    width: 260px;
  }

  .stat-card {
    min-height: 92px;
  }

  .chart,
  .rank-chart {
    height: 340px;
  }

  .empty-block {
    height: 340px;
  }

  .health-item {
    display: flex;
    gap: 12px;
    align-items: center;
    justify-content: space-between;
  }

  .health-title {
    font-weight: 500;
  }
</style>
