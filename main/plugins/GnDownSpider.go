package plugins

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"go.uber.org/zap"
	"moss/domain/core/entity"
	"moss/domain/core/service"
	"moss/domain/core/vo"
	pluginEntity "moss/domain/support/entity"
	"moss/infrastructure/utils/request"
	"regexp"
	"strings"
	"time"
)

type GnDownSpider struct {
	SourceURL  string `json:"source_url"`  // 源站URL
	CategoryID int    `json:"category_id"` // Moss分类ID
	Interval   string `json:"interval"`    // 采集间隔 (默认 "@every 1h")
	Proxy      string `json:"proxy"`       // 代理地址
	Retry      int    `json:"retry"`       // 重试次数
	Timeout    int    `json:"timeout"`     // 超时秒数
	Limit      int    `json:"limit"`       // 每次采集数量限制
	LastUpdate int64  `json:"last_update"` // 上次更新时间戳

	ctx     *pluginEntity.Plugin
	baseURL string
}

func NewGnDownSpider() *GnDownSpider {
	return &GnDownSpider{
		SourceURL:  "https://www.gndown.com",
		Interval:   "@every 1h",
		Retry:      2,
		Timeout:    30,
		Limit:      20,
		LastUpdate: 0,
	}
}

func (g *GnDownSpider) Info() *pluginEntity.PluginInfo {
	return &pluginEntity.PluginInfo{
		ID:         "GnDownSpider",
		About:      "采集 gndown.com 文章",
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
	// 解析基础URL
	g.baseURL = g.SourceURL
	return nil
}

func (g *GnDownSpider) Run(ctx *pluginEntity.Plugin) error {
	g.ctx = ctx
	g.ctx.Log.Info("开始采集 gndown.com 文章...")

	collected := 0
	skipped := 0
	errors := 0

	// 从第1页开始采集，直到达到限制或没有更多内容
	for page := 1; page <= 10; page++ { // 最多采集10页
		if collected >= g.Limit {
			break
		}

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

		for _, link := range articleLinks {
			if collected >= g.Limit {
				break
			}

			// 检查是否已存在
			slug := g.extractSlug(link)
			exists, err := service.Article.ExistsSlug(slug)
			if err != nil {
				g.ctx.Log.Error("检查文章存在性失败", zap.String("slug", slug), zap.Error(err))
				errors++
				continue
			}

			if exists {
				g.ctx.Log.Debug("文章已存在，跳过", zap.String("slug", slug))
				skipped++
				continue
			}

			// 采集文章内容
			article, err := g.fetchArticle(link)
			if err != nil {
				g.ctx.Log.Error("采集文章内容失败", zap.String("url", link), zap.Error(err))
				errors++
				continue
			}

			// 创建文章
			article.CategoryID = g.CategoryID
			if err := service.Article.Create(article); err != nil {
				g.ctx.Log.Error("创建文章失败", zap.String("title", article.Title), zap.Error(err))
				errors++
				continue
			}

			collected++
			g.ctx.Log.Info("成功采集文章", zap.String("title", article.Title), zap.String("slug", article.Slug))
		}
	}

	// 更新最后采集时间
	g.LastUpdate = time.Now().Unix()

	g.ctx.Log.Info("采集完成",
		zap.Int("采集数量", collected),
		zap.Int("跳过数量", skipped),
		zap.Int("错误数量", errors),
		zap.Int64("耗时(秒)", time.Since(time.Unix(g.LastUpdate, 0)).Nanoseconds()/1e9),
	)

	return nil
}

// 获取文章链接列表
func (g *GnDownSpider) getArticleLinks(pageURL string) ([]string, error) {
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

	var links []string
	doc.Find(".excerpt h2 a, .post-list a").Each(func(i int, s *goquery.Selection) {
		if href, exists := s.Attr("href"); exists {
			// 转换相对路径为绝对路径
			if strings.HasPrefix(href, "/") {
				href = g.baseURL + href
			}
			if strings.Contains(href, g.baseURL) && strings.HasSuffix(href, ".html") {
				links = append(links, href)
			}
		}
	})

	return links, nil
}

// 采集单篇文章
func (g *GnDownSpider) fetchArticle(articleURL string) (*entity.Article, error) {
	g.ctx.Log.Info("正在采集文章", zap.String("url", articleURL))

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

	// 提取slug
	slug := g.extractSlug(articleURL)
	if slug == "" {
		return nil, errors.New("无法提取文章slug")
	}
	article.Slug = slug

	// 提取内容
	content := g.extractContent(doc)
	if content == "" {
		return nil, errors.New("无法提取文章内容")
	}
	article.Content = content

	// 提取描述 - 从meta标签提取，避免页面结构信息
	article.Description = g.extractMetaDescription(doc)

	// 提取关键词
	article.Keywords = g.extractKeywords(doc)

	// 提取封面图片
	article.Thumbnail = g.extractThumbnail(doc, articleURL)

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

	// 提取发布时间
	article.CreateTime = g.extractTime(doc)

	return article, nil
}

// 从URL提取slug
func (g *GnDownSpider) extractSlug(articleURL string) string {
	parts := strings.Split(articleURL, "/")
	if len(parts) > 0 {
		lastPart := parts[len(parts)-1]
		return strings.TrimSuffix(lastPart, ".html")
	}
	return ""
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

// 从meta标签提取描述
func (g *GnDownSpider) extractMetaDescription(doc *goquery.Document) string {
	// 第一步：尝试从meta description提取
	metaDesc := doc.Find("meta[name='description']").AttrOr("content", "")
	if metaDesc != "" {
		return strings.TrimSpace(metaDesc)
	}

	// 第二步：尝试从OG标签提取
	ogDesc := doc.Find("meta[property='og:description']").AttrOr("content", "")
	if ogDesc != "" {
		return strings.TrimSpace(ogDesc)
	}

	// 第三步：从正文第一段提取
	articleContent := doc.Find("article.article-content, .article-content, .entry-content").First()
	if articleContent.Length() > 0 {
		// 只从正文区域的第一段提取
		firstP := articleContent.Find("p").First()
		if firstP.Length() > 0 {
			text := strings.TrimSpace(firstP.Text())
			if len(text) > 200 {
				return text[:200] + "..."
			}
			return text
		}
	}

	// 兜底：从整个内容提取（避免包含元信息）
	return g.extractDescriptionFromContent(doc)
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
	// 优先级顺序
	selectors := []string{
		"meta[property='og:image']",         // Open Graph
		"meta[name='twitter:image']",        // Twitter Card
		"article.article-content img:first", // 正文第一张图
		".article-content img:first",        // 内容区域第一张图
		"img.wp-post-image",                 // WordPress特色图片
		"img.thumb",                         // 缩略图类
	}

	for _, selector := range selectors {
		if img := doc.Find(selector).First(); img.Length() > 0 {
			var imageURL string
			if src, exists := img.Attr("content"); exists { // meta标签
				imageURL = src
			} else if src, exists := img.Attr("src"); exists {
				imageURL = src
			} else if dataSrc, exists := img.Attr("data-src"); exists {
				imageURL = dataSrc
			}

			if imageURL != "" {
				return g.convertToAbsoluteURL(imageURL, articleURL)
			}
		}
	}

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

	// 提取关键词
	if keywords := g.extractKeywords(doc); keywords != "" {
		extends["keywords"] = keywords
	}

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
	// 从面包屑导航提取
	category := doc.Find(".breadcrumbs a:last, .article-meta .cat, .meta .cat, .article-meta a[href*='category']").Last().Text()
	category = strings.TrimSpace(category)

	// 清理不需要的文字
	category = strings.TrimPrefix(category, "分类：")
	category = strings.TrimPrefix(category, "Category:")

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

// 提取原始分类路径
// 处理内容中的图片
func (g *GnDownSpider) processContentImages(content *goquery.Selection) string {
	// 处理所有图片元素
	content.Find("img").Each(func(i int, img *goquery.Selection) {
		// 处理data-src（懒加载图片）
		if dataSrc, exists := img.Attr("data-src"); exists && dataSrc != "" {
			img.SetAttr("src", g.convertToAbsoluteURL(dataSrc, ""))
			img.RemoveAttr("data-src")
		}
		
		// 处理普通src
		if src, exists := img.Attr("src"); exists && src != "" {
			absoluteURL := g.convertToAbsoluteURL(src, "")
			img.SetAttr("src", absoluteURL)
		}
		
		// 确保alt属性存在
		if alt, exists := img.Attr("alt"); !exists || alt == "" {
			img.SetAttr("alt", "article image")
		}
	})
	
	html, _ := content.Html()
	return strings.TrimSpace(html)
}
