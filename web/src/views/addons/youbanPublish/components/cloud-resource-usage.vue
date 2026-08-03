<template>
  <n-space vertical size="large">
    <n-alert type="info" :bordered="false">
      仅统计功能上线后实际发起的云端请求，不回溯历史数据；缓存命中不会重复计数。数据按用户、资源和日期聚合，可用于核算会员成本。
    </n-alert>

    <n-grid cols="1 s:2 m:4 l:4 xl:4 2xl:4" responsive="screen" :x-gap="12" :y-gap="12">
      <n-grid-item v-for="item in summaryCards" :key="item.label">
        <n-card size="small" :bordered="true" class="summary-card">
          <div class="summary-card__label">{{ item.label }}</div>
          <div class="summary-card__value">{{ item.value }}</div>
          <div class="summary-card__hint">{{ item.hint }}</div>
        </n-card>
      </n-grid-item>
    </n-grid>

    <n-space class="toolbar" align="center">
      <n-date-picker
        v-model:value="dateRange"
        type="daterange"
        clearable
        class="date-range"
        :shortcuts="dateShortcuts"
      />
      <n-select
        v-model:value="query.resourceType"
        :options="resourceOptions"
        clearable
        placeholder="资源类型"
        class="resource-select"
      />
      <n-input
        v-model:value="query.keyword"
        clearable
        placeholder="账号 / 昵称 / 账号归属"
        class="keyword-input"
        @keyup.enter="search"
      />
      <n-button type="primary" @click="search">查询</n-button>
      <n-button quaternary @click="resetQuery">重置</n-button>
    </n-space>

    <n-data-table
      :columns="columns"
      :data="rows"
      :loading="loading"
      :pagination="pagination"
      :row-key="(row) => `${row.accountId}-${row.tenantId}`"
      :scroll-x="1420"
      size="small"
      remote
    />
  </n-space>
</template>

