<template>
  <n-modal
    v-model:show="showModal"
    :show-icon="false"
    preset="dialog"
    :title="`${botKey} 用户管理`"
    :style="{ width: '1180px' }"
  >
    <n-space vertical>
      <n-space>
        <n-input v-model:value="query.keyword" clearable placeholder="搜索用户名/昵称" />
        <n-select
          v-model:value="query.memberLevel"
          :options="memberOptions"
          clearable
          placeholder="会员等级"
          class="w-160px"
        />
        <n-select
          v-model:value="query.status"
          :options="statusOptions"
          clearable
          placeholder="状态"
          class="w-140px"
        />
        <n-button type="primary" @click="load">查询</n-button>
      </n-space>
      <n-data-table :columns="columns" :data="users" :loading="loading" :pagination="false" />
    </n-space>
  </n-modal>
</template>

<script lang="ts" setup>
  import { h, reactive, ref } from 'vue';
  import { format } from 'date-fns';
  import { NButton, NDatePicker, NInputNumber, NSelect, useMessage } from 'naive-ui';
  import { botUsers, updateBotUser } from '@/api/addons/lazysheep_tggo/config';

  const message = useMessage();
  const showModal = ref(false);
  const loading = ref(false);
  const botKey = ref('');
  const users = ref<any[]>([]);
  const query = reactive({
    keyword: '',
    memberLevel: null,
    status: null,
  });
  const memberOptions = [
    { label: '普通用户', value: 0 },
    { label: '会员', value: 1 },
    { label: '高级会员', value: 2 },
  ];
  const statusOptions = [
    { label: '正常', value: 1 },
    { label: '禁用', value: 2 },
  ];

  const columns = [
    { title: 'Telegram ID', key: 'telegramId', width: 140 },
    {
      title: '用户',
      key: 'username',
      width: 220,
      render(row) {
        const name = [row.firstName, row.lastName].filter(Boolean).join(' ');
        return h('div', [h('div', row.username ? `@${row.username}` : '-'), h('div', { class: 'text-gray-400' }, name)]);
      },
    },
    {
      title: '会员等级',
      key: 'memberLevel',
      width: 150,
      render(row) {
        return h(NSelect, {
          value: row.memberLevel,
          options: memberOptions,
          onUpdateValue: (value) => {
            row.memberLevel = value;
          },
        });
      },
    },
    {
      title: '积分',
      key: 'points',
      width: 150,
      render(row) {
        return h(NInputNumber, {
          value: row.points,
          min: 0,
          showButton: false,
          onUpdateValue: (value) => {
            row.points = Number(value || 0);
          },
        });
      },
    },
    {
      title: '到期时间',
      key: 'memberExpireAt',
      width: 210,
      render(row) {
        return h(NDatePicker, {
          type: 'datetime',
          clearable: true,
          value: parseDateValue(row.memberExpireAt),
          format: 'yyyy-MM-dd HH:mm:ss',
          onUpdateValue: (value) => {
            row.memberExpireAt = value ? format(new Date(value), 'yyyy-MM-dd HH:mm:ss') : '';
          },
        });
      },
    },
    {
      title: '状态',
      key: 'status',
      width: 120,
      render(row) {
        return h(NSelect, {
          value: row.status,
          options: statusOptions,
          onUpdateValue: (value) => {
            row.status = value;
          },
        });
      },
    },
    { title: '最后活跃', key: 'lastActiveAt', width: 180 },
    {
      title: '操作',
      key: 'action',
      width: 90,
      render(row) {
        return h(NButton, { size: 'small', type: 'primary', onClick: () => save(row) }, { default: () => '保存' });
      },
    },
  ];

  function openModal(key: string) {
    botKey.value = key;
    showModal.value = true;
    load();
  }

  async function load() {
    loading.value = true;
    try {
      const res = await botUsers({
        botKey: botKey.value,
        keyword: query.keyword,
        memberLevel: query.memberLevel || 0,
        status: query.status || 0,
      });
      users.value = res?.list || [];
    } finally {
      loading.value = false;
    }
  }

  async function save(row) {
    await updateBotUser({
      id: row.id,
      memberLevel: row.memberLevel,
      points: row.points,
      memberExpireAt: row.memberExpireAt || '',
      status: row.status,
    });
    message.success('保存成功');
    load();
  }

  function parseDateValue(value?: string) {
    if (!value) {
      return null;
    }
    const timestamp = Date.parse(value.replace(' ', 'T'));
    return Number.isNaN(timestamp) ? null : timestamp;
  }

  defineExpose({ openModal });
</script>
