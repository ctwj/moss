# Quickstart: Universal Spider

**Branch**: `001-universal-spider` | **Date**: 2026-05-13

## 概述

通用多源采集插件，通过配置驱动从多个软件下载站自动采集文章。

## 前置条件

- moss 系统正常运行
- 本地分类已创建（应用软件、系统工具、编程开发等）
- Go 开发环境

## 配置示例

在 moss 后台 → 插件管理 → UniversalSpider 的配置 JSON 中：

```json
{
  "Sites": [
    {
      "Enabled": true,
      "Name": "绿软小站",
      "URL": "https://www.gndown.com",
      "SiteID": "gndown"
    },
    {
      "Enabled": true,
      "Name": "绿软部落",
      "URL": "https://www.ludown.com",
      "SiteID": "ludown"
    },
    {
      "Enabled": true,
      "Name": "无忧软件网",
      "URL": "https://www.wycad.com",
      "SiteID": "wycad"
    }
  ],
  "Interval": "@every 1h",
  "DefaultTimeout": 30,
  "DefaultRetry": 2,
  "DefaultInterval": 1,
  "DefaultCategory": 19
}
```

以上配置使用了预设选择器，不需要填写 `Selectors` 和 `CategoryMap`。

## 添加新源站

### 使用预设配置（推荐）

只需填写基本信息即可：

```json
{
  "Enabled": true,
  "Name": "新站点名",
  "URL": "https://www.example.com",
  "SiteID": "example"
}
```

如果该域名有内置预设，选择器和分类映射会自动加载。

### 自定义选择器

如果站点没有预设，或需要覆盖预设配置：

```json
{
  "Enabled": true,
  "Name": "自定义站点",
  "URL": "https://www.custom.com",
  "SiteID": "custom",
  "Selectors": {
    "ListItem": ".post-item",
    "ArticleLink": "h2 a",
    "Title": "h1",
    "Content": ".post-content, .entry-content",
    "Category": ".category a",
    "Time": ".date",
    "DownloadTag": "h4",
    "Pagination": "/page/{page}"
  },
  "CategoryMap": {
    "应用软件": 5,
    "系统工具": 6
  }
}
```

## 运行

1. **手动触发**: moss 后台 → 插件管理 → UniversalSpider → 点击"运行"
2. **定时任务**: 插件配置中启用 CronStart，设置 CronExp（如 `@every 1h`）

## 日志

采集过程中的所有操作、错误、统计信息都记录在 moss 插件日志中，可在后台查看。

## 去重机制

- 基于 slug 去重：`slug = MD5(siteID + title)[:12]`
- 同一篇文章从同一源站多次采集不会重复创建
- 不同源站的相同标题文章 slug 不同（因为 siteID 不同）

## 注意事项

- 与现有 GnDownSpider 插件并存，不会冲突
- 图片保留源站原始 URL，不下载到本地
- 请求间隔默认 1 秒，避免对源站造成压力
- 源站不可达时记录错误并继续下一个源站
