package plugins

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"moss/domain/config"
	"moss/domain/core/entity"
	"moss/domain/core/repository/context"
	"moss/domain/core/service"
	"moss/domain/core/vo"
	pluginEntity "moss/domain/support/entity"
	"moss/infrastructure/support/cache"
	"moss/infrastructure/utils/request"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/panjf2000/ants/v2"
	"go.uber.org/zap"
)

// APIConfig 单个 API 配置
type APIConfig struct {
	ID     string `json:"id"`      // 唯一标识（UUID）
	Name   string `json:"name"`    // 配置名称
	AIType string `json:"ai_type"` // AI 类型: openai/nvidia/zhipu
	ApiURL string `json:"api_url"` // API 地址（兼容 OpenAI 格式）
	ApiKey string `json:"api_key"` // API 密钥
	Model  string `json:"model"`   // 模型名称（如 gpt-4）
	Enable bool   `json:"enable"`  // 是否启用

	// RequestDelay 每次请求后的等待时间（毫秒），用于避免 API 限流
	// 默认 1000ms，NVIDIA API 建议设置 2000ms 以上
	RequestDelay int `json:"request_delay"`

	// mutex 保护同一 API 配置的并发调用，防止响应错乱
	// 注意：这是一个非导出字段，不会被 JSON 序列化
	mutex sync.Mutex `json:"-"`
}

type AISeoPlugin struct {
	// 新：多 API 配置列表
	APIConfigs []APIConfig `json:"api_configs"`

	// 旧字段（保留用于配置迁移，标记为 deprecated）
	// Deprecated: 使用 APIConfigs 替代
	AIType string `json:"ai_type"` // AI 类型: openai/nvidia/zhipu
	ApiURL string `json:"api_url"` // API 地址（兼容 OpenAI 格式）
	ApiKey string `json:"api_key"` // API 密钥
	Model  string `json:"model"`   // 模型名称（如 gpt-4）

	Enable bool `json:"enable"` // 是否启用插件

	// 各功能独立开关
	EnableTitleOptimize     bool `json:"enable_title_optimize"`     // 启用标题优化
	EnableCategoryRecommend bool `json:"enable_category_recommend"` // 启用智能分类推荐
	EnableKeywords          bool `json:"enable_keywords"`           // 启用关键词提取
	EnableDescription       bool `json:"enable_description"`        // 启用描述优化
	EnableRewrite           bool `json:"enable_rewrite"`            // 启用内容改写
	EnableTags              bool `json:"enable_tags"`               // 启用标签生成

	// 整合模式开关
	EnableIntegratedMode bool `json:"enable_integrated_mode"` // 启用整合模式（将多个功能合并到一个 AI 请求中）

	// 关键词配置
	MinKeywords int `json:"min_keywords"`  // 最少关键词数（默认 4）
	MaxKeywords int `json:"max_keywords"`  // 最多关键词数（默认 8）
	MinLongTail int `json:"min_long_tail"` // 最少长尾关键词数（默认 2）

	// 标签配置
	MinTags int `json:"min_tags"` // 最少标签数（默认 1）
	MaxTags int `json:"max_tags"` // 最多标签数（默认 3）

	// 自动发布
	AutoPublish bool `json:"auto_publish"` // SEO 处理完成后自动发布文章

	// 特殊分类名称
	OtherCategoryName string `json:"other_category_name"` // 其他分类名称

	// 定时任务
	CronEnable bool   `json:"cron_enable"` // 启用定时任务
	CronExp    string `json:"cron_exp"`    // 定时表达式
	BatchSize  int    `json:"batch_size"`  // 每次处理的文章数量（默认 10）

	// 指定文章处理
	ArticleID int `json:"article_id"` // 指定处理的文章 ID（0 表示批量处理）

	// 强制重新生成（手动触发时可选）
	ForceRegenerate bool `json:"force_regenerate"` // 是否强制重新生成（忽略已有标记）

	// 跳过已发布文章
	SkipPublished bool `json:"skip_published"` // 是否跳过已发布的文章（默认 false）

	ctx *pluginEntity.Plugin

	// legacyMutex 用于旧版 callAI 方法的并发保护（兼容旧配置）
	legacyMutex sync.Mutex
}

func NewAISeoPlugin() *AISeoPlugin {
	return &AISeoPlugin{
		APIConfigs: []APIConfig{}, // 初始化为空切片
		Enable:     true,
		// 各功能开关默认值
		EnableTitleOptimize:     true,  // 默认启用标题优化
		EnableCategoryRecommend: false, // 默认不启用分类推荐
		EnableKeywords:          true,  // 默认启用关键词提取
		EnableDescription:       true,  // 默认启用描述优化
		EnableRewrite:           false, // 默认不启用内容改写（消耗较多token）
		EnableTags:              true,  // 默认启用标签生成
		// 整合模式默认关闭，保持向后兼容
		EnableIntegratedMode: false, // 默认不启用整合模式
		// 关键词配置
		MinKeywords: 4,
		MaxKeywords: 8,
		MinLongTail: 2,
		// 标签配置
		MinTags: 1,
		MaxTags: 3,
		// 其他配置
		AutoPublish:       false,
		OtherCategoryName: "其他软件",
		CronEnable:        false,
		CronExp:           "@every 30m",
		BatchSize:         10,
		ForceRegenerate:   false,
		SkipPublished:     false,
	}
}

// migrateOldConfig 迁移旧配置到新格式
func (p *AISeoPlugin) migrateOldConfig() {
	// 如果已有新配置，跳过迁移
	if len(p.APIConfigs) > 0 {
		return
	}

	// 如果旧配置存在，迁移到新格式
	if p.ApiURL != "" && p.ApiKey != "" {
		p.ctx.Log.Info("Migrating old API config to new format",
			zap.String("ai_type", p.AIType),
			zap.String("api_url", p.ApiURL))

		p.APIConfigs = []APIConfig{{
			ID:     uuid.New().String(),
			Name:   "默认配置",
			AIType: p.AIType,
			ApiURL: p.ApiURL,
			ApiKey: p.ApiKey,
			Model:  p.Model,
			Enable: true,
		}}

		// 清空旧字段，避免重复迁移
		p.AIType = ""
		p.ApiURL = ""
		p.ApiKey = ""
		p.Model = ""
	}
}

// getEnabledAPIConfigs 获取所有启用的 API 配置
func (p *AISeoPlugin) getEnabledAPIConfigs() []APIConfig {
	var enabled []APIConfig
	for _, cfg := range p.APIConfigs {
		if cfg.Enable {
			enabled = append(enabled, cfg)
		}
	}
	return enabled
}

// Info 返回插件信息
// autoDetectAIType 根据 API URL 自动检测 AI 类型
func (p *AISeoPlugin) autoDetectAIType() {
	if p.ApiURL == "" {
		return
	}

	// 如果 ai_type 已正确设置，则跳过
	switch p.AIType {
	case "nvidia", "zhipu", "openai":
		// 检查是否与 URL 匹配
		if p.isAITypeMatchURL(p.AIType, p.ApiURL) {
			return
		}
	}

	// 根据 URL 自动识别类型
	urlLower := strings.ToLower(p.ApiURL)
	switch {
	case strings.Contains(urlLower, "nvidia"):
		p.AIType = "nvidia"
		p.ctx.Log.Info("Auto-detected AI type from URL", zap.String("ai_type", "nvidia"))
	case strings.Contains(urlLower, "bigmodel") || strings.Contains(urlLower, "zhipu"):
		p.AIType = "zhipu"
		p.ctx.Log.Info("Auto-detected AI type from URL", zap.String("ai_type", "zhipu"))
	default:
		p.AIType = "openai"
	}
}

// isAITypeMatchURL 检查 AI 类型是否与 URL 匹配
func (p *AISeoPlugin) isAITypeMatchURL(aiType, apiURL string) bool {
	urlLower := strings.ToLower(apiURL)
	switch aiType {
	case "nvidia":
		return strings.Contains(urlLower, "nvidia")
	case "zhipu":
		return strings.Contains(urlLower, "bigmodel") || strings.Contains(urlLower, "zhipu")
	case "openai":
		return !strings.Contains(urlLower, "nvidia") && !strings.Contains(urlLower, "bigmodel")
	}
	return false
}

func (p *AISeoPlugin) Info() *pluginEntity.PluginInfo {
	return &pluginEntity.PluginInfo{
		ID:         "AISeoPlugin",
		About:      "SEO插件：AI 驱动的 SEO 优化，智能分类推荐、关键词提取、描述优化、内容改写、标签生成",
		RunEnable:  true,
		CronEnable: true,
		NoOptions:  false,
		PluginInfoPersistent: pluginEntity.PluginInfoPersistent{
			CronStart: false,
			CronExp:   p.CronExp,
		},
	}
}

// Load 插件加载
func (p *AISeoPlugin) Load(ctx *pluginEntity.Plugin) error {
	p.ctx = ctx
	// 迁移旧配置到新格式
	p.migrateOldConfig()
	// 注册文章事件
	service.Article.AddCreateBeforeEvents(p)
	service.Article.AddUpdateAfterEvents(p)
	return nil
}

// Run 插件执行（定时任务或手动触发）
func (p *AISeoPlugin) Run(ctx *pluginEntity.Plugin) error {
	p.ctx = ctx

	if !p.Enable {
		p.ctx.Log.Warn("AISeoPlugin is disabled")
		return nil
	}

	// 迁移旧配置（确保兼容性）
	p.migrateOldConfig()

	// 获取启用的 API 配置
	enabledAPIs := p.getEnabledAPIConfigs()
	if len(enabledAPIs) == 0 {
		p.ctx.Log.Error("AISeoPlugin configuration is incomplete: no enabled API configurations")
		return errors.New("no enabled API configurations")
	}

	p.ctx.Log.Info("AISeoPlugin started",
		zap.Int("api_count", len(enabledAPIs)),
		zap.Int("article_id", p.ArticleID),
		zap.Bool("force_regenerate", p.ForceRegenerate),
		zap.Bool("skip_published", p.SkipPublished))

	// 如果指定了文章 ID，只处理该文章
	if p.ArticleID > 0 {
		article, err := service.Article.Get(p.ArticleID)
		if err != nil {
			p.ctx.Log.Error("Failed to get article", zap.Int("article_id", p.ArticleID), zap.Error(err))
			return err
		}

		p.ctx.Log.Info("Processing single article", zap.Int("article_id", article.ID), zap.String("title", article.Title))

		// 使用第一个启用的 API 配置
		apiCfg := enabledAPIs[0]
		if err := p.processArticleWithAPI(article, &apiCfg); err != nil {
			p.ctx.Log.Error("Failed to process article", zap.Int("article_id", article.ID), zap.Error(err))
			return err
		}

		p.ctx.Log.Info("AISeoPlugin completed", zap.Int("success", 1))
		return nil
	}

	// 获取未生成的文章
	articles, err := p.getUngeneratedArticles(p.BatchSize)
	if err != nil {
		p.ctx.Log.Error("Failed to get articles", zap.Error(err))
		return err
	}

	if len(articles) == 0 {
		p.ctx.Log.Info("No articles to process")
		return nil
	}

	p.ctx.Log.Info("AISeoPlugin processing articles",
		zap.Int("articles", len(articles)),
		zap.Int("api_count", len(enabledAPIs)))

	// 使用 ants 协程池并行处理文章
	// 协程池大小等于 API 配置数量，每个 API 独立处理分配给它的文章
	successCount := int64(0)
	var mu sync.Mutex // 保护日志顺序

	// 文章任务结构
	type articleTask struct {
		article *entity.Article
		apiCfg  *APIConfig
		index   int
		wg      *sync.WaitGroup
	}

	// 等待组
	var wg sync.WaitGroup

	// 创建协程池处理函数
	processArticleTask := func(args interface{}) {
		task := args.(*articleTask)
		defer task.wg.Done()

		mu.Lock()
		p.ctx.Log.Info("Processing article",
			zap.Int("progress", task.index+1),
			zap.Int("total", len(articles)),
			zap.Int("article_id", task.article.ID),
			zap.String("title", task.article.Title),
			zap.String("api_name", task.apiCfg.Name))
		mu.Unlock()

		if err := p.processArticleWithAPI(task.article, task.apiCfg); err != nil {
			p.ctx.Log.Error("Failed to process article",
				zap.Int("article_id", task.article.ID),
				zap.String("title", task.article.Title),
				zap.String("api_name", task.apiCfg.Name),
				zap.Error(err))
		} else {
			atomic.AddInt64(&successCount, 1)
		}
	}

	// 创建协程池，大小等于 API 配置数量
	poolSize := len(enabledAPIs)
	pool, err := ants.NewPoolWithFunc(poolSize, processArticleTask)
	if err != nil {
		p.ctx.Log.Error("Failed to create goroutine pool", zap.Error(err))
		return err
	}
	defer pool.Release()

	// 按文章索引分配给不同 API，提交任务
	for i, article := range articles {
		apiCfg := enabledAPIs[i%len(enabledAPIs)]
		wg.Add(1)
		task := &articleTask{
			article: article,
			apiCfg:  &apiCfg,
			index:   i,
			wg:      &wg,
		}
		// 阻塞等待直到有空闲 worker
		if err := pool.Invoke(task); err != nil {
			p.ctx.Log.Error("Failed to submit task",
				zap.Int("article_id", article.ID),
				zap.Error(err))
			wg.Done()
		}
	}

	// 等待所有任务完成
	wg.Wait()

	// Invalidate home page cache after batch processing if any articles were updated
	if successCount > 0 && config.Config.Cache.Enable {
		if err := cache.InvalidateHomePageCache(); err != nil {
			p.ctx.Log.Warn("Failed to invalidate home page cache after batch processing", zap.Error(err))
		}
	}

	p.ctx.Log.Info("AISeoPlugin completed",
		zap.Int64("success", successCount),
		zap.Int("total", len(articles)))

	return nil
}

