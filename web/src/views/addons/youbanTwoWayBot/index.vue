<template>
  <div class="youban-two-way-bot-page">
    <div class="n-layout-page-header">
      <n-card :bordered="false" title="双向机器人">
        <span>查看 Telegram 双向机器人、管理群和 Webhook 状态</span>
      </n-card>
    </div>

    <n-card :bordered="false" class="proCard">
      <n-space class="mb-4" justify="space-between">
        <n-space>
          <n-input
            v-model:value="query.keyword"
            clearable
            placeholder="搜索机器人名称或 Bot 用户名"
            class="youban-two-way-bot-page__search"
            @keyup.enter="loadBots"
          />
          <n-select
            v-model:value="query.status"
            clearable
            :options="statusOptions"
            placeholder="启用状态"
            class="youban-two-way-bot-page__select"
          />
          <n-button @click="handleSearch">查询</n-button>
        </n-space>
        <n-button :loading="loading" @click="loadBots">刷新</n-button>
      </n-space>

      <n-data-table
        :columns="columns"
        :data="rows"
        :loading="loading"
        :pagination="pagination"
        remote
        :row-key="(row) => row.id"
        :scroll-x="1100"
        @update:page="handlePage"
        @update:page-size="handlePageSize"
      />
    </n-card>
  </div>
</template>

<script setup lang="ts">
  import { h, onMounted, reactive, ref } from 'vue';
  import { NTag, NTooltip, useMessage } from 'naive-ui';
  import type { DataTableColumns } from 'naive-ui';

  import { BotList } from '@/api/addons/youbanTwoWayBot';

  interface TwoWayBotRow {
    id: number;
    name: string;
    botUsername?: string;
    tgAccountId?: number;
    tgAccountName?: string;
    supergroupTitle?: string;
    inviteLink?: string;
    setupStatus?: string;
    webhookStatus?: string;
    status: number;
    updatedAt?: string;
  }

  const message = useMessage();
  const loading = ref(false);
  const rows = ref<TwoWayBotRow[]>([]);

  const query = reactive({
    page: 1,
    perPage: 10,
    keyword: '',
    status: null as number | null,
  });

  const pagination = reactive({
    page: 1,
    pageSize: 10,
    itemCount: 0,
    showSizePicker: true,
    pageSizes: [10, 20, 50],
  });

  const statusOptions = [
    { label: '启用', value: 1 },
    { label: '停用', value: 0 },
  ];

  const statusMap: Record<
    string,
    { label: string; type: 'default' | 'error' | 'success' | 'warning' }
  > = {
    failed: { label: '失败', type: 'error' },
    manual_required: { label: '需处理', type: 'warning' },
    pending: { label: '待处理', type: 'default' },
    polling: { label: 'Pull模式', type: 'success' },
    ready: { label: '正常', type: 'success' },
  };

  function renderStatus(value?: number | string) {
    if (value === 1) {
      return h(NTag, { type: 'success' }, { default: () => '启用' });
    }
    if (value === 0) {
      return h(NTag, { type: 'default' }, { default: () => '停用' });
    }
    const option = statusMap[String(value || 'pending')] || statusMap.pending;
    return h(NTag, { type: option.type }, { default: () => option.label });
  }

  const columns: DataTableColumns<TwoWayBotRow> = [
    {
      title: '机器人',
      key: 'name',
      minWidth: 220,
      render(row) {
        return h('div', [
          h('div', { class: 'youban-two-way-bot-page__name' }, row.name || '-'),
          row.botUsername
            ? h(NTag, { size: 'small', type: 'info' }, { default: () => `@${row.botUsername}` })
            : h('span', { class: 'text-secondary' }, `ID：${row.id}`),
        ]);
      },
    },
    {
      title: '管理群',
      key: 'supergroupTitle',
      minWidth: 240,
      render(row) {
        return h('div', [
          h('div', row.supergroupTitle || '-'),
          row.inviteLink
            ? h(
                NTooltip,
                {},
                {
                  default: () => row.inviteLink,
                  trigger: () => h('a', { href: row.inviteLink, target: '_blank' }, '邀请链接'),
                }
              )
            : h('span', { class: 'text-secondary' }, '未生成邀请链接'),
        ]);
      },
    },
    {
      title: 'TG 账号',
      key: 'tgAccountName',
      width: 160,
      render(row) {
        return row.tgAccountName || (row.tgAccountId ? `ID：${row.tgAccountId}` : '-');
      },
    },
    {
      title: '初始化',
      key: 'setupStatus',
      width: 120,
      render(row) {
        return renderStatus(row.setupStatus);
      },
    },
    {
      title: 'Webhook',
      key: 'webhookStatus',
      width: 120,
      render(row) {
        return renderStatus(row.webhookStatus);
      },
    },
    {
      title: '启用状态',
      key: 'status',
      width: 120,
      render(row) {
        return renderStatus(row.status);
      },
    },
    { title: '更新时间', key: 'updatedAt', width: 180 },
  ];

  async function loadBots() {
    loading.value = true;
    try {
      const res = await BotList({
        page: query.page,
        perPage: query.perPage,
        keyword: query.keyword || undefined,
        status: query.status ?? undefined,
      });
      rows.value = res?.list || [];
      pagination.itemCount = res?.totalCount || 0;
      pagination.page = query.page;
      pagination.pageSize = query.perPage;
    } catch (error) {
      console.error(error);
      message.error('加载双向机器人列表失败');
    } finally {
      loading.value = false;
    }
  }

  function handleSearch() {
    query.page = 1;
    loadBots();
  }

  function handlePage(page: number) {
    query.page = page;
    loadBots();
  }

  function handlePageSize(pageSize: number) {
    query.page = 1;
    query.perPage = pageSize;
    loadBots();
  }

  onMounted(() => {
    loadBots();
  });
</script>

<style lang="less" scoped>
  .youban-two-way-bot-page {
    &__search {
      width: 260px;
    }

    &__select {
      width: 140px;
    }

    &__name {
      margin-bottom: 6px;
      font-weight: 500;
    }
  }
</style>
