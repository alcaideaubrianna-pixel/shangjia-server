<template>
  <div>
    <n-spin :show="show" description="请稍候...">
      <n-form :label-width="120" :model="formValue" :rules="rules" ref="formRef">
        <n-form-item label="机器人标识" path="botKey">
          <n-input v-model:value="formValue.botKey" placeholder="例如 default" />
        </n-form-item>

        <n-form-item label="Bot Token" path="token">
          <n-input
            v-model:value="formValue.token"
            type="password"
            show-password-on="click"
            placeholder="请输入 Telegram Bot Token"
          />
        </n-form-item>

        <n-form-item label="机器人名称" path="displayName">
          <n-input v-model:value="formValue.displayName" placeholder="用于后台识别" />
        </n-form-item>

        <n-form-item label="Webhook Secret" path="webhookSecret">
          <n-input v-model:value="formValue.webhookSecret" placeholder="Telegram secret token" />
        </n-form-item>

        <n-form-item label="Webhook 路径" path="webhookPath">
          <n-input
            v-model:value="formValue.webhookPath"
            placeholder="/api/lazysheep_tggo/webhook/default"
          />
        </n-form-item>

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

        <n-form-item label="开关">
          <n-space>
            <n-checkbox v-model:checked="formValue.enabled">启用机器人</n-checkbox>
            <n-checkbox v-model:checked="formValue.autoPull">自动采集</n-checkbox>
            <n-checkbox v-model:checked="formValue.autoForward">自动推送</n-checkbox>
            <n-checkbox v-model:checked="formValue.autoPush">绑定自动推送</n-checkbox>
          </n-space>
        </n-form-item>

        <n-form-item label="按钮能力">
          <n-space>
            <n-checkbox v-model:checked="formValue.allowVerify">查看验证</n-checkbox>
            <n-checkbox v-model:checked="formValue.allowLocation">查看位置</n-checkbox>
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
          <n-input v-model:value="formValue.memberPoints" placeholder="0 表示不启用" />
        </n-form-item>

        <n-space>
          <n-button type="primary" @click="formSubmit">保存更新</n-button>
        </n-space>
      </n-form>
    </n-spin>
  </div>
</template>

<script lang="ts" setup>
  import { onMounted, ref } from 'vue';
  import { useMessage } from 'naive-ui';
  import { getConfig, updateConfig } from '@/api/addons/lazysheep_tggo/config';

  const group = ref('basic');
  const show = ref(false);
  const formRef: any = ref(null);
  const message = useMessage();

  const memberVerifyOptions = [
    { label: '不限', value: 'none' },
    { label: '仅会员', value: 'member' },
    { label: '积分解锁', value: 'points' },
  ];

  const rules = {
    botKey: {
      required: true,
      message: '请输入机器人标识',
      trigger: 'blur',
    },
    token: {
      required: true,
      message: '请输入 Bot Token',
      trigger: 'blur',
    },
  };

  const formValue = ref(newFormValue());

  function newFormValue() {
    return {
      botKey: 'default',
      role: 'user',
      token: '',
      displayName: '',
      webhookSecret: '',
      webhookPath: '/api/lazysheep_tggo/webhook/default',
      sourceUrl: '',
      reviewChatId: 0,
      publishChatId: 0,
      enabled: true,
      autoPull: false,
      autoForward: false,
      autoPush: false,
      allowVerify: true,
      allowLocation: true,
      signFollow: false,
      memberVerify: 'none',
      memberPoints: '0',
    };
  }

  function formSubmit() {
    formRef.value.validate((errors) => {
      if (errors) {
        message.error('验证失败，请填写完整信息');
        return;
      }
      updateConfig({
        group: group.value,
        list: {
          state: toState(),
        },
      }).then((_res) => {
        message.success('更新成功');
        load();
      });
    });
  }

  function toState() {
    const botKey = formValue.value.botKey || 'default';
    const bindingKey = formValue.value.sourceUrl
      ? `${botKey}:${formValue.value.sourceUrl}:${formValue.value.publishChatId || 0}`
      : '';
    const bindings: Record<string, any> = {};
    const state = {
      bots: {
        [botKey]: {
          key: botKey,
          role: formValue.value.role || 'user',
          token: formValue.value.token,
          displayName: formValue.value.displayName,
          webhookSecret: formValue.value.webhookSecret,
          webhookPath: formValue.value.webhookPath,
          enabled: formValue.value.enabled,
          autoPull: formValue.value.autoPull,
          autoForward: formValue.value.autoForward,
        },
      },
      users: {},
      bindings,
      settings: {
        allowVerify: formValue.value.allowVerify,
        allowLocation: formValue.value.allowLocation,
        memberVerify: formValue.value.memberVerify,
        memberPoints: formValue.value.memberPoints,
        signFollow: formValue.value.signFollow,
      },
      plugins: {},
      global: {},
    };
    if (bindingKey) {
      state.bindings[bindingKey] = {
        key: bindingKey,
        botKey,
        sourceUrl: formValue.value.sourceUrl,
        reviewChatId: formValue.value.reviewChatId || 0,
        publishChatId: formValue.value.publishChatId || 0,
        status: 'enabled',
        autoPush: formValue.value.autoPush,
      };
    }
    return state;
  }

  function load() {
    show.value = true;
    getConfig({ group: group.value })
      .then((res) => {
        packForm(res?.list?.state);
      })
      .finally(() => {
        show.value = false;
      });
  }

  function packForm(state) {
    const next = newFormValue();
    const bots = state?.bots || {};
    const firstBotKey = Object.keys(bots)[0];
    if (firstBotKey) {
      const bot = bots[firstBotKey];
      next.botKey = bot.key || firstBotKey;
      next.role = bot.role === 'finance' ? 'official' : bot.role || 'user';
      next.token = bot.token || '';
      next.displayName = bot.displayName || '';
      next.webhookSecret = bot.webhookSecret || '';
      next.webhookPath = bot.webhookPath || `/api/lazysheep_tggo/webhook/${next.botKey}`;
      next.enabled = bot.enabled !== false;
      next.autoPull = !!bot.autoPull;
      next.autoForward = !!bot.autoForward;
    }
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
      next.memberPoints = state.settings.memberPoints || '0';
    }
    formValue.value = next;
  }

  onMounted(() => {
    load();
  });
</script>
