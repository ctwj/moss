package plugins

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"go.uber.org/zap"
	"golang.org/x/time/rate"

	"moss/domain/core/entity"
	"moss/domain/core/repository"
	"moss/domain/core/vo"
	pluginEntity "moss/domain/support/entity"
	"moss/infrastructure/support/cache"
	"moss/infrastructure/utils/request"
)

// FixEllipsisArticles 修复 "..." 标题与正文文章插件
// 受影响文章的标题/正文被 AI 坏响应覆盖为 "..." 占位文本（specs/003-fix-ellipsis-titles）。
// 本插件从随文保存的源站页面（extends.source_url）恢复真实标题与正文：
// 标题/正文/下载资源三者原子写回（单事务），正文图片转存本站图床，
// 恢复口径与 GnDownSpider 采集时一致（research.md D2），单篇失败不影响其他文章。
type FixEllipsisArticles struct {
	BatchSize           int    `json:"batch_size"`            // 单次执行处理的文章数量
	FetchTimeoutSeconds int    `json:"fetch_timeout_seconds"` // 抓取源页面/图片的超时时间（秒）
	RateLimitPerMinute  int    `json:"rate_limit_per_minute"` // 对源站的请求频率限制（次/分钟）
	DownRetry           int    `json:"down_retry"`            // 抓取/下载失败重试次数
	UserAgent           string `json:"user_agent"`            // 请求 UA（可选，留空使用默认）
	MinTitleChars       int    `json:"min_title_chars"`       // 标题有效性阈值：可读字符（字母/数字/汉字）最少个数
	MinContentChars     int    `json:"min_content_chars"`     // 正文有效性阈值：剥 HTML 后可读字符最少个数

	ctx     *pluginEntity.Plugin
	limiter *rate.Limiter
	fixer   *FixBrokenImages // 内部实例：复用其图片下载转存路径（transferImage，含 API 图床跟随与同名去重）
}

// 修复失败/跳过原因（contracts/api.md §4 日志格式）
const (
	ellipsisReasonNoPlaceholder     = "no_placeholder"
	ellipsisReasonNoSourceURL       = "no_source_url"
	ellipsisReasonSourceUnreachable = "source_unreachable"
	ellipsisReasonNoValidTitle      = "no_valid_title"
	ellipsisReasonNoValidContent    = "no_valid_content"
	ellipsisReasonTransferFailed    = "transfer_failed"
	ellipsisReasonUpdateFailed      = "update_failed"
)

func NewFixEllipsisArticles() *FixEllipsisArticles {
	return &FixEllipsisArticles{
		BatchSize:           50,
		FetchTimeoutSeconds: 15,
		RateLimitPerMinute:  30,
		DownRetry:           2,
		MinTitleChars:       4,
		MinContentChars:     30,
	}
}

func (p *FixEllipsisArticles) Info() *pluginEntity.PluginInfo {
	return &pluginEntity.PluginInfo{
		ID:         "FixEllipsisArticles",
		About:      "修复 \"...\" 标题与正文文章（从源站页面恢复真实标题与正文，转存图片、重建下载资源）",
		RunEnable:  true, // 允许手动执行（分批触发，已修复者自动脱离候选集）
		CronEnable: false,
	}
}

func (p *FixEllipsisArticles) Load(ctx *pluginEntity.Plugin) error {
	p.ctx = ctx
	return nil
}

func (p *FixEllipsisArticles) batchSize() int {
	if p.BatchSize <= 0 {
		return 50
	}
	return p.BatchSize
}

func (p *FixEllipsisArticles) timeout() time.Duration {
	if p.FetchTimeoutSeconds <= 0 {
		return 15 * time.Second
	}
	return time.Duration(p.FetchTimeoutSeconds) * time.Second
}

func (p *FixEllipsisArticles) minTitleChars() int {
	if p.MinTitleChars <= 0 {
		return defaultMinTitleChars
	}
	return p.MinTitleChars
}

