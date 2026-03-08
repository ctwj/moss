package baidu_utils

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// BaiduLinkInfo 百度网盘链接信息
type BaiduLinkInfo struct {
	URL      string `json:"url"`
	Password string `json:"password"`
}

// BaiduShareFile 百度网盘分享文件
type BaiduShareFile struct {
	ShareID        string `json:"share_id"`
	UserID         string `json:"user_id"`
	FSID           int64  `json:"fs_id"`
	ServerFilename string `json:"server_filename"`
	IsDir          int    `json:"is_dir"`
}

// NormalizeLink 标准化链接格式
// 支持格式：
// - https://pan.baidu.com/s/1xxx pwd
// - https://pan.baidu.com/s/1xxx?pwd=xxx
// - https://pan.baidu.com/s/1xxx 提取码：xxx
// - https://pan.baidu.com/share/init?surl=xxx
func NormalizeLink(link string) string {
	// 升级旧链接格式
	normalized := strings.ReplaceAll(link, "share/init?surl=", "s/1")

	// 替换掉 ?pwd= 或 &pwd= 为空格
	re := regexp.MustCompile(`[?&]pwd=`)
	normalized = re.ReplaceAllString(normalized, " ")

	// 替换掉提取码字样为空格
	re = regexp.MustCompile(`提取码*[：:]`)
	normalized = re.ReplaceAllString(normalized, " ")

	// 替换 http 为 https
	re = regexp.MustCompile(`^.*?(https?://)`)
	normalized = re.ReplaceAllString(normalized, "https://")

	// 替换连续的空格
	re = regexp.MustCompile(`\s+`)
	normalized = re.ReplaceAllString(normalized, " ")

	// 去除首尾空格
	normalized = strings.TrimSpace(normalized)

	return normalized
}

// ParseURLAndCode 从标准化链接中解析 URL 和提取码
func ParseURLAndCode(link string) (string, string) {
	parts := strings.SplitN(link, " ", 2)
	if len(parts) == 0 {
		return "", ""
	}

	urlStr := strings.TrimSpace(parts[0])
	code := ""
	if len(parts) == 2 {
		code = strings.TrimSpace(parts[1])
		// 提取码只取前4位
		if len(code) > 4 {
			code = code[:4]
		}
	}

	return urlStr, code
}

// ExtractSurl 从分享链接中提取 surl
func (b *BaiduUtils) ExtractSurl(shareURL string) string {
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
		return matches[1]
	}

	return ""
}

// VerifyPassCode 验证提取码
func (b *BaiduUtils) VerifyPassCode(linkURL, password string) (string, error) {
	if b.bdstoken == "" {
		if _, err := b.GetBdstoken(); err != nil {
			return "", err
		}
	}

	// 提取 surl
	surl := b.ExtractSurl(linkURL)
	if surl == "" {
		return "", fmt.Errorf("无效的分享链接")
	}

	apiURL := fmt.Sprintf("%s/share/verify", BaiduPanBaseURL)
	params := url.Values{}
	params.Set("surl", surl)
	params.Set("bdstoken", b.bdstoken)
	params.Set("t", fmt.Sprintf("%d", time.Now().UnixMilli()))
	params.Set("channel", "chunlei")
	params.Set("web", "1")
	params.Set("clienttype", "0")

	data := fmt.Sprintf(`pwd=%s&vcode=&vcode_str=`, password)

	req, err := http.NewRequest("POST", apiURL+"?"+params.Encode(), strings.NewReader(data))
	if err != nil {
		return "", err
	}

	b.setHeaders(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

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
		return "", fmt.Errorf("解析 JSON 失败: %w, 响应: %s", err, string(body))
	}

	errno, _ := result["errno"].(float64)
	if errno != 0 {
		errorCode := int(errno)
		errorMsg := ErrorCodeMap[errorCode]
		if errorMsg == "" {
			errorMsg = "未知错误"
		}
		return "", fmt.Errorf("验证提取码失败, 错误码: %d, 错误信息: %s", errorCode, errorMsg)
	}

	randsk, ok := result["randsk"].(string)
	if !ok {
		return "", fmt.Errorf("未获取到 randsk")
	}

	// 更新 Cookie
	b.updateCookie("BDCLND", randsk)

	return randsk, nil
}

