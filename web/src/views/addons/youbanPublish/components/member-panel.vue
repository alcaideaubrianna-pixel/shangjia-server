<template>
  <div>
    <n-space class="toolbar" align="center" justify="space-between">
      <n-space align="center">
        <n-select
          v-model:value="query.tenantId"
          :options="tenantOptionsWithAll"
          clearable
          filterable
          placeholder="账号归属"
          class="tenant-select"
          @update:value="reloadCurrentPane"
        />
        <n-button @click="reloadCurrentPane">查询</n-button>
      </n-space>
      <n-space>
        <n-button @click="openConfigModal">会员配置</n-button>
        <n-button @click="openFeatureModal">权限管理</n-button>
        <n-button @click="openTenantVipModal()">账号会员</n-button>
        <n-button type="primary" @click="openCouponModal()">创建优惠码</n-button>
      </n-space>
    </n-space>

    <n-tabs v-model:value="activePane" type="line" animated @update:value="handlePaneChange">
      <n-tab-pane name="orders" tab="会员订单">
        <n-data-table
          :columns="orderColumns"
          :data="orders"
          :loading="orderLoading"
          :pagination="orderPagination"
          :row-key="(row) => row.id"
          :scroll-x="1120"
          size="small"
          remote
        />
      </n-tab-pane>
      <n-tab-pane name="logs" tab="会员记录">
        <VipLogTable
          ref="vipLogTableRef"
          :active="activePane === 'logs'"
          :tenant-id="query.tenantId"
        />
      </n-tab-pane>
      <n-tab-pane name="coupons" tab="优惠码">
        <n-data-table
          :columns="couponColumns"
          :data="coupons"
          :loading="couponLoading"
          :pagination="couponPagination"
          :row-key="(row) => row.id"
          :scroll-x="1060"
          size="small"
          remote
        />
      </n-tab-pane>
    </n-tabs>

    <n-modal
      v-model:show="configVisible"
      preset="dialog"
      title="会员配置"
      positive-text="保存"
      negative-text="取消"
      @positive-click="saveConfig"
    >
      <n-form :model="configForm" label-placement="left" label-width="100">
        <n-form-item label="启用会员"><n-switch v-model:value="configForm.enabled" /></n-form-item>
        <n-form-item label="会员月价"
          ><n-input-number
            v-model:value="configForm.monthlyPrice"
            :min="0"
            :precision="2"
            clearable
        /></n-form-item>
        <n-form-item label="展示原价"
          ><n-input-number
            v-model:value="configForm.originalPrice"
            :min="0"
            :precision="2"
            clearable
        /></n-form-item>
        <n-form-item label="支付网关">
          <n-select v-model:value="configForm.paymentGateway" :options="paymentGatewayOptions" />
        </n-form-item>
        <n-form-item label="币种">
          <n-select v-model:value="configForm.currency" :options="currencyOptions" />
        </n-form-item>
        <n-form-item label="折扣文案"
          ><n-input v-model:value="configForm.discountText" clearable
        /></n-form-item>
        <n-form-item label="邀请奖励"
          ><n-input-number v-model:value="configForm.inviteRewardDays" :min="0" clearable
        /></n-form-item>
        <n-form-item label="活动标题"
          ><n-input v-model:value="configForm.activityTitle" clearable
        /></n-form-item>
        <n-form-item label="活动说明"
          ><n-input v-model:value="configForm.activityText" type="textarea" clearable
        /></n-form-item>
      </n-form>
    </n-modal>

    <n-modal
      v-model:show="featureVisible"
      preset="dialog"
      title="功能权限管理"
      positive-text="保存"
      negative-text="取消"
      :loading="featureSaving"
      @positive-click="saveFeatures"
    >
      <n-form label-placement="top">
        <n-form-item label="账号归属">
          <n-select v-model:value="featureTenantId" :options="tenantOptions" disabled />
        </n-form-item>
        <n-space vertical size="large">
          <div v-for="item in featureOptions" :key="item.code">
            <n-space align="center" justify="space-between">
              <div>
                <div>{{ item.name }}</div>
                <n-text depth="3">{{ item.description }}</n-text>
              </div>
              <n-switch v-model:value="item.enabled" />
            </n-space>
          </div>
        </n-space>
      </n-form>
    </n-modal>

    <n-modal
      v-model:show="couponVisible"
      preset="dialog"
      title="优惠码"
      positive-text="保存"
      negative-text="取消"
      @positive-click="saveCoupon"
    >
      <n-form :model="couponForm" label-placement="left" label-width="100">
        <n-form-item label="优惠码">
          <n-input v-model:value="couponForm.code" clearable placeholder="不填写则后端随机生成" />
        </n-form-item>
        <n-form-item label="类型">
          <n-radio-group v-model:value="couponForm.useType">
            <n-radio-button value="single">单次使用</n-radio-button>
            <n-radio-button value="multi">重复使用</n-radio-button>
          </n-radio-group>
        </n-form-item>
        <n-form-item v-if="couponForm.useType === 'multi'" label="可用次数">
          <n-input-number v-model:value="couponForm.totalCount" :min="1" clearable />
        </n-form-item>
        <n-form-item label="优惠金额"
          ><n-input-number v-model:value="couponForm.amount" :min="0" :precision="2" clearable
        /></n-form-item>
        <n-form-item label="有效期"
          ><n-date-picker v-model:value="couponForm.expiredAt" type="datetime" clearable
        /></n-form-item>
        <n-form-item label="备注"
          ><n-input v-model:value="couponForm.remark" type="textarea" clearable
        /></n-form-item>
      </n-form>
    </n-modal>

    <n-modal v-model:show="tenantVipVisible" preset="dialog" title="账号会员配置">
      <n-form :model="tenantVipForm" label-placement="left" label-width="100">
        <n-form-item label="账号归属">
          <n-select v-model:value="tenantVipForm.tenantId" :options="tenantOptions" filterable />
        </n-form-item>
        <n-form-item label="会员计划">
          <n-select v-model:value="tenantVipForm.level" :options="memberPlanOptions" />
        </n-form-item>
        <n-form-item v-if="tenantVipForm.level > 0" label="到期时间"
          ><n-date-picker v-model:value="tenantVipForm.expiredAt" type="datetime" clearable
        /></n-form-item>
        <n-form-item label="调整备注"
          ><n-input v-model:value="tenantVipForm.remark" type="textarea" clearable
        /></n-form-item>
      </n-form>
      <template #action>
        <n-space justify="end">
          <n-button @click="tenantVipVisible = false">取消</n-button>
          <n-button
            v-if="tenantVipForm.level > 0"
            :loading="tenantVipSaving"
            type="warning"
            @click="cancelTenantVip"
          >
            取消会员
          </n-button>
          <n-button :loading="tenantVipSaving" type="primary" @click="saveTenantVip">保存</n-button>
        </n-space>
      </template>
    </n-modal>
  </div>
