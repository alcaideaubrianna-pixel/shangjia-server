<template>
  <div>
    <div class="n-layout-page-header">
      <n-card :bordered="false" title="TG 插件配置">
        管理 Bot 能力插件的全局开关、用户可用状态和增值能力。
      </n-card>
    </div>
    <n-card :bordered="false" class="proCard">
      <BasicTable
        ref="actionRef"
        :columns="columns"
        :request="loadDataTable"
        :row-key="(row) => row.key"
        :actionColumn="actionColumn"
        :pagination="false"
        :scroll-x="1050"
      />
    </n-card>
    <n-modal
      v-model:show="showModal"
      preset="dialog"
      :show-icon="false"
      :title="modalTitle"
      :style="{ width: '720px' }"
    >
      <PluginConfigForm v-if="currentPlugin" :plugin="currentPlugin" @submit="handleConfigSubmit" />
    </n-modal>
  </div>
</template>

<script lang="ts" setup>
  import { h, reactive, ref } from 'vue';
  import { NSwitch, NTag, useMessage } from 'naive-ui';
  import { BasicTable, TableAction } from '@/components/Table';
  import { getConfig, updateConfig } from '@/api/addons/lazysheep_tggo/config';
  import PluginConfigForm from './tgPluginConfig.vue';

  const message = useMessage();
  const actionRef = ref();
  const stateRef = ref<any>(null);
  const showModal = ref(false);
  const modalTitle = ref('插件配置');
  const currentPlugin = ref<any>(null);

  const columns = [
    {
      title: '插件名称',
      key: 'name',
      width: 180,
      render(row) {
        return h('div', [
          h('div', row.name),
          h('div', { class: 'text-gray-400' }, row.subtitle || row.key),
        ]);
      },
    },
    {
      title: '分类',
      key: 'category',
      width: 100,
      render(row) {
        return h(NTag, { type: 'info', bordered: false }, { default: () => row.category });
      },
    },
    {
      title: '简介',
      key: 'description',
      width: 260,
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
      title: '全局启用',
      key: 'enabled',
      width: 110,
      render(row) {
        return h(NSwitch, {
          value: row.enabled,
          onUpdateValue: (value) => switchPlugin(row, 'enabled', value),
        });
      },
    },
    {
      title: '用户可启用',
      key: 'userEnabled',
      width: 120,
      render(row) {
        return h(NSwitch, {
          value: row.userEnabled,
          onUpdateValue: (value) => switchPlugin(row, 'userEnabled', value),
        });
      },
    },
    {
      title: '增值插件',
      key: 'paid',
      width: 110,
      render(row) {
        const type = row.paid ? 'warning' : 'success';
        const label = row.paid ? `付费 ${row.price || 0}` : '免费';
        return h(NTag, { type, bordered: false }, { default: () => label });
      },
    },
  ];

  const actionColumn = reactive({
    width: 100,
    title: '操作',
    key: 'action',
    fixed: 'right',
    render(record: Recordable) {
      return h(TableAction as any, {
        style: 'button',
        actions: [
          {
            label: '配置',
            onClick: openConfig.bind(null, record),
          },
        ],
      });
    },
  });

  async function loadDataTable() {
    const state = await ensureState(true);
    const list = Object.values(state.plugins || {}).sort((a: any, b: any) => {
      return Number(a.sort || 0) - Number(b.sort || 0);
    });
    return {
      list,
      page: 1,
      pageCount: 1,
      itemCount: list.length,
    };
  }

  async function switchPlugin(row, field, value) {
    const state = await ensureState();
    state.plugins[row.key][field] = value;
    await saveState(state);
    message.success('操作成功');
    actionRef.value?.reload();
  }

  function openConfig(record) {
    currentPlugin.value = { ...record, settings: { ...(record.settings || {}) } };
    modalTitle.value = `${record.name}配置`;
    showModal.value = true;
  }

  async function handleConfigSubmit(plugin) {
    const state = await ensureState();
    state.plugins[plugin.key] = plugin;
    await saveState(state);
    showModal.value = false;
    message.success('保存成功');
    actionRef.value?.reload();
  }

  async function ensureState(force = false) {
    if (!force && stateRef.value) {
      return normalizeState(stateRef.value);
    }
    const res = await getConfig({ group: 'plugins' });
    stateRef.value = normalizeState(res?.list?.state);
    return stateRef.value;
  }

  async function saveState(state) {
    stateRef.value = normalizeState(state);
    await updateConfig({
      group: 'plugins',
      list: {
        state: stateRef.value,
      },
    });
  }

  function normalizeState(state) {
    const next = state || {};
    next.bots = next.bots || {};
    next.users = next.users || {};
    next.bindings = next.bindings || {};
    next.settings = next.settings || {};
    next.plugins = next.plugins || {};
    next.global = next.global || {};
    return next;
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
</script>
