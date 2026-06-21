import { h, ref } from 'vue';
import { NButton, NTag } from 'naive-ui';
import { FormSchema } from '@/components/Form';

export const schemas = ref<FormSchema[]>([
  {
    field: 'keyword',
    component: 'NInput',
    label: '关键词',
    componentProps: {
      placeholder: '会员 / 手机 / 资料 / session',
      clearable: true,
    },
  },
  {
    field: 'memberId',
    component: 'NInputNumber',
    label: '会员ID',
    componentProps: {
      min: 1,
      showButton: false,
      clearable: true,
    },
  },
  {
    field: 'profileId',
    component: 'NInputNumber',
    label: '资料ID',
    componentProps: {
      min: 1,
      showButton: false,
      clearable: true,
    },
  },
  {
    field: 'status',
    component: 'NSelect',
    label: '状态',
    componentProps: {
      options: [
        { label: '已打开', value: 'opened' },
        { label: '已关闭', value: 'closed' },
      ],
      placeholder: '请选择状态',
      clearable: true,
    },
  },
  {
    field: 'hasTgTopic',
    component: 'NSelect',
    label: 'TG话题',
    componentProps: {
      options: [
        { label: '已关联', value: 1 },
        { label: '未关联', value: 2 },
      ],
      placeholder: '请选择',
      clearable: true,
    },
  },
]);

export function renderStatus(status: string) {
  const map: Record<string, { type: 'default' | 'success' | 'warning'; label: string }> = {
    opened: { type: 'success', label: '已打开' },
    closed: { type: 'default', label: '已关闭' },
  };
  const item = map[status] || { type: 'warning', label: status || '-' };
  return h(NTag, { type: item.type, bordered: false }, { default: () => item.label });
}

export function createColumns(openDetail: (row: Recordable) => void) {
  return [
    { title: 'ID', key: 'id', width: 80 },
    {
      title: '会员',
      key: 'member',
      width: 190,
      render(row) {
        const name = row.memberRealName || row.memberUsername || row.memberMobile || `#${row.memberId}`;
        return h('div', { class: 'cell-stack' }, [
          h('span', name),
          h('small', `ID ${row.memberId}${row.memberMobile ? ` / ${row.memberMobile}` : ''}`),
        ]);
      },
    },
    {
      title: '资料',
      key: 'profile',
      width: 190,
      render(row) {
        return h('div', { class: 'cell-stack' }, [
          h('span', row.profileNo || `#${row.profileId}`),
          h('small', row.profileTitle || [row.province, row.city].filter(Boolean).join(' ') || '-'),
        ]);
      },
    },
    {
      title: '会话标识',
      key: 'chatSessionId',
      width: 220,
      ellipsis: { tooltip: true },
    },
    {
      title: 'TG话题',
      key: 'tgMessageThreadId',
      width: 120,
      render(row) {
        return row.tgMessageThreadId
          ? h(NTag, { type: 'success', bordered: false }, { default: () => String(row.tgMessageThreadId) })
          : h(NTag, { type: 'default', bordered: false }, { default: () => '未关联' });
      },
    },
    {
      title: '最后消息',
      key: 'lastMessage',
      width: 260,
      ellipsis: { tooltip: true },
    },
    { title: '未读', key: 'unreadCount', width: 80 },
    { title: '消息数', key: 'messageCount', width: 90 },
    {
      title: '状态',
      key: 'status',
      width: 100,
      render(row) {
        return renderStatus(row.status);
      },
    },
    { title: '更新时间', key: 'updatedAt', width: 180 },
    {
      title: '操作',
      key: 'actions',
      width: 100,
      fixed: 'right',
      render(row) {
        return h(
          NButton,
          {
            size: 'small',
            type: 'primary',
            onClick: () => openDetail(row),
          },
          { default: () => '详情' }
        );
      },
    },
  ];
}
