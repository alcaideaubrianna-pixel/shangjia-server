<template>
  <n-tabs v-model:value="activeTab" type="line" animated>
    <n-tab-pane name="tasks" tab="导入任务">
      <n-space class="toolbar" align="center">
        <n-select
          v-model:value="query.tenantId"
          :options="tenantOptionsWithAll"
          clearable
          filterable
          placeholder="账号归属"
          class="tenant-select"
          @update:value="handleQueryTenantChange"
        />
        <n-select
          v-model:value="query.accountId"
          :options="queryAccountOptionsWithAll"
          clearable
          filterable
          placeholder="上架账号"
          class="tenant-select"
        />
        <n-input
          v-model:value="query.keyword"
          placeholder="域名 / 账号 / 备注"
          clearable
          @keyup.enter="loadTasks"
        />
        <n-button @click="loadTasks">查询</n-button>
        <n-button type="primary" @click="openCreateModal">新建导入</n-button>
        <n-button @click="loadTasks">刷新</n-button>
      </n-space>
      <n-data-table
        :columns="taskColumns"
        :data="tasks"
        :loading="loading"
        :pagination="taskPagination"
        :row-key="(row) => row.id"
        :scroll-x="1420"
        size="small"
        remote
      />
    </n-tab-pane>

    <n-tab-pane name="runs" tab="导入记录">
      <div class="run-filter">
        <div class="run-filter-grid">
          <n-select
            v-model:value="runQuery.tenantId"
            :options="tenantOptionsWithAll"
            clearable
            filterable
            placeholder="账号归属"
            @update:value="handleRunTenantChange"
          />
          <n-select
            v-model:value="runQuery.accountId"
            :options="runAccountOptionsWithAll"
            clearable
            filterable
            placeholder="上架账号"
          />
          <n-select
            v-model:value="runQuery.runType"
            :options="runTypeOptionsWithAll"
            clearable
            placeholder="类型"
          />
          <n-select
            v-model:value="runQuery.status"
            :options="statusOptionsWithAll"
            clearable
            placeholder="状态"
          />
          <n-input
            v-model:value="runQuery.keyword"
            placeholder="域名 / 账号"
            clearable
            @keyup.enter="loadRuns"
          />
          <n-space class="filter-actions" justify="end">
            <n-button @click="loadRuns">查询</n-button>
            <n-button @click="loadRuns">刷新</n-button>
          </n-space>
        </div>
      </div>
      <n-data-table
        :columns="runColumns"
        :data="runs"
        :loading="runLoading"
        :pagination="runPagination"
        :row-key="(row) => row.id"
        :scroll-x="2140"
        size="small"
        remote
      />
    </n-tab-pane>
  </n-tabs>

  <n-modal
    v-model:show="modalVisible"
    preset="dialog"
    :title="taskModalTitle"
    positive-text="保存"
    negative-text="取消"
    @positive-click="createTask"
  >
    <n-form :model="form" label-placement="left" label-width="110">
      <n-form-item label="账号归属">
        <n-select
          v-model:value="form.tenantId"
          :options="tenantOptions"
          filterable
          placeholder="请选择账号归属"
          @update:value="handleFormTenantChange"
        />
      </n-form-item>
      <n-form-item label="导入账号">
        <n-select
          v-model:value="form.accountId"
          :options="formAccountOptions"
          filterable
          placeholder="请选择导入到哪个上架账号"
        />
      </n-form-item>
      <n-form-item label="旧站域名">
        <n-input v-model:value="form.baseUrl" clearable placeholder="https://example.com" />
      </n-form-item>
      <n-form-item label="服务器 IP">
        <n-input
          v-model:value="form.serverIp"
          clearable
          placeholder="DNS失效时填写，例如 154.26.238.214"
        />
      </n-form-item>
      <n-form-item label="旧站账号">
        <n-input v-model:value="form.username" clearable />
      </n-form-item>
      <n-form-item label="旧站密码">
        <n-input
          v-model:value="form.password"
          type="password"
          show-password-on="click"
          :placeholder="form.id ? '留空表示不修改密码' : ''"
        />
      </n-form-item>
      <n-form-item label="旧站Cookie">
        <n-input
          v-model:value="form.legacyCookie"
          type="textarea"
          :autosize="{ minRows: 3, maxRows: 6 }"
          placeholder="可填写 Cookie 或完整 Set-Cookie，留空表示不修改"
        />
      </n-form-item>
      <n-form-item label="测试数量">
        <n-input-number v-model:value="form.limitCount" :min="0" :max="100000" class="w-full" />
      </n-form-item>
      <n-form-item label="每页数量">
        <n-input-number v-model:value="form.perPage" :min="1" :max="12" class="w-full" />
      </n-form-item>
      <n-form-item label="媒体并发">
        <n-input-number v-model:value="form.mediaConcurrency" :min="1" :max="20" class="w-full" />
      </n-form-item>
      <n-form-item label="导入方式">
        <n-radio-group v-model:value="form.importMode">
          <n-space>
            <n-radio value="incremental">增量更新</n-radio>
            <n-radio value="overwrite">覆盖更新</n-radio>
          </n-space>
        </n-radio-group>
      </n-form-item>
      <n-form-item label="关联频道">
        <n-select
          v-model:value="form.channelIds"
          :options="formChannelOptions"
          multiple
          filterable
          clearable
          placeholder="可选，导入后用于TG消息匹配和资料频道归属"
        />
      </n-form-item>
      <n-form-item label="代理池">
        <n-space vertical class="w-full">
          <n-switch v-model:value="proxyEnabled" />
          <n-input
            v-model:value="form.proxyPool"
            type="textarea"
            :autosize="{ minRows: 3, maxRows: 6 }"
            placeholder="一行一个代理地址"
          />
        </n-space>
      </n-form-item>
      <n-form-item label="备注">
        <n-input
          v-model:value="form.remark"
          type="textarea"
          :autosize="{ minRows: 2, maxRows: 4 }"
        />
      </n-form-item>
    </n-form>
  </n-modal>

  <n-modal
    v-model:show="runModalVisible"
    preset="dialog"
    title="创建导入记录"
    positive-text="创建并入队"
    negative-text="取消"
    @positive-click="createRun"
  >
    <n-form :model="runForm" label-placement="left" label-width="90">
      <n-form-item label="执行类型">
        <n-radio-group v-model:value="runForm.runType">
          <n-space>
            <n-radio value="import">导入</n-radio>
            <n-radio value="repair">补全</n-radio>
            <n-radio value="scan">仅扫描</n-radio>
          </n-space>
        </n-radio-group>
      </n-form-item>
      <n-form-item label="扫描范围">
        <n-radio-group v-model:value="runForm.scanMode">
          <n-space>
            <n-radio value="recent">最近N个</n-radio>
            <n-radio value="all">全量扫描</n-radio>
          </n-space>
        </n-radio-group>
      </n-form-item>
      <n-form-item v-if="runForm.scanMode === 'recent'" label="最近数量">
        <n-input-number v-model:value="runForm.recentCount" :min="1" :max="2000" class="w-full" />
      </n-form-item>
      <n-form-item label="导入方式">
        <n-radio-group v-model:value="runForm.importMode">
          <n-space>
            <n-radio value="incremental">增量更新</n-radio>
            <n-radio value="overwrite">覆盖更新</n-radio>
          </n-space>
        </n-radio-group>
      </n-form-item>
      <n-form-item v-if="runForm.runType !== 'scan'" label="TG匹配">
        <n-space vertical class="w-full">
          <n-switch v-model:value="runTgMatchEnabled" />
          <template v-if="runTgMatchEnabled">
            <n-select
              v-model:value="runForm.channelIds"
              :options="runChannelOptions"
              multiple
              filterable
              clearable
              placeholder="选择需要匹配的上架频道"
            />
            <n-input-number
              v-model:value="runForm.tgMatchDays"
              :min="1"
              :max="365"
              class="w-full"
              placeholder="拉取最近多少天TG媒体消息"
            />
          </template>
        </n-space>
      </n-form-item>
    </n-form>
  </n-modal>

  <n-modal v-model:show="logModalVisible" preset="card" :title="logModalTitle" class="log-modal">
    <n-collapse v-if="groupedLogs.length" class="log-groups">
      <n-collapse-item
        v-for="group in groupedLogs"
        :key="group.sourceNoteId"
        :title="group.title"
        :name="group.sourceNoteId"
      >
        <n-data-table
          :columns="logColumns"
          :data="group.items"
          :pagination="false"
          :scroll-x="1040"
          size="small"
        />
      </n-collapse-item>
    </n-collapse>
    <n-data-table
      v-if="ungroupedLogs.length"
      :columns="logColumns"
      :data="ungroupedLogs"
      :loading="logLoading"
      :pagination="logPagination"
      :scroll-x="1040"
      :max-height="520"
      size="small"
    />
    <template #footer>
      <n-space justify="end">
        <n-button @click="logModalVisible = false">关闭</n-button>
        <n-button type="primary" @click="clearCurrentLogs">清理日志</n-button>
      </n-space>
    </template>
  </n-modal>

  <n-modal
    v-model:show="matchModalVisible"
    preset="card"
    :title="matchModalTitle"
    class="match-modal"
  >
    <n-space vertical size="large">
      <n-form :model="matchForm" label-placement="left" label-width="92">
        <div class="match-config-grid">
          <n-form-item label="匹配频道">
            <n-select
              v-model:value="matchForm.channelIds"
              :options="matchChannelOptions"
              multiple
              filterable
              clearable
              placeholder="选择需要拉取和匹配的上架频道"
            />
          </n-form-item>
          <n-form-item label="拉取天数">
            <n-input-number v-model:value="matchForm.scanDays" :min="1" :max="365" class="w-full" />
          </n-form-item>
          <n-form-item label="自动阈值">
            <n-input-number
              v-model:value="matchForm.threshold"
              :min="80"
              :max="100"
              class="w-full"
            />
          </n-form-item>
          <n-form-item label="资料频道">
            <n-select
              v-model:value="matchItemQuery.channelId"
              :options="matchItemChannelOptions"
              clearable
              placeholder="全部频道"
              @update:value="handleMatchItemFilterChange"
            />
          </n-form-item>
          <n-form-item label="操作">
            <n-space>
              <n-button type="primary" :loading="matchStarting" @click="startImportRunMatch">
                {{ matchPrimaryButtonText }}
              </n-button>
              <n-button @click="refreshImportRunMatch">刷新</n-button>
              <n-button
                :disabled="!matchRun?.id"
                :loading="matchConfirming"
                @click="batchConfirmImportRunMatch"
              >
                批量确认自动匹配
              </n-button>
            </n-space>
          </n-form-item>
        </div>
      </n-form>

      <div v-if="matchRun?.id" class="match-status">
        <n-space align="center">
          <n-tag :type="matchRunStatusType(matchRun.status)" :bordered="false">
            {{ matchRunStatusText(matchRun.status) }}
          </n-tag>
          <span>阶段：{{ matchRun.stage || '-' }}</span>
          <span>资料：{{ matchRun.profileDone || 0 }}/{{ matchRun.profileTotal || 0 }}</span>
          <span>候选：{{ matchRun.candidateTotal || 0 }}</span>
          <span>自动：{{ matchRun.autoMatched || 0 }}</span>
          <span>待人工：{{ matchRun.manualPending || 0 }}</span>
          <span>已确认：{{ matchRun.confirmed || 0 }}</span>
        </n-space>
        <n-progress
          type="line"
          :percentage="matchRunPercent"
          :processing="matchRun.status === 'running' || matchRun.status === 'pending'"
          indicator-placement="inside"
        />
        <n-alert v-if="matchRun.errorMessage" type="error" :bordered="false">
          {{ matchRun.errorMessage }}
        </n-alert>
      </div>

      <div class="match-body">
        <n-data-table
          :columns="matchItemColumns"
          :data="matchItems"
          :loading="matchItemLoading"
          :pagination="matchItemPagination"
          :row-key="(row) => row.id"
          :row-props="matchItemRowProps"
          :scroll-x="1180"
          size="small"
          remote
        />
        <div class="candidate-panel">
          <template v-if="activeMatchItem">
            <div class="review-header">
              <div>
                <strong>{{
                  activeMatchItem.title || activeMatchItem.profileNo || activeMatchItem.profileId
                }}</strong>
                <div class="muted-text">
                  {{ activeMatchItem.channelName || activeMatchItem.channelId }}
                </div>
              </div>
              <n-space>
                <n-button size="small" @click="moveActiveMatchItem(-1)">上一条</n-button>
                <n-button size="small" @click="moveActiveMatchItem(1)">下一条</n-button>
                <n-button size="small" type="primary" @click="confirmAndNext"
                  >确认并下一条</n-button
                >
              </n-space>
            </div>

            <div class="review-grid">
              <div class="review-text">
                <div class="review-label">资料文本</div>
                <n-scrollbar class="review-scroll">
                  <div class="pre-wrap">
                    {{ activeMatchItem.plainText || activeMatchItem.title || '-' }}
                  </div>
                </n-scrollbar>
              </div>
              <div class="review-text">
                <div class="review-label">TG 消息文本</div>
                <n-tabs v-model:value="activePreviewPurpose" type="segment" size="small">
                  <n-tab-pane name="display" tab="展示资料">
                    <div class="binding-toolbar">
                      <n-tag
                        :type="activeMatchItem.displayGroupKey ? 'success' : 'warning'"
                        :bordered="false"
                      >
                        {{ selectedCandidateText(activeMatchItem.displayGroupKey) }}
                      </n-tag>
                      <n-space>
                        <n-button size="small" @click="openCandidateSearch('display')"
                          >搜索替换</n-button
                        >
                        <n-button size="small" @click="clearCandidateBinding('display')"
                          >取消</n-button
                        >
                      </n-space>
                    </div>
                    <n-scrollbar class="review-scroll">
                      <div class="pre-wrap">
                        {{ selectedCandidateCaption(activeMatchItem.displayGroupKey) }}
                      </div>
                    </n-scrollbar>
                  </n-tab-pane>
                  <n-tab-pane name="verify" tab="验证资料">
                    <div class="binding-toolbar">
                      <n-tag
                        :type="activeMatchItem.verifyGroupKey ? 'success' : 'warning'"
                        :bordered="false"
                      >
                        {{ selectedCandidateText(activeMatchItem.verifyGroupKey) }}
                      </n-tag>
                      <n-space>
                        <n-button size="small" @click="openCandidateSearch('verify')"
                          >搜索替换</n-button
                        >
                        <n-button size="small" @click="clearCandidateBinding('verify')"
                          >取消</n-button
                        >
                      </n-space>
                    </div>
                    <n-scrollbar class="review-scroll">
                      <div class="pre-wrap">
                        {{ selectedCandidateCaption(activeMatchItem.verifyGroupKey) }}
                      </div>
                    </n-scrollbar>
                  </n-tab-pane>
                </n-tabs>
              </div>
            </div>

            <n-space class="binding-actions" justify="end">
              <n-button
                size="small"
                type="error"
                ghost
                :disabled="!activeMatchItem.displayGroupKey && !activeMatchItem.verifyGroupKey"
                @click="unbindImportRunMatch(activeMatchItem)"
              >
                取消全部绑定
              </n-button>
            </n-space>

            <n-spin :show="matchCandidateLoading">
              <div class="candidate-tools">
                <span class="muted-text"
                  >点击候选卡片按“展示资料、验证资料”的顺序绑定；再次点击已绑定卡片取消。</span
                >
                <n-button
                  size="small"
                  @click="openCandidateSearch(nextCandidatePurpose(activeMatchItem))"
                >
                  搜索 TG 消息
                </n-button>
              </div>
              <div class="candidate-list">
                <div
                  v-for="candidate in matchCandidates"
                  :key="candidate.id"
                  class="candidate-card"
                  :class="candidateCardClass(candidate)"
                  @click="toggleCandidateBinding(candidate)"
                >
                  <div class="candidate-card-head">
                    <n-space align="center">
                      <n-tag
                        v-if="candidateBindingLabel(candidate)"
                        type="success"
                        :bordered="false"
                      >
                        {{ candidateBindingLabel(candidate) }}
                      </n-tag>
                      <n-tag :bordered="false">分数 {{ candidate.score || 0 }}</n-tag>
                      <n-tag :bordered="false">{{ candidate.mediaTypes || '-' }}</n-tag>
                      <span>{{ candidate.mediaCount || 0 }} 个媒体</span>
                    </n-space>
                    <span class="muted-text">{{ candidate.messageDate || '' }}</span>
                  </div>
                  <div class="review-label">TG 文案</div>
                  <div class="candidate-caption">
                    {{ candidate.captionText || '-' }}
                  </div>
                </div>
                <n-empty v-if="!matchCandidates.length" description="当前资料没有候选消息" />
              </div>
            </n-spin>
          </template>
          <n-empty v-else description="点击左侧资料行查看候选消息" />
        </div>
      </div>
    </n-space>
    <template #footer>
      <n-space justify="end">
        <n-button @click="matchModalVisible = false">关闭</n-button>
      </n-space>
    </template>
  </n-modal>

  <n-modal
    v-model:show="candidateSearchVisible"
    preset="card"
    :title="candidateSearchTitle"
    class="candidate-search-modal"
  >
    <n-space vertical size="large">
      <div class="candidate-search-bar">
        <n-input
          v-model:value="candidateSearchQuery.keyword"
          clearable
          placeholder="搜索 TG 文案 / 消息组 / media group"
          @keyup.enter="loadCandidateSearch"
        />
        <n-button type="primary" :loading="candidateSearchLoading" @click="loadCandidateSearch">
          搜索
        </n-button>
      </div>
      <div class="candidate-search-layout">
        <div class="review-text">
          <div class="review-label">资料文本</div>
          <n-scrollbar class="candidate-search-scroll">
            <div class="pre-wrap">
              {{ activeMatchItem?.plainText || activeMatchItem?.title || '-' }}
            </div>
          </n-scrollbar>
        </div>
        <div class="review-text">
          <div class="review-label">TG 消息列表</div>
          <n-spin :show="candidateSearchLoading">
            <div class="candidate-search-list">
              <div
                v-for="candidate in candidateSearchList"
                :key="candidate.id"
                class="candidate-card"
                :class="candidateCardClass(candidate)"
                @click="selectCandidateFromSearch(candidate)"
              >
                <div class="candidate-card-head">
                  <n-space align="center">
                    <n-tag v-if="candidateBindingLabel(candidate)" type="success" :bordered="false">
                      {{ candidateBindingLabel(candidate) }}
                    </n-tag>
                    <n-tag :bordered="false">分数 {{ candidate.score || 0 }}</n-tag>
                    <n-tag :bordered="false">{{ candidate.mediaTypes || '-' }}</n-tag>
                    <span>{{ candidate.mediaCount || 0 }} 个媒体</span>
                  </n-space>
                  <span class="muted-text">{{ candidate.messageDate || '' }}</span>
                </div>
                <div class="candidate-caption">
                  {{ candidate.captionText || '-' }}
                </div>
              </div>
              <n-empty v-if="!candidateSearchList.length" description="没有找到TG消息" />
            </div>
          </n-spin>
          <n-pagination
            v-model:page="candidateSearchPagination.page"
            v-model:page-size="candidateSearchPagination.pageSize"
            :item-count="candidateSearchPagination.itemCount"
            :page-sizes="[10, 20, 50]"
            show-size-picker
            class="candidate-search-pagination"
            @update:page="loadCandidateSearch"
            @update:page-size="handleCandidateSearchPageSize"
          />
        </div>
      </div>
    </n-space>
    <template #footer>
      <n-space justify="end">
        <n-button @click="candidateSearchVisible = false">关闭</n-button>
      </n-space>
    </template>
  </n-modal>
