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
          <n-tag type="success" :bordered="false">Banner {{ banners.length }} 条</n-tag>
        </n-space>
      </div>
    </n-card>

    <n-spin :show="loading">
      <n-card v-if="banners.length" :bordered="false" class="proCard banner-card">
        <n-carousel autoplay show-arrow>
          <div
            v-for="item in banners"
            :key="item.id"
            class="banner-slide"
            @click="openDetail(item)"
          >
            <img v-if="item.bannerImg" :src="item.bannerImg" :alt="item.title" />
            <div v-else class="banner-slide__fallback">{{ item.title }}</div>
            <div class="banner-slide__caption">
              <n-tag size="small" type="success" :bordered="false">Banner</n-tag>
              <span>{{ item.title }}</span>
            </div>
          </div>
        </n-carousel>
      </n-card>

      <n-card :bordered="false" class="proCard notice-card">
        <n-list v-if="announcements.length" hoverable>
          <n-list-item v-for="item in announcements" :key="item.id">
            <n-thing :title="item.title">
              <template #header-extra>
                <n-space>
                  <n-tag
                    size="small"
                    :type="item.status === 1 ? 'success' : 'warning'"
                    :bordered="false"
                  >
                    {{ item.status === 1 ? '启用' : '禁用' }}
                  </n-tag>
                  <n-tag v-if="item.isBanner === 1" size="small" type="success" :bordered="false">
                    Banner
                  </n-tag>
                  <n-tag v-if="item.categoryName" size="small" type="info" :bordered="false">
                    {{ item.categoryName }}
                  </n-tag>
                </n-space>
              </template>

              <div class="notice-meta">
                <span>发布时间：{{ formatTime(item.publishAt || item.createdAt) }}</span>
                <span>过期时间：{{ formatTime(item.expireAt) }}</span>
                <span>排序：{{ item.sort || 0 }}</span>
              </div>
              <div class="notice-content rich-content" v-html="renderRichText(item.content)"></div>

              <template #footer>
                <n-space>
                  <n-button size="small" @click="openDetail(item)">预览</n-button>
                  <n-button size="small" type="primary" @click="handleEdit(item)">编辑</n-button>
                  <n-button
                    size="small"
                    :type="item.status === 1 ? 'warning' : 'success'"
                    @click="handleStatus(item)"
                  >
                    {{ item.status === 1 ? '禁用' : '启用' }}
                  </n-button>
                </n-space>
              </template>
            </n-thing>
          </n-list-item>
        </n-list>
        <n-empty v-else description="暂无公告" />
      </n-card>
    </n-spin>

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

    <n-modal
      v-model:show="editVisible"
      preset="card"
      class="announcement-edit-modal"
      :bordered="false"
    >
      <template #header>{{ formValue.id ? '编辑APP公告' : '新增APP公告' }}</template>
      <n-form
        ref="formRef"
        :model="formValue"
        :rules="rules"
        label-placement="left"
        :label-width="90"
      >
        <n-form-item label="标题" path="title">
          <n-input v-model:value="formValue.title" placeholder="请输入公告标题" />
        </n-form-item>
        <n-form-item label="文章分类" path="categoryCode">
          <n-select
            v-model:value="formValue.categoryCode"
            :options="categoryOptions"
            placeholder="请选择文章分类"
            @update:value="handleCategoryChange"
          />
        </n-form-item>
        <n-form-item label="摘要" path="summary">
          <n-input
            v-model:value="formValue.summary"
            type="textarea"
            placeholder="请输入文章摘要，用于分类页卡片展示"
            :autosize="{ minRows: 2, maxRows: 4 }"
          />
        </n-form-item>
        <n-form-item label="正文" path="content">
          <div class="announcement-rich-editor">
            <Editor v-model:value="formValue.content" id="announcementEditor" />
            <div class="announcement-rich-preview">
              <div class="preview-title">预览</div>
              <div class="rich-content" v-html="renderRichText(formValue.content)"></div>
            </div>
          </div>
        </n-form-item>
        <n-form-item label="Banner" path="isBanner">
          <n-switch v-model:value="formValue.isBanner" :checked-value="1" :unchecked-value="0" />
        </n-form-item>
        <template v-if="formValue.isBanner === 1">
          <n-form-item label="Banner图" path="bannerImg">
            <UploadImage
              :maxNumber="1"
              :imageAspectRatio="8 / 3"
              :imageAspectRatioTolerance="0.03"
              helpText="Banner 图片比例必须为 8:3，例如 2048 × 768"
              v-model:value="formValue.bannerImg"
            />
          </n-form-item>
          <n-form-item label="跳转链接" path="bannerUrl">
            <n-input v-model:value="formValue.bannerUrl" placeholder="请输入跳转链接" />
          </n-form-item>
        </template>
        <n-form-item label="发布时间" path="publishAt">
          <DatePicker v-model:formValue="formValue.publishAt" type="datetime" />
        </n-form-item>
        <n-form-item label="过期时间" path="expireAt">
          <DatePicker v-model:formValue="formValue.expireAt" type="datetime" />
        </n-form-item>
        <n-form-item label="状态" path="status">
          <n-radio-group v-model:value="formValue.status">
            <n-radio-button :value="1" label="启用" />
            <n-radio-button :value="2" label="禁用" />
          </n-radio-group>
        </n-form-item>
        <n-form-item label="排序" path="sort">
          <n-input-number v-model:value="formValue.sort" class="sort-input" />
        </n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="editVisible = false">取消</n-button>
          <n-button type="primary" :loading="saving" @click="handleSave">保存</n-button>
        </n-space>
      </template>
    </n-modal>
  </div>
