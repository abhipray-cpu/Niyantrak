package adapters

import (
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abhipray-cpu/niyantrak/limiters"
	"github.com/gofiber/fiber/v2"
)

// TestFiberRateLimiter_Basic tests basic rate limiting
func TestFiberRateLimiter_Basic(t *testing.T) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:   true,
			Remaining: 5,
			Limit:     10,
		},
	}

	app := fiber.New()
	app.Use(FiberRateLimiter(limiter, FiberOptions{
		KeyExtractor: func(c *fiber.Ctx) string {
			return "test-key"
		},
	}))
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"message": "success"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	if resp.Header.Get("X-RateLimit-Limit") != "10" {
		t.Errorf("expected X-RateLimit-Limit: 10, got %s", resp.Header.Get("X-RateLimit-Limit"))
	}
}

// TestFiberRateLimiter_Denied tests rate limit exceeded
func TestFiberRateLimiter_Denied(t *testing.T) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:    false,
			Remaining:  0,
			Limit:      10,
			RetryAfter: 60 * time.Second,
		},
	}

	app := fiber.New()
	app.Use(FiberRateLimiter(limiter, FiberOptions{
		KeyExtractor: func(c *fiber.Ctx) string {
			return "test-key"
		},
	}))
	app.Get("/test", func(c *fiber.Ctx) error {
		t.Error("handler should not be called when rate limited")
		return c.JSON(fiber.Map{"message": "success"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}

	if resp.StatusCode != 429 {
		t.Errorf("expected status 429, got %d", resp.StatusCode)
	}
}

// TestFiberRateLimiter_DefaultKeyExtractor tests default key extraction
func TestFiberRateLimiter_DefaultKeyExtractor(t *testing.T) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:   true,
			Remaining: 5,
			Limit:     10,
		},
	}

	app := fiber.New()
	app.Use(FiberRateLimiter(limiter, FiberOptions{}))
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"message": "success"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-API-Key", "api-key-123")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

// TestFiberRateLimiter_SkipPaths tests path skipping
func TestFiberRateLimiter_SkipPaths(t *testing.T) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:   false,
			Remaining: 0,
			Limit:     10,
		},
	}

	app := fiber.New()
	app.Use(FiberRateLimiter(limiter, FiberOptions{
		SkipPaths: []string{"/health", "/metrics"},
		KeyExtractor: func(c *fiber.Ctx) string {
			return "test-key"
		},
	}))
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})
	app.Get("/api", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"message": "success"})
	})

	// Test skipped path
	req := httptest.NewRequest("GET", "/health", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("/health: expected status 200, got %d", resp.StatusCode)
	}

	// Test non-skipped path
	req = httptest.NewRequest("GET", "/api", nil)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatal(err)
	}

	if resp.StatusCode != 429 {
		t.Errorf("/api: expected status 429, got %d", resp.StatusCode)
	}
}

// TestFiberRateLimiter_CustomErrorHandler tests custom error handling
func TestFiberRateLimiter_CustomErrorHandler(t *testing.T) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:   false,
			Remaining: 0,
			Limit:     10,
		},
	}

	customHandlerCalled := false
	app := fiber.New()
	app.Use(FiberRateLimiter(limiter, FiberOptions{
		KeyExtractor: func(c *fiber.Ctx) string {
			return "test-key"
		},
		OnRateLimitExceeded: func(c *fiber.Ctx) error {
			customHandlerCalled = true
			return c.Status(418).JSON(fiber.Map{"error": "custom error"})
		},
	}))
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"message": "success"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}

	if !customHandlerCalled {
		t.Error("custom handler should have been called")
	}

	if resp.StatusCode != 418 {
		t.Errorf("expected status 418, got %d", resp.StatusCode)
	}
}

// TestFiberRateLimiter_HeadersDisabled tests disabling headers
func TestFiberRateLimiter_HeadersDisabled(t *testing.T) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:   true,
			Remaining: 5,
			Limit:     10,
		},
	}

	falseVal := false
	app := fiber.New()
	app.Use(FiberRateLimiter(limiter, FiberOptions{
		KeyExtractor: func(c *fiber.Ctx) string {
			return "test-key"
		},
		IncludeHeaders: &falseVal,
	}))
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"message": "success"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	if resp.Header.Get("X-RateLimit-Limit") != "" {
		t.Error("expected no rate limit headers")
	}
}

// BenchmarkFiberRateLimiter benchmarks the Fiber middleware
func BenchmarkFiberRateLimiter(b *testing.B) {
	limiter := &mockLimiter{
		allowResult: &limiters.LimitResult{
			Allowed:   true,
			Remaining: 5,
			Limit:     10,
		},
	}

	app := fiber.New()
	app.Use(FiberRateLimiter(limiter, FiberOptions{
		KeyExtractor: func(c *fiber.Ctx) string {
			return "test-key"
		},
	}))
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendStatus(200)
	})

	req := httptest.NewRequest("GET", "/test", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, _ := app.Test(req)
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
}