func (p *FixEllipsisArticles) minContentChars() int {
	if p.MinContentChars <= 0 {
		return defaultMinContentChars
	}
	return p.MinContentChars
}

// waitLimit 请求前限频（对源站的页面与图片请求合计限速）
func (p *FixEllipsisArticles) waitLimit() {
	n := p.RateLimitPerMinute
	if n <= 0 {
		n = 30
	}
	if p.limiter == nil {
		p.limiter = rate.NewLimiter(rate.Every(time.Minute/time.Duration(n)), 1)
	}
	_ = p.limiter.Wait(context.Background())
}

// syncFixer 将本插件的网络配置同步到内部 FixBrokenImages 实例（转存图片时沿用同一套限频/超时/UA），
// 并解析已注册的 SaveArticleImages 实例以跟随其 API 图床配置
func (p *FixEllipsisArticles) syncFixer() {
	if p.fixer == nil {
		p.fixer = NewFixBrokenImages()
	}
	p.fixer.FetchTimeoutSeconds = p.FetchTimeoutSeconds
	p.fixer.DownRetry = p.DownRetry
	p.fixer.UserAgent = p.UserAgent
	p.fixer.RateLimitPerMinute = p.RateLimitPerMinute
	p.fixer.resolveSaveArticleImages()
	// transferImage 的批次内去重 map 由 FixBrokenImages.Run 初始化；
	// 本插件直接复用其 transferImage，需自行保证已初始化
	if p.fixer.downloadCache == nil {
		p.fixer.downloadCache = make(map[string]string)
	}
}

