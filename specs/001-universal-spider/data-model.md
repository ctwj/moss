# Data Model: Universal Spider

**Branch**: `001-universal-spider` | **Date**: 2026-05-13

## Entities

### UniversalSpider（插件主结构体）

插件入口，实现 `PluginEntry` 接口。所有字段通过 JSON 序列化存储在 moss 插件配置中。

```
UniversalSpider
├── Sites           []SiteConfig     // 多源站配置数组
├── Interval        string           // 全局 Cron 表达式 (默认 "@every 1h")
├── DefaultProxy    string           // 全局默认代理
├── DefaultTimeout  int              // 全局默认超时秒数 (默认 30)
├── DefaultRetry    int              // 全局默认重试次数 (默认 2)
├── DefaultInterval int              // 全局默认请求间隔秒数 (默认 1)
├── DefaultCategory int              // 未匹配分类时的默认 CategoryID (默认 19)
├── LastUpdate      int64            // 上次采集时间戳
└── ctx             *pluginEntity.Plugin // 运行时上下文（不序列化）
```

### SiteConfig（源站配置）

单个采集源站的完整配置，是 `Sites` 数组的元素。

```
SiteConfig
├── Enabled         bool             // 是否启用此源站
├── Name            string           // 源站名称（如"绿软小站"）
├── URL             string           // 源站基础 URL（如 "https://www.gndown.com"）
├── SiteID          string           // 源站唯一标识（用于 slug 前缀，如 "gndown"）
├── Selectors       SelectorMapping  // CSS 选择器映射
├── CategoryMap     CategoryMapping  // 分类映射表
├── Proxy           string           // 站点级代理（覆盖全局）
├── Timeout         int              // 站点级超时（覆盖全局，0 表示使用全局值）
├── Retry           int              // 站点级重试（覆盖全局）
├── RequestInterval int              // 站点级请求间隔（覆盖全局）
├── MaxPages        int              // 最大翻页数（0 表示不限制）
└── DownloadSection string           // 下载区域标记文本（默认 "下载地址"）
```

### SelectorMapping（选择器映射）

定义如何从 HTML 页面中提取数据。每个字段是逗号分隔的多选择器回退链。

```
SelectorMapping
├── ListItem        string   // 列表页文章条目（默认 ".excerpt, .post-list .post-item"）
├── ArticleLink     string   // 文章链接（默认 "h2 a, a.post-title"）
├── Title           string   // 文章标题（默认 "h1"）
├── Content         string   // 正文内容（默认 "article.article-content, .article-content, .entry-content"）
├── Category        string   // 分类提取（多级回退逻辑，特殊处理）
├── Thumbnail       string   // 封面图（默认 "meta[property='og:image']" 等）
├── Time            string   // 发布时间（默认 ".time, time"）
├── SkipMarker      string   // VIP/置顶标记（默认 "span.sticky-icon"）
├── SkipTexts       string   // 需跳过的标记文本（默认 "VIP,置顶"）
├── DownloadTag     string   // 下载地址标签选择器（默认 "h5"）
└── Pagination      string   // 翻页 URL 模板（默认 "/page/{page}"）
```

### CategoryMapping（分类映射）

源站分类名称 → 本地分类 ID 的映射。使用 `map[string]int` 类型。

```
CategoryMapping = map[string]int

示例:
{
  "应用软件": 5,
  "系统工具": 6,
  "编程开发": 7,
  "安全": 8,
  "多媒体": 12,
  "网络工具": 15,
  "Mac应用": 16,
  "安卓软件": 17,
  "安卓游戏": 18,
  "其他软件": 19
}
```

### SpiderResult（采集结果统计）

单次采集任务的运行时统计，仅用于日志输出，不持久化。

```
SpiderResult
├── Collected  int     // 成功采集数量
├── Skipped    int     // 跳过（已存在）数量
├── Errors     int     // 错误数量
└── Duration   int64   // 耗时（秒）
```

## 数据流

```
插件 JSON 配置
    ↓ 加载
[]SiteConfig (内存)
    ↓ 遍历
每个 SiteConfig
    ↓ 翻页
列表页 HTML → goquery 解析 → []文章链接
    ↓ 遍历
详情页 HTML → goquery 解析 → entity.Article
    ├── Slug = MD5(siteID + title)[:12]
    ├── CategoryID = CategoryMap[extractedCategory] || DefaultCategory
    ├── Content = 清理后的正文 HTML
    ├── Res = [{key: "download_links", value: [下载链接数组]}]
    ├── Extends = [source_url, category, version, file_size, ...]
    └── Thumbnail = 封面图绝对 URL
    ↓
service.Article.Create() → 数据库持久化
```

## 验证规则

- `SiteConfig.URL` 必须非空且为有效 HTTP(S) URL
- `SiteConfig.SiteID` 必须非空（用于 slug 生成）
- `SelectorMapping.Content` 必须非空（否则无法提取正文）
- `Slug` 长度 = 12（MD5 前 12 位），通过 `gorm:"uniqueIndex"` 保证唯一性
