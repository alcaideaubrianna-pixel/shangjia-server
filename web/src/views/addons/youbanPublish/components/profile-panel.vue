<template>
  <div>
    <n-space class="toolbar" align="center">
      <n-select
        v-model:value="query.tenantId"
        :options="tenantOptionsWithAll"
        clearable
        filterable
        placeholder="账号归属"
        class="tenant-select"
        @update:value="handleTenantChange"
      />
      <n-select
        v-model:value="query.accountId"
        :options="accountOptionsWithAll"
        :disabled="!query.tenantId"
        clearable
        filterable
        placeholder="管理员 / 上架账号"
        class="tenant-select"
      />
      <n-select
        v-model:value="query.tag"
        :options="tagOptionsWithAll"
        clearable
        filterable
        placeholder="标签"
        class="status-select"
      />
      <n-select
        v-model:value="query.status"
        :options="profileStatusOptionsWithAll"
        clearable
        placeholder="资料状态"
        class="status-select"
      />
      <n-input
        v-model:value="query.keyword"
        placeholder="编号 / 标题 / 正文"
        clearable
        @keyup.enter="loadProfiles"
      />
      <n-button :loading="loading" @click="loadProfiles">查询</n-button>
      <n-button :loading="loading" @click="loadProfiles">刷新</n-button>
      <n-button type="primary" secondary :disabled="loading" @click="openTgImportModal"
        >TG频道导入</n-button
      >
      <n-button
        type="success"
        :disabled="!selectedProfileIds.length || isActionPending('batch:status:1')"
        :loading="isActionPending('batch:status:1')"
        @click="batchChangeProfileStatus(1)"
        >批量上架</n-button
      >
      <n-popconfirm
        :disabled="!selectedProfileIds.length || isActionPending('batch:status:2')"
        @positive-click="batchChangeProfileStatus(2)"
      >
        <template #trigger>
          <n-button
            type="warning"
            :disabled="!selectedProfileIds.length || isActionPending('batch:status:2')"
            :loading="isActionPending('batch:status:2')"
            >批量下架</n-button
          >
        </template>
        下架后会清理资料在 Telegram 频道中的历史消息，确认继续？
      </n-popconfirm>
      <n-button
        type="primary"
        :disabled="!selectedProfileIds.length || isActionPending('batch:review:approved')"
        :loading="isActionPending('batch:review:approved')"
        @click="batchReviewProfiles('approved')"
        >批量通过</n-button
      >
      <n-button
        :disabled="!selectedProfileIds.length || isActionPending('batch:review:rejected')"
        :loading="isActionPending('batch:review:rejected')"
        @click="batchReviewProfiles('rejected')"
        >批量驳回</n-button
      >
      <n-popconfirm
        :disabled="!selectedProfileIds.length || isActionPending('batch:delete')"
        @positive-click="batchDeleteProfiles"
      >
        <template #trigger>
          <n-button
            type="error"
            :disabled="!selectedProfileIds.length || isActionPending('batch:delete')"
            :loading="isActionPending('batch:delete')"
            >批量删除</n-button
          >
        </template>
        确认删除选中的笔记资料？
      </n-popconfirm>
      <n-popconfirm
        :disabled="!query.tenantId || !query.accountId || isActionPending('purge')"
        @positive-click="purgeDeletedProfiles"
      >
        <template #trigger>
          <n-button
            type="error"
            secondary
            :disabled="!query.tenantId || !query.accountId || isActionPending('purge')"
            :loading="isActionPending('purge')"
          >
            清空资料
          </n-button>
        </template>
        将永久删除当前账号下所有历史软删除资料及其媒体、索引和归属数据，且无法恢复。确认继续？
      </n-popconfirm>
    </n-space>

    <n-data-table
      :columns="columns"
      :data="profiles"
      :loading="loading"
      :pagination="pagination"
      :row-key="(row) => row.id"
      :checked-row-keys="checkedRowKeys"
      :scroll-x="1820"
      size="small"
      remote
      @update:checked-row-keys="handleCheckedRowKeys"
    />

    <n-modal
      v-model:show="tgImportVisible"
      preset="dialog"
      title="从TG频道导入资料"
      positive-text="创建导入任务"
      negative-text="取消"
      :loading="tgImportLoading"
      @positive-click="createTgImportTask"
    >
      <n-form :model="tgImportForm" label-placement="left" label-width="100">
        <n-form-item label="账号归属">
          <n-select
            v-model:value="tgImportForm.tenantId"
            :options="tenantOptions"
            filterable
            clearable
            placeholder="先选择租户"
            @update:value="handleImportTenantChange"
          />
        </n-form-item>
        <n-form-item label="归属账号">
          <n-select
            v-model:value="tgImportForm.accountId"
            :options="tgImportAccountOptions"
            :disabled="!tgImportForm.tenantId"
            filterable
            clearable
            placeholder="选择管理员或上架账号"
            @update:value="handleImportAccountChange"
          />
        </n-form-item>
        <n-form-item label="TG账号">
          <n-select
            v-model:value="tgImportForm.tgAccountId"
            :options="tgImportTgAccountOptions"
            filterable
            clearable
            placeholder="选择已登录的TG账号"
          />
        </n-form-item>
        <n-form-item label="频道连接">
          <n-input
            v-model:value="tgImportForm.channelUrl"
            clearable
            placeholder="https://t.me/channel 或 https://t.me/c/123456"
          />
        </n-form-item>
        <n-form-item label="拉取天数">
          <n-input-number
            v-model:value="tgImportForm.pullLimitDays"
            :min="1"
            :max="365"
            class="w-full"
          />
        </n-form-item>
      </n-form>
    </n-modal>

    <n-modal
      v-model:show="editVisible"
      preset="dialog"
      title="编辑笔记资料"
      positive-text="保存"
      negative-text="取消"
      :loading="editLoading"
      @positive-click="saveProfile"
    >
      <n-form :model="editForm" label-placement="left" label-width="90">
        <n-form-item label="标题">
          <n-input v-model:value="editForm.title" clearable />
        </n-form-item>
        <n-form-item label="省份">
          <n-input v-model:value="editForm.province" clearable />
        </n-form-item>
        <n-form-item label="城市">
          <n-input v-model:value="editForm.city" clearable />
        </n-form-item>
        <n-form-item label="是否是处女">
          <n-select v-model:value="editForm.isVirgin" :options="virginOptions" />
        </n-form-item>
        <n-form-item label="标签">
          <n-select
            v-model:value="editForm.tag"
            :options="tagOptions"
            clearable
            filterable
            placeholder="请选择标签"
          />
        </n-form-item>
        <n-form-item label="可见性">
          <n-select v-model:value="editForm.visibility" :options="visibilityOptions" />
        </n-form-item>
        <n-form-item label="正文">
          <n-input
            v-model:value="editForm.plainText"
            type="textarea"
            :autosize="{ minRows: 6, maxRows: 12 }"
          />
        </n-form-item>
        <n-form-item label="客服备注">
          <n-input
            v-model:value="editForm.customerRemark"
            type="textarea"
            :autosize="{ minRows: 2, maxRows: 4 }"
          />
        </n-form-item>
      </n-form>
    </n-modal>

    <n-modal
      v-model:show="viewVisible"
      preset="card"
      title="资料详情"
      class="profile-panel__view-modal"
    >
      <n-spin :show="viewLoading">
        <n-descriptions v-if="viewData?.profile" bordered :column="2">
          <n-descriptions-item label="资料编号">
            {{ viewData.profile.profileNo || '-' }}
          </n-descriptions-item>
          <n-descriptions-item label="状态">
            {{ profileStatusLabel(viewData.profile.status) }}
          </n-descriptions-item>
          <n-descriptions-item label="标题" :span="2">
            {{ viewData.profile.title || '-' }}
          </n-descriptions-item>
          <n-descriptions-item label="账号归属">
            {{ viewData.profile.tenantName || '-' }}
          </n-descriptions-item>
          <n-descriptions-item label="上架账号">
            {{ viewData.profile.accountName || viewData.profile.nickname || '-' }}
          </n-descriptions-item>
          <n-descriptions-item label="地区">
            {{
              [viewData.profile.province, viewData.profile.city].filter(Boolean).join(' / ') || '-'
            }}
          </n-descriptions-item>
          <n-descriptions-item label="审核状态">
            {{ reviewStatusLabel(viewData.profile.reviewStatus) }}
          </n-descriptions-item>
          <n-descriptions-item label="正文" :span="2">
            <div class="profile-panel__text">{{ viewData.profile.plainText || '-' }}</div>
          </n-descriptions-item>
          <n-descriptions-item label="媒体">
            {{ viewData.media?.length || 0 }} 个
          </n-descriptions-item>
          <n-descriptions-item label="推送频道">
            {{ viewData.pushChannels?.length || 0 }} 个
          </n-descriptions-item>
        </n-descriptions>
        <n-empty v-else description="暂无资料详情" />
      </n-spin>
    </n-modal>
  </div>
