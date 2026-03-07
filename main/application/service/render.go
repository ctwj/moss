package service

import (
	"errors"
	"fmt"
	"go.uber.org/zap"
	"moss/domain/config"
	"moss/domain/core/entity"
	coreCtx "moss/domain/core/repository/context"
	"moss/domain/core/service"
	"moss/infrastructure/support/log"
	"moss/infrastructure/support/template"
	"net/url"
	"path/filepath"
	"strconv"
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
	}
	list, err := service.Article.ListByKeyword(ctx, keyword)
	if err != nil {
		return nil, err
	}
	count, err := service.Article.CountByKeyword(keyword)
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
	articleMap["Content"] = item.Content
	articleMap["Extends"] = item.Extends
	articleMap["Res"] = item.Res

	// DEBUG: log extends data
	log.Info("article render extends", zap.Int("count", len(item.Extends)))
	for _, ext := range item.Extends {
		log.Info("article extend item", zap.String("key", ext.Key), zap.Any("value", ext.Value))
	}

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
