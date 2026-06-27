<template>
  <div class="content-announcement-page">
    <div class="n-layout-page-header">
      <n-card :bordered="false" title="公告管理"> 移动端公告、Banner 与发布状态管理 </n-card>
    </div>

    <n-card :bordered="false" class="proCard toolbar-card">
      <div class="toolbar">
        <n-space>
          <n-button type="primary" @click="handleAdd">新增公告</n-button>
          <n-button @click="loadData">刷新</n-button>
        </n-space>
        <n-space>
          <n-tag :bordered="false">共 {{ totalCount }} 条</n-tag>
          <n-tag type="success" :bordered="false">首页推荐 {{ homeRecommendations.length }} 条</n-tag>
          <n-tag type="success" :bordered="false">Banner {{ banners.length }} 条</n-tag>
        </n-space>
      </div>
    </n-card>

    <n-card :bordered="false" class="proCard table-card">
      <n-data-table
        :columns="columns"
        :data="announcements"
        :loading="loading"
        :row-key="rowKey"
        :scroll-x="1180"
      />
    </n-card>

    <n-modal
      v-model:show="detailVisible"
      preset="card"
      class="announcement-detail-modal"
      :bordered="false"
    >
      <template #header>{{ activeAnnouncement.title }}</template>
      <div class="announcement-detail-meta">
        <n-tag :type="activeAnnouncement.status === 1 ? 'success' : 'warning'" :bordered="false">
          {{ activeAnnouncement.status === 1 ? '启用' : '禁用' }}
        </n-tag>
        <n-tag v-if="activeAnnouncement.isBanner === 1" type="success" :bordered="false">
          Banner
        </n-tag>
      </div>
      <img
        v-if="activeAnnouncement.bannerImg"
        class="announcement-detail-banner"
        :src="activeAnnouncement.bannerImg"
        :alt="activeAnnouncement.title"
      />
      <div
        class="announcement-detail rich-content"
        v-html="renderRichText(activeAnnouncement.content)"
      ></div>
    </n-modal>

    <AnnouncementEditorModal
      v-model:show="editVisible"
      v-model="formValue"
      :saving="saving"
      :category-options="categoryOptions"
      @save="handleSave"
    />
  </div>
</template>

<script lang="ts" setup>
  import { computed, h, onMounted, ref } from 'vue';
  import type { DataTableColumns } from 'naive-ui';
  import { NButton, NSpace, NTag, useDialog, useMessage } from 'naive-ui';
  import { Delete, Edit, List, MaxSort, Status } from '@/api/appAnnouncement';
  import AnnouncementEditorModal from './components/AnnouncementEditorModal.vue';

  const loading = ref(false);
  const saving = ref(false);
  const message = useMessage();
  const dialog = useDialog();
  const announcementList = ref<Recordable[]>([]);
  const totalCount = ref(0);
  const detailVisible = ref(false);
  const editVisible = ref(false);
  const activeAnnouncement = ref<Recordable>({});
  const formValue = ref<Recordable>(newFormValue());
  const categoryOptions = [
    { label: '首页推荐', value: 'home' },
    { label: '关于我们', value: 'about' },
    { label: '交友成功案例', value: 'case' },
    { label: '博客', value: 'blog' },
    { label: '文档', value: 'docs' },
    { label: '新闻', value: 'news' },
  ];

  const banners = computed(() =>
    announcementList.value.filter((item) => item.isBanner === 1 && item.status === 1)
  );
  const homeRecommendations = computed(() =>
    announcementList.value.filter((item) => item.categoryCode === 'home' && item.status === 1)
  );
  const announcements = computed(() => announcementList.value);
  const columns = computed<DataTableColumns<Recordable>>(() => [
    {
      title: '文章',
      key: 'title',
      minWidth: 300,
      render(row) {
        return h('div', { class: 'announcement-title-cell' }, [
          h('div', { class: 'announcement-title-text' }, row.title || '-'),
          h('div', { class: 'announcement-summary' }, row.summary || stripHtml(row.content) || '暂无摘要'),
        ]);
      },
    },
    {
      title: '分类',
      key: 'categoryName',
      width: 130,
      render(row) {
        return h(
          NTag,
          { size: 'small', type: row.categoryCode === 'home' ? 'success' : 'info', bordered: false },
          { default: () => row.categoryName || categoryLabel(row.categoryCode) }
        );
      },
    },
    {
      title: 'Banner',
      key: 'isBanner',
      width: 110,
      render(row) {
        return h(
          NTag,
          { size: 'small', type: row.isBanner === 1 ? 'success' : 'default', bordered: false },
          { default: () => (row.isBanner === 1 ? '展示' : '不展示') }
        );
      },
    },
    {
      title: '状态',
      key: 'status',
      width: 100,
      render(row) {
        return h(
          NTag,
          { size: 'small', type: row.status === 1 ? 'success' : 'warning', bordered: false },
          { default: () => (row.status === 1 ? '启用' : '禁用') }
        );
      },
    },
    { title: '发布时间', key: 'publishAt', width: 180, render: (row) => formatTime(row.publishAt || row.createdAt) },
    { title: '过期时间', key: 'expireAt', width: 180, render: (row) => formatTime(row.expireAt) },
    { title: '排序', key: 'sort', width: 80, align: 'center' },
    {
      title: '操作',
      key: 'actions',
      width: 260,
      fixed: 'right',
      render(row) {
        return h(
          NSpace,
          { size: 8 },
          {
            default: () => [
              h(NButton, { size: 'small', onClick: () => openDetail(row) }, { default: () => '预览' }),
              h(
                NButton,
                { size: 'small', type: 'primary', onClick: () => handleEdit(row) },
                { default: () => '编辑' }
              ),
              h(
                NButton,
                { size: 'small', type: row.status === 1 ? 'warning' : 'success', onClick: () => handleStatus(row) },
                { default: () => (row.status === 1 ? '禁用' : '启用') }
              ),
              h(
                NButton,
                { size: 'small', type: 'error', secondary: true, onClick: () => handleDelete(row) },
                { default: () => '删除' }
              ),
            ],
          }
        );
      },
    },
  ]);

  onMounted(() => {
    loadData();
  });

  async function loadData() {
    loading.value = true;
    try {
      const res = await List({ page: 1, pageSize: 50 });
      announcementList.value = res?.list || [];
      totalCount.value = res?.totalCount || 0;
    } finally {
      loading.value = false;
    }
  }

  async function handleAdd() {
    formValue.value = newFormValue();
    const res = await MaxSort();
    formValue.value.sort = res?.sort || 0;
    editVisible.value = true;
  }

  function handleEdit(item: Recordable) {
    formValue.value = { ...newFormValue(), ...item };
    handleCategoryChange(formValue.value.categoryCode || 'blog');
    editVisible.value = true;
  }

  async function handleStatus(item: Recordable) {
    await Status({ id: item.id, status: item.status === 1 ? 2 : 1 });
    message.success('操作成功');
    loadData();
  }

  function handleDelete(item: Recordable) {
    dialog.warning({
      title: '删除文章',
      content: `确认删除「${item.title || item.id}」吗？删除后不可恢复。`,
      positiveText: '删除',
      negativeText: '取消',
      onPositiveClick: async () => {
        await Delete({ id: item.id });
        message.success('删除成功');
        loadData();
      },
    });
  }

  async function handleSave(value: Recordable) {
    saving.value = true;
    try {
      await Edit(value);
      message.success('保存成功');
      editVisible.value = false;
      formValue.value = { ...value };
      loadData();
    } finally {
      saving.value = false;
    }
  }

  function openDetail(item: Recordable) {
    activeAnnouncement.value = item;
    detailVisible.value = true;
  }

  function handleCategoryChange(value: string) {
    const option = categoryOptions.find((item) => item.value === value);
    formValue.value.categoryName = option?.label || '博客';
  }

  function categoryLabel(value?: string) {
    const option = categoryOptions.find((item) => item.value === value);
    return option?.label || '博客';
  }

  function formatTime(value?: string) {
    if (!value) {
      return '-';
    }
    return value;
  }

  function renderRichText(value?: string) {
    if (!value) {
      return '<p class="empty-preview">暂无内容</p>';
    }
    return value;
  }

  function stripHtml(value?: string) {
    return (value || '').replace(/<[^>]+>/g, '').replace(/\s+/g, ' ').trim().slice(0, 90);
  }

  function rowKey(row: Recordable) {
    return row.id;
  }

  function newFormValue() {
    return {
      id: 0,
      title: '',
      content: '',
      categoryCode: 'blog',
      categoryName: '博客',
      summary: '',
      isBanner: 0,
      bannerImg: '',
      bannerUrl: '',
      publishAt: '',
      expireAt: '',
      sort: 0,
      status: 1,
    };
  }
