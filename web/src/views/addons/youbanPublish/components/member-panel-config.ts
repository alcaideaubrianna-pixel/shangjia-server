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
    currency: 'USDT',
    discountText: '限时半价',
    enabled: true,
    monthlyPrice: 30,
    originalPrice: 60,
    paymentGateway: 'gmpay',
  };
}
