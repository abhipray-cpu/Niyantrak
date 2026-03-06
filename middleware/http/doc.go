// Package http provides net/http middleware that enforces rate limits and sets
// standard response headers (X-RateLimit-Limit, X-RateLimit-Remaining,
// X-RateLimit-Reset, Retry-After).
//
// Denied requests receive a 429 Too Many Requests status.
package http
