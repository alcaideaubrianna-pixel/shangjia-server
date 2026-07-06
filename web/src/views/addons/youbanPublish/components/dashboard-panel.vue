<template>
  <n-spin :show="loading">
    <n-space vertical size="large" class="dashboard-panel">
      <n-space justify="space-between" align="center">
        <div class="dashboard-title">
          <div class="dashboard-title-main">工作台</div>
          <div class="dashboard-title-sub">更新时间：{{ dashboard.updatedAt || '-' }}</div>
        </div>
        <n-space>
          <n-select v-model:value="days" :options="rangeOptions" class="range-select" />
          <n-button type="primary" @click="loadDashboard">刷新</n-button>
        </n-space>
      </n-space>

      <n-grid cols="1 s:2 m:4 l:4 xl:4 2xl:4" responsive="screen" :x-gap="12" :y-gap="12">
        <n-grid-item v-for="item in dashboard.stats" :key="item.key">
          <n-card size="small" :bordered="true">
            <n-statistic :label="item.title" :value="item.value">
              <template v-if="item.suffix" #suffix>{{ item.suffix }}</template>
            </n-statistic>
          </n-card>
        </n-grid-item>
      </n-grid>

      <n-grid cols="1 l:3" responsive="screen" :x-gap="12" :y-gap="12">
        <n-grid-item span="1 l:2">
          <n-card title="任务趋势" :bordered="true">
            <div ref="trendChartRef" class="chart"></div>
          </n-card>
        </n-grid-item>
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
      </n-grid>

      <n-grid cols="1 l:3" responsive="screen" :x-gap="12" :y-gap="12">
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
        <n-grid-item>
          <n-card title="账号归属排行" :bordered="true">
            <n-list>
              <n-list-item v-for="item in dashboard.tenantRank" :key="item.key">
                <div class="rank-row">
                  <span>{{ item.name }}</span>
                  <n-tag type="success">{{ item.value }}</n-tag>
                </div>
              </n-list-item>
            </n-list>
            <n-empty v-if="!dashboard.tenantRank.length" description="暂无排行数据" />
          </n-card>
        </n-grid-item>
        <n-grid-item>
          <n-card title="失败原因排行" :bordered="true">
            <n-list>
              <n-list-item v-for="item in dashboard.errorRank" :key="item.key">
                <div class="rank-row">
                  <span>{{ item.name }}</span>
                  <n-tag type="error">{{ item.value }}</n-tag>
                </div>
              </n-list-item>
            </n-list>
            <n-empty v-if="!dashboard.errorRank.length" description="暂无失败记录" />
          </n-card>
        </n-grid-item>
      </n-grid>
    </n-space>
  </n-spin>
</template>

<script lang="ts" setup>
  import { nextTick, onMounted, onUnmounted, reactive, ref } from 'vue';
  import useEcharts from '@/hooks/useEcharts';
  import { Dashboard } from '@/api/addons/youbanPublish';

  const loading = ref(false);
  const days = ref(7);
  const trendChartRef = ref<HTMLDivElement | null>(null);
  let trendChart: ReturnType<typeof useEcharts> | null = null;

  const rangeOptions = [
    { label: '近 7 天', value: 7 },
    { label: '近 30 天', value: 30 },
    { label: '近 90 天', value: 90 },
  ];

  const dashboard = reactive({
    stats: [] as any[],
    taskTrend: [] as any[],
    health: [] as any[],
    todos: [] as any[],
    tenantRank: [] as any[],
    errorRank: [] as any[],
    updatedAt: '',
  });

  onMounted(loadDashboard);

  onUnmounted(() => {
    trendChart?.dispose();
  });

  async function loadDashboard() {
    loading.value = true;
    try {
      const res: any = await Dashboard({ days: days.value });
      Object.assign(dashboard, {
        stats: res?.stats || [],
        taskTrend: res?.taskTrend || [],
        health: res?.health || [],
        todos: res?.todos || [],
        tenantRank: res?.tenantRank || [],
        errorRank: res?.errorRank || [],
        updatedAt: res?.updatedAt || '',
      });
      await nextTick();
      renderTrend();
    } finally {
      loading.value = false;
    }
  }

  function renderTrend() {
    if (!trendChartRef.value) return;
    trendChart = useEcharts(trendChartRef.value);
    trendChart.setOption({
      color: ['#3366ff', '#18a058', '#d03050'],
      grid: { left: 24, right: 20, top: 36, bottom: 28, containLabel: true },
      legend: { data: ['新增任务', '发布成功', '发布失败'], bottom: 0 },
      tooltip: { trigger: 'axis' },
      xAxis: {
        type: 'category',
        data: dashboard.taskTrend.map((item) => String(item.date).slice(5)),
      },
      yAxis: { type: 'value', splitLine: { lineStyle: { type: 'dashed' } } },
      series: [
        {
          name: '新增任务',
          type: 'bar',
          barMaxWidth: 18,
          data: dashboard.taskTrend.map((item) => item.created || 0),
        },
        {
          name: '发布成功',
          type: 'line',
          smooth: true,
          data: dashboard.taskTrend.map((item) => item.published || 0),
        },
        {
          name: '发布失败',
          type: 'line',
          smooth: true,
          data: dashboard.taskTrend.map((item) => item.failed || 0),
        },
      ],
    });
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
    font-size: 18px;
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

  .chart {
    height: 320px;
  }

  .health-item,
  .rank-row {
    display: flex;
    gap: 12px;
    align-items: center;
    justify-content: space-between;
  }

  .health-title {
    font-weight: 500;
  }
</style>
