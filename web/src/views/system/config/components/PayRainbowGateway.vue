<template>
  <div>
    <n-alert :show-icon="false" type="info" class="mb-4">
      彩虹易支付按标准跳转模式配置，适合常规跳转支付。
    </n-alert>
    <n-form-item label="网关地址" path="payRainbowGateway">
      <n-input v-model:value="formValue.payRainbowGateway" clearable />
    </n-form-item>
    <n-form-item label="商户ID" path="payRainbowPid">
      <n-input v-model:value="formValue.payRainbowPid" clearable />
    </n-form-item>
    <n-form-item label="MD5密钥" path="payRainbowKey">
      <n-input v-model:value="formValue.payRainbowKey" clearable />
    </n-form-item>
  </div>
</template>

<script lang="ts" setup>
  import type { PaySettingFormValue } from './pay-setting-model';

  defineModel<PaySettingFormValue>('formValue', { required: true });
</script>
