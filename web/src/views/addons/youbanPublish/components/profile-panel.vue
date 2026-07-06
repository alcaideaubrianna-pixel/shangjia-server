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
        clearable
        filterable
        placeholder="上架账号"
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
      <n-input
        v-model:value="query.keyword"
        placeholder="编号 / 标题 / 正文"
        clearable
        @keyup.enter="loadProfiles"
      />
      <n-button @click="loadProfiles">查询</n-button>
      <n-button @click="loadProfiles">刷新</n-button>
    </n-space>

    <n-data-table
      :columns="columns"
      :data="profiles"
      :loading="loading"
      :pagination="pagination"
      :row-key="(row) => row.id"
      :scroll-x="1760"
      size="small"
      remote
    />

    <n-modal v-model:show="editVisible" preset="dialog" title="编辑笔记资料" positive-text="保存" negative-text="取消" @positive-click="saveProfile">
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
        <n-form-item label="标签">
          <n-select v-model:value="editForm.tag" :options="tagOptions" clearable filterable placeholder="请选择标签" />
        </n-form-item>
        <n-form-item label="可见性">
          <n-select v-model:value="editForm.visibility" :options="visibilityOptions" />
        </n-form-item>
        <n-form-item label="状态">
          <n-select v-model:value="editForm.status" :options="profileStatusOptions" />
        </n-form-item>
        <n-form-item label="正文">
          <n-input v-model:value="editForm.plainText" type="textarea" :autosize="{ minRows: 6, maxRows: 12 }" />
        </n-form-item>
        <n-form-item label="客服备注">
          <n-input v-model:value="editForm.customerRemark" type="textarea" :autosize="{ minRows: 2, maxRows: 4 }" />
        </n-form-item>
      </n-form>
    </n-modal>
  </div>
</template>

