package service

import (
	"errors"
	"fmt"
	"moss/domain/config"
	"moss/domain/core/entity"
	coreCtx "moss/domain/core/repository/context"
	"moss/domain/core/service"
	"moss/infrastructure/persistent/db"
	"moss/infrastructure/support/template"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var Render = new(RenderService)

type RenderService struct {
}

func (r *RenderService) Index() ([]byte, error) {
	return template.Render("template/index.html", template.Binds{
		Page: template.Page{
			Name:        "index",
			Title:       config.Config.Site.Title,
			Keywords:    config.Config.Site.Keywords,
			Description: config.Config.Site.Description,
		},
	})
}

func (r *RenderService) Search(keyword string, page int) (_ []byte, err error) {
	limit := config.Config.Template.IndexList.Limit
	if limit <= 0 {
		limit = 30
	}
	if page <= 0 {
		page = 1
	}
	ctx := &coreCtx.Context{
		Limit:   limit,
		Order:   "id desc",
		Page:    page,
		Comment: "Render.Search",
		// 添加状态过滤，只搜索已发布的文章
		Where:   &coreCtx.Where{Field: "status", Operator: coreCtx.WhereOperatorEqualTrue},
	}
	list, err := service.Article.ListByKeyword(ctx, keyword)
	if err != nil {
		return nil, err
	}
	// 使用原生 SQL 进行统计，同时过滤 keyword 和 status
	like := "%" + keyword + "%"
	var count int64
	err = db.DB.Model(&entity.ArticleBase{}).
		Where("(title like ? or description like ?) and status = ?", like, like, true).
		Count(&count).Error
	if err != nil {
		return nil, err
	}
	pageTotal := computePageTotal(count, limit)
	data := &SearchPageData{
		Keyword:       keyword,
		List:          list,
		Count:         count,
		PageTotal:     pageTotal,
		ExistNextPage: pageTotal > 0 && page < pageTotal,
	}
	return template.Render("template/search.html", template.Binds{
		Page: template.Page{
			Name:        "search",
			Title:       "搜索：" + keyword + " - " + config.Config.Site.Name,
			Keywords:    keyword,
			Description: "搜索结果：" + keyword,
			PageNumber:  page,
		},
		Data: data,
	})
}

func (r *RenderService) TemplatePage(path string) ([]byte, error) {
	return template.Render(filepath.Join("page", path), template.Binds{
		Page: template.Page{
			Name: "page",
			Path: path,
		},
		Data: map[string]any{},
	})
}

func (r *RenderService) ArticleBySlug(slug string) (_ []byte, err error) {
	item, err := service.Article.GetBySlug(slug)
	if err != nil {
		return
	}
	// 检查文章状态，未发布的文章禁止访问
	if !item.Status {
		return nil, errors.New("article not published")
	}
	return r.Article(item)
}

func (r *RenderService) Article(item *entity.Article) (_ []byte, err error) {
	if item == nil {
		err = errors.New("item is nil")
		return
	}

	// Create a map with the original article and add the category
	articleMap := make(map[string]interface{})

	// Add the original article as a field
	articleMap["Article"] = *item

	// Get category if article has a category ID
	var category *entity.Category
	if item.CategoryID > 0 {
		category, err = service.Category.Get(item.CategoryID)
		if err != nil {
			// If category not found, continue without it
			category = nil
		}
	}
	articleMap["Category"] = category

	// Add individual fields so templates can access them directly
	articleMap["ID"] = item.ID
	articleMap["Slug"] = item.Slug
	articleMap["Title"] = item.Title
	articleMap["CreateTime"] = item.CreateTime
	articleMap["CreateTimeFormat"] = item.CreateTimeFormat()
	articleMap["CategoryID"] = item.CategoryID
	articleMap["Views"] = item.Views
	articleMap["Thumbnail"] = item.Thumbnail
	articleMap["Description"] = item.Description
	articleMap["Keywords"] = item.Keywords
	// 替换内容中的标签为链接
	articleMap["Content"] = r.replaceTagLinks(item.Content)
	articleMap["Extends"] = item.Extends
	articleMap["Res"] = item.Res
	// 下载暂停（版权下架）状态与正版引导 URL
	articleMap["DownloadPaused"] = item.ArticleBase.DownloadPaused
	articleMap["GenuineURL"] = item.ArticleBase.GenuineURL

	// 关键字拆分
	var keywordList []string
	if item.Keywords != "" {
		// 支持中英文逗号分隔
		keywords := strings.Split(item.Keywords, ",")
		for _, kw := range keywords {
			kw = strings.TrimSpace(kw)
			if kw != "" {
				keywordList = append(keywordList, kw)
			}
		}
		// 也尝试中文逗号
		if len(keywordList) == 0 {
			keywords = strings.Split(item.Keywords, "，")
			for _, kw := range keywords {
				kw = strings.TrimSpace(kw)
				if kw != "" {
					keywordList = append(keywordList, kw)
				}
			}
		}
	}
	articleMap["KeywordList"] = keywordList

	// 侧边栏信息：只提取 language, file_size, version
	type SidebarInfo struct {
		Key   string `json:"key"`
		Value string `json:"value"`
		Icon  string `json:"icon"`
		Label string `json:"label"`
	}
	var sidebarInfo []SidebarInfo
	sidebarKeys := map[string]struct {
		Icon  string
		Label string
	}{
		"language":  {"fas fa-globe", "语言"},
		"file_size": {"fas fa-hdd", "文件大小"},
		"version":   {"fas fa-code-branch", "版本"},
	}
	for _, ext := range item.Extends {
		if info, ok := sidebarKeys[ext.Key]; ok {
			valueStr := fmt.Sprintf("%v", ext.Value)
			sidebarInfo = append(sidebarInfo, SidebarInfo{
				Key:   ext.Key,
				Value: valueStr,
				Icon:  info.Icon,
				Label: info.Label,
			})
		}
	}
	articleMap["SidebarInfo"] = sidebarInfo

	// 处理下载资源
	// 1. 优先查找 saved 字段，如果存在且不为空，返回 saved 列表（移除 url）
	// 2. 否则查找 download_links 字段，返回列表（移除直链类型和 url）
	type DownloadItem struct {
		Type     string `json:"type"`
		Password string `json:"password,omitempty"`
		Slug     string `json:"slug"`
	}
	var downloadList []DownloadItem
	slug := item.Slug

	// 查找 saved 字段
	var savedValue []interface{}
	for _, res := range item.Res {
		if res.Key == "saved" {
			if arr, ok := res.Value.([]interface{}); ok && len(arr) > 0 {
				savedValue = arr
				break
			}
		}
	}

	if len(savedValue) > 0 {
		// 返回 saved 列表，移除 url
		for _, v := range savedValue {
			if m, ok := v.(map[string]interface{}); ok {
				downloadItem := DownloadItem{Slug: slug}
				if typeVal, ok := m["type"].(string); ok {
					downloadItem.Type = typeVal
				}
				if pwdVal, ok := m["password"].(string); ok {
					downloadItem.Password = pwdVal
				}
				if downloadItem.Type != "" {
					downloadList = append(downloadList, downloadItem)
				}
			}
		}
	} else {
		// 查找 download_links 字段
		for _, res := range item.Res {
			if res.Key == "download_links" {
				if arr, ok := res.Value.([]interface{}); ok {
					for _, v := range arr {
						if m, ok := v.(map[string]interface{}); ok {
							// 跳过直链类型
							if typeVal, ok := m["type"].(string); ok {
								if typeVal == "直链" {
									continue
								}
								downloadItem := DownloadItem{
									Type: typeVal,
									Slug: slug,
								}
								if pwdVal, ok := m["password"].(string); ok {
									downloadItem.Password = pwdVal
								}
								downloadList = append(downloadList, downloadItem)
							}
						}
					}
				}
				break
			}
		}
	}
	articleMap["DownloadList"] = downloadList

	return template.Render("template/article.html", template.Binds{
		Page: template.Page{
			Name:        "article",
			Title:       item.Title + " - " + config.Config.Site.Name,
			Keywords:    item.Keywords,
			Description: item.Description,
		},
		Data: articleMap,
	})
}

func (r *RenderService) CategoryBySlug(slug string, page int) (_ []byte, err error) {
	item, err := service.Category.GetBySlug(slug)
	if err != nil {
		return
	}
	return r.Category(item, page)
}

func (r *RenderService) Category(item *entity.Category, page int) (_ []byte, err error) {
	if item == nil {
		err = errors.New("item is nil")
		return
	}
	var pageTitle string
	if page > 1 {
		pageTitle = " - " + strconv.Itoa(page)
	}
	var title = item.Name
	if item.Title != "" {
		title = item.Title
	}
	return template.Render("template/category.html", template.Binds{
		Page: template.Page{
			Name:        "category",
			Title:       title + pageTitle + " - " + config.Config.Site.Name,
			Keywords:    item.Keywords,
			Description: item.Description,
			PageNumber:  page,
		},
		Data: item,
	})
}

func (r *RenderService) TagBySlug(slug string, page int) (_ []byte, err error) {
	item, err := service.Tag.GetBySlug(slug)
	if err != nil {
		return
	}
	return r.Tag(item, page)
}

func (r *RenderService) Tag(item *entity.Tag, page int) (_ []byte, err error) {
	if item == nil {
		err = errors.New("item is nil")
		return
	}
	var pageTitle string
	if page > 1 {
		pageTitle = " - " + strconv.Itoa(page)
	}
	var title = item.Name
	if item.Title != "" {
		title = item.Title
	}
	return template.Render("template/tag.html", template.Binds{
		Page: template.Page{
			Name:        "tag",
			Title:       title + pageTitle + " - " + config.Config.Site.Name,
			Keywords:    item.Keywords,
			Description: item.Description,
			PageNumber:  page,
		},
		Data: item,
	})
}

type SearchPageData struct {
	Keyword       string
	List          []entity.ArticleBase
	Count         int64
	PageTotal     int
	ExistNextPage bool
	DisableCount  bool
}

func (s *SearchPageData) PageURL(page int) string {
	q := url.QueryEscape(s.Keyword)
	if page <= 1 {
		return "/search?keyword=" + q
	}
	return fmt.Sprintf("/search?keyword=%s&page=%d", q, page)
}

func computePageTotal(count int64, limit int) int {
	if count <= 0 || limit <= 0 {
		return 0
	}
	total := int(count) / limit
	if int(count)%limit != 0 {
		total++
	}
	return total
}

// replaceTagLinks 将文章内容中的标签替换为链接
func (r *RenderService) replaceTagLinks(content string) string {
	if content == "" {
		return content
	}
	// 获取前100个标签
	tags, err := service.Tag.ListForAutoLink(100)
	if err != nil || len(tags) == 0 {
		return content
	}
	// 按名称长度降序排序，确保长词优先匹配
	sort.Slice(tags, func(i, j int) bool {
		return len(tags[i].Name) > len(tags[j].Name)
	})

	// 策略：一次性收集所有需要替换的位置，然后从后向前一次性替换
	// 这样避免多次替换导致位置索引错位

	// 找出所有HTML标签的位置
	tagPattern := regexp.MustCompile(`<[^>]+>`)
	htmlMatches := tagPattern.FindAllStringIndex(content, -1)

	// 找出所有 <a> 标签的范围（包括其内容）
	aTagPattern := regexp.MustCompile(`<a\s[^>]*>.*?</a>`)
	aTagMatches := aTagPattern.FindAllStringIndex(content, -1)

	// 构建一个函数检查某个位置是否在HTML标签内部（属性值等）
	isInHTMLTagAttr := func(pos, length int) bool {
		end := pos + length
		for _, m := range htmlMatches {
			if pos >= m[0] && end <= m[1] {
				return true
			}
		}
		return false
	}

	// 构建一个函数检查某个位置是否在 <a> 标签内容中（不应再创建链接）
	isInATagContent := func(pos, length int) bool {
		end := pos + length
		for _, m := range aTagMatches {
			if pos >= m[0] && end <= m[1] {
				return true
			}
		}
		return false
	}

	// 收集所有需要替换的位置 {start, end, link}
	type replaceItem struct {
		start int
		end   int
		link  string
	}
	var allReplacements []replaceItem

	// 对每个标签名收集替换位置
	for _, tag := range tags {
		if tag.Name == "" {
			continue
		}
		tagName := tag.Name
		link := fmt.Sprintf(`<a href="/tag/%s">%s</a>`, tag.Slug, tagName)

		// 找出该标签名在内容中的所有位置
		for i := 0; i <= len(content)-len(tagName); i++ {
			if content[i:i+len(tagName)] == tagName {
				// 检查是否是独立的单词（前后不是字母数字）
				beforeOK := i == 0 || !isAlphaNum(rune(content[i-1]))
				afterOK := i+len(tagName) >= len(content) || !isAlphaNum(rune(content[i+len(tagName)]))
				// 不在HTML标签属性中，也不在已有 <a> 标签内容中
				if beforeOK && afterOK && !isInHTMLTagAttr(i, len(tagName)) && !isInATagContent(i, len(tagName)) {
					allReplacements = append(allReplacements, replaceItem{
						start: i,
						end:   i + len(tagName),
						link:  link,
					})
				}
			}
		}
	}

	// 如果没有需要替换的，直接返回
	if len(allReplacements) == 0 {
		return content
	}

	// 去重：同一位置只替换一次（按起始位置去重，保留最长的匹配）
	// 由于已经按长度降序排序，先匹配的是最长的
	seen := make(map[int]bool)
	var uniqueReplacements []replaceItem
	for _, r := range allReplacements {
		if !seen[r.start] {
			seen[r.start] = true
			uniqueReplacements = append(uniqueReplacements, r)
		}
	}

	// 按位置从后向前排序
	sort.Slice(uniqueReplacements, func(i, j int) bool {
		return uniqueReplacements[i].start > uniqueReplacements[j].start
	})

	// 从后向前替换
	result := content
	for _, r := range uniqueReplacements {
		result = result[:r.start] + r.link + result[r.end:]
	}

	return result
}

// isAlphaNum 检查字符是否是字母或数字
func isAlphaNum(c rune) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}
