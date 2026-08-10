<template>
  <div class="content-note-page">
    <div class="n-layout-page-header">
      <n-card :bordered="false" title="笔记管理">
        <div class="page-title-extra">
          <span>共 {{ totalCount }} 条</span>
          <span>按导入时间倒序浏览</span>
        </div>
      </n-card>
    </div>

    <n-card :bordered="false" class="proCard filter-card">
      <BasicForm ref="searchFormRef" @register="register" @submit="handleSearch" @reset="handleReset" @keyup.enter="handleSearch" />
      <div class="filter-toolbar">
        <n-space>
          <n-button type="primary" @click="handleSearch">查询</n-button>
          <n-button @click="handleReset">重置筛选</n-button>
          <n-button @click="loadNotes">刷新</n-button>
          <n-button :type="multiSelectMode ? 'primary' : 'default'" @click="toggleMultiSelect">
            {{ multiSelectMode ? '退出多选' : '开启多选' }}
          </n-button>
        </n-space>
        <div class="filter-toolbar__summary">第 {{ pager.page }} / {{ pageCount || 1 }} 页</div>
      </div>
      <div v-if="multiSelectMode" class="batch-toolbar">
        <n-space align="center">
          <span>已选 {{ selectedIds.length }} 条</span>
          <n-button size="small" type="success" :disabled="!selectedIds.length" @click="handleBatchReview('approved')">审核通过</n-button>
          <n-button size="small" type="warning" :disabled="!selectedIds.length" @click="handleBatchReview('rejected')">驳回</n-button>
          <n-button size="small" type="warning" :disabled="!selectedIds.length" @click="handleBatchStatus(2)">冻结</n-button>
          <n-button size="small" :disabled="!selectedIds.length" @click="handleBatchStatus(1)">解冻</n-button>
          <n-button size="small" :disabled="!selectedIds.length" @click="openRemarkModal">备注</n-button>
          <n-button size="small" type="error" :disabled="!selectedIds.length" @click="handleBatchDelete">删除</n-button>
          <n-button v-if="selectedIds.length" size="small" text @click="selectedIds = []">清空</n-button>
        </n-space>
      </div>
    </n-card>

    <n-spin :show="loading">
      <div v-if="noteList.length" class="note-grid">
        <n-card v-for="item in noteList" :key="item.id" :bordered="false" class="note-card" hoverable>
          <div class="note-card__media" @click="openPreview(item, 0)">
            <n-checkbox
              v-if="multiSelectMode"
              class="note-card__selector"
              :checked="isSelected(item.id)"
              @update:checked="toggleSelected(item.id)"
              @click.stop
            />
            <img v-if="resolvePreviewUrl(primaryMedia(item)) && primaryMedia(item)?.mediaType === 'image'" :src="resolvePreviewUrl(primaryMedia(item))" class="note-card__cover" />
            <img
              v-else-if="resolvePreviewUrl(primaryMedia(item)) && primaryMedia(item)?.mediaType === 'video'"
              :src="resolvePreviewUrl(primaryMedia(item))"
              class="note-card__cover"
            />
            <video v-else-if="resolvePlayableUrl(primaryMedia(item)) && primaryMedia(item)?.mediaType === 'video'" :src="resolvePlayableUrl(primaryMedia(item))" class="note-card__cover" muted preload="metadata" />
            <div v-else class="note-card__empty">暂无预览</div>
            <div v-if="primaryMedia(item)?.mediaType === 'video'" class="note-card__play">▶</div>
            <div class="note-card__overlay" @click.stop="openDetail(item)">
              <div class="note-card__code">{{ item.profileNo || `#${item.id}` }}</div>
              <div class="note-card__channel">{{ item.channelTitle || item.channelUsername || `频道 ${item.sourceChannelId || '-'}` }}</div>
            </div>
          </div>

          <div v-if="mediaItems(item).length" class="note-card__thumbs">
            <button
              v-for="(media, index) in mediaItems(item)"
              :key="`${item.id}-${media.id || index}`"
              type="button"
              class="note-card__thumb"
              @click="openPreview(item, index)"
            >
              <img v-if="resolvePreviewUrl(media)" :src="resolvePreviewUrl(media)" />
              <video v-else-if="resolvePlayableUrl(media) && media.mediaType === 'video'" :src="resolvePlayableUrl(media)" muted preload="metadata" />
              <span v-else>-</span>
              <b v-if="media.mediaType === 'video'">视频</b>
            </button>
          </div>

          <div class="note-card__body" @click="openDetail(item)">
            <div class="note-card__title">{{ item.title || item.profileNo || '未命名笔记' }}</div>
            <div class="note-card__meta">
              <n-tag size="small" :bordered="false">{{ areaText(item) }}</n-tag>
              <n-tag size="small" :bordered="false">年龄 {{ item.age || '-' }}</n-tag>
              <n-tag size="small" :bordered="false">身高 {{ item.height || '-' }}</n-tag>
              <n-tag size="small" :bordered="false">体重 {{ item.weight || '-' }}</n-tag>
              <n-tag size="small" :bordered="false">标签 {{ item.cupSize || '-' }}</n-tag>
              <n-tag size="small" :bordered="false">陪伴 {{ item.daysWithEscort || '-' }}</n-tag>
              <n-tag size="small" :bordered="false">生活费 {{ item.expectedLivingCost || '-' }}</n-tag>
              <n-tag size="small" :bordered="false">
                来源 {{ item.sourceChannelTitle || item.sourceChannelUsername || item.sourceNoteId || '-' }}
              </n-tag>
            </div>
            <div class="note-card__params">
              <n-tag v-for="tag in paramTags(item)" :key="tag" size="small" :bordered="false" type="info">{{ tag }}</n-tag>
            </div>
            <div class="note-card__tags">
              <component :is="renderReview(item.reviewStatus)" />
              <component :is="renderImport(item.importStatus)" />
              <component :is="renderVisibility(item.visibility)" />
              <n-tag v-if="item.hasVerificationVideo" type="success" size="small" :bordered="false">验证视频</n-tag>
              <n-tag v-if="item.memberOnlyVideo" type="warning" size="small" :bordered="false">会员视频</n-tag>
              <n-tag v-if="item.status === 2" type="error" size="small" :bordered="false">冻结</n-tag>
              <n-tag v-if="item.duplicateOfId" type="warning" size="small" :bordered="false">重复 #{{ item.duplicateOfId }}</n-tag>
              <n-tag v-if="similarImageCount(item)" type="warning" size="small" :bordered="false">相似图片 {{ similarImageCount(item) }} 张</n-tag>
            </div>
            <div v-if="item.adminRemark" class="note-card__remark">备注：{{ item.adminRemark }}</div>
            <div class="note-card__text">{{ noteText(item) }}</div>
          </div>

          <template #action>
            <div class="note-card__actions">
              <span>{{ item.imageCount || 0 }} 图 / {{ item.videoCount || 0 }} 视频 / 相似 {{ similarImageCount(item) }}</span>
              <n-button size="small" type="primary" @click="openDetail(item)">详情/编辑</n-button>
            </div>
          </template>
        </n-card>
      </div>
      <n-empty v-else-if="!loading" description="当前没有符合条件的笔记" class="empty-state" />
    </n-spin>

    <div v-if="totalCount > 0" class="pagination-wrap">
      <n-pagination
        v-model:page="pager.page"
        v-model:page-size="pager.pageSize"
        :item-count="totalCount"
        :page-sizes="[12, 24, 36, 48]"
        show-size-picker
        @update:page="loadNotes"
        @update:page-size="handlePageSizeChange"
      />
    </div>

    <n-modal v-model:show="previewVisible" preset="card" class="media-preview-modal" :bordered="false">
      <template #header>{{ previewTitle }}</template>
      <div v-if="activePreviewMedia" class="preview-stage">
        <img v-if="activePreviewMedia.mediaType === 'image' && activePreviewUrl" :src="activePreviewUrl" />
        <video v-else-if="activePreviewUrl" :src="activePreviewUrl" controls autoplay preload="metadata" />
        <n-empty v-else description="媒体地址不可用" />
      </div>
      <div v-if="activePreviewList.length > 1" class="preview-thumbs">
        <button
          v-for="(media, index) in activePreviewList"
          :key="`preview-${media.id || index}`"
          type="button"
          class="preview-thumb"
          :class="{ 'is-active': previewIndex === index }"
          @click="previewIndex = index"
        >
          <img v-if="resolvePreviewUrl(media)" :src="resolvePreviewUrl(media)" />
          <video v-else-if="resolvePlayableUrl(media) && media.mediaType === 'video'" :src="resolvePlayableUrl(media)" muted preload="metadata" />
          <span v-else>-</span>
        </button>
      </div>
    </n-modal>

    <n-modal v-model:show="remarkVisible" preset="dialog" title="批量备注" positive-text="保存" negative-text="取消" @positive-click="handleBatchRemark">
      <n-input v-model:value="remarkValue" type="textarea" :autosize="{ minRows: 4, maxRows: 8 }" placeholder="输入后台备注" />
    </n-modal>

    <ViewDrawer ref="viewRef" @reloadTable="loadNotes" />
  </div>
