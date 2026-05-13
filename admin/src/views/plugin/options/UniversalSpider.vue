<template>
  <a-alert type="info" style="margin-bottom: 16px;">
    通用多源采集插件，支持配置多个源站自动采集文章。配置源站 URL 后，选择器和分类映射会自动填充预设值。
  </a-alert>

  <a-form-item label="采集间隔" help="Cron表达式，默认每小时采集一次">
    <a-input v-model="data.interval" placeholder="@every 1h" />
  </a-form-item>

  <a-form-item label="默认代理地址" help="全局代理，单个源站可单独覆盖">
    <a-input v-model="data.default_proxy" placeholder="留空则不使用代理" />
  </a-form-item>

  <a-form-item label="默认超时（秒）">
    <a-input-number v-model="data.default_timeout" :min="10" :max="300" />
  </a-form-item>

  <a-form-item label="默认重试次数">
    <a-input-number v-model="data.default_retry" :min="0" :max="5" />
  </a-form-item>

  <a-form-item label="默认请求间隔（秒）" help="采集限速，默认1秒">
    <a-input-number v-model="data.default_interval" :min="1" :max="30" />
  </a-form-item>

  <a-form-item label="默认分类ID" help="无法匹配分类时使用的默认分类">
    <a-input-number v-model="data.default_category" :min="1" />
  </a-form-item>

  <a-divider />

  <a-form-item label="源站列表">
    <div class="w-full">
      <div v-for="(site, index) in data.sites" :key="index"
           style="border: 1px solid var(--color-border-2); border-radius: 6px; padding: 16px; margin-bottom: 12px;">
        <div style="display: flex; align-items: center; justify-content: space-between; margin-bottom: 12px;">
          <a-tag :color="site.enabled ? 'green' : 'red'">{{ site.enabled ? '已启用' : '已禁用' }}</a-tag>
          <a-button size="small" type="text" status="danger" @click="removeSite(index)">
            <template #icon><icon-delete /></template>
          </a-button>
        </div>

        <a-form-item label="启用">
          <a-switch v-model="site.enabled" type="round" />
        </a-form-item>

        <a-form-item label="源站名称">
          <a-input v-model="site.name" placeholder="绿软小站" />
        </a-form-item>

        <a-form-item label="源站URL" required>
          <a-input v-model="site.url" placeholder="https://www.gndown.com" />
        </a-form-item>

        <a-form-item label="站点ID" help="用于生成唯一slug，避免重复采集">
          <a-input v-model="site.site_id" placeholder="留空自动从预设匹配" />
        </a-form-item>

        <a-form-item label="最大采集页数">
          <a-input-number v-model="site.max_pages" :min="1" :max="100" />
        </a-form-item>

        <a-form-item label="请求间隔（秒）" help="留空使用全局默认值">
          <a-input-number v-model="site.request_interval" :min="0" :max="30" placeholder="0 = 使用默认" />
        </a-form-item>

        <a-form-item label="代理地址" help="留空使用全局默认代理">
          <a-input v-model="site.proxy" placeholder="留空使用全局代理" />
        </a-form-item>

        <a-collapse :default-active-key="[]">
          <a-collapse-item header="高级设置（选择器/分类映射）" :key="'advanced-' + index">
            <a-form-item label="内容选择器">
              <a-input v-model="site.selectors.content" placeholder="留空使用预设值" />
            </a-form-item>
            <a-form-item label="列表条目选择器">
              <a-input v-model="site.selectors.list_item" placeholder="留空使用预设值" />
            </a-form-item>
            <a-form-item label="文章链接选择器">
              <a-input v-model="site.selectors.article_link" placeholder="留空使用预设值" />
            </a-form-item>
            <a-form-item label="下载标签选择器">
              <a-input v-model="site.selectors.download_tag" placeholder="留空使用预设值" />
            </a-form-item>
            <a-form-item label="下载区域标记文本">
              <a-input v-model="site.download_section" placeholder="留空使用预设值" />
            </a-form-item>
            <a-form-item label="跳过标记选择器">
              <a-input v-model="site.selectors.skip_marker" placeholder="如 span.sticky-icon" />
            </a-form-item>
            <a-form-item label="跳过标记文本" help="逗号分隔，如：VIP,置顶">
              <a-input v-model="site.selectors.skip_texts" placeholder="VIP,置顶" />
            </a-form-item>
            <a-form-item label="翻页URL模板">
              <a-input v-model="site.selectors.pagination" placeholder="/page/{page}" />
            </a-form-item>
          </a-collapse-item>
        </a-collapse>
      </div>

      <a-button type="dashed" long @click="addSite">
        <template #icon><icon-plus /></template>
        添加源站
      </a-button>
    </div>
  </a-form-item>
</template>

<script setup>
import {inject} from "vue";
const data = inject("options")

if (!data.sites) {
  data.sites = []
}

function addSite() {
  data.sites.push({
    enabled: true,
    name: "",
    url: "",
    site_id: "",
    selectors: {},
    category_map: {},
    max_pages: 3,
  })
}

function removeSite(index) {
  data.sites.splice(index, 1)
}
</script>