// ArticleCreateBefore 文章创建前事件
func (p *AISeoPlugin) ArticleCreateBefore(item *entity.Article) error {
	if !p.Enable {
		return nil
	}
	// 不在创建时自动处理，避免影响创建速度
	// 可以通过定时任务或手动触发来处理
	return nil
}

// ArticleUpdateAfter 文章更新后事件
func (p *AISeoPlugin) ArticleUpdateAfter(item *entity.Article) {
	if !p.Enable {
		return
	}
	// 不在更新时自动处理，避免频繁触发
	// 可以通过定时任务或手动触发来处理
}

// isArticleGenerated 检查文章是否已生成
func (p *AISeoPlugin) isArticleGenerated(article *entity.Article) bool {
	if article.Extends == nil {
		return false
	}
	val := article.Extends.Get("ai_seo_generated")
	if val == nil {
		return false
	}
	// 检查值是否为 true
	if boolVal, ok := val.(bool); ok {
		return boolVal
	}
	return false
}

// getUngeneratedArticles 获取未生成的文章
func (p *AISeoPlugin) getUngeneratedArticles(limit int) ([]*entity.Article, error) {
	// 直接通过 SQL 过滤，替代原来的内存过滤
	articles, err := service.Article.ListUngeneratedArticles(limit, p.SkipPublished, p.ForceRegenerate)
	if err != nil {
		p.ctx.Log.Error("Failed to get ungenerated articles", zap.Error(err))
		return nil, err
	}

	p.ctx.Log.Debug("Found ungenerated articles",
		zap.Int("count", len(articles)),
		zap.Bool("skip_published", p.SkipPublished),
		zap.Bool("force_regenerate", p.ForceRegenerate))

	return articles, nil
}

// processArticle 处理单篇文章
func (p *AISeoPlugin) processArticle(article *entity.Article) error {
	// 检查是否已生成
	if !p.ForceRegenerate && p.isArticleGenerated(article) {
		p.ctx.Log.Debug("Article already generated, skipping",
			zap.Int("article_id", article.ID),
			zap.String("title", article.Title))
		return nil
	}

	p.ctx.Log.Info("Processing article",
		zap.Int("article_id", article.ID),
		zap.String("title", article.Title),
		zap.String("current_keywords", article.Keywords),
		zap.String("current_description", article.Description))

	// 调用 AI 生成 SEO 内容
	result, err := p.generateSEOContent(article)
	if err != nil {
		p.ctx.Log.Error("Failed to generate SEO content",
			zap.Int("article_id", article.ID),
			zap.String("title", article.Title),
			zap.Error(err))
		return err
	}

	// 更新文章字段
	p.updateArticle(article, result)

	// 保存文章
	if err := service.Article.Update(article); err != nil {
		p.ctx.Log.Error("Failed to update article",
			zap.Int("article_id", article.ID),
			zap.Error(err))
		return err
	}

	// 标记为已生成（只有成功时才记录）
	p.markAsGenerated(article)

	// 自动发布文章
	if p.AutoPublish {
		oldStatus := article.Status
		oldCreateTime := article.CreateTime
		article.Status = true
		article.CreateTime = time.Now().Unix()
		p.ctx.Log.Info("Article auto-published",
			zap.Int("article_id", article.ID),
			zap.Bool("old_status", oldStatus),
			zap.Bool("new_status", article.Status),
			zap.Int64("old_create_time", oldCreateTime),
			zap.Int64("new_create_time", article.CreateTime))
	}

	// 再次保存（更新 extends）
	if err := service.Article.Update(article); err != nil {
		p.ctx.Log.Error("Failed to update article extends",
			zap.Int("article_id", article.ID),
			zap.Error(err))
		return err
	}

	// Invalidate article cache after update
	if config.Config.Cache.Enable {
		if err := cache.InvalidateArticleCache(article.URL()); err != nil {
			p.ctx.Log.Warn("Failed to invalidate article cache",
				zap.Int("article_id", article.ID),
				zap.Error(err))
		}
		// Invalidate home page cache if article is published or auto-published
		if article.Status || p.AutoPublish {
			if err := cache.InvalidateHomePageCache(); err != nil {
				p.ctx.Log.Warn("Failed to invalidate home page cache", zap.Error(err))
			}
		}
	}

	// 输出成功日志
	p.ctx.Log.Info("Article processed successfully",
		zap.Int("article_id", article.ID),
		zap.String("title", article.Title),
		zap.Int("keywords_count", len(result.Keywords)),
		zap.String("new_keywords", strings.Join(result.Keywords, ",")),
		zap.String("new_description", result.Description),
		zap.Int("tags_count", len(result.Tags)),
		zap.Bool("content_rewrited", result.ContentRewrited),
	)

	return nil
}

// processArticleWithAPI 使用指定 API 配置处理单篇文章
func (p *AISeoPlugin) processArticleWithAPI(article *entity.Article, apiCfg *APIConfig) error {
	// 检查是否已生成
	if !p.ForceRegenerate && p.isArticleGenerated(article) {
		p.ctx.Log.Debug("Article already generated, skipping",
			zap.Int("article_id", article.ID),
			zap.String("title", article.Title))
		return nil
	}

	p.ctx.Log.Info("Processing article with API config",
		zap.Int("article_id", article.ID),
		zap.String("title", article.Title),
		zap.String("api_name", apiCfg.Name),
		zap.String("api_type", apiCfg.AIType),
		zap.String("current_keywords", article.Keywords),
		zap.String("current_description", article.Description))

	// 调用 AI 生成 SEO 内容（使用指定 API 配置）
	result, err := p.generateSEOContentWithAPI(article, apiCfg)
	if err != nil {
		p.ctx.Log.Error("Failed to generate SEO content",
			zap.Int("article_id", article.ID),
			zap.String("title", article.Title),
			zap.String("api_name", apiCfg.Name),
			zap.Error(err))
		return err
	}

	// 更新文章字段
	p.updateArticle(article, result)

	// 保存文章
	if err := service.Article.Update(article); err != nil {
		p.ctx.Log.Error("Failed to update article",
			zap.Int("article_id", article.ID),
			zap.Error(err))
		return err
	}

	// 标记为已生成（只有成功时才记录）
	p.markAsGenerated(article)

	// 自动发布文章
	if p.AutoPublish {
		oldStatus := article.Status
		oldCreateTime := article.CreateTime
		article.Status = true
		article.CreateTime = time.Now().Unix()
		p.ctx.Log.Info("Article auto-published",
			zap.Int("article_id", article.ID),
			zap.Bool("old_status", oldStatus),
			zap.Bool("new_status", article.Status),
			zap.Int64("old_create_time", oldCreateTime),
			zap.Int64("new_create_time", article.CreateTime))
	}

	// 再次保存（更新 extends）
	if err := service.Article.Update(article); err != nil {
		p.ctx.Log.Error("Failed to update article extends",
			zap.Int("article_id", article.ID),
			zap.Error(err))
		return err
	}

	// Invalidate article cache after update
	if config.Config.Cache.Enable {
		if err := cache.InvalidateArticleCache(article.URL()); err != nil {
			p.ctx.Log.Warn("Failed to invalidate article cache",
				zap.Int("article_id", article.ID),
				zap.Error(err))
		}
		// Invalidate home page cache if article is published or auto-published
		if article.Status || p.AutoPublish {
			if err := cache.InvalidateHomePageCache(); err != nil {
				p.ctx.Log.Warn("Failed to invalidate home page cache", zap.Error(err))
			}
		}
	}

	// 输出成功日志
	p.ctx.Log.Info("Article processed successfully",
		zap.Int("article_id", article.ID),
		zap.String("title", article.Title),
		zap.String("api_name", apiCfg.Name),
		zap.Int("keywords_count", len(result.Keywords)),
		zap.String("new_keywords", strings.Join(result.Keywords, ",")),
		zap.String("new_description", result.Description),
		zap.Int("tags_count", len(result.Tags)),
		zap.Bool("content_rewrited", result.ContentRewrited),
	)

	return nil
}

// AISeoResult SEO 生成结果
type AISeoResult struct {
	Title           string   `json:"title"` // 优化后的标题
	CategoryID      int      `json:"category_id"`
	CategoryChanged bool     `json:"category_changed"`
	Keywords        []string `json:"keywords"`
	Description     string   `json:"description"`
	ContentRewrited bool     `json:"content_rewrited"`
	Tags            []string `json:"tags"`
}

// IntegratedResponse 整合模式 AI 响应结构
type IntegratedResponse struct {
	Title       string `json:"title"`       // 优化后的标题
	Category    string `json:"category"`    // 推荐的分类名称
	Keywords    string `json:"keywords"`    // 关键词（逗号分隔）
	Description string `json:"description"` // 优化后的描述
	Content     string `json:"content"`     // 改写后的内容
	Tags        string `json:"tags"`        // 标签（逗号分隔）
}

