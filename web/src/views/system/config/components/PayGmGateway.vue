<template>
  <div>
    <n-alert :show-icon="false" type="info" class="mb-4">
      直接对接 GMPay，适合本地联调和正式投放。
    </n-alert>
    <n-form-item label="网关地址" path="payGMPayGateway">
      <n-input v-model:value="formValue.payGMPayGateway" clearable />
    </n-form-item>
    <n-form-item label="商户ID" path="payGMPayPid">
      <n-input v-model:value="formValue.payGMPayPid" clearable />
    </n-form-item>
    <n-form-item label="Secret Key" path="payGMPayKey">
      <n-input v-model:value="formValue.payGMPayKey" clearable />
    </n-form-item>
    <n-form-item label="默认代币" path="payGMPayToken">
      <n-input v-model:value="formValue.payGMPayToken" clearable />
    </n-form-item>
    <n-form-item label="默认网络" path="payGMPayNetwork">
      <n-input v-model:value="formValue.payGMPayNetwork" clearable />
    </n-form-item>
  </div>
</template>

<script lang="ts" setup>
  import type { PaySettingFormValue } from './pay-setting-model';

  defineModel<PaySettingFormValue>('formValue', { required: true });
</script>
