<template>
  <div>
    <n-card :bordered="false" class="proCard">
      <n-tabs v-model:value="activeTab" type="line" animated @update:value="handleTabChange">
        <n-tab-pane name="dashboard" tab="工作台">
          <DashboardPanel />
        </n-tab-pane>

        <n-tab-pane name="tenants" tab="账号归属">
          <n-space class="toolbar" align="center">
            <n-input
              v-model:value="tenantQuery.keyword"
              placeholder="管理员账号 / 备注"
              clearable
              @keyup.enter="loadTenants"
            />
            <n-select
              v-model:value="tenantQuery.status"
              :options="statusOptionsWithAll"
              clearable
              placeholder="状态"
              class="status-select"
            />
            <n-button @click="loadTenants">查询</n-button>
            <n-button type="primary" @click="openTenantModal()">新增账号归属</n-button>
          </n-space>
          <n-data-table
            :columns="tenantColumns"
            :data="tenants"
            :loading="tenantLoading"
            :pagination="tenantPagination"
            :row-key="(row) => row.id"
            :scroll-x="980"
            size="small"
            remote
          />
        </n-tab-pane>

        <n-tab-pane name="accounts" tab="账号">
          <n-space class="toolbar" align="center">
            <n-select
              v-model:value="accountQuery.tenantId"
              :options="tenantOptionsWithAll"
              clearable
              filterable
              placeholder="账号归属"
              class="tenant-select"
            />
            <n-select
              v-model:value="accountQuery.accountType"
              :options="accountTypeOptionsWithAll"
              clearable
              placeholder="账号类型"
              class="status-select"
            />
            <n-input
              v-model:value="accountQuery.keyword"
              placeholder="账号 / 昵称"
              clearable
              @keyup.enter="loadAccounts"
            />
            <n-button @click="loadAccounts">查询</n-button>
            <n-button type="primary" @click="openAccountModal()">新增账号</n-button>
          </n-space>
          <n-data-table
            :columns="accountColumns"
            :data="accounts"
            :loading="accountLoading"
            :pagination="accountPagination"
            :row-key="(row) => row.id"
            :scroll-x="1280"
            size="small"
            remote
          />
        </n-tab-pane>

        <n-tab-pane name="tasks" tab="任务">
          <n-space class="toolbar" align="center">
            <n-select
              v-model:value="taskQuery.tenantId"
              :options="tenantOptionsWithAll"
              clearable
              filterable
              placeholder="账号归属"
              class="tenant-select"
            />
            <n-select
              v-model:value="taskQuery.status"
              :options="taskStatusOptionsWithAll"
              clearable
              placeholder="任务状态"
              class="status-select"
            />
            <n-input
              v-model:value="taskQuery.keyword"
              placeholder="标题 / 请求ID"
              clearable
              @keyup.enter="loadTasks"
            />
            <n-button @click="loadTasks">查询</n-button>
          </n-space>
          <n-data-table
            :columns="taskColumns"
            :data="tasks"
            :loading="taskLoading"
            :pagination="taskPagination"
            :row-key="(row) => row.id"
            :scroll-x="1320"
            size="small"
            remote
          />
        </n-tab-pane>

        <n-tab-pane name="profiles" tab="笔记资料">
          <ProfilePanel />
        </n-tab-pane>

        <n-tab-pane name="importTasks" tab="旧站导入">
          <ImportTaskPanel />
        </n-tab-pane>

        <n-tab-pane name="tags" tab="标签审核">
          <n-space class="toolbar" align="center">
            <n-input
              v-model:value="tagQuery.keyword"
              placeholder="标签名称"
              clearable
              @keyup.enter="loadTags"
            />
            <n-select
              v-model:value="tagQuery.reviewStatus"
              :options="reviewStatusOptionsWithAll"
              clearable
              placeholder="审核状态"
              class="status-select"
            />
            <n-select
              v-model:value="tagQuery.status"
              :options="statusOptionsWithAll"
              clearable
              placeholder="状态"
              class="status-select"
            />
            <n-button @click="loadTags">查询</n-button>
            <n-button type="primary" @click="openTagModal()">新增标签</n-button>
            <n-button
              :disabled="!selectedTagRowKeys.length"
              @click="batchUpdateTagReview('approved')"
              >批量通过</n-button
            >
            <n-button
              :disabled="!selectedTagRejectRows.length"
              @click="batchUpdateTagReview('rejected')"
              >批量驳回</n-button
            >
            <n-button type="error" :disabled="!selectedTagRowKeys.length" @click="batchDeleteTags"
              >批量删除</n-button
            >
          </n-space>
          <n-data-table
            :columns="tagColumns"
            :data="tags"
            :loading="tagLoading"
            :pagination="tagPagination"
            :row-key="(row) => row.id"
            :checked-row-keys="selectedTagRowKeys"
            @update:checked-row-keys="handleTagCheckedRowKeys"
            :scroll-x="980"
            size="small"
            remote
          />
        </n-tab-pane>

        <n-tab-pane name="bots" tab="Bot">
          <n-space class="toolbar" align="center">
            <n-select
              v-model:value="botQuery.tenantId"
              :options="botTenantOptions"
              clearable
              filterable
              placeholder="账号归属"
              class="tenant-select"
            />
            <n-input
              v-model:value="botQuery.keyword"
              placeholder="Bot 名称 / 用户名"
              clearable
              @keyup.enter="loadBots"
            />
            <n-select
              v-model:value="botQuery.status"
              :options="statusOptionsWithAll"
              clearable
              placeholder="状态"
              class="status-select"
            />
            <n-button @click="loadBots">查询</n-button>
            <n-button type="primary" @click="openBotModal()">新增Bot</n-button>
          </n-space>
          <n-data-table
            :columns="botColumns"
            :data="bots"
            :loading="botLoading"
            :pagination="botPagination"
            :row-key="(row) => row.id"
            :scroll-x="980"
            size="small"
            remote
          />
        </n-tab-pane>

        <n-tab-pane name="config" tab="配置">
          <n-spin :show="configLoading">
            <n-space vertical class="config-section">
              <n-form :model="telegramConfig" label-placement="left" label-width="150">
                <n-form-item label="Telegram App ID">
                  <n-input-number v-model:value="telegramConfig.appId" :min="0" class="w-full" />
                </n-form-item>
                <n-form-item label="Telegram App Hash">
                  <n-input v-model:value="telegramConfig.appHash" clearable />
                </n-form-item>
                <n-form-item label="代理地址">
                  <n-input
                    v-model:value="telegramConfig.proxyUrl"
                    placeholder="socks5://127.0.0.1:7890"
                    clearable
                  />
                </n-form-item>
                <n-form-item label="Bot运行模式">
                  <n-select
                    v-model:value="telegramConfig.botRuntimeMode"
                    :options="botRuntimeOptions"
                  />
                </n-form-item>
                <n-form-item label="Webhook 域名覆盖">
                  <n-input
                    v-model:value="telegramConfig.webhookBaseUrl"
                    :placeholder="systemDomain || '为空时使用系统配置域名'"
                    clearable
                  />
                </n-form-item>
                <n-form-item label="Webhook Secret">
                  <n-input v-model:value="telegramConfig.webhookSecret" clearable />
                </n-form-item>
                <n-form-item label="默认推送 Chat ID">
                  <n-input v-model:value="telegramConfig.defaultTargetChat" clearable />
                </n-form-item>
              </n-form>
              <n-space justify="end">
                <n-button @click="loadConfigs">重置</n-button>
                <n-button type="primary" :loading="configSaving" @click="saveConfigs"
                  >保存配置</n-button
                >
              </n-space>
            </n-space>
          </n-spin>
        </n-tab-pane>

        <n-tab-pane name="tgObserve" tab="推送观测">
          <TgObservePanel />
        </n-tab-pane>

        <n-tab-pane name="cloudResource" tab="云资源配置">
          <n-spin :show="cloudResourceLoading">
            <n-space vertical class="config-section">
              <CloudResourceConfig :model="cloudResourceConfig" />
              <n-space justify="end">
                <n-button @click="loadCloudResourceConfig">重置</n-button>
                <n-button
                  type="primary"
                  :loading="cloudResourceSaving"
                  @click="saveCloudResourceConfig"
                  >保存配置</n-button
                >
              </n-space>
            </n-space>
          </n-spin>
        </n-tab-pane>
      </n-tabs>
    </n-card>

    <n-modal
      v-model:show="tenantModalVisible"
      preset="dialog"
      :title="tenantForm.id ? '编辑账号归属' : '新增账号归属'"
      positive-text="保存"
      negative-text="取消"
      @positive-click="saveTenant"
    >
      <n-form :model="tenantForm" label-placement="left" label-width="90">
        <n-form-item label="管理员账号"
          ><n-input v-model:value="tenantForm.username" clearable placeholder="请输入管理员账号"
        /></n-form-item>
        <n-form-item label="登录密码"
          ><n-input
            v-model:value="tenantForm.password"
            type="password"
            show-password-on="click"
            clearable
            placeholder="新增时不填自动生成"
        /></n-form-item>
        <n-form-item label="状态"
          ><n-select v-model:value="tenantForm.status" :options="statusOptions"
        /></n-form-item>
        <n-form-item label="备注"
          ><n-input
            v-model:value="tenantForm.remark"
            type="textarea"
            :autosize="{ minRows: 2, maxRows: 4 }"
        /></n-form-item>
      </n-form>
    </n-modal>

    <n-modal
      v-model:show="accountModalVisible"
      preset="dialog"
      title="账号"
      positive-text="保存"
      negative-text="取消"
      @positive-click="saveAccount"
    >
      <n-form :model="accountForm" label-placement="left" label-width="110">
        <n-form-item label="账号归属"
          ><n-select v-model:value="accountForm.tenantId" :options="tenantOptions" filterable
        /></n-form-item>
        <n-form-item label="账号类型"
          ><n-select v-model:value="accountForm.accountType" :options="accountTypeOptions"
        /></n-form-item>
        <n-form-item v-if="accountForm.accountType === 'uploader'" label="管理员账号">
          <n-select
            v-model:value="accountForm.parentId"
            :options="adminAccountOptions"
            filterable
            clearable
          />
        </n-form-item>
        <n-form-item label="登录账号"
          ><n-input v-model:value="accountForm.username" clearable
        /></n-form-item>
        <n-form-item label="登录密码"
          ><n-input
            v-model:value="accountForm.password"
            type="password"
            show-password-on="click"
            clearable
            placeholder="新增为空自动生成，编辑为空不修改"
        /></n-form-item>
        <n-form-item label="账号名称"
          ><n-input v-model:value="accountForm.nickname" clearable
        /></n-form-item>
        <n-form-item label="每日额度"
          ><n-input-number v-model:value="accountForm.dailyPublishLimit" :min="0" class="w-full"
        /></n-form-item>
        <n-form-item label="直接发布"
          ><n-switch
            v-model:value="accountForm.canDirectPublish"
            :checked-value="1"
            :unchecked-value="0"
        /></n-form-item>
        <n-form-item label="状态"
          ><n-select v-model:value="accountForm.status" :options="statusOptions"
        /></n-form-item>
        <n-form-item label="备注"
          ><n-input
            v-model:value="accountForm.remark"
            type="textarea"
            :autosize="{ minRows: 2, maxRows: 4 }"
        /></n-form-item>
      </n-form>
    </n-modal>

    <n-modal
      v-model:show="resetPasswordVisible"
      preset="dialog"
      title="重置密码"
      positive-text="确认重置"
      negative-text="取消"
      @positive-click="resetPassword"
    >
      <n-form :model="resetPasswordForm" label-placement="left" label-width="90">
        <n-form-item label="账号"
          ><n-input v-model:value="resetPasswordForm.username" disabled
        /></n-form-item>
        <n-form-item label="新密码">
          <n-input
            v-model:value="resetPasswordForm.password"
            type="password"
            show-password-on="click"
            clearable
            placeholder="不填写则自动生成随机密码"
          />
        </n-form-item>
      </n-form>
    </n-modal>

    <n-modal
      v-model:show="resetPasswordResultVisible"
      preset="dialog"
      title="密码已重置"
      positive-text="复制密码"
      negative-text="确定"
      @positive-click="copyResetPassword"
    >
      <n-space vertical>
        <div>请保存以下新密码，关闭后将不再显示。</div>
        <n-input v-model:value="resetPasswordResult.password" readonly />
      </n-space>
    </n-modal>

    <n-modal
      v-model:show="botModalVisible"
      preset="dialog"
      title="Bot配置"
      positive-text="保存"
      negative-text="取消"
      @positive-click="saveBot"
    >
      <n-form :model="botForm" label-placement="left" label-width="100">
        <n-form-item label="账号归属"
          ><n-select
            v-model:value="botForm.tenantId"
            :options="botTenantOptions"
            filterable
            clearable
            placeholder="不选表示全局默认"
        /></n-form-item>
        <n-form-item label="Bot名称"
          ><n-input v-model:value="botForm.botName" clearable
        /></n-form-item>
        <n-form-item label="Bot Token"
          ><n-input
            v-model:value="botForm.botToken"
            type="password"
            show-password-on="click"
            clearable
        /></n-form-item>
        <n-form-item label="状态"
          ><n-select v-model:value="botForm.status" :options="statusOptions"
        /></n-form-item>
        <n-form-item label="备注"
          ><n-input
            v-model:value="botForm.remark"
            type="textarea"
            :autosize="{ minRows: 2, maxRows: 4 }"
        /></n-form-item>
      </n-form>
    </n-modal>

    <n-modal
      v-model:show="tagModalVisible"
      preset="dialog"
      title="标签"
      positive-text="保存"
      negative-text="取消"
      @positive-click="saveTag"
    >
      <n-form :model="tagForm" label-placement="left" label-width="90">
        <n-form-item label="标签名称"
          ><n-input
            v-model:value="tagForm.name"
            clearable
            placeholder="请输入标签名称，多个标签用逗号分隔"
        /></n-form-item>
        <n-form-item label="审核状态"
          ><n-select v-model:value="tagForm.reviewStatus" :options="reviewStatusOptions"
        /></n-form-item>
        <n-form-item label="状态"
          ><n-select v-model:value="tagForm.status" :options="statusOptions"
        /></n-form-item>
      </n-form>
    </n-modal>
  </div>