// VerifyLink 验证链接有效性（不执行转存）
func (b *BaiduUtils) VerifyLink(linkURL, password string) ([]BaiduShareFile, error) {
	// 如果有密码，先验证
	if password != "" {
		_, err := b.VerifyPassCode(linkURL, password)
		if err != nil {
			return nil, err
		}
	}

	// 获取分享文件列表
	files, err := b.GetSharedPaths(linkURL)
	if err != nil {
		return nil, err
	}

	return files, nil
}

// GetSharedPaths 获取分享文件列表
func (b *BaiduUtils) GetSharedPaths(shareURL string) ([]BaiduShareFile, error) {
	// 提取 surl
	surl := b.ExtractSurl(shareURL)
	if surl == "" {
		return nil, fmt.Errorf("无效的分享链接")
	}

	// 构建请求 URL
	var requestURL string
	if strings.Contains(shareURL, "/share/init?surl=") {
		// 格式: https://pan.baidu.com/share/init?surl={surl}
		requestURL = fmt.Sprintf("%s/share/init?surl=%s", BaiduPanBaseURL, surl)
	} else {
		// 格式: https://pan.baidu.com/s/{surl}
		requestURL = fmt.Sprintf("%s/s/%s", BaiduPanBaseURL, surl)
	}

	req, err := http.NewRequest("GET", requestURL, nil)
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

	responseStr := string(body)

	// 使用正则表达式提取参数
	shareIDRegex := regexp.MustCompile(`"shareid":(\d+),"`)
	userIDRegex := regexp.MustCompile(`"share_uk":"(\d+)","`)
	fsIDRegex := regexp.MustCompile(`"fs_id":(\d+),"`)
	serverFilenameRegex := regexp.MustCompile(`"server_filename":"([^"]+)","`)
	isDirRegex := regexp.MustCompile(`"isdir":(\d+),"`)

	shareIDs := shareIDRegex.FindAllStringSubmatch(responseStr, -1)
	userIDs := userIDRegex.FindAllStringSubmatch(responseStr, -1)
	fsIDs := fsIDRegex.FindAllStringSubmatch(responseStr, -1)
	filenames := serverFilenameRegex.FindAllStringSubmatch(responseStr, -1)
	isDirs := isDirRegex.FindAllStringSubmatch(responseStr, -1)

	if len(shareIDs) == 0 || len(userIDs) == 0 || len(fsIDs) == 0 {
		return nil, fmt.Errorf("解析分享链接响应失败, 可能是提取码错误或链接失效")
	}

	var files []BaiduShareFile
	for i := 0; i < len(fsIDs); i++ {
		file := BaiduShareFile{}
		if i < len(shareIDs) {
			file.ShareID = shareIDs[i][1]
		}
		if i < len(userIDs) {
			file.UserID = userIDs[i][1]
		}
		if i < len(fsIDs) {
			if fsID, err := parseStringToInt64(fsIDs[i][1]); err == nil {
				file.FSID = fsID
			}
		}
		if i < len(filenames) {
			file.ServerFilename = filenames[i][1]
		}
		if i < len(isDirs) {
			if isDir, err := parseStringToInt(isDirs[i][1]); err == nil {
				file.IsDir = isDir
			}
		}
		files = append(files, file)
	}

	return files, nil
}

// parseStringToInt64 解析字符串为 int64
func parseStringToInt64(s string) (int64, error) {
	var result int64
	_, err := fmt.Sscanf(s, "%d", &result)
	return result, err
}

// parseStringToInt 解析字符串为 int
func parseStringToInt(s string) (int, error) {
	var result int
	_, err := fmt.Sscanf(s, "%d", &result)
	return result, err
}