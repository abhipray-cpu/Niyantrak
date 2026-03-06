# <img src="icon.png" width="150" height="150" alt="Niyantrak" /> Niyantrak - Go Rate Limiter Library

[![GoDoc](https://pkg.go.dev/badge/github.com/abhipray-cpu/niyantrak)](https://pkg.go.dev/github.com/abhipray-cpu/niyantrak)
[![Go Report Card](https://goreportcard.com/badge/github.com/abhipray-cpu/niyantrak)](https://goreportcard.com/report/github.com/abhipray-cpu/niyantrak)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/Go-1.21+-blue)](https://golang.org/)

**Niyantrak** (Sanskrit: नियंत्रक - "Controller") is a flexible rate limiting library for Go. It provides multiple algorithms and backends to suit different use cases, from simple single-server deployments to complex distributed systems.

## Features

- **5 Rate Limiting Algorithms**: Token Bucket, Leaky Bucket, Fixed Window, Sliding Window, GCRA
- **Multiple Backends**: In-memory, Redis, PostgreSQL, and custom backends
- **HTTP & gRPC Middleware**: Standard rate limit headers support
- **Observability**: Prometheus metrics and structured logging
- **Flexible Configuration**: Support for tier-based, tenant-based, and cost-based limiting
- **Thread-Safe**: Safe for concurrent use with proper resource management

## Quick Start

### Installation

```bash
go get github.com/abhipray-cpu/niyantrak
```

### Basic Usage

```go
package main

import (
    "context"
    "log"
    "time"
    "github.com/abhipray-cpu/niyantrak"
)

func main() {
    // Create a token bucket limiter (100 requests per minute)
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
    
    // Check if request is allowed
    result := limiter.Allow(ctx, "user:123")
    if result.Error != nil {
        log.Printf("Error checking rate limit: %v", result.Error)
        return
    }

    if !result.Allowed {
        log.Printf("Rate limited! Retry after %v", result.RetryAfter)
        return
    }

    log.Printf("Request allowed. Remaining: %d/%d", result.Remaining, result.Limit)
}
```

## Documentation & Examples

To help you get started, the library includes:

- **6 Working Examples**: Demonstrating different rate limiting strategies
- **Configuration Reference**: Complete `config.yaml` with all options
- **Architecture Guide**: Understanding the design and components
- **Makefile Utilities**: 64 helpers for development and testing

### Quick Commands

```bash
# View available make targets
make help

# Setup your development environment
make setup-workspace

# Run all examples
make run-examples

# Run tests and validation
make ci-check
```

### Available Examples

- `01_basic_memory` - Simple in-memory rate limiting
- `02_basic_redis` - Distributed rate limiting with Redis
- `03_tier_based` - Different limits for different tiers
- `04_tenant_based` - Multi-tenant rate limiting
- `05_cost_based` - Variable cost per operation
- `06_composite` - Multiple limiting layers combined

See [examples/](examples/) for detailed walkthroughs and `make run-examples` to try them all.

## Testing & Validation

The library includes working examples and comprehensive tests:

| Component | Status | Details |
|-----------|--------|---------|
| **Examples** | ✅ 6 included | Runnable demonstrations |
| **Tests** | ✅ Included | Unit and integration tests |
| **Configuration** | ✅ Reference | Complete YAML examples |
| **Documentation** | ✅ Included | Multiple guides and docs |
| **Error Handling** | ✅ Covered | Proper error propagation |

### HTTP Middleware

```go
import (
    "net/http"
    "github.com/abhipray-cpu/niyantrak"
    "github.com/abhipray-cpu/niyantrak/middleware"
)

func main() {
    limiter, _ := niyantrak.New(...)
    
    // Create HTTP middleware
    httpMiddleware := middleware.HTTPMiddleware(
        limiter,
        middleware.WithKeyExtractor(func(r *http.Request) string {
            return r.Header.Get("X-API-Key")
        }),
    )

    // Use with your router
    mux := http.NewServeMux()
    mux.HandleFunc("/api/users", httpMiddleware(handleGetUsers))
    
    http.ListenAndServe(":8080", mux)
}
```

### gRPC Middleware

```go
import (
    "google.golang.org/grpc"
    "github.com/abhipray-cpu/niyantrak/middleware"
)

func main() {
    limiter, _ := niyantrak.New(...)
    
    // Create gRPC server with rate limiting
    server := grpc.NewServer(
        grpc.UnaryInterceptor(middleware.UnaryServerInterceptor(
            limiter,
            middleware.WithKeyExtractorGRPC(func(ctx context.Context) string {
                md, _ := metadata.FromIncomingContext(ctx)
                keys := md.Get("x-api-key")
                if len(keys) > 0 {
                    return keys[0]
                }
                return "unknown"
            }),
        )),
    )
    // Register services...
}
```

## Core Concepts

### Algorithms

#### Token Bucket
Best for: General-purpose rate limiting with burst allowance
- Allows burst traffic while maintaining average rate
- Configurable capacity and refill rate
- Ideal for APIs with unpredictable traffic patterns

#### Leaky Bucket
Best for: Smooth, constant-rate traffic
- Processes requests at a fixed rate
- Excellent for protecting downstream services
- Minimizes traffic spikes

#### Fixed Window
Best for: Simple, quota-based limiting
- Resets at fixed intervals (e.g., hourly, daily)
- Minimal memory overhead
- Best for dashboards and analytics

#### Sliding Window
Best for: Accurate rate limiting without gaps
- Most accurate algorithm (missing in competitors!)
- No "burst at window boundary" issue
- Slightly higher memory usage than fixed window

#### GCRA (Generic Cell Rate Algorithm)
Best for: Precise, specification-compliant rate limiting
- Used in telecom industry (ITU-T)
- Highly predictable behavior
- Ideal for SLA compliance

### Backends

#### In-Memory Backend
- **Use**: Single instance, development, high performance
- **Performance**: < 500ns p99 latency
- **Trade-off**: No persistence, not distributed

#### Redis Backend
- **Use**: Distributed systems, multi-instance setup
- **Performance**: < 5ms p99 latency
- **Features**: Automatic TTL, atomic operations via Lua

#### PostgreSQL Backend
- **Use**: Persistent limits, audit trail, complex queries
- **Performance**: < 10ms p99 latency
- **Features**: Full ACID transactions, advanced queries

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│                      Niyantrak API                       │
├──────────────────────────────────────────────────────────┤
│                    Limiter Interface                     │
├──────────────────────────────────────────────────────────┤
│  Algorithm Layer    │        Backend Layer               │
├─────────────────────┼──────────────────────┤
│ • Token Bucket      │ • In-Memory          │
│ • Leaky Bucket      │ • Redis              │
│ • Fixed Window      │ • PostgreSQL         │
│ • Sliding Window    │ • Custom Backend     │
│ • GCRA              │                      │
├─────────────────────┼──────────────────────┤
│              Middleware & Observability    │
├──────────────────────────────────────────────────────────┤
│ • HTTP Middleware   • Prometheus Metrics                 │
│ • gRPC Middleware   • Structured Logging                 │
└──────────────────────────────────────────────────────────┘
```

## Performance

Benchmark results (Apple M4, memory backend):

```
Algorithm           Backend      ns/op      Notes
────────────────────────────────────────────────────────
Token Bucket        Memory       ~340       Burst + average rate
Sliding Window      Memory       ~342       O(1) accurate
GCRA                Memory       ~332       Telecom-grade
Parallel (10 cores) Memory       ~477       Token Bucket
Multi-Key (1000)    Memory       ~504       Token Bucket
```

All operations are O(1) in time and space per key.

## Development

### Prerequisites
- Go 1.21 or higher
- Make (for convenience commands)
- Docker (optional, for Redis/PostgreSQL integration tests)

### Build & Test

```bash
# Display all available commands
make help

# Run tests
make test

# Run tests with race detector (recommended)
make test-race

# Generate coverage report
make coverage

# Run all linters
make lint

# Full CI checks
make ci-check

# Full quality assurance
make all
```

### Project Structure

```
niyantrak/
├── algorithm/          # Rate limiting algorithms (5 algorithms + clock injection)
├── backend/            # Storage backends (memory, redis, postgresql, custom)
├── limiters/           # Limiter types (basic, tier, tenant, cost, composite)
├── middleware/          # HTTP, gRPC middleware + framework adapters
├── observability/      # Logging, metrics, tracing interfaces + adapters
├── features/           # Dynamic limits, failover
├── examples/           # 6 runnable usage examples
├── integration/        # Integration tests (Redis, PostgreSQL, Cluster)
├── docs/               # Architecture & usage documentation
├── mocks/              # Generated test mocks
├── Makefile            # Build automation (64 targets)
├── builder.go          # Builder API (functional options)
├── niyantrak.go        # Public API surface
└── go.mod              # Module definition
```

## Documentation

- [Architecture](docs/architecture.md) - C4 diagrams, sequence diagrams, and design decisions
- [Usage Guide](docs/usage.md) - Installation, configuration, all algorithms, backends, and middleware
- [Examples](examples/) - 6 runnable code examples
- [Changelog](CHANGELOG.md) - Release history
- [Security](SECURITY.md) - Vulnerability reporting policy

## Security

See [SECURITY.md](SECURITY.md) for our vulnerability disclosure policy.

- **Thread-safe by default** — All operations are goroutine-safe
- **SQL injection protection** — PostgreSQL table name prefix validated at construction
- **Minimal dependency surface** — Core interfaces have zero external dependencies
- **CI security scanning** — gosec runs on every push

## Contributing

Contributions are welcome! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for details.

1. Fork the repository
2. Create a feature branch
3. Add tests for new functionality
4. Ensure `make all` passes
5. Submit a pull request

## License

MIT License - see [LICENSE](LICENSE) file for details

## Support

- **Issues**: [GitHub Issues](https://github.com/abhipray-cpu/niyantrak/issues)
- **Discussions**: [GitHub Discussions](https://github.com/abhipray-cpu/niyantrak/discussions)
- **Documentation**: [docs/](docs/)

## Resources

- [Architecture](docs/architecture.md) - C4 model, sequence diagrams, design decisions
- [Usage Guide](docs/usage.md) - Complete configuration and integration reference

## Acknowledgments

This library is inspired by the best practices from:
- golang.org/x/time/rate
- uber-go/ratelimit
- go-redis/redis_rate
- Industry rate limiting patterns and research

---

**Version**: 1.0.0
**Last Updated**: 2026-03-07
**License**: MIT

### Quick Links

- [Quick Start](#quick-start)
- [Examples](examples/)
- [Configuration](examples/config.yaml)
- [Architecture](docs/architecture.md)
- [Usage Guide](docs/usage.md)
- [Changelog](CHANGELOG.md)