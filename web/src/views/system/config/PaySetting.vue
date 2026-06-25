<template>
  <div>
    <n-spin :show="show" description="请稍候...">
      <n-form :label-width="100" :model="formValue" :rules="rules" ref="formRef">
        <n-form-item label="开启debug" path="payDebug">
          <n-switch size="large" v-model:value="formValue.payDebug" />
          <template #feedback>开启后控制台会输出支付相关的日志</template>
        </n-form-item>

        <n-divider title-placement="left">支付宝</n-divider>
        <n-alert :show-icon="false" type="info">
          确保你已经申请开通过支付宝相关产品权限，建议按照以下步骤进行配置
          <br />1.
          下载支付宝平台密钥工具（下载地址：https://opendocs.alipay.com/common/02kipk），加签方式选择证书，加密算法选择RSA2
          <br />2. 生成后的私钥请在工具中转换为PKCS1格式 <br />3.
          在支付宝中配置证书，参考地址：https://opendocs.alipay.com/common/02khjo?pathHash=5403bedd
        </n-alert>
        <n-form-item label="应用ID" path="payAliPayAppId">
          <n-input v-model:value="formValue.payAliPayAppId" placeholder="" />
          <template #feedback></template>
        </n-form-item>

        <n-form-item label="应用私钥路径" path="payAliPayPrivateKey">
          <n-input v-model:value="formValue.payAliPayPrivateKey" placeholder="" clearable />
          <template #feedback
            >RSA2 加密算法默认生成格式为 PKCS8，系统默认是RSA2加密，切记转换为 PKCS1 格式</template
          >
        </n-form-item>

        <n-form-item label="应用公钥" path="payAliPayAppCertPublicKey">
          <n-input v-model:value="formValue.payAliPayAppCertPublicKey" placeholder="" clearable />
          <template #feedback>appCertPublicKey.crt证书路径</template>
        </n-form-item>

        <n-form-item label="支付宝根证书路径" path="payAliPayRootCert">
          <n-input v-model:value="formValue.payAliPayRootCert" placeholder="" clearable />
          <template #feedback>alipayRootCert.crt证书路径"</template>
        </n-form-item>

        <n-form-item label="支付宝公钥证书路径" path="payAliPayCertPublicKeyRSA2">
          <n-input v-model:value="formValue.payAliPayCertPublicKeyRSA2" placeholder="" clearable />
          <template #feedback>alipayCertPublicKey_RSA2.crt证书路径"</template>
        </n-form-item>

        <n-divider title-placement="left">微信支付</n-divider>
        <n-form-item label="应用ID" path="payWxPayAppId">
          <n-input v-model:value="formValue.payWxPayAppId" placeholder="" />
          <template #feedback>和微信配置中的微信公众号配置保持一致</template>
        </n-form-item>

        <n-form-item label="商户ID" path="payWxPayMchId">
          <n-input v-model:value="formValue.payWxPayMchId" placeholder="" />
          <template #feedback>商户ID 或者服务商模式的 sp_mchid</template>
        </n-form-item>

        <n-form-item label="证书序列号" path="payWxPaySerialNo">
          <n-input v-model:value="formValue.payWxPaySerialNo" placeholder="" />
          <template #feedback>商户证书的证书序列号</template>
        </n-form-item>
        <n-form-item label="APIv3Key" path="payWxPayAPIv3Key">
          <n-input v-model:value="formValue.payWxPayAPIv3Key" placeholder="" clearable />
          <template #feedback>商户平台获取</template>
        </n-form-item>

        <n-form-item label="私钥" path="payWxPayPrivateKey">
          <n-input
            type="textarea"
            v-model:value="formValue.payWxPayPrivateKey"
            placeholder=""
            clearable
          />
          <template #feedback>apiclient_key.pem 读取后的内容</template>
        </n-form-item>

        <n-divider title-placement="left">QQ支付</n-divider>
        <n-form-item label="应用ID" path="payQQPayAppId">
          <n-input v-model:value="formValue.payQQPayAppId" placeholder="" />
          <template #feedback></template>
        </n-form-item>

        <n-form-item label="商户ID" path="payQQPayMchId">
          <n-input v-model:value="formValue.payQQPayMchId" placeholder="" />
          <template #feedback></template>
        </n-form-item>

        <n-form-item label="ApiKey" path="payQQPayApiKey">
          <n-input
            type="textarea"
            v-model:value="formValue.payQQPayApiKey"
            placeholder=""
            clearable
          />
          <template #feedback>API秘钥值</template>
        </n-form-item>

        <n-divider title-placement="left">彩虹易支付</n-divider>
        <n-alert :show-icon="false" type="info">
          彩虹易支付使用 V2 统一下单接口，接口类型建议填写 jump，支付成功后异步回调地址为
          /api/pay/notify/rainbow。
        </n-alert>
        <n-form-item label="网关地址" path="payRainbowGateway">
          <n-input
            v-model:value="formValue.payRainbowGateway"
            placeholder="https://pay.v8jisu.cn"
            clearable
          />
          <template #feedback>不填写时默认使用 https://pay.v8jisu.cn</template>
        </n-form-item>

        <n-form-item label="商户ID" path="payRainbowPid">
          <n-input v-model:value="formValue.payRainbowPid" placeholder="" clearable />
        </n-form-item>

        <n-form-item label="接口类型" path="payRainbowMethod">
          <n-input v-model:value="formValue.payRainbowMethod" placeholder="jump" clearable />
          <template #feedback>可填 web、jump、jsapi、app、scan、applet，H5推荐 jump</template>
        </n-form-item>

        <n-form-item label="商户私钥" path="payRainbowPrivateKey">
          <n-input
            type="textarea"
            v-model:value="formValue.payRainbowPrivateKey"
            placeholder="-----BEGIN PRIVATE KEY-----"
            clearable
          />
          <template #feedback>用于 SHA256WithRSA 签名，可填写 PEM 内容或服务器文件路径</template>
        </n-form-item>

        <n-form-item label="平台公钥" path="payRainbowPlatformPublicKey">
          <n-input
            type="textarea"
            v-model:value="formValue.payRainbowPlatformPublicKey"
            placeholder="-----BEGIN PUBLIC KEY-----"
            clearable
          />
          <template #feedback>用于验证彩虹回调签名，可填写 PEM 内容或服务器文件路径</template>
        </n-form-item>

        <n-divider title-placement="left">会员认证</n-divider>
        <n-form-item label="开启支付" path="memberVipEnabled">
          <n-switch size="large" v-model:value="formValue.memberVipEnabled" />
          <template #feedback>关闭后，移动端点击开通会员将打开客服聊天</template>
        </n-form-item>

        <n-form-item label="客服兜底" path="memberVipCustomerFallback">
          <n-switch size="large" v-model:value="formValue.memberVipCustomerFallback" />
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
          <template #feedback>支付宝、微信、USDT 等支付类型统一走彩虹易支付，使用同一个认证价格</template>
        </n-form-item>

        <div>
          <n-space>
            <n-button type="primary" @click="formSubmit">保存更新</n-button>
            <!--            <n-button type="default" @click="sendTest">测试支付</n-button>-->
          </n-space>
        </div>
      </n-form>
    </n-spin>

    <n-modal
      :block-scroll="false"
      :mask-closable="false"
      v-model:show="showModal"
      :show-icon="false"
      preset="dialog"
      title="发送测试短信"
    >
      <n-form
        :model="formParams"
        :rules="rules"
        ref="formTestRef"
        label-placement="left"
        :label-width="80"
        class="py-4"
      >
        <n-form-item label="事件模板" path="event">
          <n-select
            :options="dict.getOptionUnRef('config_sms_template')"
            v-model:value="formParams.event"
          />
        </n-form-item>

        <n-form-item label="手机号" path="mobile">
          <n-input
            placeholder="请输入接收手机号"
            v-model:value="formParams.mobile"
            :required="true"
          />
        </n-form-item>

        <n-form-item label="验证码" path="code">
          <n-input
            placeholder="请输入要接收的验证码"
            v-model:value="formParams.code"
            :required="true"
          />
        </n-form-item>
      </n-form>

      <template #action>
        <n-space>
          <n-button @click="() => (showModal = false)">关闭</n-button>
          <n-button type="info" :loading="formBtnLoading" @click="confirmForm">发送</n-button>
        </n-space>
      </template>
    </n-modal>
  </div>
