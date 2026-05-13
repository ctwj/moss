package plugins

type SelectorMapping struct {
	ListItem    string `json:"list_item"`     // 列表页文章条目
	ArticleLink string `json:"article_link"`  // 文章链接
	Title       string `json:"title"`         // 文章标题
	Content     string `json:"content"`       // 正文内容
	Category    string `json:"category"`      // 分类提取
	Thumbnail   string `json:"thumbnail"`     // 封面图
	Time        string `json:"time"`          // 发布时间
	SkipMarker  string `json:"skip_marker"`   // VIP/置顶标记选择器
	SkipTexts   string `json:"skip_texts"`    // 需跳过的标记文本（逗号分隔）
	DownloadTag string `json:"download_tag"`  // 下载地址标签选择器
	Pagination  string `json:"pagination"`    // 翻页 URL 模板
}

type CategoryMapping map[string]int

type SiteConfig struct {
	Enabled         bool            `json:"enabled"`
	Name            string          `json:"name"`
	URL             string          `json:"url"`
	SiteID          string          `json:"site_id"`
	Selectors       SelectorMapping `json:"selectors"`
	CategoryMap     CategoryMapping `json:"category_map"`
	Proxy           string          `json:"proxy"`
	Timeout         int             `json:"timeout"`
	Retry           int             `json:"retry"`
	RequestInterval int             `json:"request_interval"`
	MaxPages        int             `json:"max_pages"`
	DownloadSection string          `json:"download_section"` // 下载区域标记文本
}

type SpiderResult struct {
	Collected int
	Skipped   int
	Errors    int
	Duration  int64
}
