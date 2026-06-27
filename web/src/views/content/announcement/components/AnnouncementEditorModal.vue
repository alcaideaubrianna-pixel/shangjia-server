<template>
  <n-modal
    :show="show"
    class="article-editor-modal"
    :mask-closable="false"
    @update:show="handleShowUpdate"
  >
    <n-card :bordered="false" class="article-editor-shell" content-style="padding: 0;">
      <div class="article-editor-header">
        <div class="article-editor-title">
          <div class="article-editor-title__main">
            {{ formValue.id ? '编辑文章' : '新增文章' }}
          </div>
          <div class="article-editor-title__meta">
            <n-tag size="small" :type="formValue.status === 1 ? 'success' : 'warning'" :bordered="false">
              {{ formValue.status === 1 ? '已启用' : '已禁用' }}
            </n-tag>
            <span>{{ activeCategoryLabel }}</span>
            <span v-if="formValue.publishAt">发布时间：{{ formValue.publishAt }}</span>
          </div>
        </div>
        <n-space>
          <n-button :disabled="saving" @click="handleCancel">取消</n-button>
          <n-button type="primary" :loading="saving" @click="handleSave">保存</n-button>
        </n-space>
      </div>

      <n-form
        ref="formRef"
        :model="formValue"
        :rules="rules"
        label-placement="top"
        class="article-editor-form"
      >
        <div class="article-editor-layout">
          <div class="article-editor-main">
            <n-card :bordered="false" class="editor-panel">
              <n-form-item label="文章标题" path="title">
                <n-input
                  v-model:value="formValue.title"
                  class="article-title-input"
                  placeholder="请输入文章标题"
                  clearable
                />
              </n-form-item>

              <n-form-item label="摘要" path="summary">
                <n-input
                  v-model:value="formValue.summary"
                  type="textarea"
                  placeholder="用于分类页卡片、SEO 摘要和列表展示"
                  :autosize="{ minRows: 3, maxRows: 5 }"
                />
              </n-form-item>

              <n-form-item label="正文" path="content">
                <div class="article-content-editor">
                  <Editor v-model:value="formValue.content" id="announcementEditor" />
                </div>
              </n-form-item>
            </n-card>

            <n-card :bordered="false" class="preview-panel">
              <template #header>
                <div class="panel-title">文章预览</div>
              </template>
              <article class="article-preview">
                <h1>{{ formValue.title || '未填写标题' }}</h1>
                <p v-if="formValue.summary" class="article-preview__summary">
                  {{ formValue.summary }}
                </p>
                <div class="rich-content" v-html="previewContent"></div>
              </article>
            </n-card>
          </div>

          <aside class="article-editor-sidebar">
            <n-card :bordered="false" class="setting-panel" title="发布设置">
              <n-form-item label="状态" path="status">
                <n-radio-group v-model:value="formValue.status" class="full-width">
                  <n-radio-button :value="1" label="启用" />
                  <n-radio-button :value="2" label="禁用" />
                </n-radio-group>
              </n-form-item>
              <n-form-item label="发布时间" path="publishAt">
                <DatePicker v-model:formValue="formValue.publishAt" type="datetime" />
              </n-form-item>
              <n-form-item label="过期时间" path="expireAt">
                <DatePicker v-model:formValue="formValue.expireAt" type="datetime" />
              </n-form-item>
              <n-form-item label="排序" path="sort">
                <n-input-number v-model:value="formValue.sort" class="full-width" />
              </n-form-item>
            </n-card>

            <n-card :bordered="false" class="setting-panel" title="文章分类">
              <n-form-item label="分类" path="categoryCode">
                <n-select
                  v-model:value="formValue.categoryCode"
                  :options="categoryOptions"
                  placeholder="请选择文章分类"
                  @update:value="handleCategoryChange"
                />
              </n-form-item>
            </n-card>

            <n-card :bordered="false" class="setting-panel" title="Banner">
              <n-form-item label="设为 Banner" path="isBanner">
                <n-switch v-model:value="formValue.isBanner" :checked-value="1" :unchecked-value="0" />
              </n-form-item>
              <template v-if="formValue.isBanner === 1">
                <n-form-item label="Banner 图" path="bannerImg">
                  <UploadImage
                    v-model:value="formValue.bannerImg"
                    :maxNumber="1"
                    :imageAspectRatio="8 / 3"
                    :imageAspectRatioTolerance="0.03"
                    helpText="Banner 图片比例必须为 8:3，例如 2048 x 768"
                  />
                </n-form-item>
                <n-form-item label="跳转链接" path="bannerUrl">
                  <n-input v-model:value="formValue.bannerUrl" placeholder="请输入跳转链接" />
                </n-form-item>
              </template>
            </n-card>
          </aside>
        </div>
      </n-form>
    </n-card>
  </n-modal>
</template>

