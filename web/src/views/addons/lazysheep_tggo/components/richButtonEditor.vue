<template>
  <n-form :label-width="110" :model="formValue">
    <n-form-item label="富文本内容">
      <n-input
        v-model:value="formValue.text"
        type="textarea"
        :autosize="{ minRows: 5, maxRows: 12 }"
        placeholder="支持 Telegram HTML：<b>加粗</b> <i>斜体</i> <a href='https://example.com'>链接</a>"
      />
    </n-form-item>
    <n-form-item label="菜单显示">
      <n-switch v-model:value="formValue.menuVisible" />
    </n-form-item>
    <n-form-item label="插件命令">
      <n-checkbox v-model:checked="formValue.showPluginCommands">自动追加已启用插件命令</n-checkbox>
    </n-form-item>
    <n-form-item label="按钮布局">
      <div class="keyboard-editor">
        <div class="keyboard-panel">
          <div class="panel-header">
            <div>
              <div class="panel-title">按钮排布</div>
              <div class="panel-subtitle">按行组织，按钮会从左到右展示</div>
            </div>
            <n-space>
              <n-button size="small" @click="restoreDefault">恢复默认</n-button>
              <n-button size="small" @click="addRow">新增一行</n-button>
              <n-button size="small" @click="openImport">JSON 导入</n-button>
            </n-space>
          </div>
          <n-empty v-if="!formValue.buttons.length" description="暂无按钮行">
            <template #extra>
              <n-button size="small" type="primary" @click="addRow">新增一行</n-button>
            </template>
          </n-empty>
          <div v-else class="keyboard-rows">
            <div v-for="(row, rowIndex) in formValue.buttons" :key="rowIndex" class="keyboard-row">
              <div class="row-header">
                <div>
                  <span class="row-title">第 {{ rowIndex + 1 }} 行</span>
                  <span class="row-count">共 {{ row.length }} 个按钮</span>
                </div>
                <n-space size="small">
                  <n-button size="tiny" :disabled="rowIndex === 0" @click="moveRow(rowIndex, -1)">上移</n-button>
                  <n-button size="tiny" :disabled="rowIndex === formValue.buttons.length - 1" @click="moveRow(rowIndex, 1)">下移</n-button>
                  <n-button size="tiny" type="error" ghost @click="removeRow(rowIndex)">删除行</n-button>
                </n-space>
              </div>
              <div class="button-strip">
                <button class="add-button" type="button" @click="addButton(rowIndex, 0)">+</button>
                <template v-for="(button, buttonIndex) in row" :key="buttonIndex">
                  <div class="keyboard-button">
                    <span class="button-text">{{ button.text || '未命名' }}</span>
                    <n-tag v-if="button.adminOnly" size="small" type="warning">管理员</n-tag>
                    <n-button text size="tiny" @click="openButton(rowIndex, buttonIndex)">
                      <template #icon>
                        <n-icon><EditOutlined /></n-icon>
                      </template>
                    </n-button>
                  </div>
                  <button class="add-button" type="button" @click="addButton(rowIndex, buttonIndex + 1)">+</button>
                </template>
              </div>
              <button class="add-row-button" type="button" @click="addRow(rowIndex + 1)">+</button>
            </div>
          </div>
        </div>
      </div>
    </n-form-item>
  </n-form>

  <n-modal v-model:show="buttonModalVisible" preset="dialog" title="按钮属性" style="width: 620px">
    <n-form :model="buttonForm" label-placement="top">
      <n-form-item label="按钮文字">
        <n-input v-model:value="buttonForm.text" placeholder="例如：联系客服" />
      </n-form-item>
      <n-form-item label="动作">
        <n-select v-model:value="buttonForm.action" :options="actionOptions" />
      </n-form-item>
      <n-form-item label="权限">
        <n-checkbox v-model:checked="buttonForm.adminOnly">仅该 Bot 管理员可用</n-checkbox>
      </n-form-item>
      <n-form-item label="内容">
        <n-input
          v-model:value="buttonForm.value"
          type="textarea"
          :autosize="{ minRows: 3, maxRows: 6 }"
          placeholder="点击回复填写回复内容；打开链接填写 URL；插件功能填写插件命令或功能 key"
        />
      </n-form-item>
    </n-form>
    <template #action>
      <n-space justify="space-between" class="modal-actions">
        <n-button type="error" ghost @click="removeCurrentButton">删除按钮</n-button>
        <n-space>
          <n-button @click="buttonModalVisible = false">取消</n-button>
          <n-button type="primary" @click="saveButton">保存</n-button>
        </n-space>
      </n-space>
    </template>
  </n-modal>

  <n-modal v-model:show="importModalVisible" preset="dialog" title="JSON 导入按钮布局" style="width: 720px">
    <n-input
      v-model:value="importText"
      type="textarea"
      :autosize="{ minRows: 10, maxRows: 18 }"
      placeholder='支持导入 buttons 数组，或完整配置：{"text":"","menuVisible":true,"buttons":[[{"text":"帮助","action":"reply","value":"请联系管理员"}]]}'
    />
    <template #action>
      <n-space>
        <n-button @click="importModalVisible = false">取消</n-button>
        <n-button type="primary" @click="importJson">导入</n-button>
      </n-space>
    </template>
  </n-modal>
