package adapters

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abhipray-cpu/niyantrak/limiters"
	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// mockLimiter is a simple mock limiter for testing
type mockLimiter struct {
	allowResult *limiters.LimitResult
}

func (m *mockLimiter) Allow(ctx context.Context, key string) *limiters.LimitResult {
	return m.allowResult
}

func (m *mockLimiter) AllowN(ctx context.Context, key string, n int) *limiters.LimitResult {
	return m.allowResult
}

func (m *mockLimiter) Reset(ctx context.Context, key string) error {
	return nil
}

func (m *mockLimiter) GetStats(ctx context.Context, key string) interface{} {
	return nil
}

func (m *mockLimiter) Close() error {
	return nil
}

func (m *mockLimiter) SetLimit(ctx context.Context, key string, limit int, window interface{}) error {
	return nil
}

func (m *mockLimiter) Type() string {
	return "mock"
}

// TestGinRateLimiter_Basic tests basic rate limiting
func TestGinRateLimiter_Basic(t *testing.T) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:   true,
			Remaining: 5,
			Limit:     10,
		},
	}

	middleware := GinRateLimiter(limiter, GinOptions{
		KeyExtractor: func(c *gin.Context) string {
			return "test-key"
		},
	})

	router := gin.New()
	router.Use(middleware)
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	// Check rate limit headers
	if rec.Header().Get("X-RateLimit-Limit") != "10" {
		t.Errorf("expected X-RateLimit-Limit: 10, got %s", rec.Header().Get("X-RateLimit-Limit"))
	}

	if rec.Header().Get("X-RateLimit-Remaining") != "5" {
		t.Errorf("expected X-RateLimit-Remaining: 5, got %s", rec.Header().Get("X-RateLimit-Remaining"))
	}
}

// TestGinRateLimiter_Denied tests rate limit exceeded
func TestGinRateLimiter_Denied(t *testing.T) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:    false,
			Remaining:  0,
			Limit:      10,
			RetryAfter: 60 * time.Second,
		},
	}

	middleware := GinRateLimiter(limiter, GinOptions{
		KeyExtractor: func(c *gin.Context) string {
			return "test-key"
		},
	})

	router := gin.New()
	router.Use(middleware)
	router.GET("/test", func(c *gin.Context) {
		t.Error("handler should not be called when rate limited")
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("expected status 429, got %d", rec.Code)
	}

	if rec.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header")
	}
}

// TestGinRateLimiter_DefaultKeyExtractor tests default key extraction
func TestGinRateLimiter_DefaultKeyExtractor(t *testing.T) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:   true,
			Remaining: 5,
			Limit:     10,
		},
	}

	middleware := GinRateLimiter(limiter, GinOptions{})

	router := gin.New()
	router.Use(middleware)
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-API-Key", "api-key-123")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

// TestGinRateLimiter_IPBasedKeyExtractor tests IP-based key extraction
func TestGinRateLimiter_IPBasedKeyExtractor(t *testing.T) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:   true,
			Remaining: 5,
			Limit:     10,
		},
	}

	middleware := GinRateLimiter(limiter, GinOptions{
		KeyExtractor: func(c *gin.Context) string {
			return c.ClientIP()
		},
	})

	router := gin.New()
	router.Use(middleware)
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

// TestGinRateLimiter_SkipPaths tests path skipping
func TestGinRateLimiter_SkipPaths(t *testing.T) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:   false, // Would deny if checked
			Remaining: 0,
			Limit:     10,
		},
	}

	middleware := GinRateLimiter(limiter, GinOptions{
		SkipPaths: []string{"/health", "/metrics"},
		KeyExtractor: func(c *gin.Context) string {
			return "test-key"
		},
	})

	router := gin.New()
	router.Use(middleware)
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	router.GET("/api", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	// Test skipped path
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("/health: expected status 200, got %d", rec.Code)
	}

	// Test non-skipped path
	req = httptest.NewRequest(http.MethodGet, "/api", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("/api: expected status 429, got %d", rec.Code)
	}
}

// TestGinRateLimiter_CustomErrorHandler tests custom error handling
func TestGinRateLimiter_CustomErrorHandler(t *testing.T) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:   false,
			Remaining: 0,
			Limit:     10,
		},
	}

	customHandlerCalled := false
	middleware := GinRateLimiter(limiter, GinOptions{
		KeyExtractor: func(c *gin.Context) string {
			return "test-key"
		},
		OnRateLimitExceeded: func(c *gin.Context) {
			customHandlerCalled = true
			c.JSON(http.StatusTeapot, gin.H{"error": "custom error"})
		},
	})

	router := gin.New()
	router.Use(middleware)
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if !customHandlerCalled {
		t.Error("custom handler should have been called")
	}

	if rec.Code != http.StatusTeapot {
		t.Errorf("expected status 418, got %d", rec.Code)
	}
}

// TestGinRateLimiter_HeadersDisabled tests disabling headers
func TestGinRateLimiter_HeadersDisabled(t *testing.T) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:   true,
			Remaining: 5,
			Limit:     10,
		},
	}

	falseVal := false
	middleware := GinRateLimiter(limiter, GinOptions{
		KeyExtractor: func(c *gin.Context) string {
			return "test-key"
		},
		IncludeHeaders: &falseVal,
	})

	router := gin.New()
	router.Use(middleware)
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	if rec.Header().Get("X-RateLimit-Limit") != "" {
		t.Error("expected no rate limit headers")
	}
}

// TestGinRateLimiter_AbortOnError tests aborting on error
func TestGinRateLimiter_AbortOnError(t *testing.T) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:   true,
			Remaining: 5,
			Limit:     10,
		},
	}

	middleware := GinRateLimiter(limiter, GinOptions{
		KeyExtractor: func(c *gin.Context) string {
			return "" // Empty key should trigger error
		},
		AbortOnError: true,
	})

	router := gin.New()
	router.Use(middleware)
	router.GET("/test", func(c *gin.Context) {
		t.Error("handler should not be called on error")
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}
}

// TestGinRateLimiter_MultipleRequests tests multiple sequential requests
func TestGinRateLimiter_MultipleRequests(t *testing.T) {
	requestCount := 0
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:   true,
			Remaining: 5,
			Limit:     10,
		},
	}

	middleware := GinRateLimiter(limiter, GinOptions{
		KeyExtractor: func(c *gin.Context) string {
			return "test-key"
		},
	})

	router := gin.New()
	router.Use(middleware)
	router.GET("/test", func(c *gin.Context) {
		requestCount++
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("request %d: expected status 200, got %d", i+1, rec.Code)
		}
	}

	if requestCount != 5 {
		t.Errorf("expected 5 requests to handler, got %d", requestCount)
	}
}

// BenchmarkGinRateLimiter benchmarks the Gin middleware
func BenchmarkGinRateLimiter(b *testing.B) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:   true,
			Remaining: 5,
			Limit:     10,
		},
	}

	middleware := GinRateLimiter(limiter, GinOptions{
		KeyExtractor: func(c *gin.Context) string {
			return "test-key"
		},
	})

	router := gin.New()
	router.Use(middleware)
	router.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
	}
}
