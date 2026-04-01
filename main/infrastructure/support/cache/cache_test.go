package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"moss/domain/config"
	"moss/domain/config/aggregate"
	"moss/domain/config/entity"
	"moss/infrastructure/support/cache/core"
	"moss/infrastructure/utils/timex"
)

// setupTestCache initializes a test cache with Badger driver
func setupTestCache(t *testing.T) func() {
	// Create temp directory for cache
	tmpDir, err := os.MkdirTemp("", "moss-cache-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Initialize config
	config.Config = &aggregate.Config{
		Site:   &entity.Site{URL: "https://example.com"},
		Router: &entity.Router{ArticleRule: "/article/:slug"},
		Cache: &entity.Cache{
			Enable:       true,
			ActiveDriver: core.BadgerDriverName,
			Driver:       core.NewDriver(),
			Options: &entity.CacheOptions{
				Home:     &entity.CacheOptionItem{Enable: true, TTL: timex.Duration{Number: 30, Unit: timex.DurationMinute}},
				Article:  &entity.CacheOptionItem{Enable: true, TTL: timex.Duration{Number: 1, Unit: timex.DurationDay}},
				Category: &entity.CacheOptionItem{Enable: true, TTL: timex.Duration{Number: 8, Unit: timex.DurationHour}},
				Tag:      &entity.CacheOptionItem{Enable: true, TTL: timex.Duration{Number: 16, Unit: timex.DurationHour}},
			},
		},
	}

	// Set Badger path to temp directory
	config.Config.Cache.Driver.Badger.Path = filepath.Join(tmpDir, "badger")

	// Initialize cache
	if err := Init(); err != nil {
		t.Fatalf("Failed to initialize cache: %v", err)
	}

	// Return cleanup function
	return func() {
		// Close all drivers
		for _, item := range config.Config.Cache.Driver.Items() {
			_ = item.Close()
		}
		// Remove temp directory
		_ = os.RemoveAll(tmpDir)
	}
}

func TestCacheBasicOperations(t *testing.T) {
	cleanup := setupTestCache(t)
	defer cleanup()

	t.Run("SetAndGet", func(t *testing.T) {
		key := "test-key"
		value := []byte("test-value")

		// Set value
		err := Set("article", key, value, time.Hour)
		if err != nil {
			t.Fatalf("Failed to set cache: %v", err)
		}

		// Get value
		got, err := Get("article", key)
		if err != nil {
			t.Fatalf("Failed to get cache: %v", err)
		}

		if string(got) != string(value) {
			t.Errorf("Expected %s, got %s", value, got)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		key := "test-delete-key"
		value := []byte("test-delete-value")

		// Set value
		err := Set("article", key, value, time.Hour)
		if err != nil {
			t.Fatalf("Failed to set cache: %v", err)
		}

		// Delete value
		err = Delete("article", key)
		if err != nil {
			t.Fatalf("Failed to delete cache: %v", err)
		}

		// Get should fail
		_, err = Get("article", key)
		if err == nil {
			t.Error("Expected error when getting deleted key")
		}
	})
}

func TestDeleteByPrefix(t *testing.T) {
	cleanup := setupTestCache(t)
	defer cleanup()

	// Set multiple keys with common prefix
	keys := []string{
		"article-1",
		"article-2",
		"article-3",
	}
	prefix := "article-"

	for _, key := range keys {
		err := Set("article", key, []byte("value-"+key), time.Hour)
		if err != nil {
			t.Fatalf("Failed to set cache for key %s: %v", key, err)
		}
	}

	// Also set a key without the prefix
	err := Set("article", "other-key", []byte("other-value"), time.Hour)
	if err != nil {
		t.Fatalf("Failed to set cache for other-key: %v", err)
	}

	// Delete by prefix
	err = DeleteByPrefix("article", prefix)
	if err != nil {
		t.Fatalf("Failed to delete by prefix: %v", err)
	}

	// Verify prefixed keys are deleted
	for _, key := range keys {
		_, err := Get("article", key)
		if err == nil {
			t.Errorf("Expected key %s to be deleted", key)
		}
	}

	// Verify other key still exists
	got, err := Get("article", "other-key")
	if err != nil {
		t.Fatalf("Expected other-key to still exist: %v", err)
	}
	if string(got) != "other-value" {
		t.Errorf("Expected other-value, got %s", got)
	}
}

func TestInvalidateArticleCache(t *testing.T) {
	cleanup := setupTestCache(t)
	defer cleanup()

	articleURL := "/article/test-article"
	content := []byte("<html>test content</html>")

	// Set article cache
	err := Set("article", articleURL, content, time.Hour)
	if err != nil {
		t.Fatalf("Failed to set article cache: %v", err)
	}

	// Verify cache exists
	got, err := Get("article", articleURL)
	if err != nil {
		t.Fatalf("Failed to get article cache: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("Expected %s, got %s", content, got)
	}

	// Invalidate article cache
	err = InvalidateArticleCache(articleURL)
	if err != nil {
		t.Fatalf("Failed to invalidate article cache: %v", err)
	}

	// Verify cache is deleted
	_, err = Get("article", articleURL)
	if err == nil {
		t.Error("Expected error when getting invalidated cache")
	}
}

func TestInvalidateHomePageCache(t *testing.T) {
	cleanup := setupTestCache(t)
	defer cleanup()

	// Set home page cache entries
	homeKeys := []string{"default", "/", "/page/1", "/page/2"}
	for _, key := range homeKeys {
		err := Set("home", key, []byte("home-content-"+key), time.Hour)
		if err != nil {
			t.Fatalf("Failed to set home cache for key %s: %v", key, err)
		}
	}

	// Invalidate home page cache
	err := InvalidateHomePageCache()
	if err != nil {
		t.Fatalf("Failed to invalidate home page cache: %v", err)
	}

	// Verify all home cache entries are deleted
	for _, key := range homeKeys {
		_, err := Get("home", key)
		if err == nil {
			t.Errorf("Expected key %s to be deleted", key)
		}
	}
}

func TestCacheDisabled(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "moss-cache-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize config with cache disabled
	config.Config = &aggregate.Config{
		Cache: &entity.Cache{
			Enable:       false,
			ActiveDriver: core.BadgerDriverName,
			Driver:       core.NewDriver(),
		},
	}

	// InvalidateArticleCache should return error when cache is disabled
	err = InvalidateArticleCache("/test")
	if err == nil {
		t.Error("Expected error when cache is disabled")
	}

	// InvalidateHomePageCache should return error when cache is disabled
	err = InvalidateHomePageCache()
	if err == nil {
		t.Error("Expected error when cache is disabled")
	}
}

func TestClearBucket(t *testing.T) {
	cleanup := setupTestCache(t)
	defer cleanup()

	// Set multiple keys in a bucket
	for i := 0; i < 5; i++ {
		key := string(rune('a' + i))
		err := Set("article", key, []byte("value-"+key), time.Hour)
		if err != nil {
			t.Fatalf("Failed to set cache: %v", err)
		}
	}

	// Clear bucket
	err := ClearBucket("article")
	if err != nil {
		t.Fatalf("Failed to clear bucket: %v", err)
	}

	// Verify all keys are deleted
	for i := 0; i < 5; i++ {
		key := string(rune('a' + i))
		_, err := Get("article", key)
		if err == nil {
			t.Errorf("Expected key %s to be deleted", key)
		}
	}
}

// Benchmark cache operations
func BenchmarkSet(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "moss-cache-bench-*")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	config.Config = &aggregate.Config{
		Cache: &entity.Cache{
			Enable:       true,
			ActiveDriver: core.BadgerDriverName,
			Driver:       core.NewDriver(),
		},
	}
	config.Config.Cache.Driver.Badger.Path = filepath.Join(tmpDir, "badger")

	if err := Init(); err != nil {
		b.Fatalf("Failed to initialize cache: %v", err)
	}
	defer func() {
		for _, item := range config.Config.Cache.Driver.Items() {
			_ = item.Close()
		}
	}()

	value := []byte("test-value-content")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Set("article", string(rune(i)), value, time.Hour)
	}
}

func BenchmarkGet(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "moss-cache-bench-*")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	config.Config = &aggregate.Config{
		Cache: &entity.Cache{
			Enable:       true,
			ActiveDriver: core.BadgerDriverName,
			Driver:       core.NewDriver(),
		},
	}
	config.Config.Cache.Driver.Badger.Path = filepath.Join(tmpDir, "badger")

	if err := Init(); err != nil {
		b.Fatalf("Failed to initialize cache: %v", err)
	}
	defer func() {
		for _, item := range config.Config.Cache.Driver.Items() {
			_ = item.Close()
		}
	}()

	// Pre-populate cache
	value := []byte("test-value-content")
	for i := 0; i < 1000; i++ {
		_ = Set("article", string(rune(i)), value, time.Hour)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Get("article", string(rune(i%1000)))
	}
}

