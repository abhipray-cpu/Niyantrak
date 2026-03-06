package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/abhipray-cpu/niyantrak/limiters"
	"github.com/abhipray-cpu/niyantrak/middleware"
)

// httpMiddleware implements the middleware.HTTPMiddleware interface
type httpMiddleware struct {
	keyExtractor middleware.KeyExtractor
}

// New creates a new HTTP middleware instance
func New() middleware.HTTPMiddleware {
	return &httpMiddleware{
		keyExtractor: defaultKeyExtractor,
	}
}

// Wrap wraps an http.Handler with rate limiting
func (h *httpMiddleware) Wrap(handler http.Handler, limiter interface{}, options *middleware.HTTPOptions) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Apply rate limiting logic
		if !h.shouldCheckRateLimit(r, options) {
			handler.ServeHTTP(w, r)
			return
		}

		// Extract key
		extractor := h.getExtractor(options)
		key, err := extractor(r)
		if err != nil || key == "" {
			h.handleError(w, r, err, options)
			return
		}

		// Check rate limit
		result := h.checkLimit(r.Context(), limiter, key)

		// Format headers
		h.formatHeaders(w, result, options)

		if !result.Allowed {
			h.handleExceeded(w, r, result, options)
			return
		}

		// Proceed to handler
		handler.ServeHTTP(w, r)
	})
}

// WrapFunc wraps an http.HandlerFunc with rate limiting
func (h *httpMiddleware) WrapFunc(handlerFunc http.HandlerFunc, limiter interface{}, options *middleware.HTTPOptions) http.HandlerFunc {
	wrapped := h.Wrap(handlerFunc, limiter, options)
	return wrapped.ServeHTTP
}

// GetKeyExtractor returns the key extractor
func (h *httpMiddleware) GetKeyExtractor() middleware.KeyExtractor {
	return h.keyExtractor
}

// shouldCheckRateLimit determines if rate limiting should be applied
func (h *httpMiddleware) shouldCheckRateLimit(r *http.Request, options *middleware.HTTPOptions) bool {
	if options == nil {
		return true
	}

	// Check skip paths
	if len(options.SkipPaths) > 0 {
		for _, path := range options.SkipPaths {
			if r.URL.Path == path {
				return false
			}
		}
	}

	// Check skip methods
	if len(options.SkipMethods) > 0 {
		for _, method := range options.SkipMethods {
			if r.Method == method {
				return false
			}
		}
	}

	return true
}

// getExtractor returns the appropriate key extractor
func (h *httpMiddleware) getExtractor(options *middleware.HTTPOptions) middleware.KeyExtractor {
	if options != nil {
		// Custom extractor
		if options.Extractor != nil {
			return options.Extractor
		}

		// Custom key header
		if options.CustomKeyHeader != "" {
			return func(r *http.Request) (string, error) {
				key := r.Header.Get(options.CustomKeyHeader)
				if key == "" {
					return "", errors.New("missing custom key header")
				}
				return key, nil
			}
		}
	}

	return h.keyExtractor
}

// checkLimit checks the rate limit using the limiter
func (h *httpMiddleware) checkLimit(ctx context.Context, limiter interface{}, key string) *limiters.LimitResult {
	// Type assert the limiter
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

// formatHeaders formats and sets rate limit headers
func (h *httpMiddleware) formatHeaders(w http.ResponseWriter, result *limiters.LimitResult, options *middleware.HTTPOptions) {
	if options != nil && options.HeaderFormatter != nil {
		options.HeaderFormatter.FormatHeaders(w, result)
		return
	}

	// Default header formatting
	formatter := NewDefaultHeaderFormatter()
	formatter.FormatHeaders(w, result)
}

// handleExceeded handles rate limit exceeded
func (h *httpMiddleware) handleExceeded(w http.ResponseWriter, r *http.Request, result *limiters.LimitResult, options *middleware.HTTPOptions) {
	if options != nil && options.Handler != nil {
		options.Handler.HandleExceeded(w, r, result)
		return
	}

	// Default handler
	handler := NewDefaultRateLimitHandler()
	handler.HandleExceeded(w, r, result)
}

// handleError handles errors during rate limiting
func (h *httpMiddleware) handleError(w http.ResponseWriter, r *http.Request, err error, options *middleware.HTTPOptions) {
	if err == nil {
		err = errors.New("invalid key")
	}

	if options != nil && options.Handler != nil {
		options.Handler.HandleError(w, r, err)
		return
	}

	// Default error handler
	handler := NewDefaultRateLimitHandler()
	handler.HandleError(w, r, err)
}

// defaultKeyExtractor is the default key extraction function
func defaultKeyExtractor(r *http.Request) (string, error) {
	// Try X-API-Key header first
	if key := r.Header.Get("X-API-Key"); key != "" {
		return key, nil
	}

	// Try Authorization header
	if auth := r.Header.Get("Authorization"); auth != "" {
		return auth, nil
	}

	// Fall back to remote address
	if r.RemoteAddr != "" {
		return r.RemoteAddr, nil
	}

	return "", errors.New("no key found")
}
