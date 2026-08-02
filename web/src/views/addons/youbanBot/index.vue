<template>
  <div class="youban-bot-page">
    <n-card :bordered="false" title="全局 Bot 管理">
      <n-tabs v-model:value="activeTab" type="line" animated>
        <n-tab-pane name="bot" tab="Bot 配置">
          <n-space class="mb-4" justify="space-between">
            <n-space>
              <n-input v-model:value="botQuery.keyword" clearable placeholder="搜索名称/用户名" />
              <n-select
                v-model:value="botQuery.isOfficial"
                clearable
                :options="officialOptions"
                placeholder="官方机器人"
                class="youban-bot-page__select--official"
              />
              <n-select
                v-model:value="botQuery.status"
                clearable
                :options="statusOptions"
                placeholder="状态"
                class="youban-bot-page__select--status"
              />
              <n-button @click="loadBots">查询</n-button>
            </n-space>
            <n-space>
              <n-button @click="refreshBots">刷新状态</n-button>
              <n-button @click="restartBots">重启全部</n-button>
              <n-button type="primary" @click="openBotModal()">新增 Bot</n-button>
            </n-space>
          </n-space>

          <n-data-table
            :columns="botColumns"
            :data="botRows"
            :loading="botLoading"
            :pagination="botPagination"
            remote
            @update:page="handleBotPage"
            @update:page-size="handleBotPageSize"
          />
        </n-tab-pane>

        <n-tab-pane name="feature" tab="插件开关与配置">
          <n-space class="mb-4" justify="space-between">
            <n-space>
              <n-input v-model:value="featureQuery.keyword" clearable placeholder="搜索插件/命令" />
              <n-select
                v-model:value="featureQuery.status"
                clearable
                :options="statusOptions"
                placeholder="状态"
                class="youban-bot-page__select--status"
              />
              <n-button @click="loadFeatures">查询</n-button>
            </n-space>
            <n-button @click="loadFeatures">刷新</n-button>
          </n-space>

          <n-data-table
            :columns="featureColumns"
            :data="featureRows"
            :loading="featureLoading"
            :pagination="featurePagination"
            remote
            @update:page="handleFeaturePage"
            @update:page-size="handleFeaturePageSize"
          />
        </n-tab-pane>

        <n-tab-pane name="binding" tab="TG 绑定">
          <account-bind-panel />
        </n-tab-pane>

        <n-tab-pane name="user" tab="用户与消息">
          <user-message-panel />
        </n-tab-pane>
      </n-tabs>
    </n-card>

    <n-modal
      v-model:show="botModalVisible"
      preset="dialog"
      :title="botForm.id ? '编辑 Bot' : '新增 Bot'"
      positive-text="保存"
      negative-text="取消"
      @positive-click="saveBot"
    >
      <n-form :model="botForm" label-placement="left" label-width="110">
        <n-form-item label="Bot 名称">
          <n-input v-model:value="botForm.botName" clearable placeholder="可为空，自动读取" />
        </n-form-item>
        <n-form-item label="Bot Token">
          <n-input
            v-model:value="botForm.botToken"
            clearable
            show-password-on="click"
            type="password"
            :placeholder="botForm.id ? '不填则保留原 Token' : '请输入 Bot Token'"
          />
        </n-form-item>
        <n-form-item label="官方机器人">
          <n-switch
            v-model:value="botForm.isOfficial"
            :checked-value="1"
            :unchecked-value="0"
            checked-text="开启"
            unchecked-text="关闭"
            @update:value="handleOfficialChange"
          />
        </n-form-item>
        <n-form-item v-if="botForm.isOfficial === 1" label="默认机器人">
          <n-switch
            v-model:value="botForm.isDefault"
            :checked-value="1"
            :unchecked-value="0"
            checked-text="开启"
            unchecked-text="关闭"
          />
        </n-form-item>
        <n-form-item label="运行模式">
          <n-select v-model:value="botForm.runMode" :options="runModeOptions" />
        </n-form-item>
        <n-form-item label="Webhook地址">
          <n-input
            v-model:value="botForm.webhookUrl"
            clearable
            placeholder="为空自动使用全局官网域名"
          />
        </n-form-item>
        <n-form-item label="状态">
          <n-select v-model:value="botForm.status" :options="statusOptions" />
        </n-form-item>
        <n-form-item label="备注">
          <n-input
            v-model:value="botForm.remark"
            type="textarea"
            :autosize="{ minRows: 2, maxRows: 4 }"
          />
        </n-form-item>
      </n-form>
    </n-modal>

    <feature-config-modal
      v-model:show="featureModalVisible"
      :form="featureForm"
      :status-options="statusOptions"
      @save="saveFeature"
    />
  </div>
</template>

