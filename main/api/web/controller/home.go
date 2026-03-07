package controller

import (
	"errors"
	"github.com/gofiber/fiber/v2"
	"github.com/panjf2000/ants/v2"
	appService "moss/application/service"
	"moss/domain/config"
	"moss/domain/core/service"
	supportService "moss/domain/support/service"
	"moss/infrastructure/general/message"
	"moss/infrastructure/support/log"
	"moss/infrastructure/support/template"
	"moss/plugins"
	"strings"
)

func HomeIndex(ctx *fiber.Ctx) error {
	b, err := appService.Render.Index()
	if err != nil {
		log.Error("index controller failed", log.Err(err))
		return ctx.SendStatus(500)
	}
	return ctx.Type("html", "utf-8").SendString(string(b))
}

func HomeSearch(ctx *fiber.Ctx) error {
	keyword := strings.TrimSpace(ctx.Query("keyword"))
	if keyword == "" {
		return ctx.Redirect("/", fiber.StatusTemporaryRedirect)
	}
	page := ctx.QueryInt("page", 1)
	if page <= 0 {
		page = 1
	}
	b, err := appService.Render.Search(keyword, page)
	if err != nil {
		log.Error("search controller failed", log.Err(err))
		return ctx.SendStatus(500)
	}
	return ctx.Type("html", "utf-8").SendString(string(b))
}

func HomeCategory(ctx *fiber.Ctx) error {
	slug := ctx.Params("slug")
	page, _ := ctx.ParamsInt("page", 1)
	if page == 0 {
		page = 1
	}
	// 超出最大页数限制
	if config.Config.Template.CategoryPageList.MaxPage > 0 && page > config.Config.Template.CategoryPageList.MaxPage {
		return ctx.SendStatus(404)
	}
	b, err := appService.Render.CategoryBySlug(slug, page)
	if err != nil {
		if errors.Is(err, message.ErrRecordNotFound) {
			return ctx.Next()
		}
		log.Error("category controller failed", log.Err(err))
		return ctx.SendStatus(500)
	}
	return ctx.Type("html", "utf-8").SendString(string(b))
}

func HomeTag(ctx *fiber.Ctx) error {
	slug := ctx.Params("slug")
	page, _ := ctx.ParamsInt("page", 1)
	if page == 0 {
		page = 1
	}
	// 限制最大页数
	if config.Config.Template.TagPageList.MaxPage > 0 && page > config.Config.Template.TagPageList.MaxPage {
		return ctx.SendStatus(404)
	}
	b, err := appService.Render.TagBySlug(slug, page)
	if err != nil {
		if errors.Is(err, message.ErrRecordNotFound) {
			return ctx.Next()
		}
		log.Error("tag controller failed", log.Err(err))
		return ctx.SendStatus(500)
	}

	return ctx.Type("html", "utf-8").SendString(string(b))
}

func HomeArticle(ctx *fiber.Ctx) error {
	slug := ctx.Params("slug")
	b, err := appService.Render.ArticleBySlug(slug)
	if err != nil {
		if errors.Is(err, message.ErrRecordNotFound) {
			return ctx.Next()
		}
		log.Error("article controller failed", log.Err(err))
		return ctx.SendStatus(500)
	}
	return ctx.Type("html", "utf-8").SendString(string(b))
}

func HomeArticleViews(ctx *fiber.Ctx) error {
	if !ctx.XHR() {
		return ctx.SendStatus(200)
	}
	if config.Config.More.ArticleViewsPool == 0 || viewsPool.Free() == 0 {
		return ctx.SendStatus(200)
	}
	if err := viewsPool.Invoke(ctx.Params("slug")); err != nil {
		log.Warn("article views put in pool failed", log.Err(err))
	}
	return ctx.SendStatus(200)
}

