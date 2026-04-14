package widget

import (
	"encoding/json"
	"go.uber.org/zap"
	"math"
	"moss/domain/config"
	"moss/domain/config/entity"
	"moss/domain/core/aggregate"
	coreEntity "moss/domain/core/entity"
	"moss/domain/core/repository/context"
	"moss/domain/core/service"
	coreUtils "moss/domain/core/utils"
	"moss/infrastructure/general/constant"
	"moss/infrastructure/persistent/db"
	"moss/infrastructure/support/cache"
	"moss/infrastructure/support/log"
	"sort"
	"time"
)

type Widget struct {
}

func New() *Widget {
	return &Widget{}
}

func (*Widget) Head() string {
	return config.Config.Template.Head
}

func (*Widget) Footer() string {
	return config.Config.Template.Footer
}

func (*Widget) LogoURL() string {
	if config.Config.Template.Logo == "" {
		return ""
	}
	return constant.LogoFilePath
}

func (w *Widget) Carousel() (res []entity.TemplateCarousel) {
	if !config.Config.Template.EnableCarousel {
		return
	}
	return config.Config.Template.Carousel
}

// Menu 模板导航（带缓存，10分钟 TTL）
func (w *Widget) Menu() []aggregate.CategoryTree {
	cacheKey := "widget_menu"
	// 尝试从缓存获取
	if data, err := cache.Get("template", cacheKey); err == nil && len(data) > 0 {
		var res []aggregate.CategoryTree
		if json.Unmarshal(data, &res) == nil {
			return res
		}
	}
	// 缓存未命中，获取数据
	var items []coreEntity.Category
	var err error
	if len(config.Config.Template.Menu.Select) > 0 {
		// 根据选择调用的导航数据
		items, err = service.Category.ListByIds(context.NewContextWithComment(config.Config.Template.Menu.Limit, "", "Widget.Menu"), config.Config.Template.Menu.Select)
		items = coreUtils.SortByIds(items, config.Config.Template.Menu.Select) // 根据选择的ids排序
	} else {
		// 默认调用全部导航数据
		items, err = service.Category.List(context.NewContextWithComment(config.Config.Template.Menu.Limit, "", "Widget.Menu"))
	}
	if err != nil {
		log.Error("template widget error", zap.Error(err))
		return nil
	}
	result := coreUtils.MakeCategoryTree(coreUtils.CategoryEntityListToCategoryTreeList(items), 0)
	// 存入缓存
	if len(result) > 0 {
		if data, err := json.Marshal(result); err == nil {
			cache.Set("template", cacheKey, data, 10*time.Minute)
		}
	}
	return result
}

// Link 链接列表（带缓存，10分钟 TTL）
func (w *Widget) Link() (res []coreEntity.Link) {
	cacheKey := "widget_link"
	var err error
	// 尝试从缓存获取
	var data []byte
	if data, err = cache.Get("template", cacheKey); err == nil && len(data) > 0 {
		if json.Unmarshal(data, &res) == nil {
			return
		}
	}
	// 缓存未命中，获取数据
	res, err = service.Link.ListPublic(context.NewContextByComment("Widget.Link"))
	log.ErrorShortcut("template widget error", err)
	// 存入缓存
	if len(res) > 0 {
		if data, err = json.Marshal(res); err == nil {
			cache.Set("template", cacheKey, data, 10*time.Minute)
		}
	}
	return
}

// IndexList 首页列表（带缓存，5分钟 TTL）
func (w *Widget) IndexList() (res []coreEntity.ArticleBase) {
	cacheKey := "widget_index_list"
	// 尝试从缓存获取
	if data, err := cache.Get("template", cacheKey); err == nil && len(data) > 0 {
		if json.Unmarshal(data, &res) == nil {
			return
		}
	}
	// 缓存未命中，获取数据
	res = w.simpleList(config.Config.Template.IndexList)
	// 存入缓存
	if len(res) > 0 {
		if data, err := json.Marshal(res); err == nil {
			cache.Set("template", cacheKey, data, 5*time.Minute)
		}
	}
	return
}

// GlobalList 全局列表
func (w *Widget) GlobalList() (res []coreEntity.ArticleBase) {
	return w.simpleList(config.Config.Template.GlobalList)
}

