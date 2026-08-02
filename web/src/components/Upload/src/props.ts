import type { PropType } from 'vue';
import { NUpload } from 'naive-ui';

export const basicProps = {
  ...NUpload.props,
  fileType: {
    type: String,
    default: 'image',
  },
  accept: {
    type: String,
    default: '.jpg,.png,.jpeg,.svg,.gif,.webp',
  },
  helpText: {
    type: String as PropType<string>,
    default: '',
  },
  maxSize: {
    type: Number as PropType<number>,
    default: 0,
  },
  allowedMimeTypes: {
    type: Array as PropType<string[]>,
    default: () => [],
  },
  maxNumber: {
    type: Number as PropType<number>,
    default: Infinity,
  },
  imageAspectRatio: {
    type: Number as PropType<number>,
    default: 0,
  },
  imageAspectRatioTolerance: {
    type: Number as PropType<number>,
    default: 0.02,
  },
  imageMaxDimensionSum: {
    type: Number as PropType<number>,
    default: 0,
  },
  imageMaxAspectRatio: {
    type: Number as PropType<number>,
    default: 0,
  },
  imageMinShortSide: {
    type: Number as PropType<number>,
    default: 0,
  },
  imageMinLongSide: {
    type: Number as PropType<number>,
    default: 0,
  },
  imageRecommendedAspectRatio: {
    type: Number as PropType<number>,
    default: 0,
  },
  imageRecommendedAspectRatioTolerance: {
    type: Number as PropType<number>,
    default: 0.03,
  },
  value: {
    type: String as PropType<string>,
    default: () => '',
  },
  values: {
    type: (Array as PropType<string[]>) || (Object as PropType<object>),
    default: () => [],
  },
  width: {
    type: Number as PropType<number>,
    default: 104,
  },
  height: {
    type: Number as PropType<number>,
    default: 104, //建议不小于这个尺寸 太小页面可能显示有异常
  },
};
