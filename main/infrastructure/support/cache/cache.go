package cache

import (
	"errors"
	"moss/domain/config"
	"moss/infrastructure/support/cache/core"
	"moss/infrastructure/support/log"
	"time"
)

func init() {
	if err := Init(); err != nil {
		log.Error("init cache error", log.Err(err))
	}
}

func Init() error {
	// close all
	for _, item := range config.Config.Cache.Driver.Items() {
		_ = item.Close()
	}
	if !config.Config.Cache.Enable {
		return nil
	}
	d, err := ActiveDriver()
	if err != nil {
		return err
	}
	return d.Init()
}

func ActiveDriver() (res core.Cache, err error) {
	if !config.Config.Cache.Enable {
		return nil, errors.New("cache is disabled")
	}
	res, err = config.Config.Cache.Driver.Get(config.Config.Cache.ActiveDriver)
	if res == nil {
		return nil, errors.New("active driver is nil")
	}
	return
}

func Get(bucket, key string) ([]byte, error) {
	d, err := ActiveDriver()
	if err != nil {
		return []byte{}, err
	}
	return d.Get(bucket, key)
}

func Set(bucket, key string, val []byte, ttl time.Duration) error {
	d, err := ActiveDriver()
	if err != nil {
		return err
	}
	return d.Set(bucket, key, val, ttl)
}

func Delete(bucket, key string) error {
	d, err := ActiveDriver()
	if err != nil {
		return err
	}
	return d.Delete(bucket, key)
}

func DeleteByPrefix(bucket, prefix string) error {
	d, err := ActiveDriver()
	if err != nil {
		return err
	}
	return d.DeleteByPrefix(bucket, prefix)
}

func ClearBucket(bucket string) error {
	d, err := ActiveDriver()
	if err != nil {
		return err
	}
	return d.ClearBucket(bucket)
}

// InvalidateArticleCache invalidates the cache for a specific article by its URL.
// This should be called when article content or thumbnail is updated.
func InvalidateArticleCache(articleURL string) error {
	return Delete("article", articleURL)
}

// InvalidateHomePageCache invalidates the home page cache.
// This should be called when new articles are published or existing articles are updated.
func InvalidateHomePageCache() error {
	return ClearBucket("home")
}
