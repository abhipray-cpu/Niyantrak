# gRPC Middleware

The gRPC middleware package provides rate limiting capabilities for gRPC services. It implements the `middleware.GRPCMiddleware` interface with support for both unary and streaming RPCs.

## Features

- ✅ **Standard Library + gRPC** - Minimal dependencies
- ✅ **Interface Compliant** - Strictly follows `middleware.GRPCMiddleware` interface
- ✅ **Unary & Stream Support** - Both RPC types covered
- ✅ **Flexible Key Extraction** - Extract from metadata, headers, context
- ✅ **Method Skipping** - Skip rate limiting for specific methods (e.g., health checks)
- ✅ **Metadata Injection** - Include rate limit info in response metadata
- ✅ **Custom Status Codes** - Configure gRPC error codes
- ✅ **Thread-Safe** - No race conditions
- ✅ **Ultra-High Performance** - ~5ns per operation, zero allocations

## Installation

```go
import (
    grpcmiddleware "github.com/abhipray-cpu/niyantrak/middleware/grpc"
    "github.com/abhipray-cpu/niyantrak/limiters"
    "google.golang.org/grpc"
)
```

## Quick Start

### Basic Unary Interceptor

```go
package main

import (
    "context"
    "log"
    "net"
    "time"

    grpcmiddleware "github.com/abhipray-cpu/niyantrak/middleware/grpc"
    "github.com/abhipray-cpu/niyantrak/limiters"
    "github.com/abhipray-cpu/niyantrak/middleware"
    "google.golang.org/grpc"
)

func main() {
    // Create a basic limiter (100 requests per minute)
    limiter := limiters.NewBasicLimiter(100, time.Minute, nil)
    defer limiter.Close()

    // Create gRPC middleware
    mw := grpcmiddleware.New()

    // Configure options
    options := &middleware.GRPCOptions{
        Extractor: func(ctx context.Context) (string, error) {
            md, _ := metadata.FromIncomingContext(ctx)
            vals := md.Get("x-api-key")
            if len(vals) > 0 {
                return vals[0], nil
            }
            return "", errors.New("missing x-api-key")
        },
        EnableMetadata: true,
    }

    // Get the unary interceptor
    interceptor := mw.UnaryInterceptor(limiter, options)
    unaryInterceptor := interceptor.(grpc.UnaryServerInterceptor)

    // Create gRPC server with rate limiting
    server := grpc.NewServer(
        grpc.UnaryInterceptor(unaryInterceptor),
    )

    // Register your services...
    // pb.RegisterYourServiceServer(server, &yourService{})

    lis, err := net.Listen("tcp", ":50051")
    if err != nil {
        log.Fatal(err)
    }

    log.Println("gRPC server listening on :50051")
    server.Serve(lis)
}
```

### Stream Interceptor

```go
// Get stream interceptor
streamInterceptor := mw.StreamInterceptor(limiter, options)
streamInterceptorFunc := streamInterceptor.(grpc.StreamServerInterceptor)

// Create server with both interceptors
server := grpc.NewServer(
    grpc.UnaryInterceptor(unaryInterceptor),
    grpc.StreamServerInterceptor(streamInterceptorFunc),
)
```

## Configuration Options

### GRPCOptions

```go
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
    // Options: "RESOURCE_EXHAUSTED", "UNAVAILABLE", "PERMISSION_DENIED", etc.
    CustomStatusCode string
}
```

## Key Extraction Strategies

### 1. Default Key Extractor

The default extractor tries multiple metadata keys:

```go
mw := grpcmiddleware.New()
extractor := mw.GetKeyExtractor()
// Tries: x-api-key → authorization → error
```

### 2. Custom Metadata Key

```go
options := &middleware.GRPCOptions{
    Extractor: func(ctx context.Context) (string, error) {
        md, ok := metadata.FromIncomingContext(ctx)
        if !ok {
            return "", errors.New("no metadata")
        }
        
        vals := md.Get("client-id")
        if len(vals) > 0 {
            return vals[0], nil
        }
        return "", errors.New("missing client-id")
    },
}
```

### 3. Extract from Authorization Token

```go
options := &middleware.GRPCOptions{
    Extractor: func(ctx context.Context) (string, error) {
        md, ok := metadata.FromIncomingContext(ctx)
        if !ok {
            return "", errors.New("no metadata")
        }
        
        vals := md.Get("authorization")
        if len(vals) > 0 {
            // Extract user ID from JWT or other token
            token := vals[0]
            userID := extractUserIDFromToken(token)
            return userID, nil
        }
        return "", errors.New("missing authorization")
    },
}
```

### 4. Extract from Peer Information

```go
import "google.golang.org/grpc/peer"

options := &middleware.GRPCOptions{
    Extractor: func(ctx context.Context) (string, error) {
        p, ok := peer.FromContext(ctx)
        if !ok {
            return "", errors.New("no peer info")
        }
        
        // Use IP address as key
        return p.Addr.String(), nil
    },
}
```