</script>

<style lang="less" scoped>
  .content-announcement-page {
    .toolbar-card,
    .table-card {
      margin-bottom: 16px;
    }

    .toolbar {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 12px;
    }

    .announcement-detail-meta {
      display: flex;
      flex-wrap: wrap;
      gap: 10px;
      margin-bottom: 10px;
      color: var(--text-color-3);
      font-size: 13px;
    }

    .announcement-detail-banner {
      width: 100%;
      max-height: 260px;
      object-fit: cover;
      border-radius: 6px;
      margin-bottom: 16px;
    }

    .announcement-detail {
      line-height: 1.8;
    }

    .announcement-title-cell {
      min-width: 0;
    }

    .announcement-title-text {
      color: var(--text-color-1);
      font-weight: 600;
      line-height: 1.5;
    }

    .announcement-summary {
      margin-top: 4px;
      color: var(--text-color-3);
      font-size: 12px;
      line-height: 1.5;
      overflow: hidden;
      display: -webkit-box;
      -webkit-line-clamp: 2;
      -webkit-box-orient: vertical;
    }
  }

  :global(.rich-content) {
    color: var(--text-color-1);
    line-height: 1.8;
    word-break: break-word;
  }

  :global(.rich-content h1),
  :global(.rich-content h2),
  :global(.rich-content h3) {
    margin: 24px 0 12px;
    font-weight: 700;
    line-height: 1.35;
  }

  :global(.rich-content p) {
    margin: 0 0 12px;
  }

  :global(.rich-content ul),
  :global(.rich-content ol) {
    padding-left: 22px;
    margin: 0 0 12px;
  }

  :global(.rich-content blockquote) {
    margin: 12px 0;
    padding: 8px 12px;
    color: var(--text-color-2);
    border-left: 3px solid var(--primary-color);
    background: var(--hover-color);
  }

  :global(.rich-content code) {
    padding: 2px 5px;
    background: var(--hover-color);
    border-radius: 4px;
  }

  :global(.rich-content pre) {
    overflow: auto;
    padding: 12px;
    background: var(--hover-color);
    border-radius: 4px;
  }

  :global(.rich-content img) {
    max-width: 100%;
    border-radius: 6px;
  }

  :global(.rich-content table) {
    width: 100%;
    border-collapse: collapse;
    margin: 12px 0;
  }

  :global(.rich-content th),
  :global(.rich-content td) {
    padding: 8px;
    border: 1px solid var(--border-color);
  }
</style>
