# Research: Universal Spider

**Branch**: `001-universal-spider` | **Date**: 2026-05-13

## R1: 插件接口与配置持久化

**Decision**: 实现 `PluginEntry` 接口（Info/Load/Run），配置通过 JSON struct tags 存储在插件配置字段中。

**Rationale**: 与现有 GnDownSpider、DidiAuto 等插件完全一致的模式。moss 的 `PluginInit` 接受 `PluginEntry` 实现者，配置通过 JSON 序列化自动持久化到数据库。管理员通过 moss 后台插件配置界面编辑 JSON。

**Alternatives considered**:
- 独立配置文件：需要额外的文件管理逻辑，与 moss 插件体系不一致
- 数据库独立表：增加迁移负担，复用现有插件配置字段更简洁

## R2: 多源站配置的数据结构

**Decision**: 在插件 struct 中使用 `Sites []SiteConfig` JSON 数组字段，每个 SiteConfig 包含完整的源站配置。

**Rationale**: JSON 数组天然支持多源站，通过 moss 后台可直接编辑。SiteConfig 内嵌 SelectorMapping 和 CategoryMapping，形成自包含的配置单元。

**Alternatives considered**:
- 扁平化配置（多个 URL 字段）：不支持不同源站不同选择器
- 外部引用配置：增加复杂度，无实际收益

## R3: 选择器映射的灵活性设计

**Decision**: 每个选择器配置项支持**逗号分隔的多选择器回退链**（如 `.excerpt, .post-item`），解析时按顺序尝试直到匹配。

**Rationale**: 与现有 GnDownSpider 的 `contentSelectors []string` 模式一致。不同站点的 HTML 结构差异大，多选择器回退可以覆盖大多数情况。逗号分隔在 CSS 中是标准语法，管理员容易理解。

**Alternatives considered**:
- 每个选择器只允许一个值：太死板，无法适配结构差异
- 嵌套数组配置：JSON 结构过于复杂

## R4: 预设配置的加载策略

**Decision**: 使用域名匹配（`strings.Contains`）自动从内置 presets map 中查找预设配置。管理员只配置 URL，选择器自动填充。管理员可手动覆盖预设选择器。

**Rationale**: 按域名自动匹配最直观，覆盖即可用。允许覆盖保证灵活性。内置 presets 作为 Go map 硬编码，零外部依赖。

**Alternatives considered**:
- 每次采集时实时爬取源站结构分析：过于复杂且不可靠
- 外部 JSON 预设文件：需要文件管理，不如硬编码简洁

## R5: 下载链接提取的通用化

**Decision**: 从 GnDownSpider 的 ProcessDownloadSection 提取通用逻辑，通过 `download_section` 选择器配置"下载地址"标记元素（默认 `h5` 包含"下载地址"文本），标记之后的所有兄弟节点作为下载区域解析。

**Rationale**: 大部分软件下载站都有"下载地址"分隔标记，只是标签和文本可能不同（`h5`、`h3`、`div.title` 等）。通过配置化标记位置，可以适配不同站点。

**Alternatives considered**:
- 正则匹配整个内容中的网盘链接：无法区分正文和下载区域，会误提取
- 完全硬编码提取逻辑：不够通用

## R6: 现有源站 HTML 结构调研

**调研结果**（基于实际网站访问）：

| 站点 | CMS | 列表结构 | 文章结构 | 下载标记 |
|------|-----|----------|----------|----------|
| gndown.com | WordPress | `.excerpt h2 a` | `article.article-content` | `h5:contains(下载地址)` |
| ludown.com | WordPress | `.excerpt h2 a` | `.article-content` | `h5` 或 `h4` 下载地址 |
| wycad.com | WordPress | `.post-item a.post-title` | `.entry-content` | `h3` 或 `h4` 下载 |
| wuleba.com | WordPress | `.excerpt h2 a` | `.article-content` | `h5` 下载地址 |
| 08lr.cn | 自定义 | `.soft-list a` | `.soft-content` | 需进一步验证 |

**Conclusion**: WordPress 类站点结构高度相似，预设配置可覆盖大部分场景。