func BenchmarkDelete(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "moss-cache-bench-*")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	config.Config = &aggregate.Config{
		Cache: &entity.Cache{
			Enable:       true,
			ActiveDriver: core.BadgerDriverName,
			Driver:       core.NewDriver(),
		},
	}
	config.Config.Cache.Driver.Badger.Path = filepath.Join(tmpDir, "badger")

	if err := Init(); err != nil {
		b.Fatalf("Failed to initialize cache: %v", err)
	}
	defer func() {
		for _, item := range config.Config.Cache.Driver.Items() {
			_ = item.Close()
		}
	}()

	value := []byte("test-value-content")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := string(rune(i))
		_ = Set("article", key, value, time.Hour)
		_ = Delete("article", key)
	}
}

func BenchmarkInvalidateArticleCache(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "moss-cache-bench-*")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	config.Config = &aggregate.Config{
		Cache: &entity.Cache{
			Enable:       true,
			ActiveDriver: core.BadgerDriverName,
			Driver:       core.NewDriver(),
		},
	}
	config.Config.Cache.Driver.Badger.Path = filepath.Join(tmpDir, "badger")

	if err := Init(); err != nil {
		b.Fatalf("Failed to initialize cache: %v", err)
	}
	defer func() {
		for _, item := range config.Config.Cache.Driver.Items() {
			_ = item.Close()
		}
	}()

	value := []byte("test-value-content")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		url := string(rune(i))
		_ = Set("article", url, value, time.Hour)
		_ = InvalidateArticleCache(url)
	}
}

