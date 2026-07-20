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
      :scroll-x="1460"
      size="small"
      remote
    />
    <n-modal v-model:show="copyVisible">
      <n-card title="复制频道资料" class="copy-modal" :bordered="false">
        <n-form label-placement="top">
          <n-form-item label="源频道">
            <n-input :value="copySourceLabel" readonly />
          </n-form-item>
          <n-form-item label="目标租户">
            <n-select
              v-model:value="copyForm.targetTenantId"
              :options="tenantOptions"
              filterable
              clearable
              placeholder="请选择租户"
              @update:value="handleTargetTenantChange"
            />
          </n-form-item>
          <n-form-item label="目标账号">
            <n-select
              v-model:value="copyForm.targetAccountId"
              :options="accountOptions"
              :loading="accountLoading"
              filterable
              clearable
              placeholder="请选择租户管理员或上架账号"
            />
          </n-form-item>
        </n-form>
        <template #footer>
          <n-space justify="end">
            <n-button @click="copyVisible = false">取消</n-button>
            <n-button type="primary" :loading="copying" @click="handleCopySubmit">
              开始复制
            </n-button>
          </n-space>
        </template>
      </n-card>
    </n-modal>
  </n-space>
</template>

<script setup lang="ts">
  import { computed, h, onMounted, reactive, ref } from 'vue';
  import { NButton, NSpace, NTag, useDialog, useMessage } from 'naive-ui';
  import {
    AccountOptions,
    ChannelClear,
    ChannelCopy,
    ChannelDisable,
    ChannelMapList,
    ConfigList,
    TenantOptions,
  } from '@/api/addons/youbanFeiniuSync';

  const loading = ref(false);
  const clearing = ref(false);
  const copying = ref(false);
  const accountLoading = ref(false);
  const copyVisible = ref(false);
  const configs = ref<any[]>([]);
  const tenants = ref<any[]>([]);
  const accounts = ref<any[]>([]);
  const list = ref<any[]>([]);
  const copySource = ref<any | null>(null);
  const dialog = useDialog();
  const message = useMessage();
  const query = reactive({
    configId: null as number | null,
    keyword: '',
    syncStatus: null as string | null,
  });
  const copyForm = reactive({
    targetTenantId: null as number | null,
    targetAccountId: null as number | null,
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
  const tenantOptions = computed(() =>
    tenants.value.map((item) => ({ label: item.label || item.name, value: item.value || item.id }))
  );
  const accountOptions = computed(() =>
    accounts.value.map((item) => ({ label: item.label || item.username, value: item.value || item.id }))
  );
  const copySourceLabel = computed(() => {
    const row = copySource.value;
    if (!row) return '';
    return `${row.feiniuChannelTitle || '-'} / ${row.youbanAccountUsername || '-'}（${
      row.accountNoteCount || 0
    } 条）`;
  });
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
    { title: '账号笔记数', key: 'accountNoteCount', width: 120 },
    { title: '最后源时间', key: 'lastSourceUpdateTime', width: 170 },
    { title: '最后笔记ID', key: 'lastSourceNoteId', width: 120 },
    { title: '状态', key: 'syncStatus', width: 100, render: (row) => tag(row.syncStatus) },
    { title: '错误', key: 'errorMessage', ellipsis: { tooltip: true } },
    {
      title: '操作',
      key: 'actions',
      width: 150,
      fixed: 'right',
      render(row) {
        return h(
          NSpace,
          { size: 8 },
          {
            default: () => [
              h(
                NButton,
                {
                  size: 'small',
                  type: 'primary',
                  disabled: !row.youbanAccountId,
                  onClick: () => openCopy(row),
                },
                { default: () => '复制' }
              ),
              h(
                NButton,
                {
                  size: 'small',
                  type: 'error',
                  disabled: !row.youbanAccountId,
                  onClick: () => handleDisable(row),
                },
                { default: () => '停用' }
              ),
            ],
          }
        );
      },
    },
  ];

  async function loadConfigs() {
    const res = await ConfigList({ page: 1, pageSize: 100 });
    configs.value = res.list || [];
  }
  async function loadTenants() {
    const res = await TenantOptions({});
    tenants.value = res.list || [];
  }
  async function loadAccounts(tenantId?: number | null) {
    accounts.value = [];
    copyForm.targetAccountId = null;
    if (!tenantId) return;
    accountLoading.value = true;
    try {
      const res = await AccountOptions({ tenantId });
      accounts.value = (res.list || []).filter((item) => item.id !== copySource.value?.youbanAccountId);
    } finally {
      accountLoading.value = false;
    }
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
  async function openCopy(row) {
    copySource.value = row;
    copyForm.targetTenantId = row.youbanTenantId || null;
    copyForm.targetAccountId = null;
    copyVisible.value = true;
    if (tenants.value.length === 0) {
      await loadTenants();
    }
    await loadAccounts(copyForm.targetTenantId);
  }
  function handleTargetTenantChange(value) {
    loadAccounts(value);
  }
  async function handleCopySubmit() {
    const row = copySource.value;
    if (!row) return;
    if (!copyForm.targetTenantId || !copyForm.targetAccountId) {
      message.warning('请选择目标租户和目标账号');
      return;
    }
    copying.value = true;
    try {
      const res = await ChannelCopy({
        channelMapId: row.id,
        configId: row.configId,
        youbanAccountId: row.youbanAccountId,
        targetTenantId: copyForm.targetTenantId,
        targetAccountId: copyForm.targetAccountId,
      });
      message.success(
        `复制完成：资料 ${res.profileCount || 0} 条，任务 ${res.taskCount || 0} 条，媒体 ${
          res.mediaCount || 0
        } 条`
      );
      copyVisible.value = false;
      await loadData();
    } finally {
      copying.value = false;
    }
  }
  function handleDisable(row) {
    dialog.warning({
      title: '停用频道账号',
      content: `确认停用「${row.youbanAccountUsername || row.youbanAccountId}」吗？停用后前台上架端不可见该账号，该账号下同步来的资料、任务和媒体也会隐藏。`,
      positiveText: '确认停用',
      negativeText: '取消',
      onPositiveClick: async () => {
        const res = await ChannelDisable({
          channelMapId: row.id,
          configId: row.configId,
          youbanAccountId: row.youbanAccountId,
        });
        message.success(
          `停用完成：资料 ${res.profileCount || 0} 条，任务 ${res.taskCount || 0} 条，媒体 ${
            res.mediaCount || 0
          } 条`
        );
        await loadData();
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
  .copy-modal {
    width: 520px;
  }
</style>
