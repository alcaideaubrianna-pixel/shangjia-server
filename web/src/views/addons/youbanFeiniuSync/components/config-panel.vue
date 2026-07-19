<template>
  <n-space vertical>
    <n-space class="toolbar" align="center">
      <n-input
        v-model:value="query.keyword"
        placeholder="配置名称"
        clearable
        @keyup.enter="loadData"
      />
      <n-select
        v-model:value="query.status"
        :options="statusOptions"
        clearable
        placeholder="状态"
        class="status-select"
      />
      <n-button @click="loadData">查询</n-button>
      <n-button type="primary" @click="openModal()">新增配置</n-button>
    </n-space>
    <n-data-table
      :columns="columns"
      :data="list"
      :loading="loading"
      :row-key="(row) => row.id"
      :scroll-x="980"
      size="small"
    />
    <n-modal v-model:show="visible" preset="card" title="FeiNiu同步配置" class="config-modal">
      <n-form :model="form" label-placement="left" label-width="130">
        <n-grid :cols="2" :x-gap="16">
          <n-form-item-gi label="配置名称"><n-input v-model:value="form.name" /></n-form-item-gi>
          <n-form-item-gi label="数据库类型"
            ><n-select v-model:value="form.dbType" :options="dbTypeOptions"
          /></n-form-item-gi>
          <n-form-item-gi label="数据库地址"
            ><n-input v-model:value="form.dbHost"
          /></n-form-item-gi>
          <n-form-item-gi label="端口"
            ><n-input-number v-model:value="form.dbPort" class="w-full"
          /></n-form-item-gi>
          <n-form-item-gi label="数据库名"><n-input v-model:value="form.dbName" /></n-form-item-gi>
          <n-form-item-gi label="账号"><n-input v-model:value="form.dbUser" /></n-form-item-gi>
          <n-form-item-gi label="密码"
            ><n-input
              v-model:value="form.dbPassword"
              type="password"
              placeholder="编辑时为空则不修改"
          /></n-form-item-gi>
          <n-form-item-gi label="单批数量"
            ><n-input-number v-model:value="form.batchSize" class="w-full"
          /></n-form-item-gi>
          <n-form-item-gi label="目标租户">
            <n-select
              v-model:value="form.targetTenantId"
              :options="tenantOptions"
              clearable
              filterable
              placeholder="请选择上架租户"
              @update:value="handleTenantChange"
            />
          </n-form-item-gi>
          <n-form-item-gi label="管理员账号">
            <n-select
              v-model:value="form.targetParentAccountId"
              :options="adminAccountOptions"
              :disabled="!form.targetTenantId"
              clearable
              filterable
              placeholder="请选择管理员账号"
            />
          </n-form-item-gi>
          <n-form-item-gi label="自动创建账号"
            ><n-switch
              v-model:value="form.autoCreateAccount"
              :checked-value="1"
              :unchecked-value="2"
          /></n-form-item-gi>
          <n-form-item-gi label="同步媒体"
            ><n-switch v-model:value="form.syncMedia" :checked-value="1" :unchecked-value="2"
          /></n-form-item-gi>
          <n-form-item-gi label="同步验证资料"
            ><n-switch v-model:value="form.syncVerifyMedia" :checked-value="1" :unchecked-value="2"
          /></n-form-item-gi>
          <n-form-item-gi label="自动同步"
            ><n-switch v-model:value="form.autoSyncEnabled" :checked-value="1" :unchecked-value="2"
          /></n-form-item-gi>
          <n-form-item-gi label="同步间隔(分钟)"
            ><n-input-number
              v-model:value="form.syncIntervalMinutes"
              :min="1"
              :max="1440"
              class="w-full"
          /></n-form-item-gi>
          <n-form-item-gi label="状态"
            ><n-switch v-model:value="form.status" :checked-value="1" :unchecked-value="2"
          /></n-form-item-gi>
        </n-grid>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button :loading="testing" @click="testConfig">测试连接</n-button>
          <n-button @click="visible = false">取消</n-button>
          <n-button type="primary" :loading="saving" @click="saveConfig">保存</n-button>
        </n-space>
      </template>
    </n-modal>
  </n-space>
