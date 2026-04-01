package cache

import (
	"errors"
	"moss/domain/config"
	"moss/infrastructure/support/cache/core"
	"moss/infrastructure/support/log"
	"strings"
	"time"

	"go.uber.org/zap"
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
		log.Debug("cache get failed: cache disabled or driver error", zap.String("bucket", bucket), zap.String("key", key), zap.Error(err))
		return []byte{}, err
	}
	val, err := d.Get(bucket, key)
	if err != nil {
		log.Debug("cache miss", zap.String("bucket", bucket), zap.String("key", key), zap.Error(err))
		return []byte{}, err
	}
	log.Debug("cache hit", zap.String("bucket", bucket), zap.String("key", key), zap.Int("size", len(val)))
	return val, nil
}

func Set(bucket, key string, val []byte, ttl time.Duration) error {
	d, err := ActiveDriver()
	if err != nil {
		log.Debug("cache set skipped: cache disabled or driver error", zap.String("bucket", bucket), zap.String("key", key), zap.Error(err))
		return err
	}
	err = d.Set(bucket, key, val, ttl)
	if err != nil {
		log.Warn("cache set failed", zap.String("bucket", bucket), zap.String("key", key), zap.Error(err))
		return err
	}
	log.Debug("cache set success", zap.String("bucket", bucket), zap.String("key", key), zap.Duration("ttl", ttl), zap.Int("size", len(val)))
	return nil
}

func Delete(bucket, key string) error {
	d, err := ActiveDriver()
	if err != nil {
		log.Debug("cache delete skipped: cache disabled or driver error", zap.String("bucket", bucket), zap.String("key", key), zap.Error(err))
		return err
	}
	err = d.Delete(bucket, key)
	if err != nil {
		log.Warn("cache delete failed", zap.String("bucket", bucket), zap.String("key", key), zap.Error(err))
		return err
	}
	log.Info("cache delete success", zap.String("bucket", bucket), zap.String("key", key))
	return nil
}

func DeleteByPrefix(bucket, prefix string) error {
	d, err := ActiveDriver()
	if err != nil {
		return err
	}
	return d.DeleteByPrefix(bucket, prefix)
}

// DeleteWithVerify deletes a cache entry and verifies the deletion was successful.
// If the key still exists after deletion, it retries once before returning an error.
func DeleteWithVerify(bucket, key string) error {
	// First delete attempt
	if err := Delete(bucket, key); err != nil {
		return err
	}

	// Verify deletion succeeded
	if _, err := Get(bucket, key); err == nil {
		log.Warn("cache key still exists after delete, retrying", zap.String("bucket", bucket), zap.String("key", key))
		// Retry once
		if err := Delete(bucket, key); err != nil {
			return err
		}
		// Verify again
		if _, err := Get(bucket, key); err == nil {
			log.Warn("cache key still exists after retry", zap.String("bucket", bucket), zap.String("key", key))
			return errors.New("cache key still exists after deletion")
		}
	}

	log.Info("cache delete verified successfully", zap.String("bucket", bucket), zap.String("key", key))
	return nil
}

func ClearBucket(bucket string) error {
	d, err := ActiveDriver()
	if err != nil {
		log.Debug("cache clear bucket skipped: cache disabled or driver error", zap.String("bucket", bucket), zap.Error(err))
		return err
	}
	err = d.ClearBucket(bucket)
	if err != nil {
		log.Warn("cache clear bucket failed", zap.String("bucket", bucket), zap.Error(err))
		return err
	}
	log.Info("cache clear bucket success", zap.String("bucket", bucket))
	return nil
}

// NormalizeCacheKey normalizes a cache key to ensure consistency.
// It ensures the key starts with "/" and has no trailing slashes.
// Empty or "/" keys are converted to "default".
func NormalizeCacheKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" || key == "/" {
		return "default"
	}
	// Ensure starts with "/"
	if !strings.HasPrefix(key, "/") {
		key = "/" + key
	}
	// Remove all trailing slashes
	key = strings.TrimRight(key, "/")
	return key
}

// InvalidateArticleCache invalidates the cache for a specific article by its URL.
// This should be called when article content or thumbnail is updated.
func InvalidateArticleCache(articleURL string) error {
	normalizedKey := NormalizeCacheKey(articleURL)
	log.Info("invalidating article cache", zap.String("original_key", articleURL), zap.String("normalized_key", normalizedKey))
	return Delete("article", normalizedKey)
}

// InvalidateArticleCacheWithVerify invalidates the cache and verifies the deletion was successful.
// Returns error if deletion fails or if the key still exists after deletion.
func InvalidateArticleCacheWithVerify(articleURL string) error {
	normalizedKey := NormalizeCacheKey(articleURL)
	log.Info("invalidating article cache with verify", zap.String("original_key", articleURL), zap.String("normalized_key", normalizedKey))

	// Delete the cache entry
	if err := Delete("article", normalizedKey); err != nil {
		log.Warn("cache delete failed", zap.String("key", normalizedKey), zap.Error(err))
		return err
	}

	// Verify deletion succeeded
	if _, err := Get("article", normalizedKey); err == nil {
		log.Warn("cache still exists after delete, retrying", zap.String("key", normalizedKey))
		// Retry once
		if err := Delete("article", normalizedKey); err != nil {
			return err
		}
		// Verify again
		if _, err := Get("article", normalizedKey); err == nil {
			log.Warn("cache still exists after retry", zap.String("key", normalizedKey))
			return errors.New("cache key still exists after deletion")
		}
	}

	log.Info("article cache invalidated successfully", zap.String("key", normalizedKey))
	return nil
}

// InvalidateHomePageCache invalidates the home page cache.
// This should be called when new articles are published or existing articles are updated.
func InvalidateHomePageCache() error {
	return ClearBucket("home")
}
