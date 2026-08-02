<template>
  <BasicUpload
    :action="`${uploadUrl}${urlPrefix}/upload/file`"
    :headers="uploadHeaders"
    :data="{ type: 0 }"
    name="file"
    :width="100"
    :height="100"
    :maxNumber="maxNumber"
    :accept="accept"
    :helpText="helpText"
    :maxSize="maxSize"
    :allowedMimeTypes="allowedMimeTypes"
    :imageAspectRatio="imageAspectRatio"
    :imageAspectRatioTolerance="imageAspectRatioTolerance"
    :imageMaxDimensionSum="imageMaxDimensionSum"
    :imageMaxAspectRatio="imageMaxAspectRatio"
    :imageMinShortSide="imageMinShortSide"
    :imageMinLongSide="imageMinLongSide"
    :imageRecommendedAspectRatio="imageRecommendedAspectRatio"
    :imageRecommendedAspectRatioTolerance="imageRecommendedAspectRatioTolerance"
    @upload-change="uploadChange"
    v-model:value="image"
    v-model:values="images"
  />
</template>

<script lang="ts" setup>
  import { onMounted, reactive, ref, unref, watch } from 'vue';
  import { BasicUpload } from '@/components/Upload';
  import { useGlobSetting } from '@/hooks/setting';
  import { useUserStoreWidthOut } from '@/store/modules/user';

  export interface Props {
    value: string | string[] | null;
    maxNumber: number;
    helpText?: string;
    accept?: string;
    maxSize?: number;
    allowedMimeTypes?: string[];
    imageAspectRatio?: number;
    imageAspectRatioTolerance?: number;
    imageMaxDimensionSum?: number;
    imageMaxAspectRatio?: number;
    imageMinShortSide?: number;
    imageMinLongSide?: number;
    imageRecommendedAspectRatio?: number;
    imageRecommendedAspectRatioTolerance?: number;
  }

  const emit = defineEmits(['update:value']);
  const props = withDefaults(defineProps<Props>(), {
    value: '',
    maxNumber: 1,
    helpText: '',
    accept: '.jpg,.png,.jpeg,.svg,.gif,.webp',
    maxSize: 0,
    allowedMimeTypes: () => [],
    imageAspectRatio: 0,
    imageAspectRatioTolerance: 0.02,
    imageMaxDimensionSum: 0,
    imageMaxAspectRatio: 0,
    imageMinShortSide: 0,
    imageMinLongSide: 0,
    imageRecommendedAspectRatio: 0,
    imageRecommendedAspectRatioTolerance: 0.03,
  });
  const image = ref<string>('');
  const images = ref<string[]>([]);
  const globSetting = useGlobSetting();
  const urlPrefix = globSetting.urlPrefix || '';
  const { uploadUrl } = globSetting;
  const useUserStore = useUserStoreWidthOut();
  const uploadHeaders = reactive({
    Authorization: useUserStore.token,
    uploadType: 'image',
  });

  function uploadChange(list: string | string[]) {
    if (props.maxNumber === 1) {
      image.value = unref(list as string);
      emit('update:value', image.value);
    } else {
      images.value = unref(list as string[]);
      emit('update:value', images.value);
    }
  }

  function loadImage() {
    if (props.maxNumber === 1) {
      image.value = props.value as string;
    } else {
      images.value = props.value as string[];
    }
  }

  watch(
    () => props.value,
    () => {
      loadImage();
    },
    {
      immediate: true,
      deep: true,
    }
  );

  onMounted(async () => {
    loadImage();
  });
</script>

<style lang="less"></style>