</template>

<script lang="ts" setup>
  import { computed, onMounted, reactive, ref } from 'vue';
  import { useDialog, useMessage } from 'naive-ui';
  import { BasicForm, useForm } from '@/components/Form/index';
  import { BatchDelete, BatchRemark, BatchReview, BatchStatus, List } from '@/api/contentNote';
  import { renderImport, renderReview, renderVisibility, schemas } from './model';
  import ViewDrawer from './view.vue';

  const loading = ref(false);
  const message = useMessage();
  const dialog = useDialog();
  const noteList = ref<Recordable[]>([]);
  const totalCount = ref(0);
  const pageCount = ref(0);
  const searchFormRef = ref<any>({});
  const viewRef = ref();
  const previewVisible = ref(false);
  const previewIndex = ref(0);
  const activePreviewNote = ref<Recordable>({});
  const multiSelectMode = ref(false);
  const selectedIds = ref<number[]>([]);
  const remarkVisible = ref(false);
  const remarkValue = ref('');
  const pager = reactive({
    page: 1,
    pageSize: 24,
  });

  const [register] = useForm({
    gridProps: { cols: '1 s:1 m:2 l:3 xl:4 2xl:4' },
    labelWidth: 90,
    schemas,
  });

  const activePreviewList = computed(() => mediaItems(activePreviewNote.value));
  const activePreviewMedia = computed(() => activePreviewList.value[previewIndex.value]);
  const activePreviewUrl = computed(() => {
    const media = activePreviewMedia.value;
    if (media?.mediaType === 'video') {
      return resolvePlayableUrl(media);
    }
    return resolvePreviewUrl(media);
  });
  const previewTitle = computed(() => {
    const note = activePreviewNote.value || {};
    return `${note.profileNo || `#${note.id || '-'}`} 媒体预览`;
  });

  onMounted(() => {
    loadNotes();
  });

  async function loadNotes() {
    loading.value = true;
    try {
      const res = await List({
        ...searchFormRef.value?.formModel,
        page: pager.page,
        pageSize: pager.pageSize,
      });
      noteList.value = res.list || [];
      selectedIds.value = selectedIds.value.filter((id) => noteList.value.some((item) => item.id === id));
      totalCount.value = res.totalCount || 0;
      pageCount.value = res.pageCount || 0;
      pager.page = res.page || pager.page;
      pager.pageSize = res.pageSize || pager.pageSize;
    } finally {
      loading.value = false;
    }
  }

  function handleSearch() {
    pager.page = 1;
    loadNotes();
  }

  function handleReset() {
    pager.page = 1;
    loadNotes();
  }

  function handlePageSizeChange(pageSize: number) {
    pager.page = 1;
    pager.pageSize = pageSize;
    loadNotes();
  }

  function mediaItems(item: Recordable) {
    return Array.isArray(item?.media) ? item.media : [];
  }

  function primaryMedia(item: Recordable) {
    return mediaItems(item)[0];
  }

  function resolveMediaUrl(media: Recordable | undefined) {
    return resolvePreviewUrl(media);
  }

  function resolvePreviewUrl(media: Recordable | undefined) {
    if (!media) {
      return '';
    }
    return media.previewStoragePath || media.displayStoragePath || '';
  }

  function resolvePlayableUrl(media: Recordable | undefined) {
    if (!media) {
      return '';
    }
    if (media.mediaType === 'video') {
      return media.displayStoragePath || media.previewStoragePath || '';
    }
    return resolvePreviewUrl(media);
  }

  function areaText(item: Recordable) {
    return [item.province, item.city].filter(Boolean).join(' / ') || '地区 -';
  }

  function noteText(item: Recordable) {
    return item.plainText || item.summary || '-';
  }

  function similarImageCount(item: Recordable) {
    return mediaItems(item).filter((media) => media.mediaType === 'image' && Number(media.duplicateOfMediaId || 0) > 0).length;
  }

  function paramTags(item: Recordable) {
    const options = [
      ['canFlyToProvince', '可飞'],
      ['canGoAbroad', '出国'],
      ['canOvernight', '过夜'],
      ['canCohabitate', '同居'],
      ['hasHealthCheck', '体检'],
      ['isFullMonth', '满月'],
      ['isVirgin', '处'],
      ['acceptSm', 'SM'],
      ['noCondomAfterCheck', '无套'],
      ['allowCreampie', '内射'],
      ['hasTattoo', '纹身'],
      ['isFavorite', '收藏'],
    ];
    return options.filter(([key]) => Number(item?.[key] || 0) === 1).map(([, label]) => label);
  }

  function openDetail(item: Recordable) {
    viewRef.value?.openModal(item);
  }

  function openPreview(item: Recordable, index: number) {
    if (!mediaItems(item).length) {
      openDetail(item);
      return;
    }
    activePreviewNote.value = item;
    previewIndex.value = index;
    previewVisible.value = true;
  }

  function toggleMultiSelect() {
    multiSelectMode.value = !multiSelectMode.value;
    if (!multiSelectMode.value) {
      selectedIds.value = [];
    }
  }

  function isSelected(id: number) {
    return selectedIds.value.includes(id);
  }

  function toggleSelected(id: number) {
    if (isSelected(id)) {
      selectedIds.value = selectedIds.value.filter((item) => item !== id);
      return;
    }
    selectedIds.value = [...selectedIds.value, id];
  }

  async function handleBatchReview(reviewStatus: string) {
    await BatchReview({ ids: selectedIds.value, reviewStatus });
    message.success('批量审核完成');
    await loadNotes();
  }

  async function handleBatchStatus(status: number) {
    await BatchStatus({ ids: selectedIds.value, status });
    message.success(status === 2 ? '已冻结' : '已解冻');
    await loadNotes();
  }

  function handleBatchDelete() {
    dialog.warning({
      title: '确认删除',
      content: `确认删除已选 ${selectedIds.value.length} 条笔记？`,
      positiveText: '删除',
      negativeText: '取消',
      onPositiveClick: async () => {
        await BatchDelete({ ids: selectedIds.value });
        message.success('批量删除完成');
        selectedIds.value = [];
        await loadNotes();
      },
    });
  }

  function openRemarkModal() {
    remarkValue.value = '';
    remarkVisible.value = true;
  }

  async function handleBatchRemark() {
    await BatchRemark({ ids: selectedIds.value, adminRemark: remarkValue.value });
    message.success('批量备注完成');
    await loadNotes();
  }
