<script setup lang="ts">
  import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue';
  import { useMessage } from 'naive-ui';
  import {
    AdminChannelCacheList,
    BotList,
    RelayChannelCreate,
    RelayChannelList,
    RelayChannelSave,
    RelayChannelStatus,
    ServerTgAccountList,
    type RelayChannelModel,
  } from '@/api/addons/youbanPublish';
  import { usePermission } from '@/hooks/web/usePermission';
  import { useDictStore } from '@/store/modules/dict';

  interface Option {
    label: string;
    value: number;
  }
  interface AccountRow {
    id: number;
    tenantId: number;
    displayName?: string;
    phone?: string;
  }
  interface BotRow {
    id: number;
    tenantId: number;
    botName?: string;
    botUsername?: string;
  }
  interface ChannelRow {
    channelId: string;
    channelTitle: string;
  }
  const props = defineProps<{ tenantId: number; tenantOptions?: Option[] }>();
  const message = useMessage();
  const { hasPermission } = usePermission();
  const dict = useDictStore();
  dict.loadOptions([
    'YoubanRelayChannelStatusOptions',
    'YoubanRelayChannelHealthOptions',
    'YoubanRelayOperationStatusOptions',
  ]);
  const rows = ref<RelayChannelModel[]>([]);
  const selectedTenantId = ref(0);
  const accounts = ref<AccountRow[]>([]);
  const bots = ref<BotRow[]>([]);
  const channels = ref<ChannelRow[]>([]);
  const form = ref({
    tgAccountId: null as number | null,
    botId: null as number | null,
    targetChatId: null as string | null,
    title: '',
    capacity: 2 as number | null,
  });
  const mode = ref<'existing' | 'create'>('existing');
  const editingId = ref(0);
  const loading = ref(false);
  const listLoading = ref(false);
  const busyRows = ref(new Set<number>());
  let listSequence = 0;
  let optionSequence = 0;
  let channelSequence = 0;
  let refreshTimer: ReturnType<typeof setTimeout> | undefined;
  let disposed = false;
  const canSave = computed(() => hasPermission(['/youban_publish/publish/relay/channel/save']));
  const canCreate = computed(() => hasPermission(['/youban_publish/publish/relay/channel/create']));
  const canStatus = computed(() => hasPermission(['/youban_publish/publish/relay/channel/status']));
  const accountOptions = computed(() =>
    accounts.value.map((row) => ({
      label: row.displayName || row.phone || `账号 ${row.id}`,
      value: row.id,
    }))
  );
  const botOptions = computed(() =>
    [...new Map(bots.value.map((row) => [row.id, row])).values()]
      .filter((row) => row.tenantId === 0 || row.tenantId === selectedTenantId.value)
      .map((row) => ({ label: row.botName || row.botUsername || `Bot ${row.id}`, value: row.id }))
  );
  const channelOptions = computed(() =>
    channels.value.map((row) => ({
      label: `${row.channelTitle || row.channelId} (${row.channelId})`,
      value: row.channelId,
    }))
  );

  async function load() {
    const sequence = ++listSequence;
    if (refreshTimer) clearTimeout(refreshTimer);
    if (!selectedTenantId.value) {
      rows.value = [];
      return;
    }
    listLoading.value = true;
    try {
      const res = await RelayChannelList({ tenantId: selectedTenantId.value });
      if (sequence !== listSequence || disposed) return;
      rows.value = res?.list || [];
      if (
        rows.value.some((row) => ['pending', 'creating', 'checking'].includes(row.operationStatus))
      ) {
        refreshTimer = setTimeout(() => {
          void load();
        }, 3000);
      }
    } catch {
      // The HTTP wrapper displays the error. Stop polling until an explicit refresh.
    } finally {
      if (sequence === listSequence) listLoading.value = false;
    }
  }

  async function loadOptions() {
    const sequence = ++optionSequence;
    const tenantId = selectedTenantId.value;
    editingId.value = 0;
    accounts.value = [];
    bots.value = [];
    channels.value = [];
    form.value = { tgAccountId: null, botId: null, targetChatId: null, title: '', capacity: 2 };
    if (!tenantId) return;
    const results = await Promise.allSettled([
      ServerTgAccountList({ tenantId, page: 1, perPage: 100 }),
      BotList({ tenantId, page: 1, perPage: 500, status: 1 }),
      BotList({ page: 1, perPage: 500, status: 1 }),
    ]);
    if (sequence !== optionSequence || disposed) return;
    if (results[0].status === 'fulfilled') accounts.value = results[0].value?.list || [];
    if (results[1].status === 'fulfilled') bots.value = results[1].value?.list || [];
    if (results[2].status === 'fulfilled')
      bots.value.push(
        ...(results[2].value?.list || []).filter((row: BotRow) => row.tenantId === 0)
      );
  }

  watch(
    () => [props.tenantId, props.tenantOptions] as const,
    () => {
      const next = props.tenantId || selectedTenantId.value || props.tenantOptions?.[0]?.value || 0;
      if (next !== selectedTenantId.value) selectedTenantId.value = next;
    },
    { immediate: true, deep: true }
  );
  watch(
    selectedTenantId,
    () => {
      void loadOptions();
      void load();
    },
    { immediate: true }
  );
  watch(
    () => form.value.tgAccountId,
    async (accountId) => {
      const sequence = ++channelSequence;
      channels.value = [];
      form.value.targetChatId = null;
      if (!accountId) return;
      try {
        const res = await AdminChannelCacheList({
          tgAccountId: accountId,
          page: 1,
          perPage: 100,
          displayType: 'channel',
          canAddAdmins: 1,
        });
        if (sequence === channelSequence && !disposed) channels.value = res?.list || [];
      } catch {
        /* The HTTP wrapper displays the error. */
      }
    }
  );

  async function edit(row: RelayChannelModel) {
    editingId.value = row.id;
    mode.value = 'existing';
    form.value.tgAccountId = row.tgAccountId;
    await nextTick();
    form.value = {
      tgAccountId: row.tgAccountId,
      botId: row.botId,
      targetChatId: row.targetChatId,
      title: row.title,
      capacity: row.capacity || 2,
    };
    channels.value = [{ channelId: row.targetChatId, channelTitle: row.title }];
  }

  async function save() {
    const { tgAccountId, botId, targetChatId, title, capacity } = form.value;
    if (
      !selectedTenantId.value ||
      !tgAccountId ||
      !botId ||
      (mode.value === 'existing' && !targetChatId) ||
      (mode.value === 'create' && !title.trim())
    ) {
      message.warning('请选择租户、协议账号、Bot及频道，创建频道时请填写名称');
      return;
    }
    loading.value = true;
    try {
      const params = {
        tenantId: selectedTenantId.value,
        tgAccountId,
        botId,
        title: title.trim(),
        capacity: capacity || 2,
      };
      if (mode.value === 'create') await RelayChannelCreate(params);
      else
        await RelayChannelSave({
          ...params,
          id: editingId.value || undefined,
          targetChatId: targetChatId!,
        });
      message.success('管理任务已提交，请等待权限检查完成后启用');
      await load();
    } catch {
      /* The HTTP wrapper displays the error. */
    } finally {
      loading.value = false;
    }
  }

  async function toggle(row: RelayChannelModel) {
    if (busyRows.value.has(row.id)) return;
    busyRows.value = new Set([...busyRows.value, row.id]);
    try {
      await RelayChannelStatus({
        id: row.id,
        status: row.status === 'enabled' ? 'disabled' : 'enabled',
      });
      await load();
    } catch {
      /* The HTTP wrapper displays the error. */
    } finally {
      const next = new Set(busyRows.value);
      next.delete(row.id);
      busyRows.value = next;
    }
  }
  onBeforeUnmount(() => {
    disposed = true;
    listSequence++;
    optionSequence++;
    channelSequence++;
    if (refreshTimer) clearTimeout(refreshTimer);
  });
