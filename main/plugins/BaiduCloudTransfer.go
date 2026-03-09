package plugins

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"moss/domain/core/entity"
	"moss/domain/core/service"
	"moss/domain/core/vo"
	repositorycontext "moss/domain/core/repository/context"
	pluginEntity "moss/domain/support/entity"
	baiduUtils "moss/plugins/baidu_utils"
)

const (
	baiduPanBaseURL = "https://pan.baidu.com"
	maxRetries      = 3
)

// BaiduCloudTransfer 百度网盘转存插件
type BaiduCloudTransfer struct {
	Cookie    string `json:"cookie"`    // 完整的 Cookie（优先）
	BDUSS     string `json:"bduss"`     // BDUSS（向后兼容）
	STOKEN    string `json:"stoken"`    // STOKEN（向后兼容）
	SaveDir   string `json:"save_dir"`   // 默认保存目录
	RateLimit int    `json:"rate_limit"` // 速率限制（次/分钟）

	ctx         *pluginEntity.Plugin
	httpClient  *http.Client
	lastRequest time.Time
	mu          sync.Mutex

	// 解析后的 Cookie 值
	parsedBDUSS  string
	parsedSTOKEN string

	// 百度网盘工具实例
	baiduUtils *baiduUtils.BaiduUtils
}

// BaiduShareFile 百度网盘分享文件
type BaiduShareFile struct {
	Path         string `json:"path"`
	FsID         int64  `json:"fs_id"`
	Size         int64  `json:"size"`
	IsDir        bool   `json:"is_dir"`
	UK           int64  `json:"uk"`
	ShareID      int64  `json:"share_id"`
	Bdstoken     string `json:"bdstoken"`
	ServerName   string `json:"server_filename"`
}

// BaiduLink 百度网盘链接
type BaiduLink struct {
	Type     string `json:"type"`
	URL      string `json:"url"`
	Password string `json:"password"`
}

// BaiduSavedItem 转存记录
type BaiduSavedItem struct {
	Type      string `json:"type"`      // "百度网盘"
	URL       string `json:"url"`       // 原始分享链接
	Password  string `json:"password"`  // 提取码
	Status    string `json:"status"`    // success/failed/pending
	SavedPath string `json:"saved_path"` // 转存路径
	SavedURL  string `json:"saved_url"`  // 新分享链接
	Timestamp int64  `json:"timestamp"`  // 转存时间
	Error     string `json:"error"`     // 错误信息
}

// NewBaiduCloudTransfer 创建百度网盘转存插件
func NewBaiduCloudTransfer() *BaiduCloudTransfer {
	return &BaiduCloudTransfer{
		RateLimit: 30, // 默认30次/分钟
		SaveDir:   "", // 默认保存到根目录
	}
}

// Info 返回插件信息
func (b *BaiduCloudTransfer) Info() *pluginEntity.PluginInfo {
	return &pluginEntity.PluginInfo{
		ID:         "BaiduCloudTransfer",
		About:      "定时转存文章中的百度网盘链接",
		RunEnable:  true,
		CronEnable: true,
		PluginInfoPersistent: pluginEntity.PluginInfoPersistent{
			CronStart: true,
			CronExp:   "@every 24h",
		},
	}
}

// Load 加载插件
func (b *BaiduCloudTransfer) Load(ctx *pluginEntity.Plugin) error {
	b.ctx = ctx
	b.httpClient = &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        10,
			IdleConnTimeout:     30 * time.Second,
			DisableCompression:  true, // 禁用自动压缩处理，手动处理所有 gzip 解压缩
		},
	}

	// 清理 Cookie：移除换行符、回车符、制表符和多余空格
	b.Cookie = strings.ReplaceAll(b.Cookie, "\n", "")
	b.Cookie = strings.ReplaceAll(b.Cookie, "\r", "")
	b.Cookie = strings.ReplaceAll(b.Cookie, "\t", "")
	b.Cookie = strings.TrimSpace(b.Cookie)

	// 解析 Cookie
	if err := b.parseCookie(); err != nil {
		b.ctx.Log.Warn("解析 Cookie 失败", zap.Error(err))
	}

	// 初始化百度网盘工具实例
	b.baiduUtils = baiduUtils.NewBaiduUtils(b.Cookie, b.ctx.Log)

	b.ctx.Log.Info("百度网盘转存插件加载成功")
	return nil
}

// Run 执行插件
func (b *BaiduCloudTransfer) Run(ctx *pluginEntity.Plugin) error {
	b.ctx.Log.Info("开始执行百度网盘转存任务")
	return b.processTransfer()
}

