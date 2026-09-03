<template>

  <a-form-item label="单批文章数" help="每次执行处理的文章数量，修复完成的文章自动跳过，可反复执行直至全部完成">
    <a-input-number v-model="data.batch_size" class="input" :min="1" />
  </a-form-item>

  <a-form-item label="请求超时（秒）" help="抓取源页面/图片的超时时间">
    <a-input-number v-model="data.fetch_timeout_seconds" class="input" :min="1" />
  </a-form-item>

  <a-form-item label="每分钟请求数" help="对源站的页面与图片请求合计限速，避免触发反爬">
    <a-input-number v-model="data.rate_limit_per_minute" class="input" :min="1" />
  </a-form-item>

  <a-form-item label="重试次数">
    <a-input-number v-model="data.down_retry" class="input" :min="0" />
  </a-form-item>

  <a-form-item label="UserAgent" help="可选，留空使用默认 UA">
    <a-input v-model="data.user_agent" class="input" />
  </a-form-item>

  <a-form-item label="标题最少字符数" help="标题有效性阈值：可读字符（字母/数字/汉字）少于此数的视为占位标题">
    <a-input-number v-model="data.min_title_chars" class="input" :min="1" />
  </a-form-item>

  <a-form-item label="正文最少字符数" help="正文有效性阈值：去除 HTML 标签后的可读字符少于此数的视为占位正文">
    <a-input-number v-model="data.min_content_chars" class="input" :min="1" />
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
