package plugins

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/duke-git/lancet/v2/cryptor"
	"go.uber.org/zap"
	"moss/domain/core/entity"
	"moss/domain/core/vo"
	"moss/infrastructure/utils/request"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// --- T005: HTTP helpers ---

func normalizeBaseURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return strings.TrimSuffix(raw, "/")
	}
	return strings.TrimSuffix(fmt.Sprintf("%s://%s", u.Scheme, u.Host), "/")
}

func (u *UniversalSpider) getBody(pageURL string, site *SiteConfig) ([]byte, error) {
	timeout, retry, interval, proxy := u.resolveSiteConfig(site)
	u.waitForRequestSlot(pageURL, interval)
	return request.New().
		SetRetry(retry).
		SetProxyURLStr(proxy).
		SetTimeoutSeconds(timeout).
		GetBody(pageURL)
}

// --- T006: getArticleLinks ---

func (u *UniversalSpider) getArticleLinks(pageURL string, site *SiteConfig) ([]string, error) {
	body, err := u.getBody(pageURL, site)
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	baseURL := normalizeBaseURL(site.URL)
	sel := site.Selectors
	if sel.ListItem == "" {
		sel.ListItem = ".excerpt, .post-list .post-item"
	}
	if sel.ArticleLink == "" {
		sel.ArticleLink = "h2 a, a.post-title"
	}

	skipTexts := parseSkipTexts(sel.SkipTexts)
	skipMarker := sel.SkipMarker
	if skipMarker == "" && len(skipTexts) > 0 {
		skipMarker = "span.sticky-icon"
	}

	seen := make(map[string]struct{})
	var links []string

	doc.Find(sel.ListItem).Each(func(i int, s *goquery.Selection) {
		// Check VIP/sticky markers
		if skipMarker != "" && len(skipTexts) > 0 {
			isSkip := false
			s.Find(skipMarker).Each(func(j int, span *goquery.Selection) {
				text := strings.TrimSpace(span.Text())
				for _, skip := range skipTexts {
					if text == skip {
						isSkip = true
						return
					}
				}
			})
			if isSkip {
				return
			}
		}

		s.Find(sel.ArticleLink).Each(func(j int, a *goquery.Selection) {
			href, exists := a.Attr("href")
			if !exists {
				return
			}
			if strings.HasPrefix(href, "/") {
				href = baseURL + href
			}
			if strings.Contains(href, baseURL) && strings.HasSuffix(href, ".html") {
				if _, ok := seen[href]; !ok {
					seen[href] = struct{}{}
					links = append(links, href)
				}
			}
		})
	})

	return links, nil
}

func parseSkipTexts(text string) []string {
	if text == "" {
		return nil
	}
	var result []string
	for _, t := range strings.Split(text, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			result = append(result, t)
		}
	}
	return result
}

// --- T007: extractContent ---