</script>

<style lang="less" scoped>
  .content-note-page {
    display: flex;
    flex-direction: column;
    gap: 16px;
  }

  .page-title-extra,
  .filter-toolbar,
  .note-card__actions {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
  }

  .page-title-extra {
    color: #667085;
    font-size: 13px;
  }

  .filter-card {
    margin-bottom: 0;
  }

  .filter-toolbar {
    margin-top: 12px;
  }

  .batch-toolbar {
    margin-top: 12px;
    padding-top: 12px;
    border-top: 1px solid #eef0f3;
  }

  .filter-toolbar__summary {
    color: #667085;
    font-size: 13px;
  }

  .note-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(292px, 1fr));
    gap: 16px;
  }

  .note-card {
    overflow: hidden;
  }

  .note-card__media {
    position: relative;
    aspect-ratio: 4 / 5;
    overflow: hidden;
    border-radius: 8px;
    background: #111827;
    cursor: pointer;
  }

  .note-card__selector {
    position: absolute;
    top: 10px;
    right: 10px;
    z-index: 4;
    padding: 6px;
    border-radius: 8px;
    background: rgba(255, 255, 255, 0.88);
  }

  .note-card__cover,
  .note-card__empty {
    width: 100%;
    height: 100%;
  }

  .note-card__cover {
    object-fit: cover;
    display: block;
  }

  .note-card__empty {
    display: flex;
    align-items: center;
    justify-content: center;
    color: rgba(255, 255, 255, 0.72);
    background: #1f2937;
  }

  .note-card__play {
    position: absolute;
    top: 10px;
    left: 10px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 30px;
    height: 30px;
    color: #fff;
    border-radius: 50%;
    background: rgba(0, 0, 0, 0.58);
  }

  .note-card__overlay {
    position: absolute;
    right: 0;
    bottom: 0;
    left: 0;
    padding: 12px;
    color: #fff;
    background: linear-gradient(180deg, rgba(17, 24, 39, 0), rgba(17, 24, 39, 0.88));
  }

  .note-card__code {
    font-size: 18px;
    font-weight: 800;
  }

  .note-card__channel {
    margin-top: 4px;
    font-size: 12px;
    opacity: 0.86;
  }

  .note-card__thumbs,
  .preview-thumbs {
    display: flex;
    gap: 8px;
    overflow-x: auto;
    padding: 10px 0 0;
  }

  .note-card__thumb,
  .preview-thumb {
    position: relative;
    flex: 0 0 56px;
    width: 56px;
    height: 56px;
    padding: 0;
    overflow: hidden;
    border: 1px solid #e5e7eb;
    border-radius: 8px;
    background: #111827;
    cursor: pointer;
  }

  .note-card__thumb img,
  .note-card__thumb video,
  .preview-thumb img,
  .preview-thumb video {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
  }

  .note-card__thumb b {
    position: absolute;
    right: 4px;
    bottom: 4px;
    padding: 1px 4px;
    color: #fff;
    font-size: 10px;
    border-radius: 999px;
    background: rgba(0, 0, 0, 0.66);
  }

  .note-card__body {
    padding-top: 12px;
    cursor: pointer;
  }

  .note-card__title {
    min-height: 22px;
    color: #101828;
    font-size: 15px;
    font-weight: 700;
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
  }

  .note-card__meta,
  .note-card__params,
  .note-card__tags {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    margin-top: 8px;
  }

  .note-card__text {
    height: 70px;
    margin-top: 10px;
    color: #667085;
    font-size: 13px;
    line-height: 1.7;
    overflow: hidden;
  }

  .note-card__remark {
    margin-top: 8px;
    color: #d97706;
    font-size: 12px;
    line-height: 1.5;
  }

  .note-card__actions {
    color: #667085;
    font-size: 13px;
  }

  .pagination-wrap {
    display: flex;
    justify-content: flex-end;
    padding: 6px 0 18px;
  }

  .empty-state {
    padding: 60px 0;
  }

  .media-preview-modal {
    width: min(920px, 92vw);
  }

  .preview-stage {
    min-height: 520px;
    background: #111827;
    border-radius: 8px;
    overflow: hidden;
  }

  .preview-stage img,
  .preview-stage video {
    width: 100%;
    height: 520px;
    object-fit: contain;
    display: block;
  }

  .preview-thumb.is-active {
    border-color: #18a058;
  }
</style>
