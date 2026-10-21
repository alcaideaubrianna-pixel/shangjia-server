<script setup lang="ts">
  import { computed, ref, watch } from 'vue';
  import { useClipboard } from '@vueuse/core';
  import { NAlert, NButton, NFormItem, NInput, NModal, NSpace, useMessage } from 'naive-ui';
  import { AccountLoginLink } from '@/api/addons/youbanPublish';

  const props = defineProps<{ accountId: number; disabled?: boolean }>();
  const message = useMessage();
  const { copy } = useClipboard({ legacy: true });
  const open = ref(false);
  const loading = ref(false);
  const frontendUrl = ref(localStorage.getItem('youban:login-link:frontend-url') || '');
  const token = ref('');
  const expires = ref(0);
  const link = computed(() => {
    if (!token.value) return '';
    try {
      const url = new URL(frontendUrl.value.trim());
      if (!['https:', 'http:'].includes(url.protocol) || url.username || url.password) return '';
      // Keep credentials inside the fragment for both history and hash routers.
      const hashRouter = url.hash.startsWith('#/');
      const base = url.pathname.replace(/\/$/, '');
      url.search = '';
      if (hashRouter) {
        url.hash = `/auth/token-login#token=${encodeURIComponent(token.value)}`;
      } else {
        url.pathname = `${base}/auth/token-login`;
        url.hash = `token=${encodeURIComponent(token.value)}`;
      }
      return url.toString();
    } catch {
      return '';
    }
  });
  watch(open, (visible) => {
    if (!visible) {
      token.value = '';
      expires.value = 0;
    }
  });
  watch(frontendUrl, () => {
    token.value = '';
  });
  async function generate() {
    try {
      const url = new URL(frontendUrl.value.trim());
      if (!['https:', 'http:'].includes(url.protocol) || url.username || url.password)
        throw new Error();
    } catch {
      message.warning('请输入业务后台完整地址，例如 https://后台域名/；Hash 路由需带 /#/');
      return;
    }
    loading.value = true;
    try {
      const result = await AccountLoginLink({ id: props.accountId });
      token.value = result.token;
      expires.value = result.expires;
      localStorage.setItem('youban:login-link:frontend-url', frontendUrl.value.trim());
    } finally {
      loading.value = false;
    }
  }
  async function copyLink() {
    try {
      await copy(link.value);
      message.success('登录链接已复制');
    } catch {
      message.warning('复制失败，请手动复制下方链接');
    }
  }
</script>

<template>
  <NButton size="small" :disabled="disabled" @click="open = true">登录链接</NButton>
  <NModal
    v-model:show="open"
    preset="card"
    title="生成账号登录链接"
    class="account-login-link-modal"
  >
    <NSpace vertical>
      <NAlert type="warning"
        >仅系统超级管理员可生成。持有链接即可登录该账号，请仅发给需要访问的人。</NAlert
      >
      <NFormItem label="业务后台地址">
        <NInput
          v-model:value="frontendUrl"
          :disabled="loading"
          placeholder="业务前端站点根地址（不是 API 地址）"
        />
      </NFormItem>
      <NButton :loading="loading" type="primary" @click="generate">生成登录链接</NButton>
      <template v-if="link">
        <NInput :value="link" type="textarea" readonly :autosize="{ minRows: 3, maxRows: 6 }" />
        <div>初始有效期 {{ Math.floor(expires / 60) }} 分钟；使用后遵循正常登录会话规则。</div>
        <NButton @click="copyLink">复制链接</NButton>
      </template>
    </NSpace>
  </NModal>
</template>

<style scoped>
  .account-login-link-modal {
    width: min(94vw, 640px);
  }
</style>
