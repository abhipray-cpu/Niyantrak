# Framework Adapters

This package provides rate limiting adapters for popular Go web frameworks. Choose the adapter that matches your framework for seamless integration.

## Supported Frameworks

| Framework | Adapter | Performance | Coverage | Status |
|-----------|---------|-------------|----------|--------|
| [Gin](https://github.com/gin-gonic/gin) | `GinRateLimiter` | ~392 ns/op | 80.4% | ✅ Stable |
| [Echo](https://github.com/labstack/echo) | `EchoRateLimiter` | ~397 ns/op | 73.3% | ✅ Stable |
| [Fiber](https://github.com/gofiber/fiber) | `FiberRateLimiter` | ~3520 ns/op | 73.3% | ✅ Stable |
| [Chi](https://github.com/go-chi/chi) | `ChiRateLimiter` | ~444 ns/op | 73.3% | ✅ Stable |

## Choosing a Framework

- **Gin** - Most popular, great documentation, good performance
- **Echo** - Lightweight, fast, minimalist design
- **Fiber** - Express.js inspired, fastest router (but higher middleware overhead)
- **Chi** - Idiomatic Go, stdlib-based, lightweight

---

# Gin Framework Adapter

The Gin adapter provides seamless rate limiting integration for [Gin](https://github.com/gin-gonic/gin) web framework applications.

## Features

- ✅ **Native Gin Integration** - Works naturally with Gin middleware chain
- ✅ **Flexible Key Extraction** - Extract from headers, client IP, or custom logic
- ✅ **Path Skipping** - Exclude specific paths from rate limiting
- ✅ **Custom Handlers** - Override default rate limit and error handling
- ✅ **Header Control** - Enable/disable rate limit headers
- ✅ **High Performance** - ~377ns per operation
- ✅ **Thread-Safe** - Race-condition free
- ✅ **80.4% Test Coverage**

## Installation

```bash
go get github.com/gin-gonic/gin
```

```go
import (
    "github.com/gin-gonic/gin"
    "github.com/abhipray-cpu/niyantrak/limiters"
    "github.com/abhipray-cpu/niyantrak/middleware/adapters"
)
```

## Quick Start

### Basic Usage

```go
package main

import (
    "time"
    
    "github.com/gin-gonic/gin"
    "github.com/abhipray-cpu/niyantrak/limiters"
    "github.com/abhipray-cpu/niyantrak/middleware/adapters"
)

func main() {
    // Create rate limiter (100 requests per minute)
    limiter := limiters.NewBasicLimiter(100, time.Minute, nil)
    defer limiter.Close()

    // Create Gin router
    router := gin.Default()

    // Add rate limiting middleware
    router.Use(adapters.GinRateLimiter(limiter, adapters.GinOptions{}))

    // Define routes
    router.GET("/api/users", func(c *gin.Context) {
        c.JSON(200, gin.H{"message": "Hello, World!"})
    })

    router.Run(":8080")
}
```

## Configuration Options

### GinOptions

```go
type GinOptions struct {
    // KeyExtractor extracts the rate limit key from the Gin context
    // Default: X-API-Key header → Authorization header → Client IP
    KeyExtractor func(*gin.Context) string

    // OnRateLimitExceeded is called when rate limit is exceeded
    // Default: Returns 429 with JSON error
    OnRateLimitExceeded func(*gin.Context)

    // OnError is called when an error occurs during rate limiting
    // Default: Returns 400 with JSON error (if AbortOnError is true)
    OnError func(*gin.Context, error)

    // SkipPaths contains paths that should skip rate limiting
    SkipPaths []string

    // IncludeHeaders determines if rate limit headers should be included
    // Default: true (set to &false to disable)
    IncludeHeaders *bool

    // AbortOnError determines if the request should be aborted on error
    // Default: false (continues to handler)
    AbortOnError bool
}
```

## Examples

### API Key-Based Rate Limiting

```go
limiter := limiters.NewBasicLimiter(100, time.Minute, nil)

router := gin.Default()
router.Use(adapters.GinRateLimiter(limiter, adapters.GinOptions{
    KeyExtractor: func(c *gin.Context) string {
        return c.GetHeader("X-API-Key")
    },
}))

router.GET("/api/data", handler)
```

### IP-Based Rate Limiting

```go
router.Use(adapters.GinRateLimiter(limiter, adapters.GinOptions{
    KeyExtractor: func(c *gin.Context) string {
        return c.ClientIP()
    },
}))
```

### User-Based Rate Limiting with JWT

```go
router.Use(adapters.GinRateLimiter(limiter, adapters.GinOptions{
    KeyExtractor: func(c *gin.Context) string {
        // Extract user ID from JWT token
        token := c.GetHeader("Authorization")
        userID := extractUserFromJWT(token)
        return userID
    },
}))
```

### Skip Specific Paths

```go
router.Use(adapters.GinRateLimiter(limiter, adapters.GinOptions{
    SkipPaths: []string{
        "/health",
        "/metrics",
        "/favicon.ico",
    },
}))
```

### Custom Error Handler

```go
router.Use(adapters.GinRateLimiter(limiter, adapters.GinOptions{
    OnRateLimitExceeded: func(c *gin.Context) {
        c.JSON(429, gin.H{
            "error": "Too many requests",
            "retry_after": "60s",
        })
        c.Abort()
    },
}))
```

### Disable Rate Limit Headers

```go
falseVal := false
router.Use(adapters.GinRateLimiter(limiter, adapters.GinOptions{
    IncludeHeaders: &falseVal,
}))
```

### Abort on Key Extraction Error

```go
router.Use(adapters.GinRateLimiter(limiter, adapters.GinOptions{
    KeyExtractor: func(c *gin.Context) string {
        apiKey := c.GetHeader("X-API-Key")
        if apiKey == "" {
            return "" // Will trigger error
        }
        return apiKey
    },
    AbortOnError: true,
    OnError: func(c *gin.Context, err error) {
        c.JSON(401, gin.H{
            "error": "Missing API key",
        })
    },
}))
```

## Complete Example with Multiple Features

```go
package main

import (
    "strings"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/abhipray-cpu/niyantrak/limiters"
    "github.com/abhipray-cpu/niyantrak/middleware/adapters"
)

func main() {
    // Create rate limiter
    limiter := limiters.NewBasicLimiter(100, time.Minute, nil)
    defer limiter.Close()

    // Create router
    router := gin.Default()

    // Add rate limiting with configuration
    router.Use(adapters.GinRateLimiter(limiter, adapters.GinOptions{
        // Extract key from Authorization header or fall back to IP
        KeyExtractor: func(c *gin.Context) string {
            if auth := c.GetHeader("Authorization"); auth != "" {
                // Remove "Bearer " prefix
                return strings.TrimPrefix(auth, "Bearer ")
            }
            return c.ClientIP()
        },

        // Skip health and metrics endpoints
        SkipPaths: []string{
            "/health",
            "/metrics",
        },

        // Custom rate limit exceeded handler
        OnRateLimitExceeded: func(c *gin.Context) {
            retryAfter := c.GetHeader("Retry-After")
            c.JSON(429, gin.H{
                "error":       "rate_limit_exceeded",
                "message":     "Too many requests. Please slow down.",
                "retry_after": retryAfter,
            })
            c.Abort()
        },

        // Abort on errors
        AbortOnError: true,
        OnError: func(c *gin.Context, err error) {
            c.JSON(400, gin.H{
                "error":   "rate_limit_error",
                "message": err.Error(),
            })
        },
    }))

    // Define routes
    router.GET("/health", func(c *gin.Context) {
        c.JSON(200, gin.H{"status": "ok"})
    })

    router.GET("/api/users", func(c *gin.Context) {
        c.JSON(200, gin.H{
            "users": []string{"Alice", "Bob", "Charlie"},
        })
    })

    router.POST("/api/users", func(c *gin.Context) {
        c.JSON(201, gin.H{"message": "User created"})
    })

    // Start server
    router.Run(":8080")
}
```

## Rate Limit Headers

When enabled (default), the middleware sets the following headers:

- `X-RateLimit-Limit`: Total requests allowed in the window
- `X-RateLimit-Remaining`: Remaining requests in current window
- `X-RateLimit-Reset`: Unix timestamp when the limit resets
- `Retry-After`: Seconds to wait before retrying (when rate limited)

## Response Example

### Successful Request

```http
HTTP/1.1 200 OK
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 95
X-RateLimit-Reset: 1737639600
Content-Type: application/json

{"message": "Success"}
```

### Rate Limited Request

```http
HTTP/1.1 429 Too Many Requests
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 0
X-RateLimit-Reset: 1737639600
Retry-After: 45
Content-Type: application/json

{
  "error": "rate limit exceeded",
  "message": "Too many requests. Limit: 100 requests"
}
```

## Performance

Benchmark results on Apple M4:

```
BenchmarkGinRateLimiter-10    3130927    376.6 ns/op    1024 B/op    11 allocs/op
```

- **376.6 ns/op**: Fast middleware processing
- **1024 B/op**: Minimal memory allocation
- **11 allocs/op**: Low allocation count

## Thread Safety

The adapter is thread-safe and has been tested with the race detector:

```bash
go test ./middleware/adapters/... -race
```

## Testing

### Test Coverage

Current test coverage: **80.4%**

```bash
go test ./middleware/adapters/... -cover
```

### Example Test

```go
func TestRateLimiting(t *testing.T) {
    limiter := limiters.NewBasicLimiter(10, time.Minute, nil)
    defer limiter.Close()

    router := gin.New()
    router.Use(adapters.GinRateLimiter(limiter, adapters.GinOptions{
        KeyExtractor: func(c *gin.Context) string {
            return "test-key"
        },
    }))
    router.GET("/test", func(c *gin.Context) {
        c.JSON(200, gin.H{"message": "success"})
    })

    // Make 10 successful requests
    for i := 0; i < 10; i++ {
        req := httptest.NewRequest("GET", "/test", nil)
        rec := httptest.NewRecorder()
        router.ServeHTTP(rec, req)

        if rec.Code != 200 {
            t.Errorf("Request %d: expected 200, got %d", i+1, rec.Code)
        }
    }

    // 11th request should be rate limited
    req := httptest.NewRequest("GET", "/test", nil)
    rec := httptest.NewRecorder()
    router.ServeHTTP(rec, req)

    if rec.Code != 429 {
        t.Errorf("Expected 429, got %d", rec.Code)
    }
}
```

## Integration with Other Middleware

The Gin adapter works seamlessly with other Gin middleware:

```go
router := gin.Default()

// Add middleware in order
router.Use(gin.Logger())                          // Logging
router.Use(gin.Recovery())                        // Panic recovery
router.Use(corsMiddleware())                      // CORS
router.Use(authMiddleware())                      // Authentication
router.Use(adapters.GinRateLimiter(limiter, opts)) // Rate limiting
```

## Common Patterns

### Per-Route Rate Limiting

```go
// Global rate limit
router.Use(adapters.GinRateLimiter(globalLimiter, globalOpts))

// Stricter limit for specific route
strictLimiter := limiters.NewBasicLimiter(10, time.Minute, nil)
api := router.Group("/api")
api.Use(adapters.GinRateLimiter(strictLimiter, strictOpts))
{
    api.GET("/expensive", handler)
}
```

### Different Limits for Different Users

```go
premiumLimiter := limiters.NewBasicLimiter(1000, time.Minute, nil)
freeLimiter := limiters.NewBasicLimiter(100, time.Minute, nil)

router.Use(func(c *gin.Context) {
    userTier := getUserTier(c) // Get from JWT, database, etc.
    
    var limiter interface{}
    if userTier == "premium" {
        limiter = premiumLimiter
    } else {
        limiter = freeLimiter
    }
    
    middleware := adapters.GinRateLimiter(limiter, adapters.GinOptions{
        KeyExtractor: func(c *gin.Context) string {
            return getUserID(c)
        },
    })
    
    middleware(c)
})
```

## Best Practices

1. **Choose the right key**: Use API keys for B2B, user IDs for authenticated users, IPs for public endpoints
2. **Skip health checks**: Always exclude `/health`, `/metrics` from rate limiting
3. **Set appropriate limits**: Start conservative and adjust based on metrics
4. **Include headers**: Help clients implement proper backoff strategies
5. **Handle errors gracefully**: Don't break the application on rate limiter errors
6. **Monitor rate limits**: Track how often limits are hit
7. **Use tiered limiting**: Different limits for different user tiers
8. **Consider burst traffic**: Use algorithms that allow bursts (Token Bucket)

## Troubleshooting

### Headers Not Appearing

Ensure `IncludeHeaders` is not explicitly set to false:

```go
falseVal := false
options := adapters.GinOptions{
    IncludeHeaders: &falseVal, // This disables headers
}
```

### Rate Limiting Not Working

1. Check if the path is in `SkipPaths`
2. Verify the key extractor returns a non-empty string
3. Ensure the limiter is properly initialized

### All Requests Getting Rate Limited

- Check if you're using the same key for all requests
- Verify the limiter's limit and window are correct
- Ensure the backend storage is working (Redis, PostgreSQL)

---

# Echo Framework Adapter

The Echo adapter provides seamless rate limiting integration for [Echo](https://github.com/labstack/echo) web framework applications.

## Features

- ✅ **Native Echo Integration** - Works naturally with Echo middleware chain
- ✅ **High Performance** - ~397ns per operation
- ✅ **Automatic IP Detection** - Uses `c.RealIP()` for accurate client identification
- ✅ **Thread-Safe** - Race-condition free

## Quick Start

```go
package main

import (
    "time"
    
    "github.com/labstack/echo/v4"
    "github.com/abhipray-cpu/niyantrak/limiters"
    "github.com/abhipray-cpu/niyantrak/middleware/adapters"
)

func main() {
    limiter := limiters.NewBasicLimiter(100, time.Minute, nil)
    defer limiter.Close()

    e := echo.New()
    e.Use(adapters.EchoRateLimiter(limiter, adapters.EchoOptions{}))

    e.GET("/api/users", func(c echo.Context) error {
        return c.JSON(200, map[string]string{"message": "Hello, World!"})
    })

    e.Start(":8080")
}
```

## Configuration

```go
type EchoOptions struct {
    KeyExtractor        func(echo.Context) string
    OnRateLimitExceeded func(echo.Context) error
    OnError             func(echo.Context, error) error
    SkipPaths           []string
    IncludeHeaders      *bool
    AbortOnError        bool
}
```

## Examples

### API Key-Based Rate Limiting

```go
e.Use(adapters.EchoRateLimiter(limiter, adapters.EchoOptions{
    KeyExtractor: func(c echo.Context) string {
        return c.Request().Header.Get("X-API-Key")
    },
}))
```

### Skip Health Endpoints

```go
e.Use(adapters.EchoRateLimiter(limiter, adapters.EchoOptions{
    SkipPaths: []string{"/health", "/metrics"},
}))
```

### Custom Rate Limit Handler

```go
e.Use(adapters.EchoRateLimiter(limiter, adapters.EchoOptions{
    OnRateLimitExceeded: func(c echo.Context) error {
        return c.JSON(429, map[string]string{
            "error": "Rate limit exceeded",
        })
    },
}))
```

---

# Fiber Framework Adapter

The Fiber adapter provides seamless rate limiting integration for [Fiber](https://github.com/gofiber/fiber) v2 applications.

## Features

- ✅ **Fiber v2 Compatible** - Works with latest Fiber version
- ✅ **Express.js Style** - Familiar API for Express users
- ✅ **Fast Router** - Leverages Fiber's fast routing (note: middleware overhead ~3520ns)

## Quick Start

```go
package main

import (
    "time"
    
    "github.com/gofiber/fiber/v2"
    "github.com/abhipray-cpu/niyantrak/limiters"
    "github.com/abhipray-cpu/niyantrak/middleware/adapters"
)

func main() {
    limiter := limiters.NewBasicLimiter(100, time.Minute, nil)
    defer limiter.Close()

    app := fiber.New()
    app.Use(adapters.FiberRateLimiter(limiter, adapters.FiberOptions{}))

    app.Get("/api/users", func(c *fiber.Ctx) error {
        return c.JSON(fiber.Map{"message": "Hello, World!"})
    })

    app.Listen(":8080")
}
```

## Configuration

```go
type FiberOptions struct {
    KeyExtractor        func(*fiber.Ctx) string
    OnRateLimitExceeded func(*fiber.Ctx) error
    OnError             func(*fiber.Ctx, error) error
    SkipPaths           []string
    IncludeHeaders      *bool
    AbortOnError        bool
}
```

## Examples

### User-Based Rate Limiting

```go
app.Use(adapters.FiberRateLimiter(limiter, adapters.FiberOptions{
    KeyExtractor: func(c *fiber.Ctx) string {
        return c.Get("X-User-ID")
    },
}))
```

### Skip Static Assets

```go
app.Use(adapters.FiberRateLimiter(limiter, adapters.FiberOptions{
    SkipPaths: []string{"/static", "/assets"},
}))
```

### Custom Error Response

```go
app.Use(adapters.FiberRateLimiter(limiter, adapters.FiberOptions{
    OnError: func(c *fiber.Ctx, err error) error {
        return c.Status(400).JSON(fiber.Map{
            "error": err.Error(),
        })
    },
    AbortOnError: true,
}))
```

---

# Chi Framework Adapter

The Chi adapter provides seamless rate limiting integration for [Chi](https://github.com/go-chi/chi) v5 router.

## Features

- ✅ **Idiomatic Go** - Uses standard library patterns
- ✅ **Lightweight** - Minimal dependencies, fast performance (~444ns/op)
- ✅ **Context-Aware** - Full context.Context support
- ✅ **Stdlib Compatible** - Works with standard http.Handler

## Quick Start

```go
package main

import (
    "net/http"
    "time"
    
    "github.com/go-chi/chi/v5"
    "github.com/abhipray-cpu/niyantrak/limiters"
    "github.com/abhipray-cpu/niyantrak/middleware/adapters"
)

func main() {
    limiter := limiters.NewBasicLimiter(100, time.Minute, nil)
    defer limiter.Close()

    r := chi.NewRouter()
    r.Use(adapters.ChiRateLimiter(limiter, adapters.ChiOptions{}))

    r.Get("/api/users", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte(`{"message":"Hello, World!"}`))
    })

    http.ListenAndServe(":8080", r)
}
```

## Configuration

```go
type ChiOptions struct {
    KeyExtractor        func(*http.Request) string
    OnRateLimitExceeded func(http.ResponseWriter, *http.Request)
    OnError             func(http.ResponseWriter, *http.Request, error)
    SkipPaths           []string
    IncludeHeaders      *bool
    AbortOnError        bool
}
```

## Examples

### IP-Based Rate Limiting

```go
r.Use(adapters.ChiRateLimiter(limiter, adapters.ChiOptions{
    KeyExtractor: func(req *http.Request) string {
        return req.RemoteAddr
    },
}))
```

### Skip Admin Routes

```go
r.Use(adapters.ChiRateLimiter(limiter, adapters.ChiOptions{
    SkipPaths: []string{"/admin", "/internal"},
}))
```

### Custom Rate Limit Handler

```go
r.Use(adapters.ChiRateLimiter(limiter, adapters.ChiOptions{
    OnRateLimitExceeded: func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusTooManyRequests)
        w.Write([]byte(`{"error":"Rate limit exceeded"}`))
    },
}))
```

### Route Groups with Different Limits

```go
// Public routes - stricter limit
r.Group(func(r chi.Router) {
    publicLimiter := limiters.NewBasicLimiter(10, time.Minute, nil)
    r.Use(adapters.ChiRateLimiter(publicLimiter, adapters.ChiOptions{}))
    r.Get("/public/data", handler)
})

// Authenticated routes - higher limit
r.Group(func(r chi.Router) {
    authLimiter := limiters.NewBasicLimiter(100, time.Minute, nil)
    r.Use(authMiddleware)
    r.Use(adapters.ChiRateLimiter(authLimiter, adapters.ChiOptions{
        KeyExtractor: func(req *http.Request) string {
            return req.Header.Get("X-User-ID")
        },
    }))
    r.Get("/api/data", handler)
})
```

---

## Performance Comparison

All adapters tested on Apple M4:

| Adapter | ns/op | B/op | allocs/op | Notes |
|---------|-------|------|-----------|-------|
| **Gin** | 392.3 | 1024 | 11 | Most popular, well-documented |
| **Echo** | 397.0 | 1120 | 12 | Lightweight, minimal |
| **Chi** | 443.7 | 1392 | 13 | Stdlib-based, idiomatic |
| **Fiber** | 3520 | 6013 | 27 | Higher overhead due to fasthttp |

**Note**: All adapters provide excellent performance for most use cases. Choose based on your framework preference rather than micro-benchmarks.

## Common Patterns

### Multi-Tier Rate Limiting

```go
// Apply different limits based on user tier
func rateLimiterMiddleware(c YourContext) {
    tier := getUserTier(c)
    
    var limiter interface{}
    switch tier {
    case "premium":
        limiter = premiumLimiter // 1000/min
    case "standard":
        limiter = standardLimiter // 100/min
    default:
        limiter = freeLimiter // 10/min
    }
    
    // Use appropriate adapter for your framework
    middleware := adapters.YourFrameworkRateLimiter(limiter, options)
    middleware(c)
}
```

### Combine with Authentication

```go
// Apply rate limiting after authentication
router.Use(authMiddleware())
router.Use(adapters.YourFrameworkRateLimiter(limiter, options{
    KeyExtractor: func(c Context) string {
        return getUserID(c) // Get authenticated user ID
    },
}))
```

## Best Practices

1. **Always skip health checks** - Don't rate limit `/health`, `/metrics`, `/ready`
2. **Use appropriate keys** - API keys for partners, user IDs for users, IPs for anonymous
3. **Include rate limit headers** - Help clients implement proper backoff
4. **Handle errors gracefully** - Don't break requests on rate limiter errors
5. **Monitor limits** - Track how often limits are hit
6. **Set realistic limits** - Based on your infrastructure capacity
7. **Use burst-friendly algorithms** - Token Bucket or Sliding Window
8. **Consider geographic distribution** - Different limits for different regions

## Troubleshooting

### Issue: Rate limit headers not appearing
**Solution**: Ensure `IncludeHeaders` is not explicitly set to `false`

### Issue: All requests getting rate limited
**Solution**: Check if all requests use the same key (e.g., all extracting same header)

### Issue: Rate limiting not working
**Solution**: 
- Verify path is not in `SkipPaths`
- Ensure key extractor returns non-empty string
- Check limiter initialization

### Issue: Different behavior across frameworks
**Solution**: All adapters follow the same pattern - check key extraction logic specific to each framework's context

## License

These adapters are part of the Niyantrak rate limiting library.
