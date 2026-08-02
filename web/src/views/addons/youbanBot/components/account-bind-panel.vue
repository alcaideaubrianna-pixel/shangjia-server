<template>
  <div class="account-bind-panel">
    <n-space class="mb-4" justify="space-between">
      <n-space>
        <n-input
          v-model:value="query.keyword"
          clearable
          placeholder="搜索 TG / 系统账号 / 账号归属"
          class="account-bind-panel__keyword"
          @keyup.enter="handleSearch"
        />
        <n-select
          v-model:value="query.app"
          clearable
          :options="appOptions"
          placeholder="绑定端"
          class="account-bind-panel__select"
        />
        <n-select
          v-model:value="query.botId"
          clearable
          filterable
          :options="botOptions"
          placeholder="绑定 Bot"
          class="account-bind-panel__bot-select"
        />
        <n-select
          v-model:value="query.status"
          clearable
          :options="statusOptions"
          placeholder="绑定状态"
          class="account-bind-panel__select"
        />
        <n-button type="primary" @click="handleSearch">查询</n-button>
        <n-button @click="resetSearch">重置</n-button>
      </n-space>
      <n-button @click="loadBindings">刷新</n-button>
    </n-space>

    <n-data-table
      :columns="columns"
      :data="rows"
      :loading="loading"
      :pagination="pagination"
      :scroll-x="1450"
      remote
      @update:page="handlePage"
      @update:page-size="handlePageSize"
    />
  </div>
</template>

<script setup lang="ts">
  import { h, onMounted, reactive, ref } from 'vue';
  import { NButton, NPopconfirm, NSpace, NTag, useMessage } from 'naive-ui';

  import { AccountBindList, AccountBindUnbind, BotList } from '@/api/addons/youbanBot';

  const message = useMessage();
  const loading = ref(false);
  const rows = ref<any[]>([]);
  const botOptions = ref<Array<{ label: string; value: number }>>([]);

  const query = reactive({
    app: null as null | string,
    botId: null as null | number,
    keyword: '',
    status: 1 as null | number,
  });

  const pagination = reactive({
    itemCount: 0,
    page: 1,
    pageSize: 20,
    pageSizes: [20, 50, 100],
    showSizePicker: true,
  });

  const appOptions = [
    { label: '用户端', value: 'api' },
    { label: '总后台', value: 'admin' },
  ];

  const statusOptions = [
    { label: '已绑定', value: 1 },
    { label: '已解绑', value: 2 },
  ];

  const columns = [
    { title: 'ID', key: 'id', width: 80 },
    {
      title: '绑定端',
      key: 'app',
      width: 100,
      render: (row: any) =>
        h(
          NTag,
          { type: row.app === 'api' ? 'success' : 'info' },
          { default: () => (row.app === 'api' ? '用户端' : '总后台') }
        ),
    },
    {
      title: '系统账号',
      key: 'accountName',
      minWidth: 190,
      render: (row: any) =>
        h('div', { class: 'account-bind-panel__account' }, [
          h(
            'span',
            { class: 'account-bind-panel__primary' },
            row.accountName || row.accountUsername || '-'
          ),
          h(
            'span',
            { class: 'account-bind-panel__secondary' },
            `${row.accountUsername || '-'} · ID ${row.accountId}`
          ),
        ]),
    },
    {
      title: '账号归属',
      key: 'tenantName',
      minWidth: 150,
      render: (row: any) => row.tenantName || (row.app === 'admin' ? '系统总后台' : '-'),
    },
    {
      title: 'Telegram 用户',
      key: 'telegramUsername',
      minWidth: 220,
      render: (row: any) =>
        h('div', { class: 'account-bind-panel__account' }, [
          h(
            'span',
            { class: 'account-bind-panel__primary' },
            row.telegramUsername ? `@${row.telegramUsername}` : telegramName(row)
          ),
          h(
            'span',
            { class: 'account-bind-panel__secondary' },
            `TG ID ${row.telegramUserId || '-'}`
          ),
        ]),
    },
    {
      title: '绑定 Bot',
      key: 'botUsername',
      minWidth: 170,
      render: (row: any) => (row.botUsername ? `@${row.botUsername}` : `Bot ID ${row.botId}`),
    },
    {
      title: '状态',
      key: 'status',
      width: 100,
      render: (row: any) =>
        h(
          NTag,
          { type: row.status === 1 ? 'success' : 'default' },
          { default: () => (row.status === 1 ? '已绑定' : '已解绑') }
        ),
    },
    { title: '绑定时间', key: 'createdAt', width: 170 },
    { title: '更新时间', key: 'updatedAt', width: 170 },
    {
      title: '操作',
      key: 'actions',
      fixed: 'right',
      width: 100,
      render: (row: any) => {
        if (row.status !== 1) {
          return '-';
        }
        return h(
          NPopconfirm,
          { onPositiveClick: () => unbind(row) },
          {
            default: () =>
              `确认解除 ${row.telegramUsername ? `@${row.telegramUsername}` : '该 TG 用户'} 与系统账号的绑定？`,
            trigger: () => h(NButton, { size: 'small', type: 'error' }, { default: () => '解绑' }),
          }
        );
      },
    },
  ];

  function telegramName(row: any) {
    return (
      [row.telegramFirstName, row.telegramLastName].filter(Boolean).join(' ') || '未设置用户名'
    );
  }

  async function loadBotOptions() {
    const res: any = await BotList({ page: 1, perPage: 200 });
    botOptions.value = (res?.list || []).map((item: any) => ({
      label: item.botUsername ? `@${item.botUsername}` : item.botName || `Bot ${item.id}`,
      value: item.id,
    }));
  }

  async function loadBindings() {
    loading.value = true;
    try {
      const res: any = await AccountBindList({
        ...query,
        page: pagination.page,
        perPage: pagination.pageSize,
      });
      rows.value = res?.list || [];
      pagination.itemCount = res?.totalCount || res?.total || 0;
    } finally {
      loading.value = false;
    }
  }

  function handleSearch() {
    pagination.page = 1;
    loadBindings();
  }

  function resetSearch() {
    Object.assign(query, {
      app: null,
      botId: null,
      keyword: '',
      status: 1,
    });
    handleSearch();
  }

  function handlePage(page: number) {
    pagination.page = page;
    loadBindings();
  }

  function handlePageSize(pageSize: number) {
    pagination.pageSize = pageSize;
    pagination.page = 1;
    loadBindings();
  }

  async function unbind(row: any) {
    await AccountBindUnbind({ ids: [row.id] });
    message.success('解绑成功');
    if (rows.value.length === 1 && pagination.page > 1) {
      pagination.page -= 1;
    }
    await loadBindings();
  }

  onMounted(() => {
    loadBotOptions();
    loadBindings();
  });
</script>

<style scoped>
  .account-bind-panel__keyword {
    width: 280px;
  }

  .account-bind-panel__select {
    width: 130px;
  }

  .account-bind-panel__bot-select {
    width: 190px;
  }

  .account-bind-panel__account {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .account-bind-panel__primary {
    color: var(--n-text-color);
  }

  .account-bind-panel__secondary {
    color: var(--n-text-color-3);
    font-size: 12px;
  }
</style>