</template>

<script lang="ts" setup>
  import { computed, h, onMounted, onUnmounted, reactive, ref, watch } from 'vue';
  import { NButton, NProgress, NSpace, NTag, useMessage } from 'naive-ui';
  import {
    AccountList,
    ChannelList,
    ImportRunCancel,
    ImportRunCreate,
    ImportRunDelete,
    ImportRunList,
    ImportRunLogClear,
    ImportRunLogList,
    ImportRunMatchBatchConfirm,
    ImportRunMatchCandidateList,
    ImportRunMatchCandidateSearch,
    ImportRunMatchConfig,
    ImportRunMatchConfirm,
    ImportRunMatchItemList,
    ImportRunMatchSaveDraft,
    ImportRunMatchSkip,
    ImportRunMatchStart,
    ImportRunMatchUnbind,
    ImportRunMatchView,
    ImportRunTgSyncStart,
    ImportTaskCreate,
    ImportTaskList,
    ImportTaskView,
    TenantList,
  } from '@/api/addons/youbanPublish';

  const message = useMessage();
  const activeTab = ref('tasks');
  const loading = ref(false);
  const runLoading = ref(false);
  const logLoading = ref(false);
  const modalVisible = ref(false);
  const runModalVisible = ref(false);
  const logModalVisible = ref(false);
  const matchModalVisible = ref(false);
  const candidateSearchVisible = ref(false);
  const proxyEnabled = ref(false);
  const runTgMatchEnabled = ref(false);
  const tasks = ref<Recordable[]>([]);
  const runs = ref<Recordable[]>([]);
  const logs = ref<Recordable[]>([]);
  const tenants = ref<Recordable[]>([]);
  const accounts = ref<Recordable[]>([]);
  const channels = ref<Recordable[]>([]);
  const currentTaskId = ref<number | null>(null);
  const currentRunId = ref<number | null>(null);
  const currentMatchImportRunId = ref<number | null>(null);
  const matchActionMode = ref<'match' | 'sync'>('match');
  const matchStarting = ref(false);
  const matchConfirming = ref(false);
  const matchItemLoading = ref(false);
  const matchCandidateLoading = ref(false);
  const candidateSearchLoading = ref(false);
  const matchRun = ref<Recordable | null>(null);
  const matchChannels = ref<Recordable[]>([]);
  const matchItems = ref<Recordable[]>([]);
  const matchCandidates = ref<Recordable[]>([]);
  const candidateSearchList = ref<Recordable[]>([]);
  const activeMatchItem = ref<Recordable | null>(null);
  const activePreviewPurpose = ref<'display' | 'verify'>('display');
  const candidateSearchPurpose = ref<'display' | 'verify'>('display');
  let matchPollTimer: number | null = null;

  const query = reactive({
    tenantId: null as number | null,
    accountId: null as number | null,
    keyword: '',
  });
  const runQuery = reactive({
    tenantId: null as number | null,
    accountId: null as number | null,
    runType: undefined as string | undefined,
    status: undefined as string | undefined,
    keyword: '',
  });
  const form = reactive({
    id: null as number | null,
    sourceName: 'lyy_cms',
    tenantId: null as number | null,
    accountId: null as number | null,
    baseUrl: '',
    serverIp: '',
    username: '',
    password: '',
    legacyCookie: '',
    limitCount: 100,
    perPage: 12,
    mediaConcurrency: 4,
    importMode: 'incremental',
    channelIds: [] as number[],
    proxyEnabled: 0,
    proxyPool: '',
    remark: '',
  });
  const runForm = reactive({
    runType: 'scan',
    scanMode: 'recent',
    recentCount: 100,
    importMode: 'incremental',
    tgMatchDays: 180,
    channelIds: [] as number[],
  });
  const matchForm = reactive({
    channelIds: [] as number[],
    scanDays: 180,
    threshold: 80,
  });
  const matchItemQuery = reactive({
    channelId: null as number | null,
  });
  const candidateSearchQuery = reactive({
    keyword: '',
  });

  const taskPagination = reactive({
    page: 1,
    pageSize: 20,
    itemCount: 0,
    showSizePicker: true,
    pageSizes: [10, 20, 50],
    onUpdatePage: (page: number) => {
      taskPagination.page = page;
      loadTasks();
    },
    onUpdatePageSize: (pageSize: number) => {
      taskPagination.pageSize = pageSize;
      taskPagination.page = 1;
      loadTasks();
    },
  });
  const runPagination = reactive({
    page: 1,
    pageSize: 20,
    itemCount: 0,
    showSizePicker: true,
    pageSizes: [10, 20, 50],
    onUpdatePage: (page: number) => {
      runPagination.page = page;
      loadRuns();
    },
    onUpdatePageSize: (pageSize: number) => {
      runPagination.pageSize = pageSize;
      runPagination.page = 1;
      loadRuns();
    },
  });
  const logPagination = reactive({ pageSize: 10 });
  const candidateSearchPagination = reactive({
    page: 1,
    pageSize: 20,
    itemCount: 0,
  });
  const matchItemPagination = reactive({
    page: 1,
    pageSize: 10,
    itemCount: 0,
    showSizePicker: true,
    pageSizes: [10, 20, 50],
    onUpdatePage: (page: number) => {
      matchItemPagination.page = page;
      loadImportRunMatchItems();
    },
    onUpdatePageSize: (pageSize: number) => {
      matchItemPagination.pageSize = pageSize;
      matchItemPagination.page = 1;
      loadImportRunMatchItems();
    },
  });
  const currentRun = computed(() => runs.value.find((item) => item.id === currentRunId.value));
  const logModalTitle = computed(() =>
    currentRun.value ? `执行日志 #${currentRun.value.id}` : '执行日志'
  );
  const matchModalTitle = computed(() =>
    currentMatchImportRunId.value
      ? `${matchActionMode.value === 'sync' ? '同步TG消息' : 'TG消息匹配'} #${currentMatchImportRunId.value}`
      : matchActionMode.value === 'sync'
        ? '同步TG消息'
        : 'TG消息匹配'
  );
  const matchPrimaryButtonText = computed(() =>
    matchActionMode.value === 'sync' ? '同步TG消息' : '开始匹配'
  );
  const candidateSearchTitle = computed(
    () => `选择${candidateSearchPurpose.value === 'display' ? '展示资料' : '验证资料'} TG 消息`
  );
  const matchChannelOptions = computed(() =>
    matchChannels.value.map((item) => ({
      label: `${item.name || item.targetChatId} (${item.targetChatId})`,
      value: item.id,
    }))
  );
  const matchItemChannelOptions = computed(() => [
    { label: '全部频道', value: null },
    ...matchChannelOptions.value,
  ]);
  const matchRunPercent = computed(() => {
    if (!matchRun.value?.id) return 0;
    if (matchRun.value.status === 'success') return 100;
    if (matchRun.value.profileTotal > 0) {
      return Math.min(
        99,
        Math.round((matchRun.value.profileDone / matchRun.value.profileTotal) * 100)
      );
    }
    if (matchRun.value.status === 'running') return 35;
    return 0;
  });
  function runProgressPercent(row: Recordable) {
    if (row?.status === 'success') return 100;
    if (row?.scanMode === 'all' && row?.runType !== 'scan') {
      const scanned = Number(row?.itemDone || 0);
      const imported = Number(row?.imported || 0);
      if (scanned > 0) {
        return Math.min(100, Math.round((imported * 100) / scanned));
      }
      return 0;
    }
    return Math.min(100, Math.round(row?.percent || 0));
  }
  function runProgressText(row: Recordable) {
    if (row?.scanMode === 'all' && row?.runType !== 'scan') {
      return `${row?.imported || 0}/${row?.itemDone || 0}`;
    }
    return `${row?.itemDone || 0}/${row?.itemTotal || 0}`;
  }
  const normalizedLogs = computed(() =>
    logs.value.map((item) => ({ ...item, parsedContext: parseLogContext(item.context) }))
  );
  const groupedLogs = computed(() => {
    const groups = new Map<number, Recordable[]>();
    normalizedLogs.value.forEach((item) => {
      const sourceNoteId = Number(item.parsedContext?.sourceNoteId || 0);
      if (sourceNoteId <= 0) return;
      if (!groups.has(sourceNoteId)) groups.set(sourceNoteId, []);
      groups.get(sourceNoteId)?.push(item);
    });
    return Array.from(groups.entries()).map(([sourceNoteId, items]) => ({
      sourceNoteId,
      title: `旧站笔记 #${sourceNoteId}（${items.length} 条）`,
      items,
    }));
  });
  const ungroupedLogs = computed(() =>
    normalizedLogs.value.filter((item) => Number(item.parsedContext?.sourceNoteId || 0) <= 0)
  );

  const statusOptions = [
    { label: '待执行', value: 'pending' },
    { label: '执行中', value: 'running' },
    { label: '成功', value: 'success' },
    { label: '失败', value: 'failed' },
    { label: '已取消', value: 'canceled' },
  ];
  const runTypeOptions = [
    { label: '导入', value: 'import' },
    { label: '补全', value: 'repair' },
    { label: '仅扫描', value: 'scan' },
  ];
  const matchItemStatusMap = {
    auto_selected: ['success', '自动选择'],
    manual_pending: ['warning', '待人工'],
    confirmed: ['success', '已确认'],
    skipped: ['default', '已跳过'],
  };
  const statusOptionsWithAll = computed(() => [
    { label: '全部状态', value: undefined },
    ...statusOptions,
  ]);
  const runTypeOptionsWithAll = computed(() => [
    { label: '全部类型', value: undefined },
    ...runTypeOptions,
  ]);
  const tenantOptions = computed(() =>
    tenants.value.map((item) => ({ label: accountOwnerName(item), value: item.id }))
  );
  const tenantOptionsWithAll = computed(() => [
    { label: '全部账号归属', value: null },
    ...tenantOptions.value,
  ]);
  const queryAccountOptions = computed(() => accountOptionsByTenant(query.tenantId));
  const queryAccountOptionsWithAll = computed(() => [
    { label: '全部上架账号', value: null },
    ...queryAccountOptions.value,
  ]);
  const runAccountOptions = computed(() => accountOptionsByTenant(runQuery.tenantId));
  const runAccountOptionsWithAll = computed(() => [
    { label: '全部上架账号', value: null },
    ...runAccountOptions.value,
  ]);
  const formAccountOptions = computed(() => accountOptionsByTenant(form.tenantId));
  const formChannelOptions = computed(() => channelOptionsByTenant(form.tenantId));
  const runChannelOptions = computed(() => {
    const task = tasks.value.find((item) => item.id === currentTaskId.value);
    return channelOptionsByTenant(task?.tenantId || null);
  });
  const taskModalTitle = computed(() => (form.id ? '修改导入任务' : '新建导入任务'));

  const taskColumns = [
    { title: 'ID', key: 'id', width: 80 },
    { title: '来源', key: 'sourceName', width: 110 },
    { title: '账号归属', key: 'tenantName', width: 150 },
    { title: '旧站域名', key: 'baseUrl', width: 220, ellipsis: { tooltip: true } },
    { title: '服务器IP', key: 'serverIp', width: 140, ellipsis: { tooltip: true } },
    { title: '旧站账号', key: 'username', width: 140 },
    { title: '上架账号', key: 'accountName', width: 130 },
    { title: '测试数量', key: 'limitCount', width: 100 },
    { title: '每页数量', key: 'perPage', width: 100 },
    { title: '媒体并发', key: 'mediaConcurrency', width: 100 },
    { title: '备注', key: 'remark', width: 180, ellipsis: { tooltip: true } },
    { title: '更新时间', key: 'updatedAt', width: 180 },
    {
      title: '操作',
      key: 'actions',
      width: 220,
      fixed: 'right',
      render(row) {
        return h(
          NSpace,
          { size: 8 },
          {
            default: () => [
              h(
                NButton,
                { size: 'small', onClick: () => openEditModal(row) },
                { default: () => '修改' }
              ),
              h(
                NButton,
                { size: 'small', onClick: () => openRunModal(row.id, 'scan') },
                { default: () => '仅扫描' }
              ),
              h(
                NButton,
                { size: 'small', type: 'primary', onClick: () => openRunModal(row.id, 'import') },
                { default: () => '导入资料' }
              ),
            ],
          }
        );
      },
    },
  ];

  const runColumns = [
    { title: 'ID', key: 'id', width: 80 },
    { title: '任务ID', key: 'taskId', width: 90 },
    {
      title: '类型',
      key: 'runType',
      width: 90,
      render: (row) =>
        runTypeOptions.find((item) => item.value === row.runType)?.label || row.runType,
    },
    { title: '账号归属', key: 'tenantName', width: 150 },
    { title: '旧站域名', key: 'baseUrl', width: 220, ellipsis: { tooltip: true } },
    { title: '旧站账号', key: 'username', width: 140 },
    { title: '上架账号', key: 'accountName', width: 130 },
    {
      title: '状态',
      key: 'status',
      width: 100,
      render(row) {
        const map = {
          pending: ['default', '待执行'],
          running: ['warning', '执行中'],
          success: ['success', '成功'],
          failed: ['error', '失败'],
          canceled: ['default', '已取消'],
        };
        const item = map[row.status] || ['default', row.status || '-'];
        return h(NTag, { type: item[0] as any, bordered: false }, { default: () => item[1] });
      },
    },
    { title: '阶段', key: 'stage', width: 110 },
    {
      title: '进度',
      key: 'percent',
      width: 170,
      render: (row) =>
        h(NProgress, {
          type: 'line',
          percentage: runProgressPercent(row),
          indicatorPlacement: 'inside',
          processing: row.status === 'running',
        }),
    },
    {
      title: '资料',
      key: 'itemDone',
      width: 110,
      render: (row) => runProgressText(row),
    },
    {
      title: '媒体',
      key: 'mediaDone',
      width: 110,
      render: (row) => `${row.mediaDone || 0}/${row.mediaTotal || 0}`,
    },
    { title: '未迁移存储', key: 'mediaMissingStorage', width: 120 },
    { title: 'TG匹配', key: 'tgMatched', width: 100 },
    { title: '错误', key: 'errorMessage', width: 240, ellipsis: { tooltip: true } },
    { title: '创建时间', key: 'createdAt', width: 180 },
    { title: '开始时间', key: 'startedAt', width: 180 },
    { title: '完成时间', key: 'finishedAt', width: 180 },
    { title: '更新时间', key: 'updatedAt', width: 180 },
    {
      title: '操作',
      key: 'actions',
      width: 310,
      fixed: 'right',
      render(row) {
        return h(
          NSpace,
          { size: 8 },
          {
            default: () => [
              h(
                NButton,
                { size: 'small', onClick: () => openLogs(row.id) },
                { default: () => '日志' }
              ),
              h(
                NButton,
                {
                  size: 'small',
                  type: 'primary',
                  disabled: row.runType === 'scan' || row.status === 'running',
                  onClick: () => openImportRunMatch(row, 'sync'),
                },
                { default: () => '同步TG' }
              ),
              h(
                NButton,
                {
                  size: 'small',
                  type: 'primary',
                  disabled: row.runType === 'scan' || row.status === 'running',
                  onClick: () => openImportRunMatch(row, 'match'),
                },
                { default: () => 'TG匹配' }
              ),
              h(
                NButton,
                {
                  size: 'small',
                  disabled: row.status !== 'running' && row.status !== 'pending',
                  onClick: () => cancelRun(row.id),
                },
                { default: () => '取消' }
              ),
              h(
                NButton,
                { size: 'small', disabled: row.status === 'running', onClick: () => retryRun(row) },
                { default: () => '重试' }
              ),
              h(
                NButton,
                {
                  size: 'small',
                  type: 'error',
                  disabled: row.status === 'running',
                  onClick: () => deleteRun(row.id),
                },
                { default: () => '删除' }
              ),
            ],
          }
        );
      },
    },
  ];
  const matchItemColumns = [
    { title: '资料ID', key: 'profileId', width: 90 },
    { title: '标题', key: 'title', width: 180, ellipsis: { tooltip: true } },
    { title: '编号', key: 'profileNo', width: 100 },
    { title: '频道', key: 'channelName', width: 150, ellipsis: { tooltip: true } },
    {
      title: '状态',
      key: 'matchStatus',
      width: 110,
      render(row) {
        const item = matchItemStatusMap[row.matchStatus] || ['default', row.matchStatus || '-'];
        return h(NTag, { type: item[0] as any, bordered: false }, { default: () => item[1] });
      },
    },
    { title: '展示分', key: 'displayScore', width: 90 },
    { title: '验证分', key: 'verifyScore', width: 90 },
    { title: '总分', key: 'totalScore', width: 80 },
    { title: '展示组', key: 'displayGroupKey', width: 140, ellipsis: { tooltip: true } },
    { title: '验证组', key: 'verifyGroupKey', width: 140, ellipsis: { tooltip: true } },
    {
      title: '操作',
      key: 'actions',
      width: 220,
      fixed: 'right',
      render(row) {
        return h(
          NSpace,
          { size: 8 },
          {
            default: () => [
              h(
                NButton,
                { size: 'small', onClick: () => selectMatchItem(row) },
                { default: () => '人工匹配' }
              ),
              h(
                NButton,
                {
                  size: 'small',
                  type: 'primary',
                  disabled: row.matchStatus === 'confirmed',
                  onClick: () => confirmImportRunMatch(row),
                },
                { default: () => '确认' }
              ),
              h(
                NButton,
                {
                  size: 'small',
                  disabled: row.matchStatus === 'confirmed',
                  onClick: () => skipImportRunMatch(row),
                },
                { default: () => '跳过' }
              ),
              h(
                NButton,
                {
                  size: 'small',
                  type: 'error',
                  ghost: true,
                  disabled: !row.displayGroupKey && !row.verifyGroupKey,
                  onClick: () => unbindImportRunMatch(row),
                },
                { default: () => '取消绑定' }
              ),
            ],
          }
        );
      },
    },
  ];
  const logColumns = [
    { title: '时间', key: 'createdAt', width: 180 },
    { title: '级别', key: 'level', width: 80 },
    { title: '阶段', key: 'stage', width: 100 },
    { title: '内容', key: 'message', width: 520, ellipsis: { tooltip: true } },
    {
      title: '上下文',
      key: 'context',
      width: 160,
      ellipsis: { tooltip: true },
      render: (row) => renderLogContext(row),
    },
  ];

  onMounted(async () => {
    await loadTenants();
    await loadAccounts();
    await loadChannels();
    await loadTasks();
  });

  onUnmounted(() => {
    stopMatchPolling();
  });

  watch(activeTab, (tab) => {
    if (tab === 'runs') loadRuns();
  });

  watch(matchModalVisible, (visible) => {
    if (!visible) {
      stopMatchPolling();
      candidateSearchVisible.value = false;
    }
  });

  async function loadTenants() {
    const res: any = await TenantList({ page: 1, perPage: 200, status: 1 });
    tenants.value = res?.list || [];
  }

  async function loadAccounts() {
    const res: any = await AccountList({
      page: 1,
      perPage: 200,
      accountType: 'uploader',
      status: 1,
    });
    accounts.value = res?.list || [];
  }

  async function loadChannels() {
    const res: any = await ChannelList({
      page: 1,
      perPage: 200,
      publishDirection: 'up',
      status: 1,
    });
    channels.value = res?.list || [];
  }

  async function loadTasks() {
    loading.value = true;
    try {
      const res: any = await ImportTaskList({
        ...query,
        page: taskPagination.page,
        perPage: taskPagination.pageSize,
      });
      tasks.value = res?.list || [];
      taskPagination.itemCount = res?.totalCount || res?.total || 0;
    } finally {
      loading.value = false;
    }
  }

  async function loadRuns() {
    runLoading.value = true;
    try {
      const res: any = await ImportRunList({
        ...runQuery,
        page: runPagination.page,
        perPage: runPagination.pageSize,
      });
      runs.value = res?.list || [];
      runPagination.itemCount = res?.totalCount || res?.total || 0;
    } finally {
      runLoading.value = false;
    }
  }

  async function openImportRunMatch(row: Recordable, mode: 'match' | 'sync' = 'match') {
    matchActionMode.value = mode;
    currentMatchImportRunId.value = row.id;
    activeMatchItem.value = null;
    matchItems.value = [];
    matchCandidates.value = [];
    matchItemPagination.page = 1;
    matchItemQuery.channelId = null;
    const res: any = await ImportRunMatchConfig({ importRunId: row.id });
    matchChannels.value = res?.channels || [];
    matchRun.value = res?.latestRun || null;
    matchForm.channelIds = decodeIdJson(row.channelIdJson);
    if (!matchForm.channelIds.length && matchRun.value?.channelIds?.length) {
      matchForm.channelIds = matchRun.value.channelIds;
    }
    if (!matchForm.channelIds.length && matchChannels.value.length) {
      matchForm.channelIds = matchChannels.value.map((item) => item.id);
    }
    matchForm.scanDays = matchRun.value?.scanDays || 180;
    matchForm.threshold = Math.max(matchRun.value?.threshold || 80, 80);
    matchModalVisible.value = true;
    if (matchRun.value?.id) {
      await loadImportRunMatchItems();
      startMatchPolling();
    }
  }

  async function startImportRunMatch() {
    if (!currentMatchImportRunId.value) return false;
    if (!matchForm.channelIds.length) {
      message.warning('请选择匹配频道');
      return false;
    }
    matchStarting.value = true;
    try {
      const res: any =
        matchActionMode.value === 'sync'
          ? await ImportRunTgSyncStart({
              importRunId: currentMatchImportRunId.value,
              channelIds: matchForm.channelIds,
              scanDays: matchForm.scanDays,
            })
          : await ImportRunMatchStart({
              importRunId: currentMatchImportRunId.value,
              channelIds: matchForm.channelIds,
              threshold: matchForm.threshold,
            });
      matchRun.value = res || null;
      message.success(
        matchActionMode.value === 'sync' ? 'TG消息同步已加入队列' : 'TG消息匹配已加入队列'
      );
      await loadImportRunMatchItems();
      startMatchPolling();
    } finally {
      matchStarting.value = false;
    }
    return false;
  }

  async function refreshImportRunMatch() {
    if (!matchRun.value?.id && !currentMatchImportRunId.value) return;
    await loadImportRunMatchView();
    await loadImportRunMatchItems();
    if (activeMatchItem.value?.id) {
      await loadImportRunMatchCandidates(activeMatchItem.value);
    }
  }

  async function loadImportRunMatchView() {
    const params = matchRun.value?.id
      ? { id: matchRun.value.id }
      : { importRunId: currentMatchImportRunId.value };
    const res: any = await ImportRunMatchView(params);
    matchRun.value = res || null;
  }

  async function loadImportRunMatchItems() {
    if (!matchRun.value?.id) return;
    matchItemLoading.value = true;
    try {
      const res: any = await ImportRunMatchItemList({
        matchRunId: matchRun.value.id,
        channelId: matchItemQuery.channelId || undefined,
        page: matchItemPagination.page,
        perPage: matchItemPagination.pageSize,
      });
      matchItems.value = res?.list || [];
      matchItemPagination.itemCount = res?.totalCount || res?.total || 0;
      if (activeMatchItem.value?.id) {
        activeMatchItem.value =
          matchItems.value.find((item) => item.id === activeMatchItem.value?.id) ||
          activeMatchItem.value;
      }
      if (!activeMatchItem.value?.id && matchItems.value.length) {
        const firstPending =
          matchItems.value.find((item) => item.matchStatus === 'manual_pending') ||
          matchItems.value[0];
        await selectMatchItem(firstPending);
      }
    } finally {
      matchItemLoading.value = false;
    }
  }

  async function handleMatchItemFilterChange() {
    matchItemPagination.page = 1;
    activeMatchItem.value = null;
    matchCandidates.value = [];
    await loadImportRunMatchItems();
  }

  function matchItemRowProps(row: Recordable) {
    return {
      class: activeMatchItem.value?.id === row.id ? 'is-active-match-row' : '',
      style: 'cursor: pointer;',
      onClick: () => selectMatchItem(row),
    };
  }

  async function selectMatchItem(row: Recordable) {
    activeMatchItem.value = row;
    activePreviewPurpose.value = row.displayGroupKey ? 'display' : 'verify';
    await loadImportRunMatchCandidates(row);
  }

  async function loadImportRunMatchCandidates(row: Recordable) {
    matchCandidateLoading.value = true;
    try {
      const res: any = await ImportRunMatchCandidateList({ itemId: row.id });
      matchCandidates.value = res?.list || [];
    } finally {
      matchCandidateLoading.value = false;
    }
  }

  async function openCandidateSearch(purpose: 'display' | 'verify') {
    if (!activeMatchItem.value?.id) return;
    candidateSearchPurpose.value = purpose;
    activePreviewPurpose.value = purpose;
    candidateSearchQuery.keyword =
      activeMatchItem.value.title || activeMatchItem.value.profileNo || '';
    candidateSearchPagination.page = 1;
    candidateSearchVisible.value = true;
    await loadCandidateSearch();
  }

  async function loadCandidateSearch() {
    if (!activeMatchItem.value?.id) return;
    candidateSearchLoading.value = true;
    try {
      const res: any = await ImportRunMatchCandidateSearch({
        itemId: activeMatchItem.value.id,
        keyword: candidateSearchQuery.keyword,
        page: candidateSearchPagination.page,
        perPage: candidateSearchPagination.pageSize,
      });
      candidateSearchList.value = res?.list || [];
      candidateSearchPagination.itemCount = res?.totalCount || res?.total || 0;
    } finally {
      candidateSearchLoading.value = false;
    }
  }

  async function handleCandidateSearchPageSize(pageSize: number) {
    candidateSearchPagination.pageSize = pageSize;
    candidateSearchPagination.page = 1;
    await loadCandidateSearch();
  }

  async function selectCandidateFromSearch(row: Recordable) {
    if (!activeMatchItem.value?.id) return;
    const payload = {
      itemId: activeMatchItem.value.id,
      displayGroupKey: activeMatchItem.value.displayGroupKey || '',
      verifyGroupKey: activeMatchItem.value.verifyGroupKey || '',
    };
    if (candidateSearchPurpose.value === 'display') {
      payload.displayGroupKey = payload.displayGroupKey === row.groupKey ? '' : row.groupKey;
      if (payload.displayGroupKey && payload.verifyGroupKey === payload.displayGroupKey) {
        payload.verifyGroupKey = '';
      }
    } else {
      payload.verifyGroupKey = payload.verifyGroupKey === row.groupKey ? '' : row.groupKey;
      if (payload.verifyGroupKey && payload.displayGroupKey === payload.verifyGroupKey) {
        payload.displayGroupKey = '';
      }
    }
    await saveActiveCandidateBinding(payload, '已保存TG消息选择');
    if (!matchCandidates.value.some((item) => item.groupKey === row.groupKey)) {
      matchCandidates.value = [row, ...matchCandidates.value];
    }
    if (payload.displayGroupKey && payload.verifyGroupKey) {
      candidateSearchVisible.value = false;
    } else {
      candidateSearchPurpose.value = payload.displayGroupKey ? 'verify' : 'display';
      activePreviewPurpose.value = candidateSearchPurpose.value;
    }
  }

  async function toggleCandidateBinding(row: Recordable) {
    if (!activeMatchItem.value?.id) return;
    const payload = {
      itemId: activeMatchItem.value.id,
      displayGroupKey: activeMatchItem.value.displayGroupKey || '',
      verifyGroupKey: activeMatchItem.value.verifyGroupKey || '',
    };
    let messageText = '';
    if (payload.displayGroupKey === row.groupKey) {
      payload.displayGroupKey = '';
      messageText = '已取消展示资料绑定';
    } else if (payload.verifyGroupKey === row.groupKey) {
      payload.verifyGroupKey = '';
      messageText = '已取消验证资料绑定';
    } else {
      const purpose = nextCandidatePurpose(payload);
      if (purpose === 'display') {
        payload.displayGroupKey = row.groupKey;
        if (payload.verifyGroupKey === row.groupKey) payload.verifyGroupKey = '';
        messageText = '已绑定展示资料';
      } else {
        payload.verifyGroupKey = row.groupKey;
        if (payload.displayGroupKey === row.groupKey) payload.displayGroupKey = '';
        messageText = '已绑定验证资料';
      }
    }
    await saveActiveCandidateBinding(payload, messageText);
  }

  async function clearCandidateBinding(purpose: 'display' | 'verify') {
    if (!activeMatchItem.value?.id) return;
    const payload = {
      itemId: activeMatchItem.value.id,
      displayGroupKey: activeMatchItem.value.displayGroupKey || '',
      verifyGroupKey: activeMatchItem.value.verifyGroupKey || '',
    };
    if (purpose === 'display') payload.displayGroupKey = '';
    if (purpose === 'verify') payload.verifyGroupKey = '';
    await saveActiveCandidateBinding(
      payload,
      purpose === 'display' ? '已取消展示资料绑定' : '已取消验证资料绑定'
    );
  }

  async function saveActiveCandidateBinding(payload: Recordable, messageText: string) {
    await ImportRunMatchSaveDraft(payload);
    activeMatchItem.value = { ...activeMatchItem.value, ...payload };
    matchItems.value = matchItems.value.map((item) =>
      item.id === payload.itemId
        ? {
            ...item,
            displayGroupKey: payload.displayGroupKey,
            verifyGroupKey: payload.verifyGroupKey,
            matchStatus:
              payload.displayGroupKey && payload.verifyGroupKey
                ? 'auto_selected'
                : 'manual_pending',
          }
        : item
    );
    message.success(messageText || '已保存匹配选择');
  }

  function nextCandidatePurpose(payload: Recordable): 'display' | 'verify' {
    if (!payload.displayGroupKey) return 'display';
    if (!payload.verifyGroupKey) return 'verify';
    return 'display';
  }

  function candidateBindingLabel(candidate: Recordable) {
    if (!activeMatchItem.value) return '';
    if (candidate.groupKey === activeMatchItem.value.displayGroupKey) return '展示';
    if (candidate.groupKey === activeMatchItem.value.verifyGroupKey) return '验证';
    return '';
  }

  function candidateCardClass(candidate: Recordable) {
    return {
      'is-bound-display': candidate.groupKey === activeMatchItem.value?.displayGroupKey,
      'is-bound-verify': candidate.groupKey === activeMatchItem.value?.verifyGroupKey,
    };
  }

  function selectedCandidateText(groupKey: string) {
    if (!groupKey) return '未选择';
    const candidate = matchCandidates.value.find((item) => item.groupKey === groupKey);
    if (!candidate) return groupKey;
    return `${candidate.firstMessageId || '-'}-${candidate.lastMessageId || '-'} / ${candidate.mediaTypes || '-'} / ${candidate.mediaCount || 0}个媒体`;
  }

  function selectedCandidateCaption(groupKey: string) {
    if (!groupKey) return '未选择TG消息';
    const candidate = matchCandidates.value.find((item) => item.groupKey === groupKey);
    return candidate?.captionText || '当前页未加载该TG消息，可点击搜索替换查看。';
  }

  async function moveActiveMatchItem(offset: number) {
    if (!matchItems.value.length) return;
    const currentIndex = matchItems.value.findIndex(
      (item) => item.id === activeMatchItem.value?.id
    );
    const baseIndex = currentIndex >= 0 ? currentIndex : 0;
    const nextIndex = (baseIndex + offset + matchItems.value.length) % matchItems.value.length;
    await selectMatchItem(matchItems.value[nextIndex]);
  }

  async function confirmAndNext() {
    if (!activeMatchItem.value?.id) return;
    await confirmImportRunMatch(activeMatchItem.value, true);
  }

  async function confirmImportRunMatch(row: Recordable, goNext = false) {
    const currentIndex = matchItems.value.findIndex((item) => item.id === row.id);
    matchConfirming.value = true;
    try {
      await ImportRunMatchConfirm({ itemId: row.id });
      message.success('TG消息绑定已确认');
      activeMatchItem.value = null;
      await loadImportRunMatchView();
      await loadImportRunMatchItems();
      if (goNext && matchItems.value.length) {
        const next =
          matchItems.value
            .slice(Math.max(currentIndex, 0) + 1)
            .find((item) => item.matchStatus !== 'confirmed' && item.matchStatus !== 'skipped') ||
          matchItems.value.find(
            (item) => item.matchStatus !== 'confirmed' && item.matchStatus !== 'skipped'
          );
        if (next) await selectMatchItem(next);
      }
    } finally {
      matchConfirming.value = false;
    }
  }

  async function batchConfirmImportRunMatch() {
    if (!matchRun.value?.id) return;
    matchConfirming.value = true;
    try {
      await ImportRunMatchBatchConfirm({ matchRunId: matchRun.value.id });
      message.success('自动匹配已批量确认');
      await refreshImportRunMatch();
    } finally {
      matchConfirming.value = false;
    }
  }

  async function skipImportRunMatch(row: Recordable) {
    await ImportRunMatchSkip({ itemId: row.id });
    message.success('已跳过该资料匹配');
    await refreshImportRunMatch();
  }

  async function unbindImportRunMatch(row: Recordable) {
    if (!row?.id) return;
    await ImportRunMatchUnbind({ itemId: row.id });
    message.success('已取消TG消息绑定');
    if (activeMatchItem.value?.id === row.id) {
      activeMatchItem.value = {
        ...activeMatchItem.value,
        displayGroupKey: '',
        verifyGroupKey: '',
        matchStatus: 'manual_pending',
      };
    }
    await refreshImportRunMatch();
  }

  function startMatchPolling() {
    stopMatchPolling();
    matchPollTimer = window.setInterval(async () => {
      if (!matchModalVisible.value || !matchRun.value?.id) return;
      await loadImportRunMatchView();
      if (matchRun.value?.status === 'running' || matchRun.value?.status === 'pending') return;
      stopMatchPolling();
      await loadImportRunMatchItems();
    }, 2500);
  }

  function stopMatchPolling() {
    if (!matchPollTimer) return;
    window.clearInterval(matchPollTimer);
    matchPollTimer = null;
  }

  function openCreateModal() {
    resetTaskForm();
    if (!form.tenantId && tenants.value.length === 1) form.tenantId = tenants.value[0].id;
    modalVisible.value = true;
  }

  async function openEditModal(row: Recordable) {
    resetTaskForm();
    const detail: any = await ImportTaskView({ id: row.id });
    const data = detail || row;
    form.id = data.id;
    form.sourceName = data.sourceName || 'lyy_cms';
    form.tenantId = data.tenantId || null;
    form.accountId = data.accountId || null;
    form.baseUrl = data.baseUrl || '';
    form.serverIp = data.serverIp || '';
    form.username = data.username || '';
    form.password = '';
    form.legacyCookie = '';
    form.limitCount = data.limitCount ?? 100;
    form.perPage = data.perPage ?? 12;
    form.mediaConcurrency = data.mediaConcurrency ?? 4;
    form.importMode = getTaskImportMode(data);
    form.channelIds = decodeIdJson(data.channelIdJson);
    form.proxyEnabled = data.proxyEnabled || 0;
    form.proxyPool = data.proxyPool || '';
    form.remark = data.remark || '';
    proxyEnabled.value = data.proxyEnabled === 1;
    modalVisible.value = true;
  }

  async function createTask() {
    if (!form.tenantId) {
      message.warning('请选择账号归属');
      return false;
    }
    if (!form.accountId) {
      message.warning('请选择导入账号');
      return false;
    }
    const payload: Recordable = { ...form, proxyEnabled: proxyEnabled.value ? 1 : 0 };
    if (payload.id && !payload.password) {
      delete payload.password;
    }
    if (!payload.legacyCookie) {
      delete payload.legacyCookie;
    }
    await ImportTaskCreate(payload);
    message.success('导入任务已保存');
    await loadTasks();
  }

  function resetTaskForm() {
    form.id = null;
    form.sourceName = 'lyy_cms';
    form.tenantId = null;
    form.accountId = null;
    form.baseUrl = '';
    form.serverIp = '';
    form.username = '';
    form.password = '';
    form.legacyCookie = '';
    form.limitCount = 100;
    form.perPage = 12;
    form.mediaConcurrency = 4;
    form.importMode = 'incremental';
    form.channelIds = [];
    form.proxyEnabled = 0;
    form.proxyPool = '';
    form.remark = '';
    proxyEnabled.value = false;
  }

  function getTaskImportMode(row: Recordable) {
    if (!row.resultJson) return 'incremental';
    try {
      const data = JSON.parse(row.resultJson);
      return data?.importMode === 'overwrite' ? 'overwrite' : 'incremental';
    } catch {
      return 'incremental';
    }
  }

  function openRunModal(taskId: number, runType: string) {
    currentTaskId.value = taskId;
    runForm.runType = runType;
    runForm.scanMode = 'recent';
    runForm.recentCount = 100;
    runForm.importMode = 'incremental';
    runForm.tgMatchDays = 180;
    runForm.channelIds = decodeIdJson(
      tasks.value.find((item) => item.id === taskId)?.channelIdJson
    );
    runTgMatchEnabled.value = false;
    runModalVisible.value = true;
  }

  async function createRun() {
    if (!currentTaskId.value) return false;
    const tgMatchEnabled = runForm.runType !== 'scan' && runTgMatchEnabled.value;
    if (tgMatchEnabled && runForm.channelIds.length === 0) {
      message.warning('请选择需要匹配的TG频道');
      return false;
    }
    await ImportRunCreate({
      taskId: currentTaskId.value,
      ...runForm,
      tgMatchEnabled: tgMatchEnabled ? 1 : 0,
    });
    message.success('导入记录已创建并入队');
    activeTab.value = 'runs';
    await loadRuns();
  }

  async function retryRun(row: Recordable) {
    await ImportRunCreate({
      taskId: row.taskId,
      runType: row.runType,
      scanMode: row.scanMode,
      recentCount: row.recentCount,
      importMode: row.importMode,
    });
    message.success('已创建新的重试记录');
    await loadRuns();
  }

  async function cancelRun(id: number) {
    await ImportRunCancel({ id });
    message.success('导入记录已取消');
    await loadRuns();
  }

  async function deleteRun(id: number) {
    await ImportRunDelete({ id });
    message.success('导入记录已删除');
    await loadRuns();
  }

  async function openLogs(id: number) {
    currentRunId.value = id;
    logModalVisible.value = true;
    await loadLogs();
  }

  async function loadLogs() {
    if (!currentRunId.value) return;
    logLoading.value = true;
    try {
      const res: any = await ImportRunLogList({ runId: currentRunId.value, page: 1, perPage: 200 });
      logs.value = res?.list || [];
    } finally {
      logLoading.value = false;
    }
  }

  function parseLogContext(value: string) {
    if (!value) return {};
    try {
      return JSON.parse(value);
    } catch {
      return {};
    }
  }

  function renderLogContext(row: Recordable) {
    const context = row.parsedContext || parseLogContext(row.context);
    const parts = [];
    if (context.index && context.total) parts.push(`${context.index}/${context.total}`);
    if (context.mediaImported !== undefined && context.mediaTotal !== undefined)
      parts.push(`媒体 ${context.mediaImported}/${context.mediaTotal}`);
    if (context.size) parts.push(`${context.size}B`);
    if (context.path) parts.push(context.path);
    if (context.title) parts.push(context.title);
    return parts.join(' · ') || row.context || '';
  }

  async function clearCurrentLogs() {
    if (!currentRunId.value) return false;
    await ImportRunLogClear({ id: currentRunId.value });
    message.success('日志已清理');
    logs.value = [];
    return false;
  }

  function handleQueryTenantChange() {
    query.accountId = null;
  }

  function handleRunTenantChange() {
    runQuery.accountId = null;
  }

  function handleFormTenantChange() {
    form.accountId = null;
    form.channelIds = [];
  }

  function accountOptionsByTenant(tenantId: number | null) {
    return accounts.value
      .filter((item) => !tenantId || item.tenantId === tenantId)
      .map((item) => ({
        label: `${item.nickname || item.username} (${item.username})`,
        value: item.id,
      }));
  }

  function channelOptionsByTenant(tenantId: number | null) {
    return channels.value
      .filter((item) => !tenantId || item.tenantId === tenantId)
      .map((item) => ({
        label: `${item.channelTitle || item.channelUsername || item.targetChatId} (${item.targetChatId})`,
        value: item.id,
      }));
  }

  function matchRunStatusText(status: string) {
    const map = {
      pending: '待扫描',
      running: '扫描中',
      success: '完成',
      failed: '失败',
    };
    return map[status] || status || '-';
  }

  function matchRunStatusType(status: string) {
    const map = {
      pending: 'default',
      running: 'warning',
      success: 'success',
      failed: 'error',
    };
    return (map[status] || 'default') as any;
  }

  function decodeIdJson(value: string) {
    if (!value) return [];
    try {
      const data = JSON.parse(value);
      return Array.isArray(data) ? data.map((item) => Number(item)).filter((item) => item > 0) : [];
    } catch {
      return [];
    }
  }

  function accountOwnerName(item: Recordable) {
    return item.username
      ? `${item.name || item.tenantName || item.username} (${item.username})`
      : item.name || '-';
  }
