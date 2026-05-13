# Implementation Plan: 通用多源采集插件 (Universal Spider)

**Branch**: `001-universal-spider` | **Date**: 2026-05-13 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `specs/001-universal-spider/spec.md`

## Summary

基于现有 GnDownSpider 插件模式，创建一个配置驱动的通用多源采集插件（UniversalSpider）。每个源站通过 JSON 配置定义 CSS 选择器映射，支持翻页、去重、分类映射、下载链接提取。预设 gndown.com、ludown.com、wycad.com 等源站的默认选择器配置，管理员只需填写 URL 即可开始采集。与 GnDownSpider 并存运行。

## Technical Context

**Language/Version**: Go 1.21+
**Primary Dependencies**: goquery (HTML 解析), lancet/cryptor (MD5), robfig/cron (定时任务), zap (日志)
**Storage**: SQLite/MySQL/PostgreSQL (通过 GORM，复用现有 Article 表)
**Testing**: Go standard testing (`go test`)
**Target Platform**: Linux/Windows server
**Project Type**: moss CMS 插件（实现 PluginEntry 接口）
**Performance Goals**: 单篇文章采集 ≤ 5 秒，支持 ≥ 5 个源站
**Constraints**: 与 GnDownSpider 并存、slug 格式兼容、顺序执行
**Scale/Scope**: ≤ 10 个源站配置，每个源站每日采集 10-50 篇文章

## Constitution Check

Constitution 文件为空模板，无 gates 限制。所有功能需求在现有技术栈内可实现。

## Project Structure

### Documentation (this feature)

```text
specs/001-universal-spider/
├── plan.md              # 本文件
├── research.md          # Phase 0 研究结果
├── data-model.md        # Phase 1 数据模型
├── quickstart.md        # Phase 1 快速开始
└── tasks.md             # Phase 2 任务列表 (由 /speckit-tasks 生成)
```

### Source Code (repository root)

```text
main/plugins/
├── GnDownSpider.go              # 现有插件（保持不变）
├── UniversalSpider.go           # 新插件主文件 - 结构体定义、Info/Load/Run
├── universal_spider_types.go    # 类型定义 - SiteConfig/SelectorMapping/CategoryMapping
├── universal_spider_fetcher.go  # HTTP 请求 + HTML 解析逻辑
├── universal_spider_presets.go  # 预设源站配置（gndown/ludown/wycad/423down/portableappk）
└── universal_spider_test.go     # 测试文件

main/startup/
└── startup.go                   # 注册新插件（添加 NewUniversalSpider()）
```

**Structure Decision**: 在 `main/plugins/` 目录下新建多个文件，与现有插件并列。逻辑拆分为：类型定义、采集逻辑、预设配置三个文件，主文件负责插件生命周期。

## Architecture Design

### 核心组件

```
UniversalSpider (主入口)
  ├── SiteConfig[]         → 多源站配置数组（JSON 字段）
  │     ├── SelectorMapping  → CSS 选择器映射
  │     └── CategoryMapping  → 分类名称 → 本地 ID 映射
  ├── fetcher              → HTTP 请求 + goquery 解析
  └── presets              → 内置的预设源站配置
```

### 执行流程

```
Run()
  ├── 遍历 Sites[]（顺序执行）
  │     ├── 跳过 disabled 的站点
  │     ├── 翻页循环 (page 1, 2, 3...)
  │     │     ├── getArticleLinks() → 使用列表页选择器提取链接
  │     │     ├── 跳过 VIP/置顶（使用 skip_selectors）
  │     │     ├── 遍历链接
  │     │     │     ├── buildSlug(siteID + title) → 去重检查
  │     │     │     ├── fetchArticle() → 请求详情页
  │     │     │     │     ├── extractTitle()
  │     │     │     │     ├── extractContent() → 清理非正文元素
  │     │     │     │     ├── processDownloadSection() → 提取下载链接
  │     │     │     │     ├── extractCategory() → 分类映射
  │     │     │     │     ├── extractThumbnail()
  │     │     │     │     └── extractMeta/Keywords/Time()
  │     │     │     └── service.Article.Create()
  │     │     └── 整页已采集则停止翻页
  │     └── 记录本站统计
  └── 输出总统计日志
```

### 选择器映射配置项

| 配置键 | 用途 | 默认值（WordPress 通用） |
|--------|------|--------------------------|
| list_item | 列表页文章条目 | `.excerpt, .post-list .post-item` |
| article_link | 文章链接 | `h2 a, a.post-title` |
| title | 文章标题 | `h1` |
| content | 正文内容 | `article.article-content, .article-content, .entry-content` |
| category | 分类 | 多级回退（详见 presets） |
| thumbnail | 封面图 | `meta[property='og:image']` 等 |
| time | 发布时间 | `.time, time` |
| skip_markers | VIP/置顶标记 | `span.sticky-icon:contains(VIP)` |
| download_section | 下载地址标记 | `h5:contains(下载地址)` |
| pagination | 翻页 URL 模式 | `/page/{page}` |

### 预设源站配置

每个预设包含该站特有的选择器规则，管理员配置时只需提供 URL，系统根据域名自动匹配预设。

| 预设 ID | 站点 | 域名匹配 |
|---------|------|----------|
| gndown | 绿软小站 (gndown.com) | gndown.com |
| ludown | 绿软部落 (ludown.com) | ludown.com |
| wycad | 无忧软件网 (wycad.com) | wycad.com |
| wuleba | 吾乐吧 (wuleba.com) | wuleba.com |
| lr08 | 08绿软站 (08lr.cn) | 08lr.cn |

## Complexity Tracking

无 Constitution 违规，不需要复杂度追踪。
