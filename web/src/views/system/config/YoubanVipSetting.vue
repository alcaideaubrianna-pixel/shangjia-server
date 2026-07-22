<template>
  <div>
    <n-spin :show="show" description="请稍候...">
      <n-form ref="formRef" :label-width="120" :model="formValue">
        <n-alert :show-icon="false" type="info" class="mb-4">
          这里配置上架系统 VIP
          的前台售价、折扣活动和邀请奖励。支付通道仍使用“支付配置”里的彩虹易支付。
        </n-alert>

        <n-form-item label="启用 VIP" path="youbanPublishVipEnabled">
          <n-switch v-model:value="formValue.youbanPublishVipEnabled" size="large" />
        </n-form-item>

        <n-form-item label="会员月价" path="youbanPublishVipMonthlyPrice">
          <n-input-number
            v-model:value="formValue.youbanPublishVipMonthlyPrice"
            :min="0"
            :precision="2"
            clearable
          >
            <template #suffix>U / 月</template>
          </n-input-number>
        </n-form-item>

        <n-form-item label="展示原价" path="youbanPublishVipOriginalPrice">
          <n-input-number
            v-model:value="formValue.youbanPublishVipOriginalPrice"
            :min="0"
            :precision="2"
            clearable
          >
            <template #suffix>U</template>
          </n-input-number>
          <template #feedback>用于价格卡片划线展示，不参与实际扣款。</template>
        </n-form-item>

        <n-form-item label="折扣文案" path="youbanPublishVipDiscountText">
          <n-input v-model:value="formValue.youbanPublishVipDiscountText" clearable />
        </n-form-item>

        <n-form-item label="优惠券" path="youbanPublishVipCouponEnabled">
          <n-switch v-model:value="formValue.youbanPublishVipCouponEnabled" size="large" />
        </n-form-item>

        <n-form-item label="优惠券码" path="youbanPublishVipCouponCode">
          <n-input v-model:value="formValue.youbanPublishVipCouponCode" clearable />
          <template #feedback>前台用户输入该优惠码后，创建订单时会按优惠金额抵扣。</template>
        </n-form-item>

        <n-form-item label="优惠金额" path="youbanPublishVipCouponAmount">
          <n-input-number
            v-model:value="formValue.youbanPublishVipCouponAmount"
            :min="0"
            :precision="2"
            clearable
          >
            <template #suffix>U</template>
          </n-input-number>
        </n-form-item>

        <n-form-item label="邀请奖励" path="youbanPublishVipInviteRewardDays">
          <n-input-number
            v-model:value="formValue.youbanPublishVipInviteRewardDays"
            :min="0"
            clearable
          >
            <template #suffix>天</template>
          </n-input-number>
          <template #feedback
            >B 邀请 A 注册，A 完成 VIP 付款后，B 获得对应天数 VIP，可叠加。</template
          >
        </n-form-item>

        <n-form-item label="活动标题" path="youbanPublishVipActivityTitle">
          <n-input v-model:value="formValue.youbanPublishVipActivityTitle" clearable />
        </n-form-item>

        <n-form-item label="活动说明" path="youbanPublishVipActivityText">
          <n-input
            v-model:value="formValue.youbanPublishVipActivityText"
            type="textarea"
            clearable
          />
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
  import { getConfig, updateConfig } from '@/api/sys/config';

  const group = 'youban_publish_vip';
  const show = ref(false);
  const formRef = ref<any>();
  const message = useMessage();
  const formValue = ref({
    youbanPublishVipActivityText:
      '邀请好友注册并完成首月付款后，邀请人自动获得 1 个月 VIP，有效期可叠加。',
    youbanPublishVipActivityTitle: '邀请返会员',
    youbanPublishVipCouponAmount: 0,
    youbanPublishVipCouponCode: '',
    youbanPublishVipCouponEnabled: true,
    youbanPublishVipDiscountText: '限时半价',
    youbanPublishVipEnabled: true,
    youbanPublishVipInviteRewardDays: 30,
    youbanPublishVipMonthlyPrice: 30,
    youbanPublishVipOriginalPrice: 60,
  });

  function formSubmit() {
    formRef.value.validate((errors) => {
      if (errors) {
        message.error('验证失败，请填写完整信息');
        return;
      }
      updateConfig({ group, list: formValue.value }).then(() => {
        message.success('更新成功');
        load();
      });
    });
  }

  function load() {
    show.value = true;
    getConfig({ group })
      .then((res) => {
        formValue.value = {
          ...formValue.value,
          ...(res.list || {}),
        };
      })
      .finally(() => {
        show.value = false;
      });
  }

  onMounted(load);
</script>
