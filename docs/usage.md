# Niyantrak Usage Guide

This guide covers installation, configuration, and usage of the Niyantrak rate limiting library.

---

## Installation

```bash
go get github.com/abhipray-cpu/niyantrak
```

Requires Go 1.19 or later.

---

## Quick Start

The simplest way to use Niyantrak is via the builder API:

```go
package main

import (
    "context"
    "log"
    "time"
    "github.com/abhipray-cpu/niyantrak"
)

func main() {
    limiter, err := niyantrak.New(
        niyantrak.WithAlgorithm(niyantrak.TokenBucket),
        niyantrak.WithTokenBucketConfig(niyantrak.TokenBucketConfig{
            Capacity:     100,
            RefillRate:   100,
            RefillPeriod: time.Minute,
        }),
        niyantrak.WithMemoryBackend(),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer limiter.Close()

    ctx := context.Background()
    result := limiter.Allow(ctx, "user:123")

    if result.Error != nil {
        log.Printf("Error: %v", result.Error)
        return
    }
    if !result.Allowed {
        log.Printf("Rate limited! Retry after %v", result.RetryAfter)
        return
    }
    log.Printf("Allowed. Remaining: %d/%d", result.Remaining, result.Limit)
}
```

---

## Builder Options

`niyantrak.New()` accepts functional options:

| Option | Description | Default |
|--------|-------------|---------|
| `WithAlgorithm(t)` | Select algorithm (`TokenBucket`, `LeakyBucket`, `FixedWindow`, `SlidingWindow`, `GCRA`) | `TokenBucket` |
| `WithTokenBucketConfig(cfg)` | Token Bucket settings | 100 cap, 100/min refill |
| `WithLeakyBucketConfig(cfg)` | Leaky Bucket settings | 100 cap, 100/min leak |
| `WithFixedWindowConfig(cfg)` | Fixed Window settings | 100/min |
| `WithSlidingWindowConfig(cfg)` | Sliding Window settings | 100/min |
| `WithGCRAConfig(cfg)` | GCRA settings | 100/min, burst 10 |
| `WithMemoryBackend()` | In-memory backend | (default if none set) |
| `WithBackend(b)` | Custom/Redis/PostgreSQL backend | - |
| `WithLimit(n)` | Default limit when no explicit config | 100 |
| `WithWindow(d)` | Default time window | 1 minute |
| `WithKeyTTL(d)` | How long to retain state per key | 1 hour |

---

## Algorithms

### Token Bucket

Best for general-purpose rate limiting with burst allowance.

```go
limiter, _ := niyantrak.New(
    niyantrak.WithAlgorithm(niyantrak.TokenBucket),
    niyantrak.WithTokenBucketConfig(niyantrak.TokenBucketConfig{
        Capacity:      100,   // Maximum tokens (burst size)
        RefillRate:    10,    // Tokens added per refill period
        RefillPeriod:  time.Second,
        InitialTokens: 100,  // Start full
    }),
)
```

### Leaky Bucket

Best for smooth, constant-rate traffic.

```go
limiter, _ := niyantrak.New(
    niyantrak.WithAlgorithm(niyantrak.LeakyBucket),
    niyantrak.WithLeakyBucketConfig(niyantrak.LeakyBucketConfig{
        Capacity:   100,
        LeakRate:   10,            // Process 10 per leak period
        LeakPeriod: time.Second,
    }),
)
```

### Fixed Window

Best for simple quota-based limiting (hourly, daily).

```go
limiter, _ := niyantrak.New(
    niyantrak.WithAlgorithm(niyantrak.FixedWindow),
    niyantrak.WithFixedWindowConfig(niyantrak.FixedWindowConfig{
        Limit:  1000,
        Window: time.Hour,
    }),
)
```

### Sliding Window

Best for accurate limiting without boundary spikes.

```go
limiter, _ := niyantrak.New(
    niyantrak.WithAlgorithm(niyantrak.SlidingWindow),
    niyantrak.WithSlidingWindowConfig(niyantrak.SlidingWindowConfig{
        Limit:  100,
        Window: time.Minute,
    }),
)
```

### GCRA

Best for precise, SLA-compliant rate limiting.

```go
limiter, _ := niyantrak.New(
    niyantrak.WithAlgorithm(niyantrak.GCRA),
    niyantrak.WithGCRAConfig(niyantrak.GCRAConfig{
        Limit:     100,
        Period:    time.Minute,
        BurstSize: 10,
    }),
)
```

---

## Backends

### In-Memory

Default backend. Single-instance only, no persistence.

```go
niyantrak.WithMemoryBackend()
```

### Redis

For distributed, multi-instance deployments.

