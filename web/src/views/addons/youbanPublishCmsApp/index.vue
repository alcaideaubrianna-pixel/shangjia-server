<template>
  <div>
    <div class="n-layout-page-header">
      <n-card :bordered="false" title="CMS 应用">
        <span>管理允许接入资料开放接口的 XC-CMS 实例。应用密钥仅在创建或重置时显示一次。</span>
      </n-card>
    </div>

    <n-card :bordered="false" class="proCard">
      <n-space class="mb-4" align="center">
        <n-input
          v-model:value="query.name"
          clearable
          placeholder="应用名称"
          @keyup.enter="loadApps"
        />
        <n-select
          v-model:value="query.status"
          :options="statusOptions"
          clearable
          placeholder="全部状态"
          class="status-select"
        />
        <n-button :loading="loading" @click="loadApps">查询</n-button>
        <n-button type="primary" @click="openCreate">手动新增</n-button>
      </n-space>

      <n-data-table
        :columns="columns"
        :data="apps"
        :loading="loading"
        :row-key="(row) => row.id"
        :scroll-x="1060"
        size="small"
      />
    </n-card>

    <n-modal
      v-model:show="editorVisible"
      :mask-closable="false"
      preset="dialog"
      :title="form.id > 0 ? '编辑 CMS 应用' : '新增 CMS 应用'"
      positive-text="保存"
      negative-text="取消"
      @positive-click="saveApp"
    >
      <n-form ref="formRef" :model="form" :rules="rules" label-placement="left" label-width="92">
        <n-form-item label="应用名称" path="name">
          <n-input v-model:value="form.name" :maxlength="128" placeholder="例如：爱妃相册生产站" />
        </n-form-item>
        <n-form-item label="站点地址" path="baseUrl">
          <n-input
            v-model:value="form.baseUrl"
            :maxlength="500"
            placeholder="https://cms.example.com"
          />
        </n-form-item>
        <n-form-item label="状态" path="status">
          <n-radio-group v-model:value="form.status">
            <n-space>
              <n-radio :value="1">批准授权</n-radio>
              <n-radio :value="2">待审核</n-radio>
              <n-radio :value="3">停用</n-radio>
              <n-radio :value="4">撤销</n-radio>
            </n-space>
          </n-radio-group>
        </n-form-item>
      </n-form>
    </n-modal>

    <n-modal
      v-model:show="credentialVisible"
      :mask-closable="false"
      preset="card"
      title="请立即保存应用凭证"
      class="credential-modal"
    >
      <n-alert type="warning" :show-icon="true" class="mb-4">
        应用密钥关闭后无法再次查看。遗失时只能重置，重置后旧密钥立即失效。
      </n-alert>
      <n-form label-placement="left" label-width="92">
        <n-form-item label="App ID">
          <n-input :value="credential.appId" readonly>
            <template #suffix>
              <n-button text type="primary" @click="copyCredential(credential.appId, 'App ID')"
                >复制</n-button
              >
            </template>
          </n-input>
        </n-form-item>
        <n-form-item label="App Secret">
          <n-input :value="credential.appSecret" readonly>
            <template #suffix>
              <n-button
                text
                type="primary"
                @click="copyCredential(credential.appSecret, 'App Secret')"
                >复制</n-button
              >
            </template>
          </n-input>
        </n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button type="primary" @click="credentialVisible = false">我已保存</n-button>
        </n-space>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
  import { h, onMounted, reactive, ref } from 'vue';
  import type { DataTableColumns, FormInst, FormRules } from 'naive-ui';
  import { NButton, NPopconfirm, NSpace, NTag, useMessage } from 'naive-ui';

  import { CmsAppList, CmsAppResetSecret, CmsAppSave } from '@/api/addons/youbanPublish';

  interface CmsApp {
    id: number;
    appId: string;
    name: string;
    baseUrl: string;
    instanceId?: string;
    sourceIp?: string;
    cmsVersion?: string;
    lastHeartbeatAt?: string;
    status: number;
    createdAt?: string;
    updatedAt?: string;
  }

  interface CmsCredential extends CmsApp {
    appSecret: string;
  }

  const message = useMessage();
  const loading = ref(false);
  const saving = ref(false);
  const apps = ref<CmsApp[]>([]);
  const editorVisible = ref(false);
  const credentialVisible = ref(false);
  const formRef = ref<FormInst | null>(null);
  const query = reactive<{ name: string; status: number | null }>({ name: '', status: null });
  const form = reactive({ id: 0, name: '', baseUrl: '', status: 1 });
  const credential = reactive<CmsCredential>({
    id: 0,
    appId: '',
    appSecret: '',
    name: '',
    baseUrl: '',
    status: 1,
  });

  const statusOptions = [
    { label: '已授权', value: 1 },
    { label: '待审核', value: 2 },
    { label: '已停用', value: 3 },
    { label: '已撤销', value: 4 },
  ];
  const rules: FormRules = {
    name: [
      { required: true, message: '请输入应用名称', trigger: ['input', 'blur'] },
      { min: 2, max: 128, message: '应用名称长度应为 2 到 128 位', trigger: ['input', 'blur'] },
    ],
    baseUrl: {
      validator: (_rule, value: string) => {
        if (!value) return true;
        try {
          const url = new URL(value);
          return ['http:', 'https:'].includes(url.protocol);
        } catch {
          return false;
        }
      },
      message: '请输入完整的 HTTP 或 HTTPS 地址',
      trigger: ['input', 'blur'],
    },
  };

  const columns: DataTableColumns<CmsApp> = [
    { title: '应用名称', key: 'name', minWidth: 180 },
    {
      title: '实例 ID', key: 'instanceId', minWidth: 210,
      ellipsis: { tooltip: true }, render: (row) => row.instanceId || '手动应用',
    },
    { title: 'App ID', key: 'appId', minWidth: 190 },
    {
      title: '站点地址',
      key: 'baseUrl',
      minWidth: 220,
      ellipsis: { tooltip: true },
      render: (row) => row.baseUrl || '-',
    },
    {
      title: '来源 IP / 版本', key: 'sourceIp', minWidth: 160,
      render: (row) => [row.sourceIp, row.cmsVersion].filter(Boolean).join(' / ') || '-',
    },
    {
      title: '状态',
      key: 'status',
      width: 90,
      render: (row) =>
        h(
          NTag,
          { type: row.status === 1 ? 'success' : row.status === 2 ? 'warning' : 'default', bordered: false },
          { default: () => statusOptions.find((item) => item.value === row.status)?.label || '未知' }
        ),
    },
    { title: '最后心跳', key: 'lastHeartbeatAt', width: 180, render: (row) => row.lastHeartbeatAt || '-' },
    {
      title: '操作',
      key: 'actions',
      width: 190,
      fixed: 'right',
      render: (row) =>
        h(NSpace, null, {
          default: () => [
            h(NButton, { size: 'small', onClick: () => openEdit(row) }, { default: () => '编辑' }),
            h(
              NPopconfirm,
              { onPositiveClick: () => resetSecret(row) },
              {
                trigger: () =>
                  h(NButton, { size: 'small', type: 'warning' }, { default: () => '重置密钥' }),
                default: () => `确认重置“${row.name}”的密钥？旧密钥将立即失效。`,
              }
            ),
          ],
        }),
    },
  ];

  async function loadApps() {
    loading.value = true;
    try {
      const result = (await CmsAppList({
        name: query.name.trim(),
        status: query.status || 0,
      })) as { list?: CmsApp[] };
      apps.value = result?.list || [];
    } finally {
      loading.value = false;
    }
  }

  function resetForm(app?: CmsApp) {
    form.id = app?.id || 0;
    form.name = app?.name || '';
    form.baseUrl = app?.baseUrl || '';
    form.status = app?.status || 1;
  }

  function openCreate() {
    resetForm();
    editorVisible.value = true;
  }

  function openEdit(app: CmsApp) {
    resetForm(app);
    editorVisible.value = true;
  }

  async function saveApp(event: MouseEvent) {
    event.preventDefault();
    if (saving.value) return false;
    try {
      await formRef.value?.validate();
    } catch {
      return false;
    }
    saving.value = true;
    try {
      const result = (await CmsAppSave({
        ...form,
        name: form.name.trim(),
        baseUrl: form.baseUrl.trim(),
      })) as CmsCredential;
      editorVisible.value = false;
      message.success(form.id > 0 ? 'CMS 应用已更新' : 'CMS 应用已创建');
      if (result?.appSecret) showCredential(result);
      await loadApps();
    } finally {
      saving.value = false;
    }
    return false;
  }

  async function resetSecret(app: CmsApp) {
    const result = (await CmsAppResetSecret({ id: app.id })) as CmsCredential;
    if (!result?.appSecret) {
      message.error('密钥重置成功，但接口未返回新密钥');
      return;
    }
    showCredential(result);
    await loadApps();
  }

  function showCredential(result: CmsCredential) {
    Object.assign(credential, result);
    credentialVisible.value = true;
  }

  async function copyCredential(value: string, label: string) {
    if (!value) return;
    await navigator.clipboard.writeText(value);
    message.success(`${label} 已复制`);
  }

  onMounted(loadApps);
</script>

<style scoped>
  .status-select {
    width: 150px;
  }

  .credential-modal {
    width: min(620px, calc(100vw - 32px));
  }
</style>
