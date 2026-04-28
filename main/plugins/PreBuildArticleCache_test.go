package plugins

import (
	"os"
	"path/filepath"
	"testing"

	"moss/domain/config"
	"moss/domain/config/aggregate"
	"moss/domain/config/entity"
	articleEntity "moss/domain/core/entity"
	pluginEntity "moss/domain/support/entity"
	"moss/infrastructure/support/cache"
	"moss/infrastructure/support/cache/core"
	"moss/infrastructure/utils/timex"

	"go.uber.org/zap"
)

// setupPreBuildTestCache initializes a test cache with Badger driver
func setupPreBuildTestCache(t *testing.T) func() {
	tmpDir, err := os.MkdirTemp("", "moss-prebuild-cache-test-*")
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
			},
		},
	}

	config.Config.Cache.Driver.Badger.Path = filepath.Join(tmpDir, "badger")

	if err := cache.Init(); err != nil {
		t.Fatalf("Failed to initialize cache: %v", err)
	}

	return func() {
		for _, item := range config.Config.Cache.Driver.Items() {
			_ = item.Close()
		}
		_ = os.RemoveAll(tmpDir)
	}
}

func newTestPreBuildPlugin(t *testing.T) *PreBuildArticleCache {
	logger, _ := zap.NewDevelopment()
	plugin := NewPreBuildArticleCache()
	plugin.ctx = &pluginEntity.Plugin{}
	plugin.ctx.Log = logger
	return plugin
}

func TestPreBuildArticleCache_InvalidateCache(t *testing.T) {
	cleanup := setupPreBuildTestCache(t)
	defer cleanup()

	plugin := newTestPreBuildPlugin(t)
	plugin.EnableOnCreate = true
	plugin.EnableOnUpdate = true

	article := &articleEntity.ArticleBase{
		ID:     1,
		Slug:   "test-article",
		Title:  "Test Article",
		Status: true,
	}
	articleURL := article.URL()

	// Set initial cache
	err := cache.Set("article", articleURL, []byte("old content"), 0)
	if err != nil {
		t.Fatalf("Failed to set initial cache: %v", err)
	}

	// Verify cache exists
	got, err := cache.Get("article", articleURL)
	if err != nil {
		t.Fatalf("Failed to get cache: %v", err)
	}
	if string(got) != "old content" {
		t.Errorf("Expected 'old content', got '%s'", got)
	}

	// Invalidate cache
	plugin.invalidateCache(&articleEntity.Article{ArticleBase: *article})

	// Verify cache is deleted
	_, err = cache.Get("article", articleURL)
	if err == nil {
		t.Error("Expected cache to be invalidated")
	}
}

func TestPreBuildArticleCache_InvalidateHomePageCache(t *testing.T) {
	cleanup := setupPreBuildTestCache(t)
	defer cleanup()

	plugin := newTestPreBuildPlugin(t)

	// Set home page cache
	err := cache.Set("home", "default", []byte("home content"), 0)
	if err != nil {
		t.Fatalf("Failed to set home cache: %v", err)
	}

	// Verify cache exists
	got, err := cache.Get("home", "default")
	if err != nil {
		t.Fatalf("Failed to get home cache: %v", err)
	}
	if string(got) != "home content" {
		t.Errorf("Expected 'home content', got '%s'", got)
	}

	// Invalidate home page cache
	plugin.invalidateHomePageCache()

	// Verify cache is deleted
	_, err = cache.Get("home", "default")
	if err == nil {
		t.Error("Expected home cache to be invalidated")
	}
}

func TestPreBuildArticleCache_ArticleCreateAfter(t *testing.T) {
	cleanup := setupPreBuildTestCache(t)
	defer cleanup()

	plugin := newTestPreBuildPlugin(t)
	// Don't enable build, just test cache invalidation
	plugin.EnableOnCreate = false

	article := &articleEntity.Article{
		ArticleBase: articleEntity.ArticleBase{
			ID:     1,
			Slug:   "new-article",
			Title:  "New Article",
			Status: true,
		},
	}

	// Set existing cache (simulating stale cache)
	articleURL := article.URL()
	err := cache.Set("article", articleURL, []byte("stale content"), 0)
	if err != nil {
		t.Fatalf("Failed to set stale cache: %v", err)
	}

	// Set home page cache
	err = cache.Set("home", "default", []byte("home content"), 0)
	if err != nil {
		t.Fatalf("Failed to set home cache: %v", err)
	}

	// Call ArticleCreateAfter
	plugin.ArticleCreateAfter(article)

	// Verify article cache was NOT invalidated (EnableOnCreate=false)
	got, err := cache.Get("article", articleURL)
	if err != nil {
		t.Fatalf("Expected article cache to still exist: %v", err)
	}
	if string(got) != "stale content" {
		t.Errorf("Expected 'stale content', got '%s'", got)
	}

	// Verify home page cache was invalidated (article is published)
	_, err = cache.Get("home", "default")
	if err == nil {
		t.Error("Expected home page cache to be invalidated for published article")
	}
}

