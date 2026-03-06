package adapters

import (
	"fmt"
	"strconv"

	"github.com/abhipray-cpu/niyantrak/limiters"
	"github.com/labstack/echo/v4"
)

// EchoOptions contains configuration options for Echo rate limiter
type EchoOptions struct {
	// KeyExtractor extracts the rate limit key from the Echo context
	// If nil, defaults to extracting from X-API-Key header or client IP
	KeyExtractor func(echo.Context) string

	// OnRateLimitExceeded is called when rate limit is exceeded
	// If nil, returns 429 with JSON error
	OnRateLimitExceeded func(echo.Context) error

	// OnError is called when an error occurs during rate limiting
	// If nil, returns 400 with JSON error
	OnError func(echo.Context, error) error

	// SkipPaths contains paths that should skip rate limiting
	SkipPaths []string

	// IncludeHeaders determines if rate limit headers should be included
	// Default: true (set to &false to disable)
	IncludeHeaders *bool

	// AbortOnError determines if the request should be aborted on error
	// Default: false (continues to handler)
	AbortOnError bool
}

// EchoRateLimiter creates an Echo middleware for rate limiting
func EchoRateLimiter(limiter interface{}, options EchoOptions) echo.MiddlewareFunc {
	// Set defaults
	if options.KeyExtractor == nil {
		options.KeyExtractor = defaultEchoKeyExtractor
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Check if path should be skipped
			if shouldSkipPath(c.Request().URL.Path, options.SkipPaths) {
				return next(c)
			}

			// Extract key
			key := options.KeyExtractor(c)
			if key == "" {
				return handleEchoError(c, fmt.Errorf("empty rate limit key"), options, next)
			}

			// Check rate limit
			result := checkLimit(c.Request().Context(), limiter, key)

			// Set headers (default true unless explicitly disabled)
			shouldSetHeaders := true
			if options.IncludeHeaders != nil && !*options.IncludeHeaders {
				shouldSetHeaders = false
			}
			if shouldSetHeaders {
				setEchoRateLimitHeaders(c, result)
			}

			// Handle rate limit exceeded
			if !result.Allowed {
				return handleEchoRateLimitExceeded(c, result, options)
			}

			// Continue to next handler
			return next(c)
		}
	}
}

// defaultEchoKeyExtractor is the default key extraction function
func defaultEchoKeyExtractor(c echo.Context) string {
	// Try X-API-Key header
	if key := c.Request().Header.Get("X-API-Key"); key != "" {
		return key
	}

	// Try Authorization header
	if auth := c.Request().Header.Get("Authorization"); auth != "" {
		return auth
	}

	// Fall back to client IP
	return c.RealIP()
}

// setEchoRateLimitHeaders sets rate limit headers on the response
func setEchoRateLimitHeaders(c echo.Context, result *limiters.LimitResult) {
	c.Response().Header().Set("X-RateLimit-Limit", strconv.Itoa(result.Limit))
	c.Response().Header().Set("X-RateLimit-Remaining", strconv.Itoa(result.Remaining))

	if !result.ResetAt.IsZero() {
		c.Response().Header().Set("X-RateLimit-Reset", strconv.FormatInt(result.ResetAt.Unix(), 10))
	}

	if !result.Allowed && result.RetryAfter > 0 {
		c.Response().Header().Set("Retry-After", fmt.Sprintf("%.0f", result.RetryAfter.Seconds()))
	}
}

// handleEchoRateLimitExceeded handles rate limit exceeded scenario
func handleEchoRateLimitExceeded(c echo.Context, result *limiters.LimitResult, options EchoOptions) error {
	if options.OnRateLimitExceeded != nil {
		return options.OnRateLimitExceeded(c)
	}

	// Default handler
	return c.JSON(429, map[string]interface{}{
		"error":   "rate limit exceeded",
		"message": fmt.Sprintf("Too many requests. Limit: %d requests", result.Limit),
	})
}

// handleEchoError handles errors during rate limiting
func handleEchoError(c echo.Context, err error, options EchoOptions, next echo.HandlerFunc) error {
	if options.OnError != nil {
		return options.OnError(c, err)
	}

	// Default error handler
	if options.AbortOnError {
		return c.JSON(400, map[string]interface{}{
			"error":   "rate limit error",
			"message": err.Error(),
		})
	}

	return next(c)
}
