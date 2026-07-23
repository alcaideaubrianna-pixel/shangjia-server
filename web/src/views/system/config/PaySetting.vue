<template>
  <div>
    <n-spin :show="show" description="请稍候...">
      <n-form ref="formRef" :label-width="120" :model="formValue">
        <n-alert :show-icon="false" type="info" class="mb-4">
          这里统一管理系统支付配置。GMPay 和彩虹易支付分别独立配置。
        </n-alert>

        <n-divider title-placement="left">通用配置</n-divider>
        <n-form-item label="开启debug" path="payDebug">
          <n-switch v-model:value="formValue.payDebug" size="large" />
        </n-form-item>

        <n-form-item label="会员认证" path="memberVipEnabled">
          <n-switch v-model:value="formValue.memberVipEnabled" size="large" />
          <template #feedback>关闭后，移动端点击开通会员将打开客服聊天</template>
        </n-form-item>
        <n-form-item label="客服兜底" path="memberVipCustomerFallback">
          <n-switch v-model:value="formValue.memberVipCustomerFallback" size="large" />
          <template #feedback>支付关闭或无可用支付渠道时打开客服聊天</template>
        </n-form-item>
        <n-form-item label="认证天数" path="memberVipDays">
          <n-input-number v-model:value="formValue.memberVipDays" :min="1" clearable />
        </n-form-item>
        <n-form-item label="认证价格" path="memberVipMoney">
          <n-input-number
            v-model:value="formValue.memberVipMoney"
            :min="0"
            :precision="2"
            clearable
          >
            <template #suffix>元</template>
          </n-input-number>
        </n-form-item>

        <n-divider title-placement="left">网关配置</n-divider>
        <n-tabs type="line" animated>
          <n-tab-pane name="gmpay" tab="GMPay">
            <PayGmGateway v-model:formValue="formValue" />
          </n-tab-pane>
          <n-tab-pane name="rainbow" tab="彩虹易支付">
            <PayRainbowGateway v-model:formValue="formValue" />
          </n-tab-pane>
          <n-tab-pane name="alipay" tab="支付宝">
            <PayAliGateway v-model:formValue="formValue" />
          </n-tab-pane>
          <n-tab-pane name="wxpay" tab="微信支付">
            <PayWxGateway v-model:formValue="formValue" />
          </n-tab-pane>
          <n-tab-pane name="qqpay" tab="QQ支付">
            <PayQqGateway v-model:formValue="formValue" />
          </n-tab-pane>
        </n-tabs>

        <n-space class="mt-4">
          <n-button type="primary" @click="formSubmit">保存更新</n-button>
        </n-space>
      </n-form>
    </n-spin>
  </div>
</template>

<script lang="ts" setup>
  import { onMounted, ref } from 'vue';
  import { useMessage } from 'naive-ui';
  import { getConfig, updateConfig } from '@/api/sys/config';
  import PayAliGateway from './components/PayAliGateway.vue';
  import PayGmGateway from './components/PayGmGateway.vue';
  import PayQqGateway from './components/PayQqGateway.vue';
  import PayRainbowGateway from './components/PayRainbowGateway.vue';
  import PayWxGateway from './components/PayWxGateway.vue';
  import {
    createPaySettingFormValue,
    type PaySettingFormValue,
  } from './components/pay-setting-model';

  const group = 'pay';
  const show = ref(false);
  const formRef = ref<any>();
  const message = useMessage();
  const formValue = ref<PaySettingFormValue>(createPaySettingFormValue());

  function formSubmit() {
    formRef.value.validate((errors) => {
      if (errors) {
        message.error('验证失败，请填写完整信息');
        return;
      }
      const {
        memberVipEnabled,
        memberVipCustomerFallback,
        memberVipDays,
        memberVipMoney,
        memberVipPayItems,
        ...payConfig
      } = formValue.value;
      const memberVipConfig = {
        memberVipEnabled,
        memberVipCustomerFallback,
        memberVipDays,
        memberVipMoney,
      };
      Promise.all([
        updateConfig({ group, list: payConfig }),
        updateConfig({ group: 'member_vip', list: memberVipConfig }),
      ]).then(() => {
        message.success('更新成功');
        load();
      });
    });
  }

  function load() {
    show.value = true;
    Promise.all([getConfig({ group }), getConfig({ group: 'member_vip' })])
      .then(([payRes, memberVipRes]) => {
        formValue.value = {
          ...formValue.value,
          ...(payRes.list || {}),
          ...(memberVipRes.list || {}),
        };
      })
      .finally(() => {
        show.value = false;
      });
  }

  onMounted(load);
</script>
