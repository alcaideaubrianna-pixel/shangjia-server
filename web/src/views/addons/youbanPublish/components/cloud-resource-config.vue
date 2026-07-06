<template>
  <n-space vertical class="cloud-resource-config">
    <n-alert type="info" :bordered="false">
      云资源配置用于防扫图预览和后续发送链路。所有密钥保存到后台配置表，不使用环境变量注入。
    </n-alert>

    <n-card size="small" title="FAPIHub 抠图">
      <n-form :model="model" label-placement="left" label-width="170">
        <n-form-item label="启用背景抠图">
          <n-switch v-model:value="model.fapiHubEnabled" :checked-value="1" :unchecked-value="0" />
        </n-form-item>
        <n-form-item label="API Key">
          <n-input
            v-model:value="model.fapiHubApiKey"
            clearable
            placeholder="请输入 FAPIHub API Key"
            show-password-on="click"
            type="password"
          />
        </n-form-item>
        <n-form-item label="Endpoint">
          <n-input v-model:value="model.fapiHubEndpoint" placeholder="https://fapihub.com/v2/rembg/" />
        </n-form-item>
        <n-form-item label="Model">
          <n-input v-model:value="model.fapiHubModel" placeholder="falcon" />
        </n-form-item>
      </n-form>
    </n-card>

    <n-card size="small" title="权限要求">
      <n-space vertical class="cloud-help">
        <div>人脸检测已停用，二维码和贴图由用户手动拖拽摆放。</div>
        <div>FAPIHub 用于背景替换，保存时会调用默认预览图验证 API Key。</div>
        <div>API Key 保存后再次打开会脱敏显示。</div>
        <div>预览接口会按图片内容哈希缓存抠图结果和最终处理图，避免重复请求收费接口。</div>
      </n-space>
    </n-card>
  </n-space>
</template>

<script lang="ts" setup>
  const props = defineProps<{
    model: {
      fapiHubApiKey: string;
      fapiHubEnabled: number;
      fapiHubEndpoint: string;
      fapiHubModel: string;
    };
  }>();
</script>

<style scoped>
  .cloud-resource-config {
    max-width: 860px;
  }

  .cloud-help {
    color: var(--text-color-3);
    font-size: 13px;
    line-height: 1.7;
  }
</style>
