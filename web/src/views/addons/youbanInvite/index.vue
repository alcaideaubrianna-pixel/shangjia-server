<template>
  <div>
    <div class="n-layout-page-header">
      <n-card :bordered="false" title="邀请返现">
        <span>管理邀请返现配置、返现账单和人工新增记录</span>
      </n-card>
    </div>

    <n-card :bordered="false" class="proCard">
      <n-tabs v-model:value="activeTab" type="line" animated>
        <n-tab-pane name="records" tab="返现记录">
          <n-space class="toolbar" align="center">
            <n-input v-model:value="query.keyword" clearable placeholder="订单号 / 用户名 / 手机号" @keyup.enter="loadRecords" />
            <n-select v-model:value="query.settleStatus" :options="statusOptionsWithAll" clearable placeholder="结算状态" class="status-select" />
            <n-button @click="loadRecords">查询</n-button>
            <n-button type="primary" @click="openRecordModal()">新增记录</n-button>
          </n-space>
          <n-data-table
            :columns="recordColumns"
            :data="records"
            :loading="recordLoading"
            :pagination="recordPagination"
            :row-key="(row) => row.id"
            :scroll-x="1280"
            size="small"
            remote
          />
        </n-tab-pane>

        <n-tab-pane name="config" tab="返现配置">
          <n-spin :show="configLoading">
            <n-form :model="configForm" label-placement="left" label-width="130" class="config-form">
              <n-form-item label="启用邀请返现">
                <n-switch v-model:value="configForm.enabled" :checked-value="1" :unchecked-value="2" />
              </n-form-item>
              <n-form-item label="邀请链接域名">
                <n-input v-model:value="configForm.baseUrl" placeholder="https://yuebanby.com" />
              </n-form-item>
              <n-grid :cols="2" :x-gap="16">
                <n-form-item-gi label="一档起始单数">
                  <n-input-number v-model:value="configForm.level1Min" :min="1" class="w-full" />
                </n-form-item-gi>
                <n-form-item-gi label="一档截止单数">
                  <n-input-number v-model:value="configForm.level1Max" :min="1" class="w-full" />
                </n-form-item-gi>
                <n-form-item-gi label="一档返现比例">
                  <n-input-number v-model:value="configForm.level1Rate" :min="0" :precision="2" class="w-full">
                    <template #suffix>%</template>
                  </n-input-number>
                </n-form-item-gi>
                <n-form-item-gi label="二档起始单数">
                  <n-input-number v-model:value="configForm.level2Min" :min="1" class="w-full" />
                </n-form-item-gi>
                <n-form-item-gi label="二档返现比例">
                  <n-input-number v-model:value="configForm.level2Rate" :min="0" :precision="2" class="w-full">
                    <template #suffix>%</template>
                  </n-input-number>
                </n-form-item-gi>
                <n-form-item-gi label="人工审核">
                  <n-switch v-model:value="configForm.manualAudit" :checked-value="1" :unchecked-value="0" />
                </n-form-item-gi>
              </n-grid>
              <n-form-item label="备注">
                <n-input v-model:value="configForm.remark" type="textarea" :autosize="{ minRows: 3, maxRows: 5 }" />
              </n-form-item>
              <n-space justify="end">
                <n-button @click="loadConfig">重置</n-button>
                <n-button type="primary" :loading="savingConfig" @click="saveConfig">保存配置</n-button>
              </n-space>
            </n-form>
          </n-spin>
        </n-tab-pane>
      </n-tabs>
    </n-card>

    <n-modal v-model:show="recordModalVisible" preset="dialog" title="邀请返现记录" positive-text="保存" negative-text="取消" @positive-click="saveRecord">
      <n-form :model="recordForm" label-placement="left" label-width="110">
        <n-form-item label="邀请人ID">
          <n-input-number v-model:value="recordForm.inviterId" :min="0" class="w-full" />
        </n-form-item>
        <n-form-item label="被邀请人ID">
          <n-input-number v-model:value="recordForm.inviteeId" :min="0" class="w-full" />
        </n-form-item>
        <n-form-item label="邀请码">
          <n-input v-model:value="recordForm.inviteCode" clearable />
        </n-form-item>
        <n-form-item label="订单号">
          <n-input v-model:value="recordForm.orderSn" clearable />
        </n-form-item>
        <n-form-item label="订单金额">
          <n-input-number v-model:value="recordForm.orderAmount" :min="0" :precision="2" class="w-full" />
        </n-form-item>
        <n-form-item label="返现比例">
          <n-input-number v-model:value="recordForm.rebateRate" :min="0" :precision="2" class="w-full">
            <template #suffix>%</template>
          </n-input-number>
        </n-form-item>
        <n-form-item label="返现金额">
          <n-input-number v-model:value="recordForm.rebateAmount" :min="0" :precision="2" class="w-full" />
        </n-form-item>
        <n-form-item label="结算状态">
          <n-select v-model:value="recordForm.settleStatus" :options="statusOptions" />
        </n-form-item>
        <n-form-item label="备注">
          <n-input v-model:value="recordForm.remark" type="textarea" :autosize="{ minRows: 2, maxRows: 4 }" />
        </n-form-item>
      </n-form>
    </n-modal>
  </div>
</template>