// processTransfer 处理转存逻辑
func (b *BaiduCloudTransfer) processTransfer() error {
	// 优先检查完整 Cookie，其次检查 BDUSS
	if b.Cookie == "" && b.parsedBDUSS == "" {
		b.ctx.Log.Warn("百度网盘 Cookie 或 BDUSS 未配置，跳过转存任务")
		return nil
	}
	
	b.ctx.Log.Info("开始执行百度网盘转存任务",
		zap.Bool("use_full_cookie", b.Cookie != ""),
		zap.Bool("has_bduss", b.parsedBDUSS != ""))

	b.ctx.Log.Info("开始扫描文章中的百度网盘链接")

	// 获取所有文章
	ctx := repositorycontext.NewContext(1000, "")
	articleBases, err := service.Article.List(ctx)
	if err != nil {
		b.ctx.Log.Error("获取文章列表失败", zap.Error(err))
		return err
	}

	b.ctx.Log.Info("获取文章列表成功", zap.Int("total_articles", len(articleBases)))

	totalLinks := 0
	transferredCount := 0
	failedCount := 0
	articlesWithLinks := 0

	// 遍历文章
	for _, articleBase := range articleBases {
		// 获取完整的文章对象（包含 Res 字段）
		article, err := service.Article.Get(articleBase.ID)
		if err != nil {
			b.ctx.Log.Error("获取文章详情失败",
				zap.Int("article_id", articleBase.ID),
				zap.Error(err))
			continue
		}

		// 解析 download_links
		downloadLinks := b.parseDownloadLinks(article.Res)
		if len(downloadLinks) == 0 {
			continue
		}

		articlesWithLinks++

		b.ctx.Log.Debug("文章包含下载链接",
			zap.Int("article_id", article.ID),
			zap.Int("download_links_count", len(downloadLinks)))

		// 解析已保存的记录
		savedLinks := b.parseSavedLinks(article.Res)

		// 查找未转存的链接
		for _, link := range downloadLinks {
			// 支持多种百度网盘类型名称
			isBaiduPan := link.Type == "百度网盘" || link.Type == "百度云" || link.Type == "百度" || strings.Contains(link.Type, "百度")
			if !isBaiduPan {
				continue
			}

			totalLinks++

			// 检查是否已转存
			if b.isTransferred(link, savedLinks) {
				continue
			}

			b.ctx.Log.Info("发现未转存的百度网盘链接",
				zap.Int("article_id", article.ID),
				zap.String("url", link.URL))

			// 应用速率限制
			b.applyRateLimit()

			// 执行转存
			savedItem, err := b.transferLink(link, b.SaveDir)
			if err != nil {
				b.ctx.Log.Error("转存失败",
					zap.Int("article_id", article.ID),
					zap.String("url", link.URL),
					zap.Error(err))
				failedCount++
				
				// 检查是否是空间不足错误
				errorMsg := err.Error()
				if strings.Contains(errorMsg, "容量不足") || 
				   strings.Contains(errorMsg, "error_code: -10") || 
				   strings.Contains(errorMsg, "error_code: 20") {
					b.ctx.Log.Warn("检测到空间不足，停止后续转存任务",
						zap.Int("article_id", article.ID),
						zap.String("url", link.URL))
					// 返回错误以停止整个任务
					return fmt.Errorf("转存失败，网盘空间不足，已停止任务: %w", err)
				}
			} else {
				b.ctx.Log.Info("转存成功",
					zap.Int("article_id", article.ID),
					zap.String("url", link.URL),
					zap.String("saved_path", savedItem.SavedPath),
					zap.String("saved_url", savedItem.SavedURL))
				transferredCount++

				// 只有成功时才更新文章 Res 字段
				if err := b.updateArticleRes(article, savedItem); err != nil {
					b.ctx.Log.Error("更新文章失败",
						zap.Int("article_id", article.ID),
						zap.Error(err))
				}
			}
		}
	}

	b.ctx.Log.Info("转存任务完成",
		zap.Int("total_articles", len(articleBases)),
		zap.Int("articles_with_links", articlesWithLinks),
		zap.Int("total_links", totalLinks),
		zap.Int("transferred", transferredCount),
		zap.Int("failed", failedCount))

	return nil
}

// parseDownloadLinks 解析 download_links
func (b *BaiduCloudTransfer) parseDownloadLinks(res vo.Extends) []BaiduLink {
	var links []BaiduLink

	for _, item := range res {
		if item.Key == "download_links" {
			if value, ok := item.Value.([]any); ok {
				for _, v := range value {
					if linkMap, ok := v.(map[string]any); ok {
						link := BaiduLink{}
						if linkType, ok := linkMap["type"].(string); ok {
							link.Type = linkType
						}
						if linkURL, ok := linkMap["url"].(string); ok {
							link.URL = linkURL
						}
						if password, ok := linkMap["password"].(string); ok {
							link.Password = password
						}

						// 如果 password 字段为空，尝试从 URL 中提取 pwd 参数
						if link.Password == "" && strings.Contains(link.URL, "?pwd=") {
							parts := strings.Split(link.URL, "?pwd=")
							if len(parts) > 1 {
								// 提取 pwd 参数的值（只取前4位）
								pwdValue := parts[1]
								if len(pwdValue) > 4 {
									pwdValue = pwdValue[:4]
								}
								link.Password = pwdValue
							}
						}

						links = append(links, link)

						// 添加调试日志（通用下载链接，不限定类型）
						b.ctx.Log.Debug("解析到下载链接",
							zap.String("type", link.Type),
							zap.String("url", link.URL),
							zap.String("has_password", strconv.FormatBool(link.Password != "")))
					}
				}
			}
		}
	}

	return links
}

