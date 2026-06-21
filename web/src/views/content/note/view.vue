<template>
  <n-drawer v-model:show="showModal" :width="drawerWidth">
    <n-drawer-content closable>
      <template #header>笔记详情 #{{ data.id || '-' }}</template>
      <n-spin :show="loading">
        <n-tabs type="line" animated>
          <n-tab-pane name="basic" tab="基础信息">
            <n-space justify="end" class="mb-3">
              <n-button type="primary" :loading="saving" @click="handleSave">保存笔记</n-button>
            </n-space>
            <n-descriptions bordered label-placement="left" :column="2">
              <n-descriptions-item label="编号">{{ data.profileNo || '-' }}</n-descriptions-item>
              <n-descriptions-item label="来源">{{ data.sourceType || '-' }}</n-descriptions-item>
              <n-descriptions-item label="来源ID">{{ data.sourceNoteId || '-' }}</n-descriptions-item>
              <n-descriptions-item label="来源键">{{ data.sourceKey || '-' }}</n-descriptions-item>
              <n-descriptions-item label="频道">{{ data.channelTitle || data.channelUsername || '-' }}</n-descriptions-item>
              <n-descriptions-item label="频道ID">{{ data.sourceChannelId || '-' }}</n-descriptions-item>
              <n-descriptions-item label="TG Chat">{{ data.tgChatId || '-' }}</n-descriptions-item>
              <n-descriptions-item label="消息ID">{{ data.sourceMessageId || '-' }}</n-descriptions-item>
              <n-descriptions-item label="图片/视频">{{ data.imageCount || 0 }} / {{ data.videoCount || 0 }}</n-descriptions-item>
              <n-descriptions-item label="相似图片">{{ similarImageCount(data) }} 张</n-descriptions-item>
              <n-descriptions-item label="重复ID">{{ data.duplicateOfId || '-' }}</n-descriptions-item>
              <n-descriptions-item label="文本哈希">{{ data.sourceTextHash || '-' }}</n-descriptions-item>
              <n-descriptions-item label="地区">{{ [data.province, data.city].filter(Boolean).join(' / ') || '-' }}</n-descriptions-item>
              <n-descriptions-item label="年龄/身高/体重">{{ data.age || '-' }} / {{ data.height || '-' }} / {{ data.weight || '-' }}</n-descriptions-item>
              <n-descriptions-item label="标签">{{ data.cupSize || '-' }}</n-descriptions-item>
              <n-descriptions-item label="验证视频">{{ data.hasVerificationVideo ? '是' : '否' }}</n-descriptions-item>
              <n-descriptions-item label="会员视频">{{ data.memberOnlyVideo ? '是' : '否' }}</n-descriptions-item>
              <n-descriptions-item label="可见性">{{ data.visibility || '-' }}</n-descriptions-item>
              <n-descriptions-item label="审核状态">{{ data.reviewStatus || '-' }}</n-descriptions-item>
              <n-descriptions-item label="导入状态">{{ data.importStatus || '-' }}</n-descriptions-item>
              <n-descriptions-item label="状态">{{ data.status === 2 ? '冻结' : '正常' }}</n-descriptions-item>
              <n-descriptions-item label="发布时间">{{ data.publishedAt || '-' }}</n-descriptions-item>
              <n-descriptions-item label="导入时间">{{ data.createdAt || '-' }}</n-descriptions-item>
            </n-descriptions>

            <n-form ref="formRef" :model="formValue" label-placement="left" label-width="82" class="mt-4">
              <n-grid :cols="2" :x-gap="16">
                <n-form-item-gi label="标题" path="title" :span="2">
                  <n-input v-model:value="formValue.title" clearable />
                </n-form-item-gi>
                <n-form-item-gi label="省份" path="province">
                  <n-input v-model:value="formValue.province" clearable />
                </n-form-item-gi>
                <n-form-item-gi label="城市" path="city">
                  <n-input v-model:value="formValue.city" clearable />
                </n-form-item-gi>
                <n-form-item-gi label="年龄" path="age">
                  <n-input-number v-model:value="formValue.age" :show-button="false" clearable class="w-full" />
                </n-form-item-gi>
                <n-form-item-gi label="身高" path="height">
                  <n-input-number v-model:value="formValue.height" :show-button="false" clearable class="w-full" />
                </n-form-item-gi>
                <n-form-item-gi label="体重" path="weight">
                  <n-input-number v-model:value="formValue.weight" :show-button="false" clearable class="w-full" />
                </n-form-item-gi>
                <n-form-item-gi label="标签" path="cupSize">
                  <n-input v-model:value="formValue.cupSize" clearable />
                </n-form-item-gi>
                <n-form-item-gi label="可见性" path="visibility">
                  <n-select v-model:value="formValue.visibility" :options="visibilityOptions" />
                </n-form-item-gi>
                <n-form-item-gi label="审核" path="reviewStatus">
                  <n-select v-model:value="formValue.reviewStatus" :options="reviewOptions" />
                </n-form-item-gi>
                <n-form-item-gi label="导入" path="importStatus">
                  <n-select v-model:value="formValue.importStatus" :options="importOptions" />
                </n-form-item-gi>
                <n-form-item-gi label="状态" path="status">
                  <n-select v-model:value="formValue.status" :options="statusOptions" />
                </n-form-item-gi>
                <n-form-item-gi label="验证视频" path="hasVerificationVideo">
                  <n-select v-model:value="formValue.hasVerificationVideo" :options="yesNoOptions" />
                </n-form-item-gi>
                <n-form-item-gi label="会员视频" path="memberOnlyVideo">
                  <n-select v-model:value="formValue.memberOnlyVideo" :options="yesNoOptions" />
                </n-form-item-gi>
                <n-form-item-gi label="正文" path="plainText" :span="2">
                  <n-input v-model:value="formValue.plainText" type="textarea" :autosize="{ minRows: 8, maxRows: 18 }" />
                </n-form-item-gi>
                <n-form-item-gi label="后台备注" path="adminRemark" :span="2">
                  <n-input v-model:value="formValue.adminRemark" type="textarea" :autosize="{ minRows: 2, maxRows: 6 }" />
                </n-form-item-gi>
              </n-grid>
            </n-form>
          </n-tab-pane>

          <n-tab-pane name="source" tab="来源">
            <n-descriptions bordered label-placement="left" :column="1">
              <n-descriptions-item label="来源类型">{{ data.source?.sourceType || '-' }}</n-descriptions-item>
              <n-descriptions-item label="来源键">{{ data.source?.sourceKey || '-' }}</n-descriptions-item>
              <n-descriptions-item label="频道ID">{{ data.source?.sourceChannelId || '-' }}</n-descriptions-item>
              <n-descriptions-item label="消息ID">{{ data.source?.sourceMessageId || '-' }}</n-descriptions-item>
              <n-descriptions-item label="分组ID">{{ data.source?.sourceGroupedId || '-' }}</n-descriptions-item>
              <n-descriptions-item label="文本哈希">{{ data.source?.sourceTextHash || '-' }}</n-descriptions-item>
            </n-descriptions>
            <n-card class="mt-4" size="small" title="同步属性" :bordered="false">
              <n-descriptions v-if="attributeRows.length" bordered label-placement="left" :column="2">
                <n-descriptions-item v-for="item in attributeRows" :key="item.key" :label="item.label">
                  {{ item.value }}
                </n-descriptions-item>
              </n-descriptions>
              <n-empty v-else description="暂无同步属性" />
            </n-card>
            <n-card class="mt-4" size="small" title="原始文本" :bordered="false">
              <n-input :value="data.source?.rawText || ''" type="textarea" readonly :autosize="{ minRows: 6, maxRows: 14 }" />
            </n-card>
            <n-card class="mt-4" size="small" title="原始消息 JSON" :bordered="false">
              <n-input :value="data.source?.rawMessageJson || ''" type="textarea" readonly :autosize="{ minRows: 8, maxRows: 18 }" />
            </n-card>
          </n-tab-pane>

          <n-tab-pane name="media" tab="媒体">
            <n-data-table :columns="mediaColumns" :data="data.media || []" :pagination="false" size="small" />
          </n-tab-pane>
        </n-tabs>
      </n-spin>
    </n-drawer-content>
  </n-drawer>

  <n-modal v-model:show="previewVisible" preset="card" class="content-note-preview-modal" :bordered="false">
    <template #header>媒体预览 #{{ previewMedia?.id || '-' }}</template>
    <div class="content-note-preview-stage">
      <img v-if="previewMedia?.mediaType === 'image' && previewUrl" :src="previewUrl" />
      <video v-else-if="previewUrl" :src="previewUrl" controls autoplay preload="metadata" />
      <n-empty v-else description="媒体地址不可用" />
    </div>
  </n-modal>
