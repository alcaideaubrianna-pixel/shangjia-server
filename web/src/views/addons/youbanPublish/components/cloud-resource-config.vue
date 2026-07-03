<template>
  <n-space vertical class="cloud-resource-config">
    <n-alert type="info" :bordered="false">
      云资源配置用于防扫图、人像分割、人脸检测等能力。当前已接入腾讯云视觉，后续可在这里继续扩展阿里云、火山、百度等厂商。
    </n-alert>

    <n-card size="small" title="腾讯云视觉">
      <n-form :model="model" label-placement="left" label-width="170">
        <n-form-item label="启用腾讯云视觉">
          <n-switch
            v-model:value="model.tencentVisionEnabled"
            :checked-value="1"
            :unchecked-value="0"
          />
        </n-form-item>
        <n-form-item label="SecretId">
          <n-input
            v-model:value="model.tencentSecretId"
            clearable
            placeholder="请输入 CAM 子用户 SecretId"
          />
        </n-form-item>
        <n-form-item label="SecretKey">
          <n-input
            v-model:value="model.tencentSecretKey"
            clearable
            placeholder="请输入 CAM 子用户 SecretKey"
            show-password-on="click"
            type="password"
          />
        </n-form-item>
        <n-form-item label="Region">
          <n-input v-model:value="model.tencentRegion" placeholder="ap-guangzhou" />
        </n-form-item>
        <n-form-item label="BDA Endpoint">
          <n-input v-model:value="model.tencentBdaEndpoint" placeholder="bda.tencentcloudapi.com" />
        </n-form-item>
        <n-form-item label="IAI Endpoint">
          <n-input v-model:value="model.tencentIaiEndpoint" placeholder="iai.tencentcloudapi.com" />
        </n-form-item>
      </n-form>
    </n-card>

    <n-card size="small" title="权限要求">
      <n-space vertical class="cloud-help">
        <div>CAM 子用户建议只授权 BDA 的 SegmentPortraitPic 和 IAI 的 DetectFace。</div>
        <div>不要使用主账号密钥；SecretKey 保存后再次打开会脱敏显示。</div>
        <div>预览接口会按图片 pHash 缓存识别结果，避免重复请求云厂商。</div>
      </n-space>
    </n-card>
  </n-space>
</template>

<script lang="ts" setup>
  defineProps<{
    model: {
      tencentBdaEndpoint: string;
      tencentIaiEndpoint: string;
      tencentRegion: string;
      tencentSecretId: string;
      tencentSecretKey: string;
      tencentVisionEnabled: number;
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
