package plugins

import "strings"

type SitePreset struct {
	SiteID         string
	Name           string
	Domain         string
	Selectors      SelectorMapping
	CategoryMap    CategoryMapping
	DownloadSection string
}

var sitePresets = []SitePreset{
	{
		SiteID: "gndown",
		Name:   "绿软小站",
		Domain: "gndown.com",
		Selectors: SelectorMapping{
			ListItem:    ".excerpt, .post-list .post-item",
			ArticleLink: "h2 a, a.post-title",
			Title:       "h1",
			Content:     "article.article-content, .article-content, .entry-content",
			Thumbnail:   "meta[property='twitter:image'], meta[property='og:image']",
			Time:        ".time, time",
			SkipMarker:  "span.sticky-icon",
			SkipTexts:   "VIP,置顶",
			DownloadTag: "h5",
			Pagination:  "/page/{page}",
		},
		CategoryMap: CategoryMapping{
			"应用软件": 5, "系统工具": 6, "编程开发": 7, "安全": 8,
			"游戏娱乐": 9, "行业软件": 10, "多媒体": 12, "社交": 13,
			"网络工具": 15, "Mac应用": 16, "安卓软件": 17, "安卓游戏": 18,
			"其他软件": 19,
		},
		DownloadSection: "下载地址",
	},
	{
		SiteID: "ludown",
		Name:   "绿软部落",
		Domain: "ludown.com",
		Selectors: SelectorMapping{
			ListItem:    ".posts-wrapper .card, .post-card, .excerpt",
			ArticleLink: "h2 a, .card-title a, a.post-title",
			Title:       "h1",
			Content:     ".article-content, article.article-content, .entry-content",
			Thumbnail:   "meta[property='og:image']",
			Time:        ".time, .post-time, time",
			SkipMarker:  "span.sticky-icon, .sticky-post",
			SkipTexts:   "VIP,置顶",
			DownloadTag: "h5, h4",
			Pagination:  "/page/{page}",
		},
		CategoryMap: CategoryMapping{
			"电脑软件": 5, "系统工具": 6, "编程开发": 7, "安全": 8,
			"视频播放": 12, "图形设计": 12, "办公软件": 5, "录像软件": 12,
			"编辑工具": 5, "Mac软件": 16, "安卓软件": 17, "其他软件": 19,
		},
		DownloadSection: "下载地址",
	},
	{
		SiteID: "wycad",
		Name:   "无忧软件网",
		Domain: "wycad.com",
		Selectors: SelectorMapping{
			ListItem:    ".post-item, .excerpt, article.post",
			ArticleLink: "h2 a, a.post-title",
			Title:       "h1",
			Content:     ".entry-content, article.article-content, .article-content",
			Thumbnail:   "meta[property='og:image']",
			Time:        ".time, .post-time, time",
			SkipMarker:  "span.sticky-icon",
			SkipTexts:   "VIP,置顶",
			DownloadTag: "h3, h4",
			Pagination:  "/page/{page}",
		},
		CategoryMap: CategoryMapping{
			"办公软件": 5, "图形图像": 12, "实用软件": 5, "媒体工具": 12,
			"网页浏览": 15, "上传下载": 15, "设计软件": 12, "安全软件": 8,
			"系统工具": 6, "聊天软件": 13, "编程开发": 7, "安卓软件": 17,
			"Mac软件": 16, "IOS软件": 20, "TV软件": 19, "其他软件": 19,
		},
		DownloadSection: "下载",
	},
	{
		SiteID: "wuleba",
		Name:   "吾乐吧软件站",
		Domain: "wuleba.com",
		Selectors: SelectorMapping{
			ListItem:    ".post-list .post-item, .excerpt",
			ArticleLink: "h2 a, a.post-title",
			Title:       "h1",
			Content:     ".article-content, article.article-content, .entry-content",
			Thumbnail:   "meta[property='og:image']",
			Time:        ".time, .post-time, time",
			SkipMarker:  ".sticky, .sticky-post",
			SkipTexts:   "置顶",
			DownloadTag: "h5, h4",
			Pagination:  "/page/{page}",
		},
		CategoryMap: CategoryMapping{
			"Windows绿色软件": 5, "安卓绿色软件": 17, "电视TV软件": 19,
			"编程开发工具": 7, "教程资源": 19, "其他软件": 19,
		},
		DownloadSection: "下载地址",
	},
	{
		SiteID: "lr08",
		Name:   "08绿软站",
		Domain: "08lr.cn",
		Selectors: SelectorMapping{
			ListItem:    ".soft-list li, .soft-list a, .list-item",
			ArticleLink: "a, h2 a",
			Title:       "h1, .soft-title",
			Content:     ".soft-content, .article-content, .entry-content, #content",
			Thumbnail:   "meta[property='og:image']",
			Time:        ".time, .soft-time, time",
			SkipMarker:  "",
			SkipTexts:   "",
			DownloadTag: "h3, h4, .download-title",
			Pagination:  "/page/{page}",
		},
		CategoryMap: CategoryMapping{
			"绿色小软件": 5, "实用小工具": 5, "电子书": 19, "常用软件": 5,
			"转换软件": 5, "维护检测": 6, "注册破解": 19, "服务器软件": 19,
		},
		DownloadSection: "下载",
	},
}

func findPreset(url string) *SitePreset {
	for i := range sitePresets {
		if strings.Contains(url, sitePresets[i].Domain) {
			return &sitePresets[i]
		}
	}
	return nil
}