</template>

<script lang="ts" setup>
  import { computed, h, ref } from 'vue';
  import { NButton, NInput, NInputNumber, NSelect, NTag, useMessage } from 'naive-ui';
  import { Edit, MediaEdit, View } from '@/api/contentNote';
  import { adaModalWidth } from '@/utils/hotgo';
  import UploadImage from '@/components/Upload/uploadImage.vue';

  const emit = defineEmits(['reloadTable']);
  const message = useMessage();
  const showModal = ref(false);
  const loading = ref(false);
  const saving = ref(false);
  const data = ref<any>({});
  const formValue = ref<any>({});
  const formRef = ref<any>({});
  const drawerWidth = ref(adaModalWidth(960));
  const previewVisible = ref(false);
  const previewMedia = ref<any>();
  const previewUrl = computed(() => resolveMediaUrl(previewMedia.value));
  const attributeRows = computed(() => buildAttributeRows(data.value));

  const visibilityOptions = [
    { label: '私有', value: 'private' },
    { label: '公开', value: 'public' },
    { label: '会员', value: 'member_only' },
  ];
  const reviewOptions = [
    { label: '待审核', value: 'pending' },
    { label: '已通过', value: 'approved' },
    { label: '已拒绝', value: 'rejected' },
  ];
  const importOptions = [
    { label: '已导入', value: 'imported' },
    { label: '重复', value: 'duplicate' },
  ];
  const statusOptions = [
    { label: '正常', value: 1 },
    { label: '停用', value: 2 },
  ];
  const yesNoOptions = [
    { label: '否', value: 0 },
    { label: '是', value: 1 },
  ];

  const editableFields = [
    'id',
    'title',
    'summary',
    'plainText',
    'province',
    'city',
    'age',
    'height',
    'weight',
    'cupSize',
    'hasVerificationVideo',
    'memberOnlyVideo',
    'visibility',
    'reviewStatus',
    'importStatus',
    'adminRemark',
    'status',
  ];

  const mediaColumns = [
    { title: 'ID', key: 'id', width: 80 },
    {
      title: '类型',
      key: 'mediaType',
      width: 90,
      render(row) {
        return h(NTag, { bordered: false, type: row.mediaType === 'video' ? 'warning' : 'success' }, { default: () => row.mediaType });
      },
    },
    { title: '来源资源', key: 'sourceAssetId', width: 120 },
    { title: '重复媒体', key: 'duplicateOfMediaId', width: 120 },
    {
      title: '预览',
      key: 'mediaPreview',
      width: 180,
      render(row) {
        const previewUrl = resolvePreviewUrl(row);
        const playableUrl = resolveMediaUrl(row);
        if (!previewUrl && !playableUrl) {
          return '-';
        }
        if (row.mediaType === 'video') {
          if (previewUrl) {
            return h('img', {
              src: previewUrl,
              class: 'content-note-image',
              onClick: () => openPreview(row),
            });
          }
          return h('video', {
            src: playableUrl,
            controls: true,
            preload: 'metadata',
            playsinline: true,
            class: 'content-note-video',
            onClick: () => openPreview(row),
          });
        }
        return h('img', {
          src: previewUrl,
          class: 'content-note-image',
          onClick: () => openPreview(row),
        });
      },
    },
    {
      title: '展示图',
      key: 'displayStoragePath',
      width: 150,
      render(row) {
        if (row.mediaType === 'image') {
          return h(UploadImage, {
            value: row.displayStoragePath,
            maxNumber: 1,
            'onUpdate:value': (value) => {
              row.displayStoragePath = value;
              if (!row.previewStoragePath) {
                row.previewStoragePath = value;
              }
            },
          });
        }
        return h(NInput, {
          value: row.displayStoragePath,
          clearable: true,
          onUpdateValue(value) {
            row.displayStoragePath = value;
          },
        });
      },
    },
    {
      title: '预览图',
      key: 'previewStoragePath',
      width: 150,
      render(row) {
        if (row.mediaType === 'image') {
          return h(UploadImage, {
            value: row.previewStoragePath,
            maxNumber: 1,
            'onUpdate:value': (value) => {
              row.previewStoragePath = value;
            },
          });
        }
        return h(NInput, {
          value: row.previewStoragePath,
          clearable: true,
          onUpdateValue(value) {
            row.previewStoragePath = value;
          },
        });
      },
    },
    {
      title: '排序',
      key: 'sortIndex',
      width: 110,
      render(row) {
        return h(NInputNumber, {
          value: row.sortIndex,
          showButton: false,
          onUpdateValue(value) {
            row.sortIndex = value || 0;
          },
        });
      },
    },
    {
      title: '状态',
      key: 'status',
      width: 110,
      render(row) {
        return h(NSelect, {
          value: row.status || 1,
          options: statusOptions,
          onUpdateValue(value) {
            row.status = value;
          },
        });
      },
    },
    { title: 'MD5', key: 'binaryMd5', width: 180, ellipsis: { tooltip: true } },
    { title: '尺寸', key: 'width', width: 120, render(row) { return row.width && row.height ? `${row.width}x${row.height}` : '-'; } },
    { title: '处理', key: 'processStatus', width: 100 },
    { title: '加密', key: 'encryptStatus', width: 100 },
    {
      title: '操作',
      key: 'action',
      width: 90,
      fixed: 'right',
      render(row) {
        return h(NButton, { size: 'small', type: 'primary', onClick: () => handleMediaSave(row) }, { default: () => '保存' });
      },
    },
  ];

  async function openModal(record: Recordable) {
    showModal.value = true;
    loading.value = true;
    try {
      data.value = await View({ id: record.id });
      syncForm();
    } finally {
      loading.value = false;
    }
  }

  function syncForm() {
    const next: Recordable = {};
    editableFields.forEach((field) => {
      next[field] = data.value?.[field];
    });
    formValue.value = next;
  }

  async function handleSave() {
    saving.value = true;
    try {
      await Edit(formValue.value);
      message.success('保存成功');
      data.value = await View({ id: formValue.value.id });
      syncForm();
      emit('reloadTable');
    } finally {
      saving.value = false;
    }
  }

  async function handleMediaSave(row: Recordable) {
    await MediaEdit({
      id: row.id,
      displayStoragePath: row.displayStoragePath,
      previewStoragePath: row.previewStoragePath,
      sortIndex: row.sortIndex || 0,
      status: row.status || 1,
    });
    message.success('媒体保存成功');
    data.value = await View({ id: data.value.id });
    emit('reloadTable');
  }

  function resolveMediaUrl(row?: Recordable) {
    if (!row) {
      return '';
    }
    if (row.mediaType === 'video') {
      return row.displayStoragePath || row.previewStoragePath || '';
    }
    return resolvePreviewUrl(row);
  }

  function resolvePreviewUrl(row?: Recordable) {
    if (!row) {
      return '';
    }
    return row.previewStoragePath || row.displayStoragePath || '';
  }

  function openPreview(row: Recordable) {
    previewMedia.value = row;
    previewVisible.value = true;
  }

  function similarImageCount(row: Recordable) {
    const list = Array.isArray(row?.media) ? row.media : [];
    return list.filter((media) => media.mediaType === 'image' && Number(media.duplicateOfMediaId || 0) > 0).length;
  }

  function buildAttributeRows(row?: Recordable) {
    if (!row) {
      return [];
    }
    const labels: Record<string, string> = {
      htmlText: 'HTML正文',
      sourceCategoryCode: '分类',
      daysWithEscort: '陪伴天数',
      expectedLivingCost: '期望生活费',
      canFlyToProvince: '可飞外省',
      canGoAbroad: '可出国',
      canOvernight: '可过夜',
      canCohabitate: '可同居',
      hasHealthCheck: '有体检',
      isFullMonth: '满月',
      isVirgin: '是否处',
      acceptSm: '接受SM',
      noCondomAfterCheck: '体检后无套',
      allowCreampie: '可内射',
      hasTattoo: '有纹身',
      isFavorite: '收藏',
      sourceEditedAt: '编辑时间',
      textBlockCount: '文本块数',
      storagePolicy: '存储策略',
      sourceRemark: 'FeiNiu备注',
      sourceCreateBy: '源创建者',
      sourceCreatedAt: '源创建时间',
      sourceUpdateBy: '源更新者',
      sourceUpdatedAt: '源更新时间',
      groupParams: '分组参数',
      tagParams: '标签参数',
    };
    return Object.keys(labels)
      .map((key) => ({
        key,
        label: labels[key],
        value: formatAttributeValue(row[key]),
      }))
      .filter((item) => item.value !== '-');
  }

  function formatAttributeValue(value: any) {
    if (value === null || value === undefined || value === '') {
      return '-';
    }
    if (value === 'Y') {
      return '是';
    }
    if (value === 'N') {
      return '否';
    }
    if (typeof value === 'object') {
      return JSON.stringify(value);
    }
    return String(value);
  }

  defineExpose({ openModal });
</script>

<style lang="less" scoped>
  .content-note-video {
    width: 160px;
    height: 90px;
    object-fit: cover;
    background: #000;
    cursor: pointer;
  }

  .content-note-image {
    width: 120px;
    height: 90px;
    object-fit: cover;
    display: block;
    cursor: pointer;
    background: #f3f4f6;
  }

  .content-note-preview-modal {
    width: min(920px, 92vw);
  }

  .content-note-preview-stage {
    min-height: 520px;
    background: #111827;
    border-radius: 8px;
    overflow: hidden;
  }

  .content-note-preview-stage img,
  .content-note-preview-stage video {
    width: 100%;
    height: 520px;
    object-fit: contain;
    display: block;
  }
</style>
