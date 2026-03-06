<template>
  <a-form-item label="启用">
    <a-space>
      <a-switch v-model="data.enable_on_create" type="round">
        <template #checked>创建时</template>
        <template #unchecked>创建时</template>
      </a-switch>
      <a-switch v-model="data.enable_on_update" type="round">
        <template #checked>更新时</template>
        <template #unchecked>更新时</template>
      </a-switch>
    </a-space>
  </a-form-item>

  <a-form-item label="保存方式">
    <a-select v-model="data.upload_target" class="w-full">
      <a-option value="local">本地</a-option>
      <a-option value="api">API图床</a-option>
    </a-select>
  </a-form-item>

  <template v-if="data.enable_on_create || data.enable_on_update">

  <a-divider class="w-full" style="margin-top:0" />

  <a-tabs type="rounded">
    <a-tab-pane key="base" title="基础">
      <a-form-item label="最大宽度">
        <a-input-number v-model="data.max_width" class="numberInput" :min="0" />
      </a-form-item>
      <a-form-item label="最大高度">
        <a-input-number v-model="data.max_height" class="numberInput" :min="0" />
      </a-form-item>
      <a-form-item label="缩略图宽度">
        <a-input-number v-model="data.thumb_width" class="numberInput" :min="0" />
      </a-form-item>
      <a-form-item label="缩略图高度">
        <a-input-number v-model="data.thumb_height" class="numberInput" :min="0" />
      </a-form-item>
      <a-form-item label="缩略图最小宽度">
        <a-input-number v-model="data.thumb_min_width" class="numberInput" :min="0" />
      </a-form-item>
      <a-form-item label="缩略图最小高度">
        <a-input-number v-model="data.thumb_min_height" class="numberInput" :min="0" />
      </a-form-item>
    </a-tab-pane>

    <a-tab-pane key="more" title="高级">
      <a-form-item label="下载重试次数">
        <a-input-number v-model="data.down_retry" class="numberInput" :min="0" :max="10" />
      </a-form-item>
      <a-form-item label="始终压缩尺寸">
        <a-switch type="round" v-model="data.always_resize"/>
      </a-form-item>
      <a-form-item label="缩略图焦点裁剪">
        <a-switch type="round" v-model="data.thumb_extract_focus"/>
      </a-form-item>
      <a-form-item label="下载失败时移除">
        <a-switch type="round" v-model="data.remove_if_down_fail"/>
      </a-form-item>

      <a-form-item label="下载代理">
        <a-input v-model="data.down_proxy" class="w-full" />
      </a-form-item>

      <a-form-item label="下载 Referer">
        <a-textarea v-model="data.down_referer" :auto-size="{minRows:4,maxRows:6}"/>
      </a-form-item>
    </a-tab-pane>

    <a-tab-pane key="api" title="图床API">
      <template v-if="data.upload_target === 'api'">
        <a-form-item label="上传接口地址">
          <a-input v-model="data.api_upload_url" class="w-full" placeholder="https://api.example.com/upload" />
        </a-form-item>

        <a-form-item label="文件字段名">
          <a-input v-model="data.api_file_field" class="w-full" placeholder="file" />
        </a-form-item>

        <a-form-item label="请求超时(秒)">
          <a-input-number v-model="data.api_timeout" class="numberInput" :min="5" :max="300" />
        </a-form-item>

        <a-form-item label="上传代理">
          <a-input v-model="data.api_proxy" class="w-full" placeholder="http://127.0.0.1:7890" />
        </a-form-item>

        <a-form-item label="图床域名">
          <a-input v-model="data.api_image_domain" class="w-full" placeholder="https://img.example.com/" />
        </a-form-item>

        <a-form-item label="返回图片URL路径">
          <a-input v-model="data.api_url_path" class="w-full" placeholder="data.url" />
        </a-form-item>

        <a-form-item label="成功标识路径">
          <a-input v-model="data.api_success_path" class="w-full" placeholder="success" />
        </a-form-item>

        <a-form-item label="成功标识值">
          <a-input v-model="data.api_success_value" class="w-full" placeholder="true" />
        </a-form-item>

        <a-form-item label="请求头" help="每行一条，格式：key: value">
          <a-textarea v-model="data.api_headers" :auto-size="{minRows:3,maxRows:6}" />
        </a-form-item>

        <a-form-item label="附加表单参数" help="每行一条，格式：key=value">
          <a-textarea v-model="data.api_form_data" :auto-size="{minRows:3,maxRows:6}" />
        </a-form-item>
      </template>

      <a-alert v-else type="info">
        当前“保存方式”不是 API 图床，无需配置此分组。
      </a-alert>
    </a-tab-pane>
  </a-tabs>

  </template>

</template>


<script setup>
 import {inject} from "vue";
 const data = inject("options")

</script>

<style scoped>
.numberInput{
  width: 220px;
}
</style>
