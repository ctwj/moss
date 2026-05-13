# Tasks: 通用多源采集插件 (Universal Spider)

**Input**: Design documents from `/specs/001-universal-spider/`
**Prerequisites**: plan.md (required), spec.md (required), data-model.md, research.md

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3, US4)
- Include exact file paths in descriptions

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: 创建插件基础文件结构和类型定义

- [x] T001 Create type definitions (SiteConfig, SelectorMapping, CategoryMapping, SpiderResult) in `main/plugins/universal_spider_types.go`
- [x] T002 [P] Create plugin entry point (UniversalSpider struct, NewUniversalSpider, Info, Load, MarshalJSON) in `main/plugins/UniversalSpider.go`
- [x] T003 [P] Create preset site configurations map (gndown/ludown/wycad/wuleba/lr08) in `main/plugins/universal_spider_presets.go`
- [x] T004 Register UniversalSpider plugin in `main/startup/startup.go` (add `plugins.NewUniversalSpider()` to initPlugins)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: 核心采集逻辑，所有用户故事的基础

**CRITICAL**: 所有用户故事依赖此阶段完成

- [x] T005 Implement HTTP request helper with rate limiting (waitForRequestSlot, effectiveRequestInterval, normalizeBaseURL) in `main/plugins/universal_spider_fetcher.go`
- [x] T006 Implement getArticleLinks - list page parsing with multi-selector support and VIP/sticky skip logic in `main/plugins/universal_spider_fetcher.go`
- [x] T007 Implement extractContent - content extraction with multi-selector fallback, non-content element cleanup, and image path conversion in `main/plugins/universal_spider_fetcher.go`
- [x] T008 [P] Implement extractTitle, extractMetaDescription, extractKeywords, extractTime, extractThumbnail in `main/plugins/universal_spider_fetcher.go`
- [x] T009 Implement ProcessDownloadSection - download link extraction (type, URL, password) from content after download marker tag in `main/plugins/universal_spider_fetcher.go`
- [x] T010 Implement buildSlug - MD5 hash of siteID+title prefix (12 chars) for deduplication in `main/plugins/universal_spider_fetcher.go`
- [x] T011 Implement buildExtends - build extends field (source_url, category, version, file_size, language) in `main/plugins/universal_spider_fetcher.go`
- [x] T012 Implement isValidURL helper for download link validation in `main/plugins/universal_spider_fetcher.go`

**Checkpoint**: 基础采集工具函数就绪，可以开始用户故事实现

---

## Phase 3: User Story 1 - 从单个源站自动采集软件文章 (Priority: P1) MVP

**Goal**: 配置一个源站地址，系统自动采集文章（标题、内容、封面图、分类、下载链接）并创建到本地站点

**Independent Test**: 配置一个源站 URL（如 gndown.com），手动触发采集任务，验证文章被成功创建且字段完整

### Implementation for User Story 1

- [x] T013 [US1] Implement Run method - main entry point: iterate enabled sites, paginate, collect articles per plan.md execution flow in `main/plugins/UniversalSpider.go`
- [x] T014 [US1] Implement fetchArticle - orchestrates all extraction functions (title, content, download, category, thumbnail, meta, keywords, time) and assembles entity.Article in `main/plugins/universal_spider_fetcher.go`
- [x] T015 [US1] Implement pagination loop in Run method - stop when full page already collected, support MaxPages limit in `main/plugins/UniversalSpider.go`
- [x] T016 [US1] Add preset auto-loading in Load method - match site URL domain to presets map, auto-fill Selectors and CategoryMap if empty in `main/plugins/UniversalSpider.go`

**Checkpoint**: 单源站采集功能完整可用 - 可通过 moss 后台配置一个 gndown.com 源站并手动运行采集

---

## Phase 4: User Story 2 - 通过配置管理多个源站 (Priority: P2)

**Goal**: 通过 JSON 配置管理多个采集源站，每个源站有独立的选择器映射规则，不写代码即可添加新源站

**Independent Test**: 在配置中添加 ludown.com 和 wycad.com 两个源站，运行采集任务，验证能从两个源站依次采集文章

### Implementation for User Story 2

- [x] T017 [US2] Add multi-site sequential iteration loop in Run method - iterate Sites array in order, log per-site statistics in `main/plugins/UniversalSpider.go`
- [x] T018 [US2] Add site-level error isolation - catch errors per site, log and continue to next site instead of aborting entire run in `main/plugins/UniversalSpider.go`
- [x] T019 [US2] Implement site-level config override resolution (Timeout/Retry/RequestInterval/Proxy fall back to global defaults when site-level value is 0/empty) in `main/plugins/universal_spider_fetcher.go`
- [x] T020 [US2] Verify preset auto-loading works for multiple domains (ludown.com, wycad.com, wuleba.com, 08lr.cn) - ensure each gets correct selectors in `main/plugins/universal_spider_presets.go`

**Checkpoint**: 多源站配置采集功能完整 - 可同时配置多个源站并成功采集

---

## Phase 5: User Story 3 - 自动映射源站分类到本地分类 (Priority: P3)

**Goal**: 源站分类自动映射到本地分类体系，采集来的文章自动归类

