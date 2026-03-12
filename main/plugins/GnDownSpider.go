package plugins

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/duke-git/lancet/v2/cryptor"
	"go.uber.org/zap"
	"moss/domain/core/entity"
	"moss/domain/core/service"
	"moss/domain/core/vo"
	pluginEntity "moss/domain/support/entity"
	"moss/infrastructure/utils/request"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type GnDownSpider struct {
	SourceURL       string `json:"source_url"`       // 源站URL
	Interval        string `json:"interval"`         // 采集间隔 (默认 "@every 1h")
	Proxy           string `json:"proxy"`            // 代理地址
	Retry           int    `json:"retry"`            // 重试次数
	Timeout         int    `json:"timeout"`          // 超时秒数
	RequestInterval int    `json:"request_interval"` // 请求间隔(秒)
	LastUpdate      int64  `json:"last_update"`      // 上次更新时间戳

	ctx           *pluginEntity.Plugin
	baseURL       string
	lastRequestAt time.Time
}

func NewGnDownSpider() *GnDownSpider {
	return &GnDownSpider{
		SourceURL:       "https://www.gndown.com",
		Interval:        "@every 1h",
		Retry:           2,
		Timeout:         30,
		RequestInterval: 1,
		LastUpdate:      0,
	}
}

func (g *GnDownSpider) Info() *pluginEntity.PluginInfo {
	return &pluginEntity.PluginInfo{
		ID:         "GnDownSpider",
		About:      "GnDown爬虫",
		RunEnable:  true,
		CronEnable: true,
		PluginInfoPersistent: pluginEntity.PluginInfoPersistent{
			CronStart: false, // 用户配置后开启
			CronExp:   g.Interval,
		},
	}
}

func (g *GnDownSpider) Load(ctx *pluginEntity.Plugin) error {
	g.ctx = ctx
	// 统一为站点根URL，避免仅采集单个分类路径
	g.baseURL = g.normalizeBaseURL(g.SourceURL)
	return nil
}

func (g *GnDownSpider) Run(ctx *pluginEntity.Plugin) error {
	g.ctx = ctx
	g.lastRequestAt = time.Time{}
	g.ctx.Log.Info("开始采集 gndown.com 文章...")
	startTime := time.Now()

	collected := 0
	skipped := 0
	errors := 0

	// 从第1页开始采集，直到没有更多内容或出现整页已采集
	for page := 1; ; page++ {
		pageURL := fmt.Sprintf("%s/page/%d", g.baseURL, page)
		g.ctx.Log.Info("正在采集页面", zap.String("url", pageURL))

		articleLinks, err := g.getArticleLinks(pageURL)
		if err != nil {
			g.ctx.Log.Error("获取文章链接失败", zap.String("url", pageURL), zap.Error(err))
			errors++
			continue
		}

		if len(articleLinks) == 0 {
			g.ctx.Log.Info("页面无文章，停止采集", zap.Int("page", page))
			break
		}

		pageCollected := 0
		pageSkipped := 0
		for _, link := range articleLinks {
			// 采集文章内容
			article, err := g.fetchArticle(link)
			if err != nil {
				g.ctx.Log.Error("采集文章内容失败", zap.String("url", link), zap.Error(err))
				errors++
				continue
			}

			// 新逻辑：按 title-hash slug 去重，避免多数据源slug冲突
			existsHash, err := service.Article.ExistsSlug(article.Slug)
			if err != nil {
				g.ctx.Log.Error("检查新slug存在性失败", zap.String("slug", article.Slug), zap.Error(err))
				errors++
				continue
			}
			if existsHash {
				g.ctx.Log.Debug("文章已存在（新slug），跳过", zap.String("slug", article.Slug))
				skipped++
				pageSkipped++
				continue
			}

			// 创建文章
			if err := service.Article.Create(article); err != nil {
				g.ctx.Log.Error("创建文章失败", zap.String("title", article.Title), zap.Error(err))
				errors++
				continue
			}

			collected++
			pageCollected++
			g.ctx.Log.Info("成功采集文章", zap.String("title", article.Title), zap.String("slug", article.Slug))
		}

		// 如果当前页全部已采集过，说明后续页基本也是历史数据，停止采集
		if pageCollected == 0 && pageSkipped == len(articleLinks) {
			g.ctx.Log.Info("当前页全部已采集，停止继续翻页",
				zap.Int("page", page),
				zap.Int("page_total", len(articleLinks)))
			break
		}
	}

	// 更新最后采集时间
	g.LastUpdate = time.Now().Unix()

	g.ctx.Log.Info("采集完成",
		zap.Int("采集数量", collected),
		zap.Int("跳过数量", skipped),
		zap.Int("错误数量", errors),
		zap.Int64("耗时(秒)", int64(time.Since(startTime).Seconds())),
	)

	return nil
}