// parseIntegratedResponse 解析整合模式的 JSON 响应
func (p *AISeoPlugin) parseIntegratedResponse(content string) (*IntegratedResponse, error) {
	// 清理推理模型的思考过程
	if p.isReasoningModelForConfig(p.Model) {
		content = p.cleanReasoningResponse(content)
	}

	// 提取 JSON 部分
	// 查找第一个 { 和最后一个 }
	startIdx := strings.Index(content, "{")
	endIdx := strings.LastIndex(content, "}")
	if startIdx == -1 || endIdx == -1 || endIdx < startIdx {
		p.ctx.Log.Error("No valid JSON found in response",
			zap.String("content_preview", truncateText(content, 500)))
		return nil, errors.New("no valid JSON found in response")
	}

	jsonStr := content[startIdx : endIdx+1]
	p.ctx.Log.Debug("Extracted JSON string",
		zap.Int("json_length", len(jsonStr)),
		zap.String("json_preview", truncateText(jsonStr, 500)))

	// 修复无效的 Unicode 转义序列
	// AI 可能返回大写的 \U 转义（如 \UXXXX），而 JSON 标准只支持小写的 \u
	// 将 \U 替换为 \u（后面跟着4个十六进制字符）
	fixedJsonStr := fixInvalidUnicodeEscapes(jsonStr)
	if fixedJsonStr != jsonStr {
		p.ctx.Log.Debug("Fixed invalid Unicode escapes in JSON",
			zap.String("original_preview", truncateText(jsonStr, 200)),
			zap.String("fixed_preview", truncateText(fixedJsonStr, 200)))
	}

	// 修复无效的转义序列（如 Windows 路径中的 \P）
	// AI 可能返回包含 Windows 路径的内容，如 Thunder\Program\Thunder.exe
	// 其中 \P 不是有效的 JSON 转义序列
	fixedJsonStr = fixInvalidEscapeSequences(fixedJsonStr)

	// 清理无效的 UTF-8 字符
	// AI 可能返回包含无效 UTF-8 字符（如替换字符 \uFFFD）的响应
	cleanedJsonStr := cleanInvalidUTF8(fixedJsonStr)
	if cleanedJsonStr != fixedJsonStr {
		p.ctx.Log.Debug("Cleaned invalid UTF-8 characters in JSON",
			zap.String("original_preview", truncateText(fixedJsonStr, 200)),
			zap.String("cleaned_preview", truncateText(cleanedJsonStr, 200)))
	}

	var resp IntegratedResponse
	if err := json.Unmarshal([]byte(cleanedJsonStr), &resp); err != nil {
		// 详细记录 JSON 解析错误
		p.ctx.Log.Error("Failed to parse integrated response JSON",
			zap.Error(err),
			zap.Int("json_length", len(fixedJsonStr)),
			zap.String("json_preview", truncateText(fixedJsonStr, 1000)),
			zap.String("json_full", fixedJsonStr),
			zap.String("error_type", fmt.Sprintf("%T", err)))

		// 尝试定位错误位置
		if syntaxErr, ok := err.(*json.SyntaxError); ok {
			p.ctx.Log.Error("JSON syntax error details",
				zap.Int64("error_offset", syntaxErr.Offset),
				zap.String("error_message", syntaxErr.Error()),
				zap.String("context_around_error", getJsonErrorContext(fixedJsonStr, syntaxErr.Offset)))
		}

		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	// 清理各字段的空白字符
	resp.Title = strings.TrimSpace(resp.Title)
	resp.Category = strings.TrimSpace(resp.Category)
	resp.Keywords = strings.TrimSpace(resp.Keywords)
	resp.Description = strings.TrimSpace(resp.Description)
	resp.Content = strings.TrimSpace(resp.Content)
	resp.Tags = strings.TrimSpace(resp.Tags)

	p.ctx.Log.Info("Integrated response parsed successfully",
		zap.String("title", resp.Title),
		zap.String("category", resp.Category),
		zap.String("keywords", resp.Keywords),
		zap.Int("description_length", len(resp.Description)),
		zap.Int("content_length", len(resp.Content)),
		zap.String("tags", resp.Tags))

	return &resp, nil
}

// generateSEOContent 生成 SEO 内容
func (p *AISeoPlugin) generateSEOContent(article *entity.Article) (*AISeoResult, error) {
	result := &AISeoResult{}

	// 1. 标题优化（优先处理，因为后续关键词等依赖标题）
	if p.EnableTitleOptimize {
		title, err := p.optimizeTitle(article)
		if err != nil {
			p.ctx.Log.Warn("Failed to optimize title", zap.Error(err))
		} else if title != "" {
			result.Title = title
			p.ctx.Log.Info("Title optimized",
				zap.Int("article_id", article.ID),
				zap.String("old_title", article.Title),
				zap.String("new_title", title))
		}
	}

	// 2. 智能分类推荐
	if p.EnableCategoryRecommend && (article.CategoryID == 0 || p.isOtherCategory(article.CategoryID)) {
		categoryID, err := p.recommendCategory(article)
		if err != nil {
			p.ctx.Log.Warn("Failed to recommend category", zap.Error(err))
		} else if categoryID > 0 {
			result.CategoryID = categoryID
			result.CategoryChanged = true
			p.ctx.Log.Info("Category recommended",
				zap.Int("article_id", article.ID),
				zap.Int("category_id", categoryID))
		}
	}

	// 3. 关键词提取
	var keywords []string
	if p.EnableKeywords {
		var err error
		keywords, err = p.extractKeywords(article)
		if err != nil {
			p.ctx.Log.Warn("Failed to extract keywords", zap.Error(err))
		} else if len(keywords) > 0 {
			result.Keywords = keywords
			p.ctx.Log.Info("Keywords extracted",
				zap.Int("article_id", article.ID),
				zap.Int("count", len(keywords)),
				zap.Strings("keywords", keywords))
		} else {
			p.ctx.Log.Warn("No keywords extracted",
				zap.Int("article_id", article.ID))
		}
	}

	// 4. 描述优化
	if p.EnableDescription {
		description, err := p.optimizeDescription(article, keywords)
		if err != nil {
			p.ctx.Log.Warn("Failed to optimize description", zap.Error(err))
		} else if description != "" {
			result.Description = description
			p.ctx.Log.Info("Description optimized",
				zap.Int("article_id", article.ID),
				zap.String("description", description))
		}
	}

	// 5. 内容改写
	if p.EnableRewrite {
		content, err := p.rewriteContent(article, keywords)
		if err != nil {
			p.ctx.Log.Warn("Failed to rewrite content", zap.Error(err))
		} else if content != "" {
			article.Content = content
			result.ContentRewrited = true
			p.ctx.Log.Info("Content rewritten",
				zap.Int("article_id", article.ID))
		}
	}

	// 6. 标签生成
	if p.EnableTags {
		tags, err := p.generateTags(article, keywords)
		if err != nil {
			p.ctx.Log.Warn("Failed to generate tags", zap.Error(err))
		} else if len(tags) > 0 {
			result.Tags = tags
			p.ctx.Log.Info("Tags generated",
				zap.Int("article_id", article.ID),
				zap.Strings("tags", tags))
		}
	}

	return result, nil
}

// buildIntegratedPrompt 构建整合模式的提示词
// baiduSuggestions: 百度推荐的用户搜索意图列表
func (p *AISeoPlugin) buildIntegratedPrompt(article *entity.Article, categoryList string, baiduSuggestions []string) string {
	var promptBuilder strings.Builder

	promptBuilder.WriteString("你是一个专业的 SEO 优化专家。请对以下文章进行全面的 SEO 优化处理。\n\n")
	promptBuilder.WriteString(fmt.Sprintf("文章标题：%s\n", article.Title))
	promptBuilder.WriteString(fmt.Sprintf("文章内容：\n%s\n\n", truncateText(article.Content, 2000)))

	// 添加百度搜索建议（用户搜索意图）
	if len(baiduSuggestions) > 0 {
		promptBuilder.WriteString("【用户搜索意图参考】（来自百度推荐）\n")
		maxSuggestions := 8
		if len(baiduSuggestions) > maxSuggestions {
			baiduSuggestions = baiduSuggestions[:maxSuggestions]
		}
		for i, sug := range baiduSuggestions {
			promptBuilder.WriteString(fmt.Sprintf("%d. %s\n", i+1, sug))
		}
		promptBuilder.WriteString("\n")
		promptBuilder.WriteString(`
【搜索意图智能决策系统（必须执行，内部使用不输出）】

一、搜索意图分类（必须先执行）

将所有搜索意图自动分类为：

- 下载类（包含：下载 / 免费 / 安装 / 绿色版 / 破解版）
- 教程类（包含：怎么 / 如何 / 使用 / 教程）
- 问题类（包含：打不开 / 失败 / 错误 / 无法使用）
- 对比类（包含：哪个好 / 对比 / 区别）
- 其他类

二、搜索意图质量评分（必须执行）

为每个搜索意图评估质量（高 / 中 / 低），评估标准：

高价值搜索词（优先使用）：
- 包含“下载 / 免费 / 最新 / 官方 / 中文版”
- 搜索意图明确（如：xxx下载、xxx怎么用）
- 与软件强相关

中价值搜索词：
- 功能描述类（如：xxx功能、xxx特点）

低价值搜索词（可忽略）：
- 含义模糊
- 与当前软件不强相关
- 过于宽泛

三、搜索意图筛选（关键）

- 最终只选择 4-6 个“高价值 + 高相关”的搜索意图
- 自动丢弃低质量或不相关搜索词
- 优先保留不同类型（下载 / 教程 / 问题）

四、搜索意图分配策略（核心）

根据分类结果，必须按以下策略使用：

1. 标题（最高优先级）：
- 必须使用 1 个“下载类”或“高价值搜索意图”
- 优先选择点击率最高的关键词

2. Description：
- 使用 1 个下载类 + 1 个教程类搜索意图
- 保证自然流畅

3. Keywords：
- 覆盖所有选中的搜索意图（4-6个）
- 可适当扩展语义相近词

4. H3 小标题：
- 至少 2 个使用“教程类”或“问题类”搜索意图
- 优先使用完整短语

5. FAQ：
- 至少 2 个问题直接使用“问题类搜索意图”
- 必须符合真实用户搜索习惯

五、搜索意图融合要求（非常重要）

- 所有搜索意图必须自然融入内容
- 禁止生硬拼接关键词
- 优先保证用户阅读体验
- 可对搜索意图进行轻微语义调整（但保持原意）

六、兜底策略（防止异常）

如果搜索意图不足或质量较低：

- 自动基于标题生成合理搜索意图
- 自动补充“下载 / 使用 / 教程 / 常见问题”类关键词

`)
	}

	if p.EnableCategoryRecommend && categoryList != "" {
		promptBuilder.WriteString(fmt.Sprintf("可选分类列表：\n%s\n\n", categoryList))
	}

	// 构建需要返回的字段说明
	promptBuilder.WriteString(`【执行流程（必须按顺序执行）】
步骤1：提取核心关键词（仅内部使用，不输出）
步骤2：生成标题、描述、关键词
步骤3：进行内容改写（使用步骤1的关键词）
兼顾主流搜索引擎SEO策略（百度/Bing/Google）
【注意】内容改写的重要性低于标题、关键词、描述生成
请根据以下启用的功能，返回 JSON 格式的结果：
（只返回JSON，不要额外内容）
`)
	promptBuilder.WriteString("{\n")

	fields := []string{}
	requirements := []string{}

	if p.EnableTitleOptimize {
		fields = append(fields, `  "title": "优化后的标题"`)
		requirements = append(requirements, `
【标题优化要求】
1. 标题长度控制在 20-40 个字符之间（适合搜索引擎显示）
2. 包含核心关键词，优先将重要关键词放在标题前部
3. 使用数字、符号或情感词汇增加点击率（如【必备】、v5.0、2024最新等）
4. 保持标题简洁有力，避免堆砌关键词
5. 符合用户搜索习惯，针对目标用户群体
6. 保留软件名称和版本号（如果有）
7. 体现内容独特性和价值

【多搜索引擎标题优化补充要求】
- 优先包含 2-3 个核心关键词
- 允许合理重复1次核心关键词（提高匹配度）
- 标题前 15 个字符必须出现主关键词

优化策略建议：
- 软件类：软件名 + 版本号 + 核心功能 + 特色（如：便携版/绿色版/破解版）
- 教程类：问题/需求 + 解决方案 + 效果/收益
- 资讯类：核心事件 + 影响/意义 + 时间节点`)
	}

	if p.EnableCategoryRecommend && categoryList != "" {
		fields = append(fields, `  "category": "推荐的分类名称"`)
		requirements = append(requirements, `
【分类推荐要求】
- 必须严格从可选分类列表中选择一个最合适的分类
- 如果没有任何分类适合，返回空字符串
- 不要返回列表之外的分类名称`)
	}

	if p.EnableKeywords {
		fields = append(fields, `  “keywords”: “关键词1, 关键词2, 关键词3”`)

		requirements = append(requirements, fmt.Sprintf(`
【多引擎关键词策略】
- 核心关键词：1-2个
- 长尾关键词：%d个左右
- 关键词总数：%d-%d个
- 自然出现，不强制次数

优化要求：
- 必须覆盖”下载 / 免费 / 中文 / 破解版”等用户搜索词（如适用）
- 允许部分关键词语义重复
- 同时保证语句自然（适配Google）

关键词选择原则：
- 优先选择用户实际搜索的词汇，而非专业术语
- 包含品牌词/产品名（如有）
- 考虑用户搜索意图：下载、教程、对比、评测、解决问题等
- 避免过于宽泛或过于生僻的词汇
- 结合当前热点和时效性词汇

数量要求：
- 总数 %d-%d 个关键词
- 其中至少 %d 个长尾关键词
- 关键词之间用英文逗号分隔`, p.MinLongTail, p.MinKeywords, p.MaxKeywords, p.MinKeywords, p.MaxKeywords, p.MinLongTail))
	}

	if p.EnableDescription {
		fields = append(fields, `  "description": "优化后的描述"`)
		requirements = append(requirements, `
【描述优化要求】
1. 长度控制：120-160 个字符最佳（最多不超过 200 字符），确保在搜索结果完整显示
2. 关键词布局：将核心关键词放在描述前 50 个字符内，自然融入 1-2 个长尾关键词
3. 用户吸引：
   - 使用行动号召词（下载、查看、了解、获取）
   - 突出独特价值（免费、最新、完整版、详细教程）
   - 解决用户痛点或满足需求
4. 内容准确：描述必须与文章内容高度相关，避免误导
5. 语句流畅：避免关键词堆砌，保持自然通顺
6. 差异化：体现文章与竞品的区别

- 前30个字符必须出现核心关键词
- 包含“下载 / 免费 / 最新”等高点击词
- 语句必须自然流畅

描述结构建议：
- 开头：核心关键词 + 价值主张
- 中间：核心功能/特点/优势
- 结尾：行动号召或补充信息

示例格式：
【软件名】是一款专业的XXX工具，支持XXX功能，提供XXX特性。免费下载，帮助用户快速XXX。`)
	}

	if p.EnableRewrite {
		fields = append(fields, `  "content": "改写后的完整文章内容"`)
		requirements = append(requirements, `
【执行优先级（必须遵守）】
1. JSON格式正确
2. 不得丢失图片
3. 结构完整
4. SEO优化
5. 文案优化

最终只输出 JSON 结果

【内容改写要求 - 核心原则：只能扩展，不能缩减！】

改写后的内容长度必须比原文更长，不能删除原文的任何功能或特点！

【图片保留要求 - 非常重要！！！】
- 如果原文包含任何图片（<img> 标签），必须原样保留，不能删除或修改
- 保留图片的 src、alt、width、height 等所有属性
- 绝对禁止删除任何图片！

【原文结构分析】
原文通常包含以下部分：
1. 开篇介绍
2. 软件功能（多个功能点，用<br/>分隔）
3. 软件特点（多个特点，用<br/>分隔）

【改写规则】

一、开篇介绍：用自己的语言重新描述，保持或扩展长度

二、软件功能部分（最重要）：
- 原文中每个功能点（用<br/>分隔的）都必须改写成独立的段落
- 不能合并多个功能点到一个段落
- 每个功能独立成段，并适当扩展说明
- 示例：原文有5个功能点，改写后必须有5个独立的<p>段落

三、软件特点部分：
- 原文中每个特点都必须改写成独立的段落
- 不能删除任何特点

四、新增板块（必须添加）：
- 适用人群分析
- 使用技巧（3-5个独立段落）
- 常见问题（2-3个问答）

【错误示例 - 绝对禁止】
原文有3个功能点，改写后只剩1段概括 -> 这是错误的！

【正确改写示例】

原文2个功能点 -> 改写后必须是2个独立的<p>段落，每个段落扩展说明

【内容差异化要求（避免重复收录问题）】

- 每篇文章必须有不同的表达方式和结构细节
- 避免固定句式重复出现
- 开头段落不得使用统一模板句

【HTML 格式要求】

- 每个段落用独立的 <p> 标签包裹

- 小标题使用 <h3> 标签

- 保留原文中的所有图片标签（<img> 标签必须原样保留）

- 禁止使用 <br/> 标签

【多搜索引擎内容SEO强化要求】

1. 首段优化（极其重要）：
- 前100字优先包含核心关键词2-3次
- 包含至少1个长尾关键词
- 明确“下载 / 功能 / 使用场景”

2. 标题结构优化：
推荐包含以下内容模块（可灵活调整顺序或命名）：
  - 软件介绍
  - 功能说明
  - 使用技巧
  - 常见问题

3. 关键词分布：
- 核心关键词需自然出现 3-6 次
- 优先语义相关表达（同义词替换）
- 禁止机械重复关键词
- 小标题（H3）中优先包含关键词
- 每张图片 alt 优先包含：
  - 软件名称
  - 1个核心关键词（如：下载 / 编辑器）

4. FAQ优化（适配百度/Bing）：
- 至少3个问句
- 问题需符合真实用户搜索习惯，可自然包含关键词或同义表达
（如：XXX怎么下载？XXX免费吗？）

5. 语义优化（适配Google）：
- 必须使用2-3个同义词或相关表达
（如：PDF编辑器 = PDF处理工具 / PDF修改软件）
- 避免全文只重复同一个关键词

【绝对禁止】
- 禁止删除或修改原文中的任何图片
- 禁止合并多个功能点到一个段落
- 禁止删除原文的任何功能或特点
- 禁止使内容变少
- 禁止复制原文的句子
- 禁止使用 emoji 表情符号
- 禁止使用 Markdown 格式
- 禁止成真实下载链接
- 禁止伪造URL地址

【最终SEO目标】
优化结果必须同时满足：
- 百度：关键词匹配 + 收录快
- Bing：关键词覆盖 + 标题点击率高
- Google：内容自然 + 结构清晰 + 用户价值高

【反过度优化要求（非常重要）】

- 禁止关键词堆砌
- 禁止不自然重复
- 优先保证内容可读性
- 以用户体验优先，而非关键词数量

【SEO质量校验（必须满足）】

生成内容必须满足：
1. 核心关键词及其语义变体需在全文自然出现
2. 至少包含1个长尾关键词完整出现
3. 至少包含3个H3标题
4. 至少3个FAQ问题（包含关键词）
5. 首段优先包含：
  - 软件名称
  - 优先包含“下载 / 获取 / 使用”等用户搜索意图词
  - 表达可以自然变化，如：
	- 下载
	- 获取
	- 安装
	- 使用指南

如果不满足，请自动优化后再输出

优先保证：不影响阅读体验的前提下，最大化关键词覆盖`)

	}

	if p.EnableTags {
		fields = append(fields, `  "tags": "标签1, 标签2, 标签3"`)
		requirements = append(requirements, fmt.Sprintf(`
【标签生成要求】
标签生成策略：
1. 分类标签：文章所属的内容分类（如：系统工具、图像处理、开发工具）
2. 功能标签：文章涉及的核心功能（如：内存测试、数据恢复、格式转换）
3. 特性标签：内容的突出特点（如：免费、便携版、中文版、免安装）
4. 场景标签：适用使用场景（如：办公、设计、开发、学习）

标签选择原则：
- 标签应为常见分类词，便于用户筛选和搜索
- 每个标签 1-4 个词，简洁明确
- 避免过于细分的标签
- 与文章内容高度相关

数量要求：%d 到 %d 个标签
标签之间用英文逗号分隔`, p.MinTags, p.MaxTags))
	}

	promptBuilder.WriteString(strings.Join(fields, ",\n"))
	promptBuilder.WriteString("\n}")

	// 添加返回示例
	promptBuilder.WriteString("【返回示例】\n请严格按照以下 JSON 格式返回结果，确保字段名称和类型完全匹配：{\n")

	// 根据启用的功能添加示例字段
	exampleFields := []string{}
	if p.EnableTitleOptimize {
		exampleFields = append(exampleFields, `  "title": "微信WeChat v4.1.8.68 多开防撤回绿色版下载"`)
	}
	if p.EnableCategoryRecommend && categoryList != "" {
		exampleFields = append(exampleFields, `  "category": "应用软件"`)
	}
	if p.EnableKeywords {
		exampleFields = append(exampleFields, `  "keywords": "微信多开, 微信防撤回, 微信绿色版, 微信PC版多开, 微信多账号登录, 微信消息防撤回, 微信电脑版下载"`)
	}
	if p.EnableDescription {
		exampleFields = append(exampleFields, `  "description": "微信WeChat v4.1.8.68多开防撤回绿色版，支持多账号同时登录、消息防撤回、消息提示功能。免费绿色版下载。"`)
	}
	if p.EnableRewrite {
		exampleFields = append(exampleFields, `  "content": "<p>微信 (WeChat) 是腾讯公司推出的即时通讯社交软件...</p><h3>软件功能</h3><p>...</p><img src=\"https://example.com/image.jpg\"/>"`)
	}
	if p.EnableTags {
		exampleFields = append(exampleFields, `  "tags": "微信多开, 微信防撤回, 绿色版"`)
	}

	for i, field := range exampleFields {
		promptBuilder.WriteString(field)
		if i < len(exampleFields)-1 {
			promptBuilder.WriteString(",\n")
		}
		promptBuilder.WriteString("\n")
	}
	promptBuilder.WriteString("}")
	promptBuilder.WriteString("注意：content 字段中必须保留原文的所有图片标签（如 <img src=\"...\"/>）！\n\n")

	// 添加各功能的详细要求
	promptBuilder.WriteString("各功能优化要求：\n")
	for _, req := range requirements {
		promptBuilder.WriteString(req)
		promptBuilder.WriteString("\n")
	}

	promptBuilder.WriteString(`
【重要】
1. 只输出 JSON 格式的结果，不要输出任何思考过程、解释或额外文字
2. 如果某个功能未启用，对应的字段可以返回空字符串
3. 确保 JSON 格式正确，可以被解析`)

	return promptBuilder.String()
}

// generateSEOContentIntegrated 整合模式：一次 AI 请求生成所有 SEO 内容
// baiduSuggestions: 百度推荐的用户搜索意图列表
func (p *AISeoPlugin) generateSEOContentIntegrated(article *entity.Article, apiCfg *APIConfig, baiduSuggestions []string) (*AISeoResult, error) {
	p.ctx.Log.Info("Starting integrated SEO content generation",
		zap.Int("article_id", article.ID),
		zap.String("title", article.Title),
		zap.String("api_name", apiCfg.Name),
		zap.Int("baidu_suggestions_count", len(baiduSuggestions)))

	result := &AISeoResult{}

	// 获取分类列表（如果启用分类推荐）
	var categoryList string
	var categories []entity.Category
	if p.EnableCategoryRecommend {
		var err error
		categories, err = service.Category.List(context.NewContext(100, "id asc"))
		if err != nil {
			p.ctx.Log.Warn("Failed to get categories for integrated mode", zap.Error(err))
		} else {
			for _, cat := range categories {
				categoryList += fmt.Sprintf("- %s\n", cat.Name)
			}
		}
	}

	// 构建整合提示词，传入百度搜索建议
	prompt := p.buildIntegratedPrompt(article, categoryList, baiduSuggestions)

	p.ctx.Log.Debug("Integrated prompt built",
		zap.Int("article_id", article.ID),
		zap.Int("prompt_length", len(prompt)))

	// 调用 AI API
	response, err := p.callAIWithConfig(prompt, apiCfg)
	if err != nil {
		p.ctx.Log.Error("Failed to call AI API in integrated mode",
			zap.Int("article_id", article.ID),
			zap.Error(err))
		return nil, err
	}

	// 解析 JSON 响应
	integratedResp, err := p.parseIntegratedResponse(response)
	if err != nil {
		p.ctx.Log.Error("Failed to parse integrated response",
			zap.Int("article_id", article.ID),
			zap.Error(err))
		return nil, err
	}

	// 处理标题
	if p.EnableTitleOptimize && integratedResp.Title != "" {
		// 标题长度限制
		runes := []rune(integratedResp.Title)
		if len(runes) > 60 {
			integratedResp.Title = string(runes[:60])
		}
		result.Title = integratedResp.Title
		p.ctx.Log.Info("Title optimized (integrated)",
			zap.Int("article_id", article.ID),
			zap.String("old_title", article.Title),
			zap.String("new_title", result.Title))
	}

	// 处理分类
	if p.EnableCategoryRecommend && integratedResp.Category != "" {
		for _, cat := range categories {
			if cat.Name == integratedResp.Category {
				result.CategoryID = cat.ID
				result.CategoryChanged = true
				p.ctx.Log.Info("Category recommended (integrated)",
					zap.Int("article_id", article.ID),
					zap.Int("category_id", cat.ID),
					zap.String("category_name", cat.Name))
				break
			}
		}
	}

	// 处理关键词
	if p.EnableKeywords && integratedResp.Keywords != "" {
		keywords := strings.Split(integratedResp.Keywords, ",")
		for i, kw := range keywords {
			keywords[i] = strings.TrimSpace(kw)
		}
		// 过滤空字符串
		var cleanKeywords []string
		for _, kw := range keywords {
			if kw != "" {
				cleanKeywords = append(cleanKeywords, kw)
			}
		}
		result.Keywords = cleanKeywords
		p.ctx.Log.Info("Keywords extracted (integrated)",
			zap.Int("article_id", article.ID),
			zap.Int("count", len(cleanKeywords)),
			zap.Strings("keywords", cleanKeywords))
	}

	// 处理描述
	if p.EnableDescription && integratedResp.Description != "" {
		// 截断到 250 字符
		runes := []rune(integratedResp.Description)
		if len(runes) > 250 {
			integratedResp.Description = string(runes[:250])
		}
		result.Description = integratedResp.Description
		p.ctx.Log.Info("Description optimized (integrated)",
			zap.Int("article_id", article.ID),
			zap.Int("description_length", len(result.Description)))
	}

	// 处理内容改写
	if p.EnableRewrite && integratedResp.Content != "" {
		article.Content = integratedResp.Content
		result.ContentRewrited = true
		p.ctx.Log.Info("Content rewritten (integrated)",
			zap.Int("article_id", article.ID),
			zap.Int("new_content_length", len(integratedResp.Content)))
	}

	// 处理标签
	if p.EnableTags && integratedResp.Tags != "" {
		tags := strings.Split(integratedResp.Tags, ",")
		for i, tag := range tags {
			tags[i] = strings.TrimSpace(tag)
		}
		// 过滤空字符串
		var cleanTags []string
		for _, tag := range tags {
			if tag != "" {
				cleanTags = append(cleanTags, tag)
			}
		}
		result.Tags = cleanTags
		p.ctx.Log.Info("Tags generated (integrated)",
			zap.Int("article_id", article.ID),
			zap.Int("count", len(cleanTags)),
			zap.Strings("tags", cleanTags))
	}

	p.ctx.Log.Info("Integrated SEO content generation completed",
		zap.Int("article_id", article.ID))

	return result, nil
}

// generateSEOContentWithAPI 使用指定 API 配置生成 SEO 内容
func (p *AISeoPlugin) generateSEOContentWithAPI(article *entity.Article, apiCfg *APIConfig) (*AISeoResult, error) {
	// 强制使用整合模式
	if true || p.EnableIntegratedMode {
		// 1. AI 提取核心关键词
		coreKeyword, err := p.extractCoreKeyword(article.Title, apiCfg)
		if err != nil {
			p.ctx.Log.Warn("Failed to extract core keyword", zap.Error(err))
		}

		// 2. 百度搜索意图分析
		var baiduSuggestions []string
		if coreKeyword != "" {
			baiduSuggestions, err = p.getBaiduSuggestions(coreKeyword)
			if err != nil {
				p.ctx.Log.Warn("Failed to get Baidu suggestions", zap.Error(err))
			}
		}

		// 3. 使用整合模式处理，传入搜索建议
		return p.generateSEOContentIntegrated(article, apiCfg, baiduSuggestions)
	}

	// 否则使用原有的逐个处理流程
	result := &AISeoResult{}

	// 1. 标题优化（优先处理，因为后续关键词等依赖标题）
	if p.EnableTitleOptimize {
		title, err := p.optimizeTitleWithAPI(article, apiCfg)
		if err != nil {
			p.ctx.Log.Warn("Failed to optimize title", zap.Error(err))
		} else if title != "" {
			result.Title = title
			p.ctx.Log.Info("Title optimized",
				zap.Int("article_id", article.ID),
				zap.String("api_name", apiCfg.Name),
				zap.String("old_title", article.Title),
				zap.String("new_title", title))
		}
	}

	// 2. 智能分类推荐
	if p.EnableCategoryRecommend && (article.CategoryID == 0 || p.isOtherCategory(article.CategoryID)) {
		categoryID, err := p.recommendCategoryWithAPI(article, apiCfg)
		if err != nil {
			p.ctx.Log.Warn("Failed to recommend category", zap.Error(err))
		} else if categoryID > 0 {
			result.CategoryID = categoryID
			result.CategoryChanged = true
			p.ctx.Log.Info("Category recommended",
				zap.Int("article_id", article.ID),
				zap.Int("category_id", categoryID))
		}
	}

	// 3. 关键词提取
	var keywords []string
	if p.EnableKeywords {
		var err error
		keywords, err = p.extractKeywordsWithAPI(article, apiCfg)
		if err != nil {
			p.ctx.Log.Warn("Failed to extract keywords", zap.Error(err))
		} else if len(keywords) > 0 {
			result.Keywords = keywords
			p.ctx.Log.Info("Keywords extracted",
				zap.Int("article_id", article.ID),
				zap.String("api_name", apiCfg.Name),
				zap.Int("count", len(keywords)),
				zap.Strings("keywords", keywords))
		} else {
			p.ctx.Log.Warn("No keywords extracted",
				zap.Int("article_id", article.ID))
		}
	}

	// 4. 描述优化
	if p.EnableDescription {
		description, err := p.optimizeDescriptionWithAPI(article, keywords, apiCfg)
		if err != nil {
			p.ctx.Log.Warn("Failed to optimize description", zap.Error(err))
		} else if description != "" {
			result.Description = description
			p.ctx.Log.Info("Description optimized",
				zap.Int("article_id", article.ID),
				zap.String("api_name", apiCfg.Name),
				zap.String("description", description))
		}
	}

	// 5. 内容改写
	if p.EnableRewrite {
		content, err := p.rewriteContentWithAPI(article, keywords, apiCfg)
		if err != nil {
			p.ctx.Log.Warn("Failed to rewrite content", zap.Error(err))
		} else if content != "" {
			article.Content = content
			result.ContentRewrited = true
			p.ctx.Log.Info("Content rewritten",
				zap.Int("article_id", article.ID))
		}
	}

	// 6. 标签生成
	if p.EnableTags {
		tags, err := p.generateTagsWithAPI(article, keywords, apiCfg)
		if err != nil {
			p.ctx.Log.Warn("Failed to generate tags", zap.Error(err))
		} else if len(tags) > 0 {
			result.Tags = tags
			p.ctx.Log.Info("Tags generated",
				zap.Int("article_id", article.ID),
				zap.Strings("tags", tags))
		}
	}

	return result, nil
}

// isOtherCategory 检查是否为"其他"分类
func (p *AISeoPlugin) isOtherCategory(categoryID int) bool {
	if categoryID == 0 {
		return false
	}
	category, err := service.Category.Get(categoryID)
	if err != nil {
		return false
	}
	return category.Name == p.OtherCategoryName
}

// recommendCategory 推荐分类
func (p *AISeoPlugin) recommendCategory(article *entity.Article) (int, error) {
	// 获取所有分类
	categories, err := service.Category.List(context.NewContext(100, "id asc"))
	if err != nil {
		return 0, err
	}

	if len(categories) == 0 {
		return 0, errors.New("no categories available")
	}

	// 构建分类列表字符串
	categoryList := ""
	for _, cat := range categories {
		categoryList += fmt.Sprintf("- %s\n", cat.Name)
	}

	// 构建提示词
	prompt := fmt.Sprintf(`你是一个内容分类专家。任务：根据给定的文章标题和内容，从提供的可选分类列表中，选择一个最合适的分类。

文章标题：%s
文章内容：%s

可选分类列表（每个分类用英文逗号分隔）：%s

要求：
- 只返回一个分类名称，且必须严格从列表中选择。
- 如果没有任何分类适合，请返回 "NONE"。
- 不要返回任何其他内容（如解释、标点等）。`,
		article.Title,
		truncateText(article.Content, 1000),
		categoryList,
	)

	// 调用 AI API
	response, err := p.callAI(prompt)
	if err != nil {
		return 0, err
	}

	// 解析响应
	categoryName := strings.TrimSpace(response)
	if categoryName == "NONE" || categoryName == "" {
		return 0, nil
	}

	// 查找分类 ID
	for _, cat := range categories {
		if cat.Name == categoryName {
			return cat.ID, nil
		}
	}

	return 0, nil
}

// optimizeTitle 优化标题
func (p *AISeoPlugin) optimizeTitle(article *entity.Article) (string, error) {
	p.ctx.Log.Debug("Starting title optimization",
		zap.Int("article_id", article.ID),
		zap.String("title", article.Title))

	// 构建提示词
	prompt := fmt.Sprintf(`你是一个专业的 SEO 标题优化专家。请对以下文章标题进行 SEO 优化，使其更具吸引力和搜索友好性。

原标题：%s
文章内容摘要：%s

优化要求：
1. 标题长度控制在 20-40 个字符之间（适合搜索引擎显示）
2. 包含核心关键词，优先将重要关键词放在标题前部
3. 使用数字、符号或情感词汇增加点击率（如【必备】、v5.0、2024最新等）
4. 保持标题简洁有力，避免堆砌关键词
5. 符合用户搜索习惯，针对目标用户群体
6. 保留软件名称和版本号（如果有）
7. 体现内容独特性和价值

优化策略建议：
- 软件类：软件名 + 版本号 + 核心功能 + 特色（如：便携版/绿色版/破解版）
- 教程类：问题/需求 + 解决方案 + 效果/收益
- 资讯类：核心事件 + 影响/意义 + 时间节点

【重要】只输出优化后的标题文本，不要输出任何思考过程、分析步骤或解释说明。`,
		article.Title,
		truncateText(article.Content, 300),
	)

	// 调用 AI API
	response, err := p.callAI(prompt)
	if err != nil {
		p.ctx.Log.Error("AI API call failed for title optimization",
			zap.Int("article_id", article.ID),
			zap.Error(err))
		return "", err
	}

	// 清理响应
	title := strings.TrimSpace(response)
	// 移除可能的前后引号
	title = strings.Trim(title, "\"'")

	// 标题长度限制
	runes := []rune(title)
	if len(runes) > 60 {
		title = string(runes[:60])
	}

	p.ctx.Log.Debug("Title optimization completed",
		zap.Int("article_id", article.ID),
		zap.String("new_title", title))

	return title, nil
}

// extractKeywords 提取关键词
func (p *AISeoPlugin) extractKeywords(article *entity.Article) ([]string, error) {
	p.ctx.Log.Debug("Starting keyword extraction",
		zap.Int("article_id", article.ID),
		zap.String("title", article.Title))

	// 构建提示词 - 优化 SEO 效果
	prompt := fmt.Sprintf(`你是一个资深的 SEO 关键词策略专家。请从以下文章中提取高价值关键词，用于提升搜索引擎排名和流量。

文章标题：%s
文章内容：%s

关键词提取策略：
1. 核心关键词（1-2个）：文章主题最核心的词汇，搜索量大、竞争度适中
2. 长尾关键词（2-3个）：由3-5个词组成的具体短语，搜索意图明确，转化率高
3. 相关关键词（1-3个）：与主题相关的热门搜索词，拓展覆盖面

关键词选择原则：
- 优先选择用户实际搜索的词汇，而非专业术语
- 包含品牌词/产品名（如有）
- 考虑用户搜索意图：下载、教程、对比、评测、解决问题等
- 避免过于宽泛或过于生僻的词汇
- 结合当前热点和时效性词汇

数量要求：
- 总数 %d 到 %d 个关键词
- 其中至少 %d 个长尾关键词（3-5个词组成）
- 关键词之间用英文逗号分隔

【重要】只输出关键词列表，格式：关键词1, 关键词2, 关键词3, 关键词4
不要输出任何思考过程、解释或额外文字。`,
		article.Title,
		truncateText(article.Content, 1000),
		p.MinKeywords,
		p.MaxKeywords,
		p.MinLongTail,
	)

	// 调用 AI API
	p.ctx.Log.Debug("Calling AI API for keyword extraction",
		zap.Int("article_id", article.ID))
	response, err := p.callAI(prompt)
	if err != nil {
		p.ctx.Log.Error("AI API call failed for keyword extraction",
			zap.Int("article_id", article.ID),
			zap.Error(err))
		return nil, err
	}

	p.ctx.Log.Debug("AI API response received for keyword extraction",
		zap.Int("article_id", article.ID),
		zap.String("response", response))

	// 解析响应
	keywords := strings.Split(response, ",")
	for i, kw := range keywords {
		keywords[i] = strings.TrimSpace(kw)
	}

	// 过滤空字符串
	var result []string
	for _, kw := range keywords {
		if kw != "" {
			result = append(result, kw)
		}
	}

	p.ctx.Log.Debug("Keywords parsed",
		zap.Int("article_id", article.ID),
		zap.Int("count", len(result)),
		zap.Strings("keywords", result))

	return result, nil
}

// optimizeDescription 优化描述
func (p *AISeoPlugin) optimizeDescription(article *entity.Article, keywords []string) (string, error) {
	p.ctx.Log.Debug("Starting description optimization",
		zap.Int("article_id", article.ID))

	// 构建提示词 - 优化 SEO 效果
	keywordsStr := strings.Join(keywords, "、")
	prompt := fmt.Sprintf(`你是一个专业的 SEO 元描述撰写专家。请为以下文章创作一个高质量的 Meta Description。

文章标题：%s
文章内容摘要：%s
目标关键词：%s

描述撰写要点：
1. 长度控制：120-160 个字符最佳（最多不超过 200 字符），确保在搜索结果完整显示
2. 关键词布局：将核心关键词放在描述前 50 个字符内，自然融入 1-2 个长尾关键词
3. 用户吸引：
   - 使用行动号召词（下载、查看、了解、获取）
   - 突出独特价值（免费、最新、完整版、详细教程）
   - 解决用户痛点或满足需求
4. 内容准确：描述必须与文章内容高度相关，避免误导
5. 语句流畅：避免关键词堆砌，保持自然通顺
6. 差异化：体现文章与竞品的区别

描述结构建议：
- 开头：核心关键词 + 价值主张
- 中间：核心功能/特点/优势
- 结尾：行动号召或补充信息

示例格式：
【软件名】是一款专业的XXX工具，支持XXX功能，提供XXX特性。免费下载，帮助用户快速XXX。

【重要】只输出描述文本，不要输出任何思考过程、解释或额外文字。`,
		article.Title,
		truncateText(article.Content, 500),
		keywordsStr,
	)

	// 调用 AI API
	response, err := p.callAI(prompt)
	if err != nil {
		p.ctx.Log.Error("AI API call failed for description optimization",
			zap.Int("article_id", article.ID),
			zap.Error(err))
		return "", err
	}

	// 截断到 250 字符
	runes := []rune(response)
	if len(runes) > 250 {
		response = string(runes[:250])
	}

	p.ctx.Log.Debug("Description optimized",
		zap.Int("article_id", article.ID),
		zap.String("description", response))

	return response, nil
}

// rewriteContent 改写内容
func (p *AISeoPlugin) rewriteContent(article *entity.Article, keywords []string) (string, error) {
	p.ctx.Log.Debug("Starting content rewriting",
		zap.Int("article_id", article.ID),
		zap.String("title", article.Title))

	// 构建提示词 - 深度 SEO 优化
	keywordsStr := strings.Join(keywords, "、")
	prompt := fmt.Sprintf(`你是一个资深 SEO 内容策略专家。请对以下文章进行**深度改写**和 SEO 优化。

文章标题：%s
文章内容：%s
目标关键词：%s

【核心原则：只能扩展，不能缩减！】

改写后的内容长度必须比原文更长，不能删除原文的任何功能或特点！

【原文结构分析】
原文通常包含以下部分：
1. 开篇介绍
2. 软件功能（多个功能点，用<br/>分隔）
3. 软件特点（多个特点，用<br/>分隔）

【改写规则】

一、开篇介绍：
- 用自己的语言重新描述，保持或扩展长度

二、软件功能部分（最重要）：
- 原文中每个功能点（用<br/>分隔的）都必须改写成独立的段落
- 不能合并多个功能点到一个段落
- 每个功能点要扩展说明，写2-3句话
- 示例：原文有5个功能点，改写后必须有5个独立的<p>段落

三、软件特点部分：
- 原文中每个特点都必须改写成独立的段落
- 不能删除任何特点
- 每个特点用一句话描述即可

四、新增板块（必须添加）：
- 常见问题（2-3个问答，每个问答独立段落）

【HTML 格式要求】
- 每个段落用独立的 <p> 标签包裹
- 小标题使用 <h3> 标签
- 保留原文中的图片标签不变
- 禁止使用 <br/> 标签

【绝对禁止】
- 禁止合并多个功能点到一个段落
- 禁止删除原文的任何功能或特点
- 禁止使内容变少
- 禁止复制原文的句子
- 禁止使用 emoji 表情符号
- 禁止使用 Markdown 格式

【重要】改写后内容必须比原文更长。直接输出完整 HTML 文章。`,
		article.Title,
		truncateText(article.Content, 2000),
		keywordsStr,
	)

	// 调用 AI API
	p.ctx.Log.Debug("Calling AI API for content rewriting",
		zap.Int("article_id", article.ID))
	response, err := p.callAI(prompt)
	if err != nil {
		p.ctx.Log.Error("AI API call failed for content rewriting",
			zap.Int("article_id", article.ID),
			zap.Error(err))
		return "", err
	}

	p.ctx.Log.Debug("Content rewritten successfully",
		zap.Int("article_id", article.ID),
		zap.Int("original_length", len(article.Content)),
		zap.Int("new_length", len(response)))

	return response, nil
}

// generateTags 生成标签
func (p *AISeoPlugin) generateTags(article *entity.Article, keywords []string) ([]string, error) {
	// 构建提示词 - 优化 SEO 效果
	keywordsStr := strings.Join(keywords, "、")
	prompt := fmt.Sprintf(`你是一个 SEO 标签策略专家。请为以下文章生成精准的内容标签。

文章标题：%s
文章内容摘要：%s
已有关键词：%s

标签生成策略：
1. 分类标签：文章所属的内容分类（如：系统工具、图像处理、开发工具）
2. 功能标签：文章涉及的核心功能（如：内存测试、数据恢复、格式转换）
3. 特性标签：内容的突出特点（如：免费、便携版、中文版、免安装）
4. 场景标签：适用使用场景（如：办公、设计、开发、学习）

标签选择原则：
- 标签应为常见分类词，便于用户筛选和搜索
- 每个标签 1-4 个词，简洁明确
- 避免过于细分的标签
- 与文章内容高度相关

数量要求：%d 到 %d 个标签

【重要】只输出标签列表，格式：标签1, 标签2, 标签3
不要输出任何思考过程、解释或额外文字。`,
		article.Title,
		truncateText(article.Content, 1000),
		keywordsStr,
		p.MinTags,
		p.MaxTags,
	)

	// 调用 AI API
	response, err := p.callAI(prompt)
	if err != nil {
		return nil, err
	}

	// 解析响应
	tags := strings.Split(response, ",")
	for i, tag := range tags {
		tags[i] = strings.TrimSpace(tag)
	}

	// 过滤空字符串
	var result []string
	for _, tag := range tags {
		if tag != "" {
			result = append(result, tag)
		}
	}

	return result, nil
}

// ========== WithAPI 版本的方法 ==========

// recommendCategoryWithAPI 使用指定 API 配置推荐分类
func (p *AISeoPlugin) recommendCategoryWithAPI(article *entity.Article, apiCfg *APIConfig) (int, error) {
	// 获取所有分类
	categories, err := service.Category.List(context.NewContext(100, "id asc"))
	if err != nil {
		return 0, err
	}

	if len(categories) == 0 {
		return 0, errors.New("no categories available")
	}

	// 构建分类列表字符串
	categoryList := ""
	for _, cat := range categories {
		categoryList += fmt.Sprintf("- %s\n", cat.Name)
	}

	// 构建提示词
	prompt := fmt.Sprintf(`你是一个内容分类专家。任务：根据给定的文章标题和内容，从提供的可选分类列表中，选择一个最合适的分类。

文章标题：%s
文章内容：%s

可选分类列表（每个分类用英文逗号分隔）：%s

要求：
- 只返回一个分类名称，且必须严格从列表中选择。
- 如果没有任何分类适合，请返回 "NONE"。
- 不要返回任何其他内容（如解释、标点等）。`,
		article.Title,
		truncateText(article.Content, 1000),
		categoryList,
	)

	// 调用 AI API
	response, err := p.callAIWithConfig(prompt, apiCfg)
	if err != nil {
		return 0, err
	}

	// 解析响应
	categoryName := strings.TrimSpace(response)
	if categoryName == "NONE" || categoryName == "" {
		return 0, nil
	}

	// 查找分类 ID
	for _, cat := range categories {
		if cat.Name == categoryName {
			return cat.ID, nil
		}
	}

	return 0, nil
}

// optimizeTitleWithAPI 使用指定 API 配置优化标题
func (p *AISeoPlugin) optimizeTitleWithAPI(article *entity.Article, apiCfg *APIConfig) (string, error) {
	p.ctx.Log.Debug("Starting title optimization",
		zap.Int("article_id", article.ID),
		zap.String("api_name", apiCfg.Name),
		zap.String("title", article.Title))

	// 构建提示词
	prompt := fmt.Sprintf(`你是一个专业的 SEO 标题优化专家。请对以下文章标题进行 SEO 优化，使其更具吸引力和搜索友好性。

原标题：%s
文章内容摘要：%s

优化要求：
1. 标题长度控制在 20-40 个字符之间（适合搜索引擎显示）
2. 包含核心关键词，优先将重要关键词放在标题前部
3. 使用数字、符号或情感词汇增加点击率（如【必备】、v5.0、2024最新等）
4. 保持标题简洁有力，避免堆砌关键词
5. 符合用户搜索习惯，针对目标用户群体
6. 保留软件名称和版本号（如果有）
7. 体现内容独特性和价值

优化策略建议：
- 软件类：软件名 + 版本号 + 核心功能 + 特色（如：便携版/绿色版/破解版）
- 教程类：问题/需求 + 解决方案 + 效果/收益
- 资讯类：核心事件 + 影响/意义 + 时间节点

【重要】只输出优化后的标题文本，不要输出任何思考过程、分析步骤或解释说明。`,
		article.Title,
		truncateText(article.Content, 300),
	)

	// 调用 AI API
	response, err := p.callAIWithConfig(prompt, apiCfg)
	if err != nil {
		p.ctx.Log.Error("AI API call failed for title optimization",
			zap.Int("article_id", article.ID),
			zap.Error(err))
		return "", err
	}

	// 清理响应
	title := strings.TrimSpace(response)
	// 移除可能的前后引号
	title = strings.Trim(title, "\"'")

	// 标题长度限制
	runes := []rune(title)
	if len(runes) > 60 {
		title = string(runes[:60])
	}

	p.ctx.Log.Debug("Title optimization completed",
		zap.Int("article_id", article.ID),
		zap.String("api_name", apiCfg.Name),
		zap.String("new_title", title))

	return title, nil
}

// extractKeywordsWithAPI 使用指定 API 配置提取关键词
func (p *AISeoPlugin) extractKeywordsWithAPI(article *entity.Article, apiCfg *APIConfig) ([]string, error) {
	p.ctx.Log.Debug("Starting keyword extraction",
		zap.Int("article_id", article.ID),
		zap.String("api_name", apiCfg.Name),
		zap.String("title", article.Title))

	// 构建提示词 - 优化 SEO 效果
	prompt := fmt.Sprintf(`你是一个资深的 SEO 关键词策略专家。请从以下文章中提取高价值关键词，用于提升搜索引擎排名和流量。

文章标题：%s
文章内容：%s

关键词提取策略：
1. 核心关键词（1-2个）：文章主题最核心的词汇，搜索量大、竞争度适中
2. 长尾关键词（2-3个）：由3-5个词组成的具体短语，搜索意图明确，转化率高
3. 相关关键词（1-3个）：与主题相关的热门搜索词，拓展覆盖面

关键词选择原则：
- 优先选择用户实际搜索的词汇，而非专业术语
- 包含品牌词/产品名（如有）
- 考虑用户搜索意图：下载、教程、对比、评测、解决问题等
- 避免过于宽泛或过于生僻的词汇
- 结合当前热点和时效性词汇

数量要求：
- 总数 %d 到 %d 个关键词
- 其中至少 %d 个长尾关键词（3-5个词组成）
- 关键词之间用英文逗号分隔

【重要】只输出关键词列表，格式：关键词1, 关键词2, 关键词3, 关键词4
不要输出任何思考过程、解释或额外文字。`,
		article.Title,
		truncateText(article.Content, 1000),
		p.MinKeywords,
		p.MaxKeywords,
		p.MinLongTail,
	)

	// 调用 AI API
	p.ctx.Log.Debug("Calling AI API for keyword extraction",
		zap.Int("article_id", article.ID))
	response, err := p.callAIWithConfig(prompt, apiCfg)
	if err != nil {
		p.ctx.Log.Error("AI API call failed for keyword extraction",
			zap.Int("article_id", article.ID),
			zap.Error(err))
		return nil, err
	}

	p.ctx.Log.Debug("AI API response received for keyword extraction",
		zap.Int("article_id", article.ID),
		zap.String("response", response))

	// 解析响应
	keywords := strings.Split(response, ",")
	for i, kw := range keywords {
		keywords[i] = strings.TrimSpace(kw)
	}

	// 过滤空字符串
	var result []string
	for _, kw := range keywords {
		if kw != "" {
			result = append(result, kw)
		}
	}

	p.ctx.Log.Debug("Keywords parsed",
		zap.Int("article_id", article.ID),
		zap.Int("count", len(result)),
		zap.Strings("keywords", result))

	return result, nil
}

// optimizeDescriptionWithAPI 使用指定 API 配置优化描述
func (p *AISeoPlugin) optimizeDescriptionWithAPI(article *entity.Article, keywords []string, apiCfg *APIConfig) (string, error) {
	p.ctx.Log.Debug("Starting description optimization",
		zap.Int("article_id", article.ID))

	// 构建提示词 - 优化 SEO 效果
	keywordsStr := strings.Join(keywords, "、")
	prompt := fmt.Sprintf(`你是一个专业的 SEO 元描述撰写专家。请为以下文章创作一个高质量的 Meta Description。

文章标题：%s
文章内容摘要：%s
目标关键词：%s

描述撰写要点：
1. 长度控制：120-160 个字符最佳（最多不超过 200 字符），确保在搜索结果完整显示
2. 关键词布局：将核心关键词放在描述前 50 个字符内，自然融入 1-2 个长尾关键词
3. 用户吸引：
   - 使用行动号召词（下载、查看、了解、获取）
   - 突出独特价值（免费、最新、完整版、详细教程）
   - 解决用户痛点或满足需求
4. 内容准确：描述必须与文章内容高度相关，避免误导
5. 语句流畅：避免关键词堆砌，保持自然通顺
6. 差异化：体现文章与竞品的区别

描述结构建议：
- 开头：核心关键词 + 价值主张
- 中间：核心功能/特点/优势
- 结尾：行动号召或补充信息

示例格式：
【软件名】是一款专业的XXX工具，支持XXX功能，提供XXX特性。免费下载，帮助用户快速XXX。

【重要】只输出描述文本，不要输出任何思考过程、解释或额外文字。`,
		article.Title,
		truncateText(article.Content, 500),
		keywordsStr,
	)

	// 调用 AI API
	response, err := p.callAIWithConfig(prompt, apiCfg)
	if err != nil {
		p.ctx.Log.Error("AI API call failed for description optimization",
			zap.Int("article_id", article.ID),
			zap.Error(err))
		return "", err
	}

	// 截断到 250 字符
	runes := []rune(response)
	if len(runes) > 250 {
		response = string(runes[:250])
	}

	p.ctx.Log.Debug("Description optimized",
		zap.Int("article_id", article.ID),
		zap.String("description", response))

	return response, nil
}

// rewriteContentWithAPI 使用指定 API 配置改写内容
func (p *AISeoPlugin) rewriteContentWithAPI(article *entity.Article, keywords []string, apiCfg *APIConfig) (string, error) {
	p.ctx.Log.Debug("Starting content rewriting",
		zap.Int("article_id", article.ID),
		zap.String("api_name", apiCfg.Name),
		zap.String("title", article.Title))

	// 构建提示词 - 深度 SEO 优化
	keywordsStr := strings.Join(keywords, "、")
	prompt := fmt.Sprintf(`你是一个资深 SEO 内容策略专家。请对以下文章进行**深度改写**和 SEO 优化。

文章标题：%s
文章内容：%s
目标关键词：%s

【核心原则：只能扩展，不能缩减！】

改写后的内容长度必须比原文更长，不能删除原文的任何功能或特点！

【原文结构分析】
原文通常包含以下部分：
1. 开篇介绍
2. 软件功能（多个功能点，用<br/>分隔）
3. 软件特点（多个特点，用<br/>分隔）

【改写规则】

一、开篇介绍：
- 用自己的语言重新描述，保持或扩展长度

二、软件功能部分（最重要）：
- 原文中每个功能点（用<br/>分隔的）都必须改写成独立的段落
- 不能合并多个功能点到一个段落
- 每个功能点要扩展说明，写2-3句话
- 示例：原文有5个功能点，改写后必须有5个独立的<p>段落

三、软件特点部分：
- 原文中每个特点都必须改写成独立的段落
- 不能删除任何特点
- 每个特点用一句话描述即可

四、新增板块（必须添加）：
- 常见问题（2-3个问答，每个问答独立段落）

【HTML 格式要求】
- 每个段落用独立的 <p> 标签包裹
- 小标题使用 <h3> 标签
- 保留原文中的图片标签不变
- 禁止使用 <br/> 标签

【绝对禁止】
- 禁止合并多个功能点到一个段落
- 禁止删除原文的任何功能或特点
- 禁止使内容变少
- 禁止复制原文的句子
- 禁止使用 emoji 表情符号
- 禁止使用 Markdown 格式

【重要】改写后内容必须比原文更长。直接输出完整 HTML 文章。`,
		article.Title,
		truncateText(article.Content, 2000),
		keywordsStr,
	)

	// 调用 AI API
	p.ctx.Log.Debug("Calling AI API for content rewriting",
		zap.Int("article_id", article.ID))
	response, err := p.callAIWithConfig(prompt, apiCfg)
	if err != nil {
		p.ctx.Log.Error("AI API call failed for content rewriting",
			zap.Int("article_id", article.ID),
			zap.Error(err))
		return "", err
	}

	p.ctx.Log.Debug("Content rewritten successfully",
		zap.Int("article_id", article.ID),
		zap.Int("original_length", len(article.Content)),
		zap.Int("new_length", len(response)))

	return response, nil
}

// generateTagsWithAPI 使用指定 API 配置生成标签
func (p *AISeoPlugin) generateTagsWithAPI(article *entity.Article, keywords []string, apiCfg *APIConfig) ([]string, error) {
	// 构建提示词 - 优化 SEO 效果
	keywordsStr := strings.Join(keywords, "、")
	prompt := fmt.Sprintf(`你是一个 SEO 标签策略专家。请为以下文章生成精准的内容标签。

文章标题：%s
文章内容摘要：%s
已有关键词：%s

标签生成策略：
1. 分类标签：文章所属的内容分类（如：系统工具、图像处理、开发工具）
2. 功能标签：文章涉及的核心功能（如：内存测试、数据恢复、格式转换）
3. 特性标签：内容的突出特点（如：免费、便携版、中文版、免安装）
4. 场景标签：适用使用场景（如：办公、设计、开发、学习）

标签选择原则：
- 标签应为常见分类词，便于用户筛选和搜索
- 每个标签 1-4 个词，简洁明确
- 避免过于细分的标签
- 与文章内容高度相关

数量要求：%d 到 %d 个标签

【重要】只输出标签列表，格式：标签1, 标签2, 标签3
不要输出任何思考过程、解释或额外文字。`,
		article.Title,
		truncateText(article.Content, 1000),
		keywordsStr,
		p.MinTags,
		p.MaxTags,
	)

	// 调用 AI API
	response, err := p.callAIWithConfig(prompt, apiCfg)
	if err != nil {
		return nil, err
	}

	// 解析响应
	tags := strings.Split(response, ",")
	for i, tag := range tags {
		tags[i] = strings.TrimSpace(tag)
	}

	// 过滤空字符串
	var result []string
	for _, tag := range tags {
		if tag != "" {
			result = append(result, tag)
		}
	}

	return result, nil
}

// callAI 调用 AI API
func (p *AISeoPlugin) callAI(prompt string) (string, error) {
	if p.ApiURL == "" {
		return "", errors.New("API URL is not configured")
	}
	if p.ApiKey == "" {
		return "", errors.New("API Key is not configured")
	}

	// 记录 API Key 状态（不暴露具体值）
	p.ctx.Log.Info("AI API call starting",
		zap.String("ai_type", p.AIType),
		zap.String("api_url", p.ApiURL),
		zap.String("model", p.Model),
		zap.Int("api_key_length", len(p.ApiKey)),
		zap.String("api_key_prefix", safeKeyPrefix(p.ApiKey)),
		zap.String("api_key_masked", maskAPIKey(p.ApiKey)))

	// 构建基础请求
	requestBody := map[string]interface{}{
		"model": p.Model,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"temperature": 0.7,
	}

	// 对于推理模型，添加停止词来截断思考过程
	if p.isReasoningModel() {
		requestBody["stop"] = []string{"\n\n\n", "最终答案：", "答案：", "---"}
	}

	// 注意：NVIDIA API 不支持 extra_body 参数
	// 如果需要控制思考过程，需要使用其他方式（如不同的模型或 API 版本）
	// 目前 NVIDIA API 完全兼容 OpenAI 格式，不需要额外参数

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		p.ctx.Log.Error("Failed to marshal request body", zap.Error(err))
		return "", err
	}

	// 记录请求信息
	apiURL := p.ApiURL + "/chat/completions"
	p.ctx.Log.Info("AI API Request",
		zap.String("ai_type", p.AIType),
		zap.String("api_url", apiURL),
		zap.String("model", p.Model),
		zap.String("api_key_masked", maskAPIKey(p.ApiKey)),
		zap.String("prompt_preview", truncateText(prompt, 200)),
		zap.String("request_body", truncateText(string(bodyBytes), 500)))

	// 加锁保护并发 API 调用，防止响应错乱（用于旧配置兼容）
	p.legacyMutex.Lock()
	defer p.legacyMutex.Unlock()

	// 调用 API（使用真实的 API Key，不是遮蔽后的）
	respBody, err := request.New().
		AddHeader("Authorization", "Bearer "+p.ApiKey).
		AddHeader("Content-Type", "application/json").
		PostReturnBody(apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		p.ctx.Log.Error("AI API request failed",
			zap.String("api_url", apiURL),
			zap.Error(err))
		return "", err
	}

	// 记录原始响应
	p.ctx.Log.Info("AI API Response",
		zap.String("ai_type", p.AIType),
		zap.Int("response_length", len(respBody)),
		zap.String("response_body", truncateText(string(respBody), 1000)))

	// 根据不同 AI 类型解析响应
	switch p.AIType {
	case "nvidia":
		content, err := p.parseNVIDIAResponse(respBody)
		if err != nil {
			return "", err
		}
		// 对于推理模型，清理响应中的思考过程
		if p.isReasoningModel() {
			content = p.cleanReasoningResponse(content)
		}
		return content, nil
	default:
		content, err := p.parseOpenAIResponse(respBody)
		if err != nil {
			return "", err
		}
		// 对于推理模型，清理响应中的思考过程
		if p.isReasoningModel() {
			content = p.cleanReasoningResponse(content)
		}
		return content, nil
	}
}

