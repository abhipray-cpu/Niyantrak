# HTTP Middleware

The HTTP middleware package provides rate limiting capabilities for HTTP handlers in Go applications. It implements the `middleware.HTTPMiddleware` interface defined in the parent package.

## Features

- ✅ **Standard Library Only** - Zero external dependencies
- ✅ **Interface Compliant** - Strictly follows `middleware.HTTPMiddleware` interface
- ✅ **Flexible Key Extraction** - Multiple strategies for extracting rate limit keys
- ✅ **Customizable Handlers** - Custom handling for rate limit exceeded and errors
- ✅ **Header Formatting** - Configurable rate limit headers
- ✅ **Path & Method Skipping** - Skip rate limiting for specific paths or methods
- ✅ **Thread-Safe** - No race conditions
- ✅ **High Performance** - ~355ns per operation

## Installation

```go
import (
    "github.com/abhipray-cpu/niyantrak/middleware/http"
    "github.com/abhipray-cpu/niyantrak/limiters"
)
```

## Quick Start

### Basic Usage

```go
package main

import (
    "context"
    "net/http"
    "time"
    
    httpMiddleware "github.com/abhipray-cpu/niyantrak/middleware/http"
    "github.com/abhipray-cpu/niyantrak/limiters"
    "github.com/abhipray-cpu/niyantrak/middleware"
)

func main() {
    // Create a basic limiter (10 requests per minute)
    limiter := limiters.NewBasicLimiter(10, time.Minute, nil)
    
    // Create middleware
    mw := httpMiddleware.New()
    
    // Define your handler
    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Write([]byte("Hello, World!"))
    })
    
    // Wrap with rate limiting
    options := &middleware.HTTPOptions{
        Extractor: func(r *http.Request) (string, error) {
            return r.Header.Get("X-API-Key"), nil
        },
    }
    
    wrapped := mw.Wrap(handler, limiter, options)
    
    http.ListenAndServe(":8080", wrapped)
}
```

## Configuration Options

### HTTPOptions

The `HTTPOptions` struct provides comprehensive configuration:

```go
type HTTPOptions struct {
    // Extractor extracts the rate limit key from the request
    Extractor KeyExtractor
    
    // Handler handles rate limit exceeded and error scenarios
    Handler RateLimitHandler
    
    // HeaderFormatter formats rate limit headers
    HeaderFormatter HeaderFormatter
    
    // SkipPaths lists paths that should skip rate limiting
    SkipPaths []string
    
    // SkipMethods lists HTTP methods that should skip rate limiting
    SkipMethods []string
    
    // CustomKeyHeader specifies a custom header for key extraction
    CustomKeyHeader string
    
    // EnableDetailedHeaders enables detailed rate limit headers
    EnableDetailedHeaders bool
}
```

## Key Extraction Strategies

### 1. Default Key Extractor

The default extractor tries multiple strategies:

```go
mw := httpMiddleware.New()
extractor := mw.GetKeyExtractor()
// Tries: X-API-Key → Authorization → RemoteAddr
```

### 2. Custom Header

```go
options := &middleware.HTTPOptions{
    CustomKeyHeader: "X-Client-ID",
}
```

### 3. Custom Extractor Function

```go
options := &middleware.HTTPOptions{
    Extractor: func(r *http.Request) (string, error) {
        // Extract from query parameter
        apiKey := r.URL.Query().Get("api_key")
        if apiKey == "" {
            return "", errors.New("missing api_key")
        }
        return apiKey, nil
    },
}
```

### 4. IP-Based Rate Limiting

```go
options := &middleware.HTTPOptions{
    Extractor: func(r *http.Request) (string, error) {
        ip := r.RemoteAddr
        // Remove port if present
        if idx := strings.LastIndex(ip, ":"); idx != -1 {
            ip = ip[:idx]
        }
        return ip, nil
    },
}
```

## Path & Method Skipping

### Skip Specific Paths

```go
options := &middleware.HTTPOptions{
    SkipPaths: []string{"/health", "/metrics", "/favicon.ico"},
    Extractor: keyExtractor,
}
```

### Skip Specific Methods

```go
options := &middleware.HTTPOptions{
    SkipMethods: []string{"GET", "HEAD", "OPTIONS"},
    Extractor: keyExtractor,
}
```

## Custom Handlers

### Custom Rate Limit Exceeded Handler

```go
type customHandler struct{}

func (h *customHandler) HandleExceeded(w http.ResponseWriter, r *http.Request, result interface{}) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusTooManyRequests)
    json.NewEncoder(w).Encode(map[string]string{
        "error": "Too many requests",
        "retry_after": "60",
    })
}

func (h *customHandler) HandleError(w http.ResponseWriter, r *http.Request, err error) {
    w.WriteHeader(http.StatusInternalServerError)
    w.Write([]byte("Internal error"))
}

// Use it
options := &middleware.HTTPOptions{
    Handler: &customHandler{},
    Extractor: keyExtractor,
}
```

## Header Formatting

### Default Headers

The default formatter sets standard rate limit headers:

- `X-RateLimit-Limit`: Total requests allowed
- `X-RateLimit-Remaining`: Remaining requests
- `X-RateLimit-Reset`: Unix timestamp when limit resets
- `Retry-After`: Seconds to wait before retrying (when rate limited)

### Custom Headers

