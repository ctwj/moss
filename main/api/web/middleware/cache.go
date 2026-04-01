package middleware

import (
	"moss/domain/config"
	"moss/infrastructure/support/cache"
	"moss/infrastructure/support/log"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

func Cache(ctx *fiber.Ctx) error {

	if !config.Config.Cache.Enable || ctx.Method() != "GET" {
		return ctx.Next()
	}

	name := ctx.Route().Name
	rawKey := ctx.Path()
	option := config.Config.Cache.GetOption(name)

	if option == nil || !option.Enable {
		return ctx.Next()
	}

	// Normalize the cache key to ensure consistency
	key := cache.NormalizeCacheKey(rawKey)

	// 默认不打印错误，否则找不到文件错误会爆满
	if val, err := cache.Get(name, key); err == nil {
		log.Debug("cache middleware hit", zap.String("route", name), zap.String("key", key))
		return ctx.Type("html").Send(val)
	}

	log.Debug("cache middleware miss", zap.String("route", name), zap.String("key", key))

	next := ctx.Next()

	if ctx.Response().StatusCode() == 200 {
		if err := cache.Set(name, key, ctx.Response().Body(), option.TTL.Duration()); err != nil {
			log.Warn("set cache error", log.Err(err))
		} else {
			log.Debug("cache middleware stored", zap.String("route", name), zap.String("key", key))
		}
	}

	return next
}
