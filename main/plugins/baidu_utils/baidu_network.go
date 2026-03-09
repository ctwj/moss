package baidu_utils

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"
)

// BaiduDirItem 百度网盘目录项
type BaiduDirItem struct {
	FSID           int64  `json:"fs_id"`
	ServerFilename string `json:"server_filename"`
	Size           int64  `json:"size"`
	MD5            string `json:"md5"`
	IsDir          int    `json:"isdir"`
	Path           string `json:"path"`
	Ctime          int64  `json:"ctime"`
	Mtime          int64  `json:"mtime"`
}

// setHeaders 设置请求头
func (b *BaiduUtils) setHeaders(req *http.Request) {
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
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.0.0 Safari/537.36")

	// 设置 Cookie
	if b.Cookie != "" {
		req.Header.Set("Cookie", b.Cookie)
	}
}

// readResponseBody 读取响应体并处理 gzip 解压缩
func (b *BaiduUtils) readResponseBody(resp *http.Response) ([]byte, error) {
	var reader io.Reader = resp.Body

	// 检查是否是 gzip 压缩
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gzReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("创建 gzip reader 失败: %w", err)
		}
		defer gzReader.Close()
		reader = gzReader
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("读取响应体失败: %w", err)
	}

	return body, nil
}

// GetBdstoken 获取 bdstoken
func (b *BaiduUtils) GetBdstoken() (string, error) {
	apiURL := fmt.Sprintf("%s/api/gettemplatevariable", BaiduPanBaseURL)
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

	resp, err := b.HttpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := b.readResponseBody(resp)
	if err != nil {
		return "", err
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}

	errno, _ := result["errno"].(float64)
	if errno != 0 {
		errorCode := int(errno)
		errorMsg := ErrorCodeMap[errorCode]
		if errorMsg == "" {
			errorMsg = "未知错误"
		}
		return "", fmt.Errorf("获取 bdstoken 失败, 错误码: %d, 错误信息: %s", errorCode, errorMsg)
	}

	resultData, ok := result["result"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("解析响应失败")
	}

	bdstoken, ok := resultData["bdstoken"].(string)
	if !ok {
		return "", fmt.Errorf("响应中缺少 bdstoken")
	}

	b.mu.Lock()
	b.bdstoken = bdstoken
	b.mu.Unlock()

	return bdstoken, nil
}