<script lang="ts" setup>
  import { computed, ref, watch } from 'vue';
  import type { FormInst, FormRules } from 'naive-ui';
  import DatePicker from '@/components/DatePicker/datePicker.vue';
  import Editor from '@/components/Editor/editor.vue';
  import UploadImage from '@/components/Upload/uploadImage.vue';

  interface ICategoryOption {
    label: string;
    value: string;
  }

  interface IArticleForm {
    id: number;
    title: string;
    content: string;
    categoryCode: string;
    categoryName: string;
    summary: string;
    isBanner: number;
    bannerImg: string;
    bannerUrl: string;
    publishAt: string;
    expireAt: string;
    sort: number;
    status: number;
    [key: string]: unknown;
  }

  const props = defineProps<{
    show: boolean;
    saving: boolean;
    modelValue: IArticleForm;
    categoryOptions: ICategoryOption[];
  }>();

  const emit = defineEmits<{
    (event: 'update:show', value: boolean): void;
    (event: 'update:modelValue', value: IArticleForm): void;
    (event: 'save', value: IArticleForm): void;
  }>();

  const formRef = ref<FormInst | null>(null);
  const formValue = ref<IArticleForm>({ ...props.modelValue });

  const rules: FormRules = {
    title: { required: true, trigger: ['input', 'blur'], message: '请输入公告标题' },
    categoryCode: { required: true, trigger: ['change', 'blur'], message: '请选择文章分类' },
    content: { required: true, trigger: ['input', 'blur'], message: '请输入公告正文' },
  };

  const activeCategoryLabel = computed(() => {
    const option = props.categoryOptions.find((item) => item.value === formValue.value.categoryCode);
    return option?.label || formValue.value.categoryName || '未分类';
  });

  const previewContent = computed(() => renderRichText(formValue.value.content));

  watch(
    () => props.modelValue,
    (value) => {
      formValue.value = { ...value };
    },
    { deep: true }
  );

  function handleShowUpdate(value: boolean) {
    emit('update:show', value);
  }

  function handleCancel() {
    emit('update:show', false);
  }

  function handleCategoryChange(value: string) {
    const option = props.categoryOptions.find((item) => item.value === value);
    formValue.value.categoryName = option?.label || '博客';
  }

  function handleSave() {
    formRef.value?.validate((errors) => {
      if (errors) {
        return;
      }
      emit('update:modelValue', { ...formValue.value });
      emit('save', { ...formValue.value });
    });
  }

  function renderRichText(value?: string) {
    if (!value) {
      return '<p class="empty-preview">暂无内容</p>';
    }
    return value;
  }
</script>

<style lang="less" scoped>
  .article-editor-shell {
    width: min(1440px, calc(100vw - 32px));
    height: calc(100vh - 32px);
    overflow: hidden;
    border-radius: 10px;
  }

  .article-editor-header {
    height: 72px;
    padding: 0 24px;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
    border-bottom: 1px solid var(--border-color);
    background: var(--card-color);
  }

  .article-editor-title {
    min-width: 0;
  }

  .article-editor-title__main {
    font-size: 18px;
    font-weight: 700;
    color: var(--text-color-1);
  }

  .article-editor-title__meta {
    margin-top: 6px;
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px;
    color: var(--text-color-3);
    font-size: 13px;
  }

  .article-editor-form {
    height: calc(100vh - 104px);
    overflow: auto;
    background: var(--body-color);
  }

  .article-editor-layout {
    padding: 20px;
    display: grid;
    grid-template-columns: minmax(0, 1fr) 340px;
    gap: 18px;
    align-items: start;
  }

  .article-editor-main {
    min-width: 0;
    display: grid;
    gap: 18px;
  }

  .editor-panel,
  .preview-panel,
  .setting-panel {
    border: 1px solid var(--border-color);
    box-shadow: none;
  }

  .panel-title {
    font-size: 15px;
    font-weight: 700;
  }

  .article-title-input {
    :deep(.n-input__input-el) {
      height: 42px;
      font-size: 22px;
      font-weight: 700;
    }
  }

  .article-content-editor {
    width: 100%;
    min-height: 520px;
    overflow: hidden;
    border: 1px solid var(--border-color);
    border-radius: 6px;
    background: var(--card-color);

    :deep(.ql-editor) {
      min-height: 470px;
      font-size: 15px;
      line-height: 1.8;
    }

    :deep(.ql-toolbar.ql-snow) {
      position: sticky;
      top: 0;
      z-index: 2;
      background: var(--card-color);
      border-bottom-color: var(--border-color);
    }
  }

  .article-editor-sidebar {
    position: sticky;
    top: 20px;
    display: grid;
    gap: 14px;
  }

  .full-width {
    width: 100%;
  }

  .article-preview {
    max-width: 820px;
    min-height: 280px;
    margin: 0 auto;
    color: var(--text-color-1);

    h1 {
      margin: 0 0 12px;
      font-size: 28px;
      line-height: 1.35;
      font-weight: 800;
      word-break: break-word;
    }
  }

  .article-preview__summary {
    margin: 0 0 20px;
    padding: 12px 14px;
    color: var(--text-color-2);
    line-height: 1.7;
    background: var(--hover-color);
    border-radius: 6px;
  }

  :deep(.n-card-header) {
    padding-bottom: 10px;
  }

  :deep(.n-form-item:last-child) {
    margin-bottom: 0;
  }

  @media (max-width: 1100px) {
    .article-editor-layout {
      grid-template-columns: 1fr;
    }

    .article-editor-sidebar {
      position: static;
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }

  @media (max-width: 720px) {
    .article-editor-shell {
      width: 100vw;
      height: 100vh;
      border-radius: 0;
    }

    .article-editor-header {
      height: auto;
      min-height: 72px;
      padding: 14px 16px;
      align-items: flex-start;
      flex-direction: column;
    }

    .article-editor-form {
      height: calc(100vh - 112px);
    }

    .article-editor-layout {
      padding: 14px;
    }

    .article-editor-sidebar {
      grid-template-columns: 1fr;
    }
  }
</style>