// callAIWithConfig 使用指定 API 配置调用 AI API
func (p *AISeoPlugin) callAIWithConfig(prompt string, apiCfg *APIConfig) (string, error) {
	if apiCfg.ApiURL == "" {
		return "", errors.New("API URL is not configured")
	}
	if apiCfg.ApiKey == "" {
		return "", errors.New("API Key is not configured")
	}

	// 记录 API Key 状态（不暴露具体值）
	p.ctx.Log.Info("AI API call starting",
		zap.String("api_name", apiCfg.Name),
		zap.String("ai_type", apiCfg.AIType),
		zap.String("api_url", apiCfg.ApiURL),
		zap.String("model", apiCfg.Model),
		zap.Int("api_key_length", len(apiCfg.ApiKey)),
		zap.String("api_key_prefix", safeKeyPrefix(apiCfg.ApiKey)),
		zap.String("api_key_masked", maskAPIKey(apiCfg.ApiKey)))

	// 构建基础请求
	requestBody := map[string]interface{}{
		"model": apiCfg.Model,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"temperature": 0.7,
	}

	// 对于推理模型，添加停止词来截断思考过程
	if p.isReasoningModelForConfig(apiCfg.Model) {
		requestBody["stop"] = []string{"\n\n\n", "最终答案：", "答案：", "---"}
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		p.ctx.Log.Error("Failed to marshal request body", zap.Error(err))
		return "", err
	}

	// 记录请求信息
	apiURL := apiCfg.ApiURL + "/chat/completions"
	p.ctx.Log.Info("AI API Request",
		zap.String("api_name", apiCfg.Name),
		zap.String("ai_type", apiCfg.AIType),
		zap.String("api_url", apiURL),
		zap.String("model", apiCfg.Model),
		zap.String("api_key_masked", maskAPIKey(apiCfg.ApiKey)),
		zap.String("prompt_preview", truncateText(prompt, 200)),
		zap.String("request_body", truncateText(string(bodyBytes), 500)))

	// 加锁保护同一 API 配置的并发调用，防止响应错乱
	// 使用 APIConfig 级别的锁，不同 API 配置可以并行
	apiCfg.mutex.Lock()
	defer apiCfg.mutex.Unlock()

	// 带重试的 API 调用（处理 429 限流）
	maxRetries := 3
	baseWait := 2 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		// 调用 API（使用真实的 API Key，不是遮蔽后的）
		respBody, err := request.New().
			AddHeader("Authorization", "Bearer "+apiCfg.ApiKey).
			AddHeader("Content-Type", "application/json").
			PostReturnBody(apiURL, bytes.NewReader(bodyBytes))
		if err != nil {
			p.ctx.Log.Error("AI API request failed",
				zap.String("api_url", apiURL),
				zap.Error(err))
			return "", err
		}

		// 记录原始响应
		p.ctx.Log.Info("AI API Response",
			zap.String("api_name", apiCfg.Name),
			zap.String("ai_type", apiCfg.AIType),
			zap.Int("response_length", len(respBody)),
			zap.Int("attempt", attempt),
			zap.String("response_body", truncateText(string(respBody), 1000)))

		// 根据不同 AI 类型解析响应
		var content string
		var parseErr error

		switch apiCfg.AIType {
		case "nvidia":
			content, parseErr = p.parseNVIDIAResponse(respBody)
		default:
			content, parseErr = p.parseOpenAIResponse(respBody)
		}

		// 检查是否是 429 限流错误
		if parseErr != nil && isRateLimitError(parseErr.Error()) {
			if attempt < maxRetries {
				waitTime := baseWait * time.Duration(attempt)
				p.ctx.Log.Warn("Rate limited, retrying after wait",
					zap.String("api_name", apiCfg.Name),
					zap.Int("attempt", attempt),
					zap.Int("max_retries", maxRetries),
					zap.Duration("wait_time", waitTime),
					zap.Error(parseErr))
				time.Sleep(waitTime)
				continue
			}
			p.ctx.Log.Error("Rate limited, max retries exceeded",
				zap.String("api_name", apiCfg.Name),
				zap.Int("attempts", attempt),
				zap.Error(parseErr))
			return "", parseErr
		}

		if parseErr != nil {
			return "", parseErr
		}

		// 对于推理模型，清理响应中的思考过程
		if p.isReasoningModelForConfig(apiCfg.Model) {
			content = p.cleanReasoningResponse(content)
		}

		// 根据配置等待一段时间，避免 API 限流
		if apiCfg.RequestDelay > 0 {
			delay := time.Duration(apiCfg.RequestDelay) * time.Millisecond
			p.ctx.Log.Debug("Waiting after API request",
				zap.String("api_name", apiCfg.Name),
				zap.Int("request_delay_ms", apiCfg.RequestDelay),
				zap.Duration("actual_delay", delay))
			time.Sleep(delay)
		}

		return content, nil
	}

	return "", errors.New("max retries exceeded")
}