func TestPreBuildArticleCache_ArticleUpdateAfter(t *testing.T) {
	cleanup := setupPreBuildTestCache(t)
	defer cleanup()

	plugin := newTestPreBuildPlugin(t)
	// Don't enable build, just test cache invalidation
	plugin.EnableOnUpdate = false

	article := &articleEntity.Article{
		ArticleBase: articleEntity.ArticleBase{
			ID:     1,
			Slug:   "updated-article",
			Title:  "Updated Article",
			Status: true,
		},
	}

	// Set existing cache
	articleURL := article.URL()
	err := cache.Set("article", articleURL, []byte("old content"), 0)
	if err != nil {
		t.Fatalf("Failed to set old cache: %v", err)
	}

	// Set home page cache
	err = cache.Set("home", "default", []byte("home content"), 0)
	if err != nil {
		t.Fatalf("Failed to set home cache: %v", err)
	}

	// Call ArticleUpdateAfter
	plugin.ArticleUpdateAfter(article)

	// Verify article cache was invalidated (always invalidated on update)
	_, err = cache.Get("article", articleURL)
	if err == nil {
		t.Error("Expected article cache to be invalidated after update")
	}

	// Verify home page cache was invalidated
	_, err = cache.Get("home", "default")
	if err == nil {
		t.Error("Expected home page cache to be invalidated after update")
	}
}

func TestPreBuildArticleCache_ArticleUpdateAfter_CacheDisabled(t *testing.T) {
	// Test with cache disabled
	config.Config = &aggregate.Config{
		Cache: &entity.Cache{
			Enable: false,
			Driver: core.NewDriver(),
		},
	}

	plugin := newTestPreBuildPlugin(t)
	plugin.EnableOnUpdate = true

	article := &articleEntity.Article{
		ArticleBase: articleEntity.ArticleBase{
			ID:     1,
			Slug:   "test-article",
			Title:  "Test Article",
			Status: true,
		},
	}

	// Should not panic when cache is disabled
	plugin.ArticleUpdateAfter(article)
}

func TestPreBuildArticleCache_UnpublishedArticle(t *testing.T) {
	cleanup := setupPreBuildTestCache(t)
	defer cleanup()

	plugin := newTestPreBuildPlugin(t)
	// Don't enable build, just test cache invalidation
	plugin.EnableOnCreate = false

	article := &articleEntity.Article{
		ArticleBase: articleEntity.ArticleBase{
			ID:     1,
			Slug:   "unpublished-article",
			Title:  "Unpublished Article",
			Status: false, // Not published
		},
	}

	// Set home page cache
	err := cache.Set("home", "default", []byte("home content"), 0)
	if err != nil {
		t.Fatalf("Failed to set home cache: %v", err)
	}

	// Call ArticleCreateAfter
	plugin.ArticleCreateAfter(article)

	// Home page cache should NOT be invalidated for unpublished article
	got, err := cache.Get("home", "default")
	if err != nil {
		t.Fatalf("Expected home cache to still exist: %v", err)
	}
	if string(got) != "home content" {
		t.Errorf("Expected 'home content', got '%s'", got)
	}
}

func TestPreBuildArticleCache_MultipleUpdates(t *testing.T) {
	cleanup := setupPreBuildTestCache(t)
	defer cleanup()

	plugin := newTestPreBuildPlugin(t)
	// Don't enable build, just test cache invalidation
	plugin.EnableOnUpdate = false

	// Create multiple articles
	articles := []*articleEntity.Article{
		{ArticleBase: articleEntity.ArticleBase{ID: 1, Slug: "article-1", Title: "Article 1", Status: true}},
		{ArticleBase: articleEntity.ArticleBase{ID: 2, Slug: "article-2", Title: "Article 2", Status: true}},
		{ArticleBase: articleEntity.ArticleBase{ID: 3, Slug: "article-3", Title: "Article 3", Status: true}},
	}

	// Set cache for each article
	for _, article := range articles {
		articleURL := article.URL()
		err := cache.Set("article", articleURL, []byte("content-"+article.Slug), 0)
		if err != nil {
			t.Fatalf("Failed to set cache: %v", err)
		}
	}

	// Update each article
	for _, article := range articles {
		plugin.ArticleUpdateAfter(article)
	}

	// Verify all caches are invalidated
	for _, article := range articles {
		articleURL := article.URL()
		_, err := cache.Get("article", articleURL)
		if err == nil {
			t.Errorf("Expected cache for %s to be invalidated", article.Slug)
		}
	}
}