// 获取文章链接列表
func (g *GnDownSpider) getArticleLinks(pageURL string) ([]string, error) {
	g.waitForRequestSlot(pageURL)

	// 发送HTTP请求
	body, err := request.New().
		SetRetry(g.Retry).
		SetProxyURLStr(g.Proxy).
		SetTimeoutSeconds(g.Timeout).
		GetBody(pageURL)
	if err != nil {
		return nil, err
	}

	// 解析HTML
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	var links []string

	// 遍历每个文章块，检测VIP或置顶
	doc.Find(".excerpt, .post-list .post-item").Each(func(i int, s *goquery.Selection) {
		// 检测VIP或置顶标识
		isVIPorSticky := false
		s.Find("span.sticky-icon").Each(func(j int, span *goquery.Selection) {
			text := strings.TrimSpace(span.Text())
			if text == "VIP" || text == "置顶" {
				isVIPorSticky = true
				return
			}
		})

		// 如果是VIP或置顶，跳过此文章
		if isVIPorSticky {
			return
		}

		// 提取文章链接
		s.Find("h2 a, a.post-title").Each(func(j int, a *goquery.Selection) {
			if href, exists := a.Attr("href"); exists {
				// 转换相对路径为绝对路径
				if strings.HasPrefix(href, "/") {
					href = g.baseURL + href
				}
				if strings.Contains(href, g.baseURL) && strings.HasSuffix(href, ".html") {
					if _, ok := seen[href]; !ok {
						seen[href] = struct{}{}
						links = append(links, href)
					}
				}
			}
		})
	})

	return links, nil
}

func (g *GnDownSpider) normalizeBaseURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "https://www.gndown.com"
	}

	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return strings.TrimSuffix(raw, "/")
	}

	return strings.TrimSuffix(fmt.Sprintf("%s://%s", u.Scheme, u.Host), "/")
}

// 采集单篇文章
func (g *GnDownSpider) fetchArticle(articleURL string) (*entity.Article, error) {
	g.ctx.Log.Info("正在采集文章", zap.String("url", articleURL))
	g.waitForRequestSlot(articleURL)

	// 发送HTTP请求
	body, err := request.New().
		SetRetry(g.Retry).
		SetProxyURLStr(g.Proxy).
		SetTimeoutSeconds(g.Timeout).
		GetBody(articleURL)
	if err != nil {
		return nil, err
	}

	// 解析HTML
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	article := &entity.Article{}

	// 提取标题
	title := strings.TrimSpace(doc.Find("h1").First().Text())
	if title == "" {
		return nil, errors.New("无法提取文章标题")
	}
	article.Title = title

	// 提取slug（gndown + 标题 hash）
	slug := g.buildSlug(title)
	if slug == "" {
		return nil, errors.New("无法提取文章slug")
	}
	article.Slug = slug

	// 提取内容
	content := g.extractContent(doc)
	if content == "" {
		return nil, errors.New("无法提取文章内容")
	}
	// 处理下载地址部分：移除下载地址之后的内容并提取下载链接
	processedContent, downloadLinks := g.ProcessDownloadSection(content)
	article.Content = processedContent

	// 如果提取到下载链接，添加到extends字段
	if len(downloadLinks) > 0 {
		g.ctx.Log.Info("提取到下载链接", zap.Int("count", len(downloadLinks)))
		for i, link := range downloadLinks {
			g.ctx.Log.Info("下载链接",
				zap.Int("index", i+1),
				zap.String("type", link["type"]),
				zap.String("url", link["url"]),
				zap.String("password", link["password"]))
		}
	}

	// 提取描述 - 从meta标签提取，避免页面结构信息
	article.Description = g.extractMetaDescription(doc)

	// 提取关键词
	article.Keywords = g.extractKeywords(doc)

	// 提取封面图片
	article.Thumbnail = g.extractThumbnail(doc, articleURL)
	g.ctx.Log.Info("Extracted thumbnail", zap.String("thumbnail", article.Thumbnail), zap.String("articleURL", articleURL))

	// 构建extends字段
	extends := g.buildExtends(doc, articleURL)

	if _, err := json.Marshal(extends); err == nil {
		// 转换为Extends格式存储
		extendsItems := make([]vo.ExtendsItem, 0)
		for key, value := range extends {
			extendsItems = append(extendsItems, vo.ExtendsItem{
				Key:   key,
				Value: value,
			})
		}
		article.Extends = extendsItems
	}

	// 将下载链接存储到res字段中（而不是extends字段）
	if len(downloadLinks) > 0 {
		// 将下载链接数组直接存储为res字段的值
		article.Res = vo.Extends{
			vo.ExtendsItem{
				Key:   "download_links",
				Value: downloadLinks, // 直接存储下载链接数组
			},
		}
	}

	// 提取发布时间
	article.CreateTime = g.extractTime(doc)

	// 获取分类ID
	categoryName := g.extractCategory(doc)
	g.ctx.Log.Info("提取到分类名称", zap.String("category", categoryName))

	if categoryName != "" {
		// 尝试精确匹配系统分类
		if cat, err := service.Category.GetByName(categoryName); err == nil && cat.ID > 0 {
			article.CategoryID = cat.ID
			g.ctx.Log.Info("分类匹配成功", zap.String("category", categoryName), zap.Int("category_id", cat.ID))
		} else {
			g.ctx.Log.Warn("分类匹配失败，使用默认分类", zap.String("category", categoryName), zap.Error(err))
			// 匹配失败，使用默认分类 "其他软件"
			if defaultCat, err := service.Category.GetByName("其他软件"); err == nil && defaultCat.ID > 0 {
				article.CategoryID = defaultCat.ID
				g.ctx.Log.Info("使用默认分类", zap.String("default_category", "其他软件"), zap.Int("category_id", defaultCat.ID))
			} else {
				g.ctx.Log.Error("默认分类也找不到", zap.Error(err))
			}
		}
	} else {
		g.ctx.Log.Warn("未提取到分类名称，使用默认分类")
		// 未提取到分类，也使用默认分类
		if defaultCat, err := service.Category.GetByName("其他软件"); err == nil && defaultCat.ID > 0 {
			article.CategoryID = defaultCat.ID
			g.ctx.Log.Info("使用默认分类", zap.String("default_category", "其他软件"), zap.Int("category_id", defaultCat.ID))
		}
	}

	return article, nil
}