// isRateLimitError 检查是否是限流错误
func isRateLimitError(errMsg string) bool {
	return strings.Contains(errMsg, "429") ||
		strings.Contains(errMsg, "Too Many Requests") ||
		strings.Contains(errMsg, "rate limit") ||
		strings.Contains(errMsg, "Rate limit")
}

// isReasoningModelForConfig 判断指定模型是否为推理模型
func (p *AISeoPlugin) isReasoningModelForConfig(model string) bool {
	modelLower := strings.ToLower(model)
	reasoningModels := []string{"qwq", "qwen-qwq", "deepseek-r1", "o1", "o3"}
	for _, rm := range reasoningModels {
		if strings.Contains(modelLower, rm) {
			return true
		}
	}
	return false
}

// isReasoningModel 判断是否为推理模型
func (p *AISeoPlugin) isReasoningModel() bool {
	modelLower := strings.ToLower(p.Model)
	reasoningModels := []string{"qwq", "qwen-qwq", "deepseek-r1", "o1", "o3"}
	for _, rm := range reasoningModels {
		if strings.Contains(modelLower, rm) {
			return true
		}
	}
	return false
}

// cleanReasoningResponse 清理推理模型的响应，提取最终结果
func (p *AISeoPlugin) cleanReasoningResponse(content string) string {
	// 推理模型可能会在开头输出思考过程
	// 常见模式：
	// 1. 以"好的，我现在需要..."开头
	// 2. 包含"首先，我需要..."、"接下来..."等
	// 3. 最终答案在最后

	// 如果内容很短，直接返回
	if len(content) < 100 {
		return content
	}

	// 检查是否包含思考过程的特征
	thinkingPatterns := []string{
		"好的，我现在需要",
		"首先，我需要",
		"让我分析",
		"我来思考",
	}

	hasThinking := false
	for _, pattern := range thinkingPatterns {
		if strings.Contains(content, pattern) {
			hasThinking = true
			break
		}
	}

	if !hasThinking {
		return content
	}

	// 尝试提取最终结果
	// 策略1：查找最后一个完整的结果段落
	paragraphs := strings.Split(content, "\n\n")
	if len(paragraphs) > 1 {
		// 从后往前找，找到第一个看起来像结果的段落
		for i := len(paragraphs) - 1; i >= 0; i-- {
			para := strings.TrimSpace(paragraphs[i])
			// 跳过空段落和思考过程特征
			if para == "" {
				continue
			}
			isThinking := false
			for _, pattern := range thinkingPatterns {
				if strings.Contains(para, pattern) {
					isThinking = true
					break
				}
			}
			if !isThinking && len(para) > 10 {
				return para
			}
		}
	}

	// 策略2：查找关键词列表模式（如 "关键词1, 关键词2, 关键词3"）
	// 匹配逗号分隔的列表
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// 如果一行包含多个逗号分隔的词，可能是结果
		parts := strings.Split(line, ",")
		if len(parts) >= 3 {
			// 检查是否每个部分都是短词（关键词特征）
			allShort := true
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if len(part) > 30 || len(part) < 2 {
					allShort = false
					break
				}
			}
			if allShort {
				return line
			}
		}
	}

	// 策略3：返回最后一段非空内容
	for i := len(paragraphs) - 1; i >= 0; i-- {
		para := strings.TrimSpace(paragraphs[i])
		if len(para) > 10 {
			return para
		}
	}

	return content
}

