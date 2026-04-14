package middleware

import (
	"github.com/gofiber/fiber/v2"
	"moss/domain/config"
	"moss/infrastructure/support/log"
	"strings"
)

// isStaticFile 检查是否为静态资源文件（图片、样式、脚本、字体）
func isStaticFile(path string) bool {
	return strings.HasSuffix(path, ".js") ||
		strings.HasSuffix(path, ".css") ||
		strings.HasSuffix(path, ".png") ||
		strings.HasSuffix(path, ".jpg") ||
		strings.HasSuffix(path, ".jpeg") ||
		strings.HasSuffix(path, ".gif") ||
		strings.HasSuffix(path, ".svg") ||
		strings.HasSuffix(path, ".ico") ||
		strings.HasSuffix(path, ".woff") ||
		strings.HasSuffix(path, ".woff2") ||
		strings.HasSuffix(path, ".ttf") ||
		strings.HasSuffix(path, ".eot") ||
		strings.HasSuffix(path, ".webp")
}

func HttpLog(ctx *fiber.Ctx) error {
	next := ctx.Next()

	// 过滤静态资源，不记录日志
	path := ctx.Path()
	if isStaticFile(path) {
		return next
	}

	if log.Visitor.IsClosed() && log.Spider.IsClosed() {
		return next
	}
	log.Client.InvokePoolHTTP(log.HttpData{
		RequestTime: ctx.Context().Time(),
		Status:      ctx.Context().Response.StatusCode(),
		Depth:       ctx.Context().ConnRequestNum(),
		IP:          getRequestIP(ctx),
		Method:      ctx.Method(),
		URL:         string(ctx.Context().URI().FullURI()),
		Referer:     string(ctx.Context().Referer()),
		UserAgent:   string(ctx.Context().UserAgent()),
		Headers:     string(ctx.Request().Header.RawHeaders()),
		Path:        path,
	})
	return next
}

func getRequestIP(ctx *fiber.Ctx) (ip string) {
	for _, v := range config.Config.Router.ProxyHeader {
		if ip = ctx.Get(v); ip != "" {
			arr := strings.Split(ip, ",")
			if len(arr) == 0 {
				continue
			}
			if arr[0] != "" {
				return arr[0]
			}
		}
	}
	return ctx.IP()
}
