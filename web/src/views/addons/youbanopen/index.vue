<template>
  <div>
    <div class="n-layout-page-header">
      <n-card :bordered="false" title="开放中心">
        管理 XC-CMS 实例授权和租户平台合作申请。
      </n-card>
    </div>
    <n-tabs v-model:value="activeTab" type="line" animated>
      <n-tab-pane name="apps" tab="CMS 应用"><CmsAppPage /></n-tab-pane>
      <n-tab-pane name="bindings" tab="合作平台审核"><BindingReview /></n-tab-pane>
    </n-tabs>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import CmsAppPage from '../youbanPublishCmsApp/index.vue';
import BindingReview from './BindingReview.vue';

defineOptions({ name: 'YoubanOpen' });
const activeTab = ref('apps');
</script>
