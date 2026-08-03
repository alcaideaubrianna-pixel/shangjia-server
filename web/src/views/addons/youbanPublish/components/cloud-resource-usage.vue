<template>
  <n-space vertical size="large">
    <n-alert type="info" :bordered="false">
      按用户查询云资源调用明细。数据来自每日聚合统计表，不读取原始请求日志。
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
        v-model:value="query.resourceType"
        :options="cloudResourceOptions"
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
      :scroll-x="1600"
      size="small"
      remote
    />
  </n-space>
</template>

<script lang="ts" setup>
  import { h, onMounted, reactive, ref } from 'vue';
  import { NTag } from 'naive-ui';
  import { CloudResourceUsageList } from '@/api/addons/youbanPublish';
  import {
    cloudResourceDateShortcuts,
    cloudResourceOptions,
    cloudResourceSuccessRate,
    currentMonthRange,
    formatCloudResourceDuration,
    formatCloudResourceNumber,
    normalizeCloudResourceDateRange,
  } from './cloud-resource-usage-shared';

  const rows = ref<any[]>([]);
  const loading = ref(false);
  const query = reactive({ keyword: '', resourceType: '' });
  const dateRange = ref<[number, number] | null>(currentMonthRange());

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
    { title: '会员', key: 'vipLevel', width: 100, render: (row) => renderVip(row) },
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
      render: (row) => formatCloudResourceNumber(row.requestCount),
    },
    { title: '成功率', key: 'successRate', width: 100, render: (row) => renderSuccessRate(row) },
    {
      title: '云端抠图',
      key: 'backgroundMattingCount',
      width: 110,
      render: (row) => formatCloudResourceNumber(row.backgroundMattingCount),
    },
    {
      title: '人脸检测',
      key: 'faceDetectionCount',
      width: 110,
      render: (row) => formatCloudResourceNumber(row.faceDetectionCount),
    },
    {
      title: '配置验证',
      key: 'validationCount',
      width: 100,
      render: (row) => formatCloudResourceNumber(row.validationCount),
    },
    {
      title: '失败',
      key: 'failureCount',
      width: 90,
      render: (row) => formatCloudResourceNumber(row.failureCount),
    },
    {
      title: '平均耗时',
      key: 'avgDurationMs',
      width: 110,
      render: (row) => formatCloudResourceDuration(row.avgDurationMs),
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
      const [startDate, endDate] = normalizeCloudResourceDateRange(dateRange.value);
      const res: any = await CloudResourceUsageList({
        ...query,
        startDate,
        endDate,
        page: pagination.page,
        pageSize: pagination.pageSize,
      });
      rows.value = res?.list || [];
      pagination.itemCount = res?.totalCount || 0;
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

  function renderSuccessRate(row: any) {
    const rate = cloudResourceSuccessRate(row);
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
