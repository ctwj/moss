package plugins

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/duke-git/lancet/v2/cryptor"
	"github.com/h2non/filetype"
	"github.com/h2non/filetype/types"
	"go.uber.org/zap"
	"golang.org/x/time/rate"

	"moss/domain/config"
	"moss/domain/core/entity"
	"moss/domain/core/repository"
	pluginEntity "moss/domain/support/entity"
	"moss/infrastructure/persistent/storage"
	"moss/infrastructure/support/cache"
	"moss/infrastructure/support/upload"
	"moss/infrastructure/utils/request"
)

// FixBrokenImages 修复失效图床图片插件
// 从文章保存的历史数据源（源站页面）恢复缩略图与正文图片，并转存到当前上传图床（Upload.Domain）。
// 修复采用「全有或全无」策略：单篇文章的映射与转存全部成功才写库，任何失败保持原状。
type FixBrokenImages struct {
	BrokenDomain        string `json:"broken_domain"`        // 失效图床域名（如 image.08rj.com）
	BatchSize           int    `json:"batch_size"`           // 单次执行处理的文章数量
	FetchTimeoutSeconds int    `json:"fetch_timeout_seconds"` // 抓取源页面/图片的超时时间（秒）
	RateLimitPerMinute  int    `json:"rate_limit_per_minute"` // 对源站的请求频率限制（次/分钟）
	DownRetry           int    `json:"down_retry"`            // 下载失败重试次数
	DownReferer         string `json:"down_referer"`          // 下载 referer（可选，留空使用源页面地址）
	UserAgent           string `json:"user_agent"`            // 请求 UA（可选，留空使用默认）

	ctx           *pluginEntity.Plugin
	limiter       *rate.Limiter
	downloadCache map[string]string // 源图URL -> 新图床URL（批次内去重）
	cacheMu       sync.Mutex
}

const (
	repairStatusOK      = "ok"
	repairStatusFail    = "fail"
	repairStatusSkipped = "skipped"
)

// 缩略图候选选择器（og 优先，twitter 次之），与 spec FR-3 一致
var fixBrokenThumbSelectors = []string{
	"meta[property='og:image']",
	"meta[property='twitter:image']",
	"meta[property='twitter:image:src']",
	"meta[name='twitter:image']",
	"meta[name='twitter:image:src']",
}

// 正文容器选择器（与 GnDownSpider 采集时的正文容器一致）
var fixBrokenContentSelectors = []string{
	"article.article-content",
	".article-content",
	"article .entry-content",
	".entry-content",
	".post-content",
}

func NewFixBrokenImages() *FixBrokenImages {
	return &FixBrokenImages{
		BrokenDomain:        "image.08rj.com",
		BatchSize:           50,
		FetchTimeoutSeconds: 15,
		RateLimitPerMinute:  30,
		DownRetry:           2,
	}
}

func (p *FixBrokenImages) Info() *pluginEntity.PluginInfo {
	return &pluginEntity.PluginInfo{
		ID:         "FixBrokenImages",
		About:      "修复失效图床图片（从历史数据源恢复缩略图与正文图片，转存新图床）",
		RunEnable:  true, // 允许手动执行（分批触发，修复成功者自动脱离候选集）
		CronEnable: false,
	}
}

func (p *FixBrokenImages) Load(ctx *pluginEntity.Plugin) error {
	p.ctx = ctx
	return nil
}

func (p *FixBrokenImages) batchSize() int {
	if p.BatchSize <= 0 {
		return 50
	}
	return p.BatchSize
}

func (p *FixBrokenImages) timeout() time.Duration {
	if p.FetchTimeoutSeconds <= 0 {
		return 15 * time.Second
	}
	return time.Duration(p.FetchTimeoutSeconds) * time.Second
}

// waitLimit 请求前限频（对源站的页面与图片请求合计限速）
func (p *FixBrokenImages) waitLimit() {
	n := p.RateLimitPerMinute
	if n <= 0 {
		n = 30
	}
	if p.limiter == nil {
		p.limiter = rate.NewLimiter(rate.Every(time.Minute/time.Duration(n)), 1)
	}
	_ = p.limiter.Wait(context.Background())
}

func (p *FixBrokenImages) newRequest(referer string) *request.Request {
	r := request.New().SetTimeout(p.timeout())
	if p.DownRetry > 0 {
		r.SetRetry(p.DownRetry)
	}
	r.SetUserAgentMust(p.UserAgent)
	if p.DownReferer != "" {
		r.SetReferer(p.DownReferer)
	} else if referer != "" {
		r.SetReferer(referer)
	}
	return r
}

