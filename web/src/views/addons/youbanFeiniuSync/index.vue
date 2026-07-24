<template>
  <div>
    <div class="n-layout-page-header">
      <n-card :bordered="false" title="FeiNiu同步">
        <span>定时增量同步 FeiNiu 频道、资料和媒体到悦伴上架系统</span>
      </n-card>
    </div>
    <n-card :bordered="false" class="proCard">
      <n-tabs v-model:value="activeTab" type="line" animated>
        <n-tab-pane name="dashboard" tab="监控大盘">
          <DashboardPanel v-if="activeTab === 'dashboard'" />
        </n-tab-pane>
        <n-tab-pane name="configs" tab="同步配置">
          <ConfigPanel v-if="activeTab === 'configs'" />
        </n-tab-pane>
        <n-tab-pane name="channels" tab="频道映射">
          <ChannelPanel v-if="activeTab === 'channels'" />
        </n-tab-pane>
        <n-tab-pane name="runs" tab="运行记录">
          <RunPanel v-if="activeTab === 'runs'" />
        </n-tab-pane>
      </n-tabs>
    </n-card>
  </div>
</template>

<script setup lang="ts">
  import { defineAsyncComponent, ref } from 'vue';

  const activeTab = ref('dashboard');
  const DashboardPanel = defineAsyncComponent(() => import('./components/dashboard-panel.vue'));
  const ConfigPanel = defineAsyncComponent(() => import('./components/config-panel.vue'));
  const ChannelPanel = defineAsyncComponent(() => import('./components/channel-panel.vue'));
  const RunPanel = defineAsyncComponent(() => import('./components/run-panel.vue'));
</script>
