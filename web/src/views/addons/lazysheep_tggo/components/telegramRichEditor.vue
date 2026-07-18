<template>
  <div class="telegram-rich-editor">
    <QuillEditor
      ref="editorRef"
      theme="snow"
      content-type="html"
      :toolbar="toolbar"
      :formats="formats"
      v-model:content="content"
      @ready="readyQuill"
      @update:content="updateContent"
    />
  </div>
</template>

<script lang="ts" setup>
  import { ref, watch } from 'vue';
  import { QuillEditor } from '@vueup/vue-quill';
  import '@vueup/vue-quill/dist/vue-quill.snow.css';

  const props = defineProps({
    value: {
      type: String,
      default: '',
    },
  });
  const emit = defineEmits(['update:value']);
  const editorRef = ref();
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
    const html = sanitizeTelegramHtml(editorRef.value?.getHTML?.() || content.value || '');
    content.value = html;
    emit('update:value', html);
  }

  function sanitizeTelegramHtml(raw: string) {
    const doc = new DOMParser().parseFromString(raw || '', 'text/html');
    cleanNode(doc.body);
    return doc.body.innerHTML.replace(/<p><br><\/p>/g, '').replace(/<\/p><p>/g, '<br>').replace(/<\/?p>/g, '');
  }

  function cleanNode(node: Node) {
    Array.from(node.childNodes).forEach((child) => {
      if (child.nodeType === Node.TEXT_NODE) {
        return;
      }
      if (child.nodeType !== Node.ELEMENT_NODE) {
        child.parentNode?.removeChild(child);
        return;
      }
      const el = child as HTMLElement;
      const tag = el.tagName.toLowerCase();
      if (!['b', 'strong', 'i', 'em', 'u', 's', 'strike', 'del', 'a', 'code', 'pre', 'blockquote', 'br'].includes(tag)) {
        unwrap(el);
        return;
      }
      Array.from(el.attributes).forEach((attr) => {
        if (tag === 'a' && attr.name === 'href') {
          return;
        }
        el.removeAttribute(attr.name);
      });
      cleanNode(el);
    });
  }

  function unwrap(el: HTMLElement) {
    const parent = el.parentNode;
    if (!parent) {
      return;
    }
    while (el.firstChild) {
      parent.insertBefore(el.firstChild, el);
    }
    parent.removeChild(el);
  }
</script>

<style scoped>
  .telegram-rich-editor {
    border: 1px solid var(--n-border-color);
    border-radius: 6px;
    overflow: hidden;
    width: 100%;
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
