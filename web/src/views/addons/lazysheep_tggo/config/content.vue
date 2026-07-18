<template>
  <n-spin :show="show" description="请稍候...">
    <n-form :label-width="130" :model="formValue" ref="formRef">
      <n-form-item label="默认采集链接" path="sourceUrl">
        <n-input
          v-model:value="formValue.sourceUrl"
          placeholder="https://seats.bangchat.top/GNhE1sJ"
        />
      </n-form-item>
      <n-form-item label="审核群 ID" path="reviewChatId">
        <n-input-number v-model:value="formValue.reviewChatId" :show-button="false" />
      </n-form-item>
      <n-form-item label="推送频道 ID" path="publishChatId">
        <n-input-number v-model:value="formValue.publishChatId" :show-button="false" />
      </n-form-item>
      <n-form-item label="推送策略">
        <n-space>
          <n-checkbox v-model:checked="formValue.autoPush">绑定自动推送</n-checkbox>
          <n-checkbox v-model:checked="formValue.allowVerify">显示查看验证</n-checkbox>
          <n-checkbox v-model:checked="formValue.allowLocation">显示查看位置</n-checkbox>
          <n-checkbox v-model:checked="formValue.signFollow">签到关注校验</n-checkbox>
        </n-space>
      </n-form-item>
      <n-form-item label="会员验证策略" path="memberVerify">
        <n-select
          v-model:value="formValue.memberVerify"
          :options="memberVerifyOptions"
          placeholder="请选择"
        />
      </n-form-item>
      <n-form-item label="积分解锁" path="memberPoints">
        <n-input-number
          v-model:value="formValue.memberPoints"
          :min="0"
          :show-button="false"
          placeholder="0 表示不启用"
        />
      </n-form-item>
      <n-space>
        <n-button type="primary" @click="formSubmit">保存更新</n-button>
      </n-space>
    </n-form>
  </n-spin>
</template>

<script lang="ts" setup>
  import { onMounted, ref } from 'vue';
  import { useMessage } from 'naive-ui';
  import { getConfig, updateConfig } from '@/api/addons/lazysheep_tggo/config';

  const show = ref(false);
  const formRef = ref();
  const message = useMessage();
  const stateRef = ref<any>(null);

  const memberVerifyOptions = [
    { label: '不限', value: 'none' },
    { label: '仅会员', value: 'member' },
    { label: '积分解锁', value: 'points' },
  ];

  const formValue = ref(newFormValue());

  function newFormValue() {
    return {
      sourceUrl: '',
      reviewChatId: 0,
      publishChatId: 0,
      autoPush: false,
      allowVerify: true,
      allowLocation: true,
      signFollow: false,
      memberVerify: 'none',
      memberPoints: 0,
    };
  }

  async function formSubmit() {
    const state = normalizeState(stateRef.value);
    state.settings = {
      allowVerify: formValue.value.allowVerify,
      allowLocation: formValue.value.allowLocation,
      memberVerify: formValue.value.memberVerify,
      memberPoints: String(formValue.value.memberPoints || 0),
      signFollow: formValue.value.signFollow,
    };
    const firstBotKey = Object.keys(state.bots)[0] || 'default';
    if (formValue.value.sourceUrl) {
      const bindingKey = `${firstBotKey}:${formValue.value.sourceUrl}:${formValue.value.publishChatId || 0}`;
      state.bindings[bindingKey] = {
        key: bindingKey,
        botKey: firstBotKey,
        sourceUrl: formValue.value.sourceUrl,
        reviewChatId: formValue.value.reviewChatId || 0,
        publishChatId: formValue.value.publishChatId || 0,
        status: 'enabled',
        autoPush: formValue.value.autoPush,
      };
    }
    await updateConfig({
      group: 'plugin',
      list: {
        state,
      },
    });
    message.success('更新成功');
    load();
  }

  function load() {
    show.value = true;
    getConfig({ group: 'plugin' })
      .then((res) => {
        const state = normalizeState(res?.list?.state);
        stateRef.value = state;
        packForm(state);
      })
      .finally(() => {
        show.value = false;
      });
  }

  function packForm(state) {
    const next = newFormValue();
    const bindings = state?.bindings || {};
    const firstBindingKey = Object.keys(bindings)[0];
    if (firstBindingKey) {
      const binding = bindings[firstBindingKey];
      next.sourceUrl = binding.sourceUrl || '';
      next.reviewChatId = Number(binding.reviewChatId || 0);
      next.publishChatId = Number(binding.publishChatId || 0);
      next.autoPush = !!binding.autoPush;
    }
    if (state?.settings) {
      next.allowVerify = state.settings.allowVerify !== false;
      next.allowLocation = state.settings.allowLocation !== false;
      next.signFollow = !!state.settings.signFollow;
      next.memberVerify = state.settings.memberVerify || 'none';
      next.memberPoints = Number(state.settings.memberPoints || 0);
    }
    formValue.value = next;
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