</template>

<script lang="ts" setup>
  import { ref, onMounted } from 'vue';
  import { useMessage } from 'naive-ui';
  import { getConfig, sendTestSms, updateConfig } from '@/api/sys/config';
  import { useDictStore } from '@/store/modules/dict';

  const group = ref('pay');
  const show = ref(false);
  const showModal = ref(false);
  const formBtnLoading = ref(false);
  const formParams = ref({ mobile: '', event: '', code: '1234' });
  const rules = {};
  const formTestRef = ref<any>();
  const formRef: any = ref(null);
  const message = useMessage();
  const dict = useDictStore();

  const formValue = ref({
    payDebug: true,
    payAliPayAppId: '',
    payAliPayPrivateKey: '',
    payAliPayAppCertPublicKey: '',
    payAliPayRootCert: '',
    payAliPayCertPublicKeyRSA2: '',
    payWxPayAppId: '',
    payWxPayMchId: '',
    payWxPaySerialNo: '',
    payWxPayAPIv3Key: '',
    payWxPayPrivateKey: '',
    payQQPayAppId: '',
    payQQPayMchId: '',
    payQQPayApiKey: '',
    payRainbowGateway: 'https://pay.v8jisu.cn',
    payRainbowPid: '',
    payRainbowPrivateKey: '',
    payRainbowPlatformPublicKey: '',
    payRainbowMethod: 'jump',
    memberVipEnabled: true,
    memberVipCustomerFallback: true,
    memberVipDays: 30,
    memberVipMoney: 30,
  });

  function formSubmit() {
    formRef.value.validate((errors) => {
      if (!errors) {
        const { memberVipEnabled, memberVipCustomerFallback, memberVipDays, memberVipMoney, memberVipPayItems, ...payConfig } =
          formValue.value;
        const memberVipConfig = {
          memberVipEnabled,
          memberVipCustomerFallback,
          memberVipDays,
          memberVipMoney,
        };
        Promise.all([
          updateConfig({ group: group.value, list: payConfig }),
          updateConfig({ group: 'member_vip', list: memberVipConfig }),
        ]).then((_res) => {
          message.success('更新成功');
          load();
        });
      } else {
        message.error('验证失败，请填写完整信息');
      }
    });
  }

  function load() {
    show.value = true;
    loadOptions();
    Promise.all([getConfig({ group: group.value }), getConfig({ group: 'member_vip' })])
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

  function loadOptions() {
    dict.loadOptions(['config_sms_template', 'config_sms_drive']);
  }

  function confirmForm(e) {
    e.preventDefault();
    formBtnLoading.value = true;
    formTestRef.value.validate((errors) => {
      if (!errors) {
        sendTestSms(formParams.value).then((_res) => {
          message.success('发送成功');
          showModal.value = false;
        });
      } else {
        message.error('请填写完整信息');
      }
      formBtnLoading.value = false;
    });
  }

  onMounted(() => {
    load();
  });
</script>