// fetch 获取远程内容（校验 HTTP 状态码，4xx/5xx 视为不可达）
func (p *FixEllipsisArticles) fetch(uri string) ([]byte, error) {
	p.waitLimit()
	r := request.New().SetTimeout(p.timeout())
	if p.DownRetry > 0 {
		r.SetRetry(p.DownRetry)
	}
	r.SetUserAgentMust(p.UserAgent)
	resp, err := r.Get(uri)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected http status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// Run 批量修复入口：取候选 → 逐篇 repairOne → 汇总日志
func (p *FixEllipsisArticles) Run(ctx *pluginEntity.Plugin) error {
	p.ctx = ctx
	p.syncFixer()
	p.ctx.Log.Info("开始修复 \"...\" 标题与正文文章...")

	items, err := repository.Article.ListArticlesWithPlaceholderTitle(p.batchSize())
	if err != nil {
		p.ctx.Log.Error("获取候选文章失败", zap.Error(err))
		return err
	}
	if len(items) == 0 {
		p.ctx.Log.Info("无待修复文章（summary processed=0 ok=0 fail=0 skipped=0）")
		return nil
	}

	var ok, fail, skip int
	for _, item := range items {
		switch p.repairOne(item) {
		case repairStatusOK:
			ok++
		case repairStatusFail:
			fail++
		default:
			skip++
		}
	}
	// 批末失效首页缓存（镜像 FixBrokenImages）
	if err := cache.InvalidateHomePageCache(); err != nil {
		p.ctx.Log.Warn("失效首页缓存失败", zap.Error(err))
	}

	p.ctx.Log.Info("修复完成",
		zap.Int("summary_processed", len(items)),
		zap.Int("summary_ok", ok),
		zap.Int("summary_fail", fail),
		zap.Int("summary_skipped", skip))
	return nil
}

func (p *FixEllipsisArticles) logOne(msg string, item *entity.Article, fields ...zap.Field) {
	if p.ctx == nil || p.ctx.Log == nil {
		return
	}
	fields = append(fields, zap.String("title", item.Title))
	p.ctx.Log.Info(msg, fields...)
}

// failReason 记录失败日志并返回失败原因（供 repairOneReason 返回）
func (p *FixEllipsisArticles) failReason(item *entity.Article, reason string, fields ...zap.Field) string {
	p.logOne(fmt.Sprintf("repair fail id=%d reason=%s", item.ID, reason), item, fields...)
	return reason
}

// repairOne 修复单篇文章，返回 ok/fail/skipped。
// 流程：幂等裁决 → 读数据源 → 抓源页 → 提取并校验标题/正文 → 图片转存 →
//
//	下载资源重建 → 原子写回（标题/正文/Res 单事务）→ 失效文章页缓存。
// 任一环节失败即整篇保持原状（不产生部分写入）。
func (p *FixEllipsisArticles) repairOne(item *entity.Article) string {
	status, _ := p.repairOneReason(item)
	return status
}

// repairOneReason 同 repairOne，额外返回原因（供日志与单测断言）
func (p *FixEllipsisArticles) repairOneReason(item *entity.Article) (status, reason string) {
	// ① 幂等裁决：标题已是有效标题 → 跳过（SQL 粗筛命中但 Go 校验器终审放行）
	if IsValidArticleTitleWithMin(item.Title, p.minTitleChars()) {
		p.logOne(fmt.Sprintf("repair skipped id=%d reason=%s", item.ID, ellipsisReasonNoPlaceholder), item)
		return repairStatusSkipped, ellipsisReasonNoPlaceholder
	}

	// ② 历史数据源（随文保存的源站页面地址）
	sourceURL, _ := item.Extends.Get("source_url").(string)
	if strings.TrimSpace(sourceURL) == "" {
		return repairStatusFail, p.failReason(item, ellipsisReasonNoSourceURL)
	}

	// ③ 抓取源页面
	body, err := p.fetch(sourceURL)
	if err != nil {
		return repairStatusFail, p.failReason(item, ellipsisReasonSourceUnreachable, zap.Error(err))
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return repairStatusFail, p.failReason(item, ellipsisReasonSourceUnreachable, zap.Error(err))
	}
	baseURL, err := url.Parse(sourceURL)
	if err != nil {
		return repairStatusFail, p.failReason(item, ellipsisReasonSourceUnreachable, zap.Error(err))
	}

	// ④ 提取并校验标题
	title := extractSourceTitle(doc)
	if !IsValidArticleTitleWithMin(title, p.minTitleChars()) {
		return repairStatusFail, p.failReason(item, ellipsisReasonNoValidTitle, zap.String("title_preview", truncatePreview(title)))
	}

	// ⑤ 提取并校验正文
	content := extractSourceContent(doc, baseURL)
	if !IsValidArticleContentWithMin(content, p.minContentChars()) {
		return repairStatusFail, p.failReason(item, ellipsisReasonNoValidContent, zap.Int("content_chars", CountAlphanumericHan(content)))
	}

	// ⑥ 正文图片转存本站图床（任一失败 → 整篇放弃，不写库）
	content, err = p.transferContentImages(content, sourceURL)
	if err != nil {
		return repairStatusFail, p.failReason(item, ellipsisReasonTransferFailed, zap.Error(err))
	}

	// ⑦ 下载资源重建（口径与采集一致：剥离「下载地址」区段并提取链接）
	spider := &GnDownSpider{}
	processedContent, links := spider.ProcessDownloadSection(content)
	// 区段剥离可能缩短正文，复核阈值
	if !IsValidArticleContentWithMin(processedContent, p.minContentChars()) {
		return repairStatusFail, p.failReason(item, ellipsisReasonNoValidContent, zap.Int("content_chars", CountAlphanumericHan(processedContent)))
	}
	res := buildDownloadRes(links)

	// ⑧ 原子写回（repository.Article.Update 单事务同时更新 article 与 article_detail）
	oldTitle := item.Title
	oldLen := len([]rune(item.Content))
	item.Title = title
	item.Content = processedContent
	item.Res = res
	if err := repository.Article.Update(item); err != nil {
		return repairStatusFail, p.failReason(item, ellipsisReasonUpdateFailed, zap.Error(err))
	}
	if err := cache.InvalidateArticleCache(item.URL()); err != nil {
		p.ctx.Log.Warn("失效文章页缓存失败", zap.Int("article_id", item.ID), zap.Error(err))
	}

	p.logOne(fmt.Sprintf("repair ok id=%d title_old=%q title_new=%q content_len=%d→%d downloads=%d",
		item.ID, oldTitle, title, oldLen, len([]rune(processedContent)), len(links)), item)
	return repairStatusOK, ""
}

// buildDownloadRes 由下载链接构建文章 Res（无链接时置空，杜绝「可下载无资源」）
func buildDownloadRes(links []map[string]string) vo.Extends {
	if len(links) == 0 {
		return vo.Extends{}
	}
	return vo.Extends{{Key: "download_links", Value: links}}
}

// transferContentImages 将正文中所有 http(s) 图片转存本站图床并替换地址（批次内同源去重由 fixer 承担）
func (p *FixEllipsisArticles) transferContentImages(contentHTML, referer string) (string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(contentHTML))
	if err != nil {
		return "", err
	}
	var firstErr error
	doc.Find("img").Each(func(_ int, sel *goquery.Selection) {
		src, ok := sel.Attr("src")
		if !ok || strings.TrimSpace(src) == "" {
			return
		}
		if !strings.HasPrefix(src, "http://") && !strings.HasPrefix(src, "https://") {
			return // data: 等协议跳过
		}
		newURL, err := p.fixer.transferImage(src, referer)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("transfer %s: %w", src, err)
			}
			return
		}
		sel.SetAttr("src", newURL)
	})
	if firstErr != nil {
		return "", firstErr
	}
	return doc.Find("body").Html()
}