func (u *UniversalSpider) extractContent(doc *goquery.Document, site *SiteConfig) string {
	sel := site.Selectors.Content
	if sel == "" {
		sel = "article.article-content, .article-content, .entry-content"
	}

	selectors := splitSelectors(sel)
	for _, selector := range selectors {
		content := doc.Find(selector).First()
		if content.Length() > 0 {
			// Clean non-content elements
			content.Find("script, style, .ads, .advertisement").Remove()
			content.Find("header, footer, .meta, .post-meta, .article-meta").Remove()
			content.Find(".sidebar, .widget, .nav, .navigation").Remove()
			content.Find(".related, .relates, .recommend, .post-actions").Remove()
			content.Find(".breadcrumbs, .crumbs, .breadcrumb").Remove()

			// Process images
			baseURL := normalizeBaseURL(site.URL)
			content.Find("img").Each(func(i int, img *goquery.Selection) {
				if dataSrc, exists := img.Attr("data-src"); exists && dataSrc != "" {
					img.SetAttr("src", convertToAbsoluteURL(dataSrc, baseURL))
					img.RemoveAttr("data-src")
				}
				if src, exists := img.Attr("src"); exists && src != "" {
					if !strings.HasPrefix(src, "data:") && !strings.HasPrefix(src, "http") {
						if strings.HasPrefix(src, "//") {
							img.SetAttr("src", "https:"+src)
						} else {
							img.SetAttr("src", convertToAbsoluteURL(src, baseURL))
						}
					}
				}
				if _, exists := img.Attr("alt"); !exists {
					img.SetAttr("alt", "article image")
				}
				img.RemoveAttr("loading")
				img.RemoveAttr("decoding")
			})

			html, _ := content.Html()
			if trimmed := strings.TrimSpace(html); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

func splitSelectors(sel string) []string {
	var result []string
	for _, s := range strings.Split(sel, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			result = append(result, s)
		}
	}
	return result
}

func convertToAbsoluteURL(imageURL, baseURL string) string {
	imageURL = strings.TrimSpace(imageURL)
	if strings.HasPrefix(imageURL, "http://") || strings.HasPrefix(imageURL, "https://") {
		return imageURL
	}
	if strings.HasPrefix(imageURL, "//") {
		return "https:" + imageURL
	}
	if strings.HasPrefix(imageURL, "/") {
		return baseURL + imageURL
	}
	return baseURL + "/" + strings.TrimPrefix(imageURL, "./")
}

// --- T008: extract helpers ---

func extractTitle(doc *goquery.Document) string {
	return strings.TrimSpace(doc.Find("h1").First().Text())
}

func (u *UniversalSpider) extractMetaDescription(doc *goquery.Document) string {
	selectors := []string{
		"meta[name='twitter:description']",
		"meta[property='og:description']",
		"meta[name='description']",
	}
	for _, sel := range selectors {
		if desc := doc.Find(sel).AttrOr("content", ""); desc != "" {
			return strings.TrimSpace(desc)
		}
	}
	// Fallback: first paragraph
	contentSel := "article.article-content, .article-content, .entry-content"
	if p := doc.Find(contentSel).First().Find("p").First(); p.Length() > 0 {
		text := strings.TrimSpace(p.Text())
		if len(text) > 250 {
			return text[:250] + "..."
		}
		return text
	}
	return ""
}

func (u *UniversalSpider) extractKeywords(doc *goquery.Document) string {
	var keywords []string
	if kw := doc.Find("meta[name='keywords']").AttrOr("content", ""); kw != "" {
		keywords = append(keywords, kw)
	}
	doc.Find(".article-meta a[href*='category'], .meta a[href*='category'], .breadcrumbs a").Each(func(i int, s *goquery.Selection) {
		if cat := strings.TrimSpace(s.Text()); cat != "" && cat != "首页" {
			keywords = append(keywords, cat)
		}
	})
	seen := make(map[string]bool)
	var unique []string
	for _, kw := range keywords {
		if !seen[kw] {
			seen[kw] = true
			unique = append(unique, kw)
		}
	}
	return strings.Join(unique, ", ")
}

func (u *UniversalSpider) extractTime(doc *goquery.Document) int64 {
	sel := ".time, .post-time, .entry-time, time"
	if timeText := doc.Find(sel).First().Text(); timeText != "" {
		timeText = strings.TrimSpace(timeText)
		if t, err := time.Parse("2006-01-02 15:04:05", timeText); err == nil {
			return t.Unix()
		}
		if t, err := time.Parse("2006-01-02", timeText); err == nil {
			return t.Unix()
		}
	}
	return time.Now().Unix()
}

func (u *UniversalSpider) extractThumbnail(doc *goquery.Document, articleURL string) string {
	selectors := []string{
		"meta[property='twitter:image']",
		"meta[property='twitter:image:src']",
		"meta[name='twitter:image']",
		"meta[property='og:image']",
	}
	for _, sel := range selectors {
		if img := doc.Find(sel).AttrOr("content", ""); img != "" {
			return convertToAbsoluteURL(img, normalizeBaseURL(articleURL))
		}
	}
	// Fallback: first image in content
	if img := doc.Find("article.article-content img, .article-content img, .entry-content img").First(); img.Length() > 0 {
		if src, exists := img.Attr("data-src"); exists && src != "" {
			return convertToAbsoluteURL(src, normalizeBaseURL(articleURL))
		}
		if src, exists := img.Attr("src"); exists && src != "" {
			return convertToAbsoluteURL(src, normalizeBaseURL(articleURL))
		}
	}
	return ""
}

// --- T009: ProcessDownloadSection ---

func (u *UniversalSpider) processDownloadSection(content string, site *SiteConfig) (processedContent string, downloadLinks []map[string]string) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(content))
	if err != nil {
		return content, nil
	}

	downloadTag := site.Selectors.DownloadTag
	if downloadTag == "" {
		downloadTag = "h5"
	}
	downloadText := site.DownloadSection
	if downloadText == "" {
		downloadText = "下载地址"
	}

	var links []map[string]string

	doc.Find(downloadTag).Each(func(i int, tag *goquery.Selection) {
		if !strings.Contains(strings.TrimSpace(tag.Text()), downloadText) {
			return
		}

		sibling := tag.Next()
		for sibling != nil && sibling.Length() > 0 {
			sibling.Find("a").Each(func(j int, a *goquery.Selection) {
				linkURL, exists := a.Attr("href")
				if !exists {
					return
				}
				linkText := strings.TrimSpace(a.Text())

				// Determine real URL
				finalURL := linkURL
				if isValidDownloadURL(linkText) {
					finalURL = linkText
				} else if isValidDownloadURL(linkURL) {
					finalURL = linkURL
				} else {
					return
				}

				// Extract cloud drive type
				linkType := detectCloudDriveType(finalURL, a, sibling)

				// Extract password
				password := extractPassword(a)

				linkInfo := map[string]string{"type": linkType, "url": finalURL}
				if password != "" {
					linkInfo["password"] = password
				}

				// Dedup
				for _, existing := range links {
					if existing["url"] == finalURL {
						return
					}
				}
				links = append(links, linkInfo)
			})

			next := sibling.Next()
			sibling.Remove()
			sibling = next
		}
		tag.Remove()
		return // Only process first match
	})

	processedContent, _ = doc.Find("body").Html()
	if processedContent == "" {
		processedContent = content
	}
	return processedContent, links
}