// 生成slug: 基于标题的稳定hash，确保同一篇文章多次采集使用相同slug
func (g *GnDownSpider) buildSlug(title string) string {
	title = strings.TrimSpace(strings.ToLower(title))
	if title == "" {
		return ""
	}

	// 使用标题生成稳定hash，确保同一篇文章多次采集得到相同slug
	// 使用较短的MD5前缀来避免slug过长
	fullHash := cryptor.Md5String(title)
	return fullHash[:12] // 取前12位，足够唯一且较短
}

// 提取文章标题
func (g *GnDownSpider) extractTitle(doc *goquery.Document) string {
	return strings.TrimSpace(doc.Find("h1").First().Text())
}

// 提取文章内容
func (g *GnDownSpider) extractContent(doc *goquery.Document) string {
	// 精确的内容选择器，优先使用最具体的
	contentSelectors := []string{
		"article.article-content", // 最精准 - WordPress文章区域
		".article-content",        // 次级精确
		"article .entry-content",  // WordPress标准
		".entry-content",          // WordPress通用
		".post-content",           // 通用文章容器
	}

	for _, selector := range contentSelectors {
		if content := doc.Find(selector).First(); content.Length() > 0 {
			// 深度清理：移除所有非正文元素
			content.Find("script, style, .ads, .advertisement").Remove()
			content.Find("header, footer, .meta, .post-meta, .article-meta").Remove()
			content.Find(".sidebar, .widget, .nav, .navigation").Remove()
			content.Find(".related, .relates, .recommend, .post-actions").Remove()
			content.Find(".breadcrumbs, .crumbs, .breadcrumb").Remove()

			// 处理图片路径
			processedHTML := g.processContentImages(content)
			if processedHTML != "" {
				return processedHTML
			}
		}
	}

	return ""
}

// 提取文章描述
func (g *GnDownSpider) extractDescription(content string) string {
	// 移除HTML标签
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(content))
	if err != nil {
		return ""
	}

	text := doc.Text()
	if len(text) > 250 {
		return text[:250] + "..."
	}
	return text
}

// 转换相对路径为绝对路径
func (g *GnDownSpider) convertToAbsoluteURL(imageURL, baseURL string) string {
	imageURL = strings.TrimSpace(imageURL)

	// 已经是绝对路径
	if strings.HasPrefix(imageURL, "http://") || strings.HasPrefix(imageURL, "https://") {
		return imageURL
	}

	// 协议相对路径
	if strings.HasPrefix(imageURL, "//") {
		return "https:" + imageURL
	}

	// 根路径
	if strings.HasPrefix(imageURL, "/") {
		return g.baseURL + imageURL
	}

	// 相对路径（基于文章URL）
	if strings.Contains(imageURL, "./") || !strings.Contains(imageURL, "/") {
		articleBase := strings.TrimSuffix(g.baseURL, "/")
		return articleBase + "/" + strings.TrimPrefix(imageURL, "./")
	}

	return imageURL
}

