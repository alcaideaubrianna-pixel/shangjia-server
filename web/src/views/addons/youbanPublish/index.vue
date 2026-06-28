<template>
  <div>
    <div class="n-layout-page-header">
      <n-card :bordered="false" title="上架系统">
        <span>管理租户、管理员账号、上架账号、上架任务和 Telegram Bot 配置</span>
      </n-card>
    </div>

    <n-card :bordered="false" class="proCard">
      <n-tabs v-model:value="activeTab" type="line" animated @update:value="handleTabChange">
        <n-tab-pane name="tenants" tab="租户">
          <n-space class="toolbar" align="center">
            <n-input v-model:value="tenantQuery.keyword" placeholder="租户名称 / 联系人" clearable @keyup.enter="loadTenants" />
            <n-select v-model:value="tenantQuery.status" :options="statusOptionsWithAll" clearable placeholder="状态" class="status-select" />
            <n-button @click="loadTenants">查询</n-button>
            <n-button type="primary" @click="openTenantModal()">新增租户</n-button>
          </n-space>
          <n-data-table
            :columns="tenantColumns"
            :data="tenants"
            :loading="tenantLoading"
            :pagination="tenantPagination"
            :row-key="(row) => row.id"
            :scroll-x="980"
            size="small"
            remote
          />
        </n-tab-pane>

        <n-tab-pane name="accounts" tab="账号">
          <n-space class="toolbar" align="center">
            <n-select v-model:value="accountQuery.tenantId" :options="tenantOptionsWithAll" clearable filterable placeholder="租户" class="tenant-select" />
            <n-select v-model:value="accountQuery.accountType" :options="accountTypeOptionsWithAll" clearable placeholder="账号类型" class="status-select" />
            <n-input v-model:value="accountQuery.keyword" placeholder="账号 / 昵称" clearable @keyup.enter="loadAccounts" />
            <n-button @click="loadAccounts">查询</n-button>
            <n-button type="primary" @click="openAccountModal()">新增账号</n-button>
          </n-space>
          <n-data-table
            :columns="accountColumns"
            :data="accounts"
            :loading="accountLoading"
            :pagination="accountPagination"
            :row-key="(row) => row.id"
            :scroll-x="1280"
            size="small"
            remote
          />
        </n-tab-pane>

        <n-tab-pane name="tasks" tab="任务">
          <n-space class="toolbar" align="center">
            <n-select v-model:value="taskQuery.tenantId" :options="tenantOptionsWithAll" clearable filterable placeholder="租户" class="tenant-select" />
            <n-select v-model:value="taskQuery.status" :options="taskStatusOptionsWithAll" clearable placeholder="任务状态" class="status-select" />
            <n-input v-model:value="taskQuery.keyword" placeholder="标题 / 请求ID" clearable @keyup.enter="loadTasks" />
            <n-button @click="loadTasks">查询</n-button>
          </n-space>
          <n-data-table
            :columns="taskColumns"
            :data="tasks"
            :loading="taskLoading"
            :pagination="taskPagination"
            :row-key="(row) => row.id"
            :scroll-x="1320"
            size="small"
            remote
          />
        </n-tab-pane>

        <n-tab-pane name="bots" tab="Bot">
          <n-space class="toolbar" align="center">
            <n-select v-model:value="botQuery.tenantId" :options="botTenantOptions" clearable filterable placeholder="租户" class="tenant-select" />
            <n-input v-model:value="botQuery.keyword" placeholder="Bot 名称 / 用户名" clearable @keyup.enter="loadBots" />
            <n-select v-model:value="botQuery.status" :options="statusOptionsWithAll" clearable placeholder="状态" class="status-select" />
            <n-button @click="loadBots">查询</n-button>
            <n-button type="primary" @click="openBotModal()">新增Bot</n-button>
          </n-space>
          <n-data-table
            :columns="botColumns"
            :data="bots"
            :loading="botLoading"
            :pagination="botPagination"
            :row-key="(row) => row.id"
            :scroll-x="980"
            size="small"
            remote
          />
        </n-tab-pane>

        <n-tab-pane name="config" tab="配置">
          <n-spin :show="configLoading">
            <n-space vertical class="config-section">
              <n-form :model="telegramConfig" label-placement="left" label-width="150">
                <n-form-item label="Telegram App ID">
                  <n-input-number v-model:value="telegramConfig.appId" :min="0" class="w-full" />
                </n-form-item>
                <n-form-item label="Telegram App Hash">
                  <n-input v-model:value="telegramConfig.appHash" clearable />
                </n-form-item>
                <n-form-item label="代理地址">
                  <n-input v-model:value="telegramConfig.proxyUrl" placeholder="socks5://127.0.0.1:7890" clearable />
                </n-form-item>
                <n-form-item label="Bot运行模式">
                  <n-select v-model:value="telegramConfig.botRuntimeMode" :options="botRuntimeOptions" />
                </n-form-item>
                <n-form-item label="Webhook Base URL">
                  <n-input v-model:value="telegramConfig.webhookBaseUrl" clearable />
                </n-form-item>
                <n-form-item label="Webhook Secret">
                  <n-input v-model:value="telegramConfig.webhookSecret" clearable />
                </n-form-item>
                <n-form-item label="默认推送 Chat ID">
                  <n-input v-model:value="telegramConfig.defaultTargetChat" clearable />
                </n-form-item>
              </n-form>
              <n-divider />
              <n-form :model="accountConfig" label-placement="left" label-width="150">
                <n-form-item label="默认角色ID">
                  <n-input-number v-model:value="accountConfig.defaultRoleId" :min="1" class="w-full" />
                </n-form-item>
                <n-form-item label="默认部门ID">
                  <n-input-number v-model:value="accountConfig.defaultDeptId" :min="1" class="w-full" />
                </n-form-item>
              </n-form>
              <n-space justify="end">
                <n-button @click="loadConfigs">重置</n-button>
                <n-button type="primary" :loading="configSaving" @click="saveConfigs">保存配置</n-button>
              </n-space>
            </n-space>
          </n-spin>
        </n-tab-pane>
      </n-tabs>
    </n-card>

    <n-modal v-model:show="tenantModalVisible" preset="dialog" title="租户" positive-text="保存" negative-text="取消" @positive-click="saveTenant">
      <n-form :model="tenantForm" label-placement="left" label-width="90">
        <n-form-item label="租户名称"><n-input v-model:value="tenantForm.name" clearable /></n-form-item>
        <n-form-item label="联系人"><n-input v-model:value="tenantForm.contactName" clearable /></n-form-item>
        <n-form-item label="联系电话"><n-input v-model:value="tenantForm.contactPhone" clearable /></n-form-item>
        <n-form-item label="状态"><n-select v-model:value="tenantForm.status" :options="statusOptions" /></n-form-item>
        <n-form-item label="备注"><n-input v-model:value="tenantForm.remark" type="textarea" :autosize="{ minRows: 2, maxRows: 4 }" /></n-form-item>
      </n-form>
    </n-modal>

    <n-modal v-model:show="accountModalVisible" preset="dialog" title="账号" positive-text="保存" negative-text="取消" @positive-click="saveAccount">
      <n-form :model="accountForm" label-placement="left" label-width="110">
        <n-form-item label="租户"><n-select v-model:value="accountForm.tenantId" :options="tenantOptions" filterable /></n-form-item>
        <n-form-item label="账号类型"><n-select v-model:value="accountForm.accountType" :options="accountTypeOptions" /></n-form-item>
        <n-form-item v-if="accountForm.accountType === 'uploader'" label="管理员账号">
          <n-select v-model:value="accountForm.parentId" :options="adminAccountOptions" filterable clearable />
        </n-form-item>
        <n-form-item label="登录账号"><n-input v-model:value="accountForm.username" clearable /></n-form-item>
        <n-form-item label="登录密码"><n-input v-model:value="accountForm.password" type="password" show-password-on="click" clearable placeholder="新增为空自动生成，编辑为空不修改" /></n-form-item>
        <n-form-item label="昵称"><n-input v-model:value="accountForm.nickname" clearable /></n-form-item>
        <n-form-item label="每日额度"><n-input-number v-model:value="accountForm.dailyPublishLimit" :min="0" class="w-full" /></n-form-item>
        <n-form-item label="直接发布"><n-switch v-model:value="accountForm.canDirectPublish" :checked-value="1" :unchecked-value="0" /></n-form-item>
        <n-form-item label="状态"><n-select v-model:value="accountForm.status" :options="statusOptions" /></n-form-item>
        <n-form-item label="备注"><n-input v-model:value="accountForm.remark" type="textarea" :autosize="{ minRows: 2, maxRows: 4 }" /></n-form-item>
      </n-form>
    </n-modal>

    <n-modal v-model:show="botModalVisible" preset="dialog" title="Bot配置" positive-text="保存" negative-text="取消" @positive-click="saveBot">
      <n-form :model="botForm" label-placement="left" label-width="100">
        <n-form-item label="租户"><n-select v-model:value="botForm.tenantId" :options="botTenantOptions" filterable clearable placeholder="不选表示全局默认" /></n-form-item>
        <n-form-item label="Bot名称"><n-input v-model:value="botForm.botName" clearable /></n-form-item>
        <n-form-item label="Bot Token"><n-input v-model:value="botForm.botToken" type="password" show-password-on="click" clearable /></n-form-item>
        <n-form-item label="状态"><n-select v-model:value="botForm.status" :options="statusOptions" /></n-form-item>
        <n-form-item label="备注"><n-input v-model:value="botForm.remark" type="textarea" :autosize="{ minRows: 2, maxRows: 4 }" /></n-form-item>
      </n-form>
    </n-modal>
  </div>
