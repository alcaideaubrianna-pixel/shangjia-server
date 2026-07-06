<template>
  <n-tabs v-model:value="activeTab" type="line" animated>
    <n-tab-pane name="tasks" tab="导入任务">
      <n-space class="toolbar" align="center">
        <n-select v-model:value="query.tenantId" :options="tenantOptionsWithAll" clearable filterable placeholder="账号归属" class="tenant-select" @update:value="handleQueryTenantChange" />
        <n-select v-model:value="query.accountId" :options="queryAccountOptionsWithAll" clearable filterable placeholder="上架账号" class="tenant-select" />
        <n-input v-model:value="query.keyword" placeholder="域名 / 账号 / 备注" clearable @keyup.enter="loadTasks" />
        <n-button @click="loadTasks">查询</n-button>
        <n-button type="primary" @click="openCreateModal">新建导入</n-button>
        <n-button @click="loadTasks">刷新</n-button>
      </n-space>
      <n-data-table :columns="taskColumns" :data="tasks" :loading="loading" :pagination="taskPagination" :row-key="(row) => row.id" :scroll-x="1320" size="small" remote />
    </n-tab-pane>

    <n-tab-pane name="runs" tab="导入记录">
      <n-space class="toolbar" align="center">
        <n-select v-model:value="runQuery.tenantId" :options="tenantOptionsWithAll" clearable filterable placeholder="账号归属" class="tenant-select" @update:value="handleRunTenantChange" />
        <n-select v-model:value="runQuery.accountId" :options="runAccountOptionsWithAll" clearable filterable placeholder="上架账号" class="tenant-select" />
        <n-select v-model:value="runQuery.runType" :options="runTypeOptionsWithAll" clearable placeholder="类型" class="status-select" />
        <n-select v-model:value="runQuery.status" :options="statusOptionsWithAll" clearable placeholder="状态" class="status-select" />
        <n-input v-model:value="runQuery.keyword" placeholder="域名 / 账号" clearable @keyup.enter="loadRuns" />
        <n-button @click="loadRuns">查询</n-button>
        <n-button @click="loadRuns">刷新</n-button>
      </n-space>
      <n-data-table :columns="runColumns" :data="runs" :loading="runLoading" :pagination="runPagination" :row-key="(row) => row.id" :scroll-x="1720" size="small" remote />
    </n-tab-pane>
  </n-tabs>

  <n-modal v-model:show="modalVisible" preset="dialog" :title="taskModalTitle" positive-text="保存" negative-text="取消" @positive-click="createTask">
    <n-form :model="form" label-placement="left" label-width="110">
      <n-form-item label="账号归属">
        <n-select v-model:value="form.tenantId" :options="tenantOptions" filterable placeholder="请选择账号归属" @update:value="handleFormTenantChange" />
      </n-form-item>
      <n-form-item label="导入账号">
        <n-select v-model:value="form.accountId" :options="formAccountOptions" filterable placeholder="请选择导入到哪个上架账号" />
      </n-form-item>
      <n-form-item label="旧站域名">
        <n-input v-model:value="form.baseUrl" clearable placeholder="https://example.com" />
      </n-form-item>
      <n-form-item label="旧站账号">
        <n-input v-model:value="form.username" clearable />
      </n-form-item>
      <n-form-item label="旧站密码">
        <n-input v-model:value="form.password" type="password" show-password-on="click" :placeholder="form.id ? '留空表示不修改密码' : ''" />
      </n-form-item>
      <n-form-item label="测试数量">
        <n-input-number v-model:value="form.limitCount" :min="0" :max="100000" class="w-full" />
      </n-form-item>
      <n-form-item label="每页数量">
        <n-input-number v-model:value="form.perPage" :min="1" :max="12" class="w-full" />
      </n-form-item>
      <n-form-item label="媒体并发">
        <n-input-number v-model:value="form.mediaConcurrency" :min="1" :max="20" class="w-full" />
      </n-form-item>
      <n-form-item label="导入方式">
        <n-radio-group v-model:value="form.importMode">
          <n-space>
            <n-radio value="incremental">增量更新</n-radio>
            <n-radio value="overwrite">覆盖更新</n-radio>
          </n-space>
        </n-radio-group>
      </n-form-item>
      <n-form-item label="代理池">
        <n-space vertical class="w-full">
          <n-switch v-model:value="proxyEnabled" />
          <n-input v-model:value="form.proxyPool" type="textarea" :autosize="{ minRows: 3, maxRows: 6 }" placeholder="一行一个代理地址" />
        </n-space>
      </n-form-item>
      <n-form-item label="备注">
        <n-input v-model:value="form.remark" type="textarea" :autosize="{ minRows: 2, maxRows: 4 }" />
      </n-form-item>
    </n-form>
  </n-modal>

  <n-modal v-model:show="runModalVisible" preset="dialog" title="创建导入记录" positive-text="创建并入队" negative-text="取消" @positive-click="createRun">
    <n-form :model="runForm" label-placement="left" label-width="90">
      <n-form-item label="执行类型">
        <n-radio-group v-model:value="runForm.runType">
          <n-space>
            <n-radio value="import">导入</n-radio>
            <n-radio value="repair">补全</n-radio>
            <n-radio value="scan">仅扫描</n-radio>
          </n-space>
        </n-radio-group>
      </n-form-item>
      <n-form-item label="扫描范围">
        <n-radio-group v-model:value="runForm.scanMode">
          <n-space>
            <n-radio value="recent">最近N个</n-radio>
            <n-radio value="all">全量扫描</n-radio>
          </n-space>
        </n-radio-group>
      </n-form-item>
      <n-form-item v-if="runForm.scanMode === 'recent'" label="最近数量">
        <n-input-number v-model:value="runForm.recentCount" :min="1" :max="2000" class="w-full" />
      </n-form-item>
      <n-form-item label="导入方式">
        <n-radio-group v-model:value="runForm.importMode">
          <n-space>
            <n-radio value="incremental">增量更新</n-radio>
            <n-radio value="overwrite">覆盖更新</n-radio>
          </n-space>
        </n-radio-group>
      </n-form-item>
    </n-form>
  </n-modal>

  <n-modal v-model:show="logModalVisible" preset="card" title="执行日志" class="log-modal">
    <n-collapse v-if="groupedLogs.length" class="log-groups">
      <n-collapse-item v-for="group in groupedLogs" :key="group.sourceNoteId" :title="group.title" :name="group.sourceNoteId">
        <n-data-table
          :columns="logColumns"
          :data="group.items"
          :pagination="false"
          :scroll-x="1040"
          size="small"
        />
      </n-collapse-item>
    </n-collapse>
    <n-data-table
      v-if="ungroupedLogs.length"
      :columns="logColumns"
      :data="ungroupedLogs"
      :loading="logLoading"
      :pagination="logPagination"
      :scroll-x="1040"
      :max-height="520"
      size="small"
    />
    <template #footer>
      <n-space justify="end">
        <n-button @click="logModalVisible = false">关闭</n-button>
        <n-button type="primary" @click="clearCurrentLogs">清理日志</n-button>
      </n-space>
    </template>
  </n-modal>
