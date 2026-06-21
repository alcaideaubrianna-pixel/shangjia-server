<template>
  <div>
    <div class="n-layout-page-header">
      <n-card :bordered="false" title="会员日志">
        记录后台修改、付费开通、自助开通等 VIP 状态变更。
      </n-card>
    </div>
    <n-card :bordered="false" class="proCard">
      <BasicForm
        ref="searchFormRef"
        @register="register"
        @submit="reloadTable"
        @reset="reloadTable"
        @keyup.enter="reloadTable"
      />
      <BasicTable
        ref="actionRef"
        :columns="columns"
        :request="loadDataTable"
        :row-key="(row) => row.id"
        :scroll-x="scrollX"
        :resizeHeightOffset="-10000"
      />
    </n-card>
  </div>
</template>

<script lang="ts" setup>
  import { computed, ref } from 'vue';
  import { BasicForm, useForm } from '@/components/Form/index';
  import { BasicTable } from '@/components/Table';
  import { VipLogList } from '@/api/org/user';
  import { adaTableScrollX } from '@/utils/hotgo';
  import { columns, schemas } from './model';

  const actionRef = ref();
  const searchFormRef = ref<any>({});

  const [register] = useForm({
    gridProps: { cols: '1 s:1 m:2 l:3 xl:4 2xl:4' },
    labelWidth: 80,
    schemas,
  });

  const scrollX = computed(() => {
    return adaTableScrollX(columns, 0);
  });

  const loadDataTable = async (res) => {
    return await VipLogList({ ...searchFormRef.value?.formModel, ...res });
  };

  function reloadTable() {
    actionRef.value?.reload?.();
  }
</script>
