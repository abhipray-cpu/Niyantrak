// Package adapters provides rate-limiting middleware for popular Go HTTP
// frameworks: Gin, Chi, Echo, and Fiber.
//
// Each adapter wraps a [github.com/abhipray-cpu/niyantrak/limiters.Limiter]
// and returns the framework's native handler/middleware type.
package adapters