</template>

<script lang="ts" setup>
  import { computed, h, onMounted, reactive, ref } from 'vue';
  import { NButton, NPopconfirm, NSpace, NTag, useMessage } from 'naive-ui';
  import {
    AccountList,
    ImportTaskCreateForAccount,
    ProfileDelete,
    ProfileList,
    ProfilePurgeDeleted,
    ProfileReview,
    ProfileSave,
    ProfileStatus,
    ProfileView,
    ServerTgAccountList,
    TagList,
    TenantList,
  } from '@/api/addons/youbanPublish';

  const message = useMessage();
  const loading = ref(false);
  const editVisible = ref(false);
  const editLoading = ref(false);
  const viewVisible = ref(false);
  const viewLoading = ref(false);
  const viewData = ref<Recordable | null>(null);
  const tgImportVisible = ref(false);
  const tgImportLoading = ref(false);
  const actionKeys = ref(new Set<string>());
  const profiles = ref<Recordable[]>([]);
  const checkedRowKeys = ref<Array<number | string>>([]);
  const tenants = ref<Recordable[]>([]);
  const accounts = ref<Recordable[]>([]);
  const tgImportAccounts = ref<Recordable[]>([]);
  const tags = ref<Recordable[]>([]);
  const tgAccounts = ref<Recordable[]>([]);
  const editForm = reactive({
    id: 0,
    taskId: 0,
    title: '',
    province: '',
    city: '',
    isVirgin: 0,
    plainText: '',
    tag: '',
    customerRemark: '',
    visibility: 'private',
  });
  const virginOptions = [
    { label: '未知', value: 0 },
    { label: '是', value: 1 },
    { label: '否', value: 2 },
  ];
  const tgImportForm = reactive({
    tenantId: null as number | null,
    accountId: null as number | null,
    tgAccountId: null as number | null,
    channelUrl: '',
    pullLimitDays: 365,
  });
  const query = reactive({
    tenantId: null as number | null,
    accountId: null as number | null,
    tag: '',
    status: null as number | null,
    keyword: '',
  });
  let profileLoadSequence = 0;
  const pagination = reactive({
    page: 1,
    pageSize: 20,
    itemCount: 0,
    showSizePicker: true,
    pageSizes: [10, 20, 50, 100, 200, 500],
    onUpdatePage: (page: number) => {
      pagination.page = page;
      loadProfiles();
    },
    onUpdatePageSize: (pageSize: number) => {
      pagination.pageSize = pageSize;
      pagination.page = 1;
      loadProfiles();
    },
  });

  const tenantOptions = computed(() =>
    tenants.value.map((item) => ({ label: accountOwnerName(item), value: item.id }))
  );
  const tenantOptionsWithAll = computed(() => [
    { label: '全部账号归属', value: null },
    ...tenantOptions.value,
  ]);
  const accountOptions = computed(() =>
    accounts.value.map((item) => ({
      label: `${item.nickname || item.username} (${item.username}) · ${item.accountType === 'admin' ? '管理员' : '上架账号'}`,
      value: item.id,
    }))
  );
  const accountOptionsWithAll = computed(() => [
    { label: '全部账号', value: null },
    ...accountOptions.value,
  ]);
  const tagOptions = computed(() =>
    tags.value.map((item) => ({ label: item.name, value: String(item.id) }))
  );
  const tagOptionsWithAll = computed(() => [{ label: '全部标签', value: '' }, ...tagOptions.value]);
  const tgImportAccountOptions = computed(() =>
    tgImportAccounts.value.map((item) => ({
      label: `${item.nickname || item.username || `账号#${item.id}`} (${item.username || item.id}) · ${item.accountType === 'admin' ? '管理员' : '上架账号'}`,
      value: item.id,
    }))
  );
  const tgImportTgAccountOptions = computed(() => {
    return tgAccounts.value
      .filter((item) => !tgImportForm.tenantId || item.tenantId === tgImportForm.tenantId)
      .map((item) => ({
        label: `${item.displayName || item.telegramUsername || `TG账号#${item.id}`} · ${item.status === 'authorized' ? '已登录' : item.status || '未知状态'}`,
        value: item.id,
      }));
  });
  const visibilityOptions = [
    { label: '私密', value: 'private' },
    { label: '公开', value: 'public' },
    { label: '会员可见', value: 'member_only' },
  ];
  const profileStatusOptions = [
    { label: '上架', value: 1 },
    { label: '下架', value: 2 },
  ];
  const profileStatusOptionsWithAll = [{ label: '全部状态', value: null }, ...profileStatusOptions];
  const selectedProfileIds = computed(() =>
    checkedRowKeys.value.map((id) => Number(id)).filter((id) => id > 0)
  );

  const columns = [
    { type: 'selection', width: 48, fixed: 'left' },
    { title: 'ID', key: 'id', width: 80 },
    { title: '资料编号', key: 'profileNo', width: 120 },
    { title: '账号归属', key: 'tenantName', width: 150 },
    { title: '上架账号', key: 'nickname', width: 130 },
    { title: '标题', key: 'title', width: 220, ellipsis: { tooltip: true } },
    {
      title: '地区',
      key: 'region',
      width: 140,
      render: (row) => [row.province, row.city].filter(Boolean).join(' / ') || '-',
    },
    { title: '标签', key: 'tag', width: 140, render: (row) => renderTagNames(row.tag) },
    { title: '图片', key: 'imageCount', width: 80 },
    { title: '视频', key: 'videoCount', width: 80 },
    {
      title: '审核',
      key: 'reviewStatus',
      width: 100,
      render: (row) => renderReview(row.reviewStatus),
    },
    { title: '状态', key: 'status', width: 90, render: (row) => renderStatus(row.status) },
    {
      title: 'TG任务',
      key: 'tgStatus',
      width: 120,
      render: (row) => renderTaskStatus(row),
    },
    { title: '更新时间', key: 'updatedAt', width: 180 },
    { title: '摘要', key: 'summary', width: 260, ellipsis: { tooltip: true } },
    {
      title: '操作',
      key: 'actions',
      width: 460,
      fixed: 'right',
      render(row) {
        const profileStatus = profileStatusValue(row.status);
        const statusAction = profileStatus === 1 ? 2 : 1;
        const statusActionName = statusAction === 1 ? '上架' : '下架';
        const statusActionKey = actionKey('status', row.id);
        const reviewApprovedKey = actionKey('review:approved', row.id);
        const reviewRejectedKey = actionKey('review:rejected', row.id);
        const deleteActionKey = actionKey('delete', row.id);
        const busy = isProfileBusy(row);
        const taskProcessing = isTaskProcessing(row);
        const statusDisabled = busy || taskProcessing || isActionPending(statusActionKey);
        const editDisabled = busy || taskProcessing || isActionPending(actionKey('edit', row.id));
        const reviewDisabled = busy || taskProcessing;
        const deleteDisabled = busy || taskProcessing || isActionPending(deleteActionKey);
        return h(
          NSpace,
          { size: 8 },
          {
            default: () => [
              h(
                NButton,
                {
                  size: 'small',
                  loading: isActionPending(actionKey('view', row.id)),
                  onClick: () => viewProfile(row),
                },
                { default: () => '查看' }
              ),
              h(
                NButton,
                {
                  size: 'small',
                  type: 'primary',
                  disabled: editDisabled,
                  loading: isActionPending(actionKey('edit', row.id)),
                  onClick: () => openEdit(row),
                },
                { default: () => '编辑' }
              ),
              h(
                NPopconfirm,
                {
                  disabled: statusDisabled,
                  onPositiveClick: () => changeProfileStatus(row, statusAction),
                },
                {
                  trigger: () =>
                    h(
                      NButton,
                      {
                        size: 'small',
                        type: statusAction === 1 ? 'success' : 'warning',
                        loading: isActionPending(statusActionKey),
                        disabled: statusDisabled,
                        title: taskProcessing ? taskProcessingMessage(row) : undefined,
                      },
                      { default: () => statusActionName }
                    ),
                  default: () =>
                    statusAction === 1
                      ? '确认提交资料上架？'
                      : '下架后会清理资料在 Telegram 频道中的历史消息，确认继续？',
                }
              ),
              h(
                NButton,
                {
                  size: 'small',
                  disabled:
                    row.reviewStatus === 'approved' ||
                    reviewDisabled ||
                    isActionPending(reviewApprovedKey),
                  loading: isActionPending(reviewApprovedKey),
                  onClick: () => reviewProfile(row, 'approved'),
                },
                { default: () => '通过' }
              ),
              h(
                NButton,
                {
                  size: 'small',
                  disabled:
                    row.reviewStatus === 'rejected' ||
                    reviewDisabled ||
                    isActionPending(reviewRejectedKey),
                  loading: isActionPending(reviewRejectedKey),
                  onClick: () => reviewProfile(row, 'rejected'),
                },
                { default: () => '驳回' }
              ),
              h(
                NPopconfirm,
                {
                  disabled: deleteDisabled,
                  onPositiveClick: () => deleteProfile(row),
                },
                {
                  trigger: () =>
                    h(
                      NButton,
                      {
                        size: 'small',
                        type: 'error',
                        loading: isActionPending(deleteActionKey),
                        disabled: deleteDisabled,
                        title: taskProcessing ? taskProcessingMessage(row) : undefined,
                      },
                      { default: () => '删除' }
                    ),
                  default: () =>
                    profileStatus === 1
                      ? '删除会先下架并清理 Telegram 历史消息，确认删除该资料？'
                      : '确认删除该笔记资料？',
                }
              ),
            ],
          }
        );
      },
    },
  ];

  onMounted(async () => {
    await Promise.all([loadTenants(), loadTags(), loadTgAccounts()]);
    await loadProfiles();
  });

  async function loadTenants() {
    const res: any = await TenantList({ page: 1, perPage: 200, status: 1 });
    tenants.value = res?.list || [];
  }

  async function loadAccounts(tenantId: number | null) {
    if (!tenantId) {
      accounts.value = [];
      return;
    }
    const res: any = await AccountList({
      page: 1,
      perPage: 500,
      status: 1,
      tenantId,
    });
    accounts.value = res?.list || [];
  }

  async function loadTgImportAccounts(tenantId: number | null) {
    if (!tenantId) {
      tgImportAccounts.value = [];
      return;
    }
    const res: any = await AccountList({
      page: 1,
      perPage: 500,
      status: 1,
      tenantId,
    });
    tgImportAccounts.value = res?.list || [];
  }

  async function loadTags() {
    const res: any = await TagList({ page: 1, perPage: 200, reviewStatus: 'approved', status: 1 });
    tags.value = res?.list || [];
  }

  async function loadTgAccounts() {
    const res: any = await ServerTgAccountList({ page: 1, perPage: 500 });
    tgAccounts.value = res?.list || [];
  }

  async function loadProfiles() {
    const sequence = ++profileLoadSequence;
    loading.value = true;
    try {
      const res: any = await ProfileList({
        ...query,
        page: pagination.page,
        perPage: pagination.pageSize,
      });
      if (sequence !== profileLoadSequence) {
        return;
      }
      profiles.value = res?.list || [];
      pagination.itemCount = res?.totalCount || res?.total || 0;
      checkedRowKeys.value = [];
    } finally {
      if (sequence === profileLoadSequence) {
        loading.value = false;
      }
    }
  }

  async function handleTenantChange() {
    query.accountId = null;
    await loadAccounts(query.tenantId);
  }

  function openTgImportModal() {
    tgImportForm.tenantId = null;
    tgImportForm.accountId = null;
    tgImportForm.tgAccountId = null;
    tgImportAccounts.value = [];
    tgImportForm.channelUrl = '';
    tgImportForm.pullLimitDays = 365;
    tgImportVisible.value = true;
  }

  async function handleImportTenantChange() {
    tgImportForm.accountId = null;
    tgImportForm.tgAccountId = null;
    await loadTgImportAccounts(tgImportForm.tenantId);
  }

  function handleImportAccountChange() {
    tgImportForm.tgAccountId = null;
  }

  async function createTgImportTask() {
    if (
      !tgImportForm.tenantId ||
      !tgImportForm.accountId ||
      !tgImportForm.tgAccountId ||
      !tgImportForm.channelUrl.trim()
    ) {
      message.warning('请选择账号归属、归属账号、TG账号并输入频道连接');
      return false;
    }
    tgImportLoading.value = true;
    try {
      const result: any = await ImportTaskCreateForAccount({ ...tgImportForm });
      message.success(`TG资料导入任务已启动，任务ID：${result?.id || result?.data?.id || '-'}`);
      tgImportVisible.value = false;
      await loadProfiles();
      return true;
    } finally {
      tgImportLoading.value = false;
    }
  }

  async function viewProfile(row: Recordable) {
    const key = actionKey('view', row.id);
    if (isActionPending(key)) {
      return;
    }
    setActionPending(key, true);
    viewLoading.value = true;
    try {
      const res: any = await ProfileView({ id: row.id });
      viewData.value = {
        profile: res?.profile || row,
        media: res?.media || [],
        pushChannels: res?.pushChannels || [],
      };
      viewVisible.value = true;
    } finally {
      viewLoading.value = false;
      setActionPending(key, false);
    }
  }

  async function openEdit(row: Recordable) {
    const key = actionKey('edit', row.id);
    if (isActionPending(key)) {
      return;
    }
    setActionPending(key, true);
    editLoading.value = true;
    try {
      const res: any = await ProfileView({ id: row.id });
      const profile = res?.profile || row;
      editForm.id = profile.id;
      editForm.taskId = profile.taskId || 0;
      editForm.title = profile.title || '';
      editForm.province = profile.province || '';
      editForm.city = profile.city || '';
      editForm.isVirgin = Number(profile.isVirgin || 0);
      editForm.plainText = profile.plainText || '';
      editForm.tag = profile.tag || '';
      editForm.customerRemark = profile.customerRemark || '';
      editForm.visibility = profile.visibility || 'private';
      editVisible.value = true;
    } finally {
      editLoading.value = false;
      setActionPending(key, false);
    }
  }

  async function saveProfile() {
    const key = actionKey('edit', editForm.id);
    if (editLoading.value || isProfileBusy({ id: editForm.id })) {
      return false;
    }
    if (!editForm.title.trim()) {
      message.warning('标题不能为空');
      return false;
    }
    setActionPending(key, true);
    editLoading.value = true;
    try {
      const payload = {
        id: editForm.id,
        title: editForm.title,
        province: editForm.province,
        city: editForm.city,
        isVirgin: editForm.isVirgin,
        plainText: editForm.plainText,
        tag: editForm.tag,
        customerRemark: editForm.customerRemark,
        visibility: editForm.visibility,
      };
      await ProfileSave(payload);
      message.success('笔记资料已保存');
      editVisible.value = false;
      await loadProfiles();
      return true;
    } finally {
      editLoading.value = false;
      setActionPending(key, false);
    }
  }

  async function reviewProfile(row: Recordable, reviewStatus: string) {
    await runAction(actionKey(`review:${reviewStatus}`, row.id), async () => {
      await ProfileReview({ ids: [row.id], reviewStatus });
      message.success(reviewStatus === 'approved' ? '已审核通过' : '已驳回');
      await loadProfiles();
    });
  }

  async function deleteProfile(row: Recordable) {
    if (isProfileBusy(row)) {
      return;
    }
    await runAction(actionKey('delete', row.id), async () => {
      await ProfileDelete({ ids: [row.id] });
      message.success('笔记资料删除已提交，Telegram 清理按任务异步处理');
      await loadProfiles();
    });
  }

  function handleCheckedRowKeys(keys: Array<number | string>) {
    checkedRowKeys.value = keys;
  }

  async function batchReviewProfiles(reviewStatus: string) {
    const ids = selectedProfileIds.value;
    if (!ids.length) {
      message.warning('请选择笔记资料');
      return;
    }
    await runAction(`batch:review:${reviewStatus}`, async () => {
      await ProfileReview({ ids, reviewStatus });
      message.success(reviewStatus === 'approved' ? '已批量审核通过' : '已批量驳回');
      await loadProfiles();
    });
  }

  async function batchDeleteProfiles() {
    const ids = selectedProfileIds.value;
    if (!ids.length) {
      message.warning('请选择笔记资料');
      return;
    }
    await runAction('batch:delete', async () => {
      await ProfileDelete({ ids });
      message.success('已批量删除笔记资料，Telegram 清理任务已提交');
      await loadProfiles();
    });
  }

  async function purgeDeletedProfiles() {
    if (!query.tenantId || !query.accountId) {
      message.warning('请先选择账号归属和账号');
      return;
    }
    await runAction('purge', async () => {
      const result: any = await ProfilePurgeDeleted({
        tenantId: query.tenantId,
        accountId: query.accountId,
      });
      message.success(`清理完成，共永久删除 ${result?.deletedCount || 0} 条软删除资料`);
      await loadProfiles();
    });
  }

  async function changeProfileStatus(row: Recordable, status: number) {
    const key = actionKey('status', row.id);
    const currentStatus = profileStatusValue(row.status);
    if (currentStatus === status) {
      message.warning(
        status === 1 ? '资料当前已上架，无需重复操作' : '资料当前已下架，无需重复操作'
      );
      return;
    }
    if (isProfileBusy(row)) {
      message.warning(taskProcessingMessage(row));
      return;
    }
    await runAction(key, async () => {
      const result: any = await ProfileStatus({ ids: [row.id], status });
      message.success(
        result?.message ||
          (status === 1
            ? '资料已提交上架，正在处理 Telegram 推送'
            : '资料已提交下架，正在清理 Telegram 消息')
      );
      await loadProfiles();
    });
  }

  async function batchChangeProfileStatus(status: number) {
    const ids = selectedProfileIds.value;
    if (!ids.length) {
      message.warning('请选择笔记资料');
      return;
    }
    const key = `batch:status:${status}`;
    await runAction(key, async () => {
      const result: any = await ProfileStatus({ ids, status });
      message.success(
        result?.message ||
          (status === 1
            ? '已批量提交上架，正在处理 Telegram 推送'
            : '已批量提交下架，正在清理 Telegram 消息')
      );
      await loadProfiles();
    });
  }

  function actionKey(action: string, id: number | string) {
    return `${action}:${id}`;
  }

  function isActionPending(key: string) {
    return actionKeys.value.has(key);
  }

  function isBatchActionPending() {
    return Array.from(actionKeys.value).some((key) => key.startsWith('batch:'));
  }

  function isProfileBusy(row: Recordable) {
    const id = row?.id;
    if (!id) {
      return true;
    }
    return (
      isBatchActionPending() ||
      Array.from(actionKeys.value).some(
        (key) => key.endsWith(`:${id}`) && !key.startsWith('view:') && !key.startsWith('review:')
      )
    );
  }

  function profileStatusValue(value: unknown) {
    const status = Number(value);
    return status === 1 || status === 2 ? status : 0;
  }

  function taskStatusValue(row: Recordable) {
    return String(row?.taskStatus || row?.tgStatus || '')
      .trim()
      .toLowerCase();
  }

  function isTaskProcessing(row: Recordable) {
    return [
      'pending',
      'publishing',
      'waiting_media',
      'editing',
      'queued',
      'running',
      'sending',
      'confirming',
      'retry_wait',
      'failed_retry',
      'unknown',
    ].includes(taskStatusValue(row));
  }

  function taskProcessingMessage(row: Recordable) {
    const taskStatus = taskStatusValue(row);
    if (taskStatus === 'waiting_media') {
      return '资料媒体仍在处理中，请等待媒体任务完成后再操作';
    }
    if (taskStatus === 'editing') {
      return '资料编辑仍在处理中，请稍候再操作';
    }
    if (['failed_retry', 'retry_wait', 'unknown'].includes(taskStatus)) {
      return 'Telegram 任务正在重试或等待恢复，请稍候再操作';
    }
    return 'Telegram 任务正在处理中，请稍候再操作';
  }

  function setActionPending(key: string, pending: boolean) {
    const next = new Set(actionKeys.value);
    if (pending) {
      next.add(key);
    } else {
      next.delete(key);
    }
    actionKeys.value = next;
  }

  async function runAction(key: string, action: () => Promise<void>) {
    if (isActionPending(key)) {
      return;
    }
    setActionPending(key, true);
    try {
      await action();
    } finally {
      setActionPending(key, false);
    }
  }

  function accountOwnerName(item: Recordable) {
    return item.name || item.username || (item.id ? `账号归属#${item.id}` : '-');
  }

  function renderTagNames(value: string) {
    if (!value) return '-';
    const names = value
      .split(',')
      .map((id) => tags.value.find((item) => String(item.id) === id)?.name || id)
      .filter(Boolean);
    return names.join('、') || value;
  }

  function renderReview(value: string) {
    const map = {
      pending: ['warning', '待审核'],
      approved: ['success', '已通过'],
      rejected: ['error', '已驳回'],
    };
    const item = map[value] || ['default', value || '-'];
    return h(NTag, { type: item[0] as any, bordered: false }, { default: () => item[1] });
  }

  function renderStatus(value: number) {
    const status = profileStatusValue(value);
    const item =
      status === 1 ? ['success', '上架'] : status === 2 ? ['default', '下架'] : ['default', '-'];
    return h(NTag, { type: item[0] as any, bordered: false }, { default: () => item[1] });
  }

  function profileStatusLabel(value: number) {
    const status = profileStatusValue(value);
    return status === 1 ? '上架' : status === 2 ? '下架' : '-';
  }

  function reviewStatusLabel(value: string) {
    return (
      {
        pending: '待审核',
        approved: '已通过',
        rejected: '已驳回',
      }[value] ||
      value ||
      '-'
    );
  }

  function renderTaskStatus(row: Recordable) {
    const labels: Record<string, string> = {
      pending: '等待处理',
      waiting_media: '等待媒体',
      publishing: '发布中',
      editing: '编辑中',
      queued: '排队中',
      running: '处理中',
      sending: '发送中',
      confirming: '确认中',
      retry_wait: '等待重试',
      failed_retry: '重试中',
      unknown: '状态未知',
      sent: '已发送',
      published: '已完成',
      completed: '已完成',
      success: '成功',
      failed: '失败',
      canceled: '已取消',
      superseded: '已替代',
    };
    const value = taskStatusValue(row);
    const fallback = profileStatusValue(row?.status) === 1 ? '已完成' : '无任务';
    const label = labels[value] || value || fallback;
    const type = ['failed', 'canceled', 'unknown'].includes(value)
      ? 'error'
      : [
            'pending',
            'waiting_media',
            'publishing',
            'editing',
            'queued',
            'running',
            'sending',
            'confirming',
            'retry_wait',
            'failed_retry',
          ].includes(value)
        ? 'warning'
        : ['sent', 'published', 'completed', 'success'].includes(value)
          ? 'success'
          : 'default';
    return h(NTag, { type: type as any, bordered: false }, { default: () => label });
  }
</script>

<style scoped>
  .profile-panel__text {
    white-space: pre-wrap;
    word-break: break-word;
  }

  .profile-panel__view-modal {
    width: 720px;
  }
</style>
