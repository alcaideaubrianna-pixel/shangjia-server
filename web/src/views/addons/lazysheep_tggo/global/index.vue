<template>
  <div>
    <div class="n-layout-page-header">
      <n-card :bordered="false" title="全局配置"></n-card>
    </div>
    <n-card :bordered="false" class="proCard">
      <n-spin :show="show" description="请稍候...">
        <n-form :label-width="130" :model="formValue" ref="formRef">
          <n-form-item label="Telegram 代理" path="telegramProxy">
            <n-input
              v-model:value="formValue.telegramProxy"
              clearable
              placeholder="例如 http://127.0.0.1:7890 或 socks5://127.0.0.1:7890"
            />
          </n-form-item>
          <n-space>
            <n-button type="primary" :loading="saving" @click="formSubmit">保存更新</n-button>
          </n-space>
        </n-form>
      </n-spin>
    </n-card>
  </div>
</template>

<script lang="ts" setup>
  import { onMounted, ref } from 'vue';
  import { useMessage } from 'naive-ui';
  import {
    getConfig,
    testTelegramProxy,
    updateConfig,
  } from '@/api/addons/lazysheep_tggo/config';

  const show = ref(false);
  const saving = ref(false);
  const formRef = ref();
  const message = useMessage();
  const stateRef = ref<any>(null);
  const formValue = ref({
    telegramProxy: '',
  });

  async function formSubmit() {
    saving.value = true;
    try {
      await testTelegramProxy({
        telegramProxy: formValue.value.telegramProxy || '',
      });
    } catch (e) {
      message.error('Telegram 代理检测失败，请确认代理地址可用');
      saving.value = false;
      return;
    }
    const state = normalizeState(stateRef.value);
    state.global = {
      ...(state.global || {}),
      telegramProxy: formValue.value.telegramProxy || '',
    };
    try {
      await updateConfig({
        group: 'global',
        list: {
          state,
        },
      });
      message.success('更新成功');
      load();
    } finally {
      saving.value = false;
    }
  }

  function load() {
    show.value = true;
    getConfig({ group: 'global' })
      .then((res) => {
        const state = normalizeState(res?.list?.state);
        stateRef.value = state;
        formValue.value.telegramProxy = state.global?.telegramProxy || '';
      })
      .finally(() => {
        show.value = false;
      });
  }

  function normalizeState(state) {
    const next = state || {};
    next.bots = next.bots || {};
    next.users = next.users || {};
    next.bindings = next.bindings || {};
    next.settings = next.settings || {};
    next.plugins = next.plugins || {};
    next.global = next.global || {};
    return next;
  }

  onMounted(() => {
    load();
  });
</script>
