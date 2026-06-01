<template>
  <div>
    <div class="n-layout-page-header">
      <n-card :bordered="false" title="内容同步监控">
        FeiNiu 资料同步进度、去重结果、失败信息和手动同步入口
      </n-card>
    </div>

    <n-card :bordered="false" class="proCard">
      <n-grid cols="1 s:2 m:3 l:4 xl:4 2xl:4" responsive="screen" :x-gap="12" :y-gap="12">
        <n-grid-item v-for="item in stats" :key="item.label">
          <n-card size="small" :bordered="false" :segmented="{ content: true }">
            <template #header>
              <span>{{ item.label }}</span>
            </template>
            <div class="stat-value">{{ item.value }}</div>
            <div class="stat-desc">{{ item.desc }}</div>
          </n-card>
        </n-grid-item>
      </n-grid>

      <n-space class="mt-4" align="center">
        <n-input-number
          v-model:value="batchSize"
          :min="1"
          :max="1000"
          :step="50"
          placeholder="单次同步数量"
        />
        <n-button type="primary" :loading="running" @click="handleRun">
          手动同步
        </n-button>
        <n-button @click="refreshAll">刷新</n-button>
      </n-space>

      <n-alert v-if="overview.lastError" class="mt-4" type="error" :show-icon="false">
        最近错误：{{ overview.lastError }}
      </n-alert>
    </n-card>

    <n-card :bordered="false" class="proCard mt-4">
      <BasicForm ref="searchFormRef" @register="register" @submit="reloadTable" @reset="reloadTable" />
      <BasicTable
        ref="actionRef"
        :columns="columns"
        :request="loadDataTable"
        :row-key="(row) => row.id"
        :scroll-x="scrollX"
        :resizeHeightOffset="-10000"
        size="small"
      />
    </n-card>
  </div>
</template>

<script lang="ts" setup>
  import { computed, onMounted, ref } from 'vue';
  import { useMessage } from 'naive-ui';
  import { BasicForm, useForm } from '@/components/Form/index';
  import { BasicTable } from '@/components/Table';
  import { Overview, RunFeiNiu, RunList } from '@/api/contentImport';
  import { adaTableScrollX } from '@/utils/hotgo';
  import { columns, schemas } from './model';

  const message = useMessage();
  const actionRef = ref();
  const searchFormRef = ref<any>({});
  const batchSize = ref(200);
  const running = ref(false);
  const overview = ref<any>({});

  const [register] = useForm({
    gridProps: { cols: '1 s:1 m:2 l:3 xl:4 2xl:4' },
    labelWidth: 80,
    schemas,
  });

  const stats = computed(() => [
    { label: '资料总数', value: overview.value.profileTotal || 0, desc: '已迁移到 HotGo 的资料' },
    { label: '公开资料', value: overview.value.publicTotal || 0, desc: '前台可展示资料' },
    { label: '待审核', value: overview.value.pendingTotal || 0, desc: '默认导入后需审核' },
    { label: '重复资料', value: overview.value.duplicateTotal || 0, desc: 'duplicate_note_id / 文本哈希命中' },
    { label: '媒体总数', value: overview.value.mediaTotal || 0, desc: '图片和视频记录' },
    { label: '重复媒体', value: overview.value.duplicateMedia || 0, desc: 'MD5 复用展示图' },
    { label: '同步游标', value: overview.value.lastSourceNoteId || 0, desc: '最后来源 note_id' },
    { label: '最近状态', value: overview.value.lastRunStatus || '-', desc: `耗时 ${overview.value.lastRunCostMs || 0}ms` },
  ]);

  const scrollX = computed(() => {
    return adaTableScrollX(columns, 0);
  });

  const loadDataTable = async (res) => {
    return await RunList({ sourceName: 'feiniu', ...searchFormRef.value?.formModel, ...res });
  };

  function reloadTable() {
    actionRef.value?.reload();
  }

  async function loadOverview() {
    overview.value = await Overview({ sourceName: 'feiniu' });
  }

  async function refreshAll() {
    await loadOverview();
    reloadTable();
  }

  async function handleRun() {
    running.value = true;
    try {
      const res: any = await RunFeiNiu({ batchSize: batchSize.value });
      message.success(`同步完成：扫描 ${res.scanned}，导入 ${res.imported}，重复 ${res.duplicate}`);
      await refreshAll();
    } finally {
      running.value = false;
    }
  }

  onMounted(() => {
    refreshAll();
  });
</script>

<style lang="less" scoped>
  .stat-value {
    color: #111827;
    font-size: 28px;
    font-weight: 700;
    line-height: 1.2;
  }

  .stat-desc {
    margin-top: 6px;
    color: #6b7280;
    font-size: 12px;
  }
</style>
