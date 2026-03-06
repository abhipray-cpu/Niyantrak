package adapters

import (
	"fmt"
	"strconv"

	"github.com/abhipray-cpu/niyantrak/limiters"
	"github.com/gofiber/fiber/v2"
)

// FiberOptions contains configuration options for Fiber rate limiter
type FiberOptions struct {
	// KeyExtractor extracts the rate limit key from the Fiber context
	// If nil, defaults to extracting from X-API-Key header or client IP
	KeyExtractor func(*fiber.Ctx) string

	// OnRateLimitExceeded is called when rate limit is exceeded
	// If nil, returns 429 with JSON error
	OnRateLimitExceeded func(*fiber.Ctx) error

	// OnError is called when an error occurs during rate limiting
	// If nil, returns 400 with JSON error
	OnError func(*fiber.Ctx, error) error

	// SkipPaths contains paths that should skip rate limiting
	SkipPaths []string

	// IncludeHeaders determines if rate limit headers should be included
	// Default: true (set to &false to disable)
	IncludeHeaders *bool

	// AbortOnError determines if the request should be aborted on error
	// Default: false (continues to handler)
	AbortOnError bool
}

// FiberRateLimiter creates a Fiber middleware for rate limiting
func FiberRateLimiter(limiter interface{}, options FiberOptions) fiber.Handler {
	// Set defaults
	if options.KeyExtractor == nil {
		options.KeyExtractor = defaultFiberKeyExtractor
	}

	return func(c *fiber.Ctx) error {
		// Check if path should be skipped
		if shouldSkipPath(c.Path(), options.SkipPaths) {
			return c.Next()
		}

		// Extract key
		key := options.KeyExtractor(c)
		if key == "" {
			return handleFiberError(c, fmt.Errorf("empty rate limit key"), options)
		}

		// Check rate limit
		result := checkLimit(c.Context(), limiter, key)

		// Set headers (default true unless explicitly disabled)
		shouldSetHeaders := true
		if options.IncludeHeaders != nil && !*options.IncludeHeaders {
			shouldSetHeaders = false
		}
		if shouldSetHeaders {
			setFiberRateLimitHeaders(c, result)
		}

		// Handle rate limit exceeded
		if !result.Allowed {
			return handleFiberRateLimitExceeded(c, result, options)
		}

		// Continue to next handler
		return c.Next()
	}
}

// defaultFiberKeyExtractor is the default key extraction function
func defaultFiberKeyExtractor(c *fiber.Ctx) string {
	// Try X-API-Key header
	if key := c.Get("X-API-Key"); key != "" {
		return key
	}

	// Try Authorization header
	if auth := c.Get("Authorization"); auth != "" {
		return auth
	}

	// Fall back to client IP
	return c.IP()
}

// setFiberRateLimitHeaders sets rate limit headers on the response
func setFiberRateLimitHeaders(c *fiber.Ctx, result *limiters.LimitResult) {
	c.Set("X-RateLimit-Limit", strconv.Itoa(result.Limit))
	c.Set("X-RateLimit-Remaining", strconv.Itoa(result.Remaining))

	if !result.ResetAt.IsZero() {
		c.Set("X-RateLimit-Reset", strconv.FormatInt(result.ResetAt.Unix(), 10))
	}

	if !result.Allowed && result.RetryAfter > 0 {
		c.Set("Retry-After", fmt.Sprintf("%.0f", result.RetryAfter.Seconds()))
	}
}

// handleFiberRateLimitExceeded handles rate limit exceeded scenario
func handleFiberRateLimitExceeded(c *fiber.Ctx, result *limiters.LimitResult, options FiberOptions) error {
	if options.OnRateLimitExceeded != nil {
		return options.OnRateLimitExceeded(c)
	}

	// Default handler
	return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
		"error":   "rate limit exceeded",
		"message": fmt.Sprintf("Too many requests. Limit: %d requests", result.Limit),
	})
}

// handleFiberError handles errors during rate limiting
func handleFiberError(c *fiber.Ctx, err error, options FiberOptions) error {
	if options.OnError != nil {
		return options.OnError(c, err)
	}

	// Default error handler
	if options.AbortOnError {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "rate limit error",
			"message": err.Error(),
		})
	}

	return c.Next()
}