</template>

<script lang="ts" setup>
  import { computed, h, onMounted, reactive, ref, watch } from 'vue';
  import { NButton, NProgress, NSpace, NTag, useMessage } from 'naive-ui';
  import {
    AccountList,
    ImportRunCancel,
    ImportRunCreate,
    ImportRunDelete,
    ImportRunList,
    ImportRunLogClear,
    ImportRunLogList,
    ImportTaskCreate,
    ImportTaskList,
    ImportTaskView,
    TenantList,
  } from '@/api/addons/youbanPublish';

  const message = useMessage();
  const activeTab = ref('tasks');
  const loading = ref(false);
  const runLoading = ref(false);
  const logLoading = ref(false);
  const modalVisible = ref(false);
  const runModalVisible = ref(false);
  const logModalVisible = ref(false);
  const proxyEnabled = ref(false);
  const tasks = ref<Recordable[]>([]);
  const runs = ref<Recordable[]>([]);
  const logs = ref<Recordable[]>([]);
  const tenants = ref<Recordable[]>([]);
  const accounts = ref<Recordable[]>([]);
  const currentTaskId = ref<number | null>(null);
  const currentRunId = ref<number | null>(null);

  const query = reactive({ tenantId: null as number | null, accountId: null as number | null, keyword: '' });
  const runQuery = reactive({ tenantId: null as number | null, accountId: null as number | null, runType: undefined as string | undefined, status: undefined as string | undefined, keyword: '' });
  const form = reactive({
    id: null as number | null,
    sourceName: 'lyy_cms',
    tenantId: null as number | null,
    accountId: null as number | null,
    baseUrl: '',
    username: '',
    password: '',
    limitCount: 100,
    perPage: 12,
    mediaConcurrency: 4,
    importMode: 'incremental',
    proxyEnabled: 0,
    proxyPool: '',
    remark: '',
  });
  const runForm = reactive({ runType: 'scan', scanMode: 'recent', recentCount: 100, importMode: 'incremental' });

  const taskPagination = reactive({
    page: 1,
    pageSize: 20,
    itemCount: 0,
    showSizePicker: true,
    pageSizes: [10, 20, 50],
    onChange: (page: number) => {
      taskPagination.page = page;
      loadTasks();
    },
    onUpdatePageSize: (pageSize: number) => {
      taskPagination.pageSize = pageSize;
      taskPagination.page = 1;
      loadTasks();
    },
  });
  const runPagination = reactive({
    page: 1,
    pageSize: 20,
    itemCount: 0,
    showSizePicker: true,
    pageSizes: [10, 20, 50],
    onChange: (page: number) => {
      runPagination.page = page;
      loadRuns();
    },
    onUpdatePageSize: (pageSize: number) => {
      runPagination.pageSize = pageSize;
      runPagination.page = 1;
      loadRuns();
    },
  });
  const logPagination = reactive({ pageSize: 10 });
  const normalizedLogs = computed(() => logs.value.map((item) => ({ ...item, parsedContext: parseLogContext(item.context) })));
  const groupedLogs = computed(() => {
    const groups = new Map<number, Recordable[]>();
    normalizedLogs.value.forEach((item) => {
      const sourceNoteId = Number(item.parsedContext?.sourceNoteId || 0);
      if (sourceNoteId <= 0) return;
      if (!groups.has(sourceNoteId)) groups.set(sourceNoteId, []);
      groups.get(sourceNoteId)?.push(item);
    });
    return Array.from(groups.entries()).map(([sourceNoteId, items]) => ({
      sourceNoteId,
      title: `旧站笔记 #${sourceNoteId}（${items.length} 条）`,
      items,
    }));
  });
  const ungroupedLogs = computed(() => normalizedLogs.value.filter((item) => Number(item.parsedContext?.sourceNoteId || 0) <= 0));

  const statusOptions = [
    { label: '待执行', value: 'pending' },
    { label: '执行中', value: 'running' },
    { label: '成功', value: 'success' },
    { label: '失败', value: 'failed' },
    { label: '已取消', value: 'canceled' },
  ];
  const runTypeOptions = [
    { label: '导入', value: 'import' },
    { label: '补全', value: 'repair' },
    { label: '仅扫描', value: 'scan' },
  ];
  const statusOptionsWithAll = computed(() => [{ label: '全部状态', value: undefined }, ...statusOptions]);
  const runTypeOptionsWithAll = computed(() => [{ label: '全部类型', value: undefined }, ...runTypeOptions]);
  const tenantOptions = computed(() => tenants.value.map((item) => ({ label: accountOwnerName(item), value: item.id })));
  const tenantOptionsWithAll = computed(() => [{ label: '全部账号归属', value: null }, ...tenantOptions.value]);
  const queryAccountOptions = computed(() => accountOptionsByTenant(query.tenantId));
  const queryAccountOptionsWithAll = computed(() => [{ label: '全部上架账号', value: null }, ...queryAccountOptions.value]);
  const runAccountOptions = computed(() => accountOptionsByTenant(runQuery.tenantId));
  const runAccountOptionsWithAll = computed(() => [{ label: '全部上架账号', value: null }, ...runAccountOptions.value]);
  const formAccountOptions = computed(() => accountOptionsByTenant(form.tenantId));
  const taskModalTitle = computed(() => (form.id ? '修改导入任务' : '新建导入任务'));

  const taskColumns = [
    { title: 'ID', key: 'id', width: 80 },
    { title: '来源', key: 'sourceName', width: 110 },
    { title: '账号归属', key: 'tenantName', width: 150 },
    { title: '旧站域名', key: 'baseUrl', width: 220, ellipsis: { tooltip: true } },
    { title: '旧站账号', key: 'username', width: 140 },
    { title: '上架账号', key: 'accountName', width: 130 },
    { title: '测试数量', key: 'limitCount', width: 100 },
    { title: '每页数量', key: 'perPage', width: 100 },
    { title: '媒体并发', key: 'mediaConcurrency', width: 100 },
    { title: '备注', key: 'remark', width: 180, ellipsis: { tooltip: true } },
    { title: '更新时间', key: 'updatedAt', width: 180 },
    {
      title: '操作',
      key: 'actions',
      width: 220,
      fixed: 'right',
      render(row) {
        return h(NSpace, { size: 8 }, {
          default: () => [
            h(NButton, { size: 'small', onClick: () => openEditModal(row) }, { default: () => '修改' }),
            h(NButton, { size: 'small', onClick: () => openRunModal(row.id, 'scan') }, { default: () => '仅扫描' }),
            h(NButton, { size: 'small', type: 'primary', onClick: () => openRunModal(row.id, 'import') }, { default: () => '导入资料' }),
          ],
        });
      },
    },
  ];

  const runColumns = [
    { title: 'ID', key: 'id', width: 80 },
    { title: '任务ID', key: 'taskId', width: 90 },
    { title: '类型', key: 'runType', width: 90, render: (row) => runTypeOptions.find((item) => item.value === row.runType)?.label || row.runType },
    { title: '账号归属', key: 'tenantName', width: 150 },
    { title: '旧站域名', key: 'baseUrl', width: 220, ellipsis: { tooltip: true } },
    { title: '旧站账号', key: 'username', width: 140 },
    { title: '上架账号', key: 'accountName', width: 130 },
    {
      title: '状态',
      key: 'status',
      width: 100,
      render(row) {
        const map = { pending: ['default', '待执行'], running: ['warning', '执行中'], success: ['success', '成功'], failed: ['error', '失败'], canceled: ['default', '已取消'] };
        const item = map[row.status] || ['default', row.status || '-'];
        return h(NTag, { type: item[0] as any, bordered: false }, { default: () => item[1] });
      },
    },
    { title: '阶段', key: 'stage', width: 110 },
    { title: '进度', key: 'percent', width: 170, render: (row) => h(NProgress, { type: 'line', percentage: Math.min(100, Math.round(row.percent || 0)), indicatorPlacement: 'inside', processing: row.status === 'running' }) },
    { title: '资料', key: 'itemDone', width: 110, render: (row) => `${row.itemDone || 0}/${row.itemTotal || 0}` },
    { title: '媒体', key: 'mediaDone', width: 110, render: (row) => `${row.mediaDone || 0}/${row.mediaTotal || 0}` },
    { title: '未迁移存储', key: 'mediaMissingStorage', width: 120 },
    { title: 'TG匹配', key: 'tgMatched', width: 100 },
    { title: '错误', key: 'errorMessage', width: 240, ellipsis: { tooltip: true } },
    { title: '更新时间', key: 'updatedAt', width: 180 },
    {
      title: '操作',
      key: 'actions',
      width: 230,
      fixed: 'right',
      render(row) {
        return h(NSpace, { size: 8 }, {
          default: () => [
            h(NButton, { size: 'small', onClick: () => openLogs(row.id) }, { default: () => '日志' }),
            h(NButton, { size: 'small', disabled: row.status !== 'running' && row.status !== 'pending', onClick: () => cancelRun(row.id) }, { default: () => '取消' }),
            h(NButton, { size: 'small', disabled: row.status === 'running', onClick: () => retryRun(row) }, { default: () => '重试' }),
            h(NButton, { size: 'small', type: 'error', disabled: row.status === 'running', onClick: () => deleteRun(row.id) }, { default: () => '删除' }),
          ],
        });
      },
    },
  ];
  const logColumns = [
    { title: '时间', key: 'createdAt', width: 180 },
    { title: '级别', key: 'level', width: 80 },
    { title: '阶段', key: 'stage', width: 100 },
    { title: '内容', key: 'message', width: 520, ellipsis: { tooltip: true } },
    { title: '上下文', key: 'context', width: 160, ellipsis: { tooltip: true }, render: (row) => renderLogContext(row) },
  ];

  onMounted(async () => {
    await loadTenants();
    await loadAccounts();
    await loadTasks();
  });

  watch(activeTab, (tab) => {
    if (tab === 'runs') loadRuns();
  });

  async function loadTenants() {
    const res: any = await TenantList({ page: 1, perPage: 200, status: 1 });
    tenants.value = res?.list || [];
  }

  async function loadAccounts() {
    const res: any = await AccountList({ page: 1, perPage: 200, accountType: 'uploader', status: 1 });
    accounts.value = res?.list || [];
  }

  async function loadTasks() {
    loading.value = true;
    try {
      const res: any = await ImportTaskList({ ...query, page: taskPagination.page, perPage: taskPagination.pageSize });
      tasks.value = res?.list || [];
      taskPagination.itemCount = res?.totalCount || res?.total || 0;
    } finally {
      loading.value = false;
    }
  }

  async function loadRuns() {
    runLoading.value = true;
    try {
      const res: any = await ImportRunList({ ...runQuery, page: runPagination.page, perPage: runPagination.pageSize });
      runs.value = res?.list || [];
      runPagination.itemCount = res?.totalCount || res?.total || 0;
    } finally {
      runLoading.value = false;
    }
  }

  function openCreateModal() {
    resetTaskForm();
    if (!form.tenantId && tenants.value.length === 1) form.tenantId = tenants.value[0].id;
    modalVisible.value = true;
  }

  async function openEditModal(row: Recordable) {
    resetTaskForm();
    const detail: any = await ImportTaskView({ id: row.id });
    const data = detail || row;
    form.id = data.id;
    form.sourceName = data.sourceName || 'lyy_cms';
    form.tenantId = data.tenantId || null;
    form.accountId = data.accountId || null;
    form.baseUrl = data.baseUrl || '';
    form.username = data.username || '';
    form.password = '';
    form.limitCount = data.limitCount ?? 100;
    form.perPage = data.perPage ?? 12;
    form.mediaConcurrency = data.mediaConcurrency ?? 4;
    form.importMode = getTaskImportMode(data);
    form.proxyEnabled = data.proxyEnabled || 0;
    form.proxyPool = data.proxyPool || '';
    form.remark = data.remark || '';
    proxyEnabled.value = data.proxyEnabled === 1;
    modalVisible.value = true;
  }

  async function createTask() {
    if (!form.tenantId) {
      message.warning('请选择账号归属');
      return false;
    }
    if (!form.accountId) {
      message.warning('请选择导入账号');
      return false;
    }
    const payload: Recordable = { ...form, proxyEnabled: proxyEnabled.value ? 1 : 0 };
    if (payload.id && !payload.password) {
      delete payload.password;
    }
    await ImportTaskCreate(payload);
    message.success('导入任务已保存');
    await loadTasks();
  }

  function resetTaskForm() {
    form.id = null;
    form.sourceName = 'lyy_cms';
    form.tenantId = null;
    form.accountId = null;
    form.baseUrl = '';
    form.username = '';
    form.password = '';
    form.limitCount = 100;
    form.perPage = 12;
    form.mediaConcurrency = 4;
    form.importMode = 'incremental';
    form.proxyEnabled = 0;
    form.proxyPool = '';
    form.remark = '';
    proxyEnabled.value = false;
  }

  function getTaskImportMode(row: Recordable) {
    if (!row.resultJson) return 'incremental';
    try {
      const data = JSON.parse(row.resultJson);
      return data?.importMode === 'overwrite' ? 'overwrite' : 'incremental';
    } catch (e) {
      return 'incremental';
    }
  }

  function openRunModal(taskId: number, runType: string) {
    currentTaskId.value = taskId;
    runForm.runType = runType;
    runForm.scanMode = 'recent';
    runForm.recentCount = 100;
    runForm.importMode = 'incremental';
    runModalVisible.value = true;
  }

  async function createRun() {
    if (!currentTaskId.value) return false;
    await ImportRunCreate({ taskId: currentTaskId.value, ...runForm });
    message.success('导入记录已创建并入队');
    activeTab.value = 'runs';
    await loadRuns();
  }

  async function retryRun(row: Recordable) {
    await ImportRunCreate({ taskId: row.taskId, runType: row.runType, scanMode: row.scanMode, recentCount: row.recentCount, importMode: row.importMode });
    message.success('已创建新的重试记录');
    await loadRuns();
  }

  async function cancelRun(id: number) {
    await ImportRunCancel({ id });
    message.success('导入记录已取消');
    await loadRuns();
  }

  async function deleteRun(id: number) {
    await ImportRunDelete({ id });
    message.success('导入记录已删除');
    await loadRuns();
  }

  async function openLogs(id: number) {
    currentRunId.value = id;
    logModalVisible.value = true;
    await loadLogs();
  }

  async function loadLogs() {
    if (!currentRunId.value) return;
    logLoading.value = true;
    try {
      const res: any = await ImportRunLogList({ runId: currentRunId.value, page: 1, perPage: 200 });
      logs.value = res?.list || [];
    } finally {
      logLoading.value = false;
    }
  }

  function parseLogContext(value: string) {
    if (!value) return {};
    try {
      return JSON.parse(value);
    } catch (e) {
      return {};
    }
  }

  function renderLogContext(row: Recordable) {
    const context = row.parsedContext || parseLogContext(row.context);
    const parts = [];
    if (context.index && context.total) parts.push(`${context.index}/${context.total}`);
    if (context.mediaImported !== undefined && context.mediaTotal !== undefined) parts.push(`媒体 ${context.mediaImported}/${context.mediaTotal}`);
    if (context.size) parts.push(`${context.size}B`);
    if (context.path) parts.push(context.path);
    if (context.title) parts.push(context.title);
    return parts.join(' · ') || row.context || '';
  }

  async function clearCurrentLogs() {
    if (!currentRunId.value) return false;
    await ImportRunLogClear({ id: currentRunId.value });
    message.success('日志已清理');
    logs.value = [];
    return false;
  }

  function handleQueryTenantChange() {
    query.accountId = null;
  }

  function handleRunTenantChange() {
    runQuery.accountId = null;
  }

  function handleFormTenantChange() {
    form.accountId = null;
  }

  function accountOptionsByTenant(tenantId: number | null) {
    return accounts.value
      .filter((item) => !tenantId || item.tenantId === tenantId)
      .map((item) => ({ label: `${item.nickname || item.username} (${item.username})`, value: item.id }));
  }

  function accountOwnerName(item: Recordable) {
    return item.username ? `${item.name || item.tenantName || item.username} (${item.username})` : item.name || '-';
  }
</script>

<style scoped>
  :deep(.log-modal) {
    width: min(1080px, calc(100vw - 48px));
  }
</style>
