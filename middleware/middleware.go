package middleware

import (
	"context"
	"net/http"
)

// HTTPMiddleware provides HTTP integration for rate limiting
type HTTPMiddleware interface {
	// Wrap wraps an http.Handler with rate limiting
	Wrap(handler http.Handler, limiter interface{}, options *HTTPOptions) http.Handler

	// WrapFunc wraps an http.HandlerFunc with rate limiting
	WrapFunc(handler http.HandlerFunc, limiter interface{}, options *HTTPOptions) http.HandlerFunc

	// GetKeyExtractor returns the default key extractor
	GetKeyExtractor() KeyExtractor
}

// KeyExtractor extracts a rate limit key from an HTTP request
type KeyExtractor func(*http.Request) (string, error)

// HTTPOptions contains HTTP middleware options
type HTTPOptions struct {
	// Extractor extracts rate limit key from request
	Extractor KeyExtractor

	// Handler handles rate limit exceeded responses
	Handler RateLimitHandler

	// HeaderFormatter formats rate limit response headers
	HeaderFormatter HeaderFormatter

	// SkipPaths is list of paths to skip rate limiting
	SkipPaths []string

	// SkipMethods is list of HTTP methods to skip
	SkipMethods []string

	// CustomKeyHeader is header to use as key (X-API-Key, etc.)
	CustomKeyHeader string

	// EnableDetailedHeaders includes detailed rate limit info
	EnableDetailedHeaders bool
}

// RateLimitHandler handles rate limit exceeded responses
type RateLimitHandler interface {
	// HandleExceeded is called when rate limit is exceeded
	HandleExceeded(w http.ResponseWriter, r *http.Request, result interface{})

	// HandleError is called when rate limit check fails
	HandleError(w http.ResponseWriter, r *http.Request, err error)
}

// HeaderFormatter formats rate limit headers for HTTP responses
type HeaderFormatter interface {
	// FormatHeaders adds rate limit headers to the response
	FormatHeaders(w http.ResponseWriter, result interface{})

	// GetHeaderNames returns custom header names to use
	GetHeaderNames() *HeaderNames
}

// HeaderNames specifies custom header names
type HeaderNames struct {
	// Limit header name (default: X-RateLimit-Limit)
	Limit string

	// Remaining header name (default: X-RateLimit-Remaining)
	Remaining string

	// Reset header name (default: X-RateLimit-Reset)
	Reset string

	// RetryAfter header name (default: Retry-After)
	RetryAfter string
}

// GRPCMiddleware provides gRPC integration for rate limiting
type GRPCMiddleware interface {
	// UnaryInterceptor returns a gRPC unary server interceptor
	UnaryInterceptor(limiter interface{}, options *GRPCOptions) interface{}

	// StreamInterceptor returns a gRPC stream server interceptor
	StreamInterceptor(limiter interface{}, options *GRPCOptions) interface{}

	// GetKeyExtractor returns the default key extractor
	GetKeyExtractor() GRPCKeyExtractor
}

// GRPCKeyExtractor extracts a rate limit key from gRPC context
type GRPCKeyExtractor func(context.Context) (string, error)

// GRPCOptions contains gRPC middleware options
type GRPCOptions struct {
	// Extractor extracts rate limit key from gRPC context
	Extractor GRPCKeyExtractor

	// MetadataKey is the key to store rate limit info in gRPC metadata
	MetadataKey string

	// SkipMethods is list of gRPC methods to skip rate limiting
	// Format: "/service.Service/Method"
	SkipMethods []string

	// EnableMetadata includes rate limit info in response metadata
	EnableMetadata bool

	// CustomStatusCode is custom gRPC status code for limit exceeded
	CustomStatusCode string
}

// CustomMiddleware provides a base interface for custom middleware
type CustomMiddleware interface {
	// Apply applies rate limiting to a generic handler
	Apply(handler interface{}, limiter interface{}, options map[string]interface{}) interface{}

	// GetName returns the middleware name
	GetName() string

	// GetSupported returns list of supported integration types
	GetSupported() []string
}

// KeyExtractorBuilder helps build custom key extractors
type KeyExtractorBuilder interface {
	// FromHeader extracts key from HTTP header
	FromHeader(headerName string) KeyExtractor

	// FromQuery extracts key from query parameter
	FromQuery(paramName string) KeyExtractor

	// FromPath extracts key from URL path segment
	FromPath(segment int) KeyExtractor

	// FromBody extracts key from request body (JSON field)
	FromBody(fieldName string) KeyExtractor

	// Composite combines multiple extractors
	Composite(extractors ...KeyExtractor) KeyExtractor

	// WithFallback provides fallback extractor if primary fails
	WithFallback(primary KeyExtractor, fallback KeyExtractor) KeyExtractor
}

// ResponseWriter wraps http.ResponseWriter for rate limiting
type ResponseWriter interface {
	// Write writes response data
	Write(b []byte) (int, error)

	// WriteHeader writes HTTP status code
	WriteHeader(statusCode int)

	// Header returns response headers
	Header() http.Header

	// WithRateLimit adds rate limit information
	WithRateLimit(result interface{}) ResponseWriter
}
