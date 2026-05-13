package plugins

import (
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"moss/domain/core/entity"
	"moss/domain/core/service"
	"moss/domain/core/vo"
	pluginEntity "moss/domain/support/entity"
	"strings"
	"time"
)

type UniversalSpider struct {
	Sites           []SiteConfig `json:"sites"`
	Interval        string       `json:"interval"`
	DefaultProxy    string       `json:"default_proxy"`
	DefaultTimeout  int          `json:"default_timeout"`
	DefaultRetry    int          `json:"default_retry"`
	DefaultInterval int          `json:"default_interval"`
	DefaultCategory int          `json:"default_category"`
	LastUpdate      int64        `json:"last_update"`

	ctx           *pluginEntity.Plugin
	lastRequestAt time.Time
}

func NewUniversalSpider() *UniversalSpider {
	return &UniversalSpider{
		Interval:        "@every 1h",
		DefaultTimeout:  30,
		DefaultRetry:    2,
		DefaultInterval: 1,
		DefaultCategory: 19,
		Sites: []SiteConfig{
			{
				Enabled: true,
				Name:    "绿软小站",
				URL:     "https://www.gndown.com",
				SiteID:  "gndown",
				MaxPages: 3,
			},
			{
				Enabled: true,
				Name:    "绿软部落",
				URL:     "https://www.ludown.com",
				SiteID:  "ludown",
				MaxPages: 3,
			},
			{
				Enabled: true,
				Name:    "无忧软件网",
				URL:     "https://www.wycad.com",
				SiteID:  "wycad",
				MaxPages: 3,
			},
			{
				Enabled: true,
				Name:    "吾乐吧软件站",
				URL:     "https://www.wuleba.com",
				SiteID:  "wuleba",
				MaxPages: 3,
			},
			{
				Enabled: true,
				Name:    "08绿软站",
				URL:     "https://www.08lr.cn",
				SiteID:  "lr08",
				MaxPages: 3,
			},
		},
	}
}

func (u *UniversalSpider) Info() *pluginEntity.PluginInfo {
	return &pluginEntity.PluginInfo{
		ID:         "UniversalSpider",
		About:      "通用多源采集插件",
		RunEnable:  true,
		CronEnable: true,
		PluginInfoPersistent: pluginEntity.PluginInfoPersistent{
			CronStart: false,
			CronExp:   u.Interval,
		},
	}
}

func (u *UniversalSpider) Load(ctx *pluginEntity.Plugin) error {
	u.ctx = ctx
	// Auto-fill presets for sites with empty selectors
	for i := range u.Sites {
		u.applyPreset(&u.Sites[i])
	}
	return nil
}

func (u *UniversalSpider) Run(ctx *pluginEntity.Plugin) error {
	u.ctx = ctx
	u.lastRequestAt = time.Time{}
	u.ctx.Log.Info("开始通用多源采集任务...")

	totalResult := SpiderResult{}
	startTime := time.Now()

	for i := range u.Sites {
		site := &u.Sites[i]
		if !site.Enabled {
			u.ctx.Log.Info("源站已禁用，跳过", zap.String("site", site.Name))
			continue
		}

		u.ctx.Log.Info("开始采集源站", zap.String("name", site.Name), zap.String("url", site.URL))
		siteStart := time.Now()

		result := u.collectSite(site)
		result.Duration = int64(time.Since(siteStart).Seconds())

		totalResult.Collected += result.Collected
		totalResult.Skipped += result.Skipped
		totalResult.Errors += result.Errors

		u.ctx.Log.Info("源站采集完成",
			zap.String("site", site.Name),
			zap.Int("采集", result.Collected),
			zap.Int("跳过", result.Skipped),
			zap.Int("错误", result.Errors),
			zap.Int64("耗时(秒)", result.Duration),
		)
	}

	u.LastUpdate = time.Now().Unix()
	totalResult.Duration = int64(time.Since(startTime).Seconds())

	u.ctx.Log.Info("全部采集任务完成",
		zap.Int("总采集", totalResult.Collected),
		zap.Int("总跳过", totalResult.Skipped),
		zap.Int("总错误", totalResult.Errors),
		zap.Int64("总耗时(秒)", totalResult.Duration),
	)

	return nil
}

