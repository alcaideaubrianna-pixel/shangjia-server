<template>
  <div>
    <n-card :bordered="false" title="会员认证订单" class="proCard">
      <List orderType="member_vip" />
    </n-card>
  </div>
</template>

<script lang="ts" setup>
  import List from '@/views/asset/rechargeLog/list.vue';
</script>
