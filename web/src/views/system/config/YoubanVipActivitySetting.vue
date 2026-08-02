<template>
  <n-spin :show="loading" description="请稍候...">
    <n-form ref="formRef" :label-width="150" :model="formValue">
      <n-alert :show-icon="false" type="info" class="mb-4">
        活动关闭后，用户端不再展示入口，新的绑定或付费行为也不会产生奖励。已经到账的会员时长不会被扣除。
      </n-alert>

      <n-card title="会员赠送活动" size="small" class="mb-4">
        <n-form-item label="绑定 TG 赠会员" path="youbanPublishVipBindGiftEnabled">
          <n-switch v-model:value="formValue.youbanPublishVipBindGiftEnabled" size="large" />
          <template #feedback>每个账号归属首次绑定 Telegram 时自动赠送一次。</template>
        </n-form-item>
        <n-form-item label="绑定奖励" path="youbanPublishVipBindGiftDays">
          <n-input-number
            v-model:value="formValue.youbanPublishVipBindGiftDays"
            :disabled="!formValue.youbanPublishVipBindGiftEnabled"
            :min="1"
            clearable
          >
            <template #suffix>天</template>
          </n-input-number>
        </n-form-item>

        <n-divider />

        <n-form-item label="邀请好友绑定" path="youbanPublishVipInviteBindGiftEnabled">
          <n-switch v-model:value="formValue.youbanPublishVipInviteBindGiftEnabled" size="large" />
          <template #feedback>好友通过邀请码注册并首次绑定 Telegram 后，奖励邀请人。</template>
        </n-form-item>
        <n-form-item label="邀请绑定奖励" path="youbanPublishVipInviteBindGiftDays">
          <n-input-number
            v-model:value="formValue.youbanPublishVipInviteBindGiftDays"
            :disabled="!formValue.youbanPublishVipInviteBindGiftEnabled"
            :min="1"
            clearable
          >
            <template #suffix>天</template>
          </n-input-number>
        </n-form-item>

        <n-divider />

        <n-form-item label="邀请好友首付" path="youbanPublishVipInviteFirstPayGiftEnabled">
          <n-switch
            v-model:value="formValue.youbanPublishVipInviteFirstPayGiftEnabled"
            size="large"
          />
          <template #feedback>
            每个被邀请人只有首次真实付款会奖励一次；邀请不同好友可以叠加。
          </template>
        </n-form-item>
        <n-form-item label="邀请首付奖励" path="youbanPublishVipInviteFirstPayGiftDays">
          <n-input-number
            v-model:value="formValue.youbanPublishVipInviteFirstPayGiftDays"
            :disabled="!formValue.youbanPublishVipInviteFirstPayGiftEnabled"
            :min="1"
            clearable
          >
            <template #suffix>天</template>
          </n-input-number>
        </n-form-item>
      </n-card>

      <n-card title="用户端宣传" size="small" class="mb-4">
        <n-form-item label="活动标题" path="youbanPublishVipActivityBannerTitle">
          <n-input v-model:value="formValue.youbanPublishVipActivityBannerTitle" clearable />
        </n-form-item>
        <n-form-item label="活动说明" path="youbanPublishVipActivityBannerText">
          <n-input
            v-model:value="formValue.youbanPublishVipActivityBannerText"
            type="textarea"
            :autosize="{ minRows: 3, maxRows: 6 }"
            clearable
          />
          <template #feedback>会员页和邀请页会读取此文案；活动全部关闭时不会展示。</template>
        </n-form-item>
      </n-card>

      <n-card title="Telegram 通知" size="small" class="mb-4">
        <n-alert type="warning" class="mb-4">
          默认优先通过 Youban
          官方机器人发送。历史用户未启动官方机器人时，后端会尝试使用原绑定机器人。
        </n-alert>
        <n-form-item label="充值成功通知" path="youbanPublishVipPayNotifyEnabled">
          <n-switch v-model:value="formValue.youbanPublishVipPayNotifyEnabled" size="large" />
        </n-form-item>
        <n-form-item label="赠送到账通知" path="youbanPublishVipGiftNotifyEnabled">
          <n-switch v-model:value="formValue.youbanPublishVipGiftNotifyEnabled" size="large" />
        </n-form-item>
        <n-form-item label="后台调整通知" path="youbanPublishVipAdminAdjustNotifyEnabled">
          <n-switch
            v-model:value="formValue.youbanPublishVipAdminAdjustNotifyEnabled"
            size="large"
          />
        </n-form-item>
        <n-form-item label="会员到期通知" path="youbanPublishVipExpiredNotifyEnabled">
          <n-switch v-model:value="formValue.youbanPublishVipExpiredNotifyEnabled" size="large" />
        </n-form-item>
      </n-card>

      <n-space>
        <n-button type="primary" :loading="saving" @click="formSubmit">保存活动配置</n-button>
      </n-space>
    </n-form>
  </n-spin>
</template>

<script lang="ts" setup>
  import { onMounted, ref } from 'vue';
  import { useMessage } from 'naive-ui';
  import { getConfig, updateConfig } from '@/api/sys/config';

  const group = 'youban_publish_vip_activity';
  const loading = ref(false);
  const saving = ref(false);
  const formRef = ref<any>();
  const message = useMessage();
  const formValue = ref({
    youbanPublishVipActivityBannerText:
      '绑定 Telegram 赠送 1 天，邀请好友完成绑定赠送 3 天，好友首次开通月卡再赠送 1 个月。',
    youbanPublishVipActivityBannerTitle: '邀请好友，会员时长持续叠加',
    youbanPublishVipAdminAdjustNotifyEnabled: true,
    youbanPublishVipBindGiftDays: 1,
    youbanPublishVipBindGiftEnabled: true,
    youbanPublishVipExpiredNotifyEnabled: true,
    youbanPublishVipGiftNotifyEnabled: true,
    youbanPublishVipInviteBindGiftDays: 3,
    youbanPublishVipInviteBindGiftEnabled: true,
    youbanPublishVipInviteFirstPayGiftDays: 30,
    youbanPublishVipInviteFirstPayGiftEnabled: true,
    youbanPublishVipPayNotifyEnabled: true,
  });

  function formSubmit() {
    formRef.value?.validate((errors) => {
      if (errors) {
        message.error('验证失败，请检查活动配置');
        return;
      }
      saving.value = true;
      updateConfig({ group, list: formValue.value })
        .then(() => {
          message.success('会员活动配置已更新');
          load();
        })
        .finally(() => {
          saving.value = false;
        });
    });
  }

  function load() {
    loading.value = true;
    getConfig({ group })
      .then((res) => {
        formValue.value = {
          ...formValue.value,
          ...(res.list || {}),
        };
      })
      .finally(() => {
        loading.value = false;
      });
  }

  onMounted(load);
</script>
