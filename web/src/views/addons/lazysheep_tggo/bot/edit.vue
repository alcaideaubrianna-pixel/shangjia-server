<template>
  <n-modal
    v-model:show="showModal"
    :show-icon="false"
    preset="dialog"
    :title="formValue.key ? '编辑机器人' : '添加机器人'"
  >
    <n-form :label-width="110" :model="formValue" :rules="rules" ref="formRef">
      <n-form-item label="机器人标识" path="key">
        <n-input v-model:value="formValue.key" placeholder="例如 user-main" :disabled="isEdit" />
      </n-form-item>
      <n-form-item label="Bot Token" path="token">
        <n-input
          v-model:value="formValue.token"
          type="password"
          show-password-on="click"
          placeholder="请输入 Telegram Bot Token"
        />
      </n-form-item>
      <n-form-item label="机器人权限" path="role">
        <n-select v-model:value="formValue.role" :options="roleOptions" />
      </n-form-item>
      <n-form-item label="启用机器人">
        <n-switch v-model:value="formValue.enabled" />
      </n-form-item>
      <n-alert :show-icon="false" type="info">
        官方机器人会在保存后用于菜单绑定、插件入口和用户交互；本地开发默认走 polling 或手动拉取。
      </n-alert>
    </n-form>
    <template #action>
      <n-space>
        <n-button @click="closeModal">取消</n-button>
        <n-button type="primary" :loading="loading" @click="submit">保存</n-button>
      </n-space>
    </template>
  </n-modal>
</template>

<script lang="ts" setup>
  import { ref } from 'vue';
  import { useMessage } from 'naive-ui';
  import { inspectBot } from '@/api/addons/lazysheep_tggo/config';
  import { BotRow, newBotRow, roleOptions, rules } from './model';

  const emit = defineEmits(['submit']);
  const message = useMessage();
  const formRef = ref();
  const showModal = ref(false);
  const loading = ref(false);
  const isEdit = ref(false);
  const originalToken = ref('');
  const formValue = ref<BotRow>(newBotRow());

  function openModal(record?: BotRow) {
    isEdit.value = !!record?.key;
    formValue.value = newBotRow(record || {});
    originalToken.value = formValue.value.token || '';
    showModal.value = true;
  }

  function closeModal() {
    showModal.value = false;
  }

  function submit() {
    formRef.value?.validate(async (errors) => {
      if (errors) {
        message.error('验证失败，请填写完整信息');
        return;
      }
      loading.value = true;
      try {
        const tokenChanged = (formValue.value.token || '') !== originalToken.value;
        if (tokenChanged) {
          const info = await inspectBot({ token: formValue.value.token });
          formValue.value.displayName = info?.displayName || formValue.value.displayName;
          formValue.value.username = info?.username || formValue.value.username;
        }
        emit('submit', { ...formValue.value }, (ok = true) => {
          loading.value = false;
          if (ok) {
            closeModal();
          }
        });
      } catch (e) {
        loading.value = false;
        message.error('Bot Token 检测失败，请确认 Token 是否正确');
      }
    });
  }

  defineExpose({
    openModal,
  });
</script>
