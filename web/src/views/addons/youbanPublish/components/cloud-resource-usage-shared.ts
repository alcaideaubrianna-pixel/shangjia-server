export const cloudResourceOptions = [
  { label: '云端抠图', value: 'background_matting' },
  { label: '人脸检测', value: 'face_detection' },
];

export const cloudResourceDateShortcuts = {
  本月: currentMonthRange,
  最近7天: () => recentDaysRange(7),
  最近30天: () => recentDaysRange(30),
};

export function currentMonthRange(): [number, number] {
  const now = new Date();
  return [new Date(now.getFullYear(), now.getMonth(), 1).getTime(), endOfDay(now).getTime()];
}

export function recentDaysRange(days: number): [number, number] {
  const end = endOfDay(new Date());
  const start = new Date(end);
  start.setDate(start.getDate() - Math.max(days - 1, 0));
  start.setHours(0, 0, 0, 0);
  return [start.getTime(), end.getTime()];
}

export function normalizeCloudResourceDateRange(range: [number, number] | null) {
  const value = range || currentMonthRange();
  return [formatCloudResourceDate(value[0]), formatCloudResourceDate(value[1])];
}

export function formatCloudResourceNumber(value: number) {
  return Number(value || 0).toLocaleString('zh-CN');
}

export function formatCloudResourceDuration(value: number) {
  const duration = Number(value || 0);
  if (duration < 1000) return `${duration} ms`;
  return `${(duration / 1000).toFixed(duration >= 10000 ? 1 : 2)} s`;
}

export function cloudResourceSuccessRate(row: any) {
  const requestCount = Number(row?.requestCount || 0);
  return requestCount > 0 ? (Number(row?.successCount || 0) / requestCount) * 100 : 0;
}

export function cloudResourceLabel(resourceType: string) {
  return (
    cloudResourceOptions.find((item) => item.value === resourceType)?.label || resourceType || '-'
  );
}

function endOfDay(date: Date) {
  const value = new Date(date);
  value.setHours(23, 59, 59, 999);
  return value;
}

function formatCloudResourceDate(timestamp: number) {
  const date = new Date(timestamp);
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  return `${date.getFullYear()}-${month}-${day}`;
}