</template>

<script lang="ts" setup>
  import { ref, watch } from 'vue';
  import { useMessage } from 'naive-ui';
  import { EditOutlined } from '@vicons/antd';

  const props = defineProps({
    value: {
      type: Object,
      default: () => ({}),
    },
  });
  const emit = defineEmits(['update:value']);
  const message = useMessage();
  const actionOptions = [
    { label: '点击回复', value: 'reply' },
    { label: '打开链接', value: 'url' },
    { label: '插件功能', value: 'plugin' },
  ];
  const formValue = ref<any>(newValue());
  const selected = ref({ row: -1, button: -1 });
  const buttonModalVisible = ref(false);
  const importModalVisible = ref(false);
  const importText = ref('');
  const buttonForm = ref(newButton());

  watch(
    () => props.value,
    () => {
      formValue.value = newValue();
    },
    { immediate: true }
  );

  watch(
    formValue,
    () => {
      emit('update:value', JSON.parse(JSON.stringify(formValue.value)));
    },
    { deep: true }
  );

  function newValue() {
    const value = props.value || {};
    return {
      text: value.text || '',
      menuVisible: value.menuVisible !== false,
      showPluginCommands: value.showPluginCommands !== false,
      buttons: normalizeButtons(value.buttons || []),
    };
  }

  function newButton(data: any = {}) {
    let value = data.value || '';
    if (data.action === 'plugin' && value === '/pull 配置') {
      value = '管理员配置';
    }
    return {
      text: data.text || '',
      action: data.action || 'reply',
      value,
      adminOnly: data.adminOnly === true,
    };
  }

  function normalizeButtons(raw: any) {
    if (!Array.isArray(raw)) {
      return [];
    }
    return raw
      .filter((row) => Array.isArray(row))
      .map((row) => row.map((button) => newButton(button)));
  }

  function addRow(index?: number) {
    const row = [];
    if (typeof index === 'number') {
      formValue.value.buttons.splice(index, 0, row);
      return;
    }
    formValue.value.buttons.push(row);
  }

  function removeRow(index: number) {
    formValue.value.buttons.splice(index, 1);
  }

  function moveRow(index: number, offset: number) {
    const next = index + offset;
    if (next < 0 || next >= formValue.value.buttons.length) return;
    const row = formValue.value.buttons.splice(index, 1)[0];
    formValue.value.buttons.splice(next, 0, row);
  }

  function addButton(rowIndex: number, buttonIndex: number) {
    formValue.value.buttons[rowIndex].splice(buttonIndex, 0, newButton({ text: '新按钮' }));
    openButton(rowIndex, buttonIndex);
  }

  function openButton(row: number, button: number) {
    selected.value = { row, button };
    buttonForm.value = newButton(formValue.value.buttons?.[row]?.[button]);
    buttonModalVisible.value = true;
  }

  function saveButton() {
    const row = formValue.value.buttons?.[selected.value.row];
    if (!row || selected.value.button < 0) return;
    row[selected.value.button] = newButton(buttonForm.value);
    buttonModalVisible.value = false;
  }

  function removeCurrentButton() {
    const row = formValue.value.buttons?.[selected.value.row];
    if (!row || selected.value.button < 0) return;
    row.splice(selected.value.button, 1);
    buttonModalVisible.value = false;
  }

  function openImport() {
    importText.value = JSON.stringify({ buttons: formValue.value.buttons }, null, 2);
    importModalVisible.value = true;
  }

  function restoreDefault() {
    formValue.value.menuVisible = true;
    formValue.value.showPluginCommands = false;
    formValue.value.buttons = normalizeButtons([
      [
        { text: '创建机器人', action: 'reply', value: '请发送你的 Telegram Bot Token，系统会自动创建并绑定你的专属机器人。' },
        { text: '邀请赚积分', action: 'reply', value: '邀请功能暂未开放。' },
      ],
      [
        { text: '个人中心', action: 'plugin', value: '/profile' },
        { text: '签到', action: 'plugin', value: '/sign' },
      ],
      [
        { text: '会员中心', action: 'plugin', value: '/member' },
        { text: '反馈技术', action: 'reply', value: '请联系管理员反馈使用问题。' },
      ],
      [
        { text: '帮助', action: 'plugin', value: '/help' },
        { text: '管理员配置', action: 'plugin', value: '管理员配置', adminOnly: true },
      ],
    ]);
    message.success('已恢复默认布局');
  }

  function importJson() {
    try {
      const data = JSON.parse(importText.value);
      const buttons = Array.isArray(data) ? data : data.buttons;
      formValue.value.buttons = normalizeButtons(buttons);
      if (!Array.isArray(data) && typeof data.menuVisible === 'boolean') {
        formValue.value.menuVisible = data.menuVisible;
      }
      if (!Array.isArray(data) && typeof data.showPluginCommands === 'boolean') {
        formValue.value.showPluginCommands = data.showPluginCommands;
      }
      if (!Array.isArray(data) && typeof data.text === 'string') {
        formValue.value.text = data.text;
      }
      importModalVisible.value = false;
      message.success('导入成功');
    } catch (e) {
      message.error('JSON 格式不正确');
    }
  }