// fetch 获取远程内容（校验 HTTP 状态码，4xx/5xx 视为不可达）
func (p *FixBrokenImages) fetch(uri, referer string) ([]byte, error) {
	p.waitLimit()
	resp, err := p.newRequest(referer).Get(uri)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected http status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// transferImage 下载源图片并转存到当前上传图床，返回新地址（批次内按源URL去重）
func (p *FixBrokenImages) transferImage(imgURL, referer string) (string, error) {
	p.cacheMu.Lock()
	if cached, ok := p.downloadCache[imgURL]; ok {
		p.cacheMu.Unlock()
		return cached, nil
	}
	p.cacheMu.Unlock()

	file, err := p.fetch(imgURL, referer)
	if err != nil {
		return "", err
	}
	imageType, err := filetype.Image(file)
	if imageType == types.Unknown || err != nil {
		return "", errors.New("downloaded file is not an image")
	}
	val := storage.NewSetValueBytes(file)
	val.ContentType = imageType.MIME.Value
	// 以源 URL 的 md5 作为文件名基准：同源稳定、同名即同图
	res, err := upload.Upload(cryptor.Md5String(imgURL), imageType.Extension, val)
	if err != nil {
		return "", err
	}

	p.cacheMu.Lock()
	p.downloadCache[imgURL] = res.URL
	p.cacheMu.Unlock()
	return res.URL, nil
}

// extractImageURLFromSelection 从元素上按优先级取图片地址（meta 读 content，img 读 src/data-src/data-lazy-src）
func extractImageURLFromSelection(sel *goquery.Selection) string {
	for _, attr := range []string{"content", "src", "data-src", "data-lazy-src"} {
		if v, ok := sel.Attr(attr); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// absolutize 将相对地址按基准页面地址绝对化
func absolutize(baseURL *url.URL, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || baseURL == nil {
		return raw
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return baseURL.ResolveReference(ref).String()
}

// isValidRepairCandidate 候选图片地址是否可用于修复（http(s) 且不属于失效域名）
func isValidRepairCandidate(imgURL, brokenDomain string) bool {
	if !strings.HasPrefix(imgURL, "http://") && !strings.HasPrefix(imgURL, "https://") {
		return false
	}
	return !strings.Contains(imgURL, brokenDomain)
}

// findContentContainer 找到源页面第一个命中的正文容器
func findContentContainer(doc *goquery.Document) *goquery.Selection {
	for _, selector := range fixBrokenContentSelectors {
		c := doc.Find(selector).First()
		if c.Length() > 0 {
			return c
		}
	}
	return nil
}

// extractThumbnailImage 从源页面提取缩略图候选（og:image → twitter:image → 正文第一张图），已绝对化
func extractThumbnailImage(doc *goquery.Document, baseURL *url.URL, brokenDomain string) string {
	for _, selector := range fixBrokenThumbSelectors {
		sel := doc.Find(selector).First()
		if sel.Length() == 0 {
			continue
		}
		raw := extractImageURLFromSelection(sel)
		if raw == "" {
			continue
		}
		abs := absolutize(baseURL, raw)
		if isValidRepairCandidate(abs, brokenDomain) {
			return abs
		}
	}
	// 回退：正文第一张图（spec FR-3）
	if container := findContentContainer(doc); container != nil {
		img := container.Find("img").First()
		if img.Length() > 0 {
			raw := extractImageURLFromSelection(img)
			abs := absolutize(baseURL, raw)
			if isValidRepairCandidate(abs, brokenDomain) {
				return abs
			}
		}
	}
	return ""
}

// extractContentImages 从源页面正文容器提取图片候选列表（已绝对化，按出现顺序）
func extractContentImages(doc *goquery.Document, baseURL *url.URL, brokenDomain string) []string {
	var list []string
	container := findContentContainer(doc)
	if container == nil {
		return list
	}
	container.Find("img").Each(func(_ int, sel *goquery.Selection) {
		raw := extractImageURLFromSelection(sel)
		if raw == "" {
			return
		}
		abs := absolutize(baseURL, raw)
		if isValidRepairCandidate(abs, brokenDomain) {
			list = append(list, abs)
		}
	})
	return list
}

// contentBrokenImages 文章正文中引用了失效域名的 img 元素（按出现顺序）
func contentBrokenImages(doc *goquery.Document, brokenDomain string) []*goquery.Selection {
	var list []*goquery.Selection
	doc.Find("img").Each(func(_ int, sel *goquery.Selection) {
		src, ok := sel.Attr("src")
		if !ok || strings.TrimSpace(src) == "" {
			return
		}
		if strings.Contains(src, brokenDomain) {
			list = append(list, sel)
		}
	})
	return list
}

// contentHasBrokenImage 正文 img 是否引用了失效域名（仅 img src 维度，不误伤正文文本中的域名描述）
func contentHasBrokenImage(contentHTML, brokenDomain string) bool {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(contentHTML))
	if err != nil {
		return false
	}
	return len(contentBrokenImages(doc, brokenDomain)) > 0
}

// needsRepair 文章是否需要修复（缩略图或正文 img 引用了失效域名）
func needsRepair(item *entity.Article, brokenDomain string) bool {
	if strings.Contains(item.Thumbnail, brokenDomain) {
		return true
	}
	return contentHasBrokenImage(item.Content, brokenDomain)
}

// planContentReplacement 为正文失效图片计算替换计划：第 N 个失效 img ← 第 N 个源图。
// 源图不足时返回错误（全有或全无：调用方不得执行改写）。
func planContentReplacement(contentHTML, brokenDomain string, sourceImages []string) ([]string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(contentHTML))
	if err != nil {
		return nil, err
	}
	broken := contentBrokenImages(doc, brokenDomain)
	if len(sourceImages) < len(broken) {
		return nil, fmt.Errorf("not enough content images: broken=%d available=%d", len(broken), len(sourceImages))
	}
	return sourceImages[:len(broken)], nil
}

// applyContentReplacements 将正文失效 img 的 src 依次替换为新地址，返回替换后的 HTML
func applyContentReplacements(contentHTML, brokenDomain string, newURLs []string) (string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(contentHTML))
	if err != nil {
		return "", err
	}
	broken := contentBrokenImages(doc, brokenDomain)
	if len(newURLs) < len(broken) {
		return "", fmt.Errorf("not enough replacement urls: broken=%d available=%d", len(broken), len(newURLs))
	}
	for i, sel := range broken {
		sel.SetAttr("src", newURLs[i])
	}
	return doc.Find("body").Html()
}

func (p *FixBrokenImages) logOne(msg string, item *entity.Article, fields ...zap.Field) {
	if p.ctx == nil || p.ctx.Log == nil {
		return
	}
	fields = append(fields, zap.String("title", item.Title))
	p.ctx.Log.Info(msg, fields...)
}

func (p *FixBrokenImages) failOne(item *entity.Article, reason string) string {
	p.logOne(fmt.Sprintf("repair fail id=%d reason=%s", item.ID, reason), item)
	return repairStatusFail
}

// repairOne 修复单篇文章，返回 ok/fail/skipped。
// 流程：失效判定 → 读历史数据源 → 抓源页 → 提取 → 映射与转存 → 断言 → 全部成功才写库并失效缓存。
func (p *FixBrokenImages) repairOne(item *entity.Article, domain string) string {
	if !needsRepair(item, domain) {
		p.logOne(fmt.Sprintf("repair skipped id=%d reason=no_broken_refs", item.ID), item)
		return repairStatusSkipped
	}

	oldThumbnail := item.Thumbnail

	// 1. 历史数据源（随文保存的源站页面地址）
	sourceURL, _ := item.Extends.Get("source_url").(string)
	if strings.TrimSpace(sourceURL) == "" {
		return p.failOne(item, "no_source_url")
	}

	// 2. 抓取源页面
	body, err := p.fetch(sourceURL, "")
	if err != nil {
		p.logOne(fmt.Sprintf("repair fail id=%d reason=source_unreachable", item.ID), item, zap.Error(err))
		return repairStatusFail
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		p.logOne(fmt.Sprintf("repair fail id=%d reason=source_unreachable", item.ID), item, zap.Error(err))
		return repairStatusFail
	}
	baseURL, err := url.Parse(sourceURL)
	if err != nil {
		return p.failOne(item, "source_unreachable")
	}

	// 3. 缩略图修复计划（og:image 缺失时回退正文第一张图）
	var newThumbnail string
	if strings.Contains(oldThumbnail, domain) {
		candidate := extractThumbnailImage(doc, baseURL, domain)
		if candidate == "" {
			return p.failOne(item, "no_valid_images")
		}
		newThumbnail, err = p.transferImage(candidate, sourceURL)
		if err != nil {
			p.logOne(fmt.Sprintf("repair fail id=%d reason=transfer_failed image=%s", item.ID, candidate), item, zap.Error(err))
			return repairStatusFail
		}
	}

	// 4. 正文修复计划：第 N 个失效 img ← 第 N 张源图（先全部转存，再改写）
	var newContent string
	contentRepaired := 0
	if contentHasBrokenImage(item.Content, domain) {
		sourceImages := extractContentImages(doc, baseURL, domain)
		plan, err := planContentReplacement(item.Content, domain, sourceImages)
		if err != nil {
			return p.failOne(item, "not_enough_content_images")
		}
		newURLs := make([]string, 0, len(plan))
		for _, imgURL := range plan {
			nu, err := p.transferImage(imgURL, sourceURL)
			if err != nil {
				p.logOne(fmt.Sprintf("repair fail id=%d reason=transfer_failed image=%s", item.ID, imgURL), item, zap.Error(err))
				return repairStatusFail
			}
			newURLs = append(newURLs, nu)
		}
		newContent, err = applyContentReplacements(item.Content, domain, newURLs)
		if err != nil {
			return p.failOne(item, "no_valid_images")
		}
		contentRepaired = len(newURLs)
	}

	// 5. 写入前断言（正文按 img src 维度判断，避免误伤正文文本中的域名描述）
	finalThumbnail := oldThumbnail
	if newThumbnail != "" {
		finalThumbnail = newThumbnail
	}
	finalContent := item.Content
	if newContent != "" {
		finalContent = newContent
	}
	if strings.Contains(finalThumbnail, domain) || contentHasBrokenImage(finalContent, domain) {
		return p.failOne(item, "no_valid_images")
	}

	// 6. 全部就绪，写库 + 失效文章页缓存
	item.Thumbnail = finalThumbnail
	item.Content = finalContent
	if err := repository.Article.Update(item); err != nil {
		p.logOne("repair fail", item, zap.String("reason", "update_failed"), zap.Error(err))
		return repairStatusFail
	}
	if config.Config.Cache.Enable {
		if err := cache.InvalidateArticleCache(item.URL()); err != nil {
			p.ctx.Log.Warn("failed to invalidate article cache", zap.Int("id", item.ID), zap.Error(err))
		}
	}

	p.logOne(fmt.Sprintf("repair ok id=%d thumbnail=%s->%s content_imgs=%d", item.ID, oldThumbnail, finalThumbnail, contentRepaired), item)
	return repairStatusOK
}

// Run 手动执行一批修复（修复成功者不再含失效域名，自动脱离候选集，可重复执行至全部完成）
func (p *FixBrokenImages) Run(ctx *pluginEntity.Plugin) error {
	if p.ctx == nil {
		p.ctx = ctx
	}
	domain := strings.TrimSpace(p.BrokenDomain)
	if domain == "" {
		return errors.New("broken_domain is required")
	}
	// 重置批次状态（限频器按当前选项重建，下载去重缓存按批次隔离）
	p.limiter = nil
	p.downloadCache = map[string]string{}

	ctx.Log.Info(fmt.Sprintf("repair start batch_size=%d broken_domain=%s", p.batchSize(), domain))

	articles, err := repository.Article.ListArticlesWithImageDomain(domain, p.batchSize())
	if err != nil {
		ctx.Log.Error("query broken articles failed", zap.Error(err))
		return err
	}
	if len(articles) == 0 {
		ctx.Log.Info("无待修复文章，全部完成")
		return nil
	}

	var okCount, failCount, skipCount int
	for _, item := range articles {
		switch p.repairOne(item, domain) {
		case repairStatusOK:
			okCount++
		case repairStatusSkipped:
			skipCount++
		default:
			failCount++
		}
	}

	ctx.Log.Info(fmt.Sprintf("summary processed=%d ok=%d fail=%d skipped=%d", len(articles), okCount, failCount, skipCount))

	// 批次内有修复时失效首页缓存
	if okCount > 0 && config.Config.Cache.Enable {
		if err := cache.InvalidateHomePageCache(); err != nil {
			ctx.Log.Warn("failed to invalidate home page cache", zap.Error(err))
		}
	}
	return nil
}
