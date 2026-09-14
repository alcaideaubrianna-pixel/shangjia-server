<!-- eslint-disable vue/no-mutating-props -- Parent owns and saves this reactive settings draft. -->
<template>
  <n-space vertical class="cloud-resource-config">
    <n-alert type="info" :bordered="false">
      云资源配置用于防扫图预览和后续发送链路。所有密钥保存到后台配置表，不使用环境变量注入。
    </n-alert>

    <n-card size="small" title="人像抠图">
      <n-form :model="model" label-placement="left" label-width="170">
        <n-form-item label="服务来源">
          <n-radio-group v-model:value="model.mattingProvider">
            <n-radio-button value="aliyun">阿里云人体分割</n-radio-button>
            <n-radio-button value="tencent">腾讯云 A/B 账号</n-radio-button>
            <n-radio-button value="fapihub">FAPIHub</n-radio-button>
          </n-radio-group>
        </n-form-item>

        <template v-if="model.mattingProvider === 'aliyun'">
          <n-form-item label="AccessKey ID">
            <n-input
              v-model:value="model.aliyunAccessKeyId"
              clearable
              placeholder="请输入 AccessKey ID"
            />
          </n-form-item>
          <n-form-item label="AccessKey Secret">
            <n-input
              v-model:value="model.aliyunAccessKeySecret"
              clearable
              show-password-on="click"
              type="password"
            />
          </n-form-item>
          <n-form-item label="Endpoint">
            <n-input
              v-model:value="model.aliyunEndpoint"
              placeholder="imageseg.cn-shanghai.aliyuncs.com"
            />
          </n-form-item>
        </template>

        <template v-else-if="model.mattingProvider === 'tencent'">
          <n-form-item label="SecretId">
            <n-input
              v-model:value="model.tencentSecretId"
              clearable
              placeholder="请输入 SecretId"
            />
          </n-form-item>
          <n-form-item label="SecretKey">
            <n-input
              v-model:value="model.tencentSecretKey"
              clearable
              show-password-on="click"
              type="password"
            />
          </n-form-item>
          <n-form-item label="A 账号处理桶">
            <n-input
              v-model:value="model.tencentMattingBucket"
              clearable
              placeholder="BucketName-APPID"
            />
          </n-form-item>
          <n-form-item label="地域">
            <n-input v-model:value="model.tencentRegion" placeholder="ap-singapore" />
          </n-form-item>
          <n-form-item label="临时文件目录">
            <n-input v-model:value="model.tencentMattingPath" placeholder="youban-matting" />
          </n-form-item>
        </template>

        <template v-else>
          <n-form-item label="API Key">
            <n-input
              v-model:value="model.fapiHubApiKey"
              clearable
              show-password-on="click"
              type="password"
            />
          </n-form-item>
          <n-form-item label="Endpoint">
            <n-input
              v-model:value="model.fapiHubEndpoint"
              placeholder="https://fapihub.com/v2/rembg/"
            />
          </n-form-item>
          <n-form-item label="Model">
            <n-input v-model:value="model.fapiHubModel" placeholder="falcon" />
          </n-form-item>
        </template>

        <n-form-item label="连接测试">
          <n-button type="primary" secondary :loading="testing" @click="testConnection">
            测试当前来源
          </n-button>
        </n-form-item>
      </n-form>
    </n-card>

    <n-card size="small" title="权限要求">
      <n-space vertical class="cloud-help">
        <div>人脸检测已停用，二维码和贴图由用户手动拖拽摆放。</div>
        <div>腾讯云使用 A 账号 SG 处理桶执行人像抠图，结果写入系统现有的 B 账号 COS。</div>
        <div
          >A 账号需具备处理桶 PutObject、GetObject、DeleteObject 权限，且处理桶已绑定数据万象。</div
        >
        <div>连接测试会产生一次真实云端调用，并计入对应来源的监控。</div>
        <div>密钥保存后会脱敏显示；透明 PNG 仅保存为 COS 文件，不写入数据库。</div>
      </n-space>
    </n-card>
  </n-space>
</template>

<script lang="ts" setup>
  import { ref } from 'vue';
  import { useMessage } from 'naive-ui';
  import { CloudResourceConfigTest } from '@/api/addons/youbanPublish';

  const props = defineProps<{
    model: {
      aliyunAccessKeyId: string;
      aliyunAccessKeySecret: string;
      aliyunEndpoint: string;
      fapiHubApiKey: string;
      fapiHubEnabled: number;
      fapiHubEndpoint: string;
      fapiHubModel: string;
      mattingProvider: 'aliyun' | 'fapihub' | 'tencent';
      tencentCloudSite: string;
      tencentRegion: string;
      tencentMattingBucket: string;
      tencentMattingPath: string;
      tencentSecretId: string;
      tencentSecretKey: string;
      tencentVisionEnabled: number;
    };
  }>();

  const message = useMessage();
  const testing = ref(false);

  async function testConnection() {
    testing.value = true;
    try {
      const result: any = await CloudResourceConfigTest({
        ...props.model,
        fapiHubEnabled: props.model.mattingProvider === 'fapihub' ? 1 : 0,
        tencentCloudSite: 'intl',
        tencentVisionEnabled: 0,
      });
      const total = Number(result?.totalDurationMs || 0);
      const permission = result?.permissionSummary ? `${result.permissionSummary}；` : '';
      const details = total
        ? `${permission}处理 ${Number(result?.apiDurationMs || 0)} ms，下载 ${Number(
            result?.downloadDurationMs || 0
          )} ms，写入 B 桶 ${Number(result?.uploadDurationMs || 0)} ms，总计 ${total} ms，输出 ${Number(
            result?.outputBytes || 0
          )} bytes`
        : '凭据和接口调用正常';
      message.success(`测试成功：${details}`);
    } finally {
      testing.value = false;
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
