<template>
  <n-tabs v-model:value="activeTab" type="line" animated>
    <n-tab-pane name="user" tab="用户绑定">
      <n-space class="mb-4" justify="space-between">
        <n-space>
          <n-select
            v-model:value="userQuery.botId"
            clearable
            filterable
            :options="botOptions"
            placeholder="筛选 Bot"
            class="youban-bot-user-panel__bot-select"
          />
          <n-input
            v-model:value="userQuery.keyword"
            clearable
            placeholder="搜索 TG/昵称/上架账号"
          />
          <n-select
            v-model:value="userQuery.isBound"
            clearable
            :options="boundOptions"
            placeholder="绑定状态"
            class="youban-bot-user-panel__select"
          />
          <n-button @click="loadUsers">查询</n-button>
        </n-space>
      </n-space>
      <n-data-table
        :columns="userColumns"
        :data="userRows"
        :loading="userLoading"
        :pagination="userPagination"
        remote
        @update:page="handleUserPage"
        @update:page-size="handleUserPageSize"
      />
    </n-tab-pane>

    <n-tab-pane name="message" tab="消息日志">
      <n-space class="mb-4" justify="space-between">
        <n-space>
          <n-select
            v-model:value="messageQuery.botId"
            clearable
            filterable
            :options="botOptions"
            placeholder="筛选 Bot"
            class="youban-bot-user-panel__bot-select"
          />
          <n-input v-model:value="messageQuery.keyword" clearable placeholder="搜索内容/TG 用户" />
          <n-input v-model:value="messageQuery.telegramUserId" clearable placeholder="TG 用户 ID" />
          <n-button @click="loadMessages">查询</n-button>
        </n-space>
      </n-space>
      <n-data-table
        :columns="messageColumns"
        :data="messageRows"
        :loading="messageLoading"
        :pagination="messagePagination"
        remote
        @update:page="handleMessagePage"
        @update:page-size="handleMessagePageSize"
      />
    </n-tab-pane>
  </n-tabs>

  <n-modal
    v-model:show="sendModalVisible"
    preset="dialog"
    title="发送 Telegram 消息"
    positive-text="发送"
    negative-text="取消"
    @positive-click="sendTelegramMessage"
  >
    <n-form :model="sendForm" label-placement="left" label-width="90">
      <n-form-item label="Bot">
        <n-select v-model:value="sendForm.botId" :options="botOptions" />
      </n-form-item>
      <n-form-item label="Chat ID">
        <n-input v-model:value="sendForm.chatId" disabled />
      </n-form-item>
      <n-form-item label="消息内容">
        <n-input
          v-model:value="sendForm.text"
          type="textarea"
          :autosize="{ minRows: 4, maxRows: 8 }"
        />
      </n-form-item>
      <n-form-item label="静默发送">
        <n-switch v-model:value="sendForm.disableNotice" />
      </n-form-item>
    </n-form>
  </n-modal>
</template>

