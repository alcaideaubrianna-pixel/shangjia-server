import { h, ref } from 'vue';
import { NTag } from 'naive-ui';
import { FormSchema } from '@/components/Form';
import { defRangeShortcuts } from '@/utils/dateUtil';

const yesNoOptions = [
  { label: '是', value: 1 },
  { label: '否', value: 2 },
];

export const schemas = ref<FormSchema[]>([
  {
    field: 'profileNo',
    component: 'NInput',
    label: '编号',
    componentProps: {
      placeholder: '资料编号',
      clearable: true,
    },
  },
  {
    field: 'keyword',
    component: 'NInput',
    label: '关键词',
    componentProps: {
      placeholder: '标题 / 摘要 / 正文',
      clearable: true,
    },
  },
  {
    field: 'sourceNoteId',
    component: 'NInputNumber',
    label: '来源ID',
    componentProps: {
      placeholder: 'note_id',
      clearable: true,
      showButton: false,
    },
  },
  {
    field: 'sourceChannelId',
    component: 'NInputNumber',
    label: '频道ID',
    componentProps: {
      placeholder: 'source_channel_id',
      clearable: true,
      showButton: false,
    },
  },
  {
    field: 'channelKeyword',
    component: 'NInput',
    label: '频道',
    componentProps: {
      placeholder: '标题 / 用户名 / chat id',
      clearable: true,
    },
  },
  {
    field: 'province',
    component: 'NInput',
    label: '省份',
    componentProps: {
      placeholder: '省份',
      clearable: true,
    },
  },
  {
    field: 'city',
    component: 'NInput',
    label: '城市',
    componentProps: {
      placeholder: '城市',
      clearable: true,
    },
  },
  {
    field: 'cupSize',
    component: 'NInput',
    label: '标签',
    componentProps: {
      placeholder: '罩杯 / 标签',
      clearable: true,
    },
  },
  {
    field: 'ageMin',
    component: 'NInputNumber',
    label: '年龄起',
    componentProps: {
      min: 0,
      showButton: false,
      clearable: true,
    },
  },
  {
    field: 'ageMax',
    component: 'NInputNumber',
    label: '年龄止',
    componentProps: {
      min: 0,
      showButton: false,
      clearable: true,
    },
  },
  {
    field: 'heightMin',
    component: 'NInputNumber',
    label: '身高起',
    componentProps: {
      min: 0,
      showButton: false,
      clearable: true,
    },
  },
  {
    field: 'heightMax',
    component: 'NInputNumber',
    label: '身高止',
    componentProps: {
      min: 0,
      showButton: false,
      clearable: true,
    },
  },
  {
    field: 'weightMin',
    component: 'NInputNumber',
    label: '体重起',
    componentProps: {
      min: 0,
      showButton: false,
      clearable: true,
    },
  },
  {
    field: 'weightMax',
    component: 'NInputNumber',
    label: '体重止',
    componentProps: {
      min: 0,
      showButton: false,
      clearable: true,
    },
  },
  {
    field: 'daysMin',
    component: 'NInputNumber',
    label: '陪伴起',
    componentProps: { min: 0, showButton: false, clearable: true },
  },
  {
    field: 'daysMax',
    component: 'NInputNumber',
    label: '陪伴止',
    componentProps: { min: 0, showButton: false, clearable: true },
  },
  {
    field: 'costMin',
    component: 'NInputNumber',
    label: '生活费起',
    componentProps: { min: 0, showButton: false, clearable: true },
  },
  {
    field: 'costMax',
    component: 'NInputNumber',
    label: '生活费止',
    componentProps: { min: 0, showButton: false, clearable: true },
  },
  { field: 'canFly', component: 'NSelect', label: '可飞', componentProps: { options: yesNoOptions, placeholder: '请选择', clearable: true } },
  { field: 'canGoAbroad', component: 'NSelect', label: '出国', componentProps: { options: yesNoOptions, placeholder: '请选择', clearable: true } },
  { field: 'canOvernight', component: 'NSelect', label: '过夜', componentProps: { options: yesNoOptions, placeholder: '请选择', clearable: true } },
  { field: 'canCohabitate', component: 'NSelect', label: '同居', componentProps: { options: yesNoOptions, placeholder: '请选择', clearable: true } },
  { field: 'hasHealthCheck', component: 'NSelect', label: '体检', componentProps: { options: yesNoOptions, placeholder: '请选择', clearable: true } },
  { field: 'isFullMonth', component: 'NSelect', label: '满月', componentProps: { options: yesNoOptions, placeholder: '请选择', clearable: true } },
  { field: 'isVirgin', component: 'NSelect', label: '是否处', componentProps: { options: yesNoOptions, placeholder: '请选择', clearable: true } },
  { field: 'acceptSm', component: 'NSelect', label: 'SM', componentProps: { options: yesNoOptions, placeholder: '请选择', clearable: true } },
  { field: 'noCondom', component: 'NSelect', label: '无套', componentProps: { options: yesNoOptions, placeholder: '请选择', clearable: true } },
  { field: 'allowCreampie', component: 'NSelect', label: '内射', componentProps: { options: yesNoOptions, placeholder: '请选择', clearable: true } },
  { field: 'hasTattoo', component: 'NSelect', label: '纹身', componentProps: { options: yesNoOptions, placeholder: '请选择', clearable: true } },
  { field: 'isFavorite', component: 'NSelect', label: '收藏', componentProps: { options: yesNoOptions, placeholder: '请选择', clearable: true } },
  { field: 'homeRecommend', component: 'NSelect', label: '首页推荐', componentProps: { options: yesNoOptions, placeholder: '请选择', clearable: true } },
  {
    field: 'visibility',
    component: 'NSelect',
    label: '可见性',
    componentProps: {
      options: [
        { label: '私有', value: 'private' },
        { label: '公开', value: 'public' },
        { label: '会员', value: 'member_only' },
      ],
      placeholder: '请选择',
      clearable: true,
    },
  },
  {
    field: 'reviewStatus',
    component: 'NSelect',
    label: '审核',
    componentProps: {
      options: [
        { label: '待审核', value: 'pending' },
        { label: '已通过', value: 'approved' },
        { label: '已拒绝', value: 'rejected' },
      ],
      placeholder: '请选择',
      clearable: true,
    },
  },
  {
    field: 'importStatus',
    component: 'NSelect',
    label: '导入',
    componentProps: {
      options: [
        { label: '已导入', value: 'imported' },
        { label: '重复', value: 'duplicate' },
      ],
      placeholder: '请选择',
      clearable: true,
    },
  },
  {
    field: 'hasVideo',
    component: 'NSelect',
    label: '视频',
    componentProps: {
      options: [
        { label: '有视频', value: 1 },
        { label: '无视频', value: 2 },
      ],
      placeholder: '请选择',
      clearable: true,
    },
  },
  {
    field: 'hasVerification',
    component: 'NSelect',
    label: '验证',
    componentProps: {
      options: [
        { label: '有验证', value: 1 },
        { label: '无验证', value: 2 },
      ],
      placeholder: '请选择',
      clearable: true,
    },
  },
  {
    field: 'memberOnlyVideo',
    component: 'NSelect',
    label: '会员视频',
    componentProps: {
      options: [
        { label: '会员可见', value: 1 },
        { label: '非会员', value: 2 },
      ],
      placeholder: '请选择',
      clearable: true,
    },
  },
  {
    field: 'status',
    component: 'NSelect',
    label: '状态',
    componentProps: {
      options: [
        { label: '正常', value: 1 },
        { label: '冻结', value: 2 },
      ],
      placeholder: '请选择',
      clearable: true,
    },
  },
  {
    field: 'hasDuplicate',
    component: 'NSelect',
    label: '重复',
    componentProps: {
      options: [
        { label: '重复', value: 1 },
        { label: '非重复', value: 2 },
      ],
      placeholder: '请选择',
      clearable: true,
    },
  },
  {
    field: 'createdAt',
    component: 'NDatePicker',
    label: '导入时间',
    componentProps: {
      type: 'datetimerange',
      clearable: true,
      shortcuts: defRangeShortcuts(),
    },
  },
]);

