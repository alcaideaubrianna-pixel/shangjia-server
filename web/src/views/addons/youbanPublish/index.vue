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
            :scroll-x="1270"
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
            :scroll-x="1380"
            size="small"
            remote
          />
        </n-tab-pane>

        <n-tab-pane name="notice" tab="通知公告">
          <NoticePage />
        </n-tab-pane>

        <n-tab-pane name="channels" tab="上架频道">
          <n-space class="toolbar" align="center">
            <n-select
              v-model:value="channelQuery.tenantId"
              :options="tenantOptionsWithAll"
              clearable
              filterable
              placeholder="归属租户账号"
              class="tenant-select"
            />
            <n-input
              v-model:value="channelQuery.keyword"
              placeholder="频道名称 / 用户名 / Chat ID"
              clearable
              @keyup.enter="loadChannels"
            />
            <n-select
              v-model:value="channelQuery.status"
              :options="statusOptionsWithAll"
              clearable
              placeholder="状态"
              class="status-select"
            />
            <n-button @click="loadChannels">查询</n-button>
          </n-space>
          <n-data-table
            :columns="channelColumns"
            :data="channels"
            :loading="channelLoading"
            :pagination="channelPagination"
            :row-key="(row) => row.id"
            :scroll-x="1480"
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
            <n-button :loading="botRefreshing" @click="refreshBots()">刷新状态</n-button>
            <n-button type="primary" @click="openBotModal()">新增Bot</n-button>
          </n-space>
          <n-data-table
            :columns="botColumns"
            :data="bots"
            :loading="botLoading"
            :pagination="botPagination"
            :row-key="(row) => row.id"
            :scroll-x="1500"
            size="small"
            remote
          />
        </n-tab-pane>

        <n-tab-pane name="inviteRelations" tab="邀请关系">
          <n-space class="toolbar" align="center">
            <n-input
              v-model:value="inviteQuery.keyword"
              placeholder="邀请码 / 邀请人 / 使用账号"
              clearable
              @keyup.enter="loadInviteRelations"
            />
            <n-select
              v-model:value="inviteQuery.source"
              :options="inviteSourceOptionsWithAll"
              clearable
              placeholder="来源"
              class="status-select"
            />
            <n-select
              v-model:value="inviteQuery.status"
              :options="inviteStatusOptionsWithAll"
              clearable
              placeholder="状态"
              class="status-select"
            />
            <n-button @click="loadInviteRelations">查询</n-button>
          </n-space>
          <n-data-table
            :columns="inviteColumns"
            :data="inviteRelations"
            :loading="inviteLoading"
            :pagination="invitePagination"
            :row-key="(row) => row.id"
            :scroll-x="1680"
            size="small"
            remote
          />
        </n-tab-pane>

        <n-tab-pane name="member" tab="会员" display-directive="show">
          <MemberPanel ref="memberPanelRef" />
        </n-tab-pane>

        <n-tab-pane name="tgAccounts" tab="绑定的 TG 账号">
          <n-space class="toolbar" align="center">
            <n-input
              v-model:value="tgAccountQuery.keyword"
              placeholder="TG用户名 / 备注"
              clearable
              @keyup.enter="loadTgAccounts"
            />
            <n-select
              v-model:value="tgAccountQuery.status"
              :options="tgAccountStatusOptionsWithAll"
              clearable
              placeholder="状态"
              class="status-select"
            />
            <n-button @click="loadTgAccounts">查询</n-button>
          </n-space>
          <n-data-table
            :columns="tgAccountColumns"
            :data="tgAccounts"
            :loading="tgAccountLoading"
            :pagination="tgAccountPagination"
            :row-key="(row) => row.id"
            :scroll-x="1600"
            size="small"
            remote
          />
        </n-tab-pane>

        <n-tab-pane name="channelCaches" tab="群聊 / 频道">
          <ChannelMemberPanel ref="channelCachePanelRef" />
        </n-tab-pane>

        <n-tab-pane name="config" tab="配置">
          <n-spin :show="configLoading">
            <n-space vertical class="config-section">
              <section class="config-panel">
                <div class="config-panel-title">Telegram</div>
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
              </section>

              <section class="config-panel">
                <div class="config-panel-title">采集推送</div>
                <n-form :model="collectConfig" label-placement="left" label-width="150">
                  <n-form-item label="采集总开关">
                    <n-switch
                      v-model:value="collectConfig.collectEnabled"
                      :checked-value="1"
                      :unchecked-value="0"
                    />
                  </n-form-item>
                  <n-form-item label="实时推送延迟">
                    <n-input-number
                      v-model:value="collectConfig.realtimePushDelaySec"
                      :min="0"
                      :max="600"
                      :step="60"
                      class="w-full"
                    >
                      <template #suffix>秒</template>
                    </n-input-number>
                  </n-form-item>
                </n-form>
              </section>
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

        <n-tab-pane name="publishRecords" tab="发布记录">
          <PublishRecordPanel />
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
  import { computed, h, nextTick, onMounted, reactive, ref } from 'vue';
  import { NButton, NPopover, NSpace, NTag, useDialog, useMessage } from 'naive-ui';
  import ChannelMemberPanel from './components/channel-member-panel.vue';
  import CloudResourceConfig from './components/cloud-resource-config.vue';
  import DashboardPanel from './components/dashboard-panel.vue';
  import ImportTaskPanel from './components/import-task-panel.vue';
  import MemberPanel from './components/member-panel.vue';
  import ProfilePanel from './components/profile-panel.vue';
  import PublishRecordPanel from './components/publish-record-panel.vue';
  import NoticePage from '@/views/apply/notice/index.vue';
  import TgObservePanel from './components/tg-observe-panel.vue';
  import {
    AccountDelete,
    AccountList,
    AccountResetPassword,
    AccountSave,
    BotDelete,
    BotList,
    BotRefresh,
    BotSave,
    AdminInviteList,
    AdminTgAccountList,
    ConfigGet,
    ConfigUpdate,
    ChannelList,
    TenantDelete,
    TenantList,
    TenantSave,
    TagDelete,
    TagList,
    TagSave,
  } from '@/api/addons/youbanPublish';
  import { getConfig as getSysConfig } from '@/api/sys/config';

  const dialog = useDialog();
  const message = useMessage();
  const activeTabStorageKey = 'youban_publish_admin_active_tab';
  const storedActiveTab = sessionStorage.getItem(activeTabStorageKey);
  const activeTab = ref(storedActiveTab === 'tasks' ? 'profiles' : storedActiveTab || 'dashboard');
  const memberPanelRef = ref<InstanceType<typeof MemberPanel> | null>(null);
  const channelCachePanelRef = ref<InstanceType<typeof ChannelMemberPanel> | null>(null);

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
  const channels = ref<any[]>([]);
  const tags = ref<any[]>([]);
  const bots = ref<any[]>([]);

  const tenantLoading = ref(false);
  const accountLoading = ref(false);
  const channelLoading = ref(false);
  const tagLoading = ref(false);
  const botLoading = ref(false);
  const botRefreshing = ref(false);
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
  const channelQuery = reactive({
    tenantId: null as number | null,
    keyword: '',
    status: 0,
    publishDirection: 'up',
  });
  const tagQuery = reactive({ keyword: '', reviewStatus: '', status: 0 });
  const botQuery = reactive({ tenantId: null as number | null, keyword: '', status: 0 });
  const inviteQuery = reactive({ keyword: '', source: '', status: '' });
  const tgAccountQuery = reactive({ keyword: '', status: '' });

  const tenantPagination = createPagination(loadTenants);
  const accountPagination = createPagination(loadAccounts);
  const channelPagination = createPagination(loadChannels, 20);
  const tagPagination = createPagination(loadTags, 20);
  const botPagination = createPagination(loadBots);
  const invitePagination = createPagination(loadInviteRelations, 20);
  const tgAccountPagination = createPagination(loadTgAccounts, 20);
  const systemDomain = ref('');
  const botWebhookConfigLoaded = ref(false);
  const effectiveWebhookBaseUrl = computed(() =>
    normalizeWebhookBaseUrl(telegramConfig.webhookBaseUrl || systemDomain.value)
  );
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
  const inviteSourceOptions = [
    { label: '网页', value: 'web' },
    { label: '机器人', value: 'bot' },
  ];
  const inviteSourceOptionsWithAll = [{ label: '全部', value: '' }, ...inviteSourceOptions];
  const inviteStatusOptions = [
    { label: '有效', value: 'active' },
    { label: '已使用', value: 'used' },
    { label: '已过期', value: 'expired' },
  ];
  const inviteStatusOptionsWithAll = [{ label: '全部', value: '' }, ...inviteStatusOptions];
  const tgAccountStatusOptions = [
    { label: '待授权', value: 'pending' },
    { label: '扫描中', value: 'scanning' },
    { label: '需密码', value: 'password_required' },
    { label: '已授权', value: 'authorized' },
    { label: '已过期', value: 'expired' },
    { label: '失败', value: 'failed' },
  ];
  const tgAccountStatusOptionsWithAll = [{ label: '全部', value: '' }, ...tgAccountStatusOptions];
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
  const collectConfig = reactive(newCollectConfig());
  const cloudResourceConfig = reactive(newCloudResourceConfig());
  const inviteRelations = ref<any[]>([]);
  const inviteLoading = ref(false);
  const tgAccounts = ref<any[]>([]);
  const tgAccountLoading = ref(false);

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

  function newCollectConfig() {
    return {
      collectEnabled: 1,
      realtimePushDelaySec: 600,
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
    { title: '会员等级', key: 'vipLevel', width: 120, render: (row) => renderVipLevel(row) },
    {
      title: '会员到期',
      key: 'vipExpiredAt',
      width: 170,
      render: (row) => row.vipExpiredAt || '-',
    },
    { title: '状态', key: 'status', width: 100, render: (row) => renderStatus(row.status) },
    { title: '备注', key: 'remark', width: 260 },
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
            default: () => [
              actionButton('编辑', () => openTenantModal(row)),
              actionButton('会员配置', () => openTenantVipConfig(row)),
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
      width: 240,
      fixed: 'right',
      render(row) {
        return h(
          NSpace,
          {},
          {
            default: () => [
              actionButton('编辑', () => openAccountModal(row)),
              actionButton('会员记录', () => openAccountVipRecords(row)),
              actionButton('重置密码', () => openAccountResetPassword(row)),
              dangerButton('删除', () => deleteAccount(row.id)),
            ],
          }
        );
      },
    },
  ];

  const channelColumns = [
    { title: 'ID', key: 'id', width: 80 },
    {
      title: '归属租户账号',
      key: 'tenantUsername',
      width: 170,
      render: (row) => row.tenantUsername || tenantName(row.tenantId),
    },
    { title: '频道名称', key: 'channelTitle', width: 220 },
    {
      title: '频道用户名',
      key: 'channelUsername',
      width: 180,
      render: (row) => (row.channelUsername ? `@${row.channelUsername}` : '-'),
    },
    { title: '目标 Chat ID', key: 'targetChatId', width: 180 },
    {
      title: 'TG账号',
      key: 'tgAccountName',
      width: 180,
      render: (row) => row.tgAccountName || row.tgAccountId || '-',
    },
    {
      title: '循环上架',
      key: 'cyclePublishEnabled',
      width: 100,
      render: (row) => (row.cyclePublishEnabled === 1 ? '开启' : '关闭'),
    },
    { title: '循环天数', key: 'cyclePublishDays', width: 100 },
    { title: '状态', key: 'status', width: 100, render: (row) => renderStatus(row.status) },
    { title: '更新时间', key: 'updatedAt', width: 180 },
  ];

  const botColumns = [
    { title: 'ID', key: 'id', width: 80 },
    { title: '账号归属', key: 'tenantId', width: 160, render: (row) => tenantName(row.tenantId) },
    { title: 'Bot名称', key: 'botName', width: 180 },
    { title: '用户名', key: 'botUsername', width: 180 },
    { title: '状态', key: 'status', width: 100, render: (row) => renderStatus(row.status) },
    {
      title: 'Webhook',
      key: 'webhookStatus',
      width: 130,
      render: (row) => renderMiniTag(botWebhookStatusLabel(row), botWebhookStatusType(row)),
    },
    {
      title: 'Webhook地址',
      key: 'webhookUrl',
      width: 360,
      render: (row) => renderWebhookUrl(row),
    },
    { title: '更新时间', key: 'updatedAt', width: 170 },
    {
      title: '操作',
      key: 'actions',
      width: 210,
      fixed: 'right',
      render(row) {
        return h(
          NSpace,
          {},
          {
            default: () => [
              actionButton('编辑', () => openBotModal(row)),
              actionButton('重启', () => refreshBots(row.id)),
              dangerButton('删除', () => deleteBot(row.id)),
            ],
          }
        );
      },
    },
  ];

  const inviteColumns = [
    { title: 'ID', key: 'id', width: 80 },
    { title: '邀请码', key: 'code', width: 140 },
    {
      title: '来源',
      key: 'source',
      width: 100,
      render: (row: any) => renderMiniTag(inviteSourceLabel(row.source), 'default'),
    },
    {
      title: '邀请人租户',
      key: 'inviterTenantName',
      width: 160,
      render: (row: any) => row.inviterTenantName || row.inviterTenantId || '-',
    },
    {
      title: '邀请人账号',
      key: 'inviterUsername',
      width: 160,
      render: (row: any) => row.inviterUsername || '-',
    },
    {
      title: '邀请人昵称',
      key: 'inviterNickname',
      width: 160,
      render: (row: any) => row.inviterNickname || '-',
    },
    {
      title: '使用租户',
      key: 'usedTenantName',
      width: 160,
      render: (row: any) => row.usedTenantName || row.usedTenantId || '-',
    },
    {
      title: '使用账号',
      key: 'usedAccountName',
      width: 160,
      render: (row: any) => row.usedAccountName || '-',
    },
    {
      title: '状态',
      key: 'status',
      width: 100,
      render: (row: any) =>
        renderMiniTag(inviteStatusLabel(row.status), inviteStatusType(row.status)),
    },
    { title: '过期时间', key: 'expiresAt', width: 180, render: (row: any) => row.expiresAt || '-' },
    { title: '使用时间', key: 'usedAt', width: 180, render: (row: any) => row.usedAt || '-' },
    { title: '创建时间', key: 'createdAt', width: 180, render: (row: any) => row.createdAt || '-' },
  ];

  const tgAccountColumns = [
    { title: 'ID', key: 'id', width: 80 },
    {
      title: '账号归属',
      key: 'tenantId',
      width: 160,
      render: (row: any) => tenantName(row.tenantId),
    },
    {
      title: '显示名称',
      key: 'displayName',
      width: 180,
      render: (row: any) => row.displayName || row.nickname || '-',
    },
    {
      title: 'TG用户ID',
      key: 'telegramUserId',
      width: 180,
      render: (row: any) => row.telegramUserId || '-',
    },
    {
      title: 'TG用户名',
      key: 'telegramUsername',
      width: 180,
      render: (row: any) => row.telegramUsername || '-',
    },
    {
      title: '状态',
      key: 'status',
      width: 110,
      render: (row: any) =>
        renderMiniTag(tgAccountStatusLabel(row.status), tgAccountStatusType(row.status)),
    },
    {
      title: '最后授权',
      key: 'lastLoginAt',
      width: 180,
      render: (row: any) => row.lastLoginAt || '-',
    },
    { title: '更新时间', key: 'updatedAt', width: 180, render: (row: any) => row.updatedAt || '-' },
    {
      title: '操作',
      key: 'actions',
      width: 180,
      fixed: 'right',
      render(row: any) {
        return h(
          NSpace,
          {},
          {
            default: () => [actionButton('查看频道', () => openChannelCaches(row))],
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
    const pagination: any = reactive({
      page: 1,
      pageSize,
      itemCount: 0,
      showSizePicker: true,
      pageSizes: [10, 20, 50],
      onUpdatePage: (page) => {
        pagination.page = page;
        loader();
      },
      onUpdatePageSize: (pageSize) => {
        pagination.pageSize = pageSize;
        pagination.page = 1;
        loader();
      },
    });
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
    if (tab === 'channels') await loadChannels();
    if (tab === 'tags') await loadTags();
    if (tab === 'bots') await loadBots();
    if (tab === 'inviteRelations') await loadInviteRelations();
    if (tab === 'member') return;
    if (tab === 'tgAccounts') await loadTgAccounts();
    if (tab === 'channelCaches') return;
    if (tab === 'config') await loadConfigs();
    if (tab === 'tgObserve') return;
    if (tab === 'publishRecords') return;
    if (tab === 'cloudResource') await loadCloudResourceConfig();
  }

  async function reloadActiveTabData() {
    rememberActiveTab();
    await loadCurrentTab(activeTab.value);
  }

  function openTenantVipConfig(row: any) {
    memberPanelRef.value?.openTenantVipModal(row.id);
  }

  function openAccountVipRecords(row: any) {
    activeTab.value = 'member';
    rememberActiveTab('member');
    memberPanelRef.value?.openVipRecords(row.tenantId || row.id || 0);
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

  async function loadChannels() {
    channelLoading.value = true;
    try {
      const res: any = await ChannelList({
        ...channelQuery,
        page: channelPagination.page,
        perPage: channelPagination.pageSize,
      });
      channels.value = res?.list || [];
      channelPagination.itemCount = res?.totalCount || res?.total || 0;
    } finally {
      channelLoading.value = false;
    }
  }

  async function loadBots() {
    botLoading.value = true;
    try {
      await ensureBotWebhookConfig();
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

  async function loadInviteRelations() {
    inviteLoading.value = true;
    try {
      const res: any = await AdminInviteList({
        ...inviteQuery,
        page: invitePagination.page,
        perPage: invitePagination.pageSize,
      });
      inviteRelations.value = res?.list || [];
      invitePagination.itemCount = res?.totalCount || res?.total || 0;
    } finally {
      inviteLoading.value = false;
    }
  }

  async function loadTgAccounts() {
    tgAccountLoading.value = true;
    try {
      const res: any = await AdminTgAccountList({
        ...tgAccountQuery,
        page: tgAccountPagination.page,
        perPage: tgAccountPagination.pageSize,
      });
      tgAccounts.value = res?.list || [];
      tgAccountPagination.itemCount = res?.totalCount || res?.total || 0;
    } finally {
      tgAccountLoading.value = false;
    }
  }

  async function openChannelCaches(row: any) {
    activeTab.value = 'channelCaches';
    rememberActiveTab('channelCaches');
    await nextTick();
    await channelCachePanelRef.value?.openForTgAccount?.(row.id);
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
      const [telegramRes, collectRes, basicConfig] = await Promise.all([
        ConfigGet({ group: 'telegram' }),
        ConfigGet({ group: 'collect' }),
        getSysConfig({ group: 'basic' }),
      ]);
      applyTelegramConfig(telegramRes?.list || {}, basicConfig?.list?.basicDomain || '');
      botWebhookConfigLoaded.value = true;
      Object.assign(collectConfig, newCollectConfig(), collectRes?.list || {});
    } finally {
      configLoading.value = false;
    }
  }

  async function ensureBotWebhookConfig() {
    if (botWebhookConfigLoaded.value) return;
    const [telegramRes, basicConfig] = await Promise.all([
      ConfigGet({ group: 'telegram' }),
      getSysConfig({ group: 'basic' }),
    ]);
    applyTelegramConfig(telegramRes?.list || {}, basicConfig?.list?.basicDomain || '');
    botWebhookConfigLoaded.value = true;
  }

  function applyTelegramConfig(config: any, basicDomain: string) {
    systemDomain.value = normalizeWebhookBaseUrl(basicDomain || '');
    Object.assign(telegramConfig, newTelegramConfig(), config || {});
    if (!telegramConfig.webhookBaseUrl && systemDomain.value) {
      telegramConfig.webhookBaseUrl = systemDomain.value;
    }
  }

  async function saveConfigs() {
    configSaving.value = true;
    try {
      rememberActiveTab();
      await Promise.all([
        ConfigUpdate({ group: 'telegram', list: { ...telegramConfig } }),
        ConfigUpdate({ group: 'collect', list: { ...collectConfig } }),
      ]);
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

  async function refreshBots(id?: number) {
    const ids = id ? [id] : bots.value.map((item) => item.id).filter(Boolean);
    if (ids.length === 0) {
      message.warning('暂无可刷新的Bot');
      return;
    }
    botRefreshing.value = true;
    try {
      const res: any = await BotRefresh({ ids });
      const failed = (res?.list || []).filter((item) => item.errorMessage);
      if (failed.length > 0) {
        message.warning(`刷新完成，失败 ${failed.length} 个`);
      } else {
        message.success('Bot状态已刷新');
      }
      await loadBots();
    } finally {
      botRefreshing.value = false;
    }
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

  function normalizeWebhookBaseUrl(url: string) {
    let value = String(url || '')
      .trim()
      .replace(/\/+$/, '');
    if (!value) return '';
    if (!/^https?:\/\//i.test(value)) {
      value = `https://${value}`;
    }
    return value;
  }

  function buildBotWebhookUrl(row: any) {
    if (row?.webhookUrl) return row.webhookUrl;
    if (!row?.id || !effectiveWebhookBaseUrl.value) return '';
    return `${effectiveWebhookBaseUrl.value}/api/youban_publish/telegram/webhook?botId=${row.id}`;
  }

  function botWebhookRuntimeEnabled() {
    const mode = telegramConfig.botRuntimeMode || 'auto';
    return mode === 'webhook' || mode === 'auto';
  }

  function botWebhookStatusLabel(row: any) {
    if (row?.webhookStatusLabel) return row.webhookStatusLabel;
    if (row?.webhookStatus === 'enabled') return '已启用';
    if (row?.webhookStatus === 'disabled') return '未启用';
    if (row?.webhookStatus === 'error') return '异常';
    if (row?.webhookStatus === 'not_configured') return '未配置';
    if (row?.status !== 1) return 'Bot停用';
    if (!botWebhookRuntimeEnabled()) return 'Pull模式';
    if (!effectiveWebhookBaseUrl.value) return '未配置';
    return '已配置';
  }

  function botWebhookStatusType(row: any) {
    if (row?.webhookStatus === 'enabled') return 'success';
    if (row?.webhookStatus === 'disabled') return 'warning';
    if (row?.webhookStatus === 'error') return 'error';
    if (row?.webhookStatus === 'not_configured') return 'error';
    if (row?.status !== 1) return 'warning';
    if (!botWebhookRuntimeEnabled()) return 'info';
    if (!effectiveWebhookBaseUrl.value) return 'error';
    return 'success';
  }

  function renderWebhookUrl(row: any) {
    const url = buildBotWebhookUrl(row);
    if (!url) return '-';
    return h(
      NPopover,
      { trigger: 'hover' },
      {
        trigger: () => h('span', { class: 'webhook-url-text' }, url),
        default: () => url,
      }
    );
  }

  function renderStatus(status: number) {
    return h(
      NTag,
      { type: status === 1 ? 'success' : 'warning', bordered: false },
      { default: () => (status === 1 ? '启用' : '停用') }
    );
  }

  function renderVipLevel(row: any) {
    const level = Number(row.vipLevel || 0);
    const expiredAt = row.vipExpiredAt ? new Date(row.vipExpiredAt).getTime() : 0;
    const active = level > 0 && row.vipStatus === 1 && expiredAt > Date.now();
    const label = level === 2 ? 'SVIP计划' : level === 1 ? 'VIP计划' : '免费计划';
    return h(
      NTag,
      { type: active ? 'success' : 'default', bordered: false, size: 'small' },
      { default: () => label }
    );
  }

  function renderMiniTag(
    label: string,
    type: 'default' | 'success' | 'info' | 'warning' | 'error'
  ) {
    return h(NTag, { type, bordered: false, size: 'small' }, { default: () => label });
  }

  function inviteSourceLabel(source: string) {
    return inviteSourceOptions.find((item) => item.value === source)?.label || source || '-';
  }

  function inviteStatusLabel(status: string) {
    return inviteStatusOptions.find((item) => item.value === status)?.label || status || '-';
  }

  function inviteStatusType(status: string) {
    if (status === 'active') return 'success';
    if (status === 'used') return 'info';
    return 'warning';
  }

  function tgAccountStatusLabel(status: string) {
    return tgAccountStatusOptions.find((item) => item.value === status)?.label || status || '-';
  }

  function tgAccountStatusType(status: string) {
    if (status === 'authorized') return 'success';
    if (status === 'pending' || status === 'scanning') return 'info';
    if (status === 'password_required') return 'warning';
    if (status === 'expired' || status === 'failed') return 'error';
    return 'default';
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

  .webhook-url-text {
    display: inline-block;
    max-width: 330px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    vertical-align: middle;
  }

  .config-section {
    max-width: 860px;
  }

  .config-panel {
    border: 1px solid var(--border-color, #e5e7eb);
    border-radius: 6px;
    padding: 16px 18px 2px;
  }

  .config-panel-title {
    margin-bottom: 14px;
    font-size: 15px;
    font-weight: 600;
    line-height: 1.2;
  }
</style>
