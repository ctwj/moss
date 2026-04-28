package plugins

import (
	"fmt"
	"go.uber.org/zap"
	appService "moss/application/service"
	"moss/domain/config"
	"moss/domain/core/entity"
	"moss/domain/core/service"
	pluginEntity "moss/domain/support/entity"
	"moss/infrastructure/support/cache"
)

type PreBuildArticleCache struct {
	EnableOnCreate bool `json:"enable_on_create"` // 创建时执行
	EnableOnUpdate bool `json:"enable_on_update"` // 更新时执行

	ctx *pluginEntity.Plugin
}

func NewPreBuildArticleCache() *PreBuildArticleCache {
	return &PreBuildArticleCache{}
}

func (d *PreBuildArticleCache) Info() *pluginEntity.PluginInfo {
	return &pluginEntity.PluginInfo{
		ID:    "PreBuildArticleCache",
		About: "pre build article cache when created or updated",
	}
}

func (d *PreBuildArticleCache) Load(ctx *pluginEntity.Plugin) error {
	d.ctx = ctx
	service.Article.AddCreateAfterEvents(d)
	service.Article.AddUpdateAfterEvents(d)
	return nil
}

func (d *PreBuildArticleCache) ArticleCreateAfter(item *entity.Article) {
	if d.EnableOnCreate {
		// Delete any existing cache first
		d.invalidateCache(item)
		d.build(item, "create")
	}
	// Invalidate home page cache when new article is published
	if item.Status {
		d.invalidateHomePageCache()
	}
}

func (d *PreBuildArticleCache) ArticleUpdateAfter(item *entity.Article) {
	// Always invalidate cache on update to ensure consistency
	d.invalidateCache(item)
	if d.EnableOnUpdate {
		d.build(item, "update")
	}
	// Invalidate home page cache when article status changes
	if item.Status {
		d.invalidateHomePageCache()
	}
}

// invalidateCache deletes the cached article page
func (d *PreBuildArticleCache) invalidateCache(item *entity.Article) {
	if !config.Config.Cache.Enable {
		return
	}
	option := config.Config.Cache.GetOption("article")
	if option == nil || !option.Enable {
		return
	}
	if err := cache.InvalidateArticleCache(item.URL()); err != nil {
		d.ctx.Log.Warn("failed to invalidate article cache", zap.Error(err), zap.Int("id", item.ID))
	}
}

// invalidateHomePageCache clears the home page cache
func (d *PreBuildArticleCache) invalidateHomePageCache() {
	if !config.Config.Cache.Enable {
		return
	}
	option := config.Config.Cache.GetOption("home")
	if option == nil || !option.Enable {
		return
	}
	if err := cache.InvalidateHomePageCache(); err != nil {
		d.ctx.Log.Warn("failed to invalidate home page cache", zap.Error(err))
	}
}

func (d *PreBuildArticleCache) build(item *entity.Article, action string) {
	if !config.Config.Cache.Enable {
		d.ctx.Log.Warn("cache config is disabled")
		return
	}

	option := config.Config.Cache.GetOption("article")
	if option == nil || !option.Enable {
		d.ctx.Log.Warn("article cache config is disabled")
		return
	}

	bytes, err := appService.Render.Article(item)
	if err != nil {
		d.ctx.Log.Error("render error", zap.Error(err), zap.Int("id", item.ID))
		return
	}

	err = cache.Set("article", item.URL(), bytes, option.TTL.Duration())
	if err != nil {
		d.ctx.Log.Error("set cache error", zap.Error(err), zap.Int("id", item.ID))
		return
	}

	d.ctx.Log.Info(fmt.Sprintf("id:%d build success!", item.ID),
		zap.String("action", action),
		zap.String("title", item.Title),
		zap.String("url", item.FullURL()))
}

func (d *PreBuildArticleCache) Run(ctx *pluginEntity.Plugin) (err error) {
	return nil
}