// 提取发布时间
func (g *GnDownSpider) extractTime(doc *goquery.Document) int64 {
	// 尝试多个可能的时间选择器
	timeSelectors := []string{
		".time",
		".post-time",
		".entry-time",
		"time",
	}

	for _, selector := range timeSelectors {
		if timeText := doc.Find(selector).First().Text(); timeText != "" {
			// 尝试解析时间格式
			if t, err := time.Parse("2006-01-02 15:04:05", strings.TrimSpace(timeText)); err == nil {
				return t.Unix()
			}
			if t, err := time.Parse("2006-01-02", strings.TrimSpace(timeText)); err == nil {
				return t.Unix()
			}
		}
	}

	// 默认返回当前时间
	return time.Now().Unix()
}

// 从meta标签提取描述 - Twitter优先于OG优先于标准meta
func (g *GnDownSpider) extractMetaDescription(doc *goquery.Document) string {
	// 定义描述选择器优先级 - Twitter最优先
	descSelectors := []string{
		"meta[name='twitter:description']", // Twitter - 最优先
		"meta[property='og:description']",  // Open Graph - 次级
		"meta[name='description']",         // Standard meta - 三级
	}

	for i, selector := range descSelectors {
		if desc := doc.Find(selector).AttrOr("content", ""); desc != "" {
			g.ctx.Log.Info("Found description",
				zap.Int("priority", i+1),
				zap.String("selector", selector),
				zap.String("desc", desc[:min(len(desc), 50)]+"..."))
			return strings.TrimSpace(desc)
		}
	}

	// 兜底方案：正文第一段
	articleContent := doc.Find("article.article-content, .article-content, .entry-content").First()
	if articleContent.Length() > 0 {
		firstP := articleContent.Find("p").First()
		if firstP.Length() > 0 {
			text := strings.TrimSpace(firstP.Text())
			g.ctx.Log.Info("Using first paragraph as description", zap.String("text", text[:min(len(text), 100)]+"..."))
			if len(text) > 200 {
				return text[:200] + "..."
			}
			return text
		}
	}

	// 最终兜底：从整个内容提取（避免包含元信息）
	fallback := g.extractDescriptionFromContent(doc)
	g.ctx.Log.Warn("Using fallback description extraction", zap.String("desc", fallback[:min(len(fallback), 100)]+"..."))
	return fallback
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (g *GnDownSpider) waitForRequestSlot(targetURL string) {
	interval := time.Duration(g.effectiveRequestInterval()) * time.Second
	if interval <= 0 {
		return
	}
	if !g.lastRequestAt.IsZero() {
		nextAt := g.lastRequestAt.Add(interval)
		if wait := time.Until(nextAt); wait > 0 {
			g.ctx.Log.Debug("请求限速等待", zap.Duration("wait", wait), zap.String("url", targetURL))
			time.Sleep(wait)
		}
	}
	g.lastRequestAt = time.Now()
}

func (g *GnDownSpider) effectiveRequestInterval() int {
	if g.RequestInterval <= 0 {
		return 1
	}
	return g.RequestInterval
}

// 从正文提取描述的兜底方案
func (g *GnDownSpider) extractDescriptionFromContent(doc *goquery.Document) string {
	// 尝试从正文区域提取，避免整个页面的文本
	contentArea := doc.Find("article.article-content, .article-content, .entry-content, .post-content").First()
	if contentArea.Length() == 0 {
		return ""
	}

	// 只从正文段落提取，避免元信息
	paragraphs := contentArea.Find("p")
	if paragraphs.Length() > 0 {
		firstP := paragraphs.First()
		text := strings.TrimSpace(firstP.Text())
		if text != "" {
			if len(text) > 200 {
				return text[:200] + "..."
			}
			return text
		}
	}

	// 如果没有段落，从整个正文区域提取文本
	text := strings.TrimSpace(contentArea.Text())
	if len(text) > 200 {
		return text[:200] + "..."
	}
	return text
}

// 提取关键词
func (g *GnDownSpider) extractKeywords(doc *goquery.Document) string {
	var keywords []string

	// 第一步：从meta keywords提取
	if kw := doc.Find("meta[name='keywords']").AttrOr("content", ""); kw != "" {
		keywords = append(keywords, kw)
	}

	// 第二步：从OG标签提取
	if ogKw := doc.Find("meta[property='og:keywords']").AttrOr("content", ""); ogKw != "" {
		keywords = append(keywords, ogKw)
	}

	// 第三步：从分类标签提取
	doc.Find(".article-meta a[href*='category'], .meta a[href*='category'], .breadcrumbs a").Each(func(i int, s *goquery.Selection) {
		if cat := strings.TrimSpace(s.Text()); cat != "" && cat != "首页" && cat != "绿软小站" {
			keywords = append(keywords, cat)
		}
	})

	// 合并并去重
	if len(keywords) > 0 {
		// 简单的去重
		seen := make(map[string]bool)
		uniqueKeywords := []string{}
		for _, kw := range keywords {
			if !seen[kw] {
				seen[kw] = true
				uniqueKeywords = append(uniqueKeywords, kw)
			}
		}
		return strings.Join(uniqueKeywords, ", ")
	}

	return ""
}

// 提取封面图片
func (g *GnDownSpider) extractThumbnail(doc *goquery.Document, articleURL string) string {
	// 优先级顺序 - Twitter优先于OG
	selectors := []string{
		"meta[property='twitter:image']",     // Twitter Card - 最优先
		"meta[property='twitter:image:src']", // Twitter Card 备用
		"meta[name='twitter:image']",         // 兼容 name 写法
		"meta[name='twitter:image:src']",     // 兼容 name 写法
		"meta[property='og:image']",          // Open Graph - 次级
		"article.article-content img",        // 正文第一张图 - 兜底
	}

	for i, selector := range selectors {
		if img := doc.Find(selector).First(); img.Length() > 0 {
			var imageURL string
			if src, exists := img.Attr("content"); exists { // meta标签
				imageURL = src
				g.ctx.Log.Info("Found cover image",
					zap.Int("priority", i+1),
					zap.String("selector", selector),
					zap.String("url", imageURL))
			} else if dataSrc, exists := img.Attr("data-src"); exists {
				imageURL = dataSrc
				g.ctx.Log.Info("Found cover image",
					zap.Int("priority", i+1),
					zap.String("selector", selector),
					zap.String("url", imageURL))
			} else if dataLazySrc, exists := img.Attr("data-lazy-src"); exists {
				imageURL = dataLazySrc
				g.ctx.Log.Info("Found cover image",
					zap.Int("priority", i+1),
					zap.String("selector", selector),
					zap.String("url", imageURL))
			} else if src, exists := img.Attr("src"); exists {
				imageURL = src
				g.ctx.Log.Info("Found cover image",
					zap.Int("priority", i+1),
					zap.String("selector", selector),
					zap.String("url", imageURL))
			}

			if imageURL != "" {
				absoluteURL := g.convertToAbsoluteURL(imageURL, articleURL)
				g.ctx.Log.Info("Successfully extracted cover image",
					zap.Int("priority", i+1),
					zap.String("original", imageURL),
					zap.String("absolute", absoluteURL))
				return absoluteURL
			}
		}
	}

	g.ctx.Log.Info("No cover image found, using empty thumbnail", zap.String("articleURL", articleURL))
	return ""
}

// 构建extends字段
func (g *GnDownSpider) buildExtends(doc *goquery.Document, articleURL string) map[string]interface{} {
	extends := make(map[string]interface{})

	// 基础信息
	extends["source_url"] = articleURL

	// 提取分类信息
	if category := g.extractCategory(doc); category != "" {
		extends["category"] = category
	}

	// 提取语言信息
	if language := g.extractLanguage(doc); language != "" {
		extends["language"] = language
	}

	// 提取文件大小
	if fileSize, fileSizeBytes := g.extractFileSize(doc); fileSize != "" {
		extends["file_size"] = fileSize
		extends["file_size_bytes"] = fileSizeBytes
	}

	// ❌ 移除关键词提取 - 避免与article.Keywords字段重复
	// keywords已经存储在article.Keywords字段中，不需要在extends中重复

	// 提取版本信息
	if version := g.extractVersion(doc); version != "" {
		extends["version"] = version
	}

	// 提取原始分类路径
	if originalCategory := g.extractOriginalCategory(doc); originalCategory != "" {
		extends["original_category"] = originalCategory
	}

	return extends
}

// MarshalJSON 自定义JSON序列化，排除ctx字段
func (g *GnDownSpider) MarshalJSON() ([]byte, error) {
	type Alias GnDownSpider
	return json.Marshal(&struct {
		*Alias
	}{
		Alias: (*Alias)(g),
	})
}

// 提取分类信息
func (g *GnDownSpider) extractCategory(doc *goquery.Document) string {
	// 尝试多种选择器提取分类信息
	var category string

	// 方法1: 详情页分类提取 - 从span.item中查找分类链接
	doc.Find("span.item:contains('分类') a[rel='category tag']").Each(func(i int, s *goquery.Selection) {
		if category == "" {
			text := strings.TrimSpace(s.Text())
			if text != "" {
				category = text
			}
		}
	})

	// 方法2: 列表页分类提取 - 优先查找class为cat的a标签（最精确的匹配）
	if category == "" {
		doc.Find("a.cat").Each(func(i int, s *goquery.Selection) {
			if category == "" {
				// 获取文本内容，移除i标签等子元素的影响
				clone := s.Clone()
				clone.Find("i, img, svg").Remove() // 移除图标等元素
				text := strings.TrimSpace(clone.Text())
				if text != "" {
					category = text
				}
			}
		})
	}

	// 方法3: 从面包屑导航提取（优先选择倒数第二个，通常是分类）
	if category == "" {
		doc.Find(".breadcrumbs a").Each(func(i int, s *goquery.Selection) {
			if i == 1 { // 第二个链接通常是分类
				category = strings.TrimSpace(s.Text())
			}
		})
	}

	// 方法4: 如果面包屑没有，从文章meta信息中提取
	if category == "" {
		category = strings.TrimSpace(doc.Find(".article-meta .cat").Text())
	}

	// 方法5: 从meta中的category链接提取
	if category == "" {
		doc.Find(".article-meta a[href*='category']").Each(func(i int, s *goquery.Selection) {
			if category == "" {
				category = strings.TrimSpace(s.Text())
			}
		})
	}

	// 方法6: 从.meta区域提取
	if category == "" {
		category = strings.TrimSpace(doc.Find(".meta .cat").Text())
	}

	// 清理不需要的文字
	category = strings.TrimPrefix(category, "分类：")
	category = strings.TrimPrefix(category, "Category:")
	category = strings.TrimSpace(category)

	return category
}

// 提取语言信息
func (g *GnDownSpider) extractLanguage(doc *goquery.Document) string {
	// 从meta信息或页面标识提取
	language := doc.Find(".article-meta .pc:contains('语言'), .meta span:contains('语言'), .article-meta span:contains('语言')").Text()
	if language != "" {
		// 清理"语言："前缀
		language = strings.TrimPrefix(language, "语言：")
		language = strings.TrimPrefix(language, "Language:")
		return strings.TrimSpace(language)
	}

	// 从HTML lang属性提取
	if lang := doc.Find("html").AttrOr("lang", ""); lang != "" {
		// 转换语言代码为中文
		langMap := map[string]string{
			"zh-CN": "简体中文",
			"zh-TW": "繁體中文",
			"en":    "英文",
			"ja":    "日文",
			"ko":    "韩文",
			"ru":    "俄文",
			"de":    "德文",
			"fr":    "法文",
			"es":    "西班牙文",
		}
		if fullLang, ok := langMap[lang]; ok {
			return fullLang
		}
		return lang
	}

	// 从内容中识别语言
	metaLang := doc.Find("meta[name='language'], meta[property='og:locale']").AttrOr("content", "")
	if metaLang != "" {
		return metaLang
	}

	return "简体中文"
}

// 提取文件大小
func (g *GnDownSpider) extractFileSize(doc *goquery.Document) (string, int64) {
	// 从meta信息提取
	sizeText := doc.Find(".article-meta .pc:contains('大小'), .meta span:contains('大小'), .article-meta span:contains('大小')").Text()
	if sizeText != "" {
		// 清理"大小："前缀
		sizeText = strings.TrimPrefix(sizeText, "大小：")
		sizeText = strings.TrimPrefix(sizeText, "Size:")
		sizeText = strings.TrimSpace(sizeText)

		// 转换为字节数
		bytes := g.parseFileSize(sizeText)
		return sizeText, bytes
	}

	// 从内容中通过正则提取
	contentText := doc.Find("article.article-content, .article-content").Text()
	sizeRegex := `(?i)(\d+(?:\.\d+)?)\s*(KB|MB|GB|TB)`
	re := regexp.MustCompile(sizeRegex)
	if matches := re.FindStringSubmatch(contentText); len(matches) >= 3 {
		sizeStr := matches[1] + " " + matches[2]
		bytes := g.parseFileSize(sizeStr)
		return sizeStr, bytes
	}

	return "", 0
}

// 解析文件大小字符串为字节数
func (g *GnDownSpider) parseFileSize(sizeStr string) int64 {
	sizeStr = strings.TrimSpace(strings.ToUpper(sizeStr))

	units := map[string]int64{
		"B":  1,
		"KB": 1024,
		"MB": 1024 * 1024,
		"GB": 1024 * 1024 * 1024,
		"TB": 1024 * 1024 * 1024 * 1024,
	}

	for unit, multiplier := range units {
		if strings.HasSuffix(sizeStr, unit) {
			numeric := strings.TrimSuffix(sizeStr, unit)
			numeric = strings.TrimSpace(numeric)

			var size float64
			if _, err := fmt.Sscanf(numeric, "%f", &size); err == nil {
				return int64(size * float64(multiplier))
			}
		}
	}

	return 0
}

// 提取版本信息
func (g *GnDownSpider) extractVersion(doc *goquery.Document) string {
	// 从标题中提取版本号
	title := doc.Find("h1").First().Text()

	// 常见版本号格式正则
	versionRegexes := []string{
		`v(\d+\.?\d*)`,
		`(\d+\.\d+\.\d+)`,
		`(\d+\.\d+)`,
	}

	for _, regex := range versionRegexes {
		if re := regexp.MustCompile(regex); re != nil {
			if matches := re.FindStringSubmatch(title); len(matches) >= 2 {
				return matches[1]
			}
		}
	}

	return ""
}

// 提取原始分类路径
func (g *GnDownSpider) extractOriginalCategory(doc *goquery.Document) string {
	// 从分类链接中提取路径
	categoryLink := doc.Find(".article-meta a[href*='category'], .meta a[href*='category']").First()
	if href, exists := categoryLink.Attr("href"); exists {
		// 提取路径中的分类信息
		parts := strings.Split(href, "/")
		for i, part := range parts {
			if strings.Contains(part, "category") && i+1 < len(parts) {
				return strings.Join(parts[i+1:], "/")
			}
		}
	}
	return ""
}

// 处理内容中的图片
func (g *GnDownSpider) processContentImages(content *goquery.Selection) string {
	// 处理所有图片元素
	content.Find("img").Each(func(i int, img *goquery.Selection) {
		// 1. 处理data-src（真正的图片URL）- 最优先
		if dataSrc, exists := img.Attr("data-src"); exists && dataSrc != "" {
			img.SetAttr("src", g.convertToAbsoluteURL(dataSrc, ""))
			img.RemoveAttr("data-src")
			g.ctx.Log.Debug("Converted data-src to src", zap.String("url", dataSrc))
		}

		// 2. 处理已经是绝对路径的src（保持原样）
		if src, exists := img.Attr("src"); exists && src != "" {
			// 只转换相对路径，不处理data:协议的占位符
			if !strings.HasPrefix(src, "data:") && !strings.HasPrefix(src, "http") && !strings.HasPrefix(src, "//") {
				absoluteURL := g.convertToAbsoluteURL(src, "")
				img.SetAttr("src", absoluteURL)
				g.ctx.Log.Debug("Converted relative src", zap.String("url", src), zap.String("absolute", absoluteURL))
			} else if strings.HasPrefix(src, "//") {
				// 处理协议相对路径
				absoluteURL := "https:" + src
				img.SetAttr("src", absoluteURL)
				g.ctx.Log.Debug("Converted protocol-relative src", zap.String("url", src), zap.String("absolute", absoluteURL))
			}
		}

		// 3. 确保alt属性存在
		if alt, exists := img.Attr("alt"); !exists || alt == "" {
			img.SetAttr("alt", "article image")
		}

		// 4. 移除懒加载相关的class和属性
		img.RemoveClass("perfmatters-lazy")
		img.RemoveAttr("loading")
		img.RemoveAttr("decoding")
	})

	html, _ := content.Html()
	return strings.TrimSpace(html)
}

// 处理下载地址部分：移除下载地址之后的内容并提取下载链接
func (g *GnDownSpider) ProcessDownloadSection(content string) (processedContent string, downloadLinks []map[string]string) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(content))
	if err != nil {
		return content, nil
	}

	// 查找<h5>下载地址</h5>元素
	doc.Find("h5").Each(func(i int, h5 *goquery.Selection) {
		if strings.TrimSpace(h5.Text()) == "下载地址" {
			// 查找下一个兄弟节点
			sibling := h5.Next()

			// 保存下载链接信息
			var links []map[string]string

			// 先收集下载链接信息并移除节点
			current := sibling
			for current != nil && current.Length() > 0 {
				// 在当前节点中查找并收集链接信息
				current.Find("a").Each(func(j int, a *goquery.Selection) {
					linkURL, exists := a.Attr("href")
					if !exists {
						return
					}

					// 获取链接文本（实际显示的地址，通常是真实地址）
					linkText := strings.TrimSpace(a.Text())

					// 分析链接类型 - 简化逻辑：直接从a标签前面的文本提取类型
					linkType := ""

					// 获取a标签前面的直接文本节点内容 - 遍历父元素的直接子节点
					parent := a.Parent()
					nodes := parent.Contents()

					// 找到当前a标签的索引位置，然后查找前面的文本节点
					currentIndex := -1
					for i := range nodes.Nodes {
						if nodes.Eq(i).Get(0) == a.Get(0) {
							currentIndex = i
							break
						}
					}

					// 从当前a标签前面开始查找文本节点
					for i := currentIndex - 1; i >= 0; i-- {
						node := nodes.Eq(i)
						nodeName := goquery.NodeName(node)
						if nodeName == "#text" {
							textContent := strings.TrimSpace(node.Text())
							if textContent != "" {
								// 提取冒号前的文本作为类型
								if colonIndex := strings.Index(textContent, "："); colonIndex != -1 {
									// 使用中文冒号
									linkType = strings.TrimSpace(textContent[:colonIndex])
								} else if colonIndex := strings.Index(textContent, ":"); colonIndex != -1 {
									// 使用英文冒号
									linkType = strings.TrimSpace(textContent[:colonIndex])
								} else {
									// 如果没有冒号，取整个文本
									linkType = strings.TrimSpace(textContent)
								}
								break
							}
						}
					}

					// 优先使用链接文本作为真实URL（更准确）
					// 如果链接文本看起来像URL，优先使用它
					finalURL := linkURL // 默认使用href
					if isValidURL(linkText) {
						// 链接文本看起来像是真实地址，使用它
						finalURL = linkText
					} else if strings.Contains(linkURL, "/target/") || strings.Contains(linkURL, "gndown.com") {
						// href是跳转链接（如gndown.com/target/...），优先使用链接文本
						if isValidURL(linkText) {
							finalURL = linkText
						}
					}

					// 提取访问密码 - 查找a标签后是否还有文案（密码提示）
					password := ""

					// 获取父元素的所有子节点
					linkParent := a.Parent()
					linkChildren := linkParent.Contents()

					// 找到当前链接元素在父元素中的位置
					linkPosition := -1
					for i := range linkChildren.Nodes {
						if linkChildren.Eq(i).Get(0) == a.Get(0) {
							linkPosition = i
							break
						}
					}

					// 在找到链接位置后，检查链接后面是否有包含密码信息的文本节点
					if linkPosition != -1 {
						for j := linkPosition + 1; j < linkChildren.Length(); j++ {
							childNode := linkChildren.Eq(j)

							// 检查是否是文本节点
							nodeName := goquery.NodeName(childNode)
							if nodeName == "#text" {
								textContent := childNode.Text()
								// 查找密码模式
								if matches := regexp.MustCompile(`(?:访问密码|密码|提取码|pwd|passwd)[:：\s]*([a-zA-Z0-9]+)`).FindStringSubmatch(textContent); len(matches) > 1 {
									password = matches[1]
									break // 找到就退出
								}
							} else {
								// 如果遇到非文本节点（如<br/>等），则停止查找
								// 因为用户说"一行之内"，遇到换行标记就停止
								break
							}
						}
					}

					// 构建链接信息 - 只有当密码不为空时才包含密码字段
					linkInfo := map[string]string{
						"type": linkType,
						"url":  finalURL,
					}
					if password != "" {
						linkInfo["password"] = password
					}

					// 避免重复添加
					isDuplicate := false
					for _, existingLink := range links {
						if existingLink["url"] == finalURL {
							isDuplicate = true
							break
						}
					}
					if !isDuplicate {
						links = append(links, linkInfo)
					}
				})

				// 移动到下一个兄弟节点 before removing current (to avoid issues with Next())
				nextSibling := current.Next()

				// 移除当前节点（从下载地址开始的所有内容）
				current.Remove()

				// Move to next sibling
				current = nextSibling
			}

			// 也移除h5标签本身
			h5.Remove()

			// 保存找到的下载链接
			downloadLinks = links

			return // 只处理第一个匹配项
		}
	})

	// 返回处理后的内容
	processedContent, err = doc.Find("body").Html()
	if err != nil {
		return content, downloadLinks
	}

	return processedContent, downloadLinks
}

// isValidURL 检查字符串是否为有效的URL
func isValidURL(str string) bool {
	// 简单的URL格式检查，判断是否包含协议和域名
	lowerStr := strings.ToLower(str)
	if strings.Contains(lowerStr, "http://") || strings.Contains(lowerStr, "https://") {
		// 检查是否包含常见的域名格式
		if strings.Contains(lowerStr, ".") && (strings.Contains(lowerStr, "com") || strings.Contains(lowerStr, "cn") ||
			strings.Contains(lowerStr, "net") || strings.Contains(lowerStr, "org") ||
			strings.Contains(lowerStr, "io") || strings.Contains(lowerStr, "cc") ||
			strings.Contains(lowerStr, "baidu") || strings.Contains(lowerStr, "quark") ||
			strings.Contains(lowerStr, "lanzoub") || strings.Contains(lowerStr, "ctfile")) {
			return true
		}
	}
	return false
}
