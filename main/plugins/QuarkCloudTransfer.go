package plugins

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"moss/domain/core/entity"
	"moss/domain/core/service"
	"moss/domain/core/vo"
	pluginEntity "moss/domain/support/entity"
	repositorycontext "moss/domain/core/repository/context"
)

// QuarkCloudTransfer 夸克网盘转存插件
type QuarkCloudTransfer struct {
	Cookie     string `json:"cookie"`      // 夸克网盘 Cookie
	SaveDir    string `json:"save_dir"`    // 保存目录（默认为根目录）
	RateLimit  int    `json:"rate_limit"`  // 速率限制（次/分钟）
	AdKeywords string `json:"ad_keywords"` // 删除广告的关键词（逗号分隔）
	AdUrls     string `json:"ad_urls"`     // 添加广告的地址列表（换行分隔）

	// 运行时字段（不持久化）
	ctx         *pluginEntity.Plugin
	httpClient  *http.Client
	lastRequest time.Time
	mu          sync.Mutex
}

// QuarkLink 夸克链接结构
type QuarkLink struct {
	URL      string `json:"url"`
	Password string `json:"password"`
}

// QuarkSavedItem 保存的夸克资源记录
type QuarkSavedItem struct {
	Type       string `json:"type"`
	URL        string `json:"url"`
	Password   string `json:"password"`
	Status     string `json:"status"`
	SavedPath  string `json:"saved_path"`
	Timestamp  string `json:"timestamp"`
	Error      string `json:"error,omitempty"`
}