</template>

<script lang="ts" setup>
  import { computed, h, onMounted, reactive, ref } from 'vue';
  import { NButton, NSpace, NTag, useDialog, useMessage } from 'naive-ui';
  import {
    AccountDelete,
    AccountList,
    AccountSave,
    BotDelete,
    BotList,
    BotSave,
    ConfigGet,
    ConfigUpdate,
    TenantDelete,
    TenantList,
    TenantSave,
    TaskCancel,
    TaskList,
    TaskSubmit,
  } from '@/api/addons/youbanPublish';

  const dialog = useDialog();
  const message = useMessage();
  const activeTab = ref('tenants');

  const statusOptions = [
    { label: '启用', value: 1 },
    { label: '停用', value: 2 },
  ];
  const statusOptionsWithAll = [{ label: '全部', value: 0 }, ...statusOptions];
  const accountTypeOptions = [
    { label: '管理员账号', value: 'admin' },
    { label: '上架账号', value: 'uploader' },
  ];
  const accountTypeOptionsWithAll = [{ label: '全部', value: '' }, ...accountTypeOptions];
  const taskStatusOptions = [
    { label: '草稿', value: 'draft' },
    { label: '待发布', value: 'pending' },
    { label: '发布中', value: 'publishing' },
    { label: '已发布', value: 'published' },
    { label: '失败', value: 'failed' },
    { label: '已取消', value: 'canceled' },
  ];
  const taskStatusOptionsWithAll = [{ label: '全部', value: '' }, ...taskStatusOptions];
  const botRuntimeOptions = [
    { label: '自动', value: 'auto' },
    { label: 'Pull', value: 'pull' },
    { label: 'Webhook', value: 'webhook' },
  ];

  const tenants = ref<any[]>([]);
  const accounts = ref<any[]>([]);
  const tasks = ref<any[]>([]);
  const bots = ref<any[]>([]);

  const tenantLoading = ref(false);
  const accountLoading = ref(false);
  const taskLoading = ref(false);
  const botLoading = ref(false);
  const configLoading = ref(false);
  const configSaving = ref(false);

  const tenantModalVisible = ref(false);
  const accountModalVisible = ref(false);
  const botModalVisible = ref(false);

  const tenantQuery = reactive({ keyword: '', status: 0 });
  const accountQuery = reactive({ tenantId: null as number | null, accountType: '', keyword: '', status: 0 });
  const taskQuery = reactive({ tenantId: null as number | null, status: '', keyword: '' });
  const botQuery = reactive({ tenantId: null as number | null, keyword: '', status: 0 });

  const tenantPagination = createPagination(loadTenants);
  const accountPagination = createPagination(loadAccounts);
  const taskPagination = createPagination(loadTasks);
  const botPagination = createPagination(loadBots);

  const tenantOptions = computed(() => tenants.value.map((item) => ({ label: item.name, value: item.id })));
  const tenantOptionsWithAll = computed(() => [{ label: '全部租户', value: null }, ...tenantOptions.value]);
  const botTenantOptions = computed(() => [{ label: '全局默认', value: 0 }, ...tenantOptions.value]);
  const adminAccountOptions = computed(() =>
    accounts.value
      .filter((item) => item.accountType === 'admin' && item.tenantId === accountForm.tenantId)
      .map((item) => ({ label: `${item.nickname || item.username} (${item.username})`, value: item.id }))
  );

  const tenantForm = reactive(newTenantForm());
  const accountForm = reactive(newAccountForm());
  const botForm = reactive(newBotForm());
  const telegramConfig = reactive({
    appId: 0,
    appHash: '',
    proxyUrl: '',
    botRuntimeMode: 'auto',
    webhookBaseUrl: '',
    webhookSecret: '',
    defaultTargetChat: '',
  });
  const accountConfig = reactive({ defaultRoleId: 10, defaultDeptId: 1 });

  const tenantColumns = [
    { title: 'ID', key: 'id', width: 80 },
    { title: '租户名称', key: 'name', width: 180 },
    { title: '联系人', key: 'contactName', width: 140 },
    { title: '联系电话', key: 'contactPhone', width: 150 },
    { title: '状态', key: 'status', width: 100, render: (row) => renderStatus(row.status) },
    { title: '创建时间', key: 'createdAt', width: 170 },
    {
      title: '操作',
      key: 'actions',
      width: 160,
      fixed: 'right',
      render(row) {
        return h(NSpace, {}, { default: () => [actionButton('编辑', () => openTenantModal(row)), dangerButton('删除', () => deleteTenant(row.id))] });
      },
    },
  ];

  const accountColumns = [
    { title: 'ID', key: 'id', width: 80 },
    { title: '租户', key: 'tenantName', width: 160 },
    { title: '类型', key: 'accountType', width: 110, render: (row) => accountTypeLabel(row.accountType) },
    { title: '账号', key: 'username', width: 160 },
    { title: '昵称', key: 'nickname', width: 160 },
    { title: '每日额度', key: 'dailyPublishLimit', width: 100 },
    { title: '直接发布', key: 'canDirectPublish', width: 100, render: (row) => (row.canDirectPublish === 1 ? '是' : '否') },
    { title: '状态', key: 'status', width: 100, render: (row) => renderStatus(row.status) },
    { title: '更新时间', key: 'updatedAt', width: 170 },
    {
      title: '操作',
      key: 'actions',
      width: 160,
      fixed: 'right',
      render(row) {
        return h(NSpace, {}, { default: () => [actionButton('编辑', () => openAccountModal(row)), dangerButton('删除', () => deleteAccount(row.id))] });
      },
    },
  ];

  const taskColumns = [
    { title: 'ID', key: 'id', width: 80 },
    { title: '租户', key: 'tenantName', width: 150 },
    { title: '账号', key: 'accountUsername', width: 150 },
    { title: '标题', key: 'title', width: 220 },
    { title: '地区', key: 'city', width: 140, render: (row) => [row.province, row.city].filter(Boolean).join(' / ') || '-' },
    { title: '媒体', key: 'mediaCount', width: 80 },
    { title: '任务状态', key: 'status', width: 110, render: (row) => renderTaskStatus(row.status) },
    { title: 'TG状态', key: 'tgStatus', width: 100 },
    { title: '提交时间', key: 'submittedAt', width: 170 },
    {
      title: '操作',
      key: 'actions',
      width: 170,
      fixed: 'right',
      render(row) {
        return h(NSpace, {}, { default: () => [actionButton('提交', () => submitTask(row.id)), dangerButton('取消', () => cancelTask(row.id))] });
      },
    },
  ];

  const botColumns = [
    { title: 'ID', key: 'id', width: 80 },
    { title: '租户', key: 'tenantId', width: 160, render: (row) => tenantName(row.tenantId) },
    { title: 'Bot名称', key: 'botName', width: 180 },
    { title: '用户名', key: 'botUsername', width: 180 },
    { title: '状态', key: 'status', width: 100, render: (row) => renderStatus(row.status) },
    { title: '更新时间', key: 'updatedAt', width: 170 },
    {
      title: '操作',
      key: 'actions',
      width: 160,
      fixed: 'right',
      render(row) {
        return h(NSpace, {}, { default: () => [actionButton('编辑', () => openBotModal(row)), dangerButton('删除', () => deleteBot(row.id))] });
      },
    },
  ];

  onMounted(async () => {
    await loadTenants();
  });

  function createPagination(loader: () => void) {
    const pagination: any = {
      page: 1,
      pageSize: 10,
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

  async function handleTabChange(tab: string) {
    if (tab === 'accounts') await loadAccounts();
    if (tab === 'tasks') await loadTasks();
    if (tab === 'bots') await loadBots();
    if (tab === 'config') await loadConfigs();
  }

  async function loadTenants() {
    tenantLoading.value = true;
    try {
      const res: any = await TenantList({ ...tenantQuery, page: tenantPagination.page, perPage: tenantPagination.pageSize });
      tenants.value = res?.list || [];
      tenantPagination.itemCount = res?.totalCount || res?.total || 0;
    } finally {
      tenantLoading.value = false;
    }
  }

  async function loadAccounts() {
    accountLoading.value = true;
    try {
      const res: any = await AccountList({ ...accountQuery, page: accountPagination.page, perPage: accountPagination.pageSize });
      accounts.value = res?.list || [];
      accountPagination.itemCount = res?.totalCount || res?.total || 0;
    } finally {
      accountLoading.value = false;
    }
  }

  async function loadTasks() {
    taskLoading.value = true;
    try {
      const res: any = await TaskList({ ...taskQuery, page: taskPagination.page, perPage: taskPagination.pageSize });
      tasks.value = res?.list || [];
      taskPagination.itemCount = res?.totalCount || res?.total || 0;
    } finally {
      taskLoading.value = false;
    }
  }

  async function loadBots() {
    botLoading.value = true;
    try {
      const res: any = await BotList({ ...botQuery, page: botPagination.page, perPage: botPagination.pageSize });
      bots.value = res?.list || [];
      botPagination.itemCount = res?.totalCount || res?.total || 0;
    } finally {
      botLoading.value = false;
    }
  }

  async function loadConfigs() {
    configLoading.value = true;
    try {
      Object.assign(telegramConfig, await ConfigGet({ group: 'telegram' }));
      Object.assign(accountConfig, await ConfigGet({ group: 'account' }));
    } finally {
      configLoading.value = false;
    }
  }

  async function saveConfigs() {
    configSaving.value = true;
    try {
      await ConfigUpdate({ group: 'telegram', ...telegramConfig });
      await ConfigUpdate({ group: 'account', ...accountConfig });
      message.success('配置已保存');
    } finally {
      configSaving.value = false;
    }
  }

  function openTenantModal(row: any = null) {
    Object.assign(tenantForm, newTenantForm(), row || {});
    tenantModalVisible.value = true;
  }

  async function saveTenant() {
    await TenantSave(tenantForm);
    message.success('租户已保存');
    await loadTenants();
  }

  function openAccountModal(row: any = null) {
    Object.assign(accountForm, newAccountForm(), row || {});
    if (!accountForm.tenantId && tenants.value.length === 1) {
      accountForm.tenantId = tenants.value[0].id;
    }
    accountModalVisible.value = true;
  }

  async function saveAccount() {
    await AccountSave(accountForm);
    message.success('账号已保存');
    await loadAccounts();
  }

  function openBotModal(row: any = null) {
    Object.assign(botForm, newBotForm(), row || {});
    botModalVisible.value = true;
  }

  async function saveBot() {
    await BotSave(botForm);
    message.success('Bot已保存');
    await loadBots();
  }

  function deleteTenant(id: number) {
    confirmDelete('确认删除该租户？', async () => {
      await TenantDelete({ ids: [id] });
      await loadTenants();
    });
  }

  function deleteAccount(id: number) {
    confirmDelete('确认删除该账号？绑定的后台登录账号会同步停用。', async () => {
      await AccountDelete({ ids: [id] });
      await loadAccounts();
    });
  }

  function deleteBot(id: number) {
    confirmDelete('确认删除该 Bot？', async () => {
      await BotDelete({ ids: [id] });
      await loadBots();
    });
  }

  async function submitTask(id: number) {
    await TaskSubmit({ id });
    message.success('任务已提交');
    await loadTasks();
  }

  async function cancelTask(id: number) {
    await TaskCancel({ id });
    message.success('任务已取消');
    await loadTasks();
  }

  function confirmDelete(content: string, onConfirm: () => Promise<void>) {
    dialog.warning({
      title: '确认操作',
      content,
      positiveText: '确定',
      negativeText: '取消',
      onPositiveClick: async () => {
        await onConfirm();
        message.success('操作成功');
      },
    });
  }

  function actionButton(label: string, onClick: () => void) {
    return h(NButton, { size: 'small', quaternary: true, type: 'primary', onClick }, { default: () => label });
  }

  function dangerButton(label: string, onClick: () => void) {
    return h(NButton, { size: 'small', quaternary: true, type: 'error', onClick }, { default: () => label });
  }

  function renderStatus(status: number) {
    return h(NTag, { type: status === 1 ? 'success' : 'warning', bordered: false }, { default: () => (status === 1 ? '启用' : '停用') });
  }

  function renderTaskStatus(status: string) {
    const option = taskStatusOptions.find((item) => item.value === status);
    const type = status === 'published' ? 'success' : status === 'failed' ? 'error' : status === 'pending' ? 'warning' : 'default';
    return h(NTag, { type, bordered: false }, { default: () => option?.label || status || '-' });
  }

  function accountTypeLabel(type: string) {
    return accountTypeOptions.find((item) => item.value === type)?.label || type || '-';
  }

  function tenantName(id: number) {
    if (!id) return '全局默认';
    return tenants.value.find((item) => item.id === id)?.name || `租户 ${id}`;
  }

  function newTenantForm() {
    return { id: 0, name: '', contactName: '', contactPhone: '', remark: '', status: 1 };
  }

  function newAccountForm() {
    return {
      id: 0,
      tenantId: null,
      parentId: null,
      accountType: 'admin',
      username: '',
      password: '',
      nickname: '',
      dailyPublishLimit: 0,
      canDirectPublish: 0,
      remark: '',
      status: 1,
    };
  }

  function newBotForm() {
    return { id: 0, tenantId: 0, botName: '', botToken: '', remark: '', status: 1 };
  }
</script>

<style scoped>
  .toolbar {
    margin-bottom: 16px;
  }

  .status-select {
    width: 140px;
  }

  .tenant-select {
    width: 180px;
  }

  .config-section {
    max-width: 860px;
  }
</style>