<script lang="ts" setup>
  import { h, onMounted, reactive, ref } from 'vue';
  import { NButton, NPopconfirm, NSpace, NTag, useMessage } from 'naive-ui';
  import { Config, Delete, List, SaveConfig, SaveRecord } from '@/api/addons/youbanInvite';

  const message = useMessage();
  const activeTab = ref('records');
  const recordLoading = ref(false);
  const configLoading = ref(false);
  const savingConfig = ref(false);
  const recordModalVisible = ref(false);
  const records = ref<any[]>([]);
  const query = reactive({ keyword: '', settleStatus: '' });
  const recordPagination = reactive({
    page: 1,
    pageSize: 10,
    itemCount: 0,
    showSizePicker: true,
    pageSizes: [10, 20, 50],
    onUpdatePage: (page: number) => {
      recordPagination.page = page;
      loadRecords();
    },
    onUpdatePageSize: (pageSize: number) => {
      recordPagination.pageSize = pageSize;
      recordPagination.page = 1;
      loadRecords();
    },
  });

  const statusOptions = [
    { label: '待结算', value: 'pending' },
    { label: '已结算', value: 'settled' },
    { label: '已取消', value: 'cancelled' },
  ];
  const statusOptionsWithAll = [{ label: '全部', value: '' }, ...statusOptions];

  const configForm = reactive({
    enabled: 1,
    baseUrl: 'https://yuebanby.com',
    level1Min: 1,
    level1Max: 5,
    level1Rate: 2,
    level2Min: 6,
    level2Rate: 3,
    manualAudit: 0,
    remark: '',
  });

  const recordForm = reactive({
    id: 0,
    inviterId: 0,
    inviteeId: 0,
    inviteCode: '',
    orderSn: '',
    tradeType: 'member_vip',
    orderAmount: 0,
    rebateRate: 2,
    rebateAmount: 0,
    settleStatus: 'settled',
    remark: '',
  });

  const recordColumns = [
    { title: 'ID', key: 'id', width: 80 },
    {
      title: '邀请人',
      key: 'inviterName',
      width: 180,
      render(row) {
        return `${row.inviterName || row.inviterId || '-'} ${row.inviterMobile || ''}`;
      },
    },
    {
      title: '被邀请人',
      key: 'inviteeName',
      width: 180,
      render(row) {
        return `${row.inviteeName || row.inviteeId || '-'} ${row.inviteeMobile || ''}`;
      },
    },
    { title: '订单号', key: 'orderSn', width: 190 },
    { title: '订单金额', key: 'orderAmount', width: 110, render: (row) => money(row.orderAmount) },
    { title: '返现比例', key: 'rebateRate', width: 100, render: (row) => `${row.rebateRate || 0}%` },
    { title: '返现金额', key: 'rebateAmount', width: 110, render: (row) => money(row.rebateAmount) },
    {
      title: '状态',
      key: 'settleStatus',
      width: 100,
      render(row) {
        const option = statusOptions.find((item) => item.value === row.settleStatus);
        const type = row.settleStatus === 'settled' ? 'success' : row.settleStatus === 'pending' ? 'warning' : 'default';
        return h(NTag, { type, bordered: false }, { default: () => option?.label || row.settleStatus });
      },
    },
    { title: '创建时间', key: 'createdAt', width: 170 },
    {
      title: '操作',
      key: 'actions',
      width: 150,
      fixed: 'right',
      render(row) {
        return h(NSpace, {}, {
          default: () => [
            h(NButton, { size: 'small', quaternary: true, type: 'primary', onClick: () => openRecordModal(row) }, { default: () => '编辑' }),
            h(NPopconfirm, { onPositiveClick: () => deleteRecord(row.id) }, {
              trigger: () => h(NButton, { size: 'small', quaternary: true, type: 'error' }, { default: () => '删除' }),
              default: () => '确认删除该返现记录？',
            }),
          ],
        });
      },
    },
  ];

  onMounted(() => {
    loadRecords();
    loadConfig();
  });

  async function loadRecords() {
    recordLoading.value = true;
    try {
      const res = await List({ ...query, page: recordPagination.page, pageSize: recordPagination.pageSize });
      records.value = res.list || [];
      recordPagination.itemCount = res.totalCount || 0;
    } finally {
      recordLoading.value = false;
    }
  }

  async function loadConfig() {
    configLoading.value = true;
    try {
      Object.assign(configForm, await Config());
    } finally {
      configLoading.value = false;
    }
  }

  async function saveConfig() {
    savingConfig.value = true;
    try {
      await SaveConfig(configForm);
      message.success('配置已保存');
      await loadConfig();
    } finally {
      savingConfig.value = false;
    }
  }

  function openRecordModal(row: any = null) {
    Object.assign(recordForm, {
      id: row?.id || 0,
      inviterId: row?.inviterId || 0,
      inviteeId: row?.inviteeId || 0,
      inviteCode: row?.inviteCode || '',
      orderSn: row?.orderSn || '',
      tradeType: row?.tradeType || 'member_vip',
      orderAmount: row?.orderAmount || 0,
      rebateRate: row?.rebateRate || configForm.level1Rate || 2,
      rebateAmount: row?.rebateAmount || 0,
      settleStatus: row?.settleStatus || 'settled',
      remark: row?.remark || '',
    });
    recordModalVisible.value = true;
  }

  async function saveRecord() {
    await SaveRecord(recordForm);
    message.success('记录已保存');
    await loadRecords();
  }

  async function deleteRecord(id: number) {
    await Delete({ ids: [id] });
    message.success('记录已删除');
    await loadRecords();
  }

  function money(value: number) {
    return `￥${Number(value || 0).toFixed(2)}`;
  }
</script>

<style scoped>
  .toolbar {
    margin-bottom: 16px;
  }

  .status-select {
    width: 140px;
  }

  .config-form {
    max-width: 860px;
  }
</style>
