<template>
  <div>
    <n-drawer v-model:show="showModal" :width="dialogWidth">
      <n-drawer-content title="选项树表详情" closable>
        <n-spin :show="loading" description="请稍候...">
          <n-descriptions label-placement="left" class="py-2" column="1">
            <n-descriptions-item>
              <template #label>
                标题
              </template>
              {{ formValue.title }}
            </n-descriptions-item>
            <n-descriptions-item>
              <template #label>
                上级
              </template>
              {{ formValue.pid }}
            </n-descriptions-item>
            <n-descriptions-item label="测试分类">
              <n-tag :type="getOptionTag(options.testCategoryOption, formValue?.categoryId)" size="small" class="min-left-space">
                {{ getOptionLabel(options.testCategoryOption, formValue?.categoryId) }}
              </n-tag>
            </n-descriptions-item>
            <n-descriptions-item>
              <template #label>
                描述
              </template>
              {{ formValue.description }}
            </n-descriptions-item>
            <n-descriptions-item>
              <template #label>
                排序
              </template>
              {{ formValue.sort }}
            </n-descriptions-item>
            <n-descriptions-item label="状态">
              <n-tag :type="getOptionTag(options.sys_normal_disable, formValue?.status)" size="small" class="min-left-space">
                {{ getOptionLabel(options.sys_normal_disable, formValue?.status) }}
              </n-tag>
            </n-descriptions-item>
          </n-descriptions>
        </n-spin>
      </n-drawer-content>
    </n-drawer>
  </div>
</template>

<script lang="ts" setup>
  import { computed, ref } from 'vue';
  import { useMessage } from 'naive-ui';
  import { View } from '@/api/optionTreeDemo';
  import { State, newState, options } from './model';
  import { adaModalWidth, getOptionLabel, getOptionTag } from '@/utils/hotgo';
  import { getFileExt } from '@/utils/urlUtils';

  const message = useMessage();
  const loading = ref(false);
  const showModal = ref(false);
  const formValue = ref(newState(null));
  const dialogWidth = computed(() => {
    return adaModalWidth(580);
  });
  const fileAvatarCSS = computed(() => {
    return {
      '--n-merged-size': `var(--n-avatar-size-override, 80px)`,
      '--n-font-size': `18px`,
    };
  });

  //下载
  function download(url: string) {
    window.open(url);
  }

  function openModal(state: State) {
    showModal.value = true;
    loading.value = true;
    View({ id: state.id })
      .then((res) => {
        formValue.value = res;
      })
      .finally(() => {
        loading.value = false;
      });
  }

  defineExpose({
    openModal,
  });
</script>

<style lang="less" scoped></style>