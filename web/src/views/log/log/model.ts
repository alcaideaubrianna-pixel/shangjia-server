import { FormSchema } from '@/components/Form';
import { ref } from 'vue';
import { defRangeShortcuts } from '@/utils/dateUtil';
import { Option } from '@/utils/hotgo';
import { Dicts } from '@/api/dict/dict';

export const schemas = ref<FormSchema[]>([
  {
    field: 'reqId',
    component: 'NInput',
    label: '链路ID',
    componentProps: {
      placeholder: '请输入链路ID',
      onInput: (e: any) => {
        console.log(e);
      },
    },
  },
  {
    field: 'memberId',
    component: 'NInput',
    label: '操作人',
    componentProps: {
      placeholder: '请输入操作人ID',
      onInput: (e: any) => {
        console.log(e);
      },
    },
    rules: [{ trigger: ['blur'] }],
  },
  {
    field: 'url',
    component: 'NInput',
    label: '接口路径',
    componentProps: {
      placeholder: '请输入接口路径',
      onInput: (e: any) => {
        console.log(e);
      },
    },
  },
  {
    field: 'ip',
    component: 'NInput',
    label: '访问IP',
    componentProps: {
      placeholder: '请输入IP地址',
      onInput: (e: any) => {
        console.log(e);
      },
    },
  },
  {
    field: 'method',
    component: 'NSelect',
    label: '请求方式',
    componentProps: {
      placeholder: '请选择请求方式',
      options: [],
      onUpdateValue: (e: any) => {
        console.log(e);
      },
    },
  },
  {
    field: 'createdAt',
    component: 'NDatePicker',
    label: '访问时间',
    componentProps: {
      type: 'datetimerange',
      clearable: true,
      shortcuts: defRangeShortcuts(),
      onUpdateValue: (e: any) => {
        console.log(e);
      },
    },
  },
  {
    field: 'takeUpTime',
    component: 'NSelect',
    label: '请求耗时',
    componentProps: {
      placeholder: '请选择请求耗时',
      options: [],
      onUpdateValue: (e: any) => {
        console.log(e);
      },
    },
  },
  {
    field: 'errorCode',
    component: 'NSelect',
    label: '状态码',
    labelMessage: '支持填入自定义状态码',
    componentProps: {
      placeholder: '请选择状态码',
      options: [],
      filterable: true,
      tag: true,
      onUpdateValue: (e: any) => {
        console.log(e);
      },
    },
  },
]);

// 字典数据选项
export const options = ref({
  HTTPMethod: [] as Option[],
  HTTPHandlerTime: [] as Option[],
  HTTPApiCode: [] as Option[],
});

// 加载字典数据选项
export function loadOptions() {
  Dicts({
    types: ['HTTPMethod', 'HTTPHandlerTime', 'HTTPApiCode'],
  }).then((res) => {
    options.value = res;
    for (const item of schemas.value) {
      switch (item.field) {
        case 'method':
          item.componentProps.options = options.value.HTTPMethod;
          break;
        case 'takeUpTime':
          item.componentProps.options = options.value.HTTPHandlerTime;
          break;
        case 'errorCode':
          item.componentProps.options = options.value.HTTPApiCode;
          break;
      }
    }
  });
}
