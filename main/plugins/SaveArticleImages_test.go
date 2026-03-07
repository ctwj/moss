package plugins

import (
	"testing"
	"time"

	pluginEntity "moss/domain/support/entity"
	"go.uber.org/zap"
)

// MockPlugin implements pluginEntity.Plugin for testing
type MockPlugin struct {
	log *zap.Logger
}

func (m *MockPlugin) GetLog() *zap.Logger {
	if m.log == nil {
		m.log, _ = zap.NewDevelopment()
	}
	return m.log
}

func (m *MockPlugin) SetLog(log *zap.Logger) {
	m.log = log
}

func TestSaveArticleImages_RateLimiting(t *testing.T) {
	// 创建插件实例
	plugin := NewSaveArticleImages()
	plugin.APIRateLimitPerMinute = 5 // 设置每分钟5次限制用于测试
	plugin.APIMaxQueueSize = 10
	plugin.APIQueueTimeout = 60

	// 创建模拟插件上下文
	mockCtx := &MockPlugin{}
	plugin.ctx = &pluginEntity.Plugin{}
	plugin.ctx.Log = mockCtx.GetLog()

	// 初始化频率限制器
	err := plugin.initRateLimiter()
	if err != nil {
		t.Fatalf("initRateLimiter failed: %v", err)
	}

	// 初始化上传队列
	err = plugin.initUploadQueue()
	if err != nil {
		t.Fatalf("initUploadQueue failed: %v", err)
	}

	defer plugin.Unload()

	// 测试频率限制
	t.Run("DirectUploadWhenRateAvailable", func(t *testing.T) {
		// 应该有可用的令牌
		if !plugin.rateLimiter.Allow() {
			t.Error("Expected rate limiter to allow request")
		}
	})

	t.Run("QueueStats", func(t *testing.T) {
		stats := plugin.GetQueueStats()
		if stats["rate_limit_per_minute"] != 5 {
			t.Errorf("Expected rate limit per minute to be 5, got %v", stats["rate_limit_per_minute"])
		}
		if stats["queue_length"] != 0 {
			t.Errorf("Expected queue length to be 0, got %v", stats["queue_length"])
		}
	})

	t.Run("RateLimitExhaustion", func(t *testing.T) {
		// 重置限流器以获得完整的令牌
		plugin.initRateLimiter()

		// 消耗所有初始令牌
		allowedCount := 0
		for i := 0; i < 5; i++ {
			if plugin.rateLimiter.Allow() {
				allowedCount++
			}
		}

		// 应该允许5次请求（初始令牌数）
		if allowedCount != 5 {
			t.Errorf("Expected 5 requests to be allowed, got %d", allowedCount)
		}

		// 第6次应该被拒绝（没有令牌了）
		if plugin.rateLimiter.Allow() {
			t.Error("Expected rate limiter to deny request after exhausting tokens")
		}
	})
}

func TestSaveArticleImages_QueueProcessing(t *testing.T) {
	// 创建插件实例
	plugin := NewSaveArticleImages()
	plugin.APIRateLimitPerMinute = 10 // 设置每分钟10次限制
	plugin.APIMaxQueueSize = 5
	plugin.APIQueueTimeout = 30

	// 创建模拟插件上下文
	mockCtx := &MockPlugin{}
	plugin.ctx = &pluginEntity.Plugin{}
	plugin.ctx.Log = mockCtx.GetLog()

	// 初始化频率限制器和队列
	err := plugin.initRateLimiter()
	if err != nil {
		t.Fatalf("initRateLimiter failed: %v", err)
	}

	err = plugin.initUploadQueue()
	if err != nil {
		t.Fatalf("initUploadQueue failed: %v", err)
	}

	defer plugin.Unload()

	// 测试队列处理
	t.Run("QueueTaskCreation", func(t *testing.T) {
		// 创建一个测试任务
		task := &uploadTask{
			TaskID:    "test-task-1",
			Name:      "test",
			Ext:       ".jpg",
			ImgType:   "image/jpeg",
			File:      []byte("test image data"),
			Result:    make(chan *uploadResult, 1),
			Retries:   0,
			CreatedAt: time.Now(),
		}

		// 尝试将任务加入队列
		select {
		case plugin.uploadQueue <- task:
			// 成功加入队列
		default:
			t.Error("Failed to add task to queue")
		}

		// 检查队列长度
		if len(plugin.uploadQueue) != 1 {
			t.Errorf("Expected queue length to be 1, got %d", len(plugin.uploadQueue))
		}
	})

	t.Run("QueueProcessor", func(t *testing.T) {
		// 给队列处理器一些时间
		time.Sleep(200 * time.Millisecond)

		// 检查队列是否被处理
		stats := plugin.GetQueueStats()
		t.Logf("Queue stats after processing: %+v", stats)
	})
}

func TestUploadTask(t *testing.T) {
	// 测试上传任务结构
	task := &uploadTask{
		TaskID:    "test-123",
		Name:      "test-image",
		Ext:       ".png",
		ImgType:   "image/png",
		File:      []byte("fake image data"),
		Result:    make(chan *uploadResult, 1),
		Retries:   0,
		CreatedAt: time.Now(),
	}

	// 测试结果通道
	go func() {
		result := &uploadResult{
			URL:       "https://example.com/image.png",
			Error:     nil,
			Completed: true,
			Retried:   0,
		}
		task.Result <- result
	}()

	select {
	case result := <-task.Result:
		if result.URL != "https://example.com/image.png" {
			t.Errorf("Expected URL to be https://example.com/image.png, got %s", result.URL)
		}
		if result.Error != nil {
			t.Errorf("Expected no error, got %v", result.Error)
		}
	case <-time.After(1 * time.Second):
		t.Error("Timeout waiting for result")
	}
}

// Benchmark rate limiter performance
func BenchmarkRateLimiter(b *testing.B) {
	plugin := NewSaveArticleImages()
	plugin.APIRateLimitPerMinute = 100

	mockCtx := &MockPlugin{}
	plugin.ctx = &pluginEntity.Plugin{}
	plugin.ctx.Log = mockCtx.GetLog()

	err := plugin.initRateLimiter()
	if err != nil {
		b.Fatalf("initRateLimiter failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		plugin.rateLimiter.Allow()
	}
}