```go
import "github.com/abhipray-cpu/niyantrak/backend/redis"

redisBackend, err := redis.NewRedisBackend(redis.RedisConfig{
    Addr:     "localhost:6379",
    Password: "",
    DB:       0,
})
if err != nil {
    log.Fatal(err)
}

limiter, _ := niyantrak.New(
    niyantrak.WithBackend(redisBackend),
    // ... algorithm options
)
```

### PostgreSQL

For persistent state with audit trail.

```go
import "github.com/abhipray-cpu/niyantrak/backend/postgresql"

pgBackend, err := postgresql.NewPostgreSQLBackend(postgresql.PostgreSQLConfig{
    ConnString: "postgres://user:pass@localhost:5432/ratelimiter?sslmode=disable",
    TableName:  "rate_limits",
})
if err != nil {
    log.Fatal(err)
}

limiter, _ := niyantrak.New(
    niyantrak.WithBackend(pgBackend),
    // ... algorithm options
)
```

---

## Advanced Limiter Types

The builder API creates a `BasicLimiter`. For advanced scenarios, use the sub-packages directly.

### Tier-Based Limiting

Different limits for Free, Premium, Enterprise users.

```go
import (
    "github.com/abhipray-cpu/niyantrak/algorithm"
    "github.com/abhipray-cpu/niyantrak/backend/memory"
    "github.com/abhipray-cpu/niyantrak/limiters"
    "github.com/abhipray-cpu/niyantrak/limiters/tier"
)

backend := memory.NewMemoryBackend()
algo := algorithm.NewTokenBucket(algorithm.TokenBucketConfig{
    Capacity: 1000, RefillRate: 1000, RefillPeriod: time.Minute,
})

limiter, err := tier.NewTierBasedLimiter(algo, backend, limiters.TierConfig{
    Tiers: map[string]limiters.LimitConfig{
        "free":       {Limit: 100,  Window: time.Minute},
        "premium":    {Limit: 1000, Window: time.Minute},
        "enterprise": {Limit: 10000, Window: time.Minute},
    },
    DefaultTier: "free",
})

// Use with tier context
result := limiter.AllowWithTier(ctx, "user:123", "premium")
```

### Tenant-Based Limiting

Isolated rate limits per tenant in a multi-tenant system.

```go
import "github.com/abhipray-cpu/niyantrak/limiters/tenant"

limiter, err := tenant.NewTenantBasedLimiter(algo, backend, limiters.TenantConfig{
    Tenants: map[string]limiters.LimitConfig{
        "tenant-a": {Limit: 5000,  Window: time.Minute},
        "tenant-b": {Limit: 10000, Window: time.Minute},
    },
    DefaultLimit: limiters.LimitConfig{Limit: 100, Window: time.Minute},
})

result := limiter.AllowWithTenant(ctx, "endpoint:/api/users", "tenant-a")
```

### Cost-Based Limiting

Different operations consume different amounts of tokens.

```go
import "github.com/abhipray-cpu/niyantrak/limiters/cost"

limiter, err := cost.NewCostBasedLimiter(algo, backend, limiters.CostConfig{
    DefaultCost: 1,
    Costs: map[string]int{
        "list":   1,
        "get":    1,
        "create": 5,
        "export": 10,
    },
})

// Expensive operation costs 10 tokens
result := limiter.AllowWithCost(ctx, "user:123", "export")
```

### Composite Limiting

Apply multiple rate limits simultaneously.

```go
import "github.com/abhipray-cpu/niyantrak/limiters/composite"

limiter, err := composite.NewCompositeLimiter(
    []limiters.BasicLimiter{perMinuteLimiter, perHourLimiter, perDayLimiter},
    limiters.CompositeConfig{
        RequireAll: true, // All limits must pass
    },
)

// Checks 100/min AND 5000/hr AND 50000/day
result := limiter.Allow(ctx, "user:123")
```

---

## HTTP Middleware

### Standard net/http

```go
import (
    "net/http"
    "github.com/abhipray-cpu/niyantrak/middleware"
)

limiter, _ := niyantrak.New(/* ... */)

mw := middleware.HTTPMiddleware(limiter, middleware.HTTPOptions{
    KeyExtractor: func(r *http.Request) string {
        return r.Header.Get("X-API-Key")
    },
})

mux := http.NewServeMux()
mux.HandleFunc("/api/users", mw(handleGetUsers))
http.ListenAndServe(":8080", mux)
```

### Gin

```go
import "github.com/abhipray-cpu/niyantrak/middleware/adapters"

router := gin.Default()
router.Use(adapters.GinMiddleware(limiter, adapters.GinOptions{
    KeyExtractor: func(c *gin.Context) string {
        return c.GetHeader("X-API-Key")
    },
}))
```

### Chi

```go
r := chi.NewRouter()
r.Use(adapters.ChiMiddleware(limiter, adapters.ChiOptions{
    KeyExtractor: func(r *http.Request) string {
        return r.RemoteAddr
    },
}))
```