</script>

<style scoped>
  .keyboard-editor {
    width: 100%;
  }

  .keyboard-panel {
    border: 1px solid var(--n-border-color);
    border-radius: 8px;
    padding: 18px;
  }

  .panel-header {
    align-items: flex-start;
    display: flex;
    justify-content: space-between;
    margin-bottom: 16px;
  }

  .panel-title {
    font-size: 16px;
    font-weight: 600;
  }

  .panel-subtitle,
  .row-count {
    color: var(--n-text-color-3);
    margin-top: 6px;
  }

  .keyboard-rows {
    display: flex;
    flex-direction: column;
    gap: 14px;
  }

  .keyboard-row {
    border: 1px solid var(--n-border-color);
    border-radius: 8px;
    padding: 16px;
  }

  .row-header {
    align-items: center;
    display: flex;
    justify-content: space-between;
  }

  .row-title {
    display: block;
    font-weight: 600;
  }

  .button-strip {
    align-items: center;
    background: var(--n-color-embedded);
    border-radius: 8px;
    display: flex;
    gap: 10px;
    margin-top: 14px;
    padding: 14px;
  }

  .keyboard-button {
    align-items: center;
    background: var(--n-color);
    border: 1px solid var(--n-border-color);
    border-radius: 8px;
    display: flex;
    flex: 1;
    gap: 8px;
    justify-content: center;
    min-height: 44px;
    min-width: 120px;
    padding: 0 12px;
  }

  .button-text {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .add-button,
  .add-row-button {
    background: transparent;
    border: 1px dashed #8bb8ff;
    color: #4b8df7;
    cursor: pointer;
  }

  .add-button {
    border-radius: 16px;
    height: 42px;
    width: 28px;
  }

  .add-row-button {
    border-radius: 8px;
    display: block;
    font-size: 18px;
    height: 34px;
    margin-top: 12px;
    width: 100%;
  }

  .modal-actions {
    width: 100%;
  }
</style>