<script setup lang="ts">
  import { h, onMounted, reactive, ref } from 'vue';
  import { NButton, NPopconfirm, NSpace, NSwitch, NTag, useMessage } from 'naive-ui';

  import {
    BotDelete,
    BotList,
    BotRefresh,
    BotRestart,
    BotSave,
    FeatureList,
    FeatureSave,
  } from '@/api/addons/youbanBot';
  import FeatureConfigModal from './components/feature-config-modal.vue';
  import AccountBindPanel from './components/account-bind-panel.vue';
  import UserMessagePanel from './components/user-message-panel.vue';

  const message = useMessage();
  const activeTab = ref('bot');
  const botLoading = ref(false);
  const featureLoading = ref(false);
  const botModalVisible = ref(false);
  const featureModalVisible = ref(false);
  const botRows = ref<any[]>([]);
  const featureRows = ref<any[]>([]);

  const statusOptions = [
    { label: '启用', value: 1 },
    { label: '停用', value: 2 },
  ];
  const officialOptions = [
    { label: '是', value: 1 },
    { label: '否', value: 2 },
  ];
  const runModeOptions = [
    { label: '自动：线上 Webhook / 本地 Polling', value: 'auto' },
    { label: 'Webhook', value: 'webhook' },
    { label: 'Polling', value: 'polling' },
  ];

  const botQuery = reactive({
    page: 1,
    perPage: 10,
    keyword: '',
    isOfficial: null as number | null,
    status: null as number | null,
  });
  const featureQuery = reactive({
    page: 1,
    perPage: 10,
    keyword: '',
    status: null as number | null,
  });
  const botPagination = reactive({
    page: 1,
    pageSize: 10,
    itemCount: 0,
    showSizePicker: true,
    pageSizes: [10, 20, 50],
  });
  const featurePagination = reactive({
    page: 1,
    pageSize: 10,
    itemCount: 0,
    showSizePicker: true,
    pageSizes: [10, 20, 50],
  });
  const botForm = reactive(newBotForm());
  const featureForm = reactive(newFeatureForm());

  const botColumns = [
    { title: 'ID', key: 'id', width: 80 },
    { title: '名称', key: 'botName', minWidth: 160 },
    {
      title: '用户名',
      key: 'botUsername',
      minWidth: 160,
      render: (row: any) => (row.botUsername ? `@${row.botUsername}` : '-'),
    },
    {
      title: '官方机器人',
      key: 'isOfficial',
      width: 120,
      render: (row: any) => renderTag(row.isOfficial === 1, '是', '否'),
    },
    {
      title: '默认机器人',
      key: 'isDefault',
      width: 120,
      render: (row: any) => renderTag(row.isDefault === 1, '默认', '否'),
    },
    { title: '运行模式', key: 'runMode', width: 110, render: (row: any) => row.runMode || 'auto' },
    { title: '状态', key: 'status', width: 100, render: (row: any) => renderStatus(row.status) },
    { title: '更新时间', key: 'updatedAt', width: 170 },
    {
      title: '操作',
      key: 'actions',
      width: 180,
      render: (row: any) =>
        h(NSpace, { size: 8 }, () => [
          h(
            NButton,
            { size: 'small', onClick: () => openBotModal(row) },
            { default: () => '编辑' }
          ),
          h(
            NButton,
            { size: 'small', type: 'primary', onClick: () => restartBot(row) },
            { default: () => '重启' }
          ),
          h(
            NPopconfirm,
            { onPositiveClick: () => deleteBot(row) },
            {
              trigger: () =>
                h(NButton, { size: 'small', type: 'error' }, { default: () => '删除' }),
              default: () => '确认删除该 Bot？',
            }
          ),
        ]),
    },
  ];

  const featureColumns = [
    { title: '插件 Key', key: 'featureKey', minWidth: 140 },
    { title: '名称', key: 'name', minWidth: 150 },
    {
      title: '命令',
      key: 'command',
      width: 120,
      render: (row: any) => (row.command ? `/${row.command}` : '-'),
    },
    { title: '状态', key: 'status', width: 100, render: (row: any) => renderFeatureSwitch(row) },
    { title: '排序', key: 'sort', width: 90 },
    { title: '说明', key: 'description', minWidth: 220, ellipsis: { tooltip: true } },
    {
      title: '操作',
      key: 'actions',
      width: 100,
      render: (row: any) =>
        h(
          NButton,
          { size: 'small', onClick: () => openFeatureModal(row) },
          { default: () => '配置' }
        ),
    },
  ];

  function renderTag(active: boolean, yes: string, no: string) {
    return h(
      NTag,
      { type: active ? 'success' : 'default' },
      { default: () => (active ? yes : no) }
    );
  }

  function renderStatus(status: number) {
    return h(
      NTag,
      { type: status === 1 ? 'success' : 'warning' },
      { default: () => (status === 1 ? '启用' : '停用') }
    );
  }

  function renderFeatureSwitch(row: any) {
    return h(NSwitch, {
      value: row.status,
      checkedValue: 1,
      uncheckedValue: 2,
      onUpdateValue: async (value: number) => {
        await FeatureSave({ ...row, status: value });
        message.success('插件状态已更新');
        await loadFeatures();
      },
    });
  }

  async function loadBots() {
    botLoading.value = true;
    try {
      const res: any = await BotList({
        ...botQuery,
        page: botPagination.page,
        perPage: botPagination.pageSize,
      });
      botRows.value = res?.list || [];
      botPagination.itemCount = res?.totalCount || res?.total || 0;
    } finally {
      botLoading.value = false;
    }
  }

  async function loadFeatures() {
    featureLoading.value = true;
    try {
      const res: any = await FeatureList({
        ...featureQuery,
        page: featurePagination.page,
        perPage: featurePagination.pageSize,
      });
      featureRows.value = res?.list || [];
      featurePagination.itemCount = res?.totalCount || res?.total || 0;
    } finally {
      featureLoading.value = false;
    }
  }

  function handleBotPage(page: number) {
    botPagination.page = page;
    loadBots();
  }

  function handleBotPageSize(pageSize: number) {
    botPagination.pageSize = pageSize;
    botPagination.page = 1;
    loadBots();
  }

  function handleFeaturePage(page: number) {
    featurePagination.page = page;
    loadFeatures();
  }

  function handleFeaturePageSize(pageSize: number) {
    featurePagination.pageSize = pageSize;
    featurePagination.page = 1;
    loadFeatures();
  }

  function openBotModal(row?: any) {
    Object.assign(botForm, newBotForm(), row || {});
    botModalVisible.value = true;
  }

  function openFeatureModal(row: any) {
    Object.assign(featureForm, newFeatureForm(), row || {});
    featureForm.configValues = { ...(row?.configValues || parseJson(row?.configJson)) };
    featureForm.configSchema = ensureFeatureConfigSchema(row?.configSchema || []);
    if (featureForm.configValues.menuVisible === undefined) {
      featureForm.configValues.menuVisible = 1;
    }
    featureForm.configJson = JSON.stringify(featureForm.configValues, null, 2);
    featureModalVisible.value = true;
  }

  function handleOfficialChange(value: number) {
    if (value !== 1) {
      botForm.isDefault = 0;
    }
  }

  async function saveBot() {
    await BotSave({ ...botForm });
    message.success('保存成功');
    botModalVisible.value = false;
    await loadBots();
  }

  async function deleteBot(row: any) {
    await BotDelete({ ids: [row.id] });
    message.success('删除成功');
    await loadBots();
  }

  async function refreshBots() {
    await BotRefresh({ ids: botRows.value.map((item) => item.id) });
    message.success('刷新完成');
    await loadBots();
  }

  async function restartBot(row: any) {
    await BotRestart({ ids: [row.id] });
    message.success('重启完成');
    await loadBots();
  }

  async function restartBots() {
    await BotRestart({ ids: botRows.value.map((item) => item.id) });
    message.success('重启完成');
    await loadBots();
  }

  async function saveFeature() {
    featureForm.configJson = JSON.stringify(featureForm.configValues || {}, null, 2);
    await FeatureSave(featureForm);
    message.success('保存成功');
    featureModalVisible.value = false;
    await loadFeatures();
  }

  function newBotForm() {
    return {
      id: 0,
      botName: '',
      botUsername: '',
      botToken: '',
      isOfficial: 0,
      isDefault: 0,
      remark: '',
      runMode: 'auto',
      webhookUrl: '',
      status: 1,
    };
  }

  function ensureFeatureConfigSchema(schema: any[]) {
    const list = Array.isArray(schema) ? [...schema] : [];
    if (!list.some((item) => item?.field === 'menuVisible')) {
      list.unshift({
        field: 'menuVisible',
        label: '菜单可见',
        component: 'switch',
        default: 1,
        placeholder: '关闭后不在机器人底部菜单显示',
      });
    }
    return list;
  }

  function parseJson(text: string) {
    try {
      return text ? JSON.parse(text) : {};
    } catch {
      return {};
    }
  }

  function newFeatureForm() {
    return {
      id: 0,
      featureKey: '',
      name: '',
      command: '',
      description: '',
      configJson: '{}',
      configSchema: [] as any[],
      configValues: {} as Record<string, any>,
      sort: 0,
      status: 1,
    };
  }

  onMounted(() => {
    loadBots();
    loadFeatures();
  });
</script>

<style scoped>
  .youban-bot-page {
    padding: 16px;
  }

  .youban-bot-page__select--official {
    width: 140px;
  }

  .youban-bot-page__select--status {
    width: 120px;
  }
</style>
