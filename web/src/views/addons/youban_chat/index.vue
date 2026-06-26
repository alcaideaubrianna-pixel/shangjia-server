<template>
  <div>
    <div class="n-layout-page-header">
      <n-card :bordered="false" title="悦伴客服">
        <span>管理 Telegram Bot、频道群绑定、客服绑定和会话记录</span>
      </n-card>
    </div>

    <n-card :bordered="false" class="proCard">
      <n-tabs v-model:value="activeTab" type="line" animated @update:value="handleTabChange">
        <n-tab-pane name="conversations" tab="会话">
          <BasicForm
            ref="searchFormRef"
            @register="register"
            @submit="reloadConversationTable"
            @reset="reloadConversationTable"
            @keyup.enter="reloadConversationTable"
          />
          <n-data-table
            :columns="columns"
            :data="conversations"
            :loading="conversationLoading"
            :pagination="conversationPagination"
            :row-key="(row) => row.id"
            :scroll-x="scrollX"
            size="small"
            remote
          />
        </n-tab-pane>

        <n-tab-pane name="bots" tab="Bot管理">
          <n-space class="toolbar" align="center">
            <n-input v-model:value="botQuery.keyword" placeholder="Bot名称 / 用户名" clearable @keyup.enter="loadBots" />
            <n-select v-model:value="botQuery.status" :options="allStatusOptions" clearable placeholder="状态" class="status-select" />
            <n-button @click="loadBots">查询</n-button>
            <n-button type="primary" @click="openBotModal()">新增Bot</n-button>
          </n-space>
          <n-data-table
            :columns="botColumns"
            :data="bots"
            :loading="botLoading"
            :pagination="botPagination"
            :row-key="(row) => row.id"
            size="small"
            remote
          />
        </n-tab-pane>

        <n-tab-pane name="bindings" tab="频道群绑定">
          <n-space class="toolbar" align="center">
            <n-input v-model:value="bindingQuery.keyword" placeholder="绑定码 / 频道 / TG群" clearable @keyup.enter="loadBindings" />
            <n-select v-model:value="bindingQuery.status" :options="allStatusOptions" clearable placeholder="状态" class="status-select" />
            <n-button @click="loadBindings">查询</n-button>
            <n-button type="primary" @click="openBindingModal()">生成绑定码</n-button>
          </n-space>
          <n-data-table
            :columns="bindingColumns"
            :data="bindings"
            :loading="bindingLoading"
            :pagination="bindingPagination"
            :row-key="(row) => row.id"
            size="small"
            remote
          />
        </n-tab-pane>

        <n-tab-pane name="operators" tab="客服绑定">
          <n-space class="toolbar" align="center">
            <n-input v-model:value="operatorQuery.keyword" placeholder="客服 / TG用户名" clearable @keyup.enter="loadOperators" />
            <n-select v-model:value="operatorQuery.status" :options="allStatusOptions" clearable placeholder="状态" class="status-select" />
            <n-button @click="loadOperators">查询</n-button>
            <n-button type="primary" @click="openOperatorModal()">新增客服</n-button>
          </n-space>
          <n-data-table
            :columns="operatorColumns"
            :data="operators"
            :loading="operatorLoading"
            :pagination="operatorPagination"
            :row-key="(row) => row.id"
            size="small"
            remote
          />
        </n-tab-pane>

        <n-tab-pane name="features" tab="功能配置">
          <n-space class="toolbar" align="center">
            <n-input v-model:value="featureQuery.keyword" placeholder="功能 / 命令" clearable @keyup.enter="loadFeatures" />
            <n-select v-model:value="featureQuery.status" :options="allStatusOptions" clearable placeholder="状态" class="status-select" />
            <n-button @click="loadFeatures">查询</n-button>
          </n-space>
          <n-data-table
            :columns="featureColumns"
            :data="features"
            :loading="featureLoading"
            :pagination="featurePagination"
            :row-key="(row) => row.id"
            size="small"
            remote
          />
        </n-tab-pane>
      </n-tabs>
    </n-card>

    <n-drawer v-model:show="detailVisible" :width="720" placement="right">
      <n-drawer-content :title="detailTitle" closable>
        <n-descriptions v-if="currentConversation" bordered :column="1" size="small">
          <n-descriptions-item label="会员">
            {{ currentConversation.memberRealName || currentConversation.memberUsername || currentConversation.memberMobile || '-' }}
          </n-descriptions-item>
          <n-descriptions-item label="资料">
            {{ currentConversation.profileNo || currentConversation.profileId }} {{ currentConversation.profileTitle || '' }}
          </n-descriptions-item>
          <n-descriptions-item label="会话标识">
            {{ currentConversation.chatSessionId || '-' }}
          </n-descriptions-item>
          <n-descriptions-item label="TG路由">
            {{ currentConversation.tgChatId || '-' }} / {{ currentConversation.tgMessageThreadId || '-' }}
          </n-descriptions-item>
          <n-descriptions-item label="状态">
            <n-tag :type="conversationStatusTag(currentConversation.status).type" :bordered="false">
              {{ conversationStatusTag(currentConversation.status).label }}
            </n-tag>
          </n-descriptions-item>
        </n-descriptions>

        <n-space v-if="currentConversation" class="detail-actions" justify="end">
          <n-button type="error" ghost :disabled="messageLoading" @click="handleClearConversation">清空聊天记录</n-button>
        </n-space>

        <n-divider />
        <n-spin :show="messageLoading">
          <n-empty v-if="messages.length === 0" description="暂无聊天记录" />
          <div v-else class="message-list">
            <div v-for="item in messages" :key="item.id" class="message-item" :class="`is-${item.direction}`">
              <div class="message-meta">
                <span>{{ item.senderName || directionLabel(item.direction) }}</span>
                <span>{{ item.createdAt }}</span>
              </div>
              <div class="message-content">{{ item.content || attachmentText(item) }}</div>
              <div v-if="item.attachments?.length" class="message-attachments">
                <template v-for="attachment in item.attachments" :key="attachment.id || attachment.dataUrl || attachment.fallbackUrl">
                  <img
                    v-if="attachment.fileType === 'image'"
                    class="message-image"
                    :src="attachment.dataUrl || attachment.fallbackUrl"
                    :alt="attachment.name || 'image'"
                  />
                  <video
                    v-else-if="attachment.fileType === 'video'"
                    class="message-video"
                    :src="attachment.dataUrl || attachment.fallbackUrl"
                    controls
                  />
                  <a v-else class="message-file" :href="attachment.dataUrl || attachment.fallbackUrl" target="_blank">
                    {{ attachment.name || '查看附件' }}
                  </a>
                </template>
              </div>
            </div>
          </div>
        </n-spin>
      </n-drawer-content>
    </n-drawer>

    <n-modal v-model:show="operatorModalVisible" preset="dialog" title="客服绑定" positive-text="保存" negative-text="取消" @positive-click="saveOperator">
      <n-form :model="operatorForm" label-placement="left" label-width="120">
        <n-form-item label="后台会员ID">
          <n-input-number v-model:value="operatorForm.adminMemberId" :min="0" class="w-full" />
        </n-form-item>
        <n-form-item label="TG用户ID">
          <n-input v-model:value="operatorForm.telegramUserId" clearable />
        </n-form-item>
        <n-form-item label="TG用户名">
          <n-input v-model:value="operatorForm.telegramUsername" clearable />
        </n-form-item>
        <n-form-item label="显示名称">
          <n-input v-model:value="operatorForm.displayName" clearable />
        </n-form-item>
        <n-form-item label="绑定码">
          <n-input v-model:value="operatorForm.bindCode" clearable />
        </n-form-item>
        <n-form-item label="状态">
          <n-select v-model:value="operatorForm.status" :options="statusOptions" />
        </n-form-item>
        <n-form-item label="备注">
          <n-input v-model:value="operatorForm.remark" type="textarea" :autosize="{ minRows: 2, maxRows: 4 }" />
        </n-form-item>
      </n-form>
    </n-modal>

    <n-modal v-model:show="botModalVisible" preset="dialog" title="Bot配置" positive-text="保存" negative-text="取消" @positive-click="saveBot">
      <n-form :model="botForm" label-placement="left" label-width="120">
        <n-form-item label="Bot名称">
          <n-input v-model:value="botForm.botName" clearable />
        </n-form-item>
        <n-form-item label="Bot Token">
          <n-input v-model:value="botForm.botToken" type="password" show-password-on="click" clearable />
        </n-form-item>
        <n-form-item label="状态">
          <n-select v-model:value="botForm.status" :options="statusOptions" />
        </n-form-item>
        <n-form-item label="备注">
          <n-input v-model:value="botForm.remark" type="textarea" :autosize="{ minRows: 2, maxRows: 4 }" />
        </n-form-item>
      </n-form>
    </n-modal>

    <n-modal v-model:show="bindingModalVisible" preset="dialog" title="频道群绑定" positive-text="保存" negative-text="取消" @positive-click="saveBinding">
      <n-form :model="bindingForm" label-placement="left" label-width="130">
        <n-form-item label="绑定类型">
          <n-select v-model:value="bindingForm.bindType" :options="bindingTypeOptions" />
        </n-form-item>
        <n-form-item v-if="bindingForm.bindType === 'channel'" label="关联频道">
          <n-space vertical class="w-full">
            <n-space>
              <n-button @click="openChannelPicker(true)">选择频道</n-button>
              <n-button v-if="bindingForm.channelIds.length" quaternary @click="bindingForm.channelIds = []">清空</n-button>
            </n-space>
            <div class="selected-channels">
              {{ selectedChannelText || '未选择频道' }}
            </div>
          </n-space>
        </n-form-item>
        <n-form-item label="状态">
          <n-select v-model:value="bindingForm.status" :options="statusOptions" />
        </n-form-item>
        <n-form-item label="备注">
          <n-input v-model:value="bindingForm.remark" type="textarea" :autosize="{ minRows: 2, maxRows: 4 }" />
        </n-form-item>
      </n-form>
    </n-modal>

    <n-modal v-model:show="channelPickerVisible" preset="dialog" title="选择频道" positive-text="确定" negative-text="取消" @positive-click="confirmChannelPicker">
      <n-space vertical>
        <n-input v-model:value="channelPickerKeyword" clearable placeholder="搜索频道" />
        <n-data-table
          :columns="channelPickerColumns"
          :data="filteredChannelOptions"
          :row-key="(row) => row.value"
          :checked-row-keys="channelPickerValue"
          size="small"
          max-height="420"
          @update:checked-row-keys="channelPickerValue = $event"
        />
      </n-space>
    </n-modal>

    <n-modal v-model:show="featureModalVisible" preset="dialog" title="功能配置" positive-text="保存" negative-text="取消" @positive-click="saveFeature">
      <n-form :model="featureForm" label-placement="left" label-width="110">
        <n-form-item label="功能Key">
          <n-input v-model:value="featureForm.featureKey" disabled />
        </n-form-item>
        <n-form-item label="功能名称">
          <n-input v-model:value="featureForm.name" clearable />
        </n-form-item>
        <n-form-item label="命令">
          <n-input v-model:value="featureForm.command" clearable />
        </n-form-item>
        <n-form-item label="描述">
          <n-input v-model:value="featureForm.description" clearable />
        </n-form-item>
        <n-form-item label="配置JSON">
          <n-input v-model:value="featureForm.configJson" type="textarea" :autosize="{ minRows: 4, maxRows: 10 }" />
        </n-form-item>
        <n-form-item label="排序">
          <n-input-number v-model:value="featureForm.sort" class="w-full" />
        </n-form-item>
        <n-form-item label="状态">
          <n-select v-model:value="featureForm.status" :options="statusOptions" />
        </n-form-item>
      </n-form>
    </n-modal>
  </div>