function tag(type: 'default' | 'success' | 'warning' | 'error' | 'info', label: string) {
  return h(NTag, { type, bordered: false }, { default: () => label });
}

export function renderVisibility(value: string) {
  const map = {
    private: { type: 'default', label: '私有' },
    public: { type: 'success', label: '公开' },
    member_only: { type: 'warning', label: '会员' },
  };
  const item = map[value] || { type: 'default', label: value || '-' };
  return tag(item.type as any, item.label);
}

export function renderReview(value: string) {
  const map = {
    pending: { type: 'warning', label: '待审核' },
    approved: { type: 'success', label: '已通过' },
    rejected: { type: 'error', label: '已拒绝' },
  };
  const item = map[value] || { type: 'default', label: value || '-' };
  return tag(item.type as any, item.label);
}

export function renderImport(value: string) {
  const map = {
    imported: { type: 'success', label: '已导入' },
    duplicate: { type: 'warning', label: '重复' },
  };
  const item = map[value] || { type: 'default', label: value || '-' };
  return tag(item.type as any, item.label);
}

export const columns = [
  {
    title: 'ID',
    key: 'id',
    width: 80,
  },
  {
    title: '编号',
    key: 'profileNo',
    width: 130,
  },
  {
    title: '标题',
    key: 'title',
    width: 220,
    ellipsis: { tooltip: true },
  },
  {
    title: '地区',
    key: 'city',
    width: 120,
    render(row) {
      return [row.province, row.city].filter(Boolean).join(' / ') || '-';
    },
  },
  {
    title: '频道',
    key: 'channelTitle',
    width: 220,
    ellipsis: { tooltip: true },
    render(row) {
      return row.channelTitle || row.channelUsername || row.sourceChannelId || '-';
    },
  },
  {
    title: '来源ID',
    key: 'sourceNoteId',
    width: 130,
  },
  {
    title: '消息ID',
    key: 'sourceMessageId',
    width: 130,
  },
  {
    title: '图片',
    key: 'imageCount',
    width: 80,
  },
  {
    title: '视频',
    key: 'videoCount',
    width: 80,
  },
  {
    title: '可见性',
    key: 'visibility',
    width: 90,
    render(row) {
      return renderVisibility(row.visibility);
    },
  },
  {
    title: '审核',
    key: 'reviewStatus',
    width: 100,
    render(row) {
      return renderReview(row.reviewStatus);
    },
  },
  {
    title: '导入',
    key: 'importStatus',
    width: 100,
    render(row) {
      return renderImport(row.importStatus);
    },
  },
  {
    title: '首页推荐',
    key: 'homeRecommend',
    width: 100,
    render(row) {
      return row.homeRecommend === 1 ? tag('success', `推荐 ${row.homeSort || 0}`) : tag('default', '否');
    },
  },
  {
    title: '重复ID',
    key: 'duplicateOfId',
    width: 100,
    render(row) {
      return row.duplicateOfId || '-';
    },
  },
  {
    title: '导入时间',
    key: 'createdAt',
    width: 180,
  },
];