// 调用简单的列表
func (w *Widget) simpleList(opt *entity.TemplateList) (res []coreEntity.ArticleBase) {
	if opt.Limit <= 0 {
		return
	}
	var err error
	var ctx = context.NewContextWithComment(opt.Limit, opt.Order, "Widget.simpleList")
	// 添加状态过滤，只显示已发布的文章
	ctx.Where = &context.Where{Field: "status", Operator: context.WhereOperatorEqualTrue}
	if len(opt.CategoryIds) > 0 {
		res, err = service.Article.ListByCategoryIds(ctx, opt.CategoryIds)
	} else {
		res, err = service.Article.List(ctx)
	}
	log.ErrorShortcut("template widget error", err)
	return
}

// Breadcrumb 面包屑 通过分类ID调用
func (w *Widget) Breadcrumb(categoryID int) (res []coreEntity.Category) {
	res, err := service.Category.GetWithAncestorsReverse(context.NewContextWithComment(config.Config.More.ViewAllCategoryLimit, "", "Widget.Breadcrumb"), categoryID)
	log.ErrorShortcut("template widget error", err)
	return
}

// TagCloud 标签云（带缓存，10分钟 TTL）
func (w *Widget) TagCloud() (res []coreEntity.Tag) {
	if config.Config.Template.TagCloud.Limit <= 0 {
		return
	}
	cacheKey := "widget_tag_cloud"
	var cacheErr error
	// 尝试从缓存获取
	var cacheData []byte
	if cacheData, cacheErr = cache.Get("template", cacheKey); cacheErr == nil && len(cacheData) > 0 {
		if json.Unmarshal(cacheData, &res) == nil {
			return
		}
	}

	var err error

	// 指定了标签ID列表时使用原有逻辑
	if len(config.Config.Template.TagCloud.Select) > 0 {
		var ctx = context.NewContextWithComment(config.Config.Template.TagCloud.Limit, config.Config.Template.TagCloud.Order, "Widget.TagCloud")
		res, err = service.Tag.ListByIds(ctx, config.Config.Template.TagCloud.Select)
		log.ErrorShortcut("template widget error", err)
		if len(res) == 0 {
			return
		}
		// 按文章数量降序排序
		type tagWithCount struct {
			tag    coreEntity.Tag
			count  int64
		}
		var tagsWithCount []tagWithCount
		for _, tag := range res {
			count, _ := service.Mapping.CountByTagID(tag.ID)
			tagsWithCount = append(tagsWithCount, tagWithCount{tag: tag, count: count})
		}
		// 降序排序
		sort.Slice(tagsWithCount, func(i, j int) bool {
			return tagsWithCount[i].count > tagsWithCount[j].count
		})
		// 转换回 Tag 切片
		res = make([]coreEntity.Tag, len(tagsWithCount))
		for i, tc := range tagsWithCount {
			res[i] = tc.tag
		}
		// 存入缓存
		if len(res) > 0 {
			if data, err := json.Marshal(res); err == nil {
				cache.Set("template", cacheKey, data, 10*time.Minute)
			}
		}
		return
	}
	// 使用一条 SQL 查询标签及其文章数量（按文章数量降序）
	var ctx = context.NewContextWithComment(config.Config.Template.TagCloud.Limit, config.Config.Template.TagCloud.Order, "Widget.TagCloud")
	tagsWithCount, err := service.Tag.ListWithArticleCount(ctx, true)
	log.ErrorShortcut("template widget error", err)
	if len(tagsWithCount) == 0 {
		return
	}

	// 转换回 Tag 切片
	res = make([]coreEntity.Tag, len(tagsWithCount))
	for i, tc := range tagsWithCount {
		res[i] = tc.Tag
	}
	// 存入缓存
	if len(res) > 0 {
		if data, err := json.Marshal(res); err == nil {
			cache.Set("template", cacheKey, data, 10*time.Minute)
		}
	}
	return
}

// PageListResult 分页查询列表结果
type PageListResult struct {
	List          any   // 数据列表
	ListLength    int   // 列表长度
	Count         int64 // 数据总数
	PageNumber    int   // 当前页码
	PageTotal     int   // 总页数
	ExistNextPage bool  // 是否存在下一页
	DisableCount  bool  // 是否禁用count
}

