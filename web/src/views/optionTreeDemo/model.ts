import { h, ref } from 'vue';
import { NTag } from 'naive-ui';
import { cloneDeep } from 'lodash-es';
import { FormSchema } from '@/components/Form';
import { Dicts } from '@/api/dict/dict';
import { isNullObject } from '@/utils/is';
import { defRangeShortcuts } from '@/utils/dateUtil';
import { Option, getOptionLabel, getOptionTag } from '@/utils/hotgo';
import { renderPopoverMemberSumma, MemberSumma } from '@/utils';
import { TreeOption } from '@/api/optionTreeDemo';

export class State {
  public title = ''; // 标题
  public id = 0; // ID
  public pid = 0; // 上级
  public level = 1; // 关系树级别
  public tree = ''; // 关系树
  public categoryId = null; // 测试分类
  public description = ''; // 描述
  public sort = 0; // 排序
  public status = 1; // 状态
  public createdBy = 0; // 创建者
  public createdBySumma?: null | MemberSumma = null; // 创建者摘要信息
  public updatedBy = 0; // 更新者
  public createdAt = ''; // 创建时间
  public updatedAt = ''; // 修改时间
  public deletedAt = ''; // 删除时间

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
  title: {
    required: true,
    trigger: ['blur', 'input'],
    type: 'string',
    message: '请输入标题',
  },
};

// 表格搜索表单
export const schemas = ref<FormSchema[]>([
  {
    field: 'title',
    component: 'NInput',
    label: '标题',
    componentProps: {
      placeholder: '请输入标题',
      onUpdateValue: (e: any) => {
        console.log(e);
      },
    },
  },
  {
    field: 'categoryId',
    component: 'NSelect',
    label: '测试分类',
    defaultValue: null,
    componentProps: {
      placeholder: '请选择测试分类',
      options: [],
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
    title: '标题',
    key: 'title',
    align: 'left',
    width: 100,
  },
  {
    title: '测试分类',
    key: 'categoryId',
    align: 'left',
    width: 100,
    render(row) {
      if (isNullObject(row.categoryId)) {
        return ``;
      }
      return h(
        NTag,
        {
          style: {
            marginRight: '6px',
          },
          type: getOptionTag(options.value.testCategoryOption, row.categoryId),
          bordered: false,
        },
        {
          default: () => getOptionLabel(options.value.testCategoryOption, row.categoryId),
        }
      );
    },
  },
  {
    title: '状态',
    key: 'status',
    align: 'left',
    width: 150,
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
          type: getOptionTag(options.value.sys_normal_disable, row.status),
          bordered: false,
        },
        {
          default: () => getOptionLabel(options.value.sys_normal_disable, row.status),
        }
      );
    },
  },
  {
    title: '创建者',
    key: 'createdBy',
    align: 'left',
    width: 100,
    render(row) {
      return renderPopoverMemberSumma(row.createdBySumma);
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
  sys_normal_disable: [] as Option[],
  testCategoryOption: [] as Option[],
});

// 加载字典数据选项
export function loadOptions() {
  Dicts({
    types: ['sys_normal_disable', 'testCategoryOption'],
  }).then((res) => {
    options.value = res;
    for (const item of schemas.value) {
      switch (item.field) {
        case 'status':
          item.componentProps.options = options.value.sys_normal_disable;
          break;
        case 'categoryId':
          item.componentProps.options = options.value.testCategoryOption;
          break;
      }
    }
  });
}

// 关系树选项
export const treeOption = ref([]);

// 加载关系树选项
export function loadTreeOption() {
  TreeOption().then((res) => {
    treeOption.value = res;
  });
}