// GetDirList 获取指定目录下的文件列表
func (b *BaiduUtils) GetDirList(dir string) ([]BaiduDirItem, error) {
	if b.bdstoken == "" {
		if _, err := b.GetBdstoken(); err != nil {
			return nil, err
		}
	}

	apiURL := fmt.Sprintf("%s/api/list", BaiduPanBaseURL)
	params := url.Values{}
	params.Set("order", "time")
	params.Set("desc", "1")
	params.Set("showempty", "0")
	params.Set("web", "1")
	params.Set("page", "1")
	params.Set("num", "1000")
	params.Set("dir", dir)
	params.Set("bdstoken", b.bdstoken)

	req, err := http.NewRequest("GET", apiURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}

	b.setHeaders(req)

	resp, err := b.HttpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := b.readResponseBody(resp)
	if err != nil {
		return nil, err
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	errno, _ := result["errno"].(float64)
	if errno != 0 {
		errorCode := int(errno)
		errorMsg := ErrorCodeMap[errorCode]
		if errorMsg == "" {
			errorMsg = "未知错误"
		}
		return nil, fmt.Errorf("获取目录列表失败, 错误码: %d, 错误信息: %s", errorCode, errorMsg)
	}

	list, ok := result["list"].([]any)
	if !ok {
		return nil, fmt.Errorf("解析响应失败")
	}

	var items []BaiduDirItem
	for _, v := range list {
		itemMap, ok := v.(map[string]any)
		if !ok {
			continue
		}

		item := BaiduDirItem{}
		if fsID, ok := itemMap["fs_id"].(float64); ok {
			item.FSID = int64(fsID)
		}
		if serverFilename, ok := itemMap["server_filename"].(string); ok {
			item.ServerFilename = serverFilename
		}
		if size, ok := itemMap["size"].(float64); ok {
			item.Size = int64(size)
		}
		if md5, ok := itemMap["md5"].(string); ok {
			item.MD5 = md5
		}
		if isDir, ok := itemMap["isdir"].(float64); ok {
			item.IsDir = int(isDir)
		}
		if path, ok := itemMap["path"].(string); ok {
			item.Path = path
		}
		if ctime, ok := itemMap["ctime"].(float64); ok {
			item.Ctime = int64(ctime)
		}
		if mtime, ok := itemMap["mtime"].(float64); ok {
			item.Mtime = int64(mtime)
		}

		items = append(items, item)
	}

	return items, nil
}

// CreateDir 创建新目录
func (b *BaiduUtils) CreateDir(path string) error {
	if b.bdstoken == "" {
		if _, err := b.GetBdstoken(); err != nil {
			return err
		}
	}

	apiURL := fmt.Sprintf("%s/api/create", BaiduPanBaseURL)
	params := url.Values{}
	params.Set("a", "commit")
	params.Set("bdstoken", b.bdstoken)

	data := fmt.Sprintf(`path=%s&isdir=1&block_list=[]`, url.QueryEscape(path))

	req, err := http.NewRequest("POST", apiURL+"?"+params.Encode(), strings.NewReader(data))
	if err != nil {
		return err
	}

	b.setHeaders(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := b.HttpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := b.readResponseBody(resp)
	if err != nil {
		return err
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return err
	}

	errno, _ := result["errno"].(float64)
	if errno != 0 {
		errorCode := int(errno)
		errorMsg := ErrorCodeMap[errorCode]
		if errorMsg == "" {
			errorMsg = "未知错误"
		}
		return fmt.Errorf("创建目录失败, 错误码: %d, 错误信息: %s", errorCode, errorMsg)
	}

	return nil
}

// CreateShare 创建分享链接
// 支持单个或多个文件分享
func (b *BaiduUtils) CreateShare(fsIDs []int64, period, pwd string) (string, error) {
	if b.bdstoken == "" {
		if _, err := b.GetBdstoken(); err != nil {
			return "", err
		}
	}

	if len(fsIDs) == 0 {
		return "", fmt.Errorf("没有可分享的文件")
	}

	apiURL := fmt.Sprintf("%s/share/pset", BaiduPanBaseURL)
	params := url.Values{}
	params.Set("channel", "chunlei")
	params.Set("bdstoken", b.bdstoken)
	params.Set("clienttype", "0")
	params.Set("app_id", "250528")
	params.Set("web", "1")
	params.Set("dp-logid", fmt.Sprintf("%d", time.Now().UnixNano()))

	// 构建 fid_list JSON 数组
	var fidList strings.Builder
	fidList.WriteString("[")
	for i, fsID := range fsIDs {
		if i > 0 {
			fidList.WriteString(",")
		}
		fidList.WriteString(fmt.Sprintf("%d", fsID))
	}
	fidList.WriteString("]")

	data := fmt.Sprintf(`period=%s&pwd=%s&eflag_disable=true&channel_list=[]&schannel=4&fid_list=%s`,
		period, pwd, fidList.String())

	req, err := http.NewRequest("POST", apiURL+"?"+params.Encode(), strings.NewReader(data))
	if err != nil {
		return "", err
	}

	b.setHeaders(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// 添加详细的调试日志
	b.Logger.Debug("创建分享链接请求详情",
		zap.String("url", apiURL+"?"+params.Encode()),
		zap.String("method", "POST"),
		zap.String("data", data),
		zap.Int("fid_count", len(fsIDs)),
		zap.Int64s("fs_ids", fsIDs))

	resp, err := b.HttpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := b.readResponseBody(resp)
	if err != nil {
		b.Logger.Error("读取响应体失败", zap.Error(err))
		return "", err
	}

	// 打印响应详情
	b.Logger.Debug("创建分享链接响应详情",
		zap.Int("status", resp.StatusCode),
		zap.String("response_body", string(body)),
		zap.Any("headers", resp.Header))

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		b.Logger.Error("解析 JSON 失败", zap.Error(err), zap.String("response", string(body)))
		return "", err
	}

	errno, _ := result["errno"].(float64)
	if errno != 0 {
		errorCode := int(errno)
		errorMsg := ErrorCodeMap[errorCode]
		if errorMsg == "" {
			errorMsg = "未知错误"
		}
		b.Logger.Error("创建分享链接失败",
			zap.Int("error_code", errorCode),
			zap.String("error_msg", errorMsg),
			zap.String("response", string(body)))
		return "", fmt.Errorf("创建分享链接失败, 错误码: %d, 错误信息: %s", errorCode, errorMsg)
	}

	link, ok := result["link"].(string)
	if !ok {
		return "", fmt.Errorf("解析分享链接失败")
	}

	shareURL := link
	if pwd != "" {
		shareURL = shareURL + "?pwd=" + pwd
	}

	return shareURL, nil
}