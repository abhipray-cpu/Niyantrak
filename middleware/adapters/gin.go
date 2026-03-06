package adapters

import (
	"context"
	"fmt"
	"strconv"

	"github.com/abhipray-cpu/niyantrak/limiters"
	"github.com/gin-gonic/gin"
)

// GinOptions contains configuration options for Gin rate limiter
type GinOptions struct {
	// KeyExtractor extracts the rate limit key from the Gin context
	// If nil, defaults to extracting from X-API-Key header or client IP
	KeyExtractor func(*gin.Context) string

	// OnRateLimitExceeded is called when rate limit is exceeded
	// If nil, returns 429 with JSON error
	OnRateLimitExceeded func(*gin.Context)

	// OnError is called when an error occurs during rate limiting
	// If nil, returns 400 with JSON error
	OnError func(*gin.Context, error)

	// SkipPaths contains paths that should skip rate limiting
	SkipPaths []string

	// IncludeHeaders determines if rate limit headers should be included
	// Use pointer to distinguish between not set (nil = default true) and explicitly false
	IncludeHeaders *bool

	// AbortOnError determines if the request should be aborted on error
	// Default: false (continues to handler)
	AbortOnError bool
}

// GinRateLimiter creates a Gin middleware for rate limiting
func GinRateLimiter(limiter interface{}, options GinOptions) gin.HandlerFunc {
	// Set defaults
	if options.KeyExtractor == nil {
		options.KeyExtractor = defaultGinKeyExtractor
	}

	return func(c *gin.Context) {
		// Check if path should be skipped
		if shouldSkipPath(c.Request.URL.Path, options.SkipPaths) {
			c.Next()
			return
		}

		// Extract key
		key := options.KeyExtractor(c)
		if key == "" {
			handleError(c, fmt.Errorf("empty rate limit key"), options)
			return
		}

		// Check rate limit
		result := checkLimit(c.Request.Context(), limiter, key)

		// Set headers (default true unless explicitly disabled)
		shouldSetHeaders := true
		if options.IncludeHeaders != nil && !*options.IncludeHeaders {
			shouldSetHeaders = false
		}
		if shouldSetHeaders {
			setRateLimitHeaders(c, result)
		}

		// Handle rate limit exceeded
		if !result.Allowed {
			handleRateLimitExceeded(c, result, options)
			return
		}

		// Continue to next handler
		c.Next()
	}
}

// defaultGinKeyExtractor is the default key extraction function
func defaultGinKeyExtractor(c *gin.Context) string {
	// Try X-API-Key header
	if key := c.GetHeader("X-API-Key"); key != "" {
		return key
	}

	// Try Authorization header
	if auth := c.GetHeader("Authorization"); auth != "" {
		return auth
	}

	// Fall back to client IP
	return c.ClientIP()
}

// shouldSkipPath checks if the path should skip rate limiting
func shouldSkipPath(path string, skipPaths []string) bool {
	for _, skipPath := range skipPaths {
		if path == skipPath {
			return true
		}
	}
	return false
}

// checkLimit checks the rate limit using the limiter
func checkLimit(ctx context.Context, limiter interface{}, key string) *limiters.LimitResult {
	switch l := limiter.(type) {
	case interface {
		Allow(context.Context, string) *limiters.LimitResult
	}:
		return l.Allow(ctx, key)
	default:
		// Return denied result if limiter type is unknown
		return &limiters.LimitResult{
			Allowed:   false,
			Remaining: 0,
			Limit:     0,
		}
	}
}

// setRateLimitHeaders sets rate limit headers on the response
func setRateLimitHeaders(c *gin.Context, result *limiters.LimitResult) {
	c.Header("X-RateLimit-Limit", strconv.Itoa(result.Limit))
	c.Header("X-RateLimit-Remaining", strconv.Itoa(result.Remaining))

	if !result.ResetAt.IsZero() {
		c.Header("X-RateLimit-Reset", strconv.FormatInt(result.ResetAt.Unix(), 10))
	}

	if !result.Allowed && result.RetryAfter > 0 {
		c.Header("Retry-After", fmt.Sprintf("%.0f", result.RetryAfter.Seconds()))
	}
}

// handleRateLimitExceeded handles rate limit exceeded scenario
func handleRateLimitExceeded(c *gin.Context, result *limiters.LimitResult, options GinOptions) {
	if options.OnRateLimitExceeded != nil {
		options.OnRateLimitExceeded(c)
		c.Abort()
		return
	}

	// Default handler
	c.AbortWithStatusJSON(429, gin.H{
		"error":   "rate limit exceeded",
		"message": fmt.Sprintf("Too many requests. Limit: %d requests", result.Limit),
	})
}

// handleError handles errors during rate limiting
func handleError(c *gin.Context, err error, options GinOptions) {
	if options.OnError != nil {
		options.OnError(c, err)
		if options.AbortOnError {
			c.Abort()
		}
		return
	}

	// Default error handler
	if options.AbortOnError {
		c.AbortWithStatusJSON(400, gin.H{
			"error":   "rate limit error",
			"message": err.Error(),
		})
	} else {
		c.Next()
	}
}