## Method Skipping

Skip rate limiting for specific methods like health checks:

```go
options := &middleware.GRPCOptions{
    SkipMethods: []string{
        "/grpc.health.v1.Health/Check",
        "/grpc.health.v1.Health/Watch",
        "/myservice.MyService/Healthz",
    },
    Extractor: keyExtractor,
}
```

## Metadata Injection

Include rate limit information in response metadata:

```go
options := &middleware.GRPCOptions{
    EnableMetadata: true,
    MetadataKey:    "x-ratelimit",
    Extractor:      keyExtractor,
}
```

Clients can read the metadata:

```go
// Client-side
var header metadata.MD
_, err := client.MyMethod(ctx, req, grpc.Header(&header))

// Read rate limit info
if vals := header.Get("x-ratelimit-limit"); len(vals) > 0 {
    fmt.Println("Rate limit:", vals[0])
}
if vals := header.Get("x-ratelimit-remaining"); len(vals) > 0 {
    fmt.Println("Remaining:", vals[0])
}
```

## Custom Status Codes

Configure custom gRPC status code for rate limit exceeded:

```go
options := &middleware.GRPCOptions{
    CustomStatusCode: "UNAVAILABLE",  // Instead of default RESOURCE_EXHAUSTED
    Extractor:        keyExtractor,
}
```

Available status codes:
- `RESOURCE_EXHAUSTED` (default) - Recommended for rate limiting
- `UNAVAILABLE` - Service temporarily unavailable
- `PERMISSION_DENIED` - Client doesn't have permission
- `UNAUTHENTICATED` - Request not authenticated
- `DEADLINE_EXCEEDED` - Operation took too long

## Complete Example with Both Interceptors

```go
package main

import (
    "context"
    "errors"
    "log"
    "net"
    "time"

    grpcmiddleware "github.com/abhipray-cpu/niyantrak/middleware/grpc"
    "github.com/abhipray-cpu/niyantrak/limiters"
    "github.com/abhipray-cpu/niyantrak/middleware"
    "google.golang.org/grpc"
    "google.golang.org/grpc/metadata"
)

func main() {
    // Create limiter (1000 requests per minute)
    limiter := limiters.NewBasicLimiter(1000, time.Minute, nil)
    defer limiter.Close()

    // Create middleware
    mw := grpcmiddleware.New()

    // Configure options
    options := &middleware.GRPCOptions{
        // Extract API key from metadata
        Extractor: func(ctx context.Context) (string, error) {
            md, ok := metadata.FromIncomingContext(ctx)
            if !ok {
                return "", errors.New("no metadata")
            }

            vals := md.Get("x-api-key")
            if len(vals) == 0 {
                return "", errors.New("missing x-api-key")
            }

            return vals[0], nil
        },

        // Skip health checks
        SkipMethods: []string{
            "/grpc.health.v1.Health/Check",
            "/grpc.health.v1.Health/Watch",
        },

        // Include rate limit info in metadata
        EnableMetadata: true,
        MetadataKey:    "x-ratelimit",

        // Use default RESOURCE_EXHAUSTED status
        CustomStatusCode: "",
    }

    // Get interceptors
    unaryInt := mw.UnaryInterceptor(limiter, options).(grpc.UnaryServerInterceptor)
    streamInt := mw.StreamInterceptor(limiter, options).(grpc.StreamServerInterceptor)

    // Create gRPC server
    server := grpc.NewServer(
        grpc.ChainUnaryInterceptor(
            unaryInt,
            // Add other interceptors here (logging, auth, etc.)
        ),
        grpc.ChainStreamInterceptor(
            streamInt,
            // Add other stream interceptors here
        ),
    )

    // Register services
    // pb.RegisterMyServiceServer(server, &myServiceImpl{})

    // Start server
    lis, err := net.Listen("tcp", ":50051")
    if err != nil {
        log.Fatalf("Failed to listen: %v", err)
    }

    log.Println("gRPC server with rate limiting on :50051")
    if err := server.Serve(lis); err != nil {
        log.Fatalf("Failed to serve: %v", err)
    }
}
```

## Error Handling

When rate limit is exceeded, clients receive a gRPC error:

```go
// Client-side
resp, err := client.MyMethod(ctx, req)
if err != nil {
    if st, ok := status.FromError(err); ok {
        if st.Code() == codes.ResourceExhausted {
            // Rate limit exceeded
            fmt.Println("Rate limited:", st.Message())
            // Message includes retry info:
            // "rate limit exceeded: 100/100 requests, retry after 60 seconds"
        }
    }
}
```

## Testing Example