<script lang="ts" setup>
  import { computed, h, onMounted, reactive, ref } from 'vue';
  import { NTag } from 'naive-ui';
  import { CloudResourceUsageList } from '@/api/addons/youbanPublish';

  interface UsageSummary {
    activeUserCount: number;
    avgDurationMs: number;
    backgroundMattingCount: number;
    faceDetectionCount: number;
    failureCount: number;
    requestCount: number;
    successCount: number;
    validationCount: number;
  }

  const emptySummary = (): UsageSummary => ({
    activeUserCount: 0,
    avgDurationMs: 0,
    backgroundMattingCount: 0,
    faceDetectionCount: 0,
    failureCount: 0,
    requestCount: 0,
    successCount: 0,
    validationCount: 0,
  });

  const rows = ref<any[]>([]);
  const loading = ref(false);
  const summary = reactive<UsageSummary>(emptySummary());
  const query = reactive({ keyword: '', resourceType: '' });
  const dateRange = ref<[number, number] | null>(currentMonthRange());

  const resourceOptions = [
    { label: '云端抠图', value: 'background_matting' },
    { label: '人脸检测', value: 'face_detection' },
  ];

  const dateShortcuts = {
    本月: currentMonthRange,
    最近7天: () => recentDaysRange(7),
    最近30天: () => recentDaysRange(30),
  };

  const summaryCards = computed(() => [
    {
      label: '调用总量',
      value: formatNumber(summary.requestCount),
      hint: `成功 ${formatNumber(summary.successCount)} · 失败 ${formatNumber(summary.failureCount)}`,
    },
    {
      label: '调用用户',
      value: formatNumber(summary.activeUserCount),
      hint: '不包含后台配置验证',
    },
    {
      label: '云端抠图',
      value: formatNumber(summary.backgroundMattingCount),
      hint: `配置验证 ${formatNumber(summary.validationCount)} 次`,
    },
    {
      label: '平均耗时',
      value: formatDuration(summary.avgDurationMs),
      hint: `人脸检测 ${formatNumber(summary.faceDetectionCount)} 次`,
    },
  ]);

  const columns = [
    {
      title: '用户',
      key: 'username',
      width: 190,
      render: (row) =>
        h('div', { class: 'account-cell' }, [
          h('div', { class: 'account-cell__name' }, row.nickname || row.username || '-'),
          h(
            'div',
            { class: 'account-cell__username' },
            row.accountId ? `@${row.username || row.accountId}` : '系统配置验证'
          ),
        ]),
    },
    { title: '账号归属', key: 'tenantName', width: 160, render: (row) => row.tenantName || '-' },
    {
      title: '会员',
      key: 'vipLevel',
      width: 100,
      render: (row) => renderVip(row),
    },
    {
      title: '会员到期',
      key: 'vipExpiredAt',
      width: 170,
      render: (row) => row.vipExpiredAt || '-',
    },
    {
      title: '调用总量',
      key: 'requestCount',
      width: 100,
      render: (row) => formatNumber(row.requestCount),
    },
    {
      title: '成功率',
      key: 'successRate',
      width: 100,
      render: (row) => renderSuccessRate(row),
    },
    {
      title: '云端抠图',
      key: 'backgroundMattingCount',
      width: 110,
      render: (row) => formatNumber(row.backgroundMattingCount),
    },
    {
      title: '人脸检测',
      key: 'faceDetectionCount',
      width: 110,
      render: (row) => formatNumber(row.faceDetectionCount),
    },
    {
      title: '配置验证',
      key: 'validationCount',
      width: 100,
      render: (row) => formatNumber(row.validationCount),
    },
    {
      title: '失败',
      key: 'failureCount',
      width: 90,
      render: (row) => formatNumber(row.failureCount),
    },
    {
      title: '平均耗时',
      key: 'avgDurationMs',
      width: 110,
      render: (row) => formatDuration(row.avgDurationMs),
    },
    {
      title: '首次调用',
      key: 'firstUsageDate',
      width: 120,
      render: (row) => row.firstUsageDate || '-',
    },
    {
      title: '最后调用',
      key: 'lastCalledAt',
      width: 170,
      render: (row) => row.lastCalledAt || '-',
    },
  ];

  const pagination: any = reactive({
    page: 1,
    pageSize: 20,
    itemCount: 0,
    showSizePicker: true,
    pageSizes: [10, 20, 50, 100],
    onUpdatePage: (page) => {
      pagination.page = page;
      loadUsage();
    },
    onUpdatePageSize: (pageSize) => {
      pagination.pageSize = pageSize;
      pagination.page = 1;
      loadUsage();
    },
  });

  onMounted(loadUsage);

  async function loadUsage() {
    loading.value = true;
    try {
      const [startDate, endDate] = normalizedDateRange();
      const res: any = await CloudResourceUsageList({
        ...query,
        startDate,
        endDate,
        page: pagination.page,
        pageSize: pagination.pageSize,
      });
      rows.value = res?.list || [];
      pagination.itemCount = res?.totalCount || 0;
      Object.assign(summary, emptySummary(), res?.summary || {});
    } finally {
      loading.value = false;
    }
  }

  function search() {
    pagination.page = 1;
    loadUsage();
  }

  function resetQuery() {
    Object.assign(query, { keyword: '', resourceType: '' });
    dateRange.value = currentMonthRange();
    search();
  }

  function normalizedDateRange() {
    const range = dateRange.value || currentMonthRange();
    return [formatDate(range[0]), formatDate(range[1])];
  }

  function currentMonthRange(): [number, number] {
    const now = new Date();
    return [new Date(now.getFullYear(), now.getMonth(), 1).getTime(), endOfDay(now).getTime()];
  }

  function recentDaysRange(days: number): [number, number] {
    const end = endOfDay(new Date());
    const start = new Date(end);
    start.setDate(start.getDate() - Math.max(days - 1, 0));
    start.setHours(0, 0, 0, 0);
    return [start.getTime(), end.getTime()];
  }

  function endOfDay(date: Date) {
    const value = new Date(date);
    value.setHours(23, 59, 59, 999);
    return value;
  }

  function formatDate(timestamp: number) {
    const date = new Date(timestamp);
    const month = String(date.getMonth() + 1).padStart(2, '0');
    const day = String(date.getDate()).padStart(2, '0');
    return `${date.getFullYear()}-${month}-${day}`;
  }

  function formatNumber(value: number) {
    return Number(value || 0).toLocaleString('zh-CN');
  }

  function formatDuration(value: number) {
    const duration = Number(value || 0);
    if (duration < 1000) return `${duration} ms`;
    return `${(duration / 1000).toFixed(duration >= 10000 ? 1 : 2)} s`;
  }

  function renderSuccessRate(row: any) {
    const requestCount = Number(row.requestCount || 0);
    const rate = requestCount > 0 ? (Number(row.successCount || 0) / requestCount) * 100 : 0;
    const type = rate >= 98 ? 'success' : rate >= 90 ? 'warning' : 'error';
    return h(NTag, { type, bordered: false }, { default: () => `${rate.toFixed(1)}%` });
  }

  function renderVip(row: any) {
    if (!row.accountId) return '-';
    const enabled = Number(row.vipStatus) === 1 && Number(row.vipLevel) > 0;
    return h(
      NTag,
      { type: enabled ? 'success' : 'default', bordered: false },
      { default: () => (enabled ? `VIP ${row.vipLevel}` : '非会员') }
    );
  }
</script>

<style scoped>
  .summary-card {
    min-height: 112px;
  }

  .summary-card__label {
    color: var(--n-text-color-3);
    font-size: 13px;
  }

  .summary-card__value {
    margin-top: 8px;
    font-size: 26px;
    font-weight: 600;
    line-height: 1.2;
  }

  .summary-card__hint {
    margin-top: 8px;
    color: var(--n-text-color-3);
    font-size: 12px;
  }

  .toolbar {
    margin-top: 4px;
  }

  .date-range {
    width: 270px;
  }

  .resource-select {
    width: 160px;
  }

  .keyword-input {
    width: 260px;
  }

  :deep(.account-cell__name) {
    font-weight: 500;
  }

  :deep(.account-cell__username) {
    margin-top: 2px;
    color: var(--n-text-color-3);
    font-size: 12px;
  }
</style>