</script>

<style scoped>
  .toolbar,
  .run-filter {
    margin-bottom: 12px;
  }

  .run-filter {
    padding: 12px;
    background: var(--n-color);
    border: 1px solid var(--n-border-color);
    border-radius: 6px;
  }

  .run-filter-grid {
    display: grid;
    grid-template-columns:
      minmax(180px, 1fr) minmax(180px, 1fr) minmax(120px, 0.7fr) minmax(120px, 0.7fr)
      minmax(220px, 1.2fr) auto;
    gap: 12px;
    align-items: center;
  }

  .filter-actions {
    min-width: 136px;
  }

  @media (max-width: 1180px) {
    .run-filter-grid {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }

    .filter-actions {
      justify-content: flex-start !important;
    }
  }

  @media (max-width: 640px) {
    .run-filter-grid {
      grid-template-columns: 1fr;
    }
  }

  :deep(.log-modal) {
    width: min(1080px, calc(100vw - 48px));
  }

  :deep(.match-modal) {
    width: min(1480px, calc(100vw - 48px));
  }

  :deep(.candidate-search-modal) {
    width: min(1280px, calc(100vw - 48px));
  }

  .match-config-grid {
    display: grid;
    grid-template-columns: minmax(320px, 1.4fr) minmax(140px, 0.5fr) minmax(140px, 0.5fr) auto;
    column-gap: 12px;
    align-items: flex-start;
  }

  .match-status {
    display: grid;
    gap: 8px;
  }

  .match-body {
    display: grid;
    grid-template-columns: minmax(560px, 1fr) minmax(520px, 0.9fr);
    gap: 14px;
    align-items: flex-start;
  }

  .candidate-panel {
    min-width: 0;
    padding: 12px;
    border: 1px solid var(--n-border-color);
    border-radius: 6px;
  }

  .review-header {
    display: flex;
    justify-content: space-between;
    gap: 12px;
    align-items: center;
    margin-bottom: 12px;
  }

  .review-grid {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
    gap: 12px;
    margin-bottom: 12px;
  }

  .review-text {
    min-width: 0;
    padding: 10px;
    border: 1px solid var(--n-border-color);
    border-radius: 6px;
    background: var(--n-color);
  }

  .binding-toolbar {
    display: flex;
    justify-content: space-between;
    gap: 8px;
    align-items: center;
    margin-bottom: 8px;
  }

  .binding-actions {
    margin-bottom: 10px;
  }

  .review-label {
    margin-bottom: 6px;
    font-size: 12px;
    color: var(--n-text-color-3);
  }

  .review-scroll {
    max-height: 180px;
  }

  .pre-wrap,
  .candidate-caption {
    white-space: pre-wrap;
    word-break: break-word;
    line-height: 1.6;
  }

  .muted-text {
    color: var(--n-text-color-3);
    font-size: 12px;
  }

  .candidate-list {
    display: grid;
    gap: 10px;
    max-height: 560px;
    overflow: auto;
    padding-right: 4px;
  }

  .candidate-tools {
    display: flex;
    justify-content: space-between;
    gap: 12px;
    align-items: center;
    margin-bottom: 10px;
  }

  .candidate-search-bar {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    gap: 10px;
  }

  .candidate-search-layout {
    display: grid;
    grid-template-columns: minmax(360px, 0.9fr) minmax(520px, 1.1fr);
    gap: 12px;
  }

  .candidate-search-scroll {
    max-height: 620px;
  }

  .candidate-search-list {
    display: grid;
    gap: 10px;
    max-height: 580px;
    overflow: auto;
    padding-right: 4px;
  }

  .candidate-search-pagination {
    justify-content: flex-end;
    margin-top: 12px;
  }

  .candidate-card {
    padding: 10px;
    border: 1px solid var(--n-border-color);
    border-radius: 6px;
    cursor: pointer;
    background: var(--n-color);
  }

  .candidate-card:hover {
    border-color: var(--n-primary-color);
  }

  .candidate-card.is-bound-display,
  .candidate-card.is-bound-verify {
    border-color: var(--n-primary-color);
    background: rgba(24, 160, 88, 0.08);
  }

  .candidate-card-head {
    display: flex;
    justify-content: space-between;
    gap: 8px;
    margin-bottom: 8px;
  }

  :deep(.is-active-match-row td) {
    background: rgba(24, 160, 88, 0.08);
  }

  @media (max-width: 1180px) {
    .match-config-grid,
    .match-body,
    .review-grid,
    .candidate-search-layout {
      grid-template-columns: 1fr;
    }

    .candidate-tools,
    .binding-toolbar {
      align-items: flex-start;
      flex-direction: column;
    }
  }
</style>
