import { h, ref } from 'vue';
import { NTag } from 'naive-ui';
import { cloneDeep } from 'lodash-es';
import { FormSchema } from '@/components/Form';
import { Dicts } from '@/api/dict/dict';
import { isNullObject } from '@/utils/is';
import { defRangeShortcuts } from '@/utils/dateUtil';
import { Option, getOptionLabel, getOptionTag } from '@/utils/hotgo';
import { useUserStore } from '@/store/modules/user';

const userStore = useUserStore();

export class State {
  public id = 0; // 主键
  public tenantId = null; // 租户ID
  public merchantId = null; // 商户ID
  public userId = null; // 用户ID
  public productName = ''; // 购买产品
  public orderSn = ''; // 关联订单号
  public money = null; // 充值金额
  public remark = ''; // 备注
  public status = 1; // 订单状态
  public createdAt = ''; // 创建时间
  public updatedAt = ''; // 修改时间

  constructor(state?: Partial<State>) {
    if (state) {
      Object.assign(this, state);
    }
  }
}

export function newState(state: State | Record<string, any> | null): State {
  if (state !== null) {
    if (state instanceof State) {
      return cloneDeep(state);
    }
    return new State(state);
  }
  return new State();
}

// 表单验证规则
export const rules = {
  money: {
    required: true,
    trigger: ['blur', 'input'],
    type: 'number',
    message: '请输入充值金额',
  },
};

// 表格搜索表单
export const schemas = ref<FormSchema[]>([
  {
    field: 'tenantId',
    component: 'NInput',
    label: '租户ID',
    componentProps: {
      placeholder: '请输入租户ID',
      onUpdateValue: (e: any) => {
        console.log(e);
      },
    },
    ifShow: () => {
      return userStore.isCompanyDept;
    },
  },
  {
    field: 'merchantId',
    component: 'NInput',
    label: '商户ID',
    componentProps: {
      placeholder: '请输入商户ID',
      onUpdateValue: (e: any) => {
        console.log(e);
      },
    },
    ifShow: () => {
      return userStore.isCompanyDept || userStore.isTenantDept;
    },
  },
  {
    field: 'userId',
    component: 'NInput',
    label: '用户ID',
    componentProps: {
      placeholder: '请输入用户ID',
      onUpdateValue: (e: any) => {
        console.log(e);
      },
    },
    ifShow: () => {
      return userStore.isCompanyDept || userStore.isTenantDept || userStore.isMerchantDept;
    },
  },
  {
    field: 'orderSn',
    component: 'NInput',
    label: '订单号',
    componentProps: {
      placeholder: '请输入订单号',
      onUpdateValue: (e: any) => {
        console.log(e);
      },
    },
  },
  {
    field: 'status',
    component: 'NSelect',
    label: '订单状态',
    defaultValue: null,
    componentProps: {
      placeholder: '请选择订单状态',
      options: [],
      onUpdateValue: (e: any) => {
        console.log(e);
      },
    },
  },
  {
    field: 'createdAt',
    component: 'NDatePicker',
    label: '创建时间',
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

// 表格列
export const columns = [
  {
    title: '订单ID',
    key: 'id',
    align: 'left',
    width: 100,
  },
  {
    title: '租户ID',
    key: 'tenantId',
    align: 'left',
    width: 100,
    ifShow: () => {
      return userStore.isCompanyDept;
    },
  },
  {
    title: '商户ID',
    key: 'merchantId',
    align: 'left',
    width: 100,
    ifShow: () => {
      return userStore.isCompanyDept || userStore.isTenantDept;
    },
  },
  {
    title: '用户ID',
    key: 'userId',
    align: 'left',
    width: 100,
    ifShow: () => {
      return userStore.isCompanyDept || userStore.isTenantDept || userStore.isMerchantDept;
    },
  },
  {
    title: '购买产品',
    key: 'productName',
    align: 'left',
    width: 150,
  },
  {
    title: '订单号',
    key: 'orderSn',
    align: 'left',
    width: 200,
  },
  {
    title: '充值金额',
    key: 'money',
    align: 'left',
    width: 100,
    render(row) {
      return row.money + ' 元';
    },
  },
  {
    title: '订单状态',
    key: 'status',
    align: 'left',
    width: 100,
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
          type: getOptionTag(options.value.payStatus, row.status),
          bordered: false,
        },
        {
          default: () => getOptionLabel(options.value.payStatus, row.status),
        }
      );
    },
  },
  {
    title: '创建时间',
    key: 'createdAt',
    align: 'left',
    width: 180,
  },
];

// 字典数据选项
export const options = ref({
  payStatus: [] as Option[],
});

// 加载字典数据选项
export function loadOptions() {
  Dicts({
    types: ['payStatus'],
  }).then((res) => {
    options.value = res;
    for (const item of schemas.value) {
      switch (item.field) {
        case 'status':
          item.componentProps.options = options.value.payStatus;
          break;
      }
    }
  });
}
