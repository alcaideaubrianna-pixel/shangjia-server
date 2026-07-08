<template>
  <n-space vertical>
    <n-alert type="info" :bordered="false">
      推送观测按队列、频道、Bot 维度展示当前 TG 推送状态，统计数据由后台定时刷新。
    </n-alert>

    <n-tabs v-model:value="activePane" type="segment" @update:value="handlePaneChange">
      <n-tab-pane name="queue" tab="队列">
        <n-space class="toolbar" align="center">
          <n-select
            v-model:value="queueQuery.queueName"
            :options="queueOptions"
            clearable
            placeholder="队列"
            class="queue-select"
          />
          <n-select
            v-model:value="queueQuery.status"
            :options="statusOptions"
            clearable
            placeholder="状态"
            class="status-select"
          />
          <n-button @click="loadQueueStats">查询</n-button>
          <n-button quaternary @click="resetQueueQuery">重置</n-button>
        </n-space>
        <n-data-table
          :columns="queueColumns"
          :data="queueStats"
          :loading="queueLoading"
          :pagination="queuePagination"
          :row-key="(row) => row.id"
          :scroll-x="980"
          size="small"
          remote
        />
      </n-tab-pane>

      <n-tab-pane name="channel" tab="频道">
        <n-space class="toolbar" align="center">
          <n-input-number
            v-model:value="channelQuery.accountId"
            clearable
            placeholder="账号ID"
            class="id-input"
          />
          <n-input-number
            v-model:value="channelQuery.channelId"
            clearable
            placeholder="频道ID"
            class="id-input"
          />
          <n-input
            v-model:value="channelQuery.keyword"
            clearable
            placeholder="频道 / Chat ID / 账号"
            class="keyword-input"
            @keyup.enter="loadChannelStats"
          />
          <n-button @click="loadChannelStats">查询</n-button>
          <n-button quaternary @click="resetChannelQuery">重置</n-button>
        </n-space>
        <n-data-table
          :columns="channelColumns"
          :data="channelStats"
          :loading="channelLoading"
          :pagination="channelPagination"
          :row-key="(row) => row.id"
          :scroll-x="1460"
          size="small"
          remote
        />
      </n-tab-pane>

      <n-tab-pane name="bot" tab="Bot">
        <n-space class="toolbar" align="center">
          <n-input-number
            v-model:value="botQuery.botId"
            clearable
            placeholder="Bot ID"
            class="id-input"
          />
          <n-input
            v-model:value="botQuery.keyword"
            clearable
            placeholder="Bot 名称 / 用户名"
            class="keyword-input"
            @keyup.enter="loadBotStats"
          />
          <n-button @click="loadBotStats">查询</n-button>
          <n-button quaternary @click="resetBotQuery">重置</n-button>
        </n-space>
        <n-data-table
          :columns="botColumns"
          :data="botStats"
          :loading="botLoading"
          :pagination="botPagination"
          :row-key="(row) => row.id"
          :scroll-x="1280"
          size="small"
          remote
        />
      </n-tab-pane>
    </n-tabs>
  </n-space>
</template>