// ---- 提取（纯函数，可单测；口径与 GnDownSpider 采集一致，research.md D2） ----

// extractSourceTitle 提取源页面主标题（h1 首文本）
func extractSourceTitle(doc *goquery.Document) string {
	return strings.TrimSpace(doc.Find("h1").First().Text())
}

// extractSourceContent 提取源页面正文（容器选择器与采集/002 一致；
// 移除非正文元素，img 的 data-src → src 并绝对化相对地址）
func extractSourceContent(doc *goquery.Document, baseURL *url.URL) string {
	for _, selector := range fixBrokenContentSelectors {
		c := doc.Find(selector).First()
		if c.Length() == 0 {
			continue
		}
		// 深度清理非正文元素（口径同 GnDownSpider.extractContent）
		c.Find("script, style, .ads, .advertisement").Remove()
		c.Find("header, footer, .meta, .post-meta, .article-meta").Remove()
		c.Find(".sidebar, .widget, .nav, .navigation").Remove()
		c.Find(".related, .relates, .recommend, .post-actions").Remove()
		c.Find(".breadcrumbs, .crumbs, .breadcrumb").Remove()

		// 图片地址处理：data-src 优先，相对地址绝对化（口径同 GnDownSpider.processContentImages）
		c.Find("img").Each(func(_ int, img *goquery.Selection) {
			if dataSrc, exists := img.Attr("data-src"); exists && strings.TrimSpace(dataSrc) != "" {
				img.SetAttr("src", absolutize(baseURL, dataSrc))
				img.RemoveAttr("data-src")
			}
			if src, exists := img.Attr("src"); exists && strings.TrimSpace(src) != "" {
				if !strings.HasPrefix(src, "data:") && !strings.HasPrefix(src, "http://") && !strings.HasPrefix(src, "https://") {
					img.SetAttr("src", absolutize(baseURL, src))
				} else if strings.HasPrefix(src, "//") {
					img.SetAttr("src", absolutize(baseURL, src))
				}
			}
			if alt, exists := img.Attr("alt"); !exists || strings.TrimSpace(alt) == "" {
				img.SetAttr("alt", "article image")
			}
			img.RemoveClass("perfmatters-lazy")
			img.RemoveAttr("loading")
			img.RemoveAttr("decoding")
		})

		html, _ := c.Html()
		html = strings.TrimSpace(html)
		if html != "" {
			return html
		}
	}
	return ""
}

// truncatePreview 日志用无效值预览（前 80 字符）
func truncatePreview(s string) string {
	r := []rune(s)
	if len(r) > 80 {
		return string(r[:80])
	}
	return s
}
