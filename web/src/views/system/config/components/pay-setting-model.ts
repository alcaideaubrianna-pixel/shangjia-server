export interface PaySettingFormValue {
  payDebug: boolean;
  payAliPayAppId: string;
  payAliPayPrivateKey: string;
  payAliPayAppCertPublicKey: string;
  payAliPayRootCert: string;
  payAliPayCertPublicKeyRSA2: string;
  payWxPayAppId: string;
  payWxPayMchId: string;
  payWxPaySerialNo: string;
  payWxPayAPIv3Key: string;
  payWxPayPrivateKey: string;
  payQQPayAppId: string;
  payQQPayMchId: string;
  payQQPayApiKey: string;
  payGMPayGateway: string;
  payGMPayPid: string;
  payGMPayKey: string;
  payGMPayToken: string;
  payGMPayNetwork: string;
  payRainbowGateway: string;
  payRainbowPid: string;
  payRainbowKey: string;
  memberVipEnabled: boolean;
  memberVipCustomerFallback: boolean;
  memberVipDays: number;
  memberVipMoney: number;
}

export function createPaySettingFormValue(): PaySettingFormValue {
  return {
    memberVipCustomerFallback: true,
    memberVipDays: 30,
    memberVipEnabled: true,
    memberVipMoney: 30,
    payAliPayAppCertPublicKey: '',
    payAliPayAppId: '',
    payAliPayCertPublicKeyRSA2: '',
    payAliPayPrivateKey: '',
    payAliPayRootCert: '',
    payDebug: true,
    payQQPayApiKey: '',
    payQQPayAppId: '',
    payQQPayMchId: '',
    payGMPayGateway: 'http://127.0.0.1:18000',
    payGMPayKey: '',
    payGMPayNetwork: 'tron',
    payGMPayPid: '',
    payGMPayToken: 'usdt',
    payRainbowGateway: 'https://pay.v8jisu.cn',
    payRainbowKey: '',
    payRainbowPid: '',
    payWxPayAPIv3Key: '',
    payWxPayAppId: '',
    payWxPayMchId: '',
    payWxPayPrivateKey: '',
    payWxPaySerialNo: '',
  };
}
