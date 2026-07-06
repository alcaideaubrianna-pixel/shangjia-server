<template>
  <div>
    <n-space class="toolbar" align="center">
      <n-select
        v-model:value="query.status"
        :options="statusOptionsWithAll"
        clearable
        placeholder="状态"
        class="status-select"
      />
      <n-input v-model:value="query.keyword" placeholder="域名 / 账号 / 备注" clearable @keyup.enter="loadTasks" />
      <n-button @click="loadTasks">查询</n-button>
      <n-button type="primary" @click="openCreateModal">新建导入</n-button>
      <n-button @click="loadTasks">刷新</n-button>
    </n-space>

    <n-data-table
      :columns="columns"
      :data="tasks"
      :loading="loading"
      :pagination="pagination"
      :row-key="(row) => row.id"
      :scroll-x="1600"
      size="small"
      remote
    />

    <n-modal
      v-model:show="modalVisible"
      preset="dialog"
      title="新建旧站导入"
      positive-text="创建并入队"
      negative-text="取消"
      @positive-click="createTask"
    >
      <n-form :model="form" label-placement="left" label-width="110">
        <n-form-item label="旧站域名">
          <n-input v-model:value="form.baseUrl" clearable placeholder="https://example.com" />
        </n-form-item>
        <n-form-item label="旧站账号">
          <n-input v-model:value="form.username" clearable />
        </n-form-item>
        <n-form-item label="旧站密码">
          <n-input v-model:value="form.password" type="password" show-password-on="click" />
        </n-form-item>
        <n-form-item label="测试数量">
          <n-input-number v-model:value="form.limitCount" :min="0" :max="100000" class="w-full" />
        </n-form-item>
        <n-form-item label="每页数量">
          <n-input-number v-model:value="form.perPage" :min="1" :max="100" class="w-full" />
        </n-form-item>
        <n-form-item label="媒体并发">
          <n-input-number v-model:value="form.mediaConcurrency" :min="1" :max="20" class="w-full" />
        </n-form-item>
        <n-form-item label="代理池">
          <n-space vertical class="w-full">
            <n-switch v-model:value="proxyEnabled" />
            <n-input
              v-model:value="form.proxyPool"
              type="textarea"
              :autosize="{ minRows: 3, maxRows: 6 }"
              placeholder="一行一个代理地址"
            />
          </n-space>
        </n-form-item>
        <n-form-item label="备注">
          <n-input v-model:value="form.remark" type="textarea" :autosize="{ minRows: 2, maxRows: 4 }" />
        </n-form-item>
      </n-form>
    </n-modal>
  </div>
</template>

<script lang="ts" setup>
  import { computed, h, onMounted, reactive, ref } from 'vue';
  import { NButton, NProgress, NSpace, NTag, useMessage } from 'naive-ui';
  import {
    ImportTaskCancel,
    ImportTaskCreate,
    ImportTaskList,
    ImportTaskRetry,
    ImportTaskStart,
  } from '@/api/addons/youbanPublish';

  const message = useMessage();
  const loading = ref(false);
  const modalVisible = ref(false);
  const proxyEnabled = ref(false);
  const tasks = ref<Recordable[]>([]);
  const query = reactive({ status: undefined as string | undefined, keyword: '' });
  const form = reactive({
    sourceName: 'lyy_cms',
    baseUrl: '',
    username: '',
    password: '',
    limitCount: 100,
    perPage: 12,
    mediaConcurrency: 4,
    proxyEnabled: 0,
    proxyPool: '',
    remark: '',
  });
  const pagination = reactive({
    page: 1,
    pageSize: 20,
    itemCount: 0,
    showSizePicker: true,
    pageSizes: [10, 20, 50],
    onChange: (page: number) => {
      pagination.page = page;
      loadTasks();
    },
    onUpdatePageSize: (pageSize: number) => {
      pagination.pageSize = pageSize;
      pagination.page = 1;
      loadTasks();
    },
  });

  const statusOptions = [
    { label: '待执行', value: 'pending' },
    { label: '执行中', value: 'running' },
    { label: '成功', value: 'success' },
    { label: '失败', value: 'failed' },
    { label: '已取消', value: 'canceled' },
  ];
  const statusOptionsWithAll = computed(() => [{ label: '全部状态', value: undefined }, ...statusOptions]);

  const columns = [
    { title: 'ID', key: 'id', width: 80 },
    { title: '来源', key: 'sourceName', width: 110 },
    { title: '旧站域名', key: 'baseUrl', width: 220, ellipsis: { tooltip: true } },
    { title: '旧站账号', key: 'username', width: 140 },
    { title: '上架账号', key: 'accountName', width: 130 },
    {
      title: '状态',
      key: 'status',
      width: 100,
      render(row) {
        const map = {
          pending: ['default', '待执行'],
          running: ['warning', '执行中'],
          success: ['success', '成功'],
          failed: ['error', '失败'],
          canceled: ['default', '已取消'],
        };
        const item = map[row.status] || ['default', row.status || '-'];
        return h(NTag, { type: item[0] as any, bordered: false }, { default: () => item[1] });
      },
    },
    { title: '阶段', key: 'stage', width: 110 },
    {
      title: '进度',
      key: 'percent',
      width: 180,
      render(row) {
        return h(NProgress, {
          type: 'line',
          percentage: Math.min(100, Math.round(row.percent || 0)),
          indicatorPlacement: 'inside',
          processing: row.status === 'running',
        });
      },
    },
    { title: '资料', key: 'itemDone', width: 110, render: (row) => `${row.itemDone || 0}/${row.itemTotal || 0}` },
    { title: '媒体', key: 'mediaDone', width: 110, render: (row) => `${row.mediaDone || 0}/${row.mediaTotal || 0}` },
    { title: 'TG匹配', key: 'tgMatched', width: 100 },
    { title: '错误', key: 'errorMessage', width: 240, ellipsis: { tooltip: true } },
    { title: '更新时间', key: 'updatedAt', width: 180 },
    {
      title: '操作',
      key: 'actions',
      width: 210,
      fixed: 'right',
      render(row) {
        return h(NSpace, { size: 8 }, {
          default: () => [
            h(NButton, { size: 'small', disabled: row.status === 'running', onClick: () => startTask(row.id) }, { default: () => '启动' }),
            h(NButton, { size: 'small', disabled: row.status !== 'running', onClick: () => cancelTask(row.id) }, { default: () => '取消' }),
            h(NButton, { size: 'small', disabled: row.status === 'running', onClick: () => retryTask(row.id) }, { default: () => '重试' }),
          ],
        });
      },
    },
  ];

  onMounted(() => loadTasks());

  function openCreateModal() {
    modalVisible.value = true;
  }

  async function loadTasks() {
    loading.value = true;
    try {
      const res: any = await ImportTaskList({ ...query, page: pagination.page, perPage: pagination.pageSize });
      tasks.value = res?.list || [];
      pagination.itemCount = res?.totalCount || res?.total || 0;
    } finally {
      loading.value = false;
    }
  }

  async function createTask() {
    await ImportTaskCreate({ ...form, proxyEnabled: proxyEnabled.value ? 1 : 0 });
    message.success('导入任务已创建');
    await loadTasks();
  }

  async function startTask(id: number) {
    await ImportTaskStart({ id });
    message.success('任务已入队');
    await loadTasks();
  }

  async function cancelTask(id: number) {
    await ImportTaskCancel({ id });
    message.success('任务已取消');
    await loadTasks();
  }

  async function retryTask(id: number) {
    await ImportTaskRetry({ id });
    message.success('任务已重新入队');
    await loadTasks();
  }
</script>