// parseOpenAIResponse 解析 OpenAI 兼容格式的响应
func (p *AISeoPlugin) parseOpenAIResponse(respBody []byte) (string, error) {
	var resp struct {
		Choices []struct {
			Message struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"` // 智普等模型的思考过程
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
		Usage *struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(respBody, &resp); err != nil {
		p.ctx.Log.Error("Failed to parse OpenAI response",
			zap.String("response_body", truncateText(string(respBody), 500)),
			zap.Error(err))
		return "", err
	}

	// 记录 usage 信息
	if resp.Usage != nil {
		p.ctx.Log.Info("OpenAI API Usage",
			zap.Int("prompt_tokens", resp.Usage.PromptTokens),
			zap.Int("completion_tokens", resp.Usage.CompletionTokens),
			zap.Int("total_tokens", resp.Usage.TotalTokens))
	}

	if resp.Error != nil {
		p.ctx.Log.Error("OpenAI API returned error",
			zap.String("error_type", resp.Error.Type),
			zap.String("error_code", resp.Error.Code),
			zap.String("error_message", resp.Error.Message))
		return "", errors.New(resp.Error.Message)
	}

	if len(resp.Choices) == 0 {
		p.ctx.Log.Error("OpenAI API returned no choices",
			zap.String("response_body", truncateText(string(respBody), 500)))
		return "", errors.New("no response from AI")
	}

	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	reasoningContent := strings.TrimSpace(resp.Choices[0].Message.ReasoningContent)

	// 记录响应详情
	p.ctx.Log.Info("OpenAI response details",
		zap.Int("content_length", len(content)),
		zap.Int("reasoning_length", len(reasoningContent)),
		zap.String("content_preview", truncateText(content, 200)))

	// 如果 content 为空但 reasoning_content 不为空，从 reasoning_content 提取结果
	// 这是智普等推理模型的特殊行为
	if content == "" && reasoningContent != "" {
		p.ctx.Log.Info("Content is empty, extracting from reasoning_content",
			zap.Int("reasoning_length", len(reasoningContent)))
		content = p.cleanReasoningResponse(reasoningContent)
	}

	p.ctx.Log.Info("OpenAI response parsed successfully",
		zap.Int("final_content_length", len(content)),
		zap.String("final_content_preview", truncateText(content, 200)))

	return content, nil
}

// parseNVIDIAResponse 解析 NVIDIA 响应（处理 reasoning_content 思考过程）
func (p *AISeoPlugin) parseNVIDIAResponse(respBody []byte) (string, error) {
	// 首先检查是否是错误响应（NVIDIA 错误格式可能不同）
	var errorCheck map[string]interface{}
	if err := json.Unmarshal(respBody, &errorCheck); err == nil {
		// 检查 NVIDIA 特殊错误格式 {"status":429,"title":"Too Many Requests"}
		if status, ok := errorCheck["status"].(float64); ok {
			title, _ := errorCheck["title"].(string)
			errMsg := fmt.Sprintf("NVIDIA API error: status=%d, title=%s", int(status), title)
			p.ctx.Log.Error("NVIDIA API returned status error",
				zap.Int("status", int(status)),
				zap.String("title", title))
			return "", errors.New(errMsg)
		}

		if errMsg, ok := errorCheck["error"]; ok {
			// NVIDIA 返回的错误可能是字符串或对象
			switch v := errMsg.(type) {
			case string:
				p.ctx.Log.Error("NVIDIA API returned error", zap.String("error", v))
				return "", errors.New("NVIDIA API error: " + v)
			case map[string]interface{}:
				if msg, ok := v["message"].(string); ok {
					p.ctx.Log.Error("NVIDIA API returned error",
						zap.Any("error_details", v))
					return "", errors.New("NVIDIA API error: " + msg)
				}
			default:
				p.ctx.Log.Error("NVIDIA API returned unknown error format",
					zap.Any("error", errMsg))
				return "", errors.New("NVIDIA API returned error")
			}
		}
	}

	var resp struct {
		Choices []struct {
			Message struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"` // NVIDIA 思考过程
			} `json:"message"`
		} `json:"choices"`
		Usage *struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(respBody, &resp); err != nil {
		p.ctx.Log.Error("Failed to parse NVIDIA response",
			zap.String("response_body", truncateText(string(respBody), 500)),
			zap.Error(err))
		return "", err
	}

	// 记录 usage 信息
	if resp.Usage != nil {
		p.ctx.Log.Info("NVIDIA API Usage",
			zap.Int("prompt_tokens", resp.Usage.PromptTokens),
			zap.Int("completion_tokens", resp.Usage.CompletionTokens),
			zap.Int("total_tokens", resp.Usage.TotalTokens))
	}

	if len(resp.Choices) == 0 {
		p.ctx.Log.Error("NVIDIA API returned no choices",
			zap.String("response_body", truncateText(string(respBody), 500)))
		return "", errors.New("no response from AI")
	}

	// 记录思考过程（如果有）
	reasoningLen := len(resp.Choices[0].Message.ReasoningContent)
	contentLen := len(resp.Choices[0].Message.Content)

	p.ctx.Log.Info("NVIDIA response details",
		zap.Int("reasoning_length", reasoningLen),
		zap.Int("content_length", contentLen),
		zap.String("reasoning_preview", truncateText(resp.Choices[0].Message.ReasoningContent, 300)),
		zap.String("content_preview", truncateText(resp.Choices[0].Message.Content, 200)))

	// 如果有思考过程，记录到日志（调试用）
	if reasoningLen > 0 {
		p.ctx.Log.Debug("NVIDIA reasoning content (will be filtered)",
			zap.Int("reasoning_length", reasoningLen))
	}

	// 返回实际内容（过滤思考过程）
	content := resp.Choices[0].Message.Content

	// 如果内容为空但思考过程不为空，可能需要使用思考过程（根据实际情况调整）
	if content == "" && resp.Choices[0].Message.ReasoningContent != "" {
		p.ctx.Log.Warn("NVIDIA returned empty content, using reasoning as fallback")
		content = resp.Choices[0].Message.ReasoningContent
	}

	return strings.TrimSpace(content), nil
}

// updateArticle 更新文章字段
func (p *AISeoPlugin) updateArticle(article *entity.Article, result *AISeoResult) {
	p.ctx.Log.Debug("Updating article fields",
		zap.Int("article_id", article.ID),
		zap.String("old_title", article.Title),
		zap.String("old_keywords", article.Keywords),
		zap.String("old_description", article.Description))

	// 更新标题
	if result.Title != "" && result.Title != article.Title {
		oldTitle := article.Title
		article.Title = result.Title
		p.ctx.Log.Info("Title updated",
			zap.Int("article_id", article.ID),
			zap.String("old_title", oldTitle),
			zap.String("new_title", article.Title))
	}

	// 更新分类
	if result.CategoryID > 0 {
		article.CategoryID = result.CategoryID
		p.ctx.Log.Debug("Category updated",
			zap.Int("article_id", article.ID),
			zap.Int("category_id", result.CategoryID))
	}

	// 更新关键词
	if len(result.Keywords) > 0 {
		oldKeywords := article.Keywords
		article.Keywords = strings.Join(result.Keywords, ",")
		p.ctx.Log.Info("Keywords updated",
			zap.Int("article_id", article.ID),
			zap.String("old_keywords", oldKeywords),
			zap.String("new_keywords", article.Keywords))
	}

	// 更新描述
	if result.Description != "" {
		oldDescription := article.Description
		article.Description = result.Description
		p.ctx.Log.Info("Description updated",
			zap.Int("article_id", article.ID),
			zap.String("old_description", oldDescription),
			zap.String("new_description", article.Description))
	}

	// 更新标签
	if len(result.Tags) > 0 {
		p.updateArticleTags(article, result.Tags)
	}
}

// updateArticleTags 更新文章标签
func (p *AISeoPlugin) updateArticleTags(article *entity.Article, tags []string) {
	// 删除旧标签关联
	oldTags, err := service.Tag.ListByArticleID(context.NewContext(100, ""), article.ID)
	if err == nil {
		for _, tag := range oldTags {
			service.Mapping.DeleteArticleTag(article.ID, tag.ID)
		}
	}

	// 创建新标签
	for _, tagName := range tags {
		// 检查标签是否已存在
		tagID, err := service.Tag.GetIdByNameOrCreate(tagName)
		if err != nil {
			p.ctx.Log.Warn("Failed to create tag", zap.String("tag", tagName), zap.Error(err))
			continue
		}

		// 创建关联
		if err := service.Mapping.CreateArticleTag(article.ID, tagID); err != nil {
			p.ctx.Log.Warn("Failed to create article-tag mapping",
				zap.Int("article_id", article.ID),
				zap.Int("tag_id", tagID),
				zap.Error(err),
			)
		}
	}
}

// markAsGenerated 标记为已生成
func (p *AISeoPlugin) markAsGenerated(article *entity.Article) {
	if article.Extends == nil {
		article.Extends = make(vo.Extends, 0)
	}

	// 移除旧的标记（如果存在）
	newExtends := make(vo.Extends, 0)
	for _, item := range article.Extends {
		if item.Key != "ai_seo_generated" {
			newExtends = append(newExtends, item)
		}
	}

	// 添加新标记（只记录 true）
	newExtends = append(newExtends, vo.ExtendsItem{
		Key:   "ai_seo_generated",
		Value: true,
	})

	article.Extends = newExtends
}

// truncateText 截断文本
func truncateText(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	runes := []rune(text)
	if len(runes) <= maxLen {
		return text
	}
	return string(runes[:maxLen])
}

// maskAPIKey 遮蔽 API Key，只显示前后几位
func maskAPIKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	// 显示前4位和后4位，中间用****代替
	return key[:4] + "****" + key[len(key)-4:]
}

// safeKeyPrefix 安全地获取 API Key 前缀（用于诊断）
func safeKeyPrefix(key string) string {
	if len(key) < 4 {
		return "[too short]"
	}
	return key[:4] + "..."
}

// fixInvalidUnicodeEscapes 修复无效的 Unicode 转义序列
// AI 可能返回大写的 \U 转义（如 \Uxxxx)，而 JSON 标准只支持小写的 \u
// 将 \U 替换为 \u（后面跟着4个十六进制字符）
func fixInvalidUnicodeEscapes(jsonStr string) string {
	// 匹配 \U 后面跟着4个十六进制字符的模式
	// 例如: \U1F4A -> \u1F4A
	re := regexp.MustCompile(`\\U([0-9A-Fa-f]{4})`)
	return re.ReplaceAllString(jsonStr, `\\u$1`)
}

// fixInvalidEscapeSequences 修复 JSON 字符串中无效的转义序列
// AI 可能返回包含 Windows 路径的内容，如 Thunder\Program\Thunder.exe
// 其中 \P 不是有效的 JSON 转义序列，需要转换为 \\P
func fixInvalidEscapeSequences(jsonStr string) string {
	// JSON 标准支持的转义序列
	validEscapes := map[byte]bool{
		'"':  true,
		'\\': true,
		'/':  true,
		'b':  true,
		'f':  true,
		'n':  true,
		'r':  true,
		't':  true,
		'u':  true, // Unicode 转义
	}

	var result strings.Builder
	result.Grow(len(jsonStr) + len(jsonStr)/10) // 预分配额外空间

	i := 0
	for i < len(jsonStr) {
		if jsonStr[i] == '\\' && i+1 < len(jsonStr) {
			nextChar := jsonStr[i+1]
			if validEscapes[nextChar] {
				// 有效的转义序列，保留原样
				result.WriteByte(jsonStr[i])
				result.WriteByte(nextChar)
				i += 2
				// 如果是 \u，需要保留后面的4个十六进制字符
				if nextChar == 'u' && i+4 <= len(jsonStr) {
					result.WriteString(jsonStr[i : i+4])
					i += 4
				}
			} else {
				// 无效的转义序列，添加额外的反斜杠
				result.WriteString("\\\\")
				result.WriteByte(nextChar)
				i += 2
			}
		} else {
			result.WriteByte(jsonStr[i])
			i++
		}
	}

	return result.String()
}

// cleanInvalidUTF8 清理字符串中的无效 UTF-8 字符
// AI 可能返回包含无效 UTF-8 字符（如替换字符 \uFFFD）的响应
// 这些字符会导致 JSON 解析失败
func cleanInvalidUTF8(s string) string {
	if !utf8.ValidString(s) {
		var buf bytes.Buffer
		for _, r := range s {
			if r != utf8.RuneError {
				buf.WriteRune(r)
			} else {
				buf.WriteByte(' ')
			}
		}
		return buf.String()
	}
	return s
}

// getJsonErrorContext 获取 JSON 错误位置附近的内容，用于调试
func getJsonErrorContext(jsonStr string, offset int64) string {
	if offset < 0 || offset > int64(len(jsonStr)) {
		return "offset out of range"
	}

	// 获取错误位置前后各50个字符
	start := offset - 50
	if start < 0 {
		start = 0
	}
	end := offset + 50
	if end > int64(len(jsonStr)) {
		end = int64(len(jsonStr))
	}

	context := jsonStr[start:end]
	// 标记错误位置
	errorPos := offset - start
	if errorPos >= 0 && errorPos < int64(len(context)) {
		// 在错误位置插入标记
		context = context[:errorPos] + ">>>ERROR_HERE<<<" + context[errorPos:]
	}
	return context
}

// BaiduSugResponse 百度推荐词 API 响应结构
type BaiduSugResponse struct {
	Q string `json:"q"`
	G []struct {
		Type string `json:"type"`
		SA   string `json:"sa"`
		Q    string `json:"q"`
	} `json:"g"`
}

// getBaiduSuggestions 获取百度推荐词（用户搜索意图）
func (p *AISeoPlugin) getBaiduSuggestions(keyword string) ([]string, error) {
	if keyword == "" {
		return nil, nil
	}

	// 构建百度推荐词 API URL
	apiURL := fmt.Sprintf("https://www.baidu.com/sugrec?prod=pc&wd=%s", url.QueryEscape(keyword))

	p.ctx.Log.Info("Fetching Baidu suggestions",
		zap.String("keyword", keyword),
		zap.String("url", apiURL))

	// 发起请求
	resp, err := request.New().Get(apiURL)
	if err != nil {
		p.ctx.Log.Warn("Failed to get Baidu suggestions", zap.String("keyword", keyword), zap.Error(err))
		return nil, err
	}
	defer resp.Body.Close()

	// 读取响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		p.ctx.Log.Warn("Failed to read Baidu suggestions response", zap.Error(err))
		return nil, err
	}

	p.ctx.Log.Debug("Baidu API raw response",
		zap.String("keyword", keyword),
		zap.String("response", string(body)))

	// 解析 JSON
	var sugResp BaiduSugResponse
	if err := json.Unmarshal(body, &sugResp); err != nil {
		p.ctx.Log.Warn("Failed to parse Baidu suggestions response",
			zap.String("keyword", keyword),
			zap.String("response", string(body)),
			zap.Error(err))
		return nil, err
	}

	p.ctx.Log.Debug("Baidu API parsed response",
		zap.String("keyword", keyword),
		zap.String("query", sugResp.Q),
		zap.Int("items_count", len(sugResp.G)))

	// 提取推荐词
	var suggestions []string
	for _, item := range sugResp.G {
		p.ctx.Log.Debug("Baidu suggestion item",
			zap.String("type", item.Type),
			zap.String("sa", item.SA),
			zap.String("q", item.Q))
		if item.Q != "" {
			suggestions = append(suggestions, item.Q)
		}
	}

	p.ctx.Log.Debug("Baidu suggestions fetched",
		zap.String("keyword", keyword),
		zap.Int("count", len(suggestions)))

	return suggestions, nil
}

// extractCoreKeyword 使用 AI 从标题提取核心关键词/软件名
func (p *AISeoPlugin) extractCoreKeyword(title string, apiCfg *APIConfig) (string, error) {
	if title == "" || apiCfg == nil {
		return "", nil
	}

	p.ctx.Log.Info("Extracting core keyword from title",
		zap.String("title", title))

	prompt := fmt.Sprintf(`请从以下文章标题中提取核心关键词或软件名称。只返回一个最核心的关键词或软件名，不要包含其他内容。
文章标题：%s

请直接返回核心关键词，不要有任何解释或额外内容。`, title)

	p.ctx.Log.Debug("Core keyword prompt",
		zap.String("title", title),
		zap.String("prompt", prompt))

	response, err := p.callAIWithConfig(prompt, apiCfg)
	if err != nil {
		p.ctx.Log.Warn("Failed to extract core keyword", zap.Error(err))
		return "", err
	}

	// 清理响应
	keyword := strings.TrimSpace(response)
	keyword = strings.Trim(keyword, "\"")

	p.ctx.Log.Info("Core keyword extracted",
		zap.String("title", title),
		zap.String("raw_response", response),
		zap.String("keyword", keyword))

	return keyword, nil
}
