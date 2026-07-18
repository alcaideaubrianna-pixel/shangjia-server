<template>
  <n-form :label-width="110" :model="formValue">
    <n-form-item label="插件名称">
      <n-input v-model:value="formValue.name" />
    </n-form-item>
    <n-form-item label="插件副标题">
      <n-input v-model:value="formValue.subtitle" />
    </n-form-item>
    <n-form-item label="插件分类">
      <n-input v-model:value="formValue.category" />
    </n-form-item>
    <n-form-item label="插件简介">
      <n-input v-model:value="formValue.description" type="textarea" />
    </n-form-item>
    <n-form-item v-if="hasCommandConfig" label="命令配置">
      <n-space vertical class="full-width">
        <n-input v-model:value="formValue.command" placeholder="主命令，例如 /拉取" />
        <n-dynamic-input
          v-model:value="formValue.commands"
          :min="1"
          placeholder="输入命令，例如 /绑定"
        />
      </n-space>
    </n-form-item>
    <n-form-item label="开关">
      <n-space>
        <n-checkbox v-model:checked="formValue.enabled">全局启用</n-checkbox>
        <n-checkbox v-model:checked="formValue.userEnabled">允许用户启用</n-checkbox>
        <n-checkbox v-model:checked="formValue.paid">增值插件</n-checkbox>
      </n-space>
    </n-form-item>
    <n-form-item label="价格">
      <n-input v-model:value="formValue.price" placeholder="免费插件填 0" />
    </n-form-item>
    <n-form-item label="有效期天数">
      <n-input-number v-model:value="formValue.expireDays" :show-button="false" :min="0" />
    </n-form-item>
    <n-form-item label="排序">
      <n-input-number v-model:value="formValue.sort" :show-button="false" />
    </n-form-item>
    <template v-if="formValue.key === 'welcome'">
      <n-form-item label="欢迎语内容">
        <TelegramRichEditor v-model:value="welcomeValue.text" />
      </n-form-item>
      <n-form-item label="挂载能力">
        <n-select
          v-model:value="welcomeValue.mountedPlugins"
          multiple
          clearable
          :options="mountPluginOptions"
          placeholder="选择 /start 后需要挂载的插件能力"
        />
      </n-form-item>
    </template>
    <RichButtonEditor
      v-else-if="formValue.key === 'menu'"
      v-model:value="menuValue"
    />
    <template v-else-if="formValue.key === 'collector'">
      <n-form-item label="菜单显示">
        <n-switch v-model:value="collectorValue.menuVisible" />
      </n-form-item>
      <n-form-item label="公开入口">
        <n-space>
          <n-checkbox v-model:checked="collectorValue.showVerifyLink">显示验证视频入口</n-checkbox>
          <n-checkbox v-model:checked="collectorValue.showLocationLink">显示位置入口</n-checkbox>
        </n-space>
      </n-form-item>
      <n-form-item label="机器人查看">
        <n-switch v-model:value="collectorValue.revealInBot" />
      </n-form-item>
      <n-form-item label="验证视频合并">
        <n-switch v-model:value="collectorValue.mergeVerifyInGroup" />
      </n-form-item>
      <n-form-item label="绑定通知">
        <n-switch v-model:value="collectorValue.bindNotify" />
      </n-form-item>
      <n-form-item label="页脚">
        <n-input
          v-model:value="collectorValue.footer"
          type="textarea"
          :autosize="{ minRows: 3, maxRows: 8 }"
          placeholder="每条采集笔记底部展示的文案，支持 Telegram HTML"
        />
      </n-form-item>
      <n-form-item label="推送模板">
        <n-input
          v-model:value="collectorValue.captionTemplate"
          type="textarea"
          :autosize="{ minRows: 6, maxRows: 12 }"
          placeholder="{title} {text} {code} {verify_link} {location_link} {footer}"
        />
      </n-form-item>
      <n-form-item label="绑定提示">
        <n-input
          v-model:value="collectorValue.bindHelpText"
          type="textarea"
          :autosize="{ minRows: 3, maxRows: 6 }"
        />
      </n-form-item>
      <n-form-item label="入口文案">
        <n-space vertical class="full-width">
          <n-input v-model:value="collectorValue.verifyLinkText" placeholder="验证视频入口文案" />
          <n-input v-model:value="collectorValue.locationLinkText" placeholder="📌 点击查看位置" />
        </n-space>
      </n-form-item>
    </template>
    <template v-else-if="formValue.key === 'footer'">
      <n-form-item label="底栏文案">
        <n-input
          v-model:value="footerValue.footerText"
          type="textarea"
          :autosize="{ minRows: 4, maxRows: 10 }"
          placeholder="公开内容底部追加的文案"
        />
      </n-form-item>
      <n-form-item label="渲染模式">
        <n-select
          v-model:value="footerValue.footerMode"
          :options="[
            { label: '追加', value: 'append' },
            { label: '替换', value: 'replace' },
          ]"
        />
      </n-form-item>
      <n-form-item label="作用范围">
        <n-select
          v-model:value="footerValue.scope"
          :options="[
            { label: '公开频道', value: 'public' },
            { label: '审核群', value: 'review' },
            { label: '全部会话', value: 'all' },
          ]"
        />
      </n-form-item>
      <n-form-item label="模板">
        <n-input
          v-model:value="footerValue.template"
          type="textarea"
          :autosize="{ minRows: 3, maxRows: 8 }"
          placeholder="{footer}"
        />
      </n-form-item>
      <n-form-item label="全局替换">
        <n-checkbox v-model:checked="footerValue.replaceAll">对全部内容全局替换</n-checkbox>
      </n-form-item>
    </template>
    <template v-else-if="formValue.key === 'points'">
      <n-form-item label="积分名称">
        <n-input v-model:value="pointsValue.pointName" placeholder="积分" />
      </n-form-item>
      <n-form-item label="余额文案">
        <n-input v-model:value="pointsValue.balanceText" placeholder="当前积分：{points}" />
      </n-form-item>
      <n-form-item label="规则说明">
        <n-input
          v-model:value="pointsValue.ruleText"
          type="textarea"
          :autosize="{ minRows: 4, maxRows: 8 }"
          placeholder="积分获取和消耗规则"
        />
      </n-form-item>
    </template>
    <template v-else-if="formValue.key === 'profile'">
      <n-form-item label="积分名称">
        <n-input v-model:value="profileValue.pointName" placeholder="积分" />
      </n-form-item>
      <n-form-item label="邀请奖励">
        <n-input-number v-model:value="profileValue.inviteRewardPoints" :show-button="false" :min="0" />
      </n-form-item>
      <n-form-item label="按钮文案">
        <n-space vertical class="full-width">
          <n-input v-model:value="profileValue.refreshText" placeholder="刷新" />
          <n-input v-model:value="profileValue.signText" placeholder="签到" />
        </n-space>
      </n-form-item>
      <n-form-item label="邀请提示">
        <n-space vertical class="full-width">
          <n-input
            v-model:value="profileValue.inviteStartText"
            type="textarea"
            :autosize="{ minRows: 2, maxRows: 6 }"
            placeholder="欢迎使用机器人。"
          />
          <n-input
            v-model:value="profileValue.inviteRecordedText"
            type="textarea"
            :autosize="{ minRows: 2, maxRows: 6 }"
            placeholder="邀请关系已记录，欢迎使用机器人。"
          />
        </n-space>
      </n-form-item>
    </template>
    <template v-else-if="formValue.key === 'signin'">
      <n-form-item label="签到入口">
        <n-space vertical class="full-width">
          <n-input v-model:value="signinValue.command" placeholder="/sign" />
          <n-dynamic-input v-model:value="signinValue.commands" :min="1" placeholder="输入命令" />
        </n-space>
      </n-form-item>
      <n-form-item label="奖励积分">
        <n-input-number v-model:value="signinValue.rewardPoints" :show-button="false" :min="0" />
      </n-form-item>
      <n-form-item label="关注校验">
        <n-checkbox v-model:checked="signinValue.followRequired">签到前先关注频道</n-checkbox>
      </n-form-item>
      <n-form-item label="必关频道">
        <n-dynamic-input v-model:value="signinValue.channels" :on-create="createSigninChannel">
          <template #default="{ value }">
            <n-space vertical class="full-width">
              <n-input v-model:value="value.title" placeholder="频道名称，例如：官方频道" />
              <n-input v-model:value="value.chatId" placeholder="频道 ID 或 username，例如：-1001234567890 / @channel" />
              <n-input v-model:value="value.url" placeholder="频道链接，例如：https://t.me/channel" />
            </n-space>
          </template>
        </n-dynamic-input>
      </n-form-item>
      <n-form-item label="签到成功文案">
        <n-input
          v-model:value="signinValue.successText"
          type="textarea"
          :autosize="{ minRows: 3, maxRows: 8 }"
        />
      </n-form-item>
    </template>
    <template v-else-if="formValue.key === 'help'">
      <n-form-item label="帮助内容">
        <TelegramRichEditor v-model:value="helpValue.helpText" />
      </n-form-item>
    </template>
    <template v-else-if="formValue.key === 'rights'">
      <n-form-item label="验证视频策略">
        <n-select
          v-model:value="rightsValue.verifyMode"
          :options="[
            { label: '不限制', value: 'none' },
            { label: '仅会员', value: 'member' },
            { label: '积分查看', value: 'points' },
            { label: '会员或积分', value: 'member_or_points' },
          ]"
        />
      </n-form-item>
      <n-form-item label="位置策略">
        <n-select
          v-model:value="rightsValue.locationMode"
          :options="[
            { label: '不限制', value: 'none' },
            { label: '仅会员', value: 'member' },
            { label: '积分查看', value: 'points' },
            { label: '会员或积分', value: 'member_or_points' },
          ]"
        />
      </n-form-item>
      <n-form-item label="按钮文案">
        <n-space vertical class="full-width">
          <n-input v-model:value="rightsValue.verifyButtonText" placeholder="📒 点击查看验证视频" />
          <n-input v-model:value="rightsValue.locationButtonText" placeholder="📌 点击查看位置" />
        </n-space>
      </n-form-item>
      <n-form-item label="查看门槛">
        <n-space>
          <n-checkbox v-model:checked="rightsValue.memberOnly">仅会员可见</n-checkbox>
          <n-checkbox v-model:checked="rightsValue.showVerifyButton">显示验证按钮</n-checkbox>
          <n-checkbox v-model:checked="rightsValue.showLocationButton">显示位置按钮</n-checkbox>
        </n-space>
      </n-form-item>
      <n-form-item label="积分消耗">
        <n-input-number v-model:value="rightsValue.pointsCost" :show-button="false" :min="0" />
      </n-form-item>
      <n-form-item label="单独扣分">
        <n-space vertical class="full-width">
          <n-input-number
            v-model:value="rightsValue.verifyPointsCost"
            :show-button="false"
            :min="0"
            placeholder="验证视频消耗，留空跟随积分消耗"
          />
          <n-input-number
            v-model:value="rightsValue.locationPointsCost"
            :show-button="false"
            :min="0"
            placeholder="位置消耗，留空跟随积分消耗"
          />
        </n-space>
      </n-form-item>
      <n-form-item label="私聊提示">
        <n-input
          v-model:value="rightsValue.privateText"
          type="textarea"
          :autosize="{ minRows: 3, maxRows: 8 }"
        />
      </n-form-item>
    </template>
    <template v-else-if="formValue.key === 'map'">
      <n-form-item label="地图按钮">
        <n-select
          v-model:value="mapValue.providerButtons"
          multiple
          :options="[
            { label: '高德地图', value: 'amap' },
            { label: '百度地图', value: 'baidu' },
            { label: '腾讯地图', value: 'tencent' },
          ]"
        />
      </n-form-item>
      <n-form-item label="坐标策略">
        <n-select
          v-model:value="mapValue.coordType"
          :options="[
            { label: '自动识别', value: 'auto' },
            { label: 'WGS84', value: 'wgs84' },
            { label: 'GCJ02', value: 'gcj02' },
            { label: 'BD09', value: 'bd09' },
          ]"
        />
      </n-form-item>
      <n-form-item label="展示方式">
        <n-space>
          <n-checkbox v-model:checked="mapValue.showVenueMessage">显示 venue 消息</n-checkbox>
          <n-checkbox v-model:checked="mapValue.showLocationCard">显示位置卡片</n-checkbox>
        </n-space>
      </n-form-item>
      <n-form-item label="地址模板">
        <n-input
          v-model:value="mapValue.addressTemplate"
          type="textarea"
          :autosize="{ minRows: 3, maxRows: 8 }"
          placeholder="{title}\n{address}"
        />
      </n-form-item>
    </template>
    <template v-else-if="formValue.key === 'member'">
      <n-form-item label="会员周期">
        <n-input-number v-model:value="memberValue.durationDays" :show-button="false" :min="1" />
      </n-form-item>
      <n-form-item label="会员价格">
        <n-input v-model:value="formValue.price" placeholder="99" />
      </n-form-item>
      <n-form-item label="续费文案">
        <n-input
          v-model:value="memberValue.renewText"
          type="textarea"
          :autosize="{ minRows: 3, maxRows: 8 }"
        />
      </n-form-item>
      <n-form-item label="权益说明">
        <n-input
          v-model:value="memberValue.benefitText"
          type="textarea"
          :autosize="{ minRows: 3, maxRows: 8 }"
        />
      </n-form-item>
    </template>
    <n-form-item v-else label="配置 JSON">
      <n-input
        v-model:value="settingsText"
        type="textarea"
        :autosize="{ minRows: 5, maxRows: 10 }"
        placeholder='{"key":"value"}'
      />
    </n-form-item>
    <n-space justify="end">
      <n-button type="primary" @click="submit">保存</n-button>
    </n-space>
  </n-form>
