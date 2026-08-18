<template>
  <n-card :bordered="false" class="proCard">
    <n-space class="mb-4" align="center">
      <n-select
        v-model:value="status"
        :options="statusOptions"
        clearable
        placeholder="全部状态"
        class="status-select"
      />
      <n-button :loading="loading" @click="loadBindings">查询</n-button>
    </n-space>
    <n-data-table
      :columns="columns"
      :data="bindings"
      :loading="loading"
      :row-key="(row) => row.id"
      :scroll-x="980"
      size="small"
    />
  </n-card>
</template>

<script setup lang="ts">
  import { h, onMounted, ref } from 'vue';
  import type { DataTableColumns } from 'naive-ui';
  import { NButton, NPopconfirm, NSpace, NTag, useMessage } from 'naive-ui';
  import { BindingList, BindingStatus } from '@/api/addons/youbanOpen';

  type Status = 'approved' | 'blocked' | 'pending' | 'revoked';
  interface Binding {
    id: number;
    appId: string;
    appName?: string;
    tenantId: number;
    tenantName?: string;
    status: Status;
    reason?: string;
    requestedAt?: string;
    reviewedAt?: string;
  }

  const message = useMessage();
  const loading = ref(false);
  const status = ref<Status | null>(null);
  const bindings = ref<Binding[]>([]);
  const updatingId = ref<number | null>(null);
  const statusOptions = [
    { label: '待审核', value: 'pending' },
    { label: '已通过', value: 'approved' },
    { label: '已撤销', value: 'revoked' },
    { label: '已拉黑', value: 'blocked' },
  ];
  const statusMeta = {
    approved: { label: '已通过', type: 'success' },
    blocked: { label: '已拉黑', type: 'error' },
    pending: { label: '待审核', type: 'warning' },
    revoked: { label: '已撤销', type: 'default' },
  } as const;

  const columns: DataTableColumns<Binding> = [
    { title: '合作平台', key: 'appName', minWidth: 170, render: (row) => row.appName || row.appId },
    {
      title: '租户',
      key: 'tenantName',
      minWidth: 150,
      render: (row) => row.tenantName || `租户 ${row.tenantId}`,
    },
    { title: '申请时间', key: 'requestedAt', width: 180, render: (row) => row.requestedAt || '-' },
    {
      title: '状态',
      key: 'status',
      width: 100,
      render: (row) =>
        h(
          NTag,
          { type: statusMeta[row.status].type, bordered: false },
          { default: () => statusMeta[row.status].label }
        ),
    },
    {
      title: '说明',
      key: 'reason',
      minWidth: 160,
      ellipsis: { tooltip: true },
      render: (row) => row.reason || '-',
    },
    {
      title: '操作',
      key: 'actions',
      width: 210,
      fixed: 'right',
      render: (row) =>
        h(NSpace, null, {
          default: () => [
            action(row, 'approved', '通过', 'success'),
            action(row, 'revoked', '撤销', 'warning'),
            action(row, 'blocked', '拉黑', 'error'),
          ],
        }),
    },
  ];

  function action(
    row: Binding,
    next: Status,
    label: string,
    type: 'error' | 'success' | 'warning'
  ) {
    return h(
      NPopconfirm,
      { onPositiveClick: () => changeStatus(row, next) },
      {
        trigger: () =>
          h(
            NButton,
            {
              size: 'small',
              type,
              loading: updatingId.value === row.id,
              disabled: updatingId.value !== null || row.status === next,
            },
            { default: () => label }
          ),
        default: () => `确认将“${row.tenantName || `租户 ${row.tenantId}`}”设为${label}？`,
      }
    );
  }

  async function loadBindings() {
    loading.value = true;
    try {
      const result = (await BindingList({ status: status.value || '' })) as { list?: Binding[] };
      bindings.value = result?.list || [];
    } finally {
      loading.value = false;
    }
  }

  async function changeStatus(row: Binding, next: Status) {
    updatingId.value = row.id;
    try {
      await BindingStatus({ id: row.id, appId: row.appId, status: next, reason: '' });
      message.success('绑定状态已更新');
      await loadBindings();
    } catch (error) {
      message.error(error instanceof Error ? error.message : '绑定状态更新失败');
    } finally {
      updatingId.value = null;
    }
  }

  onMounted(loadBindings);
</script>

<style scoped>
  .status-select {
    width: 150px;
  }
</style>