**Independent Test**: 配置分类映射规则，采集文章后验证其 CategoryID 正确匹配

### Implementation for User Story 3

- [x] T021 [US3] Implement extractCategory with multi-level fallback (span.item category tag → a.cat → breadcrumbs → .article-meta → .meta .cat) using configurable selectors in `main/plugins/universal_spider_fetcher.go`
- [x] T022 [US3] Implement category mapping resolution - lookup CategoryMap, fallback to service.Category.GetByName, then to DefaultCategory in `main/plugins/universal_spider_fetcher.go`
- [x] T023 [US3] Add default category mappings to each preset configuration in `main/plugins/universal_spider_presets.go`

**Checkpoint**: 分类映射功能完整 - 采集的文章自动归入正确的本地分类

---

## Phase 6: User Story 4 - 下载链接智能提取与存储 (Priority: P4)

**Goal**: 智能识别文章中的下载链接，提取网盘类型、URL 和密码，结构化存储

**Independent Test**: 采集含下载链接的文章，验证 Res 字段包含完整的下载链接数组（type、url、password）

### Implementation for User Story 4

- [x] T024 [US4] Make ProcessDownloadSection configurable - use DownloadTag selector and DownloadSection text from SiteConfig.Selectors instead of hardcoded "h5" and "下载地址" in `main/plugins/universal_spider_fetcher.go`
- [x] T025 [US4] Add cloud drive type auto-detection (百度云/夸克云/蓝奏云/城通盘/123盘 based on URL domain patterns) in `main/plugins/universal_spider_fetcher.go`
- [x] T026 [US4] Ensure download links stored in article.Res field as `[{key: "download_links", value: [...]}]` matching GnDownSpider format in `main/plugins/universal_spider_fetcher.go`

**Checkpoint**: 下载链接提取功能完整 - 结构化存储网盘类型、URL 和密码

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: 跨用户故事的改进和测试

- [x] T027 [P] Add comprehensive logging throughout Run/fetchArticle with zap (site progress, article status, error details, final statistics) in `main/plugins/UniversalSpider.go`
- [x] T028 [P] Add unit tests for buildSlug, isValidURL, normalizeBaseURL, extractCategory, ProcessDownloadSection in `main/plugins/universal_spider_test.go`
- [x] T029 Verify plugin coexistence with GnDownSpider - ensure no slug conflicts (different siteID prefixes), both can run independently in `main/plugins/UniversalSpider.go`
- [x] T030 Compile and run `go build ./...` to verify plugin compiles correctly

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - start immediately
- **Foundational (Phase 2)**: Depends on Phase 1 types (T001) and plugin entry (T002)
- **User Story 1 (Phase 3)**: Depends on Phase 2 fetcher functions
- **User Story 2 (Phase 4)**: Depends on Phase 3 (US1) Run method
- **User Story 3 (Phase 5)**: Depends on Phase 2 (T014 fetchArticle)
- **User Story 4 (Phase 6)**: Depends on Phase 2 (T009 ProcessDownloadSection)
- **Polish (Phase 7)**: Depends on all user stories

### User Story Dependencies

- **US1 (P1)**: Depends on Phase 2 - No dependencies on other stories
- **US2 (P2)**: Depends on US1 (Run method must exist first)
- **US3 (P3)**: Can proceed in parallel with US2 after Phase 2 (only needs fetchArticle)
- **US4 (P4)**: Can proceed in parallel with US2/US3 after Phase 2 (only needs ProcessDownloadSection)

### Parallel Opportunities

- T002, T003 can run in parallel (different files)
- T008, T009, T010, T011, T012 can run in parallel (independent functions in same file, but safe to develop concurrently)
- T021-T023 (US3) and T024-T026 (US4) can run in parallel with each other after Phase 2
- T027, T028 can run in parallel (different files)

---

## Parallel Example: Phase 1

```
T001 → types.go (独立文件)
T002 → UniversalSpider.go (独立文件)
T003 → presets.go (独立文件)
→ T004 depends on T001+T002 (注册需要结构体定义)
```

## Parallel Example: After Phase 2

```
US1 (T013-T016) → 必须先完成
→ US2 (T017-T020) 和 US3 (T021-T023) 和 US4 (T024-T026) 可并行
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Phase 1: Setup → 创建 4 个文件
2. Phase 2: Foundational → 实现核心采集函数
3. Phase 3: US1 → 实现单源站采集
4. **STOP and VALIDATE**: 配置 gndown.com，手动触发，验证采集成功

### Incremental Delivery

1. Setup + Foundational → 基础就绪
2. US1 → 单源站采集可用 (MVP!)
3. US2 → 多源站配置
4. US3 → 分类映射
5. US4 → 下载链接结构化
6. Polish → 测试和日志

---

## Notes

- 所有文件在 `main/plugins/` 目录下，与现有插件并列
- 复用现有 `infrastructure/utils/request` HTTP 请求库
- 复用现有 `domain/core/entity.Article` 和 `domain/core/service.Article`
- slug 格式 = MD5(siteID + title)[:12]，与 GnDownSpider 格式兼容但 siteID 不同避免冲突
- 图片保留源站原始 URL，不做本地下载
