<template>
  <n-modal
    v-model:show="visible"
    preset="dialog"
    title="插件配置"
    positive-text="保存"
    negative-text="取消"
    @positive-click="emit('save')"
  >
    <n-form :model="form" label-placement="left" label-width="100">
      <n-form-item label="插件 Key"
        ><n-input v-model:value="form.featureKey" disabled
      /></n-form-item>
      <n-form-item label="插件名称"><n-input v-model:value="form.name" clearable /></n-form-item>
      <n-form-item label="命令"
        ><n-input v-model:value="form.command" clearable placeholder="不带 /"
      /></n-form-item>
      <n-form-item label="排序"><n-input-number v-model:value="form.sort" clearable /></n-form-item>
      <n-form-item label="状态"
        ><n-select v-model:value="form.status" :options="statusOptions"
      /></n-form-item>
      <n-form-item v-for="item in form.configSchema" :key="item.field" :label="item.label">
        <n-switch
          v-if="item.component === 'switch'"
          v-model:value="form.configValues[item.field]"
          :checked-value="1"
          :unchecked-value="0"
        />
        <n-space v-else-if="item.component === 'bot_admin_user_select'" vertical>
          <n-button @click="openAdminSelect(item.field)">选择管理员</n-button>
          <n-space v-if="selectedAdminLabels(item.field).length">
            <n-tag v-for="label in selectedAdminLabels(item.field)" :key="label" type="info">{{
              label
            }}</n-tag>
          </n-space>
          <n-text v-else depth="3">未选择管理员</n-text>
        </n-space>
        <n-select
          v-else-if="item.component === 'select'"
          v-model:value="form.configValues[item.field]"
          :options="item.options || []"
          :placeholder="item.placeholder"
        />
        <n-input
          v-else
          v-model:value="form.configValues[item.field]"
          :type="item.component === 'textarea' ? 'textarea' : 'text'"
          :placeholder="item.placeholder"
          :autosize="{ minRows: 3, maxRows: 8 }"
        />
      </n-form-item>
      <n-form-item label="配置 JSON"
        ><n-input
          v-model:value="form.configJson"
          type="textarea"
          :autosize="{ minRows: 3, maxRows: 6 }"
      /></n-form-item>
      <n-form-item label="说明"
        ><n-input
          v-model:value="form.description"
          type="textarea"
          :autosize="{ minRows: 2, maxRows: 4 }"
      /></n-form-item>
    </n-form>
  </n-modal>

  <n-modal
    v-model:show="adminSelectVisible"
    preset="dialog"
    title="选择超级通知管理员"
    positive-text="确定"
    negative-text="取消"
    @positive-click="confirmAdminSelect"
  >
    <n-input v-model:value="adminKeyword" clearable placeholder="搜索 TG / 后台账号" class="mb-3" />
    <n-data-table
      :columns="adminColumns"
      :data="filteredAdminUsers"
      :loading="adminLoading"
      :row-key="(row) => row.telegramUserId"
      :checked-row-keys="checkedAdminIds"
      max-height="420"
      @update:checked-row-keys="handleAdminChecked"
    />
  </n-modal>
</template>

<script setup lang="ts">
  import { computed, onMounted, ref } from 'vue';
  import { UserList } from '@/api/addons/youbanBot';

  const props = defineProps<{ show: boolean; form: Record<string, any>; statusOptions: any[] }>();
  const form = computed(() => props.form);
  const emit = defineEmits<{ (e: 'update:show', value: boolean): void; (e: 'save'): void }>();

  const adminUsers = ref<any[]>([]);
  const adminLoading = ref(false);
  const adminKeyword = ref('');
  const adminSelectVisible = ref(false);
  const adminSelectField = ref('');
  const checkedAdminIds = ref<any[]>([]);
  const adminColumns = [
    { type: 'selection' },
    {
      title: 'Telegram',
      key: 'telegramUsername',
      render: (row: any) =>
        row.telegramUsername ? `@${row.telegramUsername}` : row.telegramUserId,
    },
    { title: '后台账号', key: 'bindAccountName' },
    {
      title: 'Bot',
      key: 'botUsername',
      render: (row: any) => (row.botUsername ? `@${row.botUsername}` : row.botId),
    },
  ];

  const visible = computed({
    get: () => props.show,
    set: (value: boolean) => emit('update:show', value),
  });
  const filteredAdminUsers = computed(() => {
    const keyword = adminKeyword.value.trim().toLowerCase();
    if (!keyword) return adminUsers.value;
    return adminUsers.value.filter((item) =>
      [item.telegramUserId, item.telegramUsername, item.bindAccountName, item.botUsername].some(
        (v) =>
          String(v || '')
            .toLowerCase()
            .includes(keyword)
      )
    );
  });

  async function loadAdminUsers() {
    adminLoading.value = true;
    try {
      const res: any = await UserList({ page: 1, perPage: 200, isBound: 1, bindApp: 'admin' });
      adminUsers.value = res?.list || [];
    } finally {
      adminLoading.value = false;
    }
  }

  function openAdminSelect(field: string) {
    adminSelectField.value = field;
    checkedAdminIds.value = normalizeSelected(form.value.configValues[field]);
    adminSelectVisible.value = true;
    loadAdminUsers();
  }

  function handleAdminChecked(keys: any[]) {
    checkedAdminIds.value = keys;
  }

  function confirmAdminSelect() {
    form.value.configValues[adminSelectField.value] = checkedAdminIds.value;
  }

  function normalizeSelected(value: any) {
    if (Array.isArray(value)) return value.map((item) => String(item));
    if (!value) return [];
    return String(value)
      .split(',')
      .map((item) => item.trim())
      .filter(Boolean);
  }

  function selectedAdminLabels(field: string) {
    const selected = new Set(normalizeSelected(form.value.configValues[field]));
    return adminUsers.value
      .filter((item) => selected.has(String(item.telegramUserId)))
      .map(
        (item) =>
          `${item.telegramUsername ? '@' + item.telegramUsername : item.telegramUserId} / ${item.bindAccountName || '后台账号'}`
      );
  }

  onMounted(loadAdminUsers);
</script>