</template>

<script lang="ts" setup>
  import { computed, onMounted, ref } from 'vue';
  import { useMessage } from 'naive-ui';
  import { Edit, List, MaxSort, Status } from '@/api/appAnnouncement';
  import DatePicker from '@/components/DatePicker/datePicker.vue';
  import Editor from '@/components/Editor/editor.vue';
  import UploadImage from '@/components/Upload/uploadImage.vue';

  const loading = ref(false);
  const saving = ref(false);
  const message = useMessage();
  const announcementList = ref<Recordable[]>([]);
  const totalCount = ref(0);
  const detailVisible = ref(false);
  const editVisible = ref(false);
  const activeAnnouncement = ref<Recordable>({});
  const formRef = ref();
  const formValue = ref<Recordable>(newFormValue());
  const categoryOptions = [
    { label: '关于我们', value: 'about' },
    { label: '交友成功案例', value: 'case' },
    { label: '博客', value: 'blog' },
    { label: '文档', value: 'docs' },
    { label: '新闻', value: 'news' },
  ];
  const rules = {
    title: { required: true, trigger: ['input', 'blur'], message: '请输入公告标题' },
    categoryCode: { required: true, trigger: ['change', 'blur'], message: '请选择文章分类' },
    content: { required: true, trigger: ['input', 'blur'], message: '请输入公告正文' },
  };

  const banners = computed(() =>
    announcementList.value.filter((item) => item.isBanner === 1 && item.status === 1)
  );
  const announcements = computed(() => announcementList.value);

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

  function handleSave() {
    formRef.value?.validate(async (errors) => {
      if (errors) {
        return;
      }
      saving.value = true;
      try {
        await Edit(formValue.value);
        message.success('保存成功');
        editVisible.value = false;
        loadData();
      } finally {
        saving.value = false;
      }
    });
  }

  function openDetail(item: Recordable) {
    activeAnnouncement.value = item;
    detailVisible.value = true;
  }

  function handleCategoryChange(value: string) {
    const option = categoryOptions.find((item) => item.value === value);
    formValue.value.categoryName = option?.label || '博客';
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
    .banner-card {
      margin-bottom: 16px;
    }

    .toolbar {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 12px;
    }

    .banner-slide {
      position: relative;
      aspect-ratio: 8 / 3;
      overflow: hidden;
      cursor: pointer;
      background: #111827;
      border-radius: 6px;

      img {
        width: 100%;
        height: 100%;
        object-fit: cover;
        display: block;
      }
    }

    .banner-slide__fallback {
      height: 100%;
      display: flex;
      align-items: center;
      justify-content: center;
      color: #ffffff;
      font-size: 24px;
      font-weight: 600;
    }

    .banner-slide__caption {
      position: absolute;
      left: 20px;
      right: 20px;
      bottom: 18px;
      display: flex;
      gap: 10px;
      align-items: center;
      color: #ffffff;
      font-size: 18px;
      font-weight: 600;
      text-shadow: 0 1px 4px rgb(0 0 0 / 45%);
    }

    .notice-meta,
    .announcement-detail-meta {
      display: flex;
      flex-wrap: wrap;
      gap: 10px;
      margin-bottom: 10px;
      color: var(--text-color-3);
      font-size: 13px;
    }

    .notice-content {
      max-height: 84px;
      overflow: hidden;
      color: var(--text-color-2);
      line-height: 1.7;
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

    .sort-input {
      width: 100%;
    }
  }

  :global(.announcement-edit-modal) {
    width: min(1180px, calc(100vw - 48px));
  }

  :global(.announcement-edit-modal .announcement-rich-editor) {
    width: 100%;
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(320px, 0.9fr);
    gap: 16px;
  }

  :global(.announcement-edit-modal .announcement-rich-preview) {
    min-height: 420px;
    max-height: 68vh;
    overflow: auto;
    padding: 16px;
    background: var(--card-color);
    border: 1px solid var(--border-color);
    border-radius: 4px;
  }

  :global(.announcement-edit-modal .preview-title) {
    margin-bottom: 12px;
    color: var(--text-color-3);
    font-size: 13px;
    font-weight: 600;
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

  @media (max-width: 900px) {
    :global(.announcement-edit-modal .announcement-rich-editor) {
      grid-template-columns: 1fr;
    }
  }
</style>
