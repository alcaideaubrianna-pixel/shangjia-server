import { h, ref } from 'vue';
import { NTag } from 'naive-ui';
import { FormSchema } from '@/components/Form';

export const schemas = ref<FormSchema[]>([
  {
    field: 'status',
    component: 'NSelect',
    label: '运行状态',
    componentProps: {
      options: [
        { label: '运行中', value: 'running' },
        { label: '成功', value: 'success' },
        { label: '失败', value: 'failed' },
      ],
      placeholder: '请选择运行状态',
      clearable: true,
    },
  },
]);

function renderStatus(status: string) {
  const map: Record<string, { type: 'default' | 'success' | 'warning' | 'error'; label: string }> = {
    running: { type: 'warning', label: '运行中' },
    success: { type: 'success', label: '成功' },
    failed: { type: 'error', label: '失败' },
  };
  const item = map[status] || { type: 'default', label: status || '-' };
  return h(NTag, { type: item.type as any, bordered: false }, { default: () => item.label });
}

export const columns = [
  {
    title: 'ID',
    key: 'id',
    width: 80,
  },
  {
    title: '触发方式',
    key: 'triggerType',
    width: 110,
  },
  {
    title: '批量',
    key: 'batchSize',
    width: 90,
  },
  {
    title: '扫描',
    key: 'scanned',
    width: 90,
  },
  {
    title: '导入',
    key: 'imported',
    width: 90,
  },
  {
    title: '重复',
    key: 'duplicate',
    width: 90,
  },
  {
    title: '媒体',
    key: 'mediaImported',
    width: 90,
  },
  {
    title: '游标',
    key: 'lastSourceNoteId',
    width: 150,
  },
  {
    title: '状态',
    key: 'status',
    width: 100,
    render(row) {
      return renderStatus(row.status);
    },
  },
  {
    title: '错误',
    key: 'errorMessage',
    width: 260,
    ellipsis: { tooltip: true },
  },
  {
    title: '开始时间',
    key: 'startedAt',
    width: 180,
  },
  {
    title: '耗时(ms)',
    key: 'costMs',
    width: 110,
  },
];
