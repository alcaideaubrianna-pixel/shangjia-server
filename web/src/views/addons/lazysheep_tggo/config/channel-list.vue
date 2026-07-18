<template>
  <n-space vertical :size="16">
    <n-card :bordered="false">
      <n-space justify="space-between" align="center">
        <n-form inline :show-feedback="false">
          <n-form-item label="关键词">
            <n-input
              v-model:value="query.keyword"
              clearable
              placeholder="频道、链接、ID"
              @keyup.enter="load"
            />
          </n-form-item>
          <n-form-item label="状态">
            <n-select
              v-model:value="query.status"
              clearable
              :options="statusOptions"
              placeholder="全部"
            />
          </n-form-item>
        </n-form>
        <n-space>
          <n-button @click="reset">重置</n-button>
          <n-button type="primary" :loading="loading" @click="load">刷新</n-button>
        </n-space>
      </n-space>
    </n-card>

    <n-card :bordered="false">
      <n-data-table
        :columns="columns"
        :data="rows"
        :loading="loading"
        :pagination="{ pageSize: 12 }"
        :scroll-x="1880"
      />
    </n-card>
  </n-space>
</template>

<script lang="ts" setup>
  import { h, onMounted, reactive, ref } from 'vue';
  import { NButton, NEllipsis, NTag, useMessage } from 'naive-ui';
  import { channelList } from '@/api/addons/lazysheep_tggo/config';

  const message = useMessage();
  const loading = ref(false);
  const rows = ref<any[]>([]);
  const query = reactive({
    keyword: '',
    status: null as string | null,
  });

  const statusOptions = [
    { label: '正常', value: 'normal' },
    { label: '正在同步', value: 'running' },
    { label: '等待推送', value: 'pending' },
    { label: '待确认', value: 'unknown' },
    { label: '存在失败', value: 'failed' },
    { label: '自动拉取关闭', value: 'paused' },
    { label: '已停用', value: 'disabled' },
  ];

  const columns = [
    {
      title: '频道',
      key: 'chatLabel',
      width: 260,
      fixed: 'left',
      render(row) {
        return h('div', [
          h('div', row.chatLabel || row.chatTitle || row.chatId),
          h('div', { class: 'text-gray-400' }, `${row.chatId}${row.chatUsername ? ` · @${row.chatUsername}` : ''}`),
        ]);
      },
    },
    {
      title: '状态',
      key: 'workStatus',
      width: 120,
      render(row) {
        return h(
          NTag,
          { type: statusTagType(row.workStatusType), bordered: false },
          { default: () => row.workStatus || '未知' }
        );
      },
    },
    {
      title: '机器人',
      key: 'botName',
      width: 180,
      render(row) {
        return h('div', [
          h('div', row.botName || row.botKey),
          h('div', { class: 'text-gray-400' }, row.botKey),
        ]);
      },
    },
    {
      title: '绑定链接',
      key: 'sourceUrl',
      width: 320,
      render(row) {
        return h(NEllipsis, null, { default: () => row.sourceUrl || '-' });
      },
    },
    {
      title: '加入时间',
      key: 'addedAt',
      width: 170,
      render(row) {
        return row.addedAt || '-';
      },
    },
    {
      title: '添加人ID',
      key: 'addedBy',
      width: 140,
      render(row) {
        return row.addedBy || '-';
      },
    },
    {
      title: '已拉取笔记',
      key: 'noteCount',
      width: 120,
      render(row) {
        return row.noteCount || 0;
      },
    },
    {
      title: '队列',
      key: 'queue',
      width: 260,
      render(row) {
        return h('div', { class: 'queue-tags' }, [
          renderQueueTag('待', row.pending),
          renderQueueTag('中', row.doing),
          renderQueueTag('重', row.retry),
          renderQueueTag('成', row.done),
          renderQueueTag('败', row.dead, 'error'),
        ]);
      },
    },
    {
      title: '自动拉取',
      key: 'autoPull',
      width: 220,
      render(row) {
        if (row.autoPull) {
          return h(NTag, { type: 'success', bordered: false }, { default: () => '开启' });
        }
        return h('div', [
          h(NTag, { type: 'warning', bordered: false }, { default: () => '关闭' }),
          row.autoPullStopReason
            ? h('div', { class: 'text-gray-400' }, row.autoPullStopReason)
            : null,
        ]);
      },
    },
    {
      title: '拉取游标',
      key: 'cursor',
      width: 240,
      render(row) {
        return h('div', [
          h('div', `lastPullID: ${row.lastPullId || 0}`),
          h('div', { class: 'text-gray-400' }, `cursor: ${row.lastCursor || '-'}`),
        ]);
      },
    },
    {
      title: '最近错误',
      key: 'lastError',
      width: 280,
      render(row) {
        return h(NEllipsis, { lineClamp: 2 }, { default: () => row.lastError || '-' });
      },
    },
    {
      title: '操作',
      key: 'actions',
      width: 100,
      fixed: 'right',
      render(row) {
        return h(
          NButton,
          {
            size: 'small',
            quaternary: true,
            onClick: () => copy(row.sourceUrl),
          },
          { default: () => '复制链接' }
        );
      },
    },
  ];

  function statusTagType(status: string) {
    if (status === 'running') return 'info';
    if (status === 'pending') return 'warning';
    if (status === 'failed') return 'error';
    if (status === 'paused') return 'warning';
    if (status === 'disabled') return 'default';
    return 'success';
  }

  function renderQueueTag(label: string, value: number, type: any = 'info') {
    return h(NTag, { size: 'small', bordered: false, type }, { default: () => `${label}:${value || 0}` });
  }

  async function copy(text: string) {
    if (!text) return;
    await navigator.clipboard.writeText(text);
    message.success('已复制');
  }

  async function load() {
    loading.value = true;
    try {
      const res: any = await channelList({ ...query });
      rows.value = res?.list || [];
    } finally {
      loading.value = false;
    }
  }

  function reset() {
    query.keyword = '';
    query.status = null;
    load();
  }

  onMounted(load);
</script>

<style scoped>
  .queue-tags {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }
</style>
