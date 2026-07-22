<template>
  <n-data-table
    :columns="columns"
    :data="list"
    :loading="loading"
    :pagination="pagination"
    :row-key="(row) => row.id"
    :scroll-x="1180"
    size="small"
    remote
  />
</template>

<script lang="ts" setup>
  import { h, reactive, ref, watch } from 'vue';
  import { NTag } from 'naive-ui';
  import { VipLogList } from '@/api/addons/youbanPublish';

  const props = defineProps<{ active: boolean; tenantId?: number | null }>();
  const loading = ref(false);
  const list = ref<Recordable[]>([]);

  const pagination = reactive({
    page: 1,
    pageSize: 20,
    itemCount: 0,
    showSizePicker: true,
    pageSizes: [10, 20, 50],
    onUpdatePage: (page: number) => {
      pagination.page = page;
      load();
    },
    onUpdatePageSize: (pageSize: number) => {
      pagination.pageSize = pageSize;
      pagination.page = 1;
      load();
    },
  });

  const columns = [
    { title: '账号归属', key: 'tenantName', width: 160, render: (row) => row.tenantName || '-' },
    { title: '来源', key: 'source', width: 120, render: (row) => sourceLabel(row.source) },
    { title: '操作', key: 'action', width: 100, render: (row) => renderAction(row.action) },
    { title: '变更前', key: 'beforeLevel', width: 160, render: (row) => renderVip(row.beforeLevel, row.beforeExpiredAt) },
    { title: '变更后', key: 'afterLevel', width: 160, render: (row) => renderVip(row.afterLevel, row.afterExpiredAt) },
    { title: '操作人', key: 'operatorId', width: 100, render: (row) => row.operatorId || '-' },
    { title: '备注', key: 'remark', width: 220, ellipsis: { tooltip: true } },
    { title: '时间', key: 'createdAt', width: 170 },
  ];

  function planLabel(level: number) {
    if (level === 2) return 'SVIP计划';
    if (level === 1) return 'VIP计划';
    return '免费计划';
  }

  function sourceLabel(source: string) {
    if (source === 'admin_adjust') return '后台修改';
    if (source === 'pay') return '会员充值';
    if (source === 'invite_reward') return '邀请奖励';
    return source || '-';
  }

  function renderAction(action: string) {
    const type = action === 'cancel' ? 'warning' : action === 'adjust' ? 'info' : 'success';
    const label = action === 'cancel' ? '取消' : action === 'adjust' ? '调整' : '开通';
    return h(NTag, { bordered: false, size: 'small', type }, { default: () => label });
  }

  function renderVip(level: number, expiredAt?: string) {
    if (!level) return '免费计划';
    return `${planLabel(level)} ${expiredAt || ''}`.trim();
  }

  async function load(reset = false) {
    if (!props.active) return;
    if (reset) pagination.page = 1;
    loading.value = true;
    try {
      const res: any = await VipLogList({
        page: pagination.page,
        pageSize: pagination.pageSize,
        tenantId: props.tenantId,
      });
      list.value = res?.list || [];
      pagination.itemCount = res?.totalCount || res?.total || 0;
    } finally {
      loading.value = false;
    }
  }

  watch(
    () => [props.active, props.tenantId],
    () => load(true),
    { immediate: true }
  );

  defineExpose({ load });
</script>
