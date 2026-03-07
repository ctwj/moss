<template>

  <a-form-item label="源站URL" required>
    <a-input v-model="data.source_url" placeholder="https://www.gndown.com" />
  </a-form-item>

  <a-form-item label="采集间隔" help="Cron表达式，默认每小时采集一次">
    <a-input v-model="data.interval" placeholder="@every 1h" />
  </a-form-item>

  <a-form-item label="代理地址" help="可选，如：http://127.0.0.1:7890">
    <a-input v-model="data.proxy" placeholder="留空则不使用代理" />
  </a-form-item>

  <a-form-item label="重试次数">
    <a-input-number v-model="data.retry" :min="0" :max="5" />
  </a-form-item>

  <a-form-item label="超时时间（秒）">
    <a-input-number v-model="data.timeout" :min="10" :max="300" />
  </a-form-item>

  <a-form-item label="请求间隔（秒）" help="采集限速，默认1秒1次请求">
    <a-input-number v-model="data.request_interval" :min="1" :max="30" />
  </a-form-item>

</template>


<script setup>
 import {inject} from "vue";
 const data = inject("options")
</script>
