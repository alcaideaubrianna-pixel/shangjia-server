<template>
  <div>
    <n-alert :show-icon="false" type="info" class="mb-4">
      QQ 支付按商户参数配置即可，主要用于保留旧渠道兼容。
    </n-alert>
    <n-form-item label="应用ID" path="payQQPayAppId">
      <n-input v-model:value="formValue.payQQPayAppId" clearable />
    </n-form-item>
    <n-form-item label="商户ID" path="payQQPayMchId">
      <n-input v-model:value="formValue.payQQPayMchId" clearable />
    </n-form-item>
    <n-form-item label="ApiKey" path="payQQPayApiKey">
      <n-input v-model:value="formValue.payQQPayApiKey" type="textarea" clearable />
    </n-form-item>
  </div>
</template>

<script lang="ts" setup>
  import type { PaySettingFormValue } from './pay-setting-model';

  defineModel<PaySettingFormValue>('formValue', { required: true });
</script>
