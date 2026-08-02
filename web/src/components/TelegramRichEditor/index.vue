<template>
  <div class="telegram-rich-editor">
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
  import { ref, watch } from 'vue';
  import { QuillEditor } from '@vueup/vue-quill';
  import '@vueup/vue-quill/dist/vue-quill.snow.css';

  const props = withDefaults(defineProps<{ value?: string }>(), { value: '' });
  const emit = defineEmits<{ (event: 'update:value', value: string): void }>();
  const editorRef = ref<any>();
  const content = ref('');
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
    border: 1px solid var(--n-border-color);
    border-radius: 6px;
  }

  :deep(.ql-toolbar.ql-snow),
  :deep(.ql-container.ql-snow) {
    border: none;
  }

  :deep(.ql-toolbar.ql-snow) {
    border-bottom: 1px solid var(--n-border-color);
  }

  :deep(.ql-editor) {
    min-height: 180px;
    word-break: break-word;
  }
</style>
