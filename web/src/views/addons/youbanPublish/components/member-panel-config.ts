export const paymentGatewayOptions = [
  { label: 'GMPay', value: 'gmpay' },
  { label: '彩虹易支付', value: 'rainbow' },
];

export const currencyOptions = [
  { label: 'USDT', value: 'USDT' },
  { label: 'RMB', value: 'RMB' },
];

export function defaultConfig() {
  return {
    activityText: '',
    activityTitle: '',
    currency: 'USDT',
    discountText: '限时半价',
    enabled: true,
    inviteRewardDays: 30,
    monthlyPrice: 30,
    originalPrice: 60,
    paymentGateway: 'gmpay',
  };
}