<script lang="ts" setup>
  import { h, onMounted, reactive, ref } from 'vue';
  import { NTag } from 'naive-ui';
  import {
    TgObserveBotList,
    TgObserveChannelList,
    TgObserveQueueList,
  } from '@/api/addons/youbanPublish';

  const activePane = ref('queue');
  const queueStats = ref<any[]>([]);
  const channelStats = ref<any[]>([]);
  const botStats = ref<any[]>([]);
  const queueLoading = ref(false);
  const channelLoading = ref(false);
  const botLoading = ref(false);

  const queueQuery = reactive({ queueName: '', status: '' });
  const channelQuery = reactive({
    accountId: null as number | null,
    channelId: null as number | null,
    keyword: '',
  });
  const botQuery = reactive({ botId: null as number | null, keyword: '' });

  const queuePagination = createPagination(loadQueueStats, 20);
  const channelPagination = createPagination(loadChannelStats, 20);
  const botPagination = createPagination(loadBotStats, 20);

  const queueOptions = [
    { label: '高优先级', value: 'youban_publish_tg_urgent' },
    { label: '普通队列', value: 'youban_publish_tg' },
    { label: '批量队列', value: 'youban_publish_tg_bulk' },
  ];

  const statusOptions = [
    { label: '待发送', value: 'pending' },
    { label: '已调度', value: 'queued' },
    { label: '发送中', value: 'sending' },
    { label: '待重试', value: 'failed_retry' },
    { label: '已成功', value: 'sent' },
    { label: '已失败', value: 'failed' },
    { label: '已废弃', value: 'superseded' },
  ];

  const queueColumns = [
    { title: '队列', key: 'queueName', width: 220, render: (row) => queueLabel(row.queueName) },
    { title: '优先级', key: 'priorityLevel', width: 90 },
    { title: '状态', key: 'status', width: 120, render: (row) => renderStatus(row.status) },
    { title: '任务数', key: 'jobCount', width: 100 },
    { title: '最早任务', key: 'oldestJobAt', width: 170 },
    { title: '最新任务', key: 'latestJobAt', width: 170 },
    { title: '更新时间', key: 'updatedAt', width: 170 },
  ];

  const channelColumns = [
    { title: '频道ID', key: 'channelId', width: 90 },
    { title: '频道', key: 'channelTitle', width: 180, render: (row) => row.channelTitle || '-' },
    { title: 'Chat ID', key: 'targetChatId', width: 180 },
    { title: '账号', key: 'accountName', width: 150, render: (row) => row.accountName || row.accountId || '-' },
    { title: '待调度', key: 'pendingCount', width: 90 },
    { title: '已调度', key: 'queuedCount', width: 90 },
    { title: '发送中', key: 'sendingCount', width: 90 },
    { title: '待重试', key: 'retryCount', width: 90 },
    { title: '成功', key: 'sentCount', width: 90 },
    { title: '失败', key: 'failedCount', width: 90 },
    { title: '限流', key: 'rateLimitCount', width: 90 },
    { title: '最后成功', key: 'lastSentAt', width: 170 },
    { title: '最后错误', key: 'lastErrorMessage', width: 260, ellipsis: { tooltip: true } },
    { title: '更新时间', key: 'updatedAt', width: 170 },
  ];

  const botColumns = [
    { title: 'Bot ID', key: 'botId', width: 90 },
    { title: 'Bot名称', key: 'botName', width: 180, render: (row) => row.botName || '-' },
    { title: '用户名', key: 'botUsername', width: 180, render: (row) => row.botUsername || '-' },
    { title: '待调度', key: 'pendingCount', width: 90 },
    { title: '已调度', key: 'queuedCount', width: 90 },
    { title: '发送中', key: 'sendingCount', width: 90 },
    { title: '待重试', key: 'retryCount', width: 90 },
    { title: '成功', key: 'sentCount', width: 90 },
    { title: '失败', key: 'failedCount', width: 90 },
    { title: '限流', key: 'rateLimitCount', width: 90 },
    { title: '最后成功', key: 'lastSentAt', width: 170 },
    { title: '最后错误', key: 'lastErrorMessage', width: 260, ellipsis: { tooltip: true } },
    { title: '更新时间', key: 'updatedAt', width: 170 },
  ];

  onMounted(loadQueueStats);

  function createPagination(loader: () => void, pageSize = 20) {
    const pagination: any = {
      page: 1,
      pageSize,
      itemCount: 0,
      showSizePicker: true,
      pageSizes: [10, 20, 50],
      onChange: (page) => {
        pagination.page = page;
        loader();
      },
      onUpdatePageSize: (pageSize) => {
        pagination.pageSize = pageSize;
        pagination.page = 1;
        loader();
      },
    };
    return pagination;
  }

  async function handlePaneChange(pane: string) {
    if (pane === 'queue') await loadQueueStats();
    if (pane === 'channel') await loadChannelStats();
    if (pane === 'bot') await loadBotStats();
  }

  async function loadQueueStats() {
    queueLoading.value = true;
    try {
      const res: any = await TgObserveQueueList({
        ...queueQuery,
        page: queuePagination.page,
        perPage: queuePagination.pageSize,
      });
      queueStats.value = res?.list || [];
      queuePagination.itemCount = res?.totalCount || res?.total || 0;
    } finally {
      queueLoading.value = false;
    }
  }

  async function loadChannelStats() {
    channelLoading.value = true;
    try {
      const res: any = await TgObserveChannelList({
        ...channelQuery,
        page: channelPagination.page,
        perPage: channelPagination.pageSize,
      });
      channelStats.value = res?.list || [];
      channelPagination.itemCount = res?.totalCount || res?.total || 0;
    } finally {
      channelLoading.value = false;
    }
  }

  async function loadBotStats() {
    botLoading.value = true;
    try {
      const res: any = await TgObserveBotList({
        ...botQuery,
        page: botPagination.page,
        perPage: botPagination.pageSize,
      });
      botStats.value = res?.list || [];
      botPagination.itemCount = res?.totalCount || res?.total || 0;
    } finally {
      botLoading.value = false;
    }
  }

  function resetQueueQuery() {
    Object.assign(queueQuery, { queueName: '', status: '' });
    queuePagination.page = 1;
    loadQueueStats();
  }

  function resetChannelQuery() {
    Object.assign(channelQuery, { accountId: null, channelId: null, keyword: '' });
    channelPagination.page = 1;
    loadChannelStats();
  }

  function resetBotQuery() {
    Object.assign(botQuery, { botId: null, keyword: '' });
    botPagination.page = 1;
    loadBotStats();
  }

  function queueLabel(queueName: string) {
    return queueOptions.find((item) => item.value === queueName)?.label || queueName || '-';
  }

  function renderStatus(status: string) {
    const option = statusOptions.find((item) => item.value === status);
    const type =
      status === 'sent'
        ? 'success'
        : status === 'failed'
          ? 'error'
          : status === 'sending' || status === 'queued'
            ? 'info'
            : status === 'failed_retry'
              ? 'warning'
              : 'default';
    return h(NTag, { type, bordered: false }, { default: () => option?.label || status || '-' });
  }
</script>

<style scoped>
  .toolbar {
    margin-bottom: 16px;
  }

  .queue-select {
    width: 180px;
  }

  .status-select {
    width: 140px;
  }

  .id-input {
    width: 140px;
  }

  .keyword-input {
    width: 240px;
  }
</style>
