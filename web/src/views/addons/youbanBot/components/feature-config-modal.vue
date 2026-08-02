<template>
  <n-modal
    v-model:show="visible"
    preset="dialog"
    class="feature-config-modal"
    title="插件配置"
    positive-text="保存"
    negative-text="取消"
    @positive-click="emit('save')"
  >
    <n-form :model="form" label-placement="left" label-width="110" class="feature-config-form">
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
      <n-form-item
        v-for="item in form.configSchema"
        v-show="item.component !== 'hidden'"
        :key="item.field"
        :label="item.label"
      >
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
        <n-space v-else-if="item.component === 'image_upload'" vertical class="w-full">
          <UploadImage
            v-model:value="form.configValues[item.field]"
            :max-number="1"
            accept=".jpg,.jpeg"
            :max-size="5"
            :allowed-mime-types="['image/jpeg', 'image/jpg']"
            :image-max-dimension-sum="10000"
            :image-max-aspect-ratio="20"
            :image-min-short-side="360"
            :image-min-long-side="640"
            :image-recommended-aspect-ratio="16 / 9"
            :image-recommended-aspect-ratio-tolerance="0.04"
            help-text="上传后自动回填图片地址"
          />
          <n-input
            v-model:value="form.configValues[item.field]"
            clearable
            placeholder="也可直接填写 Telegram 可访问的 JPG/JPEG 外链"
          />
          <n-text depth="3">
            推荐使用 1280×720 的 16:9 横图。上传时校验 JPEG、最短边 360、最长边 640、最大 5
            MB；保存后会自动优化超大图片并生成方形 Inline 缩略图。
          </n-text>
        </n-space>
        <TelegramRichEditor
          v-else-if="item.component === 'telegram_rich_text'"
          v-model:value="form.configValues[item.field]"
        />
        <n-space v-else-if="item.component === 'telegram_buttons'" vertical class="w-full">
          <n-grid
            v-for="(button, index) in buttonItems(item.field)"
            :key="index"
            :cols="12"
            :x-gap="8"
          >
            <n-gi :span="3"><n-input v-model:value="button.text" placeholder="按钮文案" /></n-gi>
            <n-gi :span="6"><n-input v-model:value="button.url" placeholder="https://..." /></n-gi>
            <n-gi :span="2"
              ><n-input-number v-model:value="button.row" :min="0" :max="7" placeholder="行"
            /></n-gi>
            <n-gi :span="1"
              ><n-button quaternary type="error" @click="removeButton(item.field, index)"
                >删</n-button
              ></n-gi
            >
          </n-grid>
          <n-button dashed block @click="addButton(item.field)">添加 URL 按钮</n-button>
          <n-text depth="3">相同行号的按钮会排列在同一行，最多 8 个。</n-text>
        </n-space>
        <n-input
          v-else
          v-model:value="form.configValues[item.field]"
          :type="item.component === 'textarea' ? 'textarea' : 'text'"
          :placeholder="item.placeholder"
          :autosize="item.component === 'textarea' ? { minRows: 3, maxRows: 8 } : undefined"
        />
      </n-form-item>
      <n-form-item label="配置 JSON"
        ><n-input
          v-model:value="form.configJson"
          type="textarea"
          :autosize="{ minRows: 4, maxRows: 12 }"
      /></n-form-item>
      <n-form-item label="说明"
        ><n-input
          v-model:value="form.description"
          type="textarea"
          :autosize="{ minRows: 2, maxRows: 6 }"
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
    <n-space vertical class="mb-3" size="small">
      <n-select
        v-model:value="adminBotIds"
        multiple
        filterable
        clearable
        :options="superBotOptions"
        placeholder="筛选超级机器人"
      />
      <n-input v-model:value="adminKeyword" clearable placeholder="搜索 TG / 后台账号" />
    </n-space>
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
  import { BotList, UserList } from '@/api/addons/youbanBot';
  import UploadImage from '@/components/Upload/uploadImage.vue';
  import TelegramRichEditor from '@/components/TelegramRichEditor/index.vue';

  const props = defineProps<{ show: boolean; form: Record<string, any>; statusOptions: any[] }>();
  const form = computed(() => props.form);
  const emit = defineEmits<{ (e: 'update:show', value: boolean): void; (e: 'save'): void }>();

  const adminUsers = ref<any[]>([]);
  const superBots = ref<any[]>([]);
  const adminLoading = ref(false);
  const adminKeyword = ref('');
  const adminSelectVisible = ref(false);
  const adminSelectField = ref('');
  const checkedAdminIds = ref<any[]>([]);
  const adminBotIds = ref<any[]>([]);
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
      render: (row: any) =>
        row.botUsernames?.length
          ? row.botUsernames.map((item: string) => `@${item}`).join('、')
          : row.botUsername
            ? `@${row.botUsername}`
            : row.botId,
    },
  ];

  const visible = computed({
    get: () => props.show,
    set: (value: boolean) => emit('update:show', value),
  });
  const filteredAdminUsers = computed(() => {
    const keyword = adminKeyword.value.trim().toLowerCase();
    const selectedBotIds = new Set(adminBotIds.value.map((item) => String(item)));
    return adminUsers.value.filter((item) => {
      const botIds =
        Array.isArray(item.botIds) && item.botIds.length > 0 ? item.botIds : [item.botId];
      if (selectedBotIds.size > 0 && !botIds.some((botId) => selectedBotIds.has(String(botId)))) {
        return false;
      }
      if (!keyword) return true;
      return [
        item.telegramUserId,
        item.telegramUsername,
        item.bindAccountName,
        item.botUsername,
      ].some((v) =>
        String(v || '')
          .toLowerCase()
          .includes(keyword)
      );
    });
  });
  const superBotOptions = computed(() =>
    superBots.value.map((item) => ({
      label: item.botUsername ? `@${item.botUsername}` : item.botName,
      value: item.id,
    }))
  );

  function currentBotIds() {
    return adminBotIds.value.length > 0
      ? adminBotIds.value
      : superBots.value.map((item) => item.id);
  }

  function mergeUsers(list: any[]) {
    const map = new Map<string, any>();
    for (const item of list || []) {
      if (!item || !item.telegramUserId) continue;
      const key = String(item.telegramUserId);
      const exists = map.get(key);
      if (!exists) {
        map.set(key, {
          ...item,
          botIds: [item.botId].filter(Boolean),
          botUsernames: item.botUsername ? [item.botUsername] : [],
        });
        continue;
      }
      exists.botIds = Array.from(new Set([...(exists.botIds || []), item.botId].filter(Boolean)));
      exists.botUsernames = Array.from(
        new Set([...(exists.botUsernames || []), item.botUsername].filter(Boolean))
      );
      const prevAt = new Date(exists.lastMessageAt || 0).getTime();
      const nextAt = new Date(item.lastMessageAt || 0).getTime();
      if (nextAt >= prevAt) {
        exists.botId = item.botId;
        exists.botUsername = item.botUsername || exists.botUsername;
        exists.chatId = item.chatId || exists.chatId;
        exists.chatType = item.chatType || exists.chatType;
        exists.messageCount = Math.max(exists.messageCount || 0, item.messageCount || 0);
        exists.lastMessageAt = item.lastMessageAt || exists.lastMessageAt;
        exists.lastMessageText = item.lastMessageText || exists.lastMessageText;
        exists.isBound = item.isBound || exists.isBound;
        exists.bindApp = item.bindApp || exists.bindApp;
        exists.bindAccountId = item.bindAccountId || exists.bindAccountId;
        exists.bindTenantId = item.bindTenantId || exists.bindTenantId;
        exists.bindAccountName = item.bindAccountName || exists.bindAccountName;
      }
    }
    return Array.from(map.values());
  }

  async function loadAdminUsers() {
    if (superBots.value.length === 0) {
      adminUsers.value = [];
      return;
    }
    adminLoading.value = true;
    try {
      const res: any = await UserList({
        page: 1,
        perPage: 200,
        botIds: currentBotIds(),
      });
      adminUsers.value = mergeUsers(res?.list || []);
    } finally {
      adminLoading.value = false;
    }
  }

  async function loadSuperBots() {
    const res: any = await BotList({ page: 1, perPage: 200, status: 1, isOfficial: 1 });
    superBots.value = res?.list || [];
    if (adminBotIds.value.length === 0) {
      adminBotIds.value = superBots.value.map((item) => item.id);
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

  function buttonItems(field: string) {
    if (!Array.isArray(form.value.configValues[field])) {
      form.value.configValues[field] = [];
    }
    return form.value.configValues[field] as Array<{ text: string; url: string; row: number }>;
  }

  function addButton(field: string) {
    const list = buttonItems(field);
    if (list.length >= 8) return;
    list.push({ text: '', url: '', row: list.length });
  }

  function removeButton(field: string, index: number) {
    buttonItems(field).splice(index, 1);
  }

  onMounted(async () => {
    await loadSuperBots();
    await loadAdminUsers();
  });
</script>

<style scoped>
  .feature-config-modal :deep(.n-dialog) {
    width: 720px;
    max-width: calc(100vw - 32px);
  }

  .feature-config-form :deep(.n-form-item-blank),
  .feature-config-form :deep(.n-form-item-blank > *) {
    width: 100%;
  }

  .feature-config-form :deep(.n-input),
  .feature-config-form :deep(.n-input-number),
  .feature-config-form :deep(.n-select) {
    width: 100%;
  }
</style>
