<template>
  <div class="channel-member-panel">
    <n-alert type="info" :bordered="false" class="mb-16">
      成员同步会把当前频道/群聊的创建者、管理员和成员缓存到数据库，支持增量更新和导出。
    </n-alert>
    <n-space class="toolbar" align="center" justify="space-between">
      <n-space align="center" wrap>
        <n-select
          v-model:value="query.tgAccountId"
          :options="tgAccountOptions"
          clearable
          filterable
          placeholder="TG账号"
          class="tenant-select"
        />
        <n-select
          v-model:value="query.displayType"
          :options="displayTypeOptions"
          clearable
          placeholder="频道 / 群聊"
          class="status-select"
        />
        <n-select
          v-model:value="query.managementRole"
          :options="managementRoleOptions"
          clearable
          placeholder="创建者 / 管理员 / 成员"
          class="status-select"
        />
        <n-input
          v-model:value="query.keyword"
          clearable
          placeholder="频道名称 / 用户名 / Chat ID"
          class="keyword-input"
          @keyup.enter="loadChannelCaches"
        />
        <n-button @click="loadChannelCaches">查询</n-button>
        <n-button quaternary @click="resetQuery">重置</n-button>
      </n-space>
      <n-space>
        <n-button @click="loadTgAccounts(true)">刷新账号</n-button>
      </n-space>
    </n-space>
    <n-data-table
      :columns="columns"
      :data="channelCaches"
      :loading="channelLoading"
      :pagination="pagination"
      :row-key="(row) => row.id"
      :scroll-x="1680"
      size="small"
      remote
    />
    <n-modal
      v-model:show="memberModalVisible"
      preset="dialog"
      title="成员缓存"
      :mask-closable="false"
      :style="{ width: '1100px', maxWidth: '94vw' }"
    >
      <n-space vertical>
        <n-alert type="info" :bordered="false">
          {{ memberModalTitle }}
        </n-alert>
        <n-space align="center" wrap>
          <n-select
            v-model:value="memberQuery.role"
            :options="memberRoleOptions"
            clearable
            placeholder="角色"
            class="status-select"
          />
          <n-select
            v-model:value="memberQuery.status"
            :options="memberStatusOptions"
            clearable
            placeholder="状态"
            class="status-select"
          />
          <n-input
            v-model:value="memberQuery.keyword"
            clearable
            placeholder="昵称 / 用户名 / 用户ID"
            class="keyword-input"
            @keyup.enter="loadChannelMembers"
          />
          <n-button @click="loadChannelMembers">查询</n-button>
          <n-button quaternary @click="resetMemberQuery">重置</n-button>
          <n-button :loading="memberExporting" @click="exportChannelMembers">导出</n-button>
        </n-space>
        <n-data-table
          :columns="memberColumns"
          :data="members"
          :loading="memberLoading"
          :pagination="memberPagination"
          :row-key="(row) => row.id"
          :scroll-x="1380"
          size="small"
          remote
        />
      </n-space>
      <template #action>
        <n-space justify="end">
          <n-button @click="memberModalVisible = false">关闭</n-button>
          <n-button
            :disabled="!selectedCacheRow"
            type="primary"
            @click="syncMembers(selectedCacheRow)"
          >
            重新同步
          </n-button>
        </n-space>
      </template>
    </n-modal>
    <ChannelMemberSyncModal
      v-model:show="syncModalVisible"
      :cancel-loading="syncCancelLoading"
      :task="syncTask"
      @cancel="cancelSync"
    />
  </div>
