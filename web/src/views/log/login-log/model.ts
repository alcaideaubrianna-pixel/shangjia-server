import { h, ref } from 'vue';
import { NTag } from 'naive-ui';
import { cloneDeep } from 'lodash-es';
import { FormSchema } from '@/components/Form';
import { isNullObject } from '@/utils/is';
import { defRangeShortcuts } from '@/utils/dateUtil';
import { getOptionLabel, getOptionTag, Option } from '@/utils/hotgo';
import { Dicts } from '@/api/dict/dict';

export interface State {
  id: number;
  reqId: string;
  memberId: number;
  username: string;
  response: any;
  loginAt: number;
  errMsg: string;
  status: number;
  createdAt: string;
  updatedAt: string;
}

export const defaultState = {
  id: 0,
  reqId: '',
  memberId: 0,
  username: '',
  response: null,
  loginAt: 0,
  errMsg: '',
  status: 1,
  createdAt: '',
  updatedAt: '',
};

export function newState(state: State | null): State {
  if (state !== null) {
    return cloneDeep(state);
  }
  return cloneDeep(defaultState);
}

export const rules = {};

export const schemas = ref<FormSchema[]>([
  {
    field: 'username',
    component: 'NInput',
    label: '用户名',
    componentProps: {
      placeholder: '请输入用户名',
      onUpdateValue: (e: any) => {
        console.log(e);
      },
    },
  },
  {
    field: 'loginIp',
    component: 'NInput',
    label: 'IP地址',
    componentProps: {
      placeholder: '请输入IP地址',
      onUpdateValue: (e: any) => {
        console.log(e);
      },
    },
  },
  {
    field: 'status',
    component: 'NSelect',
    label: '状态',
    defaultValue: null,
    componentProps: {
      placeholder: '请选择状态',
      options: [],
      onUpdateValue: (e: any) => {
        console.log(e);
      },
    },
  },
  {
    field: 'loginAt',
    component: 'NDatePicker',
    label: '登录时间',
    componentProps: {
      type: 'datetimerange',
      clearable: true,
      shortcuts: defRangeShortcuts(),
      onUpdateValue: (e: any) => {
        console.log(e);
      },
    },
  },
]);

export const columns = [
  {
    title: '记录ID',
    key: 'id',
    width: 80,
  },
  {
    title: '用户名',
    key: 'username',
    width: 120,
    render(row) {
      return h(
        NTag,
        {
          style: {
            marginRight: '6px',
          },
          type: 'info',
          bordered: false,
        },
        {
          default: () => row.username,
        }
      );
    },
  },
  {
    title: '登录IP',
    key: 'loginIp',
    width: 160,
  },
  {
    title: 'IP归属地',
    key: 'cityLabel',
    width: 200,
  },
  {
    title: '浏览器',
    key: 'browser',
    width: 200,
  },
  {
    title: '操作系统',
    key: 'os',
    width: 150,
  },
  {
    title: '状态',
    key: 'status',
    render(row) {
      if (isNullObject(row.status)) {
        return ``;
      }
      return h(
        NTag,
        {
          style: {
            marginRight: '6px',
          },
          type: getOptionTag(options.value.sys_login_status, row.status),
          bordered: false,
        },
        {
          default: () => getOptionLabel(options.value.sys_login_status, row.status),
        }
      );
    },
    width: 150,
  },
  {
    title: '提示信息',
    key: 'errMsg',
    render(row) {
      if (row.errMsg !== '') {
        return row.errMsg;
      }

      if (row.status === 1) {
        return '登录成功';
      }
      return ``;
    },
    width: 200,
  },
  {
    title: '登录时间',
    key: 'loginAt',
    width: 180,
  },
];

// 字典数据选项
export const options = ref({
  sys_login_status: [] as Option[],
});

// 加载字典数据选项
export function loadOptions() {
  Dicts({
    types: ['sys_login_status'],
  }).then((res) => {
    options.value = res;
    for (const item of schemas.value) {
      switch (item.field) {
        case 'status':
          item.componentProps.options = options.value.sys_login_status;
          break;
      }
    }
  });
}
