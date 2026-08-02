<template>
  <div ref="containerRef" class="telegram-rich-editor" :style="editorStyle">
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
  import { computed, onBeforeUnmount, ref, watch } from 'vue';
  import { useThemeVars } from 'naive-ui';
  import { QuillEditor } from '@vueup/vue-quill';
  import '@vueup/vue-quill/dist/vue-quill.snow.css';

  const props = withDefaults(defineProps<{ value?: string }>(), { value: '' });
  const emit = defineEmits<{ (event: 'update:value', value: string): void }>();
  const containerRef = ref<HTMLElement>();
  const editorRef = ref<any>();
  const content = ref('');
  const isComposing = ref(false);
  let editorRoot: HTMLElement | null = null;
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
    ['blockquote'],
    ['code', 'code-block'],
    ['link'],
    ['clean'],
  ];
  const formats = [
    'bold',
    'italic',
    'underline',
    'strike',
    'blockquote',
    'code',
    'code-block',
    'link',
  ];

  watch(
    () => props.value,
    (value) => {
      const next = sanitizeTelegramHtml(value || '');
      const current = sanitizeTelegramHtml(editorRef.value?.getHTML?.() || content.value || '');
      if (!isComposing.value && next !== current) {
        content.value = next;
        editorRef.value?.setHTML(next);
      }
    },
    { immediate: true }
  );

  function readyQuill(quill?: any) {
    editorRef.value?.setHTML(sanitizeTelegramHtml(props.value || ''));
    editorRoot = quill?.root || editorRef.value?.getQuill?.()?.root || null;
    editorRoot?.addEventListener('compositionstart', handleCompositionStart);
    editorRoot?.addEventListener('compositionend', handleCompositionEnd);
    containerRef.value?.querySelector('.ql-code')?.setAttribute('title', '行内代码（兑换码）');
    containerRef.value?.querySelector('.ql-code-block')?.setAttribute('title', '整段代码块');
  }

  function updateContent() {
    if (isComposing.value) return;
    syncContent();
  }

  function syncContent() {
    const value = sanitizeTelegramHtml(editorRef.value?.getHTML?.() || content.value || '');
    emit('update:value', value);
  }

  function handleCompositionStart() {
    isComposing.value = true;
  }

  function handleCompositionEnd() {
    isComposing.value = false;
    window.setTimeout(syncContent, 0);
  }

  onBeforeUnmount(() => {
    editorRoot?.removeEventListener('compositionstart', handleCompositionStart);
    editorRoot?.removeEventListener('compositionend', handleCompositionEnd);
    editorRoot = null;
  });

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

  :deep(.ql-toolbar.ql-snow button.ql-code svg) {
    display: none;
  }

  :deep(.ql-toolbar.ql-snow button.ql-code::after) {
    content: '<>';
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
    font-size: 14px;
    font-weight: 700;
    line-height: 20px;
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

  :deep(.ql-editor code) {
    padding: 2px 6px;
    color: var(--telegram-editor-primary);
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
    font-size: 0.92em;
    background: var(--telegram-editor-hover);
    border-radius: 4px;
  }

  :deep(.ql-editor pre.ql-syntax) {
    border-radius: 6px;
  }
</style>
