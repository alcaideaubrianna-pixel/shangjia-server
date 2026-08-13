<template>
  <n-alert type="warning" title="发送范围" class="mb-4">
    仅发送给已绑定系统账号、且曾与所选 Bot 建立私聊的 Telegram 用户。同一 Bot 下按 Chat ID 去重。
  </n-alert>
  <n-space justify="end" class="mb-4">
    <n-button @click="openBroadcastRecords">推送记录</n-button>
  </n-space>
  <n-form ref="formRef" :model="form" :rules="rules" label-placement="top">
    <n-form-item label="发送 Bot" path="botIds">
      <n-select
        v-model:value="form.botIds"
        multiple
        filterable
        clearable
        :options="botOptions"
        placeholder="留空使用全部启用的官方 Bot"
      />
    </n-form-item>
    <n-form-item label="消息内容" path="text">
      <n-input
        v-model:value="form.text"
        type="textarea"
        show-count
        :maxlength="4096"
        :autosize="{ minRows: 8, maxRows: 16 }"
        placeholder="支持 Telegram HTML 格式"
      />
    </n-form-item>
    <n-form-item label="静默发送">
      <n-switch v-model:value="form.disableNotice" />
    </n-form-item>
    <n-popconfirm positive-text="确认推送" negative-text="取消" @positive-click="createBroadcast">
      <template #trigger>
        <n-button type="primary" :loading="submitting" :disabled="taskRunning"
          >创建推送任务</n-button
        >
      </template>
      消息将发送给所有符合条件的绑定用户，提交后不能撤回，确定继续吗？
    </n-popconfirm>
  </n-form>

  <n-card v-if="task" size="small" title="当前任务" class="mt-4">
    <n-descriptions :column="4" label-placement="top" bordered>
      <n-descriptions-item label="任务 ID">{{ task.id }}</n-descriptions-item>
      <n-descriptions-item label="状态">{{ statusLabel }}</n-descriptions-item>
      <n-descriptions-item label="收件人">{{ task.totalCount }}</n-descriptions-item>
      <n-descriptions-item label="成功">{{ task.successCount }}</n-descriptions-item>
      <n-descriptions-item label="失败">{{ task.failedCount }}</n-descriptions-item>
      <n-descriptions-item label="被屏蔽/不可达">{{ task.blockedCount }}</n-descriptions-item>
    </n-descriptions>
    <n-progress class="mt-4" type="line" :percentage="progress" :status="progressStatus" />
    <n-alert v-if="task.lastError" class="mt-4" type="error" title="最后一次失败原因">{{
      task.lastError
    }}</n-alert>
  </n-card>
</template>

<script setup lang="ts">
  import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue';
  import type { FormInst, FormRules } from 'naive-ui';
  import { useMessage } from 'naive-ui';
  import { useRouter } from 'vue-router';

  import { BotList, BroadcastCreate, BroadcastTask } from '@/api/addons/youbanBot';

  const message = useMessage();
  const router = useRouter();
  const formRef = ref<FormInst | null>(null);
  const submitting = ref(false);
  const botRows = ref<any[]>([]);
  const task = ref<any>(null);
  let pollTimer: ReturnType<typeof setTimeout> | undefined;
  const form = reactive({ botIds: [] as number[], text: '', disableNotice: false });
  const rules: FormRules = {
    text: [{ required: true, message: '请输入消息内容', trigger: ['input', 'blur'] }],
  };
  const botOptions = computed(() =>
    botRows.value
      .map((item) => ({
        label: item.botUsername ? `@${item.botUsername}` : item.botName,
        value: Number(item.id ?? item.Id),
      }))
      .filter((item) => Number.isInteger(item.value) && item.value > 0)
  );
  const taskRunning = computed(() => ['pending', 'running'].includes(task.value?.status));
  const statusLabel = computed(
    () =>
      ({ pending: '等待执行', running: '发送中', completed: '已完成', failed: '失败' })[
        task.value?.status
      ] || '-'
  );
  const progress = computed(() =>
    !task.value?.totalCount
      ? task.value?.status === 'completed'
        ? 100
        : 0
      : Math.min(
          100,
          Math.round(
            ((task.value.successCount + task.value.failedCount) / task.value.totalCount) * 100
          )
        )
  );
  const progressStatus = computed(() =>
    task.value?.status === 'failed'
      ? 'error'
      : task.value?.status === 'completed'
        ? 'success'
        : 'default'
  );

  async function createBroadcast() {
    await formRef.value?.validate();
    submitting.value = true;
    try {
      task.value = await BroadcastCreate(form);
      message.success('推送任务已创建');
      schedulePoll();
    } finally {
      submitting.value = false;
    }
  }

  function openBroadcastRecords() {
    router.push({ name: 'youbanBot', query: { view: 'broadcast-records' } });
  }

  function schedulePoll() {
    if (pollTimer) clearTimeout(pollTimer);
    if (!taskRunning.value) return;
    pollTimer = setTimeout(async () => {
      task.value = await BroadcastTask({ id: task.value.id });
      schedulePoll();
    }, 1500);
  }

  onMounted(async () => {
    const res: any = await BotList({ page: 1, perPage: 100, status: 1, isOfficial: 1 });
    botRows.value = res?.list || [];
  });
  onBeforeUnmount(() => pollTimer && clearTimeout(pollTimer));
</script>
