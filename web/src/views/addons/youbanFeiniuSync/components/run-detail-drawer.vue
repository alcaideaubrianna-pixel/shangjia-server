<template>
  <n-drawer v-model:show="visible" :width="960" placement="right">
    <n-drawer-content title="运行情况" closable>
      <n-spin :show="loading">
        <n-descriptions v-if="run" :column="3" bordered size="small" class="mb-3">
          <n-descriptions-item label="运行ID">{{ run.id }}</n-descriptions-item>
          <n-descriptions-item label="类型">{{ run.runType }}</n-descriptions-item>
          <n-descriptions-item label="状态"
            ><n-tag :type="tagType(run.status)">{{ run.status }}</n-tag></n-descriptions-item
          >
          <n-descriptions-item label="总数">{{ run.totalCount }}</n-descriptions-item>
          <n-descriptions-item label="新增">{{ run.createdCount }}</n-descriptions-item>
          <n-descriptions-item label="更新">{{ run.updatedCount }}</n-descriptions-item>
          <n-descriptions-item label="跳过">{{ run.skippedCount }}</n-descriptions-item>
          <n-descriptions-item label="失败">{{ run.failedCount }}</n-descriptions-item>
          <n-descriptions-item label="开始时间">{{ run.startedAt || '-' }}</n-descriptions-item>
        </n-descriptions>
        <n-alert v-if="run?.errorMessage" type="error" class="mb-3">{{ run.errorMessage }}</n-alert>
        <n-collapse v-if="run?.runtimeLog" class="mb-3">
          <n-collapse-item title="运行日志" name="log"
            ><n-code :code="run.runtimeLog" language="text" word-wrap
          /></n-collapse-item>
        </n-collapse>
        <n-space class="toolbar" align="center">
          <n-input
            v-model:value="query.keyword"
            placeholder="笔记编号 / 频道 / 错误"
            clearable
            @keyup.enter="loadItems"
          />
          <n-select
            v-model:value="query.status"
            :options="statusOptions"
            clearable
            placeholder="状态"
            class="status-select"
          />
          <n-select
            v-model:value="query.action"
            :options="actionOptions"
            clearable
            placeholder="动作"
            class="status-select"
          />
          <n-button @click="loadItems">查询明细</n-button>
        </n-space>
        <n-data-table
          :columns="columns"
          :data="items"
          :loading="itemLoading"
          :pagination="pagination"
          :row-key="(row) => row.id"
          :scroll-x="1180"
          size="small"
          remote
        />
      </n-spin>
    </n-drawer-content>
  </n-drawer>
</template>

<script setup lang="ts">
  import { computed, h, reactive, ref } from 'vue';
  import { NTag } from 'naive-ui';
  import { RunItems, RunView } from '@/api/addons/youbanFeiniuSync';

  const props = defineProps<{ show: boolean; runId: number | null }>();
  const emit = defineEmits<{ 'update:show': [value: boolean] }>();
  const visible = computed({ get: () => props.show, set: (value) => emit('update:show', value) });
  const loading = ref(false);
  const itemLoading = ref(false);
  const run = ref<any>(null);
  const items = ref<any[]>([]);
  const query = reactive({
    keyword: '',
    status: null as string | null,
    action: null as string | null,
  });
  const pagination = reactive({
    page: 1,
    pageSize: 20,
    itemCount: 0,
    onUpdatePage: (page: number) => {
      pagination.page = page;
      loadItems();
    },
  });
  const statusOptions = [
    { label: '成功', value: 'success' },
    { label: '失败', value: 'failed' },
  ];
  const actionOptions = [
    { label: '新增', value: 'created' },
    { label: '更新', value: 'updated' },
    { label: '跳过', value: 'skipped' },
    { label: '失败', value: 'failed' },
    { label: '待补全', value: 'pending_media' },
  ];
  const tagType = (status: string) =>
    status === 'success' ? 'success' : status === 'running' ? 'info' : 'error';
  const actionText: Record<string, string> = {
    created: '新增',
    updated: '更新',
    skipped: '跳过',
    failed: '失败',
    pending_media: '待补全',
  };
  const columns = [
    { title: '笔记ID', key: 'feiniuNoteId', width: 110 },
    { title: '笔记编号', key: 'feiniuNoteCode', width: 120 },
    { title: '频道', key: 'feiniuChannelTitle', width: 180 },
    { title: '资料ID', key: 'youbanProfileId', width: 100 },
    { title: '任务ID', key: 'youbanTaskId', width: 100 },
    {
      title: '动作',
      key: 'action',
      width: 90,
      render: (row) => actionText[row.action] || row.action,
    },
    {
      title: '状态',
      key: 'status',
      width: 90,
      render: (row) =>
        h(NTag, { type: tagType(row.status), bordered: false }, { default: () => row.status }),
    },
    { title: '源更新时间', key: 'sourceUpdatedAt', width: 170 },
    { title: '耗时(ms)', key: 'durationMs', width: 100 },
    { title: '错误', key: 'errorMessage', ellipsis: { tooltip: true } },
  ];

  async function open(id: number) {
    loading.value = true;
    try {
      run.value = await RunView({ id });
      pagination.page = 1;
      await loadItems();
    } finally {
      loading.value = false;
    }
  }
  async function loadItems() {
    if (!props.runId) return;
    itemLoading.value = true;
    try {
      const res = await RunItems({
        runId: props.runId,
        ...query,
        page: pagination.page,
        pageSize: pagination.pageSize,
      });
      items.value = res.list || [];
      pagination.itemCount = res.totalCount || res.total || 0;
    } finally {
      itemLoading.value = false;
    }
  }
  defineExpose({ open });
</script>

<style scoped>
  .toolbar,
  .mb-3 {
    margin-bottom: 12px;
  }
  .status-select {
    width: 120px;
  }
</style>