<script setup lang="ts">
  import { computed, h, onMounted, reactive, ref } from 'vue';
  import { NButton, NSpace, NTag, useMessage } from 'naive-ui';

  import { BotList, MessageList, SendMessage, UserList } from '@/api/addons/youbanBot';

  const message = useMessage();
  const activeTab = ref('user');
  const userLoading = ref(false);
  const messageLoading = ref(false);
  const userRows = ref<any[]>([]);
  const messageRows = ref<any[]>([]);
  const botRows = ref<any[]>([]);
  const sendModalVisible = ref(false);
  const boundOptions = [
    { label: '已绑定', value: 1 },
    { label: '未绑定', value: 2 },
  ];
  const userQuery = reactive({
    page: 1,
    perPage: 10,
    botId: null as number | null,
    keyword: '',
    isBound: null as number | null,
  });
  const messageQuery = reactive({
    page: 1,
    perPage: 10,
    botId: null as number | null,
    keyword: '',
    telegramUserId: '',
  });
  const sendForm = reactive({ botId: 0, chatId: '', text: '', disableNotice: false });
  const botOptions = computed(() =>
    botRows.value.map((item) => ({
      label: item.botUsername ? `@${item.botUsername}` : item.botName,
      value: item.id,
    }))
  );
  const userPagination = reactive({
    page: 1,
    pageSize: 10,
    itemCount: 0,
    showSizePicker: true,
    pageSizes: [10, 20, 50],
  });
  const messagePagination = reactive({
    page: 1,
    pageSize: 10,
    itemCount: 0,
    showSizePicker: true,
    pageSizes: [10, 20, 50],
  });

  const userColumns = [
    {
      title: 'Bot',
      key: 'botUsername',
      width: 150,
      render: (row: any) => (row.botUsername ? `@${row.botUsername}` : row.botId),
    },
    { title: 'TG用户ID', key: 'telegramUserId', width: 150 },
    {
      title: '用户名',
      key: 'telegramUsername',
      width: 150,
      render: (row: any) => (row.telegramUsername ? `@${row.telegramUsername}` : '-'),
    },
    {
      title: 'Chat',
      key: 'chatId',
      width: 160,
      render: (row: any) => `${row.chatType || '-'} / ${row.chatId || '-'}`,
    },
    { title: '绑定状态', key: 'isBound', width: 110, render: (row: any) => renderBound(row) },
    {
      title: '绑定账号',
      key: 'bindAccountName',
      width: 180,
      render: (row: any) => row.bindAccountName || '-',
    },
    {
      title: '租户ID',
      key: 'bindTenantId',
      width: 100,
      render: (row: any) => row.bindTenantId || '-',
    },
    { title: '消息数', key: 'messageCount', width: 90 },
    { title: '最后消息', key: 'lastMessageText', minWidth: 220, ellipsis: { tooltip: true } },
    {
      title: '操作',
      key: 'actions',
      width: 100,
      render: (row: any) =>
        h(NSpace, { size: 8 }, () => [
          h(
            NButton,
            { size: 'small', onClick: () => filterMessages(row) },
            { default: () => '看消息' }
          ),
          h(
            NButton,
            { size: 'small', type: 'primary', onClick: () => openSendModal(row) },
            { default: () => '发消息' }
          ),
        ]),
    },
  ];

  const messageColumns = [
    { title: '时间', key: 'createdAt', width: 170 },
    {
      title: 'Bot',
      key: 'botUsername',
      width: 150,
      render: (row: any) => (row.botUsername ? `@${row.botUsername}` : row.botId),
    },
    { title: 'TG用户ID', key: 'telegramUserId', width: 150 },
    {
      title: '用户名',
      key: 'telegramUsername',
      width: 150,
      render: (row: any) => (row.telegramUsername ? `@${row.telegramUsername}` : '-'),
    },
    { title: 'Chat', key: 'chatId', width: 150 },
    { title: '类型', key: 'messageType', width: 100 },
    { title: '消息内容', key: 'text', minWidth: 320, ellipsis: { tooltip: true } },
  ];

  function renderBound(row: any) {
    return h(
      NTag,
      { type: row.isBound ? 'success' : 'default' },
      { default: () => (row.isBound ? '已绑定' : '未绑定') }
    );
  }

  async function loadBots() {
    const res: any = await BotList({ page: 1, perPage: 100, status: 1 });
    botRows.value = res?.list || [];
  }

  async function loadUsers() {
    userLoading.value = true;
    try {
      const res: any = await UserList({
        ...userQuery,
        page: userPagination.page,
        perPage: userPagination.pageSize,
      });
      userRows.value = res?.list || [];
      userPagination.itemCount = res?.totalCount || res?.total || 0;
    } finally {
      userLoading.value = false;
    }
  }

  async function loadMessages() {
    messageLoading.value = true;
    try {
      const res: any = await MessageList({
        ...messageQuery,
        page: messagePagination.page,
        perPage: messagePagination.pageSize,
      });
      messageRows.value = res?.list || [];
      messagePagination.itemCount = res?.totalCount || res?.total || 0;
    } finally {
      messageLoading.value = false;
    }
  }

  function handleUserPage(page: number) {
    userPagination.page = page;
    loadUsers();
  }

  function handleUserPageSize(pageSize: number) {
    userPagination.pageSize = pageSize;
    userPagination.page = 1;
    loadUsers();
  }

  function handleMessagePage(page: number) {
    messagePagination.page = page;
    loadMessages();
  }

  function openSendModal(row: any) {
    sendForm.botId = row.botId;
    sendForm.chatId = row.chatId;
    sendForm.text = '';
    sendForm.disableNotice = false;
    sendModalVisible.value = true;
  }

  async function sendTelegramMessage() {
    await SendMessage(sendForm);
    message.success('发送成功');
    sendModalVisible.value = false;
  }

  function handleMessagePageSize(pageSize: number) {
    messagePagination.pageSize = pageSize;
    messagePagination.page = 1;
    loadMessages();
  }

  function filterMessages(row: any) {
    messageQuery.telegramUserId = row.telegramUserId;
    messageQuery.botId = row.botId;
    activeTab.value = 'message';
    loadMessages();
  }

  onMounted(() => {
    loadBots();
    loadUsers();
    loadMessages();
  });
</script>

<style scoped>
  .youban-bot-user-panel__select {
    width: 130px;
  }

  .youban-bot-user-panel__bot-select {
    width: 180px;
  }
</style>
