package plugins

import (
	"encoding/json"
	"moss/domain/config"
	pluginEntity "moss/domain/support/entity"
	"moss/infrastructure/support/cache"
	"strings"
	"time"

	"go.uber.org/zap"
)

// DownloadLimit 下载频率限制插件
type DownloadLimit struct {
	Enable           bool   `json:"enable"`             // 是否启用
	MaxPerDay        int    `json:"max_per_day"`        // 每日最大下载次数
	EnableCloudflare bool   `json:"enable_cloudflare"`  // 启用 Cloudflare 支持
	WhiteList        string `json:"white_list"`         // IP 白名单（逗号分隔）

	ctx *pluginEntity.Plugin
}

// NewDownloadLimit 创建下载限制插件
func NewDownloadLimit() *DownloadLimit {
	return &DownloadLimit{
		Enable:           false,
		MaxPerDay:        10,
		EnableCloudflare: true,
		WhiteList:        "",
	}
}

// Info 返回插件信息
func (d *DownloadLimit) Info() *pluginEntity.PluginInfo {
	return &pluginEntity.PluginInfo{
		ID:         "download_limit",
		About:      "资源下载次数限制",
		RunEnable:  false,
		CronEnable: false,
		NoOptions:  false,
		HideLogs:   false,
	}
}

// Load 加载插件
func (d *DownloadLimit) Load(ctx *pluginEntity.Plugin) error {
	d.ctx = ctx
	d.ctx.Log.Info("download limit plugin loaded",
		zap.Bool("enable", d.Enable),
		zap.Int("max_per_day", d.MaxPerDay),
		zap.Bool("enable_cloudflare", d.EnableCloudflare))

	// 检查缓存是否启用
	if !config.Config.Cache.Enable {
		d.ctx.Log.Warn("download limit plugin: cache is disabled, please enable cache for the plugin to work properly",
			zap.String("config", "Cache.Enable=false"))
	}

	return nil
}

// Run 定时任务（预留接口）
func (d *DownloadLimit) Run(ctx *pluginEntity.Plugin) error {
	return nil
}

// CheckLimit 检查 IP 是否超过下载限制
// 返回：是否允许下载、剩余次数
func (d *DownloadLimit) CheckLimit(ip string) (bool, int) {
	if !d.Enable {
		return true, d.MaxPerDay
	}

	// 检查白名单
	if d.isInWhiteList(ip) {
		d.ctx.Log.Debug("IP in whitelist, bypass limit", zap.String("ip", ip))
		return true, d.MaxPerDay
	}

	// 获取当前下载次数
	today := time.Now().Format("20060102")
	cacheKey := ip + "_" + today

	var downloadCount int
	if data, err := cache.Get("download_limit", cacheKey); err == nil && len(data) > 0 {
		_ = json.Unmarshal(data, &downloadCount)
	}

	remaining := d.MaxPerDay - downloadCount
	if remaining < 0 {
		remaining = 0
	}

	allowed := downloadCount < d.MaxPerDay

	d.ctx.Log.Debug("download limit check",
		zap.String("ip", ip),
		zap.Int("count", downloadCount),
		zap.Int("max", d.MaxPerDay),
		zap.Int("remaining", remaining),
		zap.Bool("allowed", allowed))

	return allowed, remaining
}

// Increment 增加下载计数
func (d *DownloadLimit) Increment(ip string) error {
	if !d.Enable {
		return nil
	}

	// 检查白名单
	if d.isInWhiteList(ip) {
		return nil
	}

	today := time.Now().Format("20060102")
	cacheKey := ip + "_" + today

	// 读取当前下载次数
	var downloadCount int
	if data, err := cache.Get("download_limit", cacheKey); err == nil && len(data) > 0 {
		_ = json.Unmarshal(data, &downloadCount)
	}

	// 增加计数
	downloadCount++
	if data, err := json.Marshal(downloadCount); err == nil {
		ttl := 24 * time.Hour
		if err := cache.Set("download_limit", cacheKey, data, ttl); err != nil {
			d.ctx.Log.Warn("update download count failed",
				zap.String("ip", ip),
				zap.Error(err))
			return err
		}
		d.ctx.Log.Info("download count incremented",
			zap.String("ip", ip),
			zap.Int("count", downloadCount))
	}

	return nil
}

// GetRemaining 获取剩余下载次数
func (d *DownloadLimit) GetRemaining(ip string) int {
	if !d.Enable {
		return d.MaxPerDay
	}

	today := time.Now().Format("20060102")
	cacheKey := ip + "_" + today

	var downloadCount int
	if data, err := cache.Get("download_limit", cacheKey); err == nil && len(data) > 0 {
		_ = json.Unmarshal(data, &downloadCount)
	}

	remaining := d.MaxPerDay - downloadCount
	if remaining < 0 {
		remaining = 0
	}

	return remaining
}

// isInWhiteList 检查 IP 是否在白名单中
func (d *DownloadLimit) isInWhiteList(ip string) bool {
	if d.WhiteList == "" {
		return false
	}

	whiteList := strings.Split(d.WhiteList, ",")
	for _, whiteIP := range whiteList {
		whiteIP = strings.TrimSpace(whiteIP)
		if whiteIP == "" {
			continue
		}
		if ip == whiteIP {
			return true
		}
	}

	return false
}

// GetClientIP 获取客户端 IP 地址
// 支持代理头和 Cloudflare
func (d *DownloadLimit) GetClientIP(ctx interface{}) string {
	// 尝试从 Fiber 上下文获取 IP
	// 这里简化处理，实际使用时需要传入正确的上下文类型
	// 在控制器中会调用此方法

	if d.EnableCloudflare {
		// Cloudflare 专用头
		if cfIP := getHeaderValue(ctx, "CF-Connecting-IP"); cfIP != "" {
			return cfIP
		}
	}

	// 通用代理头
	for _, header := range config.Config.Router.ProxyHeader {
		if ip := getHeaderValue(ctx, header); ip != "" {
			arr := strings.Split(ip, ",")
			if len(arr) > 0 && arr[0] != "" {
				return arr[0]
			}
		}
	}

	// 直接连接的 IP
	if ip := getHeaderValue(ctx, "RemoteAddr"); ip != "" {
		return ip
	}

	return ""
}

// getHeaderValue 从上下文中获取 HTTP 头（辅助函数）
func getHeaderValue(ctx interface{}, header string) string {
	// 这里需要根据实际传入的上下文类型实现
	// 在实际使用时，会在控制器中直接调用 Fiber 的方法
	return ""
}