</template>

<script setup lang="ts">
  import { computed, h, onMounted, reactive, ref } from 'vue';
  import { NButton, NTag, useDialog, useMessage } from 'naive-ui';
  import {
    AdminAccountOptions,
    ConfigAutoSync,
    ConfigDelete,
    ConfigList,
    ConfigSave,
    ConfigTest,
    RunStart,
    TenantOptions,
  } from '@/api/addons/youbanFeiniuSync';

  const message = useMessage();
  const dialog = useDialog();
  const loading = ref(false);
  const saving = ref(false);
  const testing = ref(false);
  const visible = ref(false);
  const list = ref<any[]>([]);
  const tenants = ref<any[]>([]);
  const adminAccounts = ref<any[]>([]);
  const query = reactive({ keyword: '', status: null as number | null });
  const form = reactive<any>({});
  const dbTypeOptions = [
    { label: 'MySQL', value: 'mysql' },
    { label: 'PostgreSQL', value: 'pgsql' },
  ];
  const statusOptions = [
    { label: '启用', value: 1 },
    { label: '停用', value: 2 },
  ];
  const tenantOptions = computed(() =>
    tenants.value.map((item) => ({ label: item.label, value: item.value }))
  );
  const adminAccountOptions = computed(() =>
    adminAccounts.value
      .filter((item) => !form.targetTenantId || item.tenantId === form.targetTenantId)
      .map((item) => ({ label: item.label, value: item.value }))
  );
  const tenantNameMap = computed(() =>
    Object.fromEntries(tenants.value.map((item) => [item.id, item.label]))
  );
  const accountNameMap = computed(() =>
    Object.fromEntries(adminAccounts.value.map((item) => [item.id, item.label]))
  );
  const renderStatus = (status: number) =>
    h(
      NTag,
      { type: status === 1 ? 'success' : 'error', bordered: false },
      { default: () => (status === 1 ? '启用' : '停用') }
    );
  const renderAutoSync = (row) =>
    h('div', { class: 'sync-state' }, [
      h(
        NTag,
        {
          type: row.status === 1 && row.autoSyncEnabled === 1 ? 'success' : 'warning',
          bordered: false,
        },
        { default: () => (row.status === 1 && row.autoSyncEnabled === 1 ? '运行中' : '已暂停') }
      ),
      h('span', {}, `${row.syncIntervalMinutes || 10}分钟`),
    ]);

  const columns = [
    { title: '配置名称', key: 'name' },
    {
      title: 'FeiNiu数据库',
      key: 'db',
      render: (row) => `${row.dbType}://${row.dbHost}:${row.dbPort}/${row.dbName}`,
    },
    {
      title: '目标租户',
      key: 'targetTenantId',
      width: 180,
      render: (row) => tenantNameMap.value[row.targetTenantId] || row.targetTenantId || '-',
    },
    {
      title: '管理员账号',
      key: 'targetParentAccountId',
      width: 180,
      render: (row) =>
        accountNameMap.value[row.targetParentAccountId] || row.targetParentAccountId || '-',
    },
    {
      title: '自动同步',
      key: 'autoSyncEnabled',
      width: 110,
      render: (row) => renderAutoSync(row),
    },
    { title: '最近运行', key: 'lastRunAt', width: 170 },
    { title: '状态', key: 'status', width: 80, render: (row) => renderStatus(row.status) },
    {
      title: '操作',
      key: 'actions',
      width: 300,
      render: (row) =>
        h('div', { class: 'action-cell' }, [
          h(NButton, { size: 'small', onClick: () => openModal(row) }, { default: () => '编辑' }),
          h(
            NButton,
            {
              size: 'small',
              type: row.status === 1 && row.autoSyncEnabled === 1 ? 'warning' : 'success',
              ghost: row.status === 1 && row.autoSyncEnabled === 1,
              onClick: () => toggleAutoSync(row),
            },
            { default: () => (row.status === 1 && row.autoSyncEnabled === 1 ? '暂停' : '启动') }
          ),
          h(
            NButton,
            { size: 'small', type: 'primary', onClick: () => startRun(row.id) },
            { default: () => '同步' }
          ),
          h(
            NButton,
            { size: 'small', type: 'error', ghost: true, onClick: () => removeConfig(row.id) },
            { default: () => '删除' }
          ),
        ]),
    },
  ];

  function resetForm() {
    Object.assign(form, {
      id: 0,
      name: '',
      dbType: 'mysql',
      dbHost: '127.0.0.1',
      dbPort: 3306,
      dbName: 'ruoyi-fastapi',
      dbUser: 'root',
      dbPassword: '',
      targetTenantId: 0,
      targetParentAccountId: 0,
      autoCreateAccount: 1,
      syncMedia: 1,
      syncVerifyMedia: 1,
      autoSyncEnabled: 1,
      syncIntervalMinutes: 10,
      batchSize: 100,
      status: 1,
    });
  }
  async function openModal(row?: any) {
    resetForm();
    if (!tenants.value.length) await loadOptions();
    if (row) Object.assign(form, row, { dbPassword: '' });
    visible.value = true;
  }
  async function loadOptions() {
    const [tenantRes, accountRes] = await Promise.all([TenantOptions({}), AdminAccountOptions({})]);
    tenants.value = tenantRes.list || [];
    adminAccounts.value = accountRes.list || [];
  }
  async function handleTenantChange(value: number | null) {
    form.targetParentAccountId = 0;
    if (!value) return;
    const tenant = tenants.value.find((item) => item.id === value);
    await loadAdminAccounts(value);
    if (
      tenant?.adminAccountId &&
      adminAccounts.value.some((item) => item.id === tenant.adminAccountId)
    ) {
      form.targetParentAccountId = tenant.adminAccountId;
    }
  }
  async function loadAdminAccounts(tenantId?: number) {
    const res = await AdminAccountOptions({ tenantId });
    const others = adminAccounts.value.filter((item) => tenantId && item.tenantId !== tenantId);
    adminAccounts.value = [...others, ...(res.list || [])];
  }
  async function loadData() {
    loading.value = true;
    try {
      const res = await ConfigList({ ...query, page: 1, pageSize: 100 });
      list.value = res.list || [];
    } finally {
      loading.value = false;
    }
  }
  async function saveConfig() {
    saving.value = true;
    try {
      await ConfigSave(form);
      message.success('保存成功');
      visible.value = false;
      await loadData();
    } finally {
      saving.value = false;
    }
  }
  async function testConfig() {
    testing.value = true;
    try {
      const res = await ConfigTest(form);
      message.success(res.message || '连接成功');
    } finally {
      testing.value = false;
    }
  }
  async function startRun(id: number) {
    const res = await RunStart({ configId: id });
    message.success(`同步完成，运行ID：${res.runId}`);
    await loadData();
  }
  async function toggleAutoSync(row: any) {
    const running = row.status === 1 && row.autoSyncEnabled === 1;
    await ConfigAutoSync({ id: row.id, autoSyncEnabled: running ? 2 : 1 });
    message.success(running ? '已暂停自动同步' : '已启动自动同步');
    await loadData();
  }
  function removeConfig(id: number) {
    dialog.warning({
      title: '确认删除',
      content: '删除后配置不可恢复',
      positiveText: '删除',
      negativeText: '取消',
      onPositiveClick: async () => {
        await ConfigDelete({ ids: [id] });
        message.success('删除成功');
        await loadData();
      },
    });
  }
  onMounted(async () => {
    await loadOptions();
    await loadData();
  });
</script>

<style scoped>
  .toolbar {
    margin-bottom: 12px;
  }
  .status-select {
    width: 120px;
  }
  .config-modal {
    width: 760px;
  }
  .w-full {
    width: 100%;
  }
  :deep(.action-cell) {
    display: flex;
    gap: 8px;
  }
  :deep(.sync-state) {
    display: flex;
    align-items: center;
    gap: 6px;
  }
</style>