<script lang="ts" setup>
  import { computed, h, onMounted, reactive, ref } from 'vue';
  import { NButton, NPopconfirm, NSpace, NTag, useMessage } from 'naive-ui';
  import { AccountList, ProfileDelete, ProfileList, ProfileReview, ProfileSave, ProfileView, TagList, TenantList } from '@/api/addons/youbanPublish';

  const message = useMessage();
  const loading = ref(false);
  const editVisible = ref(false);
  const profiles = ref<Recordable[]>([]);
  const tenants = ref<Recordable[]>([]);
  const accounts = ref<Recordable[]>([]);
  const tags = ref<Recordable[]>([]);
  const editForm = reactive({
    id: 0,
    taskId: 0,
    title: '',
    province: '',
    city: '',
    plainText: '',
    tag: '',
    customerRemark: '',
    visibility: 'private',
    status: 1,
  });
  const query = reactive({
    tenantId: null as number | null,
    accountId: null as number | null,
    tag: '',
    keyword: '',
  });
  const pagination = reactive({
    page: 1,
    pageSize: 20,
    itemCount: 0,
    showSizePicker: true,
    pageSizes: [10, 20, 50],
    onChange: (page: number) => {
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
    accounts.value
      .filter((item) => !query.tenantId || item.tenantId === query.tenantId)
      .map((item) => ({ label: `${item.nickname || item.username} (${item.username})`, value: item.id }))
  );
  const accountOptionsWithAll = computed(() => [
    { label: '全部上架账号', value: null },
    ...accountOptions.value,
  ]);
  const tagOptions = computed(() =>
    tags.value.map((item) => ({ label: item.name, value: String(item.id) }))
  );
  const tagOptionsWithAll = computed(() => [{ label: '全部标签', value: '' }, ...tagOptions.value]);
  const visibilityOptions = [
    { label: '私密', value: 'private' },
    { label: '公开', value: 'public' },
    { label: '会员可见', value: 'member_only' },
  ];
  const profileStatusOptions = [
    { label: '上架', value: 1 },
    { label: '下架', value: 2 },
  ];

  const columns = [
    { title: 'ID', key: 'id', width: 80 },
    { title: '资料编号', key: 'profileNo', width: 120 },
    { title: '账号归属', key: 'tenantName', width: 150 },
    { title: '上架账号', key: 'nickname', width: 130 },
    { title: '标题', key: 'title', width: 220, ellipsis: { tooltip: true } },
    { title: '地区', key: 'region', width: 140, render: (row) => [row.province, row.city].filter(Boolean).join(' / ') || '-' },
    { title: '标签', key: 'tag', width: 140, render: (row) => renderTagNames(row.tag) },
    { title: '图片', key: 'imageCount', width: 80 },
    { title: '视频', key: 'videoCount', width: 80 },
    { title: '审核', key: 'reviewStatus', width: 100, render: (row) => renderReview(row.reviewStatus) },
    { title: '状态', key: 'status', width: 90, render: (row) => renderStatus(row.status) },
    { title: '更新时间', key: 'updatedAt', width: 180 },
    { title: '摘要', key: 'summary', width: 260, ellipsis: { tooltip: true } },
    {
      title: '操作',
      key: 'actions',
      width: 300,
      fixed: 'right',
      render(row) {
        return h(NSpace, { size: 8 }, {
          default: () => [
            h(NButton, { size: 'small', onClick: () => viewProfile(row) }, { default: () => '查看' }),
            h(NButton, { size: 'small', type: 'primary', onClick: () => openEdit(row) }, { default: () => '编辑' }),
            h(NButton, { size: 'small', disabled: row.reviewStatus === 'approved', onClick: () => reviewProfile(row, 'approved') }, { default: () => '通过' }),
            h(NButton, { size: 'small', disabled: row.reviewStatus === 'rejected', onClick: () => reviewProfile(row, 'rejected') }, { default: () => '驳回' }),
            h(
              NPopconfirm,
              { onPositiveClick: () => deleteProfile(row) },
              {
                trigger: () => h(NButton, { size: 'small', type: 'error' }, { default: () => '删除' }),
                default: () => '确认删除该笔记资料？',
              }
            ),
          ],
        });
      },
    },
  ];

  onMounted(async () => {
    await Promise.all([loadTenants(), loadAccounts(), loadTags()]);
    await loadProfiles();
  });

  async function loadTenants() {
    const res: any = await TenantList({ page: 1, perPage: 200, status: 1 });
    tenants.value = res?.list || [];
  }

  async function loadAccounts() {
    const res: any = await AccountList({ page: 1, perPage: 200, accountType: 'uploader', status: 1 });
    accounts.value = res?.list || [];
  }

  async function loadTags() {
    const res: any = await TagList({ page: 1, perPage: 200, reviewStatus: 'approved', status: 1 });
    tags.value = res?.list || [];
  }

  async function loadProfiles() {
    loading.value = true;
    try {
      const res: any = await ProfileList({
        ...query,
        page: pagination.page,
        perPage: pagination.pageSize,
      });
      profiles.value = res?.list || [];
      pagination.itemCount = res?.totalCount || res?.total || 0;
    } finally {
      loading.value = false;
    }
  }

  function handleTenantChange() {
    query.accountId = null;
  }

  async function viewProfile(row: Recordable) {
    await ProfileView({ id: row.id });
    message.info('查看页面暂未开放');
  }

  async function openEdit(row: Recordable) {
    const res: any = await ProfileView({ id: row.id });
    const profile = res?.profile || row;
    editForm.id = profile.id;
    editForm.taskId = profile.taskId || 0;
    editForm.title = profile.title || '';
    editForm.province = profile.province || '';
    editForm.city = profile.city || '';
    editForm.plainText = profile.plainText || '';
    editForm.tag = profile.tag || '';
    editForm.customerRemark = profile.customerRemark || '';
    editForm.visibility = profile.visibility || 'private';
    editForm.status = profile.status || 1;
    editVisible.value = true;
  }

  async function saveProfile() {
    await ProfileSave({ ...editForm });
    message.success('笔记资料已保存');
    await loadProfiles();
  }

  async function reviewProfile(row: Recordable, reviewStatus: string) {
    await ProfileReview({ ids: [row.id], reviewStatus });
    message.success(reviewStatus === 'approved' ? '已审核通过' : '已驳回');
    await loadProfiles();
  }

  async function deleteProfile(row: Recordable) {
    await ProfileDelete({ ids: [row.id] });
    message.success('笔记资料已删除');
    await loadProfiles();
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
    const item = value === 1 ? ['success', '上架'] : value === 2 ? ['default', '下架'] : ['default', '-'];
    return h(NTag, { type: item[0] as any, bordered: false }, { default: () => item[1] });
  }
</script>
