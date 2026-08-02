<template>
  <div class="activity-panel">
    <n-tabs v-model:value="activePane" type="line" animated>
      <n-tab-pane name="config" tab="活动配置">
        <n-spin :show="activityLoading">
          <n-grid :cols="3" :x-gap="16" :y-gap="16" responsive="screen">
            <n-grid-item v-for="item in activities" :key="item.code">
              <n-card :title="item.name" size="small">
                <template #header-extra>
                  <n-tag :type="item.enabled ? 'success' : 'default'">
                    {{ item.enabled ? '已开启' : '已关闭' }}
                  </n-tag>
                </template>
                <n-text depth="3">{{ item.description }}</n-text>
                <n-space vertical class="activity-panel__card-body">
                  <n-space align="center" justify="space-between">
                    <span>活动开关</span>
                    <n-switch v-model:value="item.enabled" />
                  </n-space>
                  <n-space align="center" justify="space-between">
                    <span>奖励天数</span>
                    <n-input-number v-model:value="item.rewardDays" :min="1" :max="3650" />
                  </n-space>
                  <n-descriptions :column="1" label-placement="left" size="small">
                    <n-descriptions-item label="活动编码">{{ item.code }}</n-descriptions-item>
                    <n-descriptions-item label="累计奖励">
                      {{ item.rewardCount || 0 }} 次 / {{ item.rewardDaysTotal || 0 }} 天
                    </n-descriptions-item>
                    <n-descriptions-item label="启用时间">
                      {{ item.enabledAt || '-' }}
                    </n-descriptions-item>
                    <n-descriptions-item label="最后奖励">
                      {{ item.lastRewardAt || '-' }}
                    </n-descriptions-item>
                  </n-descriptions>
                  <n-button type="primary" block @click="saveActivity(item)">保存配置</n-button>
                </n-space>
              </n-card>
            </n-grid-item>
          </n-grid>
        </n-spin>
      </n-tab-pane>

      <n-tab-pane name="rewards" tab="奖励记录">
        <n-space class="mb-4" align="center">
          <n-select
            v-model:value="rewardQuery.activityCode"
            :options="activityOptions"
            clearable
            placeholder="活动"
            class="activity-panel__activity-select"
          />
          <n-select
            v-model:value="rewardQuery.tenantId"
            :options="tenantOptions"
            clearable
            filterable
            placeholder="账号归属"
            class="activity-panel__tenant-select"
          />
          <n-select
            v-model:value="rewardQuery.notifyStatus"
            :options="notifyStatusOptions"
            clearable
            placeholder="通知状态"
            class="activity-panel__status-select"
          />
          <n-input
            v-model:value="rewardQuery.keyword"
            clearable
            placeholder="账号归属 / 账号"
            class="activity-panel__keyword"
            @keyup.enter="searchRewards"
          />
          <n-button type="primary" @click="searchRewards">查询</n-button>
          <n-button @click="resetRewardQuery">重置</n-button>
        </n-space>
        <n-data-table
          :columns="rewardColumns"
          :data="rewards"
          :loading="rewardLoading"
          :pagination="rewardPagination"
          :scroll-x="1550"
          remote
        />
      </n-tab-pane>

      <n-tab-pane name="debug" tab="用户诊断">
        <n-space class="mb-4" align="center">
          <n-select
            v-model:value="debugTenantId"
            :options="tenantOptions"
            clearable
            filterable
            placeholder="选择账号归属"
            class="activity-panel__tenant-select"
          />
          <n-button type="primary" :disabled="!debugTenantId" @click="loadUserStatus"
            >查看状态</n-button
          >
        </n-space>
        <n-data-table
          :columns="statusColumns"
          :data="userStatuses"
          :loading="statusLoading"
          :scroll-x="1180"
        />
      </n-tab-pane>
    </n-tabs>

    <n-modal
      v-model:show="resetVisible"
      preset="dialog"
      title="重置用户活动"
      positive-text="确认重置"
      negative-text="取消"
      @positive-click="confirmReset"
    >
      <n-alert type="warning" class="mb-4">
        重置只会开启新的活动代次，不会删除历史奖励，也不会扣除已赠送的会员天数。
      </n-alert>
      <n-form label-placement="left" label-width="90">
        <n-form-item label="活动">{{ resetTarget?.name || '-' }}</n-form-item>
        <n-form-item label="当前代次">{{ resetTarget?.generation || 1 }}</n-form-item>
        <n-form-item label="重置原因">
          <n-input
            v-model:value="resetReason"
            type="textarea"
            :autosize="{ minRows: 3, maxRows: 5 }"
            placeholder="请输入重置原因"
          />
        </n-form-item>
      </n-form>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
  import { computed, h, onMounted, reactive, ref } from 'vue';
  import { NButton, NPopconfirm, NSpace, NTag, useMessage } from 'naive-ui';

  import {
    ActivityDebug,
    ActivityList,
    ActivityReset,
    ActivityRewardList,
    ActivitySave,
    ActivityUserStatus,
    TenantList,
  } from '@/api/addons/youbanPublish';

  const message = useMessage();
  const activePane = ref('config');
  const activityLoading = ref(false);
  const rewardLoading = ref(false);
  const statusLoading = ref(false);
  const resetVisible = ref(false);
  const activities = ref<any[]>([]);
  const rewards = ref<any[]>([]);
  const tenants = ref<any[]>([]);
  const userStatuses = ref<any[]>([]);
  const debugTenantId = ref<number | null>(null);
  const resetTarget = ref<any>(null);
  const resetReason = ref('');

  const rewardQuery = reactive({
    activityCode: null as null | string,
    keyword: '',
    notifyStatus: null as null | string,
    tenantId: null as null | number,
  });

  const activityOptions = computed(() =>
    activities.value.map((item) => ({ label: item.name, value: item.code }))
  );
  const tenantOptions = computed(() =>
    tenants.value.map((item) => ({ label: item.name || `账号归属 ${item.id}`, value: item.id }))
  );
  const notifyStatusOptions = [
    { label: '待发送', value: 'pending' },
    { label: '已发送', value: 'sent' },
    { label: '发送失败', value: 'failed' },
    { label: '已跳过', value: 'skipped' },
  ];

  const rewardPagination = reactive({
    itemCount: 0,
    page: 1,
    pageSize: 20,
    pageSizes: [20, 50, 100],
    showSizePicker: true,
    onUpdatePage: (page: number) => {
      rewardPagination.page = page;
      loadRewards();
    },
    onUpdatePageSize: (pageSize: number) => {
      rewardPagination.pageSize = pageSize;
      rewardPagination.page = 1;
      loadRewards();
    },
  });

  const rewardColumns = [
    { title: 'ID', key: 'id', width: 80 },
    { title: '活动', key: 'activityName', minWidth: 160 },
    { title: '代次', key: 'activityGeneration', width: 80 },
    { title: '账号归属', key: 'tenantName', minWidth: 150 },
    { title: '账号', key: 'accountUsername', minWidth: 130 },
    { title: '奖励天数', key: 'changeDays', width: 100 },
    { title: '奖励后到期', key: 'afterExpiredAt', width: 170 },
    {
      title: '通知状态',
      key: 'notifyStatus',
      width: 110,
      render: (row: any) => renderNotifyStatus(row.notifyStatus),
    },
    { title: '重试次数', key: 'notifyRetryCount', width: 90 },
    { title: '错误信息', key: 'errorMessage', minWidth: 220, ellipsis: { tooltip: true } },
    { title: '奖励时间', key: 'createdAt', width: 170 },
  ];

  const statusColumns = [
    { title: '活动', key: 'name', minWidth: 170 },
    { title: '当前代次', key: 'generation', width: 100 },
    { title: '满足条件数', key: 'eligibleCount', width: 110 },
    { title: '已奖励次数', key: 'rewardCount', width: 110 },
    { title: '已奖励天数', key: 'rewardDays', width: 110 },
    {
      title: '状态',
      key: 'status',
      width: 110,
      render: (row: any) => renderActivityStatus(row.status),
    },
    { title: '诊断结果', key: 'reason', minWidth: 240 },
    { title: '最后奖励', key: 'lastRewardAt', width: 170 },
    {
      title: '操作',
      key: 'actions',
      fixed: 'right',
      width: 230,
      render: (row: any) =>
        h(NSpace, { size: 8 }, () => [
          h(
            NButton,
            { size: 'small', onClick: () => debugActivity(row, 'evaluate') },
            { default: () => '重新计算' }
          ),
          h(
            NPopconfirm,
            { onPositiveClick: () => debugActivity(row, 'retry') },
            {
              default: () => '确认按当前真实数据重新执行奖励？幂等事件会阻止重复发放。',
              trigger: () =>
                h(NButton, { size: 'small', type: 'primary' }, { default: () => '重试奖励' }),
            }
          ),
          h(
            NButton,
            { size: 'small', type: 'warning', onClick: () => openReset(row) },
            { default: () => '重置' }
          ),
        ]),
    },
  ];

  function renderNotifyStatus(status: string) {
    const map: Record<string, { label: string; type: string }> = {
      failed: { label: '失败', type: 'error' },
      pending: { label: '待发送', type: 'warning' },
      sent: { label: '已发送', type: 'success' },
      skipped: { label: '已跳过', type: 'default' },
    };
    const item = map[status] || { label: status || '-', type: 'default' };
    return h(NTag, { type: item.type as any }, { default: () => item.label });
  }

  function renderActivityStatus(status: string) {
    const map: Record<string, { label: string; type: string }> = {
      disabled: { label: '已关闭', type: 'default' },
      eligible: { label: '可奖励', type: 'warning' },
      pending: { label: '待完成', type: 'info' },
      rewarded: { label: '已奖励', type: 'success' },
    };
    const item = map[status] || { label: status || '-', type: 'default' };
    return h(NTag, { type: item.type as any }, { default: () => item.label });
  }

  async function loadActivities() {
    activityLoading.value = true;
    try {
      const res: any = await ActivityList();
      activities.value = res?.list || [];
    } finally {
      activityLoading.value = false;
    }
  }

  async function loadTenants() {
    const res: any = await TenantList({ page: 1, perPage: 500 });
    tenants.value = res?.list || [];
  }

  async function saveActivity(item: any) {
    await ActivitySave({ code: item.code, enabled: item.enabled, rewardDays: item.rewardDays });
    message.success('活动配置已保存');
    await loadActivities();
  }

  async function loadRewards() {
    rewardLoading.value = true;
    try {
      const res: any = await ActivityRewardList({
        ...rewardQuery,
        page: rewardPagination.page,
        perPage: rewardPagination.pageSize,
      });
      rewards.value = res?.list || [];
      rewardPagination.itemCount = res?.totalCount || res?.total || 0;
    } finally {
      rewardLoading.value = false;
    }
  }

  function searchRewards() {
    rewardPagination.page = 1;
    loadRewards();
  }

  function resetRewardQuery() {
    Object.assign(rewardQuery, {
      activityCode: null,
      keyword: '',
      notifyStatus: null,
      tenantId: null,
    });
    searchRewards();
  }

  async function loadUserStatus() {
    if (!debugTenantId.value) return;
    statusLoading.value = true;
    try {
      const res: any = await ActivityUserStatus({ tenantId: debugTenantId.value });
      userStatuses.value = res?.list || [];
    } finally {
      statusLoading.value = false;
    }
  }

  async function debugActivity(row: any, action: 'evaluate' | 'retry') {
    if (!debugTenantId.value) return;
    await ActivityDebug({ action, code: row.code, tenantId: debugTenantId.value });
    message.success(action === 'retry' ? '奖励链路已重新执行' : '活动状态已重新计算');
    await Promise.all([loadUserStatus(), loadActivities(), loadRewards()]);
  }

  function openReset(row: any) {
    resetTarget.value = row;
    resetReason.value = '';
    resetVisible.value = true;
  }

  async function confirmReset() {
    if (!debugTenantId.value || !resetTarget.value) return false;
    if (!resetReason.value.trim()) {
      message.warning('请输入重置原因');
      return false;
    }
    await ActivityReset({
      code: resetTarget.value.code,
      reason: resetReason.value.trim(),
      tenantId: debugTenantId.value,
    });
    message.success('用户活动已重置，新代次将在下次触发或重试时生效');
    resetVisible.value = false;
    await loadUserStatus();
  }

  onMounted(() => {
    loadActivities();
    loadTenants();
    loadRewards();
  });
</script>

<style scoped>
  .activity-panel__card-body {
    margin-top: 16px;
  }

  .activity-panel__activity-select,
  .activity-panel__status-select {
    width: 150px;
  }

  .activity-panel__tenant-select {
    width: 220px;
  }

  .activity-panel__keyword {
    width: 220px;
  }
</style>
