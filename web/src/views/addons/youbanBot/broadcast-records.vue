<template>
  <div>
    <n-card :bordered="false" title="推送记录">
      <template #header-extra>
        <n-button @click="goBack">返回 Bot 管理</n-button>
      </template>
      <n-space class="mb-4">
        <n-select
          v-model:value="taskStatus"
          :options="taskStatusOptions"
          clearable
          placeholder="全部任务状态"
          class="status-select"
          @update:value="reloadTasks"
        />
        <n-button @click="reloadTasks">查询</n-button>
      </n-space>
      <n-data-table
        :columns="taskColumns"
        :data="taskRows"
        :loading="taskLoading"
        :row-key="(row) => row.id"
      />
      <n-pagination
        class="mt-4"
        v-model:page="taskPager.page"
        :page-size="taskPager.pageSize"
        :item-count="taskPager.total"
        show-quick-jumper
        @update:page="loadTasks"
      />
    </n-card>

    <n-modal v-model:show="detailVisible" preset="card" title="推送明细" class="detail-modal">
      <n-space class="mb-4">
        <n-select
          v-model:value="detailStatus"
          :options="detailStatusOptions"
          clearable
          placeholder="全部状态"
          class="status-select"
          @update:value="reloadDetails"
        />
        <n-input
          v-model:value="detailKeyword"
          clearable
          placeholder="TG 用户名或用户 ID"
          class="keyword-input"
          @keyup.enter="reloadDetails"
        />
        <n-button @click="reloadDetails">查询</n-button>
      </n-space>
      <n-data-table
        :columns="detailColumns"
        :data="detailRows"
        :loading="detailLoading"
        :row-key="(row) => row.id"
      />
      <n-pagination
        class="mt-4"
        v-model:page="detailPager.page"
        :page-size="detailPager.pageSize"
        :item-count="detailPager.total"
        show-quick-jumper
        @update:page="loadDetails"
      />
    </n-modal>
  </div>
</template>

<script setup lang="ts">
  import { h, onMounted, reactive, ref } from 'vue';
  import type { DataTableColumns } from 'naive-ui';
  import { NButton, NTag } from 'naive-ui';
  import { useRouter } from 'vue-router';

  import { BroadcastList, BroadcastRecipientList } from '@/api/addons/youbanBot';

  const router = useRouter();
  const taskRows = ref<any[]>([]);
  const taskLoading = ref(false);
  const taskStatus = ref<string | null>(null);
  const taskPager = reactive({ page: 1, pageSize: 10, total: 0 });
  const detailVisible = ref(false);
  const detailTaskId = ref(0);
  const detailRows = ref<any[]>([]);
  const detailLoading = ref(false);
  const detailStatus = ref<string | null>(null);
  const detailKeyword = ref('');
  const detailPager = reactive({ page: 1, pageSize: 20, total: 0 });

  const taskStatusOptions = [
    { label: '等待执行', value: 'pending' },
    { label: '发送中', value: 'running' },
    { label: '已完成', value: 'completed' },
    { label: '失败', value: 'failed' },
  ];
  const detailStatusOptions = [
    { label: '等待发送', value: 'pending' },
    { label: '发送成功', value: 'success' },
    { label: '发送失败', value: 'failed' },
  ];
  const taskLabels = {
    pending: '等待执行',
    running: '发送中',
    completed: '已完成',
    failed: '失败',
  };
  const detailLabels = { pending: '等待发送', success: '成功', failed: '失败' };

  const taskColumns: DataTableColumns<any> = [
    { title: '任务 ID', key: 'id', width: 90 },
    { title: '消息内容', key: 'text', ellipsis: { tooltip: true } },
    {
      title: '状态',
      key: 'status',
      width: 100,
      render: (row) => renderStatus(row.status, taskLabels),
    },
    { title: '收件人', key: 'totalCount', width: 80 },
    { title: '成功', key: 'successCount', width: 70 },
    { title: '失败', key: 'failedCount', width: 70 },
    { title: '不可达', key: 'blockedCount', width: 80 },
    { title: '创建时间', key: 'createdAt', width: 170 },
    {
      title: '操作',
      key: 'actions',
      width: 130,
      render: (row) =>
        h(
          NButton,
          { size: 'small', onClick: () => openDetails(row.id) },
          { default: () => '查看推送列表' }
        ),
    },
  ];
  const detailColumns: DataTableColumns<any> = [
    {
      title: 'Bot',
      key: 'botUsername',
      width: 150,
      render: (row) => (row.botUsername ? `@${row.botUsername}` : `Bot #${row.botId}`),
    },
    {
      title: 'TG 用户',
      key: 'telegramUsername',
      width: 180,
      render: (row) =>
        row.telegramUsername
          ? `@${row.telegramUsername}`
          : [row.telegramFirstName, row.telegramLastName].filter(Boolean).join(' ') || '-',
    },
    { title: 'TG 用户 ID', key: 'telegramUserId', width: 150 },
    {
      title: '状态',
      key: 'status',
      width: 100,
      render: (row) => renderStatus(row.status, detailLabels),
    },
    {
      title: '失败原因',
      key: 'errorMessage',
      ellipsis: { tooltip: true },
      render: (row) => row.errorMessage || '-',
    },
    { title: '发送时间', key: 'sentAt', width: 170, render: (row) => row.sentAt || '-' },
  ];

  function renderStatus(status: string, labels: Record<string, string>) {
    const type =
      status === 'success' || status === 'completed'
        ? 'success'
        : status === 'failed'
          ? 'error'
          : 'warning';
    return h(NTag, { type, bordered: false }, { default: () => labels[status] || status });
  }

  async function loadTasks(page = taskPager.page) {
    taskPager.page = page;
    taskLoading.value = true;
    try {
      const res: any = await BroadcastList({
        page,
        perPage: taskPager.pageSize,
        status: taskStatus.value || '',
      });
      taskRows.value = res?.list || [];
      taskPager.total = Number(res?.totalCount || 0);
    } finally {
      taskLoading.value = false;
    }
  }

  function reloadTasks() {
    loadTasks(1);
  }
  function openDetails(taskId: number) {
    detailTaskId.value = taskId;
    detailStatus.value = null;
    detailKeyword.value = '';
    detailVisible.value = true;
    loadDetails(1);
  }
  function reloadDetails() {
    loadDetails(1);
  }
  async function loadDetails(page = detailPager.page) {
    detailPager.page = page;
    detailLoading.value = true;
    try {
      const res: any = await BroadcastRecipientList({
        taskId: detailTaskId.value,
        status: detailStatus.value || '',
        keyword: detailKeyword.value,
        page,
        perPage: detailPager.pageSize,
      });
      detailRows.value = res?.list || [];
      detailPager.total = Number(res?.totalCount || 0);
    } finally {
      detailLoading.value = false;
    }
  }
  function goBack() {
    router.push({ name: 'youbanBot', query: {} });
  }

  onMounted(() => loadTasks());
</script>

<style scoped>
  .detail-modal {
    width: min(1200px, 94vw);
  }
  .status-select {
    width: 160px;
  }
  .keyword-input {
    width: 260px;
  }
</style>
