<template>
  <n-modal
    :show="show"
    preset="dialog"
    title="成员同步进度"
    :mask-closable="false"
    :style="{ width: '720px', maxWidth: '94vw' }"
    @update:show="emit('update:show', $event)"
  >
    <n-space vertical>
      <n-alert type="info" :bordered="false">{{ syncTitle }}</n-alert>
      <n-progress :percentage="syncPercent" :show-indicator="true" />
      <n-descriptions :column="2" bordered size="small">
        <n-descriptions-item label="状态">{{ task.statusText || '-' }}</n-descriptions-item>
        <n-descriptions-item label="阶段">{{ task.stageText || '-' }}</n-descriptions-item>
        <n-descriptions-item label="总进度">
          {{ task.progressDone || 0 }} / {{ task.progressTotal || 0 }}
        </n-descriptions-item>
        <n-descriptions-item label="管理员">
          {{ task.adminDone || 0 }} / {{ task.adminTotal || 0 }}
        </n-descriptions-item>
        <n-descriptions-item label="成员">
          {{ task.memberDone || 0 }} / {{ task.memberTotal || 0 }}
        </n-descriptions-item>
        <n-descriptions-item label="写入数量">{{ task.upsertedCount || 0 }}</n-descriptions-item>
        <n-descriptions-item label="失效数量">{{ task.removedCount || 0 }}</n-descriptions-item>
        <n-descriptions-item label="错误信息" :span="2">
          {{ task.errorMessage || '-' }}
        </n-descriptions-item>
      </n-descriptions>
    </n-space>
    <template #action>
      <n-space justify="end">
        <n-button @click="emit('update:show', false)">关闭</n-button>
        <n-button
          v-if="canCancelSync"
          :loading="cancelLoading"
          type="warning"
          @click="emit('cancel')"
        >
          取消同步
        </n-button>
      </n-space>
    </template>
  </n-modal>
</template>
<script lang="ts" setup>
  import { computed } from 'vue';

  const props = defineProps<{
    cancelLoading: boolean;
    show: boolean;
    task: Recordable;
  }>();
  const emit = defineEmits<{
    cancel: [];
    'update:show': [value: boolean];
  }>();

  const syncTitle = computed(() =>
    props.task.channelTitle
      ? `${props.task.channelTitle}（${props.task.channelUsername || props.task.channelId || '-'}）`
      : '成员同步任务'
  );
  const syncPercent = computed(() => Math.max(0, Math.min(100, Number(props.task.progress || 0))));
  const canCancelSync = computed(
    () => props.show && ['pending', 'running'].includes(props.task.status || '')
  );
</script>
