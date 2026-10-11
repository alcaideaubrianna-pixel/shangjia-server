<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { NButton, NForm, NFormItem, NInput, NInputNumber, NSpace, NSwitch, NTag } from 'naive-ui';
import { RelayChannelList, RelayChannelSave, RelayChannelStatus } from '@/api/addons/youbanPublish';

const props = defineProps<{ tenantId: number }>();
const rows = ref<any[]>([]);
const form = ref({ tgAccountId: 0, targetChatId: '', title: '' });
const loading = ref(false);
async function load() { const res: any = await RelayChannelList({ tenantId: props.tenantId }); rows.value = res?.list || res?.data?.list || []; }
async function save() { loading.value = true; try { await RelayChannelSave({ ...form.value, tenantId: props.tenantId, status: 'disabled' }); form.value = { tgAccountId: 0, targetChatId: '', title: '' }; await load(); } finally { loading.value = false; } }
async function toggle(row: any) { await RelayChannelStatus({ id: row.id, status: row.status === 'enabled' ? 'disabled' : 'enabled' }); await load(); }
onMounted(load);
</script>

<template>
  <n-form inline :model="form">
    <n-form-item label="协议账号"><n-input-number v-model:value="form.tgAccountId" :min="1" /></n-form-item>
    <n-form-item label="频道 Chat ID"><n-input v-model:value="form.targetChatId" placeholder="-100..." /></n-form-item>
    <n-form-item label="名称"><n-input v-model:value="form.title" /></n-form-item>
    <n-button type="primary" :loading="loading" @click="save">添加中转频道</n-button>
  </n-form>
  <n-space vertical>
    <n-space v-for="row in rows" :key="row.id" align="center">
      <span>{{ row.title || row.targetChatId }}</span>
      <n-tag :type="row.healthStatus === 'ready' ? 'success' : 'warning'">{{ row.healthStatus }}</n+      </n-tag>
      <n-switch :value="row.status === 'enabled'" @update:value="toggle(row)" />
    </n-space>
  </n-space>
</template>