</script>

<template>
  <n-space vertical size="large">
    <n-select
      v-model:value="selectedTenantId"
      :options="props.tenantOptions || []"
      placeholder="选择租户"
    />
    <n-radio-group v-model:value="mode" :disabled="!!editingId">
      <n-radio-button value="existing">添加已有频道</n-radio-button>
      <n-radio-button value="create" :disabled="!canCreate">创建新频道</n-radio-button>
    </n-radio-group>
    <n-form :model="form" label-placement="left" label-width="100">
      <n-form-item label="协议账号"
        ><n-select
          v-model:value="form.tgAccountId"
          :disabled="!!editingId"
          :options="accountOptions"
          filterable
          placeholder="选择管理中转频道的协议账号"
      /></n-form-item>
      <n-form-item label="初始Bot"
        ><n-select
          v-model:value="form.botId"
          :options="botOptions"
          filterable
          placeholder="选择推送Bot，后续Bot将在首次使用时自动接入"
      /></n-form-item>
      <n-form-item v-if="mode === 'existing'" label="频道"
        ><n-select
          v-model:value="form.targetChatId"
          :disabled="!!editingId"
          :options="channelOptions"
          filterable
          placeholder="选择频道缓存；没有结果时请先刷新协议账号频道"
      /></n-form-item>
      <n-form-item label="名称"
        ><n-input v-model:value="form.title" placeholder="频道名称"
      /></n-form-item>
      <n-form-item label="准备并发"
        ><n-input-number v-model:value="form.capacity" :min="1" :max="20"
      /></n-form-item>
      <n-button
        type="primary"
        :loading="loading"
        :disabled="mode === 'create' ? !canCreate : !canSave"
        @click="save"
        >提交中转频道配置</n-button
      >
    </n-form>
    <n-button v-if="editingId" @click="loadOptions">取消编辑</n-button>
    <n-button :loading="listLoading" @click="load">刷新频道状态</n-button>
    <n-empty v-if="!listLoading && !rows.length" description="暂无中转频道" />
    <n-card v-for="row in rows" :key="row.id" :title="row.title || row.targetChatId" size="small">
      <n-space vertical>
        <n-space align="center">
          <n-button
            v-if="canSave"
            size="small"
            :disabled="['pending', 'creating', 'checking'].includes(row.operationStatus)"
            @click="edit(row)"
            >编辑配置</n-button
          >
          <n-switch
            :value="row.status === 'enabled'"
            :loading="busyRows.has(row.id)"
            :disabled="
              !canStatus || ['pending', 'creating', 'checking'].includes(row.operationStatus)
            "
            @update:value="toggle(row)"
          />
          <n-tag>{{ dict.getLabel('YoubanRelayChannelStatusOptions', row.status) }}</n-tag>
          <n-tag>{{ dict.getLabel('YoubanRelayChannelHealthOptions', row.healthStatus) }}</n-tag>
          <n-tag>{{
            dict.getLabel('YoubanRelayOperationStatusOptions', row.operationStatus)
          }}</n-tag>
        </n-space>
        <span
          >{{ row.targetChatId }} · 协议账号 {{ row.tgAccountId }} · 容量 {{ row.capacity }} ·
          准备中 {{ row.inFlightCount }}</span
        >
        <span v-if="row.cooldownUntil">冷却至：{{ row.cooldownUntil }}</span>
        <n-alert v-if="row.lastError" type="warning" :show-icon="false">{{
          row.lastError
        }}</n-alert>
      </n-space>
    </n-card>
  </n-space>
</template>
