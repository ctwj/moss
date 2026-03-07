<template>
  <a-alert type="warning" style="margin-bottom: 16px;">
    使用此插件前，请确保系统缓存已启用，否则限制功能无法正常工作。
  </a-alert>

  <a-form-item :label="$t('enable')">
    <a-switch v-model="data.enable" type="round">
      <template #checked>{{ $t('on') }}</template>
      <template #unchecked>{{ $t('off') }}</template>
    </a-switch>
  </a-form-item>

  <a-form-item label="每日最大下载次数">
    <a-input-number v-model="data.max_per_day" class="numberInput" :min="1" :max="1000" />
  </a-form-item>

  <a-form-item label="Cloudflare 支持">
    <a-switch v-model="data.enable_cloudflare" type="round">
      <template #checked>{{ $t('on') }}</template>
      <template #unchecked>{{ $t('off') }}</template>
    </a-switch>
  </a-form-item>

  <a-form-item label="IP 白名单" help="多个 IP 用逗号分隔，如：192.168.1.100,10.0.0.1">
    <a-textarea v-model="data.white_list" placeholder="留空表示不使用白名单" />
  </a-form-item>
</template>

<script setup>
import {inject} from "vue";
const data = inject("options")
</script>

<style scoped>
.numberInput {
  width: 220px;
}
</style>