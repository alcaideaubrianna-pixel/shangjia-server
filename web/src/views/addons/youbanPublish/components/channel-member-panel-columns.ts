import { h } from 'vue';
import { NButton, NTag } from 'naive-ui';

type TagType = 'default' | 'success' | 'info' | 'warning' | 'error';

interface ChannelColumnActions {
  displayTypeLabel: (displayType: string, row: Recordable) => string;
  exportMembers: (row?: Recordable) => void;
  openMembers: (row: Recordable) => void;
  roleLabel: (role: string) => string;
  syncMembers: (row: Recordable | null) => void;
  tgAccountLabel: (id: number) => string;
}

interface MemberColumnActions {
  roleLabel: (role: string) => string;
}

export function createChannelColumns(actions: ChannelColumnActions) {
  return [
    { title: 'ID', key: 'id', width: 80 },
    {
      title: 'TG账号',
      key: 'tgAccountId',
      width: 170,
      render: (row: Recordable) => actions.tgAccountLabel(row.tgAccountId),
    },
    {
      title: '频道名称',
      key: 'channelTitle',
      width: 220,
      render: (row: Recordable) => row.channelTitle || '-',
    },
    {
      title: '频道用户名',
      key: 'channelUsername',
      width: 180,
      render: (row: Recordable) => row.channelUsername || '-',
    },
    {
      title: 'Chat ID',
      key: 'channelId',
      width: 220,
      render: (row: Recordable) => row.channelId || '-',
    },
    {
      title: '类型',
      key: 'displayType',
      width: 100,
      render: (row: Recordable) =>
        renderMiniTag(actions.displayTypeLabel(row.displayType, row), 'info'),
    },
    {
      title: '管理权限',
      key: 'managementRoleText',
      width: 120,
      render: (row: Recordable) =>
        renderMiniTag(row.managementRoleText || actions.roleLabel(row.managementRole), 'success'),
    },
    {
      title: '邀请权限',
      key: 'canInviteUsers',
      width: 100,
      render: (row: Recordable) => (row.canInviteUsers === 1 ? '是' : '否'),
    },
    {
      title: '发消息',
      key: 'canPostMessages',
      width: 100,
      render: (row: Recordable) => (row.canPostMessages === 1 ? '是' : '否'),
    },
    {
      title: '最后同步',
      key: 'lastSyncAt',
      width: 170,
      render: (row: Recordable) => row.lastSyncAt || '-',
    },
    {
      title: '操作',
      key: 'actions',
      width: 260,
      fixed: 'right',
      render(row: Recordable) {
        return h('div', { class: 'action-row' }, [
          actionButton('查看成员', () => actions.openMembers(row)),
          actionButton('同步成员', () => actions.syncMembers(row)),
          actionButton('导出', () => actions.exportMembers(row)),
        ]);
      },
    },
  ];
}

export function createMemberColumns(actions: MemberColumnActions) {
  return [
    { title: 'ID', key: 'id', width: 80 },
    {
      title: '显示名称',
      key: 'displayName',
      width: 180,
      render: (row: Recordable) => row.displayName || '-',
    },
    {
      title: '用户名',
      key: 'username',
      width: 180,
      render: (row: Recordable) => row.username || '-',
    },
    {
      title: '用户ID',
      key: 'userId',
      width: 160,
      render: (row: Recordable) => row.userId || '-',
    },
    {
      title: '角色',
      key: 'participantRoleText',
      width: 110,
      render: (row: Recordable) =>
        renderMiniTag(row.participantRoleText || actions.roleLabel(row.participantRole), 'default'),
    },
    {
      title: '状态',
      key: 'status',
      width: 90,
      render: (row: Recordable) =>
        renderMiniTag(row.status === 1 ? '有效' : '失效', row.status === 1 ? 'success' : 'warning'),
    },
    { title: '手机号', key: 'phone', width: 150, render: (row: Recordable) => row.phone || '-' },
    {
      title: '最后同步',
      key: 'lastSyncedAt',
      width: 170,
      render: (row: Recordable) => row.lastSyncedAt || '-',
    },
    {
      title: '更新时间',
      key: 'updatedAt',
      width: 170,
      render: (row: Recordable) => row.updatedAt || '-',
    },
  ];
}

function renderMiniTag(label: string, type: TagType) {
  return h(NTag, { bordered: false, size: 'small', type }, { default: () => label || '-' });
}

function actionButton(label: string, onClick: () => void) {
  return h(
    NButton,
    { size: 'small', quaternary: true, type: 'primary', onClick },
    { default: () => label }
  );
}