```go
customFormatter := httpMiddleware.NewCustomHeaderFormatter(&middleware.HeaderNames{
    Limit:      "X-Custom-Limit",
    Remaining:  "X-Custom-Remaining",
    Reset:      "X-Custom-Reset",
    RetryAfter: "X-Custom-Retry",
})

options := &middleware.HTTPOptions{
    HeaderFormatter: customFormatter,
    Extractor: keyExtractor,
}
```

## Complete Example

```go
package main

import (
    "context"
    "fmt"
    "net/http"
    "time"
    
    httpMiddleware "github.com/abhipray-cpu/niyantrak/middleware/http"
    "github.com/abhipray-cpu/niyantrak/limiters"
    "github.com/abhipray-cpu/niyantrak/middleware"
)

func main() {
    // Create limiter (100 requests per minute)
    limiter := limiters.NewBasicLimiter(100, time.Minute, nil)
    defer limiter.Close()
    
    // Create middleware
    mw := httpMiddleware.New()
    
    // Configure options
    options := &middleware.HTTPOptions{
        // Extract key from X-API-Key header
        Extractor: func(r *http.Request) (string, error) {
            key := r.Header.Get("X-API-Key")
            if key == "" {
                return "", fmt.Errorf("missing X-API-Key header")
            }
            return key, nil
        },
        
        // Skip health check and metrics endpoints
        SkipPaths: []string{"/health", "/metrics"},
        
        // Skip OPTIONS requests (CORS preflight)
        SkipMethods: []string{"OPTIONS"},
        
        // Use default handler and formatter
        Handler: httpMiddleware.NewDefaultRateLimitHandler(),
        HeaderFormatter: httpMiddleware.NewDefaultHeaderFormatter(),
    }
    
    // Define API handler
    apiHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.Write([]byte(`{"message": "API request successful"}`))
    })
    
    // Wrap with rate limiting
    rateLimitedHandler := mw.Wrap(apiHandler, limiter, options)
    
    // Setup routes
    http.Handle("/api/", rateLimitedHandler)
    http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        w.Write([]byte("OK"))
    })
    
    fmt.Println("Server starting on :8080")
    http.ListenAndServe(":8080", nil)
}
```

## Testing Example

```go
func TestRateLimiting(t *testing.T) {
    limiter := limiters.NewBasicLimiter(10, time.Minute, nil)
    defer limiter.Close()
    
    mw := httpMiddleware.New()
    
    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
    })
    
    options := &middleware.HTTPOptions{
        Extractor: func(r *http.Request) (string, error) {
            return "test-key", nil
        },
    }
    
    wrapped := mw.Wrap(handler, limiter, options)
    
    // Make 10 allowed requests
    for i := 0; i < 10; i++ {
        req := httptest.NewRequest(http.MethodGet, "/test", nil)
        rec := httptest.NewRecorder()
        wrapped.ServeHTTP(rec, req)
        
        if rec.Code != http.StatusOK {
            t.Errorf("Request %d: expected 200, got %d", i+1, rec.Code)
        }
    }
    
    // 11th request should be rate limited
    req := httptest.NewRequest(http.MethodGet, "/test", nil)
    rec := httptest.NewRecorder()
    wrapped.ServeHTTP(rec, req)
    
    if rec.Code != http.StatusTooManyRequests {
        t.Errorf("Expected 429, got %d", rec.Code)
    }
}
```

## Performance

Benchmark results on Apple M4:

```
BenchmarkHTTPMiddleware-10    3391791    355.1 ns/op    1024 B/op    11 allocs/op
```

- **355 ns/op**: Very fast processing
- **1024 B/op**: Minimal memory allocation
- **11 allocs/op**: Low allocation count

## Thread Safety

The middleware is thread-safe and has been tested with the race detector:

```bash
go test ./middleware/http/... -race
```

## Coverage

Current test coverage: **63.5%**

```bash
go test ./middleware/http/... -cover
```

## API Reference

### Functions

#### `New() middleware.HTTPMiddleware`

Creates a new HTTP middleware instance.

### Methods

#### `Wrap(handler http.Handler, limiter interface{}, options *HTTPOptions) http.Handler`

Wraps an http.Handler with rate limiting.

**Parameters:**
- `handler`: The HTTP handler to wrap
- `limiter`: A rate limiter implementing the Allow method
- `options`: Configuration options

**Returns:** Wrapped http.Handler

#### `WrapFunc(handlerFunc http.HandlerFunc, limiter interface{}, options *HTTPOptions) http.HandlerFunc`

Wraps an http.HandlerFunc with rate limiting.

**Parameters:**
- `handlerFunc`: The HTTP handler function to wrap
- `limiter`: A rate limiter implementing the Allow method
- `options`: Configuration options

**Returns:** Wrapped http.HandlerFunc

#### `GetKeyExtractor() KeyExtractor`

Returns the default key extractor function.

**Returns:** Default KeyExtractor function

### Factory Functions

#### `NewDefaultRateLimitHandler() middleware.RateLimitHandler`

Creates the default rate limit handler.

#### `NewDefaultHeaderFormatter() middleware.HeaderFormatter`

Creates the default header formatter.

#### `NewCustomHeaderFormatter(names *HeaderNames) middleware.HeaderFormatter`

Creates a header formatter with custom header names.

## License

This package is part of the Niyantrak rate limiting library.