// parseSavedLinks 解析 saved
func (b *BaiduCloudTransfer) parseSavedLinks(res vo.Extends) []BaiduSavedItem {
	var items []BaiduSavedItem

	for _, item := range res {
		if item.Key == "saved" {
			if value, ok := item.Value.([]any); ok {
				for _, v := range value {
					if itemMap, ok := v.(map[string]any); ok {
						savedItem := BaiduSavedItem{}
						if itemType, ok := itemMap["type"].(string); ok {
							savedItem.Type = itemType
						}
						if itemURL, ok := itemMap["url"].(string); ok {
							savedItem.URL = itemURL
						}
						if password, ok := itemMap["password"].(string); ok {
							savedItem.Password = password
						}
						if status, ok := itemMap["status"].(string); ok {
							savedItem.Status = status
						}
						if savedPath, ok := itemMap["saved_path"].(string); ok {
							savedItem.SavedPath = savedPath
						}
						if savedURL, ok := itemMap["saved_url"].(string); ok {
							savedItem.SavedURL = savedURL
						}
						if timestamp, ok := itemMap["timestamp"].(float64); ok {
							savedItem.Timestamp = int64(timestamp)
						}
						if errMsg, ok := itemMap["error"].(string); ok {
							savedItem.Error = errMsg
						}
						items = append(items, savedItem)
					}
				}
			}
		}
	}

	return items
}

// isTransferred 检查是否已转存
func (b *BaiduCloudTransfer) isTransferred(link BaiduLink, saved []BaiduSavedItem) bool {
	for _, item := range saved {
		// 检查类型是否匹配（支持多种百度网盘类型名称）
		isBaiduPan := item.Type == "百度网盘" || item.Type == "百度云" || item.Type == "百度" || strings.Contains(item.Type, "百度")
		if isBaiduPan && item.URL == link.URL && item.Status == "success" {
			return true
		}
	}
	return false
}

// applyRateLimit 应用速率限制
func (b *BaiduCloudTransfer) applyRateLimit() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.RateLimit <= 0 {
		return
	}

	interval := time.Minute / time.Duration(b.RateLimit)
	if !b.lastRequest.IsZero() {
		wait := time.Until(b.lastRequest.Add(interval))
		if wait > 0 {
			b.ctx.Log.Debug("速率限制等待", zap.Duration("wait", wait))
			time.Sleep(wait)
		}
	}
	b.lastRequest = time.Now()
}

// transferLink 转存链接
func (b *BaiduCloudTransfer) transferLink(link BaiduLink, saveDir string) (*BaiduSavedItem, error) {
	savedItem := &BaiduSavedItem{
		Type:      "百度网盘",
		URL:       link.URL,
		Password:  link.Password,
		Status:    "pending",
		Timestamp: time.Now().Unix(),
	}

	// 获取 bdstoken
	bdstoken, err := b.getBdstoken()
	if err != nil {
		savedItem.Status = "failed"
		savedItem.Error = err.Error()
		return savedItem, fmt.Errorf("获取 bdstoken 失败: %w", err)
	}

	// 提取 surl
	surl := b.extractSurl(link.URL)
	if surl == "" {
		savedItem.Status = "failed"
		savedItem.Error = "无效的分享链接"
		return savedItem, errors.New("无效的分享链接")
	}

			// 验证提取码（如果有）
			if link.Password != "" {
				// 传入完整的 link.URL，verifyPassCode 内部会使用暴力切片提取 surl
				randsk, err := b.verifyPassCode(link.URL, link.Password, bdstoken)
				if err != nil {
					savedItem.Status = "failed"
					savedItem.Error = err.Error()
					return savedItem, fmt.Errorf("验证提取码失败: %w", err)
				}
				b.updateCookie("BDCLND", randsk)
			}
	
			// 添加日志：开始获取分享文件列表
			b.ctx.Log.Debug("开始获取分享文件列表",
				zap.String("url", link.URL))
	
			// 获取分享文件列表（传入原始链接，使用完整的 surl）
			files, err := b.getSharedPaths(link.URL)
	if err != nil {
		savedItem.Status = "failed"
		savedItem.Error = err.Error()
		return savedItem, fmt.Errorf("获取分享文件列表失败: %w", err)
	}

	if len(files) == 0 {
		savedItem.Status = "failed"
		savedItem.Error = "分享链接中没有文件"
		return savedItem, errors.New("分享链接中没有文件")
	}

	// 执行转存
	fsIDs := make([]int64, len(files))
	for i, file := range files {
		fsIDs[i] = file.FsID
	}

	remotedir := "/" + saveDir
	savedFsIDs, err := b.transferFile(files[0].ShareID, files[0].UK, bdstoken, remotedir, fsIDs)
	if err != nil {
		savedItem.Status = "failed"
		savedItem.Error = err.Error()
		return savedItem, fmt.Errorf("转存文件失败: %w", err)
	}

	savedItem.SavedPath = remotedir

	// 创建分享链接（转存后必须分享）
	if len(savedFsIDs) > 0 {
		b.ctx.Log.Info("开始创建分享链接",
			zap.Int("article_id", 0),
			zap.Int("total_count", len(savedFsIDs)),
			zap.String("save_dir", remotedir))

		// 使用转存后的文件 ID（to_fs_id）
		// 支持分享文件和目录
		fileCount := 0
		dirCount := 0
		for i := range savedFsIDs {
			// 查找对应的文件类型
			for _, file := range files {
				if i < len(files) && file.FsID == savedFsIDs[i] {
					if !file.IsDir {
						fileCount++
					} else {
						dirCount++
					}
					break
				}
			}
		}

		b.ctx.Log.Info("准备分享内容",
			zap.Int("file_count", fileCount),
			zap.Int("directory_count", dirCount),
			zap.String("first_fs_id", strconv.FormatInt(savedFsIDs[0], 10)))

		shareURL, err := b.createShare(savedFsIDs, "0", link.Password, bdstoken)
		if err != nil {
			b.ctx.Log.Error("创建分享链接失败",
				zap.Int("file_count", fileCount),
				zap.Int("directory_count", dirCount),
				zap.Int64s("fs_ids", fsIDs),
				zap.Error(err))
			savedItem.Error = fmt.Sprintf("转存成功但创建分享失败: %s", err.Error())
			// 即使分享失败，转存仍然成功，状态保持 success
		} else {
			savedItem.SavedURL = shareURL
			b.ctx.Log.Info("创建分享链接成功",
				zap.Int("article_id", 0),
				zap.String("share_url", shareURL))
		}
	}

	savedItem.Status = "success"
	return savedItem, nil
}