// --- T010: buildSlug ---

func buildSlug(siteID, title string) string {
	title = strings.TrimSpace(strings.ToLower(title))
	if title == "" {
		return ""
	}
	fullHash := cryptor.Md5String(siteID + ":" + title)
	return fullHash[:12]
}

// --- T011: buildExtends ---

func (u *UniversalSpider) buildExtends(doc *goquery.Document, articleURL string, site *SiteConfig) []vo.ExtendsItem {
	extends := []vo.ExtendsItem{
		{Key: "source_url", Value: articleURL},
	}

	if category := u.extractCategory(doc, site); category != "" {
		extends = append(extends, vo.ExtendsItem{Key: "category", Value: category})
	}

	if version := extractVersion(doc); version != "" {
		extends = append(extends, vo.ExtendsItem{Key: "version", Value: version})
	}

	return extends
}

func extractVersion(doc *goquery.Document) string {
	title := doc.Find("h1").First().Text()
	regexes := []string{`v(\d+\.?\d*)`, `(\d+\.\d+\.\d+)`, `(\d+\.\d+)`}
	for _, regex := range regexes {
		if re := regexp.MustCompile(regex); re != nil {
			if matches := re.FindStringSubmatch(title); len(matches) >= 2 {
				return matches[1]
			}
		}
	}
	return ""
}

// --- T012: isValidDownloadURL ---

func isValidDownloadURL(str string) bool {
	lower := strings.ToLower(str)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		return false
	}
	domains := []string{".com", ".cn", ".net", ".org", ".io", ".cc", ".me", ".pro", ".tv"}
	for _, d := range domains {
		if strings.Contains(lower, d) {
			return true
		}
	}
	specialDomains := []string{"baidu", "quark", "lanzou", "ctfile", "123"}
	for _, d := range specialDomains {
		if strings.Contains(lower, d) {
			return true
		}
	}
	return false
}

// --- T021: extractCategory (US3) ---

func (u *UniversalSpider) extractCategory(doc *goquery.Document, site *SiteConfig) string {
	var category string

	// Method 1: Detail page category from span.item
	doc.Find("span.item:contains('分类') a[rel='category tag']").Each(func(i int, s *goquery.Selection) {
		if category == "" {
			if text := strings.TrimSpace(s.Text()); text != "" {
				category = text
			}
		}
	})

	// Method 2: a.cat tag
	if category == "" {
		doc.Find("a.cat").Each(func(i int, s *goquery.Selection) {
			if category == "" {
				clone := s.Clone()
				clone.Find("i, img, svg").Remove()
				if text := strings.TrimSpace(clone.Text()); text != "" {
					category = text
				}
			}
		})
	}

	// Method 3: Breadcrumbs (second link)
	if category == "" {
		doc.Find(".breadcrumbs a").Each(func(i int, s *goquery.Selection) {
			if i == 1 {
				category = strings.TrimSpace(s.Text())
			}
		})
	}

	// Method 4: .article-meta
	if category == "" {
		category = strings.TrimSpace(doc.Find(".article-meta .cat").Text())
	}

	// Method 5: .article-meta category links
	if category == "" {
		doc.Find(".article-meta a[href*='category']").Each(func(i int, s *goquery.Selection) {
			if category == "" {
				category = strings.TrimSpace(s.Text())
			}
		})
	}

	// Method 6: .meta .cat
	if category == "" {
		category = strings.TrimSpace(doc.Find(".meta .cat").Text())
	}

	category = strings.TrimPrefix(category, "分类：")
	category = strings.TrimPrefix(category, "Category:")
	return strings.TrimSpace(category)
}