</template>
<script lang="ts" setup>
  import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
  import { useMessage } from 'naive-ui';
  import {
    AdminChannelCacheList,
    AdminTgAccountList,
    ChannelMemberList,
    ChannelMemberSyncCancel,
    ChannelMemberSyncStart,
    ChannelMemberSyncView,
  } from '@/api/addons/youbanPublish';
  import { jumpExport } from '@/utils/http/axios';
  import { createChannelColumns, createMemberColumns } from './channel-member-panel-columns';
  import ChannelMemberSyncModal from './channel-member-sync-modal.vue';

  const message = useMessage();
  const channelLoading = ref(false);
  const memberLoading = ref(false);
  const memberExporting = ref(false);
  const syncCancelLoading = ref(false);
  const syncModalVisible = ref(false);
  const memberModalVisible = ref(false);
  const channelCaches = ref<Recordable[]>([]);
  const members = ref<Recordable[]>([]);
  const tgAccounts = ref<Recordable[]>([]);
  const selectedCacheRow = ref<Recordable | null>(null);
  const syncTimer = ref<number | null>(null);
  const syncTask = reactive(defaultSyncTask());
  const query = reactive({
    tgAccountId: null as number | null,
    displayType: '',
    managementRole: '',
    keyword: '',
  });
  const memberQuery = reactive({
    role: '',
    status: 0,
    keyword: '',
  });
  const pagination = createPagination(loadChannelCaches);
  const memberPagination = createPagination(loadChannelMembers, 20);
  const displayTypeOptions = [
    { label: '全部', value: '' },
    { label: '频道', value: 'channel' },
    { label: '群聊', value: 'group' },
  ];
  const managementRoleOptions = [
    { label: '全部角色', value: '' },
    { label: '创建者', value: 'owner' },
    { label: '管理员', value: 'admin' },
    { label: '成员', value: 'member' },
  ];
  const memberRoleOptions = [
    { label: '全部角色', value: '' },
    { label: '创建者', value: 'creator' },
    { label: '管理员', value: 'admin' },
    { label: '成员', value: 'member' },
  ];
  const memberStatusOptions = [
    { label: '全部状态', value: 0 },
    { label: '有效', value: 1 },
    { label: '失效', value: 2 },
  ];

  const tgAccountOptions = computed(() =>
    tgAccounts.value.map((item) => ({
      label: item.displayName || item.nickname || item.telegramUsername || `TG账号 ${item.id}`,
      value: item.id,
    }))
  );
  const memberModalTitle = computed(() =>
    selectedCacheRow.value
      ? `${selectedCacheRow.value.channelTitle || '-'} / ${selectedCacheRow.value.channelUsername || selectedCacheRow.value.channelId || '-'}`
      : '成员缓存'
  );
  const columns = createChannelColumns({
    displayTypeLabel,
    exportMembers,
    openMembers,
    roleLabel,
    syncMembers,
    tgAccountLabel,
  });
  const memberColumns = createMemberColumns({ roleLabel });

  onMounted(async () => {
    await loadTgAccounts();
    await loadChannelCaches();
  });

  onBeforeUnmount(() => stopSyncPolling());
  watch(syncModalVisible, (visible) => {
    if (!visible) stopSyncPolling();
  });

  function createPagination(loader: () => void, pageSize = 10) {
    const pagination: any = reactive({
      page: 1,
      pageSize,
      itemCount: 0,
      showSizePicker: true,
      pageSizes: [10, 20, 50],
      onUpdatePage: (page: number) => {
        pagination.page = page;
        loader();
      },
      onUpdatePageSize: (nextPageSize: number) => {
        pagination.pageSize = nextPageSize;
        pagination.page = 1;
        loader();
      },
    });
    return pagination;
  }

  async function loadTgAccounts(force = false) {
    if (!force && tgAccounts.value.length > 0) return;
    const res: any = await AdminTgAccountList({ page: 1, perPage: 200 });
    tgAccounts.value = res?.list || [];
    if (!query.tgAccountId && tgAccounts.value.length > 0) {
      query.tgAccountId = tgAccounts.value[0].id;
    }
  }

  async function loadChannelCaches() {
    channelLoading.value = true;
    try {
      if (!query.tgAccountId) {
        await loadTgAccounts();
      }
      const res: any = await AdminChannelCacheList({
        tgAccountId: query.tgAccountId,
        displayType: query.displayType,
        managementRole: query.managementRole,
        keyword: query.keyword,
        page: pagination.page,
        perPage: pagination.pageSize,
      });
      channelCaches.value = res?.list || [];
      pagination.itemCount = res?.totalCount || res?.total || 0;
    } finally {
      channelLoading.value = false;
    }
  }

  function resetQuery() {
    query.tgAccountId = tgAccounts.value[0]?.id || null;
    query.displayType = '';
    query.managementRole = '';
    query.keyword = '';
    pagination.page = 1;
    loadChannelCaches();
  }

  async function openForTgAccount(tgAccountId: number) {
    query.tgAccountId = tgAccountId;
    pagination.page = 1;
    await loadTgAccounts();
    await loadChannelCaches();
  }

  async function openMembers(row: Recordable) {
    selectedCacheRow.value = row;
    memberModalVisible.value = true;
    memberPagination.page = 1;
    memberQuery.role = '';
    memberQuery.status = 0;
    memberQuery.keyword = '';
    await loadChannelMembers();
  }

  async function loadChannelMembers() {
    if (!selectedCacheRow.value) return;
    memberLoading.value = true;
    try {
      const res: any = await ChannelMemberList({
        tgAccountId: selectedCacheRow.value.tgAccountId,
        channelId: selectedCacheRow.value.channelId,
        role: memberQuery.role,
        status: memberQuery.status,
        keyword: memberQuery.keyword,
        page: memberPagination.page,
        perPage: memberPagination.pageSize,
      });
      members.value = res?.list || [];
      memberPagination.itemCount = res?.totalCount || res?.total || 0;
    } finally {
      memberLoading.value = false;
    }
  }

  function resetMemberQuery() {
    memberQuery.role = '';
    memberQuery.status = 0;
    memberQuery.keyword = '';
    memberPagination.page = 1;
    loadChannelMembers();
  }

  async function syncMembers(row: Recordable | null) {
    if (!row) return;
    selectedCacheRow.value = row;
    const res: any = await ChannelMemberSyncStart({
      tgAccountId: row.tgAccountId,
      channelId: row.channelId,
    });
    applySyncTask(res?.list || res || {});
    syncModalVisible.value = true;
    startSyncPolling();
    await loadChannelCaches();
  }

  async function exportMembers(row?: Recordable) {
    const target = row || selectedCacheRow.value;
    if (!target) {
      message.warning('请选择频道/群聊');
      return;
    }
    jumpExport('/youban_publish/publish/admin/channel/member/export', {
      tgAccountId: target.tgAccountId,
      channelId: target.channelId,
      role: memberQuery.role,
      status: memberQuery.status,
      keyword: memberQuery.keyword,
    });
  }

  async function exportChannelMembers() {
    memberExporting.value = true;
    try {
      await exportMembers();
      message.success('已开始导出');
    } finally {
      memberExporting.value = false;
    }
  }

  async function refreshSyncTask() {
    if (!syncTask.id) return;
    const res: any = await ChannelMemberSyncView({ id: syncTask.id });
    applySyncTask(res?.list || res || {});
    if (['success', 'failed', 'canceled'].includes(syncTask.status)) {
      stopSyncPolling();
      await loadChannelCaches();
      if (memberModalVisible.value) {
        await loadChannelMembers();
      }
    }
  }

  async function cancelSync() {
    if (!syncTask.id) return;
    syncCancelLoading.value = true;
    try {
      await ChannelMemberSyncCancel({ id: syncTask.id });
      message.success('同步已取消');
      await refreshSyncTask();
    } finally {
      syncCancelLoading.value = false;
    }
  }

  function startSyncPolling() {
    stopSyncPolling();
    syncTimer.value = window.setInterval(refreshSyncTask, 2000);
  }

  function stopSyncPolling() {
    if (syncTimer.value) {
      window.clearInterval(syncTimer.value);
      syncTimer.value = null;
    }
  }

  function applySyncTask(task: Recordable) {
    Object.assign(syncTask, defaultSyncTask(), task || {});
  }

  function defaultSyncTask() {
    return {
      id: 0,
      tgAccountId: 0,
      channelId: '',
      channelTitle: '',
      channelUsername: '',
      status: '',
      stage: '',
      progressTotal: 0,
      progressDone: 0,
      adminTotal: 0,
      adminDone: 0,
      memberTotal: 0,
      memberDone: 0,
      upsertedCount: 0,
      removedCount: 0,
      errorMessage: '',
      progress: 0,
      stageText: '',
      statusText: '',
    };
  }

  function tgAccountLabel(id: number) {
    const item = tgAccounts.value.find((row) => row.id === id);
    return item
      ? item.displayName || item.nickname || item.telegramUsername || `TG账号 ${id}`
      : `TG账号 ${id || '-'}`;
  }

  function displayTypeLabel(displayType: string, row: Recordable) {
    if (displayType === 'channel') return '频道';
    if (displayType === 'group') return '群聊';
    if (row?.isBroadcast === 1) return '频道';
    if (row?.isMegagroup === 1 || String(row?.channelId || '').startsWith('-')) return '群聊';
    return '-';
  }

  function roleLabel(role: string) {
    if (role === 'creator') return '创建者';
    if (role === 'admin') return '管理员';
    if (role === 'member') return '成员';
    return role || '-';
  }

  defineExpose({
    openForTgAccount,
    loadChannelCaches,
  });
</script>

<style scoped>
  .toolbar {
    margin-bottom: 16px;
  }

  .status-select {
    width: 150px;
  }

  .tenant-select {
    width: 210px;
  }

  .keyword-input {
    width: 260px;
  }

  .action-row {
    display: flex;
    gap: 6px;
    flex-wrap: wrap;
  }

  .mb-16 {
    margin-bottom: 16px;
  }
</style>