</template>

<script lang="ts" setup>
  import { computed, h, onMounted, reactive, ref } from 'vue';
  import { NButton, NTag, useDialog, useMessage } from 'naive-ui';
  import { BasicForm, useForm } from '@/components/Form/index';
  import { adaTableScrollX } from '@/utils/hotgo';
  import {
    BindingList,
    BotList,
    ChannelOptions,
    ClearConversation,
    ConversationList,
    ConversationView,
    FeatureList,
    MessageList,
    OperatorList,
    SaveBinding,
    SaveBot,
    SaveFeature,
    SaveOperator,
  } from '@/api/addons/youban_chat';
  import { createColumns, schemas } from './model';

  const message = useMessage();
  const dialog = useDialog();
  const activeTab = ref('conversations');
  const conversationLoading = ref(false);
  const conversations = ref<any[]>([]);
  const conversationPagination = reactive(createPagination(loadConversations));
  const searchFormRef = ref<any>({});
  const detailVisible = ref(false);
  const currentConversation = ref<any>(null);
  const messages = ref<any[]>([]);
  const messageLoading = ref(false);

  const [register] = useForm({
    gridProps: { cols: '1 s:1 m:2 l:3 xl:4 2xl:4' },
    labelWidth: 80,
    schemas,
  });

  const columns = createColumns(openDetail);
  const scrollX = computed(() => adaTableScrollX(columns, 0));
  const detailTitle = computed(() => (currentConversation.value ? `会话 #${currentConversation.value.id}` : '会话详情'));

  const statusOptions = [
    { label: '启用', value: 1 },
    { label: '禁用', value: 2 },
  ];
  const allStatusOptions = [{ label: '全部', value: 0 }, ...statusOptions];
  const bindingTypeOptions = [
    { label: '频道绑定', value: 'channel' },
    { label: '全局默认', value: 'global' },
  ];

  const operatorLoading = ref(false);
  const operators = ref<any[]>([]);
  const operatorQuery = reactive({ keyword: '', status: 0 });
  const operatorPagination = reactive(createPagination(loadOperators));
  const operatorModalVisible = ref(false);
  const operatorForm = reactive<any>(newOperatorForm());

  const botLoading = ref(false);
  const bots = ref<any[]>([]);
  const botQuery = reactive({ keyword: '', status: 0 });
  const botPagination = reactive(createPagination(loadBots));
  const botModalVisible = ref(false);
  const botForm = reactive<any>(newBotForm());

  const bindingLoading = ref(false);
  const bindings = ref<any[]>([]);
  const channelOptions = ref<any[]>([]);
  const bindingQuery = reactive({ keyword: '', status: 0 });
  const bindingPagination = reactive(createPagination(loadBindings));
  const bindingModalVisible = ref(false);
  const bindingForm = reactive<any>(newBindingForm());
  const channelPickerVisible = ref(false);
  const channelPickerMultiple = ref(true);
  const channelPickerKeyword = ref('');
  const channelPickerValue = ref<any[]>([]);

  const featureLoading = ref(false);
  const features = ref<any[]>([]);
  const featureQuery = reactive({ keyword: '', status: 0 });
  const featurePagination = reactive(createPagination(loadFeatures));
  const featureModalVisible = ref(false);
  const featureForm = reactive<any>(newFeatureForm());

  const channelOptionMap = computed(() => {
    const map = new Map<number, string>();
    channelOptions.value.forEach((item) => map.set(Number(item.value), item.label));
    return map;
  });
  const selectedChannelText = computed(() =>
    (bindingForm.channelIds || []).map((id) => channelOptionMap.value.get(Number(id)) || id).join('、')
  );
  const filteredChannelOptions = computed(() => {
    const keyword = channelPickerKeyword.value.trim().toLowerCase();
    if (!keyword) {
      return channelOptions.value;
    }
    return channelOptions.value.filter((item) => String(item.label || '').toLowerCase().includes(keyword));
  });
  const channelPickerColumns = computed(() => [
    { type: 'selection', multiple: channelPickerMultiple.value },
    { title: '频道', key: 'label' },
  ]);

  const operatorColumns = [
    { title: 'ID', key: 'id', width: 80 },
    {
      title: '后台账号',
      key: 'admin',
      width: 180,
      render(row) {
        return row.adminRealName || row.adminUsername || row.adminMemberId || '-';
      },
    },
    { title: 'TG用户ID', key: 'telegramUserId', width: 160 },
    { title: 'TG用户名', key: 'telegramUsername', width: 160 },
    { title: '显示名称', key: 'displayName', width: 140 },
    { title: '绑定码', key: 'bindCode', width: 120 },
    {
      title: '状态',
      key: 'status',
      width: 100,
      render(row) {
        return renderNumberStatus(row.status);
      },
    },
    { title: '更新时间', key: 'updatedAt', width: 180 },
    {
      title: '操作',
      key: 'actions',
      width: 100,
      fixed: 'right',
      render(row) {
        return h(NButton, { size: 'small', type: 'primary', onClick: () => openOperatorModal(row) }, { default: () => '编辑' });
      },
    },
  ];

  const botColumns = [
    { title: 'ID', key: 'id', width: 80 },
    { title: 'Bot名称', key: 'botName', width: 160 },
    { title: '用户名', key: 'botUsername', width: 160 },
    {
      title: 'Token',
      key: 'botToken',
      width: 160,
      render(row) {
        return row.botToken ? `${String(row.botToken).slice(0, 8)}...` : '-';
      },
    },
    {
      title: '状态',
      key: 'status',
      width: 100,
      render(row) {
        return renderNumberStatus(row.status);
      },
    },
    { title: '更新时间', key: 'updatedAt', width: 180 },
    {
      title: '操作',
      key: 'actions',
      width: 100,
      fixed: 'right',
      render(row) {
        return h(NButton, { size: 'small', type: 'primary', onClick: () => openBotModal(row) }, { default: () => '编辑' });
      },
    },
  ];

  const bindingColumns = [
    { title: 'ID', key: 'id', width: 80 },
    { title: '绑定码', key: 'bindCode', width: 140 },
    {
      title: '类型',
      key: 'bindType',
      width: 120,
      render(row) {
        return row.bindType === 'global' ? '全局默认' : '频道绑定';
      },
    },
    {
      title: '关联频道',
      key: 'channels',
      width: 240,
      render(row) {
        if (row.bindType === 'global') {
          return '全局默认';
        }
        const ids = Array.isArray(row.channelIds) ? row.channelIds : [];
        if (ids.length === 0) {
          return row.channelTitle || row.channelUsername || row.contentChannelId || '-';
        }
        return ids.map((id) => channelOptionMap.value.get(Number(id)) || id).join('、');
      },
    },
    { title: 'Bot', key: 'botName', width: 140 },
    { title: 'TG群', key: 'tgChatTitle', width: 180 },
    { title: 'TG群ID', key: 'tgChatId', width: 160 },
    {
      title: '状态',
      key: 'status',
      width: 100,
      render(row) {
        return renderNumberStatus(row.status);
      },
    },
    {
      title: '操作',
      key: 'actions',
      width: 100,
      fixed: 'right',
      render(row) {
        return h(NButton, { size: 'small', type: 'primary', onClick: () => openBindingModal(row) }, { default: () => '编辑' });
      },
    },
  ];

  const featureColumns = [
    { title: 'ID', key: 'id', width: 80 },
    { title: '功能Key', key: 'featureKey', width: 120 },
    { title: '名称', key: 'name', width: 160 },
    { title: '命令', key: 'command', width: 120 },
    { title: '描述', key: 'description', width: 180 },
    {
      title: '状态',
      key: 'status',
      width: 100,
      render(row) {
        return renderNumberStatus(row.status);
      },
    },
    { title: '排序', key: 'sort', width: 80 },
    { title: '更新时间', key: 'updatedAt', width: 180 },
    {
      title: '操作',
      key: 'actions',
      width: 100,
      fixed: 'right',
      render(row) {
        return h(NButton, { size: 'small', type: 'primary', onClick: () => openFeatureModal(row) }, { default: () => '编辑' });
      },
    },
  ];

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

  function reloadConversationTable() {
    conversationPagination.page = 1;
    loadConversations();
  }

  async function loadConversations() {
    conversationLoading.value = true;
    try {
      const res: any = await ConversationList({
        ...searchFormRef.value?.formModel,
        page: conversationPagination.page,
        perPage: conversationPagination.pageSize,
      });
      conversations.value = res?.list || [];
      conversationPagination.itemCount = res?.totalCount || res?.total || 0;
    } finally {
      conversationLoading.value = false;
    }
  }

  async function openDetail(row: Recordable) {
    detailVisible.value = true;
    messageLoading.value = true;
    currentConversation.value = row;
    try {
      currentConversation.value = await ConversationView({ id: row.id });
      const res: any = await MessageList({ conversationId: row.id, page: 1, perPage: 100 });
      messages.value = res?.list || [];
    } finally {
      messageLoading.value = false;
    }
  }

  function handleClearConversation() {
    if (!currentConversation.value?.id) {
      return;
    }
    dialog.warning({
      title: '清空聊天记录',
      content: '确定清空当前会话的全部聊天记录？清空后 H5 也不会再显示这些消息。',
      positiveText: '确定清空',
      negativeText: '取消',
      onPositiveClick: async () => {
        messageLoading.value = true;
        try {
          await ClearConversation({ conversationId: currentConversation.value.id });
          messages.value = [];
          currentConversation.value.lastMessage = '';
          currentConversation.value.lastMessageAt = '';
          currentConversation.value.unreadCount = 0;
          message.success('聊天记录已清空');
          await loadConversations();
        } finally {
          messageLoading.value = false;
        }
      },
    });
  }

  function handleTabChange(tab: string) {
    if (tab === 'operators') {
      loadOperators();
    }
    if (tab === 'bots') {
      loadBots();
    }
    if (tab === 'bindings') {
      loadChannelOptions();
      loadBindings();
    }
    if (tab === 'features') {
      loadFeatures();
    }
  }

  async function loadOperators() {
    operatorLoading.value = true;
    try {
      const res: any = await OperatorList({ ...operatorQuery, page: operatorPagination.page, perPage: operatorPagination.pageSize });
      operators.value = res?.list || [];
      operatorPagination.itemCount = res?.totalCount || res?.total || 0;
    } finally {
      operatorLoading.value = false;
    }
  }

  function openOperatorModal(row?: any) {
    Object.assign(operatorForm, newOperatorForm(), row || {});
    operatorModalVisible.value = true;
  }

  async function saveOperator() {
    await SaveOperator(operatorForm);
    message.success('客服绑定已保存');
    await loadOperators();
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

  function openBotModal(row?: any) {
    Object.assign(botForm, newBotForm(), row || {});
    botModalVisible.value = true;
  }

  async function saveBot() {
    await SaveBot(botForm);
    message.success('Bot配置已保存');
    await loadBots();
  }

  async function loadBindings() {
    bindingLoading.value = true;
    try {
      const res: any = await BindingList({ ...bindingQuery, page: bindingPagination.page, perPage: bindingPagination.pageSize });
      bindings.value = res?.list || [];
      bindingPagination.itemCount = res?.totalCount || res?.total || 0;
    } finally {
      bindingLoading.value = false;
    }
  }

  async function loadChannelOptions() {
    const res: any = await ChannelOptions();
    channelOptions.value = res?.list || res || [];
  }

  async function openBindingModal(row?: any) {
    if (channelOptions.value.length === 0) {
      await loadChannelOptions();
    }
    Object.assign(bindingForm, newBindingForm(), row || {});
    bindingForm.channelIds = Array.isArray(row?.channelIds) ? row.channelIds : [];
    bindingModalVisible.value = true;
  }

  async function saveBinding() {
    if (bindingForm.bindType === 'channel' && (!bindingForm.channelIds || bindingForm.channelIds.length === 0)) {
      message.warning('请选择关联频道');
      return;
    }
    await SaveBinding({
      id: bindingForm.id,
      bindType: bindingForm.bindType,
      channelIds: bindingForm.bindType === 'channel' ? bindingForm.channelIds : [],
      remark: bindingForm.remark,
      status: bindingForm.status,
    });
    message.success('频道群绑定已保存');
    await loadBindings();
  }

  async function openChannelPicker(multiple = true) {
    if (channelOptions.value.length === 0) {
      await loadChannelOptions();
    }
    channelPickerMultiple.value = multiple;
    channelPickerKeyword.value = '';
    channelPickerValue.value = [...(bindingForm.channelIds || [])];
    channelPickerVisible.value = true;
  }

  function confirmChannelPicker() {
    const values = channelPickerMultiple.value ? channelPickerValue.value : channelPickerValue.value.slice(0, 1);
    bindingForm.channelIds = values.map((item) => Number(item)).filter((item) => Number.isFinite(item) && item > 0);
  }

  async function loadFeatures() {
    featureLoading.value = true;
    try {
      const res: any = await FeatureList({ ...featureQuery, page: featurePagination.page, perPage: featurePagination.pageSize });
      features.value = res?.list || [];
      featurePagination.itemCount = res?.totalCount || res?.total || 0;
    } finally {
      featureLoading.value = false;
    }
  }

  function openFeatureModal(row?: any) {
    Object.assign(featureForm, newFeatureForm(), row || {});
    featureModalVisible.value = true;
  }

  async function saveFeature() {
    if (featureForm.configJson) {
      try {
        JSON.parse(featureForm.configJson);
      } catch (error) {
        message.warning('配置JSON格式不正确');
        return;
      }
    }
    await SaveFeature(featureForm);
    message.success('功能配置已保存');
    await loadFeatures();
  }

  function newBotForm() {
    return {
      id: 0,
      botName: '',
      botToken: '',
      remark: '',
      status: 1,
    };
  }

  function newBindingForm() {
    return {
      id: 0,
      bindType: 'channel',
      channelIds: [],
      remark: '',
      status: 1,
    };
  }

  function newOperatorForm() {
    return {
      id: 0,
      adminMemberId: 0,
      telegramUserId: '',
      telegramUsername: '',
      displayName: '',
      bindCode: '',
      remark: '',
      status: 1,
    };
  }

  function newFeatureForm() {
    return {
      id: 0,
      featureKey: '',
      name: '',
      command: '',
      description: '',
      configJson: '{}',
      sort: 0,
      status: 1,
    };
  }

  function renderNumberStatus(status: number) {
    const enabled = Number(status) === 1;
    return h(NTag, { type: enabled ? 'success' : 'warning', bordered: false }, { default: () => (enabled ? '启用' : '禁用') });
  }

  function conversationStatusTag(status: string) {
    if (status === 'opened') {
      return { type: 'success' as const, label: '已打开' };
    }
    if (status === 'closed') {
      return { type: 'default' as const, label: '已关闭' };
    }
    return { type: 'warning' as const, label: status || '-' };
  }

  function directionLabel(direction: string) {
    if (direction === 'mine') return '用户';
    if (direction === 'service') return '客服';
    return '系统';
  }

  function attachmentText(item: any) {
    return item?.attachments?.length ? `[附件 ${item.attachments.length}]` : '-';
  }

  onMounted(() => {
    loadConversations();
    loadBots();
    loadChannelOptions();
    loadBindings();
    loadFeatures();
  });
</script>

<style lang="less" scoped>
  .toolbar {
    margin-bottom: 16px;
  }

  .status-select {
    width: 140px;
  }

  .w-full {
    width: 100%;
  }

  .selected-channels {
    max-height: 96px;
    overflow: auto;
    color: #374151;
    line-height: 22px;
    word-break: break-word;
  }

  .detail-actions {
    margin-top: 12px;
  }

  .message-list {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .message-item {
    border: 1px solid #e5e7eb;
    border-radius: 6px;
    padding: 10px 12px;
    background: #ffffff;
  }

  .message-item.is-mine {
    border-color: #bfdbfe;
    background: #eff6ff;
  }

  .message-item.is-service {
    border-color: #bbf7d0;
    background: #f0fdf4;
  }

  .message-meta {
    display: flex;
    justify-content: space-between;
    color: #6b7280;
    font-size: 12px;
  }

  .message-content {
    margin-top: 6px;
    color: #111827;
    white-space: pre-wrap;
    word-break: break-word;
  }

  .message-attachments {
    display: flex;
    flex-direction: column;
    gap: 8px;
    margin-top: 8px;
  }

  .message-image,
  .message-video {
    max-width: 100%;
    max-height: 320px;
    border-radius: 6px;
    object-fit: contain;
    background: #f8fafc;
  }

  .message-file {
    color: #2563eb;
    word-break: break-all;
  }
</style>
