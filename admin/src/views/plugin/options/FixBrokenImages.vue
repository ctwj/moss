<template>

  <a-form-item label="失效图床域名" required help="待修复的图床域名，如 image.08rj.com">
    <a-input v-model="data.broken_domain" class="input" placeholder="image.08rj.com" />
  </a-form-item>

  <a-form-item label="单批文章数" help="每次执行处理的文章数量，修复完成的文章自动跳过，可反复执行直至全部完成">
    <a-input-number v-model="data.batch_size" class="input" :min="1" />
  </a-form-item>

  <a-form-item label="请求超时（秒）">
    <a-input-number v-model="data.fetch_timeout_seconds" class="input" :min="1" />
  </a-form-item>

  <a-form-item label="每分钟请求数" help="对源站的页面与图片请求合计限速，避免触发反爬">
    <a-input-number v-model="data.rate_limit_per_minute" class="input" :min="1" />
  </a-form-item>

  <a-form-item label="重试次数">
    <a-input-number v-model="data.down_retry" class="input" :min="0" />
  </a-form-item>

  <a-form-item label="Referer" help="可选，下载图片时携带的 Referer，留空使用源页面地址">
    <a-input v-model="data.down_referer" class="input" />
  </a-form-item>

  <a-form-item label="UserAgent" help="可选，留空使用默认 UA">
    <a-input v-model="data.user_agent" class="input" />
  </a-form-item>

</template>


<script setup>
 import {inject} from "vue";
 const data = inject("options")
</script>
<style scoped>
.input{
  width: 220px;
}
</style>
