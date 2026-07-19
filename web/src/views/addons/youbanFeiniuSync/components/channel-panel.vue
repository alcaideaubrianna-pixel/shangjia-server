<template>
  <n-space vertical>
    <n-space class="toolbar" align="center">
      <n-select
        v-model:value="query.configId"
        :options="configOptions"
        clearable
        filterable
        placeholder="同步配置"
        class="config-select"
      />
      <n-input
        v-model:value="query.keyword"
        placeholder="频道 / 上架账号"
        clearable
        @keyup.enter="loadData"
      />
      <n-select
        v-model:value="query.syncStatus"
        :options="syncStatusOptions"
        clearable
        placeholder="同步状态"
        class="status-select"
      />
      <n-button @click="loadData">查询</n-button>
      <n-button type="error" :loading="clearing" @click="handleClear"> 清空同步数据 </n-button>
    </n-space>
    <n-data-table
      :columns="columns"
      :data="list"
      :loading="loading"
      :pagination="pagination"
      :row-key="(row) => row.id"
      :scroll-x="1160"
      size="small"
      remote
    />
  </n-space>
</template>

<script setup lang="ts">
  import { computed, h, onMounted, reactive, ref } from 'vue';
  import { NTag, useDialog, useMessage } from 'naive-ui';
  import { ChannelClear, ChannelMapList, ConfigList } from '@/api/addons/youbanFeiniuSync';

  const loading = ref(false);
  const clearing = ref(false);
  const configs = ref<any[]>([]);
  const list = ref<any[]>([]);
  const dialog = useDialog();
  const message = useMessage();
  const query = reactive({
    configId: null as number | null,
    keyword: '',
    syncStatus: null as string | null,
  });
  const pagination = reactive({
    page: 1,
    pageSize: 20,
    itemCount: 0,
    onUpdatePage: (page: number) => {
      pagination.page = page;
      loadData();
    },
  });
  const configOptions = computed(() =>
    configs.value.map((item) => ({ label: item.name, value: item.id }))
  );
  const syncStatusOptions = [
    { label: '成功', value: 'success' },
    { label: '失败', value: 'failed' },
    { label: '待处理', value: 'pending' },
  ];
  const tag = (value: string) =>
    h(
      NTag,
      {
        type: value === 'success' ? 'success' : value === 'failed' ? 'error' : 'warning',
        bordered: false,
      },
      { default: () => value || '-' }
    );
  const columns = [
    { title: 'FeiNiu频道', key: 'feiniuChannelTitle', width: 220 },
    { title: '频道ID', key: 'feiniuChannelId', width: 120 },
    { title: 'TG Chat ID', key: 'feiniuTgChatId', width: 140 },
    { title: '上架账号', key: 'youbanAccountUsername', width: 160 },
    { title: '上架账号ID', key: 'youbanAccountId', width: 120 },
    { title: '最后源时间', key: 'lastSourceUpdateTime', width: 170 },
    { title: '最后笔记ID', key: 'lastSourceNoteId', width: 120 },
    { title: '状态', key: 'syncStatus', width: 100, render: (row) => tag(row.syncStatus) },
    { title: '错误', key: 'errorMessage', ellipsis: { tooltip: true } },
  ];

  async function loadConfigs() {
    const res = await ConfigList({ page: 1, pageSize: 100 });
    configs.value = res.list || [];
  }
  async function loadData() {
    loading.value = true;
    try {
      const res = await ChannelMapList({
        ...query,
        page: pagination.page,
        pageSize: pagination.pageSize,
      });
      list.value = res.list || [];
      pagination.itemCount = res.totalCount || res.total || 0;
    } finally {
      loading.value = false;
    }
  }
  function handleClear() {
    if (!query.configId) {
      message.warning('请先选择同步配置');
      return;
    }
    const config = configs.value.find((item) => item.id === query.configId);
    dialog.warning({
      title: '清空同步数据',
      content: `确认清空${config?.name ? `「${config.name}」` : '当前配置'}已同步的数据和记录吗？该操作会删除同步映射、运行记录，并软删除 FeiNiu 导入的资料、任务和自动账号。`,
      positiveText: '确认清空',
      negativeText: '取消',
      onPositiveClick: async () => {
        clearing.value = true;
        try {
          const res = await ChannelClear({ configId: query.configId });
          message.success(
            `清空完成：资料 ${res.profileCount || 0} 条，任务 ${res.taskCount || 0} 条，账号 ${
              res.accountCount || 0
            } 个`
          );
          pagination.page = 1;
          await loadData();
        } finally {
          clearing.value = false;
        }
      },
    });
  }
  onMounted(async () => {
    await loadConfigs();
    await loadData();
  });
</script>

<style scoped>
  .toolbar {
    margin-bottom: 12px;
  }
  .config-select {
    width: 220px;
  }
  .status-select {
    width: 120px;
  }
</style>