// TestNormalizeCacheKey tests the cache key normalization function
func TestNormalizeCacheKey(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "default",
		},
		{
			name:     "single slash",
			input:    "/",
			expected: "default",
		},
		{
			name:     "path without leading slash",
			input:    "article/test",
			expected: "/article/test",
		},
		{
			name:     "path with trailing slash",
			input:    "/article/test/",
			expected: "/article/test",
		},
		{
			name:     "path with leading and trailing slash",
			input:    "/article/test/",
			expected: "/article/test",
		},
		{
			name:     "normal path",
			input:    "/article/test-article",
			expected: "/article/test-article",
		},
		{
			name:     "path with spaces",
			input:    "  /article/test  ",
			expected: "/article/test",
		},
		{
			name:     "multiple trailing slashes",
			input:    "/article/test//",
			expected: "/article/test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeCacheKey(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeCacheKey(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestInvalidateArticleCacheWithVerify tests the cache invalidation with verification
func TestInvalidateArticleCacheWithVerify(t *testing.T) {
	cleanup := setupTestCache(t)
	defer cleanup()

	t.Run("invalidate existing cache", func(t *testing.T) {
		articleURL := "/article/test-verify"
		content := []byte("<html>test content</html>")

		// Set article cache
		err := Set("article", NormalizeCacheKey(articleURL), content, time.Hour)
		if err != nil {
			t.Fatalf("Failed to set article cache: %v", err)
		}

		// Invalidate with verify
		err = InvalidateArticleCacheWithVerify(articleURL)
		if err != nil {
			t.Fatalf("Failed to invalidate article cache with verify: %v", err)
		}

		// Verify cache is deleted
		_, err = Get("article", NormalizeCacheKey(articleURL))
		if err == nil {
			t.Error("Expected error when getting invalidated cache")
		}
	})

	t.Run("invalidate non-existing cache", func(t *testing.T) {
		articleURL := "/article/non-existing"

		// Invalidate non-existing cache should not error
		err := InvalidateArticleCacheWithVerify(articleURL)
		if err != nil {
			t.Fatalf("Expected no error for non-existing cache, got: %v", err)
		}
	})

	t.Run("invalidate with normalized key", func(t *testing.T) {
		// Set cache with normalized key
		articleURL := "/article/normalized-test/"
		normalizedKey := NormalizeCacheKey(articleURL)
		content := []byte("<html>normalized test</html>")

		err := Set("article", normalizedKey, content, time.Hour)
		if err != nil {
			t.Fatalf("Failed to set article cache: %v", err)
		}

		// Invalidate with original URL (should normalize internally)
		err = InvalidateArticleCacheWithVerify(articleURL)
		if err != nil {
			t.Fatalf("Failed to invalidate article cache: %v", err)
		}

		// Verify cache is deleted
		_, err = Get("article", normalizedKey)
		if err == nil {
			t.Error("Expected error when getting invalidated cache")
		}
	})
}

// TestDeleteWithVerify tests the DeleteWithVerify function
func TestDeleteWithVerify(t *testing.T) {
	cleanup := setupTestCache(t)
	defer cleanup()

	t.Run("delete existing key", func(t *testing.T) {
		key := "test-delete-verify"
		value := []byte("test-value")

		// Set value
		err := Set("article", key, value, time.Hour)
		if err != nil {
			t.Fatalf("Failed to set cache: %v", err)
		}

		// Delete with verify
		err = DeleteWithVerify("article", key)
		if err != nil {
			t.Fatalf("Failed to delete with verify: %v", err)
		}

		// Verify deleted
		_, err = Get("article", key)
		if err == nil {
			t.Error("Expected error when getting deleted key")
		}
	})

	t.Run("delete non-existing key", func(t *testing.T) {
		// Delete non-existing key should not error
		err := DeleteWithVerify("article", "non-existing-key")
		if err != nil {
			t.Fatalf("Expected no error for non-existing key, got: %v", err)
		}
	})
}

// TestCacheKeyConsistency tests that cache keys are consistent between middleware and invalidation
func TestCacheKeyConsistency(t *testing.T) {
	cleanup := setupTestCache(t)
	defer cleanup()

	// Simulate article URL from entity.URL()
	articleSlug := "test-article-slug"
	articleURL := "/article/" + articleSlug

	// Simulate middleware storing cache (uses normalized key)
	normalizedKey := NormalizeCacheKey(articleURL)
	content := []byte("<html>article content</html>")

	err := Set("article", normalizedKey, content, time.Hour)
	if err != nil {
		t.Fatalf("Failed to set article cache: %v", err)
	}

	// Verify cache can be retrieved with same normalized key
	got, err := Get("article", normalizedKey)
	if err != nil {
		t.Fatalf("Failed to get article cache: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("Expected %s, got %s", content, got)
	}

	// Invalidate using InvalidateArticleCacheWithVerify (should use same normalization)
	err = InvalidateArticleCacheWithVerify(articleURL)
	if err != nil {
		t.Fatalf("Failed to invalidate article cache: %v", err)
	}

	// Verify cache is deleted
	_, err = Get("article", normalizedKey)
	if err == nil {
		t.Error("Expected error when getting invalidated cache")
	}
}
