package baidu_utils

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// 常量定义
const (
	BaiduPanBaseURL = "https://pan.baidu.com"
)

// 错误码映射
var ErrorCodeMap = map[int]string{
	-1:   "链接错误，链接失效或缺少提取码",
	-4:   "转存失败，无效登录。请退出账号在其他地方的登录",
	-6:   "转存失败，请用浏览器无痕模式获取 Cookie 后再试",
	-7:   "转存失败，转存文件夹名有非法字符，不能包含 < > | * ? \\ :，请改正目录名后重试",
	-8:   "转存失败，目录中已有同名文件或文件夹存在",
	-9:   "链接错误，提取码错误",
	-10:  "转存失败，容量不足",
	-12:  "链接错误，提取码错误",
	-62:  "转存失败，链接访问次数过多，请手动转存或稍后再试",
	0:    "转存成功",
	2:    "转存失败，目标目录不存在",
	4:    "转存失败，目录中存在同名文件",
	12:   "转存失败，转存文件数超过限制",
	20:   "转存失败，容量不足",
	105:  "链接错误，所访问的页面不存在",
	115:  "分享链接已失效（文件禁止分享）",
	145:  "分享链接已失效",
	-65:  "触发频率限制",
	200025: "提取码输入错误，请检查提取码",
}

// 不需要重试的错误码
var NoRetryErrors = []int{-6, 115, 145, 200025, -9}

// BaiduUtils 百度网盘工具类
type BaiduUtils struct {
	// HTTP 客户端
	HttpClient *http.Client

	// 配置
	Cookie string

	// 解析后的 Cookie
	parsedBDUSS string
	parsedSTOKEN string

	// bdstoken
	bdstoken string

	// 日志
	Logger *zap.Logger

	// 互斥锁
	mu sync.Mutex

	// 上次请求时间
	lastRequest time.Time
}

// NewBaiduUtils 创建百度网盘工具实例
func NewBaiduUtils(cookie string, logger *zap.Logger) *BaiduUtils {
	b := &BaiduUtils{
		Cookie: cookie,
		Logger: logger,
		HttpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				IdleConnTimeout:     30 * time.Second,
				DisableCompression:  true, // 禁用自动压缩处理，手动处理所有 gzip 解压缩
			},
		},
	}

	// 清理 Cookie：移除换行符、回车符、制表符和多余空格
	b.Cookie = strings.ReplaceAll(b.Cookie, "\n", "")
	b.Cookie = strings.ReplaceAll(b.Cookie, "\r", "")
	b.Cookie = strings.ReplaceAll(b.Cookie, "\t", "")
	b.Cookie = strings.TrimSpace(b.Cookie)

	// 解析 Cookie
	if err := b.parseCookie(); err != nil {
		if logger != nil {
			logger.Warn("解析 Cookie 失败", zap.Error(err))
		}
	}

	return b
}

// parseCookie 解析 Cookie 字符串
func (b *BaiduUtils) parseCookie() error {
	// 解析 Cookie 字符串
	cookies := strings.Split(b.Cookie, ";")
	for _, cookie := range cookies {
		cookie = strings.TrimSpace(cookie)
		parts := strings.SplitN(cookie, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])

			if key == "BDUSS" {
				b.parsedBDUSS = value
			} else if key == "STOKEN" {
				b.parsedSTOKEN = value
			}
		}
	}
	return nil
}

// SetCookie 设置 Cookie
func (b *BaiduUtils) SetCookie(cookie string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.Cookie = cookie
	b.Cookie = strings.ReplaceAll(b.Cookie, "\n", "")
	b.Cookie = strings.ReplaceAll(b.Cookie, "\r", "")
	b.Cookie = strings.ReplaceAll(b.Cookie, "\t", "")
	b.Cookie = strings.TrimSpace(b.Cookie)

	// 重新解析 Cookie
	if err := b.parseCookie(); err != nil && b.Logger != nil {
		b.Logger.Warn("解析 Cookie 失败", zap.Error(err))
	}
}

// updateCookie 更新 Cookie 中的某个参数
func (b *BaiduUtils) updateCookie(key, value string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// 解析现有 Cookie
	cookiesMap := make(map[string]string)
	cookies := strings.Split(b.Cookie, ";")
	for _, cookie := range cookies {
		cookie = strings.TrimSpace(cookie)
		if idx := strings.Index(cookie, "="); idx > 0 {
			cookieKey := strings.TrimSpace(cookie[:idx])
			cookieValue := strings.TrimSpace(cookie[idx+1:])
			cookiesMap[cookieKey] = cookieValue
		}
	}

	// 更新指定参数
	cookiesMap[key] = value

	// 重新构建 Cookie
	var newCookies []string
	for k, v := range cookiesMap {
		newCookies = append(newCookies, k+"="+v)
	}
	b.Cookie = strings.Join(newCookies, ";")

	// 更新解析后的 Cookie
	if key == "BDUSS" {
		b.parsedBDUSS = value
	} else if key == "STOKEN" {
		b.parsedSTOKEN = value
	}
}

// applyRateLimit 应用速率限制
func (b *BaiduUtils) applyRateLimit(rateLimit int) {
	if rateLimit <= 0 {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	interval := time.Minute / time.Duration(rateLimit)
	if !b.lastRequest.IsZero() {
		wait := time.Until(b.lastRequest.Add(interval))
		if wait > 0 {
			if b.Logger != nil {
				b.Logger.Debug("速率限制等待", zap.Duration("wait", wait))
			}
			time.Sleep(wait)
		}
	}
	b.lastRequest = time.Now()
}