</template>

<script lang="ts" setup>
  import { computed, ref, watch } from 'vue';
  import { useMessage } from 'naive-ui';
  import RichButtonEditor from '../components/richButtonEditor.vue';
  import TelegramRichEditor from '../components/telegramRichEditor.vue';

  const props = defineProps({
    plugin: {
      type: Object,
      required: true,
    },
  });
  const emit = defineEmits(['submit']);
  const message = useMessage();
  const formValue = ref<any>({});
  const settingsText = ref('{}');
  const welcomeValue = ref<any>({});
  const menuValue = ref<any>({});
  const collectorValue = ref<any>({});
  const footerValue = ref<any>({});
  const pointsValue = ref<any>({});
  const profileValue = ref<any>({});
  const signinValue = ref<any>({});
  const helpValue = ref<any>({});
  const rightsValue = ref<any>({});
  const mapValue = ref<any>({});
  const memberValue = ref<any>({});
  const hasCommandConfig = computed(() => {
    return ['collector', 'signin', 'member', 'review', 'welcome', 'menu', 'profile', 'help'].includes(formValue.value.key);
  });
  const mountPluginOptions = computed(() => {
    const key = formValue.value.key;
    return Object.values((formValue.value as any).allPlugins || {})
      .filter((item: any) => item && item.key !== key)
      .map((item: any) => ({
        label: `${item.name || item.key}${item.subtitle ? ` - ${item.subtitle}` : ''}`,
        value: item.key,
      }));
  });

  watch(
    () => props.plugin,
    (value) => {
      formValue.value = { ...(value || {}) };
      settingsText.value = JSON.stringify(formValue.value.settings || {}, null, 2);
      formValue.value.allPlugins = value?.allPlugins || {};
      formValue.value.expireDays = Number(formValue.value.expireDays || 0);
      welcomeValue.value = {
        text: formValue.value.settings?.welcomeText || '',
        mountedPlugins: normalizeMountedPlugins(formValue.value.settings),
      };
      formValue.value.command = formValue.value.settings?.command || '';
      formValue.value.commands = normalizeCommands(formValue.value.settings?.commands, formValue.value.settings?.command);
      menuValue.value = {
        menuVisible: formValue.value.settings?.menuVisible !== false,
        buttons: formValue.value.settings?.buttons || [],
        showPluginCommands: formValue.value.settings?.showPluginCommands !== false,
        command: formValue.value.settings?.command || '',
        commands: normalizeCommands(formValue.value.settings?.commands, formValue.value.settings?.command),
      };
      collectorValue.value = {
        menuVisible: formValue.value.settings?.menuVisible !== false,
        command: formValue.value.settings?.command || '/拉取',
        commands: normalizeCommands(formValue.value.settings?.commands, formValue.value.settings?.command),
        showVerifyLink: formValue.value.settings?.showVerifyLink !== false,
        showLocationLink: formValue.value.settings?.showLocationLink !== false,
        mergeVerifyInGroup: formValue.value.settings?.mergeVerifyInGroup !== false,
        bindNotify: formValue.value.settings?.bindNotify === true,
        revealInBot: formValue.value.settings?.revealInBot === true,
        footer: formValue.value.settings?.footer || '',
        captionTemplate: formValue.value.settings?.captionTemplate || '',
        bindHelpText: formValue.value.settings?.bindHelpText || '',
        verifyLinkText: formValue.value.settings?.verifyLinkText || '📒 点击查看验证视频',
        locationLinkText: formValue.value.settings?.locationLinkText || '📌 点击查看位置',
      };
      footerValue.value = {
        footerText: formValue.value.settings?.footerText || '',
        footerMode: formValue.value.settings?.footerMode || 'append',
        replaceAll: formValue.value.settings?.replaceAll === true,
        scope: formValue.value.settings?.scope || 'public',
        template: formValue.value.settings?.template || '{footer}',
      };
      pointsValue.value = {
        pointName: formValue.value.settings?.pointName || '积分',
        balanceText: formValue.value.settings?.balanceText || '当前积分：{points}',
        ruleText: formValue.value.settings?.ruleText || '',
      };
      profileValue.value = {
        pointName: formValue.value.settings?.pointName || '积分',
        refreshText: formValue.value.settings?.refreshText || '刷新',
        signText: formValue.value.settings?.signText || '签到',
        inviteRewardPoints: Number(formValue.value.settings?.inviteRewardPoints || 0),
        inviteStartText: formValue.value.settings?.inviteStartText || '欢迎使用机器人。',
        inviteRecordedText: formValue.value.settings?.inviteRecordedText || '邀请关系已记录，欢迎使用机器人。',
      };
      signinValue.value = {
        command: formValue.value.settings?.command || '/sign',
        commands: normalizeCommands(formValue.value.settings?.commands, formValue.value.settings?.command),
        rewardPoints: Number(formValue.value.settings?.rewardPoints || 0),
        followRequired: formValue.value.settings?.followRequired === true,
        channels: normalizeSigninChannels(formValue.value.settings?.channels),
        successText: formValue.value.settings?.successText || '签到成功，已完成今日任务。',
      };
      helpValue.value = {
        helpText: formValue.value.settings?.helpText || '请联系管理员获取帮助。',
      };
      rightsValue.value = {
        verifyMode: formValue.value.settings?.verifyMode || 'none',
        locationMode: formValue.value.settings?.locationMode || 'none',
        showVerifyButton: formValue.value.settings?.showVerifyButton !== false,
        showLocationButton: formValue.value.settings?.showLocationButton !== false,
        verifyButtonText: formValue.value.settings?.verifyButtonText || '📒 点击查看验证视频',
        locationButtonText: formValue.value.settings?.locationButtonText || '📌 点击查看位置',
        memberOnly: formValue.value.settings?.memberOnly === true,
        pointsCost: Number(formValue.value.settings?.pointsCost || 0),
        verifyPointsCost: formValue.value.settings?.verifyPointsCost ?? null,
        locationPointsCost: formValue.value.settings?.locationPointsCost ?? null,
        privateText: formValue.value.settings?.privateText || '请先完成会员或积分校验后查看隐藏内容。',
      };
      mapValue.value = {
        providerButtons: formValue.value.settings?.providerButtons || ['amap', 'baidu', 'tencent'],
        coordType: formValue.value.settings?.coordType || 'auto',
        showVenueMessage: formValue.value.settings?.showVenueMessage !== false,
        showLocationCard: formValue.value.settings?.showLocationCard !== false,
        addressTemplate: formValue.value.settings?.addressTemplate || '{title}\n{address}',
      };
      memberValue.value = {
        durationDays: Number(formValue.value.settings?.durationDays || 30),
        renewText: formValue.value.settings?.renewText || '会员已开通，可在到期前续费。',
        benefitText: formValue.value.settings?.benefitText || '会员可查看隐藏内容、验证视频和位置。',
      };
    },
    { immediate: true }
  );

  function submit() {
    if (formValue.value.key === 'welcome') {
      formValue.value.settings = {
        ...(formValue.value.settings || {}),
        welcomeText: welcomeValue.value.text || '',
        mountedPlugins: welcomeValue.value.mountedPlugins || [],
        mountMenu: (welcomeValue.value.mountedPlugins || []).includes('menu'),
      };
    } else if (formValue.value.key === 'menu') {
      formValue.value.settings = {
        ...(formValue.value.settings || {}),
        menuVisible: menuValue.value.menuVisible !== false,
        buttons: menuValue.value.buttons || [],
        showPluginCommands: menuValue.value.showPluginCommands !== false,
        command: menuValue.value.command || '',
        commands: normalizeCommands(menuValue.value.commands, menuValue.value.command),
      };
    } else if (formValue.value.key === 'collector') {
      formValue.value.settings = {
        ...(formValue.value.settings || {}),
        menuVisible: collectorValue.value.menuVisible !== false,
        command: collectorValue.value.command || '/拉取',
        commands: normalizeCommands(collectorValue.value.commands, collectorValue.value.command),
        showVerifyLink: collectorValue.value.showVerifyLink !== false,
        showLocationLink: collectorValue.value.showLocationLink !== false,
        mergeVerifyInGroup: collectorValue.value.mergeVerifyInGroup === true,
        bindNotify: collectorValue.value.bindNotify === true,
        revealInBot: collectorValue.value.revealInBot === true,
        footer: collectorValue.value.footer || '',
        captionTemplate: collectorValue.value.captionTemplate || '',
        bindHelpText: collectorValue.value.bindHelpText || '',
        verifyLinkText: collectorValue.value.verifyLinkText || '',
        locationLinkText: collectorValue.value.locationLinkText || '',
      };
    } else if (formValue.value.key === 'footer') {
      formValue.value.settings = {
        ...(formValue.value.settings || {}),
        footerText: footerValue.value.footerText || '',
        footerMode: footerValue.value.footerMode || 'append',
        replaceAll: footerValue.value.replaceAll === true,
        scope: footerValue.value.scope || 'public',
        template: footerValue.value.template || '{footer}',
      };
    } else if (formValue.value.key === 'points') {
      formValue.value.settings = {
        ...(formValue.value.settings || {}),
        pointName: pointsValue.value.pointName || '积分',
        balanceText: pointsValue.value.balanceText || '',
        ruleText: pointsValue.value.ruleText || '',
      };
    } else if (formValue.value.key === 'profile') {
      formValue.value.settings = {
        ...(formValue.value.settings || {}),
        pointName: profileValue.value.pointName || '积分',
        refreshText: profileValue.value.refreshText || '刷新',
        signText: profileValue.value.signText || '签到',
        inviteRewardPoints: Number(profileValue.value.inviteRewardPoints || 0),
        inviteStartText: profileValue.value.inviteStartText || '',
        inviteRecordedText: profileValue.value.inviteRecordedText || '',
        command: formValue.value.command || formValue.value.settings?.command || '/profile',
        commands: normalizeCommands(formValue.value.commands, formValue.value.command || formValue.value.settings?.command),
      };
    } else if (formValue.value.key === 'signin') {
      formValue.value.settings = {
        ...(formValue.value.settings || {}),
        command: signinValue.value.command || '/sign',
        commands: normalizeCommands(signinValue.value.commands, signinValue.value.command),
        rewardPoints: Number(signinValue.value.rewardPoints || 0),
        followRequired: signinValue.value.followRequired === true,
        channels: normalizeSigninChannels(signinValue.value.channels),
        successText: signinValue.value.successText || '',
      };
    } else if (formValue.value.key === 'help') {
      formValue.value.settings = {
        ...(formValue.value.settings || {}),
        helpText: helpValue.value.helpText || '',
        command: formValue.value.command || formValue.value.settings?.command || '/help',
        commands: normalizeCommands(formValue.value.commands, formValue.value.command || formValue.value.settings?.command),
      };
    } else if (formValue.value.key === 'rights') {
      formValue.value.settings = {
        ...(formValue.value.settings || {}),
        verifyMode: rightsValue.value.verifyMode || 'none',
        locationMode: rightsValue.value.locationMode || 'none',
        showVerifyButton: rightsValue.value.showVerifyButton !== false,
        showLocationButton: rightsValue.value.showLocationButton !== false,
        verifyButtonText: rightsValue.value.verifyButtonText || '',
        locationButtonText: rightsValue.value.locationButtonText || '',
        memberOnly: rightsValue.value.memberOnly === true,
        pointsCost: Number(rightsValue.value.pointsCost || 0),
        verifyPointsCost: normalizeOptionalNumber(rightsValue.value.verifyPointsCost),
        locationPointsCost: normalizeOptionalNumber(rightsValue.value.locationPointsCost),
        privateText: rightsValue.value.privateText || '',
      };
    } else if (formValue.value.key === 'map') {
      formValue.value.settings = {
        ...(formValue.value.settings || {}),
        providerButtons: Array.isArray(mapValue.value.providerButtons) ? mapValue.value.providerButtons : [],
        coordType: mapValue.value.coordType || 'auto',
        showVenueMessage: mapValue.value.showVenueMessage !== false,
        showLocationCard: mapValue.value.showLocationCard !== false,
        addressTemplate: mapValue.value.addressTemplate || '{title}\n{address}',
      };
    } else if (formValue.value.key === 'member') {
      formValue.value.settings = {
        ...(formValue.value.settings || {}),
        durationDays: Number(memberValue.value.durationDays || 30),
        renewText: memberValue.value.renewText || '',
        benefitText: memberValue.value.benefitText || '',
        command: formValue.value.command || formValue.value.settings?.command || '/member',
        commands: normalizeCommands(formValue.value.commands, formValue.value.command || formValue.value.settings?.command),
      };
    } else {
      try {
        const parsed = JSON.parse(settingsText.value || '{}');
        if (hasCommandConfig.value) {
          parsed.command = formValue.value.command || parsed.command || '';
          parsed.commands = normalizeCommands(formValue.value.commands, parsed.command);
        }
        formValue.value.settings = parsed;
      } catch (e) {
        message.error('配置 JSON 格式不正确');
        return;
      }
    }
    formValue.value.expireDays = Number(formValue.value.expireDays || 0);
    emit('submit', { ...formValue.value });
  }

  function normalizeMountedPlugins(settings) {
    const mounted = Array.isArray(settings?.mountedPlugins) ? settings.mountedPlugins : [];
    if (mounted.length > 0) {
      return mounted;
    }
    if (settings?.mountMenu !== false) {
      return ['menu'];
    }
    return [];
  }

  function normalizeCommands(commands, fallback) {
    const list = Array.isArray(commands) ? commands : [];
    const cleaned = list
      .map((item) => `${item || ''}`.trim())
      .filter((item) => item !== '');
    const primary = `${fallback || ''}`.trim();
    if (primary && !cleaned.includes(primary)) {
      cleaned.unshift(primary);
    }
    if (!cleaned.length) {
      cleaned.push('/拉取');
    }
    return cleaned;
  }

  function createSigninChannel() {
    return { title: '', chatId: '', url: '' };
  }

  function normalizeSigninChannels(raw: any) {
    if (!Array.isArray(raw)) {
      return [];
    }
    return raw
      .map((item) => ({
        title: item?.title || '',
        chatId: item?.chatId || item?.chat || item?.username || item?.id || '',
        url: item?.url || item?.inviteUrl || item?.link || '',
      }))
      .filter((item) => item.title || item.chatId || item.url);
  }

  function normalizeOptionalNumber(value: any) {
    if (value === null || value === undefined || value === '') {
      return undefined;
    }
    return Number(value || 0);
  }

</script>

<style scoped>
  .full-width {
    width: 100%;
  }
</style>
