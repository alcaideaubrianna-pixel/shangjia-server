<template>
  <n-space vertical class="markdown-editor" size="small">
    <n-tabs v-model:value="activeTab" type="segment">
      <n-tab-pane name="edit" tab="编辑">
        <n-input
          :value="value"
          type="textarea"
          :placeholder="placeholder"
          :autosize="{ minRows: 8, maxRows: 18 }"
          @update:value="emit('update:value', $event)"
        />
      </n-tab-pane>
      <n-tab-pane name="preview" tab="预览">
        <div v-if="previewHtml" class="markdown-preview" v-html="previewHtml"></div>
        <n-empty v-else description="暂无内容" size="small" />
      </n-tab-pane>
    </n-tabs>
    <n-text depth="3">支持标题、粗体、斜体、列表和链接；可填写多个管理后台域名。</n-text>
  </n-space>
</template>

<script lang="ts" setup>
  import { computed, ref } from 'vue';
  import MarkdownIt from 'markdown-it';

  const props = withDefaults(defineProps<{ value?: string; placeholder?: string }>(), {
    value: '',
    placeholder: '请输入 Markdown 文案',
  });
  const emit = defineEmits<{ (event: 'update:value', value: string): void }>();
  const activeTab = ref('edit');
  const markdown = new MarkdownIt({ html: false, breaks: true, linkify: true });
  const previewHtml = computed(() => markdown.render(props.value || ''));
</script>

<style scoped>
  .markdown-editor {
    width: 100%;
  }

  .markdown-preview {
    min-height: 180px;
    padding: 14px 16px;
    overflow-wrap: anywhere;
    border: 1px solid var(--n-border-color);
    border-radius: 6px;
  }

  .markdown-preview :deep(p:first-child) {
    margin-top: 0;
  }

  .markdown-preview :deep(p:last-child) {
    margin-bottom: 0;
  }
</style>
