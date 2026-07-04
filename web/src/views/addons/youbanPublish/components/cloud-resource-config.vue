<template>
  <n-space vertical class="cloud-resource-config">
    <n-alert type="info" :bordered="false">
      云资源配置用于防扫图预览和后续发送链路。所有密钥保存到后台配置表，不使用环境变量注入。
    </n-alert>

    <n-card size="small" title="腾讯云人脸检测">
      <n-form :model="model" label-placement="left" label-width="170">
        <n-form-item label="启用人脸检测">
          <n-switch
            v-model:value="model.tencentVisionEnabled"
            :checked-value="1"
            :unchecked-value="0"
          />
        </n-form-item>
        <n-form-item label="腾讯云站点">
          <n-radio-group v-model:value="model.tencentCloudSite" @update:value="applyTencentCloudSite">
            <n-radio-button value="mainland">国内云</n-radio-button>
            <n-radio-button value="intl">国际云</n-radio-button>
          </n-radio-group>
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
          <n-input v-model:value="model.tencentRegion" placeholder="国内云 ap-guangzhou，国际云 ap-singapore" />
        </n-form-item>
        <n-form-item label="BDA Endpoint">
          <n-input v-model:value="model.tencentBdaEndpoint" placeholder="bda.tencentcloudapi.com" />
        </n-form-item>
        <n-form-item label="IAI Endpoint">
          <n-input v-model:value="model.tencentIaiEndpoint" placeholder="iai.tencentcloudapi.com" />
        </n-form-item>
      </n-form>
    </n-card>

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
        <div>腾讯云只用于人脸检测，国内云使用 DetectFace，国际云使用 DetectFaceAttributes。</div>
        <div>国际云 Region 需要填写，默认使用 ap-singapore；Endpoint 默认 iai.intl.tencentcloudapi.com。</div>
        <div>FAPIHub 用于背景替换和人像背景贴图，保存时会调用默认预览图验证 API Key。</div>
        <div>SecretKey 和 API Key 保存后再次打开会脱敏显示。</div>
        <div>预览接口会按图片 pHash 缓存人脸检测、抠图结果和最终处理图，避免重复请求收费接口。</div>
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
      tencentBdaEndpoint: string;
      tencentCloudSite: string;
      tencentIaiEndpoint: string;
      tencentRegion: string;
      tencentSecretId: string;
      tencentSecretKey: string;
      tencentVisionEnabled: number;
    };
  }>();

  function applyTencentCloudSite(value: string) {
    props.model.tencentCloudSite = value;
    if (value === 'intl') {
      if (!props.model.tencentIaiEndpoint || props.model.tencentIaiEndpoint === 'iai.tencentcloudapi.com') {
        props.model.tencentIaiEndpoint = 'iai.intl.tencentcloudapi.com';
      }
      if (!props.model.tencentRegion || props.model.tencentRegion === 'ap-guangzhou') {
        props.model.tencentRegion = 'ap-singapore';
      }
      return;
    }
    if (!props.model.tencentIaiEndpoint || props.model.tencentIaiEndpoint === 'iai.intl.tencentcloudapi.com') {
      props.model.tencentIaiEndpoint = 'iai.tencentcloudapi.com';
    }
    if (!props.model.tencentRegion) {
      props.model.tencentRegion = 'ap-guangzhou';
    }
  }
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