</template>

<script lang="ts" setup>
  import { computed, h, onMounted, reactive, ref } from 'vue';
  import { NButton, NPopover, NSpace, NTag, useDialog, useMessage } from 'naive-ui';
  import CloudResourceConfig from './components/cloud-resource-config.vue';
  import DashboardPanel from './components/dashboard-panel.vue';
  import ImportTaskPanel from './components/import-task-panel.vue';
  import ProfilePanel from './components/profile-panel.vue';
  import TgObservePanel from './components/tg-observe-panel.vue';
  import {
    AccountDelete,
    AccountList,
    AccountResetPassword,
    AccountSave,
    BotDelete,
    BotList,
    BotSave,
    ConfigGet,
    ConfigUpdate,
    TenantDelete,
    TenantList,
    TenantSave,
    TaskCancel,
    TaskList,
    TaskSubmit,
    TagDelete,
    TagList,
    TagSave,
  } from '@/api/addons/youbanPublish';
  import { getConfig as getSysConfig } from '@/api/sys/config';

  const dialog = useDialog();
  const message = useMessage();
  const activeTabStorageKey = 'youban_publish_admin_active_tab';
  const activeTab = ref(sessionStorage.getItem(activeTabStorageKey) || 'dashboard');

  const statusOptions = [
    { label: '启用', value: 1 },
    { label: '停用', value: 2 },
  ];
  const statusOptionsWithAll = [{ label: '全部', value: 0 }, ...statusOptions];
  const accountTypeOptions = [
    { label: '管理员账号', value: 'admin' },
    { label: '上架账号', value: 'uploader' },
  ];
  const accountTypeOptionsWithAll = [{ label: '全部', value: '' }, ...accountTypeOptions];
  const taskStatusOptions = [
    { label: '草稿', value: 'draft' },
    { label: '待发布', value: 'pending' },
    { label: '发布中', value: 'publishing' },
    { label: '已发布', value: 'published' },
    { label: '失败', value: 'failed' },
    { label: '已取消', value: 'canceled' },
  ];
  const taskStatusOptionsWithAll = [{ label: '全部', value: '' }, ...taskStatusOptions];
  const reviewStatusOptions = [
    { label: '待审核', value: 'pending' },
    { label: '已通过', value: 'approved' },
    { label: '已驳回', value: 'rejected' },
  ];
  const reviewStatusOptionsWithAll = [{ label: '全部', value: '' }, ...reviewStatusOptions];
  const botRuntimeOptions = [
    { label: '自动', value: 'auto' },
    { label: 'Pull', value: 'pull' },
    { label: 'Webhook', value: 'webhook' },
  ];

  const tenants = ref<any[]>([]);
  const accounts = ref<any[]>([]);
  const tasks = ref<any[]>([]);
  const tags = ref<any[]>([]);
  const bots = ref<any[]>([]);

  const tenantLoading = ref(false);
  const accountLoading = ref(false);
  const taskLoading = ref(false);
  const tagLoading = ref(false);
  const botLoading = ref(false);
  const configLoading = ref(false);
  const configSaving = ref(false);
  const cloudResourceLoading = ref(false);
  const cloudResourceSaving = ref(false);

  const tenantModalVisible = ref(false);
  const accountModalVisible = ref(false);
  const resetPasswordVisible = ref(false);
  const resetPasswordResultVisible = ref(false);
  const botModalVisible = ref(false);
  const tagModalVisible = ref(false);

  const tenantQuery = reactive({ keyword: '', status: 0 });
  const accountQuery = reactive({
    tenantId: null as number | null,
    accountType: '',
    keyword: '',
    status: 0,
  });
  const taskQuery = reactive({ tenantId: null as number | null, status: '', keyword: '' });
  const tagQuery = reactive({ keyword: '', reviewStatus: '', status: 0 });
  const botQuery = reactive({ tenantId: null as number | null, keyword: '', status: 0 });

  const tenantPagination = createPagination(loadTenants);
  const accountPagination = createPagination(loadAccounts);
  const taskPagination = createPagination(loadTasks);
  const tagPagination = createPagination(loadTags, 20);
  const botPagination = createPagination(loadBots);
  const systemDomain = ref('');
  const selectedTagRowKeys = ref<number[]>([]);
  const selectedTagRows = computed(() =>
    tags.value.filter((item) => selectedTagRowKeys.value.includes(item.id))
  );
  const selectedTagRejectRows = computed(() =>
    selectedTagRows.value.filter(
      (item) => item.reviewStatus !== 'approved' && item.reviewStatus !== 'rejected'
    )
  );

  const tenantOptions = computed(() =>
    tenants.value.map((item) => ({ label: accountOwnerName(item), value: item.id }))
  );
  const tenantOptionsWithAll = computed(() => [
    { label: '全部账号归属', value: null },
    ...tenantOptions.value,
  ]);
  const botTenantOptions = computed(() => [
    { label: '全局默认', value: 0 },
    ...tenantOptions.value,
  ]);
  const adminAccountOptions = computed(() =>
    accounts.value
      .filter((item) => item.accountType === 'admin' && item.tenantId === accountForm.tenantId)
      .map((item) => ({
        label: `${item.nickname || item.username} (${item.username})`,
        value: item.id,
      }))
  );

  const tenantForm = reactive(newTenantForm());
  const accountForm = reactive(newAccountForm());
  const resetPasswordForm = reactive({ id: 0, username: '', password: '' });
  const resetPasswordResult = reactive({ password: '' });
  const botForm = reactive(newBotForm());
  const tagForm = reactive(newTagForm());
  const telegramConfig = reactive(newTelegramConfig());
  const cloudResourceConfig = reactive(newCloudResourceConfig());

  function newTelegramConfig() {
    return {
      appId: 0,
      appHash: '',
      proxyUrl: '',
      botRuntimeMode: 'auto',
      webhookBaseUrl: '',
      webhookSecret: '',
      defaultTargetChat: '',
    };
  }

  function newCloudResourceConfig() {
    return {
      tencentVisionEnabled: 0,
      tencentCloudSite: 'mainland',
      tencentSecretId: '',
      tencentSecretKey: '',
      tencentRegion: 'ap-guangzhou',
      tencentBdaEndpoint: 'bda.tencentcloudapi.com',
      tencentIaiEndpoint: 'iai.tencentcloudapi.com',
      fapiHubEnabled: 0,
      fapiHubApiKey: '',
      fapiHubEndpoint: 'https://fapihub.com/v2/rembg/',
      fapiHubModel: 'falcon',
    };
  }

  const tenantColumns = [
    { title: 'ID', key: 'id', width: 80 },
    { title: '管理员账号', key: 'username', width: 180, render: (row) => row.username || '-' },
    { title: '状态', key: 'status', width: 100, render: (row) => renderStatus(row.status) },
    { title: '备注', key: 'remark', width: 260 },
    { title: '创建时间', key: 'createdAt', width: 170 },
    {
      title: '操作',
      key: 'actions',
      width: 160,
      fixed: 'right',
      render(row) {
        return h(
          NSpace,
          {},
          {
            default: () => [
              actionButton('编辑', () => openTenantModal(row)),
              actionButton('重置密码', () => openTenantResetPassword(row)),
              dangerButton('删除', () => deleteTenant(row.id)),
            ],
          }
        );
      },
    },
  ];

  const accountColumns = [
    { title: 'ID', key: 'id', width: 80 },
    { title: '账号归属', key: 'tenantId', width: 160, render: (row) => tenantName(row.tenantId) },
    {
      title: '类型',
      key: 'accountType',
      width: 110,
      render: (row) => accountTypeLabel(row.accountType),
    },
    { title: '账号', key: 'username', width: 160 },
    { title: '账号名称', key: 'nickname', width: 160 },
    { title: '每日额度', key: 'dailyPublishLimit', width: 100 },
    {
      title: '直接发布',
      key: 'canDirectPublish',
      width: 100,
      render: (row) => (row.canDirectPublish === 1 ? '是' : '否'),
    },
    { title: '状态', key: 'status', width: 100, render: (row) => renderStatus(row.status) },
    { title: '更新时间', key: 'updatedAt', width: 170 },
    {
      title: '操作',
      key: 'actions',
      width: 160,
      fixed: 'right',
      render(row) {
        return h(
          NSpace,
          {},
          {
            default: () => [
              actionButton('编辑', () => openAccountModal(row)),
              actionButton('重置密码', () => openAccountResetPassword(row)),
              dangerButton('删除', () => deleteAccount(row.id)),
            ],
          }
        );
      },
    },
  ];

  const taskColumns = [
    { title: 'ID', key: 'id', width: 80 },
    { title: '账号归属', key: 'tenantId', width: 150, render: (row) => tenantName(row.tenantId) },
    { title: '账号', key: 'accountUsername', width: 150 },
    { title: '标题', key: 'title', width: 220 },
    {
      title: '地区',
      key: 'city',
      width: 140,
      render: (row) => [row.province, row.city].filter(Boolean).join(' / ') || '-',
    },
    { title: '媒体', key: 'mediaCount', width: 80 },
    { title: '任务状态', key: 'status', width: 110, render: (row) => renderTaskStatus(row.status) },
    { title: 'TG状态', key: 'tgStatus', width: 100 },
    { title: '提交时间', key: 'submittedAt', width: 170 },
    {
      title: '操作',
      key: 'actions',
      width: 170,
      fixed: 'right',
      render(row) {
        return h(
          NSpace,
          {},
          {
            default: () => [
              actionButton('提交', () => submitTask(row.id)),
              dangerButton('取消', () => cancelTask(row.id)),
            ],
          }
        );
      },
    },
  ];

  const botColumns = [
    { title: 'ID', key: 'id', width: 80 },
    { title: '账号归属', key: 'tenantId', width: 160, render: (row) => tenantName(row.tenantId) },
    { title: 'Bot名称', key: 'botName', width: 180 },
    { title: '用户名', key: 'botUsername', width: 180 },
    { title: '状态', key: 'status', width: 100, render: (row) => renderStatus(row.status) },
    { title: '更新时间', key: 'updatedAt', width: 170 },
    {
      title: '操作',
      key: 'actions',
      width: 160,
      fixed: 'right',
      render(row) {
        return h(
          NSpace,
          {},
          {
            default: () => [
              actionButton('编辑', () => openBotModal(row)),
              dangerButton('删除', () => deleteBot(row.id)),
            ],
          }
        );
      },
    },
  ];

  const tagColumns = [
    { type: 'selection' },
    { title: 'ID', key: 'id', width: 80 },
    { title: '标签名称', key: 'name', width: 180 },
    {
      title: '审核状态',
      key: 'reviewStatus',
      width: 110,
      render: (row) => renderReviewStatus(row.reviewStatus),
    },
    { title: '状态', key: 'status', width: 100, render: (row) => renderStatus(row.status) },
    { title: '使用次数', key: 'useCount', width: 100 },
    { title: '创建者', key: 'creatorUsername', width: 160, render: (row) => renderTagCreator(row) },
    { title: '创建时间', key: 'createdAt', width: 170 },
    {
      title: '操作',
      key: 'actions',
      width: 220,
      fixed: 'right',
      render(row) {
        return h(
          NSpace,
          {},
          {
            default: () =>
              [
                row.reviewStatus !== 'approved'
                  ? actionButton('通过', () => updateTagReview(row, 'approved'))
                  : null,
                row.reviewStatus !== 'approved' && row.reviewStatus !== 'rejected'
                  ? actionButton('驳回', () => updateTagReview(row, 'rejected'))
                  : null,
                actionButton(row.status === 1 ? '停用' : '启用', () =>
                  updateTagStatus(row, row.status === 1 ? 2 : 1)
                ),
                dangerButton('删除', () => deleteTag(row.id)),
              ].filter(Boolean),
          }
        );
      },
    },
  ];

  onMounted(async () => {
    await loadTenants();
    if (activeTab.value !== 'tenants') {
      await loadCurrentTab(activeTab.value);
    }
  });

  function createPagination(loader: () => void, pageSize = 10) {
    const pagination: any = {
      page: 1,
      pageSize,
      itemCount: 0,
      showSizePicker: true,
      pageSizes: [10, 20, 50],
      onChange: (page) => {
        pagination.page = page;
        loader();
      },
      onUpdatePageSize: (pageSize) => {
        pagination.pageSize = pageSize;
        pagination.page = 1;
        loader();
      },
    };
    return pagination;
  }

  async function handleTabChange(tab: string) {
    rememberActiveTab(tab);
    await loadCurrentTab(tab);
  }

  function rememberActiveTab(tab = activeTab.value) {
    sessionStorage.setItem(activeTabStorageKey, tab);
  }

  async function loadCurrentTab(tab: string) {
    if (tab === 'dashboard') return;
    if (tab === 'importTasks') return;
    if (tab === 'accounts') await loadAccounts();
    if (tab === 'tasks') await loadTasks();
    if (tab === 'tags') await loadTags();
    if (tab === 'bots') await loadBots();
    if (tab === 'config') await loadConfigs();
    if (tab === 'tgObserve') return;
    if (tab === 'cloudResource') await loadCloudResourceConfig();
  }

  async function reloadActiveTabData() {
    rememberActiveTab();
    await loadCurrentTab(activeTab.value);
  }

  async function loadTenants() {
    tenantLoading.value = true;
    try {
      const res: any = await TenantList({
        ...tenantQuery,
        page: tenantPagination.page,
        perPage: tenantPagination.pageSize,
      });
      tenants.value = res?.list || [];
      tenantPagination.itemCount = res?.totalCount || res?.total || 0;
    } finally {
      tenantLoading.value = false;
    }
  }

  async function loadAccounts() {
    accountLoading.value = true;
    try {
      const res: any = await AccountList({
        ...accountQuery,
        page: accountPagination.page,
        perPage: accountPagination.pageSize,
      });
      accounts.value = res?.list || [];
      accountPagination.itemCount = res?.totalCount || res?.total || 0;
    } finally {
      accountLoading.value = false;
    }
  }

  async function loadTasks() {
    taskLoading.value = true;
    try {
      const res: any = await TaskList({
        ...taskQuery,
        page: taskPagination.page,
        perPage: taskPagination.pageSize,
      });
      tasks.value = res?.list || [];
      taskPagination.itemCount = res?.totalCount || res?.total || 0;
    } finally {
      taskLoading.value = false;
    }
  }

  async function loadBots() {
    botLoading.value = true;
    try {
      const res: any = await BotList({
        ...botQuery,
        page: botPagination.page,
        perPage: botPagination.pageSize,
      });
      bots.value = res?.list || [];
      botPagination.itemCount = res?.totalCount || res?.total || 0;
    } finally {
      botLoading.value = false;
    }
  }

  async function loadTags() {
    tagLoading.value = true;
    try {
      const res: any = await TagList({
        ...tagQuery,
        page: tagPagination.page,
        perPage: tagPagination.pageSize,
      });
      tags.value = res?.list || [];
      tagPagination.itemCount = res?.totalCount || res?.total || 0;
      selectedTagRowKeys.value = selectedTagRowKeys.value.filter((id) =>
        tags.value.some((item) => item.id === id)
      );
    } finally {
      tagLoading.value = false;
    }
  }

  async function loadConfigs() {
    configLoading.value = true;
    try {
      const [addonConfig, basicConfig] = await Promise.all([
        ConfigGet({ group: 'telegram' }),
        getSysConfig({ group: 'basic' }),
      ]);
      systemDomain.value = basicConfig?.list?.basicDomain || '';
      Object.assign(telegramConfig, newTelegramConfig(), addonConfig?.list || {});
    } finally {
      configLoading.value = false;
    }
  }

  async function saveConfigs() {
    configSaving.value = true;
    try {
      rememberActiveTab();
      await ConfigUpdate({ group: 'telegram', list: { ...telegramConfig } });
      message.success('配置已保存');
    } finally {
      configSaving.value = false;
    }
  }

  async function loadCloudResourceConfig() {
    cloudResourceLoading.value = true;
    try {
      const res: any = await ConfigGet({ group: 'cloudResource' });
      Object.assign(cloudResourceConfig, newCloudResourceConfig(), res?.list || {});
    } finally {
      cloudResourceLoading.value = false;
    }
  }

  async function saveCloudResourceConfig() {
    cloudResourceSaving.value = true;
    try {
      rememberActiveTab();
      await ConfigUpdate({ group: 'cloudResource', list: { ...cloudResourceConfig } });
      message.success('云资源配置已保存');
    } finally {
      cloudResourceSaving.value = false;
    }
  }

  function openTenantModal(row: any = null) {
    Object.assign(tenantForm, newTenantForm(), row || {});
    if (row?.id) {
      tenantForm.username = '';
      tenantForm.password = '';
    }
    tenantModalVisible.value = true;
  }

  async function saveTenant() {
    tenantForm.name = '';
    await TenantSave(tenantForm);
    message.success('账号归属已保存');
    await reloadActiveTabData();
  }

  function openAccountModal(row: any = null) {
    Object.assign(accountForm, newAccountForm(), row || {});
    if (!accountForm.tenantId && tenants.value.length === 1) {
      accountForm.tenantId = tenants.value[0].id;
    }
    accountModalVisible.value = true;
  }

  async function saveAccount() {
    await AccountSave(accountForm);
    message.success('账号已保存');
    await reloadActiveTabData();
  }

  function openTenantResetPassword(row: any) {
    if (!row.adminAccountId) {
      message.warning('该账号归属暂无管理员账号');
      return;
    }
    Object.assign(resetPasswordForm, {
      id: row.adminAccountId,
      username: row.username || '',
      password: '',
    });
    resetPasswordVisible.value = true;
  }

  function openAccountResetPassword(row: any) {
    Object.assign(resetPasswordForm, { id: row.id, username: row.username || '', password: '' });
    resetPasswordVisible.value = true;
  }

  async function resetPassword() {
    const res: any = await AccountResetPassword({
      id: resetPasswordForm.id,
      password: resetPasswordForm.password,
    });
    if (res?.password) {
      resetPasswordResult.password = res.password;
      resetPasswordResultVisible.value = true;
    } else {
      message.success('密码已重置');
    }
    resetPasswordVisible.value = false;
    await reloadActiveTabData();
  }

  async function copyResetPassword() {
    if (!resetPasswordResult.password) return;
    await navigator.clipboard.writeText(resetPasswordResult.password);
    message.success('密码已复制');
  }

  function openBotModal(row: any = null) {
    Object.assign(botForm, newBotForm(), row || {});
    botModalVisible.value = true;
  }

  async function saveBot() {
    await BotSave(botForm);
    message.success('Bot已保存');
    await reloadActiveTabData();
  }

  function openTagModal(row: any = null) {
    Object.assign(tagForm, newTagForm(), row || {});
    tagModalVisible.value = true;
  }

  async function saveTag() {
    await TagSave(tagForm);
    message.success('标签已保存');
    await reloadActiveTabData();
  }

  function deleteTenant(id: number) {
    confirmDelete('确认删除该账号归属？', async () => {
      await TenantDelete({ ids: [id] });
      await reloadActiveTabData();
    });
  }

  function deleteAccount(id: number) {
    confirmDelete('确认删除该上架系统账号？', async () => {
      await AccountDelete({ ids: [id] });
      await reloadActiveTabData();
    });
  }

  function deleteBot(id: number) {
    confirmDelete('确认删除该 Bot？', async () => {
      await BotDelete({ ids: [id] });
      await reloadActiveTabData();
    });
  }

  function deleteTag(id: number) {
    confirmDelete('确认删除该标签？', async () => {
      await TagDelete({ ids: [id] });
      selectedTagRowKeys.value = selectedTagRowKeys.value.filter((item) => item !== id);
      await reloadActiveTabData();
    });
  }

  async function updateTagReview(row: any, reviewStatus: string) {
    await TagSave({ name: row.name, reviewStatus, status: row.status || 1 });
    message.success('标签审核状态已更新');
    await reloadActiveTabData();
  }

  function batchDeleteTags() {
    const ids = [...selectedTagRowKeys.value];
    if (!ids.length) return;
    confirmDelete(`确认删除选中的 ${ids.length} 个标签？`, async () => {
      await TagDelete({ ids });
      selectedTagRowKeys.value = [];
      await reloadActiveTabData();
    });
  }

  async function batchUpdateTagReview(reviewStatus: string) {
    const rows = reviewStatus === 'rejected' ? selectedTagRejectRows.value : selectedTagRows.value;
    if (!rows.length) return;
    await Promise.all(
      rows.map((row) => TagSave({ name: row.name, reviewStatus, status: row.status || 1 }))
    );
    message.success(reviewStatus === 'approved' ? '标签已批量通过' : '标签已批量驳回');
    selectedTagRowKeys.value = [];
    await reloadActiveTabData();
  }

  function handleTagCheckedRowKeys(keys: Array<number | string>) {
    selectedTagRowKeys.value = keys.map((key) => Number(key)).filter((key) => key > 0);
  }

  async function updateTagStatus(row: any, status: number) {
    await TagSave({ name: row.name, reviewStatus: row.reviewStatus || 'pending', status });
    message.success('标签状态已更新');
    await reloadActiveTabData();
  }

  async function submitTask(id: number) {
    await TaskSubmit({ id });
    message.success('任务已提交');
    await reloadActiveTabData();
  }

  async function cancelTask(id: number) {
    await TaskCancel({ id });
    message.success('任务已取消');
    await reloadActiveTabData();
  }

  function confirmDelete(content: string, onConfirm: () => Promise<void>) {
    dialog.warning({
      title: '确认操作',
      content,
      positiveText: '确定',
      negativeText: '取消',
      onPositiveClick: async () => {
        await onConfirm();
        message.success('操作成功');
      },
    });
  }

  function actionButton(label: string, onClick: () => void) {
    return h(
      NButton,
      { size: 'small', quaternary: true, type: 'primary', onClick },
      { default: () => label }
    );
  }

  function dangerButton(label: string, onClick: () => void) {
    return h(
      NButton,
      { size: 'small', quaternary: true, type: 'error', onClick },
      { default: () => label }
    );
  }

  function renderStatus(status: number) {
    return h(
      NTag,
      { type: status === 1 ? 'success' : 'warning', bordered: false },
      { default: () => (status === 1 ? '启用' : '停用') }
    );
  }

  function renderTaskStatus(status: string) {
    const option = taskStatusOptions.find((item) => item.value === status);
    const type =
      status === 'published'
        ? 'success'
        : status === 'failed'
          ? 'error'
          : status === 'pending'
            ? 'warning'
            : 'default';
    return h(NTag, { type, bordered: false }, { default: () => option?.label || status || '-' });
  }

  function renderReviewStatus(status: string) {
    const option = reviewStatusOptions.find((item) => item.value === status);
    const type = status === 'approved' ? 'success' : status === 'rejected' ? 'error' : 'warning';
    return h(NTag, { type, bordered: false }, { default: () => option?.label || status || '-' });
  }

  function renderTagCreator(row: any) {
    if (!row.createdBy) return '系统';
    const username = row.creatorUsername || `账号 ${row.createdBy}`;
    return h(
      NPopover,
      { trigger: 'click' },
      {
        trigger: () =>
          h(NButton, { size: 'small', text: true, type: 'primary' }, { default: () => username }),
        default: () => `账号归属：${row.creatorTenantName || row.creatorTenantId || '-'}`,
      }
    );
  }

  function accountTypeLabel(type: string) {
    return accountTypeOptions.find((item) => item.value === type)?.label || type || '-';
  }

  function tenantName(id: number) {
    if (!id) return '全局默认';
    const tenant = tenants.value.find((item) => item.id === id);
    return tenant ? accountOwnerName(tenant) : `账号归属 ${id}`;
  }

  function accountOwnerName(item: any) {
    return item.username || item.name || `账号归属 ${item.id}`;
  }

  function newTenantForm() {
    return { id: 0, name: '', username: '', password: '', remark: '', status: 1 };
  }

  function newAccountForm() {
    return {
      id: 0,
      tenantId: null,
      parentId: null,
      accountType: 'admin',
      username: '',
      password: '',
      nickname: '',
      dailyPublishLimit: 0,
      canDirectPublish: 0,
      remark: '',
      status: 1,
    };
  }

  function newBotForm() {
    return { id: 0, tenantId: 0, botName: '', botToken: '', remark: '', status: 1 };
  }

  function newTagForm() {
    return { name: '', reviewStatus: 'approved', status: 1 };
  }
</script>

<style scoped>
  .toolbar {
    margin-bottom: 16px;
  }

  .status-select {
    width: 140px;
  }

  .tenant-select {
    width: 180px;
  }

  .config-section {
    max-width: 860px;
  }
</style>
