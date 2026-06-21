import { h, ref } from 'vue';
import { FormSchema } from '@/components/Form';
import { defRangeShortcuts } from '@/utils/dateUtil';
import { NTag } from 'naive-ui';

export const schemas = ref<FormSchema[]>([
  {
    field: 'username',
    component: 'NInput',
    label: '用户名',
    componentProps: {
      placeholder: '请输入用户名',
    },
  },
  {
    field: 'memberId',
    component: 'NInputNumber',
    label: '用户ID',
    componentProps: {
      placeholder: '请输入用户ID',
      showButton: false,
    },
  },
  {
    field: 'source',
    component: 'NSelect',
    label: '来源',
    componentProps: {
      placeholder: '请选择来源',
      options: [
        { label: '后台修改', value: 'admin' },
        { label: '付费开通', value: 'payment' },
        { label: '自助开通', value: 'self' },
      ],
    },
  },
  {
    field: 'createdAt',
    component: 'NDatePicker',
    label: '操作时间',
    componentProps: {
      type: 'datetimerange',
      clearable: true,
      shortcuts: defRangeShortcuts(),
    },
  },
]);

function sourceLabel(source: string) {
  const map = {
    admin: '后台修改',
    payment: '付费开通',
    self: '自助开通',
  };
  return map[source] || source || '-';
}

function statusLabel(status: number) {
  return status === 1 ? '会员用户' : '普通用户';
}

export const columns = [
  {
    title: 'ID',
    key: 'id',
    width: 80,
  },
  {
    title: '用户',
    key: 'username',
    width: 160,
    render(row) {
      return `${row.username || '-'} (#${row.memberId})`;
    },
  },
  {
    title: '来源',
    key: 'source',
    width: 110,
    render(row) {
      return h(
        NTag,
        { type: row.source === 'payment' ? 'success' : 'info', bordered: false },
        { default: () => sourceLabel(row.source) }
      );
    },
  },
  {
    title: '动作',
    key: 'action',
    width: 100,
  },
  {
    title: '状态变更',
    key: 'afterStatus',
    width: 160,
    render(row) {
      return `${statusLabel(row.beforeStatus)} -> ${statusLabel(row.afterStatus)}`;
    },
  },
  {
    title: '等级变更',
    key: 'afterLevel',
    width: 120,
    render(row) {
      return `${row.beforeLevel || 0} -> ${row.afterLevel || 0}`;
    },
  },
  {
    title: '到期时间变更',
    key: 'afterExpiredAt',
    width: 320,
    render(row) {
      return `${row.beforeExpiredAt || '-'} -> ${row.afterExpiredAt || '-'}`;
    },
  },
  {
    title: '操作人',
    key: 'operatorName',
    width: 140,
    render(row) {
      return row.operatorName ? `${row.operatorName} (#${row.operatorId})` : '-';
    },
  },
  {
    title: '备注',
    key: 'remark',
    width: 220,
  },
  {
    title: '操作时间',
    key: 'createdAt',
    width: 180,
  },
];
