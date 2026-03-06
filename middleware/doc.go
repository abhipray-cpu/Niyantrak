// Package middleware defines the common types and key-extraction helpers shared
// by HTTP and gRPC rate-limiting middleware.
//
// Concrete handlers and interceptors live in sub-packages:
//   - [github.com/abhipray-cpu/niyantrak/middleware/http] — net/http handlers
//   - [github.com/abhipray-cpu/niyantrak/middleware/grpc] — gRPC interceptors
//   - [github.com/abhipray-cpu/niyantrak/middleware/adapters] — Gin, Chi, Echo, Fiber
package middleware
