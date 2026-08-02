<template>
  <div class="telegram-rich-editor" :style="editorStyle">
    <QuillEditor
      ref="editorRef"
      v-model:content="content"
      theme="snow"
      content-type="html"
      :toolbar="toolbar"
      :formats="formats"
      @ready="readyQuill"
      @update:content="updateContent"
    />
  </div>
</template>

<script lang="ts" setup>
  import { computed, ref, watch } from 'vue';
  import { useThemeVars } from 'naive-ui';
  import { QuillEditor } from '@vueup/vue-quill';
  import '@vueup/vue-quill/dist/vue-quill.snow.css';

  const props = withDefaults(defineProps<{ value?: string }>(), { value: '' });
  const emit = defineEmits<{ (event: 'update:value', value: string): void }>();
  const editorRef = ref<any>();
  const content = ref('');
  const themeVars = useThemeVars();
  const editorStyle = computed<Record<string, string>>(() => ({
    '--telegram-editor-border': themeVars.value.borderColor,
    '--telegram-editor-background': themeVars.value.cardColor,
    '--telegram-editor-toolbar': themeVars.value.actionColor,
    '--telegram-editor-text': themeVars.value.textColor2,
    '--telegram-editor-muted': themeVars.value.textColor3,
    '--telegram-editor-primary': themeVars.value.primaryColor,
    '--telegram-editor-hover': themeVars.value.hoverColor,
  }));
  const toolbar = [
    ['bold', 'italic', 'underline', 'strike'],
    ['blockquote', 'code-block'],
    ['link'],
    ['clean'],
  ];
  const formats = ['bold', 'italic', 'underline', 'strike', 'blockquote', 'code-block', 'link'];

  watch(
    () => props.value,
    (value) => {
      const next = sanitizeTelegramHtml(value || '');
      if (next !== content.value) {
        content.value = next;
        editorRef.value?.setHTML(next);
      }
    },
    { immediate: true }
  );

  function readyQuill() {
    editorRef.value?.setHTML(sanitizeTelegramHtml(props.value || ''));
  }

  function updateContent() {
    const value = sanitizeTelegramHtml(editorRef.value?.getHTML?.() || content.value || '');
    content.value = value;
    emit('update:value', value);
  }

  function sanitizeTelegramHtml(raw: string) {
    const doc = new DOMParser().parseFromString(raw || '', 'text/html');
    cleanNode(doc.body);
    return doc.body.innerHTML
      .replace(/<p><br><\/p>/g, '')
      .replace(/<\/p><p>/g, '<br>')
      .replace(/<\/?p>/g, '');
  }

  function cleanNode(node: Node) {
    Array.from(node.childNodes).forEach((child) => {
      if (child.nodeType === Node.TEXT_NODE) return;
      if (child.nodeType !== Node.ELEMENT_NODE) {
        child.parentNode?.removeChild(child);
        return;
      }
      const element = child as HTMLElement;
      const tag = element.tagName.toLowerCase();
      if (
        ![
          'b',
          'strong',
          'i',
          'em',
          'u',
          's',
          'strike',
          'del',
          'a',
          'code',
          'pre',
          'blockquote',
          'br',
        ].includes(tag)
      ) {
        unwrap(element);
        return;
      }
      Array.from(element.attributes).forEach((attribute) => {
        if (tag === 'a' && attribute.name === 'href') return;
        element.removeAttribute(attribute.name);
      });
      cleanNode(element);
    });
  }

  function unwrap(element: HTMLElement) {
    const parent = element.parentNode;
    if (!parent) return;
    while (element.firstChild) parent.insertBefore(element.firstChild, element);
    parent.removeChild(element);
  }
</script>

<style scoped>
  .telegram-rich-editor {
    width: 100%;
    overflow: hidden;
    color: var(--telegram-editor-text);
    background: var(--telegram-editor-background);
    border: 1px solid var(--telegram-editor-border);
    border-radius: 8px;
    transition:
      border-color 0.2s ease,
      box-shadow 0.2s ease;
  }

  .telegram-rich-editor:focus-within {
    border-color: var(--telegram-editor-primary);
    box-shadow: 0 0 0 2px color-mix(in srgb, var(--telegram-editor-primary) 18%, transparent);
  }

  :deep(.ql-toolbar.ql-snow),
  :deep(.ql-container.ql-snow) {
    border: none;
  }

  :deep(.ql-toolbar.ql-snow) {
    display: flex;
    min-height: 48px;
    padding: 8px 10px;
    align-items: center;
    background: var(--telegram-editor-toolbar);
    border-bottom: 1px solid var(--telegram-editor-border);
  }

  :deep(.ql-toolbar.ql-snow .ql-formats) {
    display: inline-flex;
    margin-right: 8px;
    gap: 2px;
    align-items: center;
  }

  :deep(.ql-toolbar.ql-snow button) {
    width: 32px;
    height: 32px;
    padding: 6px;
    border-radius: 6px;
    transition:
      color 0.2s ease,
      background-color 0.2s ease;
  }

  :deep(.ql-toolbar.ql-snow button:hover),
  :deep(.ql-toolbar.ql-snow button:focus-visible) {
    color: var(--telegram-editor-primary);
    background: var(--telegram-editor-hover);
  }

  :deep(.ql-snow .ql-stroke) {
    stroke: currentColor;
  }

  :deep(.ql-snow .ql-fill) {
    fill: currentColor;
  }

  :deep(.ql-toolbar.ql-snow button.ql-active) {
    color: var(--telegram-editor-primary);
    background: var(--telegram-editor-hover);
  }

  :deep(.ql-container.ql-snow) {
    min-height: 210px;
    color: var(--telegram-editor-text);
    background: var(--telegram-editor-background);
  }

  :deep(.ql-editor) {
    min-height: 210px;
    padding: 18px 20px 22px;
    font-size: 15px;
    line-height: 1.75;
    word-break: break-word;
  }

  :deep(.ql-editor.ql-blank::before) {
    right: 20px;
    left: 20px;
    color: var(--telegram-editor-muted);
    font-style: normal;
  }

  :deep(.ql-editor blockquote) {
    padding-left: 12px;
    border-left: 3px solid var(--telegram-editor-primary);
  }

  :deep(.ql-editor pre.ql-syntax) {
    border-radius: 6px;
  }
</style>
