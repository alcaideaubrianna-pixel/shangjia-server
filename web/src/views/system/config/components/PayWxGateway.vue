<template>
  <div>
    <n-alert :show-icon="false" type="info" class="mb-4">
      微信支付按官方商户号配置，支持公众号、小程序、APP、扫码和 H5。
    </n-alert>
    <n-form-item label="应用ID" path="payWxPayAppId">
      <n-input v-model:value="formValue.payWxPayAppId" clearable />
    </n-form-item>
    <n-form-item label="商户ID" path="payWxPayMchId">
      <n-input v-model:value="formValue.payWxPayMchId" clearable />
    </n-form-item>
    <n-form-item label="证书序列号" path="payWxPaySerialNo">
      <n-input v-model:value="formValue.payWxPaySerialNo" clearable />
    </n-form-item>
    <n-form-item label="APIv3Key" path="payWxPayAPIv3Key">
      <n-input v-model:value="formValue.payWxPayAPIv3Key" clearable />
    </n-form-item>
    <n-form-item label="私钥" path="payWxPayPrivateKey">
      <n-input v-model:value="formValue.payWxPayPrivateKey" type="textarea" clearable />
    </n-form-item>
  </div>
</template>

<script lang="ts" setup>
  import type { PaySettingFormValue } from './pay-setting-model';

  defineModel<PaySettingFormValue>('formValue', { required: true });
</script>