// --- T024-T025: Download helpers (US4) ---

func detectCloudDriveType(linkURL string, a, _ *goquery.Selection) string {
	lower := strings.ToLower(linkURL)

	// Try getting type from preceding text
	parent := a.Parent()
	nodes := parent.Contents()
	currentIndex := -1
	for i := range nodes.Nodes {
		if nodes.Eq(i).Get(0) == a.Get(0) {
			currentIndex = i
			break
		}
	}
	if currentIndex > 0 {
		for i := currentIndex - 1; i >= 0; i-- {
			node := nodes.Eq(i)
			if goquery.NodeName(node) == "#text" {
				text := strings.TrimSpace(node.Text())
				if colonIdx := strings.Index(text, "："); colonIdx != -1 {
					return strings.TrimSpace(text[:colonIdx])
				}
				if colonIdx := strings.Index(text, ":"); colonIdx != -1 {
					return strings.TrimSpace(text[:colonIdx])
				}
			}
		}
	}

	// Auto-detect from URL patterns
	typePatterns := []struct {
		pattern string
		name    string
	}{
		{"pan.baidu.com", "百度云"},
		{"pan.quark.cn", "夸克云"},
		{"lanzou", "蓝奏云"},
		{"lanzoub", "蓝奏云"},
		{"ctfile", "城通盘"},
		{"123912", "123盘"},
		{"aliyundrive", "阿里云盘"},
	}
	for _, p := range typePatterns {
		if strings.Contains(lower, p.pattern) {
			return p.name
		}
	}
	return ""
}

func extractPassword(a *goquery.Selection) string {
	parent := a.Parent()
	children := parent.Contents()
	position := -1
	for i := range children.Nodes {
		if children.Eq(i).Get(0) == a.Get(0) {
			position = i
			break
		}
	}
	if position == -1 {
		return ""
	}

	re := regexp.MustCompile(`(?:访问密码|密码|提取码|pwd|passwd)[:：\s]*([a-zA-Z0-9]+)`)
	for j := position + 1; j < children.Length(); j++ {
		child := children.Eq(j)
		if goquery.NodeName(child) == "#text" {
			if matches := re.FindStringSubmatch(child.Text()); len(matches) > 1 {
				return matches[1]
			}
		} else {
			break
		}
	}
	return ""
}

// --- T014: fetchArticle (US1) ---

func (u *UniversalSpider) fetchArticle(articleURL string, site *SiteConfig, baseURL string) (*entity.Article, error) {
	u.ctx.Log.Info("正在采集文章", zap.String("url", articleURL))

	body, err := u.getBody(articleURL, site)
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	article := &entity.Article{}

	// Title
	title := extractTitle(doc)
	if title == "" {
		return nil, errors.New("无法提取文章标题")
	}
	article.Title = title

	// Slug
	siteID := site.SiteID
	if siteID == "" {
		siteID = "universal"
	}
	slug := buildSlug(siteID, title)
	if slug == "" {
		return nil, errors.New("无法生成 slug")
	}
	article.Slug = slug

	// Content
	content := u.extractContent(doc, site)
	if content == "" {
		return nil, errors.New("无法提取文章内容")
	}

	// Process download section
	processedContent, downloadLinks := u.processDownloadSection(content, site)
	article.Content = processedContent

	// Description
	article.Description = u.extractMetaDescription(doc)

	// Keywords
	article.Keywords = u.extractKeywords(doc)

	// Thumbnail
	article.Thumbnail = u.extractThumbnail(doc, articleURL)

	// Extends
	extends := u.buildExtends(doc, articleURL, site)
	article.Extends = extends

	// Download links → Res field
	if len(downloadLinks) > 0 {
		article.Res = vo.Extends{
			vo.ExtendsItem{Key: "download_links", Value: downloadLinks},
		}
		u.ctx.Log.Info("提取到下载链接", zap.Int("count", len(downloadLinks)))
	}

	// Create time
	article.CreateTime = u.extractTime(doc)

	// Category
	categoryName := u.extractCategory(doc, site)
	if len(downloadLinks) == 0 {
		article.CategoryID = u.DefaultCategory
	} else if categoryName != "" {
		article.CategoryID = u.resolveCategoryID(categoryName, site)
		u.ctx.Log.Info("分类匹配", zap.String("category", categoryName), zap.Int("id", article.CategoryID))
	} else {
		article.CategoryID = u.DefaultCategory
	}

	return article, nil
}