### Echo

```go
e := echo.New()
e.Use(adapters.EchoMiddleware(limiter, adapters.EchoOptions{
    KeyExtractor: func(c echo.Context) string {
        return c.RealIP()
    },
}))
```

### Fiber

```go
app := fiber.New()
app.Use(adapters.FiberMiddleware(limiter, adapters.FiberOptions{
    KeyExtractor: func(c *fiber.Ctx) string {
        return c.IP()
    },
}))
```

---

## gRPC Middleware

```go
import (
    "google.golang.org/grpc"
    "google.golang.org/grpc/metadata"
    "github.com/abhipray-cpu/niyantrak/middleware"
)

limiter, _ := niyantrak.New(/* ... */)

server := grpc.NewServer(
    grpc.UnaryInterceptor(middleware.UnaryServerInterceptor(
        limiter,
        middleware.GRPCOptions{
            KeyExtractor: func(ctx context.Context) string {
                md, _ := metadata.FromIncomingContext(ctx)
                keys := md.Get("x-api-key")
                if len(keys) > 0 {
                    return keys[0]
                }
                return "unknown"
            },
        },
    )),
)
```

---

## Observability

### Prometheus Metrics

```go
import "github.com/abhipray-cpu/niyantrak/observability/metrics"

m := metrics.NewPrometheusMetrics(metrics.PrometheusConfig{
    Namespace: "myapp",
    Subsystem: "ratelimit",
})

// Pass to limiter via ObservabilityConfig
```

Exposed metrics:

| Metric | Type | Labels |
|--------|------|--------|
| `niyantrak_requests_total` | Counter | `result` (allowed/denied), `algorithm`, `key` |
| `niyantrak_request_duration_seconds` | Histogram | `operation` |
| `niyantrak_errors_total` | Counter | `algorithm`, `error_type` |

### Structured Logging

```go
import "github.com/abhipray-cpu/niyantrak/observability/logging"

logger := logging.NewLogger(logging.Config{
    Level:  "info",
    Format: "json",
})
```

### Distributed Tracing

```go
import "github.com/abhipray-cpu/niyantrak/observability/tracing"

tracer := tracing.NewOTelTracer(tracing.Config{
    ServiceName: "my-api",
    Endpoint:    "localhost:4317",
})
```

---

## LimitResult

Every `Allow()` / `AllowN()` call returns a `LimitResult`:

```go
type LimitResult struct {
    Allowed    bool          // Whether the request is allowed
    Remaining  int           // Requests remaining in current window
    Limit      int           // Maximum requests per window
    ResetAt    time.Time     // When the window resets
    RetryAfter time.Duration // How long to wait before retrying
    Error      error         // Non-nil if an error occurred
}
```

Usage pattern:

```go
result := limiter.Allow(ctx, "user:123")
if result.Error != nil {
    // Backend or internal error - decide how to handle
    log.Printf("rate limit error: %v", result.Error)
}
if !result.Allowed {
    // Deny the request
    w.Header().Set("Retry-After", fmt.Sprintf("%d", int(result.RetryAfter.Seconds())))
    http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
    return
}
// Proceed with the request
```

---

## Direct Constructor API

For full control, bypass the builder and construct components directly:

```go
import (
    "github.com/abhipray-cpu/niyantrak/algorithm"
    "github.com/abhipray-cpu/niyantrak/backend/memory"
    "github.com/abhipray-cpu/niyantrak/limiters"
    "github.com/abhipray-cpu/niyantrak/limiters/basic"
)

be := memory.NewMemoryBackend()
defer be.Close()

algo := algorithm.NewTokenBucket(algorithm.TokenBucketConfig{
    Capacity:     100,
    RefillRate:   10,
    RefillPeriod: time.Second,
})

limiter, err := basic.NewBasicLimiter(algo, be, limiters.BasicConfig{
    DefaultLimit:  100,
    DefaultWindow: time.Second,
    KeyTTL:        time.Hour,
})
if err != nil {
    log.Fatal(err)
}
defer limiter.Close()

result := limiter.Allow(ctx, "user:123")
```

---

## Examples

The `examples/` directory contains 6 runnable examples:

| Example | Description |
|---------|-------------|
| `01_basic_memory` | Simple in-memory rate limiting |
| `02_basic_redis` | Distributed rate limiting with Redis |
| `03_tier_based` | Different limits per subscription tier |
| `04_tenant_based` | Multi-tenant rate limiting |
| `05_cost_based` | Variable cost per operation |
| `06_composite` | Multiple rate limits combined |

Run all examples:

```bash
make run-examples
```

---

## Development

```bash
# Display all Makefile targets
make help

# Run tests
make test

# Run tests with race detector
make test-race

# Generate coverage report
make coverage

# Run linters
make lint

# Full CI pipeline
make ci-check
```
