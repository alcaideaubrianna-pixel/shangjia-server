<template>
  <n-modal
    v-model:show="showModal"
    :show-icon="false"
    preset="dialog"
    :title="`${botKey} 插件配置`"
    :style="{ width: '820px' }"
  >
    <n-data-table :columns="columns" :data="pluginList" :pagination="false" />
    <n-modal
      v-model:show="showConfigModal"
      preset="dialog"
      :show-icon="false"
      :title="currentPlugin?.name || '插件配置'"
      :style="{ width: '720px' }"
    >
      <PluginConfigForm
        v-if="currentPlugin"
        :plugin="currentPlugin"
        @submit="handleConfigSubmit"
      />
    </n-modal>
    <template #action>
      <n-space>
        <n-button @click="closeModal">取消</n-button>
        <n-button type="primary" @click="submit">保存</n-button>
      </n-space>
    </template>
  </n-modal>
</template>

<script lang="ts" setup>
  import { computed, h, ref } from 'vue';
  import { NButton, NSpace, NSwitch, NTag } from 'naive-ui';
  import PluginConfigForm from '../config/tgPluginConfig.vue';

  const emit = defineEmits(['submit']);
  const showModal = ref(false);
  const showConfigModal = ref(false);
  const botKey = ref('');
  const plugins = ref<Record<string, any>>({});
  const currentPlugin = ref<any>(null);

  const pluginList = computed(() => {
    return Object.values(plugins.value || {}).sort((a: any, b: any) => {
      return Number(a.sort || 0) - Number(b.sort || 0);
    });
  });

  const columns = [
    {
      title: '插件',
      key: 'name',
      width: 180,
      render(row) {
        return h('div', [
          h('div', row.name),
          h('div', { class: 'text-gray-400' }, row.subtitle || row.description || row.key),
          h(NTag, { bordered: false }, { default: () => row.category }),
        ]);
      },
    },
    {
      title: '启用',
      key: 'enabled',
      width: 90,
      render(row) {
        return h(NSwitch, {
          value: row.enabled,
          onUpdateValue: (value) => {
            plugins.value[row.key].enabled = value;
          },
        });
      },
    },
    {
      title: '用户可用',
      key: 'userEnabled',
      width: 100,
      render(row) {
        return h(NSwitch, {
          value: row.userEnabled,
          onUpdateValue: (value) => {
            plugins.value[row.key].userEnabled = value;
          },
        });
      },
    },
    {
      title: '增值',
      key: 'paid',
      width: 90,
      render(row) {
        return h(NTag, { type: row.paid ? 'warning' : 'success', bordered: false }, { default: () => (row.paid ? '付费' : '免费') });
      },
    },
    {
      title: '命令',
      key: 'commands',
      width: 220,
      render(row) {
        const commands = normalizeCommands(row);
        if (!commands.length) {
          return h('span', { class: 'text-gray-400' }, '未配置');
        }
        return h(
          'div',
          { style: 'display:flex;flex-wrap:wrap;gap:6px;' },
          commands.slice(0, 3).map((item) => h(NTag, { bordered: false, type: 'info' }, { default: () => item }))
        );
      },
    },
    {
      title: '操作',
      key: 'action',
      width: 90,
      render(row) {
        return h(NSpace, null, {
          default: () => [
            h(
              NButton,
              {
                size: 'small',
                onClick: () => openConfig(row),
              },
              { default: () => '配置' }
            ),
          ],
        });
      },
    },
  ];

  function openModal(key: string, value: Record<string, any>) {
    botKey.value = key;
    plugins.value = JSON.parse(JSON.stringify(value || {}));
    showModal.value = true;
  }

  function closeModal() {
    showModal.value = false;
  }

  function openConfig(plugin) {
    currentPlugin.value = JSON.parse(JSON.stringify(plugin));
    currentPlugin.value.allPlugins = JSON.parse(JSON.stringify(plugins.value || {}));
    showConfigModal.value = true;
  }

  function handleConfigSubmit(plugin) {
    plugins.value[plugin.key] = plugin;
    showConfigModal.value = false;
    currentPlugin.value = null;
    emit('submit', botKey.value, JSON.parse(JSON.stringify(plugins.value)));
  }

  function submit() {
    emit('submit', botKey.value, JSON.parse(JSON.stringify(plugins.value)));
    closeModal();
    showConfigModal.value = false;
    currentPlugin.value = null;
  }

  function normalizeCommands(row) {
    const settings = row?.settings || {};
    const list = Array.isArray(settings.commands) ? settings.commands : [];
    const fallback = `${settings.command || ''}`.trim();
    const cleaned = list.map((item) => `${item || ''}`.trim()).filter(Boolean);
    if (fallback && !cleaned.includes(fallback)) {
      cleaned.unshift(fallback);
    }
    return cleaned;
  }

  defineExpose({
    openModal,
  });
</script>
