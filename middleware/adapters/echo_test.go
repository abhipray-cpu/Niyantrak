package adapters

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abhipray-cpu/niyantrak/limiters"
	"github.com/labstack/echo/v4"
)

// TestEchoRateLimiter_Basic tests basic rate limiting
func TestEchoRateLimiter_Basic(t *testing.T) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:   true,
			Remaining: 5,
			Limit:     10,
		},
	}

	e := echo.New()
	e.Use(EchoRateLimiter(limiter, EchoOptions{
		KeyExtractor: func(c echo.Context) string {
			return "test-key"
		},
	}))
	e.GET("/test", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"message": "success"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	if rec.Header().Get("X-RateLimit-Limit") != "10" {
		t.Errorf("expected X-RateLimit-Limit: 10, got %s", rec.Header().Get("X-RateLimit-Limit"))
	}
}

// TestEchoRateLimiter_Denied tests rate limit exceeded
func TestEchoRateLimiter_Denied(t *testing.T) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:    false,
			Remaining:  0,
			Limit:      10,
			RetryAfter: 60 * time.Second,
		},
	}

	e := echo.New()
	e.Use(EchoRateLimiter(limiter, EchoOptions{
		KeyExtractor: func(c echo.Context) string {
			return "test-key"
		},
	}))
	e.GET("/test", func(c echo.Context) error {
		t.Error("handler should not be called when rate limited")
		return c.JSON(http.StatusOK, map[string]string{"message": "success"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("expected status 429, got %d", rec.Code)
	}
}

// TestEchoRateLimiter_DefaultKeyExtractor tests default key extraction
func TestEchoRateLimiter_DefaultKeyExtractor(t *testing.T) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:   true,
			Remaining: 5,
			Limit:     10,
		},
	}

	e := echo.New()
	e.Use(EchoRateLimiter(limiter, EchoOptions{}))
	e.GET("/test", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"message": "success"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-API-Key", "api-key-123")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

// TestEchoRateLimiter_SkipPaths tests path skipping
func TestEchoRateLimiter_SkipPaths(t *testing.T) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:   false,
			Remaining: 0,
			Limit:     10,
		},
	}

	e := echo.New()
	e.Use(EchoRateLimiter(limiter, EchoOptions{
		SkipPaths: []string{"/health", "/metrics"},
		KeyExtractor: func(c echo.Context) string {
			return "test-key"
		},
	}))
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})
	e.GET("/api", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"message": "success"})
	})

	// Test skipped path
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("/health: expected status 200, got %d", rec.Code)
	}

	// Test non-skipped path
	req = httptest.NewRequest(http.MethodGet, "/api", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("/api: expected status 429, got %d", rec.Code)
	}
}

// TestEchoRateLimiter_CustomErrorHandler tests custom error handling
func TestEchoRateLimiter_CustomErrorHandler(t *testing.T) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:   false,
			Remaining: 0,
			Limit:     10,
		},
	}

	customHandlerCalled := false
	e := echo.New()
	e.Use(EchoRateLimiter(limiter, EchoOptions{
		KeyExtractor: func(c echo.Context) string {
			return "test-key"
		},
		OnRateLimitExceeded: func(c echo.Context) error {
			customHandlerCalled = true
			return c.JSON(http.StatusTeapot, map[string]string{"error": "custom error"})
		},
	}))
	e.GET("/test", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"message": "success"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	if !customHandlerCalled {
		t.Error("custom handler should have been called")
	}

	if rec.Code != http.StatusTeapot {
		t.Errorf("expected status 418, got %d", rec.Code)
	}
}

// TestEchoRateLimiter_HeadersDisabled tests disabling headers
func TestEchoRateLimiter_HeadersDisabled(t *testing.T) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:   true,
			Remaining: 5,
			Limit:     10,
		},
	}

	falseVal := false
	e := echo.New()
	e.Use(EchoRateLimiter(limiter, EchoOptions{
		KeyExtractor: func(c echo.Context) string {
			return "test-key"
		},
		IncludeHeaders: &falseVal,
	}))
	e.GET("/test", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"message": "success"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	if rec.Header().Get("X-RateLimit-Limit") != "" {
		t.Error("expected no rate limit headers")
	}
}

// BenchmarkEchoRateLimiter benchmarks the Echo middleware
func BenchmarkEchoRateLimiter(b *testing.B) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:   true,
			Remaining: 5,
			Limit:     10,
		},
	}

	e := echo.New()
	e.Use(EchoRateLimiter(limiter, EchoOptions{
		KeyExtractor: func(c echo.Context) string {
			return "test-key"
		},
	}))
	e.GET("/test", func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
	}
}