// getBdstoken 获取 bdstoken
func (b *BaiduCloudTransfer) getBdstoken() (string, error) {
	apiURL := fmt.Sprintf("%s/api/gettemplatevariable", baiduPanBaseURL)
	params := url.Values{}
	params.Set("clienttype", "0")
	params.Set("app_id", "38824127")
	params.Set("web", "1")
	params.Set(`fields`, `["bdstoken","token","uk","isdocuser","servertime"]`)

	req, err := http.NewRequest("GET", apiURL+"?"+params.Encode(), nil)
	if err != nil {
		return "", err
	}

	b.setHeaders(req)

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// 读取响应体并处理 gzip 解压缩
	body, err := b.readResponseBody(resp)
	if err != nil {
		return "", fmt.Errorf("读取响应体失败: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("解析 JSON 失败: %w", err)
	}

	errno, _ := result["errno"].(float64)
	if errno != 0 {
		return "", fmt.Errorf("获取 bdstoken 失败, 错误码: %d", int(errno))
	}

	resultData, ok := result["result"].(map[string]any)
	if !ok {
		return "", errors.New("解析 result 失败")
	}

	bdstoken, ok := resultData["bdstoken"].(string)
	if !ok {
		return "", errors.New("解析 bdstoken 失败")
	}

	b.ctx.Log.Debug("成功获取 bdstoken",
		zap.String("bdstoken_length", strconv.Itoa(len(bdstoken))),
		zap.String("bdstoken_preview", bdstoken[:minBaidu(10, len(bdstoken))]+"..."))

	return bdstoken, nil
}

func minBaidu(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// verifyPassCode 验证提取码
func (b *BaiduCloudTransfer) verifyPassCode(linkURL, pwd, bdstoken string) (string, error) {
	apiURL := fmt.Sprintf("%s/share/verify", baiduPanBaseURL)

	b.ctx.Log.Debug("开始验证提取码",
		zap.String("link_url", linkURL),
		zap.String("password", pwd),
		zap.String("bdstoken_length", strconv.Itoa(len(bdstoken))))

	// 根据链接格式选择正确的 surl 提取方法
	var surl string
	if strings.Contains(linkURL, "/share/init?surl=") {
		// 格式: https://pan.baidu.com/share/init?surl={surl}&pwd=xxx
		// 使用正则表达式提取完整 surl
		re := regexp.MustCompile(`surl=([^&]+)`)
		matches := re.FindStringSubmatch(linkURL)
		if len(matches) > 1 {
			surl = matches[1]
		}
	} else {
		// 格式: https://pan.baidu.com/s/{surl}?pwd=xxx
		// 使用暴力切片提取 surl（去掉开头的 1）
		if len(linkURL) >= 48 {
			surl = linkURL[25:48]
			// 移除可能的查询参数（如 ?pwd=xxx）
			if idx := strings.Index(surl, "?"); idx != -1 {
				surl = surl[:idx]
			}
		} else {
			// 如果链接长度不够，尝试从链接中提取
			parts := strings.Split(linkURL, "/")
			if len(parts) > 0 {
				surl = parts[len(parts)-1]
				// 移除可能的查询参数
				if idx := strings.Index(surl, "?"); idx != -1 {
					surl = surl[:idx]
				}
			}
		}
	}

	b.ctx.Log.Debug("提取 surl",
		zap.String("surl", surl),
		zap.String("link_url_length", strconv.Itoa(len(linkURL))))

	params := url.Values{}
	params.Set("surl", surl)
	params.Set("bdstoken", bdstoken)
	params.Set("t", strconv.FormatInt(time.Now().UnixMilli(), 10))
	params.Set("channel", "chunlei")
	params.Set("web", "1")
	params.Set("clienttype", "0")

	data := url.Values{}
	data.Set("pwd", pwd)
	data.Set("vcode", "")
	data.Set("vcode_str", "")

	req, err := http.NewRequest("POST", apiURL+"?"+params.Encode(), strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}

	b.setHeaders(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// 读取响应体并处理 gzip 解压缩
	body, err := b.readResponseBody(resp)
	if err != nil {
		return "", fmt.Errorf("读取响应体失败: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("解析 JSON 失败: %w", err)
	}

	errno, _ := result["errno"].(float64)
	if errno != 0 {
		errorCode := int(errno)
		responseStr := string(body)
		
		// 打印详细的调试信息
		b.ctx.Log.Error("=== 验证提取码失败 - 详细信息 ===")
		b.ctx.Log.Error("请求 URL", zap.String("url", req.URL.String()))
		b.ctx.Log.Error("请求方法", zap.String("method", req.Method))
		
		// 打印所有请求头
		b.ctx.Log.Error("请求头:")
		for key, values := range req.Header {
			for _, value := range values {
				b.ctx.Log.Error(fmt.Sprintf("  %s: %s", key, value))
			}
		}
		
		// 打印请求参数
		b.ctx.Log.Error("请求参数:")
		b.ctx.Log.Error(fmt.Sprintf("  surl: %s", surl))
		b.ctx.Log.Error(fmt.Sprintf("  bdstoken: %s", bdstoken))
		b.ctx.Log.Error(fmt.Sprintf("  t: %d", time.Now().UnixMilli()))
		b.ctx.Log.Error(fmt.Sprintf("  pwd: %s", pwd))
		
		// 打印请求体
		b.ctx.Log.Error("请求体:", zap.String("body", data.Encode()))
		
		// 打印响应信息
		b.ctx.Log.Error("响应状态码:", zap.Int("status", resp.StatusCode))
		b.ctx.Log.Error("响应头:")
		for key, values := range resp.Header {
			for _, value := range values {
				b.ctx.Log.Error(fmt.Sprintf("  %s: %s", key, value))
			}
		}
		
		// 打印响应体
		b.ctx.Log.Error("响应体:", zap.String("body", responseStr))
		b.ctx.Log.Error("错误信息:", zap.String("error", fmt.Sprintf("错误码: %d, 错误信息: %s", errorCode, baiduUtils.ErrorCodeMap[errorCode])))
		b.ctx.Log.Error("========================================")

		return "", fmt.Errorf("验证提取码失败, 错误码: %d, 错误信息: %s", errorCode, baiduUtils.ErrorCodeMap[errorCode])
	}

	randsk, ok := result["randsk"].(string)
	if !ok {
		return "", errors.New("解析 randsk 失败")
	}

	b.ctx.Log.Debug("验证提取码成功",
		zap.String("surl", surl),
		zap.String("randsk_length", strconv.Itoa(len(randsk))),
		zap.String("randsk_preview", randsk[:min(10, len(randsk))]+"..."))

	return randsk, nil
}

// getSharedPaths 获取分享文件列表
func (b *BaiduCloudTransfer) getSharedPaths(shareURL string) ([]BaiduShareFile, error) {
	// 提取 surl 用于验证，但访问页面时使用原始链接
	fullSurl := b.extractSurl(shareURL)
	if fullSurl == "" {
		return nil, errors.New("无效的分享链接")
	}

	// 使用原始链接访问页面（保留开头的 "1"）
	var url string
	if strings.Contains(shareURL, "/share/init?surl=") {
		// 格式: https://pan.baidu.com/share/init?surl={surl}
		url = shareURL
	} else {
		// 格式: https://pan.baidu.com/s/{surl} - 使用原始链接
		url = shareURL
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	b.setHeaders(req)

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 读取响应体并处理 gzip 解压缩
	body, err := b.readResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("读取响应体失败: %w", err)
	}

	// 添加调试日志
	responseStr := string(body)
	previewLen := 500
	if len(responseStr) < previewLen {
		previewLen = len(responseStr)
	}

	b.ctx.Log.Debug("分享链接响应内容",
		zap.String("url", url),
		zap.String("response_length", strconv.Itoa(len(responseStr))),
		zap.String("response_preview", responseStr[:previewLen]),
		zap.Bool("contains_shareid", strings.Contains(responseStr, `"shareid"`)),
		zap.Bool("contains_fs_id", strings.Contains(responseStr, `"fs_id"`)),
		zap.Bool("contains_server_filename", strings.Contains(responseStr, `"server_filename"`)))

	return b.parseShareResponse(responseStr)
}

// parseShareResponse 解析分享链接响应
func (b *BaiduCloudTransfer) parseShareResponse(response string) ([]BaiduShareFile, error) {
	var files []BaiduShareFile

	// 使用正则表达式提取参数
	shareIDRegex := regexp.MustCompile(`"shareid":(\d+),"`)
	userIDRegex := regexp.MustCompile(`"share_uk":"(\d+)","`)
	fsIDRegex := regexp.MustCompile(`"fs_id":(\d+),"`)
	serverFilenameRegex := regexp.MustCompile(`"server_filename":"([^"]+)","`)
	isDirRegex := regexp.MustCompile(`"isdir":(\d+),"`)

	shareIDs := shareIDRegex.FindAllStringSubmatch(response, -1)
	userIDs := userIDRegex.FindAllStringSubmatch(response, -1)
	fsIDs := fsIDRegex.FindAllStringSubmatch(response, -1)
	filenames := serverFilenameRegex.FindAllStringSubmatch(response, -1)
	isDirs := isDirRegex.FindAllStringSubmatch(response, -1)

	if len(shareIDs) == 0 || len(userIDs) == 0 || len(fsIDs) == 0 {
		return nil, fmt.Errorf("解析分享链接响应失败, 可能是提取码错误或链接失效")
	}

	shareID, _ := strconv.ParseInt(shareIDs[0][1], 10, 64)
	userID, _ := strconv.ParseInt(userIDs[0][1], 10, 64)

	for i, fsIDStr := range fsIDs {
		fsID, _ := strconv.ParseInt(fsIDStr[1], 10, 64)
		filename := ""
		isDir := false

		if i < len(filenames) {
			filename = filenames[i][1]
		}
		if i < len(isDirs) {
			isDir = isDirs[i][1] == "1"
		}

		files = append(files, BaiduShareFile{
			FsID:       fsID,
			ShareID:    shareID,
			UK:         userID,
			ServerName: filename,
			IsDir:      isDir,
		})
	}

	return files, nil
}

// transferFile 转存文件到指定目录，返回转存后的文件 ID 列表
func (b *BaiduCloudTransfer) transferFile(shareID, uk int64, bdstoken, remotedir string, fsIDs []int64) ([]int64, error) {
	apiURL := fmt.Sprintf("%s/share/transfer", baiduPanBaseURL)

	params := url.Values{}
	params.Set("shareid", strconv.FormatInt(shareID, 10))
	params.Set("from", strconv.FormatInt(uk, 10))
	params.Set("bdstoken", bdstoken)
	params.Set("channel", "chunlei")
	params.Set("web", "1")
	params.Set("clienttype", "0")

	// 构建 fsidlist 字符串格式 [1,2,3]
	var fsidListStr strings.Builder
	fsidListStr.WriteString("[")
	for i, fsID := range fsIDs {
		if i > 0 {
			fsidListStr.WriteString(",")
		}
		fsidListStr.WriteString(strconv.FormatInt(fsID, 10))
	}
	fsidListStr.WriteString("]")

	data := fmt.Sprintf(`fsidlist=%s&path=%s`, fsidListStr.String(), url.QueryEscape(remotedir))

	req, err := http.NewRequest("POST", apiURL+"?"+params.Encode(), strings.NewReader(data))
	if err != nil {
		return nil, err
	}

	b.setHeaders(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 读取响应体并处理 gzip 解压缩
	body, err := b.readResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("读取响应体失败: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析 JSON 失败: %w", err)
	}

	errno, _ := result["errno"].(float64)
	if errno != 0 {
		errorCode := int(errno)
		errorMsg := baiduUtils.ErrorCodeMap[errorCode]
		if errorMsg == "" {
			errorMsg = "未知错误"
		}
		return nil, fmt.Errorf("转存失败, 错误码: %d, 错误信息: %s", errorCode, errorMsg)
	}

	// 添加调试日志：打印转存 API 的完整响应
	b.ctx.Log.Debug("转存 API 响应",
		zap.String("response", string(body)),
		zap.Int("errno", int(errno)))

	// 提取转存后的文件 ID 列表（to_fs_id）
	var savedFsIDs []int64
	if extra, ok := result["extra"].(map[string]any); ok {
		if list, ok := extra["list"].([]any); ok {
			for _, item := range list {
				if itemMap, ok := item.(map[string]any); ok {
					if toFsID, ok := itemMap["to_fs_id"].(float64); ok {
						savedFsIDs = append(savedFsIDs, int64(toFsID))
					}
				}
			}
		}
	}

	b.ctx.Log.Debug("提取转存后的文件 ID",
		zap.Int("count", len(savedFsIDs)),
		zap.Int64s("saved_fs_ids", savedFsIDs))

	if len(savedFsIDs) == 0 {
		return nil, fmt.Errorf("未能获取转存后的文件 ID")
	}

	return savedFsIDs, nil

	}

	

	// createShare 创建分享链接（使用 baidu_utils 的统一实现）
func (b *BaiduCloudTransfer) createShare(fsIDs []int64, period, pwd, bdstoken string) (string, error) {
	b.ctx.Log.Debug("开始创建分享链接",
		zap.Int("file_count", len(fsIDs)),
		zap.String("period", period),
		zap.String("has_password", strconv.FormatBool(pwd != "")))

	if len(fsIDs) == 0 {
		return "", errors.New("没有可分享的文件")
	}

	// 使用 baidu_utils 的统一实现
	shareURL, err := b.baiduUtils.CreateShare(fsIDs, period, pwd)
	if err != nil {
		b.ctx.Log.Error("创建分享链接失败", zap.Error(err))
		return "", err
	}

	b.ctx.Log.Info("分享链接创建成功", zap.String("share_url", shareURL))
	return shareURL, nil
}

// extractSurl 从分享链接中提取 surl
func (b *BaiduCloudTransfer) extractSurl(shareURL string) string {
	// 支持多种格式
	// https://pan.baidu.com/s/1xxx
	// https://pan.baidu.com/share/init?surl=xxx
	re := regexp.MustCompile(`surl[=]?([a-zA-Z0-9_-]+)`)
	matches := re.FindStringSubmatch(shareURL)
	if len(matches) > 1 {
		return matches[1]
	}

	// 尝试直接提取 /s/ 后面的部分
	re = regexp.MustCompile(`/s/([a-zA-Z0-9_-]+)`)
	matches = re.FindStringSubmatch(shareURL)
	if len(matches) > 1 {
		surl := matches[1]
		// 新格式链接以 "1" 开头，需要去掉（如: https://pan.baidu.com/s/1xxx -> xxx）
		if len(surl) > 0 && surl[0] == '1' {
			surl = surl[1:]
		}
		return surl
	}

	return ""
}

// setHeaders 设置请求头
func (b *BaiduCloudTransfer) setHeaders(req *http.Request) {
	req.Header.Set("Host", "pan.baidu.com")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.9")
	req.Header.Set("Sec-Fetch-Site", "same-site")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Referer", "https://pan.baidu.com")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8,en-US;q=0.7,en-GB;q=0.6,ru;q=0.5")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")

	// 优先使用完整 Cookie，参考 BaiduPan 项目的实现方式
	if b.Cookie != "" {
		req.Header.Set("Cookie", b.Cookie)
	} else {
		// 如果没有完整 Cookie，使用解析后的 BDUSS 和 STOKEN
		cookie := fmt.Sprintf("BDUSS=%s", b.parsedBDUSS)
		if b.parsedSTOKEN != "" {
			cookie += fmt.Sprintf("; STOKEN=%s", b.parsedSTOKEN)
		}
		req.Header.Set("Cookie", cookie)
	}
}

// readResponseBody 读取响应体并处理 gzip 解压缩
func (b *BaiduCloudTransfer) readResponseBody(resp *http.Response) ([]byte, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// 检查是否是 gzip 压缩数据（魔数：0x1f 0x8b）
	if len(body) >= 2 && body[0] == 0x1f && body[1] == 0x8b {
		gzReader, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("解压 gzip 数据失败: %w", err)
		}
		defer gzReader.Close()

		body, err = io.ReadAll(gzReader)
		if err != nil {
			return nil, fmt.Errorf("读取解压数据失败: %w", err)
		}
	}

	return body, nil
}

// updateCookie 更新 Cookie 字符串，添加或更新指定键值对
func (b *BaiduCloudTransfer) updateCookie(key, value string) {
	// 如果有完整 Cookie，更新完整 Cookie
	if b.Cookie != "" {
		// 解析 Cookie 到 map
		cookieMap := make(map[string]string)
		pairs := strings.Split(b.Cookie, ";")
		for _, pair := range pairs {
			pair = strings.TrimSpace(pair)
			if pair == "" {
				continue
			}
			kv := strings.SplitN(pair, "=", 2)
			if len(kv) == 2 {
				cookieMap[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
			}
		}
		
		// 更新或添加指定的键值对
		cookieMap[key] = value
		
		// 重新构建 Cookie 字符串
		var updatedPairs []string
		for k, v := range cookieMap {
			updatedPairs = append(updatedPairs, fmt.Sprintf("%s=%s", k, v))
		}
		b.Cookie = strings.Join(updatedPairs, "; ")
		
		b.ctx.Log.Debug("更新完整 Cookie", zap.String("key", key))
	}
	
	// 如果没有完整 Cookie，更新解析后的值（虽然这种方式不太可靠）
	if key == "BDUSS" {
		b.parsedBDUSS = value
	} else if key == "STOKEN" {
		b.parsedSTOKEN = value
	}
}

// parseCookie 解析 Cookie 字符串
func (b *BaiduCloudTransfer) parseCookie() error {
	// 优先使用完整的 Cookie
	cookieStr := b.Cookie
	if cookieStr != "" {
		b.ctx.Log.Info("使用完整 Cookie 配置")
		
		// 解析完整 Cookie 以便调试和日志记录
		pairs := strings.Split(cookieStr, ";")
		for _, pair := range pairs {
			pair = strings.TrimSpace(pair)
			kv := strings.SplitN(pair, "=", 2)
			if len(kv) != 2 {
				continue
			}
			key := strings.TrimSpace(kv[0])
			value := strings.TrimSpace(kv[1])

			switch key {
			case "BDUSS":
				b.parsedBDUSS = value
				b.ctx.Log.Debug("从完整 Cookie 中解析到 BDUSS", zap.String("length", strconv.Itoa(len(value))))
			case "STOKEN":
				b.parsedSTOKEN = value
				b.ctx.Log.Debug("从完整 Cookie 中解析到 STOKEN", zap.String("length", strconv.Itoa(len(value))))
			}
		}
		
		// 检查是否包含必要的字段
		if b.parsedBDUSS == "" {
			b.ctx.Log.Warn("完整 Cookie 中未找到 BDUSS 字段，可能 Cookie 不完整")
		}
		
		return nil
	}

	// 如果没有完整 Cookie，使用单独的 BDUSS 和 STOKEN
	b.ctx.Log.Info("使用单独的 BDUSS 和 STOKEN 配置")
	b.parsedBDUSS = b.BDUSS
	b.parsedSTOKEN = b.STOKEN

	return nil
}

// updateArticleRes 更新文章 Res 字段
func (b *BaiduCloudTransfer) updateArticleRes(article *entity.Article, savedItem *BaiduSavedItem) error {
	// 解析已保存的记录
	saved := b.parseSavedLinks(article.Res)

	// 确保 saved 中只保存成功记录，并且每个类型只有一条记录
	// 查找相同类型的记录并替换
	found := false
	for i, item := range saved {
		// 支持多种百度网盘类型名称
		isBaiduPan := item.Type == "百度网盘" || item.Type == "百度云" || item.Type == "百度" || strings.Contains(item.Type, "百度")
		newIsBaiduPan := savedItem.Type == "百度网盘" || savedItem.Type == "百度云" || savedItem.Type == "百度" || strings.Contains(savedItem.Type, "百度")

		// 如果类型相同（对于百度网盘，所有类型视为相同），则替换
		if (isBaiduPan && newIsBaiduPan) || item.Type == savedItem.Type {
			saved[i] = *savedItem
			found = true
			break
		}
	}

	// 如果没有找到相同类型的记录，则添加新记录
	if !found {
		saved = append(saved, *savedItem)
	}

	// 更新 res 字段
	found = false
	for i, item := range article.Res {
		if item.Key == "saved" {
			article.Res[i].Value = saved
			found = true
			break
		}
	}

	if !found {
		// 如果没有 saved 字段，添加一个
		article.Res = append(article.Res, vo.ExtendsItem{
			Key:   "saved",
			Value: saved,
		})
	}

	// 保存到数据库
	return service.Article.Update(article)
}

// TestCookie 测试Cookie有效性
func (b *BaiduCloudTransfer) TestCookie() (bool, error) {
	if b.baiduUtils == nil {
		return false, errors.New("baidu utils not initialized")
	}

	// 尝试获取 bdstoken 来验证 Cookie 是否有效
	bdstoken, err := b.baiduUtils.GetBdstoken()
	if err != nil {
		return false, fmt.Errorf("获取 bdstoken 失败: %w", err)
	}

	if bdstoken == "" {
		return false, errors.New("获取的 bdstoken 为空")
	}

	b.ctx.Log.Info("Cookie 测试成功", zap.String("bdstoken", bdstoken))
	return true, nil
}

// GetDirectories 获取根目录列表
func (b *BaiduCloudTransfer) GetDirectories() ([]interface{}, error) {
	if b.baiduUtils == nil {
		return nil, errors.New("baidu utils not initialized")
	}

	// 获取根目录列表
	dirs, err := b.baiduUtils.GetRootDirList()
	if err != nil {
		return nil, fmt.Errorf("获取目录列表失败: %w", err)
	}

	// 转换为前端需要的格式
	result := make([]interface{}, len(dirs))
	for i, dir := range dirs {
		result[i] = map[string]interface{}{
			"server_filename": dir.ServerFilename,
			"fs_id":          dir.FSID,
			"is_dir":         dir.IsDir,
		}
	}

	b.ctx.Log.Info("获取目录列表成功", zap.Int("count", len(dirs)))
	return result, nil
}