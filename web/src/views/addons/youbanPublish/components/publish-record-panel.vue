<template>
  <n-space vertical>
    <n-alert type="info" :bordered="false">
      发布记录用于排查任意账号的 TG 推送任务、账号消息推送、采集推送和失败重试原因。
    </n-alert>

    <n-space class="toolbar" align="center">
      <n-select
        v-model:value="query.accountId"
        :options="accountOptions"
        clearable
        filterable
        placeholder="账号"
        class="account-select"
      />
      <n-select
        v-model:value="query.action"
        :options="actionOptions"
        clearable
        placeholder="动作"
        class="status-select"
      />
      <n-select
        v-model:value="query.status"
        :options="statusOptions"
        clearable
        placeholder="结果"
        class="status-select"
      />
      <n-input
        v-model:value="query.keyword"
        clearable
        placeholder="资料 / 频道 / Bot / 详情"
        class="keyword-input"
        @keyup.enter="loadRecords"
      />
      <n-button @click="loadRecords">查询</n-button>
      <n-button quaternary @click="resetQuery">重置</n-button>
    </n-space>

    <n-data-table
      :columns="columns"
      :data="records"
      :loading="loading"
      :pagination="pagination"
      :row-key="(row) => row.id"
      :scroll-x="1680"
      size="small"
      remote
    />
  </n-space>
</template>

<script lang="ts" setup>
  import { computed, h, onMounted, reactive, ref } from 'vue';
  import { NTag } from 'naive-ui';
  import { AccountList, PublishRecordList } from '@/api/addons/youbanPublish';

  const loading = ref(false);
  const accountLoading = ref(false);
  const records = ref<any[]>([]);
  const accounts = ref<any[]>([]);

  const query = reactive({
    accountId: null as number | null,
    action: '',
    status: '',
    keyword: '',
  });

  const pagination = createPagination(loadRecords, 20);

  const accountOptions = computed(() =>
    accounts.value.map((item) => ({
      label: accountLabel(item),
      value: item.id,
    }))
  );

  const actionOptions = [
    { label: 'TG推送', value: 'publish' },
    { label: '账号消息推送', value: 'message_push' },
    { label: '采集推送', value: 'collect_publish' },
    { label: '全量推送', value: 'full_push' },
    { label: '循环上架', value: 'cycle_publish' },
    { label: '下架', value: 'delete' },
  ];

  const statusOptions = [
    { label: '处理中', value: 'sending' },
    { label: '失败/待重试', value: 'error' },
    { label: '待重试', value: 'failed_retry' },
    { label: '成功', value: 'sent' },
    { label: '最终失败', value: 'failed' },
    { label: '已废弃', value: 'superseded' },
  ];

  const columns = [
    { title: '时间', key: 'createdAt', width: 170 },
    {
      title: '账号归属',
      key: 'tenantName',
      width: 160,
      render: (row) => row.tenantName || '-',
    },
    {
      title: '上架账号',
      key: 'accountName',
      width: 150,
      render: (row) => row.accountName || row.accountId || '-',
    },
    { title: '资料', key: 'title', width: 180, render: (row) => row.title || recordTarget(row) },
    { title: '动作', key: 'action', width: 130, render: (row) => actionLabel(row.action) },
    { title: '执行方', key: 'actor', width: 160, render: (row) => recordActor(row) },
    { title: '结果', key: 'status', width: 110, render: (row) => renderStatus(row.status) },
    { title: '目标', key: 'channelTitle', width: 220, render: (row) => recordChannel(row) },
    {
      title: '详情',
      key: 'message',
      width: 520,
      ellipsis: { tooltip: true },
      render: (row) => recordMessage(row),
    },
    { title: '任务ID', key: 'jobId', width: 90 },
    { title: '资料ID', key: 'profileId', width: 90 },
  ];

  onMounted(async () => {
    await Promise.all([loadAccounts(), loadRecords()]);
  });

  function createPagination(loader: () => void, pageSize = 20) {
    const changePage = (value: number) => {
      page.page = value;
      loader();
    };
    const changePageSize = (value: number) => {
      page.pageSize = value;
      page.page = 1;
      loader();
    };
    const page: any = reactive({
      page: 1,
      pageSize,
      itemCount: 0,
      pageCount: 1,
      showSizePicker: true,
      pageSizes: [10, 20, 50, 100],
      prefix: ({ itemCount }) => `共 ${itemCount} 条`,
      onChange: changePage,
      onUpdatePage: changePage,
      onUpdatePageSize: changePageSize,
    });
    return page;
  }

  async function loadAccounts() {
    accountLoading.value = true;
    try {
      const res: any = await AccountList({ page: 1, perPage: 200, status: 0 });
      accounts.value = res?.list || [];
    } finally {
      accountLoading.value = false;
    }
  }

  async function loadRecords() {
    loading.value = true;
    try {
      const res: any = await PublishRecordList({
        ...query,
        page: pagination.page,
        pageSize: pagination.pageSize,
        perPage: pagination.pageSize,
      });
      records.value = res?.list || [];
      pagination.itemCount = res?.totalCount || res?.total || 0;
      pagination.pageCount =
        res?.pageCount || Math.max(1, Math.ceil(pagination.itemCount / pagination.pageSize));
    } finally {
      loading.value = false;
    }
  }

  function resetQuery() {
    Object.assign(query, { accountId: null, action: '', status: '', keyword: '' });
    pagination.page = 1;
    loadRecords();
  }

  function accountLabel(item: any) {
    const owner = item.tenantName ? `${item.tenantName} / ` : '';
    const name = item.nickname || item.username || `账号 ${item.id}`;
    return `${owner}${name}`;
  }

  function recordTarget(row: any) {
    if (row.profileId) return `资料 ${row.profileId}`;
    if (row.taskId) return `任务 ${row.taskId}`;
    return `记录 ${row.id}`;
  }

  function recordActor(row: any) {
    if (row.botName || row.botUsername) {
      return row.botUsername
        ? `${row.botName || 'Bot'} @${String(row.botUsername).replace(/^@/, '')}`
        : row.botName || 'Bot';
    }
    return row.accountName || '系统';
  }

  function recordChannel(row: any) {
    return (
      row.channelTitle ||
      row.channelUsername ||
      row.targetChatId ||
      (row.channelId ? `频道 ${row.channelId}` : '-')
    );
  }

  function recordMessage(row: any) {
    const progress = row.progressText ? `进度 ${row.progressText}` : '';
    return [progress, row.message].filter(Boolean).join(' · ') || '-';
  }

  function actionLabel(action: string) {
    return actionOptions.find((item) => item.value === action)?.label || action || '-';
  }

  function renderStatus(status: string) {
    const option = statusOptions.find((item) => item.value === status);
    const type =
      status === 'sent' || status === 'success'
        ? 'success'
        : status === 'failed'
          ? 'error'
          : status === 'failed_retry'
            ? 'warning'
            : status === 'sending' || status === 'pending'
              ? 'info'
              : 'default';
    return h(NTag, { type, bordered: false }, { default: () => option?.label || status || '-' });
  }
</script>

<style scoped>
  .toolbar {
    margin-bottom: 16px;
  }

  .account-select {
    width: 260px;
  }

  .status-select {
    width: 140px;
  }

  .keyword-input {
    width: 260px;
  }
</style>