func HomeDownloadRedirect(ctx *fiber.Ctx) error {
	slug := ctx.Params("slug")
	downloadType := ctx.Query("type")

	if slug == "" || downloadType == "" {
		return ctx.SendStatus(400)
	}

	// ============ IP 下载限制检查（使用插件） ============
	// 获取下载限制插件
	pluginItem, err := supportService.Plugin.Get("download_limit")
	if err != nil {
		// 插件不存在或未加载，跳过限制检查
		log.Warn("download limit plugin not found, skip limit check", log.Err(err))
	} else if downloadLimit, ok := pluginItem.Entry.(*plugins.DownloadLimit); ok && downloadLimit.Enable {
		// 获取客户端 IP
		ip := getRequestIP(ctx)

		// 检查是否超过限制
		allowed, _ := downloadLimit.CheckLimit(ip)
		if !allowed {
			// 渲染限制提示页面
			b, err := template.Render("template/downloadLimit.html", template.Binds{
				Page: template.Page{
					Name: "downloadLimit",
					Path: ctx.Path(),
				},
			})
			if err != nil {
				log.Error("render download limit page failed", log.Err(err))
				return ctx.SendStatus(500)
			}
			return ctx.Type("html", "utf-8").Status(429).SendString(string(b))
		}

		// 增加下载计数
		if err := downloadLimit.Increment(ip); err != nil {
			log.Warn("increment download count failed", log.Err(err), log.String("ip", ip))
		}
	}
	// ============ 结束限制检查 ============

	// 获取文章
	article, err := service.Article.GetBySlug(slug)
	if err != nil {
		log.Warn("download redirect: article not found", log.String("slug", slug), log.Err(err))
		return ctx.SendStatus(404)
	}

	// 在 res 字段中查找对应类型的下载链接
	var downloadURL string
	for _, resItem := range article.Res {
		if resItem.Value != nil {
			// resItem.Value 是 []any 类型
			if links, ok := resItem.Value.([]any); ok {
				for _, link := range links {
					if linkMap, ok := link.(map[string]any); ok {
						if linkType, ok := linkMap["type"].(string); ok && linkType == downloadType {
							if url, ok := linkMap["url"].(string); ok {
								downloadURL = url
								break
							}
						}
					}
				}
			}
		}
		if downloadURL != "" {
			break
		}
	}

	if downloadURL == "" {
		log.Warn("download redirect: download link not found", log.String("slug", slug), log.String("type", downloadType))
		return ctx.SendStatus(404)
	}

	// 异步统计下载点击数
	if config.Config.More.ArticleViewsPool == 0 || downloadClickPool.Free() == 0 {
		go service.Article.UpdateViewsBySlug(slug, 1)
	} else {
		downloadClickPool.Invoke(slug)
	}

	// 重定向到实际下载地址
	return ctx.Redirect(downloadURL, fiber.StatusTemporaryRedirect)
}

var downloadClickPool, _ = ants.NewPoolWithFunc(config.Config.More.ArticleViewsPool, downloadClickUpdate)

func downloadClickUpdate(val any) {
	slug, ok := val.(string)
	if !ok {
		log.Warn("download click slug transform error")
		return
	}
	if err := service.Article.UpdateViewsBySlug(slug, 1); err != nil {
		log.Warn("download click update error", log.Err(err))
	}
}

var viewsPool, _ = ants.NewPoolWithFunc(config.Config.More.ArticleViewsPool, articleViewUpdate)

func articleViewUpdate(val any) {
	slug, ok := val.(string)
	if !ok {
		log.Warn("article slug transform error in views update")
		return
	}
	if err := service.Article.UpdateViewsBySlug(slug, 1); err != nil {
		log.Warn("article views update error", log.Err(err))
	}
}

func HomeNotFound(ctx *fiber.Ctx) error {
	b, err := template.Render("template/notFound.html", template.Binds{
		Page: template.Page{
			Name: "notFound",
			Path: ctx.Path(),
		},
	})
	if err != nil {
		return ctx.SendStatus(404)
	}
	return ctx.Type("html", "utf-8").Status(404).SendString(string(b))
}

// getRequestIP 获取客户端 IP 地址
func getRequestIP(ctx *fiber.Ctx) string {
	// Cloudflare 专用头（优先）
	if cfIP := ctx.Get("CF-Connecting-IP"); cfIP != "" {
		return cfIP
	}

	// 通用代理头
	for _, v := range config.Config.Router.ProxyHeader {
		if ip := ctx.Get(v); ip != "" {
			arr := strings.Split(ip, ",")
			if len(arr) > 0 && arr[0] != "" {
				return arr[0]
			}
		}
	}

	// 直接连接的 IP
	return ctx.IP()
}