// CategoryPageList 分类页列表
func (w *Widget) CategoryPageList(categoryID, pageNumber int) (res PageListResult) {
	if pageNumber == 0 {
		pageNumber = 1
	}
	var (
		opt               = config.Config.Template.CategoryPageList
		fastOffsetMinPage = config.Config.More.FastOffsetMinPage // 加速分页查询时，最小分页数
		fastOffset        = fastOffsetMinPage > 0 && pageNumber > fastOffsetMinPage
	)
	// 获取当前分类及所有子分类的ID
	categoryIds := w.getAllCategoryIDs(categoryID)
	// 查询数据函数
	var listFun = func() (any, int) {
		ctx := &context.Context{Limit: opt.Limit, Order: opt.Order, Page: pageNumber, FastOffset: fastOffset, Comment: "Widget.CategoryPageList"}
		// 添加状态过滤，只显示已发布的文章
		ctx.Where = &context.Where{Field: "status", Operator: context.WhereOperatorEqualTrue}
		list, err := service.Article.ListByCategoryIdsWithDetail(ctx, categoryIds)
		log.ErrorShortcut("template widget error", err)
		if len(list) == 0 {
			return nil, 0
		}
		return list, len(list)
	}
	// 统计总数函数
	var countFun = func() (res int64) {
		// 使用原生SQL同时过滤分类ID和状态
		err := db.DB.Model(&coreEntity.ArticleBase{}).Where("category_id IN ? AND status = ?", categoryIds, true).Count(&res).Error
		log.ErrorShortcut("template widget error", err)
		return
	}
	return w.pageList(opt, pageNumber, listFun, countFun)
}

// getAllCategoryIDs 递归获取分类及其所有子分类的ID
func (w *Widget) getAllCategoryIDs(categoryID int) []int {
	ids := []int{categoryID}
	// 获取子分类
	children, err := service.Category.ListChildren(context.NewContextWithComment(1000, "", "Widget.getAllCategoryIDs"), categoryID)
	if err != nil {
		log.ErrorShortcut("template widget error", err)
		return ids
	}
	// 递归获取子分类的子分类
	for _, child := range children {
		ids = append(ids, w.getAllCategoryIDs(child.ID)...)
	}
	return ids
}

// TagPageList 标签页列表
func (w *Widget) TagPageList(tagID, pageNumber int) (res PageListResult) {
	// 查询数据函数
	var listFun = func() (any, int) {
		ctx := &context.Context{
			Limit:   config.Config.Template.TagPageList.Limit,
			Order:   "id desc",
			Page:    pageNumber,
			Comment: "Widget.TagPageList",
		}
		// 添加状态过滤，只显示已发布的文章
		ctx.Where = &context.Where{Field: "status", Operator: context.WhereOperatorEqualTrue}
		list, err := service.Article.ListByTagID(ctx, tagID)
		log.ErrorShortcut("template widget error", err)
		return list, len(list)
	}
	// 统计总数函数
	var countFun = func() (res int64) {
		// 使用原生SQL同时过滤标签ID和文章状态
		err := db.DB.Table("mapping_tag").
			Joins("INNER JOIN article ON mapping_tag.article_id = article.id").
			Where("mapping_tag.tag_id = ? AND article.status = ?", tagID, true).
			Count(&res).Error
		log.ErrorShortcut("template widget error", err)
		return
	}
	return w.pageList(config.Config.Template.TagPageList, pageNumber, listFun, countFun)
}

func (w *Widget) pageList(opt *entity.TemplateList, pageNumber int, listFun func() (any, int), countFun func() int64) (res PageListResult) {
	if pageNumber == 0 {
		pageNumber = 1
	}
	if opt.Limit <= 0 || (opt.MaxPage > 0 && pageNumber > opt.MaxPage) {
		return
	}
	res.PageNumber = pageNumber
	res.List, res.ListLength = listFun()
	res.DisableCount = opt.DisableCount
	if opt.DisableCount { // 如果禁用count 是否存在下一页取决于本页数量是否等于设定
		res.ExistNextPage = res.ListLength >= opt.Limit
		return
	}
	res.Count = countFun()
	res.PageTotal = w.computePageTotal(res.Count, opt.Limit, opt.MaxPage)
	res.ExistNextPage = res.PageTotal > res.PageNumber
	return
}

// 计算总页数
func (w *Widget) computePageTotal(count int64, limit, maxPage int) (res int) {
	if count == 0 || limit == 0 {
		return
	}
	res = int(math.Ceil(float64(count) / float64(limit)))
	if maxPage > 0 && res > maxPage {
		res = maxPage // 限制最大页
	}
	return
}
