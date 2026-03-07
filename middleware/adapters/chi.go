package adapters

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/abhipray-cpu/niyantrak/limiters"
)

// ChiOptions contains configuration options for Chi rate limiter
type ChiOptions struct {
	// KeyExtractor extracts the rate limit key from the HTTP request
	// If nil, defaults to extracting from X-API-Key header or client IP
	KeyExtractor func(*http.Request) string

	// OnRateLimitExceeded is called when rate limit is exceeded
	// If nil, returns 429 with JSON error
	OnRateLimitExceeded func(http.ResponseWriter, *http.Request)

	// OnError is called when an error occurs during rate limiting
	// If nil, returns 400 with JSON error
	OnError func(http.ResponseWriter, *http.Request, error)

	// SkipPaths contains paths that should skip rate limiting
	SkipPaths []string

	// IncludeHeaders determines if rate limit headers should be included
	// Default: true (set to &false to disable)
	IncludeHeaders *bool

	// AbortOnError determines if the request should be aborted on error
	// Default: false (continues to handler)
	AbortOnError bool
}

// ChiRateLimiter creates a Chi middleware for rate limiting
func ChiRateLimiter(limiter interface{}, options ChiOptions) func(http.Handler) http.Handler {
	// Set defaults
	if options.KeyExtractor == nil {
		options.KeyExtractor = defaultChiKeyExtractor
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if path should be skipped
			if shouldSkipPath(r.URL.Path, options.SkipPaths) {
				next.ServeHTTP(w, r)
				return
			}

			// Extract key
			key := options.KeyExtractor(r)
			if key == "" {
				handleChiError(w, r, fmt.Errorf("empty rate limit key"), options, next)
				return
			}

			// Check rate limit
			result := checkLimit(r.Context(), limiter, key)

			// Set headers (default true unless explicitly disabled)
			shouldSetHeaders := true
			if options.IncludeHeaders != nil && !*options.IncludeHeaders {
				shouldSetHeaders = false
			}
			if shouldSetHeaders {
				setChiRateLimitHeaders(w, result)
			}

			// Handle rate limit exceeded
			if !result.Allowed {
				handleChiRateLimitExceeded(w, r, result, options)
				return
			}

			// Continue to next handler
			next.ServeHTTP(w, r)
		})
	}
}

// defaultChiKeyExtractor is the default key extraction function
func defaultChiKeyExtractor(r *http.Request) string {
	// Try X-API-Key header
	if key := r.Header.Get("X-API-Key"); key != "" {
		return key
	}

	// Try Authorization header
	if auth := r.Header.Get("Authorization"); auth != "" {
		return auth
	}

	// Fall back to remote address
	return r.RemoteAddr
}

// setChiRateLimitHeaders sets rate limit headers on the response
func setChiRateLimitHeaders(w http.ResponseWriter, result *limiters.LimitResult) {
	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(result.Limit))
	w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(result.Remaining))

	if !result.ResetAt.IsZero() {
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(result.ResetAt.Unix(), 10))
	}

	if !result.Allowed && result.RetryAfter > 0 {
		w.Header().Set("Retry-After", fmt.Sprintf("%.0f", result.RetryAfter.Seconds()))
	}
}

// handleChiRateLimitExceeded handles rate limit exceeded scenario
func handleChiRateLimitExceeded(w http.ResponseWriter, r *http.Request, result *limiters.LimitResult, options ChiOptions) {
	if options.OnRateLimitExceeded != nil {
		options.OnRateLimitExceeded(w, r)
		return
	}

	// Default handler
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error":   "rate limit exceeded",
		"message": fmt.Sprintf("Too many requests. Limit: %d requests", result.Limit),
	})
}

// handleChiError handles errors during rate limiting
func handleChiError(w http.ResponseWriter, r *http.Request, err error, options ChiOptions, next http.Handler) {
	if options.OnError != nil {
		options.OnError(w, r, err)
		if options.AbortOnError {
			return
		}
	}

	// Default error handler
	if options.AbortOnError {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "rate limit error",
			"message": err.Error(),
		})
		return
	}

	next.ServeHTTP(w, r)
}
