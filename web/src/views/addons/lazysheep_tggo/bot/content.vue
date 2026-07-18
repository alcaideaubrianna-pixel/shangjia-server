<template>
  <div>
    <BasicTable
      ref="actionRef"
      :columns="columns"
      :request="loadDataTable"
      :row-key="(row) => row.key"
      :actionColumn="actionColumn"
      :pagination="false"
      :resizeHeightOffset="-10000"
    >
      <template #tableTitle>
        <n-button type="primary" class="min-left-space" @click="addTable">
          <template #icon>
            <n-icon>
              <PlusOutlined />
            </n-icon>
          </template>
          添加机器人
        </n-button>
      </template>
    </BasicTable>
    <Edit ref="editRef" @submit="handleSubmit" />
    <PluginModal ref="pluginRef" @submit="handlePluginSubmit" />
    <UserModal ref="userRef" />
  </div>
</template>

<script lang="ts" setup>
  import { h, reactive, ref } from 'vue';
  import { NButton, NDropdown, NSpace, useDialog, useMessage } from 'naive-ui';
  import { PlusOutlined } from '@vicons/antd';
  import { BasicTable } from '@/components/Table';
  import { bots, deleteBot, getConfig, startBot, updateConfig, upsertBot } from '@/api/addons/lazysheep_tggo/config';
  import { BotRow, columns } from './model';
  import Edit from './edit.vue';
  import PluginModal from './pluginModal.vue';
  import UserModal from './userModal.vue';

  const dialog = useDialog();
  const message = useMessage();
  const actionRef = ref();
  const editRef = ref();
  const pluginRef = ref();
  const userRef = ref();
  const stateRef = ref<any>(null);
  const startingKeys = ref<Record<string, boolean>>({});

  const actionColumn = reactive({
    width: 210,
    title: '操作',
    key: 'action',
    fixed: 'right',
    render(record: BotRow) {
      const buttons: any[] = [];
      const dropdownOptions = [
        {
          label: '插件',
          key: 'plugin',
        },
        {
          label: '用户',
          key: 'users',
        },
        {
          label: '重启',
          key: 'restart',
        },
        {
          label: '删除',
          key: 'delete',
        },
      ];
      if (record.enabled && record.runtimeStatus !== 'running') {
        buttons.push(
          h(
            NButton,
            {
              size: 'small',
              disabled: !!startingKeys.value[record.key],
              onClick: () => handleStart(record),
            },
            { default: () => (startingKeys.value[record.key] ? '启动中' : '启动') }
          )
        );
      }
      buttons.push(
        h(NButton, { size: 'small', type: 'primary', onClick: () => handleEdit(record) }, { default: () => '编辑' }),
        h(
          NDropdown,
          {
            trigger: 'click',
            options: dropdownOptions,
            onSelect: (key: string) => {
              if (key === 'plugin') {
                handlePlugin(record);
              }
              if (key === 'users') {
                handleUsers(record);
              }
              if (key === 'restart') {
                handleRestart(record);
              }
              if (key === 'delete') {
                handleDelete(record);
              }
            },
          },
          {
            default: () => h(NButton, { size: 'small' }, { default: () => '更多' }),
          }
        )
      );
      return h(NSpace, { size: 8, justify: 'center' }, { default: () => buttons });
    },
  });

  async function loadDataTable() {
    const res = await bots();
    const botMap = res?.bots || {};
    return {
      list: Object.keys(botMap).map((key) => {
        const row = botMap[key] || {};
        const rowKey = row.key || key;
        return {
          ...row,
          key: rowKey,
          starting: !!startingKeys.value[rowKey],
        };
      }),
      page: 1,
      pageCount: 1,
      itemCount: Object.keys(botMap).length,
    };
  }

  function addTable() {
    editRef.value.openModal();
  }

  function handleEdit(record: BotRow) {
    editRef.value.openModal(record);
  }

  async function handleStart(record: BotRow) {
    startingKeys.value = { ...startingKeys.value, [record.key]: true };
    reloadTable();
    try {
      await startBot({ key: record.key });
      message.success('机器人已启动');
    } catch (e) {
      message.error('机器人启动失败，请查看异常信息');
    } finally {
      const next = { ...startingKeys.value };
      delete next[record.key];
      startingKeys.value = next;
      stateRef.value = null;
      reloadTable();
    }
  }

  async function handleRestart(record: BotRow) {
    startingKeys.value = { ...startingKeys.value, [record.key]: true };
    reloadTable();
    try {
      await startBot({ key: record.key });
      message.success('机器人已重启');
    } catch (e) {
      message.error('机器人重启失败，请查看异常信息');
    } finally {
      const next = { ...startingKeys.value };
      delete next[record.key];
      startingKeys.value = next;
      stateRef.value = null;
      reloadTable();
    }
  }

  async function handlePlugin(record: BotRow) {
    const state = await ensureState();
    const bot = state.bots[record.key] || {};
    pluginRef.value.openModal(record.key, bot.plugins || state.plugins || {});
  }

  function handleUsers(record: BotRow) {
    userRef.value.openModal(record.key);
  }

  function handleDelete(record: BotRow) {
    dialog.warning({
      title: '警告',
      content: '你确定要删除该机器人？',
      positiveText: '确定',
      negativeText: '取消',
      onPositiveClick: async () => {
        await deleteBot({ key: record.key });
        stateRef.value = null;
        message.success('删除成功');
        reloadTable();
      },
    });
  }

  async function handleSubmit(record: BotRow, done: () => void) {
    try {
      await upsertBot(toBotPayload(record));
      stateRef.value = null;
      message.success('保存成功');
      done(true);
      reloadTable();
    } catch (e: any) {
      const detail = e?.message || e?.msg || '保存失败，请查看接口返回';
      message.error(detail);
      done(false);
    }
  }

  function toBotPayload(record: BotRow) {
    return {
      key: record.key,
      role: record.role,
      token: record.token,
      displayName: record.displayName,
      username: record.username,
      webhookSecret: record.webhookSecret,
      webhookPath: record.webhookPath,
      enabled: record.enabled,
      autoPull: record.autoPull,
      autoForward: record.autoForward,
      reviewEnabled: record.reviewEnabled,
    };
  }

  async function handlePluginSubmit(botKey: string, plugins: Record<string, any>) {
    const state = await ensureState();
    state.bots[botKey] = {
      ...(state.bots[botKey] || {}),
      key: botKey,
      plugins,
    };
    await saveState(state);
    stateRef.value = null;
    message.success('插件配置已保存');
    reloadTable();
  }

  function reloadTable() {
    actionRef.value?.reload();
  }

  async function ensureState() {
    if (stateRef.value) {
      return normalizeState(stateRef.value);
    }
    const res = await getConfig({ group: 'bot' });
    return normalizeState(res?.list?.state);
  }

  async function saveState(state) {
    stateRef.value = normalizeState(state);
    await updateConfig({
      group: 'bot',
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
</script>
