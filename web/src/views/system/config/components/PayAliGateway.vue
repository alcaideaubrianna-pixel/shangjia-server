<template>
  <div>
    <n-alert :show-icon="false" type="info" class="mb-4">
      按照支付宝证书模式配置即可，适合网页支付、手机网页和扫码支付。
    </n-alert>
    <n-form-item label="应用ID" path="payAliPayAppId">
      <n-input v-model:value="formValue.payAliPayAppId" clearable />
    </n-form-item>
    <n-form-item label="应用私钥路径" path="payAliPayPrivateKey">
      <n-input v-model:value="formValue.payAliPayPrivateKey" clearable />
    </n-form-item>
    <n-form-item label="应用公钥" path="payAliPayAppCertPublicKey">
      <n-input v-model:value="formValue.payAliPayAppCertPublicKey" clearable />
    </n-form-item>
    <n-form-item label="支付宝根证书路径" path="payAliPayRootCert">
      <n-input v-model:value="formValue.payAliPayRootCert" clearable />
    </n-form-item>
    <n-form-item label="支付宝公钥证书路径" path="payAliPayCertPublicKeyRSA2">
      <n-input v-model:value="formValue.payAliPayCertPublicKeyRSA2" clearable />
    </n-form-item>
  </div>
</template>

<script lang="ts" setup>
  import type { PaySettingFormValue } from './pay-setting-model';

  defineModel<PaySettingFormValue>('formValue', { required: true });
</script>