</template>

<script lang="ts" setup>
  import { computed, h, onMounted, reactive, ref } from 'vue';
  import { NButton, NSpace, NTag, useMessage } from 'naive-ui';
  import {
    TenantList,
    VipConfigSave,
    VipConfigView,
    VipCouponList,
    VipCouponSave,
    VipCouponStatus,
    VipFeatureSave,
    VipFeatureView,
    VipOrderList,
    VipTenantSave,
  } from '@/api/addons/youbanPublish';
  import VipLogTable from './VipLogTable.vue';
  import { currencyOptions, defaultConfig, paymentGatewayOptions } from './member-panel-config';

  const message = useMessage();
  const activePane = ref('orders');
  const configVisible = ref(false);
  const couponVisible = ref(false);
  const tenantVipVisible = ref(false);
  const featureVisible = ref(false);
  const featureSaving = ref(false);
  const featureTenantId = ref<number | null>(null);
  const featureOptions = ref<Recordable[]>([]);
  const orderLoading = ref(false);
  const couponLoading = ref(false);
  const tenantVipSaving = ref(false);
  const vipLogTableRef = ref<{ load: (reset?: boolean) => Promise<void> }>();
  const tenants = ref<Recordable[]>([]);
  const orders = ref<Recordable[]>([]);
  const coupons = ref<Recordable[]>([]);
  const query = reactive({ tenantId: null as number | null });
  const configForm = reactive(defaultConfig());
  const couponForm = reactive(defaultCoupon());
  const tenantVipForm = reactive(defaultTenantVip());

  const tenantOptions = computed(() =>
    tenants.value.map((item) => ({ label: tenantName(item), value: item.id }))
  );
  const tenantOptionsWithAll = computed(() => [
    { label: '全部账号归属', value: null },
    ...tenantOptions.value,
  ]);
  const memberPlanOptions = [
    { label: '免费计划', value: 0 },
    { label: 'VIP计划', value: 1 },
    { label: 'SVIP计划', value: 2 },
  ];

  const orderPagination = reactive({
    page: 1,
    pageSize: 20,
    itemCount: 0,
    showSizePicker: true,
    pageSizes: [10, 20, 50],
    onUpdatePage: (page: number) => {
      orderPagination.page = page;
      loadOrders();
    },
    onUpdatePageSize: (pageSize: number) => {
      orderPagination.pageSize = pageSize;
      orderPagination.page = 1;
      loadOrders();
    },
  });
  const couponPagination = reactive({
    page: 1,
    pageSize: 20,
    itemCount: 0,
    showSizePicker: true,
    pageSizes: [10, 20, 50],
    onUpdatePage: (page: number) => {
      couponPagination.page = page;
      loadCoupons();
    },
    onUpdatePageSize: (pageSize: number) => {
      couponPagination.pageSize = pageSize;
      couponPagination.page = 1;
      loadCoupons();
    },
  });

  const orderColumns = [
    { title: '订单号', key: 'orderNo', width: 210 },
    { title: '账号归属', key: 'tenantName', width: 160 },
    { title: '套餐', key: 'planName', width: 120 },
    {
      title: '金额',
      key: 'amount',
      width: 100,
      render: (row) => `${row.amount || 0}${row.currency || 'U'}`,
    },
    {
      title: '状态',
      key: 'statusText',
      width: 100,
      render: (row) => renderStatus(row.status, row.statusText),
    },
    { title: '支付时间', key: 'paidAt', width: 170 },
    { title: '创建时间', key: 'createdAt', width: 170 },
  ];

  const couponColumns = [
    { title: '优惠码', key: 'code', width: 180 },
    {
      title: '类型',
      key: 'useType',
      width: 110,
      render: (row) => (row.useType === 'multi' ? '重复使用' : '单次使用'),
    },
    { title: '金额', key: 'amount', width: 100, render: (row) => `${row.amount || 0}U` },
    {
      title: '使用次数',
      key: 'usedCount',
      width: 120,
      render: (row) => `${row.usedCount || 0}/${row.totalCount || 1}`,
    },
    { title: '状态', key: 'status', width: 100, render: (row) => renderCouponStatus(row.status) },
    { title: '有效期', key: 'expiredAt', width: 170 },
    {
      title: '操作',
      key: 'actions',
      width: 130,
      fixed: 'right',
      render(row) {
        return h(
          NSpace,
          {},
          {
            default: () => [
              h(
                NButton,
                { size: 'small', onClick: () => openCouponModal(row) },
                { default: () => '编辑' }
              ),
              h(
                NButton,
                { size: 'small', onClick: () => toggleCoupon(row) },
                { default: () => (row.status === 1 ? '停用' : '启用') }
              ),
            ],
          }
        );
      },
    },
  ];

  function defaultCoupon() {
    return {
      amount: 0,
      code: '',
      expiredAt: null as number | null,
      id: 0,
      remark: '',
      totalCount: 1,
      useType: 'single',
    };
  }

  function defaultTenantVip() {
    return {
      expiredAt: null as number | null,
      level: 1,
      remark: '',
      tenantId: null as number | null,
    };
  }

  function tenantName(item: Recordable) {
    return item.name || item.username || item.remark || `账号归属#${item.id}`;
  }

  function renderStatus(status: number, text?: string) {
    return h(
      NTag,
      { size: 'small', type: status === 2 ? 'success' : 'warning' },
      { default: () => text || (status === 2 ? '已支付' : '待支付') }
    );
  }

  function renderCouponStatus(status: number) {
    return h(
      NTag,
      { size: 'small', type: status === 1 ? 'success' : 'warning' },
      { default: () => (status === 1 ? '启用' : '停用') }
    );
  }

  async function loadTenants() {
    const res = await TenantList({ page: 1, pageSize: 200, status: 0 });
    tenants.value = res?.list || [];
  }

  async function loadConfig() {
    const res = await VipConfigView();
    Object.assign(configForm, defaultConfig(), res?.data || res || {});
  }

  async function loadOrders() {
    orderLoading.value = true;
    try {
      const res = await VipOrderList({
        page: orderPagination.page,
        pageSize: orderPagination.pageSize,
        tenantId: query.tenantId,
      });
      orders.value = res?.list || [];
      orderPagination.itemCount = res?.totalCount || res?.total || 0;
    } finally {
      orderLoading.value = false;
    }
  }

  async function loadCoupons() {
    couponLoading.value = true;
    try {
      const res = await VipCouponList({
        page: couponPagination.page,
        pageSize: couponPagination.pageSize,
      });
      coupons.value = res?.list || [];
      couponPagination.itemCount = res?.totalCount || res?.total || 0;
    } finally {
      couponLoading.value = false;
    }
  }

  async function openConfigModal() {
    await loadConfig();
    configVisible.value = true;
  }

  async function openFeatureModal() {
    if (!query.tenantId) {
      message.warning('请先选择账号归属');
      return;
    }
    featureTenantId.value = query.tenantId;
    const res = await VipFeatureView({ tenantId: query.tenantId });
    featureOptions.value = (res?.list || []).map((item: Recordable) => ({ ...item }));
    featureVisible.value = true;
  }

  async function saveFeatures() {
    if (!featureTenantId.value) return false;
    featureSaving.value = true;
    try {
      await VipFeatureSave({
        tenantId: featureTenantId.value,
        features: featureOptions.value.filter((item) => item.enabled).map((item) => item.code),
      });
      message.success('功能权限已保存');
      featureVisible.value = false;
      return true;
    } finally {
      featureSaving.value = false;
    }
  }

  function openCouponModal(row?: Recordable) {
    Object.assign(couponForm, defaultCoupon(), row || {});
    couponVisible.value = true;
  }

  function openTenantVipModal(tenantId?: number) {
    const currentTenantId = tenantId || query.tenantId;
    const tenant = tenants.value.find((item) => item.id === currentTenantId);
    Object.assign(tenantVipForm, defaultTenantVip(), {
      expiredAt: normalizeTimeMillis(tenant?.vipExpiredAt),
      level: tenant?.vipLevel || 1,
      tenantId: currentTenantId,
    });
    tenantVipVisible.value = true;
  }

  async function openVipRecords(tenantId?: number) {
    query.tenantId = tenantId || null;
    activePane.value = 'logs';
    await vipLogTableRef.value?.load(true);
  }

  async function saveConfig() {
    await VipConfigSave(configForm);
    message.success('会员配置已保存');
  }

  async function saveCoupon() {
    await VipCouponSave(couponForm);
    message.success('优惠码已保存');
    await loadCoupons();
  }

  async function saveTenantVip() {
    tenantVipSaving.value = true;
    try {
      await VipTenantSave(tenantVipForm);
      message.success('账号会员已更新');
      tenantVipVisible.value = false;
      await refreshVipData();
    } finally {
      tenantVipSaving.value = false;
    }
  }

  async function cancelTenantVip() {
    Object.assign(tenantVipForm, { expiredAt: null, level: 0 });
    await saveTenantVip();
  }

  async function toggleCoupon(row: Recordable) {
    await VipCouponStatus({ id: row.id, status: row.status === 1 ? 2 : 1 });
    await loadCoupons();
  }

  async function handlePaneChange(tab: string) {
    if (tab === 'orders') await loadOrders();
    if (tab === 'logs') await vipLogTableRef.value?.load(true);
    if (tab === 'coupons') await loadCoupons();
  }

  async function reloadCurrentPane() {
    await handlePaneChange(activePane.value);
  }

  async function refreshVipData() {
    await loadTenants();
    if (activePane.value === 'orders') await loadOrders();
    if (activePane.value === 'logs') await vipLogTableRef.value?.load(true);
  }

  function normalizeTimeMillis(value?: number | string | null) {
    if (!value) return null;
    if (typeof value === 'number') return value;
    const time = new Date(value).getTime();
    return Number.isNaN(time) ? null : time;
  }

  onMounted(async () => {
    await loadTenants();
    await loadOrders();
  });

  defineExpose({ openTenantVipModal, openVipRecords });
</script>

<style scoped>
  .toolbar {
    margin-bottom: 12px;
  }

  .tenant-select {
    min-width: 180px;
  }
</style>
