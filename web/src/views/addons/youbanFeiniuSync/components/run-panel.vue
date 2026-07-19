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
      <n-select
        v-model:value="query.status"
        :options="runStatusOptions"
        clearable
        placeholder="运行状态"
        class="status-select"
      />
      <n-date-picker v-model:value="dateRange" type="daterange" clearable />
      <n-button @click="loadData">查询</n-button>
      <n-button type="primary" :disabled="!query.configId" @click="startRun">立即同步</n-button>
    </n-space>
    <n-data-table
      :columns="columns"
      :data="list"
      :loading="loading"
      :pagination="pagination"
      :row-key="(row) => row.id"
      :scroll-x="1180"
      size="small"
      remote
    />
    <RunDetailDrawer ref="drawerRef" v-model:show="drawerVisible" :run-id="currentRunId" />
  </n-space>
</template>

<script setup lang="ts">
  import { computed, h, nextTick, onMounted, reactive, ref } from 'vue';
  import { NButton, NTag, useMessage } from 'naive-ui';
  import { ConfigList, RunList, RunStart } from '@/api/addons/youbanFeiniuSync';
  import RunDetailDrawer from './run-detail-drawer.vue';

  const message = useMessage();
  const loading = ref(false);
  const configs = ref<any[]>([]);
  const list = ref<any[]>([]);
  const dateRange = ref<[number, number] | null>(null);
  const drawerVisible = ref(false);
  const currentRunId = ref<number | null>(null);
  const drawerRef = ref<InstanceType<typeof RunDetailDrawer> | null>(null);
  const query = reactive({ configId: null as number | null, status: null as string | null });
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
  const runStatusOptions = [
    { label: '运行中', value: 'running' },
    { label: '成功', value: 'success' },
    { label: '失败', value: 'failed' },
  ];
  const tagType = (status: string) =>
    status === 'success' ? 'success' : status === 'running' ? 'info' : 'error';
  const columns = [
    { title: 'ID', key: 'id', width: 90 },
    { title: '配置ID', key: 'configId', width: 90 },
    { title: '类型', key: 'runType', width: 90 },
    {
      title: '状态',
      key: 'status',
      width: 90,
      render: (row) =>
        h(NTag, { type: tagType(row.status), bordered: false }, { default: () => row.status }),
    },
    { title: '总数', key: 'totalCount', width: 80 },
    { title: '新增', key: 'createdCount', width: 80 },
    { title: '更新', key: 'updatedCount', width: 80 },
    { title: '跳过', key: 'skippedCount', width: 80 },
    { title: '失败', key: 'failedCount', width: 80 },
    { title: '开始时间', key: 'startedAt', width: 170 },
    { title: '结束时间', key: 'finishedAt', width: 170 },
    { title: '错误', key: 'errorMessage', ellipsis: { tooltip: true } },
    {
      title: '操作',
      key: 'actions',
      width: 90,
      fixed: 'right',
      render: (row) =>
        h(
          NButton,
          { size: 'small', type: 'primary', onClick: () => openDetail(row.id) },
          { default: () => '情况' }
        ),
    },
  ];

  async function loadConfigs() {
    const res = await ConfigList({ page: 1, pageSize: 100 });
    configs.value = res.list || [];
  }
  function dateParams() {
    if (!dateRange.value) return {};
    return { startDate: formatDate(dateRange.value[0]), endDate: formatDate(dateRange.value[1]) };
  }
  async function loadData() {
    loading.value = true;
    try {
      const res = await RunList({
        ...query,
        ...dateParams(),
        page: pagination.page,
        pageSize: pagination.pageSize,
      });
      list.value = res.list || [];
      pagination.itemCount = res.totalCount || res.total || 0;
    } finally {
      loading.value = false;
    }
  }
  async function startRun() {
    if (!query.configId) return;
    const res = await RunStart({ configId: query.configId });
    message.success(`同步完成，运行ID：${res.runId}`);
    await loadData();
  }
  async function openDetail(id: number) {
    currentRunId.value = id;
    drawerVisible.value = true;
    await nextTick();
    drawerRef.value?.open(id);
  }
  function formatDate(value: number) {
    const d = new Date(value);
    const m = `${d.getMonth() + 1}`.padStart(2, '0');
    const day = `${d.getDate()}`.padStart(2, '0');
    return `${d.getFullYear()}-${m}-${day}`;
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
