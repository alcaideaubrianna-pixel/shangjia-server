<template>
  <n-space vertical class="w-full">
    <n-text depth="3"
      >按上架账号绑定状态切换。实际显示仍受插件开关和用户权限限制；默认布局随系统更新。</n-text
    >
    <n-space>
      <n-button @click="update(null)">恢复默认布局</n-button>
      <n-button @click="update([])">清空菜单</n-button>
      <n-button @click="addRow">添加一行</n-button>
    </n-space>
    <n-text v-if="value === null || value === undefined">使用系统默认布局</n-text>
    <n-card v-for="(row, ri) in rows" :key="ri" size="small" :title="`第 ${ri + 1} 行`">
      <n-space vertical>
        <n-space v-for="(button, bi) in row" :key="bi" align="center">
          <n-select
            :value="button.featureKey"
            :options="options"
            placeholder="选择插件"
            class="feature-select"
            @update:value="patch(ri, bi, { featureKey: $event })"
          />
          <n-select
            :value="button.style || ''"
            :options="colors"
            class="color-select"
            @update:value="patch(ri, bi, { style: $event })"
          />
          <n-button :disabled="bi === 0" @click="moveButton(ri, bi, -1)">←</n-button>
          <n-button :disabled="bi === row.length - 1" @click="moveButton(ri, bi, 1)">→</n-button>
          <n-button type="error" @click="removeButton(ri, bi)">删除</n-button>
        </n-space>
        <n-space>
          <n-button :disabled="row.length >= 4" @click="addButton(ri)">添加按钮</n-button>
          <n-button :disabled="ri === 0" @click="moveRow(ri, -1)">上移行</n-button>
          <n-button :disabled="ri === rows.length - 1" @click="moveRow(ri, 1)">下移行</n-button>
          <n-button type="error" @click="removeRow(ri)">删除行</n-button>
        </n-space>
      </n-space>
    </n-card>
    <n-text>布局预览（不同用户可能隐藏无权限按钮）</n-text>
    <div v-for="(row, ri) in rows" :key="`preview-${ri}`" class="keyboard-row">
      <n-button
        v-for="(button, bi) in row"
        :key="bi"
        :type="buttonType(button.style)"
        class="keyboard-button"
        >{{ options.find((o) => o.value === button.featureKey)?.label || '请选择插件' }}</n-button
      >
    </div>
    <n-input
      v-model:value="jsonDraft"
      type="textarea"
      :autosize="{ minRows: 3, maxRows: 12 }"
      placeholder="null 使用默认；[] 隐藏全部；二维数组定义每行按钮"
    />
    <n-button @click="applyJson">应用 JSON</n-button>
    <n-alert v-if="error" type="error">{{ error }}</n-alert>
  </n-space>
</template>

<script setup lang="ts">
  import { computed, ref, watch } from 'vue';
  type Button = { featureKey: string; style?: string };
  const props = defineProps<{
    value?: Button[][] | null;
    options: Array<{ label: string; value: string }>;
    styleOptions: Array<{ label: string; value: string }>;
  }>();
  const emit = defineEmits<{ (e: 'update:value', value: Button[][] | null): void }>();
  const rows = computed(() => props.value || []);
  const error = ref('');
  const jsonDraft = ref('');
  const colors = computed(() => props.styleOptions);
  watch(
    () => props.value,
    (value) => {
      jsonDraft.value = JSON.stringify(value ?? null, null, 2);
    },
    { immediate: true, deep: true }
  );
  function update(value: Button[][] | null) {
    error.value = '';
    emit('update:value', value);
  }
  function edit(fn: (value: Button[][]) => void) {
    const value: Button[][] = JSON.parse(JSON.stringify(rows.value));
    fn(value);
    update(value);
  }
  function nextButton() {
    const used = new Set(rows.value.flat().map((button) => button.featureKey));
    const option = props.options.find((item) => !used.has(item.value));
    return option ? { featureKey: option.value, style: '' } : null;
  }
  function addRow() {
    const button = nextButton();
    if (button && rows.value.length < 12) edit((v) => v.push([button]));
  }
  function addButton(ri: number) {
    const button = nextButton();
    if (button) edit((v) => v[ri].push(button));
  }
  function patch(ri: number, bi: number, value: Partial<Button>) {
    edit((v) => {
      v[ri][bi] = { ...v[ri][bi], ...value };
    });
  }
  function removeRow(ri: number) {
    edit((v) => v.splice(ri, 1));
  }
  function removeButton(ri: number, bi: number) {
    edit((v) => {
      v[ri].splice(bi, 1);
      if (!v[ri].length) v.splice(ri, 1);
    });
  }
  function moveRow(ri: number, delta: number) {
    edit((v) => {
      [v[ri], v[ri + delta]] = [v[ri + delta], v[ri]];
    });
  }
  function moveButton(ri: number, bi: number, delta: number) {
    edit((v) => {
      [v[ri][bi], v[ri][bi + delta]] = [v[ri][bi + delta], v[ri][bi]];
    });
  }
  function buttonType(style?: string): 'default' | 'primary' | 'success' | 'error' {
    return style === 'danger'
      ? 'error'
      : style === 'primary' || style === 'success'
        ? style
        : 'default';
  }
  function applyJson() {
    try {
      const value = JSON.parse(jsonDraft.value);
      if (value !== null) {
        if (!Array.isArray(value) || value.length > 12) throw new Error('需要最多12行的二维数组');
        const seen = new Set();
        for (const row of value) {
          if (!Array.isArray(row) || !row.length || row.length > 4)
            throw new Error('每行需要1至4个按钮');
          for (const button of row) {
            if (
              !button ||
              !props.options.some((o) => o.value === button.featureKey) ||
              seen.has(button.featureKey)
            )
              throw new Error('插件无效或重复');
            if (!colors.value.some((o) => o.value === (button.style || '')))
              throw new Error('颜色无效');
            seen.add(button.featureKey);
          }
        }
      }
      update(value);
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'JSON格式错误';
    }
  }
</script>

<style scoped>
  .feature-select {
    width: 160px;
  }
  .color-select {
    width: 95px;
  }
  .keyboard-row {
    display: flex;
    gap: 8px;
  }
  .keyboard-button {
    flex: 1;
  }
</style>