// NewQuarkCloudTransfer 创建夸克网盘转存插件实例
func NewQuarkCloudTransfer() *QuarkCloudTransfer {
	return &QuarkCloudTransfer{
		SaveDir:   "0",    // 默认保存到根目录
		RateLimit: 10,    // 默认每分钟10次
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Info 返回插件信息
func (q *QuarkCloudTransfer) Info() *pluginEntity.PluginInfo {
	return &pluginEntity.PluginInfo{
		ID:         "QuarkCloudTransfer",
		About:      "定时转存文章中的夸克网盘链接",
		RunEnable:  true,
		CronEnable: true,
		PluginInfoPersistent: pluginEntity.PluginInfoPersistent{
			CronStart: true,
			CronExp:   "@every 24h",
		},
	}
}

// Load 加载插件
func (q *QuarkCloudTransfer) Load(ctx *pluginEntity.Plugin) error {
	q.ctx = ctx

	// 清理 Cookie
	q.Cookie = strings.TrimSpace(q.Cookie)
	q.Cookie = strings.ReplaceAll(q.Cookie, "\n", "")
	q.Cookie = strings.ReplaceAll(q.Cookie, "\r", "")
	q.Cookie = strings.ReplaceAll(q.Cookie, "\t", "")

	// 解析广告关键词
	if q.AdKeywords != "" {
		q.ctx.Log.Info("广告关键词配置", zap.String("keywords", q.AdKeywords))
	}

	// 解析广告 URL 列表
	if q.AdUrls != "" {
		adURLs := q.parseAdURLs(q.AdUrls)
		q.ctx.Log.Info("广告 URL 配置", zap.Int("count", len(adURLs)))
	}

	q.ctx.Log.Info("夸克网盘转存插件加载成功")
	return nil
}

// Run 执行插件
func (q *QuarkCloudTransfer) Run(ctx *pluginEntity.Plugin) error {
	q.ctx.Log.Info("开始执行夸克网盘转存任务")
	return q.processTransfer()
}

// processTransfer 处理转存逻辑
func (q *QuarkCloudTransfer) processTransfer() error {
	// 检查 Cookie 是否配置
	if q.Cookie == "" {
		q.ctx.Log.Warn("夸克网盘 Cookie 未配置，跳过转存任务")
		return nil
	}

	// 获取文章列表
	articleBases, err := service.Article.List(repositorycontext.NewContext(1000, ""))
	if err != nil {
		q.ctx.Log.Error("获取文章列表失败", zap.Error(err))
		return err
	}

	q.ctx.Log.Info("开始扫描文章", zap.Int("total", len(articleBases)))

	successCount := 0
	failedCount := 0
	skipCount := 0

	// 遍历文章
	for _, articleBase := range articleBases {
		article, err := service.Article.Get(articleBase.ID)
		if err != nil {
			q.ctx.Log.Error("获取文章详情失败",
				zap.Int("article_id", articleBase.ID),
				zap.Error(err))
			continue
		}

		// 提取夸克链接
		quarkLinks := q.extractQuarkLinks(article.Res)
		if len(quarkLinks) == 0 {
			skipCount++
			continue
		}

		q.ctx.Log.Info("发现夸克链接",
			zap.Int("article_id", article.ID),
			zap.String("title", article.Title),
			zap.Int("link_count", len(quarkLinks)))

		// 处理每个链接
		for _, link := range quarkLinks {
			q.applyRateLimit()

			// 提取分享 ID
			shareID := q.extractShareID(link.URL)
			if shareID == "" {
				q.ctx.Log.Warn("无效的夸克链接格式", zap.String("url", link.URL))
				failedCount++
				continue
			}

			// 执行转存
			savedItem, err := q.transferQuarkLink(shareID, link.URL, link.Password)
			if err != nil {
				q.ctx.Log.Error("转存失败",
					zap.Int("article_id", article.ID),
					zap.String("url", link.URL),
					zap.Error(err))
				failedCount++
				continue
			}

			// 更新文章
			if err := q.updateArticleRes(article, savedItem); err != nil {
				q.ctx.Log.Error("更新文章失败",
					zap.Int("article_id", article.ID),
					zap.Error(err))
			} else {
				successCount++
			}

			// 避免频繁请求
			time.Sleep(1 * time.Second)
		}
	}

	q.ctx.Log.Info("转存任务完成",
		zap.Int("success", successCount),
		zap.Int("failed", failedCount),
		zap.Int("skipped", skipCount))

	return nil
}

// isQuarkURL 判断是否为夸克网盘链接
func isQuarkURL(url string) bool {
	return strings.Contains(url, "pan.quark.cn")
}

// extractQuarkLinks 从文章的 download_links 中提取夸克链接
func (q *QuarkCloudTransfer) extractQuarkLinks(res vo.Extends) []QuarkLink {
	var links []QuarkLink

	for _, item := range res {
		if item.Key == "download_links" {
			if value, ok := item.Value.([]any); ok {
				for _, v := range value {
					if linkMap, ok := v.(map[string]any); ok {
						url := ""
						password := ""

						// 提取 URL 和密码
						if urlVal, ok := linkMap["url"].(string); ok {
							url = urlVal
						}
						if pwdVal, ok := linkMap["password"].(string); ok {
							password = pwdVal
						}

						// 通过 URL 判断是否为夸克链接
						if url != "" && isQuarkURL(url) {
							links = append(links, QuarkLink{
								URL:      url,
								Password: password,
							})
						}
					}
				}
			}
		}
	}

	return links
}

// extractShareID 从 URL 中提取分享 ID
func (q *QuarkCloudTransfer) extractShareID(url string) string {
	if idx := strings.Index(url, "/s/"); idx != -1 {
		shareID := url[idx+3:]
		// 移除可能的查询参数和锚点
		if idx := strings.Index(shareID, "?"); idx != -1 {
			shareID = shareID[:idx]
		}
		if idx := strings.Index(shareID, "#"); idx != -1 {
			shareID = shareID[:idx]
		}
		return shareID
	}
	return ""
}

// parseSavedLinks 解析已保存的记录
func (q *QuarkCloudTransfer) parseSavedLinks(res vo.Extends) []QuarkSavedItem {
	var saved []QuarkSavedItem

	for _, item := range res {
		if item.Key == "saved" {
			if value, ok := item.Value.([]any); ok {
				for _, v := range value {
					if savedMap, ok := v.(map[string]any); ok {
						savedItem := QuarkSavedItem{
							Type:      "quark",
							URL:       "",
							Password:  "",
							Status:    "",
							SavedPath: "",
							Timestamp: "",
						}

						if typ, ok := savedMap["type"].(string); ok {
							savedItem.Type = typ
						}
						if url, ok := savedMap["url"].(string); ok {
							savedItem.URL = url
						}
						if pwd, ok := savedMap["password"].(string); ok {
							savedItem.Password = pwd
						}
						if status, ok := savedMap["status"].(string); ok {
							savedItem.Status = status
						}
						if path, ok := savedMap["saved_path"].(string); ok {
							savedItem.SavedPath = path
						}
						if ts, ok := savedMap["timestamp"].(string); ok {
							savedItem.Timestamp = ts
						}
						if err, ok := savedMap["error"].(string); ok {
							savedItem.Error = err
						}

						saved = append(saved, savedItem)
					}
				}
			}
		}
	}

	return saved
}

// updateArticleRes 更新文章的 res 字段
func (q *QuarkCloudTransfer) updateArticleRes(article *entity.Article, savedItem *QuarkSavedItem) error {
	// 解析已保存的记录
	saved := q.parseSavedLinks(article.Res)

	// 查找相同类型的记录并替换
	found := false
	for i, item := range saved {
		if item.Type == "quark" {
			saved[i] = *savedItem
			found = true
			break
		}
	}

	// 如果没找到，添加新记录
	if !found {
		saved = append(saved, *savedItem)
	}

	// 更新 res 字段
	for i, item := range article.Res {
		if item.Key == "saved" {
			article.Res[i].Value = saved
			break
		}
	}

	// 保存到数据库
	return service.Article.Update(article)
}

// applyRateLimit 应用速率限制
func (q *QuarkCloudTransfer) applyRateLimit() {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.RateLimit <= 0 {
		return
	}

	interval := time.Minute / time.Duration(q.RateLimit)
	if !q.lastRequest.IsZero() {
		wait := time.Until(q.lastRequest.Add(interval))
		if wait > 0 {
			q.ctx.Log.Debug("速率限制等待", zap.Duration("wait", wait))
			time.Sleep(wait)
		}
	}
	q.lastRequest = time.Now()
}

// transferQuarkLink 转存夸克链接
func (q *QuarkCloudTransfer) transferQuarkLink(shareID, originalURL, password string) (*QuarkSavedItem, error) {
	q.ctx.Log.Info("开始转存夸克链接", zap.String("share_id", shareID), zap.String("original_url", originalURL))

	savedItem := &QuarkSavedItem{
		Type:      "quark",
		URL:       originalURL,
		Password:  password,
		Status:    "failed",
		Timestamp: time.Now().Format("2006-01-02T15:04:05Z"),
	}

	// 获取 stoken
	q.ctx.Log.Debug("正在获取stoken", zap.String("share_id", shareID))
	stoken, err := q.getStoken(shareID)
	if err != nil {
		savedItem.Error = fmt.Sprintf("获取stoken失败: %v", err)
		q.ctx.Log.Error("获取stoken失败", zap.String("share_id", shareID), zap.Error(err))
		return savedItem, err
	}
	q.ctx.Log.Debug("获取stoken成功", zap.String("share_id", shareID))

	// 替换空格为 + 号
	stoken = strings.ReplaceAll(stoken, " ", "+")

	// 获取分享详情
	q.ctx.Log.Debug("正在获取分享详情", zap.String("share_id", shareID))
	shareDetail, err := q.getShare(shareID, stoken)
	if err != nil {
		savedItem.Error = fmt.Sprintf("获取分享详情失败: %v", err)
		q.ctx.Log.Error("获取分享详情失败", zap.String("share_id", shareID), zap.Error(err))
		return savedItem, err
	}
	q.ctx.Log.Info("获取分享详情成功", zap.String("share_id", shareID), zap.String("title", shareDetail.Share.Title))

	// 检查是否为检验模式
	// 这里默认为转存模式（IsType == 0）

	// 提取文件信息
	fidList := make([]string, 0)
	fidTokenList := make([]string, 0)
	title := shareDetail.Share.Title

	for _, item := range shareDetail.List {
		fidList = append(fidList, item.Fid)
		fidTokenList = append(fidTokenList, item.ShareFidToken)
	}

	if len(fidList) == 0 {
		savedItem.Error = "文件列表为空"
		q.ctx.Log.Error("文件列表为空", zap.String("share_id", shareID))
		return savedItem, fmt.Errorf("文件列表为空")
	}

	q.ctx.Log.Info("提取文件信息成功", zap.String("share_id", shareID), zap.Int("file_count", len(fidList)), zap.String("save_dir", q.SaveDir))

	// 转存资源
	q.ctx.Log.Info("开始转存资源", zap.String("share_id", shareID), zap.String("save_dir", q.SaveDir), zap.Int("file_count", len(fidList)))
	saveResult, err := q.getShareSave(shareID, stoken, fidList, fidTokenList)
	if err != nil {
		savedItem.Error = fmt.Sprintf("转存失败: %v", err)
		q.ctx.Log.Error("转存失败", zap.String("share_id", shareID), zap.Error(err))
		return savedItem, err
	}

	taskID := saveResult.TaskID
	q.ctx.Log.Info("转存任务已创建", zap.String("task_id", taskID))

	// 等待转存完成
	q.ctx.Log.Info("等待转存完成", zap.String("task_id", taskID))
	myData, err := q.waitForTask(taskID)
	if err != nil {
		savedItem.Error = fmt.Sprintf("等待转存完成失败: %v", err)
		q.ctx.Log.Error("等待转存完成失败", zap.String("task_id", taskID), zap.Error(err))
		return savedItem, err
	}

	// 获取保存的目录ID
	if len(myData.SaveAs.SaveAsTopFids) == 0 {
		savedItem.Error = "转存后未获取到文件ID"
		q.ctx.Log.Error("转存后未获取到文件ID", zap.String("task_id", taskID))
		return savedItem, fmt.Errorf("转存后未获取到文件ID")
	}

	dirID := myData.SaveAs.SaveAsTopFids[0]
	savedItem.SavedPath = dirID
	q.ctx.Log.Info("转存完成", zap.String("task_id", taskID), zap.String("dir_id", dirID), zap.Int("saved_count", len(myData.SaveAs.SaveAsTopFids)))

	// 删除广告文件
	if q.AdKeywords != "" {
		q.ctx.Log.Info("开始删除广告文件", zap.String("dir_id", dirID))
		if err := q.deleteAdFiles(dirID); err != nil {
			q.ctx.Log.Warn("删除广告文件失败", zap.Error(err))
		}
	}

	// 添加自定义广告
	if q.AdUrls != "" {
		q.ctx.Log.Info("开始添加自定义广告", zap.String("dir_id", dirID))
		if err := q.addAd(dirID); err != nil {
			q.ctx.Log.Warn("添加广告文件失败", zap.Error(err))
		}
	}

	// 分享资源
	q.ctx.Log.Info("开始创建分享", zap.String("dir_id", dirID), zap.String("title", title))
	shareBtnResult, err := q.getShareBtn(myData.SaveAs.SaveAsTopFids, title)
	if err != nil {
		savedItem.Error = fmt.Sprintf("创建分享失败: %v", err)
		q.ctx.Log.Error("创建分享失败", zap.Error(err))
		return savedItem, err
	}

	// 等待分享完成
	q.ctx.Log.Info("等待分享完成", zap.String("share_task_id", shareBtnResult.TaskID))
	shareTaskResult, err := q.waitForTask(shareBtnResult.TaskID)
	if err != nil {
		savedItem.Error = fmt.Sprintf("等待分享完成失败: %v", err)
		q.ctx.Log.Error("等待分享完成失败", zap.String("share_task_id", shareBtnResult.TaskID), zap.Error(err))
		return savedItem, err
	}

	q.ctx.Log.Info("分享完成", zap.String("share_id", shareTaskResult.ShareID))

	// 获取分享密码
	passwordResult, err := q.getSharePassword(shareTaskResult.ShareID)
	if err != nil {
		savedItem.Error = fmt.Sprintf("获取分享密码失败: %v", err)
		return savedItem, err
	}

	// 确定最终fid（关键修复）
	var fid string
	if len(myData.SaveAs.SaveAsTopFids) > 1 {
		// 多个文件，使用逗号分隔
		fid = strings.Join(myData.SaveAs.SaveAsTopFids, ",")
	} else {
		// 单个文件，使用FirstFile.Fid
		fid = passwordResult.FirstFile.Fid
	}

	// 构建最终分享链接
	shareURL := passwordResult.ShareURL + "?pwd=" + passwordResult.Code
	savedItem.URL = shareURL
	savedItem.Password = passwordResult.Code
	savedItem.Status = "success"

	q.ctx.Log.Info("转存成功",
		zap.String("share_id", shareID),
		zap.String("share_url", shareURL),
		zap.String("code", passwordResult.Code),
		zap.String("fid", fid))

	return savedItem, nil
}

// ==================== 夸克网盘 API 调用方法 ====================

// getStoken 获取 stoken
func (q *QuarkCloudTransfer) getStoken(shareID string) (string, error) {
	data := map[string]interface{}{
		"passcode": "",
		"pwd_id":   shareID,
	}

	queryParams := map[string]string{
		"pr":           "ucpro",
		"fr":           "pc",
		"uc_param_str": "",
	}

	respData, err := q.httpPost("https://drive-pc.quark.cn/1/clouddrive/share/sharepage/token", data, queryParams)
	if err != nil {
		return "", err
	}

	var response struct {
		Status  int          `json:"status"`
		Message string       `json:"message"`
		Data    StokenResult `json:"data"`
	}

	if err := json.Unmarshal(respData, &response); err != nil {
		return "", err
	}

	if response.Status != 200 {
		return "", fmt.Errorf(response.Message)
	}

	return response.Data.Stoken, nil
}

// getShare 获取分享详情
func (q *QuarkCloudTransfer) getShare(shareID, stoken string) (*ShareResult, error) {
	queryParams := map[string]string{
		"pr":            "ucpro",
		"fr":            "pc",
		"uc_param_str":  "",
		"pwd_id":        shareID,
		"stoken":        stoken,
		"pdir_fid":      "0",
		"force":         "0",
		"_page":         "1",
		"_size":         "100",
		"_fetch_banner": "1",
		"_fetch_share":  "1",
		"_fetch_total":  "1",
		"_sort":         "file_type:asc,updated_at:desc",
	}

	respData, err := q.httpGet("https://drive-pc.quark.cn/1/clouddrive/share/sharepage/detail", queryParams)
	if err != nil {
		return nil, err
	}

	var response struct {
		Status  int         `json:"status"`
		Message string      `json:"message"`
		Data    ShareResult `json:"data"`
	}

	if err := json.Unmarshal(respData, &response); err != nil {
		return nil, err
	}

	if response.Status != 200 {
		return nil, fmt.Errorf(response.Message)
	}

	return &response.Data, nil
}

// getShareSave 转存分享
func (q *QuarkCloudTransfer) getShareSave(shareID, stoken string, fidList, fidTokenList []string) (*SaveResult, error) {
	return q.getShareSaveToDir(shareID, stoken, fidList, fidTokenList, q.SaveDir)
}

// getShareSaveToDir 转存分享到指定目录
func (q *QuarkCloudTransfer) getShareSaveToDir(shareID, stoken string, fidList, fidTokenList []string, toPdirFid string) (*SaveResult, error) {
	data := map[string]interface{}{
		"pwd_id":         shareID,
		"stoken":         stoken,
		"fid_list":       fidList,
		"fid_token_list": fidTokenList,
		"to_pdir_fid":    toPdirFid,
	}

	queryParams := map[string]string{
		"pr":           "ucpro",
		"fr":           "pc",
		"uc_param_str": "",
	}

	respData, err := q.httpPost("https://drive-pc.quark.cn/1/clouddrive/share/sharepage/save", data, queryParams)
	if err != nil {
		return nil, err
	}

	var response struct {
		Status  int        `json:"status"`
		Message string     `json:"message"`
		Data    SaveResult `json:"data"`
	}

	if err := json.Unmarshal(respData, &response); err != nil {
		return nil, err
	}

	if response.Status != 200 {
		return nil, fmt.Errorf(response.Message)
	}

	return &response.Data, nil
}

// getShareBtn 分享按钮
func (q *QuarkCloudTransfer) getShareBtn(fidList []string, title string) (*ShareBtnResult, error) {
	data := map[string]interface{}{
		"fid_list":     fidList,
		"title":        title,
		"url_type":     1,
		"expired_type": 1, // 永久分享
	}

	queryParams := map[string]string{
		"pr":           "ucpro",
		"fr":           "pc",
		"uc_param_str": "",
	}

	respData, err := q.httpPost("https://drive-pc.quark.cn/1/clouddrive/share", data, queryParams)
	if err != nil {
		return nil, err
	}

	var response struct {
		Status  int            `json:"status"`
		Message string         `json:"message"`
		Data    ShareBtnResult `json:"data"`
	}

	if err := json.Unmarshal(respData, &response); err != nil {
		return nil, err
	}

	if response.Status != 200 {
		return nil, fmt.Errorf(response.Message)
	}

	return &response.Data, nil
}

// getShareTask 获取分享任务状态
func (q *QuarkCloudTransfer) getShareTask(taskID string, retryIndex int) (*TaskResult, error) {
	timestamp := time.Now().UnixNano() / int64(time.Millisecond)
	timestampStr := strconv.FormatInt(timestamp, 10)
	if len(timestampStr) > 13 {
		timestampStr = timestampStr[:13]
	}

	queryParams := map[string]string{
		"pr":           "ucpro",
		"fr":           "pc",
		"uc_param_str": "",
		"task_id":      taskID,
		"retry_index":  fmt.Sprintf("%d", retryIndex),
		"__dt":         "21192",
		"__t":          timestampStr,
	}

	respData, err := q.httpGet("https://drive-pc.quark.cn/1/clouddrive/task", queryParams)
	if err != nil {
		return nil, err
	}

	var response struct {
		Status  int        `json:"status"`
		Message string     `json:"message"`
		Data    TaskResult `json:"data"`
	}

	if err := json.Unmarshal(respData, &response); err != nil {
		return nil, err
	}

	if response.Status != 200 {
		return nil, fmt.Errorf(response.Message)
	}

	return &response.Data, nil
}

// getSharePassword 获取分享密码
func (q *QuarkCloudTransfer) getSharePassword(shareID string) (*PasswordResult, error) {
	queryParams := map[string]string{
		"pr":           "ucpro",
		"fr":           "pc",
		"uc_param_str": "",
	}

	data := map[string]interface{}{
		"share_id": shareID,
	}

	respData, err := q.httpPost("https://drive-pc.quark.cn/1/clouddrive/share/password", data, queryParams)
	if err != nil {
		return nil, err
	}

	var response struct {
		Status  int            `json:"status"`
		Message string         `json:"message"`
		Data    PasswordResult `json:"data"`
	}

	if err := json.Unmarshal(respData, &response); err != nil {
		return nil, err
	}

	if response.Status != 200 {
		return nil, fmt.Errorf(response.Message)
	}

	return &response.Data, nil
}

// waitForTask 等待任务完成
func (q *QuarkCloudTransfer) waitForTask(taskID string) (*TaskResult, error) {
	maxRetries := 50
	retryDelay := 2 * time.Second

	for retryIndex := 0; retryIndex < maxRetries; retryIndex++ {
		result, err := q.getShareTask(taskID, retryIndex)
		if err != nil {
			if strings.Contains(err.Error(), "capacity limit[{0}]") {
				return nil, fmt.Errorf("容量不足")
			}
			return nil, err
		}

		if result.Status == 2 { // 任务完成
			return result, nil
		}

		time.Sleep(retryDelay)
	}

	return nil, fmt.Errorf("任务超时")
}

// ==================== 广告处理方法 ====================

// deleteAdFiles 删除广告文件
func (q *QuarkCloudTransfer) deleteAdFiles(pdirFid string) error {
	q.ctx.Log.Info("开始删除广告文件", zap.String("dir_id", pdirFid))

	// 获取目录文件列表
	fileList, err := q.getDirFile(pdirFid)
	if err != nil {
		q.ctx.Log.Warn("获取目录文件失败", zap.Error(err))
		return err
	}

	if len(fileList) == 0 {
		q.ctx.Log.Info("目录为空，无需删除广告文件")
		return nil
	}

	// 解析广告关键词
	adKeywords := q.parseAdKeywords(q.AdKeywords)
	if len(adKeywords) == 0 {
		q.ctx.Log.Info("未配置广告关键词，跳过删除")
		return nil
	}

	// 删除包含广告关键词的文件
	deletedCount := 0
	for _, file := range fileList {
		if fileName, ok := file["file_name"].(string); ok {
			if q.checkKeywordsInFilename(fileName, adKeywords) {
				if fid, ok := file["fid"].(string); ok {
					q.ctx.Log.Info("删除广告文件", zap.String("name", fileName), zap.String("fid", fid))
					if err := q.deleteSingleFile(fid); err != nil {
						q.ctx.Log.Warn("删除广告文件失败", zap.String("fid", fid), zap.Error(err))
					} else {
						deletedCount++
					}
				}
			}
		}
	}

	q.ctx.Log.Info("广告文件删除完成", zap.Int("deleted_count", deletedCount))
	return nil
}

// addAd 添加个人自定义广告
func (q *QuarkCloudTransfer) addAd(dirID string) error {
	q.ctx.Log.Info("开始添加自定义广告", zap.String("dir_id", dirID))

	// 解析广告 URL 列表
	adURLs := q.parseAdURLs(q.AdUrls)
	if len(adURLs) == 0 {
		q.ctx.Log.Info("未配置广告 URL，跳过添加")
		return nil
	}

	// 随机选择一个广告文件
	rand.Seed(time.Now().UnixNano())
	selectedAdURL := adURLs[rand.Intn(len(adURLs))]

	q.ctx.Log.Info("选择广告文件", zap.String("url", selectedAdURL))

	// 提取广告文件 ID
	adShareID := q.extractShareID(selectedAdURL)
	if adShareID == "" {
		return fmt.Errorf("无效的广告链接格式: %s", selectedAdURL)
	}

	// 获取广告文件的 stoken
	stoken, err := q.getStoken(adShareID)
	if err != nil {
		return fmt.Errorf("获取广告文件 stoken 失败: %v", err)
	}
	stoken = strings.ReplaceAll(stoken, " ", "+")

	// 获取广告文件详情
	adDetail, err := q.getShare(adShareID, stoken)
	if err != nil {
		return fmt.Errorf("获取广告文件详情失败: %v", err)
	}

	if len(adDetail.List) == 0 {
		return fmt.Errorf("广告文件详情为空")
	}

	// 获取第一个广告文件的信息
	adFile := adDetail.List[0]
	fid := adFile.Fid
	shareFidToken := adFile.ShareFidToken

	// 保存广告文件到目标目录
	saveResult, err := q.getShareSaveToDir(adShareID, stoken, []string{fid}, []string{shareFidToken}, dirID)
	if err != nil {
		return fmt.Errorf("保存广告文件失败: %v", err)
	}

	// 等待保存完成
	_, err = q.waitForTask(saveResult.TaskID)
	if err != nil {
		return fmt.Errorf("等待广告文件保存完成失败: %v", err)
	}

	q.ctx.Log.Info("广告文件添加成功")
	return nil
}

// getDirFile 获取指定文件夹的文件列表
func (q *QuarkCloudTransfer) getDirFile(pdirFid string) ([]map[string]interface{}, error) {
	queryParams := map[string]string{
		"pr":              "ucpro",
		"fr":              "pc",
		"uc_param_str":    "",
		"pdir_fid":        pdirFid,
		"_page":           "1",
		"_size":           "50",
		"_fetch_total":    "1",
		"_fetch_sub_dirs": "0",
		"_sort":           "updated_at:desc",
	}

	respData, err := q.httpGet("https://drive-pc.quark.cn/1/clouddrive/file/sort", queryParams)
	if err != nil {
		return nil, err
	}

	var response struct {
		Status  int    `json:"status"`
		Message string `json:"message"`
		Data    struct {
			List []map[string]interface{} `json:"list"`
		} `json:"data"`
	}

	if err := json.Unmarshal(respData, &response); err != nil {
		return nil, err
	}

	if response.Status != 200 {
		return nil, fmt.Errorf(response.Message)
	}

	return response.Data.List, nil
}

// deleteSingleFile 删除单个文件
func (q *QuarkCloudTransfer) deleteSingleFile(fileID string) error {
	data := map[string]interface{}{
		"action_type":  2,
		"filelist":     []string{fileID},
		"exclude_fids": []string{},
	}

	queryParams := map[string]string{
		"pr":           "ucpro",
		"fr":           "pc",
		"uc_param_str": "",
	}

	respData, err := q.httpPost("https://drive-pc.quark.cn/1/clouddrive/file/delete", data, queryParams)
	if err != nil {
		return err
	}

	var response struct {
		Status  int    `json:"status"`
		Message string `json:"message"`
		Data    struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
	}

	if err := json.Unmarshal(respData, &response); err != nil {
		return err
	}

	if response.Status != 200 {
		return fmt.Errorf(response.Message)
	}

	// 如果有任务ID，等待任务完成
	if response.Data.TaskID != "" {
		_, err := q.waitForTask(response.Data.TaskID)
		if err != nil {
			return err
		}
	}

	return nil
}

// parseAdKeywords 解析广告关键词（支持中英文逗号）
func (q *QuarkCloudTransfer) parseAdKeywords(keywordsStr string) []string {
	if keywordsStr == "" {
		return []string{}
	}

	// 使用正则表达式同时匹配中英文逗号
	re := regexp.MustCompile(`[,，]`)
	parts := re.Split(keywordsStr, -1)

	var result []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	return result
}

// parseAdURLs 解析广告 URL 列表（按换行符分割）
func (q *QuarkCloudTransfer) parseAdURLs(urlsStr string) []string {
	if urlsStr == "" {
		return []string{}
	}

	// 按换行符分割
	lines := strings.Split(urlsStr, "\n")
	var result []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	return result
}

// checkKeywordsInFilename 检查文件名是否包含指定关键词
func (q *QuarkCloudTransfer) checkKeywordsInFilename(filename string, keywords []string) bool {
	lowercaseFilename := strings.ToLower(filename)

	for _, keyword := range keywords {
		if strings.Contains(lowercaseFilename, strings.ToLower(keyword)) {
			return true
		}
	}

	return false
}

// ==================== HTTP 请求方法 ====================

// httpGet 发送 GET 请求
func (q *QuarkCloudTransfer) httpGet(url string, queryParams map[string]string) ([]byte, error) {
	// 构建完整 URL
	if len(queryParams) > 0 {
		params := make([]string, 0, len(queryParams))
		for k, v := range queryParams {
			params = append(params, fmt.Sprintf("%s=%s", k, v))
		}
		if len(params) > 0 {
			url = url + "?" + strings.Join(params, "&")
		}
	}

	// 创建请求
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// 设置请求头
	q.setRequestHeaders(req)

	// 发送请求
	resp, err := q.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := q.readResponseBody(resp)
	if err != nil {
		return nil, err
	}

	return body, nil
}

// httpPost 发送 POST 请求
func (q *QuarkCloudTransfer) httpPost(url string, data interface{}, queryParams map[string]string) ([]byte, error) {
	// 序列化请求体
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	// 构建完整 URL
	if len(queryParams) > 0 {
		params := make([]string, 0, len(queryParams))
		for k, v := range queryParams {
			params = append(params, fmt.Sprintf("%s=%s", k, v))
		}
		if len(params) > 0 {
			url = url + "?" + strings.Join(params, "&")
		}
	}

	// 创建请求
	req, err := http.NewRequest("POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, err
	}

	// 设置请求头
	q.setRequestHeaders(req)
	req.Header.Set("Content-Type", "application/json;charset=UTF-8")

	// 发送请求
	resp, err := q.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := q.readResponseBody(resp)
	if err != nil {
		return nil, err
	}

	return body, nil
}

// setRequestHeaders 设置请求头
func (q *QuarkCloudTransfer) setRequestHeaders(req *http.Request) {
	// 基础请求头（参考 demo 实现的完整请求头）
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	req.Header.Set("Content-Type", "application/json;charset=UTF-8")
	req.Header.Set("Sec-Ch-Ua", `"Chromium";v="122", "Not(A:Brand";v="24", "Google Chrome";v="122"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"Windows"`)
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-site")
	req.Header.Set("Referer", "https://pan.quark.cn/")
	req.Header.Set("Referrer-Policy", "strict-origin-when-cross-origin")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	// Cookie
	if q.Cookie != "" {
		req.Header.Set("Cookie", q.Cookie)
	}
}

// readResponseBody 读取响应体
func (q *QuarkCloudTransfer) readResponseBody(resp *http.Response) ([]byte, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP请求失败: %d, %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// ==================== 数据结构定义 ====================

// StokenResult stoken 结果
type StokenResult struct {
	Stoken string `json:"stoken"`
	Title  string `json:"title"`
}

// ShareResult 分享结果
type ShareResult struct {
	Share struct {
		Title string `json:"title"`
	} `json:"share"`
	List []struct {
		Fid           string `json:"fid"`
		ShareFidToken string `json:"share_fid_token"`
	} `json:"list"`
}

// SaveResult 保存结果
type SaveResult struct {
	TaskID string `json:"task_id"`
}

// ShareBtnResult 分享按钮结果
type ShareBtnResult struct {
	TaskID string `json:"task_id"`
}

// TaskResult 任务结果
type TaskResult struct {
	Status  int    `json:"status"`
	ShareID string `json:"share_id"`
	SaveAs  struct {
		SaveAsTopFids []string `json:"save_as_top_fids"`
	} `json:"save_as"`
}

// PasswordResult 密码结果
type PasswordResult struct {
	ShareURL   string `json:"share_url"`
	ShareTitle string `json:"share_title"`
	Code       string `json:"code"`
	FirstFile  struct {
		Fid string `json:"fid"`
	} `json:"first_file"`
}

// TestCookie 测试 Cookie 有效性
func (q *QuarkCloudTransfer) TestCookie() (bool, error) {
	// 获取用户信息
	userInfo, err := q.getUserInfo()
	if err != nil {
		return false, err
	}

	// 检查是否成功获取到用户信息
	if userInfo == nil {
		return false, fmt.Errorf("获取用户信息失败")
	}

	return true, nil
}

// TestCookieWithCookie 使用指定的 Cookie 测试有效性
func (q *QuarkCloudTransfer) TestCookieWithCookie(cookie string) (bool, error) {
	// 保存原始 Cookie
	originalCookie := q.Cookie
	defer func() {
		// 恢复原始 Cookie
		q.Cookie = originalCookie
	}()

	// 临时设置测试 Cookie
	q.Cookie = cookie

	// 获取用户信息
	userInfo, err := q.getUserInfo()
	if err != nil {
		return false, err
	}

	// 检查是否成功获取到用户信息
	if userInfo == nil {
		return false, fmt.Errorf("获取用户信息失败")
	}

	return true, nil
}

// GetDirectories 获取根目录列表
func (q *QuarkCloudTransfer) GetDirectories() ([]interface{}, error) {
	if q.Cookie == "" {
		return nil, fmt.Errorf("Cookie 为空，请先配置")
	}

	// 获取根目录文件列表（pdir_fid = "0" 表示根目录）
	files, err := q.getDirFile("0")
	if err != nil {
		return nil, fmt.Errorf("获取根目录列表失败: %w", err)
	}

	// 过滤出文件夹
	result := make([]interface{}, 0)
	for _, file := range files {
		// 检查是否为文件夹
		if isDir, ok := file["dir"].(bool); ok {
			// dir: true = 文件夹, false = 文件
			if isDir {
				result = append(result, map[string]interface{}{
					"server_filename": file["file_name"],
					"fid":            file["fid"],
					"is_dir":         true,
				})
			}
		}
	}

	q.ctx.Log.Info("获取目录列表成功", zap.Int("count", len(result)))
	return result, nil
}

// getUserInfo 获取用户信息
func (q *QuarkCloudTransfer) getUserInfo() (*QuarkUserInfo, error) {
	// 保存原始 Cookie
	originalCookie := q.Cookie

	// 临时设置测试 Cookie（确保使用传入的 Cookie）
	if q.Cookie == "" {
		return nil, fmt.Errorf("Cookie 为空")
	}

	// 创建临时 HTTP 客户端，确保使用完整的请求头
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// 获取用户基本信息
	queryParams := map[string]string{
		"platform": "pc",
		"fr":       "pc",
	}

	// 构建请求 URL
	accountInfoURL := "https://pan.quark.cn/account/info"
	if len(queryParams) > 0 {
		params := make([]string, 0, len(queryParams))
		for k, v := range queryParams {
			params = append(params, fmt.Sprintf("%s=%s", k, v))
		}
		if len(params) > 0 {
			accountInfoURL = accountInfoURL + "?" + strings.Join(params, "&")
		}
	}

	req, err := http.NewRequest("GET", accountInfoURL, nil)
	if err != nil {
		return nil, err
	}

	// 设置完整的请求头（参考 demo 实现）
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	req.Header.Set("Content-Type", "application/json;charset=UTF-8")
	req.Header.Set("Sec-Ch-Ua", `"Chromium";v="122", "Not(A:Brand";v="24", "Google Chrome";v="122"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"Windows"`)
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-site")
	req.Header.Set("Referer", "https://pan.quark.cn/")
	req.Header.Set("Referrer-Policy", "strict-origin-when-cross-origin")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Cookie", q.Cookie)

	// 发送请求
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("获取用户信息失败: %v", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %v", err)
	}

	// 解析响应
	var response struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
		Data    struct {
			Nickname  string `json:"nickname"`
			AvatarUri string `json:"avatarUri"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("解析用户信息失败: %v", err)
	}

	if !response.Success || response.Code != "OK" {
		return nil, fmt.Errorf("获取用户信息失败: code=%s, body=%s", response.Code, string(body))
	}

	// 恢复原始 Cookie
	q.Cookie = originalCookie

	// 获取用户详细信息（容量和会员信息）
	queryParams1 := map[string]string{
		"pr":              "ucpro",
		"fr":              "pc",
		"uc_param_str":    "",
		"fetch_subscribe": "true",
		"_ch":             "home",
		"fetch_identity":  "true",
	}

	// 构建请求 URL
	memberURL := "https://drive-pc.quark.cn/1/clouddrive/member"
	if len(queryParams1) > 0 {
		params := make([]string, 0, len(queryParams1))
		for k, v := range queryParams1 {
			params = append(params, fmt.Sprintf("%s=%s", k, v))
		}
		if len(params) > 0 {
			memberURL = memberURL + "?" + strings.Join(params, "&")
		}
	}

	req1, err := http.NewRequest("GET", memberURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建会员信息请求失败: %v", err)
	}

	// 设置相同的请求头
	req1.Header.Set("Accept", "application/json, text/plain, */*")
	req1.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	req1.Header.Set("Content-Type", "application/json;charset=UTF-8")
	req1.Header.Set("Sec-Ch-Ua", `"Chromium";v="122", "Not(A:Brand";v="24", "Google Chrome";v="122"`)
	req1.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req1.Header.Set("Sec-Ch-Ua-Platform", `"Windows"`)
	req1.Header.Set("Sec-Fetch-Dest", "empty")
	req1.Header.Set("Sec-Fetch-Mode", "cors")
	req1.Header.Set("Sec-Fetch-Site", "same-site")
	req1.Header.Set("Referer", "https://pan.quark.cn/")
	req1.Header.Set("Referrer-Policy", "strict-origin-when-cross-origin")
	req1.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req1.Header.Set("Cookie", q.Cookie)

	// 发送请求
	resp1, err := client.Do(req1)
	if err != nil {
		return nil, fmt.Errorf("获取会员信息失败: %v", err)
	}
	defer resp1.Body.Close()

	// 读取响应
	body1, err := io.ReadAll(resp1.Body)
	if err != nil {
		return nil, fmt.Errorf("读取会员信息响应失败: %v", err)
	}

	var memberResponse struct {
		Status  int    `json:"status"`
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			TotalCapacity int64  `json:"total_capacity"`
			UseCapacity   int64  `json:"use_capacity"`
			MemberType    string `json:"member_type"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body1, &memberResponse); err != nil {
		return nil, fmt.Errorf("解析会员信息失败: %v", err)
	}

	if memberResponse.Status != 200 || memberResponse.Code != 0 {
		return nil, fmt.Errorf("获取会员信息失败: %s, body=%s", memberResponse.Message, string(body1))
	}

	return &QuarkUserInfo{
		Username:    response.Data.Nickname,
		VIPStatus:   memberResponse.Data.MemberType != "NORMAL",
		UsedSpace:   memberResponse.Data.UseCapacity,
		TotalSpace:  memberResponse.Data.TotalCapacity,
	}, nil
}

// QuarkUserInfo 夸克用户信息
type QuarkUserInfo struct {
	Username    string `json:"username"`
	VIPStatus   bool   `json:"vip_status"`
	UsedSpace   int64  `json:"used_space"`
	TotalSpace  int64  `json:"total_space"`
}