func (u *UniversalSpider) collectSite(site *SiteConfig) SpiderResult {
	result := SpiderResult{}
	baseURL := normalizeBaseURL(site.URL)
	if baseURL == "" {
		u.ctx.Log.Error("源站 URL 无效", zap.String("url", site.URL))
		result.Errors++
		return result
	}

	for page := 1; ; page++ {
		maxPages := site.MaxPages
		if maxPages > 0 && page > maxPages {
			u.ctx.Log.Info("达到最大翻页数限制", zap.Int("max_pages", maxPages))
			break
		}

		pageURL := buildPageURL(baseURL, page, site.Selectors.Pagination)
		u.ctx.Log.Info("正在采集页面", zap.String("url", pageURL))

		articleLinks, err := u.getArticleLinks(pageURL, site)
		if err != nil {
			u.ctx.Log.Error("获取文章链接失败", zap.String("url", pageURL), zap.Error(err))
			result.Errors++
			continue
		}

		if len(articleLinks) == 0 {
			u.ctx.Log.Info("页面无文章，停止采集", zap.Int("page", page))
			break
		}

		pageCollected := 0
		pageSkipped := 0

		for _, link := range articleLinks {
			article, err := u.fetchArticle(link, site, baseURL)
			if err != nil {
				u.ctx.Log.Error("采集文章失败", zap.String("url", link), zap.Error(err))
				result.Errors++
				continue
			}

			exists, err := service.Article.ExistsSlug(article.Slug)
			if err != nil {
				u.ctx.Log.Error("检查 slug 失败", zap.String("slug", article.Slug), zap.Error(err))
				result.Errors++
				continue
			}
			if exists {
				u.ctx.Log.Debug("文章已存在，跳过", zap.String("slug", article.Slug))
				result.Skipped++
				pageSkipped++
				continue
			}

			if err := service.Article.Create(article); err != nil {
				u.ctx.Log.Error("创建文章失败", zap.String("title", article.Title), zap.Error(err))
				result.Errors++
				continue
			}

			result.Collected++
			pageCollected++
			u.ctx.Log.Info("成功采集文章", zap.String("title", article.Title), zap.String("slug", article.Slug))
		}

		if pageCollected == 0 && pageSkipped == len(articleLinks) {
			u.ctx.Log.Info("当前页全部已采集，停止翻页",
				zap.Int("page", page),
				zap.Int("total", len(articleLinks)))
			break
		}
	}

	return result
}

func (u *UniversalSpider) MarshalJSON() ([]byte, error) {
	type Alias UniversalSpider
	return json.Marshal(&struct{ *Alias }{Alias: (*Alias)(u)})
}

func (u *UniversalSpider) applyPreset(site *SiteConfig) {
	if site.URL == "" {
		return
	}
	preset := findPreset(site.URL)
	if preset == nil {
		return
	}

	// Auto-fill site_id from preset if empty
	if site.SiteID == "" {
		site.SiteID = preset.SiteID
	}
	if site.Name == "" {
		site.Name = preset.Name
	}

	// Auto-fill selectors if empty
	if site.Selectors.Content == "" {
		site.Selectors = preset.Selectors
	}
	if site.DownloadSection == "" {
		site.DownloadSection = preset.DownloadSection
	}

	// Auto-fill category map if empty
	if len(site.CategoryMap) == 0 {
		site.CategoryMap = preset.CategoryMap
	}
}

// resolveSiteConfig returns effective config with global defaults applied
func (u *UniversalSpider) resolveSiteConfig(site *SiteConfig) (timeout, retry, interval int, proxy string) {
	timeout = site.Timeout
	if timeout <= 0 {
		timeout = u.DefaultTimeout
	}
	retry = site.Retry
	if retry <= 0 {
		retry = u.DefaultRetry
	}
	interval = site.RequestInterval
	if interval <= 0 {
		interval = u.DefaultInterval
	}
	proxy = site.Proxy
	if proxy == "" {
		proxy = u.DefaultProxy
	}
	return
}

func (u *UniversalSpider) waitForRequestSlot(targetURL string, interval int) {
	if interval <= 0 {
		interval = 1
	}
	dur := time.Duration(interval) * time.Second
	if !u.lastRequestAt.IsZero() {
		if wait := time.Until(u.lastRequestAt.Add(dur)); wait > 0 {
			u.ctx.Log.Debug("请求限速等待", zap.Duration("wait", wait), zap.String("url", targetURL))
			time.Sleep(wait)
		}
	}
	u.lastRequestAt = time.Now()
}

func (u *UniversalSpider) resolveCategoryID(categoryName string, site *SiteConfig) int {
	if categoryName == "" {
		return u.DefaultCategory
	}

	// 1. Try CategoryMap
	if site.CategoryMap != nil {
		if id, ok := site.CategoryMap[categoryName]; ok {
			return id
		}
	}

	// 2. Try service lookup by name
	if cat, err := service.Category.GetByName(categoryName); err == nil && cat.ID > 0 {
		return cat.ID
	}

	// 3. Fallback to default
	return u.DefaultCategory
}

func (u *UniversalSpider) storeArticle(article *entity.Article, downloadLinks []map[string]string) {
	if len(downloadLinks) > 0 {
		article.Res = vo.Extends{
			vo.ExtendsItem{
				Key:   "download_links",
				Value: downloadLinks,
			},
		}
	} else {
		article.CategoryID = u.DefaultCategory
	}
}

func buildPageURL(baseURL string, page int, paginationPattern string) string {
	if page <= 1 {
		return baseURL
	}
	if paginationPattern == "" {
		return fmt.Sprintf("%s/page/%d", baseURL, page)
	}
	return strings.ReplaceAll(paginationPattern, "{page}", fmt.Sprintf("%d", page))
}