```go
func TestGRPCRateLimiting(t *testing.T) {
    limiter := limiters.NewBasicLimiter(10, time.Minute, nil)
    defer limiter.Close()

    mw := grpcmiddleware.New()

    options := &middleware.GRPCOptions{
        Extractor: func(ctx context.Context) (string, error) {
            return "test-key", nil
        },
    }

    interceptor := mw.UnaryInterceptor(limiter, options)
    unaryInterceptor := interceptor.(grpc.UnaryServerInterceptor)

    handler := func(ctx context.Context, req interface{}) (interface{}, error) {
        return "response", nil
    }

    info := &grpc.UnaryServerInfo{
        FullMethod: "/test.Service/Method",
    }

    // Make 10 allowed requests
    for i := 0; i < 10; i++ {
        _, err := unaryInterceptor(context.Background(), "req", info, handler)
        if err != nil {
            t.Errorf("Request %d failed: %v", i+1, err)
        }
    }

    // 11th request should be rate limited
    _, err := unaryInterceptor(context.Background(), "req", info, handler)
    if err == nil {
        t.Error("Expected rate limit error")
    }

    st, ok := status.FromError(err)
    if !ok || st.Code() != codes.ResourceExhausted {
        t.Error("Expected ResourceExhausted status")
    }
}
```

## Performance

Benchmark results on Apple M4:

```
BenchmarkGRPCMiddleware_UnaryInterceptor-10     229865846    5.235 ns/op    0 B/op    0 allocs/op
BenchmarkGRPCMiddleware_StreamInterceptor-10    211840999    5.667 ns/op    0 B/op    0 allocs/op
```

- **~5 ns/op**: Ultra-fast processing
- **0 B/op**: Zero memory allocation
- **0 allocs/op**: No GC pressure

This makes it suitable for high-throughput gRPC services.

## Thread Safety

The middleware is thread-safe and has been tested with the race detector:

```bash
go test ./middleware/grpc/... -race
```

## Coverage

Current test coverage: **59.1%**

```bash
go test ./middleware/grpc/... -cover
```

## Integration with Popular gRPC Patterns

### With Chain Interceptors

```go
import "github.com/grpc-ecosystem/go-grpc-middleware"

server := grpc.NewServer(
    grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(
        rateLimitInterceptor,
        loggingInterceptor,
        authInterceptor,
        tracingInterceptor,
    )),
)
```

### With Health Checks

```go
import "google.golang.org/grpc/health"
import healthpb "google.golang.org/grpc/health/grpc_health_v1"

// Skip health check endpoints
options := &middleware.GRPCOptions{
    SkipMethods: []string{
        "/grpc.health.v1.Health/Check",
        "/grpc.health.v1.Health/Watch",
    },
}

// Register health service
healthServer := health.NewServer()
healthpb.RegisterHealthServer(server, healthServer)
```

### Per-Service Rate Limiting

```go
// Different limits for different services
userLimiter := limiters.NewBasicLimiter(100, time.Minute, nil)
adminLimiter := limiters.NewBasicLimiter(1000, time.Minute, nil)

options := &middleware.GRPCOptions{
    Extractor: func(ctx context.Context) (string, error) {
        // Extract service name from context
        method, _ := grpc.Method(ctx)
        
        // Choose limiter based on service
        if strings.HasPrefix(method, "/admin") {
            // Higher limit for admin
            return "admin:" + extractKey(ctx), nil
        }
        return "user:" + extractKey(ctx), nil
    },
}
```

## API Reference

### Functions

#### `New() middleware.GRPCMiddleware`

Creates a new gRPC middleware instance.

### Methods

#### `UnaryInterceptor(limiter interface{}, options *GRPCOptions) interface{}`

Returns a gRPC unary server interceptor.

**Parameters:**
- `limiter`: A rate limiter implementing the Allow method
- `options`: Configuration options

**Returns:** `grpc.UnaryServerInterceptor` (as interface{})

#### `StreamInterceptor(limiter interface{}, options *GRPCOptions) interface{}`

Returns a gRPC stream server interceptor.

**Parameters:**
- `limiter`: A rate limiter implementing the Allow method
- `options`: Configuration options

**Returns:** `grpc.StreamServerInterceptor` (as interface{})

#### `GetKeyExtractor() GRPCKeyExtractor`

Returns the default key extractor function.

**Returns:** Default GRPCKeyExtractor function

## Best Practices

1. **Always skip health checks** - Don't rate limit health check endpoints
2. **Use metadata for keys** - Store API keys or tokens in gRPC metadata
3. **Enable metadata injection** - Help clients implement backoff strategies
4. **Chain interceptors properly** - Rate limiting should come before expensive operations
5. **Use appropriate status codes** - `RESOURCE_EXHAUSTED` is recommended for rate limiting
6. **Implement client-side backoff** - Respect retry-after information
7. **Monitor rate limit metrics** - Track exceeded limits and adjust as needed

## License

This package is part of the Niyantrak rate limiting library.
